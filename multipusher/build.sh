#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-v1.0.3}"
BUILD_TIME="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
GIT_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"

mkdir -p bin
go mod tidy
go mod vendor
go build \
  -ldflags "-s -w -X 'main.Version=${VERSION}' -X 'main.BuildTime=${BUILD_TIME}' -X 'main.GitCommit=${GIT_COMMIT}'" \
  -o bin/multipusher.exe \
  .
