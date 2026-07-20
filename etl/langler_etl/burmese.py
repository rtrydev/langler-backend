import hashlib
import json
import re
import unicodedata
from collections import defaultdict
from importlib import resources
from pathlib import Path

from . import topics
from .sources import (
    CURATED_BURMESE_GRAMMAR,
    CURATED_BURMESE_SCRIPT,
    SOURCES,
)

LEVELS = ["A1", "A2", "B1", "B2", "C1", "C2"]
VERIFIED_READING_SOURCES = {
    "chuuhtetnaing/myanmar-c4-dataset": "ODC-BY 1.0",
    "chuuhtetnaing/myanmar-wikipedia-dataset": "CC BY-SA 3.0 and GFDL",
    "chuuhtetnaing/myanmar-fineweb-2-dataset": "ODC-BY 1.0",
    "chuuhtetnaing/myanmar-culturax-dataset": "ODC-BY 1.0",
    "chuuhtetnaing/myanmar-facebook-flores-dataset": "CC BY-SA 4.0",
}
_MYANMAR = re.compile(r"[\u1000-\u109f\uaa60-\uaa7f\ua9e0-\ua9ff]+")
_SPACE = re.compile(r"\s+")
_STACKS = {
    "\u1060": "\u1039\u1000", "\u1061": "\u1039\u1001", "\u1062": "\u1039\u1002",
    "\u1063": "\u1039\u1003", "\u1065": "\u1039\u1005", "\u1066": "\u1039\u1006",
    "\u1067": "\u1039\u1006", "\u1068": "\u1039\u1007", "\u1069": "\u1039\u1008",
    "\u106c": "\u1039\u100b", "\u106d": "\u1039\u100c", "\u1070": "\u1039\u100f",
    "\u1071": "\u1039\u1010", "\u1072": "\u1039\u1010", "\u1073": "\u1039\u1011",
    "\u1074": "\u1039\u1011", "\u1075": "\u1039\u1012", "\u1076": "\u1039\u1013",
    "\u1077": "\u1039\u1014", "\u1078": "\u1039\u1015", "\u1079": "\u1039\u1016",
    "\u107a": "\u1039\u1017", "\u107b": "\u1039\u1018", "\u107c": "\u1039\u1019",
    "\u1085": "\u1039\u101c",
}


def normalize_text(value: str, detector=None) -> str:
    text = unicodedata.normalize("NFC", value.replace("\u200b", "").replace("\ufeff", ""))
    if not _MYANMAR.search(text):
        return _SPACE.sub(" ", text).strip()
    if detector is None:
        from myanmartools import ZawgyiDetector

        detector = ZawgyiDetector()
    if detector.get_zawgyi_probability(text) > 0.05:
        text = zawgyi_to_unicode(text)
    return _SPACE.sub(" ", unicodedata.normalize("NFC", text)).strip()


def zawgyi_to_unicode(text: str) -> str:
    text = re.sub(r"[\u103b\u107e-\u1084]", "\u103c", text)
    text = re.sub(r"[\u103a\u107d]", "\u103b", text)
    for source, target in _STACKS.items():
        text = text.replace(source, target)
    replacements = (
        ("\u106a", "\u1009"), ("\u106b", "\u100a"), ("\u108f", "\u1014"),
        ("\u1090", "\u101b"), ("\u1086", "\u103f"),
        ("\u1064", "\u1004\u103a\u1039"),
        ("\u108b", "\u1004\u103a\u1039\u102d"),
        ("\u108c", "\u1004\u103a\u1039\u102e"),
        ("\u108d", "\u103d\u103e"),
        ("\u108e", "\u1004\u103a\u1039\u1036"),
        ("\u1033", "\u102f"), ("\u1034", "\u1030"),
        ("\u1088", "\u103e\u102f"), ("\u1089", "\u103e\u1030"),
        ("\u103d", "\u103e"), ("\u103c", "\u103d"),
        ("\u1094", "\u1037"), ("\u1095", "\u1037"),
    )
    for source, target in replacements:
        text = text.replace(source, target)
    text = re.sub(r"\u1039(?![\u1000-\u1021])", "\u103a", text)
    text = re.sub(r"\u1031([\u1000-\u1021])", lambda match: match.group(1) + "\u1031", text)
    text = re.sub(r"\u1031([\u103b-\u103e]+)([\u1000-\u1021])", lambda match: match.group(2) + match.group(1) + "\u1031", text)
    text = re.sub(r"([\u1000-\u1021])\u1031([\u103b-\u103e]+)", lambda match: match.group(1) + match.group(2) + "\u1031", text)
    text = re.sub(r"([\u1000-\u1021])\u1039\u1004\u103a", lambda match: "\u1004\u103a\u1039" + match.group(1), text)
    text = re.sub(r"\u1037+", "\u1037", text)
    text = re.sub(r"\u1032+", "\u1032", text)
    return text


