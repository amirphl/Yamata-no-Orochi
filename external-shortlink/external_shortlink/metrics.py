from __future__ import annotations

import os

from prometheus_client import CONTENT_TYPE_LATEST, CollectorRegistry, Counter, Gauge, Histogram, generate_latest, multiprocess


REDIRECTS = Counter("external_shortlink_redirect_requests_total", "Redirect requests", ("status",))
REDIRECT_LATENCY = Histogram(
    "external_shortlink_redirect_latency_seconds",
    "End-to-end redirect handler latency",
    buckets=(0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0),
)
UNKNOWN_CODES = Counter("external_shortlink_unknown_codes_total", "Unknown short-code requests")
POSTGRES_ERRORS = Counter("external_shortlink_postgres_errors_total", "PostgreSQL operation failures", ("operation",))
POOL_TIMEOUTS = Counter("external_shortlink_postgres_pool_timeouts_total", "PostgreSQL pool or operation timeouts")
SPOOL_EVENTS = Gauge("external_shortlink_spool_events", "Events awaiting PostgreSQL replay", multiprocess_mode="livemax")
SPOOL_BYTES = Gauge("external_shortlink_spool_bytes", "Bytes used by the durable click spool", multiprocess_mode="livemax")
SPOOL_OLDEST_AGE = Gauge("external_shortlink_spool_oldest_event_age_seconds", "Age of the oldest spooled click", multiprocess_mode="livemax")
SPOOL_DROPPED = Counter("external_shortlink_spool_rejected_total", "Clicks that could not be durably spooled", ("reason",))
MAPPING_UPLOADS = Counter("external_shortlink_mapping_upload_total", "Mapping upload requests", ("result",))
LAST_MAPPING_UPLOAD = Gauge("external_shortlink_last_successful_mapping_upload_timestamp_seconds", "Unix time of the last successful mapping upload", multiprocess_mode="livemax")
LAST_ACK_ID = Gauge("external_shortlink_last_acknowledged_click_id", "Highest production-acknowledged click ID", multiprocess_mode="livemax")
DATABASE_BYTES = Gauge("external_shortlink_database_size_bytes", "Current PostgreSQL database size", multiprocess_mode="livemax")
CACHE_LOOKUPS = Counter("external_shortlink_cache_lookups_total", "Link-cache lookups", ("result",))


def render() -> tuple[bytes, str]:
    multiprocess_dir = os.getenv("PROMETHEUS_MULTIPROC_DIR")
    if multiprocess_dir:
        registry = CollectorRegistry()
        multiprocess.MultiProcessCollector(registry)
        return generate_latest(registry), CONTENT_TYPE_LATEST
    return generate_latest(), CONTENT_TYPE_LATEST
