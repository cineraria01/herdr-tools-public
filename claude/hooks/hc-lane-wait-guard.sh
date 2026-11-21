#!/bin/bash
# hc-lane-wait-guard: PreToolUse(Bash) 훅. `hc-lane wait`를 셸 분리 실행(& · nohup · disown · setsid)하면 막는다.
# 2026-11-17 지휘가 두 번 `hc-lane wait … >/dev/null &`·`hc-lane wait … | tail -3 >/dev/null &`로 걸어 대기 결과를 못 봤다
# (hc-lane의 /dev/null 거절은 파이프 뒤 /dev/null로 피해 갔다). 기다리려면 포그라운드로 돌리거나 Bash run_in_background로 돌린다.
# jq가 없거나 입력을 못 읽으면 막지 않는다(편의 막이라 fail-open).
# Go 판(cmd/herdr-hooks — 같은 판정)으로 넘긴다. HERDR_HOOKS_IMPL=bash(또는 HERDR_IMPL=bash)면 아래 셸 판 그대로(2026-11-21 hooks-go).
# 바이너리가 없거나 소스보다 낡았으면 이번은 셸 판, 짓기는 뒤에서(훅 제한 안에 짓지 않는다 — lib/go-run.sh GO_RUN_BUILD=bg)
# 실행기는 이 파일이 놓인 배치의 것 하나만(저장소 lib/ · 설치 ~/.local/lib/), 없는데 Go 판을 못박았으면 1 — cotenant-guard.sh와 같다
_hd=${BASH_SOURCE[0]%/*} _hg=${BASH_SOURCE[0]%/*}/../../.local/lib/go-run.sh
[ -r "$_hd/../../cmd/herdr-hooks/main.go" ] && _hg=$_hd/../../lib/go-run.sh
if [ -r "$_hg" ]; then . "$_hg" && GO_RUN_BUILD=bg go_run herdr-hooks hc-lane-wait-guard || exit 1
elif [ "${HERDR_HOOKS_IMPL:-${HERDR_IMPL:-auto}}" = go ]; then echo "hc-lane-wait-guard: 공통 실행기가 없다: $_hg (HERDR_HOOKS_IMPL=go — 셸 판으로 넘어가지 않는다)" >&2; exit 1; fi
JQ=/usr/bin/jq; [ -x "$JQ" ] || JQ=$(command -v jq) || exit 0
in=$(cat) || exit 0
[ "$(printf '%s' "$in" | "$JQ" -r '.tool_input.run_in_background // false' 2>/dev/null)" = true ] && exit 0
cmd=$(printf '%s' "$in" | "$JQ" -r '.tool_input.command // ""' 2>/dev/null) || exit 0
printf '%s' "$cmd" | grep -Eq 'hc-lane[[:space:]]+wait' || exit 0
# ponytail: 명령 전체에서 홀로 선 &를 찾는다(따옴표 안·다른 명령의 &도 걸림). &&·>&·&>·|&·<&는 뺀다
amp=$(printf '%s' "$cmd" | sed -E 's/&&//g; s/[<>|]&//g; s/&>//g')
if printf '%s' "$amp" | grep -q '&' \
  || printf '%s' "$cmd" | grep -Eq '(^|[^[:alnum:]_-])(nohup|disown|setsid)([^[:alnum:]_-]|$)'; then
  echo "hc-lane wait는 분리(&) 실행하지 말고 Bash run_in_background로 돌려 알림을 받으세요(또는 포그라운드로). 분리하면 대기 결과를 아무도 못 봅니다." >&2
  exit 2
fi
exit 0
