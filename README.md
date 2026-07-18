# langler-backend

Go backend for [Langler](https://langler.rtrydev.com) — an invitation-only, BYOAI language-learning app for Japanese, Burmese, and Polish.

Self-contained Go binaries (`bootstrap`) running on AWS Lambda (`provided.al2023`, arm64) behind an API Gateway HTTP API. One Lambda per route group under `cmd/`. Storage is a DynamoDB single table. This repo also hosts the offline Python ETL for reference data under `etl/`.

## Layout

```
cmd/api/                    Lambda entrypoint: wires adapters into services (API Gateway HTTP API v2 payload)
cmd/authorizer/             Lambda entrypoint: validates scoped machine tokens
internal/domain/            entities, value objects, domain services (pure Go)
internal/ports/inbound/     use-case interfaces the outside world invokes
internal/ports/outbound/    capabilities the domain requires from the outside world
internal/application/       use-case orchestration implementing inbound ports
internal/adapters/inbound/  Lambda handlers and event decoders
internal/adapters/outbound/ DynamoDB, S3, SQS, HTTP client implementations
build/                      zipped bootstrap artifacts (gitignored), consumed by terraform
```

## Build

```sh
make build      # cross-compiles all Lambdas for linux/arm64 into build/<name>.zip
make test
```

Infrastructure lives in `langler-tf-infrastructure`, which points at the zip artifacts produced here.

The browser API uses Cognito access tokens. Agent frameworks use a separate
machine API guarded by expiring, revocable `lang_sk_` tokens; see
[`docs/agent-authoring.md`](docs/agent-authoring.md).

## Reference data ETL

The offline Python pipeline that ingests the Japanese reference sources (JMdict, KANJIDIC2, KanjiVG, KRADFILE, Tanaka/Tatoeba examples, JLPT lists) and loads the DynamoDB reference partition and the assets bucket lives in [`etl/`](etl/README.md). Refresh sequence: `langler-etl download` → `build` → `load`.
