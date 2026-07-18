from dataclasses import dataclass


@dataclass(frozen=True)
class Source:
    id: str
    url: str
    license: str
    attribution: str
    asset_pattern: str | None = None


_ALL = [
    Source(
        id="jmdict-simplified",
        url="https://api.github.com/repos/scriptin/jmdict-simplified/releases/latest",
        asset_pattern=r"^jmdict-eng-\d[^/]*\.json\.zip$",
        license="CC BY-SA 4.0 (EDRDG)",
        attribution="JMdict, Electronic Dictionary Research and Development Group; JSON by scriptin/jmdict-simplified",
    ),
    Source(
        id="kanjidic2",
        url="http://www.edrdg.org/kanjidic/kanjidic2.xml.gz",
        license="CC BY-SA 4.0 (EDRDG)",
        attribution="KANJIDIC2, Electronic Dictionary Research and Development Group",
    ),
    Source(
        id="kanjivg",
        url="https://api.github.com/repos/KanjiVG/kanjivg/releases/latest",
        asset_pattern=r"^kanjivg-\d+-main\.zip$",
        license="CC BY-SA 3.0",
        attribution="KanjiVG, Ulrich Apel",
    ),
    Source(
        id="kradfile",
        url="http://ftp.edrdg.org/pub/Nihongo/kradfile.gz",
        license="CC BY-SA 4.0 (EDRDG)",
        attribution="KRADFILE, Electronic Dictionary Research and Development Group",
    ),
    Source(
        id="tatoeba",
        url="http://ftp.edrdg.org/pub/Nihongo/examples.utf.gz",
        license="CC BY 2.0 FR",
        attribution="Tatoeba / Tanaka Corpus, as distributed by EDRDG",
    ),
    Source(
        id="tanos-jlpt",
        url="https://github.com/jamsinclair/open-anki-jlpt-decks",
        license="CC BY (Jonathan Waller, tanos.co.uk)",
        attribution="Jonathan Waller, tanos.co.uk",
    ),
    Source(
        id="wordfreq",
        url="https://github.com/rspeer/wordfreq",
        license="Apache 2.0 (data CC BY-SA 4.0)",
        attribution="wordfreq, Robyn Speer",
    ),
]

SOURCES = {source.id: source for source in _ALL}

CURATED_KANA = Source(
    id="langler-curated",
    url="langler-backend/etl/langler_etl/data/kana.json",
    license="CC0",
    attribution="Langler project, original kana inventory",
)

CURATED_GRAMMAR = Source(
    id="langler-curated",
    url="langler-backend/etl/langler_etl/data/grammar_ja.json",
    license="CC BY-SA 4.0",
    attribution="Langler project, original grammar topic inventory",
)

REGISTRY = _ALL + [CURATED_KANA, CURATED_GRAMMAR]
