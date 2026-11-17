#!/bin/bash
# Tests for ~/.claude/hooks/hc-lane-wait-guard.sh. Feeds PreToolUse JSON through the same wrapper settings.json uses
# (a missing hook file = exit 0, so HOOK=/nonexistent shows the state before the hook). Run: bash <this file>  (exit 0 = pass)
HOOK=${HOOK:-$(cd "$(dirname "$0")/.." && pwd)/hc-lane-wait-guard.sh}   # the hook beside this tests folder (repo or ~/.claude/hooks)
pass=0 fail=0
# case NAME WANT_RC COMMAND [RUN_IN_BACKGROUND]
case_() {
  local inp err rc
  inp=$(jq -cn --arg c "$3" --arg b "${4-}" \
    '{tool_name: "Bash", tool_input: ({command: $c} + (if $b == "" then {} else {run_in_background: ($b == "true")} end))}')
  err=$(printf '%s' "$inp" | HOOK="$HOOK" /bin/bash -c '[ -r "$HOOK" ] || exit 0; exec /bin/bash "$HOOK"' 2>&1 >/dev/null)
  rc=$?
  if [ "$rc" = "$2" ] && { [ "$2" = 0 ] || printf '%s' "$err" | grep -q 'run_in_background'; }; then
    pass=$((pass + 1)); echo "ok   $1"
  else
    fail=$((fail + 1)); echo "FAIL $1 (rc=$rc, want $2) err=[$err]"
  fi
}
# 막아야 할 것: 분리 실행
case_ "끝 &"                 2 'hc-lane wait --mine --timeout 5 &'
case_ "파이프 뒤 /dev/null &" 2 'hc-lane wait --mine --timeout 5 2>&1 | tail -3 >/dev/null &'
case_ "nohup"                2 'nohup hc-lane wait --mine --timeout 5 >/tmp/w.log 2>&1'
case_ "중간 &"               2 'hc-lane wait --mine & sleep 1'
case_ "disown"               2 'hc-lane wait --mine >/tmp/w.log 2>&1 & disown'
case_ "setsid"               2 'setsid hc-lane wait --mine >/tmp/w.log'
case_ "bash -c 안의 &"       2 "bash -c 'hc-lane wait --mine >/tmp/w &'"
case_ "run_in_background false" 2 'hc-lane wait --mine &' false
# 통과할 것
case_ "포그라운드 파이프"     0 'hc-lane wait --mine --timeout 5 2>&1 | tail -3'
case_ "run_in_background true" 0 'hc-lane wait --mine --timeout 600' true
case_ "&& echo"              0 'hc-lane wait --mine --timeout 5 && echo done'
case_ "2>&1 · &>"            0 'hc-lane wait --mine 2>&1 | tail -3; hc-lane wait --mine &>/tmp/w.log'
case_ "다른 명령의 &"         0 'sleep 5 & wait'
case_ "명령 없음"             0 ''
echo "pass=$pass fail=$fail"
[ "$fail" = 0 ]
