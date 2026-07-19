# Reference Data Contract

Shared contract between the Python ETL (`etl/`) and the Go reference API. Both sides
must conform to the item shapes below; changing an attribute here is a breaking change
for the other side.

All reference data lives in the application DynamoDB table under a per-language
partition. Keys are language-agnostic: Japanese uses `ja`; Polish (`pl`) and Burmese
(`my`) reuse the same layout later without API changes.

## Key layout

| Record | PK | SK |
|---|---|---|
| Vocabulary | `REF#<lang>` | `VOCAB#<level>#<id>` |
| Grammar topic | `REF#<lang>` | `GRAMMAR#<level>#<topicId>` |
| Script glyph (kana) | `REF#<lang>` | `SCRIPT#KANA#<seq>` |
| Script glyph (kanji) | `REF#<lang>` | `SCRIPT#KANJI#<level>#<glyph>` |

- `<level>` is an uppercase band label (`N5`…`N1` for Japanese; CEFR bands like `A1`
  for other languages later).
- Vocabulary `<id>` is the stable upstream identifier (JMdict sequence number), so
  re-running the ETL overwrites in place and stays idempotent.
- Kana `<seq>` is a zero-padded gojūon ordering key (`H001`… for hiragana, `K001`…
  for katakana) so a key-ordered query returns the inventory in teaching order.
- All queries are `Query` with `PK = REF#<lang>` and a `begins_with` condition on
  `SK`. Never `Scan`.

## Common attributes

Every record carries:

| Attribute | Type | Notes |
|---|---|---|
| `PK`, `SK` | S | As above |
| `lang` | S | ISO 639-1/3 code, lowercase (`ja`) |
| `sourceId` | S | Primary upstream source identifier (`jmdict-simplified`, `kanjidic2`, `tanos-jlpt`, `langler-curated`, …) |
| `license` | S | License of the primary source, human-readable (`CC BY-SA 4.0 (EDRDG)`) |

Records merged from several sources keep the primary source in `sourceId`/`license`
and declare the rest in an `attribution` map: `attribution.<part> = {sourceId, license}`
(e.g. `attribution.strokes = {sourceId: "kanjivg", license: "CC BY-SA 3.0"}`).

## Vocabulary item

| Attribute | Type | Notes |
|---|---|---|
| `headword` | S | Kanji form if it exists, else kana form |
| `reading` | S | Kana reading |
| `gloss` | L of S | English glosses, first sense first, at most 5 |
| `pos` | L of S | jmdict-simplified part-of-speech tags (`n`, `v5u`, …) |
| `level` | S | JLPT band from the community list (`N5`…`N1`) |
| `freqBand` | N | Optional. 1 (most frequent) … 8, derived from wordfreq zipf: `band = clamp(round(8 - zipf), 1, 8)` |
| `topics` | L of S | 1–3 curated topic slugs from `topics_ja.json` (`food-drink`, …); also drives the `topic` query filter |
| `example` | M | Optional: `{text S, translation S, sourceId S, license S}` (Tatoeba/Tanaka pair) |

## Grammar topic item

| Attribute | Type | Notes |
|---|---|---|
| `topicId` | S | Stable kebab-case id (`particle-wa`) |
| `name` | S | Display name (`Topic particle は`) |
| `level` | S | JLPT band |
| `description` | S | 1–2 sentence original description |
| `example` | M | `{text S, translation S}` — original example sentence |

## Topic catalog item

`SK = TOPIC#<level>#<slug>`, one item per (level, topic) pair, aggregated by the
ETL from the per-word `topics` tags in `langler_etl/data/topics_ja.json`
(18-slug curated taxonomy; every built vocab word carries 1–3 slugs, enforced
at build time).

| Attribute | Type | Notes |
|---|---|---|
| `slug` | S | Taxonomy slug (`food-drink`) |
| `name` | S | Display name (`Food & drink`) |
| `description` | S | One-line learner-facing description |
| `level` | S | JLPT band |
| `vocabIds` | L of S | Reference ids (`N5#1234567`) of the level's words tagged with this topic |

## Script glyph item

Common: `glyph` (S), `scriptType` (S: `kana` or `kanji`), `readings` (M of L of S).

Kana (`scriptType = kana`):

| Attribute | Type | Notes |
|---|---|---|
| `name` | S | e.g. `hiragana a` |
| `readings` | M | `{romaji: ["a"]}` |
| `kanaScript` | S | `hiragana` or `katakana` |

Kanji (`scriptType = kanji`):

| Attribute | Type | Notes |
|---|---|---|
| `name` | S | Primary English meaning |
| `meanings` | L of S | All English meanings |
| `readings` | M | `{on: [S], kun: [S]}` |
| `level` | S | Mapped from KANJIDIC2 `jlpt` (old 4→N5, 3→N4, 2→N2, 1→N1; approximate — no N3 in KANJIDIC2) |
| `grade` | N | Optional jōyō grade |
| `strokeCount` | N | From KANJIDIC2 |
| `strokeDataRef` | S | Asset key of the KanjiVG SVG, e.g. `kanjivg/050cd.svg` |
| `components` | L of S | Radical decomposition from KRADFILE |

## Assets

KanjiVG SVGs are uploaded unmodified to the reference-assets S3 bucket under
`kanjivg/<5-digit-lowercase-hex-codepoint>.svg` and served through CloudFront.
`strokeDataRef` stores that key; clients resolve it against the assets CDN domain.

## API surface (Go)

- `GET /reference/vocab?lang&level&topic&limit&cursor`
- `GET /reference/grammar?lang&level&limit&cursor`
- `GET /reference/scripts?lang&type&level&limit&cursor`

`lang` is required. `level`, `topic` (vocab), and `type` (scripts: `kana`/`kanji`)
are optional filters. Responses are `{"items": [...], "nextCursor": "..."}`;
`nextCursor` is an opaque pagination token (absent on the last page). `limit`
defaults to 50, capped at 200. `topic` filters on the `topics` attribute server-side
within the key-scoped query (a filter expression, not a Scan).

Vocabulary and grammar items additionally expose a stable reference id derived
from the sort key: `id` = the SK with its `VOCAB#`/`GRAMMAR#` prefix stripped
(`N4#1311125`, `N5#desu-da`). Lesson imports cite these ids in `referencedVocab`
and `referencedGrammar` (see `docs/lessons.md`); import validation resolves them
back to `VOCAB#<id>`/`GRAMMAR#<id>` keys with `BatchGetItem`.
