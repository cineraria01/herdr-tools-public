#!/bin/bash
# launchd 문맥의 Go 판 막기(2026-11-21 go-launchd-tcc — launchd가 띄운 새 Go 바이너리가 ~/Documents 첫 접근에서 macOS 개인 정보 보호 창에 37분 멈춤):
# (가) lib/go-run.sh: 터미널 밖(TERM·HC_TERMINAL_ID·SSH_CONNECTION 없음)이고 조상에 herdr-core·sshd가 없으면 auto는 셸 판, 안내는 도구마다 한 번
# (나) bin/hc-launchd-guard: 같은 이름 터미널이 이 기계에 살아 있으면 안 띄움, 없으면 hc run으로 띄움, hc run이 안 되면 그 자리에서 셸 판,
#      살아 있는지 모르면(hc ls 실패·멈춤·목록 아님·state 모름) 75로 끝내고 세 번 이어질 때만 셸 판(2026-11-21 gofix-core B06·B07·M2·M3)
# (다) launchd/*.plist: 지킴이에 넘기는 -- 뒤 명령을 가짜 hc 아래서 실제로 돌려 0인지(gofix-core B01·M1·T1 — 저장소에서만)
# 실행: bash tests/hc-launchd-guard.test.sh   (go·hc 없이 돈다 — 가짜 바이너리·가짜 hc·가짜 ps·가짜 python3)
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
hang() { sleep "$1" & echo $! >"$T_LOG.pid"; wait; }   # 멈춘 hc 흉내 — 지킴이가 무리째 거두는지 pid로 본다
case $1 in
  ls) [ -z "${HC_FAKE_LS_SLEEP-}" ] || hang "$HC_FAKE_LS_SLEEP"
      [ "${HC_FAKE_LS_RC:-0}" = 0 ] || exit "$HC_FAKE_LS_RC"
      n=$(cat "$T_STATE" 2>/dev/null); [ "$n" = FAIL ] && exit 1; [ -n "$n" ] && HC_FAKE_LS=$n
      [ -n "${HC_FAKE_LS-}" ] || HC_FAKE_LS='{"terminals":[]}'
      printf '%s\n' "$HC_FAKE_LS" ;;
  run) [ -z "${HC_FAKE_RUN_SLEEP-}" ] || hang "$HC_FAKE_RUN_SLEEP"
       [ -z "${HC_FAKE_RUN_TAKES-}" ] || echo "$HC_FAKE_RUN_TAKES" >"$T_STATE"
       if [ -n "${HC_FAKE_RUN_EXEC-}" ]; then while [ "$1" != -- ]; do shift; done; shift; "$@"; exit; fi   # -- 뒤 명령을 그대로 돌림
       exit "${HC_FAKE_RUN_RC:-0}" ;;
esac
EOF
printf '#!/bin/sh\necho "tool HERDR_IMPL=${HERDR_IMPL-} FAKETOOL_IMPL=${FAKETOOL_IMPL-} $*"\nexit 7\n' >"$T/stub/faketool"
# 가짜 python3: 파이썬 파일(첫 줄 #!…python)이 아니면 진짜처럼 SyntaxError로 1, 맞으면 sh로 돌린다(시험 파일 몸통은 sh)
printf '#!/bin/sh\nIFS= read -r l <"$1"\ncase $l in "#!"*python*) ;; *) echo "SyntaxError: invalid syntax ($1)" >&2; exit 1 ;; esac\nexec /bin/sh "$@"\n' >"$T/stub/python3"
printf '#!/bin/bash\necho "wrap HERDR_IMPL=${HERDR_IMPL-} WRAP_IMPL=${WRAP_IMPL-} $*"\n' >"$T/stub/wrap"
printf '#!/usr/bin/env python3\necho "pytool HERDR_IMPL=${HERDR_IMPL-} PYTOOL_IMPL=${PYTOOL_IMPL-} $*"\n' >"$T/stub/pytool"
chmod +x "$T/stub/hc" "$T/stub/faketool" "$T/stub/python3" "$T/stub/wrap" "$T/stub/pytool"
live() { printf '{"terminals":[{"address":"%s","name":"%s","state":"live"}]}' "$1" "$2"; }
ga=(faketool a 'b c')   # 지킴이에 넘기는 -- 뒤 명령(사례가 바꿨다 되돌린다)
# 바깥 상한 15초: 지킴이가 스스로 끝나지 않으면(옛 판 — 상한 없음) 142로 드러난다
g_() { : >"$T/log"; rm -f "$T/state" "$T/log.pid"; (unset XDG_STATE_HOME; export T_LOG=$T/log T_STATE=$T/state HOME=$T/home PATH="$T/stub:$PATH"; for kv in "$@"; do export "$kv"; done
  perl -e 'alarm shift; exec @ARGV' 15 "$G" job1 -- "${ga[@]}"; echo "rc $?") 2>"$T/gerr"; }
