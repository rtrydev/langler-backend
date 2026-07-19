import csv
import hashlib
import json
import re
import tarfile
import xml.etree.ElementTree as ET
from collections import defaultdict
from importlib import resources
from pathlib import Path

from .sources import (
    CURATED_POLISH_GRAMMAR,
    CURATED_POLISH_ORTHOGRAPHY,
    CURATED_POLISH_TOPICS,
    SOURCES,
)

LEVELS = ["A1", "A2", "B1", "B2", "C1", "C2"]
MAX_NKJP_RANK = 120_000
_WORD = re.compile(r"[A-Za-zĄĆĘŁŃÓŚŹŻąćęłńóśźż-]+")
_POS = {
    "noun": "n",
    "verb": "v",
    "adj": "adj",
    "adv": "adv",
    "pron": "pron",
    "prep": "prep",
    "conj": "conj",
    "det": "det",
    "num": "num",
    "intj": "intj",
}


class PolishExampleIndex:
    def __init__(self):
        self._examples: dict[str, tuple[str, str, str, str]] = {}

    def add(self, text: str, translation: str, source_id: str, license_name: str) -> None:
        if not text or not translation:
            return
        for word in set(_WORD.findall(text.casefold())):
            current = self._examples.get(word)
            if current is None or len(text) < len(current[0]):
                self._examples[word] = (text, translation, source_id, license_name)

    def lookup(self, word: str) -> dict | None:
        found = self._examples.get(word.casefold())
        if found is None:
            return None
        text, translation, source_id, license_name = found
        return {
            "text": text,
            "translation": translation,
            "sourceId": source_id,
            "license": license_name,
        }


def load_kaikki(path: Path):
    with path.open(encoding="utf-8") as source:
        for line in source:
            if not line.strip():
                continue
            entry = json.loads(line)
            if entry.get("lang_code") == "pl" or entry.get("lang") == "Polish":
                yield entry


def load_frequency(path: Path) -> dict[str, int]:
    ranks: dict[str, int] = {}
    with path.open(encoding="utf-8-sig", errors="replace") as source:
        for line_number, line in enumerate(source, 1):
            fields = [field.strip() for field in re.split(r"[\t;,]", line.rstrip())]
            if len(fields) < 2:
                fields = line.split()
            if len(fields) < 2:
                continue
            numeric = next((int(field) for field in fields if field.isdigit()), line_number)
            word = next((field for field in fields if _WORD.fullmatch(field) and not field.isdigit()), "")
            if word:
                ranks.setdefault(word.casefold(), numeric)
    return ranks


def load_tatoeba(sentences_path: Path, links_path: Path, english_path: Path | None = None) -> PolishExampleIndex:
    sentences: dict[int, tuple[str, str]] = {}
    for path in [sentences_path, english_path]:
        if path is None:
            continue
        with path.open(encoding="utf-8") as source:
            for row in csv.reader(source, delimiter="\t"):
                if len(row) >= 3 and row[0].isdigit() and row[1] in {"pol", "eng"}:
                    sentences[int(row[0])] = (row[1], row[2])
    index = PolishExampleIndex()
    source = SOURCES["tatoeba-pl"]
    with links_path.open(encoding="utf-8") as links:
        for row in csv.reader(links, delimiter="\t"):
            if len(row) < 2 or not row[0].isdigit() or not row[1].isdigit():
                continue
            left = sentences.get(int(row[0]))
            right = sentences.get(int(row[1]))
            if left and right and left[0] == "pol" and right[0] == "eng":
                index.add(left[1], right[1], source.id, source.license)
    return index


def load_nkjp_examples(path: Path, translations: dict[str, str] | None = None) -> PolishExampleIndex:
    index = PolishExampleIndex()
    source = SOURCES["nkjp-1m"]
    translations = translations or {}
    for text in load_nkjp_sentences(path):
        translation = translations.get(text)
        if translation:
            index.add(text, translation, source.id, source.license)
    return index


def load_nkjp_sentences(path: Path) -> list[str]:
    if path.is_dir():
        roots = (
            ET.parse(xml_path).getroot()
            for xml_path in sorted(path.rglob("ann_morphosyntax.xml"))
        )
        return _sentences_from_roots(roots)
    if path.name.endswith((".tar.gz", ".tgz")):
        with tarfile.open(path, mode="r:gz") as archive:
            roots = (
                ET.parse(extracted).getroot()
                for member in archive.getmembers()
                if member.isfile()
                and member.name.endswith("ann_morphosyntax.xml")
                and (extracted := archive.extractfile(member)) is not None
            )
            return _sentences_from_roots(roots)
    return _sentences_from_roots([ET.parse(path).getroot()])


def _sentences_from_roots(roots) -> list[str]:
    sentences = []
    for root in roots:
        for sentence in root.iter():
            if sentence.tag.rsplit("}", 1)[-1] not in {"s", "sentence"}:
                continue
            tokens = []
            for field in sentence.iter():
                if field.tag.rsplit("}", 1)[-1] != "f" or field.get("name") != "orth":
                    continue
                value = next(
                    (
                        node.text.strip()
                        for node in field.iter()
                        if node.tag.rsplit("}", 1)[-1] == "string"
                        and node.text
                        and node.text.strip()
                    ),
                    "",
                )
                if value:
                    tokens.append(value)
            text = _join_polish_tokens(tokens)
            if 20 <= len(text) <= 240:
                sentences.append(text)
    return sentences


