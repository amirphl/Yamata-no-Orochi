from __future__ import annotations

import asyncio
import logging
from datetime import datetime, timedelta, timezone
from typing import Any

import asyncpg

from .config import Settings
from . import metrics


LOGGER = logging.getLogger("external_shortlink.database")


LINK_COLUMNS = (
    "link_id",
    "code",
    "long_url",
    "short_url",
    "source_link_id",
    "campaign_id",
    "client_id",
    "scenario_id",
    "scenario_name",
    "phone_number",
    "source_created_at",
    "source_updated_at",
)


def _as_datetime(value: Any) -> datetime | None:
    if value is None or isinstance(value, datetime):
        return value
    if isinstance(value, str):
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
        if parsed.tzinfo is None:
            raise ValueError("spooled timestamps must include a timezone")
        return parsed.astimezone(timezone.utc)
    raise TypeError("timestamp must be datetime, RFC3339 string, or null")


class DatabaseUnavailable(RuntimeError):
    pass


class MappingConflict(RuntimeError):
    def __init__(self, codes: list[str]) -> None:
        super().__init__("one or more codes already map to another destination")
        self.codes = codes


class Database:
    def __init__(self, settings: Settings) -> None:
        self.settings = settings
        self.pool: asyncpg.Pool | None = None
        self._connect_lock = asyncio.Lock()

    async def connect(self) -> bool:
        if self.pool is not None:
            return True
        async with self._connect_lock:
            if self.pool is not None:
                return True
            try:
                self.pool = await asyncpg.create_pool(
                    dsn=self.settings.database_url,
                    min_size=self.settings.pool_min_size,
                    max_size=self.settings.pool_max_size,
                    command_timeout=self.settings.db_command_timeout_seconds,
                    max_inactive_connection_lifetime=300.0,
                    timeout=min(self.settings.db_command_timeout_seconds, 10.0),
                )
            except Exception as exc:
                metrics.POSTGRES_ERRORS.labels("connect").inc()
                LOGGER.error("PostgreSQL pool connection failed: %s", type(exc).__name__)
                self.pool = None
                return False
        return True

    async def close(self) -> None:
        pool, self.pool = self.pool, None
        if pool is not None:
            await pool.close()

    def _pool(self) -> asyncpg.Pool:
        if self.pool is None:
            raise DatabaseUnavailable("PostgreSQL pool is unavailable")
        return self.pool

    async def ping(self) -> bool:
        try:
            return bool(await self._pool().fetchval("SELECT TRUE"))
        except Exception:
            metrics.POSTGRES_ERRORS.labels("ping").inc()
            return False

    async def lookup_link(self, code: str) -> dict[str, Any] | None:
        row = await self._pool().fetchrow(
            f"SELECT {', '.join(LINK_COLUMNS)} FROM links WHERE code = $1", code
        )
        return dict(row) if row is not None else None

    async def preload_links(self, limit: int) -> list[dict[str, Any]]:
        if limit <= 0:
            return []
        rows = await self._pool().fetch(
            f"SELECT {', '.join(LINK_COLUMNS)} FROM links ORDER BY link_id DESC LIMIT $1", limit
        )
        return [dict(row) for row in rows]

    async def insert_click(self, event: dict[str, Any]) -> None:
        await self._pool().execute(
            """
            INSERT INTO clicks (
                event_id, short_code, link_id, long_url, short_url,
                source_link_id, campaign_id, client_id, scenario_id,
                scenario_name, phone_number, link_created_at, link_updated_at,
                clicked_at, client_ip, user_agent, referer
            ) VALUES (
                $1::uuid, $2, $3, $4, $5,
                $6, $7, $8, $9,
                $10, $11, $12::timestamptz, $13::timestamptz,
                $14::timestamptz, $15, $16, $17
            )
            ON CONFLICT (event_id) DO NOTHING
            """,
            event["event_id"],
            event["short_code"],
            event["link_id"],
            event["long_url"],
            event.get("short_url"),
            event.get("source_link_id"),
            event.get("campaign_id"),
            event.get("client_id"),
            event.get("scenario_id"),
            event.get("scenario_name"),
            event.get("phone_number"),
            _as_datetime(event.get("link_created_at")),
            _as_datetime(event.get("link_updated_at")),
            _as_datetime(event["clicked_at"]),
            event.get("client_ip"),
            event.get("user_agent"),
            event.get("referer"),
        )

    async def insert_spooled_clicks(self, events: list[dict[str, Any]]) -> None:
        if not events:
            return
        async with self._pool().acquire() as connection:
            async with connection.transaction():
                await connection.executemany(
                    """
                    INSERT INTO clicks (
                        event_id, short_code, link_id, long_url, short_url,
                        source_link_id, campaign_id, client_id, scenario_id,
                        scenario_name, phone_number, link_created_at, link_updated_at,
                        clicked_at, client_ip, user_agent, referer
                    ) VALUES (
                        $1::uuid, $2, $3, $4, $5,
                        $6, $7, $8, $9,
                        $10, $11, $12::timestamptz, $13::timestamptz,
                        $14::timestamptz, $15, $16, $17
                    )
                    ON CONFLICT (event_id) DO NOTHING
                    """,
                    [
                        (
                            event["event_id"], event["short_code"], event["link_id"], event["long_url"],
                            event.get("short_url"), event.get("source_link_id"), event.get("campaign_id"),
                            event.get("client_id"), event.get("scenario_id"), event.get("scenario_name"),
                            event.get("phone_number"), _as_datetime(event.get("link_created_at")),
                            _as_datetime(event.get("link_updated_at")), _as_datetime(event["clicked_at"]),
                            event.get("client_ip"), event.get("user_agent"), event.get("referer"),
                        )
                        for event in events
                    ],
                )

    async def upload_links(self, links: list[dict[str, Any]]) -> tuple[int, int, list[dict[str, Any]]]:
        pool = self._pool()
        async with pool.acquire() as connection:
            async with connection.transaction():
                await connection.execute(
                    """
                    CREATE TEMP TABLE incoming_links (
                        code VARCHAR(64), long_url VARCHAR(4096), short_url VARCHAR(4096),
                        source_link_id BIGINT, campaign_id BIGINT, client_id BIGINT,
                        scenario_id BIGINT, scenario_name VARCHAR(512), phone_number VARCHAR(32),
                        source_created_at TIMESTAMPTZ, source_updated_at TIMESTAMPTZ
                    ) ON COMMIT DROP
                    """
                )
                await connection.copy_records_to_table(
                    "incoming_links",
                    records=[
                        (
                            item["code"], item["long_url"], item.get("short_url"), item.get("source_link_id"),
                            item.get("campaign_id"), item.get("client_id"), item.get("scenario_id"),
                            item.get("scenario_name"), item.get("phone_number"), item.get("source_created_at"),
                            item.get("source_updated_at"),
                        )
                        for item in links
                    ],
                    columns=(
                        "code", "long_url", "short_url", "source_link_id", "campaign_id", "client_id",
                        "scenario_id", "scenario_name", "phone_number", "source_created_at", "source_updated_at",
                    ),
                )
                conflicts = await connection.fetch(
                    """
                    SELECT incoming.code
                    FROM incoming_links AS incoming
                    JOIN links AS existing USING (code)
                    WHERE existing.long_url <> incoming.long_url
                    ORDER BY incoming.code
                    LIMIT 100
                    """
                )
                if conflicts:
                    raise MappingConflict([row["code"] for row in conflicts])
                inserted_rows = await connection.fetch(
                    """
                    INSERT INTO links (
                        code, long_url, short_url, source_link_id, campaign_id,
                        client_id, scenario_id, scenario_name, phone_number,
                        source_created_at, source_updated_at
                    )
                    SELECT code, long_url, short_url, source_link_id, campaign_id,
                           client_id, scenario_id, scenario_name, phone_number,
                           source_created_at, source_updated_at
                    FROM incoming_links
                    ON CONFLICT (code) DO NOTHING
                    RETURNING code
                    """
                )
                # Destination mappings are immutable, but optional source
                # metadata may be backfilled or refreshed by an idempotent
                # production retry using the same code and destination.
                await connection.execute(
                    """
                    UPDATE links AS existing
                    SET short_url = COALESCE(incoming.short_url, existing.short_url),
                        source_link_id = COALESCE(incoming.source_link_id, existing.source_link_id),
                        campaign_id = COALESCE(incoming.campaign_id, existing.campaign_id),
                        client_id = COALESCE(incoming.client_id, existing.client_id),
                        scenario_id = COALESCE(incoming.scenario_id, existing.scenario_id),
                        scenario_name = COALESCE(incoming.scenario_name, existing.scenario_name),
                        phone_number = COALESCE(incoming.phone_number, existing.phone_number),
                        source_created_at = COALESCE(incoming.source_created_at, existing.source_created_at),
                        source_updated_at = COALESCE(incoming.source_updated_at, existing.source_updated_at)
                    FROM incoming_links AS incoming
                    WHERE existing.code = incoming.code
                      AND existing.long_url = incoming.long_url
                    """
                )
                persisted = await connection.fetch(
                    f"""
                    SELECT {', '.join('links.' + column for column in LINK_COLUMNS)}
                    FROM links JOIN incoming_links USING (code)
                    ORDER BY links.link_id
                    """
                )
        created = len(inserted_rows)
        return created, len(links) - created, [dict(row) for row in persisted]

    async def fetch_clicks(self, after_id: int, limit: int) -> tuple[list[dict[str, Any]], bool]:
        rows = await self._pool().fetch(
            """
            SELECT click_id, event_id::text AS event_id, short_code, link_id, long_url, short_url,
                   source_link_id, campaign_id, client_id, scenario_id, scenario_name,
                   phone_number, link_created_at, link_updated_at, clicked_at,
                   client_ip, user_agent, referer
            FROM clicks
            WHERE click_id > $1
            ORDER BY click_id ASC
            LIMIT $2
            """,
            after_id,
            limit + 1,
        )
        has_more = len(rows) > limit
        result = [dict(row) for row in rows[:limit]]
        return result, has_more

    async def acknowledge(self, through_click_id: int) -> int:
        async with self._pool().acquire() as connection:
            async with connection.transaction():
                current = int(
                    await connection.fetchval(
                        """
                        SELECT through_click_id
                        FROM click_acknowledgements
                        WHERE singleton = TRUE
                        FOR UPDATE
                        """
                    )
                )
                if through_click_id <= current:
                    return current

                exists = bool(
                    await connection.fetchval(
                        "SELECT EXISTS (SELECT 1 FROM clicks WHERE click_id = $1)", through_click_id
                    )
                )
                if not exists:
                    raise ValueError("through_click_id is not a persisted click_id")

                await connection.execute(
                    """
                    UPDATE clicks
                    SET acknowledged_at = COALESCE(acknowledged_at, CURRENT_TIMESTAMP)
                    WHERE click_id > $1 AND click_id <= $2
                    """,
                    current,
                    through_click_id,
                )
                return int(
                    await connection.fetchval(
                        """
                        UPDATE click_acknowledgements
                        SET through_click_id = $1,
                            acknowledged_at = CURRENT_TIMESTAMP
                        WHERE singleton = TRUE
                        RETURNING through_click_id
                        """,
                        through_click_id,
                    )
                )

    async def purge_acknowledged(self, retention_days: int) -> int:
        cutoff = datetime.now(timezone.utc) - timedelta(days=retention_days)
        result = await self._pool().execute(
            """
            DELETE FROM clicks
            WHERE acknowledged_at < $1
            """,
            cutoff,
        )
        return int(result.rsplit(" ", 1)[-1])

    async def database_size(self) -> int:
        return int(await self._pool().fetchval("SELECT pg_database_size(current_database())"))
