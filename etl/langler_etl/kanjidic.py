import xml.etree.ElementTree as ET
from dataclasses import dataclass
from pathlib import Path

OLD_JLPT_TO_LEVEL = {"4": "N5", "3": "N4", "2": "N2", "1": "N1"}


@dataclass(frozen=True)
class Kanji:
    glyph: str
    level: str
    meanings: list[str]
    on: list[str]
    kun: list[str]
    grade: int | None
    stroke_count: int


def parse_kanjidic(path: Path) -> list[Kanji]:
    kanji = []
    for _, elem in ET.iterparse(str(path)):
        if elem.tag != "character":
            continue
        parsed = _parse_character(elem)
        if parsed is not None:
            kanji.append(parsed)
        elem.clear()
    return kanji


def _parse_character(elem: ET.Element) -> Kanji | None:
    jlpt = elem.findtext("misc/jlpt")
    if jlpt is None:
        return None
    grade = elem.findtext("misc/grade")
    readings = list(elem.iterfind("reading_meaning/rmgroup/reading"))
    return Kanji(
        glyph=elem.findtext("literal"),
        level=OLD_JLPT_TO_LEVEL[jlpt],
        meanings=[
            m.text
            for m in elem.iterfind("reading_meaning/rmgroup/meaning")
            if "m_lang" not in m.attrib
        ],
        on=[r.text for r in readings if r.get("r_type") == "ja_on"],
        kun=[r.text for r in readings if r.get("r_type") == "ja_kun"],
        grade=int(grade) if grade is not None else None,
        stroke_count=int(elem.findtext("misc/stroke_count")),
    )
