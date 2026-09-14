package tts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSynthesize_RejectsOverlongInstruct(t *testing.T) {
	e := &Engine{BinPath: "llama-tts", ModelPath: "model.gguf"}
	err := e.Synthesize(context.Background(), SynthesizeOptions{
		Text:       "hello",
		Instruct:   strings.Repeat("x", maxInstructChars+1),
		OutWavPath: "/tmp/does-not-matter.wav",
	})
	if err == nil {
		t.Fatal("expected an error for instruct text over the length cap, got nil")
	}
}

func TestEffectiveSeed_ZeroUsesDefault(t *testing.T) {
	if got := effectiveSeed(0); got != DefaultSeed {
		t.Errorf("effectiveSeed(0) = %d, want DefaultSeed (%d)", got, DefaultSeed)
	}
}

func TestEffectiveSeed_ExplicitValuesPassThrough(t *testing.T) {
	for _, seed := range []int{-1, 1, 42, 12345} {
		if got := effectiveSeed(seed); got != seed {
			t.Errorf("effectiveSeed(%d) = %d, want unchanged", seed, got)
		}
	}
}

// writeFakeLlamaTTS creates a stand-in "llama-tts" shell script that
// records one invocation per call (appends a line to counterFile) and
// writes dummy bytes to whatever --output path it's given, without doing
// any real synthesis. Lets tests prove Synthesize's cache-hit path
// actually skips the subprocess, without needing the real (1.4GB) model.
func writeFakeLlamaTTS(t *testing.T, dir, counterFile string) string {
	t.Helper()
	script := `#!/bin/sh
echo invoked >> "` + counterFile + `"
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output" ]; then
    shift
    echo "fake wav data" > "$1"
    break
  fi
  shift
done
exit 0
`
	binPath := filepath.Join(dir, "llama-tts")
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return binPath
}

func TestSynthesize_SecondIdenticalCallSkipsSubprocess(t *testing.T) {
	dir := t.TempDir()
	counterFile := filepath.Join(dir, "invocations.log")
	os.WriteFile(counterFile, nil, 0o644)
	bin := writeFakeLlamaTTS(t, dir, counterFile)

	modelPath := filepath.Join(dir, "model.gguf")
	os.WriteFile(modelPath, []byte("model"), 0o644)

	e := &Engine{BinPath: bin, ModelPath: modelPath, CacheDir: filepath.Join(dir, "cache")}

	opts := SynthesizeOptions{Text: "hello world", Lang: "en", OutWavPath: filepath.Join(dir, "out1.wav")}
	if err := e.Synthesize(context.Background(), opts); err != nil {
		t.Fatalf("first call: %v", err)
	}

	opts.OutWavPath = filepath.Join(dir, "out2.wav")
	if err := e.Synthesize(context.Background(), opts); err != nil {
		t.Fatalf("second call: %v", err)
	}

	log, err := os.ReadFile(counterFile)
	if err != nil {
		t.Fatal(err)
	}
	invocations := len(strings.Split(strings.TrimSpace(string(log)), "\n"))
	if invocations != 1 {
		t.Errorf("fake llama-tts invoked %d times, want 1 (second call should have hit the cache)", invocations)
	}

	out1, _ := os.ReadFile(filepath.Join(dir, "out1.wav"))
	out2, _ := os.ReadFile(filepath.Join(dir, "out2.wav"))
	if string(out1) != string(out2) {
		t.Errorf("cached output differs from original: %q vs %q", out1, out2)
	}
}

func TestSynthesize_AcceptsInstructAtLimit(t *testing.T) {
	e := &Engine{BinPath: "llama-tts", ModelPath: "model.gguf"}
	err := e.Synthesize(context.Background(), SynthesizeOptions{
		Text:       "hello",
		Instruct:   strings.Repeat("x", maxInstructChars),
		OutWavPath: "/tmp/does-not-matter.wav",
	})
	// Rejected later for a different reason (no real llama-tts binary at
	// that path) — the point here is it must NOT be the length-cap error.
	if err != nil && strings.Contains(err.Error(), "too long") {
		t.Fatalf("instruct at exactly the cap was rejected as too long: %v", err)
	}
}
