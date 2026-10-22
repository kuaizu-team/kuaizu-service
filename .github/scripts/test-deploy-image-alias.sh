#!/usr/bin/env bash
set -euo pipefail

# Run the actual deployment gates with mocked services; never contact production.
workflow="$(cd "$(dirname "$0")/.." && pwd)/workflows/deploy.yml"
test_root="$(mktemp -d)"
trap 'rm -rf "$test_root"' EXIT
sed 's/\r$//' "$workflow" | awk '
  /^            sha256sum -c kuaizu-server.tar.gz.sha256$/ {copy=1}
  copy && /^          EOF$/ {exit}
  copy {sub(/^            /, ""); print}
' | sed -e 's/\${{ env.VERSION }}/test-version/g' \
        -e 's/\${{ needs.verify.outputs.image_id }}/sha256:verified/g' \
  > "$test_root/deploy.sh"
test -s "$test_root/deploy.sh"

docker() {
  case "$1" in
    image)
      if [ "$ALIAS_TEST_CASE" = image-mismatch ]; then echo sha256:wrong;
      else echo sha256:verified; fi ;;
    inspect)
      if [ "$ALIAS_TEST_CASE" = runtime-mismatch ]; then echo sha256:wrong;
      else echo sha256:verified; fi ;;
    cp) printf 'test binary\n' > "$3" ;;
    load) cat >/dev/null ;;
    tag)
      [ "$ALIAS_TEST_CASE" != tag-failure ] || return 1
      printf '%s %s\n' "$2" "$3" > latest-image ;;
    *) return 1 ;;
  esac
}
sha256sum() {
  case "$2:$ALIAS_TEST_CASE" in
    kuaizu-server.tar.gz.sha256:artifact-failure|../binaries.sha256:binaries-failure) return 1 ;;
  esac
}
gunzip() { printf 'mock image archive\n'; }
compose() { [ "$ALIAS_TEST_CASE" != compose-failure ]; }
ensure_compose_owned_or_absent() { :; }
wait_http() {
  case "$1:$ALIAS_TEST_CASE" in
    kuaizu-api:api-failure|kuaizu-console:console-failure) return 1 ;;
  esac
}
export -f docker sha256sum gunzip compose ensure_compose_owned_or_absent wait_http

for scenario in pass artifact-failure image-mismatch compose-failure api-failure console-failure binaries-failure runtime-mismatch tag-failure; do
  export ALIAS_TEST_CASE="$scenario"
  mkdir "$test_root/$scenario"
  (
    cd "$test_root/$scenario"
    touch kuaizu-server.tar.gz
    printf '{}\n' > release-metadata.json
    printf 'sha256:previous kuaizu-server:latest\n' > latest-image
    if bash -euo pipefail "$test_root/deploy.sh" > result.log 2>&1; then
      test "$scenario" = pass
      test "$(cat latest-image)" = 'sha256:verified kuaizu-server:latest'
    else
      if [ "$scenario" = pass ]; then cat result.log; exit 1; fi
      test "$(cat latest-image)" = 'sha256:previous kuaizu-server:latest'
    fi
  )
  echo "Deployment image alias: $scenario passed"
done
