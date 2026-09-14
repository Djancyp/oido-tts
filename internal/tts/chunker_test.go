package tts

import (
	"strings"
	"testing"
)

func TestChunkText_ShortTextIsOneChunk(t *testing.T) {
	got := ChunkText("Hello there, how are you doing today?")
	if len(got) != 1 {
		t.Fatalf("got %d chunks, want 1: %v", len(got), got)
	}
}

func TestChunkText_Empty(t *testing.T) {
	if got := ChunkText("   "); got != nil {
		t.Fatalf("got %v, want nil for empty input", got)
	}
}

func TestChunkText_LongTextSplitsWithinBounds(t *testing.T) {
	text := ""
	for i := 0; i < 20; i++ {
		text += "This is a test sentence with several words in it. "
	}

	chunks := ChunkText(text)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks for long text, got %d", len(chunks))
	}
	for _, c := range chunks {
		if wordCount(c) > maxChunkWords {
			t.Errorf("chunk exceeds max words (%d): %q", maxChunkWords, c)
		}
	}
}

func TestChunkText_VeryLongSingleSentenceStillSplits(t *testing.T) {
	text := ""
	for i := 0; i < 60; i++ {
		text += "word "
	}
	// no punctuation at all — one giant "sentence"
	chunks := ChunkText(text)
	if len(chunks) < 2 {
		t.Fatalf("expected the long run-on to be split, got %d chunk(s)", len(chunks))
	}
	total := 0
	for _, c := range chunks {
		w := wordCount(c)
		if w > maxChunkWords {
			t.Errorf("chunk exceeds max words: %d", w)
		}
		total += w
	}
	if total != 60 {
		t.Errorf("total words across chunks = %d, want 60 (no words lost/duplicated)", total)
	}
}

func TestChunkText_LongSentenceSplitsOnClausePunctuation(t *testing.T) {
	// One run-on sentence, no [.!?], but plenty of commas — forced splits
	// should land after a comma, not mid-clause on a raw word count.
	text := "First we gather the ingredients, then we mix the batter carefully, next we preheat the oven to the right temperature, and finally we bake it until golden brown and delicious"

	chunks := ChunkText(text)
	if len(chunks) < 2 {
		t.Fatalf("expected the long sentence to be split, got %d chunk(s)", len(chunks))
	}
	for i, c := range chunks {
		if wordCount(c) > maxChunkWords {
			t.Errorf("chunk %d exceeds max words: %q", i, c)
		}
		// every chunk but the last should end right after a clause boundary
		if i < len(chunks)-1 {
			last := c[len(c)-1]
			if last != ',' {
				t.Errorf("chunk %d does not end on punctuation: %q", i, c)
			}
		}
	}
}

func TestChunkText_OverlongClauseStaysWholeUnderHardMax(t *testing.T) {
	// One clause between commas is, on its own, over maxChunkWords (20)
	// but under hardMaxChunkWords (40) and has no further punctuation —
	// it should stay whole rather than get a raw word-count cut, same
	// leniency a whole unpunctuated sentence gets.
	longClause := "we need to gather every single ingredient listed on the recipe card before we can even think about " +
		"starting to preheat the oven for the very long bake ahead of us" // 25 words
	if wordCount(longClause) <= maxChunkWords {
		t.Fatalf("test setup bug: longClause has %d words, want > %d", wordCount(longClause), maxChunkWords)
	}
	text := "First, " + longClause + ", then we bake it until golden brown."

	chunks := ChunkText(text)
	for _, c := range chunks {
		if strings.Contains(c, longClause) {
			return // found intact in one chunk, as expected
		}
	}
	t.Fatalf("clause got split mid-way instead of kept whole (no chunk contains it intact): %v", chunks)
}

func TestChunkText_UnpunctuatedSentenceUnderHardMaxStaysWhole(t *testing.T) {
	// Reported case: 26 words, single sentence, no comma/semicolon/colon
	// anywhere — only the old hard 20-word cap forced an artificial cut,
	// landing the silence-gap seam mid-sentence with no pause behind it.
	text := "The railway authority also said former CIA chief David Petraeus had been on board another train at Yahodyn station at the time of the drone strike."

	chunks := ChunkText(text)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1 (sentence has no punctuation to split on and is under hardMaxChunkWords): %v", len(chunks), chunks)
	}
	if chunks[0] != text {
		t.Errorf("chunk = %q, want unchanged %q", chunks[0], text)
	}
}

