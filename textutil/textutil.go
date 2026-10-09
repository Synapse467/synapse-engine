// Package textutil turns text into comparable terms. It is deliberately simple and has no
// learned components, so the same text always yields the same terms.
package textutil

import (
	"strings"
	"unicode"
)

var stop = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a about above after again all also am an and any are as at be because been before being below
		between both but by can could did do does doing down during each few for from further had has have having he her here
		hers him his how i if in into is it its just me more most my no nor not now of off on once only or other our out over
		own same she should so some such than that the their them then there these they this those through to too under until
		up very was we were what when where which while who whom why will with would you your tell please explain`) {
		stop[w] = true
	}
}

// Terms splits text into lower-case, stemmed terms, dropping common words. The order is the order
// of appearance and repeats are kept, so callers can count them.
func Terms(text string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		word := b.String()
		b.Reset()
		if stop[word] {
			return
		}
		if stem := Stem(word); len([]rune(stem)) >= 2 && !stop[stem] {
			out = append(out, stem)
		}
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		} else {
			flush()
		}
	}
	flush()
	return out
}

// Unique returns the distinct terms of text, in order of first appearance.
func Unique(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range Terms(text) {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// Stem removes a few common English endings. It is intentionally conservative: a wrong stem only
// matters when it makes two different words look the same, so it only strips endings that are
// safe to strip.
func Stem(word string) string {
	stem := stripSuffix(word)
	if len(stem) > 4 && strings.HasSuffix(stem, "e") {
		stem = stem[:len(stem)-1]
	}
	return stem
}

func stripSuffix(word string) string {
	n := len(word)
	switch {
	case n > 5 && strings.HasSuffix(word, "ies"):
		return word[:n-3] + "y"
	case n > 6 && (strings.HasSuffix(word, "tion") || strings.HasSuffix(word, "sion")):
		return word[:n-3]
	case n > 6 && strings.HasSuffix(word, "ing"):
		return undouble(word[:n-3])
	case n > 5 && strings.HasSuffix(word, "ed"):
		return undouble(word[:n-2])
	case n > 4 && strings.HasSuffix(word, "es") && (strings.HasSuffix(word, "ses") || strings.HasSuffix(word, "xes") || strings.HasSuffix(word, "ches") || strings.HasSuffix(word, "shes")):
		return word[:n-2]
	case n > 3 && strings.HasSuffix(word, "s") && !strings.HasSuffix(word, "ss") && !strings.HasSuffix(word, "us") && !strings.HasSuffix(word, "is"):
		return word[:n-1]
	}
	return word
}

func undouble(word string) string {
	n := len(word)
	if n >= 3 && word[n-1] == word[n-2] && !strings.ContainsRune("aeioulsz", rune(word[n-1])) {
		return word[:n-1]
	}
	return word
}

// Sentences splits text into sentences and returns each with its byte offsets in the original
// text. It splits on '.', '!' and '?' followed by whitespace, and on line breaks inside lists.
func Sentences(text string) []Span {
	var out []Span
	start := 0
	emit := func(end int) {
		s, e := start, end
		for s < e && isSpace(text[s]) {
			s++
		}
		for e > s && isSpace(text[e-1]) {
			e--
		}
		if e > s {
			out = append(out, Span{Start: s, End: e, Text: text[s:e]})
		}
		start = end
	}
	for i := 0; i < len(text); i++ {
		c := text[i]
		if (c == '.' || c == '!' || c == '?') && (i+1 == len(text) || isSpace(text[i+1])) {
			// Keep abbreviations such as "e.g." and "i.e." together, and decimals are not followed by space.
			if c == '.' && i >= 3 && (text[i-3:i+1] == "e.g." || text[i-3:i+1] == "i.e.") {
				continue
			}
			emit(i + 1)
		}
	}
	emit(len(text))
	return out
}

// Span is a piece of text with its byte offsets in the original.
type Span struct {
	Start, End int
	Text       string
}

func isSpace(b byte) bool { return b == ' ' || b == '\n' || b == '\t' || b == '\r' }
