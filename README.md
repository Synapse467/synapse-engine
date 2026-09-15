# Synapse private expertise service

Python 3.14, FastAPI, uv. Only the API may call this service with its configured bearer token and already-authorized scope. Bind to loopback locally and a private network in deployment.

```sh
uv sync --locked
uv run --env-file .env uvicorn synapse_ai.main:app --host 127.0.0.1 --port 8000
uv run pytest
uv run ruff check src tests
uv run mypy src
```

Implemented: verbatim source candidate extraction with stable source references; conservative lexical retrieval restricted to approved scoped items; exact excerpt answers with attribution; unsupported-query abstention; coverage-planned interview prompts; text/PDF/DOCX extraction plus Tesseract OCR fallback for textless PDFs (fails 503 if `tesseract` is not installed — never fabricates text); heuristic + optional ClamAV uploaded-file scanning; OpenTelemetry tracing when `OTEL_EXPORTER_OTLP_ENDPOINT` is set; configurable HTTP transcription adapter; expert golden-case evaluation of expected elements, forbidden elements, citations, and designated unsupported questions.

This is an extractive implementation, not a general LLM impersonation of the expert. Candidate confidence describes source-text matching, not factual truth. No model silently fills gaps. Evaluation requires both supported and unsupported expert-authored cases to pass the PRD thresholds.

Transcription requires a configured provider; absent keys return an explicit unavailable error (a `SYNAPSE_ENV != production`-gated text-fixture adapter exists for tests/demos only — never a fabricated transcript in production). Semantic/vector search is intentionally deferred. Host packages (`tesseract-ocr`, optional ClamAV) and provider keys are listed in `../user_task.md`.
