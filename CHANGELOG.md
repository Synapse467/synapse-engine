# Changelog

All notable changes are recorded here. This project follows semantic versioning once it is tagged.

## [Unreleased] 0.1.0

A ground-up redesign: the engine is now a deterministic Go library with no model, network or configuration.

- `synapse`: open a verified capsule and ask it questions, with licenses, quotas, revocation and usage logging.
- Keyword retrieval (BM25) with an explicit decision to decline, exceptions that travel with their rules, and rendered answers with citations and attribution.
- Extraction of proposed items from documents; evaluation with generated or expert-written suites.
- `gateway`: an HTTP handler and client that enforce licenses authoritatively with signed requests.
- `mcpserver`: expose capsules to AI agents over MCP.
- Removed: the Python service and its model and API-key requirements.
