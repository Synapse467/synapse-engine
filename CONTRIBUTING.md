# Contributing

Thank you for helping. A few things keep Synapse trustworthy.

## Before you start

- Read `AGENTS.md` (the rules every change follows) and synapse-core's `docs/REPOSITORIES.md` (which repository owns what).
- Open an issue first for anything that changes how answers are chosen, when the engine declines, or what the gateway checks.

## Build and test

Go 1.26 or later. The engine depends on synapse-core; clone it beside this repository and use a `go.work` file:

```bash
go work init ./synapse-engine ./synapse-core
cd synapse-engine
go vet ./... && go test ./...
```

## What a good change has

- Tests: meaningful ones, for the supported cases and for malformed or hostile input.
- For anything that changes retrieval: an evaluation-style test showing it still declines questions the capsule does not cover. Tuning that makes a capsule answer more will also make it wrong more often; show both.
- Documentation updated in the same change (`docs/how-answers-work.md`, `docs/evaluation.md`).
- No secrets, no real customer data, no real documents in fixtures. Use made-up examples.
- Formatted code (`gofmt`) and no `go vet` warnings.
- A note in `CHANGELOG.md` for anything a user would notice.

## Commit messages

Describe what changed and why in the first line (imperative mood), then details if needed.
