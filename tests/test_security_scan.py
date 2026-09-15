import asyncio
import base64
import io
import zipfile

import pytest
from fastapi import HTTPException

from synapse_ai.main import EICAR_SIGNATURE, Document, document, scan_binary


def _document_request(content: bytes, filename: str, content_type: str) -> Document:
    return Document(
        capsuleId="c1",
        idempotencyKey="doc-test",
        allowedScope={"sourceIds": ["s1"]},
        sourceId="s1",
        contentType=content_type,
        filename=filename,
        contentBase64=base64.b64encode(content).decode(),
    )


def test_scan_binary_rejects_eicar_test_signature():
    with pytest.raises(HTTPException) as exc_info:
        scan_binary(EICAR_SIGNATURE, "notes.txt")
    assert exc_info.value.status_code == 422
    assert "EICAR" in exc_info.value.detail


def test_scan_binary_rejects_windows_executable():
    with pytest.raises(HTTPException) as exc_info:
        scan_binary(b"MZ\x90\x00\x03\x00\x00\x00fake pe body", "resume.pdf")
    assert exc_info.value.status_code == 422
    assert "executable" in exc_info.value.detail


def test_scan_binary_rejects_elf_executable():
    with pytest.raises(HTTPException):
        scan_binary(b"\x7fELF" + b"\x00" * 32, "notes.docx")


def test_scan_binary_rejects_zip_decompression_bomb():
    buffer = io.BytesIO()
    huge = b"0" * (10 * 1024 * 1024)  # 10 MiB of a single repeated byte, compresses ~1000x
    with zipfile.ZipFile(buffer, "w", zipfile.ZIP_DEFLATED) as archive:
        archive.writestr("payload.txt", huge)
    with pytest.raises(HTTPException) as exc_info:
        scan_binary(buffer.getvalue(), "bomb.docx")
    assert exc_info.value.status_code == 422
    assert "decompression-bomb" in exc_info.value.detail


def test_scan_binary_allows_ordinary_text():
    scan_binary(b"Ordinary plain-text expertise notes.", "notes.txt")  # must not raise


def test_scan_binary_requires_clamav_when_configured_required(monkeypatch):
    monkeypatch.setenv("CLAMAV_REQUIRED", "true")
    monkeypatch.delenv("CLAMAV_CMD", raising=False)
    with pytest.raises(HTTPException) as exc_info:
        scan_binary(b"Ordinary plain-text expertise notes.", "notes.txt")
    assert exc_info.value.status_code == 503


def test_document_endpoint_rejects_eicar_upload():
    req = _document_request(EICAR_SIGNATURE, "malware.txt", "text/plain")
    with pytest.raises(HTTPException) as exc_info:
        asyncio.run(document(req))
    assert exc_info.value.status_code == 422
