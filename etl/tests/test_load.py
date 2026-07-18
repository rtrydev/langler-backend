import json
from decimal import Decimal

from langler_etl import load


class FakeBatchWriter:
    def __init__(self):
        self.items = []

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False

    def put_item(self, Item):
        self.items.append(Item)


class FakeTable:
    def __init__(self):
        self.writer = FakeBatchWriter()

    def batch_writer(self):
        return self.writer


class FakeS3:
    def __init__(self):
        self.calls = []

    def put_object(self, **kwargs):
        self.calls.append(kwargs)


def test_iter_records_parses_numbers_as_decimal(tmp_path):
    ref = tmp_path / "reference" / "ja"
    ref.mkdir(parents=True)
    (ref / "vocab.jsonl").write_text(
        json.dumps({"SK": "VOCAB#N5#1", "freqBand": 3, "score": 1.5}) + "\n",
        encoding="utf-8",
    )
    records = list(load.iter_records(ref))
    assert records == [{"SK": "VOCAB#N5#1", "freqBand": 3, "score": Decimal("1.5")}]


def test_load_table_puts_every_record():
    table = FakeTable()
    records = [{"PK": "REF#ja", "SK": f"VOCAB#N5#{n}"} for n in range(5)]
    written = load.load_table(table, iter(records), write_rate=100000)
    assert written == 5
    assert table.writer.items == records


def test_sync_assets_sets_content_type_and_cache_control(tmp_path):
    assets = tmp_path / "assets" / "kanjivg"
    assets.mkdir(parents=True)
    (assets / "06c34.svg").write_text("<svg/>", encoding="utf-8")
    (assets / "04e9c.svg").write_text("<svg/>", encoding="utf-8")
    s3 = FakeS3()
    uploaded = load.sync_assets(s3, "assets-bucket", assets)
    assert uploaded == 2
    assert [c["Key"] for c in s3.calls] == ["kanjivg/04e9c.svg", "kanjivg/06c34.svg"]
    for call in s3.calls:
        assert call["Bucket"] == "assets-bucket"
        assert call["ContentType"] == "image/svg+xml"
        assert call["CacheControl"] == "public, max-age=31536000, immutable"
