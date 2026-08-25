from __future__ import annotations

import asyncio
import hmac
import json
import logging
import re
import time
import uuid
from contextlib import asynccontextmanager
from datetime import datetime, timezone
from typing import Any
from urllib.parse import urlsplit

from starlette.applications import Starlette
from starlette.requests import Request
from starlette.responses import JSONResponse, PlainTextResponse, RedirectResponse, Response
from starlette.routing import Route

from . import metrics
from .cache import LinkCache
from .config import Settings
from .database import Database, DatabaseUnavailable, MappingConflict
from .spool import DurableSpool, SpoolFullError


LOGGER = logging.getLogger("external_shortlink")
CODE_RE = re.compile(r"^[A-Za-z0-9_-]{1,64}$")
RESERVED_CODES = frozenset(("api", "healthz", "readyz", "metrics"))


def _utc_iso(value: Any) -> Any:
    if isinstance(value, datetime):
        return value.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")
    return value


def _clean_optional_string(value: Any, name: str, maximum: int) -> str | None:
    if value is None:
        return None
    if not isinstance(value, str):
        raise ValueError(f"{name} must be a string or null")
    value = value.strip()
    if not value:
        return None
    if len(value) > maximum or any(ord(char) < 32 for char in value):
        raise ValueError(f"{name} is invalid or too long")
    return value


def _optional_id(value: Any, name: str) -> int | None:
    if value is None:
        return None
    if isinstance(value, bool) or not isinstance(value, int) or value < 0 or value > 9_223_372_036_854_775_807:
        raise ValueError(f"{name} must be a non-negative 64-bit integer or null")
    return value


def _optional_timestamp(value: Any, name: str) -> datetime | None:
    if value is None or value == "":
        return None
    if not isinstance(value, str):
        raise ValueError(f"{name} must be an RFC3339 timestamp or null")
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as exc:
        raise ValueError(f"{name} must be an RFC3339 timestamp") from exc
    if parsed.tzinfo is None:
        raise ValueError(f"{name} must include a timezone")
    return parsed.astimezone(timezone.utc)


def _validate_link(item: Any) -> dict[str, Any]:
    if not isinstance(item, dict):
        raise ValueError("each link must be an object")
    code = item.get("code")
    if not isinstance(code, str) or not CODE_RE.fullmatch(code) or code.lower() in RESERVED_CODES:
        raise ValueError("code must be 1-64 URL-safe characters and must not be reserved")
    long_url = item.get("long_url")
    if not isinstance(long_url, str) or len(long_url) > 4096 or any(char.isspace() for char in long_url):
        raise ValueError("long_url is invalid or too long")
    parsed = urlsplit(long_url)
    try:
        hostname = parsed.hostname
    except ValueError as exc:
        raise ValueError("long_url contains an invalid host") from exc
    if parsed.scheme.lower() not in ("http", "https") or not parsed.netloc or not hostname:
        raise ValueError("long_url must be an absolute http:// or https:// URL")
    return {
        "code": code,
        "long_url": long_url,
        "short_url": _clean_optional_string(item.get("short_url"), "short_url", 4096),
        "source_link_id": _optional_id(item.get("source_link_id"), "source_link_id"),
        "campaign_id": _optional_id(item.get("campaign_id"), "campaign_id"),
        "client_id": _optional_id(item.get("client_id"), "client_id"),
        "scenario_id": _optional_id(item.get("scenario_id"), "scenario_id"),
        "scenario_name": _clean_optional_string(item.get("scenario_name"), "scenario_name", 512),
        "phone_number": _clean_optional_string(item.get("phone_number"), "phone_number", 32),
        "source_created_at": _optional_timestamp(item.get("source_created_at"), "source_created_at"),
        "source_updated_at": _optional_timestamp(item.get("source_updated_at"), "source_updated_at"),
    }


def _authorized(request: Request) -> bool:
    expected = request.app.state.settings.api_token
    authorization = request.headers.get("authorization", "")
    scheme, _, supplied = authorization.partition(" ")
    return scheme.lower() == "bearer" and bool(supplied) and hmac.compare_digest(supplied, expected)


