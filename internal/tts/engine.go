// Package tts wraps llama.cpp's model-agnostic TTS binary (llama-tts) to
// synthesize speech from text, optionally cloning a voice from a short
// reference clip. It shells out rather than binding via ffi because the
// TTS decode pipeline (per-model vocoder, e.g. Qwen3-TTS) isn't yet
// exposed by yzma (the ffi lib used elsewhere in this project's ecosystem
// for whisper.cpp) — revisit native ffi if/when that support lands
// upstream.
//
// The vendored llama-tts build (see build/Taskfile.yml's vendor:llama-tts)
// carries a local patch adding --tts-instruct (natural-language style/
// emotion steering for Qwen3-TTS); upstream llama.cpp doesn't have this
// flag yet, so Synthesize's Instruct option is a no-op against a stock
// build (the binary will error on the unrecognized flag).
//
// Synthesize also transparently caches results on disk (see cache.go) —
// an exact repeat of the same call (same text/lang/instruct/seed/speaker
// reference content, against the same model files) skips the llama-tts
// subprocess entirely.
package tts

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// llama-tts reports progress on stderr in two shapes: a periodic line
// ("frames generated: 42, speed: 3.10 frames/s") and a final summary
// ("generated 42 frames, 12345 bytes of WAV audio (24000 Hz)").
var (
	framesPeriodicRE = regexp.MustCompile(`frames generated: (\d+)`)
	framesFinalRE    = regexp.MustCompile(`generated (\d+) frames`)
)

// DefaultSeed pins llama-tts's RNG (-s) so the default voice — no
// SpeakerFile given — sounds the same call to call instead of a new
// random voice each time. Qwen3-TTS Base has no voice-identity
// conditioning at all without a reference clip: the very first codec
// token is drawn from an unconstrained, temperature-driven distribution,
// so with llama-tts's own default (-1, a fresh random seed per run) the
// RNG draw alone decides who it sounds like — including between chunks
// of the *same* Compose request, since each chunk is its own llama-tts
// invocation. Verified: two runs with a fixed seed produce byte-identical
// wav output; two runs with -1 differ in both content and length.
const DefaultSeed = 42

// effectiveSeed applies the SynthesizeOptions.Seed zero-value convention:
// 0 becomes DefaultSeed, anything else (including -1 for "random") passes
// through unchanged.
func effectiveSeed(seed int) int {
	if seed == 0 {
		return DefaultSeed
	}
	return seed
}

// maxInstructChars bounds --tts-instruct input. Every instruct token gets
// tokenized and prepended as its own embedding row ahead of the actual
// TTS prompt (see the llama.cpp patch in build/Taskfile.yml's
// vendor:llama-tts), with no cap of its own — an unbounded instruct
// string would grow every request's prompt/context cost linearly with no
// benefit past a sentence or two of style description.
const maxInstructChars = 500

// Engine synthesizes speech using a local gguf TTS model (default:
// Qwen3-TTS-12Hz-1.7B-Base-GGUF) via the llama-tts CLI binary.
type Engine struct {
	// BinPath is the path to llama-tts. Defaults to looking it up on
	// PATH if empty.
	BinPath string
	// ModelPath is the path to the TTS gguf backbone model file.
	ModelPath string
	// MmprojPath is the path to the model's mmproj gguf file
	// (Qwen3-TTS ships one alongside the backbone). Required for
	// models that need it; empty omits -mm.
	MmprojPath string
	// CacheDir holds previously synthesized wavs, keyed on everything
	// that affects their content (see cacheKey in cache.go) — an exact
	// repeat request skips the llama-tts subprocess entirely. Set by New
	// to a directory under os.UserCacheDir(); empty disables caching.
	CacheDir string
}

// New creates an Engine. modelPath must point to a gguf TTS backbone
// model file (e.g. Qwen3-TTS-12Hz-1.7B-Base-Q4_K_M.gguf); mmprojPath
// points to the matching mmproj gguf (required by Qwen3-TTS, empty for
// models that don't use one); binPath may be empty to resolve llama-tts
// from PATH.
func New(binPath, modelPath, mmprojPath string) (*Engine, error) {
	if modelPath == "" {
		return nil, fmt.Errorf("model path required")
	}
	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("model not found: %w", err)
	}
	if mmprojPath != "" {
		if _, err := os.Stat(mmprojPath); err != nil {
			return nil, fmt.Errorf("mmproj not found: %w", err)
		}
	}
	if binPath == "" {
		resolved, err := exec.LookPath("llama-tts")
		if err != nil {
			return nil, fmt.Errorf("llama-tts not found on PATH: %w", err)
		}
		binPath = resolved
	}
	var cacheDir string
	if dir, err := os.UserCacheDir(); err == nil {
		cacheDir = filepath.Join(dir, "oido-tts", "synth-cache")
	}
	return &Engine{BinPath: binPath, ModelPath: modelPath, MmprojPath: mmprojPath, CacheDir: cacheDir}, nil
}

