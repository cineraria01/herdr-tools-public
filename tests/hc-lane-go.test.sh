#!/bin/bash
# hc-lane 계약 시험(tests/hclane — 옛 tests/hc-lane.test.sh·hc-lane-stale-land.test.sh를 옮긴 Go 시험, 2026-11-21 go-tests-lane)을 Go 판으로.
# 셸 판은 tests/hc-lane-bash.test.sh. 전체가 몇 분 걸려 tests/hc-merger-go.test.sh의 go test ./...(300초)에서는 빼고 여기서 돈다.
# 깔린 곳(~/.local/bin/tests)에는 tests/hclane이 없어 건너뛴다(ok로 끝남).
# 실행: bash tests/hc-lane-go.test.sh   (다른 사본은 HC_LANE_BIN=<절대 경로>)
set -u
here=$(cd "$(dirname "$0")" && pwd)
[ -d "$here/hclane" ] && [ -f "$here/../go.mod" ] || { echo "ok - 저장소 밖이라 건너뜀(tests/hclane 없음)"; exit 0; }
gob=$(command -v go 2>/dev/null) || gob=
for a in /opt/homebrew/bin/go /usr/local/go/bin/go /usr/local/bin/go; do [ -n "$gob" ] || [ ! -x "$a" ] || gob=$a; done
[ -n "$gob" ] || { echo "FAIL - go가 없다(brew install go)"; exit 1; }
cd "$here/.." || exit 1
export GOTOOLCHAIN=local GOCACHE=${GOCACHE:-$PWD/.bin/go-build}
impl=${HC_LANE_IMPL:-${HERDR_IMPL:-go}}
fail=0
if [ "$impl" = go ]; then
  gofmt=${gob%/go}/gofmt; [ -x "$gofmt" ] || gofmt=gofmt
  f=$("$gofmt" -l tests/hclane)
  if [ -z "$f" ]; then echo "ok - gofmt tests/hclane"; else echo "FAIL - gofmt: $f"; fail=1; fi
fi
if out=$(HC_LANE_IMPL=$impl "$gob" test -count=1 -timeout 1800s -v ./tests/hclane/ 2>&1); then echo "ok - $impl 판 hc-lane 계약 시험"
else echo "FAIL - $impl 판 hc-lane 계약 시험"; printf '%s\n' "$out" | grep -E 'FAIL [0-9]+ - |^(--- FAIL|panic)'; printf '%s\n' "$out" | tail -20; fail=1; fi
printf '%s\n' "$out" | grep -E '[0-9]+개 중 실패 [0-9]+개' | sed 's/^ *//'
exit $fail
