#!/usr/bin/env python3

import os
import csv
import json
import time
import math
import random
import sys
import logging
import re
from typing import Any, Dict, List, Optional

try:
	from openai import OpenAI  # openai>=1.x
except Exception:  # pragma: no cover
	OpenAI = None  # type: ignore

# Max numbers (rows) to process; 0 means process all rows
MAX_NUMBERS_TO_PROCESS_DEFAULT = 1000


def setup_logging() -> None:
	level_name = os.environ.get("LOG_LEVEL", "INFO").upper()
	level = getattr(logging, level_name, logging.INFO)
	logging.basicConfig(
		level=level,
		format="%(asctime)s %(levelname)s %(message)s",
	)
	logging.getLogger("httpx").setLevel(logging.WARNING)


def configure_csv_field_limit() -> None:
	# Allow overriding via env; default to a large safe value for many platforms
	# Use min(sys.maxsize, 2**31-1) to avoid OverflowError on platforms with smaller C long
	default_limit = min(sys.maxsize, 2_147_483_647)
	limit_env = os.environ.get("CSV_FIELD_SIZE_LIMIT")
	try:
		limit = int(limit_env) if limit_env else default_limit
		csv.field_size_limit(limit)
		logging.debug("csv.field_size_limit set to %d", limit)
	except Exception:
		# Final fallback
		csv.field_size_limit(2_147_483_647)
		logging.debug("csv.field_size_limit set to fallback %d", 2_147_483_647)


def load_prompt(prompt_path: str) -> str:
	with open(prompt_path, "r", encoding="utf-8") as f:
		return f.read().strip()


def parse_texts_cell(cell: str) -> List[str]:
	if cell is None:
		return []
	cell_str = cell.strip()
	if not cell_str:
		return []
	# Attempt JSON parsing first
	try:
		parsed = json.loads(cell_str)
		if isinstance(parsed, list):
			return [str(x) for x in parsed]
		# If not list, fall through
	except Exception:
		pass
	# Fallback to Python literal eval (safer subset via json after replacing quotes if needed)
	# Attempt to coerce typical malformed JSON (single quotes) to valid JSON
	replaced = cell_str.replace("'", '"')
	try:
		parsed2 = json.loads(replaced)
		if isinstance(parsed2, list):
			return [str(x) for x in parsed2]
	except Exception:
		pass
	# As a last resort, return a naive split; better than losing everything
	return [s.strip().strip('"') for s in cell_str.strip('[]').split(",") if s.strip()]


def parse_openai_answer(answer: str) -> Dict[str, Any]:
	"""Parse the new format OpenAI answer with the updated structure."""
	result: Dict[str, Any] = {
		"brand_name": None,
		"number_type": None,  # نوع سرشماره
		"customer_segment": None,
		"customer_description": None,
		"customer_website": None,
		"customer_persona": None,
		"customer_tags_raw": None,
	}
	
	# Add tag columns dynamically (now only 1-3 tags, no scores)
	for i in range(1, 4):
		result[f"tag{i}"] = None
	
	if not answer:
		return result
	
	text = answer.replace("\r", "")
	
	def extract_value(label: str) -> Optional[str]:
		"""Extract value for a given label using multiple patterns."""
		patterns = [
			r"-\s*\*\*" + re.escape(label) + r":\*\*\s*(.+)$",
			r"\*\*" + re.escape(label) + r":\*\*\s*(.+)$",
			re.escape(label) + r"\s*[:：]\s*(.+)$",
		]
		for pattern in patterns:
			match = re.search(pattern, text, re.MULTILINE)
			if match:
				return match.group(1).strip()
		return None
	
	# Extract main fields
	result["brand_name"] = extract_value("نام برند")
	result["number_type"] = extract_value("نوع سرشماره")
	result["customer_segment"] = extract_value("سگمنت مشتری")
	result["customer_description"] = extract_value("توضیح مشتری")
	result["customer_website"] = extract_value("وب‌سایت مشتری") or extract_value("وب سایت مشتری")
	result["customer_persona"] = extract_value("پرسونای مخاطب مشتری")
	
	# Check if this is تبلیغاتی - if so, we don't need to extract tags
	if result["number_type"] == "سرشماره تبلیغاتی":
		return result
	
	# Extract tags - use simpler line-by-line approach (no scores)
	lines = text.split('\n')
	tag_section_lines = []
	in_tag_section = False
	
	for line in lines:
		if "تگ‌های مخاطب مشتری:" in line:
			in_tag_section = True
			continue
		elif in_tag_section and line.strip().startswith('- **'):
			# We've reached the next section, stop
			break
		elif in_tag_section and line.strip():
			tag_section_lines.append(line.strip())
	
	if tag_section_lines:
		# Store the raw tags text
		result["customer_tags_raw"] = '\n'.join(tag_section_lines)
		
		# Extract individual tags from the section (no scores)
		tag_pattern = re.compile(r"-\s*(.+?)(?:\s*\(نمره ارتباط:\s*[0-9\u06F0-\u06F9]+\))?\s*$")
		tags = []
		
		for line in tag_section_lines:
			match = tag_pattern.search(line)
			if match:
				tag_text = match.group(1).strip()
				tags.append(tag_text)
		
		# Fill tag columns with extracted data (only up to 3 tags now, no scores)
		for i, tag_text in enumerate(tags, 1):
			if i <= 3:  # Safety limit - now only 3 tags
				result[f"tag{i}"] = tag_text
	
	return result


