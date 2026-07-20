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
    assert school["keywords"] == ["school", "study"]
    assert school["sourceId"] and school["license"]
    assert by_sk["TOPIC#N4#school-learning"]["vocabIds"] == ["N4#1341350"]


@pytest.mark.parametrize("language", ["ja", "pl", "my"])
def test_curated_topic_file_is_consistent(language):
    data = topics.load_topics(language)
    slug_pattern = re.compile(r"^[a-z0-9][a-z0-9-]{0,63}$")
    slugs = {topic["slug"] for topic in data["topics"]}
    assert len(slugs) == len(data["topics"])
    for topic in data["topics"]:
        assert slug_pattern.match(topic["slug"])
        assert topic["name"] and topic["description"]
        assert topic["keywords"], topic["slug"]
        assert all(keyword == keyword.lower() for keyword in topic["keywords"])
    assert data["assignments"]
    for word_id, assigned in data["assignments"].items():
        assert 1 <= len(assigned) <= topics.MAX_SLUGS_PER_WORD, word_id
        assert len(set(assigned)) == len(assigned), word_id
        assert all(slug in slugs for slug in assigned), word_id


def test_curated_topic_files_share_the_core_taxonomy():
    by_language = {
        language: {topic["slug"]: topic for topic in topics.load_topics(language)["topics"]}
        for language in ("ja", "pl", "my")
    }
    core = set.intersection(*(set(entries) for entries in by_language.values()))
    assert len(core) >= 20
    for slug in core:
        definitions = [entries[slug] for entries in by_language.values()]
        assert len({definition["name"] for definition in definitions}) == 1, slug
        assert len({definition["description"] for definition in definitions}) == 1, slug


@pytest.mark.parametrize("language", ["ja", "pl", "my"])
def test_no_topic_file_ships_a_catch_all_slug(language):
    slugs = {topic["slug"] for topic in topics.load_topics(language)["topics"]}
    assert not slugs & {"everyday-life", "abstract-concepts", "daily-life", "misc", "other"}


def test_apply_topics_rejects_a_catch_all_sized_topic():
    total = topics.MIN_VOCAB_FOR_SHARE_CHECK
    dominant = int(total * (topics.MAX_TOPIC_SHARE + 0.1))
    records = [_vocab(f"VOCAB#N5#{index}", "N5") for index in range(total)]
    data = {
        "topics": FIXTURE_TOPICS["topics"],
        "assignments": {
            str(index): ["school-learning"] if index < dominant else ["nature-weather"]
            for index in range(total)
        },
    }
    with pytest.raises(ValueError, match="catch-all threshold"):
        topics.apply_topics(records, data)
