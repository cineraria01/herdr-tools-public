#!/bin/bash
# 작은 도구 계약 시험을 셸 판으로도: hc-sweep·herdr-balance·ovis-split·worker-mac-check·worker-mac-setup의 Go 계약 시험
# (tests/util — 옛 tests/<도구>.test.sh를 옮긴 것, 2026-11-21 go-tests-util)을 HERDR_IMPL=bash로 한 번 더.
# 기본 판(go)은 tests/hc-merger-go.test.sh의 go test ./...가 돈다. 두 판의 결과가 다르면 Go를 고친다. 셸 판을 걷는 날 이 파일도 지운다.
# 실행: bash tests/util-bash.test.sh   (저장소에서만 — 깔지 않는다)
set -u
here=$(cd "$(dirname "$0")" && pwd)
[ -d "$here/util" ] || { echo "ok - tests/util 없음(깔린 곳) — 건너뜀"; exit 0; }
gob=$(command -v go 2>/dev/null) || gob=
for a in /opt/homebrew/bin/go /usr/local/go/bin/go /usr/local/bin/go; do [ -n "$gob" ] || [ ! -x "$a" ] || gob=$a; done
[ -n "$gob" ] || { echo "FAIL - go가 없다(brew install go)"; exit 1; }
cd "$here/.." || exit 1
export GOTOOLCHAIN=local GOCACHE=${GOCACHE:-$PWD/.bin/go-build}
if out=$(HERDR_IMPL=bash "$gob" test -count=1 -timeout 600s ./tests/util/ 2>&1); then echo "ok - 셸 판 작은 도구(tests/util)"
else echo "FAIL - 셸 판 작은 도구(tests/util)"; printf '%s\n' "$out" | grep -E 'FAIL - ' ; printf '%s\n' "$out" | tail -30; exit 1; fi