async def _admin_json(request: Request) -> tuple[dict[str, Any] | None, Response | None]:
    if not _authorized(request):
        return None, JSONResponse({"error": "unauthorized"}, status_code=401, headers={"WWW-Authenticate": "Bearer"})
    settings: Settings = request.app.state.settings
    content_length = request.headers.get("content-length")
    if content_length:
        try:
            if int(content_length) > settings.max_admin_body_bytes:
                return None, JSONResponse({"error": "request body too large"}, status_code=413)
        except ValueError:
            return None, JSONResponse({"error": "invalid content-length"}, status_code=400)
    body = await request.body()
    if len(body) > settings.max_admin_body_bytes:
        return None, JSONResponse({"error": "request body too large"}, status_code=413)
    try:
        decoded = json.loads(body)
    except (UnicodeDecodeError, json.JSONDecodeError):
        return None, JSONResponse({"error": "invalid JSON"}, status_code=400)
    if not isinstance(decoded, dict):
        return None, JSONResponse({"error": "JSON body must be an object"}, status_code=400)
    return decoded, None


async def healthz(_: Request) -> Response:
    return JSONResponse({"status": "ok"})


async def readyz(request: Request) -> Response:
    database: Database = request.app.state.database
    ready = await database.ping()
    return JSONResponse({"status": "ready" if ready else "not_ready", "postgres": ready}, status_code=200 if ready else 503)


async def prometheus_metrics(_: Request) -> Response:
    payload, content_type = metrics.render()
    return Response(payload, headers={"Content-Type": content_type})


def _click_event(link: dict[str, Any], request: Request) -> dict[str, Any]:
    client_ip = request.client.host if request.client is not None else None
    return {
        "event_id": str(uuid.uuid4()),
        "short_code": link["code"],
        "link_id": link["link_id"],
        "long_url": link["long_url"],
        "short_url": link.get("short_url"),
        "source_link_id": link.get("source_link_id"),
        "campaign_id": link.get("campaign_id"),
        "client_id": link.get("client_id"),
        "scenario_id": link.get("scenario_id"),
        "scenario_name": link.get("scenario_name"),
        "phone_number": link.get("phone_number"),
        "link_created_at": _utc_iso(link.get("source_created_at")),
        "link_updated_at": _utc_iso(link.get("source_updated_at")),
        "clicked_at": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        "client_ip": client_ip[:64] if client_ip else None,
        "user_agent": request.headers.get("user-agent", "")[:1024] or None,
        "referer": request.headers.get("referer", "")[:2048] or None,
    }


async def redirect(request: Request) -> Response:
    started = time.perf_counter()
    status = "500"
    try:
        code = request.path_params["code"]
        if not CODE_RE.fullmatch(code) or code.lower() in RESERVED_CODES:
            status = "404"
            metrics.UNKNOWN_CODES.inc()
            return PlainTextResponse("not found", status_code=404)
        cache: LinkCache = request.app.state.cache
        link = cache.get(code)
        if link is None:
            metrics.CACHE_LOOKUPS.labels("miss").inc()
            try:
                link = await asyncio.wait_for(
                    request.app.state.database.lookup_link(code),
                    timeout=request.app.state.settings.link_lookup_timeout_seconds,
                )
            except (asyncio.TimeoutError, DatabaseUnavailable):
                metrics.POOL_TIMEOUTS.inc()
                status = "503"
                return PlainTextResponse("temporarily unavailable", status_code=503)
            except Exception:
                metrics.POSTGRES_ERRORS.labels("lookup_link").inc()
                status = "503"
                return PlainTextResponse("temporarily unavailable", status_code=503)
            if link is None:
                metrics.UNKNOWN_CODES.inc()
                status = "404"
                return PlainTextResponse("not found", status_code=404)
            cache.put(link)
        else:
            metrics.CACHE_LOOKUPS.labels("hit").inc()

        event = _click_event(link, request)
        try:
            await asyncio.wait_for(
                request.app.state.database.insert_click(event),
                timeout=request.app.state.settings.click_insert_timeout_seconds,
            )
        except Exception as exc:
            if isinstance(exc, asyncio.TimeoutError):
                metrics.POOL_TIMEOUTS.inc()
            else:
                metrics.POSTGRES_ERRORS.labels("insert_click").inc()
            try:
                await request.app.state.spool.enqueue(event)
            except SpoolFullError:
                metrics.SPOOL_DROPPED.labels("capacity").inc()
                LOGGER.critical("click spool capacity exhausted", extra={"event_id": event["event_id"]})
            except Exception:
                metrics.SPOOL_DROPPED.labels("write_error").inc()
                LOGGER.exception("click could not be written to durable spool", extra={"event_id": event["event_id"]})
        status = "302"
        return RedirectResponse(link["long_url"], status_code=302)
    finally:
        metrics.REDIRECTS.labels(status).inc()
        metrics.REDIRECT_LATENCY.observe(time.perf_counter() - started)


