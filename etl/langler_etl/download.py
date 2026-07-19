import gzip
import io
import json
import re
import tarfile
import urllib.request
import zipfile
import bz2
from pathlib import Path

from .sources import SOURCES, Source

USER_AGENT = "langler-etl (language-learning reference data pipeline)"
JLPT_LEVELS = ["n5", "n4", "n3", "n2", "n1"]
JLPT_CSV_URL = "https://raw.githubusercontent.com/jamsinclair/open-anki-jlpt-decks/main/src/{level}.csv"
MYG2P_URL = "https://raw.githubusercontent.com/ye-kyaw-thu/myG2P/master/ver2/myg2p.ver2.0.txt"


def fetch(url: str) -> bytes:
    request = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    with urllib.request.urlopen(request) as response:
        return response.read()


def resolve_github_asset(source: Source) -> str:
    release = json.loads(fetch(source.url))
    pattern = re.compile(source.asset_pattern)
    for asset in release["assets"]:
        if pattern.match(asset["name"]):
            return asset["browser_download_url"]
    raise LookupError(f"no release asset matching {source.asset_pattern} at {source.url}")


def download_all(data_dir: Path) -> dict:
    data_dir.mkdir(parents=True, exist_ok=True)
    resolved_path = data_dir / "resolved.json"
    resolved = json.loads(resolved_path.read_text()) if resolved_path.exists() else {}

    _download_jmdict(data_dir, resolved)
    _download_kanjidic(data_dir, resolved)
    _download_kanjivg(data_dir, resolved)
    _download_kradfile(data_dir, resolved)
    _download_examples(data_dir, resolved)
    _download_jlpt(data_dir, resolved)

    resolved_path.write_text(json.dumps(resolved, indent=2) + "\n")
    return resolved


def download_polish(data_dir: Path) -> dict:
    data_dir.mkdir(parents=True, exist_ok=True)
    resolved_path = data_dir / "resolved-pl.json"
    resolved = json.loads(resolved_path.read_text()) if resolved_path.exists() else {}
    _download_nkjp_frequency(data_dir, resolved)
    target = data_dir / "kaikki-pl.jsonl"
    if not _skip(target):
        url = SOURCES["kaikki-pl"].url
        print(f"downloading {url}")
        target.write_bytes(gzip.decompress(fetch(url)))
        resolved["kaikki-pl"] = url

    sentences = data_dir / "tatoeba-sentences.tsv"
    if not _skip(sentences):
        url = "https://downloads.tatoeba.org/exports/per_language/pol/pol_sentences.tsv.bz2"
        print(f"downloading {url}")
        sentences.write_bytes(bz2.decompress(fetch(url)))
        resolved["tatoeba-pl-sentences"] = url

    english = data_dir / "tatoeba-english.tsv"
    if not _skip(english):
        url = "https://downloads.tatoeba.org/exports/per_language/eng/eng_sentences.tsv.bz2"
        print(f"downloading {url}")
        english.write_bytes(bz2.decompress(fetch(url)))
        resolved["tatoeba-pl-english"] = url

    links = data_dir / "tatoeba-links.tsv"
    if not _skip(links):
        url = "https://downloads.tatoeba.org/exports/links.tar.bz2"
        print(f"downloading {url}")
        archive = tarfile.open(fileobj=io.BytesIO(fetch(url)), mode="r:bz2")
        member = next(member for member in archive.getmembers() if member.name.endswith("links.csv"))
        extracted = archive.extractfile(member)
        if extracted is None:
            raise LookupError(f"links.csv is missing from {url}")
        links.write_bytes(extracted.read())
        resolved["tatoeba-pl-links"] = url

    resolved_path.write_text(json.dumps(resolved, indent=2) + "\n")
    return resolved


def download_burmese(data_dir: Path) -> dict:
    data_dir.mkdir(parents=True, exist_ok=True)
    resolved_path = data_dir / "resolved-my.json"
    resolved = json.loads(resolved_path.read_text()) if resolved_path.exists() else {}
    target = data_dir / "kaikki-my.jsonl"
    if not _skip(target):
        url = SOURCES["kaikki-my"].url
        print(f"downloading {url}")
        target.write_bytes(gzip.decompress(fetch(url)))
        resolved["kaikki-my"] = url
    myg2p = data_dir / "myg2p.ver2.0.txt"
    if not _skip(myg2p):
        print(f"downloading {MYG2P_URL}")
        myg2p.write_bytes(fetch(MYG2P_URL))
        resolved["myg2p-headwords"] = MYG2P_URL
    resolved_path.write_text(json.dumps(resolved, indent=2) + "\n")
    return resolved


