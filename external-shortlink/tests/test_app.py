from __future__ import annotations

import asyncio
from pathlib import Path
from typing import Any

import httpx

from external_shortlink.app import create_app
from external_shortlink.config import Settings
from external_shortlink.database import DatabaseUnavailable, MappingConflict


def settings(tmp_path: Path) -> Settings:
    return Settings(
        database_url="postgresql://unused",
        api_token="x" * 32,
        spool_path=tmp_path / "spool.sqlite3",
        pool_min_size=1,
        pool_max_size=2,
        db_command_timeout_seconds=1,
        click_insert_timeout_seconds=0.1,
        link_lookup_timeout_seconds=0.1,
        cache_max_entries=100,
        cache_preload_entries=0,
        admin_batch_max_links=100,
        click_fetch_max_limit=100,
        max_admin_body_bytes=1024 * 1024,
        spool_max_bytes=1024 * 1024,
        spool_max_events=1000,
        spool_replay_batch_size=10,
        spool_replay_interval_seconds=60,
        acknowledged_retention_days=7,
        purge_interval_seconds=3600,
    )


class FakeSpool:
    def __init__(self) -> None:
        self.events: list[dict[str, Any]] = []

    async def open(self) -> None:
        pass

    async def close(self) -> None:
        pass

    async def enqueue(self, event: dict[str, Any]) -> None:
        self.events.append(event)

    async def peek(self, limit: int) -> list[dict[str, Any]]:
        return self.events[:limit]

    async def remove(self, event_ids: list[str]) -> None:
        self.events = [event for event in self.events if event["event_id"] not in event_ids]

    async def stats(self) -> tuple[int, int, float]:
        return len(self.events), 0, 0


class FakeDatabase:
    def __init__(self) -> None:
        self.pool: object | None = object()
        self.links: dict[str, dict[str, Any]] = {
            "abc1": {
                "link_id": 7,
                "code": "abc1",
                "long_url": "https://example.com/destination",
                "short_url": "https://short.example/abc1",
                "source_link_id": 42,
                "campaign_id": 9,
                "client_id": None,
                "scenario_id": None,
                "scenario_name": None,
                "phone_number": None,
                "source_created_at": None,
                "source_updated_at": None,
            }
        }
        self.clicks: list[dict[str, Any]] = []
        self.fail_clicks = False

    async def connect(self) -> bool:
        return True

    async def close(self) -> None:
        pass

    async def ping(self) -> bool:
        return self.pool is not None

    async def preload_links(self, limit: int) -> list[dict[str, Any]]:
        return []

    async def lookup_link(self, code: str) -> dict[str, Any] | None:
        if self.pool is None:
            raise DatabaseUnavailable()
        return self.links.get(code)

    async def insert_click(self, event: dict[str, Any]) -> None:
        if self.fail_clicks:
            raise DatabaseUnavailable()
        self.clicks.append(event)

    async def insert_spooled_clicks(self, events: list[dict[str, Any]]) -> None:
        if self.fail_clicks:
            raise DatabaseUnavailable()
        self.clicks.extend(events)

    async def upload_links(self, links: list[dict[str, Any]]) -> tuple[int, int, list[dict[str, Any]]]:
        persisted = []
        created = 0
        for item in links:
            current = self.links.get(item["code"])
            if current and current["long_url"] != item["long_url"]:
                raise MappingConflict([item["code"]])
            if current is None:
                created += 1
                current = {"link_id": len(self.links) + 1, **item}
                self.links[item["code"]] = current
            persisted.append(current)
        return created, len(links) - created, persisted

    async def fetch_clicks(self, after_id: int, limit: int):
        return [], False

    async def acknowledge(self, through_click_id: int) -> int:
        return through_click_id

    async def purge_acknowledged(self, retention_days: int) -> int:
        return 0

    async def database_size(self) -> int:
        return 1


def auth() -> dict[str, str]:
    return {"Authorization": "Bearer " + "x" * 32}


def test_redirect_is_temporary_and_records_click(tmp_path: Path) -> None:
    database = FakeDatabase()
    spool = FakeSpool()
    app = create_app(settings(tmp_path), database, spool)

    async def scenario():
        async with app.router.lifespan_context(app):
            async with httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://test") as client:
                return await client.get("/abc1", follow_redirects=False, headers={"Referer": "https://ref.example/"})

    response = asyncio.run(scenario())
    assert response.status_code == 302
    assert response.headers["location"] == "https://example.com/destination"
    assert len(database.clicks) == 1
    assert database.clicks[0]["short_code"] == "abc1"
    assert database.clicks[0]["referer"] == "https://ref.example/"
    assert spool.events == []