U=$T/home/.local/state/herdr-go/guard-unknown-job1   # 「모름」이 이어진 횟수
gone() { p=$(cat "$T/log.pid" 2>/dev/null); [ -n "$p" ] && ! kill -0 "$p" 2>/dev/null && echo 거둠 || echo "남음($p)"; }
t "같은 이름 터미널이 살아 있으면 아무것도 안 함" "$(g_ "HC_FAKE_LS=$(live t5 job1)")" "rc 0"
t "그때 hc run 없음" "$(grep -c '^run' "$T/log")" 0
t "다른 기계(주소에 /)의 같은 이름은 안 침 — 띄움" "$(g_ "HC_FAKE_LS=$(live box/t5 job1)")" "rc 0"
t "다른 이름만 있으면 띄움" "$(g_ "HC_FAKE_LS=$(live t5 job2)"; grep -c '^run' "$T/log")" "rc 0
1"
g_ >/dev/null
t "띄우는 명령(코어 문맥, 터미널 표식을 지우고 도구 절대 경로)" "$(grep '^run' "$T/log")" \
  "run --bg --no-parent --outlive-parent --rm --name job1 --cwd $T/home -- /usr/bin/env -u HC_TERMINAL_ID -u HC_SCOPE -u HC_TERMINAL_SOCKET $T/stub/faketool a b c"
