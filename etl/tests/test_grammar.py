import re
from collections import Counter

import pytest

from langler_etl.grammar import grammar_records

KEBAB = re.compile(r"^[a-z0-9]+(-[a-z0-9]+)*$")


@pytest.fixture(scope="module")
def records():
    return grammar_records()


def test_counts_per_level(records):
    counts = Counter(r["level"] for r in records)
    assert counts == {"N5": 25, "N4": 25, "N3": 20, "N2": 20, "N1": 15}


def test_topic_ids_unique_and_kebab_case(records):
    ids = [r["topicId"] for r in records]
    assert len(set(ids)) == len(ids)
    for topic_id in ids:
        assert KEBAB.match(topic_id), topic_id


def test_every_topic_complete(records):
    for record in records:
        assert record["PK"] == "REF#ja"
        assert record["SK"] == f"GRAMMAR#{record['level']}#{record['topicId']}"
        assert record["lang"] == "ja"
        assert record["name"]
        assert record["description"]
        assert record["example"]["text"]
        assert record["example"]["translation"]
        assert record["sourceId"] == "langler-curated"
        assert record["license"] == "CC BY-SA 4.0"


def test_levels_valid(records):
    assert {r["level"] for r in records} == {"N5", "N4", "N3", "N2", "N1"}
