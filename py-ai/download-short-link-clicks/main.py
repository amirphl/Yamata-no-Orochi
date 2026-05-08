#!/usr/bin/env python3
"""
Download short-links clicks CSV for a scenario range [from, to) using an existing Admin access token.

Usage:
  pip install requests
  python main.py \
    --base-url http://localhost:8080 \
    --token <ADMIN_ACCESS_TOKEN> \
    --scenario-from 100 \
    --scenario-to 120 \
    --output clicks_100_120.csv

Notes:
- Requires a valid Admin access token (Bearer) with permission to access admin endpoints.
- The script will attempt to use the filename from Content-Disposition if --output is not provided.
"""

import argparse
import sys
from typing import Tuple

import requests


def download_clicks_csv_range(
	session: requests.Session,
	base_url: str,
	token: str,
	scenario_from: int,
	scenario_to: int,
	timeout: int,
	verify_ssl: bool,
) -> Tuple[str, bytes]:
	"""Call admin range endpoint and return (filename, content)."""
	if scenario_from <= 0 or scenario_to <= 0:
		raise ValueError("scenario_from and scenario_to must be > 0")
	if scenario_to <= scenario_from:
		raise ValueError("scenario_to must be greater than scenario_from")

	url = f"{base_url.rstrip('/')}/api/v1/admin/short-links/download-with-clicks-range"
	headers = {"Authorization": f"Bearer {token}"}
	payload = {"scenario_from": scenario_from, "scenario_to": scenario_to}

	r = session.post(url, json=payload, headers=headers, timeout=timeout, verify=verify_ssl)
	r.raise_for_status()

	# Default filename fallback
	filename = f"short_links_with_clicks_scenarios_{scenario_from}_to_{scenario_to}.csv"
	cd = r.headers.get("Content-Disposition") or r.headers.get("content-disposition")
	if cd and "filename=" in cd:
		try:
			filename = cd.split("filename=", 1)[1].strip().strip('"')
		except Exception:
			pass
	return filename, r.content


def main() -> int:
	parser = argparse.ArgumentParser(description="Download short-links clicks CSV for a scenario range [from, to) using an Admin access token.")
	parser.add_argument("--base-url", default="http://localhost:8080", help="API base URL, e.g. http://localhost:8080")
	parser.add_argument("--token", required=True, help="Admin access token (Bearer)")
	parser.add_argument("--scenario-from", type=int, required=True, help="Scenario start (inclusive)")
	parser.add_argument("--scenario-to", type=int, required=True, help="Scenario end (exclusive)")
	parser.add_argument("--output", default="", help="Output file path (optional)")
	parser.add_argument("--timeout", type=int, default=60, help="HTTP timeout seconds")
	parser.add_argument("--insecure", action="store_true", help="Disable TLS certificate verification")
	args = parser.parse_args()

	verify_ssl = not args.insecure

	with requests.Session() as session:
		filename, content = download_clicks_csv_range(
			session=session,
			base_url=args.base_url,
			token=args.token,
			scenario_from=args.scenario_from,
			scenario_to=args.scenario_to,
			timeout=args.timeout,
			verify_ssl=verify_ssl,
		)

		out_path = args.output or filename
		with open(out_path, "wb") as f:
			f.write(content)
		print(f"Saved CSV to: {out_path}")

	return 0


if __name__ == "__main__":
	sys.exit(main())