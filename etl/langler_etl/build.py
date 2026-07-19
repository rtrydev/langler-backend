import json
import shutil
from collections import Counter
from pathlib import Path

from . import examples, grammar, jlpt, jmdict, kana, kanjidic, kradfile, topics
from .kanjidic import Kanji
from .sources import REGISTRY, SOURCES


def build(data_dir: Path, out_dir: Path, band=jmdict.freq_band, topic_data=None) -> dict:
    ref_dir = out_dir / "reference" / "ja"
    assets_dir = out_dir / "assets" / "kanjivg"
    ref_dir.mkdir(parents=True, exist_ok=True)
    assets_dir.mkdir(parents=True, exist_ok=True)

    levels = jlpt.load_levels(data_dir / "jlpt")
    index = examples.ExampleIndex.from_file(data_dir / "examples.utf")
    vocab = jmdict.build_vocab(jmdict.load_words(data_dir / "jmdict-eng.json"), levels, index, band=band)

    if topic_data is None:
        topic_data = topics.load_topics()
    topics.apply_topics(vocab, topic_data)
    topic_items = topics.topic_records(vocab, topic_data)

    svg_dir = data_dir / "kanjivg"
    components = kradfile.parse_kradfile(data_dir / "kradfile.txt")
    kanji = [
        kanji_record(k, svg_dir, components.get(k.glyph))
        for k in kanjidic.parse_kanjidic(data_dir / "kanjidic2.xml")
    ]

    kana_items = kana.kana_records()
    grammar_items = grammar.grammar_records()

    _write_jsonl(ref_dir / "vocab.jsonl", vocab)
    _write_jsonl(ref_dir / "grammar.jsonl", grammar_items)
    _write_jsonl(ref_dir / "scripts.jsonl", kana_items + kanji)
    _write_jsonl(ref_dir / "topics.jsonl", topic_items)

    for item in kanji:
        if "strokeDataRef" in item:
            name = item["strokeDataRef"].removeprefix("kanjivg/")
            shutil.copyfile(svg_dir / name, assets_dir / name)

    manifest = _manifest(vocab, grammar_items, kana_items, kanji, topic_items, data_dir)
    (out_dir / "manifest.json").write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    return manifest


def kanji_record(k: Kanji, svg_dir: Path, components: list[str] | None) -> dict:
    source = SOURCES["kanjidic2"]
    item = {
        "PK": "REF#ja",
        "SK": f"SCRIPT#KANJI#{k.level}#{k.glyph}",
        "lang": "ja",
        "glyph": k.glyph,
        "scriptType": "kanji",
        "name": k.meanings[0] if k.meanings else k.glyph,
        "meanings": k.meanings,
        "readings": {"on": k.on, "kun": k.kun},
        "level": k.level,
        "strokeCount": k.stroke_count,
        "sourceId": source.id,
        "license": source.license,
    }
    if k.grade is not None:
        item["grade"] = k.grade
    attribution = {}
    svg_name = f"{ord(k.glyph):05x}.svg"
    if (svg_dir / svg_name).exists():
        item["strokeDataRef"] = f"kanjivg/{svg_name}"
        attribution["strokes"] = {"sourceId": SOURCES["kanjivg"].id, "license": SOURCES["kanjivg"].license}
    if components:
        item["components"] = components
        attribution["components"] = {"sourceId": SOURCES["kradfile"].id, "license": SOURCES["kradfile"].license}
    if attribution:
        item["attribution"] = attribution
    return item


def _write_jsonl(path: Path, items: list[dict]) -> None:
    with path.open("w", encoding="utf-8") as f:
        for item in items:
            f.write(json.dumps(item, ensure_ascii=False) + "\n")


def _manifest(vocab, grammar_items, kana_items, kanji, topic_items, data_dir: Path) -> dict:
    resolved_path = data_dir / "resolved.json"
    resolved = json.loads(resolved_path.read_text()) if resolved_path.exists() else {}
    sources = []
    for source in REGISTRY:
        if source.id.endswith("-pl") or source.id.startswith("nkjp-") or source.id in {
            "kaikki-pl",
            "certyfikat-polish",
            "langler-curated-pl-grammar",
            "langler-curated-pl-orthography",
            "langler-curated-pl-topics",
            "morfeusz-sgjp",
        }:
            continue
        entry = {
            "id": source.id,
            "url": source.url,
            "license": source.license,
            "attribution": source.attribution,
        }
        if source.id in resolved:
            entry["resolvedUrl"] = resolved[source.id]
        sources.append(entry)
    return {
        "counts": {
            "vocab": _level_counts(vocab),
            "grammar": _level_counts(grammar_items),
            "kanji": _level_counts(kanji),
            "kana": _kana_counts(kana_items),
            "topics": _level_counts(topic_items),
        },
        "sources": sources,
    }


def _level_counts(items: list[dict]) -> dict:
    counts = Counter(item["level"] for item in items)
    ordered = {level: counts[level] for level in jlpt.LEVELS if level in counts}
    ordered["total"] = len(items)
    return ordered


def _kana_counts(items: list[dict]) -> dict:
    counts = Counter(item["kanaScript"] for item in items)
    return {**counts, "total": len(items)}
