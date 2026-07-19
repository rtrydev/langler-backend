import json
import tarfile
from collections import Counter
from io import BytesIO

import pytest

from langler_etl import build_polish, polish


class RecognizingAnalyzer:
    def analyse(self, token):
        tag = "ign" if token == "Qwerty" else "subst:sg:nom:m3"
        return [(0, 1, (token, token.casefold(), tag, [], []))]


@pytest.fixture
def entries():
    return [
        {
            "lang_code": "pl",
            "word": "król",
            "pos": "noun",
            "senses": [{"glosses": ["king", "male monarch"]}],
        },
        {
            "lang": "Polish",
            "word": "czytać",
            "pos": "verb",
            "senses": [{"glosses": ["to read"]}],
        },
        {
            "lang_code": "en",
            "word": "king",
            "pos": "noun",
            "senses": [{"glosses": ["monarch"]}],
        },
    ]


def test_polish_vocab_is_cefr_banded_and_licensed(entries):
    examples = polish.PolishExampleIndex()
    examples.add("Król mieszka w zamku.", "The king lives in a castle.", "tatoeba-pl", "CC BY 2.0 FR")
    records = polish.build_vocab(
        entries,
        {"król": 800, "czytać": 5000},
        examples,
        band=lambda word: {"król": 2, "czytać": 4}[word],
    )
    by_word = {record["headword"]: record for record in records}

    king = by_word["król"]
    assert king["PK"] == "REF#pl"
    assert king["SK"].startswith("VOCAB#A1#pl-")
    assert king["levelApproximate"] is True
    assert king["sourceId"] == "kaikki-pl"
    assert king["license"] == "CC BY-SA 4.0 and GFDL"
    assert king["attribution"]["level"]["approximate"] is True
    assert king["example"]["text"] == "Król mieszka w zamku."
    assert by_word["czytać"]["level"] == "B1"


def test_polish_vocab_excludes_inflected_forms_and_words_outside_frequency_cap(entries):
    entries.extend(
        [
            {
                "lang_code": "pl",
                "word": "króla",
                "pos": "noun",
                "senses": [
                    {
                        "glosses": ["genitive singular of król"],
                        "tags": ["form-of"],
                        "form_of": [{"word": "król"}],
                    }
                ],
            },
            {
                "lang_code": "pl",
                "word": "rzadkość",
                "pos": "noun",
                "senses": [{"glosses": ["rarity"]}],
            },
        ]
    )

    records = polish.build_vocab(
        entries,
        {"król": 800, "czytać": 5000, "króla": 900, "rzadkość": polish.MAX_NKJP_RANK + 1},
        polish.PolishExampleIndex(),
        band=lambda _word: 4,
    )

    assert {record["headword"] for record in records} == {"król", "czytać"}


@pytest.mark.parametrize(
    ("band", "level"),
    [(1, "A1"), (2, "A2"), (3, "B1"), (4, "B1"), (5, "B2"), (6, "C1"), (7, "C1"), (8, "C2")],
)
def test_frequency_bands_map_to_approximate_cefr(band, level):
    assert polish.cefr_for_band(band) == level


def test_combined_band_uses_the_more_common_corpus_signal():
    assert polish.combined_band("król", 800, band=lambda _word: 2) == 1
    assert polish.combined_band("specjalizm", 70_000, band=lambda _word: 8) == 8


def test_grammar_inventory_and_orthography_cover_every_band_and_classic_traps():
    grammar = polish.grammar_records(analyzer=RecognizingAnalyzer())
    counts = Counter(record["level"] for record in grammar)
    assert all(counts[level] >= 8 for level in polish.LEVELS)
    assert len(grammar) >= 100
    assert all(record["sourceId"] == "certyfikat-polish" for record in grammar)
    assert all(record["attribution"]["wording"]["sourceId"] == "langler-curated-pl-grammar" for record in grammar)
    assert all(record["attribution"]["morphologyValidation"]["coveragePercent"] >= 80 for record in grammar)
    assert all(record["introducedAt"] == record["level"] for record in grammar)

    notes = polish.orthography_records()
    patterns = {record["glyph"] for record in notes}
    assert {"ó / u", "rz / ż", "ch / h"} <= patterns
    assert all(record["SK"].startswith("SCRIPT#ORTHOGRAPHY#") for record in notes)


def test_build_polish_writes_reference_partition(tmp_path):
    data = tmp_path / "data"
    output = tmp_path / "output"
    data.mkdir()
    entries = []
    suffixes = iter("abcdefghijklmnopqrstuvwx")
    for index, level in enumerate(polish.LEVELS, 1):
        for offset in range(4):
            entries.append(
                {
                    "lang_code": "pl",
                    "word": f"słowo{next(suffixes)}",
                    "pos": "noun",
                    "senses": [{"glosses": ["everyday thing"]}],
                }
            )
    (data / "kaikki-pl.jsonl").write_text(
        "".join(json.dumps(entry, ensure_ascii=False) + "\n" for entry in entries),
        encoding="utf-8",
    )
    (data / "nkjp-frequency.tsv").write_text(
        "".join(f"{index}\t{entry['word']}\n" for index, entry in enumerate(entries, 1)),
        encoding="utf-8",
    )
    values = iter([1] * 4 + [2] * 4 + [3] * 4 + [5] * 4 + [6] * 4 + [8] * 4)
    manifest = build_polish.build(data, output, band=lambda word: next(values))

    ref = output / "reference" / "pl"
    assert {path.name for path in ref.glob("*.jsonl")} == {
        "grammar.jsonl",
        "scripts.jsonl",
        "topics.jsonl",
        "vocab.jsonl",
    }
    assert manifest["counts"]["vocab"]["total"] == 24
    assert manifest["counts"]["grammar"]["total"] >= 100


def test_nkjp_archive_reconstructs_annotated_sentences(tmp_path):
    xml = """<teiCorpus xmlns='http://www.tei-c.org/ns/1.0'><TEI><text><body><p><s>
      <seg><fs><f name='orth'><string>Ala</string></f></fs></seg>
      <seg><fs><f name='orth'><string>ma</string></f></fs></seg>
      <seg><fs><f name='orth'><string>bardzo</string></f></fs></seg>
      <seg><fs><f name='orth'><string>miłego</string></f></fs></seg>
      <seg><fs><f name='orth'><string>kota</string></f></fs></seg>
      <seg><fs><f name='orth'><string>.</string></f></fs></seg>
    </s></p></body></text></TEI></teiCorpus>""".encode()
    archive_path = tmp_path / "nkjp.tar.gz"
    with tarfile.open(archive_path, "w:gz") as archive:
        info = tarfile.TarInfo("sample/ann_morphosyntax.xml")
        info.size = len(xml)
        archive.addfile(info, BytesIO(xml))

    assert polish.load_nkjp_sentences(archive_path) == ["Ala ma bardzo miłego kota."]


def test_morphology_validation_rejects_mostly_unknown_examples():
    with pytest.raises(ValueError, match="insufficient Morfeusz coverage"):
        polish.morphology_validation("Qwerty Qwerty Qwerty Ala", RecognizingAnalyzer())
