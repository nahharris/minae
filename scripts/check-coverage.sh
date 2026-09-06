#!/usr/bin/env bash
#
# Fail if total statement coverage has dropped below the ratchet in
# .coverage-floor. Run it after `go test -coverprofile=...`.
#
#   ./scripts/check-coverage.sh [profile] [floor-file]
#
set -euo pipefail

profile="${1:-coverage.out}"
floor_file="${2:-.coverage-floor}"

if [[ ! -f "$profile" ]]; then
	echo "coverage profile not found: $profile" >&2
	exit 1
fi
if [[ ! -f "$floor_file" ]]; then
	echo "coverage floor file not found: $floor_file" >&2
	exit 1
fi

# The floor file carries comment lines explaining the ratchet; the value is the
# first line that is not a comment.
floor="$(grep -v '^[[:space:]]*#' "$floor_file" | tr -d '[:space:]' | grep -m1 .)"
total="$(go tool cover -func="$profile" | awk '/^total:/ { gsub(/%/, "", $NF); print $NF }')"

if [[ -z "$total" ]]; then
	echo "could not read a total from $profile" >&2
	exit 1
fi

echo "total coverage: ${total}%  (floor: ${floor}%)"

if ! awk -v t="$total" -v f="$floor" 'BEGIN { exit (t + 1e-9 >= f) ? 0 : 1 }'; then
	echo "coverage ${total}% is below the floor of ${floor}%" >&2
	exit 1
fi

# Nudge the ratchet forward once there is real slack, so the floor keeps pace
# with the tests instead of quietly going stale.
if awk -v t="$total" -v f="$floor" 'BEGIN { exit (t >= f + 2) ? 0 : 1 }'; then
	echo "note: coverage exceeds the floor by 2 points or more; consider raising $floor_file"
fi