def _join_polish_tokens(tokens: list[str]) -> str:
    text = " ".join(tokens)
    text = re.sub(r"\s+([,.;:!?%\)\]])", r"\1", text)
    return re.sub(r"([\(\[])\s+", r"\1", text)


def wordfreq_band(word: str) -> int | None:
    import wordfreq

    zipf = wordfreq.zipf_frequency(word, "pl")
    if zipf == 0:
        return None
    thresholds = (6.0, 5.5, 5.0, 4.5, 4.0, 3.5, 3.0)
    return next((band for band, threshold in enumerate(thresholds, 1) if zipf >= threshold), 8)


def nkjp_band(rank: int | None) -> int | None:
    if rank is None:
        return None
    limits = (1500, 3500, 7000, 12000, 20000, 35000, 60000)
    return next((band for band, limit in enumerate(limits, 1) if rank <= limit), 8)


def combined_band(word: str, rank: int | None, band=wordfreq_band) -> int | None:
    values = [value for value in (nkjp_band(rank), band(word)) if value is not None]
    return min(values) if values else None


def cefr_for_band(band: int) -> str:
    return {1: "A1", 2: "A2", 3: "B1", 4: "B1", 5: "B2", 6: "C1", 7: "C1", 8: "C2"}[band]


def build_vocab(
    entries,
    ranks: dict[str, int],
    examples: PolishExampleIndex,
    band=wordfreq_band,
    topic_data: dict | None = None,
) -> list[dict]:
    topic_data = topic_data or load_topics()
    by_word: dict[str, dict] = {}
    for entry in entries:
        if entry.get("lang_code") not in {None, "pl"} or entry.get("lang") not in {None, "Polish"}:
            continue
        word = str(entry.get("word", "")).strip()
        if not word or " " in word or word.startswith("-") or word.endswith("-") or not _WORD.fullmatch(word):
            continue
        senses = [sense for sense in entry.get("senses", []) if usable_sense(sense)]
        if not senses:
            continue
        glosses = []
        for sense in senses:
            for gloss in sense.get("glosses", []):
                if isinstance(gloss, str) and gloss not in glosses:
                    glosses.append(gloss)
        if not glosses:
            continue
        key = word.casefold()
        rank = ranks.get(key)
        if rank is None or rank > MAX_NKJP_RANK:
            continue
        frequency = combined_band(word, rank, band)
        if frequency is None:
            continue
        record = by_word.get(key)
        if record is None:
            record = vocab_record(entry, word, glosses, frequency, examples.lookup(word), topic_data)
            by_word[key] = record
        else:
            for gloss in glosses:
                if gloss not in record["gloss"] and len(record["gloss"]) < 5:
                    record["gloss"].append(gloss)
            pos = _POS.get(str(entry.get("pos", "")), str(entry.get("pos", "")))
            if pos and pos not in record["pos"]:
                record["pos"].append(pos)
    return sorted(by_word.values(), key=lambda item: (LEVELS.index(item["level"]), item["freqBand"], item["headword"].casefold()))


def usable_sense(sense: dict) -> bool:
    if not sense.get("glosses") or sense.get("form_of"):
        return False
    tags = {
        str(tag).casefold()
        for tag in (sense.get("tags") or []) + (sense.get("raw_tags") or [])
    }
    return not tags.intersection(
        {"form-of", "archaic", "historical", "middle polish", "obsolete", "old polish"}
    )


def vocab_record(entry: dict, word: str, glosses: list[str], frequency: int, example: dict | None, topic_data: dict) -> dict:
    source = SOURCES["kaikki-pl"]
    nkjp = SOURCES["nkjp-frequency"]
    freq = SOURCES["wordfreq"]
    stable = hashlib.sha1(f"{word.casefold()}|{entry.get('pos', '')}".encode()).hexdigest()[:16]
    level = cefr_for_band(frequency)
    pos = _POS.get(str(entry.get("pos", "")), str(entry.get("pos", "")))
    record = {
        "PK": "REF#pl",
        "SK": f"VOCAB#{level}#pl-{stable}",
        "lang": "pl",
        "headword": word,
        "reading": word,
        "gloss": glosses[:5],
        "pos": [pos] if pos else [],
        "level": level,
        "levelApproximate": True,
        "freqBand": frequency,
        "topics": classify_topics(glosses, topic_data),
        "sourceId": source.id,
        "license": source.license,
        "attribution": {
            "frequency": [
                {"sourceId": nkjp.id, "license": nkjp.license},
                {"sourceId": freq.id, "license": freq.license},
            ],
            "level": {"method": "frequency-band approximation", "approximate": True},
        },
    }
    if example:
        record["example"] = example
    return record


