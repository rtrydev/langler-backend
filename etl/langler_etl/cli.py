import argparse
import json
from pathlib import Path

ETL_ROOT = Path(__file__).resolve().parent.parent


def main(argv=None) -> None:
    parser = argparse.ArgumentParser(prog="langler-etl")
    sub = parser.add_subparsers(dest="command", required=True)

    download_cmd = sub.add_parser("download", help="download and cache all upstream sources")
    download_cmd.add_argument("--data-dir", type=Path, default=ETL_ROOT / ".data")
    download_cmd.add_argument("--language", choices=("ja", "pl", "all"), default="ja")

    build_cmd = sub.add_parser("build", help="normalize sources into DynamoDB-shaped JSONL and assets")
    build_cmd.add_argument("--data-dir", type=Path, default=ETL_ROOT / ".data")
    build_cmd.add_argument("--out", type=Path, default=ETL_ROOT / ".build")
    build_cmd.add_argument("--language", choices=("ja", "pl", "all"), default="ja")

    load_cmd = sub.add_parser("load", help="upsert built records into DynamoDB and sync assets to S3")
    load_cmd.add_argument("--table", required=True)
    load_cmd.add_argument("--assets-bucket")
    load_cmd.add_argument("--out", type=Path, default=ETL_ROOT / ".build")
    load_cmd.add_argument("--write-rate", type=float, default=20.0)
    load_cmd.add_argument("--language", choices=("ja", "pl", "all"), default="all")
    load_cmd.add_argument(
        "--kind", choices=("vocab", "grammar", "scripts", "topics", "all"), default="all"
    )

    embed_cmd = sub.add_parser("embed", help="embed built vocab with Bedrock into a binary vector index")
    embed_cmd.add_argument("--out", type=Path, default=ETL_ROOT / ".build")
    embed_cmd.add_argument("--region", default="eu-central-1")
    embed_cmd.add_argument("--model")
    embed_cmd.add_argument("--language", choices=("ja", "pl"), default="ja")

    args = parser.parse_args(argv)
    if args.command == "download":
        from . import download

        if args.language in {"ja", "all"}:
            download.download_all(args.data_dir)
        if args.language in {"pl", "all"}:
            download.download_polish(args.data_dir)
    elif args.command == "build":
        from . import build

        manifests = []
        if args.language in {"ja", "all"}:
            manifests.append(build.build(args.data_dir, args.out))
        if args.language in {"pl", "all"}:
            from . import build_polish

            manifests.append(build_polish.build(args.data_dir, args.out))
        print(json.dumps({manifest.get("language", "ja"): manifest["counts"] for manifest in manifests}, indent=2))
    elif args.command == "load":
        from . import load

        written, uploaded = load.run(
            args.table,
            args.assets_bucket,
            args.out,
            args.write_rate,
            args.language,
            args.kind,
        )
        print(f"wrote {written} items, uploaded {uploaded} assets")
    elif args.command == "embed":
        from . import embeddings

        path = embeddings.embed_corpus(
            args.out, args.region, args.model or embeddings.MODEL_ID, args.language
        )
        print(f"wrote {path}")
