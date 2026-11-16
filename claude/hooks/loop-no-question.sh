#!/bin/bash
# loop-no-question: 무인 루프(이름이나 job 라벨에 loop가 든 hc 터미널)에서 질문 창(AskUserQuestion)을 막는다.
# 2026-11-07 사용자 결정 "질문 창 막는 훅 (추천)": 151 D1 루프가 질문 창에서 2시간 멈췄다.
# hc 터미널이 아니거나 hc를 못 읽으면 막지 않는다.
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
