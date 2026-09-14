package tts

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)


// sentenceSplitRE splits on sentence-ending punctuation followed by
// whitespace, keeping the punctuation with the preceding sentence.
var sentenceSplitRE = regexp.MustCompile(`(?:[.!?]+|[。！？]+)(?:\s+|$)`)

// clauseSplitRE splits on secondary punctuation (comma, semicolon, colon,
// or a spaced dash) — the fallback split point for a sentence that
// overruns maxChunkWords with no [.!?] boundary of its own. Each chunk is
// synthesized separately and joined with a fixed silence gap (see
// wav.Concat in appservice.go), so a cut at a raw word boundary is audible
// as a stutter/pause the original text never had; cutting at a clause
// boundary instead lands the gap where a speaker would naturally pause.
var clauseSplitRE = regexp.MustCompile(`(?:[,;:，；：]+(?:\s+|$))|(?:\s[-—–]+\s)`)

// missingSpaceRE catches sentence punctuation glued directly to the next
// sentence with no space (e.g. "week.Tell me" from scraped/pasted text),
// including when a closing quote/bracket sits between the punctuation and
// the next sentence (e.g. `week."Nobody`). sentenceSplitRE requires
// trailing whitespace to split, and wordCount (strings.Fields) treats the
// glued pair as one word — so an unspaced run-on silently undercounts and
// gets sent to the model as one giant unstructured chunk, which is what
// causes rushed/cut-off audio. Inserting the missing space before any
// splitting fixes both.
// \p{Lu} (Unicode uppercase letter) rather than [A-Z] so accented and
// non-Latin capitals (É, А, Α, …) are also recognized as a new sentence.
var missingSpaceRE = regexp.MustCompile(`([.!?。！？]["'”’)\]]*)([\p{Lu}\p{Han}])`)

// abbreviations end in a period but are not sentence-final even when
// immediately followed by a capitalized word with no space ("Dr.Jones",
// "St.Louis"). Naively inserting a space there makes sentenceSplitRE treat
// the abbreviation as a one-word sentence of its own — a lone "Dr." chunk
// synthesized with a silence gap before "Jones", which is a worse defect
// than the run-on this fix exists to prevent. Lower-cased, no trailing dot.
var abbreviations = map[string]bool{
	"mr": true, "mrs": true, "ms": true, "dr": true, "prof": true,
	"st": true, "sr": true, "jr": true, "vs": true, "etc": true,
	"inc": true, "ltd": true, "co": true, "no": true, "fig": true,
	"capt": true, "col": true, "gen": true, "lt": true, "sgt": true,
	"rev": true, "hon": true, "rep": true, "sen": true, "gov": true,
	"pres": true, "ave": true, "blvd": true, "corp": true, "dept": true,
	"univ": true, "assn": true, "bros": true,
}

// fixMissingSpaces inserts the space missingSpaceRE finds missing, except
// after a known abbreviation (see abbreviations) or after a single letter
// that is itself part of an acronym-style run like "U.S." (a dot right
// before it) — in both cases the [.!?] isn't actually sentence-final, so
// splitting there would manufacture a seam instead of removing one.
func fixMissingSpaces(text string) string {
	matches := missingSpaceRE.FindAllStringSubmatchIndex(text, -1)
	if matches == nil {
		return text
	}

	var b strings.Builder
	last := 0
	for _, m := range matches {
		punctStart, punctEnd := m[2], m[3]
		word, wordStart := precedingWord(text, punctStart)

		acronymLetter := len(word) == 1 && wordStart > 0 && text[wordStart-1] == '.'
		if abbreviations[strings.ToLower(word)] || acronymLetter {
			continue
		}

		b.WriteString(text[last:punctEnd])
		b.WriteByte(' ')
		last = punctEnd
	}
	b.WriteString(text[last:])
	return b.String()
}

// precedingWord returns the run of letters immediately before index end,
// and the index it starts at.
func precedingWord(text string, end int) (string, int) {
	start := end
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(text[:start])
		if !unicode.IsLetter(r) {
			break
		}
		start -= size
	}
	return text[start:end], start
}

// minChunkWords/maxChunkWords bound each chunk. Qwen3-TTS (unlike
// Pocket TTS) has no chunking of its own in llama.cpp — it generates
// the whole prompt as one autoregressive run, which is why long text
// degrades in quality and starts to sound rushed. Splitting on sentence
// boundaries within this word range keeps each call inside the range
// the model was actually tuned for.
const (
	minChunkWords = 8
	maxChunkWords = 20

	// hardMaxChunkWords is the ceiling past which a sentence with no
	// clause punctuation at all gets forcibly split on a raw word
	// boundary. Below this, a lone unpunctuated sentence is kept whole
	// even though it overruns maxChunkWords: every chunk boundary becomes
	// an audible silence-gap seam (wav.Concat in appservice.go), so a cut
	// with no natural pause behind it — no comma, no period — is a worse
	// defect than one chunk running a bit long and sounding slightly
	// rushed. Only a genuine run-on past this length is worth the seam.
	hardMaxChunkWords = 40
)

