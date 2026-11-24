#!/bin/bash
# herdr-tools 저장소 검증 — hc-lane land·병합기가 `bash .claude/herdr-verify.sh`로 부른다(종료 0만 통과). 저장소 간 계약이라 파일은 남기고,
# 본문은 Go(tests/repo/verify): go vet ./... · go test ./tests/repo/(gofmt·셸 문법·줄 수 예산·개인 값·비밀 꼴) → go test -race
# (Go 계약 시험 전부 — TestCotenantGuard 홀로, 그다음 조각 여럿 함께: 2026-11-23 verify-shard, 한 꾸러미 안 차례 실행이 검증 시간의
# 거의 전부였다), 단계마다 고아 검사. 인자 --cheap-only는 싼 검사까지(.claude/herdr-precheck.sh). 환경 HERDR_VERIFY_ALL·
# HERDR_VERIFY_JOBS(함께 도는 시험 바이너리 자리 수, 없으면 코어 수, 자리가 모자란 조각 뒤의 들어가는 조각을 먼저 띄운다)·
# HERDR_VERIFY_NO_LOCK=1(기계 단위 검증 잠금 — 사용자 캐시 herdr-tools/verify.lock, 무거운 단계 앞, 30분 상한 뒤 그대로 진행 — 을 안 본다)·
# HERDR_VERIFY_RAMTMP(macOS에서 조각 단계의 TMPDIR을 메모리 디스크로 — 없으면 켬·0 끔·<MB> 크기, 기계 메모리의 1/4을 넘으면 안 켬,
# 못 만들면 알림 한 줄 뒤 보통 임시 폴더로: 2026-11-24 verify-ramdisk, 디스크의 작은 쓰기가 병목이었다)·
# HERDR_VERIFY_SCOPE(조각마다 입력 지문과 이 기계의 통과 기록 — 사용자 캐시 herdr-tools/verify-pass: 없음·observe는 전부 돌되 건너뛸 수
# 있었던 조각을 알리고 모든 조각이 기록과 같을 때만 건너뜀, on은 기록과 같은 조각을 건너뜀, off는 지문 없이 전부 — 2026-11-24 verify-scope,
# docs/repo/tests.md 「검증의 범위」)·HERDR_VERIFY_NO_LINT는 tests/repo/verify/main.go·
# tests/repo/secrets_test.go 머리말, 조각 표는 docs/repo/tests.md 「검증의 조각 실행」.
set -u
cd "$(dirname "$0")/.." || exit 1
# 추측 검증 기록(docs/hc-lane/merger-spec-verify.md): hc-lane land는 같은 트리·같은 이 스크립트 blob의 원격 기록 refs/herdr/verified/<트리>가 있을 때만
# HERDR_VERIFIED_TREE=<트리>를 넘긴다. 지금 HEAD^{tree}가 그 값이고, 추적 파일이 깨끗하고, 원격에 그 기록이 아직 있을 때만 건너뛴다
# (--cheap-only는 늘 돈다). 하나라도 아니거나 조회가 실패하면 그대로 검증한다
if [ -n "${HERDR_VERIFIED_TREE-}" ] && [ "${1-}" != --cheap-only ]; then
  vt_=$(git rev-parse -q --verify 'HEAD^{tree}' 2>/dev/null) || vt_=
  if [ "$vt_" = "$HERDR_VERIFIED_TREE" ] && vs_=$(git status --porcelain --untracked-files=no 2>/dev/null) && [ -z "$vs_" ] &&
    vr_=$(git ls-remote origin "refs/herdr/verified/$vt_" 2>/dev/null) && [ -n "$vr_" ]; then
    echo "건너뜀 검증 — 이 트리($vt_)는 이미 통과(refs/herdr/verified)"
    exit 0
  fi
  echo "HERDR_VERIFIED_TREE가 지금 트리·작업 트리·원격 기록과 맞지 않다 — 그대로 검증한다"
fi
command -v go >/dev/null 2>&1 || PATH=/opt/homebrew/bin:/usr/local/go/bin:/usr/local/bin:$PATH   # launchd의 짧은 PATH
export GOTOOLCHAIN=local
d=$(mktemp -d "${TMPDIR:-/tmp}/herdr-verify.XXXXXX") || exit 1
go build -o "$d/verify" ./tests/repo/verify || { echo "실패 검증 본문(tests/repo/verify)을 못 지음"; exit 1; }
exec "$d/verify" "$@"
