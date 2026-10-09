// Package extract proposes capsule items from a document. It reads the document's structure
// (headings, numbered lists, examples) and a few plain-language cues ("always", "unless"), with
// no model and no network. Its output is a set of proposals: nothing becomes part of a capsule
// until the expert approves it.
//
// It will miss things and occasionally propose the wrong kind of item. That is by design: the
// review step is where the expert's judgement enters, and every proposal carries a citation of
// the exact words it came from so the expert can check it quickly.
package extract

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Synapse467/synapse-core/capsule"
	"github.com/Synapse467/synapse-engine/textutil"
)

// Proposal is an item the extractor found, with the confidence label drafts record.
type Proposal struct {
	Item       capsule.Item
	Confidence string
}

var (
	headingRe   = regexp.MustCompile(`^\s{0,3}#{1,6}\s+(.+?)\s*#*\s*$`)
	numberedRe  = regexp.MustCompile(`^\s*(?:\d{1,2}[.)]|[-*])\s+(.+)$`)
	numberRe    = regexp.MustCompile(`^\s*\d{1,2}[.)]\s+`)
	exampleRe   = regexp.MustCompile(`(?i)^\s*(?:for example|example|case study|case|in one case|scenario)\b[:,]?\s*`)
	exceptRe    = regexp.MustCompile(`(?i)\b(?:except|unless|does not apply|do not apply|doesn't apply|not applicable|the exception)\b`)
	ruleRe      = regexp.MustCompile(`(?i)\b(?:always|never|must|should|avoid|prefer|rule of thumb|as a rule|do not|don't|make sure|ensure)\b|^\s*(?:if|when)\b[^,]{3,},`)
	claimRe     = regexp.MustCompile(`(?i)\b(?:is|are|means|causes|requires|because|leads to|results in)\b`)
	conditionRe = regexp.MustCompile(`(?i)^\s*(?:when|if|after|before|in case)\b.{3,}`)
)

const minClaimWords = 7

// FromText finds proposals in a document. sourceID names the document in citations, and
// contributor is the address the items are attributed to. Citation offsets are byte offsets into
// text, so text must be the exact bytes of the file that is recorded as the source.
func FromText(sourceID, text, contributor string) []Proposal {
	var out []Proposal
	blocks := splitBlocks(text)
	heading := ""
	var lastRule *int  // index in out of the last rule/claim in this section, for attaching exceptions
	var pending *block // a paragraph ending in ":" that introduces the list after it

	for _, block := range blocks {
		introBlock := pending
		pending = nil
		if m := headingRe.FindStringSubmatch(block.text); m != nil && !strings.Contains(block.text, "\n") {
			heading = strings.TrimSpace(m[1])
			lastRule = nil
			continue
		}
		lines := strings.Split(block.text, "\n")
		if strings.HasSuffix(block.text, ":") && !strings.Contains(block.text, "\n") {
			copyBlock := block
			pending = &copyBlock
			continue
		}

		if steps, intro := numberedSteps(lines); len(steps) >= 2 {
			if introBlock != nil && intro == "" {
				intro = flatten(introBlock.text)
				block.start = introBlock.start // cite the introduction together with the steps
				block.text = introBlock.text + "\n" + block.text
			}
			item := capsule.Item{
				Type:        capsule.Procedure,
				Title:       titleFor("", firstNonEmpty(heading, intro), "Procedure"),
				Steps:       steps,
				Contributor: contributor,
				Tags:        tagsFor(heading),
				Citations:   []capsule.Citation{cite(sourceID, text, block.start, block.end)},
			}
			if conditionRe.MatchString(intro) {
				item.Conditions = []string{strings.TrimSpace(strings.TrimRight(intro, ":."))}
			}
			if valid(item) {
				out = append(out, Proposal{Item: item, Confidence: "extracted"})
				lastRule = nil
			}
			continue
		}

		if loc := exampleRe.FindStringIndex(block.text); loc != nil {
			body := strings.TrimSpace(block.text[loc[1]:])
			if utf8.RuneCountInString(body) >= 40 {
				item := capsule.Item{
					Type:        capsule.Case,
					Title:       titleFor(heading, body, "Example"),
					Body:        flatten(body),
					Contributor: contributor,
					Tags:        tagsFor(heading),
					Citations:   []capsule.Citation{cite(sourceID, text, block.start, block.end)},
				}
				if valid(item) {
					out = append(out, Proposal{Item: item, Confidence: "extracted"})
				}
			}
			continue
		}

		for _, s := range textutil.Sentences(block.text) {
			sentence := stripBullet(flatten(s.Text))
			if utf8.RuneCountInString(sentence) < 20 {
				continue
			}
			abs := block.start + s.Start
			citation := cite(sourceID, text, abs, abs+len(s.Text))

			if exceptRe.MatchString(sentence) {
				if lastRule != nil {
					prev := &out[*lastRule].Item
					prev.Exceptions = append(prev.Exceptions, sentence)
					prev.Citations = append(prev.Citations, citation)
				} else if item, ok := ruleItem(sentence, heading, contributor, citation); ok {
					out = append(out, Proposal{Item: item, Confidence: "extracted"})
					idx := len(out) - 1
					lastRule = &idx
				}
				continue
			}
			if ruleRe.MatchString(sentence) {
				if item, ok := ruleItem(sentence, heading, contributor, citation); ok {
					out = append(out, Proposal{Item: item, Confidence: "extracted"})
					idx := len(out) - 1
					lastRule = &idx
				}
				continue
			}
			if len(strings.Fields(sentence)) >= minClaimWords && claimRe.MatchString(sentence) {
				item := capsule.Item{
					Type:        capsule.Claim,
					Title:       titleFor(heading, sentence, "Claim"),
					Body:        sentence,
					Contributor: contributor,
					Tags:        tagsFor(heading),
					Citations:   []capsule.Citation{citation},
				}
				if valid(item) {
					out = append(out, Proposal{Item: item, Confidence: "extracted"})
					idx := len(out) - 1
					lastRule = &idx
				}
			}
		}
	}
	return out
}

