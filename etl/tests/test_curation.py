import json

import pytest

from langler_etl import curation

TOPIC_DATA = {
    "topics": [
        {
            "slug": "food-dining",
            "name": "Food & dining",
            "description": "Meals and eating out",
            "keywords": ["food", "eat", "meal", "restaurant", "bread"],
        },
        {
            "slug": "weather-seasons",
            "name": "Weather & seasons",
            "description": "Weather and the climate",
            "keywords": ["weather", "rain", "snow", "storm", "winter"],
        },
        {
            "slug": "directions-transport",
            "name": "Directions & transport",
            "description": "Getting around",
            "keywords": ["road", "train", "bus", "station", "drive"],
        },
    ]
}


def _vocab(word_id, glosses, level="C1"):
    return {"PK": "REF#pl", "SK": f"VOCAB#{level}#{word_id}", "level": level, "gloss": glosses}


def test_gloss_tokens_keeps_whole_words_only():
    tokens = curation.gloss_tokens(["a mango, to go abroad"])
    assert "mango" in tokens
    assert "go" not in tokens
    assert "abroad" in tokens


def test_gloss_tokens_drops_stopwords_and_lexicographic_labels():
    assert curation.gloss_tokens(["(obsolete, plural) the bread"]) == ["bread"]


def test_classify_uses_keywords_before_any_seed_examples():
    seed = {"a": ["food-dining"]}
    vocab = [_vocab("a", ["a loaf of bread"])]
    model = curation.train(vocab, seed, TOPIC_DATA)
    assert curation.classify(["heavy rain and snow"], model) == ["weather-seasons"]


def test_classify_learns_from_seed_glosses():
    seed = {str(index): ["directions-transport"] for index in range(6)}
    vocab = [_vocab(str(index), ["a tram stop near the crossing"]) for index in range(6)]
    model = curation.train(vocab, seed, TOPIC_DATA)
    assert curation.classify(["the tram crossing"], model) == ["directions-transport"]


def test_classify_always_returns_at_least_one_topic():
    seed = {"a": ["food-dining"]}
    vocab = [_vocab("a", ["bread"])]
    model = curation.train(vocab, seed, TOPIC_DATA)
    assert len(curation.classify([], model)) >= 1
    assert len(curation.classify(["zzzz"], model)) >= 1


def test_classify_never_exceeds_the_slug_limit():
    seed = {"a": ["food-dining"], "b": ["weather-seasons"], "c": ["directions-transport"]}
    vocab = [_vocab("a", ["bread"]), _vocab("b", ["rain"]), _vocab("c", ["train"])]
    model = curation.train(vocab, seed, TOPIC_DATA)
    assert len(curation.classify(["bread rain train"], model)) <= 3


def test_extend_keeps_seed_assignments_verbatim():
    seed = {"a": ["weather-seasons", "food-dining"]}
    vocab = [_vocab("a", ["bread"]), _vocab("b", ["a winter storm"])]
    assignments = curation.extend(vocab, seed, TOPIC_DATA)
    assert assignments["a"] == ["weather-seasons", "food-dining"]
    assert assignments["b"] == ["weather-seasons"]


def test_extend_assigns_every_word():
    seed = {"a": ["food-dining"]}
    vocab = [_vocab("a", ["bread"]), _vocab("b", []), _vocab("c", ["qqq"])]
    assignments = curation.extend(vocab, seed, TOPIC_DATA)
    assert set(assignments) == {"a", "b", "c"}
    assert all(assignments.values())


def test_train_rejects_an_empty_seed():
    with pytest.raises(ValueError, match="seed assignments"):
        curation.train([_vocab("a", ["bread"])], {}, TOPIC_DATA)


def test_load_seed_merges_batches_for_one_language_only(tmp_path):
    (tmp_path / "pl-0000.json").write_text(json.dumps({"a": ["food-dining"]}), encoding="utf-8")
    (tmp_path / "pl-0001.json").write_text(json.dumps({"b": ["weather-seasons"]}), encoding="utf-8")
    (tmp_path / "my-0000.json").write_text(json.dumps({"c": ["food-dining"]}), encoding="utf-8")

    assert curation.load_seed(tmp_path, "pl") == {"a": ["food-dining"], "b": ["weather-seasons"]}
