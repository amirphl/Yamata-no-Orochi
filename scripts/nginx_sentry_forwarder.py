#!/usr/bin/env python3
import json
import os
import secrets
import sys
import time
import urllib.error
import urllib.parse
import urllib.request


def getenv(name: str, default: str = "") -> str:
    return os.getenv(name, default).strip()


class SentryStoreClient:
    def __init__(self) -> None:
        self.dsn = getenv("SENTRY_DSN")
        if not self.dsn:
            raise RuntimeError("SENTRY_DSN is not configured")

        parsed = urllib.parse.urlparse(self.dsn)
        if parsed.scheme not in ("http", "https"):
            raise RuntimeError("SENTRY_DSN scheme must be http or https")
        if not parsed.netloc or not parsed.username:
            raise RuntimeError("SENTRY_DSN is invalid")

        project_id = parsed.path.strip("/")
        if not project_id or not project_id.isdigit():
            raise RuntimeError("SENTRY_DSN project ID must be numeric")

        self.store_url = f"{parsed.scheme}://{parsed.netloc}/api/{project_id}/store/"
        sentry_secret = parsed.password or ""
        self.auth_header = (
            "Sentry sentry_version=7, "
            "sentry_client=yamata-nginx-forwarder/1.0, "
            f"sentry_key={parsed.username}, "
            f"sentry_secret={sentry_secret}"
        )
        self.environment = getenv("SENTRY_ENVIRONMENT", getenv("APP_ENV", "production"))
        self.release = getenv("SENTRY_RELEASE", getenv("VERSION", "unknown"))
        self.server_name = getenv("SENTRY_SERVER_NAME", "nginx-beta")
        self.timeout = float(getenv("SENTRY_TIMEOUT_SECONDS", "2"))
        self.public_base_url = getenv("SENTRY_PUBLIC_BASE_URL")
        self.sentry_ui_domain = getenv("SENTRY_UI_DOMAIN")

    def send(self, payload: dict) -> None:
        body = json.dumps(payload).encode("utf-8")
        request = urllib.request.Request(
            self.store_url,
            data=body,
            method="POST",
            headers={
                "Content-Type": "application/json",
                "X-Sentry-Auth": self.auth_header,
            },
        )
        with urllib.request.urlopen(request, timeout=self.timeout) as response:
            if response.status >= 400:
                raise RuntimeError(f"upstream returned {response.status}")

    def event(
        self,
        *,
        message: str,
        level: str,
        status: int | None,
        request_data: dict,
        extra: dict,
    ) -> dict:
        tags = {
            "log.source": "nginx",
        }
        if status is not None:
            tags["http.status_code"] = str(status)
        if "method" in request_data:
            tags["http.method"] = request_data["method"]

        return {
            "event_id": secrets.token_hex(16),
            "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "platform": "python",
            "logger": "yamata-nginx-forwarder",
            "level": level,
            "server_name": self.server_name,
            "environment": self.environment,
            "release": self.release,
            "message": message[:2048],
            "tags": tags,
            "request": request_data,
            "extra": extra,
        }


class Tailer:
    def __init__(self, path: str) -> None:
        self.path = path
        self.handle = None
        self._inode: int | None = None

    def open(self) -> None:
        self.handle = open(self.path, "r", encoding="utf-8", errors="replace")
        if self.handle.seekable():
            self.handle.seek(0, os.SEEK_END)
        self._inode = os.fstat(self.handle.fileno()).st_ino

    def _rotated(self) -> bool:
        try:
            return os.stat(self.path).st_ino != self._inode
        except FileNotFoundError:
            return False

    def poll(self) -> list[str]:
        if self.handle is None:
            self.open()
        elif self._rotated():
            self.handle.close()
            self.open()

        lines = []
        while True:
            line = self.handle.readline()
            if not line:
                break
            lines.append(line.rstrip("\n"))
        return lines


