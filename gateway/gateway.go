// Package gateway serves a capsule over HTTP, for owners who want licenses enforced by the
// holder of the capsule rather than on the honour system.
//
// Using a capsule from a file is cooperative: whoever holds the file could ignore the license
// terms. A gateway is authoritative: the capsule never leaves the owner's machine, every question
// arrives as a signed request, and the owner's own usage log is the record. It needs no accounts and
// no configuration; identity is the Stellar keypair that signs each request.
package gateway

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/Synapse467/synapse-core/license"
	"github.com/Synapse467/synapse-core/usage"
	"github.com/Synapse467/synapse-engine/retrieve"
	"github.com/Synapse467/synapse-engine/synapse"
)

// MaxBody bounds a request body.
const MaxBody = 1 << 20

// DefaultMaxAge is how far a signed request's time may differ from the gateway's clock.
const DefaultMaxAge = 5 * time.Minute

// Config describes what to serve.
type Config struct {
	Capsule *synapse.Capsule
	// Log records every request. It is required, so the owner's record is never skipped.
	Log *usage.Log
	// Revocations is consulted on every request, so a revocation takes effect immediately.
	// It may be nil.
	Revocations func() (license.RevocationSet, error)
	MaxAge      time.Duration
	Retrieval   retrieve.Options
	// Now is for tests; it defaults to time.Now.
	Now func() time.Time
}

// AskBody is the body of POST /v1/ask.
type AskBody struct {
	// Request is the signed question. It is required for licensed capsules.
	Request *license.SignedRequest `json:"request,omitempty"`
	// License is the licensee's license file. It travels with the request, so the gateway needs
	// no database of licenses.
	License *license.License `json:"license,omitempty"`
	// Question and Purpose are used only for unsigned requests to open capsules.
	Question string `json:"question,omitempty"`
	Purpose  string `json:"purpose,omitempty"`
	// Commercial, AITraining and Derivative declare how the answer will be used.
	Commercial bool `json:"commercial,omitempty"`
	AITraining bool `json:"aiTraining,omitempty"`
	Derivative bool `json:"derivative,omitempty"`
}

// ErrorBody is returned with every non-200 response.
type ErrorBody struct {
	Error    string            `json:"error"`
	Code     string            `json:"code,omitempty"`
	Decision *license.Decision `json:"decision,omitempty"`
}

type server struct {
	cfg    Config
	replay *license.ReplayGuard
}

// New returns the gateway's http.Handler. Mount it anywhere, or serve it directly.
func New(cfg Config) (http.Handler, error) {
	if cfg.Capsule == nil || cfg.Log == nil {
		return nil, errors.New("gateway: a capsule and a usage log are required")
	}
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = DefaultMaxAge
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	s := &server{cfg: cfg, replay: license.NewReplayGuard(2 * cfg.MaxAge)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) { write(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /v1/capsule", s.info)
	mux.HandleFunc("POST /v1/ask", s.ask)
	return mux, nil
}

func write(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func fail(w http.ResponseWriter, status int, code, message string) {
	write(w, status, ErrorBody{Error: message, Code: code})
}

func (s *server) info(w http.ResponseWriter, r *http.Request) { write(w, 200, s.cfg.Capsule.Info()) }

func (s *server) ask(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBody))
	if err != nil {
		fail(w, http.StatusRequestEntityTooLarge, "too_large", "the request is too large")
		return
	}
	var body AskBody
	if err := strictDecode(data, &body); err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "the request is not valid JSON of the expected shape")
		return
	}
	now := s.cfg.Now()
	access := synapse.Access{
		Commercial: body.Commercial, AITraining: body.AITraining, Derivative: body.Derivative,
		Now: now, Log: s.cfg.Log, Retrieval: s.cfg.Retrieval,
	}
	question := body.Question

	if body.Request != nil {
		req := body.Request
		if err := req.Verify(now, s.cfg.MaxAge); err != nil {
			fail(w, http.StatusUnauthorized, "bad_signature", err.Error())
			return
		}
		if !s.replay.FirstUse(req.Grantee, req.Nonce, now) {
			fail(w, http.StatusConflict, "replayed", "this request has already been used")
			return
		}
		if req.Capsule != s.cfg.Capsule.Raw.Hash {
			fail(w, http.StatusConflict, "wrong_capsule", "the request was made for a different version of the capsule")
			return
		}
		if body.License != nil && req.License != body.License.Hash {
			fail(w, http.StatusBadRequest, "license_mismatch", "the request was not signed for the license that was sent")
			return
		}
		access.Grantee, access.Purpose, access.License, question = req.Grantee, req.Purpose, body.License, req.Question
	} else {
		// An unsigned request is allowed only for open capsules, which need no identity.
		if body.License != nil || !s.cfg.Capsule.Raw.Manifest.Policy.Open {
			fail(w, http.StatusUnauthorized, "signature_required", "this capsule needs a signed request and a license")
			return
		}
		access.Purpose = body.Purpose
	}
	if question == "" {
		fail(w, http.StatusBadRequest, "no_question", "a question is required")
		return
	}
	if s.cfg.Revocations != nil {
		set, err := s.cfg.Revocations()
		if err != nil {
			fail(w, http.StatusInternalServerError, "revocations_unavailable", "the revocation list could not be read, so no license can be honoured")
			return
		}
		access.Revoked = set
	}

	reply, err := s.cfg.Capsule.Ask(question, access)
	if errors.Is(err, synapse.ErrQuestionTooLong) {
		fail(w, http.StatusBadRequest, "question_too_long", "the question is too long")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "not_recorded", "the use could not be recorded, so no answer was given")
		return
	}
	if !reply.Decision.Allowed {
		d := reply.Decision
		write(w, http.StatusForbidden, ErrorBody{Error: d.Reason, Code: string(d.Code), Decision: &d})
		return
	}
	write(w, http.StatusOK, reply)
}

func strictDecode(data []byte, into any) error {
	dec := json.NewDecoder(bytesReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("trailing data")
	}
	return nil
}
