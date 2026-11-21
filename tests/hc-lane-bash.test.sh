#!/bin/bash
# hc-lane 계약 시험을 bash 판으로도(2026-11-21 hc-lane-go-land·-start): 기본(tests/hc-lane-go.test.sh)은 Go 판이 맡은 하위 명령(띄우기·
# clean·gc·park·land·guard·lease·fetch …)을 Go로 돈다. 되돌리기 스위치 HC_LANE_IMPL=bash가 같은 계약을 지키는지 — 두 판의 결과가 다르면 Go를 고친다.
# 시험 본체는 Go 시험 tests/hclane(2026-11-21 go-tests-lane이 셸 시험에서 옮김). 병합기 시험은 hc-lane-merger-bash.test.sh가 따로.
# bash 판을 지우는 날 이 파일도 지운다.
# 실행: bash tests/hc-lane-bash.test.sh
here=$(cd "$(dirname "$0")" && pwd)
HC_LANE_IMPL=bash exec bash "$here/hc-lane-go.test.sh"
