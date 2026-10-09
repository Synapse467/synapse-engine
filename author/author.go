// Package author turns documents and an expert's decisions into a published capsule. It is the
// code behind `synapse add`, `synapse eval` and `synapse publish`, and can be used on its own to
// build capsules from other programs.
package author

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Synapse467/synapse-core/capsule"
	"github.com/Synapse467/synapse-core/identity"
	"github.com/Synapse467/synapse-engine/eval"
	"github.com/Synapse467/synapse-engine/extract"
)

// Ingested describes what adding a document did.
type Ingested struct {
	Source   capsule.Source
	Proposed int // items added to the draft for review
	Existing int // items already in the draft
	Skipped  int // proposals that could not form a valid item
}

// AddDocument records a document as a source (by hash only) and proposes the items found in it.
// The text is the exact bytes of the file, because citation offsets refer to them. Nothing
// proposed is part of the capsule until the expert approves it.
func AddDocument(d *capsule.Draft, id, title string, text []byte) (*Ingested, error) {
	if strings.TrimSpace(string(text)) == "" {
		return nil, errors.New("author: the document is empty")
	}
	source := capsule.Source{ID: id, Title: title, SHA256: capsule.HashText(string(text)), Bytes: len(text)}
	if err := d.AddSource(source); err != nil {
		return nil, err
	}
	out := &Ingested{Source: source}
	for _, p := range extract.FromText(id, string(text), d.Owner) {
		_, err := d.Propose(p.Item, p.Confidence)
		switch {
		case err == nil:
			out.Proposed++
		case strings.Contains(err.Error(), "already in the draft"):
			out.Existing++
		default:
			out.Skipped++
		}
	}
	return out, nil
}

// Corpus is the evaluation input for a draft: its approved items and the sources they cite.
func Corpus(d *capsule.Draft, texts map[string][]byte) eval.Corpus {
	c := eval.Corpus{Contributors: map[string]bool{d.Owner: true}, Sources: d.Sources, SourceText: texts}
	for _, contributor := range d.Contributors {
		c.Contributors[contributor.Address] = true
	}
	for _, item := range d.Items {
		if item.Status == capsule.Approved {
			c.Items = append(c.Items, item.Item)
		}
	}
	return c
}

// Evaluate runs a suite against the draft's approved items. With no suite it generates one.
func Evaluate(d *capsule.Draft, suite *eval.Suite, texts map[string][]byte, th eval.Thresholds) (eval.Report, eval.Suite, error) {
	corpus := Corpus(d, texts)
	if len(corpus.Items) == 0 {
		return eval.Report{}, eval.Suite{}, errors.New("author: no items are approved yet")
	}
	var s eval.Suite
	if suite != nil {
		s = *suite
	} else {
		s = eval.Generate(corpus.Items)
	}
	report, err := eval.Run(corpus, s, th)
	return report, s, err
}

// Publish evaluates the draft, and only if it passes, seals the next version signed by the owner and
// marks it published in the draft. The returned report is always set, so a failure can be explained.
func Publish(d *capsule.Draft, owner *identity.Identity, suite *eval.Suite, texts map[string][]byte, now time.Time, th eval.Thresholds) (*capsule.Capsule, eval.Report, error) {
	if owner.Address() != d.Owner {
		return nil, eval.Report{}, errors.New("author: only the draft's owner can publish it")
	}
	report, _, err := Evaluate(d, suite, texts, th)
	if err != nil {
		return nil, report, err
	}
	if !report.Evaluation.Passed {
		return nil, report, fmt.Errorf("author: the capsule did not pass its evaluation (coverage %s, refusals %s, citations %s, attribution %s)",
			pct(report.Evaluation.CoverageBP), pct(report.Evaluation.AbstentionBP),
			pct(report.Evaluation.CitationValidityBP), pct(report.Evaluation.AttributionBP))
	}
	manifest, err := d.Build(now.UTC().Format(time.RFC3339), report.Evaluation)
	if err != nil {
		return nil, report, err
	}
	c, err := capsule.Seal(manifest, owner)
	if err != nil {
		return nil, report, err
	}
	d.MarkPublished(c)
	return c, report, nil
}

func pct(bp int) string { return fmt.Sprintf("%d.%02d%%", bp/100, bp%100) }
