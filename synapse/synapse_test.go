package synapse

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Synapse467/synapse-core/capsule"
	"github.com/Synapse467/synapse-core/identity"
	"github.com/Synapse467/synapse-core/license"
	"github.com/Synapse467/synapse-core/usage"
)

func mustIdentity(t *testing.T) *identity.Identity {
	t.Helper()
	id, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func build(t *testing.T, owner *identity.Identity, open bool) *capsule.Capsule {
	t.Helper()
	items := []capsule.Item{
		{ID: "deposit", Type: capsule.Heuristic, Title: "Protect tenancy deposits within thirty days",
			Body: "A landlord must protect a tenancy deposit in an approved scheme within thirty days of receiving it.", Contributor: owner.Address(), Authored: true},
		{ID: "notice", Type: capsule.Procedure, Title: "Serve a valid possession notice",
			Steps: []string{"Use the prescribed form", "Give the required notice period", "Keep proof of service"}, Contributor: owner.Address(), Authored: true},
	}
	m := capsule.Manifest{
		Slug: "tenancy", Title: "Tenancy Law Basics", Domain: "law", Scope: "England", Version: 1, Owner: owner.Address(),
		CreatedAt: capsule.Now(), Knowledge: items,
		Evaluation: capsule.Evaluation{SuiteHash: strings.Repeat("a", 64), Cases: 5, Passed: true, CitationValidityBP: 10000, CoverageBP: 10000, AbstentionBP: 10000, AttributionBP: 10000},
		Policy:     capsule.Policy{Open: open, Purposes: []string{"research", "education"}},
	}
	c, err := capsule.Seal(m, owner)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func open(t *testing.T, c *capsule.Capsule) *Capsule {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x.capsule.json")
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func TestAnOpenCapsuleAnswersWithNoSetup(t *testing.T) {
	c := open(t, build(t, mustIdentity(t), true))
	r, err := c.Ask("When must a landlord protect a deposit?", Access{Purpose: "research"})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Decision.Allowed || !r.Answered || !strings.Contains(r.Markdown, "thirty days") {
		t.Fatalf("unexpected reply %+v", r)
	}
	if !strings.Contains(r.Markdown, "Tenancy Law Basics") {
		t.Fatal("the answer must say where it came from")
	}
}

func TestAnOpenCapsuleStillLimitsPurposeAndUse(t *testing.T) {
	c := open(t, build(t, mustIdentity(t), true))
	for name, a := range map[string]Access{
		"wrong purpose":     {Purpose: "marketing"},
		"commercial use":    {Purpose: "research", Commercial: true},
		"AI training":       {Purpose: "research", AITraining: true},
		"no purpose at all": {},
		"derivative work":   {Purpose: "research", Derivative: true},
	} {
		r, err := c.Ask("deposit", a)
		if err != nil {
			t.Fatal(err)
		}
		if r.Decision.Allowed || r.Answered {
			t.Errorf("%s was allowed", name)
		}
	}
}

func TestAClosedCapsuleNeedsALicense(t *testing.T) {
	c := open(t, build(t, mustIdentity(t), false))
	r, _ := c.Ask("deposit protection", Access{Purpose: "research"})
	if r.Decision.Allowed || r.Decision.Code != license.LicenseRequired {
		t.Fatalf("a closed capsule answered without a license: %+v", r.Decision)
	}
	if !strings.Contains(r.Markdown, "Access denied") {
		t.Fatal("the denial should be explained to the person asking")
	}
}

func TestOpeningATamperedCapsuleFails(t *testing.T) {
	owner := mustIdentity(t)
	c := build(t, owner, true)
	c.Manifest.Knowledge[0].Body = "A landlord may keep a deposit for as long as they like."
	path := filepath.Join(t.TempDir(), "x.capsule.json")
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("a capsule whose text was edited after signing was opened")
	}
}

func TestALicensedCapsuleAnswersOnlyTheLicensee(t *testing.T) {
	owner, buyer, other := mustIdentity(t), mustIdentity(t), mustIdentity(t)
	c := open(t, build(t, owner, false))
	lic, err := license.Issue(license.Terms{
		Capsule: license.CapsuleRef{Owner: owner.Address(), Slug: "tenancy"}, Grantee: buyer.Address(),
		Purposes: []string{"research"}, MaxQueries: 2,
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := c.Ask("deposit protection scheme", Access{Purpose: "research", Grantee: buyer.Address(), License: lic}); !r.Answered {
		t.Fatalf("the licensee was refused: %+v", r.Decision)
	}
	if r, _ := c.Ask("deposit protection scheme", Access{Purpose: "research", Grantee: other.Address(), License: lic}); r.Decision.Allowed {
		t.Fatal("someone else used the licensee's license")
	}
}

func TestQuotaIsCountedFromTheUsageLogAndRefusalsAreFree(t *testing.T) {
	owner, buyer := mustIdentity(t), mustIdentity(t)
	c := open(t, build(t, owner, false))
	lic, _ := license.Issue(license.Terms{
		Capsule: license.CapsuleRef{Owner: owner.Address(), Slug: "tenancy"}, Grantee: buyer.Address(),
		Purposes: []string{"research"}, MaxQueries: 2,
	}, owner)
	log, err := usage.Open(filepath.Join(t.TempDir(), "usage.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	access := Access{Purpose: "research", Grantee: buyer.Address(), License: lic, Log: log}

	// A question the capsule cannot answer must not use up the quota.
	if r, _ := c.Ask("What is the capital of Australia?", access); r.Answered || !r.Decision.Allowed {
		t.Fatalf("expected an allowed but unanswered request: %+v", r)
	}
	for i := 0; i < 2; i++ {
		if r, _ := c.Ask("deposit protection scheme", access); !r.Answered {
			t.Fatalf("question %d should be answered: %+v", i+1, r.Decision)
		}
	}
	r, _ := c.Ask("deposit protection scheme", access)
	if r.Decision.Allowed || r.Decision.Code != license.QuotaExhausted {
		t.Fatalf("the quota was not enforced: %+v", r.Decision)
	}
	if log.Count(lic.Terms.ID) != 2 {
		t.Fatalf("expected 2 billable uses, got %d", log.Count(lic.Terms.ID))
	}
	if err := log.Verify(); err != nil {
		t.Fatal(err)
	}
}

func TestTheLogNeverContainsTheQuestion(t *testing.T) {
	c := open(t, build(t, mustIdentity(t), true))
	log, _ := usage.Open(filepath.Join(t.TempDir(), "usage.jsonl"))
	secret := "our confidential client Smithson wants to evict tenants"
	c.Ask(secret, Access{Purpose: "research", Log: log})
	for _, e := range log.Events() {
		if e.QuestionSHA256 == "" || strings.Contains(e.QuestionSHA256, "Smithson") {
			t.Fatalf("unexpected event %+v", e)
		}
	}
}

func TestRevokedLicensesAreRefused(t *testing.T) {
	owner, buyer := mustIdentity(t), mustIdentity(t)
	c := open(t, build(t, owner, false))
	lic, _ := license.Issue(license.Terms{
		Capsule: license.CapsuleRef{Owner: owner.Address(), Slug: "tenancy"}, Grantee: buyer.Address(), Purposes: []string{"research"},
	}, owner)
	rev, err := license.Revoke(lic, owner, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	set := license.RevocationSet{}
	if err := set.Add(*rev); err != nil {
		t.Fatal(err)
	}
	r, _ := c.Ask("deposit", Access{Purpose: "research", Grantee: buyer.Address(), License: lic, Revoked: set})
	if r.Decision.Allowed || r.Decision.Code != license.Revoked {
		t.Fatalf("a revoked license was honoured: %+v", r.Decision)
	}
}

func TestAnAnswerIsWithheldIfTheUseCannotBeRecorded(t *testing.T) {
	owner, buyer := mustIdentity(t), mustIdentity(t)
	c := open(t, build(t, owner, false))
	lic, _ := license.Issue(license.Terms{
		Capsule: license.CapsuleRef{Owner: owner.Address(), Slug: "tenancy"}, Grantee: buyer.Address(), Purposes: []string{"research"},
	}, owner)
	dir := t.TempDir()
	log, _ := usage.Open(filepath.Join(dir, "usage.jsonl"))
	// Make the log unwritable by replacing its file with a directory.
	broken := filepath.Join(dir, "broken")
	log2, err := usage.Open(broken)
	if err != nil {
		t.Fatal(err)
	}
	_ = log
	removeAndMakeDir(t, broken)
	r, err := c.Ask("deposit protection scheme", Access{Purpose: "research", Grantee: buyer.Address(), License: lic, Log: log2})
	if err == nil || r != nil {
		t.Fatalf("an answer was given although the use could not be recorded: %v %+v", err, r)
	}
}

func TestFailedOpenReportsAReason(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("a missing file was opened")
	}
	if _, err := Parse([]byte(`{"format":"synapse.capsule/1"}`)); err == nil {
		t.Fatal("an empty capsule was accepted")
	}
}

func TestARefusedQuestionDoesNotReduceTheRemainingCount(t *testing.T) {
	owner, buyer := mustIdentity(t), mustIdentity(t)
	c := open(t, build(t, owner, false))
	lic, _ := license.Issue(license.Terms{
		Capsule: license.CapsuleRef{Owner: owner.Address(), Slug: "tenancy"}, Grantee: buyer.Address(),
		Purposes: []string{"research"}, MaxQueries: 3,
	}, owner)
	r, _ := c.Ask("What is the capital of Australia?", Access{Purpose: "research", Grantee: buyer.Address(), License: lic})
	if r.Answered || r.Decision.Remaining != 3 {
		t.Fatalf("remaining after a refusal = %d, want 3", r.Decision.Remaining)
	}
	r, _ = c.Ask("deposit protection scheme", Access{Purpose: "research", Grantee: buyer.Address(), License: lic})
	if !r.Answered || r.Decision.Remaining != 2 {
		t.Fatalf("remaining after an answer = %d, want 2", r.Decision.Remaining)
	}
}

func TestAHugeQuestionIsRefusedBeforeAnyWork(t *testing.T) {
	c := open(t, build(t, mustIdentity(t), true))
	log, _ := usage.Open(filepath.Join(t.TempDir(), "usage.jsonl"))
	_, err := c.Ask(strings.Repeat("deposit ", MaxQuestion), Access{Purpose: "research", Log: log})
	if err == nil || err != ErrQuestionTooLong {
		t.Fatalf("expected ErrQuestionTooLong, got %v", err)
	}
	if len(log.Events()) != 0 {
		t.Fatal("a refused oversized question was recorded as a use")
	}
	if _, err := c.Ask(strings.Repeat("a", MaxQuestion), Access{Purpose: "research"}); err != nil {
		t.Fatalf("a question at the limit was refused: %v", err)
	}
}
