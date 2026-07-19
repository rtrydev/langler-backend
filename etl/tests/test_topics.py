import re

import pytest
from conftest import FIXTURE_TOPICS

from langler_etl import topics


def _vocab(sk: str, level: str) -> dict:
    return {"PK": "REF#ja", "SK": sk, "level": level}


def test_apply_topics_sets_assignments():
    records = [_vocab("VOCAB#N5#1206900", "N5"), _vocab("VOCAB#N4#1341350", "N4")]
    topics.apply_topics(records, FIXTURE_TOPICS)
    assert records[0]["topics"] == ["school-learning"]
    assert records[1]["topics"] == ["school-learning", "language-communication"]


def test_apply_topics_rejects_missing_assignment():
    records = [_vocab("VOCAB#N5#9999999", "N5")]
    with pytest.raises(ValueError, match="9999999"):
        topics.apply_topics(records, FIXTURE_TOPICS)


def test_apply_topics_rejects_unknown_slug():
    data = {
        "topics": FIXTURE_TOPICS["topics"],
        "assignments": {"1206900": ["space-travel"]},
    }
    with pytest.raises(ValueError, match="space-travel"):
        topics.apply_topics([_vocab("VOCAB#N5#1206900", "N5")], data)


def test_topic_records_group_by_level_and_slug():
    records = [
        _vocab("VOCAB#N5#1206900", "N5"),
        _vocab("VOCAB#N5#1466420", "N5"),
        _vocab("VOCAB#N4#1341350", "N4"),
    ]
    topics.apply_topics(records, FIXTURE_TOPICS)
    items = topics.topic_records(records, FIXTURE_TOPICS)

    by_sk = {item["SK"]: item for item in items}
    assert set(by_sk) == {
        "TOPIC#N5#school-learning",
        "TOPIC#N5#nature-weather",
        "TOPIC#N4#school-learning",
        "TOPIC#N4#language-communication",
    }
    school = by_sk["TOPIC#N5#school-learning"]
    assert school["vocabIds"] == ["N5#1206900"]
    assert school["slug"] == "school-learning"
    assert school["name"] == "School & learning"
    assert school["level"] == "N5"
    assert school["lang"] == "ja"
    assert school["sourceId"] and school["license"]
    assert by_sk["TOPIC#N4#school-learning"]["vocabIds"] == ["N4#1341350"]


def test_curated_topic_file_is_consistent():
    data = topics.load_topics()
    slug_pattern = re.compile(r"^[a-z0-9][a-z0-9-]{0,63}$")
    slugs = {topic["slug"] for topic in data["topics"]}
    assert len(slugs) == len(data["topics"])
    for topic in data["topics"]:
        assert slug_pattern.match(topic["slug"])
        assert topic["name"] and topic["description"]
    assert data["assignments"]
    for word_id, assigned in data["assignments"].items():
        assert word_id.isdigit()
        assert 1 <= len(assigned) <= 3, word_id
        assert all(slug in slugs for slug in assigned), word_id