def load_kaikki(path: Path):
    with path.open(encoding="utf-8") as source:
        for line in source:
            if line.strip():
                entry = json.loads(line)
                if entry.get("lang_code") == "my" or entry.get("lang") == "Burmese":
                    yield entry


def load_frequency_lexicon(path: Path, detector=None) -> dict[str, dict]:
    rows = {}
    with path.open(encoding="utf-8") as source:
        for line in source:
            if not line.strip() or line.startswith("#"):
                continue
            fields = line.rstrip("\n").split("\t")
            if len(fields) < 2 or not fields[1].isdigit():
                continue
            surface = normalize_text(fields[0], detector)
            if surface:
                rows[surface] = {
                    "frequency": int(fields[1]),
                    "reading": normalize_text(fields[2], detector) if len(fields) > 2 else "",
                }
    return rows


def level_for_rank(rank: int, total: int) -> str:
    percentile = rank / max(total, 1)
    if percentile <= 0.02:
        return "A1"
    if percentile <= 0.06:
        return "A2"
    if percentile <= 0.15:
        return "B1"
    if percentile <= 0.32:
        return "B2"
    if percentile <= 0.60:
        return "C1"
    return "C2"


def build_vocab(entries, frequency: dict[str, dict], detector=None) -> list[dict]:
    source = SOURCES["kaikki-my"]
    freq_source = SOURCES["myanmar-c4-frequency"]
    ordered = sorted(frequency, key=lambda word: (-frequency[word]["frequency"], word))
    ranks = {word: index for index, word in enumerate(ordered, 1)}
    by_word = {}
    for entry in entries:
        word = normalize_text(str(entry.get("word", "")), detector)
        if not word or not _MYANMAR.fullmatch(word):
            continue
        glosses = []
        for sense in entry.get("senses") or []:
            if sense.get("form_of"):
                continue
            for gloss in sense.get("glosses") or []:
                if isinstance(gloss, str) and gloss.strip() and gloss.strip() not in glosses:
                    glosses.append(normalize_text(gloss, detector))
        record = by_word.get(word)
        if record is None:
            stats = frequency.get(word, {"frequency": 0, "reading": ""})
            record = vocab_record(entry, word, glosses, stats, ranks.get(word, len(ranks) + 1), len(ranks), detector)
            by_word[word] = record
        else:
            record["gloss"] = list(dict.fromkeys([*record["gloss"], *glosses]))[:5]
            pos = str(entry.get("pos", "")).strip()
            if pos and pos not in record["pos"]:
                record["pos"].append(pos)
    glossed = [record for record in by_word.values() if record["gloss"]]
    return sorted(glossed, key=lambda item: (LEVELS.index(item["level"]), item["freqBand"], item["headword"]))


def word_id(word: str) -> str:
    return "my-" + hashlib.sha1(word.encode()).hexdigest()[:16]


def vocab_record(entry: dict, word: str, glosses: list[str], frequency: dict, rank: int, total: int, detector=None) -> dict:
    source = SOURCES["kaikki-my"]
    freq_source = SOURCES["myanmar-c4-frequency"]
    level = level_for_rank(rank, total)
    stable = word_id(word)
    reading = normalize_text(frequency.get("reading") or entry_reading(entry), detector)
    record = {
        "PK": "REF#my",
        "SK": f"VOCAB#{level}#{stable}",
        "lang": "my",
        "headword": word,
        "reading": reading,
        "gloss": glosses[:5],
        "pos": [normalize_text(str(entry["pos"]), detector)] if entry.get("pos") else [],
        "level": level,
        "levelApproximate": True,
        "freqBand": LEVELS.index(level) + 1,
        "sourceId": source.id,
        "license": source.license,
        "attribution": {
            "frequency": {"sourceId": freq_source.id, "license": freq_source.license, "count": frequency["frequency"], "rank": rank},
            "level": {"method": "C4 frequency-rank approximation", "approximate": True},
        },
    }
    example = entry_example(entry, detector)
    if example:
        record["example"] = {**example, "sourceId": source.id, "license": source.license}
    return record


def entry_reading(entry: dict) -> str:
    for form in entry.get("forms") or []:
        if "romanization" in (form.get("tags") or []) and form.get("form"):
            return str(form["form"])
    return ""


