// Package mcpserver exposes capsules to AI agents as Model Context Protocol tools, over stdio.
//
// An agent such as Claude Desktop, Claude Code or Cursor can then consult an expert's capsule
// the way it would call any tool, and the answer comes back with the expert's own wording,
// citations and attribution. Nothing is generated: no model, API key or network is involved on the
// capsule's side. Licensing still applies, because every call goes through synapse.Capsule.Ask.
package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/Synapse467/synapse-engine/synapse"
)

// ProtocolVersion is the MCP revision this server speaks by default. If a client asks for another
// revision the server answers with its own, as the protocol specifies.
const ProtocolVersion = "2025-06-18"

// MaxLine bounds one incoming message.
const MaxLine = 1 << 20

// Server serves one or more capsules.
type Server struct {
	Name    string
	Version string
	// Capsules are the capsules the agent may consult.
	Capsules []*synapse.Capsule
	// Access builds the access for a call: who is asking, with which license, logging where. The
	// purpose the agent states is passed in, and a default is used when it states none.
	Access func(c *synapse.Capsule, purpose string) synapse.Access
	// DefaultPurpose is used when the agent does not say what the answer is for.
	DefaultPurpose string
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Serve reads newline-delimited JSON-RPC messages from in and writes replies to out until in
// closes or ctx ends.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	if len(s.Capsules) == 0 {
		return errors.New("mcpserver: there are no capsules to serve")
	}
	var mu sync.Mutex
	send := func(r response) {
		mu.Lock()
		defer mu.Unlock()
		data, _ := json.Marshal(r)
		out.Write(append(data, '\n'))
	}
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), MaxLine)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			send(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}})
			continue
		}
		isNotification := len(req.ID) == 0
		result, rpcErr := s.handle(req)
		if isNotification {
			continue
		}
		send(response{JSONRPC: "2.0", ID: req.ID, Result: result, Error: rpcErr})
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func (s *Server) handle(req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := ProtocolVersion
		if p.ProtocolVersion != "" && p.ProtocolVersion == ProtocolVersion {
			version = p.ProtocolVersion
		}
		name, ver := s.Name, s.Version
		if name == "" {
			name = "synapse"
		}
		if ver == "" {
			ver = "dev"
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": name, "version": ver},
			"instructions":    "Consult an expert's capsule with synapse_ask. Answers are the expert's own approved words; when the capsule does not cover a question it says so, and you should not guess.",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": s.tools()}, nil
	case "tools/call":
		return s.call(req.Params)
	case "notifications/initialized", "notifications/cancelled":
		return nil, nil
	default:
		return nil, &rpcError{-32601, "method not found"}
	}
}

func (s *Server) tools() []map[string]any {
	slugs := make([]string, 0, len(s.Capsules))
	for _, c := range s.Capsules {
		slugs = append(slugs, c.Raw.Manifest.Slug)
	}
	return []map[string]any{
		{
			"name":        "synapse_capsules",
			"description": "List the expertise capsules available, with who owns each and what it covers.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			"name": "synapse_ask",
			"description": "Ask a question of an expertise capsule. The answer is the expert's own approved guidance with " +
				"citations and attribution, or a statement that the capsule does not cover the question. " +
				"Do not answer from your own knowledge when it says it is not covered.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"question"},
				"properties": map[string]any{
					"question": map[string]any{"type": "string", "description": "The question to ask."},
					"capsule":  map[string]any{"type": "string", "description": "Which capsule to ask (its slug). Optional when only one is available.", "enum": slugs},
					"purpose":  map[string]any{"type": "string", "description": "What the answer will be used for, such as research or education. Licenses limit purposes."},
				},
			},
		},
	}
}

func (s *Server) call(raw json.RawMessage) (any, *rpcError) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &rpcError{-32602, "invalid parameters"}
	}
	switch p.Name {
	case "synapse_capsules":
		var b strings.Builder
		for _, c := range s.Capsules {
			i := c.Info()
			fmt.Fprintf(&b, "- %s (%s) v%d by %s: %s — %s. %d items.\n", i.Slug, i.Title, i.Version, i.Owner, i.Domain, i.Scope, i.Items)
		}
		return textResult(b.String(), false), nil
	case "synapse_ask":
		var a struct {
			Question string `json:"question"`
			Capsule  string `json:"capsule"`
			Purpose  string `json:"purpose"`
		}
		if err := json.Unmarshal(p.Arguments, &a); err != nil || strings.TrimSpace(a.Question) == "" {
			return textResult("A question is required.", true), nil
		}
		c, err := s.pick(a.Capsule)
		if err != nil {
			return textResult(err.Error(), true), nil
		}
		purpose := a.Purpose
		if purpose == "" {
			purpose = s.DefaultPurpose
		}
		access := synapse.Access{Purpose: purpose}
		if s.Access != nil {
			access = s.Access(c, purpose)
		}
		reply, err := c.Ask(a.Question, access)
		if err != nil {
			return textResult("The question could not be processed: "+err.Error(), true), nil
		}
		return textResult(reply.Markdown, !reply.Decision.Allowed), nil
	default:
		return nil, &rpcError{-32602, "unknown tool"}
	}
}

func (s *Server) pick(slug string) (*synapse.Capsule, error) {
	if slug == "" {
		if len(s.Capsules) == 1 {
			return s.Capsules[0], nil
		}
		return nil, errors.New("Several capsules are available; say which one with the capsule argument. Use synapse_capsules to list them.")
	}
	for _, c := range s.Capsules {
		if c.Raw.Manifest.Slug == slug {
			return c, nil
		}
	}
	return nil, fmt.Errorf("There is no capsule called %q. Use synapse_capsules to list them.", slug)
}

func textResult(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []map[string]string{{"type": "text", "text": text}},
		"isError": isError,
	}
}
