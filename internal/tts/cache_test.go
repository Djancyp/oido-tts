package tts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTestModel(t *testing.T, dir string) (binPath, modelPath, mmprojPath string) {
	t.Helper()
	binPath = filepath.Join(dir, "llama-tts")
	modelPath = filepath.Join(dir, "model.gguf")
	mmprojPath = filepath.Join(dir, "mmproj.gguf")
	for _, p := range []string{binPath, modelPath, mmprojPath} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return binPath, modelPath, mmprojPath
}

func TestCacheKey_SameInputsSameKey(t *testing.T) {
	dir := t.TempDir()
	bin, model, mmproj := writeTestModel(t, dir)
	e := &Engine{BinPath: bin, ModelPath: model, MmprojPath: mmproj}

	opts := SynthesizeOptions{Text: "hello", Lang: "en", Instruct: "calm"}
	k1, err := e.cacheKey(opts)
	if err != nil {
		t.Fatal(err)
	}
	k2, err := e.cacheKey(opts)
	if err != nil {
		t.Fatal(err)
	}
	if k1 != k2 {
		t.Errorf("same inputs produced different keys: %q vs %q", k1, k2)
	}
}

func TestCacheKey_DifferentTextDifferentKey(t *testing.T) {
	dir := t.TempDir()
	bin, model, mmproj := writeTestModel(t, dir)
	e := &Engine{BinPath: bin, ModelPath: model, MmprojPath: mmproj}

	k1, _ := e.cacheKey(SynthesizeOptions{Text: "hello"})
	k2, _ := e.cacheKey(SynthesizeOptions{Text: "goodbye"})
	if k1 == k2 {
		t.Error("different text produced the same cache key")
	}
}

func TestCacheKey_DifferentInstructDifferentKey(t *testing.T) {
	dir := t.TempDir()
	bin, model, mmproj := writeTestModel(t, dir)
	e := &Engine{BinPath: bin, ModelPath: model, MmprojPath: mmproj}

	k1, _ := e.cacheKey(SynthesizeOptions{Text: "hello", Instruct: "excited"})
	k2, _ := e.cacheKey(SynthesizeOptions{Text: "hello", Instruct: "calm"})
	k3, _ := e.cacheKey(SynthesizeOptions{Text: "hello"})
	if k1 == k2 || k1 == k3 || k2 == k3 {
		t.Error("different instruct (including empty) should each produce a distinct key")
	}
}

func TestCacheKey_SpeakerFileHashedByContentNotPath(t *testing.T) {
	dir := t.TempDir()
	bin, model, mmproj := writeTestModel(t, dir)
	e := &Engine{BinPath: bin, ModelPath: model, MmprojPath: mmproj}

	// Two different paths, identical content — must produce the same key,
	// since a self-clone chained reference gets a new temp path every run.
	pathA := filepath.Join(dir, "ref-a.wav")
	pathB := filepath.Join(dir, "ref-b.wav")
	os.WriteFile(pathA, []byte("same audio bytes"), 0o644)
	os.WriteFile(pathB, []byte("same audio bytes"), 0o644)

	kA, err := e.cacheKey(SynthesizeOptions{Text: "hello", SpeakerFile: pathA})
	if err != nil {
		t.Fatal(err)
	}
	kB, err := e.cacheKey(SynthesizeOptions{Text: "hello", SpeakerFile: pathB})
	if err != nil {
		t.Fatal(err)
	}
	if kA != kB {
		t.Errorf("identical speaker file content produced different keys: %q vs %q", kA, kB)
	}

	// Different content, same path reused — must produce a different key.
	os.WriteFile(pathA, []byte("different audio bytes"), 0o644)
	kA2, err := e.cacheKey(SynthesizeOptions{Text: "hello", SpeakerFile: pathA})
	if err != nil {
		t.Fatal(err)
	}
	if kA2 == kA {
		t.Error("changed speaker file content produced the same key")
	}
}

