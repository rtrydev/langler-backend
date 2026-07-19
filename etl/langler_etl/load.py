import json
import time
from decimal import Decimal
from pathlib import Path


def iter_records(ref_dir: Path):
    for path in sorted(ref_dir.glob("*.jsonl")):
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


def sync_embeddings(s3, bucket: str, embeddings_dir: Path) -> int:
    count = 0
    for path in sorted(embeddings_dir.glob("*.embed")):
        s3.put_object(
            Bucket=bucket,
            Key=f"embeddings/{path.name}",
            Body=path.read_bytes(),
            ContentType="application/octet-stream",
            CacheControl="public, max-age=300, must-revalidate",
        )
        count += 1
    return count


def run(table_name: str, bucket: str | None, out_dir: Path, write_rate: float) -> tuple[int, int]:
    import boto3

    table = boto3.resource("dynamodb").Table(table_name)
    written = load_table(table, iter_records(out_dir / "reference" / "ja"), write_rate)
    uploaded = 0
    if bucket:
        s3 = boto3.client("s3")
        uploaded = sync_assets(s3, bucket, out_dir / "assets" / "kanjivg")
        embeddings_dir = out_dir / "embeddings"
        if embeddings_dir.is_dir():
            uploaded += sync_embeddings(s3, bucket, embeddings_dir)
    return written, uploaded
