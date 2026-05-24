"""
Daily SSL certificate watcher for beta environment.
- Parses nginx config(s) to find certificate paths (ssl_certificate directives).
- Supports multiple conf files and includes; expands ${VAR} using environment.
- Checks expiration dates; if expired or within threshold days, sends SMS alert.

Environment variables:
  CERT_ALERT_PHONE              (required) target phone number, e.g. 98912xxxxxxx
  CERT_ALERT_THRESHOLD_DAYS     (default: 7)
  CERT_ALERT_CONF_PATH          (default: /etc/nginx/sites-available/yamata-beta.conf)
  CERT_ALERT_CONF_GLOBS         (default: "/etc/nginx/sites-enabled/*.conf,/etc/nginx/conf.d/*.conf")
  CERT_ALERT_CERT_PATHS         optional, comma-separated extra cert paths
  CERT_ALERT_LOG_LEVEL          (default: INFO)
  SMS_PROVIDER_DOMAIN           (required) SMS provider host (no scheme)
  SMS_API_KEY                   (required)
  SMS_SOURCE_NUMBER             (required) sender number
  SMS_RETRY_COUNT               (default: 3)
  SMS_VALIDITY_PERIOD           (default: 300) seconds
  SMS_TIMEOUT                   (default: 30s) simple duration (30s/5m/2h) or seconds
  DOMAIN / API_DOMAIN / ...     used for ${VAR} expansion inside the nginx conf paths

Run forever: performs a check on start, then every 24h.
"""

from __future__ import annotations

import os
import re
import time
import glob
import logging
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, List, Tuple

import requests
from cryptography import x509
from cryptography.hazmat.backends import default_backend

# ---------- Helpers ----------


def _parse_timedelta(s: str) -> float:
    """Parse simple duration strings like '30s', '5m', '2h'. Return seconds."""
    if not s:
        return 30.0
    m = re.match(r"^(\d+)([smhd])$", s.strip(), re.IGNORECASE)
    if not m:
        # fallback: try float seconds
        try:
            return float(s)
        except Exception:
            return 30.0
    value, unit = int(m.group(1)), m.group(2).lower()
    mult = {"s": 1, "m": 60, "h": 3600, "d": 86400}[unit]
    return float(value * mult)


def load_cert_expiry(p: Path) -> datetime:
    data = p.read_bytes()
    # Take first certificate in the bundle
    pem_blocks = re.split(b"(?=-----BEGIN CERTIFICATE-----)", data)
    for block in pem_blocks:
        block = block.strip()
        if not block:
            continue
        cert = x509.load_pem_x509_certificate(block, default_backend())
        return cert.not_valid_after.replace(tzinfo=timezone.utc)
    raise ValueError(f"No certificate found in {p}")


def find_cert_paths(conf_path: Path, env_map: Dict[str, str]) -> List[Path]:
    paths: List[Path] = []
    if conf_path.exists():
        text = conf_path.read_text()
        for match in re.finditer(r"ssl_certificate\s+([^;]+);", text):
            raw = match.group(1).strip()
            expanded = os.path.expandvars(_expand_env(raw, env_map))
            paths.append(Path(expanded))
        for match in re.finditer(r"include\s+([^;]+);", text):
            inc_raw = match.group(1).strip()
            inc_expanded = os.path.expandvars(_expand_env(inc_raw, env_map))
            for inc_path in glob.glob(inc_expanded):
                paths.extend(find_cert_paths(Path(inc_path), env_map))
    return paths


def _expand_env(value: str, env_map: Dict[str, str]) -> str:
    out = value
    for k, v in env_map.items():
        out = out.replace(f"${{{k}}}", v)
    return out


def unique_paths(paths: List[Path]) -> List[Path]:
    seen = set()
    out: List[Path] = []
    for p in paths:
        q = p.resolve() if p.exists() else p
        if q in seen:
            continue
        seen.add(q)
        out.append(p)
    return out


# ---------- SMS ----------


