package eval

import (
	"strings"
	"testing"

	"github.com/Synapse467/synapse-core/capsule"
)

const owner = "GOWNER"

const runbook = "Rotate signing keys every ninety days. Rotate immediately after any suspected exposure of a key.\n"

func items() []capsule.Item {
	start := 0
	end := len("Rotate signing keys every ninety days.")
	quote := runbook[start:end]
	keys := capsule.Item{
		ID: "keys", Type: capsule.Heuristic, Title: "Rotate signing keys every ninety days",
		Body:        "Rotate signing keys on a fixed schedule, and immediately after any suspected exposure.",
		Contributor: owner, Tags: []string{"security"},
		Citations: []capsule.Citation{capsule.NewCitation("runbook", start, end, quote)},
	}
	retries := capsule.Item{
		ID: "retries", Type: capsule.Claim, Title: "Retries need exponential backoff",
		Body:        "Retrying immediately amplifies load during an outage; use exponential backoff with random jitter.",
		Contributor: owner, Authored: true,
	}
	exception := capsule.Item{
		ID: "keys-hsm", Type: capsule.Exception, Title: "Hardware-backed keys follow the vendor schedule",
		Body: "Keys held in a hardware module are rotated by the vendor.", Contributor: owner, Authored: true,
		AppliesTo: []string{"keys"},
	}
	return []capsule.Item{keys, retries, exception}
}

func corpus() Corpus {
	return Corpus{
		Items:        items(),
		Contributors: map[string]bool{owner: true},
		Sources:      []capsule.Source{{ID: "runbook", Title: "Runbook", SHA256: capsule.HashText(runbook), Bytes: len(runbook)}},
		SourceText:   map[string][]byte{"runbook": []byte(runbook)},
	}
}

func TestGeneratedSuiteIsValidAndCoversEveryNonExceptionItem(t *testing.T) {
	s := Generate(items())
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, c := range s.Cases {
		have[c.ID] = true
	}
	for _, id := range []string{"title-keys", "title-retries", "reword-keys", "reword-retries"} {
		if !have[id] {
			t.Errorf("missing generated case %s", id)
		}
	}
	if have["title-keys-hsm"] {
		t.Error("exceptions are returned alongside their rule and should not get their own case")
	}
}

func TestGeneratedSuiteIsDeterministicAndHashed(t *testing.T) {
	a, b := Generate(items()), Generate(items())
	ha, _ := a.Hash()
	hb, _ := b.Hash()
	if ha != hb || len(ha) != 64 {
		t.Fatal("the same capsule produced different suites")
	}
	reversed := items()
	reversed[0], reversed[2] = reversed[2], reversed[0]
	if hr, _ := Generate(reversed).Hash(); hr != ha {
		t.Fatal("the suite depends on the order of the items")
	}
}

func TestGeneratedRefusalCasesNeverShareWordsWithTheCapsule(t *testing.T) {
	about := []capsule.Item{{
		ID: "bread", Type: capsule.Claim, Title: "Sourdough bread needs a mature starter", Body: "Bake bread only with a mature starter.",
		Contributor: owner, Authored: true,
	}}
	for _, c := range Generate(about).Cases {
		if c.Expect == ExpectAbstain && strings.Contains(strings.ToLower(c.Question), "sourdough") {
			t.Fatalf("a refusal case asks about the capsule's own subject: %s", c.Question)
		}
	}
}

func TestAHealthyCapsulePasses(t *testing.T) {
	r, err := Run(corpus(), Generate(items()), DefaultThresholds)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Evaluation.Passed {
		t.Fatalf("a healthy capsule failed: %+v failures=%+v", r.Evaluation, r.Failures)
	}
	e := r.Evaluation
	if e.CitationValidityBP != 10000 || e.AttributionBP != 10000 || e.AbstentionBP != 10000 {
		t.Fatalf("unexpected scores %+v", e)
	}
	if e.SuiteHash == "" || e.Cases == 0 {
		t.Fatal("the evaluation must record the suite it ran")
	}
}

func TestAWrongCitationFailsEvaluation(t *testing.T) {
	c := corpus()
	c.Items = items()
	c.Items[0].Citations[0].Start = 5 // no longer where the quote is
	r, _ := Run(c, Generate(items()), DefaultThresholds)
	if r.Evaluation.Passed || r.Evaluation.CitationValidityBP != 0 {
		t.Fatalf("a misplaced quote passed: %+v", r.Evaluation)
	}
}

