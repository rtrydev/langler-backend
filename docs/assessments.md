# Placement Assessment Contract

How Langler estimates a learner's level: test assembly from leveled reference
data, staged progression, scoring, storage, and the API surface. The engine is
language-agnostic; Japanese (JLPT `N5`…`N1`) ships first, and Polish/Burmese
(CEFR `A1`…`C2`) reuse the same code once their reference data exists.

Estimates are **guidance, not certification**. No official JLPT or CEFR word
lists exist; item pools come from community-mapped reference data, so every
result is approximate and the API and UI label it that way.

## Test shape

A placement test is a sequence of **stages**, one per band, in ascending
difficulty (`N5 → N1`, `A1 → C2`). Bands come from the lesson level catalog
(`lesson.LevelsFor`), so a new language is configuration, not new code.

Each stage holds 10 auto-scorable multiple-choice items (4 options each)
drawn from the band's reference partition:

| Kind | Count | Prompt | Correct option | Distractors |
|---|---|---|---|---|
| `vocab` | 6 | Vocabulary headword | First gloss | Trap-ranked glosses of other band vocabulary |
| `grammar` | 2 | Grammar example sentence | Example translation | Trap-ranked band grammar example translations |
| `reading` | 2 | Vocabulary example sentence | Example translation | Trap-ranked band vocabulary example translations |

Distractors are **traps**, not random draws — each candidate distractor is
scored for how plausible it stays under partial knowledge and the top three
are used. Vocabulary distractors prefer words sharing Han characters with the
headword (recognising one kanji must not identify the answer), a matching
part of speech, a matching gloss shape (verb glosses against verb glosses),
and a similar reading. Sentence distractors prefer translations sharing
content words with the correct translation and sentences sharing Han
characters with the prompt, so understanding a fragment of the sentence still
leaves several consistent options. Ties fall back to random pool order.

When a band lacks enough grammar topics or example sentences, the missing
items fall back to `vocab` kind; a band without at least 4 usable vocabulary
entries cannot be assembled and the start request fails. Correct items are
sampled randomly per session, options are shuffled, and correct indexes never
leave the backend while the assessment is in progress.

Stages are built lazily: starting an assessment builds only the first stage;
each stage is appended when the previous one is passed. A learner who fails
the first stage answers 10 items; reaching the top band of a five-band plan
means 50. Sessions are untimed.

## Progression and scoring

- A stage **passes** at ≥ 75% correct (8 of 10).
- Passing the last band, or failing any stage, **completes** the assessment.
- The estimate is the **highest passed band**; when no stage passes, the
  estimate is the lowest band and the result carries `floor: true` ("start at
  the beginning" framing).
- Confidence is derived from the accuracy gap between the highest passed
  stage and the failed stage: ≥ 0.35 → `high`, ≥ 0.15 → `medium`, otherwise
  `low`. With no failed stage, final-stage accuracy ≥ 0.9 → `high`, else
  `medium`; with no passed stage, failed accuracy ≤ 0.4 → `high`, ≤ 0.6 →
  `medium`, else `low`.

Seeded answer patterns behave monotonically: a learner answering correctly
through `N3` and incorrectly above lands on `N3`; the same pattern at `N5` or
`N1` lands on those bands (covered by application tests).

## Key layout

| Record | PK | SK |
|---|---|---|
| Assessment session | `USER#<cognito sub>` | `ASSESSMENT#<assessmentId>` |
| Profile level | `USER#<cognito sub>` | `PROFILE#LEVEL#<lang>` |

The session item stores the full state: language, status (`in_progress` /
`completed`), built stages (items with correct indexes, submitted answers,
per-stage tallies), result fields, `startedAt`/`completedAt`, and a `version`
for optimistic concurrency (same conditional-put scheme as progress items).
Completion writes the session and the profile level in one transaction; the
latest completed assessment always owns the profile level. Listing is a
key-scoped `Query` on the `ASSESSMENT#` prefix. Never `Scan`.

## API surface (Go)

All routes require the Cognito JWT authorizer; the owner is the token's `sub`.
The machine API does not expose assessments.

- `POST /assessments` — body `{"language": "ja"}`. Builds stage 1 and returns
  `201` with the session view.
- `POST /assessments/{id}/answers` — body
  `{"stageIndex": 0, "answers": [0, 3, ...]}`; answers are positional over
  the stage's items. `stageIndex` must match the current stage. Returns the
  updated view: either the next stage or the completed result. Submitting the
  same answers to an already-scored stage replays idempotently; diverging
  answers or concurrent writes return `409`.
- `GET /assessments` — history, newest first:
  `{"items": [{assessmentId, language, status, estimatedLevel?, confidence?,
  floor?, startedAt, completedAt?}]}`.
- `GET /assessments/{id}` — the session view.
- `GET /profile/levels` — `{"levels": [{language, level, assessmentId,
  updatedAt}]}`; the UI uses this to pre-select lesson creation and prompt
  levels. The learner can always override the default.

The session view is `{assessmentId, language, status, guidance, startedAt,
completedAt?, stage?, result?}`. `stage` (in progress) is `{index, band,
bandCount, items: [{kind, prompt, options}]}` — no correct answers. `result`
(completed) is `{estimatedLevel, confidence, floor, bands: [{band, correct,
total, passed}]}`. `guidance` is a fixed disclaimer sentence so no client can
present an estimate without its framing.
