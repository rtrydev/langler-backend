import json
from importlib import resources

from .sources import CURATED_TOPICS


def load_topics() -> dict:
    return json.loads(
        resources.files("langler_etl.data").joinpath("topics_ja.json").read_text("utf-8")
    )


def apply_topics(vocab: list[dict], data: dict) -> None:
    known = {topic["slug"] for topic in data["topics"]}
    assignments = data["assignments"]
    missing = []
    for record in vocab:
        word_id = record["SK"].rsplit("#", 1)[-1]
        topics = assignments.get(word_id)
        if not topics:
            missing.append(word_id)
            continue
        unknown = [slug for slug in topics if slug not in known]
        if unknown:
            raise ValueError(f"word {word_id} has unknown topic slugs {unknown}")
        record["topics"] = topics
    if missing:
        raise ValueError(
            f"{len(missing)} vocab words have no topic assignment, e.g. {missing[:5]}"
        )


def topic_records(vocab: list[dict], data: dict) -> list[dict]:
    members: dict[tuple[str, str], list[str]] = {}
    for record in vocab:
        ref_id = record["SK"].removeprefix("VOCAB#")
        for slug in record["topics"]:
            members.setdefault((record["level"], slug), []).append(ref_id)
    meta = {topic["slug"]: topic for topic in data["topics"]}
    return [
        {
            "PK": "REF#ja",
            "SK": f"TOPIC#{level}#{slug}",
            "lang": "ja",
            "slug": slug,
            "name": meta[slug]["name"],
            "description": meta[slug]["description"],
            "level": level,
            "vocabIds": ids,
            "sourceId": CURATED_TOPICS.id,
            "license": CURATED_TOPICS.license,
        }
        for (level, slug), ids in sorted(members.items())
    ]
