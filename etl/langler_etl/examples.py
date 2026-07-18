import re
from pathlib import Path

_TOKEN_BASE = re.compile(r"^[^([{~]+")


class ExampleIndex:
    def __init__(self):
        self._marked: dict[str, tuple[str, str]] = {}
        self._unmarked: dict[str, tuple[str, str]] = {}

    @classmethod
    def from_file(cls, path: Path) -> "ExampleIndex":
        index = cls()
        sentence = None
        with path.open(encoding="utf-8") as f:
            for line in f:
                line = line.rstrip("\n")
                if line.startswith("A: "):
                    sentence = parse_a_line(line)
                elif line.startswith("B: ") and sentence is not None:
                    for base, marked in parse_b_line(line):
                        index.add(base, marked, *sentence)
                    sentence = None
        return index

    def add(self, word: str, marked: bool, text: str, translation: str) -> None:
        pool = self._marked if marked else self._unmarked
        current = pool.get(word)
        if current is None or len(text) < len(current[0]):
            pool[word] = (text, translation)

    def lookup(self, word: str) -> dict | None:
        pair = self._marked.get(word) or self._unmarked.get(word)
        if pair is None:
            return None
        return {"text": pair[0], "translation": pair[1]}


def parse_a_line(line: str) -> tuple[str, str]:
    japanese, _, english = line[3:].partition("\t")
    return japanese, english.rsplit("#ID=", 1)[0]


def parse_b_line(line: str) -> list[tuple[str, bool]]:
    tokens = []
    for token in line[3:].split():
        match = _TOKEN_BASE.match(token)
        if match:
            tokens.append((match.group(0), "~" in token))
    return tokens
