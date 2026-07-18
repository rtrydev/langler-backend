from langler_etl import jlpt


def test_assigns_level_from_list(data_dir):
    levels = jlpt.load_levels(data_dir / "jlpt")
    assert levels["学校"] == "N5"
    assert levels["辞書"] == "N4"
    assert levels["経済"] == "N3"


def test_word_on_multiple_lists_takes_easiest(data_dir):
    levels = jlpt.load_levels(data_dir / "jlpt")
    assert levels["会う"] == "N5"


def test_unlisted_word_absent(data_dir):
    levels = jlpt.load_levels(data_dir / "jlpt")
    assert "猫" not in levels
