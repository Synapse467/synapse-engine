// Package synapse is the whole integration surface for using a capsule in a Go program:
//
//	c, err := synapse.Open("tax-rules.capsule.json") // verified offline: hash, signatures, structure
//	reply, err := c.Ask("When must a deposit be protected?", synapse.Access{Purpose: "research"})
//	fmt.Println(reply.Markdown)
//
// There is nothing to configure and no account to create. A capsule is a file; it is checked
// before it is used; licenses are files checked the same way; and the engine answers only from the
// expert's approved items, or says it cannot.
package synapse

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Synapse467/synapse-core/capsule"
	"github.com/Synapse467/synapse-core/license"
	"github.com/Synapse467/synapse-core/usage"
	"github.com/Synapse467/synapse-engine/retrieve"
)

// Capsule is a verified capsule ready to answer questions.
type Capsule struct {
	Raw    *capsule.Capsule
	Report capsule.Report
	ix     *retrieve.Index
}

// Info summarises a capsule without exposing its knowledge.
type Info struct {
	Slug       string             `json:"slug"`
	Title      string             `json:"title"`
	Domain     string             `json:"domain"`
	Scope      string             `json:"scope"`
	Version    int                `json:"version"`
	Hash       string             `json:"hash"`
	Owner      string             `json:"owner"`
	Items      int                `json:"items"`
	Open       bool               `json:"open"`
	Evaluation capsule.Evaluation `json:"evaluation"`
}

// Open reads a capsule file and verifies it completely. A capsule that fails verification is
// never returned: there is no way to use a capsule that has been altered.
func Open(path string) (*Capsule, error) {
	c, err := capsule.Load(path)
	if err != nil {
		return nil, err
	}
	return New(c)
}

// Parse verifies a capsule held in memory.
func Parse(data []byte) (*Capsule, error) {
	c, err := capsule.Parse(data)
	if err != nil {
		return nil, err
	}
	return New(c)
}

// New verifies an already-decoded capsule.
func New(c *capsule.Capsule) (*Capsule, error) {
	report := capsule.Verify(c, nil)
	if !report.OK {
		return nil, fmt.Errorf("synapse: this capsule failed verification: %s", report.Issues[0])
	}
	return &Capsule{Raw: c, Report: report, ix: retrieve.FromCapsule(c)}, nil
}

// Info describes the capsule.
func (c *Capsule) Info() Info {
	m := c.Raw.Manifest
	return Info{
		Slug: m.Slug, Title: m.Title, Domain: m.Domain, Scope: m.Scope, Version: m.Version, Hash: c.Raw.Hash,
		Owner: m.Owner, Items: len(m.Knowledge), Open: m.Policy.Open, Evaluation: m.Evaluation,
	}
}

// Access describes who is asking and why. Everything is optional except Purpose: an open capsule
// needs only a purpose, and a licensed one also needs the license and the asker's address.
type Access struct {
	Purpose    string
	Grantee    string // the asker's Stellar address; required to use a license
	License    *license.License
	Revoked    license.RevocationSet
	Commercial bool
	AITraining bool
	Derivative bool
	Now        time.Time // defaults to the current time
	// Log records the consultation (as a hash of the question, never the question). When set,
	// license quotas are counted from it, and an answer is withheld if it cannot be recorded.
	Log       *usage.Log
	Retrieval retrieve.Options
}

// Reply is the result of a question.
type Reply struct {
	Decision license.Decision `json:"decision"`
	Answered bool             `json:"answered"`
	Answer   retrieve.Answer  `json:"answer"`
	Markdown string           `json:"markdown"`
	Capsule  Info             `json:"capsule"`
}

// MaxQuestion is the longest question, in characters, that Ask accepts. Real questions are far
// shorter; the limit keeps a hostile one from making retrieval needlessly slow.
const MaxQuestion = 4000

// ErrQuestionTooLong is returned by Ask for a question longer than MaxQuestion.
var ErrQuestionTooLong = errors.New("synapse: the question is too long")

// Ask checks that the asker may use the capsule, then answers from it or says it cannot.
//
// A denied request returns a Reply whose Decision explains why; it is not an error. An error means
// the request could not be processed at all (for example, the usage log could not be written).
func (c *Capsule) Ask(question string, a Access) (*Reply, error) {
	if utf8.RuneCountInString(question) > MaxQuestion {
		return nil, ErrQuestionTooLong
	}
	now := a.Now
	if now.IsZero() {
		now = time.Now()
	}
	id := usage.OpenLicense
	if a.License != nil {
		id = a.License.Terms.ID
	}
	used := 0
	if a.Log != nil {
		used = a.Log.Count(id)
	}
	decision := license.Access(a.License, c.Raw, a.Revoked, license.Request{
		Grantee: a.Grantee, Purpose: a.Purpose, Now: now, Used: used,
		Commercial: a.Commercial, AITraining: a.AITraining, Derivative: a.Derivative,
	})
	reply := &Reply{Decision: decision, Capsule: c.Info()}

	record := func(allowed bool, code string) error {
		if a.Log == nil {
			return nil
		}
		sum := sha256.Sum256([]byte(question))
		_, err := a.Log.Append(usage.Event{
			At: now.UTC().Format(time.RFC3339), License: id, Capsule: c.Raw.Hash, Version: c.Raw.Manifest.Version,
			Grantee: a.Grantee, Purpose: a.Purpose, QuestionSHA256: hex.EncodeToString(sum[:]),
			Allowed: allowed, Code: code,
		})
		return err
	}

	if !decision.Allowed {
		if err := record(false, string(decision.Code)); err != nil {
			return nil, err
		}
		reply.Markdown = "**Access denied.** " + capitalise(decision.Reason) + ".\n"
		return reply, nil
	}

	answer := c.ix.Ask(question, a.Retrieval)
	reply.Answer = answer
	reply.Answered = answer.Answered
	if !answer.Answered && reply.Decision.Remaining >= 0 {
		// The decision counted this question against the quota, but a refusal is free.
		reply.Decision.Remaining++
	}
	// An answer that cannot be recorded is not given, so the usage record is never behind what
	// was delivered. A refusal to answer costs nothing and is recorded as not allowed so it does
	// not count against a quota.
	code, allowedUse := string(decision.Code), true
	if !answer.Answered {
		code, allowedUse = "not_covered", false
	}
	if err := record(allowedUse, code); err != nil {
		return nil, fmt.Errorf("synapse: the use could not be recorded, so no answer was given: %w", err)
	}
	reply.Markdown = retrieve.Render(answer, retrieve.SourceOf(c.Raw), retrieve.Names(c.Raw))
	return reply, nil
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// LoadLicense reads and verifies a license file.
func LoadLicense(path string) (*license.License, error) {
	l, err := license.Load(path)
	if err != nil {
		return nil, err
	}
	if err := l.Verify(); err != nil {
		return nil, err
	}
	return l, nil
}
