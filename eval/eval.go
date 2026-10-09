// Package eval tests a capsule before it is published. A capsule only carries a passing
// evaluation if its suite of questions was run against exactly the items inside it, so a buyer
// can see that the expertise was checked, and by which suite (the suite's hash is recorded).
//
// The suite can be written by the expert, or generated from the capsule's own items. A
// generated suite checks that the capsule answers what it claims to cover and refuses what it
// does not; an expert-written suite is the stronger test and should be preferred.
package eval

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Synapse467/synapse-core/canonical"
	"github.com/Synapse467/synapse-core/capsule"
	"github.com/Synapse467/synapse-engine/retrieve"
	"github.com/Synapse467/synapse-engine/textutil"
)

// Format is the value of a suite's "format" field.
const Format = "synapse.eval/1"

// Expectations a case can have.
const (
	ExpectAnswer  = "answer"
	ExpectAbstain = "abstain"
)

// Case is one question and what a good capsule does with it.
type Case struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Expect   string   `json:"expect"`
	Items    []string `json:"items,omitempty"` // for "answer": any of these items must be among the results
}

// Suite is a set of cases.
type Suite struct {
	Format string `json:"format"`
	Cases  []Case `json:"cases"`
}

// Hash identifies a suite, so an evaluation can say exactly which questions were asked.
func (s Suite) Hash() (string, error) { return canonical.Hash(s) }

// Validate checks that a suite is well formed.
func (s Suite) Validate() error {
	if s.Format != Format {
		return fmt.Errorf("eval: the suite format must be %q", Format)
	}
	seen := map[string]bool{}
	answers, abstains := 0, 0
	for _, c := range s.Cases {
		if c.ID == "" || seen[c.ID] {
			return errors.New("eval: every case needs a unique id")
		}
		seen[c.ID] = true
		if strings.TrimSpace(c.Question) == "" {
			return fmt.Errorf("eval: case %s has no question", c.ID)
		}
		switch c.Expect {
		case ExpectAnswer:
			answers++
			if len(c.Items) == 0 {
				return fmt.Errorf("eval: case %s expects an answer but names no item", c.ID)
			}
		case ExpectAbstain:
			abstains++
		default:
			return fmt.Errorf("eval: case %s must expect %q or %q", c.ID, ExpectAnswer, ExpectAbstain)
		}
	}
	if answers == 0 || abstains == 0 {
		return errors.New("eval: a suite needs at least one case that should be answered and one that should be refused")
	}
	return nil
}

// offTopic questions are used to build refusal cases. Any that share a word with the capsule are
// dropped, so a generated refusal case can never unfairly punish a capsule that really does
// cover the subject.
var offTopic = []string{
	"What is the capital of Australia?",
	"How do I bake sourdough bread?",
	"Who won the football world cup in 1998?",
	"What is the airspeed velocity of an unladen swallow?",
	"How tall is Mount Everest?",
	"Which planet has the most moons?",
	"What does a giraffe eat?",
	"How do I tie a bowline knot?",
	"Who painted the Mona Lisa?",
	"What is the boiling point of mercury?",
}

