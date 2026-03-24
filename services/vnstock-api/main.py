from datetime import date

from fastapi import FastAPI, HTTPException, Query
from vnstock import Vnstock

import cache
from helpers import stock, df_to_records, safe_val

app = FastAPI(title="vnstock-api")


@app.get("/health")
def health():
    return {"status": "ok"}


@app.get("/price/{symbol}")
def get_price(symbol: str):
    sym = symbol.upper()
    if cached := cache.get(cache.price_cache, sym):
        return cached
    try:
        df = stock(sym).trading.price_board([sym])
        if df is None or df.empty:
            raise HTTPException(404, f"No price data for {sym}")
        row = df.iloc[0]
        cols = [c[-1] if isinstance(c, tuple) else c for c in df.columns]
        data = dict(zip(cols, [safe_val(v) for v in row.values]))
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
        raise HTTPException(502, f"Upstream error: {exc}") from exc
    cache.put(cache.price_cache, sym, result)
    return result


@app.get("/history/{symbol}")
def get_history(
    symbol: str,
    start: str = Query(default=str(date.today().replace(month=1, day=1))),
    end: str = Query(default=str(date.today())),
    interval: str = Query(default="1D"),
):
    sym = symbol.upper()
    key = f"{sym}:{start}:{end}:{interval}"
    if cached := cache.get(cache.history_cache, key):
        return cached
    try:
        df = stock(sym).quote.history(start=start, end=end, interval=interval)
        if df is None or df.empty:
            raise HTTPException(404, f"No history data for {sym}")
        records = df_to_records(df)
    except HTTPException:
        raise
    except Exception as exc:
        raise HTTPException(502, f"Upstream error: {exc}") from exc
    result = {"symbol": sym, "interval": interval, "data": records}
    cache.put(cache.history_cache, key, result)
    return result


@app.get("/intraday/{symbol}")
def get_intraday(symbol: str):
    sym = symbol.upper()
    if cached := cache.get(cache.intraday_cache, sym):
        return cached
    try:
        df = stock(sym).quote.intraday()
        if df is None or df.empty:
            raise HTTPException(404, f"No intraday data for {sym}")
        records = df_to_records(df)
    except HTTPException:
        raise
    except Exception as exc:
        raise HTTPException(502, f"Upstream error: {exc}") from exc
    result = {"symbol": sym, "data": records}
    cache.put(cache.intraday_cache, sym, result)
    return result


@app.get("/financials/{symbol}")
def get_financials(
    symbol: str,
    type: str = Query(default="balance_sheet"),
    period: str = Query(default="quarter"),
):
    sym = symbol.upper()
    key = f"{sym}:{type}:{period}"
    if cached := cache.get(cache.financial_cache, key):
        return cached
    try:
        fin = stock(sym).finance
        if type == "income_statement":
            df = fin.income_statement(period=period)
        elif type == "cash_flow":
            df = fin.cash_flow(period=period)
        else:
            df = fin.balance_sheet(period=period)
        if df is None or df.empty:
            raise HTTPException(404, f"No financial data for {sym}")
        records = df_to_records(df)
    except HTTPException:
        raise
    except Exception as exc:
        raise HTTPException(502, f"Upstream error: {exc}") from exc
    result = {"symbol": sym, "type": type, "period": period, "data": records}
    cache.put(cache.financial_cache, key, result)
    return result


@app.get("/screening")
def get_screening(exchange: str = Query(default="")):
    key = exchange.upper()
    if cached := cache.get(cache.screening_cache, key):
        return cached
    try:
        df = Vnstock().stock(symbol="ACB", source="VCI").listing.all_symbols()
        if df is None or df.empty:
            raise HTTPException(502, "No listing data available")
        records = df_to_records(df)
        if exchange:
            ex = exchange.upper()
            records = [r for r in records if str(r.get("exchange", "")).upper() == ex]
    except HTTPException:
        raise
    except Exception as exc:
        raise HTTPException(502, f"Upstream error: {exc}") from exc
    result = {"exchange": key or "ALL", "data": records}
    cache.put(cache.screening_cache, key, result)
    return result


# --- New endpoints ---


@app.get("/indicators/{symbol}")
def get_indicators(
    symbol: str,
    indicators: str = Query(default="SMA_20,RSI_14,MACD"),
    start: str = Query(default=str(date.today().replace(month=1, day=1))),
    end: str = Query(default=str(date.today())),
):
    """Compute technical indicators on OHLCV data using pandas-ta."""
    import pandas as pd
    import pandas_ta as ta

    sym = symbol.upper()
    key = f"{sym}:{indicators}:{start}:{end}"
    if cached := cache.get(cache.indicator_cache, key):
        return cached
    try:
        df = stock(sym).quote.history(start=start, end=end, interval="1D")
        if df is None or df.empty:
            raise HTTPException(404, f"No history data for {sym}")

        indicator_list = [i.strip() for i in indicators.split(",") if i.strip()]
        for name in indicator_list:
            parts = name.split("_")
            kind = parts[0].lower()
            length = int(parts[1]) if len(parts) > 1 and parts[1].isdigit() else None
            kwargs = {"length": length} if length else {}
            fn = getattr(ta, kind, None)
            if fn is None:
                continue
            result_series = fn(df["close"], **kwargs)
            if isinstance(result_series, pd.DataFrame):
                df = pd.concat([df, result_series], axis=1)
            elif result_series is not None:
                df[name] = result_series

        df = df.dropna().tail(60)
        records = df_to_records(df)
    except HTTPException:
        raise
    except Exception as exc:
        raise HTTPException(502, f"Indicator error: {exc}") from exc
    result = {"symbol": sym, "indicators": indicator_list, "data": records}
    cache.put(cache.indicator_cache, key, result)
    return result


@app.get("/news/{symbol}")
def get_news(symbol: str):
    """Get latest news for a stock symbol."""
    sym = symbol.upper()
    if cached := cache.get(cache.news_cache, sym):
        return cached
    try:
        df = stock(sym).company.news()
        if df is None or df.empty:
            raise HTTPException(404, f"No news for {sym}")
        # Keep only useful columns, drop full HTML content
        keep = ["news_title", "news_short_content", "news_source_link", "public_date"]
        cols = [c for c in keep if c in df.columns]
        records = df_to_records(df[cols]) if cols else df_to_records(df)
    except HTTPException:
        raise
    except Exception as exc:
        raise HTTPException(502, f"News error: {exc}") from exc
    result = {"symbol": sym, "data": records}
    cache.put(cache.news_cache, sym, result)
    return result


@app.get("/bonds")
def get_bonds():
    """Get bond listings (placeholder — VCI source does not support bond type)."""
    raise HTTPException(501, "Bond data not yet supported on VCI source")


@app.get("/events/{symbol}")
def get_events(symbol: str):
    """Get corporate events for a stock symbol."""
    sym = symbol.upper()
    if cached := cache.get(cache.event_cache, sym):
        return cached
    try:
        df = stock(sym).company.events()
        if df is None or df.empty:
            raise HTTPException(404, f"No event data for {sym}")
        records = df_to_records(df)
    except HTTPException:
        raise
    except Exception as exc:
        raise HTTPException(502, f"Event error: {exc}") from exc
    result = {"symbol": sym, "data": records}
    cache.put(cache.event_cache, sym, result)
    return result
