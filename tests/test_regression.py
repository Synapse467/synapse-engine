"""Prompt/method regression suite (PRD §22 "prompt/model regression suite").

Synapse AI has no LLM in the loop for retrieval/extraction/evaluation
today — every method is deterministic (`extractive-v1`, `verbatim-candidate-
extraction-v1`, etc, see README). A "prompt/model regression suite" for a
deterministic system means golden-fixture snapshot tests: a frozen set of
(input, expected output) pairs that must keep producing the exact same
classification/answer/abstention decision release over release. If a future
change to the ranking/classification heuristics — or a future move to a real
LLM — changes these outputs, this suite fails loudly instead of silently
drifting, and the method-name version suffix (`-v1` -> `-v2`) must be bumped
alongside a deliberate update to `fixtures/golden_cases.json`.
"""

import json
from pathlib import Path

import pytest

from synapse_ai.main import Extract, Item, Query, _answer, _extract

FIXTURES = json.loads(
    (Path(__file__).parent / "fixtures" / "golden_cases.json").read_text()
)


@pytest.mark.parametrize(
    "case", FIXTURES["extractionCases"], ids=lambda c: c["name"]
)
def test_extraction_golden_case(case: dict) -> None:
    req = Extract(
        capsuleId="cap-1",
        idempotencyKey=f"regress-{case['name']}",
        allowedScope={"sourceIds": [case["sourceId"]]},
        sourceId=case["sourceId"],
        text=case["text"],
    )
    result = _extract(req)
    kinds = [item["kind"] for item in result["items"]]
    assert kinds == case["expectedKinds"], (
        f"extraction classification drifted for golden case {case['name']!r}: "
        f"expected {case['expectedKinds']}, got {kinds}"
    )
    assert result["method"] == "verbatim-candidate-extraction-v1"


@pytest.mark.parametrize("case", FIXTURES["queryCases"], ids=lambda c: c["name"])
def test_query_golden_case(case: dict) -> None:
    item = Item(
        id="k1",
        sourceId="s1",
        contributor="Fixture Expert",
        kind="HEURISTIC",
        text=case["knowledgeText"],
        status="APPROVED",
    )
    req = Query(
        capsuleId=case["capsuleId"],
        idempotencyKey=f"regress-{case['name']}",
        allowedScope={"knowledgeIds": ["k1"]},
        query=case["query"],
        version="1.0.0",
        knowledge=[item],
    )
    result = _answer(req)
    assert result["abstained"] == case["expectedAbstained"], (
        f"query abstention behavior drifted for golden case {case['name']!r}"
    )
    assert result["method"] == "extractive-v1"
