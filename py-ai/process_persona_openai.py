#!/usr/bin/env python3
import argparse
import os
import sys
from typing import Optional
import csv
import re


def read_file_text(path: str) -> str:
    with open(path, "r", encoding="utf-8") as f:
        return f.read()


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Read 'Customer Persona.xlsx', substitute column C into 'AI command.txt' placeholder, send to OpenAI, and print outputs."
    )
    parser.add_argument(
        "--excel",
        default="Customer Persona.xlsx",
        help="Path to the Excel file (default: Customer Persona.xlsx)",
    )
    parser.add_argument(
        "--ai-command",
        default="AI command.txt",
        help="Path to the AI command template file (default: AI command.txt)",
    )
    parser.add_argument(
        "--output-csv",
        default="persona_results.csv",
        help="Path to the output CSV file (default: persona_results.csv)",
    )
    parser.add_argument(
        "--column-index",
        type=int,
        default=2,
        help="Zero-based column index to use from Excel rows (default: 2, which is column C)",
    )
    parser.add_argument(
        "--model",
        default=os.getenv("OPENAI_MODEL", "gpt-4o-mini"),
        help="OpenAI model to use (default: env OPENAI_MODEL or gpt-4o-mini)",
    )
    parser.add_argument(
        "--max-rows",
        type=int,
        default=None,
        help="Max number of rows to process (optional)",
    )
    parser.add_argument(
        "--skip-header",
        action="store_true",
        help="Skip the first data row (useful if the first row is a header)",
    )
    parser.add_argument(
        "--start-index",
        type=int,
        default=None,
        help="Zero-based row index in the Excel sheet to start processing from",
    )

    args = parser.parse_args()

    excel_path = os.path.abspath(args.excel)
    ai_cmd_path = os.path.abspath(args.ai_command)
    output_csv_path = os.path.abspath(args.output_csv)

    if not os.path.isfile(excel_path):
        print(f"ERROR: Excel file not found: {excel_path}", file=sys.stderr)
        sys.exit(1)
    if not os.path.isfile(ai_cmd_path):
        print(f"ERROR: AI command template file not found: {ai_cmd_path}", file=sys.stderr)
        sys.exit(1)

    api_key = os.getenv("OPENAI_API_KEY")
    if not api_key:
        print("ERROR: OPENAI_API_KEY environment variable is not set.", file=sys.stderr)
        sys.exit(1)

    template_text = read_file_text(ai_cmd_path)
    placeholder = "Customer Persona.xlsx row"
    if placeholder not in template_text:
        print(
            f"WARNING: Placeholder '{placeholder}' not found in {ai_cmd_path}. Proceeding anyway.",
            file=sys.stderr,
        )

    try:
        import pandas as pd  # type: ignore
    except Exception as exc:
        print("ERROR: pandas is required. Install dependencies from requirements.txt", file=sys.stderr)
        print(str(exc), file=sys.stderr)
        sys.exit(1)

    try:
        # header=None ensures we treat the first row as data. Users can --skip-header if needed.
        df = pd.read_excel(excel_path, header=None, engine="openpyxl")
    except Exception:
        # Fallback without specifying engine
        df = pd.read_excel(excel_path, header=None)

    if df.shape[1] <= args.column_index:
        print(
            f"ERROR: Excel has {df.shape[1]} column(s), but --column-index={args.column_index} was requested.",
            file=sys.stderr,
        )
        sys.exit(1)

    start_row_index = 1 if args.skip_header and df.shape[0] > 0 else 0
    if args.start_index is not None:
        if args.start_index < 0 or args.start_index >= df.shape[0]:
            print(f"ERROR: --start-index must be in [0, {df.shape[0]-1}]. Got {args.start_index}", file=sys.stderr)
            sys.exit(1)
        start_row_index = args.start_index

    try:
        from openai import OpenAI  # type: ignore
    except Exception as exc:
        print("ERROR: openai SDK is required. Install dependencies from requirements.txt", file=sys.stderr)
        print(str(exc), file=sys.stderr)
        sys.exit(1)

    client = OpenAI()

    processed = 0
    rows_to_write = []

    def normalize_cell_value(cell):
        text = "" if cell is None else str(cell).strip()
        return "" if text == "" or text.lower() == "nan" else text

    def extract_numbered_lines(text):
        lines = text.splitlines()
        result = []
        pattern = re.compile(r'^\s*(?:\d+|[۰-۹]+)[\.\)\-]\s*')
        for line in lines:
            m = pattern.match(line)
            if m:
                # Strip the matched numeric prefix
                stripped = line[m.end():].strip()
                result.append(stripped)
        return result

    for row_index in range(start_row_index, df.shape[0]):
        if args.max_rows is not None and processed >= args.max_rows:
            break

        value = df.iat[row_index, args.column_index]
        if value is None:
            continue
        text_value = str(value).strip()
        if text_value == "" or text_value.lower() == "nan":
            continue

        prompt = template_text.replace(placeholder, text_value)

        try:
            response = client.chat.completions.create(
                model=args.model,
                messages=[{"role": "user", "content": prompt}],
                temperature=0.2,
            )
            output = response.choices[0].message.content if response.choices else ""
        except Exception as exc:
            output = f"ERROR calling OpenAI: {exc}"

        print(output)

        col0 = normalize_cell_value(df.iat[row_index, 0]) if df.shape[1] >= 1 else ""
        col1 = normalize_cell_value(df.iat[row_index, 1]) if df.shape[1] >= 2 else ""
        col2 = normalize_cell_value(df.iat[row_index, 2]) if df.shape[1] >= 3 else ""
        numbered_cells = extract_numbered_lines(output)
        rows_to_write.append([col0, col1, col2] + numbered_cells)

        processed += 1

    with open(output_csv_path, 'w', encoding='utf-8', newline='') as f:
        writer = csv.writer(f)
        writer.writerows(rows_to_write)
    print(f"Saved {len(rows_to_write)} rows to {output_csv_path}", file=sys.stderr)


if __name__ == "__main__":
    main() 