def send_sms(recipient: str, body: str, cfg: Dict[str, str], timeout_s: float) -> None:
    url = f"https://{cfg['SMS_PROVIDER_DOMAIN']}/api/v3.0.1/send"
    payload = [
        {
            "srcNum": cfg["SMS_SOURCE_NUMBER"],
            "recipient": recipient,
            "body": body,
            "customerId": None,
            "retryCount": int(cfg.get("SMS_RETRY_COUNT", 3)),
            "type": 1,
            "validityPeriod": int(cfg.get("SMS_VALIDITY_PERIOD", 300)),
        }
    ]
    resp = requests.post(
        url,
        json=payload,
        headers={"Content-Type": "application/json", "x-api-key": cfg["SMS_API_KEY"]},
        timeout=timeout_s,
    )
    resp.raise_for_status()
    results = resp.json()
    # basic validation like Go service
    for r in results:
        if r.get("statusCode") != 200 or r.get("status") != "ACCEPTED":
            raise RuntimeError(f"SMS delivery failed for {r.get('recipient')}: {r}")


# ---------- Main logic ----------


def check_and_notify() -> None:
    log = logging.getLogger("cert_monitor")
    env_map = {k: v for k, v in os.environ.items()}
    conf_path = Path(
        os.getenv("CERT_ALERT_CONF_PATH", "/etc/nginx/sites-available/yamata-beta.conf")
    )
    threshold_days = int(os.getenv("CERT_ALERT_THRESHOLD_DAYS", "7"))
    recipient = os.getenv("CERT_ALERT_PHONE")
    if not recipient:
        raise SystemExit("CERT_ALERT_PHONE is required")

    sms_cfg_keys = ["SMS_PROVIDER_DOMAIN", "SMS_API_KEY", "SMS_SOURCE_NUMBER"]
    missing = [k for k in sms_cfg_keys if not env_map.get(k)]
    if missing:
        raise SystemExit(f"Missing SMS config envs: {', '.join(missing)}")

    timeout_s = _parse_timedelta(os.getenv("SMS_TIMEOUT", "30s"))

    conf_globs = os.getenv(
        "CERT_ALERT_CONF_GLOBS",
        "/etc/nginx/sites-enabled/*.conf,/etc/nginx/conf.d/*.conf",
    )
    conf_files: List[Path] = [conf_path]
    for g in conf_globs.split(","):
        g = g.strip()
        if not g:
            continue
        for p in glob.glob(g):
            conf_files.append(Path(p))

    log.info(
        "Scanning configs: %s",
        ", ".join(str(p) for p in conf_files if Path(p).exists()),
    )

    paths: List[Path] = []
    for cf in conf_files:
        paths.extend(find_cert_paths(cf, env_map))

    extra = os.getenv("CERT_ALERT_CERT_PATHS", "")
    if extra:
        paths.extend(Path(p.strip()) for p in extra.split(",") if p.strip())

    # If nothing found, try default letsencrypt path using DOMAIN
    default_domain = env_map.get("DOMAIN")
    if default_domain:
        paths.append(Path(f"/etc/letsencrypt/live/{default_domain}/fullchain.pem"))

    paths = unique_paths(paths)
    if not paths:
        raise SystemExit("No certificate paths found to check")

    now = datetime.now(timezone.utc)
    alerts: List[Tuple[Path, datetime, int]] = []
    for p in paths:
        if not p.exists():
            log.warning("cert path missing: %s", p)
            continue
        try:
            expiry = load_cert_expiry(p)
        except Exception as e:
            log.warning("unable to read cert %s: %s", p, e)
            continue
        days_left = int((expiry - now).total_seconds() // 86400)
        log.info("checked %s -> expires %s (%sd)", p, expiry.date(), days_left)
        if days_left <= threshold_days:
            alerts.append((p, expiry, days_left))

    if not alerts:
        log.info("All certificates healthy. Checked %d paths.", len(paths))
        return

    for p, exp, days_left in alerts:
        status = "expired" if days_left < 0 else f"{days_left}d left"
        msg = (
            f"[Yamata Beta] SSL cert alert: {p.name} expires on {exp.date()} UTC ({status}). "
            f"Path: {p}"
        )
        send_sms(recipient, msg, env_map, timeout_s)
        log.warning("Alert sent for %s: %s", p, status)


def main() -> None:
    log_level = os.getenv("CERT_ALERT_LOG_LEVEL", "INFO").upper()
    logging.basicConfig(
        level=getattr(logging, log_level, logging.INFO),
        format="%(asctime)s [%(levelname)s] %(name)s %(message)s",
    )
    # one run on start, then every 24h
    while True:
        try:
            check_and_notify()
        except Exception as e:
            logging.getLogger("cert_monitor").exception(
                "Fatal error during check: %s", e
            )
        time.sleep(24 * 3600)


if __name__ == "__main__":
    main()
