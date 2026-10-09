# synapse-engine

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