t "hc run이 실패하면 그 자리에서 셸 판(도구 종료 코드 그대로)" "$(g_ HC_FAKE_RUN_RC=1)" "tool HERDR_IMPL=bash FAKETOOL_IMPL=bash a b c
rc 7"
# 셸 판으로 넘길 때 도구별 스위치(<도구>_IMPL)도 bash로 — 그것이 HERDR_IMPL보다 앞서 Go 판이 도는 것을 막는다(B08)
t "셸 판: 도구별 스위치가 go여도 bash로 덮음" "$(g_ HC_FAKE_RUN_RC=1 FAKETOOL_IMPL=go)" "tool HERDR_IMPL=bash FAKETOOL_IMPL=bash a b c
rc 7"
# 「모름」(B07·M2): 살아 있는지 모르면 띄우지 않고 75 — launchd가 다시 부른다. 세 번 이어질 때만 셸 판, 살아 있음을 보면 셈을 지움
rm -f "$U"
t "모름: 터미널이 살아 있는데 hc ls만 실패 — 띄우지 않고 75" "$(g_ HC_FAKE_LS_RC=3 "HC_FAKE_LS=$(live t5 job1)")" "rc 75"
t "그때 hc run 없음" "$(grep -c '^run' "$T/log")" 0
t "모름 두 번째도 75" "$(g_ 'HC_FAKE_LS=[]')" "rc 75"
t "모름이 세 번 이어지면 셸 판" "$(g_ HC_FAKE_LS_RC=3)" "tool HERDR_IMPL=bash FAKETOOL_IMPL=bash a b c
rc 7"
g_ "HC_FAKE_LS=$(live t5 job1)" >/dev/null
t "살아 있음을 보면 셈을 지움" "$([ -e "$U" ] && echo 있음 || echo 없음)" 없음
t "모름: 같은 이름 항목에 state가 없으면 띄우지 않고 75" "$(g_ 'HC_FAKE_LS={"terminals":[{"address":"t5","name":"job1"}]}')" "rc 75"
t "그때 hc run 없음" "$(grep -c '^run' "$T/log")" 0
t "모름: hc run 실패 뒤 hc ls가 실패해도 75" "$(g_ HC_FAKE_RUN_RC=1 HC_FAKE_RUN_TAKES=FAIL)" "rc 75"
g_ >/dev/null
t "띄움에 성공하면 셈을 지움" "$([ -e "$U" ] && echo 있음 || echo 없음)" 없음
# 상한(B06·M3): hc ls·hc run이 멈춰도 지킴이는 끝나고, 멈춘 무리를 거둔다
s=$SECONDS
t "hc ls가 멈추면 상한에서 끊고 모름(75)" "$(g_ HC_FAKE_LS_SLEEP=12 HC_LAUNCHD_GUARD_LS_SECS=1)" "rc 75"
t "그 hc 무리를 거둠" "$(gone)" 거둠
t "hc run이 멈추면 상한에서 끊고(그사이 안 떴으면) 셸 판" "$(g_ HC_FAKE_RUN_SLEEP=12 HC_LAUNCHD_GUARD_RUN_SECS=1)" "tool HERDR_IMPL=bash FAKETOOL_IMPL=bash a b c
rc 7"
t "그 hc 무리를 거둠" "$(gone)" 거둠
t "두 멈춤 모두 바깥 상한 전에 끝남" "$([ $((SECONDS - s)) -lt 10 ] && echo 예 || echo "아니오 $((SECONDS - s))초")" 예
# python3로 감싼 진입점(B01·B08): 이미 깔린 plist가 bash 포장을 python3로 넘겨도 산다. 파이썬 파일이면 해석기를 그대로
ga=(python3 "$T/stub/wrap" --watch)
g_ >/dev/null
t "python3 + bash 포장: 해석기를 빼고 포장을 직접 띄움" "$(grep '^run' "$T/log")" \
  "run --bg --no-parent --outlive-parent --rm --name job1 --cwd $T/home -- /usr/bin/env -u HC_TERMINAL_ID -u HC_SCOPE -u HC_TERMINAL_SOCKET $T/stub/wrap --watch"
t "python3 + bash 포장: 셸 판도 포장을 직접(스위치는 포장 이름)" "$(g_ HC_FAKE_RUN_RC=1 WRAP_IMPL=go)" "wrap HERDR_IMPL=bash WRAP_IMPL=bash --watch
rc 0"
ga=(python3 "$T/stub/pytool" z)
g_ >/dev/null
t "python3 + 파이썬 파일: 해석기 그대로" "$(grep '^run' "$T/log")" \
  "run --bg --no-parent --outlive-parent --rm --name job1 --cwd $T/home -- /usr/bin/env -u HC_TERMINAL_ID -u HC_SCOPE -u HC_TERMINAL_SOCKET $T/stub/python3 $T/stub/pytool z"
t "python3 + 파이썬 파일: 셸 판 스위치는 그 파일 이름" "$(g_ HC_FAKE_RUN_RC=1 PYTOOL_IMPL=go)" "pytool HERDR_IMPL=bash PYTOOL_IMPL=bash z
rc 0"
ga=(faketool a 'b c')
t "hc run 실패가 이름 겹침(그사이 다른 지킴이가 띄움)이면 셸 판 없이 끝" \
  "$(g_ HC_FAKE_RUN_RC=1 "HC_FAKE_RUN_TAKES=$(live t9 job1)")" "rc 0"
t "--log: 셸 판 출력이 그 파일로" "$(g_ HC_FAKE_RUN_RC=1 >/dev/null; : >"$T/log"; (export T_LOG=$T/log HOME=$T/home PATH="$T/stub:$PATH" HC_FAKE_RUN_RC=1
  "$G" --log "$T/job.log" job1 -- faketool z; echo "rc $?"); tail -1 "$T/job.log")" "rc 7
