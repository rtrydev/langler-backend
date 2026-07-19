import json
from collections import Counter
from pathlib import Path

from . import burmese
from .build import _write_jsonl
from .sources import REGISTRY


def build(data_dir: Path, out_dir: Path, detector=None) -> dict:
    if detector is None:
        from myanmartools import ZawgyiDetector

        detector = ZawgyiDetector()
    ref_dir = out_dir / "reference" / "my"
    ref_dir.mkdir(parents=True, exist_ok=True)
    frequency_path = _first_existing(
        data_dir / "BurmeseLexiconSource.tsv",
        Path(__file__).resolve().parents[3] / "myanmar-ime" / "Packages" / "BurmeseIMECore" / "Data" / "BurmeseLexiconSource.tsv",
    )
    if frequency_path is None:
        raise FileNotFoundError("BurmeseLexiconSource.tsv is required; copy the myanmar-ime corpus_builder artifact into the ETL data directory")
    kaikki_path = _first_existing(
        data_dir / "kaikki-my.jsonl",
        data_dir / "dictionary-burmese.jsonl",
        Path(__file__).resolve().parents[3] / "myangler-web" / "data" / "dictionary-burmese.jsonl",
    )
    if kaikki_path is None:
        raise FileNotFoundError("kaikki-my.jsonl is required")
    myg2p_path = _first_existing(
        data_dir / "myg2p.ver2.0.txt",
        Path(__file__).resolve().parents[3] / "myG2P" / "ver2" / "myg2p.ver2.0.txt",
    )
    if myg2p_path is None:
        raise FileNotFoundError("myg2p.ver2.0.txt is required; run download --language my")

    frequency = burmese.load_frequency_lexicon(frequency_path, detector)
    myg2p_headwords = burmese.load_myg2p(myg2p_path, detector)
    topics = burmese.load_topics()
    vocab = burmese.build_vocab(burmese.load_kaikki(kaikki_path), frequency, myg2p_headwords, topics, detector)
    grammar = burmese.grammar_records(detector)
    scripts = burmese.script_records()
    topic_items = burmese.topic_records(vocab, topics)
    readings = _build_readings(data_dir, frequency, detector)

    _write_jsonl(ref_dir / "vocab.jsonl", vocab)
    _write_jsonl(ref_dir / "grammar.jsonl", grammar)
    _write_jsonl(ref_dir / "scripts.jsonl", scripts)
    _write_jsonl(ref_dir / "topics.jsonl", topic_items)
    _write_jsonl(ref_dir / "readings.jsonl", readings)
    _build_client_ngram(data_dir, out_dir)

    source_ids = {
        "kaikki-my", "myg2p-headwords", "myanmar-c4-frequency", "myword-ngram",
        "langler-curated-my-grammar", "langler-curated-my-script", "langler-curated-my-topics",
    }
    manifest = {
        "language": "my",
        "counts": {
            "vocab": _counts(vocab), "grammar": _counts(grammar), "scripts": {"total": len(scripts)},
            "topics": _counts(topic_items), "readings": _counts(readings),
        },
        "sources": [{"id": source.id, "url": source.url, "license": source.license, "attribution": source.attribution}
                    for source in REGISTRY if source.id in source_ids],
    }
    (out_dir / "manifest-my.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return manifest


def _first_existing(*paths: Path) -> Path | None:
    return next((path for path in paths if path.is_file()), None)


def _counts(items: list[dict]) -> dict:
    counts = Counter(item["level"] for item in items)
    return {**{level: counts[level] for level in burmese.LEVELS}, "total": len(items)}


def _build_readings(data_dir: Path, frequency: dict[str, dict], detector) -> list[dict]:
    manifest_path = data_dir / "readings-my" / "manifest.json"
    if not manifest_path.is_file():
        return []
    manifest = json.loads(manifest_path.read_text("utf-8"))
    paths = [(manifest_path.parent / item["file"], item["sourceId"], item["license"]) for item in manifest["sources"]]
    words = sorted(frequency, key=len, reverse=True)

    def segment(text: str) -> list[str]:
        result = []
        position = 0
        while position < len(text):
            found = next((word for word in words if text.startswith(word, position)), None)
            if found:
                result.append(found)
                position += len(found)
            else:
                result.append(text[position])
                position += 1
        return result

    return burmese.passage_records(paths, frequency, segment, detector)


def _build_client_ngram(data_dir: Path, out_dir: Path) -> None:
    source = _first_existing(
        data_dir / "ngram.json",
        Path(__file__).resolve().parents[3] / "myangler-web" / "tools" / "data-pipeline" / "build" / "ngram.json",
    )
    if source is None:
        return
    payload = burmese.prune_ngram_asset(json.loads(source.read_text("utf-8")))
    target = out_dir / "assets" / "burmese" / "myword-ngram.json"
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(payload, ensure_ascii=False, separators=(",", ":")) + "\n", encoding="utf-8")
