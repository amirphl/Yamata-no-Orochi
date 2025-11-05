#!/usr/bin/env python3
import os
import re
import uuid
import time
import math
import argparse
import pandas as pd
import requests
import sys

API_URL = "https://jazebeh.ir/api/v1/bot/campaigns/audience-spec"

PERSIAN_DIGITS = "۰۱۲۳۴۵۶۷۸۹"
EN_DIGITS      = "0123456789"
DIGIT_MAP = {ord(p): e for p, e in zip(PERSIAN_DIGITS, EN_DIGITS)}

def to_int(val) -> int:
    if pd.isna(val):
        return 0
    s = str(val).translate(DIGIT_MAP)
    s = re.sub(r"[^\d]", "", s)
    return int(s) if s else 0

def parse_tags(tag_cell: str):
    if tag_cell is None or (isinstance(tag_cell, float) and pd.isna(tag_cell)):
        return []
    s = str(tag_cell).strip()
    if not s:
        return []
    parts = re.split(r"[;,]", s)
    return [p.strip() for p in parts if p.strip()]

def clamp_range(n_rows: int, start_1based: int | None, end_1based: int | None):
    lo = 0 if start_1based is None else max(0, start_1based - 1)
    hi = n_rows - 1 if end_1based is None else min(n_rows - 1, end_1based - 1)
    if lo > hi:
        return None
    return lo, hi

def main():
    parser = argparse.ArgumentParser(description="Push audience specs to Jazebeh API for rows in a CSV.")
    parser.add_argument("--csv", default="stat_with_tags.csv",
                        help="Path to input CSV (default: stat_with_tags.csv)")
    parser.add_argument("-f", "--from-row", type=int, dest="from_row",
                        help="1-based row number to start from (inclusive)")
    parser.add_argument("-t", "--to-row", type=int, dest="to_row",
                        help="1-based row number to end at (inclusive)")
    parser.add_argument("--sleep", type=float, default=0.2,
                        help="Seconds to sleep between calls (default: 0.2)")
    parser.add_argument("--timeout", type=int, default=30,
                        help="HTTP timeout seconds (default: 30)")
    parser.add_argument("--log", default="audience_spec_push_log.csv",
                        help="Path to write per-row results (default: audience_spec_push_log.csv)")
    args = parser.parse_args()

    # token only from env
    token = os.getenv("JAZEBEH_API_TOKEN", "").strip()
    if not token:
        print("ERROR: JAZEBEH_API_TOKEN environment variable is missing or empty.", file=sys.stderr)
        sys.exit(1)

    # load CSV
    try:
        df = pd.read_csv(args.csv, dtype=str)
    except Exception as e:
        print(f"ERROR reading CSV: {e}", file=sys.stderr)
        sys.exit(1)

    required = {"category", "job", "tag ids", "audience_count_all"}
    missing = required - set(df.columns)
    if missing:
        print(f"ERROR: CSV missing columns: {', '.join(sorted(missing))}", file=sys.stderr)
        sys.exit(1)

    # normalize & compute
    df["category"] = df["category"].fillna("").str.strip()
    df["job"] = df["job"].fillna("").str.strip()
    df["audience_count_all"] = df["audience_count_all"].map(to_int)
    df["available_audience"] = (df["audience_count_all"] // 3).astype(int)
    df["tags_list"] = df["tag ids"].apply(parse_tags)

    # apply row window
    n = len(df)
    bounds = clamp_range(n, args.from_row, args.to_row)
    if bounds is None:
        print("Nothing to do: selected window is empty.", file=sys.stderr)
        sys.exit(0)
    lo, hi = bounds
    df = df.iloc[lo:hi+1].copy()

    session = requests.Session()
    session.headers.update({
        "Content-Type": "application/json",
        "Authorization": f"Bearer {token}",
    })

    logs = []
    success = failed = 0

    for idx_global, row in df.iterrows():
        payload = {
            "segment": row["category"],
            "subsegment": row["job"],
            "tags": row["tags_list"],
            "available_audience": int(row["available_audience"]),
        }

        if not payload["segment"] or not payload["subsegment"]:
            failed += 1
            logs.append({"csv_row_1based": idx_global + 1, "status": "skipped", "reason": "missing segment/subsegment"})
            continue
        if len(payload["tags"]) == 0:
            failed += 1
            logs.append({"csv_row_1based": idx_global + 1, "status": "skipped", "reason": "no tags"})
            continue

        try:
            resp = session.post(
                API_URL,
                json=payload,
                headers={"X-Request-ID": str(uuid.uuid4())},
                timeout=args.timeout
            )
            if resp.ok:
                success += 1
                logs.append({"csv_row_1based": idx_global + 1, "status": "ok", "code": resp.status_code})
            else:
                failed += 1
                logs.append({"csv_row_1based": idx_global + 1, "status": "error",
                             "code": resp.status_code, "body": (resp.text or "")[:500]})
        except requests.RequestException as e:
            failed += 1
            logs.append({"csv_row_1based": idx_global + 1, "status": "exception", "error": str(e)})

        time.sleep(args.sleep)

    pd.DataFrame(logs).to_csv(args.log, index=False, encoding="utf-8-sig")
    print(f"Done. Success={success}, Failed/Skipped={failed}, Total_Selected={len(df)}")
    print(f"Log saved to {args.log}")

if __name__ == "__main__":
    main()
