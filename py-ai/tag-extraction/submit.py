#!/usr/bin/env python3
import os
import uuid
import time
import argparse
import pandas as pd
import requests
import sys
import ast

API_URL = "https://jazebeh.ir/api/v1/bot/campaigns/audience-spec"


def clean_str(s: object) -> str:
    if s is None or (isinstance(s, float) and pd.isna(s)):
        return ""
    return str(s).strip()


def parse_tags(cell: str):
    s = clean_str(cell)
    if not s:
        return []
    try:
        v = ast.literal_eval(s)
        if isinstance(v, (list, tuple)):
            return [clean_str(str(x)) for x in v if clean_str(str(x))]
    except Exception:
        pass
    # fallback: split by comma or semicolon
    parts = [p.strip() for p in s.strip("[](){} ").split(",")]
    return [p for p in parts if p]


def main():
    parser = argparse.ArgumentParser(description="Push audience specs to API from stat_with_tags.csv.")
    parser.add_argument("--csv", default="stat_with_tags.csv",
                        help="Path to input CSV (default: stat_with_tags.csv)")
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

    required = {"level1", "level2", "level3", "tags", "available_audience"}
    missing = required - set(df.columns)
    if missing:
        print(f"ERROR: CSV missing columns: {', '.join(sorted(missing))}", file=sys.stderr)
        sys.exit(1)

    df["level1"] = df["level1"].map(clean_str)
    df["level2"] = df["level2"].map(clean_str)
    df["level3"] = df["level3"].map(clean_str)
    df["tags_list"] = df["tags"].map(parse_tags)
    df["available_audience"] = df["available_audience"].map(lambda x: int(clean_str(x) or "10000"))

    # optional level2 metadata columns produced by main.py
    for col in ("level2_one_line", "level2_inclusion", "level2_exclusion"):
        if col in df.columns:
            df[col] = df[col].map(clean_str)
        else:
            df[col] = ""

    # Filter invalid rows
    df = df[(df["level1"] != "") & (df["level2"] != "") & (df["level3"] != "")].copy()

    session = requests.Session()
    session.headers.update({
        "Content-Type": "application/json",
        "Authorization": f"Bearer {token}",
    })

    logs = []
    success = failed = 0

    for idx0, row in df.iterrows():
        tags = row["tags_list"]
        if not isinstance(tags, list) or len(tags) == 0:
            failed += 1
            logs.append({"row": int(idx0) + 1, "status": "skipped", "reason": "no tags"})
            continue

        payload = {
            "level1": row["level1"],
            "level2": row["level2"],
            "level3": row["level3"],
            "tags": [str(t) for t in tags],
            "available_audience": int(row["available_audience"]),
        }
        # attach level2 metadata when available
        level2_meta = {
            "one_line": clean_str(row.get("level2_one_line", "")),
            "inclusion": clean_str(row.get("level2_inclusion", "")),
            "exclusion": clean_str(row.get("level2_exclusion", "")),
        }
        # only include a metadata object if any field is non-empty
        if any(level2_meta.values()):
            payload["metadata"] = level2_meta

        try:
            print(payload)
            print("----")
            resp = session.post(
                API_URL,
                json=payload,
                headers={"X-Request-ID": str(uuid.uuid4())},
                timeout=args.timeout,
            )
            if resp.ok:
                success += 1
                logs.append({
                    "row": int(idx0) + 1,
                    "status": "ok",
                    "code": resp.status_code,
                    "levels": f"{payload['level1']}|{payload['level2']}|{payload['level3']}",
                    "tags_count": len(payload["tags"]),
                })
            else:
                failed += 1
                logs.append({
                    "row": int(idx0) + 1,
                    "status": "error",
                    "code": resp.status_code,
                    "levels": f"{payload['level1']}|{payload['level2']}|{payload['level3']}",
                    "body": (resp.text or "")[:500],
                })
        except requests.RequestException as e:
            failed += 1
            logs.append({
                "row": int(idx0) + 1,
                "status": "exception",
                "levels": f"{payload['level1']}|{payload['level2']}|{payload['level3']}",
                "error": str(e),
            })

        time.sleep(args.sleep)

    pd.DataFrame(logs).to_csv(args.log, index=False, encoding="utf-8-sig")
    print(f"Done. Success={success}, Failed/Skipped={failed}, Total={len(df)}")
    print(f"Log saved to {args.log}")


if __name__ == "__main__":
    main()
