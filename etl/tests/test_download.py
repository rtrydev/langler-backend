import gzip

import pytest

from langler_etl import download


def test_convert_nkjp_unigrams_writes_frequency_ranks(tmp_path):
    source = tmp_path / "1grams.gz"
    target = tmp_path / "nkjp-frequency.tsv"
    with gzip.open(source, "wt", encoding="utf-8") as compressed:
        compressed.write("20 kot\n15 pies\n15 żaba\n")

    download.convert_nkjp_unigrams(source, target)

    assert target.read_text(encoding="utf-8") == (
        "1\tkot\t20\n2\tpies\t15\n3\tżaba\t15\n"
    )


def test_convert_nkjp_unigrams_rejects_unsorted_counts(tmp_path):
    source = tmp_path / "1grams.gz"
    with gzip.open(source, "wt", encoding="utf-8") as compressed:
        compressed.write("15 kot\n20 pies\n")

    with pytest.raises(ValueError, match="not frequency-sorted"):
        download.convert_nkjp_unigrams(source, tmp_path / "nkjp-frequency.tsv")
