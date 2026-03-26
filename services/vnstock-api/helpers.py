import math
from typing import Any
from vnstock import Vnstock

# Singleton Vnstock instance — avoids re-initializing on every request
_vnstock = Vnstock()


def stock(symbol: str):
    return _vnstock.stock(symbol=symbol, source="VCI")


def safe_val(v: Any) -> Any:
    try:
        v = v.item()
    except AttributeError:
        pass
    # Replace NaN/Inf with None for JSON compatibility
    if isinstance(v, float) and (math.isnan(v) or math.isinf(v)):
        return None
    return v


def df_to_records(df) -> list[dict]:
    records = df.to_dict(orient="records")
    return [{k: safe_val(v) for k, v in row.items()} for row in records]
