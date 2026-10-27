#!/usr/bin/env bash
# Builds the Lambda artifact into .go-build/bootstrap for provided.al2023 on
# arm64. Plain cross-compilation -- no Docker, no SDK image.
#
#   ./build.sh && sam deploy --template template.yaml ...
#
# Deploy without `sam build`: template.yaml's CodeUri points at .go-build/.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

rm -rf .go-build
mkdir -p .go-build

# lambda.norpc drops the legacy RPC mode provided.al2023 never uses.
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
	go build -tags lambda.norpc -trimpath -ldflags="-s -w" -o .go-build/bootstrap ./cmd/lambda

ls -la .go-build
