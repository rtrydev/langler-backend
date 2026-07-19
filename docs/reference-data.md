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
| Polish orthography note | `REF#pl` | `SCRIPT#ORTHOGRAPHY#<seq>` |
| Burmese script glyph | `REF#my` | `SCRIPT#BURMESE#<seq>` |
| Reading passage | `REF#<lang>` | `READING#<level>#<id>` |

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
| `level` | S | JLPT band from the community list (`N5`…`N1`) or approximate Polish CEFR band (`A1`…`C2`) |
| `levelApproximate` | BOOL | Polish and Burmese vocabulary: always true because CEFR is inferred from corpus frequency rather than an official word list |
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
| `example` | M | `{text S, translation S, sourceId S, license S}` — original example sentence |
| `category` | S | Polish only: `morphology`, `syntax`, `word-formation`, or `register` |
| `introducedAt` | S | Polish only: first CEFR level at which the topic appears |
| `descriptorRef` | S | Polish only: section in the official adult certification inventory |

Polish grammar records use the state certification curriculum as their primary
source and additionally carry `attribution.wording` for the hand-reviewed
Langler description, `attribution.morphologyValidation` for Morfeusz with SGJP
data, and `attribution.evidence` for NKJP. When a
matching sentence is found in the locally supplied NKJP 1M corpus, an
`evidence` map preserves its text, source id, and license for review; the API
continues to serve the hand-reviewed learner example. Each topic is stored at its
first-introduced level. Polish and Burmese grammar queries for a CEFR level return that level
and all lower levels in descending, target-first order; Japanese grammar queries
remain exact-level queries.

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
| `keywords` | L of S | Lowercase English keywords used to match free-text lesson topics to this topic |
| `vocabIds` | L of S | Reference ids (`N5#1234567`) of the level's words tagged with this topic |

## Script glyph item

Common: `glyph` (S), `scriptType` (S: `kana` or `kanji`), `readings` (M of L of S).

Polish orthography notes use `scriptType = orthography`, a display pattern in
`glyph`, the note title in `name`, and the explanation plus examples in
`meanings`. They are returned by `GET /reference/scripts?lang=pl&type=orthography`.

Burmese script records use `scriptType = burmese` and
`readings.romanization`, exported from the myanmar-ime-aligned Hybrid Burmese
tables. They are returned by `GET /reference/scripts?lang=my&type=burmese`.

## Reading passage item

`text`, `level`, `levelApproximate`, and `coverage` describe a Unicode-normalized
passage selected from an explicitly licensed corpus. Burmese levels are approximate:
the 90th-percentile segmented token rank and at least 80% C4-lexicon coverage determine
the band. `GET /reference/readings?lang=my&level=A2` retrieves one difficulty band.

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
The pruned GPL-3.0 myWord client model is uploaded as
`burmese/myword-ngram.json`; the unpruned tens-of-megabytes source model remains an
offline ETL input.

`langler-etl embed` builds a vocabulary embedding index (`embeddings/ja-vocab.embed`,
~8 MB) by embedding every built vocab record (headword + reading + glosses) with
Bedrock cohere.embed-multilingual-v3 and quantizing the unit vectors to int8.
Binary layout: 4-byte big-endian header length, JSON header
(`{version, model, dims, count, ids}`), then `count × dims` int8 bytes.
`langler-etl load --assets-bucket` uploads it next to the SVGs; the API Lambda
fetches each configured language index once per container over the CDN for
semantic topic matching.

## API surface (Go)

- `GET /reference/vocab?lang&level&topic&limit&cursor`
- `GET /reference/grammar?lang&level&limit&cursor`
- `GET /reference/scripts?lang&type&level&limit&cursor`
- `GET /reference/readings?lang&level&limit&cursor`

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
