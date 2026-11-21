#!/bin/bash
# 병합기 Go 판(lib/hc-merger) 단위 시험: gofmt·go vet·go test -race(프로세스 무리 정리·잠금 안 물림·git 모름 → 쉼·우선순위·복구 단계별 죽기).
# go가 없으면 실패한다 — 병합기 Go 판을 쓰는 기계에는 go가 있어야 한다(없으면 hc-lane이 경고하고 bash 판으로 돈다)
# 실행: bash tests/hc-merger-go.test.sh
set -u
here=$(cd "$(dirname "$0")" && pwd)
src=$here/../lib/hc-merger
[ -f "$src/go.mod" ] || src=$here/../../lib/hc-merger   # 깔린 곳(~/.local/bin/tests → ~/.local/lib/hc-merger)
gob=$(command -v go 2>/dev/null) || gob=
for a in /opt/homebrew/bin/go /usr/local/go/bin/go /usr/local/bin/go; do [ -n "$gob" ] || [ ! -x "$a" ] || gob=$a; done
[ -n "$gob" ] || { echo "FAIL - go가 없다(brew install go)"; exit 1; }
cd "$src" || { echo "FAIL - 소스 없음: $src"; exit 1; }
export GOTOOLCHAIN=local GOCACHE=${GOCACHE:-$PWD/.bin/go-build}
fail=0
gofmt=${gob%/go}/gofmt; [ -x "$gofmt" ] || gofmt=gofmt
f=$("$gofmt" -l .)
if [ -z "$f" ]; then echo "ok - gofmt"; else echo "FAIL - gofmt: $f"; fail=1; fi
if "$gob" vet ./...; then echo "ok - go vet"; else echo "FAIL - go vet"; fail=1; fi
if "$gob" test -race -count=1 ./...; then echo "ok - go test -race"; else echo "FAIL - go test -race"; fail=1; fi
exit $fail
