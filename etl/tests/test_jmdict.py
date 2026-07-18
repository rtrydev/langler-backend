import pytest
import wordfreq

from langler_etl import jlpt, jmdict
from langler_etl.examples import ExampleIndex


@pytest.fixture
def vocab(data_dir):
    words = jmdict.load_words(data_dir / "jmdict-eng.json")
    levels = jlpt.load_levels(data_dir / "jlpt")
    index = ExampleIndex.from_file(data_dir / "examples.utf")
    records = jmdict.build_vocab(words, levels, index, band=lambda headword, reading: 3)
    return {record["headword"]: record for record in records}


def test_only_listed_words_included(vocab):
    assert set(vocab) == {"学校", "犬", "会う", "ああ", "経済", "辞書"}


def test_headword_reading_gloss_pos(vocab):
    school = vocab["学校"]
    assert school["SK"] == "VOCAB#N5#1206900"
    assert school["reading"] == "がっこう"
    assert school["gloss"] == ["school"]
    assert school["pos"] == ["n"]
    assert school["level"] == "N5"
    assert school["topics"] == []


def test_first_kanji_form_is_headword_and_easiest_level_wins(vocab):
    meet = vocab["会う"]
    assert meet["level"] == "N5"
    assert meet["SK"] == "VOCAB#N5#1198880"
    assert meet["reading"] == "あう"


def test_kana_only_word_matches_on_reading(vocab):
    ah = vocab["ああ"]
    assert ah["headword"] == ah["reading"] == "ああ"
    assert ah["level"] == "N5"


def test_gloss_capped_at_five_from_first_sense(vocab):
    assert len(vocab["辞書"]["gloss"]) == 5
    assert vocab["犬"]["gloss"] == ["dog"]


def test_example_embedded_with_source(vocab):
    assert vocab["学校"]["example"] == {
        "text": "私は学校に行きます。",
        "translation": "I go to school.",
        "sourceId": "tatoeba",
        "license": "CC BY 2.0 FR",
    }
    assert "example" not in vocab["経済"]


def test_source_and_attribution(vocab):
    school = vocab["学校"]
    assert school["sourceId"] == "jmdict-simplified"
    assert school["license"] == "CC BY-SA 4.0 (EDRDG)"
    assert school["attribution"]["level"] == {
        "sourceId": "tanos-jlpt",
        "license": "CC BY (Jonathan Waller, tanos.co.uk)",
    }
    assert school["freqBand"] == 3
    assert school["attribution"]["frequency"]["sourceId"] == "wordfreq"


def test_freq_band_omitted_when_band_none(data_dir):
    words = jmdict.load_words(data_dir / "jmdict-eng.json")
    levels = jlpt.load_levels(data_dir / "jlpt")
    records = jmdict.build_vocab(words, levels, ExampleIndex(), band=lambda h, r: None)
    for record in records:
        assert "freqBand" not in record
        assert "frequency" not in record["attribution"]


@pytest.mark.parametrize(
    ("zipfs", "expected"),
    [
        ({"学校": 5.1}, 3),
        ({"学校": 1.4}, 7),
        ({"学校": 0.2}, 8),
        ({"学校": 7.6}, 1),
        ({"がっこう": 4.0}, 4),
        ({}, None),
    ],
)
def test_freq_band_math(monkeypatch, zipfs, expected):
    monkeypatch.setattr(wordfreq, "zipf_frequency", lambda word, lang: zipfs.get(word, 0.0))
    assert jmdict.freq_band("学校", "がっこう") == expected
