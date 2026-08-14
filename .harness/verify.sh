#!/usr/bin/env bash
# 闭环验证：退出码 0 = 通过。每项检查失败立即退出并给出可定位的输出。
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> gofmt"
unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
  echo "gofmt needed:"
  echo "$unformatted"
  exit 1
fi

echo "==> go vet"
go vet ./...

echo "==> update source tests"
go test -race -count=1 ./cmd/1api/ -run 'TestUpdate|TestPreferGitee|TestGiteeInstall|TestShellQuote'

echo "==> go test -race"
go test -race -count=1 ./...

echo "verify: ok"
