import json

import pytest
from conftest import FIXTURE_DATA, FIXTURE_TOPICS

from langler_etl import build


@pytest.fixture(scope="module")
def built(tmp_path_factory):
    out_dir = tmp_path_factory.mktemp("out")
    manifest = build.build(
        FIXTURE_DATA, out_dir, band=lambda headword, reading: 3, topic_data=FIXTURE_TOPICS
    )
    return out_dir, manifest


def _read_jsonl(path):
    return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines()]


def test_manifest_counts_match_output_lines(built):
    out_dir, manifest = built
    ref = out_dir / "reference" / "ja"
    vocab = _read_jsonl(ref / "vocab.jsonl")
    grammar = _read_jsonl(ref / "grammar.jsonl")
    scripts = _read_jsonl(ref / "scripts.jsonl")
    assert manifest["counts"]["vocab"]["total"] == len(vocab) == 6
    assert manifest["counts"]["vocab"] == {"N5": 4, "N4": 1, "N3": 1, "total": 6}
    assert manifest["counts"]["grammar"]["total"] == len(grammar) == 105
    assert manifest["counts"]["kanji"] == {"N5": 1, "N2": 1, "N1": 1, "total": 3}
    assert manifest["counts"]["kana"]["total"] == 208
    assert len(scripts) == 208 + 3


def test_every_item_carries_source_and_license(built):
    out_dir, _ = built
    ref = out_dir / "reference" / "ja"
    for name in ("vocab.jsonl", "grammar.jsonl", "scripts.jsonl", "topics.jsonl"):
        for item in _read_jsonl(ref / name):
            assert item["PK"] == "REF#ja"
            assert item["SK"]
            assert item["lang"] == "ja"
            assert item["sourceId"], item["SK"]
            assert item["license"], item["SK"]


def test_kanji_record_merges_svg_and_components(built):
    out_dir, _ = built
    scripts = _read_jsonl(out_dir / "reference" / "ja" / "scripts.jsonl")
    kanji = {item["glyph"]: item for item in scripts if item["scriptType"] == "kanji"}

    water = kanji["水"]
    assert water["SK"] == "SCRIPT#KANJI#N5#水"
    assert water["name"] == "water"
    assert water["readings"] == {"on": ["スイ"], "kun": ["みず", "みず-"]}
    assert water["strokeCount"] == 4
    assert water["grade"] == 1
    assert water["strokeDataRef"] == "kanjivg/06c34.svg"
    assert water["components"] == ["水"]
    assert water["attribution"]["strokes"] == {"sourceId": "kanjivg", "license": "CC BY-SA 3.0"}
    assert water["attribution"]["components"]["sourceId"] == "kradfile"

    person = kanji["者"]
    assert "strokeDataRef" not in person
    assert "strokes" not in person.get("attribution", {})


def test_assets_copied_for_ingested_kanji_only(built):
    out_dir, _ = built
    assets = sorted(p.name for p in (out_dir / "assets" / "kanjivg").glob("*.svg"))
    assert assets == ["04e9c.svg", "06c34.svg"]


def test_manifest_lists_source_registry(built):
    _, manifest = built
    by_id = {}
    for source in manifest["sources"]:
        assert source["id"] and source["url"] is not None
        assert source["license"] and source["attribution"]
        by_id.setdefault(source["id"], []).append(source)
    assert set(by_id) == {
        "jmdict-simplified",
        "kanjidic2",
        "kanjivg",
        "kradfile",
        "tatoeba",
        "tanos-jlpt",
        "wordfreq",
        "langler-curated",
    }
    assert len(by_id["langler-curated"]) == 3
