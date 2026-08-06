#!/usr/bin/env bash
# 闭环验证：退出码 0 = 通过。每项检查失败立即退出并给出可定位的输出。
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> gofmt check"
test -z "$(gofmt -l .)" || { echo "gofmt dirty:"; gofmt -l .; exit 1; }

echo "==> go vet"
go vet ./...

echo "==> go test -race"
go test -race ./...

echo "==> OMO companion sync tests"
go test -race ./internal/tools/ ./internal/profile/ -count=1 \
  -run 'TestOpenCodeApplyAuthSyncsOMO|TestOpenCodeApplyAuthSkipsMissingOMO|TestSyncOMOFromOpenCodeLive|TestOpenCodeSwitchSyncsOMO'

echo "==> build"
go build -trimpath -o /tmp/charon-verify ./cmd/charon

echo "verify OK"
