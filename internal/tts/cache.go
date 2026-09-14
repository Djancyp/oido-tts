package tts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// maxCacheBytes caps the on-disk synth cache; oldest entries (by mtime,
// touched on every hit) are evicted once a write would exceed it. A var,
// not a const, so tests can shrink it instead of staging 500MB of
// fixtures.
var maxCacheBytes int64 = 500 * 1024 * 1024 // 500MB

// cacheKey derives a stable identity for one synthesis call from
// everything that affects its output: text, lang, instruct, seed, the
// engine's own bin/model/mmproj files (path+mtime+size, so rebuilding
// the binary or switching GGUF naturally invalidates old entries), and
// the speaker reference's *content* rather than its path — a self-clone
// chained reference (see appservice.go's runSequentialJobs) is a temp
// file that gets a new path every single run even when its content, and
// therefore the correct cache entry, is identical.
//
// opts.Threads deliberately isn't part of the key: it only controls
// llama-tts's CPU parallelism, not (in principle) its output. No current
// caller varies Threads across otherwise-identical calls, but if one ever
// does, be aware this assumes thread count never changes the sampled
// result (floating-point non-associativity across thread counts could
// theoretically violate that).
func (e *Engine) cacheKey(opts SynthesizeOptions) (string, error) {
	h := sha256.New()
	fmt.Fprintf(h, "text=%s\x00lang=%s\x00instruct=%s\x00seed=%d\x00",
		opts.Text, opts.Lang, opts.Instruct, effectiveSeed(opts.Seed))

	for _, p := range []string{e.BinPath, e.ModelPath, e.MmprojPath} {
		if p == "" {
			continue
		}
		info, err := os.Stat(p)
		if err != nil {
			return "", fmt.Errorf("stat %s: %w", p, err)
		}
		fmt.Fprintf(h, "file=%s\x00mtime=%d\x00size=%d\x00", p, info.ModTime().UnixNano(), info.Size())
	}

	if opts.SpeakerFile != "" {
		f, err := os.Open(opts.SpeakerFile)
		if err != nil {
			return "", fmt.Errorf("open speaker file: %w", err)
		}
		defer f.Close()
		if _, err := io.Copy(h, f); err != nil {
			return "", fmt.Errorf("hash speaker file: %w", err)
		}
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func (e *Engine) cachePath(key string) string {
	return filepath.Join(e.CacheDir, key+".wav")
}

// cacheGet copies a cached wav to dst if present, touching its mtime so
// size-based eviction treats it as recently used. Returns false (never an
// error) on any miss or failure — caching is a pure optimization, never a
// reason to fail synthesis. Note for callers driving a progress bar off
// SynthesizeOptions.OnProgress: a hit never calls it (there's no
// subprocess to report frames from), so a cache-hit job's slice of the
// bar can visually stall until the next job's real progress arrives.
func (e *Engine) cacheGet(key, dst string) bool {
	if e.CacheDir == "" {
		return false
	}
	src := e.cachePath(key)
	data, err := os.ReadFile(src)
	if err != nil {
		return false
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return false
	}
	now := time.Now()
	_ = os.Chtimes(src, now, now)
	return true
}

// cachePut saves a freshly synthesized wav into the cache and evicts the
// least-recently-used entries if that pushes the cache over its size cap.
// Best-effort: failures here don't fail the synthesis that already
// succeeded. Writes via a temp file + rename rather than straight to the
// final path — a crash mid-write (SIGKILL, power loss) must never leave a
// truncated file sitting at the real cache path, since cacheGet has no
// way to tell a corrupt file from a valid one and would serve it as a
// silent "hit": broken audio with no error anywhere.
func (e *Engine) cachePut(key, wavPath string) {
	if e.CacheDir == "" {
		return
	}
	if err := os.MkdirAll(e.CacheDir, 0o755); err != nil {
		return
	}
	data, err := os.ReadFile(wavPath)
	if err != nil {
		return
	}
	final := e.cachePath(key)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, final); err != nil { // atomic within the same filesystem
		os.Remove(tmp)
		return
	}
	e.evictLRU()
}

func (e *Engine) evictLRU() {
	entries, err := os.ReadDir(e.CacheDir)
	if err != nil {
		return
	}

	type cacheFile struct {
		path    string
		size    int64
		modTime time.Time
	}
	var files []cacheFile
	var total int64
	for _, ent := range entries {
		// A leftover .tmp is always a crash mid-write (the rename that
		// would remove it either completes or the whole file including
		// ".tmp" gets deleted on failure) — never a valid, resumable
		// cache entry, so it's cleaned up rather than counted.
		if strings.HasSuffix(ent.Name(), ".tmp") {
			os.Remove(filepath.Join(e.CacheDir, ent.Name()))
			continue
		}
		info, err := ent.Info()
		if err != nil {
			continue
		}
		files = append(files, cacheFile{filepath.Join(e.CacheDir, ent.Name()), info.Size(), info.ModTime()})
		total += info.Size()
	}
	if total <= maxCacheBytes {
		return
	}

	sort.Slice(files, func(i, j int) bool { return files[i].modTime.Before(files[j].modTime) })
	for _, f := range files {
		if total <= maxCacheBytes {
			break
		}
		if err := os.Remove(f.path); err == nil {
			total -= f.size
		}
	}
}
