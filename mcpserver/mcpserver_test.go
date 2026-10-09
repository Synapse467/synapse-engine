package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Synapse467/synapse-core/capsule"
	"github.com/Synapse467/synapse-core/identity"
	"github.com/Synapse467/synapse-engine/synapse"
)

func testCapsule(t *testing.T, slug string, open bool) *synapse.Capsule {
	t.Helper()
	owner, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	c, err := capsule.Seal(capsule.Manifest{
		Slug: slug, Title: "Notes " + slug, Domain: "ops", Scope: "demo", Version: 1, Owner: owner.Address(), CreatedAt: capsule.Now(),
		Knowledge: []capsule.Item{{
			ID: "keys", Type: capsule.Heuristic, Title: "Rotate signing keys every ninety days",
			Body: "Rotate signing keys on a fixed schedule.", Contributor: owner.Address(), Authored: true,
		}},
		Evaluation: capsule.Evaluation{SuiteHash: strings.Repeat("a", 64), Cases: 3, Passed: true, CitationValidityBP: 10000, CoverageBP: 10000, AbstentionBP: 10000, AttributionBP: 10000},
		Policy:     capsule.Policy{Open: open, Purposes: []string{"research", "assistant"}},
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	s, err := synapse.New(c)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func run(t *testing.T, s *Server, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := s.Serve(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var replies []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("not JSON: %q", line)
		}
		replies = append(replies, m)
	}
	return replies
}

func toolText(t *testing.T, reply map[string]any) (string, bool) {
	t.Helper()
	result := reply["result"].(map[string]any)
	content := result["content"].([]any)[0].(map[string]any)
	return content["text"].(string), result["isError"].(bool)
}

func server(t *testing.T, open bool, slugs ...string) *Server {
	s := &Server{Name: "synapse", Version: "test", DefaultPurpose: "assistant"}
	for _, slug := range slugs {
		s.Capsules = append(s.Capsules, testCapsule(t, slug, open))
	}
	return s
}

func TestInitializeListAndCall(t *testing.T) {
	s := server(t, true, "ops")
	replies := run(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"synapse_ask","arguments":{"question":"How often are signing keys rotated?"}}}`,
	)
	if len(replies) != 3 {
		t.Fatalf("a notification must not get a reply; got %d replies", len(replies))
	}
	init := replies[0]["result"].(map[string]any)
	if init["protocolVersion"] != ProtocolVersion || init["serverInfo"].(map[string]any)["name"] != "synapse" {
		t.Fatalf("unexpected initialize result %+v", init)
	}
	tools := replies[1]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	text, isErr := toolText(t, replies[2])
	if isErr || !strings.Contains(text, "ninety days") {
		t.Fatalf("unexpected answer (error=%v): %s", isErr, text)
	}
}

func TestAnUncoveredQuestionIsNotAnErrorButSaysSo(t *testing.T) {
	replies := run(t, server(t, true, "ops"),
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"synapse_ask","arguments":{"question":"What is the capital of Australia?"}}}`)
	text, isErr := toolText(t, replies[0])
	if isErr || !strings.Contains(text, "Not covered") {
		t.Fatalf("error=%v text=%s", isErr, text)
	}
}

func TestLicensingStillAppliesThroughTheAgent(t *testing.T) {
	replies := run(t, server(t, false, "ops"),
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"synapse_ask","arguments":{"question":"signing keys"}}}`)
	text, isErr := toolText(t, replies[0])
	if !isErr || !strings.Contains(text, "Access denied") {
		t.Fatalf("a closed capsule answered an agent without a license: %s", text)
	}
}

func TestSeveralCapsulesNeedAChoice(t *testing.T) {
	s := server(t, true, "ops", "sec")
	replies := run(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"synapse_ask","arguments":{"question":"signing keys"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"synapse_ask","arguments":{"question":"signing keys","capsule":"sec"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"synapse_ask","arguments":{"question":"signing keys","capsule":"nope"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"synapse_capsules"}}`,
	)
	if _, isErr := toolText(t, replies[0]); !isErr {
		t.Error("an ambiguous call was answered")
	}
	if text, isErr := toolText(t, replies[1]); isErr || !strings.Contains(text, "ninety days") {
		t.Errorf("a named capsule was not answered: %s", text)
	}
	if _, isErr := toolText(t, replies[2]); !isErr {
		t.Error("an unknown capsule was answered")
	}
	if text, _ := toolText(t, replies[3]); !strings.Contains(text, "ops") || !strings.Contains(text, "sec") {
		t.Errorf("listing is incomplete: %s", text)
	}
}

func TestProtocolErrors(t *testing.T) {
	replies := run(t, server(t, true, "ops"),
		`not json`,
		`{"jsonrpc":"2.0","id":1,"method":"resources/list"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"unknown"}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":"bad"}`,
		`{"jsonrpc":"2.0","id":4,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"synapse_ask","arguments":{}}}`,
	)
	codes := []float64{-32700, -32601, -32602, -32602}
	for i, want := range codes {
		got := replies[i]["error"].(map[string]any)["code"].(float64)
		if got != want {
			t.Errorf("reply %d: code %v want %v", i, got, want)
		}
	}
	if _, ok := replies[4]["result"]; !ok {
		t.Error("ping must succeed")
	}
	if _, isErr := toolText(t, replies[5]); !isErr {
		t.Error("a call with no question must be reported as an error")
	}
}

func TestIdsOfAnyTypeAreEchoed(t *testing.T) {
	replies := run(t, server(t, true, "ops"), `{"jsonrpc":"2.0","id":"abc","method":"ping"}`)
	if replies[0]["id"] != "abc" {
		t.Fatalf("id = %v", replies[0]["id"])
	}
}

func TestNeedsAtLeastOneCapsule(t *testing.T) {
	if err := (&Server{}).Serve(context.Background(), strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Fatal("a server with nothing to serve started")
	}
}

func TestAccessHookReceivesTheStatedPurpose(t *testing.T) {
	s := server(t, true, "ops")
	var got string
	s.Access = func(c *synapse.Capsule, purpose string) synapse.Access {
		got = purpose
		return synapse.Access{Purpose: purpose}
	}
	run(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"synapse_ask","arguments":{"question":"signing keys","purpose":"research"}}}`)
	if got != "research" {
		t.Fatalf("purpose = %q", got)
	}
	run(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"synapse_ask","arguments":{"question":"signing keys"}}}`)
	if got != "assistant" {
		t.Fatalf("the default purpose was not used: %q", got)
	}
}
