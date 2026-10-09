# Evaluation

A capsule cannot be published until it passes an evaluation, and the evaluation travels with it:

```json
"evaluation": {
  "suiteHash": "479bc1c4…", "cases": 25, "passed": true,
  "coverageBp": 10000, "abstentionBp": 10000, "citationValidityBp": 10000, "attributionBp": 10000
}
```

A buyer can read that and know the expertise was tested, on how many questions, and against which suite (the hash identifies the exact questions, so the same suite can be re-run to check the claim).

## The four scores

Scores are in basis points: 10000 means 100%.

| Score | Question it answers | Default minimum |
|---|---|---|
| **Coverage** | Of the questions the capsule should answer, how many did it answer with the expected item among the results? | 85% |
| **Abstention** | Of the questions it should *not* answer, how many did it decline? | 90% |
| **Citation validity** | Does every citation's quote match its recorded hash, and, where the original document is available, appear at exactly the cited place in a document whose hash is the one recorded? | 100% |
| **Attribution** | Does every item returned name a contributor who is listed on the capsule, and is it either cited or marked as written directly by the expert? | 100% |

Citations and attribution are all-or-nothing because a single forged quote or unattributed item undermines the point of the capsule.

## The suite

A suite is a JSON file:

```json
{
  "format": "synapse.eval/1",
  "cases": [
    { "id": "rotation", "question": "How often should signing keys be rotated?", "expect": "answer", "items": ["item-3fa9c1d2e4b7"] },
    { "id": "offtopic", "question": "What is the capital of Australia?", "expect": "abstain" }
  ]
}
```

A suite must contain at least one case that should be answered and one that should be declined.

### Generated suites

With no suite file, `synapse eval` generates one from the approved items:

- **Title case**: each item's title, as a question. It must retrieve that item.
- **Reword case**: four distinctive words from the item's body, in sorted order, as a question. It must retrieve the item, which checks that the item can be found by something other than its title.
- **Refusal cases**: up to ten fixed off-topic questions. Any that share a word with the capsule's vocabulary are dropped, so a generated refusal can never unfairly penalise a capsule that really does cover the subject.
- Exception items get no case of their own: they are returned alongside the rule they qualify.

A generated suite is a **smoke test**. It catches a broken capsule: items nobody can find, a capsule so broad it answers everything, a forged quote. It does not catch an item that is wrong, or questions your users ask that the expert never thought of.

### Your own suite

An evaluation written by the expert is a much stronger statement. Write down the questions you would expect, in the words your users would use, and which item should answer each. Start from the generated one:

```bash
synapse eval --write-suite     # writes synapse.eval.json
# edit it: add the questions people really ask, and some you know you do not cover
synapse publish                # uses synapse.eval.json automatically
```

If your own question fails, the capsule does not publish. That is the point: improve the item's title, tags or wording, or accept that the question is not covered and mark it as `abstain`.

## Why the original documents matter at publish time

Citation checks are strongest when the original source is still present, because the quote can be matched against it. `synapse add` keeps a private copy of each document in `.synapse/sources/` for this reason. If a copy is missing, the evaluation says so and falls back to checking each quote against its own hash. If a document has changed since it was added, publishing is refused: add the new version under a new name (`synapse add --as notes-v2 notes.md`) and re-approve the items it proposes.

## Using it as a library

```go
report, suite, err := author.Evaluate(draft, nil, sourceTexts, eval.DefaultThresholds)
fmt.Println(report.Evaluation.Passed, report.Failures)
```

`eval.Run` takes any `eval.Corpus` (items, contributors, sources and optional source text), so it can also gate a pipeline that builds capsules from another system.
