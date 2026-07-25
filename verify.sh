#!/usr/bin/env bash
# Full-stack verification in one command, so checking the base is healthy does
# not mean watching an agent run a long loop.
#
#   ./verify.sh              everything offline: vet, build, Go tests, frontend
#   ./verify.sh --live       + real API calls (needs a provider key in config)
#   ./verify.sh --build      + wails build and the NSIS installer
#   ./verify.sh --live --build
#
# Every stage runs even if an earlier one fails — one run, whole picture. Exits
# non-zero if any stage failed.
#
# Run from Git Bash. From PowerShell: bash ./verify.sh

set -uo pipefail
cd "$(dirname "$0")"

LIVE=0
BUILD=0
for arg in "$@"; do
  case "$arg" in
    --live) LIVE=1 ;;
    --build) BUILD=1 ;;
    -h|--help) sed -n '2,14p' "$0" | sed 's/^# \?//'; exit 0 ;;
    *) echo "unknown flag: $arg (try --help)"; exit 2 ;;
  esac
done

PASSED=(); FAILED=(); SKIPPED=()
LOG_DIR="$(mktemp -d)"

# stage NAME COMMAND...
# Output goes to a temp file and is only printed when the stage fails — a green
# run stays readable, a red one shows everything needed to debug it.
stage() {
  local name="$1"; shift
  local log="$LOG_DIR/${name//[^a-zA-Z0-9]/_}.log"
  local start=$SECONDS
  printf '  %-28s ' "$name"
  if "$@" >"$log" 2>&1; then
    printf 'ok    %ds\n' "$((SECONDS - start))"
    PASSED+=("$name")
  else
    printf 'FAIL  %ds\n' "$((SECONDS - start))"
    FAILED+=("$name")
    echo "  ---- $name output ----"
    sed 's/^/  | /' "$log" | tail -40
    echo "  ---- end $name ----"
  fi
}

skip() {
  printf '  %-28s skipped (%s)\n' "$1" "$2"
  SKIPPED+=("$1")
}

fe() { (cd desktop/frontend && "$@"); }

echo
echo "Aetox verification  —  $(git rev-parse --short HEAD 2>/dev/null || echo 'no git') on $(git branch --show-current 2>/dev/null || echo '?')"
echo

echo "Go"
stage "vet" go vet ./...
stage "build" go build ./...
# -count=1 defeats the test cache: a cached pass proves nothing about a tree
# that just changed underneath it.
stage "test" go test -count=1 ./...

echo
echo "Frontend"
stage "vitest" fe npx vitest run
stage "svelte-check" fe npx svelte-check --threshold error
stage "vite build" fe npx vite build

echo
echo "Live (real API)"
if [ "$LIVE" = 1 ]; then
  # Skipped by default because they cost money and need a key. They answer what
  # fakes cannot: that tool-call progress really arrives from the configured
  # provider, and that the API accepts every tool definition in one batch.
  stage "provider progress" env AETOX_LIVE=1 go test -count=1 -timeout 15m ./internal/model/ -run TestLive
  stage "all tools accepted" env AETOX_LIVE=1 go test -count=1 -timeout 10m ./desktop/ -run TestLive
else
  skip "provider progress" "pass --live"
  skip "all tools accepted" "pass --live"
fi

echo
echo "Package"
if [ "$BUILD" = 1 ]; then
  stage "wails build" bash -c 'cd desktop && wails build -nsis'
else
  skip "wails build" "pass --build"
fi

echo
echo "─────────────────────────────────────────"
printf '%d passed' "${#PASSED[@]}"
[ "${#FAILED[@]}"  -gt 0 ] && printf ', %d FAILED' "${#FAILED[@]}"
[ "${#SKIPPED[@]}" -gt 0 ] && printf ', %d skipped' "${#SKIPPED[@]}"
printf '  (%dm%ds)\n' "$((SECONDS / 60))" "$((SECONDS % 60))"

if [ "${#FAILED[@]}" -gt 0 ]; then
  printf 'failed: %s\n' "${FAILED[*]}"
  rm -rf "$LOG_DIR"
  exit 1
fi
if [ "$BUILD" = 1 ]; then
  echo "installer: desktop/build/bin/aetox-amd64-installer.exe"
fi
rm -rf "$LOG_DIR"
echo "all good"
