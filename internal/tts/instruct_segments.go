package tts

import (
	"regexp"
	"strconv"
	"strings"
)

// InstructSegment is one stretch of Compose text under a single instruct
// (empty Instruct means "no tag has applied yet — caller supplies its own
// default", e.g. the sidebar Style field).
//
// GapBeforeMs is the timing to apply before this segment's audio, from an
// inline [pause:Ns]/[silence:Nms] tag immediately preceding it; NoGap means
// no tag was given and the caller's normal chunk-boundary gap applies. A
// real value is a signed ms offset — positive inserts silence, negative
// overlaps this segment's audio back into the tail of the previous one (two
// speakers talking over each other); 0 is an explicit "no gap", distinct
// from NoGap so a caller can tell "no gap requested" from "no tag at all".
type InstructSegment struct {
	Text        string
	Instruct    string
	GapBeforeMs int
}

// NoGap marks a segment/job with no explicit pause tag. Kept well outside
// maxPauseMs's range so it can never collide with a real (possibly
// negative) parsed duration.
const NoGap = -1 << 30

// pauseTagRE recognizes a pause/silence tag's inner content — the same
// "[...]" bracket inlineTagRE already isolates, checked against this to
// tell a timing tag from an emotion tag like "[whisper]". A leading "-"
// requests overlap (this segment starts that many ms before the previous
// one ends) instead of extra silence.
var pauseTagRE = regexp.MustCompile(`(?i)^(?:pause|silence):(-?\d+(?:\.\d+)?)(ms|s)$`)

// maxPauseMs caps the magnitude of a user-specified pause/overlap so a typo
// (an extra zero, a misplaced unit) can't stall playback for minutes or
// overlap two clips into nonsense; clamped rather than rejected so one bad
// tag doesn't fail an entire synth job.
const maxPauseMs = 30000

// parsePauseTag parses a pause/silence tag's inner content into a signed
// duration in milliseconds, clamped to +/-maxPauseMs. ok is false for
// anything else (an emotion tag), which the caller then treats as an
// instruct.
func parsePauseTag(tag string) (ms int, ok bool) {
	m := pauseTagRE.FindStringSubmatch(strings.TrimSpace(tag))
	if m == nil {
		return 0, false
	}
	val, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	if strings.EqualFold(m[2], "s") {
		val *= 1000
	}
	return min(max(int(val), -maxPauseMs), maxPauseMs), true
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
	pendingGapMs := NoGap
	last := 0
	for _, m := range matches {
		if piece := strings.TrimSpace(text[last:m[0]]); piece != "" {
			segments = append(segments, InstructSegment{Text: piece, Instruct: currentInstruct, GapBeforeMs: pendingGapMs})
			pendingGapMs = NoGap
		}
		tag := text[m[2]:m[3]]
		if ms, ok := parsePauseTag(tag); ok {
			pendingGapMs = ms
		} else {
			currentInstruct = tag
		}
		last = m[1]
	}
	if piece := strings.TrimSpace(text[last:]); piece != "" {
		segments = append(segments, InstructSegment{Text: piece, Instruct: currentInstruct, GapBeforeMs: pendingGapMs})
	}
	return segments
}
