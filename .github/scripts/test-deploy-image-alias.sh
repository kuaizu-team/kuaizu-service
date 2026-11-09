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
        -e 's/\${{ github.sha }}/test-commit/g' \
        -e 's/\${{ needs.verify.outputs.archive_sha256 }}/trusted-archive-hash/g' \
  > "$test_root/deploy.sh"
test -s "$test_root/deploy.sh"

docker() {
  case "$1" in
    image)
      case "$4" in
        '{{.Id}}')
          [ "$ALIAS_TEST_CASE" != image-missing ] || return 1
          echo sha256:server ;;
        *image.revision*)
          if [ "$ALIAS_TEST_CASE" = image-mismatch ]; then echo wrong-commit;
          else echo test-commit; fi ;;
        *image.version*)
          if [ "$ALIAS_TEST_CASE" = version-mismatch ]; then echo wrong-version;
          else echo test-version; fi ;;
        *) return 1 ;;
      esac ;;
    create)
      [ "$ALIAS_TEST_CASE" != probe-failure ] || return 1
      echo stopped-probe ;;
    rm) touch probe-removed ;;
    inspect)
      if [ "$ALIAS_TEST_CASE" = runtime-mismatch ]; then echo sha256:wrong;
      else echo sha256:server; fi ;;
    cp)
      [ "$ALIAS_TEST_CASE" != copy-failure ] || return 1
      printf 'test binary\n' > "$3" ;;
    load) cat >/dev/null ;;
    tag)
      [ "$ALIAS_TEST_CASE" != tag-failure ] || return 1
      printf '%s %s\n' "$2" "$3" > latest-image ;;
    *) return 1 ;;
  esac
}
sha256sum() {
  if [ "$2" = - ]; then
    read -r checksum
    [ "$checksum" = 'trusted-archive-hash  kuaizu-server.tar.gz' ] || return 1
  fi
  if [ "$2" = ../binaries.sha256 ]; then
    case "$PWD:$ALIAS_TEST_CASE" in
      */.release-preflight:preflight-failure|*/.release-runtime:binaries-failure) return 1 ;;
    esac
  fi
  case "$2:$ALIAS_TEST_CASE" in
    kuaizu-server.tar.gz.sha256:artifact-failure) return 1 ;;
    -:trusted-archive-failure) return 1 ;;
  esac
}
gunzip() { printf 'mock image archive\n'; }
compose() {
  touch compose-called
  [ "$ALIAS_TEST_CASE" != compose-failure ]
}
fail_with_logs() { echo "$1"; exit 1; }
ensure_compose_owned_or_absent() { :; }
wait_http() {
  case "$1:$ALIAS_TEST_CASE" in
    kuaizu-api:api-failure|kuaizu-console:console-failure) return 1 ;;
  esac
}
export -f docker sha256sum gunzip compose ensure_compose_owned_or_absent wait_http fail_with_logs

for scenario in pass artifact-failure trusted-archive-failure image-missing image-mismatch version-mismatch probe-failure copy-failure preflight-failure compose-failure api-failure console-failure binaries-failure runtime-mismatch tag-failure; do
  export ALIAS_TEST_CASE="$scenario"
  mkdir "$test_root/$scenario"
  (
    cd "$test_root/$scenario"
    touch kuaizu-server.tar.gz
    printf '{}\n' > release-metadata.json
    printf 'sha256:previous kuaizu-server:latest\n' > latest-image
    if bash -euo pipefail "$test_root/deploy.sh" > result.log 2>&1; then
      test "$scenario" = pass
      test "$(cat latest-image)" = 'sha256:server kuaizu-server:latest'
      test "$(cat .release-runtime/server-image-id.txt)" = sha256:server
    else
      if [ "$scenario" = pass ]; then cat result.log; exit 1; fi
      test "$(cat latest-image)" = 'sha256:previous kuaizu-server:latest'
    fi
    case "$scenario" in
      artifact-failure|trusted-archive-failure|image-missing|image-mismatch|version-mismatch|probe-failure|copy-failure|preflight-failure) test ! -f compose-called ;;
    esac
    case "$scenario" in
      artifact-failure|trusted-archive-failure|image-missing|image-mismatch|version-mismatch|probe-failure) : ;;
      *) test -f probe-removed ;;
    esac
  )
  echo "Deployment image alias: $scenario passed"
done
