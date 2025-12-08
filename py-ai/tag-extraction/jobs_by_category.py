#!/usr/bin/env python3
import argparse
import json
import sys
import pandas as pd


def clean_str(s: object) -> str:
    if s is None or (isinstance(s, float) and pd.isna(s)):
        return ""
    return str(s).strip()


def main() -> int:
    parser = argparse.ArgumentParser(description="Extract jobs of each category from Excel 'stat' sheet and save as JSON.")
    parser.add_argument("--excel", default="Customer Segmentation.xlsx",
                        help="Path to input Excel (default: Customer Segmentation.xlsx)")
    parser.add_argument("--sheet", default="stat",
                        help="Excel sheet name (default: stat)")
    parser.add_argument("--out", default="jobs_by_category.json",
                        help="Output JSON file path (default: jobs_by_category.json)")
    args = parser.parse_args()

    # Read sheet
    try:
        df = pd.read_excel(args.excel, sheet_name=args.sheet, dtype=str)
    except Exception as e:
        print(f"ERROR reading Excel: {e}", file=sys.stderr)
        return 1

    # Expect columns: category, job
    required = {"category", "job"}
    missing = required - set(df.columns)
    if missing:
        print(f"ERROR: Sheet '{args.sheet}' missing columns: {', '.join(sorted(missing))}", file=sys.stderr)
        return 1

    # Normalize and filter
    df["category"] = df["category"].map(clean_str)
    df["job"] = df["job"].map(clean_str)
    df = df[(df["category"] != "") & (df["job"] != "")].copy()

    # Group and aggregate unique, sorted jobs per category
    grouped = (
        df.groupby("category", as_index=True)["job"]
          .apply(lambda s: sorted({clean_str(x) for x in s if clean_str(x)}))
          .to_dict()
    )

    # Remove any categories that ended up empty (defensive)
    result = {cat: jobs for cat, jobs in grouped.items() if isinstance(jobs, list) and len(jobs) > 0}

    # Write JSON with UTF-8 and readable formatting
    try:
        with open(args.out, "w", encoding="utf-8") as f:
            json.dump(result, f, ensure_ascii=False, indent=2)
    except Exception as e:
        print(f"ERROR writing JSON: {e}", file=sys.stderr)
        return 1

    print(f"Wrote {len(result)} categories to {args.out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main()) 