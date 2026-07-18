import argparse
import json
from pathlib import Path

ETL_ROOT = Path(__file__).resolve().parent.parent


def main(argv=None) -> None:
    parser = argparse.ArgumentParser(prog="langler-etl")
    sub = parser.add_subparsers(dest="command", required=True)

    download_cmd = sub.add_parser("download", help="download and cache all upstream sources")
    download_cmd.add_argument("--data-dir", type=Path, default=ETL_ROOT / ".data")

    build_cmd = sub.add_parser("build", help="normalize sources into DynamoDB-shaped JSONL and assets")
    build_cmd.add_argument("--data-dir", type=Path, default=ETL_ROOT / ".data")
    build_cmd.add_argument("--out", type=Path, default=ETL_ROOT / ".build")

    load_cmd = sub.add_parser("load", help="upsert built records into DynamoDB and sync assets to S3")
    load_cmd.add_argument("--table", required=True)
    load_cmd.add_argument("--assets-bucket")
    load_cmd.add_argument("--out", type=Path, default=ETL_ROOT / ".build")
    load_cmd.add_argument("--write-rate", type=float, default=20.0)

    args = parser.parse_args(argv)
    if args.command == "download":
        from . import download

        download.download_all(args.data_dir)
    elif args.command == "build":
        from . import build

        manifest = build.build(args.data_dir, args.out)
        print(json.dumps(manifest["counts"], indent=2))
    elif args.command == "load":
        from . import load

        written, uploaded = load.run(args.table, args.assets_bucket, args.out, args.write_rate)
        print(f"wrote {written} items, uploaded {uploaded} assets")
