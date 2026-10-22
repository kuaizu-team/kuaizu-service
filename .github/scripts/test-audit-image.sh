#!/usr/bin/env bash
set -euo pipefail

# Exercise the release gate without starting Docker or business workers.
audit_script="$(cd "$(dirname "$0")" && pwd)/audit-image.sh"
export AUDIT_TEST_ROOT
AUDIT_TEST_ROOT="$(mktemp -d)"

docker() {
  case "$1" in
    create) printf 'stopped-test-container\n' ;;
    cp) printf 'test binary\n' > "$3" ;;
    rm) touch "$AUDIT_TEST_ROOT/$AUDIT_TEST_CASE.removed" ;;
    image) printf 'sha256:test-image\n' ;;
    *) return 1 ;;
  esac
}

go() {
  printf '%s: %s\n' "$3" "$AUDIT_TEST_GO"
}

govulncheck() {
  return "$AUDIT_TEST_SCAN_STATUS"
}
export -f docker go govulncheck

for scenario in pass wrong-go scanner-failure; do
  export AUDIT_TEST_CASE="$scenario"
  export AUDIT_TEST_GO=go1.26.10
  export AUDIT_TEST_SCAN_STATUS=0
  case "$scenario" in
    wrong-go) AUDIT_TEST_GO=go1.25.0 ;;
    scanner-failure) AUDIT_TEST_SCAN_STATUS=3 ;;
  esac
  if bash "$audit_script" test-image "$AUDIT_TEST_ROOT/$scenario" go1.26.10 > "$AUDIT_TEST_ROOT/$scenario.log" 2>&1; then
    if [ "$scenario" != pass ]; then
      echo "Gate incorrectly accepted $scenario"
      exit 1
    fi
    test -s "$AUDIT_TEST_ROOT/$scenario/binaries.sha256"
    test -s "$AUDIT_TEST_ROOT/$scenario/image-id.txt"
  else
    if [ "$scenario" = pass ]; then
      cat "$AUDIT_TEST_ROOT/$scenario.log"
      exit 1
    fi
    test ! -f "$AUDIT_TEST_ROOT/$scenario/binaries.sha256"
  fi
  test -f "$AUDIT_TEST_ROOT/$scenario.removed"
  echo "Image audit gate: $scenario passed"
done