def append_parsed_csv(output_csv_path: str, header: List[str], row: Dict[str, Any]) -> None:
	file_exists = os.path.exists(output_csv_path)
	with open(output_csv_path, "a", encoding="utf-8", newline="") as f:
		writer = csv.DictWriter(f, fieldnames=header)
		if not file_exists:
			writer.writeheader()
		writer.writerow(row)


def process_csv(
	csv_path: str,
	prompt_path: str,
	jsonl_output_path: str,
	json_output_path: str,
	limit_per_row: int = 100,
	model: Optional[str] = None,
	max_rows: int = MAX_NUMBERS_TO_PROCESS_DEFAULT,
	parsed_csv_output_path: Optional[str] = None,
) -> None:
	if OpenAI is None:
		raise RuntimeError("The 'openai' package (v1.x) is required. Install with: pip install openai")

	api_key = os.environ.get("OPENAI_API_KEY")
	if not api_key:
		raise RuntimeError("Please set the OPENAI_API_KEY environment variable.")

	client = OpenAI()
	model_name = model or os.environ.get("OPENAI_MODEL", "gpt-4o-mini")
	base_prompt = load_prompt(prompt_path)

	logging.info(
		"Starting processing. csv=%s prompt=%s model=%s limit_per_row=%d max_rows=%s",
		csv_path,
		prompt_path,
		model_name,
		limit_per_row,
		"all" if max_rows == 0 else str(max_rows),
	)

	configure_csv_field_limit()

	# Prepare outputs
	# JSONL for incremental progress
	jsonl_file = open(jsonl_output_path, "w", encoding="utf-8")
	all_rows: List[Dict[str, Any]] = []

	# Parsed CSV header
	parsed_header = [
		"row_index",
		"number",
		"official_name",
		"brand_name",
		"name_surname",
		"start_date",
		"haveText",
		"out_brand_name",
		"number_type",  # New field
		"customer_segment",
		"customer_description",
		"customer_website",
		"customer_persona",
		"customer_tags_json",
	]
	parsed_csv_path = parsed_csv_output_path or os.path.join(os.path.dirname(json_output_path), "openai_results_parsed.csv")

	with open(csv_path, "r", encoding="utf-8-sig", newline="") as f:
		reader = csv.DictReader(f)
		for row_idx, row in enumerate(reader, start=1):
			# if row_idx < 1000:
			# 	continue

			if max_rows and row_idx > max_rows:
				logging.info("Reached max_rows=%d. Stopping further processing.", max_rows)
				break

			texts_raw = row.get("texts", "")
			texts = parse_texts_cell(texts_raw)
			texts = texts[: max(0, limit_per_row)]

			logging.info(
				"Row %d number=%s texts_in_row=%d processing_first=%d",
				row_idx,
				row.get("number"),
				len(parse_texts_cell(texts_raw)),
				len(texts),
			)

			logging.info("texts: %s", texts)

			# Build user message in the new JSON format expected by the prompt
			user_message = f"{base_prompt}\n\nورودی:\n{{\n  \"number\": \"{row.get('number')}\",\n  \"official_name\": \"{row.get('official_name')}\",\n  \"brand_name\": \"{row.get('brand_name')}\",\n  \"name_surname\": \"{row.get('name_surname')}\",\n  \"start_date\": \"{row.get('start_date')}\",\n  \"messages\": {json.dumps(texts, ensure_ascii=False)}\n}}"
			logging.info("user message: %s", user_message)
			
			messages = [
				{"role": "system", "content": "You are a helpful assistant."},
				{"role": "user", "content": user_message},
			]

			try:
				answer = call_openai_with_retry(client, model_name, messages)
			except Exception as e:
				logging.exception("OpenAI call error on row=%d", row_idx)
				answer = f"ERROR: {e}"

			# Write an incremental record to JSONL
			jsonl_file.write(json.dumps({
				"row_index": row_idx,
				"number": row.get("number"),
				"texts": texts,
				"openai_result": answer,
			}, ensure_ascii=False) + "\n")
			jsonl_file.flush()

			# Parse answer into structured fields and append to parsed CSV
			parsed = parse_openai_answer(answer)
			parsed_row = {
				"row_index": row_idx,
				"number": row.get("number"),
				"official_name": row.get("official_name"),
				"brand_name": row.get("brand_name"),
				"name_surname": row.get("name_surname"),
				"start_date": row.get("start_date"),
				"haveText": row.get("haveText"),
				"out_brand_name": parsed.get("brand_name"),
				"number_type": parsed.get("number_type"),  # New field
				"customer_segment": parsed.get("customer_segment"),
				"customer_description": parsed.get("customer_description"),
				"customer_website": parsed.get("customer_website"),
				"customer_persona": parsed.get("customer_persona"),
				"customer_tags_json": json.dumps(parsed.get("customer_tags_raw", ""), ensure_ascii=False),
			}
			append_parsed_csv(parsed_csv_path, parsed_header, parsed_row)

			# Also keep grouped JSON for completeness
			all_rows.append({
				"row_index": row_idx,
				"number": row.get("number"),
				"texts": texts,
				"openai_result": answer,
				"parsed": parsed,
			})

	jsonl_file.close()

	# Write grouped JSON array at the end
	with open(json_output_path, "w", encoding="utf-8") as jf:
		json.dump(all_rows, jf, ensure_ascii=False, indent=2)

	logging.info("Finished. Wrote %s and %s and appended parsed rows to %s", jsonl_output_path, json_output_path, parsed_csv_path)


