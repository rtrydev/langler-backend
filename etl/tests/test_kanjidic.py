import pytest

from langler_etl import kanjidic


@pytest.fixture
def kanji(data_dir):
    return {k.glyph: k for k in kanjidic.parse_kanjidic(data_dir / "kanjidic2.xml")}


def test_entries_without_jlpt_excluded(kanji):
    assert set(kanji) == {"水", "者", "亜"}


def test_old_jlpt_levels_map_to_new_bands(kanji):
    assert kanji["水"].level == "N5"
    assert kanji["者"].level == "N2"
    assert kanji["亜"].level == "N1"
    assert kanjidic.OLD_JLPT_TO_LEVEL == {"4": "N5", "3": "N4", "2": "N2", "1": "N1"}


def test_readings_split_by_type(kanji):
    water = kanji["水"]
    assert water.on == ["スイ"]
    assert water.kun == ["みず", "みず-"]


def test_meanings_exclude_other_languages(kanji):
    assert kanji["水"].meanings == ["water"]
    assert kanji["亜"].meanings == ["Asia", "rank next", "come after"]


def test_grade_and_stroke_count(kanji):
    assert kanji["水"].grade == 1
    assert kanji["水"].stroke_count == 4
    assert kanji["者"].stroke_count == 8