func TestChunkText_NoSpaceAfterPeriodStillSplits(t *testing.T) {
	// Reported case: pasted text with no space after sentence punctuation
	// ("week.Tell") was silently treated as one giant word by strings.Fields,
	// undercounting the whole paragraph and sending it whole to the model —
	// causing rushed/cut-off audio.
	text := "Tell me how many real-time conversations you've had with customers this week.Tell me how many your competitor has had and I'll tell you the odds they beat you.Not your feature velocity. Not your funding. Not your follower count.Conversations. This week. With people who pay you or might.It's the only leading indicator I've ever found that never lies."

	chunks := ChunkText(text)
	if len(chunks) < 2 {
		t.Fatalf("expected the unspaced run-on to be split into multiple chunks, got %d: %v", len(chunks), chunks)
	}
	for i, c := range chunks {
		if wordCount(c) > maxChunkWords {
			t.Errorf("chunk %d exceeds max words: %q", i, c)
		}
	}
}

func TestChunkText_MissingSpaceScenarios(t *testing.T) {
	tests := []struct {
		name string
		text string
		// wantIn: substrings that must each appear as their own trimmed
		// chunk (or be findable across chunk boundaries) after fixing —
		// i.e. the glued pair split apart with a space inserted.
		wantSplitAfter []string // punctuation+letter pairs that must NOT remain glued in any chunk
	}{
		{
			name:           "period glued to capital",
			text:           "This is a test sentence with several words.Followed by another one right after it with no space.",
			wantSplitAfter: []string{"words.Followed"},
		},
		{
			name:           "exclamation glued to capital",
			text:           "Watch out for that ledge!Seriously, it drops off right there with nothing to catch you.",
			wantSplitAfter: []string{"ledge!Seriously"},
		},
		{
			name:           "question mark glued to capital",
			text:           "Have you talked to five customers this week?Because that number predicts more than your roadmap does.",
			wantSplitAfter: []string{"week?Because"},
		},
		{
			name:           "multiple consecutive glued sentences",
			text:           "First point here.Second point right after.Third point glued on too.Fourth point closes it out nicely.",
			wantSplitAfter: []string{"here.Second", "after.Third", "too.Fourth"},
		},
		{
			name:           "glued sentence after closing quote",
			text:           "She said \"we ship every week.\"Nobody on the team remembers the last time that wasn't true.",
			wantSplitAfter: []string{"week.\"Nobody"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := ChunkText(tt.text)
			joined := strings.Join(chunks, " ")
			for _, glued := range tt.wantSplitAfter {
				if strings.Contains(joined, glued) {
					t.Errorf("glued pair %q still present after chunking: %v", glued, chunks)
				}
			}
			// no words lost or duplicated by the space-insertion + split
			wantWords := wordCount(fixMissingSpaces(tt.text))
			gotWords := 0
			for _, c := range chunks {
				gotWords += wordCount(c)
			}
			if gotWords != wantWords {
				t.Errorf("word count = %d, want %d (words lost/duplicated): %v", gotWords, wantWords, chunks)
			}
		})
	}
}

func TestChunkText_MissingSpaceDoesNotTouchDecimalsOrAbbreviations(t *testing.T) {
	// A period between a digit and a digit (3.5), or before a lowercase
	// letter (e.g.next), is not a sentence boundary — missingSpaceRE must
	// only fire on punctuation directly followed by an uppercase letter
	// (or CJK), so these must survive untouched.
	tests := []struct {
		name string
		text string
	}{
		{"decimal number", "The part costs 3.5 dollars per unit, which is cheap."},
		{"lowercase after period", "See the appendix, e.g.next section covers it in more depth than the summary."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := ChunkText(tt.text)
			joined := strings.Join(chunks, " ")
			if !strings.Contains(joined, "3.5") && tt.name == "decimal number" {
				t.Errorf("decimal got mangled: %v", chunks)
			}
			if !strings.Contains(joined, "e.g.next") && tt.name == "lowercase after period" {
				t.Errorf("abbreviation got mangled: %v", chunks)
			}
		})
	}
}

