#!/bin/bash
# worker-mac-check·worker-mac-setup 시험을 bash 판으로도(2026-11-21 util-go): 기본 실행(tests/worker-mac.test.sh·worker-mesh.test.sh)은 Go 판이다.
# 되돌리기 스위치 WORKER_MAC_CHECK_IMPL=bash·WORKER_MAC_SETUP_IMPL=bash가 같은 계약을 지키는지 — 두 판의 결과가 다르면 Go를 고친다.
# bash 판(lib/worker-mac-check·lib/worker-mac-setup)을 지우는 날 이 파일도 지운다.
# 실행: bash tests/worker-mac-bash.test.sh
set -u
here=$(cd "$(dirname "$0")" && pwd)
fail=0
for t in worker-mac worker-mesh; do
  if WORKER_MAC_CHECK_IMPL=bash WORKER_MAC_SETUP_IMPL=bash bash "$here/$t.test.sh"; then echo "ok - bash 판 $t"; else echo "FAIL - bash 판 $t"; fail=1; fi
done
exit $fail
