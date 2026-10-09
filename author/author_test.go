package author

import (
	"strings"
	"testing"
	"time"

	"github.com/Synapse467/synapse-core/capsule"
	"github.com/Synapse467/synapse-core/identity"
	"github.com/Synapse467/synapse-engine/eval"
	"github.com/Synapse467/synapse-engine/synapse"
)

const runbook = `# Key management

Always rotate signing keys every ninety days. This does not apply to keys held in a hardware module, which follow the vendor schedule.

Never share a signing key between two services.

## Rolling back

If health checks fail after a release, follow these steps:

1. Stop the rollout.
2. Redeploy the previous image.
3. Confirm the health checks pass again.

Retries without backoff are dangerous because they multiply load during an outage for every caller.
`

func newDraft(t *testing.T) (*capsule.Draft, *identity.Identity) {
	t.Helper()
	owner, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	d, err := capsule.NewDraft("ops", "Ops Notes", "ops", "demo", owner.Address())
	if err != nil {
		t.Fatal(err)
	}
	d.Policy = capsule.Policy{Open: true, Purposes: []string{"research"}}
	return d, owner
}

func approveAll(t *testing.T, d *capsule.Draft) {
	t.Helper()
	for _, item := range d.Items {
		if err := d.Approve(item.ID); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAddDocumentRecordsTheSourceByHashAndProposesItems(t *testing.T) {
	d, _ := newDraft(t)
	got, err := AddDocument(d, "runbook", "Runbook", []byte(runbook))
	if err != nil {
		t.Fatal(err)
	}
	if got.Proposed < 3 || len(d.Items) != got.Proposed {
		t.Fatalf("unexpected result %+v with %d items", got, len(d.Items))
	}
	if len(d.Sources) != 1 || d.Sources[0].SHA256 != capsule.HashText(runbook) || d.Sources[0].Bytes != len(runbook) {
		t.Fatalf("source not recorded by hash: %+v", d.Sources)
	}
	for _, item := range d.Items {
		if item.Status != capsule.Pending {
			t.Fatalf("a proposal must wait for the expert's approval, got %s", item.Status)
		}
	}
}

func TestAddingTheSameDocumentTwiceAddsNothing(t *testing.T) {
	d, _ := newDraft(t)
	first, _ := AddDocument(d, "runbook", "Runbook", []byte(runbook))
	second, err := AddDocument(d, "runbook", "Runbook", []byte(runbook))
	if err != nil {
		t.Fatal(err)
	}
	if second.Proposed != 0 || second.Existing != first.Proposed {
		t.Fatalf("second ingest: %+v first: %+v", second, first)
	}
}

func TestAChangedDocumentWithTheSameIDIsRefused(t *testing.T) {
	d, _ := newDraft(t)
	AddDocument(d, "runbook", "Runbook", []byte(runbook))
	if _, err := AddDocument(d, "runbook", "Runbook", []byte(runbook+"\nExtra line.")); err == nil {
		t.Fatal("a different document under the same id was accepted")
	}
}

func TestEmptyDocumentsAreRefused(t *testing.T) {
	d, _ := newDraft(t)
	if _, err := AddDocument(d, "x", "X", []byte("  \n ")); err == nil {
		t.Fatal("an empty document was accepted")
	}
}

func TestNothingPublishesUntilItemsAreApproved(t *testing.T) {
	d, owner := newDraft(t)
	AddDocument(d, "runbook", "Runbook", []byte(runbook))
	if _, _, err := Publish(d, owner, nil, map[string][]byte{"runbook": []byte(runbook)}, time.Now(), eval.DefaultThresholds); err == nil {
		t.Fatal("a draft with no approved items was published")
	}
}

func TestPublishProducesAVerifiableCapsuleAndAdvancesTheDraft(t *testing.T) {
	d, owner := newDraft(t)
	AddDocument(d, "runbook", "Runbook", []byte(runbook))
	approveAll(t, d)
	texts := map[string][]byte{"runbook": []byte(runbook)}

	c, report, err := Publish(d, owner, nil, texts, time.Now(), eval.DefaultThresholds)
	if err != nil {
		t.Fatalf("publish failed: %v (%+v)", err, report.Failures)
	}
	if !report.Evaluation.Passed || c.Manifest.Version != 1 || d.Published != 1 || d.PreviousHash != c.Hash {
		t.Fatalf("unexpected outcome: %+v version=%d", report.Evaluation, c.Manifest.Version)
	}
	opened, err := synapse.New(c)
	if err != nil {
		t.Fatalf("the published capsule does not verify: %v", err)
	}
	reply, _ := opened.Ask("How do I roll back after failed health checks?", synapse.Access{Purpose: "research"})
	if !reply.Answered || !strings.Contains(reply.Markdown, "Redeploy the previous image") {
		t.Fatalf("the published capsule does not answer: %+v", reply)
	}

	// A second publish continues the chain.
	d.Items[0].Body = d.Items[0].Body + " (clarified)"
	c2, _, err := Publish(d, owner, nil, texts, time.Now().Add(time.Hour), eval.DefaultThresholds)
	if err != nil {
		t.Fatal(err)
	}
	if c2.Manifest.Version != 2 || c2.Manifest.Previous != c.Hash {
		t.Fatalf("version 2 does not continue version 1: %+v", c2.Manifest)
	}
	if r := capsule.Verify(c2, c); !r.OK {
		t.Fatalf("version 2 does not verify against version 1: %v", r.Issues)
	}
}

func TestPublishRefusesWhenTheSourceNoLongerMatches(t *testing.T) {
	d, owner := newDraft(t)
	AddDocument(d, "runbook", "Runbook", []byte(runbook))
	approveAll(t, d)
	_, report, err := Publish(d, owner, nil, map[string][]byte{"runbook": []byte(runbook + "edited later")}, time.Now(), eval.DefaultThresholds)
	if err == nil || report.Evaluation.Passed {
		t.Fatal("published although a cited document had changed")
	}
	if d.Published != 0 {
		t.Fatal("a failed publish must not advance the draft")
	}
}

func TestOnlyTheOwnerCanPublish(t *testing.T) {
	d, _ := newDraft(t)
	AddDocument(d, "runbook", "Runbook", []byte(runbook))
	approveAll(t, d)
	stranger, _ := identity.Generate()
	if _, _, err := Publish(d, stranger, nil, nil, time.Now(), eval.DefaultThresholds); err == nil {
		t.Fatal("someone other than the owner published")
	}
}

func TestRejectedItemsNeverReachTheCapsule(t *testing.T) {
	d, owner := newDraft(t)
	AddDocument(d, "runbook", "Runbook", []byte(runbook))
	approveAll(t, d)
	rejected := d.Items[0]
	if err := d.Reject(rejected.ID); err != nil {
		t.Fatal(err)
	}
	c, _, err := Publish(d, owner, nil, map[string][]byte{"runbook": []byte(runbook)}, time.Now(), eval.DefaultThresholds)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := c.Item(rejected.ID); found {
		t.Fatal("a rejected item was published")
	}
}
