#!/bin/bash
# Tests for ~/.claude/hooks/loop-no-question.sh. Feeds PreToolUse JSON through the same wrapper settings.json uses
# (a missing hook file = exit 0, so HOOK=/nonexistent shows the state before the hook). Run: bash <this file>  (exit 0 = pass)
# hc는 PATH 앞의 가짜 hc로 바꾼다: FAKE_JSON을 그대로 찍고 FAKE_RC로 끝난다.
HOOK=${HOOK:-$(cd "$(dirname "$0")/.." && pwd)/loop-no-question.sh}   # the hook beside this tests folder (repo or ~/.claude/hooks)
. "$(dirname "$0")/hooks-impl.sh"   # 판: HERDR_HOOKS_IMPL=go(기본)·bash
T=$(mktemp -d)
mkdir -p "$T/bin" "$T/home"
cat > "$T/bin/hc" <<'EOF'
#!/bin/bash
[ "$1 $3" = "get --json" ] || exit 9
printf '%s' "$FAKE_JSON"
exit "${FAKE_RC:-0}"
EOF
chmod +x "$T/bin/hc"
pass=0 fail=0
# case NAME WANT_RC TERMINAL_ID FAKE_JSON [FAKE_RC [PATH]]
case_() {
  local err rc
  err=$(printf '%s' '{"tool_name":"AskUserQuestion","tool_input":{}}' \
    | env HOME="$T/home" PATH="${6:-$T/bin:/usr/bin:/bin}" HC_TERMINAL_ID="$3" FAKE_JSON="$4" FAKE_RC="${5:-0}" HOOK="$HOOK" \
      /bin/bash -c '[ -r "$HOOK" ] || exit 0; exec /bin/bash "$HOOK"' 2>&1 >/dev/null)
  rc=$?
  if [ "$rc" = "$2" ] && { [ "$2" = 0 ] || printf '%s' "$err" | grep -q '질문 창을 띄우지 않는다'; }; then
    pass=$((pass + 1)); echo "ok   $1"
  else
    fail=$((fail + 1)); echo "FAIL $1 (rc=$rc, want $2) err=[$err]"
  fi
}
# 막을 것: 이름이나 job 라벨에 loop(대소문자 무관)
case_ "이름 loop"        2 t1 '{"name":"d1-loop-150","labels":{}}'
case_ "이름 대문자 LOOP" 2 t1 '{"name":"Inquiry-LOOP"}'
case_ "job 라벨 loop"    2 t1 '{"name":"worker","labels":{"job":"inquiry-loop"}}'
case_ "이름 없음·job loop" 2 t1 '{"labels":{"job":"loop"}}'
# 통과할 것
case_ "loop 아닌 이름"   0 t1 '{"name":"hooks-go","labels":{"job":"lane"}}'
case_ "hc 터미널 아님"   0 ''  '{"name":"d1-loop"}'
case_ "hc get 실패"      0 t1 '{"name":"d1-loop"}' 1
case_ "hc 없음"          0 t1 '{"name":"d1-loop"}' 0 /usr/bin:/bin
case_ "JSON 아님"        0 t1 'not json loop'
case_ "다른 키의 loop"   0 t1 '{"name":"w","labels":{"role":"loop"},"cwd":"/loop"}'
echo "pass=$pass fail=$fail"
[ "$fail" = 0 ]
