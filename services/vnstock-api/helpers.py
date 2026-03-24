from typing import Any
from vnstock import Vnstock


def stock(symbol: str):
    return Vnstock().stock(symbol=symbol, source="VCI")


def safe_val(v: Any) -> Any:
    try:
        return v.item()
    except AttributeError:
        return v


def df_to_records(df) -> list[dict]:
    records = df.to_dict(orient="records")
    return [{k: safe_val(v) for k, v in row.items()} for row in records]
