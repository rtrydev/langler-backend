import json

import pytest

from langler_etl import burmese


class Detector:
    def __init__(self, probability=0):
        self.probability = probability

    def get_zawgyi_probability(self, _text):
        return self.probability


class ZawgyiOnlyDetector:
    def get_zawgyi_probability(self, text):
        return 1 if text == "ေက်ာင္း" else 0


def test_normalize_text_converts_common_zawgyi_storage():
    assert burmese.normalize_text("ေက်ာင္း", Detector(1)) == "ကျောင်း"


def test_frequency_bands_are_approximate_cefr():
    assert [burmese.level_for_rank(rank, 100) for rank in [1, 5, 10, 30, 50, 90]] == [
        "A1", "A2", "B1", "B2", "C1", "C2"
    ]


def test_build_vocab_keeps_frequency_headwords_without_glosses():
    entries = [{
        "word": "ကျောင်း",
        "lang_code": "my",
        "pos": "noun",
        "forms": [{"form": "kyaung:", "tags": ["romanization"]}],
        "senses": [{"glosses": ["school"], "examples": [{"text": "ေက်ာင္း", "translation": "school"}]}],
    }]
    frequency = {
        "ကျောင်း": {"frequency": 100, "reading": "kyaung:"},
    }
    records = burmese.build_vocab(entries, frequency, {"သွား"}, detector=ZawgyiOnlyDetector())
    assert len(records) == 2
    glossed = next(item for item in records if item["headword"] == "ကျောင်း")
    assert glossed["gloss"] == ["school"]
    assert glossed["example"]["text"] == "ကျောင်း"
    unglossed = next(item for item in records if item["headword"] == "သွား")
    assert unglossed["sourceId"] == "myg2p-headwords"
    assert unglossed["gloss"] == []
    assert unglossed["level"] == "C2"
    assert all(item["levelApproximate"] for item in records)


def test_load_myg2p_reads_headword_column(tmp_path):
    source = tmp_path / "myg2p.txt"
    source.write_text("19663\tသုတ\tသု တ\tthu. ta.\tθṵ ta̰\n")
    assert burmese.load_myg2p(source, Detector()) == {"သုတ"}


def test_grammar_inventory_covers_a1_through_b1():
    records = burmese.grammar_records(Detector())
    levels = {record["level"] for record in records}
    assert {"A1", "A2", "B1"} <= levels
    assert len(records) >= 30
    assert all(record["reviewed"] for record in records)


def test_passage_sources_require_verified_license(tmp_path):
    source = tmp_path / "reading.jsonl"
    source.write_text(json.dumps({"text": "မြန်မာစာ ဖတ်လေ့ကျင့်ရန် စာပိုဒ်တစ်ပိုဒ် ဖြစ်ပါတယ်။"}, ensure_ascii=False) + "\n")
    with pytest.raises(ValueError, match="unverified license"):
        burmese.passage_records(
            [(source, "unknown", "unknown")],
            {"မြန်မာစာ": {"frequency": 10}},
            lambda text: [text],
        )


def test_passage_records_accept_verified_flores_source(tmp_path):
    source = tmp_path / "flores.jsonl"
    text = "မြန်မာစာ ဖတ်လေ့ကျင့်ရန် ရေးထားသော စာပိုဒ်တိုတစ်ပိုဒ် ဖြစ်ပါတယ်။"
    source.write_text(json.dumps({"sentence": text}, ensure_ascii=False) + "\n")
    records = burmese.passage_records(
        [(source, "chuuhtetnaing/myanmar-facebook-flores-dataset", "CC BY-SA 4.0")],
        {"မြန်မာစာ": {"frequency": 10}},
        lambda _text: ["မြန်မာစာ"],
        Detector(),
    )
    assert records[0]["license"] == "CC BY-SA 4.0"
    assert records[0]["levelApproximate"] is True


def test_pruned_ngram_only_keeps_selected_burmese_vocabulary():
    payload = {
        "format": "myword-ngram/v1",
        "unigram": {"ကျောင်း": 100, "စာ": 30, "rare": 1000, "ရှား": 2},
        "bigram": {"ကျောင်း": {"စာ": 20, "ရှား": 30}, "rare": {"စာ": 20}},
    }
    result = burmese.prune_ngram_asset(payload, maximum_unigrams=10)
    assert result["unigram"] == {"ကျောင်း": 100, "စာ": 30}
    assert result["bigram"] == {"ကျောင်း": {"စာ": 20}}


def test_topic_records_stay_within_dynamodb_item_size():
    vocab = [{"SK": f"VOCAB#C2#my-{index:016d}", "level": "C2", "topics": ["everyday-life"]} for index in range(20_000)]
    records = burmese.topic_records(vocab)
    assert len(records[0]["vocabIds"]) == 5000
    assert len(json.dumps(records[0], ensure_ascii=False).encode()) < 400_000
