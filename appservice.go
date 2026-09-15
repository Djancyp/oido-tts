package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"oido-tts/internal/recorder"
	"oido-tts/internal/synth"
	"oido-tts/internal/tts"
	"oido-tts/internal/wav"
)

// App is the bound service exposing text-to-speech synthesis, voice
// recording, and file/folder pickers to the frontend.
type App struct {
	ctx context.Context

	mu     sync.RWMutex
	engine *tts.Engine

	rec *recorder.Recorder

	// synthMu guards cancelSynth, which doubles as a "something is
	// running" flag: Compose and Podcast share this so they can never
	// run concurrently — running two llama-tts pipelines at once is
	// exactly the memory problem the sequential rewrite exists to avoid.
	synthMu     sync.Mutex
	cancelSynth context.CancelFunc
}

// NewApp creates a new App service.
func NewApp() *App {
	return &App{rec: recorder.New()}
}

func (a *App) setEngine(eng *tts.Engine) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.engine = eng
}

func (a *App) getEngine() *tts.Engine {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.engine
}

// ServiceStartup is called by Wails when the service starts, mirroring
// the v2 startup(ctx) hook: it resolves the TTS engine from bundled
// resources or env var overrides.
func (a *App) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	a.ctx = ctx

	binPath, modelPath, mmprojPath := synth.ResolveEnginePaths()
	if modelPath == "" {
		return nil // engine stays nil until Configure is called from the UI
	}
	eng, err := tts.New(binPath, modelPath, mmprojPath)
	if err != nil {
		println("WARN: tts engine not ready:", err.Error())
		return nil
	}
	a.setEngine(eng)
	return nil
}

// Configure (re)initializes the TTS engine with explicit paths, for when
// env vars aren't set and the user picks paths from the UI instead.
func (a *App) Configure(binPath, modelPath, mmprojPath string) error {
	eng, err := tts.New(binPath, modelPath, mmprojPath)
	if err != nil {
		return err
	}
	a.setEngine(eng)
	return nil
}

// SynthesizeResult is returned to the frontend after synthesis. AudioB64
// is the wav file base64-encoded so the webview can play it directly via
// a data: URL without needing filesystem access. SavedPath is set only
// when the caller supplied an outputDir — otherwise nothing is kept on
// disk beyond the call.
type SynthesizeResult struct {
	AudioB64  string `json:"audioB64"`
	SavedPath string `json:"savedPath,omitempty"`
}

// ProgressEvent is emitted on the "tts:progress" event as synthesis
// runs. Percent is a rough heuristic (there's no true total — the model
// decides generation length via its own end-of-speech token), capped
// below 100 until the process actually finishes.
type ProgressEvent struct {
	Percent float64 `json:"percent"`
}

// beginJob claims the shared single-job slot (see App.cancelSynth) and
// returns a context bound to it plus a release func that must be
// deferred. Fails if Compose or Podcast already has a job running —
// they're never allowed to run at the same time (see App.synthMu doc).
func (a *App) beginJob(timeout time.Duration) (context.Context, func(), error) {
	a.synthMu.Lock()
	if a.cancelSynth != nil {
		a.synthMu.Unlock()
		return nil, nil, fmt.Errorf("another synthesis job is already running")
	}
	ctx, cancel := context.WithTimeout(a.ctx, timeout)
	a.cancelSynth = cancel
	a.synthMu.Unlock()

	release := func() {
		a.synthMu.Lock()
		a.cancelSynth = nil
		a.synthMu.Unlock()
		cancel()
	}
	return ctx, release, nil
}

// jobTimeout budgets a synthesis run by how many sequential llama-tts calls
// it needs: each call reloads the full model from scratch (no shared
// server process), which measured ~15-20s even for a short chunk on this
// hardware — a fixed cap sized for a few sentences silently truncates any
// longer text once real chunk count grows (a full article easily hits
// 20-30 chunks). perJobBudget is deliberately generous since a timeout
// mid-run fails the entire request, not just the slow chunk.
func jobTimeout(jobCount int) time.Duration {
	const (
		floor        = 2 * time.Minute
		perJobBudget = 60 * time.Second
	)
	return floor + time.Duration(jobCount)*perJobBudget
}

