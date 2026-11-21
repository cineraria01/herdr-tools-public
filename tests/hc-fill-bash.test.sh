#!/bin/bash
# hc-fill 계약 시험을 셸 판으로도(2026-11-21 hc-fill-go): 기본 실행(tests/hc-fill.test.sh)은 Go 판이다. 되돌리기 스위치
# HC_FILL_IMPL=bash가 같은 계약을 지키는지 — 두 판의 결과가 다르면 Go를 고친다. 셸 판을 지우는 날 이 파일도 지운다.
# 실행: bash tests/hc-fill-bash.test.sh
set -u
here=$(cd "$(dirname "$0")" && pwd)
if HC_FILL_IMPL=bash bash "$here/hc-fill.test.sh"; then echo "ok - bash 판 hc-fill"; else echo "FAIL - bash 판 hc-fill"; exit 1; fi