def parse_request(raw_request: str) -> tuple[str, str]:
    parts = raw_request.split(" ")
    if len(parts) < 2:
        return "", ""
    return parts[0], parts[1]


def build_request_url(
    base_url: str, path: str, host: str = "", scheme: str = ""
) -> str:
    if host and path:
        return f"{scheme or 'https'}://{host}{path}"
    if not base_url or not path:
        return path
    return urllib.parse.urljoin(base_url.rstrip("/") + "/", path.lstrip("/"))


def should_skip_access_entry(
    client: SentryStoreClient, host: str, method: str, path: str
) -> bool:
    if host == client.sentry_ui_domain and method.upper() == "POST":
        if path.startswith("/api/") and path.endswith("/store/"):
            return True
        if path.startswith("/api/") and path.endswith("/envelope/"):
            return True
    return False


def handle_access_line(client: SentryStoreClient, line: str) -> None:
    if not line:
        return

    try:
        entry = json.loads(line)
    except json.JSONDecodeError:
        return

    try:
        status = int(entry.get("status", "0"))
    except ValueError:
        return

    if status < 400:
        return

    method, path = parse_request(entry.get("request", ""))
    host = entry.get("host", "")
    scheme = entry.get("scheme", "https")
    if should_skip_access_entry(client, host, method, path):
        return

    request_data = {
        "method": method,
        "url": build_request_url(
            client.public_base_url, path, host=host, scheme=scheme
        ),
        "headers": {
            "Host": host,
            "User-Agent": entry.get("http_user_agent", ""),
            "X-Forwarded-For": entry.get("http_x_forwarded_for", ""),
            "X-Real-IP": entry.get("http_x_real_ip", ""),
        },
    }

    message = f"Nginx returned HTTP {status} for {method} {path}".strip()
    extra = {
        "remote_addr": entry.get("remote_addr"),
        "request_time": entry.get("request_time"),
        "request_id": entry.get("request_id"),
        "upstream_addr": entry.get("upstream_addr"),
        "upstream_status": entry.get("upstream_status"),
        "upstream_response_time": entry.get("upstream_response_time"),
        "referrer": entry.get("http_referrer"),
    }

    client.send(
        client.event(
            message=message,
            level="error" if status >= 500 else "warning",
            status=status,
            request_data=request_data,
            extra=extra,
        )
    )


def handle_error_line(client: SentryStoreClient, line: str) -> None:
    if not line:
        return

    request_data = {
        "url": build_request_url(client.public_base_url, "/"),
        "method": "NGINX",
    }
    extra = {"line": line[:4096]}
    client.send(
        client.event(
            message=line[:2048],
            level="error",
            status=None,
            request_data=request_data,
            extra=extra,
        )
    )


def main() -> int:
    access_log = getenv("NGINX_ACCESS_LOG", "/var/log/nginx/access.log")
    error_log = getenv("NGINX_ERROR_LOG", "/var/log/nginx/error.log")
    poll_interval = float(getenv("NGINX_LOG_POLL_INTERVAL_SECONDS", "0.5"))

    try:
        client = SentryStoreClient()
    except RuntimeError as exc:
        print(f"sentry forwarder disabled: {exc}", file=sys.stderr)
        while True:
            time.sleep(60)

    access_tailer = Tailer(access_log)
    error_tailer = Tailer(error_log)

    while True:
        try:
            for line in access_tailer.poll():
                handle_access_line(client, line)
            for line in error_tailer.poll():
                handle_error_line(client, line)
        except FileNotFoundError:
            time.sleep(poll_interval)
        except urllib.error.URLError as exc:
            print(f"sentry forwarder network error: {exc}", file=sys.stderr)
            time.sleep(poll_interval)
        except Exception as exc:  # noqa: BLE001
            print(f"sentry forwarder error: {exc}", file=sys.stderr)
            time.sleep(poll_interval)

        time.sleep(poll_interval)


if __name__ == "__main__":
    raise SystemExit(main())
