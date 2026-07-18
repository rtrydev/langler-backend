import csv
from pathlib import Path

LEVELS = ["N5", "N4", "N3", "N2", "N1"]


def load_levels(jlpt_dir: Path) -> dict[str, str]:
    levels: dict[str, str] = {}
    for level in LEVELS:
        with (jlpt_dir / f"{level.lower()}.csv").open(newline="", encoding="utf-8") as f:
            for row in csv.DictReader(f):
                word = (row.get("expression") or "").strip()
                if word and word not in levels:
                    levels[word] = level
    return levels
