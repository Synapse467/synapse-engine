# Synapse private expertise service

Python 3.14, FastAPI, uv. Only the API may call this service with its configured bearer token and already-authorized scope. Bind to loopback locally and a private network in deployment.

```sh
uv sync --locked
uv run --env-file .env uvicorn synapse_ai.main:app --host 127.0.0.1 --port 8000
uv run pytest
uv run ruff check src tests
uv run mypy src
```

Implemented: verbatim source candidate extraction with stable source references; conservative lexical retrieval restricted to approved scoped items; exact excerpt answers with attribution; unsupported-query abstention; coverage-planned interview prompts; text/PDF/DOCX extraction; configurable HTTP transcription adapter; expert golden-case evaluation of expected elements, forbidden elements, citations, and designated unsupported questions.

This is an extractive implementation, not a general LLM impersonation of the expert. Candidate confidence describes source-text matching, not factual truth. No model silently fills gaps. Evaluation requires both supported and unsupported expert-authored cases to pass the PRD thresholds.

Remaining: semantic/full-text/vector search, richer structured extraction and contradiction detection, correction-aware evaluation generation, image OCR, uploaded-file scanner, provider-specific retention configuration, prompt/model regression suites, idempotent provider processing, and richer OpenTelemetry instrumentation. Transcription requires a configured provider; absent keys return an explicit unavailable error.
