#!/usr/bin/env bash

# Validate certificate files referenced by the generated Nginx configuration.
# This script never issues, renews, installs, or schedules certificates.

set -Eeuo pipefail

readonly CONFIG_FILE="${1:-/srv/yamata/docker/nginx/sites-available/generated/beta/yamata.conf}"
readonly MIN_VALID_DAYS="${YAMATA_CERT_MIN_VALID_DAYS:-0}"

die() {
	printf '[certificate-check] ERROR: %s\n' "$*" >&2
	exit 1
}

[[ -f "$CONFIG_FILE" ]] || die "Nginx configuration does not exist: $CONFIG_FILE"
[[ "$MIN_VALID_DAYS" =~ ^[0-9]+$ ]] || die "YAMATA_CERT_MIN_VALID_DAYS must be a non-negative integer"
((MIN_VALID_DAYS <= 36500)) || die "YAMATA_CERT_MIN_VALID_DAYS must not exceed 36500"

for command_name in awk grep mktemp openssl sha256sum sort; do
	command -v "$command_name" >/dev/null 2>&1 || die "Required command is missing: $command_name"
done

if [[ $EUID -eq 0 ]]; then
	PRIVILEGED=()
else
	command -v sudo >/dev/null 2>&1 || die "sudo is required to read private certificate material"
	sudo -v
	PRIVILEGED=(sudo)
fi
readonly PRIVILEGED

WORK_DIR="$(mktemp -d /tmp/yamata-certificate-check.XXXXXX)"
cleanup() {
	rm -rf -- "$WORK_DIR"
}
trap cleanup EXIT
: >"$WORK_DIR/certificate-public-keys"
: >"$WORK_DIR/private-public-keys"

DIRECTIVES="$(
	awk '
		/^[[:space:]]*ssl_(certificate|certificate_key|trusted_certificate)[[:space:]]/ {
			path=$2
			gsub(/;/, "", path)
			print $1 "\t" path
		}
	' "$CONFIG_FILE" | sort -u
)"
readonly DIRECTIVES
[[ -n "$DIRECTIVES" ]] || die "No TLS certificate directives found in $CONFIG_FILE"

minimum_seconds=$((MIN_VALID_DAYS * 86400))
while IFS=$'\t' read -r directive certificate_path; do
	[[ -n "$certificate_path" ]] || continue
	"${PRIVILEGED[@]}" test -f "$certificate_path" || die "Missing file: $certificate_path"
	if [[ "$directive" == ssl_certificate_key ]]; then
		"${PRIVILEGED[@]}" openssl pkey -in "$certificate_path" -check -noout >/dev/null 2>&1 ||
			die "Invalid private key: $certificate_path"
		key_hash="$(
			"${PRIVILEGED[@]}" openssl pkey -in "$certificate_path" -pubout -outform DER 2>/dev/null |
				sha256sum | awk '{print $1}'
		)"
		printf '%s\t%s\n' "$key_hash" "$certificate_path" >>"$WORK_DIR/private-public-keys"
	else
		"${PRIVILEGED[@]}" openssl x509 -in "$certificate_path" \
			-checkend "$minimum_seconds" -noout >/dev/null 2>&1 ||
			die "Certificate expires within $MIN_VALID_DAYS day(s) or is invalid: $certificate_path"
		certificate_hash="$(
			"${PRIVILEGED[@]}" openssl x509 -in "$certificate_path" -pubkey -noout 2>/dev/null |
				openssl pkey -pubin -outform DER 2>/dev/null |
				sha256sum | awk '{print $1}'
		)"
		printf '%s\n' "$certificate_hash" >>"$WORK_DIR/certificate-public-keys"
	fi
	printf '[certificate-check] valid: %s\n' "$certificate_path"
done <<<"$DIRECTIVES"

while IFS=$'\t' read -r key_hash key_path; do
	grep -Fxq "$key_hash" "$WORK_DIR/certificate-public-keys" ||
		die "Private key does not match any configured certificate: $key_path"
done <"$WORK_DIR/private-public-keys"

printf '[certificate-check] All referenced certificate material is valid; nothing was issued or renewed\n'
