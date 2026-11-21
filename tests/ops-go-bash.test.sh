#!/bin/bash
# 운영 도구 셸 판 계약 시험: herdr-sync·herdr-tools-config·herdr-tools-publish·herdr-settings-merge의 Go 계약 시험
# (cmd/<도구>/contract*_test.go — 옛 tests/<도구>.test.sh를 옮긴 것, 2026-11-21 go-tests-ops)을 HERDR_IMPL=bash로 한 번 더.
# 기본 판(go)은 tests/hc-merger-go.test.sh의 go test ./...가 돈다. 두 판의 결과가 다르면 Go를 고친다. 셸 판을 걷는 날 이 파일도 지운다.
# 깔린 곳(~/.local/lib/herdr-go)에서는 계약 시험이 저장소 밖이라 건너뛴다(ok로 끝남).
# 실행: bash tests/ops-go-bash.test.sh
set -u
here=$(cd "$(dirname "$0")" && pwd)
src=$here/..
[ -f "$src/go.mod" ] || src=$here/../../lib/herdr-go   # 깔린 곳(~/.local/bin/tests → ~/.local/lib/herdr-go)
gob=$(command -v go 2>/dev/null) || gob=
for a in /opt/homebrew/bin/go /usr/local/go/bin/go /usr/local/bin/go; do [ -n "$gob" ] || [ ! -x "$a" ] || gob=$a; done
[ -n "$gob" ] || { echo "FAIL - go가 없다(brew install go)"; exit 1; }
cd "$src" || { echo "FAIL - 소스 없음: $src"; exit 1; }
export GOTOOLCHAIN=local GOCACHE=${GOCACHE:-$PWD/.bin/go-build}
fail=0
for t in herdr-tools-config herdr-settings-merge herdr-tools-publish herdr-sync; do
  [ -d "cmd/$t" ] || continue
  if out=$(HERDR_IMPL=bash "$gob" test -count=1 -timeout 600s -run '^TestContract$' "./cmd/$t" 2>&1); then echo "ok - bash 판 $t"
  else echo "FAIL - bash 판 $t"; printf '%s\n' "$out" | grep -E 'FAIL [0-9]+ - ' ; printf '%s\n' "$out" | tail -20; fail=1; fi
done
exit $fail
