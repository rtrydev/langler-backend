import json
from collections import Counter
from pathlib import Path

from . import polish
from .build import _write_jsonl
from .sources import REGISTRY


def build(data_dir: Path, out_dir: Path, band=polish.wordfreq_band, topic_data=None) -> dict:
    ref_dir = out_dir / "reference" / "pl"
    ref_dir.mkdir(parents=True, exist_ok=True)

    examples = polish.PolishExampleIndex()
    sentences = data_dir / "tatoeba-sentences.tsv"
    links = data_dir / "tatoeba-links.tsv"
    if sentences.exists() and links.exists():
        english = data_dir / "tatoeba-english.tsv"
        examples = polish.load_tatoeba(sentences, links, english if english.exists() else None)

    ranks = polish.load_frequency(data_dir / "nkjp-frequency.tsv")
    topics = topic_data or polish.load_topics()
    vocab = polish.build_vocab(
        polish.load_kaikki(data_dir / "kaikki-pl.jsonl"),
        ranks,
        examples,
        band=band,
        topic_data=topics,
    )
    nkjp_dir = data_dir / "nkjp-1m"
    nkjp_archive = data_dir / "NKJP-PodkorpusMilionowy-1.2.tar.gz"
    nkjp_path = nkjp_dir if nkjp_dir.exists() else nkjp_archive
    evidence = polish.load_nkjp_sentences(nkjp_path) if nkjp_path.exists() else []
    grammar = polish.grammar_records(evidence)
    scripts = polish.orthography_records()
    topic_items = polish.topic_records(vocab, topics)

    _write_jsonl(ref_dir / "vocab.jsonl", vocab)
    _write_jsonl(ref_dir / "grammar.jsonl", grammar)
    _write_jsonl(ref_dir / "scripts.jsonl", scripts)
    _write_jsonl(ref_dir / "topics.jsonl", topic_items)

    manifest = {
        "language": "pl",
        "counts": {
            "vocab": _counts(vocab),
            "grammar": _counts(grammar),
            "orthography": {"total": len(scripts)},
            "topics": _counts(topic_items),
        },
        "sources": [
            {
                "id": source.id,
                "url": source.url,
                "license": source.license,
                "attribution": source.attribution,
            }
            for source in REGISTRY
            if source.id in {
                "kaikki-pl",
                "nkjp-1m",
                "nkjp-frequency",
                "tatoeba-pl",
                "wordfreq",
                "certyfikat-polish",
                "morfeusz-sgjp",
                "langler-curated-pl-grammar",
                "langler-curated-pl-orthography",
                "langler-curated-pl-topics",
            }
        ],
    }
    (out_dir / "manifest-pl.json").write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    return manifest


def _counts(items: list[dict]) -> dict:
    counts = Counter(item["level"] for item in items)
    return {**{level: counts[level] for level in polish.LEVELS}, "total": len(items)}
