#!/usr/bin/env bash

# Install canonical production operation helpers in /usr/local/sbin.

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR

if [[ $EUID -eq 0 ]]; then
	INSTALL=(install)
else
	command -v sudo >/dev/null 2>&1 || {
		printf '[install-operations] ERROR: sudo is required\n' >&2
		exit 1
	}
	INSTALL=(sudo install)
fi

"${INSTALL[@]}" -d -m 755 /usr/local/libexec
"${INSTALL[@]}" -m 755 "$SCRIPT_DIR/extract_pg_dump_copy.py" \
	/usr/local/libexec/yamata-extract-pg-dump-copy.py

"${INSTALL[@]}" -m 755 "$SCRIPT_DIR/apply-yamata-required-migrations.sh" \
	/usr/local/sbin/apply-yamata-required-migrations
"${INSTALL[@]}" -m 755 "$SCRIPT_DIR/tune-yamata-restore.sh" \
	/usr/local/sbin/tune-yamata-restore
"${INSTALL[@]}" -m 755 "$SCRIPT_DIR/restore-yamata-audience-profiles.sh" \
	/usr/local/sbin/restore-yamata-audience-profiles
"${INSTALL[@]}" -m 755 "$SCRIPT_DIR/restore-yamata-scheduler-runtime-data.sh" \
	/usr/local/sbin/restore-yamata-scheduler-runtime-data
"${INSTALL[@]}" -m 755 "$SCRIPT_DIR/run-yamata-data-restore.sh" \
	/usr/local/sbin/run-yamata-data-restore
"${INSTALL[@]}" -m 755 "$SCRIPT_DIR/check-yamata-certificates.sh" \
	/usr/local/sbin/check-yamata-certificates
"${INSTALL[@]}" -m 755 "$SCRIPT_DIR/check-yamata-production.sh" \
	/usr/local/sbin/check-yamata-production

printf '[install-operations] Canonical helpers installed in /usr/local/sbin\n'
