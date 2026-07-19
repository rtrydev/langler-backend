from pathlib import Path

import pytest

FIXTURE_DATA = Path(__file__).parent / "fixtures" / "data"

FIXTURE_TOPICS = {
    "topics": [
        {"slug": "school-learning", "name": "School & learning", "description": "Study and school life"},
        {"slug": "nature-weather", "name": "Nature & weather", "description": "Animals, plants, and weather"},
        {"slug": "language-communication", "name": "Language & communication", "description": "Speaking and writing"},
        {"slug": "abstract-concepts", "name": "Abstract concepts", "description": "Ideas and ways of thinking"},
    ],
    "assignments": {
        "1206900": ["school-learning"],
        "1466420": ["nature-weather"],
        "1198880": ["language-communication"],
        "1565440": ["abstract-concepts"],
        "1250090": ["abstract-concepts"],
        "1341350": ["school-learning", "language-communication"],
        "1467640": ["nature-weather"],
    },
}


@pytest.fixture
def data_dir() -> Path:
    return FIXTURE_DATA
