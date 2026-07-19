# langler-etl

Offline, run-on-demand ETL that ingests the Japanese, Burmese, and Polish reference sources, normalizes them
to the item contract in [`docs/reference-data.md`](../docs/reference-data.md), and loads
the DynamoDB reference partition (`PK=REF#ja`) plus the KanjiVG stroke SVGs in the
assets S3 bucket. Re-running it is idempotent: keys are deterministic and every load is
an overwrite in place.

## Sources

| Source | URL | License | Attribution |
|---|---|---|---|
| jmdict-simplified (JMdict JSON) | https://github.com/scriptin/jmdict-simplified (latest release, `jmdict-eng-*.json.zip`) | CC BY-SA 4.0 (EDRDG) | JMdict, Electronic Dictionary Research and Development Group; JSON by scriptin/jmdict-simplified |
| KANJIDIC2 | http://www.edrdg.org/kanjidic/kanjidic2.xml.gz | CC BY-SA 4.0 (EDRDG) | KANJIDIC2, Electronic Dictionary Research and Development Group |
| KanjiVG | https://github.com/KanjiVG/kanjivg (latest release, `kanjivg-*-main.zip`) | CC BY-SA 3.0 | KanjiVG, Ulrich Apel |
| KRADFILE | http://ftp.edrdg.org/pub/Nihongo/kradfile.gz | CC BY-SA 4.0 (EDRDG) | KRADFILE, Electronic Dictionary Research and Development Group |
| Tanaka/Tatoeba examples | http://ftp.edrdg.org/pub/Nihongo/examples.utf.gz | CC BY 2.0 FR | Tatoeba / Tanaka Corpus, as distributed by EDRDG |
| JLPT vocabulary lists | https://github.com/jamsinclair/open-anki-jlpt-decks (`src/n5.csv`…`n1.csv`) | CC BY (Jonathan Waller, tanos.co.uk) | Jonathan Waller, tanos.co.uk |
| wordfreq (frequency bands) | https://github.com/rspeer/wordfreq | Apache 2.0 (data CC BY-SA 4.0) | wordfreq, Robyn Speer |
| Kana inventory | `langler_etl/data/kana.json` | CC0 | Langler project, original |
| Grammar topics | `langler_etl/data/grammar_ja.json` | CC BY-SA 4.0 | Langler project, original wording (topic naming follows conventional JLPT inventories) |
| Kaikki Polish / Wiktextract | https://kaikki.org/dictionary/Polish/ | CC BY-SA 4.0 + GFDL | Kaikki.org, Wiktionary contributors, Tatu Ylonen |
| NKJP 1M subcorpus | http://clip.ipipan.waw.pl/NationalCorpusOfPolish | CC BY 3.0 | National Corpus of Polish |
| NKJP frequency list | http://www.nkjp.pl/ | NKJP terms of use | National Corpus of Polish |
| Tatoeba Polish-English | https://tatoeba.org/en/downloads | CC BY 2.0 FR | Tatoeba contributors |
| Polish certification curriculum | https://certyfikatpolski.pl/wp-content/uploads/2018/05/rozp_26_2_16.pdf | Official legal text (Dz.U. 2016 poz. 405) | State adult A1–C2 grammar inventories |
| Morfeusz 2 with SGJP data | https://morfeusz.sgjp.pl/ | BSD-2-Clause | Institute of Computer Science PAS and SGJP authors |
| Polish grammar, topics, and orthography notes | `langler_etl/data/{grammar,topics,orthography}_pl.json` | CC BY-SA 4.0 / CC0 | Langler project, hand-reviewed original wording mapped to the certification curriculum |
| Kaikki Burmese / Wiktextract | https://kaikki.org/dictionary/Burmese/ | CC BY-SA 4.0 + GFDL | Kaikki.org, Wiktionary contributors, Tatu Ylonen |
| myG2P headwords | https://github.com/ye-kyaw-thu/myG2P | CC BY-NC-SA 4.0 | Ye Kyaw Thu |
| Myanmar-C4 frequency lexicon | https://huggingface.co/datasets/chuuhtetnaing/myanmar-c4-dataset | ODC-BY 1.0 | C4 counts prepared by the myanmar-ime corpus builder |
| myWord n-grams | https://github.com/ye-kyaw-thu/myWord | GPL-3.0 | Ye Kyaw Thu; pruned client segmentation asset |
| Burmese grammar, topics, and script inventory | `langler_etl/data/{grammar,topics,script}_my.json` | CC BY-SA 4.0 / CC0 | Langler project, hand-reviewed A1–B1 grammar and myanmar-ime-aligned romanization |

