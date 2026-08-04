#!/usr/bin/env bash

# Launch a selective plain-SQL restore as a transient systemd background job.
# Usage: run-yamata-data-restore.sh audience|scheduler DUMP_FILE [PROJECT_DIR] [UNIT_NAME] [--allow-active-backend]

set -Eeuo pipefail

readonly RESTORE_KIND="${1:-}"
readonly DUMP_ARGUMENT="${2:-}"
readonly PROJECT_DIR="${3:-/srv/yamata}"
readonly REQUESTED_UNIT="${4:-}"
readonly ACTIVE_BACKEND_OPTION="${5:-}"

die() {
	printf '[restore-launcher] ERROR: %s\n' "$*" >&2
	exit 1
}

case "$RESTORE_KIND" in
	audience)
		helper=/usr/local/sbin/restore-yamata-audience-profiles
		description='Yamata audience profiles restore'
		case "$ACTIVE_BACKEND_OPTION" in
			"") extra_arguments=() ;;
			--allow-active-backend) extra_arguments=(--allow-active-backend) ;;
			*) die "Unknown option: $ACTIVE_BACKEND_OPTION" ;;
		esac
		;;
	scheduler)
		helper=/usr/local/sbin/restore-yamata-scheduler-runtime-data
		description='Yamata scheduler runtime restore'
		extra_arguments=()
		[[ -z "$ACTIVE_BACKEND_OPTION" ]] || die "--allow-active-backend is valid only for audience restores"
		;;
		*) die "Usage: $(basename "$0") audience|scheduler DUMP_FILE [PROJECT_DIR] [UNIT_NAME] [--allow-active-backend]" ;;
esac

[[ -n "$DUMP_ARGUMENT" ]] || die "Dump file is required"
DUMP_FILE="$(readlink -f "$DUMP_ARGUMENT")"
readonly DUMP_FILE
[[ -f "$DUMP_FILE" ]] || die "Dump does not exist: $DUMP_FILE"
[[ -x "$helper" ]] || die "Helper is not installed: $helper"
[[ -d "$PROJECT_DIR" ]] || die "Project directory does not exist: $PROJECT_DIR"

for command_name in systemctl systemd-run journalctl; do
	command -v "$command_name" >/dev/null 2>&1 || die "Required command is missing: $command_name"
done

unit="${REQUESTED_UNIT:-yamata-${RESTORE_KIND}-restore-$(date -u +%Y%m%dT%H%M%SZ)}"
unit="${unit%.service}"
[[ "$unit" =~ ^[A-Za-z0-9_.@-]+$ ]] || die "Invalid systemd unit name: $unit"
readonly unit helper description

if [[ $EUID -eq 0 ]]; then
	SYSTEM=()
else
	command -v sudo >/dev/null 2>&1 || die "sudo is required"
	sudo -v
	SYSTEM=(sudo)
fi
readonly SYSTEM

"${SYSTEM[@]}" systemd-run \
	--unit="$unit" \
	--description="$description" \
	--property=Type=exec \
	--property=Restart=no \
	"$helper" "$DUMP_FILE" "$PROJECT_DIR" "${extra_arguments[@]}"

printf '[restore-launcher] Started %s.service\n' "$unit"
printf '[restore-launcher] Follow: sudo journalctl -u %s -f -o cat\n' "$unit"
printf '[restore-launcher] Progress: sudo journalctl -u %s -o cat --no-pager | grep progress= | tail -1\n' "$unit"
printf '[restore-launcher] Status: sudo systemctl show %s -p ActiveState -p SubState -p Result -p ExecMainStatus\n' "$unit"
