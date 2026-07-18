import json
from pathlib import Path

from .examples import ExampleIndex
from .sources import SOURCES


def load_words(path: Path) -> list[dict]:
    with path.open(encoding="utf-8") as f:
        return json.load(f)["words"]


def freq_band(headword: str, reading: str) -> int | None:
    import wordfreq

    zipf = wordfreq.zipf_frequency(headword, "ja")
    if zipf == 0:
        zipf = wordfreq.zipf_frequency(reading, "ja")
    if zipf == 0:
        return None
    return min(max(round(8 - zipf), 1), 8)


def build_vocab(
    words: list[dict],
    levels: dict[str, str],
    examples: ExampleIndex,
    band=freq_band,
) -> list[dict]:
    records = []
    for word in words:
        kanji_forms = [k["text"] for k in word.get("kanji", [])]
        kana_forms = [k["text"] for k in word.get("kana", [])]
        if not kana_forms:
            continue
        headword = kanji_forms[0] if kanji_forms else kana_forms[0]
        reading = kana_forms[0]
        level = levels.get(headword)
        if level is None and not kanji_forms:
            level = levels.get(reading)
        if level is None:
            continue
        records.append(
            _record(word, headword, reading, level, band(headword, reading), examples.lookup(headword))
        )
    return records


def _record(
    word: dict,
    headword: str,
    reading: str,
    level: str,
    band: int | None,
    example: dict | None,
) -> dict:
    sense = word["sense"][0]
    source = SOURCES["jmdict-simplified"]
    jlpt_source = SOURCES["tanos-jlpt"]
    item = {
        "PK": "REF#ja",
        "SK": f"VOCAB#{level}#{word['id']}",
        "lang": "ja",
        "headword": headword,
        "reading": reading,
        "gloss": [g["text"] for g in sense["gloss"]][:5],
        "pos": list(sense["partOfSpeech"]),
        "level": level,
        "topics": [],
        "sourceId": source.id,
        "license": source.license,
        "attribution": {
            "level": {"sourceId": jlpt_source.id, "license": jlpt_source.license},
        },
    }
    if band is not None:
        freq_source = SOURCES["wordfreq"]
        item["freqBand"] = band
        item["attribution"]["frequency"] = {"sourceId": freq_source.id, "license": freq_source.license}
    if example is not None:
        tatoeba = SOURCES["tatoeba"]
        item["example"] = {**example, "sourceId": tatoeba.id, "license": tatoeba.license}
    return item
