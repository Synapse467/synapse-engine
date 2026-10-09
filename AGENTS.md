# synapse-engine working rules

- Use Go. Formats, signatures and license decisions come from `synapse-core`; never reimplement them here.
- The engine never generates text. Every answer is made of approved items shown as approved. Do not add a model call, a network call or a source of randomness to retrieval, extraction or evaluation.
- Output must be deterministic: same capsule and question, same answer. Break ties by item ID.
- Declining to answer is a feature. Any change to retrieval needs an evaluation case showing it still declines uncovered questions.
- Treat documents and questions as sensitive. Never log them. Usage records hold hashes.
- A use must be recorded before an answer is returned; if it cannot be recorded, do not answer.
- The gateway must fail closed and must not reveal capsule content to unauthenticated callers (`/v1/capsule` is metadata only).
- Add meaningful tests for each supported input shape and for malformed input, including hostile bodies and replayed requests.
- Keep boundaries in synapse-core's `docs/REPOSITORIES.md`; do not copy CLI behaviour here.
- Do not put credentials, private keys, tokens or real customer data in this workspace.
