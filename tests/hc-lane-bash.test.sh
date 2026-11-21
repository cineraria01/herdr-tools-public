#!/bin/bash
# hc-lane 계약 시험을 bash 판으로도(2026-11-21 hc-lane-go-land·-start): 기본 실행(tests/hc-lane*.test.sh)은 Go 판이 맡은 하위 명령(띄우기·
# clean·gc·park·land·guard·lease·fetch …)을 Go로 돈다. 되돌리기 스위치 HC_LANE_IMPL=bash가 같은 계약을 지키는지 — 두 판의 결과가 다르면 Go를 고친다.
# 병합기 시험은 hc-lane-merger-bash.test.sh(병합기 bash 판 + 착지 Go 판)가 따로. bash 판을 지우는 날 이 파일도 지운다.
# 실행: bash tests/hc-lane-bash.test.sh
set -u
here=$(cd "$(dirname "$0")" && pwd)
fail=0
for t in hc-lane hc-lane-stale-land; do
  [ -f "$here/$t.test.sh" ] || continue   # 깔린 사본(~/.local/bin/tests/)에는 없는 것도 있다
  if HC_LANE_IMPL=bash bash "$here/$t.test.sh"; then echo "ok - bash 판 $t"; else echo "FAIL - bash 판 $t"; fail=1; fi
done
exit $fail
