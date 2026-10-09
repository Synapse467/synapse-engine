// Package retrieve answers questions from a capsule by finding the approved items that cover
// them. It does not generate text: every answer is made of the expert's own approved words,
// with their citations and attribution, and when the capsule does not cover a question the
// answer is to say so.
package retrieve

import (
	"math"
	"sort"

	"github.com/Synapse467/synapse-core/capsule"
	"github.com/Synapse467/synapse-engine/textutil"
)

// Options tune retrieval. The zero value gives the defaults.
type Options struct {
	// MinCoverage is the share of the question's distinctive terms an item must cover for the
	// capsule to answer. Below it the engine abstains. Default 0.5.
	MinCoverage float64
	// MaxResults caps how many items are returned. Default 3.
	MaxResults int
	// Suggest lists the closest topics when abstaining. Default false: a gateway may not want to
	// reveal titles to someone whose question is not covered.
	Suggest bool
}

func (o Options) withDefaults() Options {
	if o.MinCoverage <= 0 {
		o.MinCoverage = 0.5
	}
	if o.MaxResults <= 0 {
		o.MaxResults = 3
	}
	return o
}

const (
	k1 = 1.2
	b  = 0.75
)

type doc struct {
	item   capsule.Item
	tf     map[string]float64
	length float64
}

// Index is a searchable view of a capsule's items.
type Index struct {
	docs   []doc
	df     map[string]int
	avgLen float64
}

// NewIndex builds an index over items. Items are weighted by where a term appears: titles, tags
// and "applies to" count most, then conditions, then the rest.
func NewIndex(items []capsule.Item) *Index {
	ix := &Index{df: map[string]int{}}
	var total float64
	for _, item := range items {
		d := doc{item: item, tf: map[string]float64{}}
		add := func(text string, weight float64) {
			for _, term := range textutil.Terms(text) {
				d.tf[term] += weight
				d.length += weight
			}
		}
		add(item.Title, 3)
		for _, s := range item.Tags {
			add(s, 3)
		}
		for _, s := range item.AppliesTo {
			add(s, 2)
		}
		for _, s := range item.Conditions {
			add(s, 2)
		}
		add(item.Body, 1)
		for _, s := range item.Steps {
			add(s, 1)
		}
		for _, s := range item.Exceptions {
			add(s, 1)
		}
		add(item.Rationale, 1)
		for term := range d.tf {
			ix.df[term]++
		}
		total += d.length
		ix.docs = append(ix.docs, d)
	}
	if len(ix.docs) > 0 {
		ix.avgLen = total / float64(len(ix.docs))
	}
	return ix
}

// FromCapsule indexes a capsule's knowledge.
func FromCapsule(c *capsule.Capsule) *Index { return NewIndex(c.Manifest.Knowledge) }

// Vocabulary reports whether a term occurs anywhere in the index.
func (ix *Index) Vocabulary(term string) bool { return ix.df[term] > 0 }

// weight is how much a question term counts toward coverage. Words the capsule has never seen are
// usually filler ("often", "best"), so they count half as much as words it knows, which keeps
// natural questions answerable without letting a mostly-foreign question through.
func (ix *Index) weight(term string) float64 {
	if ix.df[term] == 0 {
		return ix.idf(term) * 0.5
	}
	return ix.idf(term)
}

func (ix *Index) idf(term string) float64 {
	n := float64(len(ix.docs))
	df := float64(ix.df[term])
	return math.Log(1 + (n-df+0.5)/(df+0.5))
}

// Result is one item that answers the question.
type Result struct {
	Item     capsule.Item `json:"item"`
	Score    float64      `json:"score"`
	Coverage float64      `json:"coverage"` // 0 to 1: how much of the question this item covers
	Matched  []string     `json:"matched"`  // the question terms found in the item
}

// Answer is what the engine returns. When Answered is false, Results is empty and Reason says why.
type Answer struct {
	Answered bool     `json:"answered"`
	Reason   string   `json:"reason,omitempty"`
	Results  []Result `json:"results,omitempty"`
	// Caveats are exception items that apply to the results. They are returned even if they
	// would not have matched the question on their own, because leaving out an exception can
	// turn a correct answer into a wrong one.
	Caveats []Result `json:"caveats,omitempty"`
	// Nearest lists the closest topic titles when abstaining, if Options.Suggest is set.
	Nearest []string `json:"nearest,omitempty"`
}

// Abstention reasons.
const (
	ReasonEmpty      = "the question has no searchable words"
	ReasonNotCovered = "the capsule does not cover this question"
)

// Ask finds the items that answer a question, or abstains.
func (ix *Index) Ask(question string, opts Options) Answer {
	opts = opts.withDefaults()
	terms := textutil.Unique(question)
	if len(terms) == 0 {
		return Answer{Reason: ReasonEmpty}
	}
	if len(ix.docs) == 0 {
		return Answer{Reason: ReasonNotCovered}
	}
	var totalIDF float64
	for _, t := range terms {
		totalIDF += ix.weight(t)
	}
	scored := make([]Result, 0, len(ix.docs))
	for _, d := range ix.docs {
		var score, matchedIDF float64
		var matched []string
		for _, t := range terms {
			f := d.tf[t]
			if f == 0 {
				continue
			}
			matched = append(matched, t)
			matchedIDF += ix.idf(t)
			score += ix.idf(t) * (f * (k1 + 1)) / (f + k1*(1-b+b*d.length/ix.avgLen))
		}
		coverage := 0.0
		if totalIDF > 0 {
			coverage = matchedIDF / totalIDF
		}
		scored = append(scored, Result{Item: d.item, Score: score, Coverage: coverage, Matched: matched})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].Item.ID < scored[j].Item.ID
	})

	best := scored[0]
	if best.Coverage < opts.MinCoverage {
		a := Answer{Reason: ReasonNotCovered}
		if opts.Suggest {
			for _, r := range scored {
				if r.Score <= 0 || len(a.Nearest) == 3 {
					break
				}
				a.Nearest = append(a.Nearest, r.Item.Title)
			}
		}
		return a
	}
	answer := Answer{Answered: true}
	for _, r := range scored {
		if len(answer.Results) == opts.MaxResults {
			break
		}
		// Keep only items that cover the question well and are not far weaker than the best.
		if r.Coverage >= opts.MinCoverage && r.Score >= best.Score*0.5 {
			answer.Results = append(answer.Results, r)
		}
	}
	answer.Caveats = ix.caveats(answer.Results)
	return answer
}

func (ix *Index) caveats(results []Result) []Result {
	chosen := map[string]bool{}
	tags := map[string]bool{}
	for _, r := range results {
		chosen[r.Item.ID] = true
		for _, tag := range r.Item.Tags {
			tags[tag] = true
		}
	}
	var out []Result
	for _, d := range ix.docs {
		if d.item.Type != capsule.Exception || chosen[d.item.ID] {
			continue
		}
		for _, target := range d.item.AppliesTo {
			if chosen[target] || tags[target] {
				out = append(out, Result{Item: d.item, Coverage: 1})
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Item.ID < out[j].Item.ID })
	return out
}