async def upload_links(request: Request) -> Response:
    body, error = await _admin_json(request)
    if error is not None:
        metrics.MAPPING_UPLOADS.labels("auth_or_validation_error").inc()
        return error
    raw_links = body.get("links")
    settings: Settings = request.app.state.settings
    if not isinstance(raw_links, list) or not raw_links or len(raw_links) > settings.admin_batch_max_links:
        metrics.MAPPING_UPLOADS.labels("validation_error").inc()
        return JSONResponse({"error": f"links must contain 1-{settings.admin_batch_max_links} items"}, status_code=400)
    try:
        links = [_validate_link(item) for item in raw_links]
        unique: dict[str, dict[str, Any]] = {}
        for item in links:
            prior = unique.get(item["code"])
            if prior is not None and prior["long_url"] != item["long_url"]:
                raise ValueError(f"code {item['code']!r} appears with different destinations")
            unique[item["code"]] = item
        links = list(unique.values())
    except ValueError as exc:
        metrics.MAPPING_UPLOADS.labels("validation_error").inc()
        return JSONResponse({"error": str(exc)}, status_code=400)
    try:
        created, existing, persisted = await request.app.state.database.upload_links(links)
    except MappingConflict as exc:
        metrics.MAPPING_UPLOADS.labels("conflict").inc()
        return JSONResponse({"error": str(exc), "conflicting_codes": exc.codes}, status_code=409)
    except Exception:
        metrics.POSTGRES_ERRORS.labels("upload_links").inc()
        metrics.MAPPING_UPLOADS.labels("database_error").inc()
        LOGGER.exception("mapping upload failed")
        return JSONResponse({"error": "mapping persistence failed"}, status_code=503)
    for link in persisted:
        request.app.state.cache.put(link)
    metrics.MAPPING_UPLOADS.labels("success").inc()
    metrics.LAST_MAPPING_UPLOAD.set(time.time())
    return JSONResponse({"persisted": len(persisted), "created": created, "existing": existing})


async def fetch_clicks(request: Request) -> Response:
    if not _authorized(request):
        return JSONResponse({"error": "unauthorized"}, status_code=401, headers={"WWW-Authenticate": "Bearer"})
    try:
        after_id = int(request.query_params.get("after_id", "0"))
        limit = int(request.query_params.get("limit", "10000"))
    except ValueError:
        return JSONResponse({"error": "after_id and limit must be integers"}, status_code=400)
    settings: Settings = request.app.state.settings
    if after_id < 0 or limit < 1 or limit > settings.click_fetch_max_limit:
        return JSONResponse({"error": f"after_id must be non-negative and limit must be 1-{settings.click_fetch_max_limit}"}, status_code=400)
    try:
        clicks, has_more = await request.app.state.database.fetch_clicks(after_id, limit)
    except Exception:
        metrics.POSTGRES_ERRORS.labels("fetch_clicks").inc()
        return JSONResponse({"error": "click fetch failed"}, status_code=503)
    serializable = [{key: _utc_iso(value) for key, value in click.items()} for click in clicks]
    next_after_id = serializable[-1]["click_id"] if serializable else after_id
    return JSONResponse({"clicks": serializable, "next_after_id": next_after_id, "has_more": has_more})


async def acknowledge_clicks(request: Request) -> Response:
    body, error = await _admin_json(request)
    if error is not None:
        return error
    through = body.get("through_click_id")
    if isinstance(through, bool) or not isinstance(through, int) or through < 0:
        return JSONResponse({"error": "through_click_id must be a non-negative integer"}, status_code=400)
    try:
        acknowledged = await request.app.state.database.acknowledge(through)
    except ValueError as exc:
        return JSONResponse({"error": str(exc)}, status_code=409)
    except Exception:
        metrics.POSTGRES_ERRORS.labels("acknowledge").inc()
        return JSONResponse({"error": "acknowledgement failed"}, status_code=503)
    metrics.LAST_ACK_ID.set(acknowledged)
    return JSONResponse({"through_click_id": acknowledged})


