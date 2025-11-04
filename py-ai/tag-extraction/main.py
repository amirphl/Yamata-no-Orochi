import pandas as pd
import re

# --- config ---
EXCEL_PATH = "Customer Segmentation.xlsx"          # <- change to your file path
STAT_SHEET = "stat"
TAGS_SHEET = "tags"
OUTPUT_CSV = "stat_with_tags.csv"

# --- helpers ---
PERSIAN_DIGITS = "۰۱۲۳۴۵۶۷۸۹"
EN_DIGITS      = "0123456789"
DIGIT_MAP = {ord(p): e for p, e in zip(PERSIAN_DIGITS, EN_DIGITS)}

def to_int(val):
    """Convert strings like '23,825,515' (or Persian digits) to int; keep NA if empty."""
    if pd.isna(val):
        return pd.NA
    s = str(val).translate(DIGIT_MAP)
    s = re.sub(r"[^\d]", "", s)  # strip commas, spaces, etc.
    return int(s) if s else pd.NA

def clean_str(s):
    return s.strip() if isinstance(s, str) else s

# --- load ---
stat = pd.read_excel(EXCEL_PATH, sheet_name=STAT_SHEET, dtype=str)
tags = pd.read_excel(EXCEL_PATH, sheet_name=TAGS_SHEET, dtype=str)

# --- normalize key columns for safe matching ---
# stat: category, job
stat["category"] = stat["category"].map(clean_str)
stat["job"] = stat["job"].map(clean_str)

# tags: category, jobs -> rename to job; id as string for joining/aggregation
tags = tags.rename(columns={"jobs": "job"})
tags["category"] = tags["category"].map(clean_str)
tags["job"] = tags["job"].map(clean_str)
tags["id"] = tags["id"].map(lambda x: clean_str(str(x)) if pd.notna(x) else x)

# --- aggregate tag ids per (category, job) ---
# Some (category, job) may have multiple tag rows
tag_map = (
    tags.dropna(subset=["category", "job", "id"])
        .groupby(["category", "job"], as_index=False)["id"]
        .agg(lambda x: sorted(set(x)))   # de-dup + sort
        .rename(columns={"id": "tag ids"})
)

# --- merge into stat ---
merged = stat.merge(tag_map, on=["category", "job"], how="left")

# join lists of ids into a single string (e.g., "14522;14526")
merged["tag ids"] = merged["tag ids"].apply(
    lambda v: ";".join(v) if isinstance(v, list) else ""
)

# --- clean numeric columns (optional but recommended) ---
# Adjust these names if your headers differ exactly:
count_all_col = "audience_count_all"
count_33_col  = "audience_count_33$"

if count_all_col in merged.columns:
    merged[count_all_col] = merged[count_all_col].map(to_int)

if count_33_col in merged.columns:
    merged[count_33_col] = merged[count_33_col].map(to_int)

# --- select & order output columns ---
out_cols = ["category", "job", "tag ids", count_all_col, count_33_col]
out = merged[out_cols]

# --- save CSV (UTF-8 with BOM to play nice with Excel/Persian text) ---
out.to_csv(OUTPUT_CSV, index=False, encoding="utf-8-sig")

print(f"Done. Wrote {len(out)} rows to {OUTPUT_CSV}")
