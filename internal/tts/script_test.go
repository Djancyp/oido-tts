package tts

import "testing"

func TestParseScript_Basic(t *testing.T) {
	script := "HOST: Welcome back to the show.\n\nGUEST: Thanks for having me."
	turns := ParseScript(script)

	if len(turns) != 2 {
		t.Fatalf("got %d turns, want 2: %+v", len(turns), turns)
	}
	if turns[0].Speaker != "HOST" || turns[0].Text != "Welcome back to the show." {
		t.Errorf("turn 0 = %+v", turns[0])
	}
	if turns[1].Speaker != "GUEST" || turns[1].Text != "Thanks for having me." {
		t.Errorf("turn 1 = %+v", turns[1])
	}
}

func TestParseScript_MultilineParagraphFoldsIntoOneTurn(t *testing.T) {
	script := "HOST: This is a long thought\nthat wraps onto a second line\nand even a third."
	turns := ParseScript(script)

	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1: %+v", len(turns), turns)
	}
	want := "This is a long thought that wraps onto a second line and even a third."
	if turns[0].Text != want {
		t.Errorf("text = %q, want %q", turns[0].Text, want)
	}
}

func TestParseScript_SkipsNonConformingParagraphs(t *testing.T) {
	script := "This has no speaker prefix.\n\nHOST: But this does."
	turns := ParseScript(script)

	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1 (non-conforming paragraph skipped): %+v", len(turns), turns)
	}
	if turns[0].Speaker != "HOST" {
		t.Errorf("speaker = %q, want HOST", turns[0].Speaker)
	}
}

func TestParseScript_PreservesSpeakerOrderAndRepeats(t *testing.T) {
	script := "HOST: One.\n\nGUEST: Two.\n\nHOST: Three."
	turns := ParseScript(script)

	if len(turns) != 3 {
		t.Fatalf("got %d turns, want 3", len(turns))
	}
	wantSpeakers := []string{"HOST", "GUEST", "HOST"}
	for i, w := range wantSpeakers {
		if turns[i].Speaker != w {
			t.Errorf("turn %d speaker = %q, want %q", i, turns[i].Speaker, w)
		}
	}
}

func TestParseScript_Empty(t *testing.T) {
	if got := ParseScript("   \n\n  "); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestParseScript_TrailingBracketParsedAsInstruct(t *testing.T) {
	script := "HOST: I can't believe you actually did that. [shocked, a little panicked]\n\nGUEST: Relax, it's fine."
	turns := ParseScript(script)

	if len(turns) != 2 {
		t.Fatalf("got %d turns, want 2: %+v", len(turns), turns)
	}
	if turns[0].Text != "I can't believe you actually did that." {
		t.Errorf("text = %q, want tag stripped", turns[0].Text)
	}
	if turns[0].Instruct != "shocked, a little panicked" {
		t.Errorf("instruct = %q, want %q", turns[0].Instruct, "shocked, a little panicked")
	}
	if turns[1].Instruct != "" {
		t.Errorf("turn 1 instruct = %q, want empty (no tag)", turns[1].Instruct)
	}
}

func TestParseScript_BracketOnlyTurnIsSkipped(t *testing.T) {
	// a turn that's just a bracket tag with no spoken text has nothing to
	// synthesize, so it's dropped like any other empty turn
	script := "HOST: [laughs]\n\nGUEST: That's actually true."
	turns := ParseScript(script)

	if len(turns) != 1 || turns[0].Speaker != "GUEST" {
		t.Fatalf("got %+v, want only the GUEST turn", turns)
	}
}

func TestParseScript_LeadingPauseTagSetsOverlap(t *testing.T) {
	script := "HOST: So anyway, I was thinking...\n\nGUEST: [pause:-300ms] Yeah, totally!"
	turns := ParseScript(script)

	if len(turns) != 2 {
		t.Fatalf("got %d turns, want 2: %+v", len(turns), turns)
	}
	if turns[0].GapBeforeMs != NoGap {
		t.Errorf("turn 0 GapBeforeMs = %d, want NoGap (no tag)", turns[0].GapBeforeMs)
	}
	if turns[1].GapBeforeMs != -300 {
		t.Errorf("turn 1 GapBeforeMs = %d, want -300", turns[1].GapBeforeMs)
	}
	if turns[1].Text != "Yeah, totally!" {
		t.Errorf("turn 1 text = %q, want tag stripped", turns[1].Text)
	}
}

func TestParseScript_LeadingAndTrailingTagsBothParsed(t *testing.T) {
	script := "GUEST: [pause:500ms] Wait, really? [shocked]"
	turns := ParseScript(script)

	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1: %+v", len(turns), turns)
	}
	if turns[0].GapBeforeMs != 500 {
		t.Errorf("GapBeforeMs = %d, want 500", turns[0].GapBeforeMs)
	}
	if turns[0].Instruct != "shocked" {
		t.Errorf("Instruct = %q, want shocked", turns[0].Instruct)
	}
	if turns[0].Text != "Wait, really?" {
		t.Errorf("text = %q, want both tags stripped", turns[0].Text)
	}
}

func TestParseScript_LeadingEmotionTagIsNotTreatedAsPause(t *testing.T) {
	// A leading tag that isn't pause/silence syntax (e.g. an emotion tag
	// meant to color the whole turn) must be left for extractInstructTag/
	// spoken text, not silently eaten as a timing tag.
	script := "HOST: [whisper] Keep it down."
	turns := ParseScript(script)

	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1: %+v", len(turns), turns)
	}
	if turns[0].GapBeforeMs != NoGap {
		t.Errorf("GapBeforeMs = %d, want NoGap", turns[0].GapBeforeMs)
	}
	if turns[0].Text != "[whisper] Keep it down." {
		t.Errorf("text = %q, want leading emotion tag left in place", turns[0].Text)
	}
}

func TestParseScript_SpeakerNameWithSpaceAndNumber(t *testing.T) {
	script := "Guest 2: Hello there."
	turns := ParseScript(script)
	if len(turns) != 1 || turns[0].Speaker != "Guest 2" {
		t.Fatalf("got %+v, want speaker \"Guest 2\"", turns)
	}
}
