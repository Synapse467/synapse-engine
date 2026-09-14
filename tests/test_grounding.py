from synapse_ai.main import Evaluation, GoldenCase, Item, Query, answer, evaluate


def request(text: str, status: str = "APPROVED", allowed: bool = True) -> Query:
    return Query(
        capsuleId="c1",
        idempotencyKey="test",
        allowedScope={"knowledgeIds": ["k1"] if allowed else []},
        query=text,
        version="1.0.0",
        knowledge=[
            Item(
                id="k1",
                sourceId="s1",
                contributor="Fictional Expert",
                kind="HEURISTIC",
                text="Check operating changes before replacing equipment components.",
                status=status,
            )
        ],
    )


def test_grounded_answer():
    result = answer(request("What should I check before replacing equipment?"))
    assert not result["abstained"]
    assert result["citations"][0]["contributor"] == "Fictional Expert"
    assert result["citations"][0]["quote"] in result["text"]


def test_unsupported_abstains():
    assert answer(request("What is the best chocolate cake recipe?"))["abstained"]


def test_pending_excluded():
    assert answer(request("replacing equipment", status="PENDING"))["abstained"]


def test_scope_excluded():
    assert answer(request("replacing equipment", allowed=False))["abstained"]


def test_evaluation_requires_supported_and_unsupported_golden_cases():
    source = request("replacing equipment")
    suite = Evaluation(
        capsuleId="c1",
        idempotencyKey="eval",
        allowedScope=source.allowedScope,
        knowledge=source.knowledge,
    )
    assert not evaluate(suite)["passed"]
    suite.goldenCases = [
        GoldenCase(
            question="What should I check before replacing equipment?",
            expectedElements=["Check operating changes"],
        ),
        GoldenCase(
            question="What is the best chocolate cake recipe?", unsupported=True
        ),
    ]
    assert evaluate(suite)["passed"]
    suite.goldenCases[0].expectedElements = [
        "An invented procedure not present in the capsule"
    ]
    assert not evaluate(suite)["passed"]


def test_generate_evals_produces_supported_and_unsupported_cases():
    from synapse_ai.main import GenerateEvals, generate_evals

    source = request("replacing equipment")
    req = GenerateEvals(
        capsuleId="c1",
        idempotencyKey="gen-evals",
        allowedScope=source.allowedScope,
        knowledge=source.knowledge,
    )
    result = generate_evals(req)
    assert result["count"] > 0
    supported = [c for c in result["cases"] if not c["unsupported"]]
    unsupported = [c for c in result["cases"] if c["unsupported"]]
    assert len(supported) >= 1
    assert len(unsupported) >= 2
    assert any("Check operating changes" in c["expectedElements"][0] for c in supported)


def test_transcribe_endpoint():
    import asyncio
    import base64
    from synapse_ai.main import TranscribeRequest, transcribe

    req = TranscribeRequest(
        capsuleId="c1",
        idempotencyKey="transcribe-test",
        allowedScope={"sourceIds": ["s1"]},
        contentBase64=base64.b64encode(b"Transcribed audio notes from expert.").decode(),
        filename="notes.txt",
        contentType="text/plain",
    )
    res = asyncio.run(transcribe(req))
    assert "Transcribed audio notes" in res["text"]

