#!/usr/bin/env bash
set -euo pipefail

image="${1:?image reference is required}"
output="${2:?evidence directory is required}"
expected_go="${3:?expected Go version is required}"
mkdir -p "$output"

# Creating a stopped container only reads the image; it never starts workers.
container="$(docker create "$image")"
trap 'docker rm "$container" >/dev/null' EXIT

for binary in kuaizu-server kuaizu-admin; do
  docker cp "$container:/app/$binary" "$output/$binary"
  go version -m "$output/$binary" | tee "$output/$binary.buildinfo.txt"
  actual_go="$(awk 'NR == 1 {print $NF}' "$output/$binary.buildinfo.txt")"
  if [ "$actual_go" != "$expected_go" ]; then
    echo "::error::$binary was built with $actual_go; expected $expected_go"
    exit 1
  fi
  govulncheck -mode=binary "$output/$binary" | tee "$output/$binary.vuln.txt"
done

(
  cd "$output"
  sha256sum kuaizu-server kuaizu-admin > binaries.sha256
)
docker image inspect --format '{{.Id}}' "$image" > "$output/image-id.txt"