func TestCacheKey_ModelChangeDifferentKey(t *testing.T) {
	dir := t.TempDir()
	bin, model, mmproj := writeTestModel(t, dir)
	e := &Engine{BinPath: bin, ModelPath: model, MmprojPath: mmproj}

	k1, _ := e.cacheKey(SynthesizeOptions{Text: "hello"})

	// Simulate swapping in a different model file at the same path (new
	// mtime/size) — must invalidate old entries automatically.
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(model, []byte("different model bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	k2, _ := e.cacheKey(SynthesizeOptions{Text: "hello"})
	if k1 == k2 {
		t.Error("changing the model file produced the same cache key")
	}
}

func TestCacheGetPut_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	bin, model, mmproj := writeTestModel(t, dir)
	e := &Engine{BinPath: bin, ModelPath: model, MmprojPath: mmproj, CacheDir: filepath.Join(dir, "cache")}

	key := "testkey"
	src := filepath.Join(dir, "generated.wav")
	os.WriteFile(src, []byte("fake wav bytes"), 0o644)

	e.cachePut(key, src)

	dst := filepath.Join(dir, "restored.wav")
	if !e.cacheGet(key, dst) {
		t.Fatal("expected cache hit after cachePut")
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "fake wav bytes" {
		t.Errorf("restored content = %q", got)
	}
}

func TestCacheGet_MissReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	e := &Engine{CacheDir: filepath.Join(dir, "cache")}
	if e.cacheGet("nonexistent", filepath.Join(dir, "out.wav")) {
		t.Error("expected miss for a key never written")
	}
}

func TestCacheGetPut_DisabledWhenCacheDirEmpty(t *testing.T) {
	dir := t.TempDir()
	e := &Engine{} // CacheDir left empty
	src := filepath.Join(dir, "generated.wav")
	os.WriteFile(src, []byte("x"), 0o644)

	e.cachePut("key", src) // must not panic or create anything
	if e.cacheGet("key", filepath.Join(dir, "out.wav")) {
		t.Error("expected caching to be a no-op with CacheDir empty")
	}
}

func TestCachePut_LeavesNoTmpFileOnSuccess(t *testing.T) {
	dir := t.TempDir()
	e := &Engine{CacheDir: filepath.Join(dir, "cache")}
	src := filepath.Join(dir, "generated.wav")
	os.WriteFile(src, []byte("audio"), 0o644)

	e.cachePut("key", src)

	entries, err := os.ReadDir(e.CacheDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range entries {
		if strings.HasSuffix(ent.Name(), ".tmp") {
			t.Errorf("leftover tmp file after successful cachePut: %s", ent.Name())
		}
	}
	if _, err := os.Stat(filepath.Join(e.CacheDir, "key.wav")); err != nil {
		t.Errorf("expected key.wav to exist: %v", err)
	}
}

func TestEvictLRU_CleansUpOrphanedTmpFile(t *testing.T) {
	// Simulates a crash between WriteFile(tmp) and the rename: a bare
	// ".tmp" file with no matching final entry should be removed, not
	// counted toward the cache size or left behind forever.
	dir := t.TempDir()
	e := &Engine{CacheDir: dir}
	orphan := filepath.Join(dir, "orphaned.wav.tmp")
	os.WriteFile(orphan, []byte("partial write"), 0o644)

	e.evictLRU()

	if _, err := os.Stat(orphan); err == nil {
		t.Error("orphaned .tmp file should have been cleaned up by evictLRU")
	}
}

func TestEvictLRU_UnderCapRemovesNothing(t *testing.T) {
	dir := t.TempDir()
	e := &Engine{CacheDir: dir}
	orig := maxCacheBytes
	maxCacheBytes = 1024
	defer func() { maxCacheBytes = orig }()

	os.WriteFile(filepath.Join(dir, "a.wav"), []byte("small"), 0o644)
	e.evictLRU()
	if _, err := os.Stat(filepath.Join(dir, "a.wav")); err != nil {
		t.Error("a.wav was evicted despite being under the cap")
	}
}

func TestEvictLRU_RemovesOldestFirstUntilUnderCap(t *testing.T) {
	dir := t.TempDir()
	e := &Engine{CacheDir: dir}
	orig := maxCacheBytes
	maxCacheBytes = 25 // each file below is 10 bytes; cap fits 2 of 3
	defer func() { maxCacheBytes = orig }()

	// oldest -> newest: a, b, c
	for i, n := range []string{"a", "b", "c"} {
		p := filepath.Join(dir, n+".wav")
		if err := os.WriteFile(p, []byte("0123456789"), 0o644); err != nil {
			t.Fatal(err)
		}
		mt := time.Now().Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}

	e.evictLRU()

	if _, err := os.Stat(filepath.Join(dir, "a.wav")); err == nil {
		t.Error("a.wav (oldest) should have been evicted")
	}
	for _, n := range []string{"b", "c"} {
		if _, err := os.Stat(filepath.Join(dir, n+".wav")); err != nil {
			t.Errorf("%s.wav should NOT have been evicted", n)
		}
	}
}
