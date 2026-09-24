#!/usr/bin/env bash
set -euo pipefail

: "${EXTERNAL_SHORTLINK_RUNTIME_PASSWORD:?EXTERNAL_SHORTLINK_RUNTIME_PASSWORD is required}"

psql --set=ON_ERROR_STOP=1 \
  --username "$POSTGRES_USER" \
  --dbname "$POSTGRES_DB" \
  --set=runtime_password="$EXTERNAL_SHORTLINK_RUNTIME_PASSWORD" <<'SQL'
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'external_shortlink_runtime') THEN
        CREATE ROLE external_shortlink_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
    END IF;
END
$$;

ALTER ROLE external_shortlink_runtime PASSWORD :'runtime_password';
REVOKE ALL ON DATABASE external_shortlink FROM PUBLIC;
GRANT CONNECT ON DATABASE external_shortlink TO external_shortlink_runtime;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO external_shortlink_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO external_shortlink_runtime;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO external_shortlink_runtime;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO external_shortlink_runtime;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO external_shortlink_runtime;
SQL
