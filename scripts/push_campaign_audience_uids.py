#!/usr/bin/env python3
"""
Push audience UIDs (and their short-link codes) to Jazebeh for a list of campaigns.

For each campaign ID the script:
  1. Fetches audience_ids (bigint[]) and audience_codes (text[]) from
     processed_campaigns (latest processed row for that campaign).
  2. Resolves the UID for each audience_id by querying audience_profiles.
     Order is preserved so that uid[i] pairs correctly with code[i].
  3. POSTs to POST /api/v1/bot/campaigns/{id}/audience-uids in chunks of
     CHUNK_SIZE items, mirroring the Go PushCampaignAudienceUIDs logic.

Usage:
  python push_campaign_audience_uids.py \\
      --campaign-ids 101 102 103 \\
      --db-host 127.0.0.1 --db-port 5432 --db-name yamata \\
      --db-user postgres --db-password secret \\
      --jazebeh-domain https://jazebeh.ir \\
      --bot-username botuser --bot-password botpass
"""

import argparse
import sys

import psycopg2
import psycopg2.extras
import requests

CHUNK_SIZE = 5000  # mirrors audienceUIDChunkSize in bot_client.go


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Push audience UIDs and short-link codes to Jazebeh."
    )
    parser.add_argument(
        "--campaign-ids",
        required=True,
        nargs="+",
        type=int,
        metavar="ID",
        help="One or more campaign IDs",
    )
    # DB args
    parser.add_argument("--db-host", default="127.0.0.1", help="PostgreSQL host")
    parser.add_argument("--db-port", type=int, default=5432, help="PostgreSQL port")
    parser.add_argument("--db-name", required=True, help="Database name")
    parser.add_argument("--db-user", required=True, help="Database user")
    parser.add_argument("--db-password", required=True, help="Database password")
    # Jazebeh args
    parser.add_argument(
        "--jazebeh-domain",
        default="https://jazebeh.ir",
        help="Jazebeh API domain (no trailing slash)",
    )
    parser.add_argument("--bot-username", required=True, help="Jazebeh bot username")
    parser.add_argument("--bot-password", required=True, help="Jazebeh bot password")
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Compute UID/code pairs but do not push to Jazebeh",
    )
    return parser.parse_args()


# ---------------------------------------------------------------------------
# Jazebeh helpers
# ---------------------------------------------------------------------------


def jazebeh_login(domain: str, username: str, password: str) -> str:
    url = domain.rstrip("/") + "/api/v1/bot/auth/login"
    resp = requests.post(
        url,
        json={"username": username, "password": password},
        timeout=30,
    )
    resp.raise_for_status()
    body = resp.json()
    if not body.get("success"):
        raise RuntimeError(f"Jazebeh login failed: {body.get('message')}")
    token = body["data"]["session"]["access_token"]
    if not token:
        raise RuntimeError("Jazebeh login returned empty access token")
    return token


def jazebeh_push_audience_uids(
    domain: str, token: str, campaign_id: int, uids: list[str], codes: list[str]
) -> None:
    """Push uid/code pairs in CHUNK_SIZE chunks, mirroring Go PushCampaignAudienceUIDs."""
    if not uids:
        return

    url = domain.rstrip("/") + f"/api/v1/bot/campaigns/{campaign_id}/audience-uids"

    for start in range(0, len(uids), CHUNK_SIZE):
        end = min(start + CHUNK_SIZE, len(uids))
        items = [
            {"uid": uids[i], "code": codes[i] if i < len(codes) else ""}
            for i in range(start, end)
        ]
        resp = requests.post(
            url,
            json={"items": items},
            headers={"Authorization": f"Bearer {token}"},
            timeout=60,
        )
        resp.raise_for_status()
        body = resp.json()
        if not body.get("success"):
            raise RuntimeError(
                f"Push audience UIDs failed for campaign {campaign_id} "
                f"chunk [{start},{end}): {body.get('message')}"
            )
        print(f"  chunk [{start},{end}) pushed ({end - start} items)")


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


