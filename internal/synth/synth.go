// Package synth holds the engine-resolution and job-running logic shared
// by every frontend to the TTS engine — the Wails desktop app
// (appservice.go) and the MCP server (cmd/oido-tts-mcp) both drive the
// same pipeline through this package, so a fix or behavior change here
// applies to both without duplication.
package synth

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"oido-tts/internal/tts"
	"oido-tts/internal/wav"
)

// ResolveEnginePaths finds the llama-tts binary and model files: first a
// resources/ directory shipped alongside the running executable (a
// packaged app works standalone with no configuration), then LLAMA_TTS_BIN/
// TTS_MODEL_PATH/TTS_MMPROJ_PATH env vars, which override whatever bundled
// paths were found. Returns empty strings for anything neither source
// provides.
func ResolveEnginePaths() (binPath, modelPath, mmprojPath string) {
	binPath, modelPath, mmprojPath = resolveBundledPaths()
	if v := os.Getenv("LLAMA_TTS_BIN"); v != "" {
		binPath = v
	}
	if v := os.Getenv("TTS_MODEL_PATH"); v != "" {
		modelPath = v
	}
	if v := os.Getenv("TTS_MMPROJ_PATH"); v != "" {
		mmprojPath = v
	}
	return binPath, modelPath, mmprojPath
}

// resolveBundledPaths looks for a resources/ directory shipped alongside
// the running executable (resources/bin/llama-tts + friends,
// resources/models/*.gguf). Returns empty strings for anything not found.
func resolveBundledPaths() (binPath, modelPath, mmprojPath string) {
	exe, err := os.Executable()
	if err != nil {
		return "", "", ""
	}
	base := filepath.Join(filepath.Dir(exe), "resources")

	bin := filepath.Join(base, "bin", "llama-tts")
	if _, err := os.Stat(bin); err == nil {
		binPath = bin
	}

	modelPath, mmprojPath = classifyModelFiles(filepath.Join(base, "models"))
	return binPath, modelPath, mmprojPath
}

// classifyModelFiles splits the gguf files in modelsDir into the TTS
// backbone model and its mmproj companion (identified by the "mmproj-"
// filename prefix llama.cpp uses). If more than one file matches either
// role, the first one wins and the rest are logged and ignored — silently
// picking the wrong model is worse than a warning.
func classifyModelFiles(modelsDir string) (modelPath, mmprojPath string) {
	matches, _ := filepath.Glob(filepath.Join(modelsDir, "*.gguf"))
	for _, m := range matches {
		if strings.HasPrefix(filepath.Base(m), "mmproj-") {
			if mmprojPath != "" {
				println("WARN: multiple mmproj gguf files in", modelsDir, "- using", mmprojPath, "ignoring", m)
				continue
			}
			mmprojPath = m
		} else {
			if modelPath != "" {
				println("WARN: multiple backbone gguf files in", modelsDir, "- using", modelPath, "ignoring", m)
				continue
			}
			modelPath = m
		}
	}
	return modelPath, mmprojPath
}

// NewEngineFromEnv resolves engine paths via ResolveEnginePaths and
// constructs a tts.Engine from them. Returns an error naming what's
// missing if no model path was found by either source.
func NewEngineFromEnv() (*tts.Engine, error) {
	binPath, modelPath, mmprojPath := ResolveEnginePaths()
	if modelPath == "" {
		return nil, fmt.Errorf("no TTS model found: bundle resources/models/*.gguf alongside the executable, or set TTS_MODEL_PATH")
	}
	return tts.New(binPath, modelPath, mmprojPath)
}

// Job is one piece to synthesize: some text, which voice to clone (empty
// for the model's default voice), and an optional per-job style/emotion
// instruct (empty omits it).
type Job struct {
	Text        string
	SpeakerFile string
	Instruct    string
}

// BuildComposeJobs splits text into synthesis jobs. text can carry inline
// "[emotion]" tags (see tts.SplitInstructSegments) that override instruct
// for the stretch of text they color — a tag always starts a new job
// group, so two different emotions never end up merged into one
// llama-tts call by tts.ChunkText's normal word-count grouping. Untagged
// text (before the first tag, or all of it if there are none) falls back
// to instruct, the caller-supplied default.
func BuildComposeJobs(text, speakerFile, instruct string) []Job {
	var jobs []Job
	for _, seg := range tts.SplitInstructSegments(text) {
		segInstruct := seg.Instruct
		if segInstruct == "" {
			segInstruct = instruct
		}
		for _, c := range tts.ChunkText(seg.Text) {
			jobs = append(jobs, Job{Text: c, SpeakerFile: speakerFile, Instruct: segInstruct})
		}
	}
	return jobs
}

