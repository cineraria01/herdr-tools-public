#!/bin/bash
# hc-sweep 계약 시험을 셸 판(lib/hc-sweep/*.py)으로도(2026-11-21 util-go): 기본 실행(tests/hc-sweep*.test.sh)은 Go 판이다. 되돌리기 스위치
# HC_SWEEP_IMPL=bash가 같은 계약을 지키는지 — 두 판의 결과가 다르면 Go를 고친다. 셸 판을 지우는 날 이 파일도 지운다.
# 실행: bash tests/hc-sweep-bash.test.sh
set -u
here=$(cd "$(dirname "$0")" && pwd)
fail=0
for t in hc-sweep hc-sweep-disk; do
  if HC_SWEEP_IMPL=bash bash "$here/$t.test.sh"; then echo "ok - 셸 판 $t"; else echo "FAIL - 셸 판 $t"; fail=1; fi
done
exit $fail