func TestChunkText_ChinesePunctuationGluedToNextSentence(t *testing.T) {
	// CJK text commonly has no space between sentences at all — the fix
	// must also catch full-width punctuation glued directly to the next
	// (Han-script) sentence.
	text := "这是第一句话，讲的是产品的核心功能。这是第二句话，紧跟在句号后面没有空格，讲的是用户反馈和市场验证的重要性。"

	chunks := ChunkText(text)
	joined := strings.Join(chunks, "")
	if strings.Contains(joined, "。这是第二句话") && len(chunks) < 2 {
		t.Errorf("expected Chinese run-on to be split at the glued sentence boundary, got: %v", chunks)
	}
}

func TestChunkText_AlreadySpacedTextUnaffectedByFix(t *testing.T) {
	// Sanity check: normally-punctuated text (space after every sentence
	// boundary) must chunk identically before and after the fix.
	text := "This is a normal sentence. Here is another one, spaced properly. And a third to round it out nicely."
	chunks := ChunkText(text)
	joined := strings.Join(chunks, " ")
	if strings.Contains(joined, "  ") {
		t.Errorf("fix introduced a double space into normally-punctuated text: %v", chunks)
	}
	wantWords := wordCount(text)
	gotWords := 0
	for _, c := range chunks {
		gotWords += wordCount(c)
	}
	if gotWords != wantWords {
		t.Errorf("word count = %d, want %d", gotWords, wantWords)
	}
}

func TestChunkText_MoreMissingSpaceScenarios(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		wantSplitAfter []string // glued pairs that must not survive in any chunk
	}{
		{
			name:           "ellipsis glued to capital",
			text:           "I waited by the door for a while...Eventually I gave up and went back inside to finish the report.",
			wantSplitAfter: []string{"while...Eventually"},
		},
		{
			name:           "double punctuation glued to capital",
			text:           "Are you serious right now?!Yes, completely serious, and I would do it again without hesitation.",
			wantSplitAfter: []string{"now?!Yes"},
		},
		{
			name:           "accented capital after period",
			text:           "Il pleut beaucoup aujourd'hui dans toute la region.Élise part quand même demain matin sous la pluie battante.",
			wantSplitAfter: []string{"region.Élise"},
		},
		{
			name:           "cyrillic capital after period",
			text:           "Мы закончили работу над проектом на этой неделе.Команда очень довольна результатом и планирует отдохнуть.",
			wantSplitAfter: []string{"неделе.Команда"},
		},
		{
			name:           "single quote glued to capital",
			text:           "He finally said 'that's the deal'.Nobody in the room said anything back to him after that moment.",
			wantSplitAfter: []string{"deal'.Nobody"},
		},
		{
			name: "newline separated glued sentences",
			text: "First line ends here.Second line starts right after.\nThird line is separate.Fourth line is glued too.",
			wantSplitAfter: []string{
				"here.Second", "after.\nThird", "separate.Fourth",
			},
		},
		{
			name: "messy real-world paragraph mixing multiple issues",
			text: "Tell me how many real-time conversations you've had with customers this week.Tell me how many your competitor has had and I'll tell you the odds they beat you!Not your feature velocity.Not your funding.Not your follower count.Conversations.This week.With people who pay you or might.It's the only leading indicator I've ever found that never lies.",
			wantSplitAfter: []string{
				"week.Tell", "you!Not", "velocity.Not", "funding.Not",
				"count.Conversations", "Conversations.This", "week.With", "might.It",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := ChunkText(tt.text)
			joined := strings.Join(chunks, " ")
			for _, glued := range tt.wantSplitAfter {
				if strings.Contains(joined, glued) {
					t.Errorf("glued pair %q still present after chunking: %v", glued, chunks)
				}
			}
			fixed := fixMissingSpaces(tt.text)
			wantWords := wordCount(fixed)
			gotWords := 0
			for _, c := range chunks {
				gotWords += wordCount(c)
			}
			if gotWords != wantWords {
				t.Errorf("word count = %d, want %d (words lost/duplicated): %v", gotWords, wantWords, chunks)
			}
		})
	}
}

