#!/bin/bash
# herdr-balance 시험을 셸 판으로도(2026-11-21 util-go): 기본 실행(tests/herdr-balance.test.sh)은 Go 판이다. 되돌리기 스위치
# HERDR_BALANCE_IMPL=bash(lib/herdr-balance/herdr-balance.py)가 같은 기대값을 지나는지 — 두 판이 다르면 Go를 고친다. 셸 판을 지우는 날 이 파일도 지운다.
# 실행: bash tests/herdr-balance-bash.test.sh
set -u
here=$(cd "$(dirname "$0")" && pwd)
if HERDR_BALANCE_IMPL=bash bash "$here/herdr-balance.test.sh"; then echo "ok - 셸 판 herdr-balance"; else echo "FAIL - 셸 판 herdr-balance"; exit 1; fi
