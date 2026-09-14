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
		{Text: "Welcome back everyone. It's good to be here.", Instruct: ""},
		{Text: "I have huge news today, you won't believe it! Seriously, huge.", Instruct: "excited"},
		{Text: "Anyway, let's get into it.", Instruct: "calm"},
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
	if got[0] != (InstructSegment{Text: "Hello", Instruct: ""}) {
		t.Errorf("segment 0 = %+v", got[0])
	}
	if got[1] != (InstructSegment{Text: "world", Instruct: "b"}) {
		t.Errorf("segment 1 = %+v", got[1])
	}
}

func TestSplitInstructSegments_TagOnlyNoSpokenText(t *testing.T) {
	if got := SplitInstructSegments("[laughs]"); got != nil {
		t.Fatalf("got %v, want nil (tag with nothing to speak)", got)
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
