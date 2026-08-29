#!/usr/bin/env bash
# Full-stack verification in one command, so checking the base is healthy does
# not mean watching an agent run a long loop.
#
#   ./verify.sh              everything offline: vet, build, Go tests (shuffled,
#                            + race when a C compiler is installed), frontend
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

# golangci-lint gates a named set of linters that are all at zero; gosec runs
# beside it as a report, not a gate. Which linters, and why the rest are not on
# yet, is docs/DECISIONS.md §141 — .golangci.yml is the configuration.
#
# Skipped loudly for the same reason the race stage is: a check believed to be
# running and not running is worse than one that was never added.
if command -v golangci-lint >/dev/null 2>&1; then
  stage "lint" golangci-lint run ./...
  # Reported, not gating — the same device ci.yml uses on the unix jobs. Most
  # of gosec's 817 findings on this tree are the program doing its job (reading
  # files named by a variable, running commands built at runtime), so it is
  # worth reading and not worth blocking on.
  #
  # --enable-only, not `--default=none --enable=gosec`: in golangci-lint v2
  # --enable APPENDS to the enable list in .golangci.yml, so that form re-ran the
  # thirteen gating linters as well and the summary line below printed their last
  # bullet ("* wastedassign: 2") instead of a gosec count. --enable-only replaces
  # the section and still honours the file's exclusions, so third_party stays
  # out. Pinned by TestGosecReportIsGosecOnly; see docs/DECISIONS.md §141.5.
  golangci-lint run ./... --enable-only=gosec --issues-exit-code=0 >"$LOG_DIR/gosec.log" 2>&1 || true
  # An empty log printed after the colon reads as a pass nobody checked, which is
  # the failure this stage exists to avoid. Say it out loud instead.
  gosec_summary="$(tail -1 "$LOG_DIR/gosec.log")"
  [ -n "$gosec_summary" ] || gosec_summary="no output — did gosec run?"
  printf '  %s\n' "· gosec (reported, not gating): $gosec_summary"
else
  skip "lint" "NOT CHECKED — golangci-lint is not on PATH. Install it: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"
  printf '  %s
' "‼ the linters did NOT run: golangci-lint is not on this machine"
fi

stage "build" go build ./...
# -count=1 defeats the test cache: a cached pass proves nothing about a tree
# that just changed underneath it.
#
# -shuffle=on because several things here are package-level state a test can
# leave behind — the progress tracker's clock, the cockpit store, the identity
# dir — and a suite that only passes in file order is a suite that will fail on
# somebody else's machine.
#
# -timeout well under the 10m default: the engine now parks goroutines on
# purpose (a sub-agent waiting on ask_main), so a deadlock is a real failure
# mode and it should report in minutes rather than at the end of a coffee break.
stage "test" go test -count=1 -shuffle=on -timeout 5m ./...

# The race detector is the check this codebase most needs and least has: a
# delegate runs on its own goroutine, tool events arrive from it while the main
# turn is still writing, and the runner's map, the parked-question slot and the
# desktop's tool history are all touched from both sides. None of that is
# provable by reading.
#
# It needs cgo, which needs a C compiler, which this machine does not have — so
# the stage skips with the command that fixes it rather than silently not
# existing. The moment gcc is installed it starts running with no edit here.
if command -v "$(go env CC)" >/dev/null 2>&1; then
  stage "race" env CGO_ENABLED=1 go test -count=1 -race -timeout 15m ./...
else
  # Loud, not quiet. This skipped silently for months while ARCHITECTURE.md
  # asserted the opposite, so the one thing that checks the delegate goroutine
  # and the parked ask_main slot was believed to be running and was not — which
  # is how a CI failure that had nothing to do with concurrency was read as a
  # data race for six days. A skip that is easy to miss is worse than no stage.
  skip "race" "NOT CHECKED — no C compiler on PATH. Install one (scoop install gcc) or trust only CI's ubuntu job"
  printf '  %s
' "‼ the race detector did NOT run: this machine has no $(go env CC) on PATH"
fi

echo
echo "Frontend"
stage "vitest" fe npx vitest run
stage "svelte-check" fe npx svelte-check --threshold error
stage "vite build" fe npx vite build

echo
echo "Live (real API)"
if [ "$LIVE" = 1 ]; then
  # Skipped by default because they cost money and need a key. They answer what
  # fakes cannot: that a real model, given the real tool batch, does the thing.
  #
  # Named one stage per claim rather than one stage for the whole desktop
  # package: when a live run goes red at 03:00 the stage name should say what
  # broke, not "the live tests".
  stage "provider progress" env AETOX_LIVE=1 go test -count=1 -timeout 15m ./internal/model/ -run TestLive
  stage "tool batch accepted" env AETOX_LIVE=1 go test -count=1 -timeout 10m ./desktop/ -run TestLiveAllToolsAccepted
  stage "chat writes files" env AETOX_LIVE=1 go test -count=1 -timeout 10m ./desktop/ -run TestLiveUnfocusedChat
  stage "sub-agents" env AETOX_LIVE=1 go test -count=1 -timeout 15m ./desktop/ -run 'TestLiveSubAgent|TestLiveTwoSubAgents'
else
  skip "provider progress" "pass --live"
  skip "tool batch accepted" "pass --live"
  skip "chat writes files" "pass --live"
  skip "sub-agents" "pass --live"
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