async def _reconnect_loop(app: Starlette) -> None:
    while True:
        try:
            if app.state.database.pool is None:
                connected = await app.state.database.connect()
                if connected:
                    for item in await app.state.database.preload_links(app.state.settings.cache_preload_entries):
                        app.state.cache.put(item)
            await asyncio.sleep(5.0)
        except asyncio.CancelledError:
            raise
        except Exception:
            LOGGER.exception("PostgreSQL reconnect/preload failed")
            await asyncio.sleep(5.0)


async def _spool_replay_loop(app: Starlette) -> None:
    settings: Settings = app.state.settings
    while True:
        try:
            events = await app.state.spool.peek(settings.spool_replay_batch_size)
            if events and app.state.database.pool is not None:
                await app.state.database.insert_spooled_clicks(events)
                await app.state.spool.remove([event["event_id"] for event in events])
            count, size, age = await app.state.spool.stats()
            metrics.SPOOL_EVENTS.set(count)
            metrics.SPOOL_BYTES.set(size)
            metrics.SPOOL_OLDEST_AGE.set(age)
            if app.state.database.pool is not None:
                metrics.DATABASE_BYTES.set(await app.state.database.database_size())
        except asyncio.CancelledError:
            raise
        except Exception:
            metrics.POSTGRES_ERRORS.labels("spool_replay").inc()
            LOGGER.exception("durable click spool replay failed")
        await asyncio.sleep(settings.spool_replay_interval_seconds)


async def _purge_loop(app: Starlette) -> None:
    settings: Settings = app.state.settings
    while True:
        try:
            await asyncio.sleep(settings.purge_interval_seconds)
            if app.state.database.pool is not None:
                purged = await app.state.database.purge_acknowledged(settings.acknowledged_retention_days)
                if purged:
                    LOGGER.info("purged acknowledged clicks", extra={"count": purged})
        except asyncio.CancelledError:
            raise
        except Exception:
            metrics.POSTGRES_ERRORS.labels("purge").inc()
            LOGGER.exception("acknowledged-click purge failed")


def create_app(
    settings: Settings | None = None,
    database: Database | None = None,
    spool: DurableSpool | None = None,
) -> Starlette:
    @asynccontextmanager
    async def lifespan(app: Starlette):
        resolved = settings or Settings.from_env()
        app.state.settings = resolved
        app.state.database = database or Database(resolved)
        app.state.spool = spool or DurableSpool(resolved.spool_path, resolved.spool_max_bytes, resolved.spool_max_events)
        app.state.cache = LinkCache(resolved.cache_max_entries)
        await app.state.spool.open()
        if await app.state.database.connect():
            try:
                for item in await app.state.database.preload_links(resolved.cache_preload_entries):
                    app.state.cache.put(item)
            except Exception:
                metrics.POSTGRES_ERRORS.labels("preload").inc()
                LOGGER.exception("link cache preload failed")
        tasks = [
            asyncio.create_task(_reconnect_loop(app), name="postgres-reconnect"),
            asyncio.create_task(_spool_replay_loop(app), name="spool-replay"),
            asyncio.create_task(_purge_loop(app), name="click-purge"),
        ]
        try:
            yield
        finally:
            for task in tasks:
                task.cancel()
            await asyncio.gather(*tasks, return_exceptions=True)
            await app.state.database.close()
            await app.state.spool.close()

    return Starlette(
        debug=False,
        lifespan=lifespan,
        routes=[
            Route("/healthz", healthz, methods=["GET"]),
            Route("/readyz", readyz, methods=["GET"]),
            Route("/metrics", prometheus_metrics, methods=["GET"]),
            Route("/api/v1/links/batch", upload_links, methods=["POST"]),
            Route("/api/v1/clicks", fetch_clicks, methods=["GET"]),
            Route("/api/v1/clicks/ack", acknowledge_clicks, methods=["POST"]),
            Route("/{code:str}", redirect, methods=["GET"]),
        ],
    )


app = create_app()
