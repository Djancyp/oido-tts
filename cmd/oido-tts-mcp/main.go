// Command oido-tts-mcp exposes oido-tts's local text-to-speech engine as
// an MCP server over stdio, so any MCP client (Claude Desktop, Claude
// Code, etc.) can call it as a tool directly — no UI involved. It shares
// the exact same synthesis pipeline as the desktop app (internal/synth),
// just without the Wails event/progress plumbing, which has no meaning
// over a synchronous stdio tool call.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"oido-tts/internal/synth"
	"oido-tts/internal/tts"
)

func main() {
	engine, err := synth.NewEngineFromEnv()
	if err != nil {
		log.Fatalf("oido-tts-mcp: %v", err)
	}

	// jobMu serializes every call into the engine across both tools. The
	// MCP transport dispatches concurrent tool calls in their own
	// goroutines (see the SDK's jsonrpc2 layer), but tts.Engine and its
	// on-disk cache (internal/tts/cache.go) have no synchronization of
	// their own, and each call shells out to llama-tts, which loads the
	// full model into memory independently — the same "locking up the
	// machine" problem App.beginJob exists to prevent on the desktop
	// side (see appservice.go's App.synthMu doc). One job at a time here
	// too, for the same reason.
	var jobMu sync.Mutex

	server := mcp.NewServer(&mcp.Implementation{Name: "oido-tts", Version: "1.0.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name: "synthesize_speech",
		Description: "Convert text to speech using a local Qwen3-TTS model. Writes a wav file " +
			"to disk and returns its path. Runs fully on-device — no audio or text leaves the machine. " +
			"output_dir is a local filesystem path chosen by the caller with no sandboxing beyond normal " +
			"file permissions; only pass paths you trust.",
	}, synthesizeHandler(engine, &jobMu))

	mcp.AddTool(server, &mcp.Tool{
		Name: "build_podcast",
		Description: "Turn a two-speaker script into one stitched podcast-style episode wav file. " +
			"The script must use \"Speaker: line\" per paragraph (e.g. \"HOST: Welcome back.\"); " +
			"each distinct speaker name gets its own voice. Runs fully on-device. output_dir is a local " +
			"filesystem path chosen by the caller with no sandboxing beyond normal file permissions; " +
			"only pass paths you trust.",
	}, buildPodcastHandler(engine, &jobMu))

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("oido-tts-mcp: server error: %v", err)
	}
}

// resolveOutPath returns where a generated wav should be written: the
// caller's outputDir if given, otherwise a stable per-user cache
// directory. Unlike the desktop app's Synthesize/BuildPodcast (which
// return audio inline as base64 for webview playback and delete the
// scratch file), an MCP tool call has no playback surface — the file path
// *is* the result, so nothing is ever deleted here.
func resolveOutPath(outputDir, prefix string) (string, error) {
	dir := outputDir
	if dir == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			cacheDir = os.TempDir()
		}
		dir = filepath.Join(cacheDir, "oido-tts", "mcp")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%d.wav", prefix, time.Now().UnixNano())), nil
}

// SynthesizeInput is synthesize_speech's argument schema. Fields without
// `omitempty` are required by the generated JSON schema (see
// github.com/google/jsonschema-go's struct-tag inference).
type SynthesizeInput struct {
	Text string `json:"text" jsonschema:"the text to speak"`
	Lang string `json:"lang,omitempty" jsonschema:"BCP-47-ish language code such as en, zh, ja, de, fr, es, it, pt, ko, ru; omit for the model's default"`
	// SpeakerFile is a path to a local wav/mp3 reference clip to clone a
	// voice from; empty uses the model's default voice.
	SpeakerFile string `json:"speaker_file,omitempty" jsonschema:"local filesystem path to a wav/mp3 reference clip to clone a voice from; omit for the model's default voice"`
	Instruct    string `json:"instruct,omitempty" jsonschema:"optional natural-language style/emotion instruction, e.g. 'speak with a hint of panic creeping into your voice' (requires a llama-tts build with --tts-instruct support)"`
	// OutputDir, if set, is where the wav is saved; otherwise a stable
	// per-user cache directory is used. Either way the path is returned
	// and the file is kept — see resolveOutPath.
	OutputDir string `json:"output_dir,omitempty" jsonschema:"directory to save the generated wav into; omit to use a default cache directory"`
}

