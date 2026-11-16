#!/bin/bash
# herdr-settings-merge 시험: 임시 HOME마다 settings.json·base를 두고 돌린다. 실제 ~/.claude·hc·네트워크는 쓰지 않는다.
# sh로 한 번, dash가 있으면 dash로 한 번 더 돈다. 실행: bash tests/herdr-settings-merge.test.sh
set -u
here=$(cd "$(dirname "$0")" && pwd)
MERGE=${HERDR_SETTINGS_MERGE:-$here/../bin/herdr-settings-merge}
KEYS=$here/../config.example/settings/keys.json  # 가짜 예시 목록(진짜는 개인 설정)
T=$(mktemp -d "${TMPDIR:-/tmp}/hsm-test.XXXXXX") && T=$(cd "$T" && pwd -P) || exit 1
trap 'rm -rf "$T"' EXIT

n=0 fails=0
t() { n=$((n + 1)); if eval "$2"; then echo "ok $n - $1"; else fails=$((fails + 1)); echo "FAIL $n - $1"; echo "  out: $out"; echo "  err: $(cat "$T/err")"; fi; }
ck() { printf '%s\n' "$(printf '%s' "$1" | jq -cS .)" | cksum | awk '{print $1, $2}'; }
# fresh <settings JSON> [base JSON]: 새 임시 HOME
fresh() {
  H=$T/home$n.$RANDOM; mkdir -p "$H/.claude"
  printf '%s\n' "$1" >"$H/.claude/settings.json"
  if [ -n "${2-}" ]; then mkdir -p "$H/.local/state/herdr-sync"; printf '%s\n' "$2" >"$H/.local/state/herdr-sync/settings-base.json"; fi
}
# run <입력 JSON> [인자…]: $out·$rc. 기계 이름은 $M(기본 w1)
run() { local in=$1; shift; out=$(printf '%s' "$in" | HOME=$H HERDR_MACHINE=${M-w1} HERDR_KEYS=$KEYS $SH "$MERGE" "$@" 2>"$T/err"); rc=$?; }
S() { jq -cS "$1" "$H/.claude/settings.json"; }
has() { printf '%s\n' "$out" | grep -Fxq -- "$1"; }
nobak() { [ ! -d "$H/.local/state/herdr-sync/bak" ]; }
mode() { ls -l "$1" | cut -c1-10; }
GUARD='/bin/bash -c '"'"'exec /bin/bash "$HOME/.claude/hooks/cotenant-guard.sh"'"'"
MOSHI='{"matcher":"Bash","hooks":[{"type":"command","command":"moshi-hook.sh"}]}'

