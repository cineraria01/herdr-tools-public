# hooks-impl.sh: 훅 시험이 source한다 — 어느 판으로 돌릴지 정한다(2026-11-21 hooks-go).
#   HERDR_HOOKS_IMPL=go(기본): 이 시험만의 Go 바이너리를 지어(-tags hookstest — 시험 손잡이 HERDR_HOOKS_TEST_* 가 먹는 빌드)
#     HERDR_HOOKS_BIN으로 넘긴다. 훅 셸 파일이 lib/go-run.sh로 그 바이너리를 exec한다(못 쓰면 종료 1 — 셸 판으로 슬쩍 넘어가지 않는다)
#   HERDR_HOOKS_IMPL=bash: 셸 판 그대로(hooks-bash.test.sh가 부른다)
# go가 없으면 go 판 시험은 실패한다(Go 판을 쓰는 기계에는 go가 있어야 한다)
export HERDR_HOOKS_IMPL=${HERDR_HOOKS_IMPL:-go}
unset HERDR_IMPL HERDR_HOOKS_TEST_GIT HERDR_HOOKS_TEST_NOW_OFFSET HERDR_HOOKS_TEST_SECONDS
if [ "$HERDR_HOOKS_IMPL" = go ]; then
  hi_here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
  hi_src=$hi_here/../../..   # 저장소 claude/hooks/tests → 루트
  [ -f "$hi_src/go.mod" ] || hi_src=$hi_here/../../../.local/lib/herdr-go   # 깔린 곳 ~/.claude/hooks/tests → ~/.local/lib/herdr-go
  hi_go=$(command -v go 2>/dev/null) || hi_go=
  for hi_a in /opt/homebrew/bin/go /usr/local/go/bin/go /usr/local/bin/go; do [ -n "$hi_go" ] || [ ! -x "$hi_a" ] || hi_go=$hi_a; done
  [ -n "$hi_go" ] || { echo "FAIL go가 없다(brew install go) — HERDR_HOOKS_IMPL=bash로 셸 판만 볼 수 있다"; exit 1; }
  hi_bin=$(mktemp -d "${TMPDIR:-/tmp}/herdr-hooks-bin.XXXXXX") || exit 1
  (cd "$hi_src" && GOCACHE=${GOCACHE:-$PWD/.bin/go-build} GOPATH=${GOPATH:-$PWD/.bin/gopath} GOTOOLCHAIN=local CGO_ENABLED=0 \
    "$hi_go" build -tags hookstest -trimpath -buildvcs=false -o "$hi_bin/herdr-hooks" ./cmd/herdr-hooks) ||
    { echo "FAIL herdr-hooks 짓기 실패($hi_src)"; exit 1; }
  export HERDR_HOOKS_BIN=$hi_bin/herdr-hooks
  echo "판 go ($HERDR_HOOKS_BIN)"
else
  echo "판 $HERDR_HOOKS_IMPL"
fi
