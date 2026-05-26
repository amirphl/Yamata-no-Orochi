"""
Periodic HTTP endpoint monitor for external domains.

It fetches a configured URL on an interval, follows redirects, and opens an
incident when the endpoint keeps failing after exponential-backoff retries.
When a new incident starts, the monitor sends an SMS to the configured admin
phone numbers. Optionally, it can also send a recovery SMS when the endpoint
becomes healthy again.

Environment variables:
  DOMAIN_MONITOR_URL                (default: http://jo1n.ir/9e13x)
  DOMAIN_MONITOR_INTERVAL           (default: 5m)
  DOMAIN_MONITOR_TIMEOUT            (default: 20s)
  DOMAIN_MONITOR_EXPECTED_STATUSES  (default: 200)
  DOMAIN_MONITOR_ALERT_PHONES       optional, comma-separated; falls back to ADMIN_MOBILE
  DOMAIN_MONITOR_RETRY_COUNT        (default: 3) retries after the first failed attempt
  DOMAIN_MONITOR_BACKOFF_INITIAL    (default: 5s)
  DOMAIN_MONITOR_BACKOFF_MULTIPLIER (default: 2)
  DOMAIN_MONITOR_BACKOFF_MAX        (default: 1m)
  DOMAIN_MONITOR_RECOVERY_SMS       (default: true)
  DOMAIN_MONITOR_VERIFY_TLS         (default: true)
  DOMAIN_MONITOR_LOG_LEVEL          (default: INFO)
  DOMAIN_MONITOR_USER_AGENT         optional custom user agent
  ADMIN_MOBILE                      fallback admin phone list
  SMS_PROVIDER_DOMAIN               (required)
  SMS_API_KEY                       (required)
  SMS_SOURCE_NUMBER                 (required)
  SMS_RETRY_COUNT                   (default: 3)
  SMS_VALIDITY_PERIOD               (default: 300) seconds
  SMS_TIMEOUT                       (default: 30s)
"""

from __future__ import annotations

import logging
import os
import re
import time
from datetime import datetime, timezone
from typing import Dict, Iterable, List

import requests


def _parse_timedelta(value: str, default_seconds: float) -> float:
    if not value:
        return default_seconds
    match = re.match(r"^(\d+)([smhd])$", value.strip(), re.IGNORECASE)
    if not match:
        try:
            return float(value)
        except Exception:
            return default_seconds
    amount, unit = int(match.group(1)), match.group(2).lower()
    multiplier = {"s": 1, "m": 60, "h": 3600, "d": 86400}[unit]
    return float(amount * multiplier)


def _parse_bool(value: str, default: bool) -> bool:
    if value is None:
        return default
    normalized = value.strip().lower()
    if normalized in {"1", "true", "yes", "on"}:
        return True
    if normalized in {"0", "false", "no", "off"}:
        return False
    return default


def _parse_csv(value: str) -> List[str]:
    return [item.strip() for item in (value or "").split(",") if item.strip()]


def _parse_expected_statuses(value: str) -> List[int]:
    statuses: List[int] = []
    for item in _parse_csv(value or "200"):
        try:
            statuses.append(int(item))
        except ValueError:
            continue
    return statuses or [200]


def _truncate(value: str, limit: int = 180) -> str:
    if len(value) <= limit:
        return value
    return value[: limit - 3] + "..."


def send_sms(recipients: Iterable[str], body: str, cfg: Dict[str, str], timeout_s: float) -> None:
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
        for recipient in recipients
    ]
    response = requests.post(
        url,
        json=payload,
        headers={"Content-Type": "application/json", "x-api-key": cfg["SMS_API_KEY"]},
        timeout=timeout_s,
    )
    response.raise_for_status()
    results = response.json()
    for result in results:
        if result.get("statusCode") != 200 or result.get("status") != "ACCEPTED":
            raise RuntimeError(
                f"SMS delivery failed for {result.get('recipient')}: {result}"
            )


