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
}

// instructTagRE matches a trailing "[...]" tag on a turn's text — the
// per-turn emotion/style hint (e.g. "Host: I can't believe it! [excited]").
// Stripped from the spoken text and carried separately as Turn.Instruct.
// Any trailing bracket is treated this way unconditionally — a turn that
// legitimately ends with a bracket for another reason (a citation, a
// sound cue meant to be spoken) will have it silently eaten too.
var instructTagRE = regexp.MustCompile(`\[([^\[\]]+)\]\s*$`)

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
		text, instruct := extractInstructTag(text)
		if text == "" {
			continue
		}
		turns = append(turns, Turn{Speaker: speaker, Text: text, Instruct: instruct})
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