func TestChunkText_MissingSpaceLeavesNumbersAndListsAlone(t *testing.T) {
	// A period between two digits (decimals, "3.5") or before a digit
	// (versions, list markers like "1.2 Next step") is never a sentence
	// boundary on its own — missingSpaceRE only fires before an uppercase
	// letter, so digit-adjacent periods must stay untouched.
	tests := []struct {
		name string
		text string
		want string // substring that must survive verbatim
	}{
		{"decimal mid-sentence", "The measured rate is 3.5 percent, which beats last quarter's number.", "3.5"},
		{"version number", "We shipped version 2.1 today and it fixed the crash from last week.", "2.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := ChunkText(tt.text)
			joined := strings.Join(chunks, " ")
			if !strings.Contains(joined, tt.want) {
				t.Errorf("expected %q to survive untouched, got: %v", tt.want, chunks)
			}
		})
	}
}

func TestChunkText_NumberedListItemGetsSplitAtCapital(t *testing.T) {
	// A digit before the period ("1.Gather") does NOT block the
	// uppercase-after check — each numbered list item reads as its own
	// sentence, which is the right behavior for TTS (list items are
	// natural pause points, same as any other sentence boundary).
	text := "Steps: 1.Gather ingredients 2.Mix batter 3.Bake for forty minutes."

	chunks := ChunkText(text)
	joined := strings.Join(chunks, " ")
	for _, glued := range []string{"1.Gather", "2.Mix", "3.Bake"} {
		if strings.Contains(joined, glued) {
			t.Errorf("glued pair %q still present after chunking: %v", glued, chunks)
		}
	}
}

func TestChunkText_TrailingGluedSentenceAtEnd(t *testing.T) {
	// The glued pair is the very last thing in the string, right up
	// against EOF — no trailing space to rely on for the final split.
	text := "This part is fine and normal.Nothing.However this final part right at the end.Is glued too"

	chunks := ChunkText(text)
	joined := strings.Join(chunks, " ")
	for _, glued := range []string{"fine and normal.Nothing", "Nothing.However", "end.Is"} {
		if strings.Contains(joined, glued) {
			t.Errorf("glued pair %q still present: %v", glued, chunks)
		}
	}
}

func TestChunkText_NumberedHeaderWithShortFragmentsAndQuotes(t *testing.T) {
	// Reported case: numbered section headers ("2. Offer"), very short
	// standalone fragments (below minChunkWords, no comma to lean on), and
	// a quoted word mid-sentence ("Everyone" is a phone book.) — none of
	// these are glued, but they stress short-sentence accumulation and
	// quote handling in ways the earlier examples didn't.
	text := "2. Offer\n" +
		"Outbound doesn't fix a weak offer. It exposes it.\n" +
		"Deal size under €5k? Outbound won't pay off.\n" +
		"One offer per segment.\n" +
		"3. ICP\n" +
		"\"Everyone\" is a phone book. Pick one segment.\n" +
		"The best buyers already tried. They failed.\n" +
		"ICP in your head? Can't be scored."

	chunks := ChunkText(text)
	if len(chunks) == 0 {
		t.Fatalf("got no chunks for non-empty input")
	}
	for i, c := range chunks {
		if wordCount(c) > maxChunkWords {
			t.Errorf("chunk %d exceeds max words: %q", i, c)
		}
	}
	wantWords := wordCount(strings.Join(strings.Fields(text), " "))
	gotWords := 0
	for _, c := range chunks {
		gotWords += wordCount(c)
	}
	if gotWords != wantWords {
		t.Errorf("word count across chunks = %d, want %d (words lost/duplicated): %v", gotWords, wantWords, chunks)
	}
}

func TestChunkText_ShortFragmentsBelowMinWordsGetGrouped(t *testing.T) {
	// A run of very short sentences (each under minChunkWords, no clause
	// punctuation to lean on) should still get grouped up toward
	// minChunkWords rather than synthesized one tiny fragment at a time —
	// each extra chunk boundary is an audible silence-gap seam.
	text := "One offer per segment. Pick one segment. They failed. Can't be scored."

	chunks := ChunkText(text)
	tinyChunks := 0
	for _, c := range chunks {
		if wordCount(c) < minChunkWords {
			tinyChunks++
		}
	}
	// the very last chunk is allowed to be short (nothing left to merge
	// into it); anything else under minChunkWords is an avoidable seam.
	if tinyChunks > 1 {
		t.Errorf("got %d chunks under minChunkWords (%d), want at most 1 (the trailing remainder): %v", tinyChunks, minChunkWords, chunks)
	}
}