class DomainMonitor:
    def __init__(self) -> None:
        self.log = logging.getLogger("domain_monitor")
        self.env = dict(os.environ)
        self.url = os.getenv("DOMAIN_MONITOR_URL", "http://jo1n.ir/9e13x")
        self.interval_s = _parse_timedelta(
            os.getenv("DOMAIN_MONITOR_INTERVAL", "5m"), 300.0
        )
        self.timeout_s = _parse_timedelta(
            os.getenv("DOMAIN_MONITOR_TIMEOUT", "20s"), 20.0
        )
        self.sms_timeout_s = _parse_timedelta(os.getenv("SMS_TIMEOUT", "30s"), 30.0)
        self.expected_statuses = _parse_expected_statuses(
            os.getenv("DOMAIN_MONITOR_EXPECTED_STATUSES", "200")
        )
        self.alert_phones = _parse_csv(
            os.getenv("DOMAIN_MONITOR_ALERT_PHONES") or os.getenv("ADMIN_MOBILE", "")
        )
        self.retry_count = max(0, int(os.getenv("DOMAIN_MONITOR_RETRY_COUNT", "3")))
        self.backoff_initial_s = _parse_timedelta(
            os.getenv("DOMAIN_MONITOR_BACKOFF_INITIAL", "5s"), 5.0
        )
        self.backoff_multiplier = max(
            1.0, float(os.getenv("DOMAIN_MONITOR_BACKOFF_MULTIPLIER", "2"))
        )
        self.backoff_max_s = _parse_timedelta(
            os.getenv("DOMAIN_MONITOR_BACKOFF_MAX", "1m"), 60.0
        )
        self.recovery_sms_enabled = _parse_bool(
            os.getenv("DOMAIN_MONITOR_RECOVERY_SMS", "true"), True
        )
        self.verify_tls = _parse_bool(
            os.getenv("DOMAIN_MONITOR_VERIFY_TLS", "true"), True
        )
        self.user_agent = os.getenv(
            "DOMAIN_MONITOR_USER_AGENT",
            "yamata-domain-monitor/1.0",
        )
        self.session = requests.Session()
        self.incident_open = False
        self.incident_started_at: datetime | None = None

        sms_cfg_keys = ["SMS_PROVIDER_DOMAIN", "SMS_API_KEY", "SMS_SOURCE_NUMBER"]
        missing_sms_cfg = [key for key in sms_cfg_keys if not self.env.get(key)]
        if missing_sms_cfg:
            raise SystemExit(f"Missing SMS config envs: {', '.join(missing_sms_cfg)}")
        if not self.alert_phones:
            raise SystemExit(
                "DOMAIN_MONITOR_ALERT_PHONES or ADMIN_MOBILE must contain at least one number"
            )

    def run_forever(self) -> None:
        while True:
            try:
                self.run_cycle()
            except Exception as exc:
                self.log.exception("Fatal error during monitor cycle: %s", exc)
            time.sleep(self.interval_s)

    def run_cycle(self) -> None:
        started_at = datetime.now(timezone.utc)
        self.log.info("Checking %s", self.url)
        error_message = self._check_with_retries()
        if error_message is None:
            self._handle_success(started_at)
            return
        self._handle_failure(started_at, error_message)

    def _check_with_retries(self) -> str | None:
        attempts = self.retry_count + 1
        backoff_s = max(0.0, self.backoff_initial_s)
        last_error = "unknown error"
        for attempt in range(1, attempts + 1):
            try:
                self._perform_check()
                if attempt > 1:
                    self.log.info("Recovered during retry attempt %d/%d", attempt, attempts)
                return None
            except Exception as exc:
                last_error = str(exc)
                self.log.warning("Attempt %d/%d failed: %s", attempt, attempts, last_error)
                if attempt >= attempts:
                    break
                sleep_for = min(backoff_s, self.backoff_max_s)
                self.log.info("Sleeping %.1fs before retry", sleep_for)
                time.sleep(sleep_for)
                backoff_s = min(
                    max(1.0, backoff_s * self.backoff_multiplier),
                    self.backoff_max_s,
                )
        return f"failed after {attempts} attempts: {_truncate(last_error)}"

    def _perform_check(self) -> None:
        response = self.session.get(
            self.url,
            allow_redirects=True,
            timeout=self.timeout_s,
            verify=self.verify_tls,
            headers={"User-Agent": self.user_agent},
        )
        if response.status_code not in self.expected_statuses:
            raise RuntimeError(
                f"unexpected status={response.status_code} final_url={response.url}"
            )
        self.log.info(
            "Healthy response status=%s final_url=%s",
            response.status_code,
            response.url,
        )

    def _handle_success(self, checked_at: datetime) -> None:
        if not self.incident_open:
            return
        started_at = self.incident_started_at or checked_at
        duration_s = int((checked_at - started_at).total_seconds())
        self.log.info("Incident resolved after %ss", duration_s)
        if self.recovery_sms_enabled:
            msg = (
                f"[Yamata Beta] jo1n.ir recovered. URL={self.url}. "
                f"Duration={duration_s}s."
            )
            try:
                self._send_alert(msg)
            except Exception as exc:
                self.log.exception("Failed to send recovery SMS: %s", exc)
        self.incident_open = False
        self.incident_started_at = None

    def _handle_failure(self, checked_at: datetime, error_message: str) -> None:
        if self.incident_open:
            self.log.warning("Incident still open: %s", error_message)
            return
        msg = (
            f"[Yamata Beta] jo1n.ir incident. URL={self.url}. "
            f"Error={_truncate(error_message)}."
        )
        self._send_alert(msg)
        self.incident_open = True
        self.incident_started_at = checked_at
        self.log.warning("Incident opened: %s", error_message)

    def _send_alert(self, body: str) -> None:
        send_sms(self.alert_phones, body, self.env, self.sms_timeout_s)
        self.log.info("Sent SMS alert to %s", ",".join(self.alert_phones))


def main() -> None:
    log_level = os.getenv("DOMAIN_MONITOR_LOG_LEVEL", "INFO").upper()
    logging.basicConfig(
        level=getattr(logging, log_level, logging.INFO),
        format="%(asctime)s [%(levelname)s] %(name)s %(message)s",
    )
    DomainMonitor().run_forever()


if __name__ == "__main__":
    main()
