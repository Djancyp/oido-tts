package tts

import (
	"regexp"
	"strings"
)

// InstructSegment is one stretch of Compose text under a single instruct
// (empty Instruct means "no tag has applied yet — caller supplies its own
// default", e.g. the sidebar Style field).
type InstructSegment struct {
	Text     string
	Instruct string
}

// inlineTagRE matches every "[...]" tag anywhere in Compose text — unlike
// instructTagRE (script.go), which only matches a tag trailing a whole
// podcast turn, this must find every occurrence so none of them leak into
// the spoken text and get read aloud by the model.
var inlineTagRE = regexp.MustCompile(`\[([^\[\]]+)\]`)

// SplitInstructSegments splits Compose text at every "[emotion]" tag. A
// tag sets the instruct for everything that follows it, up to the next
// tag — so it marks where a mode starts rather than annotating only the
// text immediately before it. Tag markers are stripped from every
// returned segment's Text; text before the first tag (or all of it, if
// there are no tags) gets Instruct == "".
func SplitInstructSegments(text string) []InstructSegment {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	matches := inlineTagRE.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return []InstructSegment{{Text: text}}
	}

	var segments []InstructSegment
	currentInstruct := ""
	last := 0
	for _, m := range matches {
		if piece := strings.TrimSpace(text[last:m[0]]); piece != "" {
			segments = append(segments, InstructSegment{Text: piece, Instruct: currentInstruct})
		}
		currentInstruct = text[m[2]:m[3]]
		last = m[1]
	}
	if piece := strings.TrimSpace(text[last:]); piece != "" {
		segments = append(segments, InstructSegment{Text: piece, Instruct: currentInstruct})
	}
	return segments
}
