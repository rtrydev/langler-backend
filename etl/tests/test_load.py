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

    def delete_item(self, Key):
        self.deleted.append(Key)


class FakeTable:
    def __init__(self, pages=None):
        self.writer = FakeBatchWriter()
        self.writer.deleted = []
        self.pages = pages or []
        self.queries = []

    def batch_writer(self):
        return self.writer

    def query(self, **kwargs):
        self.queries.append(kwargs)
        return self.pages[len(self.queries) - 1]


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


def test_iter_records_filters_by_kind(tmp_path):
    ref = tmp_path / "reference" / "pl"
    ref.mkdir(parents=True)
    (ref / "vocab.jsonl").write_text('{"SK":"VOCAB#A1#1"}\n', encoding="utf-8")
    (ref / "grammar.jsonl").write_text('{"SK":"GRAMMAR#A1#one"}\n', encoding="utf-8")

    assert list(load.iter_records(ref, "grammar")) == [{"SK": "GRAMMAR#A1#one"}]


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


def test_sync_embeddings_filters_by_language(tmp_path):
    embeddings = tmp_path / "embeddings"
    embeddings.mkdir()
    (embeddings / "ja-vocab.embed").write_bytes(b"ja")
    (embeddings / "pl-vocab.embed").write_bytes(b"pl")
    s3 = FakeS3()

    uploaded = load.sync_embeddings(s3, "assets-bucket", embeddings, "pl")

    assert uploaded == 1
    assert s3.calls[0]["Key"] == "embeddings/pl-vocab.embed"
    assert s3.calls[0]["Body"] == b"pl"
    assert s3.calls[0]["CacheControl"] == "public, max-age=300, must-revalidate"


def _topics_dir(tmp_path, keys):
    ref = tmp_path / "reference" / "ja"
    ref.mkdir(parents=True)
    (ref / "topics.jsonl").write_text(
        "".join(json.dumps({"SK": key}) + "\n" for key in keys), encoding="utf-8"
    )
    return ref


def test_prune_topics_deletes_only_keys_absent_from_the_build(tmp_path):
    ref = _topics_dir(tmp_path, ["TOPIC#N5#food-dining", "TOPIC#N5#home-housing"])
    table = FakeTable(pages=[{
        "Items": [
            {"SK": "TOPIC#N5#food-dining"},
            {"SK": "TOPIC#N5#abstract-concepts"},
            {"SK": "TOPIC#N4#daily-life"},
        ]
    }])

    pruned = load.prune_topics(table, "ja", ref, write_rate=100000)

    assert pruned == 2
    assert table.writer.deleted == [
        {"PK": "REF#ja", "SK": "TOPIC#N5#abstract-concepts"},
        {"PK": "REF#ja", "SK": "TOPIC#N4#daily-life"},
    ]


def test_prune_topics_follows_query_pagination(tmp_path):
    ref = _topics_dir(tmp_path, ["TOPIC#N5#food-dining"])
    table = FakeTable(pages=[
        {"Items": [{"SK": "TOPIC#N5#stale-one"}], "LastEvaluatedKey": {"SK": "TOPIC#N5#stale-one"}},
        {"Items": [{"SK": "TOPIC#N5#food-dining"}, {"SK": "TOPIC#N5#stale-two"}]},
    ])

    assert load.prune_topics(table, "ja", ref, write_rate=100000) == 2
    assert len(table.queries) == 2
    assert "ExclusiveStartKey" in table.queries[1]


def test_prune_topics_skips_when_no_topics_were_built(tmp_path):
    ref = tmp_path / "reference" / "ja"
    ref.mkdir(parents=True)
    table = FakeTable()

    assert load.prune_topics(table, "ja", ref, write_rate=100000) == 0
    assert table.queries == []
