"""OpenTelemetry wiring for the Synapse AI service (PRD §22 observability:
"OpenTelemetry-compatible tracing, structured JSON logs, request/job
correlation IDs").

Zero-config-safe by design: importing/using this module never requires a
running collector. If `OTEL_EXPORTER_OTLP_ENDPOINT` is unset, spans are still
created (so `span(...)` is always safe to use throughout the codebase) but
are not exported anywhere — no network calls, no console spam, safe for
tests and local dev. Set `OTEL_EXPORTER_OTLP_ENDPOINT` (and optionally
`OTEL_EXPORTER_OTLP_HEADERS` for auth) to point at a real OTLP/HTTP
collector (Honeycomb, Grafana Tempo, Datadog, etc.) in any real deployment.
"""

from __future__ import annotations

import os
from collections.abc import Iterator
from contextlib import contextmanager
from typing import Any

from opentelemetry import trace
from opentelemetry.sdk.resources import SERVICE_NAME, Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor

_TRACER_NAME = "synapse-ai"
_configured = False


def setup_telemetry(app: Any | None = None) -> None:
    """Idempotent. Configures a global TracerProvider once per process, and
    instruments the given FastAPI app (if provided) for automatic
    request-level spans."""
    global _configured
    if not _configured:
        resource = Resource.create({SERVICE_NAME: "synapse-ai"})
        provider = TracerProvider(resource=resource)
        endpoint = os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
        if endpoint:
            # Imported lazily: pulls in `requests` + protobuf only when an
            # exporter destination is actually configured.
            from opentelemetry.exporter.otlp.proto.http.trace_exporter import (
                OTLPSpanExporter,
            )

            headers = {}
            raw_headers = os.getenv("OTEL_EXPORTER_OTLP_HEADERS", "")
            for pair in raw_headers.split(","):
                if "=" in pair:
                    k, v = pair.split("=", 1)
                    headers[k.strip()] = v.strip()
            provider.add_span_processor(
                BatchSpanProcessor(
                    OTLPSpanExporter(endpoint=endpoint, headers=headers or None)
                )
            )
        trace.set_tracer_provider(provider)
        _configured = True
    if app is not None:
        try:
            from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor

            FastAPIInstrumentor.instrument_app(app)
        except (ImportError, AttributeError, TypeError) as err:
            # Instrumentation is best-effort observability, never a reason to
            # fail startup of the private AI service.
            import logging

            logging.getLogger(__name__).warning(
                "FastAPI OpenTelemetry instrumentation skipped: %s", err
            )


@contextmanager
def span(name: str, **attributes: Any) -> Iterator[None]:
    """Convenience context manager: `with span("extract.sentences", sourceId=x): ...`"""
    tracer = trace.get_tracer(_TRACER_NAME)
    with tracer.start_as_current_span(name) as current:
        for key, value in attributes.items():
            if value is not None:
                current.set_attribute(key, value)
        yield
