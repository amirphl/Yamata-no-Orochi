#!/usr/bin/env bash

# Source this file, then call load_yamata_env_file PATH. Values are parsed as
# data by load_dotenv.py; the environment file is never evaluated as shell code.

load_yamata_env_file() {
	local environment_file="$1"
	local loader_dir loader_output key value
	# Shell tracing would print secret values during assignment. Keep it disabled
	# for the remainder of the calling deployment process.
	set +x
	if ! command -v python3 >/dev/null 2>&1; then
		printf 'ERROR: python3 is required to load %s safely\n' "$environment_file" >&2
		return 1
	fi
	if [ ! -f "$environment_file" ] || [ -L "$environment_file" ]; then
		printf 'ERROR: dotenv path must be a regular, non-symlink file: %s\n' "$environment_file" >&2
		return 1
	fi
	loader_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
	loader_output="$(mktemp /tmp/yamata-env.XXXXXX)" || return 1
	if ! python3 "$loader_dir/load_dotenv.py" "$environment_file" >"$loader_output"; then
		rm -f -- "$loader_output"
		return 1
	fi
	while IFS= read -r -d '' key && IFS= read -r -d '' value; do
		printf -v "$key" '%s' "$value"
		export "$key"
	done <"$loader_output"
	rm -f -- "$loader_output"
}