// SynthesizeOptions configures one synthesis call.
type SynthesizeOptions struct {
	// Text to speak.
	Text string
	// Lang is the BCP-47-ish language code accepted by --tts-lang
	// (e.g. "en", "zh", "ja", "es"). Empty uses the model default.
	Lang string
	// SpeakerFile is an optional path to a reference wav/mp3 clip for
	// voice cloning. Empty uses the model's default voice.
	SpeakerFile string
	// Instruct is an optional natural-language style/emotion instruction
	// accepted by --tts-instruct (e.g. "speak with a hint of panic").
	// Requires a llama-tts build with instruct support (this project's
	// vendored build; not yet upstream). Empty omits -tts-instruct.
	Instruct string
	// Seed pins llama-tts's RNG (-s). 0 (the Go zero value) uses
	// DefaultSeed, so repeated calls with no SpeakerFile keep sounding
	// like the same speaker; pass -1 explicitly for a fresh random voice
	// each call, or any other value to pin a specific alternate voice.
	Seed int
	// OutWavPath is where the resulting wav file is written.
	OutWavPath string
	// OnProgress, if set, is called from the reading goroutine each time
	// llama-tts reports a new frame count. Frame count only ever
	// increases; there's no known total (generation length depends on
	// the model's own EOS decision), so callers estimate a target
	// themselves if they want a percentage.
	OnProgress func(frames int)
	// Threads caps llama-tts's CPU thread count (-t). Leave 0 to let
	// llama-tts pick its own default; set explicitly when running
	// several chunks concurrently so they don't all claim every core.
	Threads int
}

// Synthesize runs the OuteTTS binary and writes a wav file to
// opts.OutWavPath.
func (e *Engine) Synthesize(ctx context.Context, opts SynthesizeOptions) error {
	if opts.Text == "" {
		return fmt.Errorf("text required")
	}
	if opts.OutWavPath == "" {
		return fmt.Errorf("output path required")
	}
	if len(opts.Instruct) > maxInstructChars {
		return fmt.Errorf("instruct text too long (%d chars, max %d)", len(opts.Instruct), maxInstructChars)
	}
	if err := os.MkdirAll(filepath.Dir(opts.OutWavPath), 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	// Cache lookup. keyErr staying non-nil (CacheDir unset, or a
	// stat/hash failure computing the key) just means "no cache key
	// available this call" — falls through to a normal synthesis, never
	// fails the request. Skipped entirely when caching is off so a large
	// SpeakerFile doesn't get hashed for nothing.
	key, keyErr := "", fmt.Errorf("caching disabled")
	if e.CacheDir != "" {
		key, keyErr = e.cacheKey(opts)
	}
	if keyErr == nil && e.cacheGet(key, opts.OutWavPath) {
		return nil
	}

	args := []string{
		"-m", e.ModelPath,
		"-p", opts.Text,
		"-s", strconv.Itoa(effectiveSeed(opts.Seed)),
		"--output", opts.OutWavPath,
	}
	if e.MmprojPath != "" {
		args = append(args, "-mm", e.MmprojPath)
	}
	if opts.Lang != "" {
		args = append(args, "--tts-lang", opts.Lang)
	}
	if opts.SpeakerFile != "" {
		args = append(args, "--tts-speaker-file", opts.SpeakerFile)
	}
	if opts.Instruct != "" {
		args = append(args, "--tts-instruct", opts.Instruct)
	}
	if opts.Threads > 0 {
		args = append(args, "-t", strconv.Itoa(opts.Threads))
	}

	cmd := exec.CommandContext(ctx, e.BinPath, args...)
	// llama-tts links against sibling libllama*/libggml*.so files; when
	// bundled alongside the binary (not installed system-wide) the
	// dynamic linker needs pointing at that directory explicitly.
	cmd.Env = append(os.Environ(), "LD_LIBRARY_PATH="+filepath.Dir(e.BinPath))

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}
	cmd.Stdout = nil

	var captured strings.Builder
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(io.TeeReader(stderr, &captured))
		for scanner.Scan() {
			line := scanner.Text()
			if opts.OnProgress == nil {
				continue
			}
			if m := framesPeriodicRE.FindStringSubmatch(line); m != nil {
				if n, err := strconv.Atoi(m[1]); err == nil {
					opts.OnProgress(n)
				}
			} else if m := framesFinalRE.FindStringSubmatch(line); m != nil {
				if n, err := strconv.Atoi(m[1]); err == nil {
					opts.OnProgress(n)
				}
			}
		}
	}()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start synthesize: %w", err)
	}
	<-done // drain stderr before Wait, or the pipe can deadlock on a full buffer
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("synthesize: %w: %s", err, captured.String())
	}
	if _, err := os.Stat(opts.OutWavPath); err != nil {
		return fmt.Errorf("expected output wav missing: %w", err)
	}
	if keyErr == nil {
		e.cachePut(key, opts.OutWavPath)
	}
	return nil
}
