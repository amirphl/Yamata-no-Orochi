#!/usr/bin/env python3
"""Generate synthetic audience profiles from tag IDs already in PostgreSQL.

Rows are streamed to ``public.audience_profiles`` with PostgreSQL COPY.  The
generated phone numbers are sequential and the generated UIDs include the phone
number, making both values unique within a run without retaining a large set in
memory.

DB_PASSWORD is read from the environment.  If it is absent, an interactive
terminal receives a hidden password prompt.
"""

from __future__ import annotations

import argparse
import io
import os
import random
import re
import sys
import time
from collections.abc import Iterable, Sequence
from dataclasses import dataclass

from script_common import read_secret, validate_database_port

DEFAULT_PHONE_START = 989_010_000_000
DEFAULT_BATCH_SIZE = 25_000
DEFAULT_TAGS_PER_PROFILE = 20
DEFAULT_COLORS = ("white", "pink", "black")
INT32_MIN = -(2**31)
INT32_MAX = 2**31 - 1
SAFE_TEXT_RE = re.compile(r"^[A-Za-z0-9_-]+$")


@dataclass(frozen=True)
class SyntheticProfile:
    uid: str
    phone_number: str
    tags: tuple[int, ...]
    color: str
    normalized_score: float


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Generate synthetic audience_profiles rows with sequential phone "
            "numbers and random tag assignments."
        )
    )
    parser.add_argument(
        "--count",
        required=True,
        type=int,
        help="Number of profiles to insert (required as an accident guard)",
    )
    parser.add_argument("--db-host", default="localhost")
    parser.add_argument("--db-port", type=int, default=5432)
    parser.add_argument("--db-name", default="somedb")
    parser.add_argument("--db-user", default="postgres")
    parser.add_argument("--db-sslmode", default=os.getenv("DB_SSL_MODE", "disable"))
    parser.add_argument(
        "--phone-start",
        type=int,
        default=DEFAULT_PHONE_START,
        help=f"First generated phone number (default: {DEFAULT_PHONE_START})",
    )
    parser.add_argument(
        "--tags-per-profile",
        type=int,
        default=DEFAULT_TAGS_PER_PROFILE,
        help=f"Distinct random tags per profile (default: {DEFAULT_TAGS_PER_PROFILE})",
    )
    parser.add_argument(
        "--batch-size",
        type=int,
        default=DEFAULT_BATCH_SIZE,
        help=f"Rows committed per COPY batch (default: {DEFAULT_BATCH_SIZE})",
    )
    parser.add_argument(
        "--uid-prefix",
        default="syn_",
        help="Prefix for generated UIDs (default: syn_)",
    )
    parser.add_argument(
        "--colors",
        nargs="+",
        default=list(DEFAULT_COLORS),
        help="Colors to assign randomly (default: white pink black)",
    )
    parser.add_argument(
        "--include-inactive-tags",
        action="store_true",
        help="Read all tags instead of only rows where is_active is true",
    )
    parser.add_argument(
        "--seed",
        type=int,
        help="Optional deterministic random seed",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Read tags and print up to five generated rows without inserting",
    )
    args = parser.parse_args()

    validate_database_port(parser, args.db_port)
    if args.count <= 0:
        parser.error("--count must be greater than zero")
    if args.phone_start <= 0:
        parser.error("--phone-start must be greater than zero")
    if args.tags_per_profile <= 0:
        parser.error("--tags-per-profile must be greater than zero")
    if args.batch_size <= 0:
        parser.error("--batch-size must be greater than zero")
    if not args.uid_prefix or not SAFE_TEXT_RE.fullmatch(args.uid_prefix):
        parser.error("--uid-prefix may contain only letters, digits, _ and -")
    if not args.colors:
        parser.error("--colors must not be empty")
    for color in args.colors:
        if len(color) > 20 or not SAFE_TEXT_RE.fullmatch(color):
            parser.error("each --colors value must be 1-20 letters, digits, _ or -")

    final_phone = args.phone_start + args.count - 1
    if len(str(final_phone)) > 20:
        parser.error("the final phone number would exceed varchar(20)")
    longest_uid = f"{args.uid_prefix}{final_phone}_{'f' * 12}"
    if len(longest_uid) > 255:
        parser.error("generated UIDs would exceed varchar(255)")
    return args


def connect_database(args: argparse.Namespace, password: str):
    try:
        import psycopg2
    except ImportError as exc:
        raise RuntimeError(
            "psycopg2 is required; install scripts/requirements.txt"
        ) from exc

    return psycopg2.connect(
        host=args.db_host,
        port=args.db_port,
        dbname=args.db_name,
        user=args.db_user,
        password=password,
        sslmode=args.db_sslmode,
        connect_timeout=10,
        application_name="generate_synthetic_audience_profiles",
    )


def fetch_tag_ids(connection, include_inactive: bool) -> list[int]:
    condition = "" if include_inactive else "WHERE is_active IS TRUE"
    with connection.cursor() as cursor:
        cursor.execute(f"SELECT id FROM public.tags {condition} ORDER BY id")
        tag_ids = [row[0] for row in cursor.fetchall()]
    invalid = [tag_id for tag_id in tag_ids if not INT32_MIN <= tag_id <= INT32_MAX]
    if invalid:
        raise RuntimeError(
            f"tags.id contains a value outside the integer[] range: {invalid[0]}"
        )
    return tag_ids


