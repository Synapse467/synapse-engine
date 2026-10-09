package gateway

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Synapse467/synapse-core/capsule"
	"github.com/Synapse467/synapse-core/identity"
	"github.com/Synapse467/synapse-core/license"
	"github.com/Synapse467/synapse-core/usage"
	"github.com/Synapse467/synapse-engine/synapse"
)

func newID(t *testing.T) *identity.Identity {
	t.Helper()
	id, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func sealed(t *testing.T, owner *identity.Identity, open bool) *synapse.Capsule {
	t.Helper()
	m := capsule.Manifest{
		Slug: "ops", Title: "Operations Notes", Domain: "ops", Scope: "demo", Version: 1, Owner: owner.Address(), CreatedAt: capsule.Now(),
		Knowledge: []capsule.Item{{
			ID: "keys", Type: capsule.Heuristic, Title: "Rotate signing keys every ninety days",
			Body: "Rotate signing keys on a fixed schedule and immediately after any suspected exposure.", Contributor: owner.Address(), Authored: true,
		}},
		Evaluation: capsule.Evaluation{SuiteHash: strings.Repeat("a", 64), Cases: 3, Passed: true, CitationValidityBP: 10000, CoverageBP: 10000, AbstentionBP: 10000, AttributionBP: 10000},
		Policy:     capsule.Policy{Open: open, Purposes: []string{"research"}},
	}
	c, err := capsule.Seal(m, owner)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := synapse.New(c)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

type rig struct {
	url     string
	owner   *identity.Identity
	buyer   *identity.Identity
	capsule *synapse.Capsule
	log     *usage.Log
	revoked license.RevocationSet
	clock   *time.Time
}

func start(t *testing.T, open bool) *rig {
	t.Helper()
	r := &rig{owner: newID(t), buyer: newID(t), revoked: license.RevocationSet{}}
	r.capsule = sealed(t, r.owner, open)
	log, err := usage.Open(filepath.Join(t.TempDir(), "usage.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	r.log = log
	now := time.Now()
	r.clock = &now
	h, err := New(Config{
		Capsule: r.capsule, Log: log,
		Revocations: func() (license.RevocationSet, error) { return r.revoked, nil },
		Now:         func() time.Time { return *r.clock },
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	r.url = srv.URL
	return r
}

func (r *rig) license(t *testing.T, queries int) *license.License {
	t.Helper()
	l, err := license.Issue(license.Terms{
		Capsule: license.CapsuleRef{Owner: r.owner.Address(), Slug: "ops"}, Grantee: r.buyer.Address(),
		Purposes: []string{"research"}, MaxQueries: queries,
	}, r.owner)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestHealthAndInfo(t *testing.T) {
	r := start(t, false)
	c := Client{BaseURL: r.url}
	info, err := c.Info(context.Background())
	if err != nil || info.Slug != "ops" || info.Hash != r.capsule.Raw.Hash {
		t.Fatalf("%v %+v", err, info)
	}
	resp, err := http.Get(r.url + "/v1/health")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("health: %v", err)
	}
	resp.Body.Close()
}

func TestInfoDoesNotRevealKnowledge(t *testing.T) {
	r := start(t, false)
	resp, err := http.Get(r.url + "/v1/capsule")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	if strings.Contains(buf.String(), "ninety days") {
		t.Fatal("the public summary leaked the capsule's knowledge")
	}
}

func TestLicenseeIsAnswered(t *testing.T) {
	r := start(t, false)
	lic := r.license(t, 0)
	reply, err := Client{BaseURL: r.url}.Ask(context.Background(), r.buyer, lic, "research", "How often should signing keys be rotated?", Use{})
	if err != nil {
		t.Fatal(err)
	}
	if !reply.Answered || !strings.Contains(reply.Markdown, "ninety days") {
		t.Fatalf("unexpected reply %+v", reply)
	}
	if r.log.Count(lic.Terms.ID) != 1 {
		t.Fatal("the gateway did not record the use")
	}
}

func TestAnUnsignedRequestToAClosedCapsuleIsRefused(t *testing.T) {
	r := start(t, false)
	resp, err := http.Post(r.url+"/v1/ask", "application/json", strings.NewReader(`{"question":"keys","purpose":"research"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestOpenCapsulesNeedNoIdentity(t *testing.T) {
	r := start(t, true)
	resp, err := http.Post(r.url+"/v1/ask", "application/json", strings.NewReader(`{"question":"how are signing keys rotated","purpose":"research"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	resp, _ = http.Post(r.url+"/v1/ask", "application/json", strings.NewReader(`{"question":"signing keys","purpose":"marketing"}`))
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a purpose outside the policy got status %d", resp.StatusCode)
	}
}

func TestSomeoneElseCannotUseALicense(t *testing.T) {
	r := start(t, false)
	lic := r.license(t, 0)
	thief := newID(t)
	_, err := Client{BaseURL: r.url}.Ask(context.Background(), thief, lic, "research", "signing keys", Use{})
	var remote *RemoteError
	if !errors.As(err, &remote) || remote.Status != http.StatusForbidden || remote.Body.Code != string(license.WrongGrantee) {
		t.Fatalf("a license was used by someone it was not granted to: %v", err)
	}
}

func TestQuotaAndRevocationAreEnforcedByTheGateway(t *testing.T) {
	r := start(t, false)
	lic := r.license(t, 2)
	c := Client{BaseURL: r.url}
	for i := 0; i < 2; i++ {
		if _, err := c.Ask(context.Background(), r.buyer, lic, "research", "how are signing keys rotated", Use{}); err != nil {
			t.Fatalf("question %d: %v", i+1, err)
		}
	}
	_, err := c.Ask(context.Background(), r.buyer, lic, "research", "how are signing keys rotated", Use{})
	var remote *RemoteError
	if !errors.As(err, &remote) || remote.Body.Code != string(license.QuotaExhausted) {
		t.Fatalf("the quota was not enforced: %v", err)
	}

	lic2 := r.license(t, 0)
	rev, _ := license.Revoke(lic2, r.owner, *r.clock)
	r.revoked.Add(*rev)
	_, err = c.Ask(context.Background(), r.buyer, lic2, "research", "how are signing keys rotated", Use{})
	if !errors.As(err, &remote) || remote.Body.Code != string(license.Revoked) {
		t.Fatalf("a revoked license was honoured: %v", err)
	}
}

func TestDeclaredUseIsChecked(t *testing.T) {
	r := start(t, false)
	lic := r.license(t, 0) // not licensed for commercial use
	_, err := Client{BaseURL: r.url}.Ask(context.Background(), r.buyer, lic, "research", "signing keys", Use{Commercial: true})
	var remote *RemoteError
	if !errors.As(err, &remote) || remote.Body.Code != string(license.CommercialNotAllowed) {
		t.Fatalf("commercial use was not refused: %v", err)
	}
}

func TestReplayedRequestsAreRefused(t *testing.T) {
	r := start(t, false)
	lic := r.license(t, 0)
	signed, _ := license.NewRequest(r.buyer, r.capsule.Raw.Hash, lic.Hash, "research", "how are signing keys rotated", *r.clock)
	body := AskBody{Request: signed, License: lic}
	post := func() int {
		var buf bytes.Buffer
		buf.WriteString(mustJSON(t, body))
		resp, err := http.Post(r.url+"/v1/ask", "application/json", &buf)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := post(); code != 200 {
		t.Fatalf("first use: %d", code)
	}
	if code := post(); code != http.StatusConflict {
		t.Fatalf("a captured request was accepted twice: %d", code)
	}
}

func TestOldAndForgedRequestsAreRefused(t *testing.T) {
	r := start(t, false)
	lic := r.license(t, 0)
	old, _ := license.NewRequest(r.buyer, r.capsule.Raw.Hash, lic.Hash, "research", "keys", r.clock.Add(-time.Hour))
	forged, _ := license.NewRequest(r.buyer, r.capsule.Raw.Hash, lic.Hash, "research", "keys", *r.clock)
	forged.Question = "a different question than was signed"
	for name, req := range map[string]*license.SignedRequest{"old": old, "forged": forged} {
		resp, err := http.Post(r.url+"/v1/ask", "application/json", strings.NewReader(mustJSON(t, AskBody{Request: req, License: lic})))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s request got status %d", name, resp.StatusCode)
		}
	}
}

func TestRequestsForAnotherVersionAreRefused(t *testing.T) {
	r := start(t, false)
	lic := r.license(t, 0)
	signed, _ := license.NewRequest(r.buyer, strings.Repeat("b", 64), lic.Hash, "research", "keys", *r.clock)
	resp, _ := http.Post(r.url+"/v1/ask", "application/json", strings.NewReader(mustJSON(t, AskBody{Request: signed, License: lic})))
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestMalformedAndOversizedBodiesAreRefused(t *testing.T) {
	r := start(t, true)
	for name, body := range map[string]string{
		"not json":      "nope",
		"unknown field": `{"question":"x","purpose":"research","surprise":1}`,
		"trailing":      `{"question":"x","purpose":"research"} {}`,
		"no question":   `{"purpose":"research"}`,
		"too big":       `{"question":"` + strings.Repeat("a", MaxBody+10) + `"}`,
	} {
		resp, err := http.Post(r.url+"/v1/ask", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode < 400 || resp.StatusCode >= 500 {
			t.Errorf("%s: status %d", name, resp.StatusCode)
		}
	}
}

func TestNewRequiresACapsuleAndALog(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("a gateway with nothing to serve was created")
	}
}

func TestOnlyDeclaredRoutesExist(t *testing.T) {
	r := start(t, true)
	for _, path := range []string{"/v1/ask", "/", "/v1/capsule/knowledge", "/v1/usage"} {
		resp, err := http.Get(r.url + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == 200 {
			t.Errorf("GET %s unexpectedly succeeded", path)
		}
	}
}

func TestAnOversizedQuestionIsABadRequest(t *testing.T) {
	r := start(t, true)
	body := `{"question":"` + strings.Repeat("deposit ", synapse.MaxQuestion) + `","purpose":"research"}`
	resp, err := http.Post(r.url+"/v1/ask", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}
}
