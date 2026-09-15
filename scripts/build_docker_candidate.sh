#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  cat <<'USAGE'
Usage: scripts/build_docker_candidate.sh [image-tag] [oci-output]

Build a clean linux/amd64 + linux/arm64 candidate OCI image without leaving
cross-compiled binaries in the repository. Set BASEWORK_VERSION and
BASEWORK_BUILD_DATE to control version metadata, and BUILDX_BUILDER to select
the buildx builder.
USAGE
  exit 0
fi

command -v go >/dev/null
command -v git >/dev/null
command -v docker >/dev/null
docker buildx version >/dev/null

short_commit="$(git rev-parse --short HEAD)"
image_tag="${1:-basework:candidate-${short_commit}}"
output_path="${2:-${TMPDIR:-/tmp}/basework-candidate-${short_commit}.oci}"
version="${BASEWORK_VERSION:-$(git describe --tags --always --dirty)}"
build_date="${BASEWORK_BUILD_DATE:-$(date -u '+%Y-%m-%dT%H:%M:%SZ')}"
builder="${BUILDX_BUILDER:-}"

stage_dir="$(mktemp -d "${TMPDIR:-/tmp}/basework-docker-candidate.XXXXXX")"
cleanup() {
  rm -rf "$stage_dir"
}
trap cleanup EXIT

mkdir -p "$stage_dir/linux/amd64" "$stage_dir/linux/arm64"
cp docker/Dockerfile.goreleaser "$stage_dir/Dockerfile.goreleaser"
mkdir -p "$(dirname "$output_path")"

ldflags="-s -w -X main.version=${version} -X main.commit=${short_commit} -X main.date=${build_date}"
for arch in amd64 arm64; do
  echo "building linux/${arch} candidate binary"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
    go build -tags 'sqlite memory' -ldflags "$ldflags" \
    -o "$stage_dir/linux/${arch}/basework" ./cmd/basework
done

build_args=(buildx build --platform linux/amd64,linux/arm64
  --file "$stage_dir/Dockerfile.goreleaser"
  --tag "$image_tag"
  --output "type=oci,dest=${output_path}")
if [[ -n "$builder" ]]; then
  build_args+=(--builder "$builder")
fi

docker "${build_args[@]}" "$stage_dir"
shasum -a 256 "$output_path"
