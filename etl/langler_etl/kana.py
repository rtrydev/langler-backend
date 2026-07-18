import json
from importlib import resources

from .sources import CURATED_KANA


def kana_records() -> list[dict]:
    entries = json.loads(
        resources.files("langler_etl.data").joinpath("kana.json").read_text("utf-8")
    )
    records = []
    for script, glyph_key, prefix in (("hiragana", "h", "H"), ("katakana", "k", "K")):
        for position, entry in enumerate(entries, start=1):
            records.append(
                {
                    "PK": "REF#ja",
                    "SK": f"SCRIPT#KANA#{prefix}{position:03d}",
                    "lang": "ja",
                    "glyph": entry[glyph_key],
                    "scriptType": "kana",
                    "kanaScript": script,
                    "name": f"{script} {entry.get('name', entry['romaji'][0])}",
                    "readings": {"romaji": entry["romaji"]},
                    "sourceId": CURATED_KANA.id,
                    "license": CURATED_KANA.license,
                }
            )
    return records