func TestChunkText_NumberedHeaderAloneIsNotMangled(t *testing.T) {
	// "2. Offer" — digit, period, space, capital word, no sentence-ending
	// punctuation after it. Must not be merged into a following unrelated
	// sentence in a way that loses or duplicates the header text.
	text := "2. Offer\nOutbound doesn't fix a weak offer. It exposes it."

	chunks := ChunkText(text)
	joined := strings.Join(chunks, " ")
	if !strings.Contains(joined, "2. Offer") {
		t.Errorf("header %q lost or mangled: %v", "2. Offer", chunks)
	}
}

func TestChunkText_CurrencyAndQuestionMarkFragments(t *testing.T) {
	// Short question fragments with a currency symbol glued to a digit
	// ("€5k?") must not confuse word counting or get treated as a
	// sentence-boundary letter (€ is not \p{Lu}).
	text := "Deal size under €5k? Outbound won't pay off."

	chunks := ChunkText(text)
	joined := strings.Join(chunks, " ")
	if !strings.Contains(joined, "€5k?") {
		t.Errorf("currency fragment mangled: %v", chunks)
	}
}

func TestChunkText_AbbreviationNotSplitAloneAtChunkBoundary(t *testing.T) {
	// Bug found while testing: naively inserting a space after every
	// [.!?]+capital pair turns "Dr.Jones" into "Dr. Jones", and
	// sentenceSplitRE then treats "Dr." as a complete one-word sentence.
	// If the accumulate() boundary happens to land right there (forced by
	// a big piece immediately after), "Dr." gets synthesized alone with a
	// silence gap before "Jones" — a worse defect than the run-on this
	// whole fix exists to prevent. Construct exactly that boundary
	// pressure: a full sentence, then the abbreviation, then a giant
	// unpunctuated run-on that forces an overflow flush right after it.
	sentenceA := "After the long meeting concluded everyone packed up their notes and left quietly."
	longRunOn := strings.Repeat("word ", 25)
	text := sentenceA + " Dr.Jones then walked into " + longRunOn

	chunks := ChunkText(text)
	for i, c := range chunks {
		trimmed := strings.TrimSpace(c)
		if abbreviations[strings.ToLower(strings.TrimSuffix(trimmed, "."))] && trimmed[len(trimmed)-1] == '.' {
			t.Errorf("chunk %d is a lone abbreviation with nothing else in it: %q (chunks: %v)", i, c, chunks)
		}
	}
	joined := strings.Join(chunks, " ")
	if !strings.Contains(joined, "Dr.Jones") {
		t.Errorf("expected \"Dr.Jones\" to stay attached, got: %v", chunks)
	}
}

func TestChunkText_CommonAbbreviationsStayAttachedToFollowingName(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string // the glued abbreviation+name pair that must survive
	}{
		{"Mr", "We spoke with Mr.Smith yesterday about the contract renewal terms.", "Mr.Smith"},
		{"Mrs", "We met Mrs.Adams at the fundraiser last night downtown.", "Mrs.Adams"},
		{"Dr", "Dr.Jones will see you now in the east wing office.", "Dr.Jones"},
		{"St (saint/street)", "The hotel is on St.Patrick Street near downtown.", "St.Patrick"},
		{"Prof", "Prof.Lee published a new paper on the subject this year.", "Prof.Lee"},
		{"vs", "It's Chelsea vs.Arsenal this weekend at the stadium.", "vs.Arsenal"},
		{"etc", "We packed snacks, water, maps, etc.Then we left for the trip.", "etc.Then"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := ChunkText(tt.text)
			joined := strings.Join(chunks, " ")
			if !strings.Contains(joined, tt.want) {
				t.Errorf("expected %q to stay attached, got: %v", tt.want, chunks)
			}
		})
	}
}

func TestChunkText_AcronymDotsDoNotProduceLoneSingleLetterChunk(t *testing.T) {
	// "U.S." / "D.C." style acronyms: each internal dot is followed
	// directly by another capital letter. Whatever fixMissingSpaces does
	// with the internal dots, it must never leave a bare single letter
	// ("U.", "S.", "C.") as its own chunk.
	text := "The report cites the U.S.Department of Commerce as its primary source for this data."

	chunks := ChunkText(text)
	for i, c := range chunks {
		trimmed := strings.TrimSpace(c)
		if len(trimmed) <= 2 && wordCount(trimmed) == 1 {
			t.Errorf("chunk %d is a bare single-letter fragment: %q (chunks: %v)", i, c, chunks)
		}
	}
}

