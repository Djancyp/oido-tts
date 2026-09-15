package tts

import (
	"regexp"
	"strings"
)

// Turn is one speaker's line(s) in a podcast script.
type Turn struct {
	Speaker string
	Text    string
	// Instruct is an optional natural-language style/emotion hint parsed
	// from a trailing "[...]" tag on the turn, e.g. "Host: line [excited]".
	// Empty means no instruct for this turn.
	Instruct string
	// GapBeforeMs is this turn's timing relative to the previous turn,
	// from a leading "[pause:Ns]"/"[silence:Nms]" tag, e.g.
	// "Guest: [pause:-300ms] Yeah, totally!" to talk over the host's last
	// 300ms. NoGap means no tag was given and the default turn gap applies.
	GapBeforeMs int
}

// instructTagRE matches a trailing "[...]" tag on a turn's text — the
// per-turn emotion/style hint (e.g. "Host: I can't believe it! [excited]").
// Stripped from the spoken text and carried separately as Turn.Instruct.
// Any trailing bracket is treated this way unconditionally — a turn that
// legitimately ends with a bracket for another reason (a citation, a
// sound cue meant to be spoken) will have it silently eaten too.
var instructTagRE = regexp.MustCompile(`\[([^\[\]]+)\]\s*$`)

// leadingTagRE matches a "[...]" tag at the very start of a turn's text —
// where a [pause:Ns]/[silence:Nms] timing tag lives, e.g.
// "Guest: [pause:-300ms] Yeah, totally!" to overlap into the previous turn.
var leadingTagRE = regexp.MustCompile(`^\[([^\[\]]+)\]\s*`)

// ParseScript parses a simple "NAME: line" script into turns, one per
// paragraph (blank-line separated). A paragraph's first line must start
// with "Speaker:"; any following lines in the same paragraph are folded
// into that turn's text. A trailing "[emotion]" tag on the folded text is
// parsed off into Turn.Instruct rather than spoken aloud. Paragraphs that
// don't start with a recognized "word:" prefix are skipped.
func ParseScript(script string) []Turn {
	var turns []Turn

	for _, para := range splitParagraphs(script) {
		lines := strings.Split(para, "\n")
		speaker, first, ok := splitSpeakerLine(lines[0])
		if !ok {
			continue
		}

		textLines := []string{first}
		for _, l := range lines[1:] {
			l = strings.TrimSpace(l)
			if l != "" {
				textLines = append(textLines, l)
			}
		}

		text := strings.TrimSpace(strings.Join(textLines, " "))
		if text == "" {
			continue
		}
		text, gapMs := extractLeadingPauseTag(text)
		text, instruct := extractInstructTag(text)
		if text == "" {
			continue
		}
		turns = append(turns, Turn{Speaker: speaker, Text: text, Instruct: instruct, GapBeforeMs: gapMs})
	}

	return turns
}

// extractInstructTag splits a trailing "[emotion]" tag off text, returning
// the spoken text (tag removed, trimmed) and the tag's contents (empty if
// there was no tag).
func extractInstructTag(text string) (spoken, instruct string) {
	m := instructTagRE.FindStringSubmatchIndex(text)
	if m == nil {
		return text, ""
	}
	spoken = strings.TrimSpace(text[:m[0]])
	instruct = text[m[2]:m[3]]
	return spoken, instruct
}

// extractLeadingPauseTag splits a leading "[pause:Ns]"/"[silence:Nms]" tag
// off text, returning the spoken text (tag removed, trimmed) and its
// duration in ms (NoGap if there was no leading tag, or the leading tag
// wasn't a pause tag — e.g. an emotion tag meant to color the whole turn is
// left in place for extractInstructTag or the spoken text itself).
func extractLeadingPauseTag(text string) (spoken string, gapMs int) {
	m := leadingTagRE.FindStringSubmatchIndex(text)
	if m == nil {
		return text, NoGap
	}
	ms, ok := parsePauseTag(text[m[2]:m[3]])
	if !ok {
		return text, NoGap
	}
	return strings.TrimSpace(text[m[1]:]), ms
}

func splitParagraphs(script string) []string {
	var out []string
	var cur []string
	for _, line := range strings.Split(script, "\n") {
		if strings.TrimSpace(line) == "" {
			if len(cur) > 0 {
				out = append(out, strings.Join(cur, "\n"))
				cur = nil
			}
			continue
		}
		cur = append(cur, line)
	}
	if len(cur) > 0 {
		out = append(out, strings.Join(cur, "\n"))
	}
	return out
}

// splitSpeakerLine splits "Name: rest of line" into ("Name", "rest of
// line", true). Speaker names are a short run of letters/digits/spaces/
// underscores/hyphens before the first colon — long enough for "Host",
// "Guest 2", "Dr. Ada" wouldn't match (the period breaks it, treated as
// not a speaker line and skipped, same as any other non-conforming line).
func splitSpeakerLine(line string) (speaker, rest string, ok bool) {
	idx := strings.IndexByte(line, ':')
	if idx <= 0 || idx > 30 {
		return "", "", false
	}
	name := strings.TrimSpace(line[:idx])
	if name == "" {
		return "", "", false
	}
	for _, r := range name {
		if !(r == ' ' || r == '_' || r == '-' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return "", "", false
		}
	}
	return name, strings.TrimSpace(line[idx+1:]), true
}
