<p align="center"><img src="assets/logo.svg" alt="Synapse logo" width="112"></p>

<h1 align="center">synapse-engine</h1>

<p align="center"><b>Deterministic answering, extraction, evaluation, gateway and MCP server for Synapse capsules.</b></p>

<p align="center">
  <a href="https://github.com/Synapse467/synapse-engine/actions/workflows/ci.yml"><img src="https://github.com/Synapse467/synapse-engine/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/Synapse467/synapse-engine/blob/main/LICENSE"><img src="https://img.shields.io/github/license/Synapse467/synapse-engine?color=blue" alt="License: MIT"></a>
  <a href="https://github.com/Synapse467/synapse-engine/releases"><img src="https://img.shields.io/github/v/release/Synapse467/synapse-engine?color=brightgreen" alt="Latest release"></a>
  <img src="https://img.shields.io/github/go-mod/go-version/Synapse467/synapse-engine?color=00ADD8" alt="Go version">
  <a href="https://github.com/Synapse467/synapse-engine/issues"><img src="https://img.shields.io/github/issues/Synapse467/synapse-engine?color=orange" alt="Open issues"></a>
  <a href="https://github.com/Synapse467/synapse-engine/issues?q=is%3Aopen+label%3A%22help+wanted%22"><img src="https://img.shields.io/badge/help%20wanted-welcome-8A2BE2" alt="Help wanted"></a>
  <img src="https://img.shields.io/badge/built%20for-Stellar-black" alt="Built for Stellar">
</p>

<p align="center">
  <a href="https://cjay-1.gitbook.io/synapse-docs/">Documentation</a> ·
  <a href="https://github.com/Synapse467/synapse-engine/releases">Releases</a> ·
  <a href="https://github.com/Synapse467/synapse-engine/issues">Issues</a> ·
  <a href="CONTRIBUTING.md">Contributing</a> ·
  <a href="SECURITY.md">Security</a>
</p>

---


The part of Synapse that **reads and answers**. It turns documents into proposed knowledge, tests a capsule before it is published, and answers questions from a published capsule using only what the expert approved.

It is a Go library with no model, no network and no configuration. Everything is deterministic: the same capsule and the same question always give the same answer.

```go
import "github.com/Synapse467/synapse-engine/synapse"

c, err := synapse.Open("tenancy-law.capsule.json")   // verified offline: a tampered file does not open
reply, err := c.Ask("When must a deposit be protected?", synapse.Access{Purpose: "research"})

fmt.Println(reply.Markdown) // the expert's own words, with citations and attribution
fmt.Println(reply.Answered) // false if the capsule does not cover the question
```

That is the whole integration. Licenses, quotas, revocation and a tamper-evident usage log are options on `synapse.Access`.

## Packages

| Package | What it is for |
|---|---|
| [`synapse`](synapse) | **Start here.** Open a verified capsule and ask it questions, under a license or an open policy, with optional usage logging. |
| [`gateway`](gateway) | An `http.Handler` (and client) that serves a capsule and enforces its licenses authoritatively, with signed requests and replay protection. |
| [`mcpserver`](mcpserver) | Expose capsules to AI agents as MCP tools over stdio. |
| [`author`](author) | Build capsules: add documents, review proposals, evaluate, publish. |
| [`extract`](extract) | Propose items from a document: headings, numbered procedures, rules, exceptions, examples. |
| [`eval`](eval) | Test a capsule: generate or load a question suite, score coverage, refusals, citations and credit. |
| [`retrieve`](retrieve) | The search and abstention logic behind `Ask`. |
| [`textutil`](textutil) | Tokenising and light stemming shared by the above. |

## Answers cannot be invented

The engine never writes text. An answer is the approved items that best match the question, shown exactly as the expert approved them, with their exceptions, their citations and their author. When nothing matches well enough it says so:

```
**Not covered.** The capsule does not cover this question.
```

This is a design choice with a cost. It is explained honestly in [`docs/how-answers-work.md`](docs/how-answers-work.md): how ranking works, how it decides to decline, and where keyword retrieval will let you down.

## Testing before publishing

A capsule only carries `passed: true` if a suite of questions was run against exactly its items and met the thresholds. The suite's hash and size are recorded in the capsule, so a buyer can see what it was tested on. How the suite is generated, what each score means and why a generated suite is weaker than one you write is in [`docs/evaluation.md`](docs/evaluation.md).

## Integrating

- **A Go program**: `synapse.Open` and `Ask`, as above.
- **A web service**: mount `gateway.New(...)` in your router. See [`gateway`](gateway).
- **An AI agent**: `mcpserver.Server`, or `synapse mcp` from [`synapse-cli`](https://github.com/Synapse467/synapse-cli).
- **Another language**: run `synapse serve` and call its HTTP API, or implement the formats from [`SPEC.md`](https://github.com/Synapse467/synapse-core/blob/main/SPEC.md).

## Build and test

```bash
go test ./...
```

The module depends on [`synapse-core`](https://github.com/Synapse467/synapse-core) (tagged `v0.1.0`) for the formats and builds on its own. To work on both together, add a `go.work` file that lists the two folders.

## License

MIT. See [LICENSE](LICENSE).

## Maintainers

| Maintainer | Role | Contact |
| --- | --- | --- |
| [Synapse467](https://github.com/Synapse467) | Organization owner, releases | [GitHub issues](https://github.com/Synapse467/synapse-engine/issues) |

## Community

Ask questions and propose changes in [GitHub issues](https://github.com/Synapse467/synapse-engine/issues). Read the [documentation](https://cjay-1.gitbook.io/synapse-docs/) first; the [FAQ](https://cjay-1.gitbook.io/synapse-docs/project/faq) answers the common questions.

## Contributors

<a href="https://github.com/Synapse467/synapse-engine/graphs/contributors"><img src="https://contrib.rocks/image?repo=Synapse467/synapse-engine" alt="Contributors"></a>