def ensure_target_is_compatible(connection, phone_start: int, final_phone: int) -> None:
    with connection.cursor() as cursor:
        cursor.execute("SELECT to_regclass('public.audience_profiles')")
        if cursor.fetchone()[0] is None:
            raise RuntimeError("public.audience_profiles does not exist")

        # Split at powers of ten so every lexical range contains equal-width
        # digit strings and can use the varchar phone-number index correctly.
        range_start = phone_start
        while range_start <= final_phone:
            digit_count = len(str(range_start))
            range_end = min(final_phone, (10**digit_count) - 1)
            cursor.execute(
                """
                SELECT phone_number
                FROM public.audience_profiles
                WHERE phone_number >= %s AND phone_number <= %s
                LIMIT 1
                """,
                (str(range_start), str(range_end)),
            )
            conflict = cursor.fetchone()
            if conflict is not None:
                raise RuntimeError(
                    "the requested phone range overlaps existing data; first "
                    f"conflicting value found: {conflict[0]}"
                )
            range_start = range_end + 1


def generate_profiles(
    *,
    start_offset: int,
    count: int,
    phone_start: int,
    uid_prefix: str,
    tag_ids: Sequence[int],
    tags_per_profile: int,
    colors: Sequence[str],
    rng: random.Random,
) -> Iterable[SyntheticProfile]:
    for offset in range(start_offset, start_offset + count):
        phone_number = str(phone_start + offset)
        # The phone component guarantees uniqueness within the requested range;
        # the suffix makes the synthetic UID visibly random.
        uid = f"{uid_prefix}{phone_number}_{rng.getrandbits(48):012x}"
        yield SyntheticProfile(
            uid=uid,
            phone_number=phone_number,
            tags=tuple(rng.sample(tag_ids, tags_per_profile)),
            color=rng.choice(colors),
            normalized_score=rng.random(),
        )


def profiles_to_copy_buffer(profiles: Iterable[SyntheticProfile]) -> io.StringIO:
    output = io.StringIO()
    for profile in profiles:
        tags_literal = "{" + ",".join(str(tag) for tag in profile.tags) + "}"
        output.write(
            f"{profile.uid}\t{profile.phone_number}\t{tags_literal}\t"
            f"{profile.color}\t{profile.normalized_score:.17g}\n"
        )
    output.seek(0)
    return output


def copy_batch(connection, buffer: io.StringIO) -> None:
    with connection.cursor() as cursor:
        cursor.copy_expert(
            """
            COPY public.audience_profiles
                (uid, phone_number, tags, color, normalized_score)
            FROM STDIN WITH (FORMAT text)
            """,
            buffer,
        )


def print_dry_run(profiles: Iterable[SyntheticProfile]) -> None:
    print("uid | phone_number | tags | color | normalized_score")
    for profile in profiles:
        print(
            f"{profile.uid} | {profile.phone_number} | {list(profile.tags)} | "
            f"{profile.color} | {profile.normalized_score:.6f}"
        )


def main() -> int:
    args = parse_args()
    try:
        password = read_secret("DB_PASSWORD", "Local PostgreSQL password: ")
        connection = connect_database(args, password)
        connection.autocommit = False
        try:
            tag_ids = fetch_tag_ids(connection, args.include_inactive_tags)
            if len(tag_ids) < args.tags_per_profile:
                raise RuntimeError(
                    f"only {len(tag_ids)} eligible tags exist; "
                    f"{args.tags_per_profile} are required per profile"
                )

            final_phone = args.phone_start + args.count - 1
            ensure_target_is_compatible(connection, args.phone_start, final_phone)
            connection.rollback()  # End the read-only preflight transaction.

            rng = random.Random(args.seed)
            if args.dry_run:
                print_dry_run(
                    generate_profiles(
                        start_offset=0,
                        count=min(args.count, 5),
                        phone_start=args.phone_start,
                        uid_prefix=args.uid_prefix,
                        tag_ids=tag_ids,
                        tags_per_profile=args.tags_per_profile,
                        colors=args.colors,
                        rng=rng,
                    )
                )
                return 0

            started_at = time.monotonic()
            inserted = 0
            while inserted < args.count:
                batch_count = min(args.batch_size, args.count - inserted)
                profiles = generate_profiles(
                    start_offset=inserted,
                    count=batch_count,
                    phone_start=args.phone_start,
                    uid_prefix=args.uid_prefix,
                    tag_ids=tag_ids,
                    tags_per_profile=args.tags_per_profile,
                    colors=args.colors,
                    rng=rng,
                )
                copy_batch(connection, profiles_to_copy_buffer(profiles))
                connection.commit()
                inserted += batch_count

                elapsed = max(time.monotonic() - started_at, 0.001)
                print(
                    f"inserted={inserted}/{args.count} "
                    f"rate={inserted / elapsed:,.0f} rows/s",
                    flush=True,
                )

            print(
                f"Done: inserted {inserted} synthetic profiles; phone range "
                f"{args.phone_start}..{final_phone}"
            )
            return 0
        except Exception:
            connection.rollback()
            raise
        finally:
            connection.close()
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print("Interrupted; the current batch was rolled back.", file=sys.stderr)
        return 130


if __name__ == "__main__":
    raise SystemExit(main())
