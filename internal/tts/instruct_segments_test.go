package tts

import "testing"

func TestSplitInstructSegments_NoTags(t *testing.T) {
	got := SplitInstructSegments("Just plain text, nothing tagged.")
	if len(got) != 1 || got[0].Instruct != "" {
		t.Fatalf("got %+v, want one untagged segment", got)
	}
	if got[0].Text != "Just plain text, nothing tagged." {
		t.Errorf("text = %q", got[0].Text)
	}
}

func TestSplitInstructSegments_Empty(t *testing.T) {
	if got := SplitInstructSegments("   "); got != nil {
		t.Fatalf("got %v, want nil for empty input", got)
	}
}

func TestSplitInstructSegments_TagAppliesForward(t *testing.T) {
	text := "Welcome back everyone. It's good to be here. [excited] I have huge news today, you won't believe it! Seriously, huge. [calm] Anyway, let's get into it."
	got := SplitInstructSegments(text)

	want := []InstructSegment{
		{Text: "Welcome back everyone. It's good to be here.", Instruct: "", GapBeforeMs: NoGap},
		{Text: "I have huge news today, you won't believe it! Seriously, huge.", Instruct: "excited", GapBeforeMs: NoGap},
		{Text: "Anyway, let's get into it.", Instruct: "calm", GapBeforeMs: NoGap},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d segments, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("segment %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSplitInstructSegments_NoTextNeverLeaksTagIntoSpeech(t *testing.T) {
	// The exact bug being fixed: a tag mid-text must never survive into
	// any segment's spoken Text.
	text := "Hello there. [excited] This part is loud! [calm] This part is quiet."
	for _, seg := range SplitInstructSegments(text) {
		if containsBracket(seg.Text) {
			t.Errorf("tag leaked into spoken text: %q", seg.Text)
		}
	}
}

func TestSplitInstructSegments_LeadingTag(t *testing.T) {
	// A tag right at the start has no preceding text to attach to — only
	// the segment(s) after it should exist.
	got := SplitInstructSegments("[whisper] Keep it down.")
	if len(got) != 1 || got[0].Instruct != "whisper" || got[0].Text != "Keep it down." {
		t.Fatalf("got %+v", got)
	}
}

func TestSplitInstructSegments_BackToBackTagsOverride(t *testing.T) {
	// No text between two tags — the second simply wins for what follows.
	got := SplitInstructSegments("Hello [a][b] world")
	if len(got) != 2 {
		t.Fatalf("got %d segments, want 2: %+v", len(got), got)
	}
	if got[0] != (InstructSegment{Text: "Hello", Instruct: "", GapBeforeMs: NoGap}) {
		t.Errorf("segment 0 = %+v", got[0])
	}
	if got[1] != (InstructSegment{Text: "world", Instruct: "b", GapBeforeMs: NoGap}) {
		t.Errorf("segment 1 = %+v", got[1])
	}
}

func TestSplitInstructSegments_TagOnlyNoSpokenText(t *testing.T) {
	if got := SplitInstructSegments("[laughs]"); got != nil {
		t.Fatalf("got %v, want nil (tag with nothing to speak)", got)
	}
}

func TestSplitInstructSegments_PauseTagSetsGapOnNextSegment(t *testing.T) {
	got := SplitInstructSegments("First part. [pause:2s] Second part.")
	want := []InstructSegment{
		{Text: "First part.", Instruct: "", GapBeforeMs: NoGap},
		{Text: "Second part.", Instruct: "", GapBeforeMs: 2000},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d segments, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("segment %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSplitInstructSegments_PauseTagDoesNotBecomeInstruct(t *testing.T) {
	got := SplitInstructSegments("[pause:500ms] Hello there.")
	if len(got) != 1 || got[0].Instruct != "" || got[0].GapBeforeMs != 500 {
		t.Fatalf("got %+v, want empty instruct and 500ms gap", got)
	}
}

func TestSplitInstructSegments_ExplicitZeroPauseDiffersFromNoTag(t *testing.T) {
	got := SplitInstructSegments("First part. [pause:0s] Second part.")
	if len(got) != 2 {
		t.Fatalf("got %d segments, want 2: %+v", len(got), got)
	}
	if got[0].GapBeforeMs != NoGap {
		t.Errorf("segment 0 GapBeforeMs = %d, want NoGap (no tag preceded it)", got[0].GapBeforeMs)
	}
	if got[1].GapBeforeMs != 0 {
		t.Errorf("segment 1 GapBeforeMs = %d, want 0 (explicit [pause:0s], distinct from NoGap)", got[1].GapBeforeMs)
	}
}

func TestSplitInstructSegments_PauseTagClampedToMax(t *testing.T) {
	got := SplitInstructSegments("[pause:999s] Too long.")
	if len(got) != 1 || got[0].GapBeforeMs != maxPauseMs {
		t.Fatalf("got %+v, want gap clamped to %d", got, maxPauseMs)
	}
}

func TestParsePauseTag(t *testing.T) {
	tests := []struct {
		tag    string
		wantMs int
		wantOk bool
	}{
		{"pause:500ms", 500, true},
		{"pause:2s", 2000, true},
		{"SILENCE:1.5s", 1500, true},
		{"pause:2S", 2000, true},
		{"pause:-300ms", -300, true},
		{"pause:-1s", -1000, true},
		{"pause:-999s", -maxPauseMs, true},
		{"excited", 0, false},
		{"pause:abc", 0, false},
	}
	for _, tt := range tests {
		ms, ok := parsePauseTag(tt.tag)
		if ms != tt.wantMs || ok != tt.wantOk {
			t.Errorf("parsePauseTag(%q) = (%d, %v), want (%d, %v)", tt.tag, ms, ok, tt.wantMs, tt.wantOk)
		}
	}
}

func containsBracket(s string) bool {
	for _, r := range s {
		if r == '[' || r == ']' {
			return true
		}
	}
	return false
}
