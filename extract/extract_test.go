package extract

import (
	"strings"
	"testing"

	"github.com/Synapse467/synapse-core/capsule"
)

const doc = `# Incident response

## Rolling back

If health checks fail after a release, follow these steps:

1. Stop the rollout.
2. Redeploy the previous image.
3. Confirm the health checks pass again.

## Keys

Always rotate signing keys every ninety days. This does not apply to keys held in a hardware module, which follow the vendor schedule.

Never share a signing key between two services.

Signing keys are the root of trust for every release the team publishes.

Example: In March a leaked key was rotated within an hour, and no release had to be recalled because the rotation ran before the next publish.

Thanks everyone for reading.
`

func byType(ps []Proposal, typ capsule.ItemType) []Proposal {
	var out []Proposal
	for _, p := range ps {
		if p.Item.Type == typ {
			out = append(out, p)
		}
	}
	return out
}

func TestFindsEachKindOfItem(t *testing.T) {
	ps := FromText("runbook", doc, "GOWNER")
	if len(byType(ps, capsule.Procedure)) != 1 {
		t.Fatalf("expected one procedure, got %d in %+v", len(byType(ps, capsule.Procedure)), ps)
	}
	if len(byType(ps, capsule.Heuristic)) < 2 {
		t.Fatalf("expected the two rules, got %d", len(byType(ps, capsule.Heuristic)))
	}
	if len(byType(ps, capsule.Case)) != 1 {
		t.Fatalf("expected one example, got %d", len(byType(ps, capsule.Case)))
	}
	if len(byType(ps, capsule.Claim)) != 1 {
		t.Fatalf("expected one claim, got %d", len(byType(ps, capsule.Claim)))
	}
}

func TestProcedureKeepsStepsConditionAndHeadingTags(t *testing.T) {
	p := byType(FromText("runbook", doc, "GOWNER"), capsule.Procedure)[0].Item
	if len(p.Steps) != 3 || p.Steps[1] != "Redeploy the previous image." {
		t.Fatalf("unexpected steps %v", p.Steps)
	}
	if len(p.Conditions) != 1 || !strings.HasPrefix(p.Conditions[0], "If health checks fail") {
		t.Fatalf("unexpected conditions %v", p.Conditions)
	}
	if len(p.Tags) == 0 {
		t.Fatal("the section heading should become tags")
	}
}

func TestAnExceptionIsAttachedToTheRuleItQualifies(t *testing.T) {
	var rule *capsule.Item
	for _, p := range FromText("runbook", doc, "GOWNER") {
		if strings.Contains(p.Item.Body, "ninety days") {
			item := p.Item
			rule = &item
		}
	}
	if rule == nil {
		t.Fatal("the rotation rule was not found")
	}
	if len(rule.Exceptions) != 1 || !strings.Contains(rule.Exceptions[0], "hardware module") {
		t.Fatalf("the exception was not attached: %v", rule.Exceptions)
	}
	if len(rule.Citations) != 2 {
		t.Fatalf("the exception's words should be cited too, got %d citations", len(rule.Citations))
	}
}

func TestEveryCitationPointsAtTheExactBytes(t *testing.T) {
	for _, p := range FromText("runbook", doc, "GOWNER") {
		for _, c := range p.Item.Citations {
			if doc[c.Start:c.End] != c.Quote {
				t.Errorf("citation of %q does not match the source bytes: %q", p.Item.Title, c.Quote)
			}
			if c.QuoteSHA256 != capsule.HashText(c.Quote) {
				t.Errorf("citation hash is wrong for %q", p.Item.Title)
			}
		}
	}
}

func TestOffsetsAreBytesEvenWithMultibyteText(t *testing.T) {
	text := "# Café\n\nAlways grind the beans fresh, because stale beans taste flat. Über-fresh is best.\n"
	for _, p := range FromText("s", text, "G") {
		for _, c := range p.Item.Citations {
			if text[c.Start:c.End] != c.Quote {
				t.Fatalf("multi-byte offsets are wrong: %q", c.Quote)
			}
		}
	}
}

func TestEveryProposalIsAValidItem(t *testing.T) {
	for _, p := range FromText("runbook", doc, "GOWNER") {
		item := p.Item
		item.ID = capsule.ItemID(item)
		if issues := item.Validate(); len(issues) > 0 {
			t.Errorf("%q is not valid: %v", p.Item.Title, issues)
		}
		if p.Confidence != "extracted" {
			t.Errorf("confidence = %q", p.Confidence)
		}
		if p.Item.Contributor != "GOWNER" {
			t.Errorf("contributor = %q", p.Item.Contributor)
		}
	}
}

func TestSmallTalkIsIgnored(t *testing.T) {
	for _, p := range FromText("runbook", doc, "GOWNER") {
		if strings.Contains(p.Item.Body, "Thanks everyone") {
			t.Fatal("small talk became an item")
		}
	}
}

func TestEmptyAndStructurelessInputYieldNothing(t *testing.T) {
	for _, text := range []string{"", "\n\n\n", "# Only a heading", "ok.", "word"} {
		if ps := FromText("s", text, "G"); len(ps) != 0 {
			t.Errorf("%q produced %d proposals", text, len(ps))
		}
	}
}

func TestExtractionIsDeterministic(t *testing.T) {
	a, b := FromText("runbook", doc, "G"), FromText("runbook", doc, "G")
	if len(a) != len(b) {
		t.Fatal("different number of proposals")
	}
	for i := range a {
		if capsule.ItemID(a[i].Item) != capsule.ItemID(b[i].Item) {
			t.Fatalf("proposal %d differs between runs", i)
		}
	}
}

func TestWindowsLineEndingsAreHandled(t *testing.T) {
	crlf := strings.ReplaceAll(doc, "\n", "\r\n")
	ps := FromText("runbook", crlf, "G")
	if len(byType(ps, capsule.Procedure)) != 1 {
		t.Fatalf("CRLF input lost the procedure: %d proposals", len(ps))
	}
	for _, p := range ps {
		for _, c := range p.Item.Citations {
			if crlf[c.Start:c.End] != c.Quote {
				t.Fatalf("CRLF offsets are wrong for %q", c.Quote)
			}
		}
	}
}
