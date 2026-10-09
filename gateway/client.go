package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Synapse467/synapse-core/identity"
	"github.com/Synapse467/synapse-core/license"
	"github.com/Synapse467/synapse-engine/synapse"
)

// Client asks a gateway questions.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// RemoteError is returned when the gateway refuses or fails a request.
type RemoteError struct {
	Status int
	Body   ErrorBody
}

func (e *RemoteError) Error() string {
	if e.Body.Code != "" {
		return fmt.Sprintf("the gateway refused the request (%s): %s", e.Body.Code, e.Body.Error)
	}
	return fmt.Sprintf("the gateway returned status %d: %s", e.Status, e.Body.Error)
}

// Use declares how an answer will be used. Declaring it is what lets a license limit it.
type Use struct {
	Commercial, AITraining, Derivative bool
}

func (c Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (c Client) url(path string) string { return strings.TrimRight(c.BaseURL, "/") + path }

// Info fetches the capsule's public summary.
func (c Client) Info(ctx context.Context) (*synapse.Info, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url("/v1/capsule"), nil)
	if err != nil {
		return nil, err
	}
	var info synapse.Info
	if err := c.do(req, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// Ask signs a question as the given identity and sends it. lic may be nil for an open capsule.
func (c Client) Ask(ctx context.Context, who *identity.Identity, lic *license.License, purpose, question string, use Use) (*synapse.Reply, error) {
	info, err := c.Info(ctx)
	if err != nil {
		return nil, err
	}
	licenseHash := ""
	if lic != nil {
		licenseHash = lic.Hash
	}
	signed, err := license.NewRequest(who, info.Hash, licenseHash, purpose, question, time.Now())
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(AskBody{
		Request: signed, License: lic, Commercial: use.Commercial, AITraining: use.AITraining, Derivative: use.Derivative,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("/v1/ask"), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	var reply synapse.Reply
	if err := c.do(req, &reply); err != nil {
		return nil, err
	}
	return &reply, nil
}

func (c Client) do(req *http.Request, into any) error {
	resp, err := c.http().Do(req)
	if err != nil {
		return fmt.Errorf("the gateway could not be reached: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		var body ErrorBody
		if json.Unmarshal(data, &body) != nil {
			body.Error = strings.TrimSpace(string(data))
		}
		return &RemoteError{Status: resp.StatusCode, Body: body}
	}
	return json.Unmarshal(data, into)
}
