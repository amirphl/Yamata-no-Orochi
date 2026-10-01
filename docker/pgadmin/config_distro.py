"""Production security policy for the pgAdmin container.

This file is mounted read-only so the container can run with a read-only root
filesystem. PGADMIN_ALLOWED_HOST is injected by Compose from the deployment
domain; credentials are deliberately not present in this file.
"""

import os

# Accept the single Nginx virtual host only, preventing Host-header injection.
ALLOWED_HOSTS = [os.environ["PGADMIN_ALLOWED_HOST"]]

# Exactly one trusted proxy (the nginx container) adds these headers.
PROXY_X_FOR_COUNT = 1
PROXY_X_PROTO_COUNT = 1
PROXY_X_HOST_COUNT = 1
PROXY_X_PORT_COUNT = 1
PROXY_X_PREFIX_COUNT = 0

# This isolated administrative service intentionally has no outbound Internet
# route. Avoid an otherwise harmless but user-visible 500 when the web UI
# checks pgAdmin's public update feed.
UPGRADE_CHECK_ENABLED = False

# Keep pgAdmin's own, local user/password authentication enabled.
AUTHENTICATION_SOURCES = ["internal"]
SECURITY_PASSWORD_HASH = "pbkdf2_sha512"
PASSWORD_LENGTH_MIN = 16
MAX_LOGIN_ATTEMPTS = 3
LOGIN_ATTEMPT_FIELDS = ["password"]

# Database passwords must be entered for each connection rather than retained
# in pgAdmin's persistent configuration volume. Do not retain query history
# either. This also avoids external Gravatar lookups that would disclose
# administrator email hashes.
ALLOW_SAVE_PASSWORD = False
MAX_QUERY_HIST_STORED = 0
SHOW_GRAVATAR_IMAGE = False

# Cookie and session hardening for the HTTPS-only administrative endpoint.
SESSION_COOKIE_SECURE = True
SESSION_COOKIE_HTTPONLY = True
SESSION_COOKIE_SAMESITE = "Strict"
SESSION_EXPIRATION_TIME = 1
USER_INACTIVITY_TIMEOUT = 900
OVERRIDE_USER_INACTIVITY_TIMEOUT = False
ENHANCED_COOKIE_PROTECTION = True

# This deployment connects directly to the internal PostgreSQL service; do not
# allow stored SSH tunnel passwords or an SSH-tunnel escape path from the UI.
SUPPORT_SSH_TUNNEL = False
ALLOW_SAVE_TUNNEL_PASSWORD = False

LOGIN_BANNER = "Restricted administrative system. Authorized use only."
