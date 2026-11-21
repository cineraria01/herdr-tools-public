#!/bin/bash
# hc-fill 계약 시험을 셸 판으로도: Go 계약 시험(tests/hcfill — 옛 tests/hc-fill.test.sh를 옮긴 것, 2026-11-21 go-tests-fill)을
# HC_FILL_IMPL=bash로 한 번 더. 기본 판(go)은 tests/hc-merger-go.test.sh의 go test ./...가 돈다. 되돌리기 스위치가 같은 계약을
# 지키는지 — 두 판의 결과가 다르면 Go를 고친다. 셸 판을 지우는 날 이 파일도 지운다.
# 실행: bash tests/hc-fill-bash.test.sh   (저장소에서만 — 깔지 않는다)
set -u
here=$(cd "$(dirname "$0")" && pwd)
gob=$(command -v go 2>/dev/null) || gob=
for a in /opt/homebrew/bin/go /usr/local/go/bin/go /usr/local/bin/go; do [ -n "$gob" ] || [ ! -x "$a" ] || gob=$a; done
[ -n "$gob" ] || { echo "FAIL - go가 없다(brew install go)"; exit 1; }
cd "$here/.." || exit 1
export GOTOOLCHAIN=local GOCACHE=${GOCACHE:-$PWD/.bin/go-build}
if out=$(HC_FILL_IMPL=bash "$gob" test -count=1 -timeout 900s -run '^TestHcFill$' ./tests/hcfill/ 2>&1); then echo "ok - bash 판 hc-fill"
else echo "FAIL - bash 판 hc-fill"; printf '%s\n' "$out" | grep -E 'FAIL [0-9]+ - '; printf '%s\n' "$out" | tail -20; exit 1; fi