// Synthesize converts text to speech, optionally cloning a voice from
// speakerFile (a wav/mp3 reference clip). lang is a language code such as
// "en", "zh", "ja" — empty uses the model default. instruct is an optional
// natural-language style/emotion instruction (e.g. "speak with a hint of
// panic") used as the default for any text with no inline "[emotion]" tag
// of its own (see buildComposeJobs); empty omits it entirely for untagged
// text. Requires a llama-tts build with --tts-instruct support (see
// internal/tts.Engine's package doc) — on a stock build this will fail
// the whole request rather than silently ignoring it. When outputDir is
// non-empty, the generated wav is kept there permanently; otherwise it's
// written to a scratch cache dir and removed once read. Progress is
// reported via the "tts:progress" event while generation runs.
func (a *App) Synthesize(text, lang, speakerFile, instruct, outputDir string) (*SynthesizeResult, error) {
	engine := a.getEngine()
	if engine == nil {
		return nil, fmt.Errorf("tts engine not configured: set TTS_MODEL_PATH or call Configure")
	}

	keep := outputDir != ""
	outDir := outputDir
	if !keep {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			cacheDir = os.TempDir()
		}
		outDir = filepath.Join(cacheDir, "oido-tts")
	}
	outPath := filepath.Join(outDir, fmt.Sprintf("oido-tts-%d.wav", time.Now().UnixNano()))
	if !keep {
		defer os.Remove(outPath) // audio is returned as base64; the file on disk is scratch space only
	}

	jobs := synth.BuildComposeJobs(text, speakerFile, instruct)

	ctx, release, err := a.beginJob(jobTimeout(len(jobs)))
	if err != nil {
		return nil, err
	}
	defer release()

	app := application.Get()
	if err := synth.RunSequentialJobs(ctx, engine, jobs, lang, outPath, true, func(percent float64) {
		app.Event.Emit("tts:progress", ProgressEvent{Percent: percent})
	}); err != nil {
		app.Event.Emit("tts:progress", ProgressEvent{Percent: 0})
		if ctx.Err() != nil {
			return nil, fmt.Errorf("stopped")
		}
		return nil, err
	}
	app.Event.Emit("tts:progress", ProgressEvent{Percent: 100})

	wavBytes, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("read generated wav: %w", err)
	}

	result := &SynthesizeResult{AudioB64: base64.StdEncoding.EncodeToString(wavBytes)}
	if keep {
		result.SavedPath = outPath
	}
	return result, nil
}

// BuildPodcast turns a "Speaker: line" script into one stitched episode,
// giving each speaker their own saved voice (voices maps a speaker name,
// as it appears in the script, to a speakerFile path — same kind of path
// PickSpeakerFile/ConfirmRecording return). Unrecognized or unmapped
// speaker names fall back to the model's default voice. A trailing
// "[emotion]" tag on a turn (e.g. "Host: line [excited]") is parsed off by
// tts.ParseScript and sent as that turn's instruct — see the same caveats
// as Synthesize's instruct param (requires a patched llama-tts build,
// effect on the Base checkpoint unverified). Shares the same
// sequential one-model-at-a-time pipeline and progress/stop plumbing as
// Synthesize (see runSequentialJobs, beginJob) — events are emitted on
// "podcast:progress"/"podcast:chunk" instead, so a Podcast tab's
// listeners don't collide with a Compose tab's.
func (a *App) BuildPodcast(script string, voices map[string]string, lang, outputDir string) (*SynthesizeResult, error) {
	engine := a.getEngine()
	if engine == nil {
		return nil, fmt.Errorf("tts engine not configured: set TTS_MODEL_PATH or call Configure")
	}

	turns := tts.ParseScript(script)
	if len(turns) == 0 {
		return nil, fmt.Errorf(`no speaker turns found — use "Speaker: line" per paragraph`)
	}

	keep := outputDir != ""
	outDir := outputDir
	if !keep {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			cacheDir = os.TempDir()
		}
		outDir = filepath.Join(cacheDir, "oido-tts")
	}
	outPath := filepath.Join(outDir, fmt.Sprintf("podcast-%d.wav", time.Now().UnixNano()))
	if !keep {
		defer os.Remove(outPath)
	}

	ctx, release, err := a.beginJob(jobTimeout(len(turns)))
	if err != nil {
		return nil, err
	}
	defer release()

	jobs := make([]synth.Job, len(turns))
	for i, t := range turns {
		jobs[i] = synth.Job{Text: t.Text, SpeakerFile: voices[t.Speaker], Instruct: t.Instruct, GapBeforeMs: t.GapBeforeMs}
	}

	app := application.Get()
	if err := synth.RunSequentialJobs(ctx, engine, jobs, lang, outPath, false, func(percent float64) {
		app.Event.Emit("podcast:progress", ProgressEvent{Percent: percent})
	}); err != nil {
		app.Event.Emit("podcast:progress", ProgressEvent{Percent: 0})
		if ctx.Err() != nil {
			return nil, fmt.Errorf("stopped")
		}
		return nil, err
	}
	app.Event.Emit("podcast:progress", ProgressEvent{Percent: 100})

	wavBytes, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("read generated wav: %w", err)
	}

	result := &SynthesizeResult{AudioB64: base64.StdEncoding.EncodeToString(wavBytes)}
	if keep {
		result.SavedPath = outPath
	}
	return result, nil
}

