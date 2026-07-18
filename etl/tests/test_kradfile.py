from langler_etl.kradfile import parse_kradfile


def test_parses_components(data_dir):
    components = parse_kradfile(data_dir / "kradfile.txt")
    assert components["亜"] == ["｜", "一", "口"]
    assert components["者"] == ["土", "ノ", "日"]


def test_skips_comments(data_dir):
    components = parse_kradfile(data_dir / "kradfile.txt")
    assert len(components) == 3