def _download_nkjp_frequency(data_dir: Path, resolved: dict) -> None:
    target = data_dir / "nkjp-frequency.tsv"
    if _skip(target):
        return
    archive = data_dir / "1grams.gz"
    url = SOURCES["nkjp-frequency"].url
    if not _skip(archive):
        print(f"downloading {url}")
        archive.write_bytes(fetch(url))
    convert_nkjp_unigrams(archive, target)
    resolved["nkjp-frequency"] = url


def convert_nkjp_unigrams(source_path: Path, target_path: Path) -> None:
    previous_count = None
    with gzip.open(source_path, mode="rt", encoding="utf-8", errors="strict") as source:
        with target_path.open("w", encoding="utf-8") as target:
            for rank, line in enumerate(source, 1):
                fields = line.rstrip().split(maxsplit=1)
                if len(fields) != 2 or not fields[0].isdigit():
                    raise ValueError(f"invalid NKJP unigram at line {rank}")
                count = int(fields[0])
                if previous_count is not None and count > previous_count:
                    raise ValueError(f"NKJP unigrams are not frequency-sorted at line {rank}")
                previous_count = count
                target.write(f"{rank}\t{fields[1]}\t{count}\n")


def _skip(target: Path) -> bool:
    if target.exists():
        print(f"present, skipping: {target}")
        return True
    return False


def _download_jmdict(data_dir: Path, resolved: dict) -> None:
    target = data_dir / "jmdict-eng.json"
    if _skip(target):
        return
    url = resolve_github_asset(SOURCES["jmdict-simplified"])
    print(f"downloading {url}")
    archive = zipfile.ZipFile(io.BytesIO(fetch(url)))
    member = next(name for name in archive.namelist() if name.endswith(".json"))
    target.write_bytes(archive.read(member))
    resolved["jmdict-simplified"] = url


def _download_kanjidic(data_dir: Path, resolved: dict) -> None:
    target = data_dir / "kanjidic2.xml"
    if _skip(target):
        return
    url = SOURCES["kanjidic2"].url
    print(f"downloading {url}")
    target.write_bytes(gzip.decompress(fetch(url)))
    resolved["kanjidic2"] = url


def _download_kanjivg(data_dir: Path, resolved: dict) -> None:
    target = data_dir / "kanjivg"
    if target.exists() and any(target.iterdir()):
        print(f"present, skipping: {target}")
        return
    url = resolve_github_asset(SOURCES["kanjivg"])
    print(f"downloading {url}")
    archive = zipfile.ZipFile(io.BytesIO(fetch(url)))
    target.mkdir(parents=True, exist_ok=True)
    plain_svg = re.compile(r"(?:^|/)kanji/([0-9a-f]{5})\.svg$")
    for name in archive.namelist():
        match = plain_svg.search(name)
        if match:
            (target / f"{match.group(1)}.svg").write_bytes(archive.read(name))
    resolved["kanjivg"] = url


def _download_kradfile(data_dir: Path, resolved: dict) -> None:
    target = data_dir / "kradfile.txt"
    if _skip(target):
        return
    url = SOURCES["kradfile"].url
    print(f"downloading {url}")
    target.write_text(gzip.decompress(fetch(url)).decode("euc_jp"), encoding="utf-8")
    resolved["kradfile"] = url


def _download_examples(data_dir: Path, resolved: dict) -> None:
    target = data_dir / "examples.utf"
    if _skip(target):
        return
    url = SOURCES["tatoeba"].url
    print(f"downloading {url}")
    target.write_bytes(gzip.decompress(fetch(url)))
    resolved["tatoeba"] = url


def _download_jlpt(data_dir: Path, resolved: dict) -> None:
    jlpt_dir = data_dir / "jlpt"
    jlpt_dir.mkdir(parents=True, exist_ok=True)
    urls = []
    for level in JLPT_LEVELS:
        target = jlpt_dir / f"{level}.csv"
        url = JLPT_CSV_URL.format(level=level)
        urls.append(url)
        if _skip(target):
            continue
        print(f"downloading {url}")
        target.write_bytes(fetch(url))
    resolved["tanos-jlpt"] = urls
