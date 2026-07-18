# langler-etl

Offline, run-on-demand ETL that ingests the Japanese reference sources, normalizes them
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
langler-etl load --table <table> --assets-bucket <bucket>
```

- `download` resolves the latest jmdict-simplified and KanjiVG releases via the GitHub
  API; delete a file from `.data/` to force a re-download.
- `build` writes `reference/ja/{vocab,grammar,scripts}.jsonl` already in final DynamoDB
  item shape, `assets/kanjivg/*.svg` for exactly the ingested kanji, and
  `manifest.json` with per-level counts and the source audit trail.
- `load` batch-upserts every JSONL record (PutItem overwrite semantics) throttled to
  `--write-rate` items/sec (default 20, sized for a 25 WCU table) and syncs the SVGs to
  `s3://<bucket>/kanjivg/` with `Content-Type: image/svg+xml` and
  `Cache-Control: public, max-age=31536000, immutable`. Omit `--assets-bucket` to load
  only DynamoDB.

Required AWS permissions for `load`:

- `dynamodb:BatchWriteItem` and `dynamodb:PutItem` on the target table
- `s3:PutObject` on `arn:aws:s3:::<assets-bucket>/kanjivg/*` (only when
  `--assets-bucket` is given)

Credentials come from the standard AWS SDK chain (`AWS_PROFILE`, environment, SSO).
