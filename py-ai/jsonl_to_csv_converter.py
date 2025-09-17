#!/usr/bin/env python3

import json
import csv
import re
import sys
import os
from typing import Dict, Any, List, Optional


def parse_openai_result(openai_result: str) -> Dict[str, Any]:
    """Parse the new format OpenAI result string and extract structured fields."""
    result = {
        "brand_name": None,
        "number_type": None,  # New field: نوع سرشماره
        "customer_segment": None,
        "customer_description": None,
        "customer_website": None,
        "customer_persona": None,
        "customer_tags_raw": None,  # Raw tags text
    }
    
    # Add tag columns dynamically (now only 1-3 tags, no scores)
    for i in range(1, 4):
        result[f"tag{i}"] = None
    
    if not openai_result:
        return result
    
    text = openai_result.replace("\r", "")
    
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
    result["number_type"] = extract_value("نوع سرشماره")  # New field
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


def convert_jsonl_to_csv(jsonl_path: str, csv_path: str) -> None:
    """Convert JSONL file to CSV with parsed OpenAI results."""
    
    # First pass: determine the maximum number of tags across all rows
    max_tags = 0
    with open(jsonl_path, 'r', encoding='utf-8') as jsonl_file:
        for line in jsonl_file:
            try:
                data = json.loads(line.strip())
                if "openai_result" in data:
                    parsed = parse_openai_result(data["openai_result"])
                    # Count non-None tags (no scores)
                    tag_count = sum(1 for i in range(1, 4) if parsed.get(f"tag{i}") is not None)
                    max_tags = max(max_tags, tag_count)
            except:
                continue
    
    print(f"Maximum number of tags found: {max_tags}")
    
    # Define CSV columns based on max tags found
    csv_columns = [
        "row_index",
        "number",
        "brand_name",
        "number_type",  # New column
        "customer_segment", 
        "customer_description",
        "customer_website",
        "customer_persona",
        "customer_tags_raw",  # Raw tags text column
    ]
    
    # Add tag columns (only tags, no scores)
    for i in range(1, max_tags + 1):
        csv_columns.extend([f"tag{i}"])
    
    print(f"CSV will have {len(csv_columns)} columns")
    
    with open(jsonl_path, 'r', encoding='utf-8') as jsonl_file, \
         open(csv_path, 'w', encoding='utf-8', newline='') as csv_file:
        
        writer = csv.DictWriter(csv_file, fieldnames=csv_columns)
        writer.writeheader()
        
        for line_num, line in enumerate(jsonl_file, 1):
            try:
                # Parse JSON line
                data = json.loads(line.strip())
                
                # Extract basic fields
                row = {
                    "row_index": data.get("row_index"),
                    "number": data.get("number"),
                }
                
                # Parse OpenAI result if present
                if "openai_result" in data:
                    parsed_result = parse_openai_result(data["openai_result"])
                    
                    # Add main fields
                    for field in ["brand_name", "number_type", "customer_segment", "customer_description", 
                                "customer_website", "customer_persona", "customer_tags_raw"]:
                        row[field] = parsed_result.get(field)
                    
                    # Add tag fields (only up to max_tags, no scores)
                    for i in range(1, max_tags + 1):
                        row[f"tag{i}"] = parsed_result.get(f"tag{i}")
                else:
                    # Fill with None if no OpenAI result
                    for col in csv_columns[2:]:  # Skip row_index and number
                        row[col] = None
                
                writer.writerow(row)
                
            except json.JSONDecodeError as e:
                print(f"Error parsing line {line_num}: {e}")
                continue
            except Exception as e:
                print(f"Error processing line {line_num}: {e}")
                continue
    
    print(f"Conversion completed. Output saved to: {csv_path}")


def main():
    if len(sys.argv) != 3:
        print("Usage: python3 jsonl_to_csv_converter.py <input_jsonl_file> <output_csv_file>")
        print("Example: python3 jsonl_to_csv_converter.py openai_results.jsonl parsed_results.csv")
        sys.exit(1)
    
    input_file = sys.argv[1]
    output_file = sys.argv[2]
    
    if not os.path.exists(input_file):
        print(f"Error: Input file '{input_file}' not found.")
        sys.exit(1)
    
    try:
        convert_jsonl_to_csv(input_file, output_file)
    except Exception as e:
        print(f"Error during conversion: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main() 