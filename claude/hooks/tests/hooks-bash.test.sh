#!/bin/bash
# 훅 시험을 셸 판(HERDR_HOOKS_IMPL=bash)으로도 — 두 판이 같은 판정을 지키는지(다르면 Go를 고친다). 다른 훅 시험은 기본이 Go 판이다.
# 셸 판을 걷는 날(「셸 판 걷기」 레인) 이 파일도 지운다. 실행: bash <이 파일>
here=$(cd "$(dirname "$0")" && pwd)
fail=0
for t in "$here"/*.test.sh; do
  case $t in */hooks-bash.test.sh) continue ;; esac
  if HERDR_HOOKS_IMPL=bash bash "$t" >"${TMPDIR:-/tmp}/hooks-bash.$$.log" 2>&1; then
    echo "ok   bash 판 ${t##*/}"
  else
    echo "FAIL bash 판 ${t##*/}"; grep -E '^FAIL' "${TMPDIR:-/tmp}/hooks-bash.$$.log" | head -20; fail=1
  fi
done
rm -f "${TMPDIR:-/tmp}/hooks-bash.$$.log"
exit $fail