func ruleItem(sentence, heading, contributor string, citation capsule.Citation) (capsule.Item, bool) {
	item := capsule.Item{
		Type:        capsule.Heuristic,
		Title:       titleFor(heading, sentence, "Rule"),
		Body:        sentence,
		Contributor: contributor,
		Tags:        tagsFor(heading),
		Citations:   []capsule.Citation{citation},
	}
	if conditionRe.MatchString(sentence) {
		if i := strings.Index(sentence, ","); i > 0 {
			item.Conditions = []string{strings.TrimSpace(sentence[:i])}
		}
	}
	return item, valid(item)
}

func valid(item capsule.Item) bool {
	item.ID = "x"
	for _, issue := range item.Validate() {
		if !strings.Contains(issue, "the id is empty") {
			return false
		}
	}
	return true
}

type block struct {
	text       string
	start, end int
}

// splitBlocks splits text at blank lines and returns each block with its byte offsets.
func splitBlocks(text string) []block {
	var out []block
	start := -1
	last := 0
	pos := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		blank := strings.TrimSpace(line) == ""
		if blank {
			if start >= 0 {
				out = append(out, block{text: strings.TrimRight(text[start:last], "\r\n \t"), start: start, end: last})
				start = -1
			}
		} else {
			if start < 0 {
				start = pos + (len(line) - len(strings.TrimLeft(line, " \t")))
			}
			last = pos + len(strings.TrimRight(line, "\r\n \t"))
		}
		pos += len(line)
	}
	if start >= 0 {
		out = append(out, block{text: strings.TrimRight(text[start:last], "\r\n \t"), start: start, end: last})
	}
	return out
}

func numberedSteps(lines []string) (steps []string, intro string) {
	for i, line := range lines {
		if m := numberedRe.FindStringSubmatch(line); m != nil && (numberRe.MatchString(line) || i > 0) {
			steps = append(steps, strings.TrimSpace(m[1]))
		} else if i == 0 && len(lines) > 1 {
			intro = strings.TrimSpace(line)
		} else if len(steps) > 0 {
			// A wrapped continuation of the previous step.
			steps[len(steps)-1] += " " + strings.TrimSpace(line)
		} else {
			return nil, ""
		}
	}
	return steps, intro
}

func cite(sourceID, text string, start, end int) capsule.Citation {
	quote := text[start:end]
	if utf8.RuneCountInString(quote) > capsule.MaxQuote {
		runes := []rune(quote)
		quote = string(runes[:capsule.MaxQuote])
		end = start + len(quote)
	}
	return capsule.NewCitation(sourceID, start, end, quote)
}

func flatten(s string) string { return strings.Join(strings.Fields(s), " ") }

func stripBullet(s string) string {
	s = strings.TrimSpace(s)
	for _, p := range []string{"- ", "* "} {
		s = strings.TrimPrefix(s, p)
	}
	return numberRe.ReplaceAllString(s, "")
}

func titleFor(heading, text, fallback string) string {
	text = flatten(text)
	if text == "" {
		text = fallback
	}
	text = strings.TrimRight(text, ".:!?")
	if r, size := utf8.DecodeRuneInString(text); r != utf8.RuneError && unicode.IsLower(r) {
		text = string(unicode.ToUpper(r)) + text[size:]
	}
	runes := []rune(text)
	if len(runes) > 80 {
		cut := string(runes[:80])
		if i := strings.LastIndex(cut, " "); i > 40 {
			cut = cut[:i]
		}
		text = cut + "…"
	}
	if heading != "" && strings.EqualFold(text, fallback) {
		return heading
	}
	return text
}

func tagsFor(heading string) []string {
	if heading == "" {
		return nil
	}
	tags := textutil.Unique(heading)
	if len(tags) > 5 {
		tags = tags[:5]
	}
	return tags
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
