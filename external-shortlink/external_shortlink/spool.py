from __future__ import annotations

import asyncio
import json
import os
import sqlite3
import threading
import time
from pathlib import Path
from typing import Any


class SpoolFullError(RuntimeError):
    pass


class DurableSpool:
    """A process-safe SQLite WAL spool; calls are moved off the event loop."""

    def __init__(self, path: Path, max_bytes: int, max_events: int) -> None:
        self.path = path
        self.max_bytes = max_bytes
        self.max_events = max_events
        self._connection: sqlite3.Connection | None = None
        self._lock = threading.Lock()
        self._approximate_count = 0
        self._approximate_bytes = 0
        self._writes_since_refresh = 0

    async def open(self) -> None:
        await asyncio.to_thread(self._open)

    def _open(self) -> None:
        self.path.parent.mkdir(mode=0o750, parents=True, exist_ok=True)
        connection = sqlite3.connect(self.path, timeout=2.0, check_same_thread=False, isolation_level=None)
        connection.execute("PRAGMA journal_mode=WAL")
        connection.execute("PRAGMA synchronous=FULL")
        connection.execute("PRAGMA busy_timeout=2000")
        connection.execute(
            """
            CREATE TABLE IF NOT EXISTS click_spool (
                event_id TEXT PRIMARY KEY,
                payload TEXT NOT NULL,
                created_unix REAL NOT NULL
            )
            """
        )
        connection.execute("CREATE INDEX IF NOT EXISTS idx_click_spool_created ON click_spool(created_unix)")
        self._connection = connection
        self._refresh_capacity_unlocked(connection)

    async def close(self) -> None:
        await asyncio.to_thread(self._close)

    def _close(self) -> None:
        with self._lock:
            if self._connection is not None:
                self._connection.close()
                self._connection = None

    def _conn(self) -> sqlite3.Connection:
        if self._connection is None:
            raise RuntimeError("spool is not open")
        return self._connection

    async def enqueue(self, event: dict[str, Any]) -> None:
        payload = json.dumps(event, ensure_ascii=True, separators=(",", ":"))
        await asyncio.to_thread(self._enqueue, event["event_id"], payload)

    def _enqueue(self, event_id: str, payload: str) -> None:
        with self._lock:
            connection = self._conn()
            if self._writes_since_refresh >= 100:
                self._refresh_capacity_unlocked(connection)
            if self._approximate_count >= self.max_events:
                raise SpoolFullError("event limit reached")
            payload_bytes = len(payload.encode("utf-8"))
            if self._approximate_bytes + payload_bytes > self.max_bytes:
                raise SpoolFullError("byte limit reached")
            cursor = connection.execute(
                "INSERT OR IGNORE INTO click_spool(event_id, payload, created_unix) VALUES (?, ?, ?)",
                (event_id, payload, time.time()),
            )
            if cursor.rowcount:
                self._approximate_count += 1
                self._approximate_bytes += payload_bytes
                self._writes_since_refresh += 1

    async def peek(self, limit: int) -> list[dict[str, Any]]:
        return await asyncio.to_thread(self._peek, limit)

    def _peek(self, limit: int) -> list[dict[str, Any]]:
        with self._lock:
            rows = self._conn().execute(
                "SELECT payload FROM click_spool ORDER BY created_unix, event_id LIMIT ?", (limit,)
            ).fetchall()
        return [json.loads(row[0]) for row in rows]

    async def remove(self, event_ids: list[str]) -> None:
        if not event_ids:
            return
        await asyncio.to_thread(self._remove, event_ids)

    def _remove(self, event_ids: list[str]) -> None:
        placeholders = ",".join("?" for _ in event_ids)
        with self._lock:
            connection = self._conn()
            connection.execute(f"DELETE FROM click_spool WHERE event_id IN ({placeholders})", event_ids)
            self._refresh_capacity_unlocked(connection)

    async def stats(self) -> tuple[int, int, float]:
        return await asyncio.to_thread(self._stats)

    def _stats(self) -> tuple[int, int, float]:
        with self._lock:
            connection = self._conn()
            count, oldest = connection.execute("SELECT COUNT(*), MIN(created_unix) FROM click_spool").fetchone()
            size = self._size_unlocked(connection)
            self._approximate_count = int(count)
            self._approximate_bytes = size
            self._writes_since_refresh = 0
        age = max(0.0, time.time() - oldest) if oldest is not None else 0.0
        return int(count), size, age

    def _size_unlocked(self, connection: sqlite3.Connection) -> int:
        return sum(
            os.path.getsize(candidate)
            for candidate in (self.path, Path(str(self.path) + "-wal"), Path(str(self.path) + "-shm"))
            if candidate.exists()
        )

    def _refresh_capacity_unlocked(self, connection: sqlite3.Connection) -> None:
        self._approximate_count = int(connection.execute("SELECT COUNT(*) FROM click_spool").fetchone()[0])
        self._approximate_bytes = self._size_unlocked(connection)
        self._writes_since_refresh = 0
