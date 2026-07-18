from langler_etl.examples import ExampleIndex, parse_a_line, parse_b_line


def test_a_line_splits_text_and_strips_id():
    text, translation = parse_a_line("A: 私は学生です。\tI am a student.#ID=1_2")
    assert text == "私は学生です。"
    assert translation == "I am a student."


def test_b_line_strips_annotations():
    tokens = parse_b_line("B: 彼(かれ)[01]{彼の}~ は 学校~ に 行く{行きます}")
    assert tokens == [("彼", True), ("は", False), ("学校", True), ("に", False), ("行く", False)]


def test_lookup_prefers_marked_sentence_over_shorter_unmarked(data_dir):
    index = ExampleIndex.from_file(data_dir / "examples.utf")
    example = index.lookup("学校")
    assert example == {"text": "私は学校に行きます。", "translation": "I go to school."}


def test_lookup_marked_wins_for_dog(data_dir):
    index = ExampleIndex.from_file(data_dir / "examples.utf")
    assert index.lookup("犬")["text"] == "犬が好きです。"


def test_lookup_matches_base_form_of_inflected_token(data_dir):
    index = ExampleIndex.from_file(data_dir / "examples.utf")
    assert index.lookup("行く")["text"] == "私は学校に行きます。"
    assert index.lookup("居る")["text"] == "大きい犬がいる。"


def test_lookup_unmarked_takes_shortest():
    index = ExampleIndex()
    index.add("犬", False, "とても大きい犬がいる。", "There is a very big dog.")
    index.add("犬", False, "犬がいる。", "There is a dog.")
    assert index.lookup("犬")["text"] == "犬がいる。"


def test_lookup_unknown_word_returns_none(data_dir):
    index = ExampleIndex.from_file(data_dir / "examples.utf")
    assert index.lookup("経済") is None