// Generate builds a suite from a capsule's items: each item must be found by its own title and by
// a rewording made from its other words, and unrelated questions must be refused.
func Generate(items []capsule.Item) Suite {
	s := Suite{Format: Format}
	vocab := map[string]bool{}
	for _, item := range items {
		for _, t := range textutil.Terms(item.Title + " " + item.Body + " " + strings.Join(item.Tags, " ") + " " + strings.Join(item.Steps, " ")) {
			vocab[t] = true
		}
	}
	sorted := append([]capsule.Item(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	for _, item := range sorted {
		if item.Type == capsule.Exception {
			continue // exceptions are returned alongside the rule they qualify, not on their own
		}
		s.Cases = append(s.Cases, Case{ID: "title-" + item.ID, Question: item.Title, Expect: ExpectAnswer, Items: []string{item.ID}})
		if q := reword(item); q != "" {
			s.Cases = append(s.Cases, Case{ID: "reword-" + item.ID, Question: q, Expect: ExpectAnswer, Items: []string{item.ID}})
		}
	}
	n := 0
	for _, q := range offTopic {
		shared := false
		for _, t := range textutil.Terms(q) {
			if vocab[t] {
				shared = true
			}
		}
		if !shared {
			n++
			s.Cases = append(s.Cases, Case{ID: fmt.Sprintf("refuse-%02d", n), Question: q, Expect: ExpectAbstain})
		}
	}
	return s
}

// reword asks about an item using a few distinctive words from its body, in sorted order, so the
// question does not simply repeat the title.
func reword(item capsule.Item) string {
	title := map[string]bool{}
	for _, t := range textutil.Terms(item.Title) {
		title[t] = true
	}
	var picked []string
	for _, t := range textutil.Unique(item.Body) {
		if !title[t] && len(t) > 3 {
			picked = append(picked, t)
		}
		if len(picked) == 4 {
			break
		}
	}
	if len(picked) < 2 {
		return ""
	}
	sort.Strings(picked)
	return "about " + strings.Join(picked, " ")
}

// Thresholds are the scores a capsule must reach to pass, in basis points.
type Thresholds struct {
	Coverage, Abstention, CitationValidity, Attribution int
}

// DefaultThresholds are the minimums for publishing.
var DefaultThresholds = Thresholds{Coverage: 8500, Abstention: 9000, CitationValidity: 10000, Attribution: 10000}

// Failure describes one case that did not behave as expected.
type Failure struct {
	Case   string `json:"case"`
	Reason string `json:"reason"`
}

// Report is the outcome of a run.
type Report struct {
	Evaluation capsule.Evaluation `json:"evaluation"`
	Failures   []Failure          `json:"failures,omitempty"`
}

// Corpus is what gets evaluated: the items, the people who may be credited, and the sources.
type Corpus struct {
	Items        []capsule.Item
	Contributors map[string]bool   // addresses that may be named as contributors
	Sources      []capsule.Source  // recorded sources
	SourceText   map[string][]byte // the original documents by source ID, if available
}

// Run asks every question in the suite and scores the result. Quotes are checked against their
// recorded hash, and, when the original document is available, against the document itself.
func Run(corpus Corpus, suite Suite, thresholds Thresholds) (Report, error) {
	if err := suite.Validate(); err != nil {
		return Report{}, err
	}
	hash, err := suite.Hash()
	if err != nil {
		return Report{}, err
	}
	ix := retrieve.NewIndex(corpus.Items)
	var report Report
	var answerable, covered, refusable, refused, shown, attributed int

	for _, c := range suite.Cases {
		answer := ix.Ask(c.Question, retrieve.Options{})
		switch c.Expect {
		case ExpectAnswer:
			answerable++
			if !answer.Answered {
				report.Failures = append(report.Failures, Failure{c.ID, "the capsule refused a question it should answer"})
				break
			}
			found := false
			for _, r := range answer.Results {
				for _, want := range c.Items {
					if r.Item.ID == want {
						found = true
					}
				}
			}
			if !found {
				report.Failures = append(report.Failures, Failure{c.ID, "the expected item was not among the results"})
				break
			}
			covered++
			for _, r := range answer.Results {
				shown++
				if corpus.Contributors[r.Item.Contributor] && (r.Item.Authored || len(r.Item.Citations) > 0) {
					attributed++
				}
			}
		case ExpectAbstain:
			refusable++
			if answer.Answered {
				report.Failures = append(report.Failures, Failure{c.ID, "the capsule answered a question it should refuse"})
			} else {
				refused++
			}
		}
	}

	validCitations, totalCitations := checkCitations(corpus, &report)

	e := capsule.Evaluation{
		SuiteHash:          hash,
		Cases:              len(suite.Cases),
		CoverageBP:         ratio(covered, answerable),
		AbstentionBP:       ratio(refused, refusable),
		CitationValidityBP: ratio(validCitations, totalCitations),
		AttributionBP:      ratio(attributed, shown),
	}
	e.Passed = e.CoverageBP >= thresholds.Coverage && e.AbstentionBP >= thresholds.Abstention &&
		e.CitationValidityBP >= thresholds.CitationValidity && e.AttributionBP >= thresholds.Attribution
	report.Evaluation = e
	return report, nil
}

// ratio returns part/whole in basis points. An empty whole counts as perfect: there was nothing
// that could have gone wrong.
func ratio(part, whole int) int {
	if whole == 0 {
		return 10000
	}
	return part * 10000 / whole
}

func checkCitations(corpus Corpus, report *Report) (valid, total int) {
	recorded := map[string]string{}
	for _, s := range corpus.Sources {
		recorded[s.ID] = s.SHA256
	}
	for _, item := range corpus.Items {
		for i, c := range item.Citations {
			total++
			label := fmt.Sprintf("citation %d of %s", i+1, item.ID)
			if c.QuoteSHA256 != capsule.HashText(c.Quote) {
				report.Failures = append(report.Failures, Failure{label, "the quote does not match its recorded hash"})
				continue
			}
			sha, known := recorded[c.Source]
			if !known {
				report.Failures = append(report.Failures, Failure{label, "it cites a source that is not recorded"})
				continue
			}
			if text, have := corpus.SourceText[c.Source]; have {
				if capsule.HashText(string(text)) != sha {
					report.Failures = append(report.Failures, Failure{label, "the source document has changed since it was recorded"})
					continue
				}
				if c.Start < 0 || c.End > len(text) || c.Start > c.End || string(text[c.Start:c.End]) != c.Quote {
					report.Failures = append(report.Failures, Failure{label, "the quote is not at the cited place in the source"})
					continue
				}
			}
			valid++
		}
	}
	return valid, total
}
