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

# Constant available audience per instruction
agg["available_audience"] = 10000

# Prepare output columns
out = agg.rename(columns={"level3_final": "level3"})[[
    "level1", "level2", "level3", "tags", "available_audience"
]]

# Persist CSV (UTF-8 with BOM for Excel compatibility)
out.to_csv(OUTPUT_CSV, index=False, encoding="utf-8-sig")

print(f"Done. Wrote {len(out)} grouped rows to {OUTPUT_CSV}")
