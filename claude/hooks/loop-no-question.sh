#!/bin/bash
# loop-no-question: 무인 루프(이름이나 job 라벨에 loop가 든 hc 터미널)에서 질문 창(AskUserQuestion)을 막는다.
# 2026-11-07 사용자 결정 "질문 창 막는 훅 (추천)": 151 D1 루프가 질문 창에서 2시간 멈췄다.
# hc 터미널이 아니거나 hc를 못 읽으면 막지 않는다.
# Go 판(cmd/herdr-hooks — 같은 판정)으로 넘긴다. HERDR_HOOKS_IMPL=bash(또는 HERDR_IMPL=bash)면 아래 셸 판 그대로(2026-11-21 hooks-go).
# 바이너리가 없거나 소스보다 낡았으면 이번은 셸 판, 짓기는 뒤에서(훅 제한 안에 짓지 않는다 — lib/go-run.sh GO_RUN_BUILD=bg)
for _hg in "${BASH_SOURCE[0]%/*}/../../lib/go-run.sh" "${BASH_SOURCE[0]%/*}/../../.local/lib/go-run.sh"; do
  [ -r "$_hg" ] || continue
  . "$_hg" && GO_RUN_BUILD=bg go_run herdr-hooks loop-no-question || exit 1
  break
done
cat >/dev/null 2>&1
[ -n "$HC_TERMINAL_ID" ] || exit 0
HC=$(command -v hc 2>/dev/null || echo "$HOME/.local/bin/hc")
[ -x "$HC" ] || exit 0
j=$("$HC" get "$HC_TERMINAL_ID" --json 2>/dev/null) || exit 0
name=$(printf '%s' "$j" | plutil -extract name raw -o - - 2>/dev/null)
job=$(printf '%s' "$j" | plutil -extract labels.job raw -o - - 2>/dev/null)
case "$name $job" in
  *[Ll][Oo][Oo][Pp]*) ;;
  *) exit 0 ;;
esac
cat >&2 <<EOF
무인 루프(${name:-$HC_TERMINAL_ID})에서는 질문 창을 띄우지 않는다. 답할 사람이 없어 루프 전체가 멈춘다.
- 되돌릴 수 있는 판단은 추천안으로 정해 상태 파일에 「가정/결정」으로 적고 진행한다.
- 사용자만 정할 일(되돌릴 수 없는 일·비용·정책)은 상태 파일 「결정 필요」에 적고 그 건만 보류한 채 다음 일을 계속한다.
EOF
exit 2
