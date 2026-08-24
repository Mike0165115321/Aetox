package skill

// Finding the part of a page that answers the question, without asking a model.
//
// This is the other half of §177. That decision removed the summarizer because
// the pattern it copied needs a cheap fast reader to run the pass on, and Aetox
// has no such model. What it did not say — because nobody had thought of it
// yet — is that the reader does not have to be a model at all.
//
// BM25 is the ranking function every full-text search has used for thirty
// years. It is arithmetic over word counts: no index to build, no weights to
// download, no GPU, no round trip. On one page split into a few dozen passages
// it runs in well under a millisecond, which is a different order of magnitude
// from the 57 seconds the summarizer cost on the call that started all this.
//
// Owner, 24 Aug, on why the heavy half stayed out: *"BM25 มันเบาเครื่องนะ"*. He
// is right, and the reframing that follows from it is the useful part —
// **BM25 does not need a web index, it needs a corpus.** Perplexity runs it
// over the web because the web is their corpus. The corpus here is the page
// already in hand, which is a corpus they cannot see.
//
// **What this cannot do**, said out loud because the ceiling matters when
// choosing what to build next: it is lexical. A page that says "disable
// automatic reattempts" does not match a search for "stop the retry loop", and
// nothing in this file will ever make it. That gap is what embeddings buy and
// what they charge for.

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// BM25's two constants, at the values three decades of information retrieval
// have settled on. k1 damps the payoff of a term appearing many times in one
// passage; b decides how hard a long passage is penalised for its length.
// Named rather than inlined so a reader can see there are exactly two dials
// here and that neither was tuned by guessing.
const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// passageSize is the length a page is cut into before scoring.
//
// Small enough that a hit points at a paragraph rather than at a chapter, big
// enough to carry its own context — a match handed back without the sentences
// around it is a citation, and the caller wanted an answer. Cuts land on blank
// lines where there are any, so this is a target rather than a ruler.
const passageSize = 900

type passage struct {
	at   int // byte offset in the page, so the caller can ask for more around it
	text string
	toks []string
}

// splitPassages cuts a page into scoreable pieces, preferring the boundaries
// the page already has.
//
// Paragraph first, then any line, then a hard cut. A page with no newlines at
// all — minified text, a JSON blob — still divides, because a document that
// refuses to be chunked would otherwise score as one enormous passage and win
// or lose entirely.
func splitPassages(body string) []passage {
	var out []passage
	for at := 0; at < len(body); {
		end := at + passageSize
		if end >= len(body) {
			out = append(out, newPassage(at, body[at:]))
			break
		}
		// Look for a break in the last third, so a cut never lands far from
		// where the passage was going to end anyway.
		window := body[at:end]
		floor := passageSize * 2 / 3
		cut := strings.LastIndex(window, "\n\n")
		if cut < floor {
			cut = strings.LastIndexByte(window, '\n')
		}
		if cut < floor {
			cut = strings.LastIndexByte(window, ' ')
		}
		if cut < floor {
			cut = passageSize
		}
		out = append(out, newPassage(at, body[at:at+cut]))
		at += cut
	}
	return out
}

func newPassage(at int, text string) passage {
	return passage{at: at, text: text, toks: tokenize(text)}
}

// tokenize turns text into the terms BM25 counts.
//
// **Two rules, because Thai has no spaces.** A whitespace tokenizer is correct
// for Latin script and useless for Thai: a whole sentence arrives as one token
// that matches nothing. So a token containing Thai or CJK characters is emitted
// as its character trigrams instead of whole.
//
// That is not an invention — it is what this product already decided. The FTS
// tables in desktop/db.go are declared `tokenize='trigram'` for exactly this
// reason. Doing something different here would mean Aetox ranked Thai two ways
// depending on which door you came in.
//
// Latin stays word-based rather than also going to trigrams, because trigrams
// flatten IDF: "the" and "retry" start looking alike when both are three
// letters at a time, and IDF is the half of BM25 that knows which words matter.
func tokenize(s string) []string {
	var out []string
	for _, field := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		// Mn is part of a word, not a break in one.
		//
		// Thai tone marks and most of its vowels are nonspacing marks, so
		// IsLetter says no to them and a plain letters-and-digits split cuts
		// ขนส่งทางราง into ขนส and งทางราง at the mai ek. Measured 24 Aug:
		// the query came apart at exactly that character, and it happens to
		// ส่ง, ที่, ไม่, น้ำ — most of the common words in the language.
		// The trigrams of the fragments still matched enough to work, which is
		// the kind of luck that hides a bug rather than the kind that excuses
		// one.
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.Is(unicode.Mn, r)
	}) {
		if isWideScript(field) {
			out = append(out, trigrams(field)...)
			continue
		}
		out = append(out, field)
	}
	return out
}

// isWideScript reports whether a token is written in a script that does not
// separate words with spaces. Thai is the one that matters here; the CJK
// ranges are included because the same argument applies to them and leaving
// them out would be a bug waiting for its first Chinese page.
func isWideScript(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Thai, r) || unicode.Is(unicode.Han, r) ||
			unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) ||
			unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

