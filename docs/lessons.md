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
| `cloze` | `{"text": "…{{1}}…", "blanks": [{"index": 1, "answer": "…", "hint"?}]}` — markers and blanks must match 1:1 |
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
   ids). A `connected` lesson must contain a `reading` exercise with
   `genre: short_story`, a title, a non-empty passage, and at least one question;
   `foundational` is the explicit opt-out while a learner cannot decode connected
   text.
3. **Language hooks** (`domain/lesson` script hooks): Japanese content fields must
   contain Japanese script; a slot exists for Burmese orthography checks later.
4. **Sanitization**: control characters and HTML/markup are rejected everywhere;
   imported strings are data, never instructions.
5. **Reference integrity** (`application/lessons`): every `referencedVocab` /
   `referencedGrammar` id must exist in the reference partition for the lesson
   language (`BatchGetItem` on `REF#<lang>` / `VOCAB#<id>`·`GRAMMAR#<id>`).
6. **Idempotency**: `PutItem` with `attribute_not_exists(PK)`; re-importing an
   existing `lessonId` is a no-op that returns the stored lesson
   (`"created": false`). The SHA-256 of the raw import body is stored as
   `contentHash`.

Failures return `400 {"error": "lesson validation failed", "issues": [{"path",
"message"}]}` with all issues collected in one pass, so the user can paste the
report back to their AI.

## Key layout

| Record | PK | SK |
|---|---|---|
| Lesson | `USER#<cognito sub>` | `LESSON#<lessonId>` |

Items store the full lesson document under `dynamodbav` attributes plus
`createdAt` (RFC 3339) and `contentHash`. Listing is a key-scoped `Query`
(`begins_with(SK, "LESSON#")`) with the same opaque cursor scheme as the
reference API. Never `Scan`.

## API surface (Go)

All routes require the Cognito JWT authorizer; the owner is the token's `sub`.

- `POST /lessons/prompt` — build the copy-paste generation prompt.
  Body: `{"language", "level", "topic"?, "exerciseTypes": [...],
  "readingStage"?, "length"?: "short"|"standard"|"long", "includeReference"?}`.
  `connected` (default) forces a `reading` exercise into the requested types and
  demands a grounded short story; `foundational` removes it. With
  `includeReference` (default true) a slice of level-matched vocab and grammar
  with their reference ids is embedded.
- `POST /lessons/import` — validate and store a lesson document. `201` with a
  summary on first import, `200` with `"created": false` on replay.
- `GET /lessons?limit&cursor` — summaries (`{"items": [...], "nextCursor"}`).
- `GET /lessons/{id}` — the full stored document plus `createdAt`.
- `DELETE /lessons/{id}` — `204`; `404` if absent.
