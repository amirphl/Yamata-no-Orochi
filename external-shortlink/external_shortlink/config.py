from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path


def _int(name: str, default: int, minimum: int = 1) -> int:
    raw = os.getenv(name, str(default))
    try:
        value = int(raw)
    except ValueError as exc:
        raise RuntimeError(f"{name} must be an integer") from exc
    if value < minimum:
        raise RuntimeError(f"{name} must be at least {minimum}")
    return value


def _float(name: str, default: float, minimum: float = 0.0) -> float:
    raw = os.getenv(name, str(default))
    try:
        value = float(raw)
    except ValueError as exc:
        raise RuntimeError(f"{name} must be numeric") from exc
    if value < minimum:
        raise RuntimeError(f"{name} must be at least {minimum}")
    return value


@dataclass(frozen=True, slots=True)
class Settings:
    database_url: str
    api_token: str
    spool_path: Path
    pool_min_size: int
    pool_max_size: int
    db_command_timeout_seconds: float
    click_insert_timeout_seconds: float
    link_lookup_timeout_seconds: float
    cache_max_entries: int
    cache_preload_entries: int
    admin_batch_max_links: int
    click_fetch_max_limit: int
    max_admin_body_bytes: int
    spool_max_bytes: int
    spool_max_events: int
    spool_replay_batch_size: int
    spool_replay_interval_seconds: float
    acknowledged_retention_days: int
    purge_interval_seconds: float

    @classmethod
    def from_env(cls) -> "Settings":
        database_url = os.getenv("EXTERNAL_SHORTLINK_DATABASE_URL", "").strip()
        api_token = os.getenv("EXTERNAL_SHORTLINK_API_TOKEN", "").strip()
        if not database_url:
            raise RuntimeError("EXTERNAL_SHORTLINK_DATABASE_URL is required")
        if len(api_token) < 32:
            raise RuntimeError("EXTERNAL_SHORTLINK_API_TOKEN must contain at least 32 characters")
        pool_min = _int("EXTERNAL_SHORTLINK_POOL_MIN_SIZE", 2)
        pool_max = _int("EXTERNAL_SHORTLINK_POOL_MAX_SIZE", 20)
        if pool_min > pool_max:
            raise RuntimeError("EXTERNAL_SHORTLINK_POOL_MIN_SIZE cannot exceed POOL_MAX_SIZE")
        return cls(
            database_url=database_url,
            api_token=api_token,
            spool_path=Path(os.getenv("EXTERNAL_SHORTLINK_SPOOL_PATH", "/var/lib/external-shortlink/click-spool.sqlite3")),
            pool_min_size=pool_min,
            pool_max_size=pool_max,
            db_command_timeout_seconds=_float("EXTERNAL_SHORTLINK_DB_COMMAND_TIMEOUT_SECONDS", 5.0, 0.01),
            click_insert_timeout_seconds=_float("EXTERNAL_SHORTLINK_CLICK_INSERT_TIMEOUT_SECONDS", 0.025, 0.001),
            link_lookup_timeout_seconds=_float("EXTERNAL_SHORTLINK_LINK_LOOKUP_TIMEOUT_SECONDS", 0.150, 0.001),
            cache_max_entries=_int("EXTERNAL_SHORTLINK_CACHE_MAX_ENTRIES", 100_000),
            cache_preload_entries=_int("EXTERNAL_SHORTLINK_CACHE_PRELOAD_ENTRIES", 50_000, 0),
            admin_batch_max_links=_int("EXTERNAL_SHORTLINK_ADMIN_BATCH_MAX_LINKS", 10_000),
            click_fetch_max_limit=_int("EXTERNAL_SHORTLINK_CLICK_FETCH_MAX_LIMIT", 10_000),
            max_admin_body_bytes=_int("EXTERNAL_SHORTLINK_MAX_ADMIN_BODY_BYTES", 16 * 1024 * 1024),
            spool_max_bytes=_int("EXTERNAL_SHORTLINK_SPOOL_MAX_BYTES", 2 * 1024 * 1024 * 1024),
            spool_max_events=_int("EXTERNAL_SHORTLINK_SPOOL_MAX_EVENTS", 5_000_000),
            spool_replay_batch_size=_int("EXTERNAL_SHORTLINK_SPOOL_REPLAY_BATCH_SIZE", 500),
            spool_replay_interval_seconds=_float("EXTERNAL_SHORTLINK_SPOOL_REPLAY_INTERVAL_SECONDS", 1.0, 0.05),
            acknowledged_retention_days=_int("EXTERNAL_SHORTLINK_ACK_RETENTION_DAYS", 7),
            purge_interval_seconds=_float("EXTERNAL_SHORTLINK_PURGE_INTERVAL_SECONDS", 3600.0, 1.0),
        )
