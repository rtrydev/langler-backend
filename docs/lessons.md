# Lesson Contract

Lesson schema v1: the JSON document a user's AI produces, the validation applied on
import, and the DynamoDB layout for stored lessons. The copy-paste flow (langler-ui)
and the future agentic import path share this contract.

## Document shape

```json
{
  "schemaVersion": "1.0",
  "lessonId": "<client-generated UUID>",
  "language": "ja | my | pl",
  "level": "N5-N1 for ja, A1-C2 for my/pl",
  "title": "...",
  "description": "optional",
  "topic": "optional",
  "tags": ["optional", "max 10"],
  "readingStage": "connected | foundational",
  "sourceModel": "optional generating AI",
  "estimatedMinutes": 18,
  "exercises": [
    {
      "exerciseId": "unique within the lesson",
      "type": "cloze | translation | ordering | matching | reading | writing_prompt | script_practice",
      "prompt": "learner-facing instruction",
      "points": 8,
      "referencedVocab": ["N4#1311125"],
      "referencedGrammar": ["N4#te-form"],
      "payload": {}
    }
  ]
}
```

An unknown top-level or exercise field is a validation error (`metadata` is
tolerated and ignored). Payload shapes by type:

| Type | Payload |
|---|---|
| `cloze` | `{"text": "…{{1}}…", "blanks": [{"index": 1, "answer": "…", "alternates"?: ["…"], "hint"?}]}` — markers and blanks must match 1:1 |
| `translation` | `{"source": "…", "reference"?}` |
| `ordering` | `{"items": ["…"], "translation"?}` — items in correct order, 2–20 |
| `matching` | `{"pairs": [{"left": "…", "right": "…"}]}` — 2–20 pairs |
| `reading` | `{"genre": "short_story", "title", "passage", "annotations"?: [{"surface", "reading"?, "gloss"?}], "questions": [{"question", "kind": "multiple_choice"\|"short_answer", "options"?, "answer"?}]}` |
| `writing_prompt` | optional `{"guidance"?, "modelAnswer"?}`; the task lives in `prompt` |
| `script_practice` | `{"items": [{"glyph", "reading"?, "meaning"?}]}` |

## Validation layers

1. **Structural** (`adapters/inbound/httpapi`): strict JSON decode, unknown fields
   rejected, per-type payload decode, 256 KiB body cap.
2. **Semantic** (`domain/lesson.New`): enum allow-lists, size limits, cross-field
   rules (cloze markers, multiple-choice answers among options, unique exercise
   ids). A `connected` lesson must open with a `reading` exercise (its first
   array element) with `genre: short_story`, a title, a non-empty passage, and at
   least one question, so the story introduces the language before the exercises
   test it; `foundational` is the explicit opt-out while a learner cannot decode
   connected text.
3. **Language hooks** (`domain/lesson` script hooks): Japanese content fields must
   contain Japanese script; a slot exists for Burmese orthography checks later.
4. **Sanitization**: control characters and HTML/markup are rejected everywhere;
   imported strings are data, never instructions.
5. **Reference integrity** (`application/lessons`): every `referencedVocab` /
   `referencedGrammar` id must exist in the reference partition for the lesson
   language (`BatchGetItem` on `REF#<lang>` / `VOCAB#<id>`·`GRAMMAR#<id>`).
6. **Idempotency**: `POST /lessons/import` requires an `Idempotency-Key` header.
   A transaction creates both the lesson and a user-scoped marker containing the
   lesson id. Reusing the key with the same body returns that original lesson
   (`"created": false`); reusing it with different content returns `409`. The
   SHA-256 of the raw import body is stored as `contentHash`; the idempotency key
   itself is SHA-256 hashed before storage.

Failures return `400 {"error": "lesson validation failed", "issues": [{"path",
"message"}]}` with all issues collected in one pass, so the user can paste the
report back to their AI.

## Key layout

| Record | PK | SK |
|---|---|---|
| Lesson | `USER#<cognito sub>` | `LESSON#<lessonId>` |
| Lesson result | `USER#<owner>` | `RESULT#<lessonId>#<completed timestamp>#<attemptId>` |
| Import marker | `USER#<owner>` | `IDEMPOTENCY#<sha256(key)>` |

Items store the full lesson document under `dynamodbav` attributes plus
`createdAt` (RFC 3339) and `contentHash`. Listing is a key-scoped `Query`
(`begins_with(SK, "LESSON#")`) with the same opaque cursor scheme as the
reference API. Never `Scan`.

## API surface (Go)

All routes require the Cognito JWT authorizer; the owner is the token's `sub`.

- `POST /lessons/prompt` — build the copy-paste generation prompt.
  Body: `{"language", "level", "topic"?, "topicSlug"?, "exerciseTypes": [...],
  "readingStage"?, "length"?: "short"|"standard"|"long", "includeReference"?}`.
  `connected` (default) forces a `reading` exercise into the requested types and
  demands a grounded short story that opens the lesson, followed by exercises
  ordered from recognition to production; `foundational` removes it. With
  `includeReference` (default true) a slice of level-matched vocab and grammar
  with their reference ids is embedded. The slice prefers items the owner has
  no SRS record for yet, so repeated prompts walk the level instead of
  repeating the first page. When `topicSlug` names a curated topic
  (`TOPIC#<level>#<slug>` reference items, e.g. `food-drink`), the vocab slice
  is drawn from that topic's word list instead of the whole level; an unknown
  slug for the level is a `400` validation error. A free-text `topic` without
  a slug is keyword-matched against the curated topics' `keywords` (up to the
  two best matches feed the slice); matched or not, a free-text topic switches
  the vocabulary to candidate-pool mode — a larger slice (30 matched / 40
  level-wide) plus an instruction that the generating model should build the
  lesson from the ~20 items that fit the topic and reference only those.
- `GET /lessons/topics?lang&level` — the curated topic list for a level with
  per-user coverage: `{"topics": [{"slug", "name", "description", "wordCount",
  "coveredCount"}]}`, sorted least-covered first. `coveredCount` counts the
  topic's words that already have an SRS record for the caller.
- `POST /lessons/import` — validate and store a lesson document with an
  `Idempotency-Key` header. `201` with a summary on first import, `200` with
  `"created": false` on replay. The machine API exposes this same path through
  the token owner injected by its Lambda authorizer.
- `GET /lessons?limit&cursor` — summaries (`{"items": [...], "nextCursor"}`).
- `GET /lessons/{id}` — the full stored document plus `createdAt`.
- `DELETE /lessons/{id}` — `204`; `404` if absent.
- `POST /lessons/{id}/results` — validate and persist a completed attempt with
  aggregate auto/self scores and a per-exercise breakdown. Result records are
  user-scoped raw outcomes; no SRS schedule is created at this stage.
