package retrieve

import (
	"strings"
	"testing"

	"github.com/Synapse467/synapse-core/capsule"
)

const owner = "GOWNER"

func item(id string, typ capsule.ItemType, title, body string) capsule.Item {
	return capsule.Item{ID: id, Type: typ, Title: title, Body: body, Contributor: owner, Authored: true}
}

func corpus() []capsule.Item {
	rollback := item("rollback", capsule.Procedure, "Roll back a failed deployment", "")
	rollback.Steps = []string{"Stop the rollout", "Redeploy the previous image", "Confirm health checks pass"}
	rollback.Conditions = []string{"health checks fail after a release"}
	rollback.Tags = []string{"deploy", "incident"}

	keys := item("keys", capsule.Heuristic, "Rotate signing keys every ninety days", "Rotate signing keys on a fixed schedule, and immediately after any suspected exposure.")
	keys.Tags = []string{"security", "keys"}

	except := item("keys-hsm", capsule.Exception, "Hardware-backed keys", "Keys held in a hardware module are rotated by the vendor schedule instead.")
	except.AppliesTo = []string{"keys"}

	retries := item("retries", capsule.Claim, "Retries need backoff", "Retrying immediately after a failure amplifies load; use exponential backoff with jitter.")
	retries.Tags = []string{"reliability"}

	return []capsule.Item{rollback, keys, except, retries}
}

func TestAnswersWhenTheCapsuleCoversTheQuestion(t *testing.T) {
	a := NewIndex(corpus()).Ask("How do I roll back a failed deployment?", Options{})
	if !a.Answered || len(a.Results) == 0 {
		t.Fatalf("expected an answer, got %+v", a)
	}
	if a.Results[0].Item.ID != "rollback" {
		t.Fatalf("the best result should be the rollback procedure, got %s", a.Results[0].Item.ID)
	}
	if a.Results[0].Coverage < 0.9 {
		t.Fatalf("coverage should be high for a direct question, got %.2f", a.Results[0].Coverage)
	}
}

func TestAbstainsWhenTheQuestionIsOutsideTheCapsule(t *testing.T) {
	for _, q := range []string{
		"What is the capital of Australia?",
		"How do I bake sourdough bread?",
		"Should I buy bitcoin?",
	} {
		if a := NewIndex(corpus()).Ask(q, Options{}); a.Answered {
			t.Errorf("answered an uncovered question %q with %s", q, a.Results[0].Item.ID)
		}
	}
}

func TestAbstainsWhenOnlyAWeakPartOfTheQuestionMatches(t *testing.T) {
	// "deployment" is covered, but the question is mostly about something else.
	a := NewIndex(corpus()).Ask("What deployment pricing tiers does the vendor offer for enterprise invoices?", Options{})
	if a.Answered {
		t.Fatalf("a question that only shares one word must not be answered: %+v", a.Results)
	}
}

func TestEmptyAndStopwordOnlyQuestionsAbstain(t *testing.T) {
	ix := NewIndex(corpus())
	for _, q := range []string{"", "   ", "the and of", "???"} {
		a := ix.Ask(q, Options{})
		if a.Answered || a.Reason != ReasonEmpty {
			t.Errorf("question %q: %+v", q, a)
		}
	}
}

func TestAnEmptyIndexAbstains(t *testing.T) {
	if a := NewIndex(nil).Ask("anything about keys", Options{}); a.Answered {
		t.Fatal("an empty capsule answered")
	}
}

func TestExceptionsTravelWithTheirAnswers(t *testing.T) {
	a := NewIndex(corpus()).Ask("How often should signing keys be rotated?", Options{})
	if !a.Answered || a.Results[0].Item.ID != "keys" {
		t.Fatalf("unexpected answer %+v", a)
	}
	if len(a.Caveats) != 1 || a.Caveats[0].Item.ID != "keys-hsm" {
		t.Fatalf("the exception to the key rotation rule was left out: %+v", a.Caveats)
	}
}

func TestMatchingSurvivesInflectionAndWordOrder(t *testing.T) {
	a := NewIndex(corpus()).Ask("backoff when retrying failures", Options{})
	if !a.Answered || a.Results[0].Item.ID != "retries" {
		t.Fatalf("got %+v", a)
	}
}

func TestResultsAreDeterministic(t *testing.T) {
	ix := NewIndex(corpus())
	first := ix.Ask("keys rotation schedule exposure", Options{})
	for i := 0; i < 20; i++ {
		again := ix.Ask("keys rotation schedule exposure", Options{})
		if len(again.Results) != len(first.Results) || again.Results[0].Item.ID != first.Results[0].Item.ID || again.Results[0].Score != first.Results[0].Score {
			t.Fatal("the same question gave different answers")
		}
	}
}

func TestMaxResultsIsHonoured(t *testing.T) {
	var many []capsule.Item
	for _, id := range []string{"a1", "a2", "a3", "a4", "a5"} {
		many = append(many, item(id, capsule.Claim, "Widget safety rule "+id, "Always check the widget before use."))
	}
	if a := NewIndex(many).Ask("widget safety", Options{MaxResults: 2}); len(a.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(a.Results))
	}
}

func TestSuggestionsAreOffUnlessAsked(t *testing.T) {
	ix := NewIndex(corpus())
	if a := ix.Ask("quantum teleportation of keys and invoices payroll", Options{}); a.Answered || len(a.Nearest) != 0 {
		t.Fatalf("titles were revealed without being asked: %+v", a)
	}
	if a := ix.Ask("quantum teleportation of keys and invoices payroll", Options{Suggest: true}); a.Answered || len(a.Nearest) == 0 {
		t.Fatalf("expected suggestions: %+v", a)
	}
}

func TestRenderShowsExpertWordsAttributionAndCitations(t *testing.T) {
	it := item("keys", capsule.Heuristic, "Rotate signing keys", "Rotate on a schedule.")
	it.Authored = false
	it.Citations = []capsule.Citation{capsule.NewCitation("runbook", 0, 10, "Rotate keys")}
	a := Answer{Answered: true, Results: []Result{{Item: it}}}
	text := Render(a, Source{Title: "Ops Capsule", Version: 3, Hash: strings.Repeat("a", 64), Owner: owner}, map[string]string{owner: "Ada"})
	for _, want := range []string{"Rotate signing keys", "Ada (GOWNER)", "Rotate keys", "runbook", "version 3", "aaaaaaaaaaaa"} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered answer is missing %q:\n%s", want, text)
		}
	}
	if got := Render(Answer{Reason: ReasonNotCovered}, Source{}, nil); !strings.Contains(got, "Not covered") {
		t.Fatalf("abstention not rendered: %s", got)
	}
}
