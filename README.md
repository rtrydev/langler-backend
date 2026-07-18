# langler-backend

Go backend for [Langler](https://langler.rtrydev.com) — an invitation-only, BYOAI language-learning app for Japanese, Burmese, and Polish.

Self-contained Go binaries (`bootstrap`) running on AWS Lambda (`provided.al2023`, arm64) behind an API Gateway HTTP API. One Lambda per route group under `cmd/`. Storage is a DynamoDB single table. This repo will also host the offline Python ETL for reference data under `tools/` (not yet scaffolded).

## Layout

```
cmd/api/        hello-world Lambda (API Gateway HTTP API v2 payload)
build/          zipped bootstrap artifacts (gitignored), consumed by terraform
```

## Build

```sh
make build      # cross-compiles all Lambdas for linux/arm64 into build/<name>.zip
make test
```

Infrastructure lives in `langler-tf-infrastructure`, which points at the zip artifacts produced here.
