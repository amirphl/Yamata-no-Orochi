#!/usr/bin/env python3
import argparse
import csv
import sys
from collections import defaultdict
from pathlib import Path

import pandas as pd
import psycopg2


# Hardcoded database connection settings.
DB_CONFIG = {
    "host": "127.0.0.1",
    "port": 5432,
    "dbname": "yamata",
    "user": "postgres",
    "password": "postgres",
}

CAMPAIGN_TITLE_FILTER = "%TorobPay%"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Read UIDs from an XLSX file and export the TorobPay campaigns each UID "
            "participated in."
        )
    )
    parser.add_argument("input_xlsx", help="Path to the input XLSX file")
    parser.add_argument("output_csv", help="Path to the output CSV file")
    return parser.parse_args()


def read_uids_from_excel(path: str) -> list[str]:
    df = pd.read_excel(path, engine="openpyxl")
    if df.empty:
        return []

    first_column = df.iloc[:, 0]
    uids: list[str] = []
    seen: set[str] = set()
    for raw_value in first_column.tolist():
        if pd.isna(raw_value):
            continue
        uid = str(raw_value).strip()
        if not uid or uid in seen:
            continue
        seen.add(uid)
        uids.append(uid)
    return uids


def fetch_campaigns_by_uid(uids: list[str]) -> dict[str, list[str]]:
    if not uids:
        return {}

    query = """
        WITH processed_campaign_titles AS (
            SELECT
                pc.id,
                pc.created_at,
                pc.audience_ids,
                COALESCE(
                    pc.campaign_json->>'title',
                    pc.campaign_json#>>'{spec,title}'
                ) AS campaign_title
            FROM processed_campaigns AS pc
        )
        SELECT
            ap.uid,
            pct.campaign_title
        FROM processed_campaign_titles AS pct
        JOIN audience_profiles AS ap
            ON ap.id = ANY(pct.audience_ids)
        WHERE pct.campaign_title ILIKE %s
          AND ap.uid = ANY(%s)
          AND pct.campaign_title IS NOT NULL
        ORDER BY ap.uid, pct.created_at, pct.id
    """

    campaigns_by_uid: dict[str, list[str]] = defaultdict(list)
    seen_pairs: set[tuple[str, str]] = set()

    with psycopg2.connect(**DB_CONFIG) as conn:
        with conn.cursor() as cur:
            cur.execute(query, (CAMPAIGN_TITLE_FILTER, uids))
            for uid, campaign_title in cur.fetchall():
                if not campaign_title:
                    continue
                pair = (uid, campaign_title)
                if pair in seen_pairs:
                    continue
                seen_pairs.add(pair)
                campaigns_by_uid[uid].append(campaign_title)

    return dict(campaigns_by_uid)


def write_output_csv(
    path: str, uids: list[str], campaigns_by_uid: dict[str, list[str]]
) -> None:
    with open(path, "w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(["uid", "campaign_titles"])
        for uid in uids:
            writer.writerow([uid, ",".join(campaigns_by_uid.get(uid, []))])


def aggregate_uids_by_campaign_title(
    uids: list[str], campaigns_by_uid: dict[str, list[str]]
) -> dict[str, list[str]]:
    uids_by_campaign_title: dict[str, list[str]] = defaultdict(list)
    seen_pairs: set[tuple[str, str]] = set()

    for uid in uids:
        for campaign_title in campaigns_by_uid.get(uid, []):
            pair = (campaign_title, uid)
            if pair in seen_pairs:
                continue
            seen_pairs.add(pair)
            uids_by_campaign_title[campaign_title].append(uid)

    return dict(uids_by_campaign_title)


def campaign_aggregate_output_path(output_csv_path: str) -> str:
    path = Path(output_csv_path)
    if path.suffix:
        return str(path.with_name(f"{path.stem}_by_campaign{path.suffix}"))
    return str(path.with_name(f"{path.name}_by_campaign"))


def write_campaign_aggregate_csv(
    path: str, uids_by_campaign_title: dict[str, list[str]]
) -> None:
    with open(path, "w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(["campaign_title", "uids"])
        for campaign_title, title_uids in uids_by_campaign_title.items():
            writer.writerow([campaign_title, ",".join(title_uids)])

        writer.writerow([])
        writer.writerow(["campaign_title", "num_uids"])
        for campaign_title, title_uids in uids_by_campaign_title.items():
            writer.writerow([campaign_title, len(title_uids)])


def main() -> int:
    args = parse_args()

    try:
        uids = read_uids_from_excel(args.input_xlsx)
        campaigns_by_uid = fetch_campaigns_by_uid(uids)
        write_output_csv(args.output_csv, uids, campaigns_by_uid)
        aggregate_output_csv = campaign_aggregate_output_path(args.output_csv)
        uids_by_campaign_title = aggregate_uids_by_campaign_title(
            uids, campaigns_by_uid
        )
        write_campaign_aggregate_csv(aggregate_output_csv, uids_by_campaign_title)
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1

    print(f"Wrote {len(uids)} rows to {args.output_csv}")
    print(
        f"Wrote {len(uids_by_campaign_title)} campaign rows to {aggregate_output_csv}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
