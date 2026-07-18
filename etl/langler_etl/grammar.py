import json
from importlib import resources

from .sources import CURATED_GRAMMAR


def grammar_records() -> list[dict]:
    topics = json.loads(
        resources.files("langler_etl.data").joinpath("grammar_ja.json").read_text("utf-8")
    )
    return [
        {
            "PK": "REF#ja",
            "SK": f"GRAMMAR#{topic['level']}#{topic['topicId']}",
            "lang": "ja",
            "topicId": topic["topicId"],
            "name": topic["name"],
            "level": topic["level"],
            "description": topic["description"],
            "example": topic["example"],
            "sourceId": CURATED_GRAMMAR.id,
            "license": CURATED_GRAMMAR.license,
        }
        for topic in topics
    ]
