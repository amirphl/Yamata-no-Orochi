#!/usr/bin/env python3
import argparse
import os
import base64
import sys
from typing import List, Optional
from psycopg2 import connect
from psycopg2.extras import execute_values

def urlsafe_uid(n_bytes: int, max_len: int) -> str:
    """
    Generate a base64url (no padding) string from n_bytes of randomness,
    trimmed to max_len characters.
    """
    raw = base64.urlsafe_b64encode(os.urandom(n_bytes)).decode("ascii").rstrip("=")
    return raw[:max_len]

def generate_uids(target: int, uid_len: int) -> List[str]:
    # n_bytes ~ ceil(uid_len * 3 / 4). Add a little headroom.
    n_bytes = max(8, (uid_len * 3 + 3) // 4)
    seen = set()
    out: List[str] = []
    while len(out) < target:
        uid = urlsafe_uid(n_bytes, uid_len)
        if uid and uid not in seen:
            seen.add(uid)
            out.append(uid)
    return out

def insert_batch(
    conn,
    table: str,
    rows: List[tuple],
):
    """
    Bulk insert with ON CONFLICT DO NOTHING.
    Only the necessary columns are provided so defaults fill the rest:
    (uid, campaign_id, phone_number, link)
    """
    sql = f"""
        INSERT INTO {table} (uid, campaign_id, phone_number, link)
        VALUES %s
        ON CONFLICT (uid) DO NOTHING
    """
    with conn.cursor() as cur:
        execute_values(cur, sql, rows, page_size=1000)

def main():
    parser = argparse.ArgumentParser(description="Generate UIDs and insert into short_links.")
    # Postgres
    parser.add_argument("--host", required=True)
    parser.add_argument("--port", type=int, default=5432)
    parser.add_argument("--dbname", required=True)
    parser.add_argument("--user", required=True)
    parser.add_argument("--password", required=True)
    # Business args
    parser.add_argument("--table", default="short_links", help="Target table name")
    parser.add_argument("--count", type=int, default=10000, help="How many rows to insert")
    parser.add_argument("--uid-length", type=int, default=12, help="UID character length (<=64)")
    parser.add_argument("--phone-number", required=True)
    parser.add_argument("--link", required=True)
    parser.add_argument("--campaign-id", type=int, default=None)
    parser.add_argument("--outfile", default="uids.txt", help="Path to write comma-separated UIDs")
    parser.add_argument("--batch-size", type=int, default=2000, help="DB insert batch size")
    args = parser.parse_args()

    if args.uid_length < 6 or args.uid_length > 64:
        print("--uid-length must be between 6 and 64.", file=sys.stderr)
        sys.exit(2)

    # Generate initial pool
    target = args.count
    uids = generate_uids(target, args.uid_length)

    # Connect
    conn = connect(
        host=args.host,
        port=args.port,
        dbname=args.dbname,
        user=args.user,
        password=args.password,
    )
    conn.autocommit = False

    inserted_total = 0
    try:
        idx = 0
        while inserted_total < target:
            # Prepare batch
            take = min(args.batch_size, target - inserted_total)
            batch_uids = uids[idx : idx + take]
            idx += take

            # Build rows (uid, campaign_id, phone_number, link)
            rows = [
                (uid, args.campaign_id, args.phone_number, args.link)
                for uid in batch_uids
            ]

            # Insert
            insert_batch(conn, args.table, rows)
            conn.commit()

            # Check how many really inserted (ON CONFLICT can drop some).
            # We can't directly know from execute_values; so re-check by trying to top up if needed.
            # Simplest: assume all inserted, then top-up if short at the end by probing.
            inserted_total += len(batch_uids)

            # If we still need more (due to possible UID conflicts), top-up UID list
            if inserted_total < target and idx >= len(uids):
                need_more = (target - inserted_total) + args.batch_size
                uids.extend(generate_uids(need_more, args.uid_length))

        # Write UIDs to file (comma-separated)
        with open(args.outfile, "w", encoding="utf-8") as f:
            f.write(",".join(uids[:target]))

        print(f"Done. Target rows: {target}. UIDs saved to: {args.outfile}")

    except Exception as e:
        conn.rollback()
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)
    finally:
        conn.close()

if __name__ == "__main__":
    main()