def load_topics() -> dict:
    return json.loads(resources.files("langler_etl.data").joinpath("topics_pl.json").read_text("utf-8"))


def classify_topics(glosses: list[str], data: dict) -> list[str]:
    text = " ".join(glosses).casefold()
    matches = []
    for topic in data["topics"]:
        if any(keyword.casefold() in text for keyword in topic["keywords"]):
            matches.append(topic["slug"])
    return matches[:3] or ["everyday-life"]


def topic_records(vocab: list[dict], data: dict | None = None) -> list[dict]:
    data = data or load_topics()
    meta = {topic["slug"]: topic for topic in data["topics"]}
    members: dict[tuple[str, str], list[str]] = defaultdict(list)
    for record in vocab:
        for slug in record["topics"]:
            members[(record["level"], slug)].append(record["SK"].removeprefix("VOCAB#"))
    source = CURATED_POLISH_TOPICS
    return [
        {
            "PK": "REF#pl",
            "SK": f"TOPIC#{level}#{slug}",
            "lang": "pl",
            "slug": slug,
            "name": meta[slug]["name"],
            "description": meta[slug]["description"],
            "level": level,
            "keywords": meta[slug]["keywords"],
            "vocabIds": ids,
            "sourceId": source.id,
            "license": source.license,
        }
        for (level, slug), ids in sorted(members.items())
    ]


def grammar_records(evidence_sentences: list[str] | None = None, analyzer=None) -> list[dict]:
    topics = json.loads(resources.files("langler_etl.data").joinpath("grammar_pl.json").read_text("utf-8"))
    inventory = SOURCES["certyfikat-polish"]
    evidence_source = SOURCES["nkjp-1m"]
    morphology_source = SOURCES["morfeusz-sgjp"]
    if analyzer is None:
        import morfeusz2

        analyzer = morfeusz2.Morfeusz()
    records = []
    for topic in topics:
        validation = morphology_validation(topic["example"]["text"], analyzer)
        record = {
            "PK": "REF#pl",
            "SK": f"GRAMMAR#{topic['level']}#{topic['topicId']}",
            "lang": "pl",
            "topicId": topic["topicId"],
            "name": topic["name"],
            "level": topic["level"],
            "description": topic["description"],
            "example": {
                **topic["example"],
                "sourceId": CURATED_POLISH_GRAMMAR.id,
                "license": CURATED_POLISH_GRAMMAR.license,
            },
            "sourceId": inventory.id,
            "license": inventory.license,
            "category": topic["category"],
            "introducedAt": topic["level"],
            "descriptorRef": topic["descriptorRef"],
            "attribution": {
                "wording": {"sourceId": CURATED_POLISH_GRAMMAR.id, "license": CURATED_POLISH_GRAMMAR.license},
                "evidence": {"sourceId": evidence_source.id, "license": evidence_source.license},
                "morphologyValidation": {
                    "sourceId": morphology_source.id,
                    "license": morphology_source.license,
                    **validation,
                },
            },
        }
        evidence_text = best_evidence(topic["example"]["text"], evidence_sentences or [])
        if evidence_text:
            record["evidence"] = {
                "text": evidence_text,
                "sourceId": evidence_source.id,
                "license": evidence_source.license,
            }
        records.append(record)
    return records


def morphology_validation(text: str, analyzer) -> dict:
    tokens = _WORD.findall(text)
    unknown = []
    for token in tokens:
        interpretations = analyzer.analyse(token)
        if not any(result[2][2] != "ign" for result in interpretations):
            unknown.append(token)
    coverage = 1 if not tokens else (len(tokens) - len(unknown)) / len(tokens)
    if coverage < 0.8:
        raise ValueError(f"grammar example has insufficient Morfeusz coverage: {text}")
    return {
        "coveragePercent": round(coverage * 100),
        "unknownTokens": unknown,
    }


def best_evidence(example: str, sentences: list[str]) -> str | None:
    stop = {"jest", "są", "i", "w", "na", "do", "z", "że", "to", "się", "nie", "a", "ale"}
    terms = {word for word in _WORD.findall(example.casefold()) if len(word) > 3 and word not in stop}
    if not terms:
        return None
    scored = []
    for sentence in sentences:
        words = set(_WORD.findall(sentence.casefold()))
        score = len(terms & words)
        if score:
            scored.append((score, -len(sentence), sentence))
    return max(scored)[2] if scored else None


def orthography_records() -> list[dict]:
    notes = json.loads(resources.files("langler_etl.data").joinpath("orthography_pl.json").read_text("utf-8"))
    source = CURATED_POLISH_ORTHOGRAPHY
    return [
        {
            "PK": "REF#pl",
            "SK": f"SCRIPT#ORTHOGRAPHY#{index:03d}",
            "lang": "pl",
            "glyph": note["pattern"],
            "scriptType": "orthography",
            "name": note["name"],
            "meanings": [note["description"], *note.get("examples", [])],
            "readings": {"contrasts": note.get("contrasts", [])},
            "sourceId": source.id,
            "license": source.license,
        }
        for index, note in enumerate(notes, 1)
    ]
