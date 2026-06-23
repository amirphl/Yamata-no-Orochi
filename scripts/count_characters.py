#!/usr/bin/env python3

"""Standalone mirror of CampaignFlowImpl.countCharacters for scenario testing."""

from __future__ import annotations

import argparse
import json
from dataclasses import asdict, dataclass


SMS_PLATFORM = "sms"


def _has_value(value: str | None) -> bool:
    return value is not None and value.strip() != ""


def count_characters(
    text: str,
    ad_link: str | None = None,
    short_link_domain: str | None = None,
    platform: str = SMS_PLATFORM,
) -> int:
    if text == "":
        if platform == SMS_PLATFORM:
            return 6
        return 0

    text_to_count = text

    has_ad_link = _has_value(ad_link)
    has_short_link_domain = _has_value(short_link_domain)

    if has_ad_link and has_short_link_domain:
        short_link_text = short_link_domain.strip()
        short_link_text = short_link_text + "/123456"
        text_to_count = text_to_count.replace("{YOUR_LINK}", short_link_text)
    elif has_ad_link:
        resolved_ad_link = ad_link.strip()
        if "{uid}" in resolved_ad_link:
            resolved_ad_link = resolved_ad_link.replace("{uid}", "123456")
        text_to_count = text_to_count.replace("{YOUR_LINK}", resolved_ad_link)

    count = 0
    for char in text_to_count:
        if 32 <= ord(char) <= 126:
            count += 1
        else:
            count += 1

    if platform == SMS_PLATFORM:
        count += 6

    return count


def calculate_parts(
    content: str | None,
    ad_link: str | None = None,
    short_link_domain: str | None = None,
    platform: str = SMS_PLATFORM,
) -> int:
    if platform != SMS_PLATFORM:
        return 1

    if content is None or content == "":
        return 1

    char_count = count_characters(content, ad_link, short_link_domain, platform)

    if char_count <= 70:
        return 1
    if char_count <= 132:
        return 2
    if char_count <= 198:
        return 3
    if char_count <= 264:
        return 4
    if char_count <= 330:
        return 5
    return 6


@dataclass
class ScenarioResult:
    text: str
    ad_link: str | None
    short_link_domain: str | None
    platform: str
    expanded_text: str
    character_count: int
    parts: int


def expand_text(text: str, ad_link: str | None, short_link_domain: str | None) -> str:
    text_to_count = text
    has_ad_link = _has_value(ad_link)
    has_short_link_domain = _has_value(short_link_domain)

    if has_ad_link and has_short_link_domain:
        short_link_text = short_link_domain.strip()
        if not short_link_text.startswith("https://"):
            short_link_text = "https://" + short_link_text
        return text_to_count.replace("{YOUR_LINK}", short_link_text)

    if has_ad_link:
        resolved_ad_link = ad_link.strip()
        if "{uid}" in resolved_ad_link:
            resolved_ad_link = resolved_ad_link.replace("{uid}", "123456")
        return text_to_count.replace("{YOUR_LINK}", resolved_ad_link)

    return text_to_count


def build_result(
    text: str,
    ad_link: str | None,
    short_link_domain: str | None,
    platform: str,
) -> ScenarioResult:
    return ScenarioResult(
        text=text,
        ad_link=ad_link,
        short_link_domain=short_link_domain,
        platform=platform,
        expanded_text=expand_text(text, ad_link, short_link_domain),
        character_count=count_characters(text, ad_link, short_link_domain, platform),
        parts=calculate_parts(text, ad_link, short_link_domain, platform),
    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Standalone tester for campaign countCharacters/calculateParts logic."
    )
    parser.add_argument("--text", default="", help="Campaign text content.")
    parser.add_argument("--ad-link", default=None, help="Ad link used for {YOUR_LINK}.")
    parser.add_argument(
        "--short-link-domain",
        default=None,
        help="Short link domain. If set with ad-link, it overrides ad-link substitution.",
    )
    parser.add_argument(
        "--platform",
        default=SMS_PLATFORM,
        help="Campaign platform. Default: sms",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        help="Print machine-readable JSON output.",
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    result = build_result(
        text=args.text,
        ad_link=args.ad_link,
        short_link_domain=args.short_link_domain,
        platform=args.platform,
    )

    if args.json:
        print(json.dumps(asdict(result), ensure_ascii=False, indent=2))
        return

    print(f"platform: {result.platform}")
    print(f"text: {result.text}")
    print(f"expanded_text: {result.expanded_text}")
    print(f"character_count: {result.character_count}")
    print(f"parts: {result.parts}")


if __name__ == "__main__":
    main()
