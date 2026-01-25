import pandas as pd

# --- config ---
EXCEL_PATH = "Customer Segmentation.xlsx"  # input Excel path
SHEET_NAME = "New Stat"                    # required sheet name
OUTPUT_CSV = "stat_with_tags.csv"         # output CSV for review/debug

# --- helpers ---

def clean_str(s: object) -> str:
    if s is None or (isinstance(s, float) and pd.isna(s)):
        return ""
    return str(s).strip()

# --- load ---
df = pd.read_excel(EXCEL_PATH, sheet_name=SHEET_NAME, dtype=str)

# Expected columns (case-sensitive as provided)
# id	Src_number	Layer1_category	Layer2_category	Layer3_Category

# Normalize/rename for internal use if necessary
col_map = {
    "id": "tag_id",
    "Layer1_category": "level1",
    "Layer2_category": "level2",
    "Layer3_Category": "level3",
}

missing = [c for c in col_map.keys() if c not in df.columns]
if missing:
    raise SystemExit(f"Missing required columns in sheet '{SHEET_NAME}': {missing}")

work = df[list(col_map.keys())].rename(columns=col_map).copy()
work["tag_id"] = work["tag_id"].map(clean_str)
work["level1"] = work["level1"].map(clean_str)
work["level2"] = work["level2"].map(clean_str)
work["level3"] = work["level3"].map(clean_str)

# Fallback: if level3 empty, use level2
work["level3_final"] = work.apply(
    lambda r: r["level3"] if r["level3"] else r["level2"], axis=1
)

# Aggregate tag ids per (level1, level2, level3_final)
# Skip empty tag ids
agg = (
    work.groupby(["level1", "level2", "level3_final"], as_index=False)["tag_id"]
        .agg(lambda xs: sorted({clean_str(x) for x in xs if clean_str(x)}))
        .rename(columns={"tag_id": "tags"})
)

# Filter out groups that have no tag ids
agg = agg[agg["tags"].map(lambda v: isinstance(v, list) and len(v) > 0)].copy()

# Load audience stats (layer counts)
try:
    stats_df = pd.read_excel(EXCEL_PATH, sheet_name="layersStat", dtype=str)
except Exception:
    stats_df = None

available = None
if stats_df is not None:
    stat_cols = {
        "layer1_category": "level1",
        "layer2_category": "level2",
        "layer3_category": "level3",
        "distinct_users": "distinct_users",
        "calculated_at": "calculated_at",
    }
    missing_stats = [c for c in stat_cols.keys() if c not in stats_df.columns]
    if not missing_stats:
        stats = stats_df[list(stat_cols.keys())].rename(columns=stat_cols).copy()
        stats["level1"] = stats["level1"].map(clean_str)
        stats["level2"] = stats["level2"].map(clean_str)
        stats["level3"] = stats["level3"].map(clean_str)
        stats["distinct_users"] = pd.to_numeric(stats["distinct_users"], errors="coerce").fillna(0).astype(int)
        stats["calculated_at"] = pd.to_datetime(stats["calculated_at"], errors="coerce")
        # Keep the latest calculated row per level combo
        stats = (
            stats.sort_values(["level1", "level2", "level3", "calculated_at"])
            .groupby(["level1", "level2", "level3"], as_index=False)
            .tail(1)
        )
        available = stats[["level1", "level2", "level3", "distinct_users"]]

# Prepare output columns and attach available audience from stats if present
out = agg.rename(columns={"level3_final": "level3"})[[
    "level1", "level2", "level3", "tags"
]]

if available is not None:
    out = out.merge(available, how="left", on=["level1", "level2", "level3"])
    out = out.rename(columns={"distinct_users": "available_audience"})
    out["available_audience"] = out["available_audience"].fillna(0).astype(int)
else:
    out["available_audience"] = 0

# --- Attach level2 metadata from definitions sheet (if present) ---
try:
    defs = pd.read_excel(EXCEL_PATH, sheet_name="definitions", dtype=str)
except Exception:
    defs = None

if defs is not None:
    # Expected Persian columns: 'شناسه فارسی', 'توضیح یک خطی', 'شمول', 'عدم شمول'
    # Normalize column names to exact strings as they appear in sheet
    keys = [c for c in defs.columns]
    if "شناسه فارسی" in keys:
        meta_map = {}
        for _, r in defs.iterrows():
            key = clean_str(r.get("شناسه فارسی"))
            if not key:
                continue
            meta_map[key] = {
                "one_line": clean_str(r.get("توضیح یک خطی")),
                "inclusion": clean_str(r.get("شمول")),
                "exclusion": clean_str(r.get("عدم شمول")),
            }

        # Map metadata into out DataFrame columns
        def lookup_meta(l2: str, field: str) -> str:
            if not l2:
                return ""
            m = meta_map.get(l2)
            return m.get(field, "") if isinstance(m, dict) else ""

        out["level2_one_line"] = out["level2"].map(lambda v: lookup_meta(v, "one_line"))
        out["level2_inclusion"] = out["level2"].map(lambda v: lookup_meta(v, "inclusion"))
        out["level2_exclusion"] = out["level2"].map(lambda v: lookup_meta(v, "exclusion"))
    else:
        # definitions sheet missing expected key column; add empty metadata columns
        out["level2_one_line"] = ""
        out["level2_inclusion"] = ""
        out["level2_exclusion"] = ""
else:
    # definitions sheet not found; create empty metadata columns
    out["level2_one_line"] = ""
    out["level2_inclusion"] = ""
    out["level2_exclusion"] = ""

# Persist CSV (UTF-8 with BOM for Excel compatibility)
out.to_csv(OUTPUT_CSV, index=False, encoding="utf-8-sig")

print(f"Done. Wrote {len(out)} grouped rows to {OUTPUT_CSV}")
