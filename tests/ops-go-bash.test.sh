#!/bin/bash
# 운영 도구 셸 판 계약 시험: herdr-sync·herdr-tools-config·herdr-tools-publish·herdr-settings-merge의 시험을 HERDR_IMPL=bash로 한 번 더.
# 각 시험 파일은 기본으로 Go 판을 못박아 돈다(2026-11-21 ops-go). 두 판의 결과가 다르면 Go를 고친다. 셸 판을 걷는 날 이 파일도 지운다.
# 실행: bash tests/ops-go-bash.test.sh
set -u
here=$(cd "$(dirname "$0")" && pwd)
fail=0
for t in herdr-tools-config herdr-settings-merge herdr-tools-publish herdr-sync; do
  [ -f "$here/$t.test.sh" ] || continue   # 깔린 곳에는 일부만 있다
  if out=$(HERDR_IMPL=bash bash "$here/$t.test.sh" 2>&1); then echo "ok - bash 판 $t"
  else echo "FAIL - bash 판 $t"; printf '%s\n' "$out" | grep -E '^(FAIL|not ok)' ; printf '%s\n' "$out" | tail -20; fail=1; fi
done
exit $fail