def test_click_database_failure_uses_durable_spool_and_still_redirects(tmp_path: Path) -> None:
    database = FakeDatabase()
    database.fail_clicks = True
    spool = FakeSpool()
    app = create_app(settings(tmp_path), database, spool)

    async def scenario():
        async with app.router.lifespan_context(app):
            async with httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://test") as client:
                return await client.get("/abc1", follow_redirects=False)

    response = asyncio.run(scenario())
    assert response.status_code == 302
    assert len(spool.events) == 1
    assert spool.events[0]["event_id"]


def test_cached_mapping_redirects_while_postgres_is_unavailable(tmp_path: Path) -> None:
    database = FakeDatabase()
    spool = FakeSpool()
    app = create_app(settings(tmp_path), database, spool)

    async def scenario():
        async with app.router.lifespan_context(app):
            async with httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://test") as client:
                assert (await client.get("/abc1", follow_redirects=False)).status_code == 302
                database.pool = None
                database.fail_clicks = True
                return await client.get("/abc1", follow_redirects=False)

    response = asyncio.run(scenario())
    assert response.status_code == 302
    assert len(spool.events) == 1


def test_unknown_code_is_404(tmp_path: Path) -> None:
    app = create_app(settings(tmp_path), FakeDatabase(), FakeSpool())

    async def scenario():
        async with app.router.lifespan_context(app):
            async with httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://test") as client:
                return await client.get("/missing", follow_redirects=False)

    assert asyncio.run(scenario()).status_code == 404


def test_mapping_api_auth_validation_idempotency_and_conflict(tmp_path: Path) -> None:
    app = create_app(settings(tmp_path), FakeDatabase(), FakeSpool())
    async def scenario():
        async with app.router.lifespan_context(app):
            async with httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://test") as client:
                assert (await client.post("/api/v1/links/batch", json={"links": []})).status_code == 401
                payload = {"links": [{"code": "new1", "long_url": "https://example.org/a", "campaign_id": 1}]}
                first = await client.post("/api/v1/links/batch", json=payload, headers=auth())
                second = await client.post("/api/v1/links/batch", json=payload, headers=auth())
                conflict = await client.post(
                    "/api/v1/links/batch",
                    json={"links": [{"code": "new1", "long_url": "https://example.org/b"}]},
                    headers=auth(),
                )
                invalid = await client.post(
                    "/api/v1/links/batch",
                    json={"links": [{"code": "bad", "long_url": "javascript:alert(1)"}]},
                    headers=auth(),
                )
                return first, second, conflict, invalid

    first, second, conflict, invalid = asyncio.run(scenario())
    assert first.status_code == 200 and first.json()["created"] == 1
    assert second.status_code == 200 and second.json()["existing"] == 1
    assert conflict.status_code == 409
    assert invalid.status_code == 400


def test_reserved_paths_cannot_be_mappings(tmp_path: Path) -> None:
    app = create_app(settings(tmp_path), FakeDatabase(), FakeSpool())
    async def scenario():
        async with app.router.lifespan_context(app):
            async with httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://test") as client:
                response = await client.post(
                    "/api/v1/links/batch",
                    json={"links": [{"code": "healthz", "long_url": "https://example.org"}]},
                    headers=auth(),
                )
                health = await client.get("/healthz")
                return response, health

    response, health = asyncio.run(scenario())
    assert response.status_code == 400
    assert health.status_code == 200


def test_concurrent_redirects_survive_postgres_failure(tmp_path: Path) -> None:
    database = FakeDatabase()
    database.fail_clicks = True
    spool = FakeSpool()
    app = create_app(settings(tmp_path), database, spool)

    async def scenario():
        async with app.router.lifespan_context(app):
            # Prime the worker-local mapping cache before simulating total DB loss.
            async with httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://test") as client:
                assert (await client.get("/abc1", follow_redirects=False)).status_code == 302
                database.pool = None
                responses = await asyncio.gather(
                    *(client.get("/abc1", follow_redirects=False) for _ in range(100))
                )
                return responses

    responses = asyncio.run(scenario())
    assert all(response.status_code == 302 for response in responses)
    assert len(spool.events) == 101