def entry_example(entry: dict, detector=None) -> dict | None:
    for sense in entry.get("senses") or []:
        for example in sense.get("examples") or []:
            text = normalize_text(str(example.get("text", "")), detector)
            translation = normalize_text(str(example.get("translation") or example.get("english") or ""), detector)
            if text and translation:
                return {"text": text, "translation": translation}
    return None


def load_topics() -> dict:
    return topics.load_topics("my")


def topic_records(vocab: list[dict], data=None) -> list[dict]:
    return topics.topic_records(vocab, data or load_topics(), "my")


def grammar_records(detector=None) -> list[dict]:
    grammar_topics = json.loads(resources.files("langler_etl.data").joinpath("grammar_my.json").read_text("utf-8"))
    return [{
        "PK": "REF#my", "SK": f"GRAMMAR#{topic['level']}#{topic['topicId']}", "lang": "my",
        "topicId": topic["topicId"], "name": topic["name"], "level": topic["level"],
        "description": topic["description"], "category": topic["category"], "introducedAt": topic["level"],
        "reviewed": True,
        "example": {
            "text": normalize_text(topic["example"]["text"], detector),
            "translation": topic["example"]["translation"],
            "sourceId": CURATED_BURMESE_GRAMMAR.id, "license": CURATED_BURMESE_GRAMMAR.license,
        },
        "sourceId": CURATED_BURMESE_GRAMMAR.id, "license": CURATED_BURMESE_GRAMMAR.license,
    } for topic in grammar_topics]


def script_records() -> list[dict]:
    items = json.loads(resources.files("langler_etl.data").joinpath("script_my.json").read_text("utf-8"))
    return [{
        "PK": "REF#my", "SK": f"SCRIPT#BURMESE#{index:03d}", "lang": "my",
        "glyph": item["glyph"], "scriptType": "burmese", "name": item["name"],
        "readings": {"romanization": item["romanization"]},
        "sourceId": CURATED_BURMESE_SCRIPT.id, "license": CURATED_BURMESE_SCRIPT.license,
    } for index, item in enumerate(items, 1)]


def passage_records(paths: list[tuple[Path, str, str]], known_words: dict[str, dict], segment, detector=None) -> list[dict]:
    rank = {word: index for index, word in enumerate(sorted(known_words, key=lambda word: -known_words[word]["frequency"]), 1)}
    total = len(rank)
    records = []
    seen = set()
    for path, source_id, license_name in paths:
        if VERIFIED_READING_SOURCES.get(source_id) != license_name:
            raise ValueError(f"reading source {source_id} has an unverified license: {license_name}")
        with path.open(encoding="utf-8") as source:
            for line in source:
                row = json.loads(line)
                if source_id == "chuuhtetnaing/myanmar-culturax-dataset" and str(row.get("source", "")).casefold() != "mc4":
                    continue
                text = normalize_text(str(row.get("text") or row.get("sentence") or ""), detector)
                if not 40 <= len(text) <= 1200 or text in seen:
                    continue
                tokens = [token for token in segment(text) if _MYANMAR.search(token)]
                covered = [rank[token] for token in tokens if token in rank]
                if not tokens or len(covered) / len(tokens) < 0.8:
                    continue
                difficult_rank = sorted(covered)[max(0, int(len(covered) * 0.9) - 1)]
                level = level_for_rank(difficult_rank, total)
                stable = hashlib.sha1(f"{source_id}|{text}".encode()).hexdigest()[:16]
                records.append({
                    "PK": "REF#my", "SK": f"READING#{level}#{stable}", "lang": "my", "level": level,
                    "levelApproximate": True, "text": text, "coverage": round(len(covered) / len(tokens), 3),
                    "sourceId": source_id, "license": license_name,
                })
                seen.add(text)
    return records


def prune_ngram_asset(payload: dict, minimum_unigram=20, minimum_bigram=10, maximum_unigrams=12000) -> dict:
    selected = dict(sorted(
        ((word, count) for word, count in payload["unigram"].items() if count >= minimum_unigram and _MYANMAR.search(word)),
        key=lambda item: (-item[1], item[0]),
    )[:maximum_unigrams])
    bigram = {}
    for previous, following in payload["bigram"].items():
        if previous not in selected:
            continue
        kept = {word: count for word, count in following.items() if word in selected and count >= minimum_bigram}
        if kept:
            bigram[previous] = kept
    return {
        "format": payload["format"], "source": payload.get("source", {}),
        "unigram_count": len(selected), "unigram_total": sum(selected.values()),
        "bigram_count": sum(len(items) for items in bigram.values()),
        "bigram_total": sum(sum(items.values()) for items in bigram.values()),
        "pruned": True, "license": SOURCES["myword-ngram"].license,
        "unigram": selected, "bigram": bigram,
    }