tool HERDR_IMPL=bash FAKETOOL_IMPL=bash z"
: >"$T/log"; (export T_LOG=$T/log HOME=$T/home PATH="$T/stub:$PATH"; "$G" --log "$T/job.log" job1 -- faketool z) >/dev/null 2>&1
t "--log: 띄울 때 출력을 그 파일로 잇는 sh 포장" "$(grep '^run' "$T/log")" \
  "run --bg --no-parent --outlive-parent --rm --name job1 --cwd $T/home -- /bin/sh -c exec \"\$@\" >>\"\$0\" 2>&1 $T/job.log /usr/bin/env -u HC_TERMINAL_ID -u HC_SCOPE -u HC_TERMINAL_SOCKET $T/stub/faketool z"
t "이름이 hc 꼴(a-z 0-9 - _, 32자)이 아니면 2" "$( (PATH="$T/stub:$PATH" "$G" Job.1 -- faketool) 2>/dev/null; echo $?)" 2
t "-- 뒤 도구가 없으면 2" "$( (PATH="$T/stub:$PATH" "$G" job1 --) 2>/dev/null; echo $?)" 2
t "도구를 못 찾으면 127" "$( (PATH="$T/stub:$PATH" "$G" job1 -- 없는도구) 2>/dev/null; echo $?)" 127

# ---- (다) launchd/*.plist (저장소에서만 — 깔린 곳에는 plist가 없다) ----
# plist의 sh -c 줄을 가짜 HOME에서 그대로 돌린다: ~/.local/bin에 진짜 지킴이, 가짜 hc(run은 -- 뒤 명령을 실제로 돌림)·python3,
# 도구 자리에는 저장소 bin/<도구>와 첫 줄(해석기)이 같은 대역. 도구가 해석기와 어긋나면 hc run·셸 판 모두 1이라 0이 아니다
L=$here/../launchd
if [ -d "$L" ]; then
  PH=$T/plhome; mkdir -p "$PH/.local/bin" "$PH/Library/Logs"
  ln -s "$G" "$PH/.local/bin/hc-launchd-guard"; cp "$T/stub/hc" "$T/stub/python3" "$PH/.local/bin/"
  for p in "$L"/*.plist; do
    if command -v plutil >/dev/null 2>&1; then c=$(plutil -extract ProgramArguments.2 raw -o - "$p")
    else c=$(grep -o '<string>[^<]*hc-launchd-guard[^<]*</string>' "$p" | sed 's/<[^>]*>//g; s/&amp;/\&/g'); fi
    w=${c#* -- } n=${w%% *} py=
    case $n in python*) py=1; w=${w#* }; n=${w%% *}; n=${n%\"}; n=${n##*/} ;; esac
    { IFS= read -r l <"$here/../bin/$n" && printf '%s\necho ran %s\n' "$l" "$n"; } >"$PH/.local/bin/$n"; chmod +x "$PH/.local/bin/$n"
    pyf="아니오($l)"; case $l in '#!'*python*) pyf=예 ;; esac
    [ -z "$py" ] || t "${p##*/}: python3로 넘기는 ${n}은 파이썬 파일" "$pyf" 예
    t "${p##*/}: -- 뒤 명령이 가짜 hc 아래서 실제로 돌아 0" "$( (unset XDG_STATE_HOME; export HOME=$PH T_LOG=$T/pl.log T_STATE=$T/pl.state HC_FAKE_RUN_EXEC=1
      perl -e 'alarm shift; exec @ARGV' 15 /bin/sh -c "$c"; echo "rc $?") 2>/dev/null) $(cat "$PH/Library/Logs/"*.log "$PH/.local/state/"*/*.log 2>/dev/null | grep -c "^ran $n\$")" "rc 0 1"
    find "$PH" -name '*.log' -type f -delete
  done
fi

rm -rf "$T"
exit $fail
