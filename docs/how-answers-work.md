# How answers are made

There is no language model in the engine. An answer is a ranked selection of the expert's approved items. This page explains each step, so you can predict what the capsule will do, and judge whether this approach suits your expertise.

## 1. The question becomes terms

`textutil.Terms` lower-cases the text, splits it into letters and digits, removes common words ("the", "how", "should"…) and applies a conservative stemmer: *rotating*, *rotated*, *rotation* and *rotate* all reduce to `rotat`; *policies* to `policy`; *boxes* to `box`. The stemmer only strips endings it can strip safely, because a wrong stem merges two different words, which is worse than missing a match.

## 2. Items are indexed with weights

Each item's text is split into the same terms, counted with a weight for where each appears:

| Where | Weight |
|---|---|
| Title, tags | 3 |
| "Applies to", conditions | 2 |
| Body, steps, exceptions, rationale | 1 |

Experts can therefore steer retrieval by choosing titles and tags well.

## 3. Scoring: BM25

Items are ranked with BM25 (k₁ = 1.2, b = 0.75), the standard keyword-ranking formula. Rare terms count for more than common ones, repeated terms count for diminishing returns, and long items are not favoured merely for being long.

## 4. Deciding to answer or decline

Ranking alone would always return *something*. The step that matters is the decision not to.

For each item the engine computes **coverage**: the share of the question's distinctive wording that the item contains, with each term weighted by how rare it is in this capsule. A question whose rare words are all in the item has coverage 1.0.

- Words the capsule has never seen count for **half** weight. They are usually filler ("often", "best", "typically"), and they should reduce confidence a little without refusing a question that is otherwise squarely covered.
- If the best item's coverage is below **0.5**, the engine declines.
- Otherwise it returns up to three items whose coverage clears 0.5 and whose score is at least half the best score.

This is why "What is the capital of Australia?" gets *Not covered* from a tenancy capsule, and why a question that shares only one word ("deployment") with an item is declined when the rest of it is about something else.

## 5. Exceptions travel with their rules

A rule that is correct *except in one case* gives a wrong answer if the case is left out. So after choosing the answer, the engine adds every `exception` item whose `appliesTo` names a returned item (or one of its tags), even if that exception would not have matched the question on its own. Exceptions an expert recorded inside an item (the `exceptions` list) are always shown with it.

## 6. The answer is shown as approved

`retrieve.Render` writes each item's title, body, ordered steps, conditions, exceptions, reasoning and limits exactly as approved, followed by who contributed it and the supporting quotes (shortened for display; the full quote stays in the capsule). The last line names the capsule, version, short hash and owner, so an answer can always be traced to a specific signed file.

## What this approach is good at

- **Never inventing anything.** There is no step where text can be fabricated.
- **Explainability.** `Result.Matched` lists the question words found in each item; every answer can be traced to specific approved text.
- **Consistency.** The same question gets the same answer, which matters when a professional relies on it.
- **Cost and privacy.** Nothing leaves the machine and nothing is charged.

## Where it will let you down

- **Paraphrase.** A question that shares no words with the capsule will be declined even if it means the same thing. *"Can I keep a tenant's money?"* will not find an item about *deposits*. Mitigate with tags and titles that use the words your users use.
- **Synthesis.** It will not combine two items into a new conclusion. It shows both; the reader (or their AI agent) does the combining.
- **Negation and numbers.** It matches words, not meaning. "Not required" and "required" look alike to it.
- **Short questions.** One-word questions have little to match on.

If these limits matter for your use, the intended pattern is to let an AI agent do the language work and use the capsule as the authoritative source: `synapse mcp` exists for exactly that.

## Tuning

`retrieve.Options` has `MinCoverage` (default 0.5), `MaxResults` (default 3) and `Suggest` (list the nearest titles when declining; off by default so a gateway does not reveal titles to someone it is declining). Lowering `MinCoverage` makes the capsule answer more and be wrong more often; the evaluation suite is how you find out what a change does.
