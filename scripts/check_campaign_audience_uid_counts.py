#!/usr/bin/env python3
"""
Compare audience UID counts in data/campaign_audience_uids/ files against
the num_audience field in the campaigns table.

For each {campaign_id}.jsonl file found:
  1. Counts unique UIDs (deduplicating by uid key, mirroring Go readCampaignAudienceUIDs).
  2. Fetches campaigns.num_audience for that campaign_id.
  3. Logs a MISMATCH line when the counts differ.

Usage:
  python check_campaign_audience_uid_counts.py \\
      --db-host 127.0.0.1 --db-port 5432 --db-name yamata \\
      --db-user postgres --db-password secret \\
      [--dir path/to/campaign_audience_uids] \\
      [--campaign-ids 101 102 103]
"""

import argparse
import json
import logging
import os
import sys

import psycopg2

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(message)s",
    datefmt="%Y-%m-%dT%H:%M:%SZ",
)
log = logging.getLogger(__name__)

DEFAULT_DIR = os.path.join("data", "campaign_audience_uids")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Check that campaign_audience_uids file UID counts match campaigns.num_audience."
    )
    parser.add_argument(
        "--dir",
        default=DEFAULT_DIR,
        help=f"Directory containing {{campaign_id}}.jsonl files (default: {DEFAULT_DIR})",
    )
    parser.add_argument(
        "--campaign-ids",
        nargs="+",
        type=int,
        metavar="ID",
        help="Only check these campaign IDs (default: all files in --dir)",
    )
    # DB args
    parser.add_argument("--db-host", default="127.0.0.1", help="PostgreSQL host")
    parser.add_argument("--db-port", type=int, default=5432, help="PostgreSQL port")
    parser.add_argument("--db-name", required=True, help="Database name")
    parser.add_argument("--db-user", required=True, help="Database user")
    parser.add_argument("--db-password", required=True, help="Database password")
    return parser.parse_args()


# ---------------------------------------------------------------------------
# File helpers
# ---------------------------------------------------------------------------


def discover_campaign_ids(directory: str) -> list[int]:
    """Return sorted campaign IDs from *.jsonl filenames in directory."""
    ids = []
    try:
        entries = os.listdir(directory)
    except FileNotFoundError:
        log.error("Directory not found: %s", directory)
        sys.exit(1)

    for name in entries:
        if not name.endswith(".jsonl"):
            continue
        stem = name[: -len(".jsonl")]
        try:
            ids.append(int(stem))
        except ValueError:
            log.warning("Skipping non-integer filename: %s", name)
    return sorted(ids)


def count_unique_uids(filepath: str) -> tuple[int, int]:
    """
    Count unique UIDs in a .jsonl file, mirroring Go readCampaignAudienceUIDs.
    Returns (unique_uid_count, total_line_count).
    Skips blank lines and records with empty uid.
    """
    seen: set[str] = set()
    total_lines = 0
    with open(filepath, encoding="utf-8") as f:
        for raw in f:
            line = raw.strip()
            if not line:
                continue
            total_lines += 1
            try:
                record = json.loads(line)
            except json.JSONDecodeError as exc:
                log.warning("JSON parse error in %s: %s", filepath, exc)
                continue
            uid = (record.get("uid") or "").strip()
            if uid:
                seen.add(uid)
    return len(seen), total_lines


# ---------------------------------------------------------------------------
# DB helpers
# ---------------------------------------------------------------------------


def connect_db(args: argparse.Namespace):
    return psycopg2.connect(
        host=args.db_host,
        port=args.db_port,
        dbname=args.db_name,
        user=args.db_user,
        password=args.db_password,
    )


def fetch_num_audiences(cur, campaign_ids: list[int]) -> dict[int, int | None]:
    """Return {campaign_id: num_audience} for all requested IDs in one query."""
    if not campaign_ids:
        return {}
    cur.execute(
        "SELECT id, num_audience FROM campaigns WHERE id = ANY(%s)",
        (campaign_ids,),
    )
    result: dict[int, int | None] = {}
    for row in cur.fetchall():
        cid, num = row
        result[cid] = int(num) if num is not None else None
    return result


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------


def main() -> None:
    args = parse_args()

    if args.campaign_ids:
        campaign_ids = sorted(set(args.campaign_ids))
        log.info("Checking %d campaign ID(s) supplied via --campaign-ids", len(campaign_ids))
    else:
        campaign_ids = discover_campaign_ids(args.dir)
        log.info("Discovered %d .jsonl file(s) in %s", len(campaign_ids), args.dir)

    if not campaign_ids:
        log.info("Nothing to check.")
        return

    conn = connect_db(args)
    conn.autocommit = True
    cur = conn.cursor()

    num_audience_map = fetch_num_audiences(cur, campaign_ids)

    cur.close()
    conn.close()

    ok_count = 0
    mismatch_count = 0
    skip_count = 0

    for campaign_id in campaign_ids:
        filepath = os.path.join(args.dir, f"{campaign_id}.jsonl")

        if not os.path.exists(filepath):
            log.warning(
                "campaign_id=%d  FILE NOT FOUND: %s (was listed but no file on disk)",
                campaign_id,
                filepath,
            )
            skip_count += 1
            continue

        unique_uids, total_lines = count_unique_uids(filepath)

        if campaign_id not in num_audience_map:
            log.warning(
                "campaign_id=%d  NOT IN DB: file has %d unique UIDs (%d lines) but campaign row not found",
                campaign_id,
                unique_uids,
                total_lines,
            )
            skip_count += 1
            continue

        num_audience = num_audience_map[campaign_id]

        if num_audience is None:
            log.warning(
                "campaign_id=%d  num_audience=NULL: file has %d unique UIDs (%d lines) — cannot compare",
                campaign_id,
                unique_uids,
                total_lines,
            )
            skip_count += 1
            continue

        if unique_uids != num_audience:
            diff = unique_uids - num_audience
            log.warning(
                "MISMATCH  campaign_id=%d  file_unique_uids=%d  db_num_audience=%d  diff=%+d  file=%s",
                campaign_id,
                unique_uids,
                num_audience,
                diff,
                filepath,
            )
            mismatch_count += 1
        else:
            log.info(
                "OK        campaign_id=%d  unique_uids=%d  num_audience=%d",
                campaign_id,
                unique_uids,
                num_audience,
            )
            ok_count += 1

    log.info(
        "Summary: ok=%d mismatch=%d skipped=%d total=%d",
        ok_count,
        mismatch_count,
        skip_count,
        len(campaign_ids),
    )

    if mismatch_count > 0:
        sys.exit(1)


if __name__ == "__main__":
    main()
