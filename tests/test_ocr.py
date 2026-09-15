import asyncio
import base64
import io

import pytest
from fastapi import HTTPException

from synapse_ai.main import Document, document, ocr_pdf_pages


def _blank_pdf_bytes() -> bytes:
    import pypdfium2 as pdfium

    pdf = pdfium.PdfDocument.new()
    pdf.new_page(200, 200)
    buf = io.BytesIO()
    pdf.save(buf)
    return buf.getvalue()


def test_ocr_pdf_pages_fails_loudly_without_tesseract_binary():
    """This sandbox intentionally does not have the `tesseract` OS binary
    installed (see README/user_task.md — it is a one-line apt-get in the
    Dockerfile/host for real deployments). The code must never fabricate OCR
    text when the engine is missing; it must raise a clear 503."""
    with pytest.raises(HTTPException) as exc_info:
        ocr_pdf_pages(_blank_pdf_bytes())
    assert exc_info.value.status_code == 503
    assert "tesseract" in exc_info.value.detail.lower()


def test_document_endpoint_routes_textless_pdf_through_ocr_and_fails_loudly():
    req = Document(
        capsuleId="c1",
        idempotencyKey="ocr-doc-test",
        allowedScope={"sourceIds": ["s1"]},
        sourceId="s1",
        contentType="application/pdf",
        filename="scanned.pdf",
        contentBase64=base64.b64encode(_blank_pdf_bytes()).decode(),
    )
    with pytest.raises(HTTPException) as exc_info:
        asyncio.run(document(req))
    # Proves the OCR fallback path is actually reached (not silently
    # swallowed into the generic 422 "no readable text" error) and that it
    # refuses to invent content when the OCR engine isn't installed.
    assert exc_info.value.status_code == 503
