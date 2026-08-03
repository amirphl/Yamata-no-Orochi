#!/usr/bin/env python3
"""Emit only allowlisted COPY sections from a PostgreSQL plain-text dump.

All SQL and psql meta-commands outside COPY data are discarded. This makes a
data-only dump safe to feed to psql without trusting arbitrary statements that
may have been added to the file.
"""

from __future__ import annotations

import argparse
import re
import sys
from contextlib import nullcontext
from pathlib import Path

COPY_HEADER = re.compile(
    r"^COPY public\.([a-z_][a-z0-9_]*) \(([^\r\n]*)\) FROM stdin;$"
)
IDENTIFIER = re.compile(r"^[a-z_][a-z0-9_]*$")


class DumpValidationError(ValueError):
    pass


def extract_copy_sections(
    source: Path | None, allowed_tables: set[str], output: object = sys.stdout
) -> set[str]:
    if not allowed_tables or any(not IDENTIFIER.fullmatch(t) for t in allowed_tables):
        raise DumpValidationError("allowed table names must be simple PostgreSQL identifiers")

    found: set[str] = set()
    current_table: str | None = None
    source_context = (
        source.open("r", encoding="utf-8", errors="strict", newline="")
        if source is not None
        else nullcontext(sys.stdin)
    )
    with source_context as handle:
        for line_number, raw_line in enumerate(handle, start=1):
            line = raw_line.rstrip("\r\n")
            if "\x00" in line:
                raise DumpValidationError(f"NUL byte at line {line_number}")

            if current_table is not None:
                print(line, file=output)
                if line == r"\.":
                    current_table = None
                continue

            match = COPY_HEADER.fullmatch(line)
            if match is None:
                continue
            table = match.group(1)
            if table not in allowed_tables:
                raise DumpValidationError(
                    f"dump contains non-allowlisted COPY table public.{table}"
                )
            if table in found:
                raise DumpValidationError(f"duplicate COPY section for public.{table}")
            columns = [column.strip() for column in match.group(2).split(",")]
            if not columns or any(not IDENTIFIER.fullmatch(column) for column in columns):
                raise DumpValidationError(
                    f"invalid COPY column list for public.{table} at line {line_number}"
                )
            if len(columns) != len(set(columns)):
                raise DumpValidationError(
                    f"duplicate COPY column for public.{table} at line {line_number}"
                )
            found.add(table)
            current_table = table
            print(line, file=output)

    if current_table is not None:
        raise DumpValidationError(f"unterminated COPY section for public.{current_table}")
    missing = allowed_tables - found
    if missing:
        raise DumpValidationError(
            "dump is missing COPY sections for: " + ", ".join(sorted(missing))
        )
    return found


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("dump_file", help="plain dump path, or - for stdin")
    parser.add_argument("tables", nargs="+")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        if args.dump_file == "-" and hasattr(sys.stdin, "reconfigure"):
            sys.stdin.reconfigure(encoding="utf-8", errors="strict", newline="")
        source = None if args.dump_file == "-" else Path(args.dump_file)
        extract_copy_sections(source, set(args.tables))
    except (DumpValidationError, OSError, UnicodeError) as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
