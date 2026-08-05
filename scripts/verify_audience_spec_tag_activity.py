#!/usr/bin/env python3
"""Verify Redis audience-spec tags against the file export and PostgreSQL.

This script is read-only with respect to Redis and PostgreSQL. It exits with:

* 0 when every Redis tag is active in PostgreSQL;
* 2 when verification completes but discrepancies are found;
* 1 when inputs, Redis, or PostgreSQL cannot be read safely.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from collections import defaultdict
from collections.abc import Iterable, Mapping, Sequence
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from rebuild_audience_spec_cache import (
    CACHE_TTL_SECONDS,
    PLATFORMS,
    STAT_FIELDS,
    redis_key,
)
from script_common import read_secret


class VerificationError(RuntimeError):
    """Raised when verification cannot be completed reliably."""


def _env_int(name: str, default: int | None = None) -> int | None:
    raw = os.getenv(name)
    if raw is None or not raw.strip():
        return default
    try:
        value = int(raw)
    except ValueError as exc:
        raise VerificationError(f"environment variable {name} must be an integer") from exc
    if value < 0:
        raise VerificationError(
            f"environment variable {name} must be greater than or equal to zero"
        )
    return value


def _load_tags_file(path: Path) -> dict[int, dict[str, Any]]:
    try:
        with path.open("r", encoding="utf-8") as handle:
            rows = json.load(handle)
    except FileNotFoundError as exc:
        raise VerificationError(f"tags file does not exist: {path}") from exc
    except json.JSONDecodeError as exc:
        raise VerificationError(
            f"tags file is invalid JSON ({path}:{exc.lineno}:{exc.colno}): {exc.msg}"
        ) from exc
    except OSError as exc:
        raise VerificationError(f"cannot read tags file {path}: {exc}") from exc

    if not isinstance(rows, list):
        raise VerificationError("tags file must contain a top-level JSON list")

    indexed: dict[int, dict[str, Any]] = {}
    for index, row in enumerate(rows):
        if not isinstance(row, dict):
            raise VerificationError(f"tags row {index} must be a JSON object")
        tag_id = row.get("id")
        if isinstance(tag_id, bool) or not isinstance(tag_id, int) or tag_id <= 0:
            raise VerificationError(f"tags row {index} has an invalid id")
        if tag_id in indexed:
            raise VerificationError(f"tags file contains duplicate id {tag_id}")
        if type(row.get("is_active")) is not bool:
            raise VerificationError(
                f"tags row {index} field 'is_active' must be a JSON boolean"
            )
        indexed[tag_id] = row
    return indexed


def _parse_tag_id(value: Any, location: str) -> int:
    if isinstance(value, bool):
        raise VerificationError(f"Redis tag at {location} must be a positive integer ID")
    if isinstance(value, int):
        tag_id = value
    elif isinstance(value, str) and value.strip().isdigit():
        tag_id = int(value.strip())
    else:
        raise VerificationError(
            f"Redis tag {value!r} at {location} must be a positive integer ID"
        )
    if tag_id <= 0:
        raise VerificationError(f"Redis tag at {location} must be positive")
    return tag_id


def extract_redis_tag_locations(
    spec: Mapping[str, Any], platform: str
) -> tuple[dict[int, list[str]], int]:
    """Return tag ID -> hierarchy paths and the number of leaves inspected."""
    locations: dict[int, list[str]] = defaultdict(list)
    leaf_count = 0
    for level1, level2_map in spec.items():
        if not isinstance(level1, str) or not isinstance(level2_map, dict):
            raise VerificationError(f"invalid level1 node in Redis platform {platform}")
        for level2, node in level2_map.items():
            if not isinstance(level2, str) or not isinstance(node, dict):
                raise VerificationError(
                    f"invalid level2 node at {platform} / {level1} / {level2}"
                )
            items = node.get("items", {})
            if not isinstance(items, dict):
                raise VerificationError(
                    f"items must be an object at {platform} / {level1} / {level2}"
                )
            for level3, leaf in items.items():
                path = f"{platform} / {level1} / {level2} / {level3}"
                if not isinstance(level3, str) or not isinstance(leaf, dict):
                    raise VerificationError(f"invalid level3 leaf at {path}")
                tags = leaf.get("tags")
                if not isinstance(tags, list) or not tags:
                    raise VerificationError(f"tags must be a non-empty list at {path}")
                for field in ("available_audience", *STAT_FIELDS):
                    value = leaf.get(field)
                    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
                        raise VerificationError(
                            f"{field} must be a non-negative integer at {path}"
                        )
                if platform == "sms":
                    expected_capacity = leaf["white_users"] + leaf["pink_users"] // 3
                else:
                    expected_capacity = (
                        leaf["black_users"] + leaf["white_users"] + leaf["pink_users"]
                    )
                if leaf["available_audience"] != expected_capacity:
                    raise VerificationError(
                        f"available_audience does not match {platform} statistics at {path}"
                    )
                leaf_count += 1
                seen_in_leaf: set[int] = set()
                for raw_tag_id in tags:
                    tag_id = _parse_tag_id(raw_tag_id, path)
                    if tag_id in seen_in_leaf:
                        raise VerificationError(f"duplicate tag {tag_id} at {path}")
                    seen_in_leaf.add(tag_id)
                    locations[tag_id].append(path)
    if leaf_count == 0:
        raise VerificationError(f"Redis audience spec for {platform} contains no leaves")
    return dict(locations), leaf_count


def load_redis_specs(
    redis_url: str,
    redis_db: int | None,
    prefix: str,
    platforms: Iterable[str],
) -> tuple[dict[str, Mapping[str, Any]], dict[str, str]]:
    try:
        import redis  # type: ignore[import-not-found]
    except ImportError as exc:
        raise VerificationError(
            "the 'redis' package is required; run "
            "'python3 -m pip install -r scripts/requirements-audience-spec.txt'"
        ) from exc

    options: dict[str, Any] = {
        "decode_responses": False,
        "socket_connect_timeout": 10,
        "socket_timeout": 30,
        "health_check_interval": 30,
    }
    if redis_db is not None:
        options["db"] = redis_db
    try:
        client = redis.Redis.from_url(redis_url, **options)
    except (redis.exceptions.RedisError, ValueError) as exc:
        raise VerificationError("invalid or unusable Redis URL") from exc

    specs: dict[str, Mapping[str, Any]] = {}
    keys: dict[str, str] = {}
    try:
        client.ping()
        for platform in platforms:
            key = redis_key(prefix, platform)
            payload = client.get(key)
            if payload is None:
                raise VerificationError(f"Redis key does not exist: {key}")
            try:
                decoded = json.loads(payload)
            except (json.JSONDecodeError, UnicodeDecodeError) as exc:
                raise VerificationError(f"Redis key {key} does not contain valid JSON") from exc
            if not isinstance(decoded, dict):
                raise VerificationError(f"Redis key {key} must contain a JSON object")
            ttl = client.ttl(key)
            if ttl <= 0 or ttl > CACHE_TTL_SECONDS:
                raise VerificationError(
                    f"Redis key {key} has invalid TTL {ttl}; expected 1..{CACHE_TTL_SECONDS} seconds"
                )
            specs[platform] = decoded
            keys[platform] = key
    except redis.exceptions.RedisError as exc:
        raise VerificationError(f"Redis read failed: {exc}") from exc
    finally:
        try:
            client.close()
        except redis.exceptions.RedisError:
            pass
    return specs, keys


def load_database_tags(
    tag_ids: Sequence[int], args: argparse.Namespace
) -> dict[int, dict[str, Any]]:
    try:
        import psycopg2  # type: ignore[import-not-found]
    except ImportError as exc:
        raise VerificationError(
            "the 'psycopg2' package is required; run "
            "'python3 -m pip install -r scripts/requirements-audience-spec.txt'"
        ) from exc

    try:
        connection = psycopg2.connect(
            host=args.db_host,
            port=args.db_port,
            dbname=args.db_name,
            user=args.db_user,
            password=args.db_password,
            sslmode=args.db_sslmode,
            connect_timeout=10,
            application_name="verify_audience_spec_tag_activity",
        )
        connection.set_session(readonly=True, autocommit=True)
        with connection.cursor() as cursor:
            cursor.execute(
                "SELECT id, name, is_active FROM tags WHERE id = ANY(%s)",
                (list(tag_ids),),
            )
            rows = cursor.fetchall()
    except psycopg2.Error as exc:
        raise VerificationError(f"PostgreSQL read failed: {exc}") from exc
    finally:
        if "connection" in locals():
            connection.close()

    return {
        int(tag_id): {"id": int(tag_id), "name": name, "is_active": is_active}
        for tag_id, name, is_active in rows
    }


def _tag_detail(
    tag_id: int,
    file_tags: Mapping[int, Mapping[str, Any]],
    database_tags: Mapping[int, Mapping[str, Any]],
    locations: Mapping[int, list[str]],
) -> dict[str, Any]:
    file_tag = file_tags.get(tag_id)
    database_tag = database_tags.get(tag_id)
    if database_tag is None:
        database_status = "missing"
    elif database_tag.get("is_active") is True:
        database_status = "active"
    else:
        database_status = "inactive"
    return {
        "id": tag_id,
        "database_status": database_status,
        "database_name": database_tag.get("name") if database_tag else None,
        "file_status": (
            "missing"
            if file_tag is None
            else "active" if file_tag.get("is_active") is True else "inactive"
        ),
        "file_name": file_tag.get("name") if file_tag else None,
        "display_title": file_tag.get("display_title") if file_tag else None,
        "redis_locations": sorted(locations.get(tag_id, [])),
    }


def build_verification_report(
    specs: Mapping[str, Mapping[str, Any]],
    redis_keys: Mapping[str, str],
    file_tags: Mapping[int, Mapping[str, Any]],
    database_tags: Mapping[int, Mapping[str, Any]],
) -> dict[str, Any]:
    all_locations: dict[int, list[str]] = defaultdict(list)
    platform_summary: dict[str, Any] = {}
    for platform, spec in specs.items():
        locations, leaf_count = extract_redis_tag_locations(spec, platform)
        for tag_id, paths in locations.items():
            all_locations[tag_id].extend(paths)
        platform_summary[platform] = {
            "redis_key": redis_keys[platform],
            "leaf_count": leaf_count,
            "unique_tag_count": len(locations),
        }

    redis_ids = set(all_locations)
    file_active_ids = {
        tag_id for tag_id, tag in file_tags.items() if tag.get("is_active") is True
    }
    database_active_ids = {
        tag_id
        for tag_id, tag in database_tags.items()
        if tag.get("is_active") is True
    }

    redis_not_active_in_database = sorted(redis_ids - database_active_ids)
    file_active_not_database_active = sorted(
        (redis_ids & file_active_ids) - database_active_ids
    )
    unverified_by_file = sorted(redis_ids - file_active_ids)

    def details(ids: Iterable[int]) -> list[dict[str, Any]]:
        return [
            _tag_detail(tag_id, file_tags, database_tags, all_locations)
            for tag_id in ids
        ]

    # PostgreSQL is the execution-time authority. File discrepancies are reported,
    # but only a Redis ID that is missing/inactive in PostgreSQL fails verification.
    passed = not redis_not_active_in_database
    return {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "passed": passed,
        "platforms": platform_summary,
        "counts": {
            "unique_redis_tags": len(redis_ids),
            "redis_tags_active_in_database": len(redis_ids & database_active_ids),
            "redis_tags_not_active_in_database": len(redis_not_active_in_database),
            "file_active_but_database_not_active": len(
                file_active_not_database_active
            ),
            "redis_tags_not_verified_active_by_file": len(unverified_by_file),
        },
        # This is the primary list requested by the operator.
        "file_active_but_database_not_active": details(
            file_active_not_database_active
        ),
        # This broader list verifies every ID actually present in Redis, including
        # the required external test tag which may be absent from the tags export.
        "redis_tags_not_active_in_database": details(
            redis_not_active_in_database
        ),
        "redis_tags_not_verified_active_by_file": details(unverified_by_file),
    }


def _write_report(path: Path, report: Mapping[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_name(f".{path.name}.tmp-{os.getpid()}")
    content = json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n"
    try:
        with temporary.open("x", encoding="utf-8") as handle:
            handle.write(content)
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary, path)
    finally:
        try:
            temporary.unlink()
        except FileNotFoundError:
            pass


def parse_args(argv: Sequence[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Verify audience-spec Redis tag IDs against tags.json and PostgreSQL."
    )
    parser.add_argument("--tags", type=Path, default=Path("tags.json"))
    parser.add_argument(
        "--platform",
        action="append",
        choices=PLATFORMS,
        dest="platforms",
        help="platform to verify; repeat for multiple platforms (default: sms)",
    )
    parser.add_argument(
        "--redis-db", type=int, default=_env_int("CACHE_REDIS_DB")
    )
    parser.add_argument(
        "--redis-prefix", default=os.getenv("CACHE_REDIS_PREFIX", "yamata:")
    )
    parser.add_argument("--db-host", default=os.getenv("DB_HOST", "127.0.0.1"))
    parser.add_argument("--db-port", type=int, default=_env_int("DB_PORT", 5432))
    parser.add_argument("--db-name", default=os.getenv("DB_NAME"))
    parser.add_argument("--db-user", default=os.getenv("DB_USER"))
    parser.add_argument(
        "--db-sslmode", default=os.getenv("DB_SSL_MODE", "require")
    )
    parser.add_argument(
        "--output",
        type=Path,
        default=Path("audience-tag-activity-report.json"),
        help="JSON report path (default: audience-tag-activity-report.json)",
    )
    args = parser.parse_args(argv)
    args.redis_url = os.getenv("CACHE_REDIS_URL")
    if not args.platforms:
        args.platforms = ["sms"]
    args.platforms = list(dict.fromkeys(args.platforms))
    if not args.redis_url:
        parser.error("CACHE_REDIS_URL is required")
    if args.redis_db is not None and args.redis_db < 0:
        parser.error("--redis-db must be greater than or equal to zero")
    if args.db_port is None or not 1 <= args.db_port <= 65535:
        parser.error("--db-port must be between 1 and 65535")
    if not args.db_name:
        parser.error("--db-name or DB_NAME is required")
    if not args.db_user:
        parser.error("--db-user or DB_USER is required")
    try:
        args.db_password = read_secret("DB_PASSWORD", "Database password: ")
    except RuntimeError as exc:
        parser.error(str(exc))
    return args


def main(argv: Sequence[str] | None = None) -> int:
    os.umask(0o077)
    try:
        args = parse_args(argv)
        file_tags = _load_tags_file(args.tags)
        specs, redis_keys = load_redis_specs(
            args.redis_url,
            args.redis_db,
            args.redis_prefix,
            args.platforms,
        )
        redis_ids: set[int] = set()
        for platform, spec in specs.items():
            locations, _ = extract_redis_tag_locations(spec, platform)
            redis_ids.update(locations)
        database_tags = load_database_tags(sorted(redis_ids), args)
        report = build_verification_report(
            specs, redis_keys, file_tags, database_tags
        )
        _write_report(args.output, report)

        counts = report["counts"]
        print(
            f"Verified {counts['unique_redis_tags']} unique Redis tags: "
            f"{counts['redis_tags_active_in_database']} active in PostgreSQL, "
            f"{counts['redis_tags_not_active_in_database']} missing/inactive"
        )
        print(
            "File-active Redis tags missing/inactive in PostgreSQL: "
            f"{counts['file_active_but_database_not_active']}"
        )
        mismatches = report["file_active_but_database_not_active"]
        if mismatches:
            print(
                "Mismatched file-active tag IDs: "
                + ", ".join(str(row["id"]) for row in mismatches)
            )
        print(f"Report written to {args.output}")
        return 0 if report["passed"] else 2
    except (VerificationError, OSError) as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
