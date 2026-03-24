import threading
from cachetools import TTLCache

_lock = threading.Lock()

price_cache: TTLCache = TTLCache(maxsize=500, ttl=30)
intraday_cache: TTLCache = TTLCache(maxsize=100, ttl=30)
history_cache: TTLCache = TTLCache(maxsize=200, ttl=300)
financial_cache: TTLCache = TTLCache(maxsize=200, ttl=3600)
screening_cache: TTLCache = TTLCache(maxsize=50, ttl=300)
news_cache: TTLCache = TTLCache(maxsize=200, ttl=300)
indicator_cache: TTLCache = TTLCache(maxsize=200, ttl=300)
bond_cache: TTLCache = TTLCache(maxsize=50, ttl=3600)
event_cache: TTLCache = TTLCache(maxsize=200, ttl=3600)


def get(cache: TTLCache, key: str):
    with _lock:
        return cache.get(key)


def put(cache: TTLCache, key: str, value):
    with _lock:
        cache[key] = value