// ChunkText splits text into pieces of roughly minChunkWords to
// maxChunkWords, breaking at sentence boundaries where possible. A
// sentence longer than maxChunkWords is split at clause punctuation
// (comma/semicolon/colon/dash) if it has any, kept whole if it doesn't
// (up to hardMaxChunkWords), and only cut on a raw word boundary as a
// last resort. Returns a single chunk (the trimmed input) for short text.
func ChunkText(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	text = fixMissingSpaces(text)

	sentences := splitSentences(text)
	if len(sentences) <= 1 && wordCount(text) <= maxChunkWords {
		return []string{text}
	}

	return accumulate(sentences, splitLongSentence)
}

// accumulate groups pieces (sentences, or clauses within one overlong
// sentence) into chunks of roughly minChunkWords to maxChunkWords words.
// Any single piece already over maxChunkWords is broken up further by
// overflow before being added as its own chunk(s).
func accumulate(pieces []string, overflow func(string) []string) []string {
	var chunks []string
	var cur []string
	curWords := 0

	flush := func() {
		if len(cur) > 0 {
			chunks = append(chunks, strings.Join(cur, " "))
			cur = nil
			curWords = 0
		}
	}

	for _, p := range pieces {
		pw := wordCount(p)

		if pw > maxChunkWords {
			flush()
			chunks = append(chunks, overflow(p)...)
			continue
		}

		if curWords > 0 && curWords+pw > maxChunkWords {
			flush()
		}
		cur = append(cur, p)
		curWords += pw

		if curWords >= minChunkWords {
			flush()
		}
	}
	flush()

	return chunks
}

// splitLongSentence breaks a sentence with no [.!?] boundary but too many
// words for one chunk. It prefers cutting at comma/semicolon/colon/dash
// clause boundaries over a raw word-count cut. Any piece — the whole
// sentence when it has no clause punctuation, or an individual clause
// that's itself still too long once split — is kept whole up to
// hardMaxChunkWords rather than manufacture a seam with no pause behind
// it; only past that ceiling does a raw word-count cut happen.
func splitLongSentence(sent string) []string {
	clauses := splitClauses(sent)
	if len(clauses) > 1 {
		return accumulate(clauses, keepWholeOrSplitWords)
	}
	return keepWholeOrSplitWords(sent)
}

// keepWholeOrSplitWords is the last-resort overflow handler for a piece
// with no further punctuation to split on: kept intact up to
// hardMaxChunkWords, cut on a raw word boundary only past that.
func keepWholeOrSplitWords(s string) []string {
	if wordCount(s) <= hardMaxChunkWords {
		return []string{s}
	}
	return splitWords(s)
}

func splitSentences(text string) []string {
	return splitOn(sentenceSplitRE, text)
}

func splitClauses(text string) []string {
	return splitOn(clauseSplitRE, text)
}

func splitOn(re *regexp.Regexp, text string) []string {
	var out []string
	last := 0
	for _, loc := range re.FindAllStringIndex(text, -1) {
		out = append(out, strings.TrimSpace(text[last:loc[1]]))
		last = loc[1]
	}
	if last < len(text) {
		out = append(out, strings.TrimSpace(text[last:]))
	}
	return out
}

func splitWords(sent string) []string {
	words := tokenize(sent)
	var out []string
	for i := 0; i < len(words); i += maxChunkWords {
		end := min(i+maxChunkWords, len(words))
		out = append(out, joinTokens(words[i:end]))
	}
	return out
}

// tokenize splits s into word-equivalent units for length purposes: each
// Han character (Chinese/Japanese) is its own token, since CJK prose has
// no spaces between words at all — strings.Fields alone would count a
// whole Han sentence as a single "word", letting a long multi-sentence
// Han paragraph silently evade maxChunkWords entirely and get sent to the
// model as one giant unstructured chunk (the same rushed/cut-off defect
// as the missing-space-after-period English case, just via undercounting
// instead of a missing split point). Everything else is split on
// whitespace as usual.
func tokenize(s string) []string {
	var out []string
	for f := range strings.FieldsSeq(s) {
		start := 0
		for i, r := range f {
			if !unicode.Is(unicode.Han, r) {
				continue
			}
			if i > start {
				out = append(out, f[start:i])
			}
			out = append(out, f[i:i+utf8.RuneLen(r)])
			start = i + utf8.RuneLen(r)
		}
		if start < len(f) {
			out = append(out, f[start:])
		}
	}
	return out
}

// joinTokens re-assembles tokens from tokenize, adding a space between
// them except between two adjacent Han tokens (Han prose has no inter-word
// spaces; inserting ASCII spaces there would read as unnatural, mid-word
// gaps once synthesized).
func joinTokens(tokens []string) string {
	var b strings.Builder
	for i, tok := range tokens {
		if i > 0 && !(isHanToken(tokens[i-1]) && isHanToken(tok)) {
			b.WriteByte(' ')
		}
		b.WriteString(tok)
	}
	return b.String()
}

func isHanToken(tok string) bool {
	r, _ := utf8.DecodeRuneInString(tok)
	return unicode.Is(unicode.Han, r)
}

func wordCount(s string) int {
	return len(tokenize(s))
}
