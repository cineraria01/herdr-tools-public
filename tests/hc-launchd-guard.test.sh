#!/bin/bash
# launchd 문맥의 Go 판 막기(2026-11-21 go-launchd-tcc — launchd가 띄운 새 Go 바이너리가 ~/Documents 첫 접근에서 macOS 개인 정보 보호 창에 37분 멈춤):
# (가) lib/go-run.sh: 터미널 밖(TERM·HC_TERMINAL_ID·SSH_CONNECTION 없음)이고 조상에 herdr-core·sshd가 없으면 auto는 셸 판, 안내는 도구마다 한 번
# (나) bin/hc-launchd-guard: 같은 이름 터미널이 이 기계에 살아 있으면 안 띄움, 없으면 hc run으로 띄움, hc가 안 되면 그 자리에서 셸 판
# 실행: bash tests/hc-launchd-guard.test.sh   (go·hc 없이 돈다 — 가짜 바이너리·가짜 hc·가짜 ps)
set -u
here=$(cd "$(dirname "$0")" && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/launchd-guard.XXXXXX") || exit 1
fail=0
t() { if [ "$2" = "$3" ]; then echo "ok - $1"; else echo "FAIL - $1 — 바람 「$3」, 얻음 「$2」"; fail=1; fi; }

# ---- (가) go_run ----
mkdir -p "$T/mod/lib" "$T/home" "$T/ps-core" "$T/ps-none"
gr=$here/../lib/go-run.sh G=$here/../bin/hc-launchd-guard
[ -f "$gr" ] || gr=$here/../../lib/go-run.sh G=$here/../hc-launchd-guard   # 깔린 곳(~/.local/bin/tests)
cp "$gr" "$T/mod/lib/go-run.sh" || exit 1
printf '#!/bin/sh\n[ "$1" = --version ] && exit 0\necho go "$@"\n' >"$T/fakebin"; chmod +x "$T/fakebin"
# 가짜 ps: 진짜 ps 출력에서 부모가 1인 줄을 herdr-core로(조상 끝이 코어), 또는 herdr-core·sshd 이름을 지움(launchd 흉내)
printf '#!/bin/sh\n/bin/ps "$@" | sed -E "s#^( *[0-9]+ +1 ).*#\\\\1/x/Herdr Core.app/Contents/MacOS/herdr-core#"\n' >"$T/ps-core/ps"
printf '#!/bin/sh\n/bin/ps "$@" | sed -E "s#herdr-core|sshd#masked#g"\n' >"$T/ps-none/ps"
chmod +x "$T/ps-core/ps" "$T/ps-none/ps"
# gd_ <ps 폴더> [k=v…]: 터미널 밖(TERM·HC_TERMINAL_ID·SSH_CONNECTION 없음)에서 go_run
gd_() { local p=$1; shift
  (unset HC_FAKE_IMPL HERDR_IMPL TERM HC_TERMINAL_ID SSH_CONNECTION XDG_STATE_HOME; export HOME=$T/home PATH="$T/$p:$PATH" HC_FAKE_BIN=$T/fakebin
   for kv in "$@"; do export "$kv"; done
   . "$T/mod/lib/go-run.sh"; go_run hc-fake a b; echo "돌아옴 $?") 2>"$T/err"; }
t "launchd 흉내: auto는 셸 판(0으로 돌아옴)" "$(gd_ ps-none)" "돌아옴 0"
t "그 안내 한 줄" "$(grep -c 'launchd' "$T/err")" 1
t "안내 표식" "$([ -e "$T/home/.local/state/herdr-go/launchd-noted-hc-fake" ] && echo 있음)" 있음
t "두 번째는 안내 없이 셸 판(TERM=dumb — launchd의 bash가 스스로 두는 값 — 도 터미널 없음)" "$(gd_ ps-none TERM=dumb)" "돌아옴 0"
t "두 번째 안내 없음" "$(grep -c 'launchd' "$T/err")" 0
t "launchd 흉내라도 IMPL=go면 Go 판" "$(gd_ ps-none HC_FAKE_IMPL=go)" "go a b"
t "IMPL=bash는 그대로 0" "$(gd_ ps-none HC_FAKE_IMPL=bash)" "돌아옴 0"
t "Herdr 터미널(HC_TERMINAL_ID)이면 Go 판" "$(gd_ ps-none HC_TERMINAL_ID=t1)" "go a b"
t "터미널(TERM)이면 Go 판" "$(gd_ ps-none TERM=xterm)" "go a b"
t "SSH면 Go 판" "$(gd_ ps-none 'SSH_CONNECTION=1 2 3 4')" "go a b"
t "터미널 밖이라도 조상에 herdr-core(hc exec·코어 문맥)면 Go 판" "$(gd_ ps-core)" "go a b"
printf '#!/bin/sh\nexit 1\n' >"$T/ps-none/ps"
t "ps를 못 읽으면 셸 판(창에 멈추는 쪽을 고르지 않음)" "$(gd_ ps-none)" "돌아옴 0"

# ---- (나) hc-launchd-guard ----
mkdir -p "$T/stub"
cat >"$T/stub/hc" <<'EOF'
#!/bin/bash
echo "$*" >>"$T_LOG"
case $1 in
  ls) [ "${HC_FAKE_LS_RC:-0}" = 0 ] || exit "$HC_FAKE_LS_RC"
      n=$(cat "$T_STATE" 2>/dev/null); [ -n "$n" ] && HC_FAKE_LS=$n
      [ -n "${HC_FAKE_LS-}" ] || HC_FAKE_LS='{"terminals":[]}'
      printf '%s\n' "$HC_FAKE_LS" ;;
  run) [ -z "${HC_FAKE_RUN_TAKES-}" ] || echo "$HC_FAKE_RUN_TAKES" >"$T_STATE"; exit "${HC_FAKE_RUN_RC:-0}" ;;
