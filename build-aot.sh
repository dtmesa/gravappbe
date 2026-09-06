#!/usr/bin/env bash
# Builds the Native AOT Lambda artifact into .aot-build/.
#
# This does not go through `sam build`: its dotnet builder passes
# --self-contained False, which the SDK rejects when PublishAot is set, and the
# provided.al2023 build image ships no dotnet SDK at all. Compiling in the
# Amazon Linux 2023 dotnet image instead keeps the binary's glibc compatible
# with the provided.al2023 runtime it has to run on.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
IMAGE="public.ecr.aws/sam/build-dotnet10:latest-x86_64"
OUT="$ROOT/.aot-build"

rm -rf "$OUT"
mkdir -p "$OUT"

MSYS_NO_PATHCONV=1 docker run --rm --entrypoint /bin/bash \
	-v "$(cygpath -w "$ROOT/src/Gravity.Api" 2>/dev/null || echo "$ROOT/src/Gravity.Api")":/src \
	-v "$(cygpath -w "$OUT" 2>/dev/null || echo "$OUT")":/out \
	-w /src "$IMAGE" -c '
		set -e
		dotnet publish -c Release -r linux-x64 --self-contained true -p:PublishAot=true -o /tmp/publish
		# provided.al2023 invokes an executable named bootstrap.
		mv /tmp/publish/Gravity.Api /tmp/publish/bootstrap
		# The separate debug file more than doubles the package for no runtime benefit.
		rm -f /tmp/publish/*.dbg
		rm -rf /out/* 2>/dev/null || true
		cp -rf /tmp/publish/. /out/
		chmod +x /out/bootstrap
	'

echo "artifact:"
ls -la "$OUT"
