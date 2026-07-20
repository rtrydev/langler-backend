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
    Source(
        id="kaikki-pl",
        url="https://kaikki.org/dictionary/Polish/kaikki.org-dictionary-Polish.jsonl.gz",
        license="CC BY-SA 4.0 and GFDL",
        attribution="Kaikki.org Polish dictionary; Wiktionary contributors; Wiktextract by Tatu Ylonen",
    ),
    Source(
        id="nkjp-1m",
        url="http://clip.ipipan.waw.pl/NationalCorpusOfPolish",
        license="CC BY 3.0",
        attribution="National Corpus of Polish (NKJP), 1-million-word manually annotated subcorpus",
    ),
    Source(
        id="nkjp-frequency",
        url="https://zil.ipipan.waw.pl/NKJPNGrams?action=AttachFile&do=get&target=1grams.gz",
        license="CC BY",
        attribution="NKJP 1-grams from the balanced 300-million-token subcorpus",
    ),
    Source(
        id="tatoeba-pl",
        url="https://tatoeba.org/en/downloads",
        license="CC BY 2.0 FR",
        attribution="Tatoeba Polish-English sentence pairs",
    ),
    Source(
        id="certyfikat-polish",
        url="https://certyfikatpolski.pl/wp-content/uploads/2018/05/rozp_26_2_16.pdf",
        license="Official legal text (Dz.U. 2016 poz. 405)",
        attribution="State Certificate Examinations in Polish as a Foreign Language, adult A1-C2 grammar inventories",
    ),
    Source(
        id="morfeusz-sgjp",
        url="https://morfeusz.sgjp.pl/",
        license="BSD-2-Clause",
        attribution="Morfeusz 2 morphological analyser with SGJP inflectional data, Institute of Computer Science PAS",
    ),
    Source(
        id="kaikki-my",
        url="https://kaikki.org/dictionary/Burmese/kaikki.org-dictionary-Burmese.jsonl.gz",
        license="CC BY-SA 4.0 and GFDL",
        attribution="Kaikki.org Burmese dictionary; Wiktionary contributors; Wiktextract by Tatu Ylonen",
    ),
    Source(
        id="myanmar-c4-frequency",
        url="https://huggingface.co/datasets/chuuhtetnaing/myanmar-c4-dataset",
        license="ODC-BY 1.0",
        attribution="Myanmar-C4 frequency counts prepared by the myanmar-ime corpus builder",
    ),
    Source(
        id="myword-ngram",
        url="https://github.com/ye-kyaw-thu/myWord",
        license="GPL-3.0",
        attribution="myWord Burmese word-segmentation statistics by Ye Kyaw Thu",
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

CURATED_TOPICS = Source(
    id="langler-curated",
    url="langler-backend/etl/langler_etl/data/topics_ja.json",
    license="CC BY-SA 4.0",
    attribution="Langler project, original vocabulary topic classification",
)

CURATED_POLISH_GRAMMAR = Source(
    id="langler-curated-pl-grammar",
    url="langler-backend/etl/langler_etl/data/grammar_pl.json",
    license="CC BY-SA 4.0",
    attribution="Langler project, original hand-reviewed descriptions and examples mapped to certyfikat descriptors",
)

CURATED_POLISH_ORTHOGRAPHY = Source(
    id="langler-curated-pl-orthography",
    url="langler-backend/etl/langler_etl/data/orthography_pl.json",
    license="CC0",
    attribution="Langler project, original Polish orthography notes",
)

CURATED_POLISH_TOPICS = Source(
    id="langler-curated-pl-topics",
    url="langler-backend/etl/langler_etl/data/topics_pl.json",
    license="CC0",
    attribution="Langler project, original Polish vocabulary topic taxonomy",
)

CURATED_BURMESE_GRAMMAR = Source(
    id="langler-curated-my-grammar",
    url="langler-backend/etl/langler_etl/data/grammar_my.json",
    license="CC BY-SA 4.0",
    attribution="Langler project, hand-reviewed Burmese grammar topic inventory and examples",
)

CURATED_BURMESE_SCRIPT = Source(
    id="langler-curated-my-script",
    url="langler-backend/etl/langler_etl/data/script_my.json",
    license="CC0",
    attribution="Langler project, Burmese script inventory derived from Unicode names and myanmar-ime romanization data",
)

CURATED_BURMESE_TOPICS = Source(
    id="langler-curated-my-topics",
    url="langler-backend/etl/langler_etl/data/topics_my.json",
    license="CC0",
    attribution="Langler project, original Burmese vocabulary topic taxonomy",
)

REGISTRY = _ALL + [
    CURATED_KANA,
    CURATED_GRAMMAR,
    CURATED_TOPICS,
    CURATED_POLISH_GRAMMAR,
    CURATED_POLISH_ORTHOGRAPHY,
    CURATED_POLISH_TOPICS,
    CURATED_BURMESE_GRAMMAR,
    CURATED_BURMESE_SCRIPT,
    CURATED_BURMESE_TOPICS,
]
