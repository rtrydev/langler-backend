import gzip
import io
import json
import re
import urllib.request
import zipfile
from pathlib import Path

from .sources import SOURCES, Source

USER_AGENT = "langler-etl (language-learning reference data pipeline)"
JLPT_LEVELS = ["n5", "n4", "n3", "n2", "n1"]
JLPT_CSV_URL = "https://raw.githubusercontent.com/jamsinclair/open-anki-jlpt-decks/main/src/{level}.csv"


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