suite() {
  echo "# SH=$SH"
  # 같음: base가 없어도 같으면 같음, base를 남긴다
  fresh '{"language":"한국어","theme":"dark"}'
  run '{"values":{".language":"한국어"}}'
  t "같음: 종료 0·같음 줄" '[ $rc = 0 ] && [ "$out" = "같음 .language" ]'
  t "같음: base에 cksum, settings·백업은 그대로" '[ "$(jq -r ".[\".language\"]" "$H/.local/state/herdr-sync/settings-base.json")" = "$(ck "\"한국어\"")" ] && nobak && [ "$(S .theme)" = "\"dark\"" ]'

  # 씀: 현재 = base → 맥 값으로, 백업·권한·base
  fresh '{"model":"old","theme":"dark"}' "{\".model\":\"$(ck '"old"')\"}"
  chmod 640 "$H/.claude/settings.json"; before=$(cat "$H/.claude/settings.json")
  run '{"values":{".model":"new"}}'
  t "씀: 종료 0·씀 줄" '[ $rc = 0 ] && [ "$out" = "씀 .model" ]'
  t "씀: 값이 바뀌고 다른 키는 그대로" '[ "$(S .model)" = "\"new\"" ] && [ "$(S .theme)" = "\"dark\"" ]'
  t "씀: 권한 그대로(640)" '[ "$(mode "$H/.claude/settings.json")" = "-rw-r-----" ]'
  t "씀: bak/<시각>/settings.json에 원본" '[ "$(cat "$H"/.local/state/herdr-sync/bak/*/settings.json)" = "$before" ]'
  t "씀: base가 새 값으로" '[ "$(jq -r ".[\".model\"]" "$H/.local/state/herdr-sync/settings-base.json")" = "$(ck "\"new\"")" ]'
  t "씀: 임시 파일을 남기지 않음" '[ -z "$(ls "$H/.claude" | grep -v "^settings.json$")" ]'
  run '{"values":{".model":"new"}}'
  t "씀 뒤 다시: 같음" '[ $rc = 0 ] && [ "$out" = "같음 .model" ]'

  # 충돌: 현재 ≠ base ≠ 맥
  fresh '{"model":"mine"}' "{\".model\":\"$(ck '"old"')\"}"
  run '{"values":{".model":"new"}}'
  t "충돌: 종료 1·충돌 줄·쓰지 않음" '[ $rc = 1 ] && [ "$out" = "충돌 .model" ] && [ "$(S .model)" = "\"mine\"" ] && nobak'
  # 처음(base 없음)이면서 다름 → 충돌. 대상에 키가 없어도(null) 같다
  fresh '{"model":"mine"}'
  run '{"values":{".model":"new",".language":"한국어"}}'
  t "처음·다름: 충돌(키가 없는 것도)" '[ $rc = 1 ] && has "충돌 .model" && has "충돌 .language" && [ "$(S .model)" = "\"mine\"" ]'
  t "처음·다름: base를 만들지 않음" '[ ! -e "$H/.local/state/herdr-sync/settings-base.json" ]'
  run '{"values":{".model":"new",".language":"한국어"}}' --take .model
  t "--take <키>: 그 키만 씀, 나머지는 충돌" '[ $rc = 1 ] && has "씀 .model" && has "충돌 .language" && [ "$(S .model)" = "\"new\"" ] && [ "$(S .language)" = null ]'
  run '{"values":{".model":"new",".language":"한국어"}}' --take '*'
  t "--take *: 전부 씀, 종료 0" '[ $rc = 0 ] && has "같음 .model" && has "씀 .language" && [ "$(S .language)" = "\"한국어\"" ]'

  # 예외: lx(예시 keys.json의 exceptions)의 .modelSettings·.effortLevel은 --take *로도 손대지 않는다
  fresh '{"effortLevel":"medium","modelSettings":{"a":1},"model":"x"}'
  M=lx run '{"values":{".effortLevel":"xhigh",".modelSettings":{"a":2},".model":"x"}}' --take '*'
  t "예외: 예외 줄, 값 그대로, 종료 0" '[ $rc = 0 ] && has "예외 .effortLevel" && has "예외 .modelSettings" && has "같음 .model" && [ "$(S .effortLevel)" = "\"medium\"" ] && [ "$(S .modelSettings.a)" = 1 ]'
  M=w2 run '{"values":{".effortLevel":"xhigh"}}' --take '*'
  t "예외: 다른 기계는 맞춘다" '[ $rc = 0 ] && [ "$out" = "씀 .effortLevel" ]'

  # 참고: 다르면 참고 줄만, 같으면 없음, refs가 없으면 없음. 이 기계 HOME은 맥 HOME으로 되돌려 비교
  fresh '{}'; echo "{\"statusLine\":{\"command\":\"$H/.claude/sl.sh\"},\"permissions\":{\"allow\":[\"x\"]}}" >"$H/.claude/settings.json"
  run "{\"values\":{},\"home\":\"/Users/mac\",\"refs\":{\".statusLine\":\"$(ck '{"command":"/Users/mac/.claude/sl.sh"}')\",\".permissions\":\"$(ck '{"allow":["y"]}')\",\".enabledPlugins\":\"$(ck null)\"}}"
  t "참고: 다른 것만 참고 줄, 종료 0, 쓰지 않음" '[ $rc = 0 ] && [ "$out" = "참고 .permissions" ] && nobak'
  run '{"values":{}}'
  t "참고: refs가 없으면 찍지 않음" '[ $rc = 0 ] && [ -z "$out" ]'

  # 훅: 없으면 더하고, 다른 훅 항목은 그대로, 다시 돌리면 같음
  fresh "{\"hooks\":{\"PreToolUse\":[$MOSHI],\"Stop\":[$MOSHI]}}"
  hin=$(jq -cn --arg c "$GUARD" '{values:{},hooks:[{event:"PreToolUse",matcher:"Edit|Bash",command:$c,timeout:5,onFailure:"block"}]}')
  run "$hin"
  t "훅 더하기: 씀 줄, 종료 0" '[ $rc = 0 ] && [ "$out" = "씀 hook:PreToolUse:cotenant-guard.sh" ]'
  exp=$(jq -cnS --arg c "$GUARD" '{matcher:"Edit|Bash",hooks:[{type:"command",command:$c,timeout:5,onFailure:"block"}]}')
  t "훅 더하기: 끝에 붙고 필드 그대로" '[ "$(S ".hooks.PreToolUse[1]")" = "$exp" ]'
  t "훅 더하기: 다른 훅 항목은 그대로" '[ "$(S ".hooks.PreToolUse[0]")" = "$(printf "%s" "$MOSHI" | jq -cS .)" ] && [ "$(S ".hooks.Stop")" = "$(printf "[%s]" "$MOSHI" | jq -cS .)" ]'
  run "$hin"
  t "훅: 다시 돌리면 같음, 하나만 있음" '[ $rc = 0 ] && [ "$out" = "같음 hook:PreToolUse:cotenant-guard.sh" ] && [ "$(S ".hooks.PreToolUse|length")" = 2 ]'
  # 훅 3-way: 옛 command가 base와 같으면 그 항목만 바꾸고, 같은 묶음의 다른 훅은 남긴다
  old='{"matcher":"Edit","hook":{"command":"old cotenant-guard.sh","type":"command"}}'
  fresh '{"hooks":{"PreToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"old cotenant-guard.sh"},{"type":"command","command":"moshi-hook.sh"}]}]}}' "{\"hook:PreToolUse:cotenant-guard.sh\":\"$(ck "$old")\"}"
  run "$hin"
  t "훅 바꾸기: 씀, 같은 묶음의 다른 훅은 남김, 옛 항목은 없음" '[ $rc = 0 ] && [ "$out" = "씀 hook:PreToolUse:cotenant-guard.sh" ] && [ "$(S "[.hooks.PreToolUse[].hooks[].command]")" = "$(jq -cn --arg c "$GUARD" "[\"moshi-hook.sh\",\$c]")" ]'
  fresh '{"hooks":{"PreToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"mine cotenant-guard.sh"}]}]}}'
  run "$hin"
  t "훅: 처음이면서 다르면 충돌, 쓰지 않음" '[ $rc = 1 ] && [ "$out" = "충돌 hook:PreToolUse:cotenant-guard.sh" ] && nobak'
  run '{"values":{},"hooks":[{"event":"PreToolUse","matcher":"Bash","command":"other.sh"}]}'
  t "훅: 맞출 목록에 없는 훅을 주면 종료 2" '[ $rc = 2 ] && [ -z "$out" ] && nobak'

  # 대상에서 지운 훅은 base가 있어도 다시 더한다. 훅 command 안의 맥 HOME도 바꾼다
  fresh '{"hooks":{"PreToolUse":[]}}' "{\"hook:PreToolUse:loop-no-question.sh\":\"$(ck '{"matcher":"x"}')\"}"
  run '{"home":"/Users/mac","values":{},"hooks":[{"event":"PreToolUse","matcher":"Ask","command":"/Users/mac/.claude/hooks/loop-no-question.sh"}]}'
  t "훅: 지운 훅을 다시 더하고 HOME을 바꿈" '[ $rc = 0 ] && [ "$out" = "씀 hook:PreToolUse:loop-no-question.sh" ] && [ "$(S ".hooks.PreToolUse[0].hooks[0].command")" = "\"$H/.claude/hooks/loop-no-question.sh\"" ]'

  # 목록 밖 키·참고·이벤트는 받지 않는다(종료 2, 쓰지 않음)
  fresh '{"hooks":{"Stop":[]},"permissions":{"allow":["x"]}}'
  run '{"values":{".hooks":{},".permissions":{}}}' --take '*'
  t "목록 밖 값 키: 종료 2, 그대로" '[ $rc = 2 ] && [ -z "$out" ] && [ "$(S .permissions.allow[0])" = "\"x\"" ] && nobak'
  run '{"values":{},"refs":{".hooks":"1 2"}}'
  t "목록 밖 참고 키: 종료 2" '[ $rc = 2 ] && [ -z "$out" ]'
  run '{"values":{},"hooks":[{"event":"Stop","matcher":"","command":"cotenant-guard.sh"}]}'
  t "keys.json에 없는 이벤트의 훅: 종료 2" '[ $rc = 2 ] && [ -z "$out" ] && nobak'

  # 중간 키가 객체가 아니면(env가 문자열) 「씀」이라 하지 않고 종료 1, 아무것도 쓰지 않음
  fresh '{"env":"oops"}'
  run '{"values":{".env.ENABLE_TOOL_SEARCH":"1"}}' --take '*'
  t "읽을 수 없는 경로: 종료 1, 씀 없음, 오류 한 줄, 그대로" '[ $rc = 1 ] && ! has "씀 .env.ENABLE_TOOL_SEARCH" && [ "$(wc -l <"$T/err")" -eq 1 ] && [ "$(S .env)" = "\"oops\"" ] && nobak && [ ! -e "$H/.local/state/herdr-sync/settings-base.json" ]'

  fresh '{"model":"a","env":"oops"}'
  run '{"values":{".model":"b",".env.ENABLE_TOOL_SEARCH":"1"}}' --take '*'
  t "읽기 실패: 앞에서 판정한 키의 씀 줄도 내지 않음, 그대로" '[ $rc = 1 ] && [ -z "$out" ] && [ "$(S .model)" = "\"a\"" ] && nobak'
  run '{"values":{},"hooks":[{"event":"PreToolUse","matcher":"A","command":"loop-no-question.sh"},{"event":"PreToolUse","matcher":"B","command":"x loop-no-question.sh"}]}'
  t "같은 판별 글자의 훅 둘: 종료 2" '[ $rc = 2 ] && [ -z "$out" ] && nobak'
  run '{"values":{},"hooks":[{"event":"PreToolUse","matcher":"A","command":"cotenant-guard.sh && loop-no-question.sh"}]}'
  t "판별 글자 둘이 든 훅: 종료 2" '[ $rc = 2 ] && [ -z "$out" ] && nobak'

  # 연달아 두 번 써도 백업이 둘 남는다(같은 초면 -<pid>)
  fresh '{"model":"orig"}'
  run '{"values":{".model":"v1"}}' --take '*'; run '{"values":{".model":"v2"}}' --take '*'
  t "백업: 두 번 쓰면 둘, 처음 값이 남음" '[ "$(ls "$H/.local/state/herdr-sync/bak" | wc -l)" -eq 2 ] && cat "$H"/.local/state/herdr-sync/bak/*/settings.json | grep -Fq orig'

  # HOME 바꾸기: 맥 HOME과 그 아래만, /Users/macx는 그대로. 중첩 env 키는 다른 env를 남긴다
  fresh '{"env":{"KEEP":"1"}}'
  run '{"home":"/Users/mac","values":{".env.ENABLE_TOOL_SEARCH":"/Users/mac/a:/Users/macx/b",".tui":"/Users/mac",".autoMode":{"x":["at /Users/mac, here /Users/mac.d"]}}}' --take '*'
  t "HOME: 맥 HOME을 이 기계 HOME으로" '[ $rc = 0 ] && [ "$(S .env.ENABLE_TOOL_SEARCH)" = "\"$H/a:/Users/macx/b\"" ] && [ "$(S .tui)" = "\"$H\"" ] && [ "$(S .env.KEEP)" = "\"1\"" ]'
  t "HOME: 글 속(뒤에 쉼표)도 바꾸고 /Users/mac.d는 그대로" '[ "$(S .autoMode.x[0])" = "\"at $H, here /Users/mac.d\"" ]'
  run '{"home":"/Users/mac","values":{".env.ENABLE_TOOL_SEARCH":"/Users/mac/a:/Users/macx/b"}}'
  t "HOME: 바꾼 값과 같으면 같음" '[ $rc = 0 ] && [ "$out" = "같음 .env.ENABLE_TOOL_SEARCH" ]'
  run '{"values":{".env.ENABLE_TOOL_SEARCH":null}}'
  t "null 값: base와 같으면 키를 지움" '[ $rc = 0 ] && [ "$out" = "씀 .env.ENABLE_TOOL_SEARCH" ] && [ "$(S ".env|has(\"ENABLE_TOOL_SEARCH\")")" = false ]'

  # 깨진 settings.json: 손대지 않고 종료 1, 표준 오류 한 줄
  fresh '{"model": "x",'
  run '{"values":{".model":"y"}}' --take '*'
  t "깨짐: 종료 1, 출력 없음, 오류 한 줄, 파일 그대로" '[ $rc = 1 ] && [ -z "$out" ] && [ "$(wc -l <"$T/err")" -eq 1 ] && [ "$(cat "$H/.claude/settings.json")" = "{\"model\": \"x\"," ] && nobak'

  # --check: 아무것도 쓰지 않는다
  fresh '{"model":"old"}' "{\".model\":\"$(ck '"old"')\"}"
  cp -p "$H/.local/state/herdr-sync/settings-base.json" "$T/base.before"
  run "{\"values\":{\".model\":\"new\",\".language\":\"x\"},\"hooks\":[{\"event\":\"PreToolUse\",\"matcher\":\"B\",\"command\":\"loop-no-question.sh\"}]}" --check --take '*'
  t "--check: 씀을 보이고 종료 1" '[ $rc = 1 ] && has "씀 .model" && has "씀 .language" && has "씀 hook:PreToolUse:loop-no-question.sh"'
  t "--check: settings·base·백업 모두 그대로" '[ "$(S .model)" = "\"old\"" ] && cmp -s "$T/base.before" "$H/.local/state/herdr-sync/settings-base.json" && nobak'
  fresh '{"model":"x"}'
  run '{"values":{".model":"x"}}' --check
  t "--check: 다 같으면 종료 0, base도 만들지 않음" '[ $rc = 0 ] && [ ! -e "$H/.local/state/herdr-sync" ]'

  # 심볼릭 링크 settings.json은 링크를 살리고 대상에 쓴다
  fresh '{}'; mv "$H/.claude/settings.json" "$H/real.json"; echo '{"model":"old"}' >"$H/real.json"; ln -s "$H/real.json" "$H/.claude/settings.json"
  run '{"values":{".model":"new"}}' --take .model
  t "링크: 링크는 그대로, 대상 파일이 바뀜" '[ $rc = 0 ] && [ -L "$H/.claude/settings.json" ] && [ "$(jq -r .model "$H/real.json")" = new ]'

  # 잘못 부름: 종료 2
  fresh '{}'
  run '{"values":{}}' --bogus;          t "잘못 부름: 모르는 인자" '[ $rc = 2 ]'
  run '{"values":{}}' --take;           t "잘못 부름: --take 뒤 키 없음" '[ $rc = 2 ]'
  run 'not json';                       t "잘못 부름: 입력이 JSON이 아님" '[ $rc = 2 ] && nobak'
  run '{"hooks":[]}';                   t "잘못 부름: values 없음" '[ $rc = 2 ]'
  M= run '{"values":{}}';               t "잘못 부름: HERDR_MACHINE 없음" '[ $rc = 2 ]'
}

SH=sh suite
if command -v dash >/dev/null 2>&1; then SH=dash suite; else echo "# dash 없음, 건너뜀"; fi
echo "${n}개 중 실패 ${fails}개"
[ $fails = 0 ]
