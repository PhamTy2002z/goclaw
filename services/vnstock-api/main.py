from datetime import date
import threading
from typing import Any

from cachetools import TTLCache
from fastapi import FastAPI, HTTPException, Query
from vnstock import Vnstock

app = FastAPI(title="vnstock-api")

# Thread-safe TTL caches
_lock = threading.Lock()
price_cache: TTLCache = TTLCache(maxsize=500, ttl=30)
intraday_cache: TTLCache = TTLCache(maxsize=100, ttl=30)
history_cache: TTLCache = TTLCache(maxsize=200, ttl=300)
financial_cache: TTLCache = TTLCache(maxsize=200, ttl=3600)
screening_cache: TTLCache = TTLCache(maxsize=50, ttl=300)


def _stock(symbol: str):
    return Vnstock().stock(symbol=symbol, source="VCI")


def _safe_val(v: Any) -> Any:
    try:
        return v.item()
    except AttributeError:
        return v


def _df_to_records(df) -> list[dict]:
    records = df.to_dict(orient="records")
    return [{k: _safe_val(v) for k, v in row.items()} for row in records]


@app.get("/health")
def health():
    return {"status": "ok"}


@app.get("/price/{symbol}")
def get_price(symbol: str):
    sym = symbol.upper()
    with _lock:
        if sym in price_cache:
            return price_cache[sym]

    try:
        stock = _stock(sym)
        df = stock.trading.price_board([sym])
        if df is None or df.empty:
            raise HTTPException(status_code=404, detail=f"No price data for {sym}")

        row = df.iloc[0]
        # Flatten MultiIndex columns if present
        cols = [c[-1] if isinstance(c, tuple) else c for c in df.columns]
        data = dict(zip(cols, [_safe_val(v) for v in row.values]))

        # Map to canonical response fields
        result = {
            "symbol": sym,
            "price": data.get("match_price") or data.get("price") or data.get("close"),
            "change": data.get("price_change") or data.get("change"),
            "change_pct": data.get("price_change_ratio") or data.get("change_pct"),
            "volume": data.get("match_vol") or data.get("volume"),
            "bid": data.get("best_bid_price1") or data.get("bid"),
            "ask": data.get("best_offer_price1") or data.get("ask"),
            "raw": data,
        }
    except HTTPException:
        raise
    except Exception as exc:
        raise HTTPException(status_code=502, detail=f"Upstream error: {exc}") from exc

    with _lock:
        price_cache[sym] = result
    return result


@app.get("/history/{symbol}")
def get_history(
    symbol: str,
    start: str = Query(default=str(date.today().replace(month=1, day=1))),
    end: str = Query(default=str(date.today())),
    interval: str = Query(default="1D"),
):
    sym = symbol.upper()
    cache_key = f"{sym}:{start}:{end}:{interval}"
    with _lock:
        if cache_key in history_cache:
            return history_cache[cache_key]

    try:
        stock = _stock(sym)
        df = stock.quote.history(start=start, end=end, interval=interval)
        if df is None or df.empty:
            raise HTTPException(status_code=404, detail=f"No history data for {sym}")
        records = _df_to_records(df)
    except HTTPException:
        raise
    except Exception as exc:
        raise HTTPException(status_code=502, detail=f"Upstream error: {exc}") from exc

    result = {"symbol": sym, "interval": interval, "data": records}
    with _lock:
        history_cache[cache_key] = result
    return result


@app.get("/intraday/{symbol}")
def get_intraday(symbol: str):
    sym = symbol.upper()
    with _lock:
        if sym in intraday_cache:
            return intraday_cache[sym]

    try:
        stock = _stock(sym)
        df = stock.quote.intraday()
        if df is None or df.empty:
            raise HTTPException(status_code=404, detail=f"No intraday data for {sym}")
        records = _df_to_records(df)
    except HTTPException:
        raise
    except Exception as exc:
        raise HTTPException(status_code=502, detail=f"Upstream error: {exc}") from exc

    result = {"symbol": sym, "data": records}
    with _lock:
        intraday_cache[sym] = result
    return result


@app.get("/financials/{symbol}")
def get_financials(
    symbol: str,
    type: str = Query(default="balance_sheet"),
    period: str = Query(default="quarter"),
):
    sym = symbol.upper()
    cache_key = f"{sym}:{type}:{period}"
    with _lock:
        if cache_key in financial_cache:
            return financial_cache[cache_key]

    try:
        stock = _stock(sym)
        fin = stock.finance
        if type == "income_statement":
            df = fin.income_statement(period=period)
        elif type == "cash_flow":
            df = fin.cash_flow(period=period)
        else:
            df = fin.balance_sheet(period=period)

        if df is None or df.empty:
            raise HTTPException(status_code=404, detail=f"No financial data for {sym}")
        records = _df_to_records(df)
    except HTTPException:
        raise
    except Exception as exc:
        raise HTTPException(status_code=502, detail=f"Upstream error: {exc}") from exc

    result = {"symbol": sym, "type": type, "period": period, "data": records}
    with _lock:
        financial_cache[cache_key] = result
    return result


@app.get("/screening")
def get_screening(exchange: str = Query(default="")):
    cache_key = exchange.upper()
    with _lock:
        if cache_key in screening_cache:
            return screening_cache[cache_key]

    try:
        df = Vnstock().stock(symbol="ACB", source="VCI").listing.all_symbols()
        if df is None or df.empty:
            raise HTTPException(status_code=502, detail="No listing data available")

        records = _df_to_records(df)
        if exchange:
            ex = exchange.upper()
            records = [r for r in records if str(r.get("exchange", "")).upper() == ex]
    except HTTPException:
        raise
    except Exception as exc:
        raise HTTPException(status_code=502, detail=f"Upstream error: {exc}") from exc

    result = {"exchange": exchange.upper() or "ALL", "data": records}
    with _lock:
        screening_cache[cache_key] = result
    return result
