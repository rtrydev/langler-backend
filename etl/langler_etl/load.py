import json
import time
from decimal import Decimal
from pathlib import Path


def iter_records(ref_dir: Path, kind: str = "all"):
    paths = ref_dir.glob("*.jsonl") if kind == "all" else [ref_dir / f"{kind}.jsonl"]
    for path in sorted(path for path in paths if path.is_file()):
        with path.open(encoding="utf-8") as f:
            for line in f:
                if line.strip():
                    yield json.loads(line, parse_float=Decimal)


def load_table(table, records, write_rate: float) -> int:
    interval = 1.0 / write_rate
    count = 0
    with table.batch_writer() as batch:
        for record in records:
            batch.put_item(Item=record)
            count += 1
            time.sleep(interval)
    return count


def sync_assets(s3, bucket: str, assets_dir: Path) -> int:
    count = 0
    for path in sorted(assets_dir.glob("*.svg")):
        s3.put_object(
            Bucket=bucket,
            Key=f"kanjivg/{path.name}",
            Body=path.read_bytes(),
            ContentType="image/svg+xml",
            CacheControl="public, max-age=31536000, immutable",
        )
        count += 1
    return count


def sync_burmese_assets(s3, bucket: str, assets_dir: Path) -> int:
    count = 0
    for path in sorted(assets_dir.glob("*.json")):
        s3.put_object(
            Bucket=bucket,
            Key=f"burmese/{path.name}",
            Body=path.read_bytes(),
            ContentType="application/json; charset=utf-8",
            CacheControl="public, max-age=31536000, immutable",
        )
        count += 1
    return count


def sync_embeddings(s3, bucket: str, embeddings_dir: Path, language: str = "all") -> int:
    count = 0
    paths = (
        embeddings_dir.glob("*.embed")
        if language == "all"
        else [embeddings_dir / f"{language}-vocab.embed"]
    )
    for path in sorted(path for path in paths if path.is_file()):
        s3.put_object(
            Bucket=bucket,
            Key=f"embeddings/{path.name}",
            Body=path.read_bytes(),
            ContentType="application/octet-stream",
            CacheControl="public, max-age=300, must-revalidate",
        )
        count += 1
    return count


def run(
    table_name: str,
    bucket: str | None,
    out_dir: Path,
    write_rate: float,
    language: str = "all",
    kind: str = "all",
) -> tuple[int, int]:
    import boto3

    table = boto3.resource("dynamodb").Table(table_name)
    written = 0
    reference_dir = out_dir / "reference"
    ref_dirs = (
        reference_dir.iterdir()
        if language == "all"
        else [reference_dir / language]
    )
    for ref_dir in sorted(ref_dirs):
        if ref_dir.is_dir():
            written += load_table(table, iter_records(ref_dir, kind), write_rate)
    uploaded = 0
    if bucket and kind == "all":
        s3 = boto3.client("s3")
        if language in {"ja", "all"}:
            uploaded = sync_assets(s3, bucket, out_dir / "assets" / "kanjivg")
        if language in {"my", "all"}:
            uploaded += sync_burmese_assets(s3, bucket, out_dir / "assets" / "burmese")
        embeddings_dir = out_dir / "embeddings"
        if embeddings_dir.is_dir():
            uploaded += sync_embeddings(s3, bucket, embeddings_dir, language)
    return written, uploaded
