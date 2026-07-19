import json
import struct
from pathlib import Path

MODEL_ID = "cohere.embed-multilingual-v3"
BATCH_SIZE = 96


def word_text(record: dict) -> str:
    gloss = "; ".join(record["gloss"])
    return f"{record['headword']} ({record['reading']}) — {gloss}"


def quantize(vector: list[float]) -> bytes:
    norm = sum(x * x for x in vector) ** 0.5 or 1.0
    return bytes((round(x / norm * 127)) & 0xFF for x in vector)


def write_index(path: Path, model_id: str, dims: int, ids: list[str], vectors: list[bytes]) -> None:
    header = json.dumps(
        {"version": 1, "model": model_id, "dims": dims, "count": len(ids), "ids": ids},
        separators=(",", ":"),
    ).encode("utf-8")
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("wb") as f:
        f.write(struct.pack(">I", len(header)))
        f.write(header)
        for vector in vectors:
            f.write(vector)


def read_index(path: Path) -> tuple[dict, bytes]:
    raw = path.read_bytes()
    (header_len,) = struct.unpack(">I", raw[:4])
    header = json.loads(raw[4 : 4 + header_len].decode("utf-8"))
    return header, raw[4 + header_len :]


def embed_corpus(out_dir: Path, region: str, model_id: str = MODEL_ID, language: str = "ja") -> Path:
    import boto3

    client = boto3.client("bedrock-runtime", region_name=region)
    records = []
    with (out_dir / "reference" / language / "vocab.jsonl").open(encoding="utf-8") as f:
        for line in f:
            if line.strip():
                records.append(json.loads(line))

    ids = [record["SK"].removeprefix("VOCAB#") for record in records]
    texts = [word_text(record) for record in records]
    dims = None
    vectors: list[bytes] = []
    for offset in range(0, len(texts), BATCH_SIZE):
        body = json.dumps(
            {
                "texts": texts[offset : offset + BATCH_SIZE],
                "input_type": "search_document",
                "truncate": "END",
            }
        )
        response = client.invoke_model(modelId=model_id, body=body)
        for vector in json.loads(response["body"].read())["embeddings"]:
            if dims is None:
                dims = len(vector)
            elif len(vector) != dims:
                raise ValueError("embedding dimensions changed mid-corpus")
            vectors.append(quantize(vector))

    path = out_dir / "embeddings" / f"{language}-vocab.embed"
    write_index(path, model_id, dims or 0, ids, vectors)
    return path