// StopSynthesis cancels an in-progress Synthesize or BuildPodcast call,
// if any. The currently-running llama-tts process is killed
// (exec.CommandContext ties its lifetime to the context); the sequential
// job loop then sees the cancellation and stops before starting the next
// piece. A no-op if nothing is running.
func (a *App) StopSynthesis() {
	a.synthMu.Lock()
	cancel := a.cancelSynth
	a.synthMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// PickSpeakerFile opens a native file picker for a reference wav/mp3
// clip to clone a voice from, returning the chosen path (empty if
// cancelled).
func (a *App) PickSpeakerFile() (string, error) {
	return application.Get().Dialog.OpenFile().
		SetTitle("Choose a voice reference clip").
		CanChooseFiles(true).
		AddFilter("Audio files", "*.wav;*.mp3").
		PromptForSingleSelection()
}

// PickOutputFolder opens a native folder picker for where generated
// audio should be saved, returning the chosen path (empty if cancelled).
func (a *App) PickOutputFolder() (string, error) {
	return application.Get().Dialog.OpenFile().
		SetTitle("Choose an output folder").
		CanChooseFiles(false).
		CanChooseDirectories(true).
		PromptForSingleSelection()
}

// SaveGeneratedAudio opens a native "Save As" dialog defaulting to
// suggestedName and writes audioB64 (the wav bytes returned by Synthesize/
// BuildPodcast) to the chosen path — for a result that was only played
// back, not auto-saved via outputDir. defaultDir, when non-empty, is the
// directory the dialog opens in (e.g. the sidebar's configured Output
// folder), so a user who already picked one doesn't have to navigate back
// to it every time. Returns the chosen path, or "" if the user cancelled
// the dialog.
func (a *App) SaveGeneratedAudio(audioB64, suggestedName, defaultDir string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(audioB64)
	if err != nil {
		return "", fmt.Errorf("decode audio: %w", err)
	}

	dialog := application.Get().Dialog.SaveFile().
		SetMessage("Save audio").
		SetFilename(suggestedName).
		AddFilter("WAV audio", "*.wav")
	if defaultDir != "" {
		dialog.SetDirectory(defaultDir)
	}

	path, err := dialog.PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write audio file: %w", err)
	}
	return path, nil
}

// StartRecording begins capturing microphone audio via the system's
// arecord for use as a voice-cloning reference. See internal/recorder
// for why this happens natively instead of via the browser's
// getUserMedia (which WebKitGTK disables by default on Linux).
func (a *App) StartRecording() error {
	return a.rec.Start()
}

// StopRecording ends the in-progress recording and returns the raw
// audio base64-encoded so the UI can preview it before committing.
func (a *App) StopRecording() (string, error) {
	data, err := a.rec.Stop()
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// ConfirmRecording "perfects" the last stopped recording — trims
// leading/trailing silence and normalizes volume — and saves it to a
// stable per-user location so it can be reused as a cloning reference
// across requests and future launches, exactly like a file picked via
// PickSpeakerFile.
func (a *App) ConfirmRecording() (string, error) {
	rawPath := a.rec.LastPath()
	if rawPath == "" {
		return "", fmt.Errorf("no recording to confirm")
	}

	pcm, err := wav.Read(rawPath)
	if err != nil {
		return "", fmt.Errorf("read recording: %w", err)
	}
	pcm = wav.Normalize(wav.TrimSilence(pcm, -40), 0.9)

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	dir := filepath.Join(cacheDir, "oido-tts", "voices")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create voices dir: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("recorded-%d.wav", time.Now().UnixNano()))
	if err := wav.Write(path, pcm); err != nil {
		return "", fmt.Errorf("write recording: %w", err)
	}

	a.rec.Discard()
	return path, nil
}

// DiscardRecording throws away the last stopped recording (the user hit
// "record again" or cancelled).
func (a *App) DiscardRecording() {
	a.rec.Discard()
}