// estimateFrames ballparks how many 12Hz codec frames Qwen3-TTS will need
// for a given text, from an average speaking rate (~150 words/min, ~5
// chars/word). It's a heuristic for a progress bar, not a real target —
// actual pacing varies with punctuation, emphasis, and language.
func estimateFrames(text string) int {
	const (
		charsPerSecond  = 150.0 * 5.0 / 60.0 // ~12.5
		framesPerSecond = 12.0
	)
	estimated := int(float64(len(text)) / charsPerSecond * framesPerSecond)
	return max(estimated, 8)
}

func capPercent(p float64) float64 {
	if p > 95 {
		return 95 // reserve the last stretch for actual completion
	}
	return p
}

// RunSequentialJobs runs each job through the engine strictly one at a
// time, in order, and concatenates the results into outPath. Used by both
// Compose (tts.ChunkText splits one text into same-voice pieces) and
// Podcast (tts.ParseScript splits a script into per-speaker turns, each
// with its own voice).
//
// Qwen3-TTS has no chunking of its own in llama.cpp (unlike e.g. Pocket
// TTS) — it generates a whole prompt as one autoregressive run, which is
// why long text degrades in quality and starts to sound rushed; splitting
// into pieces sized for what the model was tuned for fixes that. Running
// those pieces is sequential, not concurrent, because each llama-tts call
// loads the full model into memory independently — concurrency multiplies
// that per worker, which is what was locking up the machine on long text.
// Sequential keeps memory to one model's worth at a time, regardless of
// how many jobs there are.
//
// selfCloneDefaultVoice, when true, fixes the voice-drift problem for jobs
// with no SpeakerFile: pinning tts.Engine's RNG seed only guarantees
// identical output for identical text, not a consistent voice across
// *different* text — the actual text tokens condition the same
// autoregressive sequence codec sampling draws from, so different chunks
// of one request can still land on different voices even with the same
// seed. So instead, once the first such job's audio exists, it's reused
// as a real --tts-speaker-file reference for every later default-voice
// job — a genuine voice-clone anchor, not a seed trick, so it holds
// regardless of what the text says. Compose-style single-speaker callers
// pass true (all jobs are meant to be one speaker); Podcast-style callers
// pass false, since with independent default-voice speakers, naively
// chaining off "the first default-voice job" would clone them onto the
// same voice.
//
// onProgress receives 0-100 as generation proceeds.
func RunSequentialJobs(ctx context.Context, engine *tts.Engine, jobs []Job, lang, outPath string, selfCloneDefaultVoice bool, onProgress func(percent float64)) error {
	if len(jobs) == 0 {
		return fmt.Errorf("nothing to synthesize")
	}

	if len(jobs) == 1 {
		estimated := estimateFrames(jobs[0].Text)
		if err := engine.Synthesize(ctx, tts.SynthesizeOptions{
			Text:        jobs[0].Text,
			Lang:        lang,
			SpeakerFile: jobs[0].SpeakerFile,
			Instruct:    jobs[0].Instruct,
			OutWavPath:  outPath,
			OnProgress: func(frames int) {
				onProgress(capPercent(float64(frames) / float64(estimated) * 100))
			},
		}); err != nil {
			return err
		}
		return nil
	}

	jobDir := filepath.Join(filepath.Dir(outPath), fmt.Sprintf("jobs-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		return fmt.Errorf("create job dir: %w", err)
	}
	defer os.RemoveAll(jobDir)

	var totalEstimated int
	for _, j := range jobs {
		totalEstimated += estimateFrames(j.Text)
	}

	clips := make([]*wav.PCM, len(jobs))
	var framesDoneBefore int
	var selfCloneRef string

	for i, j := range jobs {
		jobPath := filepath.Join(jobDir, fmt.Sprintf("job-%03d.wav", i))
		estimated := estimateFrames(j.Text)
		doneBefore := framesDoneBefore

		speakerFile := j.SpeakerFile
		if speakerFile == "" && selfCloneDefaultVoice && selfCloneRef != "" {
			speakerFile = selfCloneRef
		}

		if err := engine.Synthesize(ctx, tts.SynthesizeOptions{
			Text:        j.Text,
			Lang:        lang,
			SpeakerFile: speakerFile,
			Instruct:    j.Instruct,
			OutWavPath:  jobPath,
			OnProgress: func(frames int) {
				onProgress(capPercent(float64(doneBefore+frames) / float64(totalEstimated) * 100))
			},
		}); err != nil {
			return fmt.Errorf("part %d: %w", i, err)
		}
		framesDoneBefore += estimated

		if selfCloneDefaultVoice && j.SpeakerFile == "" && selfCloneRef == "" {
			selfCloneRef = jobPath
		}

		pcm, err := wav.Read(jobPath)
		if err != nil {
			return fmt.Errorf("read part %d: %w", i, err)
		}
		clips[i] = pcm
	}

	if err := wav.Write(outPath, wav.Concat(clips, 200)); err != nil {
		return fmt.Errorf("write final wav: %w", err)
	}
	return nil
}
