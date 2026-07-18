import pytest

from langler_etl.kana import kana_records


@pytest.fixture(scope="module")
def records():
    return kana_records()


def test_full_inventory(records):
    assert len(records) == 208
    hiragana = [r for r in records if r["kanaScript"] == "hiragana"]
    katakana = [r for r in records if r["kanaScript"] == "katakana"]
    assert len(hiragana) == len(katakana) == 104


def test_gojuon_ordering_base_then_voiced_then_youon(records):
    by_sk = {r["SK"]: r for r in records}
    assert by_sk["SCRIPT#KANA#H001"]["glyph"] == "あ"
    assert by_sk["SCRIPT#KANA#H046"]["glyph"] == "ん"
    assert by_sk["SCRIPT#KANA#H047"]["glyph"] == "が"
    assert by_sk["SCRIPT#KANA#H067"]["glyph"] == "ぱ"
    assert by_sk["SCRIPT#KANA#H072"]["glyph"] == "きゃ"
    assert by_sk["SCRIPT#KANA#H104"]["glyph"] == "ぴょ"
    assert by_sk["SCRIPT#KANA#K001"]["glyph"] == "ア"
    assert by_sk["SCRIPT#KANA#K104"]["glyph"] == "ピョ"


def test_sequence_keys_unique_and_sortable(records):
    keys = [r["SK"] for r in records]
    assert len(set(keys)) == len(keys)
    hiragana_keys = [k for k in keys if "#H" in k]
    assert hiragana_keys == sorted(hiragana_keys)


def test_record_shape(records):
    a = records[0]
    assert a == {
        "PK": "REF#ja",
        "SK": "SCRIPT#KANA#H001",
        "lang": "ja",
        "glyph": "あ",
        "scriptType": "kana",
        "kanaScript": "hiragana",
        "name": "hiragana a",
        "readings": {"romaji": ["a"]},
        "sourceId": "langler-curated",
        "license": "CC0",
    }


def test_every_record_has_romaji(records):
    for record in records:
        assert record["readings"]["romaji"]
