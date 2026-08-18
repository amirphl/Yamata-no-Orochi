from __future__ import annotations

import asyncio
from pathlib import Path

import pytest

from external_shortlink.spool import DurableSpool, SpoolFullError


def _event(number: int) -> dict[str, object]:
    return {
        "event_id": f"00000000-0000-0000-0000-{number:012d}",
        "short_code": "abc1",
        "link_id": 1,
        "long_url": "https://example.com",
        "clicked_at": "2026-08-18T00:00:00Z",
    }


def test_spool_is_durable_across_reopen_and_supports_concurrent_writers(tmp_path: Path) -> None:
    path = tmp_path / "clicks.sqlite3"

    async def scenario() -> None:
        spool = DurableSpool(path, max_bytes=10 * 1024 * 1024, max_events=1000)
        await spool.open()
        await asyncio.gather(*(spool.enqueue(_event(number)) for number in range(100)))
        await spool.close()

        reopened = DurableSpool(path, max_bytes=10 * 1024 * 1024, max_events=1000)
        await reopened.open()
        events = await reopened.peek(100)
        assert len(events) == 100
        await reopened.remove([str(event["event_id"]) for event in events])
        assert (await reopened.stats())[0] == 0
        await reopened.close()

    asyncio.run(scenario())


def test_spool_enforces_event_limit(tmp_path: Path) -> None:
    async def scenario() -> None:
        spool = DurableSpool(tmp_path / "bounded.sqlite3", max_bytes=10 * 1024 * 1024, max_events=1)
        await spool.open()
        try:
            await spool.enqueue(_event(1))
            with pytest.raises(SpoolFullError):
                await spool.enqueue(_event(2))
        finally:
            await spool.close()

    asyncio.run(scenario())

