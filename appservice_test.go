package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resolveBundledPaths resolves paths relative to os.Executable(), which
// in `go test` is the compiled test binary — not something we control.
// So this test exercises the model/mmproj split logic directly by
// duplicating the glob+classify step against a temp dir, since that's
// the only non-trivial branch in resolveBundledPaths worth covering.
func TestClassifyModelFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"Qwen3-TTS-12Hz-1.7B-Base-Q4_K_M.gguf",
		"mmproj-Qwen3-TTS-12Hz-1.7B-Base-Q8_0.gguf",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	modelPath, mmprojPath := classifyModelFiles(dir)

	if filepath.Base(modelPath) != "Qwen3-TTS-12Hz-1.7B-Base-Q4_K_M.gguf" {
		t.Errorf("modelPath = %q, want backbone gguf", modelPath)
	}
	if filepath.Base(mmprojPath) != "mmproj-Qwen3-TTS-12Hz-1.7B-Base-Q8_0.gguf" {
		t.Errorf("mmprojPath = %q, want mmproj gguf", mmprojPath)
	}
}

func TestBuildComposeJobs_NoTagsUsesSidebarInstructThroughout(t *testing.T) {
	jobs := buildComposeJobs("Hello there. How are you today?", "voice.wav", "calm and friendly")
	if len(jobs) == 0 {
		t.Fatal("expected at least one job")
	}
	for _, j := range jobs {
		if j.Instruct != "calm and friendly" {
			t.Errorf("job Instruct = %q, want sidebar default", j.Instruct)
		}
		if j.SpeakerFile != "voice.wav" {
			t.Errorf("job SpeakerFile = %q, want voice.wav", j.SpeakerFile)
		}
	}
}

func TestBuildComposeJobs_InlineTagOverridesSidebarForItsSegment(t *testing.T) {
	text := "Calm opener here. [excited] Huge news, you won't believe it!"
	jobs := buildComposeJobs(text, "", "calm and friendly")

	var sawDefault, sawExcited bool
	for _, j := range jobs {
		switch j.Instruct {
		case "calm and friendly":
			sawDefault = true
		case "excited":
			sawExcited = true
		}
		if strings.ContainsAny(j.Text, "[]") {
			t.Errorf("tag leaked into job text: %q", j.Text)
		}
	}
	if !sawDefault || !sawExcited {
		t.Errorf("jobs = %+v, want both the sidebar default and the inline override represented", jobs)
	}
}

func TestBuildComposeJobs_InlineTagWithNoSidebarDefaultLeavesUntaggedPartEmpty(t *testing.T) {
	jobs := buildComposeJobs("Plain intro. [excited] Loud part!", "", "")
	if len(jobs) == 0 {
		t.Fatal("expected at least one job")
	}
	if jobs[0].Instruct != "" {
		t.Errorf("first job Instruct = %q, want empty (no sidebar default set)", jobs[0].Instruct)
	}
}

func TestClassifyModelFiles_AmbiguousKeepsFirst(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.gguf", "b.gguf"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	modelPath, _ := classifyModelFiles(dir)

	if modelPath == "" {
		t.Fatal("expected one model path to win, got none")
	}
}
