#!/bin/bash
# 병합기 계약 시험을 bash 판으로도(2026-11-21 merger-go): 기본 실행(tests/hc-lane-merger*.test.sh)은 Go 판이다. 되돌리기 스위치
# HC_LANE_MERGER_IMPL=bash가 같은 계약을 지키는지 — 두 판의 결과가 다르면 Go를 고친다. bash 판을 지우는 날 이 파일도 지운다.
# 실행: bash tests/hc-lane-merger-bash.test.sh
set -u
here=$(cd "$(dirname "$0")" && pwd)
fail=0
for t in hc-lane-merger hc-lane-merger-recovery; do
  if HC_LANE_MERGER_IMPL=bash bash "$here/$t.test.sh"; then echo "ok - bash 판 $t"; else echo "FAIL - bash 판 $t"; fail=1; fi
done
exit $fail