func TestWordCount_HanCharactersCountedIndividually(t *testing.T) {
	// Bug found while testing: Chinese prose has no spaces between words
	// at all, so strings.Fields alone counted an entire Han sentence as
	// ONE "word" — a whole multi-sentence Chinese paragraph could then
	// never exceed maxChunkWords and would get sent to the model as one
	// giant unstructured chunk (the same rushed/cut-off defect as the
	// missing-space English case, just via undercounting rather than a
	// missing split point). Each Han character must count as its own unit.
	got := wordCount("今天天气很好")
	if got != 6 {
		t.Errorf("wordCount(6 hanzi) = %d, want 6", got)
	}

	mixed := wordCount("Hello 世界 test")
	// "Hello"(1) + 世(1) + 界(1) + "test"(1) = 4
	if mixed != 4 {
		t.Errorf("wordCount(mixed) = %d, want 4", mixed)
	}
}

func TestChunkText_LongChinesePargraphSplitsAcrossMultipleChunks(t *testing.T) {
	// Reported-adjacent case: a real Chinese paragraph, no spaces
	// anywhere (normal for CJK prose), five sentences separated only by
	// 。— before the wordCount fix this stayed one giant chunk regardless
	// of length. Must now split into multiple reasonably-sized pieces.
	text := "今天天气很好。我们决定去公园散步。路上遇到了很多朋友，大家聊得很开心。晚上回家后开始准备晚饭。这是一个非常愉快的周末。"

	chunks := ChunkText(text)
	if len(chunks) < 2 {
		t.Fatalf("expected the long Chinese paragraph to split into multiple chunks, got %d: %v", len(chunks), chunks)
	}
	for i, c := range chunks {
		if wordCount(c) > maxChunkWords {
			t.Errorf("chunk %d exceeds max words (%d hanzi-equivalent): %q", i, wordCount(c), c)
		}
	}
	// no hanzi lost or duplicated (compare rune counts, ignoring
	// whitespace inserted between chunks/sentences)
	stripSpace := func(s string) string { return strings.ReplaceAll(s, " ", "") }
	wantRunes := len([]rune(stripSpace(text)))
	gotRunes := 0
	for _, c := range chunks {
		gotRunes += len([]rune(stripSpace(c)))
	}
	if gotRunes != wantRunes {
		t.Errorf("rune count across chunks = %d, want %d (hanzi lost/duplicated): %v", gotRunes, wantRunes, chunks)
	}
}

func TestChunkText_PunctuationRunsNeverProduceEmptyChunk(t *testing.T) {
	// Adversarial punctuation-only / repeated-punctuation inputs must
	// never yield an empty chunk (which would be a silent/dead audio
	// segment once synthesized).
	cases := []string{
		"Wait??!!Really???!!!Yes indeed it is quite true and confirmed for certain this time around completely.",
		"Hmm.....So then what happened next in the story after that point in time.",
		"Really?!?!?!That seems excessive for a simple question honestly speaking to be fair.",
		".",
		"...",
		"!!!",
		"A.",
		".A",
		"Mr.",
		"Mr.Mr.Mr.",
	}
	for _, c := range cases {
		chunks := ChunkText(c)
		for i, ch := range chunks {
			if strings.TrimSpace(ch) == "" {
				t.Errorf("input %q produced an empty chunk at index %d: %v", c, i, chunks)
			}
		}
	}
}

func TestChunkText_PreservesAllWords(t *testing.T) {
	text := "One two three. Four five six seven eight nine ten eleven. Twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty twenty-one twenty-two twenty-three twenty-four twenty-five twenty-six twenty-seven twenty-eight twenty-nine thirty thirty-one."
	wantWords := wordCount(text)

	chunks := ChunkText(text)
	got := 0
	for _, c := range chunks {
		got += wordCount(c)
	}
	if got != wantWords {
		t.Errorf("word count across chunks = %d, want %d", got, wantWords)
	}
}