The same registry lives in `langler_etl/sources.py`; every emitted record pulls its
`sourceId`/`license` from there, and `manifest.json` in the build output records the
registry plus the URLs actually downloaded.

## Setup

```sh
cd etl
python3 -m venv .venv
.venv/bin/pip install -e ".[dev]"
.venv/bin/python -m pytest
```

`wordfreq` is installed with its `cjk` extra — Japanese lookups need the MeCab
tokenizer it brings.

If your AWS credentials come from `aws login` (the login credential provider),
`load` additionally needs `.venv/bin/pip install "botocore[crt]"`.

## Refreshing the data

```sh
langler-etl download                 # cache all sources into etl/.data/ (skips files already present)
langler-etl build                    # normalize into etl/.build/ (JSONL + SVG assets + manifest.json)
langler-etl load --language ja --table <table> --assets-bucket <bucket>
langler-etl load --language pl --table <table> --assets-bucket <bucket>
langler-etl load --language my --table <table> --assets-bucket <bucket>
langler-etl load --language pl --kind grammar --table <table>
```

Use `--language pl` on `download`, `build`, or `embed` for Polish, or
`--language all` on `download` and `build`. The Polish download fetches the
CC-BY NKJP `1grams.gz` archive and converts its frequency-sorted occurrence
counts into `.data/nkjp-frequency.tsv` ranks before fetching Kaikki and the
Tatoeba Polish, English, and link exports. An already downloaded `1grams.gz`
is reused. For grammar evidence, put `NKJP-PodkorpusMilionowy-1.2.tar.gz`
directly in `.data/`; the build streams its annotated sentences without extracting
the 2.4 GB archive. An already extracted `.data/nkjp-1m/` directory also works.
Polish grammar topics are stored only at their first certification level, validated
against Morfeusz during the build, and served cumulatively through the requested CEFR
level.

For Burmese, `download --language my` fetches Kaikki and myG2P v2. Copy the myanmar-ime
`BurmeseLexiconSource.tsv` corpus-builder artifact and myangler-web `ngram.json`
into `.data/` before building. Every string passes through `myanmar-tools`
Zawgyi detection, conversion when needed, and NFC cleanup. The full n-gram model
stays offline; the build emits a frequency-pruned
`assets/burmese/myword-ngram.json`. Optional reading corpora live under
`.data/readings-my/` with a `manifest.json` containing `{file, sourceId, license}`
for each JSONL source. Accepted pairs are Myanmar C4 or FineWeb2 with `ODC-BY 1.0`,
Myanmar Wikipedia with `CC BY-SA 3.0 and GFDL`, and Myanmar FLORES with
`CC BY-SA 4.0`. CulturaX accepts `ODC-BY 1.0` only for rows whose `source` is
`mC4`; OSCAR rows remain excluded because only their packaging and metadata are
CC0. The build rejects every other source/license pair, then retains passages with
at least 80% frequency-lexicon coverage and labels their difficulty approximate.

- `download` resolves the latest jmdict-simplified and KanjiVG releases via the GitHub
  API; delete a file from `.data/` to force a re-download.
- `build` writes `reference/<lang>/{vocab,grammar,scripts,topics}.jsonl` already in final DynamoDB
  item shape, `assets/kanjivg/*.svg` for exactly the ingested kanji, and
  `assets/burmese/myword-ngram.json` when its source is present, and language
  manifests with per-level counts and the source audit trail.
- `load` batch-upserts the selected language's JSONL records (PutItem overwrite
  semantics) throttled to `--write-rate` items/sec (default 20, sized for a 25 WCU
  table). When `--assets-bucket` is set, it syncs that language's embedding index and
  Japanese SVGs when applicable. Use `--language all` to publish every built language.
  Use `--kind` to refresh one record family without rewriting the rest of the partition;
  asset syncing occurs only for `--kind all`.

Required AWS permissions for `load`:

- `dynamodb:BatchWriteItem` and `dynamodb:PutItem` on the target table
- `s3:PutObject` on `arn:aws:s3:::<assets-bucket>/kanjivg/*` (only when
  `--assets-bucket` is given), plus `arn:aws:s3:::<assets-bucket>/{burmese,embeddings}/*`

Credentials come from the standard AWS SDK chain (`AWS_PROFILE`, environment, SSO).
