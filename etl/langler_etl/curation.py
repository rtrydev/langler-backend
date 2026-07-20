import json
import math
import re
from collections import Counter, defaultdict
from pathlib import Path

from . import topics

_TOKEN = re.compile(r"[a-z]{3,}")
_STOPWORDS = frozenset(
    """
    the and for that with this from which their there where when what who whom whose
    are was were been being have has had not but you your they them his her its our
    one two any all some such each other than then also may can will would could should
    something someone anything used using use way ways kind sort type form manner
    especially typically usually often sometimes generally particularly literally
    figuratively obsolete archaic dialectal informal formal slang chiefly
    plural singular masculine feminine neuter verb noun adjective adverb pronoun
    prefix suffix particle classifier abbreviation initialism alternative spelling
    """.split()
)
# Seed counts from a topic's own keywords so a topic matches its theme before it has examples.
KEYWORD_WEIGHT = 8
SMOOTHING = 0.4
# A runner-up topic must score at least this share of the winner's posterior to be added.
SECONDARY_RATIO = 0.45
# Without calibration the classifier drifts toward broad topics, recreating a soft catch-all;
# these fit a per-topic bias so the extended tail matches the curated topic distribution.
CALIBRATION_ROUNDS = 25
CALIBRATION_RATE = 0.25


def gloss_tokens(glosses: list[str]) -> list[str]:
    tokens = []
    for gloss in glosses:
        for token in _TOKEN.findall(gloss.casefold()):
            if token not in _STOPWORDS:
                tokens.append(token)
    return tokens


def train(vocab: list[dict], seed: dict[str, list[str]], data: dict) -> dict:
    counts: dict[str, Counter] = defaultdict(Counter)
    documents: Counter = Counter()
    vocabulary: set[str] = set()
    seeded = 0

    for topic in data["topics"]:
        for keyword in topic["keywords"]:
            for token in gloss_tokens([keyword]):
                counts[topic["slug"]][token] += KEYWORD_WEIGHT
                vocabulary.add(token)

    for record in vocab:
        word_id = record["SK"].rsplit("#", 1)[-1]
        slugs = seed.get(word_id)
        if not slugs:
            continue
        seeded += 1
        tokens = gloss_tokens(record.get("gloss", []))
        vocabulary.update(tokens)
        for slug in slugs:
            documents[slug] += 1
            counts[slug].update(tokens)

    if not seeded:
        raise ValueError("cannot train a topic classifier without seed assignments")

    slugs = [topic["slug"] for topic in data["topics"]]
    assigned = sum(documents.values()) or 1
    return {
        "counts": counts,
        "totals": {slug: sum(tokens.values()) for slug, tokens in counts.items()},
        "bias": {slug: 0.0 for slug in slugs},
        "target": {slug: documents[slug] / assigned for slug in slugs},
        "size": len(vocabulary),
        "slugs": slugs,
        "seeded": seeded,
    }


def classify(glosses: list[str], model: dict, limit: int = topics.MAX_SLUGS_PER_WORD) -> list[str]:
    tokens = gloss_tokens(glosses)
    scores = {}
    for slug in model["slugs"]:
        counts = model["counts"].get(slug, Counter())
        total = model["totals"].get(slug, 0)
        score = model["bias"][slug]
        for token in tokens:
            score += math.log(
                (counts.get(token, 0) + SMOOTHING) / (total + SMOOTHING * model["size"])
            )
        scores[slug] = score

    best = max(scores.values())
    posteriors = {slug: math.exp(score - best) for slug, score in scores.items()}
    ranked = sorted(posteriors.items(), key=lambda item: (-item[1], item[0]))
    chosen = [ranked[0][0]]
    for slug, posterior in ranked[1:limit]:
        if posterior >= SECONDARY_RATIO:
            chosen.append(slug)
    return chosen


def calibrate(model: dict, glosses: list[list[str]], rounds: int = CALIBRATION_ROUNDS) -> dict:
    if not glosses:
        return model
    for _ in range(rounds):
        predicted = Counter(slug for gloss in glosses for slug in classify(gloss, model))
        total = sum(predicted.values()) or 1
        for slug in model["slugs"]:
            target = model["target"][slug]
            if target <= 0:
                continue
            share = predicted[slug] / total
            model["bias"][slug] += CALIBRATION_RATE * math.log(target / max(share, 1e-6))
    return model


def extend(vocab: list[dict], seed: dict[str, list[str]], data: dict) -> dict[str, list[str]]:
    model = train(vocab, seed, data)
    tail = [
        record.get("gloss", [])
        for record in vocab
        if record["SK"].rsplit("#", 1)[-1] not in seed
    ]
    calibrate(model, tail)
    assignments = {}
    for record in vocab:
        word_id = record["SK"].rsplit("#", 1)[-1]
        slugs = seed.get(word_id)
        assignments[word_id] = slugs if slugs else classify(record.get("gloss", []), model)
    return assignments


def holdout_accuracy(vocab: list[dict], seed: dict[str, list[str]], data: dict, fold: int = 5) -> float:
    by_id = {record["SK"].rsplit("#", 1)[-1]: record for record in vocab}
    seeded = sorted(word_id for word_id in seed if word_id in by_id)
    held = {word_id for index, word_id in enumerate(seeded) if index % fold == 0}
    model = train(vocab, {k: v for k, v in seed.items() if k not in held}, data)
    hits = 0
    for word_id in held:
        predicted = classify(by_id[word_id].get("gloss", []), model)
        if set(predicted) & set(seed[word_id]):
            hits += 1
    return hits / max(len(held), 1)


def load_seed(seed_dir: Path, language: str) -> dict[str, list[str]]:
    seed: dict[str, list[str]] = {}
    for path in sorted(seed_dir.glob(f"{language}-*.json")):
        seed.update(json.loads(path.read_text("utf-8")))
    return seed


def curate(language: str, vocab: list[dict], seed: dict[str, list[str]], data_dir: Path) -> dict:
    definitions = topics.load_topics(language)
    known = {topic["slug"] for topic in definitions["topics"]}
    unknown = {slug for slugs in seed.values() for slug in slugs} - known
    if unknown:
        raise ValueError(f"seed assignments use unknown slugs {sorted(unknown)}")

    assignments = extend(vocab, seed, definitions)
    payload = {
        "topics": definitions["topics"],
        "assignments": {word_id: assignments[word_id] for word_id in sorted(assignments)},
        "curation": {
            "method": "curated per-word classification, extended to the uncurated tail by a naive-Bayes gloss classifier trained on the curated set",
            "curated": sum(1 for word_id in assignments if word_id in seed),
            "extended": sum(1 for word_id in assignments if word_id not in seed),
            "seeded": sorted(word_id for word_id in assignments if word_id in seed),
        },
    }
    path = data_dir / f"topics_{language}.json"
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=1) + "\n", encoding="utf-8")
    return payload["curation"]