func TestAChangedSourceFailsEvaluation(t *testing.T) {
	c := corpus()
	c.SourceText = map[string][]byte{"runbook": []byte(runbook + "an edit made after the fact")}
	if r, _ := Run(c, Generate(items()), DefaultThresholds); r.Evaluation.Passed {
		t.Fatal("a source that changed after being recorded passed")
	}
}

func TestAForgedQuoteFailsEvaluation(t *testing.T) {
	c := corpus()
	c.Items = items()
	c.Items[0].Citations[0].Quote = "Never rotate keys."
	if r, _ := Run(c, Generate(items()), DefaultThresholds); r.Evaluation.Passed {
		t.Fatal("a quote that does not match its hash passed")
	}
}

func TestAnUnlistedContributorFailsAttribution(t *testing.T) {
	c := corpus()
	c.Contributors = map[string]bool{}
	r, _ := Run(c, Generate(items()), DefaultThresholds)
	if r.Evaluation.Passed || r.Evaluation.AttributionBP != 0 {
		t.Fatalf("answers without a known contributor passed: %+v", r.Evaluation)
	}
}

func TestACapsuleThatAnswersEverythingFailsAbstention(t *testing.T) {
	// A capsule so broad that it matches the refusal questions too.
	broad := capsule.Item{
		ID: "all", Type: capsule.Claim, Title: "Everything", Contributor: owner, Authored: true,
		Body: "capital Australia bake sourdough bread football world cup swallow velocity Everest moons giraffe knot Mona Lisa mercury boiling planet",
	}
	suite := Suite{Format: Format, Cases: []Case{
		{ID: "a", Question: "Everything", Expect: ExpectAnswer, Items: []string{"all"}},
		{ID: "r", Question: "What is the capital of Australia?", Expect: ExpectAbstain},
	}}
	r, err := Run(Corpus{Items: []capsule.Item{broad}, Contributors: map[string]bool{owner: true}}, suite, DefaultThresholds)
	if err != nil {
		t.Fatal(err)
	}
	if r.Evaluation.Passed || r.Evaluation.AbstentionBP != 0 {
		t.Fatalf("answering an unrelated question passed: %+v", r.Evaluation)
	}
}

func TestAnExpertSuiteIsHonoured(t *testing.T) {
	suite := Suite{Format: Format, Cases: []Case{
		{ID: "q1", Question: "How often do we rotate signing keys?", Expect: ExpectAnswer, Items: []string{"keys"}},
		{ID: "q2", Question: "Do we use exponential backoff for retries?", Expect: ExpectAnswer, Items: []string{"retries"}},
		{ID: "q3", Question: "What is our parental leave policy?", Expect: ExpectAbstain},
	}}
	r, err := Run(corpus(), suite, DefaultThresholds)
	if err != nil || !r.Evaluation.Passed {
		t.Fatalf("expert suite failed: %v %+v %+v", err, r.Evaluation, r.Failures)
	}
}

func TestMalformedSuitesAreRejected(t *testing.T) {
	bad := map[string]Suite{
		"wrong format":    {Format: "nope"},
		"no cases":        {Format: Format},
		"no refusal case": {Format: Format, Cases: []Case{{ID: "a", Question: "q", Expect: ExpectAnswer, Items: []string{"x"}}}},
		"no answer case":  {Format: Format, Cases: []Case{{ID: "a", Question: "q", Expect: ExpectAbstain}}},
		"answer w/o item": {Format: Format, Cases: []Case{{ID: "a", Question: "q", Expect: ExpectAnswer}, {ID: "b", Question: "q", Expect: ExpectAbstain}}},
		"duplicate ids":   {Format: Format, Cases: []Case{{ID: "a", Question: "q", Expect: ExpectAbstain}, {ID: "a", Question: "q", Expect: ExpectAnswer, Items: []string{"x"}}}},
		"empty question":  {Format: Format, Cases: []Case{{ID: "a", Question: " ", Expect: ExpectAbstain}, {ID: "b", Question: "q", Expect: ExpectAnswer, Items: []string{"x"}}}},
		"unknown expect":  {Format: Format, Cases: []Case{{ID: "a", Question: "q", Expect: "maybe"}}},
	}
	for name, s := range bad {
		if s.Validate() == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