// SynthesizeOutput is synthesize_speech's structured result.
type SynthesizeOutput struct {
	Path string `json:"path" jsonschema:"filesystem path to the generated wav file"`
}

func synthesizeHandler(engine *tts.Engine, jobMu *sync.Mutex) mcp.ToolHandlerFor[SynthesizeInput, SynthesizeOutput] {
	errf := func(format string, args ...any) (*mcp.CallToolResult, SynthesizeOutput, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}},
			IsError: true,
		}, SynthesizeOutput{}, nil
	}

	return func(ctx context.Context, req *mcp.CallToolRequest, in SynthesizeInput) (*mcp.CallToolResult, SynthesizeOutput, error) {
		if len(in.Text) == 0 {
			return errf("text must not be empty")
		}

		outPath, err := resolveOutPath(in.OutputDir, "oido-tts")
		if err != nil {
			return errf("%v", err)
		}

		jobs := synth.BuildComposeJobs(in.Text, in.SpeakerFile, in.Instruct)

		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		jobMu.Lock()
		err = synth.RunSequentialJobs(ctx, engine, jobs, in.Lang, outPath, true, func(float64) {})
		jobMu.Unlock()
		if err != nil {
			return errf("synthesis failed: %v", err)
		}

		out := SynthesizeOutput{Path: outPath}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Saved speech to " + outPath}},
		}, out, nil
	}
}

// BuildPodcastInput is build_podcast's argument schema.
type BuildPodcastInput struct {
	Script string `json:"script" jsonschema:"the podcast script, one \"Speaker: line\" paragraph per turn, e.g. 'HOST: Welcome back.\\n\\nGUEST: Thanks for having me.'; a trailing [emotion] tag on a line (e.g. 'HOST: line [excited]') sets that turn's style"`
	// Voices maps a speaker name exactly as it appears in the script
	// (e.g. "HOST", "GUEST") to a local wav/mp3 reference clip path to
	// clone that speaker's voice from. Unmapped or unrecognized speakers
	// fall back to the model's default voice.
	Voices map[string]string `json:"voices,omitempty" jsonschema:"maps a speaker name as it appears in the script to a local wav/mp3 file path to clone that speaker's voice from; speakers not listed use the model's default voice"`
	Lang   string            `json:"lang,omitempty" jsonschema:"BCP-47-ish language code such as en, zh, ja, de, fr, es, it, pt, ko, ru; omit for the model's default"`
	OutputDir string         `json:"output_dir,omitempty" jsonschema:"directory to save the generated wav into; omit to use a default cache directory"`
}

// BuildPodcastOutput is build_podcast's structured result.
type BuildPodcastOutput struct {
	Path string `json:"path" jsonschema:"filesystem path to the generated wav file"`
}

func buildPodcastHandler(engine *tts.Engine, jobMu *sync.Mutex) mcp.ToolHandlerFor[BuildPodcastInput, BuildPodcastOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in BuildPodcastInput) (*mcp.CallToolResult, BuildPodcastOutput, error) {
		errf := func(format string, args ...any) (*mcp.CallToolResult, BuildPodcastOutput, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}},
				IsError: true,
			}, BuildPodcastOutput{}, nil
		}

		turns := tts.ParseScript(in.Script)
		if len(turns) == 0 {
			return errf(`no speaker turns found — use "Speaker: line" per paragraph`)
		}

		outPath, err := resolveOutPath(in.OutputDir, "podcast")
		if err != nil {
			return errf("%v", err)
		}

		jobs := make([]synth.Job, len(turns))
		for i, t := range turns {
			jobs[i] = synth.Job{Text: t.Text, SpeakerFile: in.Voices[t.Speaker], Instruct: t.Instruct, GapBeforeMs: t.GapBeforeMs}
		}

		ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
		defer cancel()
		jobMu.Lock()
		err = synth.RunSequentialJobs(ctx, engine, jobs, in.Lang, outPath, false, func(float64) {})
		jobMu.Unlock()
		if err != nil {
			return errf("podcast synthesis failed: %v", err)
		}

		out := BuildPodcastOutput{Path: outPath}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Saved podcast to " + outPath}},
		}, out, nil
	}
}
