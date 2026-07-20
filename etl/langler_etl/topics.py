import json
from collections import Counter, defaultdict
from importlib import resources

from .sources import CURATED_BURMESE_TOPICS, CURATED_POLISH_TOPICS, CURATED_TOPICS

TOPIC_SOURCES = {
    "ja": CURATED_TOPICS,
    "pl": CURATED_POLISH_TOPICS,
    "my": CURATED_BURMESE_TOPICS,
}

MAX_SLUGS_PER_WORD = 3
MAX_VOCAB_IDS = 5000
# A topic holding more than this share of the lexicon is a catch-all bucket, not a theme a learner picks.
MAX_TOPIC_SHARE = 0.25
# Share is only meaningful across a real lexicon; test and sample corpora are far smaller.
MIN_VOCAB_FOR_SHARE_CHECK = 500


def load_topics(language: str = "ja") -> dict:
    return json.loads(
        resources.files("langler_etl.data")
        .joinpath(f"topics_{language}.json")
        .read_text("utf-8")
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
        if len(topics) > MAX_SLUGS_PER_WORD:
            raise ValueError(
                f"word {word_id} has {len(topics)} topic slugs, maximum is {MAX_SLUGS_PER_WORD}"
            )
        record["topics"] = list(dict.fromkeys(topics))
    if missing:
        raise ValueError(
            f"{len(missing)} vocab words have no topic assignment, e.g. {missing[:5]}"
        )
    _reject_catch_all(vocab)


def _reject_catch_all(vocab: list[dict]) -> None:
    if len(vocab) < MIN_VOCAB_FOR_SHARE_CHECK:
        return
    counts = Counter(slug for record in vocab for slug in record["topics"])
    limit = len(vocab) * MAX_TOPIC_SHARE
    oversized = sorted(
        (slug, count) for slug, count in counts.items() if count > limit
    )
    if oversized:
        summary = ", ".join(f"{slug} {count} ({count / len(vocab):.1%})" for slug, count in oversized)
        raise ValueError(
            f"topics exceed the {MAX_TOPIC_SHARE:.0%} catch-all threshold over {len(vocab)} words: {summary}"
        )


def topic_records(vocab: list[dict], data: dict, language: str = "ja") -> list[dict]:
    source = TOPIC_SOURCES[language]
    members: dict[tuple[str, str], list[str]] = defaultdict(list)
    for record in vocab:
        ref_id = record["SK"].removeprefix("VOCAB#")
        for slug in record["topics"]:
            members[(record["level"], slug)].append(ref_id)
    meta = {topic["slug"]: topic for topic in data["topics"]}
    return [
        {
            "PK": f"REF#{language}",
            "SK": f"TOPIC#{level}#{slug}",
            "lang": language,
            "slug": slug,
            "name": meta[slug]["name"],
            "description": meta[slug]["description"],
            "level": level,
            "keywords": meta[slug]["keywords"],
            "vocabIds": ids[:MAX_VOCAB_IDS],
            "sourceId": source.id,
            "license": source.license,
        }
        for (level, slug), ids in sorted(members.items())
    ]