def fetch_processed_campaign(
    cur, campaign_id: int
) -> tuple[int, list[int], list[str]] | None:
    """
    Returns (processed_campaign_id, audience_ids, audience_codes) for the latest
    processed_campaign row matching campaign_id, or None if not found.
    """
    cur.execute(
        """
        SELECT id, audience_ids, audience_codes
        FROM processed_campaigns
        WHERE campaign_id = %s
        ORDER BY id DESC
        LIMIT 1
        """,
        (campaign_id,),
    )
    row = cur.fetchone()
    if row is None:
        return None
    pc_id, audience_ids, audience_codes = row
    # psycopg2 returns postgres arrays as Python lists already
    return pc_id, list(audience_ids or []), list(audience_codes or [])


def resolve_uids(cur, audience_ids: list[int]) -> dict[int, str]:
    """
    Fetch uid for each id in audience_ids from audience_profiles.
    Returns a mapping {id: uid}.
    """
    if not audience_ids:
        return {}
    cur.execute(
        "SELECT id, uid FROM audience_profiles WHERE id = ANY(%s)",
        (audience_ids,),
    )
    return {row[0]: row[1] for row in cur.fetchall()}


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------


def main() -> None:
    args = parse_args()

    print(f"Campaigns: {args.campaign_ids}")
    print(f"Dry-run  : {args.dry_run}")

    conn = connect_db(args)
    conn.autocommit = True
    cur = conn.cursor()

    token: str | None = None
    if not args.dry_run:
        print("Logging in to Jazebeh...")
        token = jazebeh_login(args.jazebeh_domain, args.bot_username, args.bot_password)
        print("Login successful.")

    if token is None:
        print("Jazebeh Access Token is empty. Exiting ...")
    success_count = 0
    skip_count = 0
    error_count = 0

    for campaign_id in args.campaign_ids:
        print(f"\n--- campaign_id={campaign_id} ---")

        result = fetch_processed_campaign(cur, campaign_id)
        if result is None:
            print(f"  SKIP: no processed_campaign found for campaign_id={campaign_id}")
            skip_count += 1
            continue

        pc_id, audience_ids, audience_codes = result
        print(
            f"  processed_campaign_id={pc_id} "
            f"audience_ids={len(audience_ids)} audience_codes={len(audience_codes)}"
        )

        if not audience_ids:
            print(f"  SKIP: audience_ids is empty for processed_campaign_id={pc_id}")
            skip_count += 1
            continue

        if len(audience_codes) != len(audience_ids):
            print(
                f"  WARNING: audience_codes length ({len(audience_codes)}) != "
                f"audience_ids length ({len(audience_ids)}) — codes will be padded with empty strings"
            )

        # Resolve UIDs from audience_profiles, preserving the order of audience_ids
        uid_map = resolve_uids(cur, audience_ids)
        missing = [aid for aid in audience_ids if aid not in uid_map]
        if missing:
            print(
                f"  WARNING: {len(missing)} audience_id(s) not found in audience_profiles "
                f"(first 10: {missing[:10]}); those entries will use empty UID"
            )

        uids = [uid_map.get(aid, "") for aid in audience_ids]
        codes = [
            audience_codes[i] if i < len(audience_codes) else ""
            for i in range(len(uids))
        ]

        print(f"  resolved {len(uids)} uid/code pairs")

        if args.dry_run:
            print("  DRY-RUN: skipping push to Jazebeh")
            # Show a small sample
            sample = [
                {"uid": uids[i], "code": codes[i]} for i in range(min(3, len(uids)))
            ]
            print(f"  sample (first {len(sample)}): {sample}")
            success_count += 1
            continue

        try:
            assert token is not None
            jazebeh_push_audience_uids(
                args.jazebeh_domain, token, campaign_id, uids, codes
            )
            print(
                f"  OK: pushed {len(uids)} uid/code pairs for campaign_id={campaign_id}"
            )
            success_count += 1
        except Exception as exc:
            print(f"  ERROR: {exc}", file=sys.stderr)
            error_count += 1

    cur.close()
    conn.close()

    print(f"\nDone. success={success_count} skipped={skip_count} errors={error_count}")
    if error_count > 0:
        sys.exit(1)


if __name__ == "__main__":
    main()