def call_openai_with_retry(client: Any, model: str, messages: List[Dict[str, str]], max_retries: int = 5, timeout_seconds: int = 60) -> str:
	attempt = 0
	while True:
		try:
			resp = client.chat.completions.create(
				model=model,
				messages=messages,
				max_tokens=512,
				temperature=0.2,
				top_p=1.0,
				timeout=timeout_seconds,
			)
			content = resp.choices[0].message.content if resp and resp.choices else ""
			return content or ""
		except Exception as e:  # Handle rate limits, timeouts, transient failures
			attempt += 1
			if attempt > max_retries:
				logging.error("OpenAI call failed after %d attempts: %s", attempt - 1, e)
				raise
			# Exponential backoff with jitter
			sleep_seconds = min(2 ** attempt + random.uniform(0, 1), 30)
			logging.warning("OpenAI call failed (attempt %d). Retrying in %.2fs...", attempt, sleep_seconds)
			time.sleep(sleep_seconds)


def main(argv: List[str]) -> int:
	setup_logging()
	# Defaults to filenames in the current workspace directory
	base_dir = os.getcwd()
	csv_path = os.path.join(base_dir, "final_result_withText.csv")
	prompt_path = os.path.join(base_dir, "prompt.txt")
	jsonl_output_path = os.path.join(base_dir, "openai_results.jsonl")
	json_output_path = os.path.join(base_dir, "openai_results.json")
	parsed_csv_output_path = os.path.join(base_dir, "openai_results_parsed.csv")

	# Optional CLI overrides
	args = argv[1:]
	for arg in args:
		if arg.startswith("--csv="):
			csv_path = arg.split("=", 1)[1]
		elif arg.startswith("--prompt="):
			prompt_path = arg.split("=", 1)[1]
		elif arg.startswith("--jsonl="):
			jsonl_output_path = arg.split("=", 1)[1]
		elif arg.startswith("--json="):
			json_output_path = arg.split("=", 1)[1]
		elif arg.startswith("--parsed-csv="):
			parsed_csv_output_path = arg.split("=", 1)[1]
		elif arg.startswith("--model="):
			os.environ["OPENAI_MODEL"] = arg.split("=", 1)[1]
		elif arg.startswith("--limit="):
			os.environ["ROW_TEXT_LIMIT"] = arg.split("=", 1)[1]
		elif arg.startswith("--max-rows="):
			os.environ["MAX_NUMBERS_TO_PROCESS"] = arg.split("=", 1)[1]
		elif arg.startswith("--csv-field-limit="):
			os.environ["CSV_FIELD_SIZE_LIMIT"] = arg.split("=", 1)[1]

	limit = 100
	try:
		limit = int(os.environ.get("ROW_TEXT_LIMIT", "20"))
	except ValueError:
		limit = 100

	try:
		max_rows = int(os.environ.get("MAX_NUMBERS_TO_PROCESS", str(MAX_NUMBERS_TO_PROCESS_DEFAULT)))
	except ValueError:
		max_rows = MAX_NUMBERS_TO_PROCESS_DEFAULT

	process_csv(
		csv_path=csv_path,
		prompt_path=prompt_path,
		jsonl_output_path=jsonl_output_path,
		json_output_path=json_output_path,
		limit_per_row=limit,
		model=os.environ.get("OPENAI_MODEL"),
		max_rows=max_rows,
		parsed_csv_output_path=parsed_csv_output_path,
	)
	return 0


if __name__ == "__main__":
	sys.exit(main(sys.argv)) 
