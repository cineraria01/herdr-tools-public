#!/bin/bash
# ovis-split 시험을 셸 판으로도(2026-11-21 util-go): 기본 실행(tests/ovis-split.test.sh)은 Go 판이다. 되돌리기 스위치
# OVIS_SPLIT_IMPL=bash(lib/ovis-split/ovis-split.py)가 같은 기대값을 지나는지 — 두 판이 다르면 Go를 고친다. 셸 판을 지우는 날 이 파일도 지운다.
# 실행: bash tests/ovis-split-bash.test.sh
set -u
here=$(cd "$(dirname "$0")" && pwd)
if OVIS_SPLIT_IMPL=bash bash "$here/ovis-split.test.sh"; then echo "ok - 셸 판 ovis-split"; else echo "FAIL - 셸 판 ovis-split"; exit 1; fi
