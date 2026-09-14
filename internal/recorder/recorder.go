// Package recorder captures microphone audio via the system's `arecord`
// (ALSA) CLI. WebKitGTK (the Linux webview Wails uses) disables
// getUserMedia by default and needs low-level GTK permission wiring
// Wails' Go API doesn't expose, plus GStreamer plugins for device
// enumeration — shelling out to arecord sidesteps all of that.
package recorder

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// MaxSeconds caps a single recording; arecord enforces this itself via
// -d, so a runaway recording can't happen even if Stop is never called.
const MaxSeconds = 30

// Recorder manages one microphone recording at a time via arecord.
type Recorder struct {
	mu   sync.Mutex
	cmd  *exec.Cmd
	path string
}

// New creates a Recorder.
func New() *Recorder {
	return &Recorder{}
}

// Start begins recording mono 16-bit PCM at 24kHz (matching what the TTS
// pipeline expects for a reference clip) to a fresh temp file.
func (r *Recorder) Start() (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cmd != nil {
		return fmt.Errorf("already recording")
	}

	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "oido-tts", "tmp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("rec-%d.wav", time.Now().UnixNano()))

	cmd := exec.Command("arecord",
		"-q",
		"-f", "S16_LE",
		"-c", "1",
		"-r", "24000",
		"-d", fmt.Sprintf("%d", MaxSeconds),
		path,
	)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start arecord: %w", err)
	}

	r.cmd = cmd
	r.path = path
	return nil
}

// Stop signals the in-progress recording to finish (arecord finalizes
// the WAV header on SIGINT) and returns the raw audio bytes for preview.
func (r *Recorder) Stop() ([]byte, error) {
	r.mu.Lock()
	cmd := r.cmd
	path := r.path
	r.mu.Unlock()

	if cmd == nil {
		return nil, fmt.Errorf("not recording")
	}

	_ = cmd.Process.Signal(syscall.SIGINT)
	_ = cmd.Wait() // arecord exits non-zero on SIGINT; the file is still valid

	r.mu.Lock()
	r.cmd = nil
	r.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read recording: %w", err)
	}
	return data, nil
}

// LastPath returns the temp wav path from the most recent Stop, for
// Confirm to process, or "" if nothing has been recorded yet.
func (r *Recorder) LastPath() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.path
}

// Discard removes the temp recording file, if any.
func (r *Recorder) Discard() {
	r.mu.Lock()
	path := r.path
	r.path = ""
	r.mu.Unlock()
	if path != "" {
		os.Remove(path)
	}
}
