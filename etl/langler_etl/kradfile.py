from pathlib import Path


def parse_kradfile(path: Path) -> dict[str, list[str]]:
    components = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line or line.startswith("#"):
            continue
        glyph, separator, parts = line.partition(" : ")
        if separator and parts.strip():
            components[glyph.strip()] = parts.split()
    return components