esac
EOF
printf '#!/bin/sh\necho "tool HERDR_IMPL=${HERDR_IMPL-} $*"\nexit 7\n' >"$T/stub/faketool"
chmod +x "$T/stub/hc" "$T/stub/faketool"
live() { printf '{"terminals":[{"address":"%s","name":"%s","state":"live"}]}' "$1" "$2"; }
g_() { : >"$T/log"; rm -f "$T/state"; (export T_LOG=$T/log T_STATE=$T/state HOME=$T/home PATH="$T/stub:$PATH"; for kv in "$@"; do export "$kv"; done
  "$G" job1 -- faketool a 'b c'; echo "rc $?") 2>"$T/gerr"; }
t "같은 이름 터미널이 살아 있으면 아무것도 안 함" "$(g_ "HC_FAKE_LS=$(live t5 job1)")" "rc 0"
t "그때 hc run 없음" "$(grep -c '^run' "$T/log")" 0
t "다른 기계(주소에 /)의 같은 이름은 안 침 — 띄움" "$(g_ "HC_FAKE_LS=$(live box/t5 job1)")" "rc 0"
t "다른 이름만 있으면 띄움" "$(g_ "HC_FAKE_LS=$(live t5 job2)"; grep -c '^run' "$T/log")" "rc 0
1"
g_ >/dev/null
t "띄우는 명령(코어 문맥, 터미널 표식을 지우고 도구 절대 경로)" "$(grep '^run' "$T/log")" \
  "run --bg --no-parent --outlive-parent --rm --name job1 --cwd $T/home -- /usr/bin/env -u HC_TERMINAL_ID -u HC_SCOPE -u HC_TERMINAL_SOCKET $T/stub/faketool a b c"
t "hc run이 실패하면 그 자리에서 셸 판(도구 종료 코드 그대로)" "$(g_ HC_FAKE_RUN_RC=1)" "tool HERDR_IMPL=bash a b c
rc 7"
t "hc ls가 실패하면 셸 판" "$(g_ HC_FAKE_LS_RC=3)" "tool HERDR_IMPL=bash a b c
rc 7"
t "hc ls가 목록이 아니면 셸 판" "$(g_ 'HC_FAKE_LS=[]')" "tool HERDR_IMPL=bash a b c
rc 7"
t "hc run 실패가 이름 겹침(그사이 다른 지킴이가 띄움)이면 셸 판 없이 끝" \
  "$(g_ HC_FAKE_RUN_RC=1 "HC_FAKE_RUN_TAKES=$(live t9 job1)")" "rc 0"
t "--log: 셸 판 출력이 그 파일로" "$(g_ HC_FAKE_RUN_RC=1 >/dev/null; : >"$T/log"; (export T_LOG=$T/log HOME=$T/home PATH="$T/stub:$PATH" HC_FAKE_RUN_RC=1
  "$G" --log "$T/job.log" job1 -- faketool z; echo "rc $?"); tail -1 "$T/job.log")" "rc 7
tool HERDR_IMPL=bash z"
: >"$T/log"; (export T_LOG=$T/log HOME=$T/home PATH="$T/stub:$PATH"; "$G" --log "$T/job.log" job1 -- faketool z) >/dev/null 2>&1
t "--log: 띄울 때 출력을 그 파일로 잇는 sh 포장" "$(grep '^run' "$T/log")" \
  "run --bg --no-parent --outlive-parent --rm --name job1 --cwd $T/home -- /bin/sh -c exec \"\$@\" >>\"\$0\" 2>&1 $T/job.log /usr/bin/env -u HC_TERMINAL_ID -u HC_SCOPE -u HC_TERMINAL_SOCKET $T/stub/faketool z"
t "이름이 hc 꼴(a-z 0-9 - _, 32자)이 아니면 2" "$( (PATH="$T/stub:$PATH" "$G" Job.1 -- faketool) 2>/dev/null; echo $?)" 2
t "-- 뒤 도구가 없으면 2" "$( (PATH="$T/stub:$PATH" "$G" job1 --) 2>/dev/null; echo $?)" 2
t "도구를 못 찾으면 127" "$( (PATH="$T/stub:$PATH" "$G" job1 -- 없는도구) 2>/dev/null; echo $?)" 127

rm -rf "$T"
exit $fail
