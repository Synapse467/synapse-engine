# Synapse private AI service — production image.
# Includes the tesseract-ocr OS binary so scanned-PDF OCR (see
# src/synapse_ai/main.py:ocr_pdf_pages) works out of the box; without it the
# service still runs correctly, it just returns an explicit 503 for scanned
# documents instead of silently failing.
FROM python:3.14-slim AS base

RUN apt-get update \
    && apt-get install -y --no-install-recommends tesseract-ocr tesseract-ocr-eng \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir uv

WORKDIR /app
COPY pyproject.toml uv.lock ./
RUN uv sync --locked --no-dev --no-install-project

COPY src ./src
RUN uv sync --locked --no-dev

ENV PATH="/app/.venv/bin:$PATH"
EXPOSE 8000

# Binds to all interfaces inside the container network only — the compose/
# orchestration layer must keep this service on a private network, never
# publicly exposed (only synapse-api should reach it). See README.
CMD ["uvicorn", "synapse_ai.main:app", "--host", "0.0.0.0", "--port", "8000"]
