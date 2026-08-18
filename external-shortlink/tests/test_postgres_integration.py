from __future__ import annotations

import asyncio
import os
import uuid
from datetime import datetime, timezone
from pathlib import Path

import pytest

from external_shortlink.config import Settings
from external_shortlink.database import Database, MappingConflict


DATABASE_URL = os.getenv("EXTERNAL_SHORTLINK_TEST_DATABASE_URL")
pytestmark = pytest.mark.skipif(not DATABASE_URL, reason="EXTERNAL_SHORTLINK_TEST_DATABASE_URL is not configured")


def _settings() -> Settings:
    return Settings(
        database_url=DATABASE_URL or "",
        api_token="x" * 32,
        spool_path=Path("/tmp/not-used.sqlite3"),
        pool_min_size=1,
        pool_max_size=2,
        db_command_timeout_seconds=2,
        click_insert_timeout_seconds=0.1,
        link_lookup_timeout_seconds=0.1,
        cache_max_entries=100,
        cache_preload_entries=0,
        admin_batch_max_links=100,
        click_fetch_max_limit=100,
        max_admin_body_bytes=1024 * 1024,
        spool_max_bytes=1024 * 1024,
        spool_max_events=100,
        spool_replay_batch_size=10,
        spool_replay_interval_seconds=1,
        acknowledged_retention_days=7,
        purge_interval_seconds=3600,
    )


def test_postgres_mapping_click_cursor_and_idempotency() -> None:
    async def scenario() -> None:
        database = Database(_settings())
        assert await database.connect()
        assert database.pool is not None
        async with database.pool.acquire() as connection:
            await connection.execute("TRUNCATE clicks, links RESTART IDENTITY")
            await connection.execute(
                "UPDATE click_acknowledgements SET through_click_id = 0, acknowledged_at = CURRENT_TIMESTAMP WHERE singleton"
            )
        try:
            mapping = {"code": "integration1", "long_url": "https://example.com/a", "campaign_id": 123}
            created, existing, persisted = await database.upload_links([mapping])
            assert (created, existing, len(persisted)) == (1, 0, 1)
            created, existing, _ = await database.upload_links([mapping])
            assert (created, existing) == (0, 1)
            with pytest.raises(MappingConflict):
                await database.upload_links([{"code": "integration1", "long_url": "https://example.com/b"}])

            link = await database.lookup_link("integration1")
            assert link is not None
            first = {
                "event_id": str(uuid.uuid4()),
                "short_code": link["code"],
                "link_id": link["link_id"],
                "long_url": link["long_url"],
                "clicked_at": datetime.now(timezone.utc).isoformat(),
            }
            second = {**first, "event_id": str(uuid.uuid4())}
            await database.insert_click(first)
            await database.insert_click(first)
            await database.insert_spooled_clicks([second])

            clicks, has_more = await database.fetch_clicks(0, 1)
            assert len(clicks) == 1 and has_more
            page_two, has_more = await database.fetch_clicks(clicks[0]["click_id"], 10)
            assert len(page_two) == 1 and not has_more
            assert page_two[0]["click_id"] > clicks[0]["click_id"]
            assert await database.acknowledge(page_two[0]["click_id"]) == page_two[0]["click_id"]
        finally:
            await database.close()

    asyncio.run(scenario())

