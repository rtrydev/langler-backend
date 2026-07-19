import struct

from langler_etl import embeddings


def test_quantize_produces_unit_scaled_int8():
    raw = embeddings.quantize([3.0, 4.0])
    values = struct.unpack("2b", raw)
    assert values == (76, 102)


def test_index_round_trip(tmp_path):
    path = tmp_path / "ja-vocab.embed"
    vectors = [embeddings.quantize([1.0, 0.0]), embeddings.quantize([0.0, -1.0])]
    embeddings.write_index(path, "test-model", 2, ["N5#1", "N4#2"], vectors)

    header, blob = embeddings.read_index(path)
    assert header == {
        "version": 1,
        "model": "test-model",
        "dims": 2,
        "count": 2,
        "ids": ["N5#1", "N4#2"],
    }
    assert struct.unpack("4b", blob) == (127, 0, 0, -127)


def test_word_text_combines_headword_reading_gloss():
    record = {"headword": "駅", "reading": "えき", "gloss": ["station", "train station"]}
    assert embeddings.word_text(record) == "駅 (えき) — station; train station"
