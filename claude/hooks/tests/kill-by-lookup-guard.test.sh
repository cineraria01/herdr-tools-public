#!/bin/bash
# Tests for ~/.claude/hooks/kill-by-lookup-guard.sh. Feeds PreToolUse JSON through the same wrapper settings.json uses
# (a missing hook file = exit 0, so HOOK=/nonexistent shows the state before the hook). Run: bash <this file>  (exit 0 = pass)
HOOK=${HOOK:-$HOME/.claude/hooks/kill-by-lookup-guard.sh}
pass=0 fail=0
# case NAME WANT_RC COMMAND
case_() {
  local inp err rc
  inp=$(jq -cn --arg c "$3" '{tool_name: "Bash", tool_input: {command: $c}}')
  err=$(printf '%s' "$inp" | HOOK="$HOOK" /bin/bash -c '[ -r "$HOOK" ] || exit 0; exec /bin/bash "$HOOK"' 2>&1 >/dev/null)
  rc=$?
  if [ "$rc" = "$2" ] && { [ "$2" = 0 ] || printf '%s' "$err" | grep -q 'pid만 kill'; }; then
    pass=$((pass + 1)); echo "ok   $1"
  else
    fail=$((fail + 1)); echo "FAIL $1 (rc=$rc, want $2) err=[$err]"
  fi
}
# 막아야 할 것: 포트·이름으로 찾아 죽이기, 범위 kill
case_ "lsof -ti | xargs kill"        2 'lsof -ti :3000 | xargs kill'
case_ "kill \$(lsof -t)"             2 'kill $(lsof -t -i:8080)'
case_ "kill -9 \`lsof -ti\`"         2 'kill -9 `lsof -ti:3000`'
case_ "lsof -nP -t 변수 경유"         2 'pids=$(lsof -nP -t -iTCP:3000 -sTCP:LISTEN); [ -n "$pids" ] && kill $pids'
case_ "\"\$(lsof -t)\" 큰따옴표 안"    2 'kill "$(lsof -ti:3000)"'
case_ "fuser -k"                     2 'fuser -k 3000/tcp'
case_ "pkill -f"                     2 'pkill -f "hc-lane wait --mine"'
case_ "pkill 이름"                    2 'pkill socat'
case_ "killall"                      2 'killall node'
case_ "/usr/bin/killall"             2 '/usr/bin/killall -9 socat'
case_ "kill \$(pgrep -f)"            2 'kill $(pgrep -f next-server)'
case_ "pgrep | xargs kill"           2 'pgrep -f "next dev" | xargs kill -TERM'
case_ "ps | grep | awk | xargs kill" 2 "ps aux | grep socat | awk '{print \$2}' | xargs kill"
case_ "kill -9 -1"                   2 'kill -9 -1'
case_ "kill 0"                       2 'kill 0'
case_ "kill -s TERM -- -1"           2 'kill -s TERM -- -1'
case_ "bash -c 안의 pkill"           2 "bash -c 'pkill -f socat'"
case_ "ssh 원격 fuser -k"            2 "ssh mini-m4 'fuser -k 3000/tcp'"
case_ "앞 명령 뒤 && pkill"          2 'cd /tmp && pkill -f next'
# 통과할 것
case_ "kill 숫자 pid"                0 'kill 12345'
case_ "kill \$PID"                   0 'kill "$PID"; wait "$PID" 2>/dev/null'
case_ "kill -9 \$SERVER_PID"         0 'kill -9 $SERVER_PID'
case_ "kill %1"                      0 'kill %1'
case_ "kill -0 확인"                 0 'kill -0 $PID 2>/dev/null && echo alive'
case_ "kill -1 <pid> (HUP)"          0 'kill -1 4321'
case_ "kill \$(cat pid 파일)"        0 'kill $(cat /tmp/next.pid)'
case_ "lsof -i 조회만"               0 'lsof -nP -iTCP:3000 -sTCP:LISTEN'
case_ "lsof -ti 조회만"              0 'lsof -ti :3000'
case_ "pgrep 조회만"                 0 'pgrep -fl socat'
case_ "hc kill 주소"                 0 'hc kill mini-m4/w3'
case_ "pkill -P 부모 pid"            0 'pkill -P $$'
case_ "kill \$(pgrep -P 부모)"       0 'kill $(pgrep -P 4321)'
case_ "echo 설명"                    0 "echo '포트 정리: lsof -ti :3000 | xargs kill 은 쓰지 말 것'"
case_ "grep 패턴"                    0 'grep -rn "pkill -f" ~/.claude/hooks'
case_ "커밋 메시지"                  0 'git commit -m "fix: killall·fuser -k 막기" -- a.sh'
case_ "heredoc 본문"                 0 $'git commit -F - <<\'EOF\'\nkill-by-lookup-guard: pkill -f·killall·kill $(lsof -ti) 막기\nEOF'
case_ "주석"                         0 'kill $PID  # pkill -f 대신'
case_ "skill 낱말"                   0 'ls ~/.claude/skills/kill-switch'
case_ "명령 없음"                    0 ''
echo "pass=$pass fail=$fail"
[ "$fail" = 0 ]