func trigrams(s string) []string {
	r := []rune(s)
	if len(r) <= 3 {
		return []string{s}
	}
	out := make([]string, 0, len(r)-2)
	for i := 0; i+3 <= len(r); i++ {
		out = append(out, string(r[i:i+3]))
	}
	return out
}

// scorePassages ranks the passages of one page against a query.
//
// Standard BM25 with the passages as the corpus: term frequency within a
// passage, inverse document frequency across them, length-normalised. Returns
// scores positionally aligned with the input.
func scorePassages(ps []passage, query string) []float64 {
	scores := make([]float64, len(ps))
	terms := tokenize(query)
	if len(ps) == 0 || len(terms) == 0 {
		return scores
	}

	total := 0
	for _, p := range ps {
		total += len(p.toks)
	}
	avgLen := float64(total) / float64(len(ps))
	if avgLen == 0 {
		return scores
	}

	// How many passages each term appears in, counted once per term rather than
	// once per occurrence of it in the query.
	seenTerm := map[string]bool{}
	docFreq := map[string]int{}
	for _, t := range terms {
		if seenTerm[t] {
			continue
		}
		seenTerm[t] = true
		for _, p := range ps {
			for _, w := range p.toks {
				if w == t {
					docFreq[t]++
					break
				}
			}
		}
	}

	// Every term's IDF, worked out once, because it is now needed twice: for
	// the score, and as the WEIGHT the coverage below is measured in.
	//
	// The +0.5s are BM25's own smoothing, and the outer 1+ keeps a term that
	// appears in every passage worth a little rather than nothing.
	n := float64(len(ps))
	termIDF := map[string]float64{}
	var want float64
	for t := range seenTerm {
		df := float64(docFreq[t])
		termIDF[t] = logOnePlus((n - df + 0.5) / (df + 0.5))
		want += termIDF[t]
	}
	if want == 0 {
		return scores
	}

	for i, p := range ps {
		freq := map[string]int{}
		for _, w := range p.toks {
			freq[w]++
		}
		length := float64(len(p.toks))
		var score, have float64
		for t := range seenTerm {
			f := float64(freq[t])
			if f == 0 {
				continue
			}
			have += termIDF[t]
			score += termIDF[t] * (f * (bm25K1 + 1)) /
				(f + bm25K1*(1-bm25B+bm25B*length/avgLen))
		}
		// COVERAGE, weighted by IDF.
		//
		// Plain BM25 sums per term, so a passage carrying one query term twenty
		// times and none of the others can outscore one carrying most of them
		// once. In English that is a passage that only says "the"; in Thai it is
		// worse, because a query is trigrams and trigrams collide across word
		// boundaries.
		//
		// Measured 24 Aug on th.wikipedia's ประเทศไทย: a search for ขนส่งทางราง
		// (rail transport) returned the country infobox first — area,
		// population, GDP — because ตารางกิโลเมตร (square kilometres) contains
		// ราง, and the infobox is short and says it many times.
		//
		// **Counting matches would not have been enough**, which is why this is
		// a weight and not a fraction. Scaling by 1-of-9 removed the infobox and
		// promoted ทางทิศใต้ in its place — a passage that also matched exactly one
		// trigram, just a different one. Matching a term the whole page uses is
		// not the same achievement as matching one only this passage has, and
		// IDF is the number that already knows the difference. So coverage is
		// the share of the query's INFORMATION a passage accounts for, not the
		// share of its tokens.
		if have > 0 {
			scores[i] = score * have / want
		}
	}
	return scores
}

// logOnePlus guards math.Log1p against the one input that has no answer, so the
// formula above can be read without a branch in the middle of it.
func logOnePlus(x float64) float64 {
	if x <= -1 {
		return 0
	}
	return math.Log1p(x)
}

// selectPassages picks the best passages that fit in budget and returns them in
// READING order, not in score order.
//
// That is the whole difference between a search result and an answer. Ranked
// output hands the model paragraph 40 before paragraph 3 and leaves it to
// reassemble the argument; document order gives it the page with the
// irrelevant parts taken out, which is what it asked for.
//
// Returns nil when nothing matched at all, so the caller can say so plainly
// rather than handing back the top of the page as though it were a hit.
func selectPassages(ps []passage, query string, budget int) []passage {
	scores := scorePassages(ps, query)
	idx := make([]int, 0, len(ps))
	for i, s := range scores {
		if s > 0 {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		return nil
	}
	sort.SliceStable(idx, func(a, b int) bool { return scores[idx[a]] > scores[idx[b]] })

	chosen := map[int]bool{}
	used := 0
	for _, i := range idx {
		if used > 0 && used+len(ps[i].text) > budget {
			continue
		}
		chosen[i] = true
		used += len(ps[i].text)
		if used >= budget {
			break
		}
	}

	out := make([]passage, 0, len(chosen))
	for i := range ps {
		if chosen[i] {
			out = append(out, ps[i])
		}
	}
	return out
}
