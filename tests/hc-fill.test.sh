#!/bin/bash
# hc-fill 시험: 가짜 hc·hc-lane·git·herdr-balance(PATH 앞)와 저장소의 예시 설정(config.example/hc-fill.tsv)으로.
# 실제 레인·기계·네트워크는 쓰지 않는다. 실행: bash tests/hc-fill.test.sh  (다른 사본은 HC_FILL=<경로>, 설정은 HC_FILL_EXAMPLE=<경로>)
set -u
here=$(cd "$(dirname "$0")" && pwd)
export HC_REAL_GIT="$(command -v git)"
FILL=${HC_FILL:-$here/../bin/hc-fill}
[ -f "$FILL" ] || FILL=$here/../hc-fill
EX=${HC_FILL_EXAMPLE:-$here/../config.example/hc-fill.tsv}
[ -f "$EX" ] || EX=$here/../../share/hc-fill/hc-fill.example.tsv   # 설치된 사본(~/.local/bin/tests → ~/.local/share/hc-fill)
EXA=${HC_FILL_AUDIT_EXAMPLE:-$here/../config.example/hc-fill-audit.md}   # 자동 감사 예시 틀
[ -f "$EXA" ] || EXA=$here/../../share/hc-fill/hc-fill-audit.example.md
T=$(mktemp -d "${TMPDIR:-/tmp}/hc-fill-test.XXXXXX") && T=$(cd "$T" && pwd -P) || exit 1
. "$here/isolate.sh" || exit 1
bg=
tree() { for c in $(pgrep -P "$1"); do tree "$c"; done; echo "$1"; }   # 손자까지(lockf가 띄운 안쪽 watch가 고아로 남지 않게)
# killtree: 먼저 STOP으로 멈춰 새 자식(watch 루프의 sleep)을 못 띄우게 한 뒤 다시 세어 끈다 — 세고 끄는 사이에 뜬 sleep이
# 고아로 남아 herdr-verify 고아 검사에 걸렸다(2026-11-19 fill-auto-audit 검증, 작업 중 발견해 함께 고침)
killtree() { kill -STOP $(tree "$1") 2>/dev/null; ktp=$(tree "$1"); kill $ktp 2>/dev/null; kill -CONT $ktp 2>/dev/null; }
# watch는 $T 안 링크로 띄워 명령 줄에 $T가 들어간다 → 이 시험의 watch만 골라 세고, 끝에 남은 것이 있으면 마저 끈다
leftw() { pgrep -f "$T/hc-fill watch"; }
trap '[ -z "$bg" ] || killtree "$bg"; for w in $(leftw); do killtree "$w"; done; rm -rf "$T"' EXIT   # 남은 watch도 같은 방식(목록 잡아 신호 경합 없음)
mkdir -p "$T/bin" "$T/conf"
cp "$EX" "$T/conf/hc-fill.tsv" || { echo "FAIL - 예시 설정이 없다: $EX"; exit 1; }

# 가짜 hc: ls·machine ls는 $HC_FAKE_LS·$HC_FAKE_MACHINES(JSON). get은 주소가 $HC_FAKE_NOSTATUS(공백 목록)에 있으면 status null.
# exec는 -- 뒤 명령을 그 자리에서 돌린다(받은 표준 입력 그대로). 그 밖의 호출은 $T/hc.log에 남긴다(kill 등이 없어야 한다).
cat >"$T/bin/hc" <<'EOF'
#!/bin/bash
case $1 in
  ls) [ -z "${HC_FAKE_LS_FAIL-}" ] || exit 3
      case " $* " in *" --label hc-lane=1 "*) e='{"terminals":[]}'; printf '%s\n' "${HC_FAKE_LS_LANE:-$e}"; exit "${HC_FAKE_LABEL_RC:-0}" ;; esac
      printf '%s\n' "${HC_FAKE_LS:-{\"terminals\":[]\}}" ;;
  machine) printf '%s\n' "$HC_FAKE_MACHINES" ;;
  get) case " ${HC_FAKE_GONE-} " in *" $2 "*) echo '{"error":{"kind":"not_found"}}'; exit 3 ;; esac
       case " ${HC_FAKE_FINISHED-} " in *" $2 "*) echo "{\"address\":\"$2\",\"state\":\"finished\",\"status\":null}"; exit 0 ;; esac
       # 실행 키: created_ms는 $HC_FAKE_CREATED(「<주소>=<ms>」 공백 목록)에 있으면 그 값, 없으면 1700000000000. name은 주소 끝 낱말
       c=1700000000000; for x in ${HC_FAKE_CREATED-}; do case $x in "$2="*) c=${x#*=} ;; esac; done
       case " ${HC_FAKE_NOSTATUS-} " in
         *" $2 "*) echo "{\"address\":\"$2\",\"created_ms\":$c,\"name\":\"${2##*/}\",\"state\":\"live\",\"status\":null}" ;;
         *) echo "{\"address\":\"$2\",\"created_ms\":$c,\"name\":\"${HC_FAKE_GET_NAME:-${2##*/}}\",\"state\":\"live\",\"status\":{\"app\":\"claude-code\",\"state\":\"working\"}}" ;;
       esac ;;
  exec) echo "$*" >>"$T_LOG/hc.log"; case "$*" in *"${HC_FAKE_EXEC_FAIL:-@@없음@@}"*) exit 255 ;; esac; while [ "$1" != -- ]; do shift; done; shift; exec "$@" ;;
  send) echo "$*" >>"$T_LOG/send.log"; case " ${HC_FAKE_SEND_FAIL-} " in *" $2 "*) exit 1 ;; esac ;;   # 실패시킬 주소 목록
  *) echo "$*" >>"$T_LOG/hc.log" ;;
esac
EOF
# 가짜 hc-lane: 받은 인자를 한 줄에 하나씩 $T/lane.log에. --print-steps는 착지 줄 대신 표식, 띄우기는 JSON(주소 = <기계>/<이름>).
cat >"$T/bin/hc-lane" <<'EOF'
#!/bin/bash
if [ "${1-}" = coord ] && [ "${2-}" = --check ]; then   # 지휘가 지금 살아 있나: $HC_FAKE_COORD_GONE(공백 목록)에 든 주소는 사라짐
  echo "${3-}" >>"$T_LOG/coordcheck.log"; case " ${HC_FAKE_COORD_GONE-} " in *" ${3-} "*) echo "hc-lane coord --check: ${3-} 터미널이 없거나 끝남(가짜)" >&2; exit 1 ;; esac; exit 0; fi
if [ "${1-}" = coord ] && [ "${2-}" = --set ]; then   # 지휘 정보 남기기: $HC_FAKE_COORD_SET_FAIL(공백 목록)에 든 주소는 지휘가 아님
  echo "${3-}" >>"$T_LOG/coordset.log"; case " ${HC_FAKE_COORD_SET_FAIL-} " in *" ${3-} "*) echo "hc-lane coord --set: ${3-} 는 지휘가 아니다(가짜)" >&2; exit 1 ;; esac; echo "${3-}"; exit 0; fi
if [ "${1-}" = coord ]; then [ -n "${HC_FAKE_COORD-}" ] || { echo "hc-lane coord: 지휘 없음(가짜)" >&2; exit 1; }; echo "$HC_FAKE_COORD"; exit 0; fi   # 지휘 정보 확인
if [ "${1-}" = report ]; then echo "$* (터미널 ${HC_TERMINAL_ID:-없음})" >>"$T_LOG/report.log"; exit "${HC_FAKE_REPORT_RC:-0}"; fi   # 놀고 있음 알림 사건
if [ "${1-}" = events ]; then echo "$*" >>"$T_LOG/events.log"; [ -z "${HC_FAKE_EVENTS_FAIL-}" ] || exit 1; cat "$T_LOG/events" 2>/dev/null; exit 0; fi
if [ "${1-}" = gc ]; then echo "$*" >>"$T_LOG/gc.log"; echo "지움 0 · 남김 0"; exit "${HC_FAKE_GC_RC:-0}"; fi   # gc는 따로 기록
{ echo "---"; for a in "$@"; do echo "$a"; done; } >>"$T_LOG/lane.log"
[ "${1-}" = --print-steps ] || [ "${2-}" = --print-steps ] || [ "${3-}" = --print-steps ] || echo "${HC_LANE_COORD:-없음}" >>"$T_LOG/coord.log"   # 띄울 때 넘긴 지휘(watch 대상)
[ "${1-}" = --print-steps ] || [ "${2-}" = --print-steps ] || [ "${3-}" = --print-steps ] || echo "${HC_LANE_FILL:-없음}" >>"$T_LOG/fill.log"   # hc-fill 레인 표시(lanes.tsv 넷째 칸 fill)
[ "${1-}" = --print-steps ] || [ "${2-}" = --print-steps ] || [ "${3-}" = --print-steps ] || echo "${HC_LANE_OWNER:-없음}" >>"$T_LOG/owner.log"   # 레인 주인(lanes.tsv 셋째 칸)
on= name=
while [ $# -gt 0 ]; do case $1 in --on) on=$2; shift 2 ;; --name) name=$2; shift 2 ;; --print-steps) steps=1; shift ;; --) break ;; *) shift ;; esac; done
if [ -n "${steps-}" ]; then echo "### 착지 STEPS $name ${on:-local}"; exit 0; fi
echo "$name ${HERDR_LAND_PRIORITY:-없음}" >>"$T_LOG/landprio.log"   # 착지 줄 우선(그 레인을 기다리는 줄 수)
case " ${HC_FAKE_LANE_FAIL-} " in *" ${on:-here}/$name "*) echo "hc-lane: 가짜 실패" >&2; exit 1 ;; esac
case " ${HC_FAKE_LANE_HELD-} " in *" $name "*) echo "hc-lane: 이미 돎 — refs/hc-lanes/$name 를 다른 실행이 쥐고 있다" >&2; exit 10 ;; esac   # 임대가 남은 레인: 어느 기계든 10
[ -z "${HC_FAKE_LANE_HOOK-}" ] || eval "$HC_FAKE_LANE_HOOK" >/dev/null 2>&1   # 띄우는 동안 끼어드는 일(대기열 rm·add 등)
# $HC_FAKE_LANE_START의 「<기계>/<이름>=<종료>」: 띄우고 터미널은 남긴 채 시작 확인 실패(5·6·7)로 끝난다(진짜 hc-lane처럼 JSON 끝 줄)
for x in ${HC_FAKE_LANE_START-}; do case $x in "${on:-here}/$name="*)
  echo "hc-lane: 시작 안 됨 ${on:+$on/}$name: 가짜 로그인 막힘" >&2; echo "{\"lane\":\"${on:+$on/}$name\",\"name\":\"$name\",\"start\":\"login\"}"; exit "${x#*=}" ;; esac; done
echo "레인 $name"
[ -z "${HC_FAKE_LANE_NOAPP-}" ] || [ -z "$on" ] || echo "hc-lane: Herdr 창이 없어 $on 공간 이름은 못 붙였다(창을 열면 $on/$name 가 보인다)" >&2   # 원격 표시 알림
echo "{\"lane\":\"${on:+$on/}$name\",\"name\":\"$name\"}"
EOF
# 가짜 git: fetch는 기록, symbolic-ref는 origin/main, log --grep=<정규식>은 $T/commits(「sha 제목」 줄)에서 grep -E.
cat >"$T/bin/git" <<'EOF'
#!/bin/bash
echo "$*" >>"$T_LOG/git.log"
if [ -n "${HC_REAL_REPO-}" ] && [ "${1-}" = -C ] && [ "${2-}" = "$HC_REAL_REPO" ]; then exec "$HC_REAL_GIT" "$@"; fi
for a in "$@"; do case $a in
  fetch) [ -z "${HC_FAKE_FETCH_FAIL-}" ] || exit 1   # FETCH_HOOK: 첫 fetch 때 한 번 돌릴 셸 글(바퀴 도중 대기열 바꾸기)
         [ -z "${HC_FAKE_FETCH_HOOK-}" ] || [ -e "$T_LOG/fetch-hook.done" ] || { : >"$T_LOG/fetch-hook.done"; bash -c "$HC_FAKE_FETCH_HOOK" >/dev/null 2>&1; }
         exit 0 ;;
  remote) [ -z "${HC_FAKE_NOORIGIN-}" ] || exit 2; exit 0 ;;   # NOORIGIN: 원격 없는 저장소(get-url 종료 2, ls-remote도 실패)
  # 착지 표식: ls-remote origin 'refs/hc-landed/<레인>/*'는 $T/landed/<레인>(「sha<TAB>ref」 줄, 없으면 진짜처럼 빈 답·종료 0)에서 ·
  # HC_FAKE_LSREMOTE_FAIL: 조회 실패(128) · HC_FAKE_MB_RC: merge-base --is-ancestor 종료(기본 0 = 조상)
  ls-remote) case ${!#} in refs/hc-landed/*)
      [ -z "${HC_FAKE_LSREMOTE_FAIL-}" ] || exit 128; r_=${!#}; r_=${r_#refs/hc-landed/}; cat "$T_LOG/landed/${r_%/\*}" 2>/dev/null; exit 0 ;; esac
    [ -z "${HC_FAKE_MQ_FAIL-}${HC_FAKE_NOORIGIN-}" ] || exit 128   # 병합기 큐·켬 표식: 큐에 든 레인 · 켜진 병합기 기계
    [ -z "${HC_FAKE_MQ-}" ] || printf '0123abcd\trefs/merge-queue/1700000000-%s\n' "$HC_FAKE_MQ"
    [ -z "${HC_FAKE_MGM-}" ] || printf '0123abcd\trefs/herdr/merger/%s\n' "$HC_FAKE_MGM"; exit 0 ;;
  symbolic-ref) echo origin/main; exit 0 ;;
  # 완료 sha를 40자로: <sha>^{commit} → 뒤를 0으로 채운 40자($HC_FAKE_SHA40이면 그 값). $HC_FAKE_REVNONE과 같은 sha는 없음(종료 1, 출력 없음)
  *'^{commit}') s_=${a%'^{commit}'}; [ "$s_" != "${HC_FAKE_REVNONE-}" ] || exit 1; [ -z "${HC_FAKE_SHA40-}" ] || { echo "$HC_FAKE_SHA40"; exit 0; }
    printf '%s%0*d\n' "$s_" $((40 - ${#s_})) 0; exit 0 ;;
  --grep=*) grep -E -- "${a#--grep=}" "$T_LOG/commits" 2>/dev/null | head -n 1; exit 0 ;;
  push) exit "${HC_FAKE_PUSH_RC:-0}" ;;
  merge-base) exit "${HC_FAKE_MB_RC:-0}" ;;
esac; done
case " $* " in *" log "*) [ -z "${HC_FAKE_LOG_FAIL-}" ] || exit 128; cat "$T_LOG/commits"; [ -z "${HC_FAKE_PARTIAL_LOG-}" ] || exit 128 ;; esac   # LOG_FAIL: origin/<기본> 참조 없음
EOF
cat >"$T/bin/herdr-balance" <<'EOF'
#!/bin/bash
echo "$*" >>"$T_LOG/balance.log"
EOF
chmod +x "$T/bin/"*
export PATH="$T/bin:$PATH" HOME="$T/home" T_LOG="$T" HERDR_TOOLS_CONFIG="$T/conf" HC_FILL_WAIT=0 HC_FILL_POLL=0 HC_FAKE_COORD=t8kk
# macOS awk의 ==는 strcoll — en_US.UTF-8에서 "판정"=="완료"가 참이었다(2026-11-19 fill-then). 그 로캘로 돌려 늘 잡히게
export LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8
unset XDG_STATE_HOME   # hc-lane 레인 기록(lanes.tsv)을 가짜 HOME 아래에서 읽게
unset HERDR_LAND_PRIORITY   # 레인 안에서 돌려도 물려받은 값이 띄우기 시험을 바꾸지 않게
unset HC_TERMINAL_ID   # 시험을 지휘 터미널에서 돌려도 add가 그 주소를 주인으로 잡지 않게(주인 시험은 따로 준다)
mkdir -p "$HOME"
: >"$T/commits"

n=0 fails=0
t() { n=$((n + 1)); if eval "$2"; then echo "ok $n - $1"; else fails=$((fails + 1)); echo "FAIL $n - $1"; printf '%s\n' "${out-}" | sed 's/^/    | /'; fi; }
run() { out=$("$FILL" "$@" 2>&1); rc=$?; }
Q=$HOME/.local/share/hc-fill/queue.tsv
# 기계 상태: mj box-1 box-2 … (적은 기계만 connected)
mj() { s='{"machines":['; c=; for m in box-1 box-2 shared slow; do st=disconnected; case " $* " in *" $m "*) st=connected ;; esac; s="$s$c{\"name\":\"$m\",\"state\":\"$st\"}"; c=,; done; export HC_FAKE_MACHINES="$s]}"; }
# 터미널: lj <주소>:<agent>:<이름>[:<상태, 기본 working>[:<cwd>]] …
lj() { s='{"terminals":[{"address":"t0","agent":"claude-code","name":"coord","state":"live","status":"working","cwd":"/"}'; c=,; for x in "$@"; do IFS=: read -r a g nm st cw <<<"$x"; s="$s$c{\"address\":\"$a\",\"agent\":\"$g\",\"name\":\"$nm\",\"state\":\"live\",\"status\":\"${st:-working}\",\"cwd\":\"${cw:-/}\"}"; c=,; done; export HC_FAKE_LS="$s]}"; }
fresh() { rm -rf "$HOME/.local/share/hc-fill" "$T/report.log" "$T/lane.log" "$T/landprio.log" "$T/events" "$T/hc.log" "$T/balance.log" "$T/git.log" "$T/gc.log" "$T/coord.log" "$T/owner.log" "$T/coordset.log" "$T/coordcheck.log"; : >"$T/commits"; mj box-1 box-2 shared slow; lj; unset HC_FAKE_LS_LANE HC_FAKE_NOSTATUS HC_FAKE_LANE_FAIL HC_FAKE_FETCH_FAIL HC_FAKE_GONE HC_FAKE_EXEC_FAIL HC_FAKE_LANE_START HC_FAKE_FINISHED HC_FAKE_COORD_GONE HC_FAKE_REPORT_RC HC_FAKE_SEND_FAIL; }
brief() { printf '# 지시문 %s\n본문\n' "$1" >"$T/$1.md"; echo "$T/$1.md"; }
add() { "$FILL" add --name "$1" --project app --brief "$(brief "$1")" "${@:2}" >/dev/null 2>&1; }
where() { awk -F'\t' -v n="$1" '($1 "") == (n "") {print $6, $7, $8}' "$Q"; }   # 상태 기계 주소(이름은 글자로 — 10과 010이 다르게)
launched() { grep -Fxc -- "$1" "$T/lane.log" 2>/dev/null; }          # hc-lane 띄우기 인자 줄 수

# 0. 도구와 설정
t "도구가 있다" '[ -f "$FILL" ]'
fresh
rm "$T/conf/hc-fill.tsv"; run run
t "설정 없음: 종료 1, 설정 경로와 예시 안내" '[ $rc = 1 ] && printf "%s" "$out" | grep -Fq "$T/conf/hc-fill.tsv" && printf "%s" "$out" | grep -Fq config.example'
cp "$EX" "$T/conf/hc-fill.tsv"

# 설치하는 시험과 격리 도우미가 같은 폴더에 있어야 한다.
if [ -f "$here/../config.example/sync.list" ]; then
  t "배포 목록에 공통 시험 격리 도우미가 있음" 'awk '\''$2==".local/bin/tests/isolate.sh" && $3=="tools:tests/isolate.sh" && ($4=="macos" || $4=="all") {f=1} END {exit !f}'\'' "$here/../config.example/sync.list"'
fi

# 1. add · ls · rm
fresh
run add --name lane-a --project app --brief "$(brief lane-a)"
t "add: 종료 0, 대기열에 한 줄(대기)" '[ $rc = 0 ] && [ "$(where lane-a)" = "wait - -" ]'
run add --name lane-a --project app --brief "$T/lane-a.md"
t "add: 같은 이름 거절" '[ $rc = 1 ] && [ "$(grep -c . "$Q")" = 1 ]'
run add --name Bad --project app --brief "$T/lane-a.md"
t "add: 이름 글자 거절" '[ $rc != 0 ]'
run add --name lane-b --project nope --brief "$T/lane-a.md"
t "add: 설정에 없는 프로젝트 거절" '[ $rc != 0 ]'
run add --name lane-b --project app --brief "$T/없음.md"
t "add: 없는 지시문 거절" '[ $rc != 0 ]'
run add --name lane-b --project app --brief "$T/lane-a.md" --only box-9
t "add: 설정에 없는 기계 --only 거절" '[ $rc != 0 ]'
run ls
t "ls: 레인과 대기 상태" '[ $rc = 0 ] && printf "%s" "$out" | grep -q "lane-a.*대기"'
run rm lane-a
t "rm: 대기열에서 빠지고 아무것도 끄지 않음" '[ $rc = 0 ] && [ -z "$(where lane-a)" ] && [ ! -e "$T/hc.log" ]'
run rm lane-zz
t "rm: 없는 레인은 종료 1" '[ $rc = 1 ]'

# 2. 차례와 상한: box-1(2) → box-2(2) → shared → …
fresh
add l1; add l2; add l3
run run
t "차례: 종료 0" '[ $rc = 0 ]'
t "차례: l1·l2는 box-1, l3은 상한 2를 넘어 box-2" '[ "$(where l1)" = "up box-1 box-1/l1" ] && [ "$(where l2)" = "up box-1 box-1/l2" ] && [ "$(where l3)" = "up box-2 box-2/l3" ]'
t "띄우기: hc-lane --on <기계> --name --repo <그 기계 경로> -- <설정의 launch 틀>" 'grep -Fx -A1 -- --repo "$T/lane.log" | grep -Fxq /home/me/src/app && [ "$(launched example-agent)" = 0 ] && grep -Fxq "exec example-agent --model example-model \"\$1\"" "$T/lane.log"'
t "띄우기: 원격 표시 알림이 없으면 「알림: Herdr 창」 줄 없음" '! printf "%s" "$out" | grep -q "알림: Herdr 창"'
run_l1="$HOME/.local/share/hc-fill/runs/l1-run.md"
t "띄우기: run 지시문 = 지시문 + 착지 줄(그 기계 HOME 아래)" 'grep -Fq "# 지시문 l1" "$run_l1" && grep -Fq "### 착지 STEPS l1 box-1" "$run_l1"'
t "띄우기: 첫 지시는 설정 task 틀로 run 경로를 담은 한 인자" 'grep -Fxq "$run_l1 지시문을 읽고 진행하라." "$T/lane.log"'
t "띄우기: run 지시문은 hc exec -i --on 으로 그 기계에" 'grep -q "^exec -i --on box-1 " "$T/hc.log"'
run run
t "다시 돌려도 떠 있는 레인을 또 띄우지 않음" '[ "$(grep -Fxc -- "--name" "$T/lane.log")" = 6 ] && ! printf "%s" "$out" | grep -q "띄움"'

fresh
lj box-1/x1:claude-code:a box-1/x2:claude-code:b box-1/p:env:proxy box-2/z:zsh:shell
add l1
run run
t "상한: box-1에 에이전트 2개면 box-2로(다른 종류는 세지 않음)" '[ "$(where l1)" = "up box-2 box-2/l1" ]'

# 2b. 원격 띄우기의 창 표시 알림(hc-lane이 그 기계 공간에 못 붙였을 때)은 띄우기는 그대로, 알림 한 줄
fresh
add nv
HC_FAKE_LANE_NOAPP=1 run run
t "원격 표시 알림: 띄움은 그대로, hc-lane 알림을 「알림: Herdr 창 …」 한 줄로" '[ "$(where nv)" = "up box-1 box-1/nv" ] && [ "$(printf "%s\n" "$out" | grep -c "^알림: Herdr 창이 없어 box-1 ")" = 1 ] || { printf "%s\n" "$out"; false; }'

# 3. 「n개 미만일 때만」과 레인상한: shared는 상한 3·레인상한 2
fresh
lj box-1/a:claude-code:a box-1/b:claude-code:b box-2/a:claude-code:a2 box-2/b:claude-code:b2 shared/d1:claude-code:loop-x shared/d2:claude-code:loop-y shared/q:zsh:loop-q
add s1; add s2
run run
t "shared: 에이전트 2개(루프 셸은 안 셈)면 1개만 더 → 다음은 slow" '[ "$(where s1)" = "up shared shared/s1" ] && [ "$(where s2)" = "up slow slow/s2" ]'
fresh
lj box-1/a:claude-code:a box-1/b:claude-code:b box-2/a:claude-code:a2 box-2/b:claude-code:b2
add s1; add s2; add s3
run run
t "shared: 비어 있어도 레인상한 2 → 셋째는 slow" '[ "$(where s1)" = "up shared shared/s1" ] && [ "$(where s2)" = "up shared shared/s2" ] && [ "$(where s3)" = "up slow slow/s3" ]'
lj box-1/a:claude-code:a box-1/b:claude-code:b box-2/a:claude-code:a2 box-2/b:claude-code:b2 shared/s1:claude-code:s1 shared/s2:claude-code:s2 slow/s3:claude-code:s3
add s4
run run
t "shared: 다음 바퀴에도 hc-fill 레인 2개가 살아 있으면 안 띄움 → here" '[ "$(where s4)" = "up here s4" ]'
t "here(local): --on 없이 띄우고 herdr-balance --main <설정 coord> --add" '! awk "/^---\$/ {b=\"\"} {b=b \"\\n\" \$0} END {print b}" "$T/lane.log" | grep -Fxq -- --on && grep -Fxq -- "--main t0 --add s4" "$T/balance.log"'
t "here(local): 경로는 local 기계의 repo 줄" 'grep -Fxq /Users/me/src/app "$T/lane.log"'
t "레인을 띄울 때 hc-lane coord가 확인한 지휘 주소(설정 coord가 아님)를 HC_LANE_COORD로 넘김(지휘의 watch가 없으면 hc-lane이 띄운다 — 감사 [중간] 1·2차 B)" '[ "$(sort -u "$T/coord.log")" = t8kk ] || { cat "$T/coord.log"; false; }'
t "레인을 띄울 때 HC_LANE_FILL=1(hc-lane이 레인 기록에 fill — 완료가 마지막 보고, 재감사 [낮음] 4)" '[ "$(sort -u "$T/fill.log")" = 1 ] || { cat "$T/fill.log"; false; }'

# 3b. 자리 세기: working·blocked는 늘 셈. done·idle은 작업 트리(<저장소>.wt/…)가 그 기계에 남은 쓰기 레인만 셈.
#     읽기 전용(저장소 폴더 그대로)·작업 트리가 정리된 끝난 레인은 안 셈. all 칸 기계(shared)는 다 셈.
W=$T/w; mkdir -p "$W/app" "$W/app.wt/k1" "$W/app.wt/k2"
fresh
lj "box-1/k1:claude-code:k1:done:$W/app.wt/k1" "box-1/k2:claude-code:k2:idle:$W/app.wt/k2"
add f1
run run
t "자리: 쓰기 레인 done·idle + 작업 트리 있음 → 셈(box-1 꽉 참 → box-2)" '[ "$(where f1)" = "up box-2 box-2/f1" ] && printf "%s" "$out" | grep -q "자리 box-1 box-1/k1 k1: done, 작업 트리 있음 → 셈"'
fresh
lj "box-1/au:claude-code:audit:done:$W/app" "box-1/gone:claude-code:gone:done:$W/app.wt/gone" "box-1/b:claude-code:b:blocked:$W/app"
add f2
run run
t "자리: 읽기 전용 감사 done·작업 트리 정리된 done은 안 셈, blocked는 셈(box-1에 1개 → box-1)" '[ "$(where f2)" = "up box-1 box-1/f2" ] && printf "%s" "$out" | grep -q "자리 box-1 box-1/au audit: done, 읽기 전용(작업 트리 아님) → 안 셈" && printf "%s" "$out" | grep -q "자리 box-1 box-1/gone gone: done, 작업 트리 없음 → 안 셈"'
fresh
lj box-1/a:claude-code:a box-1/b:claude-code:b box-2/a:claude-code:a2 box-2/b:claude-code:b2 "shared/l1:claude-code:loop-1:done:/" "shared/l2:claude-code:loop-2:done:/" "shared/l3:claude-code:loop-3:idle:/"
add f3
run run
t "자리: all 칸 기계는 done도 다 셈(shared 3개 → slow)" '[ "$(where f3)" = "up slow slow/f3" ]'
# 2026-11-19 hcfill-idle-seats: hc-lane 레인이 상태 훅을 아직 안 보내면 hc ls가 agent를 argv 첫 낱말(env)로, status를 null로 준다.
# 그 레인도 살아 있는 에이전트라 자리를 차지한다(워커 기계 감사 레인 둘을 안 세어 「자리 빔」 알림). 라벨 없는 env(프록시 등)는 안 셈
fresh
lane_env='{"address":"box-1/e1","agent":"env","name":"audit-e1","state":"live","cwd":"/"}'
export HC_FAKE_LS="{\"terminals\":[{\"address\":\"t0\",\"agent\":\"claude-code\",\"name\":\"coord\",\"state\":\"live\",\"status\":\"working\",\"cwd\":\"/\"},$lane_env,{\"address\":\"box-1/a\",\"agent\":\"claude-code\",\"name\":\"a\",\"state\":\"live\",\"status\":\"working\",\"cwd\":\"/\"},{\"address\":\"box-1/px\",\"agent\":\"env\",\"name\":\"proxy\",\"state\":\"live\",\"cwd\":\"/\"}]}"
export HC_FAKE_LS_LANE="{\"terminals\":[$lane_env]}"
add f4
run run
t "자리: 상태 없는 hc-lane 레인(agent env)도 셈, 라벨 없는 env는 안 셈(box-1 꽉 참 → box-2)" '[ "$(where f4)" = "up box-2 box-2/f4" ] && printf "%s" "$out" | grep -Fq "자리 box-1 box-1/e1 audit-e1: env, 상태 모름(hc-lane 레인) → 셈" && ! printf "%s" "$out" | grep -Fq "box-1/px" || { printf "%s\n" "$out"; false; }'

# 3b-2. 끝난 레인이 자리를 붙잡지 않게(2026-11-19 fill-seat-free — 착지한 레인·지휘를 몇 시간 기다리는 레인이 444바퀴 자리를 막음).
#   done·idle + 작업 트리라도 ① 레인 이름이 그 프로젝트 local 저장소 origin/<기본> 커밋 제목 머리에 있으면(선행 판정과 같은 규칙) 안 셈
#   ② 쉰 지 설정 hold분(기본 60, 0이면 끔)이 넘으면 안 셈. 쉬기 시작한 때는 $DATA/held.tsv(주소<TAB>처음 본 시각)
HELD=$HOME/.local/share/hc-fill/held.tsv
fresh
lj "box-1/k1:claude-code:k1:done:$W/app.wt/k1" "box-1/k2:claude-code:k2:idle:$W/app.wt/k2"
printf '%s\n' "9999ffff fix(x): k1-extra 다른 레인" "aaaa1111 feat(x): k1 끝" >"$T/commits"
add f4
run run
t "자리 ①: done + 작업 트리 + 착지 커밋 → 안 셈(box-1에 1개 → box-1에 뜸)" '[ "$(where f4)" = "up box-1 box-1/f4" ] && printf "%s" "$out" | grep -Fq "자리 box-1 box-1/k1 k1: done, 착지함(aaaa1111) → 안 셈" && printf "%s" "$out" | grep -Fq "자리 box-1 box-1/k2 k2: idle, 작업 트리 있음 → 셈" || { printf "%s\n" "$out"; false; }'
t "자리 ①: fetch는 바퀴에 한 번(자리 판정과 선행 판정이 같이 씀)" '[ "$(grep -c "fetch -q origin" "$T/git.log")" = 1 ]'
fresh
lj "box-1/k1:claude-code:k1:done:$W/app.wt/k1" "box-1/k2:claude-code:k2:idle:$W/app.wt/k2"
echo "aaaa1111 k1: 끝" >"$T/commits"
add f4b
HC_FAKE_FETCH_FAIL=1 run run
t "자리 ①: fetch 실패면 판정 못 함 → 셈(box-1 꽉 참 → box-2)" '[ "$(where f4b)" = "up box-2 box-2/f4b" ] && printf "%s" "$out" | grep -Fq "자리 box-1 box-1/k1 k1: done, 작업 트리 있음 → 셈" || { printf "%s\n" "$out"; false; }'
fresh
lj "box-1/k1:claude-code:k1:done:$W/app.wt/k1" "box-1/k2:claude-code:k2:idle:$W/app.wt/k2"
mkdir -p "${HELD%/*}"; now=$(date +%s); printf 'box-1/k1\t%s\nbox-1/k2\t%s\n' $((now - 61 * 60 - 5)) $((now - 10 * 60)) >"$HELD"
add f5
run run
t "자리 ②: 착지 없음 + 쉰 지 hold(60분) 넘음 → 안 셈, hold 안 → 셈(box-1에 뜸)" '[ "$(where f5)" = "up box-1 box-1/f5" ] && printf "%s" "$out" | grep -Fq "자리 box-1 box-1/k1 k1: done, 쉰 지 61분(hold 60) → 안 셈" && printf "%s" "$out" | grep -Fq "자리 box-1 box-1/k2 k2: idle, 작업 트리 있음 → 셈" || { printf "%s\n" "$out"; false; }'
t "자리 ②: held.tsv의 처음 본 시각은 그대로 둔다" '[ "$(awk -F"\t" "\$1==\"box-1/k1\" {print \$2}" "$HELD")" = $((now - 61 * 60 - 5)) ]'
fresh
lj "box-1/k1:claude-code:k1:done:$W/app.wt/k1" "box-1/k2:claude-code:k2:idle:$W/app.wt/k2"
mkdir -p "${HELD%/*}"; now=$(date +%s); printf 'box-1/k1\t%s\nbox-1/k2\t%s\n' $((now - 30 * 60)) $((now - 10 * 60)) >"$HELD"
add f6
run run
t "자리 ③: 쉰 지 hold 안(30분) → 셈(box-1 꽉 참 → box-2)" '[ "$(where f6)" = "up box-2 box-2/f6" ]'
# 자리 ③-2(2026-11-21 fill-submit-release): 마지막 사건이 「제출」(병합기 큐에 넣고 끝남)인 done·idle 레인은 hold 유예 없이 안 셈.
# 옛 판은 쉰 지 hold(60분)가 지나야 놓았다(제출 → 놓음 61~231분). 같은 레인이 다시 일하면(working) 그대로 셈
fresh
add k1; run run   # k1이 box-1/k1에 up — 사건은 up 줄 기계에서 읽는다
lj "box-1/k1:claude-code:k1:idle:$W/app.wt/k1" "box-1/k2:claude-code:k2:idle:$W/app.wt/k2"
printf '2026-11-21T10:00:00Z\tbox-1\tbox-1/k1\tk1\t제출\tabc12345\tsub-1 → refs/merge-queue/1-k1\t3\n' >"$T/events"
add f6s
run run
t "자리 ③-2: 제출 뒤 idle 레인은 쉰 지 hold 안이어도 안 셈(box-1에 뜸), 제출 없는 레인은 셈" '[ "$(where f6s)" = "up box-1 box-1/f6s" ] && printf "%s" "$out" | grep -Fq "자리 box-1 box-1/k1 k1: idle, 제출함(병합기 큐 — 레인은 끝) → 안 셈" && printf "%s" "$out" | grep -Fq "자리 box-1 box-1/k2 k2: idle, 작업 트리 있음 → 셈" || { printf "%s\n" "$out"; false; }'
lj "box-1/k1:claude-code:k1:working:$W/app.wt/k1" "box-1/k2:claude-code:k2:idle:$W/app.wt/k2" "box-1/f6s:claude-code:f6s"
add f6t
run run
t "자리 ③-2: 제출 뒤 다시 일하는(working) 레인은 셈(box-1 꽉 참 → box-2)" '[ "$(where f6t)" = "up box-2 box-2/f6t" ] || { printf "%s\n" "$out"; false; }'
fresh
sed -i '' 's/^hold .*/hold     0/' "$T/conf/hc-fill.tsv"
lj "box-1/k1:claude-code:k1:done:$W/app.wt/k1" "box-1/k2:claude-code:k2:idle:$W/app.wt/k2"
mkdir -p "${HELD%/*}"; now=$(date +%s); printf 'box-1/k1\t%s\nbox-1/k2\t%s\n' $((now - 600 * 60)) $((now - 600 * 60)) >"$HELD"
add f7
run run
t "자리 ④: hold 0이면 끔 — 오래 쉬어도 셈(box-2)" '[ "$(where f7)" = "up box-2 box-2/f7" ] && grep -q "^hold *0$" "$T/conf/hc-fill.tsv"'
cp "$EX" "$T/conf/hc-fill.tsv"
fresh
lj "box-1/k1:claude-code:k1:working:$W/app.wt/k1" "box-1/k2:claude-code:k2:idle:$W/app.wt/k2"
mkdir -p "${HELD%/*}"; now=$(date +%s); printf 'box-1/k1\t%s\nbox-1/gone\t%s\n' $((now - 90 * 60)) $((now - 90 * 60)) >"$HELD"
add f8
run run
t "자리 ⑤: working으로 바뀌거나 사라진 주소는 held.tsv에서 빠지고, 새로 쉰 주소는 들어감" '! grep -q "^box-1/k1	" "$HELD" && ! grep -q "^box-1/gone	" "$HELD" && grep -q "^box-1/k2	[0-9][0-9]*$" "$HELD" || { cat "$HELD"; false; }'
fresh
lj "box-1/k1:claude-code:k1:done:$W/app.wt/k1"
add f9
run run --dry-run
t "자리: dry-run은 held.tsv를 쓰지 않음" '[ ! -e "$HELD" ]'
# 감사 [낮음] 2: 0으로 시작하는 분 값(08·09)도 십진수로 — $((…))가 8진수로 읽다 죽지 않게(hold·gc·idle·early_wait)
fresh
sed -i '' 's/^hold .*/hold     08/; s/^gc .*/gc       09/; s/^idle .*/idle     08/; s/^early_wait .*/early_wait 09/' "$T/conf/hc-fill.tsv"
lj "box-1/k1:claude-code:k1:done:$W/app.wt/k1" "box-1/k2:claude-code:k2:idle:$W/app.wt/k2"
mkdir -p "${HELD%/*}"; now=$(date +%s); printf 'box-1/k1\t%s\nbox-1/k2\t%s\n' $((now - 9 * 60)) $((now - 2 * 60)) >"$HELD"
add fb
run run
t "설정: hold 08·gc 09·idle 08·early_wait 09도 십진수 — 종료 0, 쉰 지 9분(hold 8) → 안 셈" '[ $rc = 0 ] && [ "$(where fb)" = "up box-1 box-1/fb" ] && printf "%s" "$out" | grep -Fq "쉰 지 9분(hold 8) → 안 셈" || { printf "%s\n" "$out"; false; }'
cp "$EX" "$T/conf/hc-fill.tsv"
# 감사 [낮음] 4: 감사·조사 같은 읽기 레인이 레인 작업 트리를 cwd로 써도, hc-lane 레인 기록(lanes.tsv)의 그 주소 마지막 종류가 read면 안 셈
LT=$HOME/.local/state/hc-lane/lanes.tsv
fresh
lj "box-1/k1:claude-code:audit-k1:done:$W/app.wt/k1" "box-1/k2:claude-code:k2:idle:$W/app.wt/k2"
mkdir -p "${LT%/*}"; printf 'box-1/k1\told-lane\tt0\nbox-1/k1\taudit-k1\tt0\tread\nbox-1/k2\tk2\tt0\tfill\n' >"$LT"
add fc
run run
t "자리: 레인 기록 종류 read(작업 트리 cwd 감사) → 읽기 레인 → 안 셈, fill은 지금처럼 셈(box-1에 뜸)" '[ "$(where fc)" = "up box-1 box-1/fc" ] && printf "%s" "$out" | grep -Fq "자리 box-1 box-1/k1 audit-k1: done, 읽기 레인(레인 기록) → 안 셈" && printf "%s" "$out" | grep -Fq "자리 box-1 box-1/k2 k2: idle, 작업 트리 있음 → 셈" || { printf "%s\n" "$out"; false; }'
rm -f "$LT"
# 감사 [낮음] 1: 자리 세기 전에 끝나는 바퀴(빈 대기열 + idle 0)는 held.tsv를 비운다 — 묵은 시각을 물려받지 않게. dry-run은 그대로
fresh
sed -i '' 's/^idle .*/idle     0/' "$T/conf/hc-fill.tsv"
mkdir -p "${HELD%/*}"; printf 'box-1/k1\t1\n' >"$HELD"
run run --dry-run
t "일찍 끝나는 바퀴: dry-run은 held.tsv를 그대로 둠" '[ $rc = 0 ] && grep -q "^box-1/k1	1$" "$HELD"'
run run
t "일찍 끝나는 바퀴(빈 대기열 + idle 0): held.tsv를 비움" '[ $rc = 0 ] && [ -f "$HELD" ] && [ ! -s "$HELD" ] || { printf "%s\n" "$out"; cat "$HELD"; false; }'
cp "$EX" "$T/conf/hc-fill.tsv"
cp "$EX" "$T/conf/hc-fill.tsv"; echo "hold     5" >>"$T/conf/hc-fill.tsv"
fresh; add fa
run run
t "설정: hold 줄 두 번이면 종료 1" '[ $rc = 1 ] && printf "%s" "$out" | grep -q "hold 줄이 두 번"'
cp "$EX" "$T/conf/hc-fill.tsv"

# 4. 선행 조건: origin/<기본>의 커밋 메시지에 레인 이름(단어 경계)
fresh
add pa; add pb --after pa,pc --no-early   # 선행 판정만 본다(먼저 띄움은 10f)
printf '%s\n' "1111aaaa feat: pa-extra 다른 레인" "4444dddd docs(x): pz 문서 번호 (main에 pa가 먼저 착지)" "5555eeee test: 주입 (pa 감사 반영)" >"$T/commits"
run run
t "선행: 제목 머리가 아닌 언급(괄호·문장 안)은 안 셈" '[ "$(where pb | cut -d" " -f1)" = wait ] && printf "%s" "$out" | grep -q "선행 pb ← pa: 아직" && printf "%s" "$out" | grep -q "선행 pb ← pc: 아직"'
t "선행: fetch는 local 기계 저장소에서 한 번" '[ "$(grep -c "fetch -q origin" "$T/git.log")" = 1 ] && grep -q "^-C /Users/me/src/app fetch" "$T/git.log"'
printf '2222bbbb feat(x): pa 끝\n3333cccc pc: 고침\n' >>"$T/commits"
run run
t "선행: 둘 다 들어가면 띄움, 판정 줄에 커밋" '[ "$(where pb | cut -d" " -f1)" = up ] && printf "%s" "$out" | grep -q "선행 pb ← pa: 들어감 (2222bbbb"'
fresh
add pd --after pa
export HC_FAKE_FETCH_FAIL=1; echo "2222bbbb pa" >"$T/commits"
run run
t "선행: fetch 실패면 띄우지 않음" '[ "$(where pd | cut -d" " -f1)" = wait ] && printf "%s" "$out" | grep -q "fetch 실패"'
unset HC_FAKE_FETCH_FAIL

# 3c. 설정 읽기: 키 혼자면 빈 값, 같은 키 두 번은 멈춤
fresh
sed -i '' 's/^coord .*/coord/' "$T/conf/hc-fill.tsv"
add cf --only here
run run
t "설정: coord 키 혼자면 빈 값 → 붙이지 않음" '[ "$(where cf)" = "up here cf" ] && [ ! -e "$T/balance.log" ]'
fresh; add cn --only here
HC_FAKE_COORD= run run
t "지휘 정보가 없거나 안 맞으면(hc-lane coord 실패) HC_LANE_COORD를 안 넘기고 「지휘 없음」 한 줄(레인은 띄움)" '[ "$(where cn)" = "up here cn" ] && [ "$(tail -n 1 "$T/coord.log")" = 없음 ] && printf "%s" "$out" | grep -Fq "지휘 없음" || { printf "%s\n" "$out"; false; }'
cp "$EX" "$T/conf/hc-fill.tsv"; echo "local    other" >>"$T/conf/hc-fill.tsv"
fresh; add cg
run run
t "설정: 같은 키 두 줄이면 종료 1, 키 이름, 띄우지 않음" '[ $rc = 1 ] && printf "%s" "$out" | grep -q "local 줄이 두 번" && [ ! -e "$T/lane.log" ]'
cp "$EX" "$T/conf/hc-fill.tsv"

# 3d. hc ls에 설정한 agent 종류가 하나도 없으면(지휘 자신도 그 종류) 그 바퀴는 띄우지 않음
fresh
add ag
export HC_FAKE_LS='{"terminals":[{"address":"box-1/p","agent":"env","name":"proxy","state":"live","status":null,"cwd":"/"}]}'
run run
t "agent 종류 0개: 경고, 종료 1, 띄우지 않음" '[ $rc = 1 ] && printf "%s" "$out" | grep -q "claude-code 종류가 하나도 없다" && [ ! -e "$T/lane.log" ]'

# 5. 두 번 띄우지 않음: 같은 이름이 어디선가 살아 있으면 떠 있음으로 표시
fresh
lj "slow/k1:claude-code:dup:working:/Users/other/src/app.wt/dup"; export HC_FAKE_LS_LANE='{"terminals":[{"address":"slow/k1"}]}'
add dup
run run
t "같은 이름 살아 있음(hc-lane 레인·그 프로젝트 폴더): hc-lane 안 부르고 떠 있음으로 표시" '[ ! -e "$T/lane.log" ] && [ "$(where dup)" = "up slow slow/k1" ] && printf "%s" "$out" | grep -q "이미 떠 있음 dup"'
# 4차 B 높음 7(fill-dup-guard → fill-existing-defects): 이름만 같은 남의 터미널을 채택하면 그 레인이 영영 안 뜬다 — 라벨 hc-lane=1과
# 그 기계 그 프로젝트 저장소(또는 그 .wt/ 아래) cwd가 다 맞을 때만 채택
fresh
lj "slow/k1:claude-code:dup:working:/Users/other/src/app.wt/dup" "box-2/k2:claude-code:dup2:working:/elsewhere/dup2"
export HC_FAKE_LS_LANE='{"terminals":[{"address":"box-2/k2"}]}'
add dup; add dup2
run run
t "같은 이름이어도 hc-lane 레인(라벨 hc-lane=1)이 아니면 채택하지 않고 띄움" '[ "$(where dup)" = "up box-1 box-1/dup" ] || { printf "%s\n" "$out"; false; }'
t "같은 이름 hc-lane 레인이어도 그 프로젝트 폴더가 아니면 채택하지 않고 띄움" '[ "$(where dup2)" = "up box-1 box-1/dup2" ] && ! printf "%s" "$out" | grep -q "이미 떠 있음" || { printf "%s\n" "$out"; false; }'

# 6. 연결 안 된 기계는 건너뜀
fresh
mj box-2 shared slow
add c1
run run
t "연결 안 됨: box-1 건너뛰고 box-2" '[ "$(where c1)" = "up box-2 box-2/c1" ] && printf "%s" "$out" | grep -q "건너뜀 box-1: 연결 안 됨"'

# 7. --only
fresh
add o1 --only slow,here
run run
t "--only: 고른 기계만(차례 안에서)" '[ "$(where o1)" = "up slow slow/o1" ]'
fresh
"$FILL" add --name o2 --project tools --brief "$(brief o2)" >/dev/null 2>&1
run run
t "프로젝트 경로가 있는 기계만: tools는 here" '[ "$(where o2)" = "up here o2" ]'

# 8. 띄운 뒤 status가 안 잡히면 그 기계를 빼고 다음 기계로(레인은 버리지 않음, 아무것도 끄지 않음)
fresh
export HC_FAKE_NOSTATUS="box-1/st"
add st
run run
t "status 안 잡힘: box-2로 다시, 남긴 것 기록" '[ "$(where st)" = "up box-2 box-2/st" ] && awk -F"\t" "\$1==\"st\"" "$Q" | grep -Fq "box-1=box-1/st" && printf "%s" "$out" | grep -q "상태 안 잡힘 st"'
t "status 안 잡힘: hc kill 등 부르지 않음" '[ ! -s "$T/hc.log" ] || ! grep -vq "^exec " "$T/hc.log"'
fresh
export HC_FAKE_LANE_FAIL="box-1/lf"
add lf
run run
t "hc-lane 실패: 다음 기계로" '[ "$(where lf)" = "up box-2 box-2/lf" ] && printf "%s" "$out" | grep -q "실패 lf @box-1"'
unset HC_FAKE_LANE_FAIL
fresh; export HC_FAKE_LANE_HELD=lh; add lh; run run; unset HC_FAKE_LANE_HELD
t "hc-lane 종료 10(이미 돎 — 임대 남음): 못 띄움으로 읽어 다음 기계도 10이면 이번 바퀴 포기(줄은 wait 그대로)" '[ "$(where lh)" = "wait - -" ] && [ "$(grep -c "^lh$" "$T/lane.log")" -ge 2 ] && printf "%s" "$out" | grep -q "실패 lh @box-1" || { printf "%s\n" "$out"; false; }'
fresh
export HC_FAKE_LANE_START="box-1/s6=6"
add s6
run run
t "hc-lane 시작 확인 실패(종료 5·6·7): 남은 터미널을 stuck으로 기록하고 그 기계 빼고 다음 기계로(감사 [중간] 1)" '[ "$(where s6)" = "up box-2 box-2/s6" ] && awk -F"\t" "\$1==\"s6\"" "$Q" | grep -Fq "box-1=box-1/s6" && printf "%s" "$out" | grep -q "상태 안 잡힘 s6 @box-1 box-1/s6" && printf "%s" "$out" | grep -q "가짜 로그인 막힘"'

fresh
mj box-1
export HC_FAKE_NOSTATUS="box-1/rv"
add rv --only box-1,box-2
run run
t "남김: 다른 자리가 없으면 대기로 남음" '[ "$(where rv)" = "wait - -" ]'
unset HC_FAKE_NOSTATUS; mj box-1 box-2; lj box-1/rv:claude-code:rv
run run
t "남긴 터미널이 살아나면 중복 위험 찍고 그 레인은 더 띄우지 않음" 'printf "%s" "$out" | grep -q "중복 위험 rv box-1/rv" && [ "$(where rv)" = "wait - -" ] && [ "$(grep -c "^--name$" "$T/lane.log")" = 2 ]'  # 1바퀴째 box-1 하나(print-steps + 띄우기)뿐

fresh
export HC_FAKE_NOSTATUS="box-1/ru"
add ru
run run; unset HC_FAKE_NOSTATUS
lj box-1/ru:claude-code:ru box-2/ru:claude-code:ru
run run
t "떠 있는 레인도 남긴 터미널이 살아나면 중복 위험(둘 다 주소)" 'printf "%s" "$out" | grep -q "중복 위험 ru box-1/ru: .*지금 떠 있는 box-2/ru"'

# 9. --dry-run은 찍기만
fresh
add d1; add d2; add d3
before=$(cat "$Q")
run run --dry-run
t "dry-run: 무엇을 어디에(상한 반영)" '[ $rc = 0 ] && printf "%s" "$out" | grep -q "\[dry-run\] 띄울 d1 → box-1" && printf "%s" "$out" | grep -q "\[dry-run\] 띄울 d3 → box-2"'
t "dry-run: hc-lane·hc exec·herdr-balance·대기열 변화 없음, fetch 없음" '[ ! -e "$T/lane.log" ] && [ ! -e "$T/hc.log" ] && [ ! -e "$T/balance.log" ] && [ "$(cat "$Q")" = "$before" ] && ! grep -q fetch "$T/git.log" 2>/dev/null'

# 9b. hc가 실패하거나 엉뚱한 출력이면 빈 표로 읽지 않고 멈춘다(자리가 다 빈 것처럼 띄우면 안 된다)
fresh
add h1
export HC_FAKE_LS_FAIL=1; run run; unset HC_FAKE_LS_FAIL
t "hc ls 실패: 종료 1, 띄우지 않음" '[ $rc = 1 ] && [ ! -e "$T/lane.log" ] && [ "$(where h1)" = "wait - -" ]'
HC_FAKE_LABEL_RC=1 run run
t "라벨 조회 실패: 출력이 빈 목록이어도 대기 줄을 띄우지 않음" '[ $rc = 1 ] && [ "$(where h1)" = "wait - -" ] && [ ! -e "$T/lane.log" ]'
HC_FAKE_LS_LANE='{}' run run
t "라벨 응답에 terminals가 없으면 이번 바퀴 보류" '[ $rc = 1 ] && [ ! -e "$T/lane.log" ]'
export HC_FAKE_LS='{"error":{"kind":"usage"}}'; run run; unset HC_FAKE_LS
t "hc ls 출력에 terminals 없음: 종료 1, 띄우지 않음" '[ $rc = 1 ] && [ ! -e "$T/lane.log" ]'
export HC_FAKE_MACHINES=''; run run
t "hc machine ls 빈 출력: 종료 1, 띄우지 않음" '[ $rc = 1 ] && [ ! -e "$T/lane.log" ]'

# 10. ls: 떠 있음 · 끝남
fresh
add e1; add e2
run run
lj box-1/e1:claude-code:e1
run ls
t "ls: 살아 있으면 떠 있음, 사라졌으면 끝남" 'printf "%s" "$out" | grep -q "e1.*떠 있음" && printf "%s" "$out" | grep -q "e2.*끝남"'

# 10b. 끝난 레인 정리: 설정 gc 분마다 한 번, repo 줄의 기계·저장소마다 hc-lane gc(--integ는 local 저장소만)
fresh
mj box-1 shared
run run
t "gc: 대기열이 비어도 첫 바퀴에 돈다" '[ $rc = 0 ] && [ -s "$T/gc.log" ] || { cat "$T/gc.log"; false; }'
t "gc: local 저장소는 --on 없이 --integ, 프로젝트마다" 'grep -Fxq "gc --integ --repo /Users/me/src/app" "$T/gc.log" && grep -Fxq "gc --integ --repo /Users/me/src/tools" "$T/gc.log"'
t "gc: 연결된 다른 기계는 --on, --integ 없이" 'grep -Fxq "gc --on box-1 --repo /home/me/src/app" "$T/gc.log" && grep -Fxq "gc --on shared --repo /srv/me/app" "$T/gc.log"'
t "gc: 연결 안 된 기계는 건너뜀과 알림" '! grep -Fq "box-2" "$T/gc.log" && printf "%s" "$out" | grep -Fq "gc 건너뜀 app @box-2: 연결 안 됨"'
t "gc: 출력 줄에 프로젝트·기계" 'printf "%s" "$out" | grep -Fq "gc app @box-1: 지움 0 · 남김 0"'
rm -f "$T/gc.log"; add g1
run run
t "gc: 간격(60분) 안의 다음 바퀴는 안 돈다" '[ ! -e "$T/gc.log" ]'
echo 0 >"$HOME/.local/share/hc-fill/gc.last"
HC_FAKE_GC_RC=1 run run
t "gc: 간격이 지나면 다시, 실패해도 바퀴는 종료 0과 알림" '[ $rc = 0 ] && [ -s "$T/gc.log" ] && printf "%s" "$out" | grep -Fq "gc 실패 app @here (종료 1)"'
fresh
sed -i.bak "s/^gc .*/gc       0/" "$T/conf/hc-fill.tsv"
run run
t "gc: 설정 gc 0이면 끔" '[ $rc = 0 ] && [ ! -e "$T/gc.log" ]'
sed -i.bak "s/^gc .*/gc       60/" "$T/conf/hc-fill.tsv"
run run --dry-run
t "gc: dry-run은 --dry-run을 넘기고 시각을 적지 않음" 'grep -Fxq "gc --integ --dry-run --repo /Users/me/src/app" "$T/gc.log" && [ ! -e "$HOME/.local/share/hc-fill/gc.last" ]'

# 10c. 받은편지: 설정 inbox 줄의 기계 폴더에서 걷어 대기열에, 끝나면 보고를 요청한 기계 done/으로 되돌림
#      가짜 hc exec는 이 기계에서 돌므로 기계마다 폴더를 따로 둔다(shared는 ~/ 풀기 시험)
#      2026-11-20 fill-handoff-integrity: 요청 ID(대기열 11칸 #<ID>)·장부 inbox.tsv·고정본 replies/<ID>.md·요청자 표시 done/.hc-fill/
IS=$T/in-slow IB=$T/in-box2 IH=$HOME/.local/share/hc-fill/inbox RP=$HOME/.local/share/hc-fill/reports
LED=$HOME/.local/share/hc-fill/inbox.tsv RPL=$HOME/.local/share/hc-fill/replies
ifresh() { fresh; rm -rf "$IS" "$IB" "$RP" "$IH"; cp "$EX" "$T/conf/hc-fill.tsv"; printf 'inbox slow %s\ninbox box-2 %s\ninbox shared ~/.local/share/hc-fill/inbox\n' "$IS" "$IB" >>"$T/conf/hc-fill.tsv"; }
req() {  # req <폴더> <이름> <project> [after] [only] — 지시문 먼저, .req는 마지막에 mv
  mkdir -p "$1"; printf '# 요청 %s\n' "$2" >"$1/$2.brief.md"
  { printf 'project\t%s\n' "$3"; [ -z "${4-}" ] || [ "$4" = - ] || printf 'after\t%s\n' "$4"; [ -z "${5-}" ] || printf 'only\t%s\n' "$5"; } >"$1/$2.req.tmp" && mv "$1/$2.req.tmp" "$1/$2.req"; }
rid() { awk -F'\t' -v n="$1" '$1==n {r = $11; sub(/^.*#/, "", r); print r; exit}' "$Q"; }   # 그 줄의 요청 ID
lst() { awk -F'\t' -v r="$1" '$1==r {print $4}' "$LED" 2>/dev/null | tail -n 1; }           # 장부 상태
ino() { stat -f %i "$1" 2>/dev/null; }
ifresh
mj box-1 shared slow
add dupl
req "$IS" inq-a app - slow; req "$IS" nopr nope; req "$IS" Bad.X app; req "$IS" dupl app; req "$IS" badonly app - box-9
req "$IS" nobrief app; rm "$IS/nobrief.brief.md"
req "$IS" emptyb app; printf ' \n\n' >"$IS/emptyb.brief.md"
req "$IH" inq-h app - box-1
req "$IB" inq-off app
run run --dry-run
t "받은편지 dry-run: 받을·거절할만 찍고 아무것도 옮기지 않음" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fq "[dry-run] 받을 slow inq-a (app)" && printf "%s" "$out" | grep -Fq "[dry-run] 거절할 slow nopr: 설정에 없는 project: nope" && [ -f "$IS/inq-a.req" ] && [ ! -e "$IS/rejected" ] && [ -z "$(where inq-a)" ] && [ ! -e "$IS/.hc-fill" ]'
run run
RA=$(rid inq-a)
t "받은편지: 받아 대기열에(되돌릴곳 = 요청 기계:폴더/done/이름.md#<요청 ID>), 같은 바퀴에 띄움, 장부 open" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fq "받음 slow inq-a → 대기열 (app, 우선 높음, 요청 $RA)" && [ "$(where inq-a)" = "up slow slow/inq-a" ] && [ "$(awk -F"\t" "\$1==\"inq-a\" {print \$11}" "$Q")" = "slow:$IS/done/inq-a.md#$RA" ] && case $RA in r*-??????) true ;; *) false ;; esac && [ "$(lst "$RA")" = open ] || { printf "%s\n" "$out"; cat "$Q" "$LED"; false; }'
t "받은편지: 지시문을 briefs/<기계>/<이름>.<요청 ID>.md로 복사, 요청 쪽 둘은 taken/<이름>.<ID>.*으로, 접수 표시는 지움" 'grep -Fq "# 요청 inq-a" "$HOME/.local/share/hc-fill/briefs/slow/inq-a.$RA.md" && [ -f "$IS/taken/inq-a.$RA.req" ] && [ -f "$IS/taken/inq-a.$RA.brief.md" ] && [ ! -e "$IS/inq-a.req" ] && [ ! -e "$IS/.hc-fill/claim/inq-a" ]'
t "받은편지: run 지시문 끝에 결과 보고 절(요청 ID 보고 경로·mv로 두기·결과 줄 꼴)" 'f=$HOME/.local/share/hc-fill/runs/inq-a-run.md; grep -Fq "# 요청 inq-a" "$f" && grep -Fq "~/.local/share/hc-fill/reports/inq-a.$RA.md" "$f" && grep -Fq "결과: 고침 <sha>" "$f" && grep -Fq "mv" "$f"'
t "받은편지 거절: 사유 한 줄 rejected/<이름>.why, 둘은 rejected/로" 'grep -Fxq "설정에 없는 project: nope" "$IS/rejected/nopr.why" && [ -f "$IS/rejected/nopr.req" ] && [ -f "$IS/rejected/nopr.brief.md" ] && [ ! -e "$IS/nopr.req" ] && [ -z "$(where nopr)" ]'
t "받은편지 거절: 이름 글자·같은 이름·없는 only 기계·지시문 없음·빈 지시문" 'grep -q "이름은" "$IS/rejected/Bad.X.why" && grep -Fq "이미 대기열에 있는 이름" "$IS/rejected/dupl.why" && grep -Fq "box-9" "$IS/rejected/badonly.why" && grep -Fq "nobrief.brief.md가 없거나 비었다" "$IS/rejected/nobrief.why" && grep -Fq "emptyb.brief.md가 없거나 비었다" "$IS/rejected/emptyb.why" && [ -f "$IS/rejected/emptyb.req" ] && [ -z "$(where emptyb)" ] && [ "$(grep -c "^dupl	" "$Q")" = 1 ]'
RH=$(rid inq-h)
t "받은편지: ~/ 는 그 기계 HOME으로(shared)" '[ -f "$IH/taken/inq-h.$RH.req" ] && [ "$(awk -F"\t" "\$1==\"inq-h\" {print \$11}" "$Q")" = "shared:~/.local/share/hc-fill/inbox/done/inq-h.md#$RH" ]'
t "받은편지: 연결 안 된 기계는 건너뜀, 요청은 그대로" 'printf "%s" "$out" | grep -Fq "받은편지 건너뜀 box-2: 연결 안 됨" && [ -f "$IB/inq-off.req" ] && [ -z "$(where inq-off)" ]'
t "되돌려 주기: 손으로 넣은 레인(dupl)의 run 지시문에는 결과 보고 절 없음" '! grep -Fq "결과 보고" "$HOME/.local/share/hc-fill/runs/dupl-run.md"'

# 되돌려 주기
lj slow/inq-a:claude-code:inq-a:working box-1/inq-h:claude-code:inq-h:working
req "$IH" inq-h app - box-1
run run
t "되돌려 주기: 보고가 없으면 쓰지 않음" '[ ! -e "$IS/done/inq-a.md" ] && [ "$(where inq-a | cut -d" " -f1)" = up ] && [ "$(lst "$RA")" = open ]'
t "받은편지: 진행 중인 이름으로 새 요청 → 거절 사유, 대기열 줄 하나" 'grep -Fq "이미 대기열에 있는 이름" "$IH/rejected/inq-h.why" && [ ! -e "$IH/inq-h.req" ] && [ "$(grep -c "^inq-h	" "$Q")" = 1 ]'
mkdir -p "$RP"; printf '쓰는 중\n' >"$RP/inq-a.$RA.md"
lj slow/inq-a:claude-code:inq-a:done box-1/inq-h:claude-code:inq-h:working
run run
t "되돌려 주기: 마지막 줄이 「결과: …」가 아닌 반쪽 보고는 보내지 않음(터미널이 살아 있으면 기다림)" '[ ! -e "$IS/done/inq-a.md" ] && [ ! -e "$RPL/$RA.md" ] && [ "$(lst "$RA")" = open ]'
printf '고친 것 설명\n결과: 고침 abc12345\n' >"$RP/inq-a.$RA.md"
lj slow/inq-a:claude-code:inq-a:idle box-1/inq-h:claude-code:inq-h:working
run run
t "되돌려 주기: 완성 보고 → 터미널이 idle로 살아 있어도·착지가 없어도 요청 기계 done/<이름>.md에, 장부·대기열 sent" '[ "$(cat "$IS/done/inq-a.md")" = "$(cat "$RP/inq-a.$RA.md")" ] && [ "$(where inq-a | cut -d" " -f1)" = sent ] && [ "$(lst "$RA")" = sent ] && printf "%s" "$out" | grep -Fq "되돌려 줌 inq-a → slow:$IS/done/inq-a.md (결과: 고침 abc12345)" && grep -q "^exec -i --on slow " "$T/hc.log" || { printf "%s\n" "$out"; cat "$LED"; false; }'
t "되돌려 주기: 고정본 replies/<ID>.md, 요청자 표시 .hc-fill/<ID>.sent(해시)·owner/<이름>" 'cmp -s "$RPL/$RA.md" "$IS/done/inq-a.md" && [ "$(cat "$IS/done/.hc-fill/$RA.sent")" = "$(shasum -a 256 "$RPL/$RA.md" | cut -d" " -f1)" ] && [ "$(cat "$IS/done/.hc-fill/owner/inq-a")" = "$RA" ]'
rm "$IS/done/inq-a.md"; run run
t "되돌려 주기: sent는 다시 쓰지 않음, ls에 되돌려줌" '[ ! -e "$IS/done/inq-a.md" ] && "$FILL" ls 2>&1 | grep -q "inq-a.*되돌려줌.*받은편지 slow"'
req "$IS" inq-a app - slow
run run
t "받은편지: 끝난(sent) 이름으로 새 요청 → 삼키지 않고 rejected/<이름>.why로 알림" 'grep -Fq "이미 끝난 이름" "$IS/rejected/inq-a.why" && [ -f "$IS/rejected/inq-a.req" ] && [ ! -e "$IS/inq-a.req" ] && [ "$(where inq-a | cut -d" " -f1)" = sent ]'
"$FILL" rm inq-a >/dev/null; rm -rf "$IS/rejected"; req "$IS" inq-a app - slow; lj
run run
RA2=$(rid inq-a)
t "다시 받음: 같은 이름 새 요청은 새 요청 ID, 보고 경로도 새 ID(옛 보고는 그대로 둠)" '[ "$(where inq-a)" = "up slow slow/inq-a" ] && [ -n "$RA2" ] && [ "$RA2" != "$RA" ] && [ -f "$RP/inq-a.$RA.md" ] && grep -Fq "reports/inq-a.$RA2.md" "$HOME/.local/share/hc-fill/runs/inq-a-run.md"'
lj slow/inq-a:claude-code:inq-a:done box-1/inq-h:claude-code:inq-h:working
run run
t "다시 띄움: 옛 요청의 보고를 새 요청 결과로 보내지 않음" '[ ! -e "$IS/done/inq-a.md" ] && [ "$(where inq-a | cut -d" " -f1)" = up ] && [ "$(lst "$RA2")" = open ]'
# 요청자 done/<이름>.md에 다른 요청의 다른 결과가 있으면 덮지 않고 held + 지휘 한 번
printf '남의 결과\n' >"$IS/done/inq-a.md"; printf '두 번째\n결과: 넘김 사유\n' >"$RP/inq-a.$RA2.md"
run run
t "되돌려 주기: 그 자리에 다른 요청의 다른 결과가 있으면 덮지 않고 held, 지휘에 한 번(키 hf:<ID>:held)" '[ "$(cat "$IS/done/inq-a.md")" = "남의 결과" ] && [ "$(lst "$RA2")" = held ] && grep -Fq -- "--key hf:$RA2:held" "$T/report.log" || { printf "%s\n" "$out"; cat "$LED"; false; }'
mj shared slow
run run
t "되돌려 주기: 레인 기계가 연결 안 됨이면 사라진 것으로 보지 않고 미룸" '[ ! -e "$IH/done/inq-h.md" ] && [ "$(where inq-h | cut -d" " -f1)" = up ]'
mj box-1 shared slow; lj
run run
t "되돌려 주기: hc ls에 없어도 hc get이 not_found가 아니면 실패를 쓰지 않음" '[ ! -e "$IH/done/inq-h.md" ] && [ "$(where inq-h | cut -d" " -f1)" = up ] && printf "%s" "$out" | grep -q "되돌려 주기 미룸 inq-h"'
export HC_FAKE_FINISHED=box-1/inq-h
run run; unset HC_FAKE_FINISHED
t "되돌려 주기: 보고 없이 프로세스가 끝난(finished) 레인도 「결과: 실패 보고 없음」" '[ "$(cat "$IH/done/inq-h.md" 2>/dev/null)" = "결과: 실패 보고 없음" ] && [ "$(where inq-h | cut -d" " -f1)" = sent ]'

# 완성 보고의 종류와 값이 모두 있어야 고정한다.
ifresh; req "$IS" fmt app - slow; run run; ri=$(rid fmt)
mkdir -p "$RP"; lj slow/fmt:claude-code:fmt:working
for last in '결과: 고침중' '결과: 고침 미정' '결과: 실패  ' '결과: 넘김'; do
  printf '%s\n' "$last" >"$RP/fmt.$ri.md"; run run
  t "완성 전 결과 줄 보류($last)" '[ "$(lst "$ri")" = open ] && [ ! -e "$IS/done/fmt.md" ]'
done
printf '결과: 넘김 다음 담당자가 처리\n' >"$RP/fmt.$ri.md"; run run
t "완성된 이유를 쓰면 같은 요청에 한 번 회신" '[ "$(lst "$ri")" = sent ] && cmp -s "$RP/fmt.$ri.md" "$IS/done/fmt.md"'

# 종료 판정은 오류 종류와 현재 터미널의 이름·상태를 본다.
ifresh; req "$IS" not_found app - slow; run run
lj; run run
t "정상 터미널의 이름 not_found를 종료 오류로 오인하지 않음" '[ "$(lst "$(rid not_found)")" = open ] && [ ! -e "$IS/done/not_found.md" ]'
ifresh; req "$IS" listed app - slow; run run
lj "slow/listed:claude-code:listed:done:/"; HC_FAKE_FINISHED=slow/listed run run
t "목록에 남은 finished 터미널도 보고 없음으로 회신" 'grep -Fq "결과: 실패 보고 없음" "$IS/done/listed.md"'
ifresh; req "$IS" old-in app - slow; run run
lj "slow/old-in:claude-code:other:working:/"
HC_FAKE_GET_NAME=other run run
t "목록과 hc get 모두 주소 재사용을 확인하면 옛 요청을 닫음" 'grep -Fq "결과: 실패 보고 없음" "$IS/done/old-in.md"'

# 같은 주소와 이름이어도 created_ms가 바뀌면 옛 요청의 프로세스는 끝났다.
ifresh; req "$IS" same-end app - slow; run run; ri=$(rid same-end)
lj "slow/same-end:claude-code:same-end:working:/"
HC_FAKE_CREATED="slow/same-end=1700000555000" run run
t "같은 주소·이름의 새 프로세스가 떠 있어도 옛 요청은 종료로 판정" '[ "$(lst "$ri")" = sent ] && grep -Fxq "결과: 실패 보고 없음" "$IS/done/same-end.md"'

# 반쪽 보고 + 터미널 사라짐 → 「결과: 실패 보고 불완전」
ifresh
req "$IS" inq-g app - slow; run run; RG=$(rid inq-g)
mkdir -p "$RP"; printf '반쯤 썼다\n' >"$RP/inq-g.$RG.md"
lj; export HC_FAKE_GONE=slow/inq-g
run run; unset HC_FAKE_GONE
t "되돌려 주기: 반쪽 보고 + 터미널 끝남 확정 → 그 내용에 「결과: 실패 보고 불완전」" '[ "$(tail -n 1 "$IS/done/inq-g.md" 2>/dev/null)" = "결과: 실패 보고 불완전" ] && grep -Fq "반쯤 썼다" "$IS/done/inq-g.md" && [ "$(lst "$RG")" = sent ]'

# 중단 주입: 공개 뒤 로컬 기록 전에 죽음 → 다음 바퀴는 같은 고정본으로 ALREADY(요청자 파일을 다시 쓰지 않음), 장부 sent
ifresh
req "$IS" inq-k app - slow; run run; RK=$(rid inq-k)
mkdir -p "$RP"; printf '결과: 고침 1234567\n' >"$RP/inq-k.$RK.md"; lj slow/inq-k:claude-code:inq-k:done
HC_FILL_CRASH_AT=after-publish run run
t "중단 after-publish: 그 지점에서 죽음, 요청자에는 공개됨, 장부는 아직 deliver" 'grep -Fxq after-publish "$HOME/.local/share/hc-fill/crash.log" && [ -f "$IS/done/inq-k.md" ] && [ "$(lst "$RK")" = deliver ]'
i0=$(ino "$IS/done/inq-k.md")
run run
t "중단 뒤: 같은 요청·같은 내용으로 재시도 → ALREADY, 요청자 파일은 그대로(공개 1번), 장부·대기열 sent" '[ "$(ino "$IS/done/inq-k.md")" = "$i0" ] && [ "$(lst "$RK")" = sent ] && [ "$(where inq-k | cut -d" " -f1)" = sent ] || { printf "%s\n" "$out"; cat "$LED"; false; }'
# 같은 이름의 두 요청이 같은 본문을 냈어도 요청자 경로는 첫 요청 소유다.
ifresh; req "$IS" inq-id app - slow; run run; ri1=$(rid inq-id)
mkdir -p "$RP"; printf '결과: 넘김 확인 필요\n' >"$RP/inq-id.$ri1.md"; run run
"$FILL" rm inq-id >/dev/null; req "$IS" inq-id app - slow; run run; ri2=$(rid inq-id)
cp "$RP/inq-id.$ri1.md" "$RP/inq-id.$ri2.md"; run run
t "같은 결과라도 다른 요청의 파일이면 둘째 요청은 held" '[ -n "$ri2" ] && [ "$ri1" != "$ri2" ] && [ "$(lst "$ri1")" = sent ] && [ "$(lst "$ri2")" = held ] && [ ! -e "$IS/done/.hc-fill/$ri2.sent" ] && [ "$(cat "$IS/done/.hc-fill/owner/inq-id")" = "$ri1" ] && grep -Fq "hf:$ri2:held" "$T/report.log"'
ifresh; req "$IS" old-file app - slow; run run; ri=$(rid old-file)
mkdir -p "$RP" "$IS/done"; printf '결과: 넘김 확인 필요\n' >"$RP/old-file.$ri.md"
cp "$RP/old-file.$ri.md" "$IS/done/old-file.md"; old_inode=$(ino "$IS/done/old-file.md"); run run
t "소유 표시 없는 예전 결과는 같은 해시면 그대로 회수" '[ "$(lst "$ri")" = sent ] && [ "$(ino "$IS/done/old-file.md")" = "$old_inode" ] && [ "$(cat "$IS/done/.hc-fill/owner/old-file")" = "$ri" ]'

# 고정 뒤 장부 전에 죽음 → 다음 바퀴가 그 고정본으로 이어 감(보고가 그 사이 바뀌어도 처음 고정본)
ifresh; req "$IS" inq-once app - slow; run run; RO=$(rid inq-once)
mkdir -p "$RP" "$T/receipt-bin"; printf '결과: 고침 abc1234\n' >"$RP/inq-once.$RO.md"
cat >"$T/receipt-bin/mv" <<'EOF'
#!/bin/bash
/bin/mv "$@" || exit 1
if [ "${!#}" = "$RECEIPT_PATH" ]; then /bin/mv "$RECEIPT_PATH" "$RECEIPT_PATH.read"; exit 1; fi
EOF
chmod +x "$T/receipt-bin/mv"; RECEIPT_PATH=$IS/done/inq-once.md PATH="$T/receipt-bin:$PATH" run run
t "공개 직후 중단: 요청자가 결과를 먼저 수거하고 보내는 쪽에는 완료 표시가 없음" '[ -s "$IS/done/inq-once.md.read" ] && [ ! -e "$IS/done/.hc-fill/$RO.sent" ] && [ "$(lst "$RO")" = deliver ]'
run run
t "공개 직후 중단: 이미 수거한 결과를 다시 만들지 않고 sent 복구" '[ ! -e "$IS/done/inq-once.md" ] && [ -s "$IS/done/.hc-fill/$RO.sent" ] && [ "$(lst "$RO")" = sent ]'

ifresh
req "$IS" inq-f app - slow; run run; RF=$(rid inq-f)
mkdir -p "$RP"; printf '결과: 고침 aaaaaaa\n' >"$RP/inq-f.$RF.md"; lj slow/inq-f:claude-code:inq-f:done
HC_FILL_CRASH_AT=after-reply-fix run run
printf '결과: 고침 bbbbbbb\n' >"$RP/inq-f.$RF.md"
run run
t "중단 after-reply-fix: 고정본이 확정본 — 다음 바퀴에 처음 내용으로 공개(장부 sent)" '[ "$(cat "$IS/done/inq-f.md")" = "결과: 고침 aaaaaaa" ] && [ "$(lst "$RF")" = sent ]'
t "고정 뒤 중단해도 처음 결과의 종류 보존" '[ "$(cut -f 5 "$LED")" = result ]'
# 잘린 입력(장부 크기 다름) → BADINPUT, 공개 0
ifresh
req "$IS" inq-b app - slow; run run; RB=$(rid inq-b)
mkdir -p "$RP"; printf '결과: 고침 ccccccc\n' >"$RP/inq-b.$RB.md"; lj slow/inq-b:claude-code:inq-b:done
HC_FILL_CRASH_AT=after-reply-ledger run run
awk -F'\t' -v OFS='\t' -v r="$RB" '$1==r {$7 = $7 + 5} 1' "$LED" >"$LED.t" && mv "$LED.t" "$LED"
run run
t "공개: 받은 내용 크기·해시가 다르면 BADINPUT — 공개 안 함, 다음 바퀴에 다시" '[ ! -e "$IS/done/inq-b.md" ] && [ "$(lst "$RB")" = deliver ] && printf "%s" "$out" | grep -Fq "BADINPUT"'

# 취소도 전달: 결과 전 rm → 취소문, 결과 고정 뒤 rm → 결과가 감
ifresh; mj box-1 shared slow
add pre1   # 착지 안 된 선행 — inq-c는 대기로 남는다
req "$IS" inq-c app pre1 slow; run run; RC=$(rid inq-c)
"$FILL" rm inq-c >/dev/null
t "rm(결과 전): 고정본은 취소문, 장부 deliver, 대기열에서 빠짐" 'grep -q "^결과: 실패 취소 — hc-fill rm" "$RPL/$RC.md" && [ "$(lst "$RC")" = deliver ] && [ -z "$(where inq-c)" ]'
run run
t "rm 뒤 바퀴: 취소문이 요청자에게 감, 장부 sent" 'grep -q "^결과: 실패 취소" "$IS/done/inq-c.md" && [ "$(lst "$RC")" = sent ]'
req "$IS" inq-d app - slow; run run; RD=$(rid inq-d)
mkdir -p "$RP"; printf '결과: 고침 ddddddd\n' >"$RP/inq-d.$RD.md"; lj slow/inq-d:claude-code:inq-d:done
HC_FILL_CRASH_AT=after-reply-fix run run
"$FILL" rm inq-d >/dev/null; run run
t "rm(결과 고정 뒤): 결과가 이긴다 — 요청자에 결과, 취소문 아님" '[ "$(cat "$IS/done/inq-d.md")" = "결과: 고침 ddddddd" ] && [ "$(lst "$RD")" = sent ]'
req "$IS" inq-e app - box-2; run run; RE=$(rid inq-e)   # box-2는 연결 안 됨 — 대기로 남는다
HC_FILL_CRASH_AT=rm-after-cancel "$FILL" rm inq-e >/dev/null 2>&1
t "중단 rm-after-cancel: 취소문은 고정, 줄은 아직 대기열에" '[ -f "$RPL/$RE.md" ] && [ "$(where inq-e | cut -d" " -f1)" = wait ]'
: >"$T/lane.log"; run run
t "그 뒤 바퀴: 결과가 고정된 대기 줄은 빼고(띄우지 않음) 취소문을 보냄" '[ -z "$(where inq-e)" ] && ! grep -Fxq inq-e "$T/lane.log" && grep -q "^결과: 실패 취소" "$IS/done/inq-e.md" && [ "$(lst "$RE")" = sent ] || { printf "%s\n" "$out"; false; }'
req "$IS" inq-stop app - box-2; run run; RS_=$(rid inq-stop)
HC_FILL_CRASH_AT=after-reply-fix "$FILL" rm inq-stop >/dev/null 2>&1
mj box-2 shared slow; : >"$T/lane.log"; run run
t "대기 줄 취소문 고정 직후 중단: 다시 띄우지 않고 취소를 회신" '[ -z "$(where inq-stop)" ] && ! grep -Fxq inq-stop "$T/lane.log" && [ "$(lst "$RS_")" = sent ] && grep -q "^결과: 실패 취소" "$IS/done/inq-stop.md"'
# 빈 대기열 + idle 0 + 장부 deliver → 그 바퀴에 공개(빈 대기열 조기 종료보다 복구가 먼저)
ifresh; sed -i '' 's/^idle .*/idle 0/' "$T/conf/hc-fill.tsv"; grep -q '^idle ' "$T/conf/hc-fill.tsv" || echo "idle 0" >>"$T/conf/hc-fill.tsv"; mj box-1 shared slow
req "$IS" inq-z app - box-2; run run; RZ=$(rid inq-z)
HC_FILL_CRASH_AT=rm-after-cancel "$FILL" rm inq-z >/dev/null 2>&1
awk -F'\t' '$1!="inq-z"' "$Q" >"$Q.t" && mv "$Q.t" "$Q"
run run
t "빈 대기열 + idle 0 + 장부 deliver: 그 바퀴에 공개" '[ ! -s "$Q" ] && grep -q "^결과: 실패 취소" "$IS/done/inq-z.md" && [ "$(lst "$RZ")" = sent ] || { printf "%s\n" "$out"; false; }'

# 접수 중단: 접수 표시를 둔 뒤 죽음 → 다음 바퀴가 같은 요청 ID로 잇는다(줄 하나)
ifresh
req "$IS" inq-q app - slow
HC_FILL_CRASH_AT=inbox-after-claim run run
CQ=$(cut -d " " -f 1 "$IS/.hc-fill/claim/inq-q" 2>/dev/null)
t "중단 inbox-after-claim: 접수 표시만 남고 대기열·장부 없음" '[ -n "$CQ" ] && [ -z "$(where inq-q)" ] && [ -f "$IS/inq-q.req" ]'
run run
t "그 뒤 바퀴: 같은 요청 ID로 받음(대기열 줄 하나), taken/<이름>.<ID>, 접수 표시 지움" '[ "$(rid inq-q)" = "$CQ" ] && [ "$(grep -c "^inq-q	" "$Q")" = 1 ] && [ -f "$IS/taken/inq-q.$CQ.req" ] && [ ! -e "$IS/.hc-fill/claim/inq-q" ]'
ifresh
req "$IS" inq-w app - slow
HC_FILL_CRASH_AT=inbox-after-add run run
run run
t "중단 inbox-after-add: 다음 바퀴는 옮기기만(거절·중복 줄 없음)" '[ "$(grep -c "^inq-w	" "$Q")" = 1 ] && [ -f "$IS/taken/inq-w.$(rid inq-w).req" ] && [ ! -e "$IS/rejected" ]'
# 접수 표시가 있는데 원본이 바뀌었다(새 요청) → 옛 접수가 끝날 때까지 손대지 않음
ifresh
req "$IS" inq-v app - slow
HC_FILL_CRASH_AT=inbox-after-claim run run
printf '# 바뀐 새 요청\n' >"$IS/inq-v.brief.md"
run run
t "접수 표시와 원본 해시가 다르면(새 요청) 옮기지도 받지도 않음" '[ -z "$(where inq-v)" ] && [ -f "$IS/inq-v.req" ] && [ -f "$IS/inq-v.brief.md" ] && printf "%s" "$out" | grep -Fq "접수 중인 요청"'
# .req 없이 남은 지시문은 건드리지 않음
ifresh
mkdir -p "$IS/taken"; printf '# 새 지시문\n' >"$IS/inq-u.brief.md"; printf 'project\tapp\n' >"$IS/taken/inq-u.req"
run run
t ".req 없이 남은 지시문(소속 불명)은 옮기지 않음" '[ -f "$IS/inq-u.brief.md" ] && [ ! -e "$IS/taken/inq-u.brief.md" ] && [ -z "$(where inq-u)" ]'

# 접수 원본은 taken에 도착했고 이전 표시를 치우기 전에 같은 이름의 새 요청이 도착했다.
ifresh; req "$IS" again app - slow
HC_FILL_CRASH_AT=inbox-after-add run run; oi=$(rid again)
mkdir -p "$IS/taken"; mv "$IS/again.brief.md" "$IS/taken/again.$oi.brief.md"; mv "$IS/again.req" "$IS/taken/again.$oi.req"
run rm again
req "$IS" again app - slow; echo '새 요청 내용' >>"$IS/again.brief.md"; run run
t "taken까지 도착한 옛 표시가 같은 이름의 새 요청을 영구 보류하지 않음" '[ -n "$(rid again)" ] && [ "$(rid again)" != "$oi" ] && [ -f "$IS/taken/again.$(rid again).req" ]'

# 미리보기에서 원격 접수 표시를 지우지 않는다.
ifresh; mkdir -p "$IS/.hc-fill/claim"; echo orphan >"$IS/.hc-fill/claim/orphan"
run run --dry-run
t "받은편지 dry-run은 남은 접수 표시도 보존" '[ -f "$IS/.hc-fill/claim/orphan" ]'
run run
t "실제 바퀴는 요청 없는 표시를 회수" '[ ! -e "$IS/.hc-fill/claim/orphan" ]'

# sent 투영이 대기열 잠금을 기다리는 사이 rm·add로 같은 이름의 새 줄이 들어왔다.
ifresh; req "$IS" project-sent app - slow; run run; ri=$(rid project-sent)
mkdir -p "$RP" "$T/projection-bin"; printf '결과: 넘김 완료\n' >"$RP/project-sent.$ri.md"
brief replacement >/dev/null
cat >"$T/projection-bin/lockf" <<'EOF'
#!/bin/sh
case " $* " in *" _set project-sent sent "*)
  "$FILL_UNDER_TEST" rm project-sent >/dev/null &&
    "$FILL_UNDER_TEST" add --name project-sent --project app --brief "$T_LOG/replacement.md" --only slow >/dev/null || exit 1 ;;
esac
exec /usr/bin/lockf "$@"
EOF
chmod +x "$T/projection-bin/lockf"; FILL_UNDER_TEST="$FILL" PATH="$T/projection-bin:$PATH" run run
t "옛 회신의 sent는 새 줄에 투영하지 않음" '[ "$(lst "$ri")" = sent ] && [ "$(where project-sent | cut -d" " -f1)" != sent ] && grep -Fq "# 지시문 replacement" "${Q%/*}/briefs/added/project-sent.md"'

# 접수 도중 rm: 대기열은 기록됐고 원격 .req가 아직 남아 있다.
for gap in 장부전 장부후; do
  ifresh; req "$IS" cutin app - slow
  HC_FILL_CRASH_AT=inbox-after-add run run
  ri=$(rid cutin)
  [ "$gap" != 장부전 ] || : >"$LED"
  run rm cutin; run run
  t "접수 $gap 중단 뒤 rm: 같은 요청을 다시 띄우지 않고 취소 회신" '[ -n "$ri" ] && [ -z "$(where cutin)" ] && [ "$(lst "$ri")" = sent ] && grep -Fq "결과: 실패 취소" "$IS/done/cutin.md" && [ -f "$IS/taken/cutin.$ri.req" ]'
done

# 병합기 2단계(2차 높음 6): 병합기에 제출하고 끝난 받은편지 레인은 그 제출 ID의 병합기 결과가 나온 뒤 회신하고 결과를 끝 줄로 붙인다
ifresh; mj box-1 shared slow
req "$IS" inq-m app - slow
run run; RM=$(rid inq-m)
lj slow/inq-m:claude-code:inq-m:done
mkdir -p "$RP"; printf '고친 것\n결과: 고침 abc12345\n' >"$RP/inq-m.$RM.md"
HC_FAKE_EVENTS_FAIL=1 run run
t "되돌려 주기: 사건 조회 실패 때 완성 보고를 고정하지 않음" '[ "$(lst "$RM")" = open ] && [ ! -e "$RPL/$RM.md" ]'
echo "모름 slow" >"$T/events"; run run
t "되돌려 주기: 제출자 기계 사건이 모름이면 완성 보고도 미룸" '[ "$(lst "$RM")" = open ] && [ ! -e "$IS/done/inq-m.md" ]'
printf '2026-11-20T10:00:00Z\tslow\tslow/inq-m\tinq-m\t제출\tabc12345\tslow-20261120T100000-9 → refs/merge-queue/1-inq-m\t2\n' >"$T/events"
HC_FAKE_MGM=box-2 run run
t "되돌려 주기: 제출하고 끝난 레인은 완성 보고가 있어도 병합기 결과 전에는 회신 안 함(고정도 안 함)" '[ ! -e "$IS/done/inq-m.md" ] && [ ! -e "$RPL/$RM.md" ] && [ "$(where inq-m | cut -d" " -f1)" = up ] && printf "%s" "$out" | grep -Fq "병합기 결과 기다림(제출 ID slow-20261120T100000-9)" || { printf "%s\n" "$out"; false; }'
printf '2026-11-20T10:04:00Z\tbox-2\tbox-2/t9\tinq-m\t막힘\t-\t제출 ID slow-20261120T100000-90, 로그\t-\n' >>"$T/events"
HC_FAKE_MGM=box-2 run run
t "병합기 회신: ID 앞부분만 같은 다른 제출의 막힘은 무시" '[ "$(lst "$RM")" = open ] && [ ! -e "$RPL/$RM.md" ]'
printf '2026-11-20T10:05:00Z\tbox-2\tbox-2/t9\tinq-m\t착지\tdef4567890ab\t병합기 착지 — 제출 ID slow-20261120T100000-9 (갈래점 x)\t-\n' >>"$T/events"
HC_FAKE_MGM=box-2 run run
t "되돌려 주기: 그 제출 ID의 병합기 착지 뒤 회신, 끝 줄에 착지 sha" '[ "$(tail -n 1 "$IS/done/inq-m.md" 2>/dev/null)" = "착지 def4567890ab (병합기 box-2, 제출 ID slow-20261120T100000-9)" ] && head -n 1 "$IS/done/inq-m.md" | grep -Fxq "고친 것" && [ "$(where inq-m | cut -d" " -f1)" = sent ] && [ "$(lst "$RM")" = sent ] || { printf "%s\n" "$out"; cat "$IS/done/inq-m.md"; false; }'

# 제출 뒤에 중간 보고가 있어도 같은 실행의 제출 결과를 기다린다.
ifresh; req "$IS" post app - slow; run run; ri=$(rid post)
mkdir -p "$RP"; printf '결과: 고침 abc1234\n' >"$RP/post.$ri.md"
printf '2026-11-20T10:00:00Z\tslow\tslow/post\tpost\t제출\tabc1234\tsub-1 → 큐\t1\n2026-11-20T10:01:00Z\tslow\tslow/post\tpost\t중간\t-\t정리\t2\n' >"$T/events"
HC_FAKE_MGM=box-2 run run
t "제출 뒤 중간 보고가 끼어도 착지 전 회신을 보류" '[ "$(lst "$ri")" = open ] && [ ! -f "$IS/done/post.md" ]'
printf '2026-11-20T10:02:00Z\tbox-2\tbox-2/t9\tpost\t착지\tdef1234\t제출 ID sub-1\t-\n' >>"$T/events"; HC_FAKE_MGM=box-2 run run
t "제출 뒤 중간 보고가 있어도 정확한 착지로 회신" '[ "$(lst "$ri")" = sent ] && tail -n 1 "$IS/done/post.md" | grep -Fq def1234'
t "목록에 쓸 마지막 보고도 같은 제출의 착지" 'grep -q "^post	착지	def1234	" "${Q%/*}/last.tsv"'

# 지시문 있음 확인이 한 번 실패해도 거절하지 않음
ifresh
req "$IS" inq-c app - slow
export HC_FAKE_EXEC_FAIL='then echo yes'
run run; unset HC_FAKE_EXEC_FAIL
t "받은편지: 지시문 확인 실패는 거절이 아니라 다음 바퀴에" '[ ! -e "$IS/rejected" ] && [ -f "$IS/inq-c.req" ] && printf "%s" "$out" | grep -q "inq-c: 지시문을 확인하지 못했다, 다음 바퀴에"'
run run
t "받은편지: 다음 바퀴에 받음" '[ -f "$IS/taken/inq-c.$(rid inq-c).req" ] && [ "$(where inq-c | cut -d" " -f1)" = up ]'

# 옛 줄(11칸에 #요청 ID 없음) 옮기기: 결정적 ID rl…, 떠 있던 줄은 옛 보고 경로, 옛 차례로 넣고 못 옮긴 요청은 옮기기만
ifresh
req "$IS" inq-r app - slow
mkdir -p "$(dirname "$Q")"
printf 'inq-r\tapp\t%s\t-\tslow\twait\t-\t-\t-\t2026-01-01T00:00\tslow:%s/done/inq-r.md\n' "$(brief inq-r)" "$IS" >"$Q"
printf 'inq-l\tapp\t%s\t-\tslow\tup\tslow\tslow/inq-l\t-\t2026-01-01T00:00\tslow:%s/done/inq-l.md\n' "$(brief inq-l)" "$IS" >>"$Q"
mkdir -p "$RP"; printf '결과: 고침 eeeeeee\n' >"$RP/inq-l.md"; lj slow/inq-l:claude-code:inq-l:done
run run --dry-run
t "옛 줄: dry-run은 옮기지 않음" '! grep -q "#" "$Q"'
run run
RR=$(rid inq-r)
t "옛 줄 옮기기: 결정적 요청 ID rl…·장부 open, 옛 차례로 못 옮긴 요청은 그 ID로 taken/으로 옮기기만(거절·중복 줄 없음)" 'case $RR in rl????????) true ;; *) false ;; esac && [ -f "$IS/taken/inq-r.$RR.req" ] && [ ! -e "$IS/rejected" ] && [ "$(grep -c "^inq-r	" "$Q")" = 1 ] || { printf "%s\n" "$out"; cat "$Q"; false; }'
t "옛 줄 옮기기: 이미 떠 있던 줄은 옛 보고 경로(reports/<레인>.md)로 되돌려 줌" '[ "$(cat "$IS/done/inq-l.md")" = "결과: 고침 eeeeeee" ] && [ "$(lst "$(rid inq-l)")" = sent ] || { printf "%s\n" "$out"; cat "$LED"; false; }'

# 10칸 옛 줄도 읽는다
ifresh
mkdir -p "$(dirname "$Q")"; brief old1 >/dev/null
printf 'old1\tapp\t%s\t-\t-\twait\t-\t-\t-\t2026-01-01T00:00\n' "$T/old1.md" >"$Q"
run run
t "10칸 옛 줄: 띄우고 되돌려 주기 없음" '[ "$(where old1)" = "up box-1 box-1/old1" ] && ! grep -Fq "결과 보고" "$HOME/.local/share/hc-fill/runs/old1-run.md"'
lj
run run
t "10칸 옛 줄: 터미널이 사라져도 되돌려 주기 하지 않음" '[ "$(where old1 | cut -d" " -f1)" = up ] && ! printf "%s" "$out" | grep -q "되돌려"'

# 10b. 지시문은 add 때 데이터 폴더로 복사, 못 읽는·빈 지시문은 add·run 모두 막는다(launchd가 ~/Documents를 못 읽어 빈 레인을 띄운 일)
fresh
run add --name cp1 --project app --brief "$(brief cp1)"
CP1=$HOME/.local/share/hc-fill/briefs/added/cp1.md
t "add: 지시문 내용을 briefs/added/<레인>.md로 복사, 대기열은 그 경로" '[ $rc = 0 ] && grep -Fq "# 지시문 cp1" "$CP1" && [ "$(awk -F"\t" "\$1==\"cp1\" {print \$3}" "$Q")" = "$CP1" ]'
printf '# 바뀐 원본\n' >"$T/cp1.md"; run add --name cp1 --project app --brief "$T/cp1.md"
t "add: 같은 이름 거절은 이미 넣은 사본을 덮지 않음" '[ $rc = 1 ] && grep -Fq "# 지시문 cp1" "$CP1" && [ -z "$(ls "${CP1%/*}" | grep -v "^cp1.md$")" ]'
brief nr >/dev/null; chmod 000 "$T/nr.md"; run add --name nr --project app --brief "$T/nr.md"; chmod 600 "$T/nr.md"
t "add: 읽지 못하는 지시문 거절(대기열·사본 없음)" '[ $rc = 1 ] && [ -z "$(where nr)" ] && [ ! -e "${CP1%/*}/nr.md" ] && printf "%s" "$out" | grep -q "지시문을 읽지 못함"'
: >"$T/emp.md"; run add --name emp --project app --brief "$T/emp.md"
t "add: 빈 지시문 거절" '[ $rc = 1 ] && [ -z "$(where emp)" ] && printf "%s" "$out" | grep -q "지시문을 읽지 못함(빈 파일)"'
printf ' \n\t\n' >"$T/ws.md"; run add --name ws --project app --brief "$T/ws.md"
t "add: 공백·줄바꿈만 든 지시문도 빈 것으로 거절" '[ $rc = 1 ] && [ -z "$(where ws)" ] && printf "%s" "$out" | grep -q "지시문을 읽지 못함(빈 파일)"'
# 옛 꼴(데이터 폴더 밖 경로) 줄: 못 읽거나 비었으면 띄우지 않고 다음 레인으로
fresh
mkdir -p "$(dirname "$Q")"; brief ob1 >/dev/null; : >"$T/ob2.md"; brief ob3 >/dev/null; chmod 000 "$T/ob1.md"
printf 'ob1\tapp\t%s\t-\t-\twait\t-\t-\t-\t2026-01-01T00:00\t-\nob2\tapp\t%s\t-\t-\twait\t-\t-\t-\t2026-01-01T00:00\t-\nob3\tapp\t%s\t-\t-\twait\t-\t-\t-\t2026-01-01T00:00\t-\n' "$T/ob1.md" "$T/ob2.md" "$T/ob3.md" >"$Q"
run run; chmod 600 "$T/ob1.md"
t "run: 읽지 못하는 지시문은 띄우지 않고 실패로 남김" '[ "$(where ob1)" = "wait - -" ] && [ "$(launched ob1)" = 0 ] && printf "%s" "$out" | grep -q "실패 ob1: 지시문을 읽지 못함("'
t "run: 빈 지시문도 띄우지 않음" '[ "$(where ob2)" = "wait - -" ] && [ "$(launched ob2)" = 0 ] && printf "%s" "$out" | grep -q "실패 ob2: 지시문을 읽지 못함(빈 파일)"'
t "run: 다음 레인은 그대로 띄우고 run 지시문에 본문이 들어감" '[ "$(where ob3)" = "up box-1 box-1/ob3" ] && grep -Fq "# 지시문 ob3" "$HOME/.local/share/hc-fill/runs/ob3-run.md"'

# 10d. 주인 지휘(2026-11-17 hcfill-owner): 지휘 tq1가 넣은 레인을 다른 지휘가 넘겨받아 tq1가 알림을 못 받았다.
# add는 부른 지휘($HC_TERMINAL_ID·--coord)를 대기열 열두째 칸에 적고 지휘 정보를 남기며, 띄울 때 그 지휘를 HC_LANE_OWNER·HC_LANE_COORD로
fresh
HC_TERMINAL_ID=tq1 run add --name ow1 --project app --brief "$(brief ow1)"
t "주인: add가 부른 지휘를 열두째 칸에 적고 hc-lane coord --set으로 지휘 정보를 남김" '[ $rc = 0 ] && [ "$(awk -F"\t" "\$1==\"ow1\" {print \$12}" "$Q")" = tq1 ] && [ "$(cat "$T/coordset.log")" = tq1 ] || { printf "%s\n" "$out"; cat "$Q"; false; }'
run add --name ow2 --project app --brief "$(brief ow2)" --coord tq2
t "주인: --coord로 바꿈" '[ $rc = 0 ] && [ "$(awk -F"\t" "\$1==\"ow2\" {print \$12}" "$Q")" = tq2 ]'
add ow3
t "주인: 터미널 밖 add는 주인 없음(-)" '[ "$(awk -F"\t" "\$1==\"ow3\" {print \$12}" "$Q")" = - ]'
HC_FAKE_COORD_SET_FAIL=t5ln HC_TERMINAL_ID=t5ln run add --name ow4 --project app --brief "$(brief ow4)"
t "주인: 지휘 터미널이 아니면(레인 안) 주인 없이 넣고 한 줄" '[ $rc = 0 ] && [ "$(awk -F"\t" "\$1==\"ow4\" {print \$12}" "$Q")" = - ] && printf "%s" "$out" | grep -Fq "주인 없이" || { printf "%s\n" "$out"; false; }'
HC_FAKE_COORD_SET_FAIL=t5ln run add --name ow5 --project app --brief "$(brief ow5)" --coord t5ln
t "주인: --coord가 지휘가 아니면 거절" '[ $rc = 1 ] && [ -z "$(where ow5)" ]'
run ls
t "ls: 주인 칸" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -q "^ow1 .*tq1" && printf "%s\n" "$out" | grep -q "^ow2 .*tq2" && printf "%s\n" "$out" | head -n 1 | grep -q 주인 || { printf "%s\n" "$out"; false; }'
run run
t "띄울 때 주인을 HC_LANE_OWNER·HC_LANE_COORD로(주인 없는 줄은 hc-lane coord 지휘 t8kk, 주인 없음)" '[ "$(paste -d" " "$T/owner.log" "$T/coord.log" | tr "\n" ,)" = "tq1 tq1,tq2 tq2,없음 t8kk,없음 t8kk," ] || { paste "$T/owner.log" "$T/coord.log"; printf "%s\n" "$out"; false; }'
fresh
HC_TERMINAL_ID=tq1 run add --name ow6 --project app --brief "$(brief ow6)" --only here
run run
t "로컬 레인은 주인 지휘 옆에 붙임(herdr-balance --main <주인>)" 'grep -Fxq -- "--main tq1 --add ow6" "$T/balance.log" || { cat "$T/balance.log" 2>/dev/null; printf "%s\n" "$out"; false; }'
fresh
mkdir -p "$(dirname "$Q")"; brief ow7 >/dev/null
printf 'ow7\tapp\t%s\t-\t-\twait\t-\t-\t-\t2026-01-01T00:00\t-\n' "$T/ow7.md" >"$Q"
run run
t "11칸 옛 줄은 주인 없음으로 띄움" '[ "$(where ow7)" = "up box-1 box-1/ow7" ] && [ "$(cat "$T/owner.log")" = 없음 ] || { printf "%s\n" "$out"; false; }'
t "11칸 옛 줄은 주인 없음으로 띄움(주인 확인 안 함)" '[ ! -e "$T/coordcheck.log" ]'
# 감사 [중간] 1(a): 대기열 레인은 --after 사슬로 몇 시간 뒤에 뜬다 — 그 사이 주인 지휘가 끝났으면 죽은 주소를 넘기지 않고 주인 없이,
# watch는 살아 있는 지휘(hc-lane coord)에게
fresh
HC_TERMINAL_ID=tq1 run add --name ow8 --project app --brief "$(brief ow8)"
HC_FAKE_COORD_GONE=tq1 run run
t "주인 지휘가 띄우기 전에 사라지면 주인 없이(-) 띄우고 watch는 hc-lane coord 지휘에게, 한 줄 알림" '[ "$(where ow8)" = "up box-1 box-1/ow8" ] && [ "$(cat "$T/owner.log")" = 없음 ] && [ "$(cat "$T/coord.log")" = t8kk ] && [ "$(cat "$T/coordcheck.log")" = tq1 ] && printf "%s" "$out" | grep -Fq "주인 지휘 tq1 가 지금 없다" || { printf "%s\n" "$out"; paste "$T/owner.log" "$T/coord.log"; false; }'

# 10e. 우선순위(2026-11-17 hcfill-priority): 대기열 열셋째 칸 urgent|high|normal|low. run은 선행이 풀린 대기 레인을
#      높은 것부터, 같으면 넣은 차례로. 옛 줄(칸 없음)은 보통. 받은편지 요청은 기본 높음
tail3() { awk -F'\t' -v n="$1" '$1==n {print NF, $12, $13}' "$Q"; }   # 칸 수 · 주인 · 우선
prio_of() { awk -F'\t' -v n="$1" '$1==n {print ($13 == "" ? "-" : $13)}' "$Q"; }
names() { awk 'p {print; p=0} $0=="--name" {p=1}' "$T/lane.log" | uniq | tr '\n' ' '; }   # 띄운 차례(--print-steps 줄과 겹치는 이름은 uniq)
fresh
add n1; add lo --priority 낮음; add h1 --priority high; add u1 --priority 긴급; add n2 --priority normal; add h2 --priority 높음
t "우선순위 add: 한·영 값을 열셋째 칸에 urgent|high|normal|low로, 안 주면 normal" '[ "$(prio_of n1) $(prio_of lo) $(prio_of h1) $(prio_of u1) $(prio_of n2) $(prio_of h2)" = "normal low high urgent normal high" ] || { cat "$Q"; false; }'
run run
t "우선순위 차례: 긴급 > 높음 > 보통 > 낮음, 같은 값은 넣은 차례(가장 높은 것이 첫 기계)" '[ "$(names)" = "u1 h1 h2 n1 n2 lo " ] && [ "$(where u1)" = "up box-1 box-1/u1" ] || { echo "차례: $(names)"; false; }'
run add --name bad --project app --brief "$(brief bad)" --priority 아주
t "우선순위 add: 모르는 값 거절(대기열 없음)" '[ $rc != 0 ] && [ -z "$(where bad)" ]'
run ls
t "ls: 우선 칸(한국어)" '[ $rc = 0 ] && printf "%s\n" "$out" | head -n 1 | grep -q 우선 && printf "%s\n" "$out" | grep -q "^u1 .*긴급" && printf "%s\n" "$out" | grep -q "^lo .*낮음" || { printf "%s\n" "$out"; false; }'

fresh
add hb --priority high --only slow --after nope; add ll --priority low --only slow
run run
t "우선순위: 선행이 안 들어간 높은 것은 건너뛰고 들어간(없는) 낮은 것을 띄움" '[ "$(where hb | cut -d" " -f1)" = wait ] && [ "$(where ll)" = "up slow slow/ll" ] || { printf "%s\n" "$out"; false; }'

fresh
lj slow/x:claude-code:x
add wn --only slow; add wu --only slow --priority urgent
run run
t "긴급: 자리가 없으면 기다림(돌던 것을 끊지 않음)" '[ "$(where wu | cut -d" " -f1)" = wait ] && [ "$(where wn | cut -d" " -f1)" = wait ] && ! grep -q "^kill\|^stop" "$T/hc.log" 2>/dev/null'
lj
run run
t "긴급: 다음 빈자리를 먼저 넣은 보통보다 먼저 잡음" '[ "$(where wu)" = "up slow slow/wu" ] && [ "$(where wn | cut -d" " -f1)" = wait ] || { printf "%s\n" "$out"; false; }'

fresh
mkdir -p "$(dirname "$Q")"; brief od1 >/dev/null; brief od2 >/dev/null
printf 'od1\tapp\t%s\t-\t-\twait\t-\t-\t-\t2026-01-01T00:00\t-\nod2\tapp\t%s\t-\t-\twait\t-\t-\t-\t2026-01-01T00:00\t-\ttq9\n' "$T/od1.md" "$T/od2.md" >"$Q"
awk -F'\t' -v OFS='\t' '{$5="slow"} 1' "$Q" >"$Q.t" && mv "$Q.t" "$Q"   # 둘 다 --only slow(자리 1)
add oh --only slow --priority high
run ls
t "옛 줄(11·12칸): ls에 보통" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -q "^od1 .*보통" && printf "%s\n" "$out" | grep -q "^od2 .*보통" || { printf "%s\n" "$out"; false; }'
run run
t "옛 줄(11·12칸)은 보통: 뒤에 넣은 높음이 먼저" '[ "$(where oh)" = "up slow slow/oh" ] && [ "$(where od1 | cut -d" " -f1)" = wait ] || { printf "%s\n" "$out"; cat "$Q"; false; }'
run prio od2 urgent
t "prio: 12칸 옛 줄도 열셋째 칸에 적고 주인은 그대로" '[ $rc = 0 ] && [ "$(prio_of od2)" = urgent ] && [ "$(awk -F"\t" "\$1==\"od2\" {print \$12}" "$Q")" = tq9 ] || { printf "%s\n" "$out"; cat "$Q"; false; }'
run prio od1 긴급
t "prio: 11칸 옛 줄은 주인 칸을 -로 채우고 열셋째 칸에" '[ $rc = 0 ] && [ "$(tail3 od1)" = "13 - urgent" ] || { printf "%s\n" "$out"; cat "$Q"; false; }'
run prio oh low
t "prio: 떠 있는(대기 아닌) 레인은 거절" '[ $rc = 1 ] && [ "$(prio_of oh)" = high ]'
run prio nope high
t "prio: 없는 레인은 종료 1" '[ $rc = 1 ]'
run prio od1 아주
t "prio: 모르는 값은 거절, 그대로" '[ $rc != 0 ] && [ "$(prio_of od1)" = urgent ]'

fresh
add pn1 --only slow; add pn2 --only slow
run prio pn2 높음
run run
t "prio: 바꾼 레인이 먼저 뜸" '[ "$(where pn2)" = "up slow slow/pn2" ] && [ "$(where pn1 | cut -d" " -f1)" = wait ] || { printf "%s\n" "$out"; false; }'

ifresh
mj box-1 shared slow
req "$IS" iq-d app; req "$IS" iq-l app; printf 'priority\t낮음\n' >>"$IS/iq-l.req"; req "$IS" iq-b app; printf 'priority\t아주\n' >>"$IS/iq-b.req"
run run
t "받은편지: priority 없으면 높음, 적혀 있으면 그 값, 모르는 값은 거절" '[ "$(prio_of iq-d)" = high ] && [ "$(prio_of iq-l)" = low ] && [ -z "$(where iq-b)" ] && grep -Fq "priority" "$IS/rejected/iq-b.why" || { printf "%s\n" "$out"; cat "$Q"; false; }'

# 10f. 먼저 띄움(2026-11-18 hcfill-early 사용자 「노는 워커 없이 … 자동으로 처리 되도록」): 큰 일을 쪼개 대기열이 한두 선행에
# 묶이면 자리가 비어도 띄울 것이 없었다. 띄울 수 있는 것을 다 띄우고도 자리가 남으면 선행에만 묶인 레인을 「먼저 띄움」 절과 함께 띄운다
col14() { awk -F'\t' -v n="$1" '$1==n {print $14}' "$Q"; }
fresh
add e1 --after pa,pc --only slow
run run
t "먼저 띄움: 선행이 안 들어간 레인을 빈 자리에 띄우고 대기열 먼저 칸 early" '[ "$(where e1)" = "up slow slow/e1" ] && [ "$(col14 e1)" = early ] && printf "%s" "$out" | grep -q "먼저 띄움 e1 → slow slow/e1" || { printf "%s\n" "$out"; cat "$Q"; false; }'
rf=$HOME/.local/share/hc-fill/runs/e1-run.md
t "먼저 띄움 절: run 지시문 = 지시문 + 착지 줄 + 먼저 띄움 절(기다림·기한·rebase)" 'grep -Fq "# 지시문 e1" "$rf" && grep -Fq "### 착지 STEPS e1 slow" "$rf" && grep -q "^## 먼저 띄움" "$rf" && grep -Fq "선행 대기 초과" "$rf" && grep -Fq "git rebase origin/main" "$rf" && grep -Fq "선행 레인(pa, pc)" "$rf" || { cat "$rf"; false; }'
t "먼저 띄움 절: 선행마다 판정 줄이 따로(두 선행을 한 줄로 이어 붙이지 않음)" '[ "$(grep -c "grep -Eq" "$rf")" = 2 ] && ! grep "grep -Eq" "$rf" | grep -Eq "pa.*pc|pc.*pa" || { cat "$rf"; false; }'
# 기다림 블록의 조건을 그대로 돌려 본다: 가짜 git(fetch 성공, log는 $T/subjects)
mkdir -p "$T/eg"; printf '#!/bin/sh\ncase $1 in fetch) exit 0 ;; log) cat "$T_LOG/subjects" ;; esac\n' >"$T/eg/git"; chmod +x "$T/eg/git"
cond=$(awk '/^until /{f=1; sub(/^until /, "")} f && /^do /{exit} f' "$rf")
eg() { printf '%s\n' "$@" >"$T/subjects"; PATH="$T/eg:$PATH" sh -c "if $cond
then echo yes; else echo no; fi" 2>&1; }
t "먼저 띄움 기다림: 하나만 들어가면 아직" '[ "$(eg "feat(x): pa 끝")" = no ]'
t "먼저 띄움 기다림: 둘 다 제목 머리에 들어가면 풀림(after_ok와 같은 규칙)" '[ "$(eg "feat(x): pa 끝" "pc: 고침")" = yes ] || { eg "feat(x): pa 끝" "pc: 고침"; printf "%s\n" "$cond"; false; }'
t "먼저 띄움 기다림: 문장 안 언급·이름 앞머리(pa-extra)는 안 셈" '[ "$(eg "feat: pa-extra" "docs: pa 와 pc 언급" "fix(y): 고침 (pc)")" = no ]'
run ls
t "ls: 먼저 띄움(선행 …)" 'printf "%s" "$out" | grep "^e1 " | grep -Fq "먼저 띄움(선행 pa,pc)" || { printf "%s\n" "$out"; false; }'
# 2026-11-19 fill-ls-early-state: 띄울 때 적힌 비고 그대로라 선행이 들어가 본 작업 중인 레인도 「선행을 기다리는」 것처럼 보여
# 지휘가 두 번 틀리게 보고했다. 떠 있는 먼저 띄움 레인은 local 저장소의 지금 있는 origin/<기본>으로(ls는 fetch 안 함) 선행을 본다
eln() { printf "%s\n" "$out" | grep "^e1 "; }
lj slow/e1:claude-code:e1; rm -f "$T/git.log"; run ls
t "ls 먼저 띄움: 떠 있고 선행 모두 아직 → 「선행 기다림 pa,pc」, fetch 안 함" 'eln | grep -Fq "먼저 띄움(선행 기다림 pa,pc)" && ! grep -q fetch "$T/git.log" || { printf "%s\n" "$out"; cat "$T/git.log"; false; }'
echo "aaaa1111 feat(x): pa 끝" >"$T/commits"; run ls
t "ls 먼저 띄움: 일부 → 「선행 기다림 pc · 들어감 pa」" 'eln | grep -Fq "먼저 띄움(선행 기다림 pc · 들어감 pa)" || { printf "%s\n" "$out"; false; }'
echo "bbbb2222 pc: 고침" >>"$T/commits"; rm -f "$T/git.log"; run ls
t "ls 먼저 띄움: 모두 들어감 → 「선행 들어감 — 본 작업」, fetch 안 함" 'eln | grep -Fq "먼저 띄움(선행 들어감 — 본 작업)" && ! grep -q fetch "$T/git.log" || { printf "%s\n" "$out"; cat "$T/git.log"; false; }'
sed -i.bak '/^repo  *app  *here /d' "$T/conf/hc-fill.tsv"; run ls; mv "$T/conf/hc-fill.tsv.bak" "$T/conf/hc-fill.tsv"
t "ls 먼저 띄움: 판정 못 함(local 저장소 줄 없음) → 지금처럼 「선행 pa,pc」" 'eln | grep -Fq "먼저 띄움(선행 pa,pc)" || { printf "%s\n" "$out"; false; }'
export HC_FAKE_LOG_FAIL=1; run ls; unset HC_FAKE_LOG_FAIL
t "ls 먼저 띄움: 판정 못 함(origin/<기본> 없음 — git log 실패) → 「선행 pa,pc」" 'eln | grep -Fq "먼저 띄움(선행 pa,pc)" || { printf "%s\n" "$out"; false; }'
lj; run ls
t "ls 먼저 띄움: 끝난 레인은 선행이 들어갔어도 그대로 「선행 pa,pc」" 'eln | grep "끝남" | grep -Fq "먼저 띄움(선행 pa,pc)" || { printf "%s\n" "$out"; false; }'
fresh
add e1 --after pa --only slow
run run --dry-run
t "먼저 띄움 dry-run: 찍기만" 'printf "%s" "$out" | grep -Fq "[dry-run] 먼저 띄울 e1 → slow" && [ "$(where e1 | cut -d" " -f1)" = wait ] && [ ! -e "$T/lane.log" ] || { printf "%s\n" "$out"; false; }'

fresh
run add --name e2 --project app --brief "$(brief e2)" --after pa --no-early --only slow
t "add --no-early: 대기열 먼저 칸 no-early, 알림에 「먼저 띄움 끔」" '[ $rc = 0 ] && [ "$(col14 e2)" = no-early ] && printf "%s" "$out" | grep -Fq "먼저 띄움 끔"'
run run
t "--no-early: 자리가 비어도 먼저 띄우지 않음" '[ "$(where e2 | cut -d" " -f1)" = wait ] && [ ! -e "$T/lane.log" ] || { printf "%s\n" "$out"; false; }'
run ls
t "ls: --no-early 줄에 「먼저 띄움 끔」" 'printf "%s" "$out" | grep "^e2 " | grep -Fq "먼저 띄움 끔"'

fresh   # 띄울 수 있는 것이 먼저, 먼저 띄움은 우선순위·넣은 차례
add el --after pa --priority low --only slow; add n1 --only slow; add eh --after pa --priority high --only slow
run run
t "먼저 띄움은 띄울 수 있는 것(n1) 다음: slow 자리 1을 n1이 쓰고 먼저 띄움 없음" '[ "$(where n1)" = "up slow slow/n1" ] && [ "$(where eh | cut -d" " -f1)" = wait ] && [ "$(where el | cut -d" " -f1)" = wait ] || { printf "%s\n" "$out"; false; }'
fresh
add el --after pa --priority low --only slow; add eh --after pa --priority high --only slow
run run
t "먼저 띄움 차례: 우선순위 높은 eh가 먼저" '[ "$(where eh)" = "up slow slow/eh" ] && [ "$(where el | cut -d" " -f1)" = wait ] || { printf "%s\n" "$out"; false; }'
fresh
export HC_FAKE_FETCH_FAIL=1; add ef --after pa --only slow; run run; unset HC_FAKE_FETCH_FAIL
t "선행 판정 못 함(fetch 실패)은 먼저 띄우지 않음" '[ "$(where ef | cut -d" " -f1)" = wait ] && [ ! -e "$T/lane.log" ] || { printf "%s\n" "$out"; false; }'

fresh   # 상한: 설정 early = 살아 있는 먼저 띄움 레인 수
sed -i.bak 's/^early$/early 1/' "$T/conf/hc-fill.tsv"
add ea --after pa --only box-1; add eb --after pa --only box-1
run run
t "먼저 띄움 상한 1: 하나만 띄우고 나머지는 대기, 한 줄" '[ "$(where ea)" = "up box-1 box-1/ea" ] && [ "$(where eb | cut -d" " -f1)" = wait ] && printf "%s" "$out" | grep -Fq "먼저 띄움 상한 1개" || { printf "%s\n" "$out"; false; }'
lj box-1/ea:claude-code:ea
run run
t "먼저 띄움 상한: 띄운 것이 살아 있으면 다음 바퀴에도 더 안 띄움" '[ "$(where eb | cut -d" " -f1)" = wait ] || { printf "%s\n" "$out"; false; }'
lj
run run
t "먼저 띄움 상한: 띄운 것이 끝나면 다음 것을 띄움" '[ "$(where eb)" = "up box-1 box-1/eb" ] || { printf "%s\n" "$out"; false; }'
sed -i.bak 's/^early 1$/early 0/' "$T/conf/hc-fill.tsv"
fresh; add ez --after pa --only slow; run run
t "설정 early 0: 먼저 띄움 끔" '[ "$(where ez | cut -d" " -f1)" = wait ] && [ ! -e "$T/lane.log" ]'
cp "$EX" "$T/conf/hc-fill.tsv"

# 감사 [중간] 1: 선행이 대기열에 wait로 남아 있으면 뒤 레인을 먼저 띄우지 않는다(뒤 레인이 자리를 다 먹어 선행이 기한까지 못 뜨는 교착)
fresh   # 시나리오 1: 뒤 레인 b1이 우선순위가 높아도 선행 a1이 먼저 자리를 잡는다
add a1 --after z1 --only slow; add b1 --after a1 --only slow --priority high
run run
t "교착 1: 선행 a1(대기열 wait)을 기다리는 b1은 먼저 띄우지 않고 a1을 먼저 띄움" '[ "$(where a1)" = "up slow slow/a1" ] && [ "$(where b1 | cut -d" " -f1)" = wait ] && printf "%s" "$out" | grep -Fq "먼저 띄움 건너뜀 b1: 선행 a1" || { printf "%s\n" "$out"; cat "$Q"; false; }'
fresh   # 시나리오 2: 선행 a2가 --no-early면 b2도 먼저 띄우지 않고, z2가 들어가면 a2가 뜬다
add a2 --after z2 --only slow --no-early; add b2 --after a2 --only slow
run run
t "교착 2: --no-early 선행 a2가 대기열 wait면 b2를 먼저 띄우지 않음(자리 비움)" '[ "$(where b2 | cut -d" " -f1)" = wait ] && [ "$(where a2 | cut -d" " -f1)" = wait ] && [ ! -e "$T/lane.log" ] || { printf "%s\n" "$out"; false; }'
echo "7777aaaa feat: z2 끝" >"$T/commits"; run run
t "교착 2: z2가 들어가면 a2가 자리를 잡음(b2는 그 뒤)" '[ "$(where a2)" = "up slow slow/a2" ] && [ "$(where b2 | cut -d" " -f1)" = wait ] || { printf "%s\n" "$out"; false; }'
fresh   # 같은 바퀴에 선행이 먼저 띄움으로 떴으면 뒤 레인도 후보(자리 2인 box-1)
add c1 --after z3 --only box-1 --priority high; add d1 --after c1 --only box-1
run run
t "같은 바퀴에 선행 c1이 먼저 띄움으로 뜨면 d1도 먼저 띄움" '[ "$(where c1)" = "up box-1 box-1/c1" ] && [ "$(where d1)" = "up box-1 box-1/d1" ] || { printf "%s\n" "$out"; false; }'

# 감사 [낮음] 6: 먼저 띄운 레인은 선행이 들어간 뒤에도 다시 뜨지 않는다(살아 있을 때·끝난 뒤), 옛 10·12칸 줄은 14칸으로 채움
fresh
mkdir -p "$(dirname "$Q")"; brief o10 >/dev/null; brief o12 >/dev/null
printf 'o10\tapp\t%s\tz4\tslow\twait\t-\t-\t-\t2026-01-01T00:00\n' "$T/o10.md" >"$Q"
printf 'o12\tapp\t%s\tz4\tbox-1\twait\t-\t-\t-\t2026-01-01T00:00\t-\t-\n' "$T/o12.md" >>"$Q"
nf14() { awk -F'\t' -v n="$1" '$1==n {print NF, $12, $13, $14}' "$Q"; }
run run
t "옛 10·12칸 줄도 먼저 띄우고 14칸(우선 normal, 먼저 early)으로 채움" '[ "$(where o10)" = "up slow slow/o10" ] && [ "$(where o12)" = "up box-1 box-1/o12" ] && [ "$(nf14 o10)" = "14 - normal early" ] && [ "$(nf14 o12)" = "14 - normal early" ] || { printf "%s\n" "$out"; cat "$Q"; false; }'
echo "8888bbbb fix: z4 끝" >"$T/commits"; : >"$T/lane.log"
lj slow/o10:claude-code:o10 box-1/o12:claude-code:o12; run run
t "먼저 띄운 레인: 선행이 들어가도(터미널 살아 있음) 다시 안 띄움" '[ "$(launched --name)" = 0 ] || { printf "%s\n" "$out"; false; }'
lj; run run
t "먼저 띄운 레인: 터미널이 끝난 뒤에도 다시 안 띄움" '[ "$(launched --name)" = 0 ] && [ "$(where o10 | cut -d" " -f1)" = up ] || { printf "%s\n" "$out"; false; }'

# 10g. 놀고 있음 알림: 바퀴 끝에도 자리가 남은 기계가 설정 idle분(예시 10) 넘게 이어지면 hc-lane report 사건(watch가 지휘에 보냄)
IT=$HOME/.local/share/hc-fill/idle.tsv
back() { awk -F'\t' -v OFS='\t' -v c="$1" -v d="$2" '{$c -= d} 1' "$IT" >"$IT.x" && mv "$IT.x" "$IT"; }   # back <칸> <초>: 시각을 앞당김
unsent() { awk -F'\t' -v OFS='\t' '{$3 = 0} 1' "$IT" >"$IT.x" && mv "$IT.x" "$IT"; }   # 보낸 적 없는 것으로(받는 쪽 시험을 바퀴마다 다시 보내려고)
fresh
run run
t "놀고 있음: 처음 빈 것은 시각만 적고 알리지 않음(대기열이 비어도 자리를 봄)" '[ $rc = 0 ] && [ ! -e "$T/report.log" ] && grep -q "^box-1	" "$IT" && grep -q "^here	" "$IT" || { printf "%s\n" "$out"; cat "$IT"; false; }'
back 2 660; run run
t "놀고 있음: idle분 넘으면 사건 한 번 — 기계·자리 수·분, 레인 이름 hc-fill-idle, 터미널 밖으로" '[ "$(grep -c . "$T/report.log")" = 1 ] && grep -Fq "report 중간 - box-1 자리 2개 11분째 빔" "$T/report.log" && grep -Fq "here 자리 4개 11분째 빔" "$T/report.log" && grep -Fq "대기열에 띄울 것 없음 --name hc-fill-idle" "$T/report.log" && grep -Fq "(터미널 없음)" "$T/report.log" || { cat "$T/report.log"; printf "%s\n" "$out"; false; }'
t "놀고 있음: 대기 줄이 없으면 모든 지휘(--to 없음)" '! grep -q -- --to "$T/report.log"'
run run
t "놀고 있음: 30분 안에는 다시 안 보냄" '[ "$(grep -c . "$T/report.log")" = 1 ]'
# 2026-11-19 idle-once(사용자 「알림 대기열 빌 때 한 번만 오게 해줘」): 사유가 「대기열에 띄울 것 없음」이면 비기 시작한 뒤 한 번만
back 3 1860; run run
t "놀고 있음(대기열 빔): 계속 비어도 30분 뒤 다시 안 보냄" '[ "$(grep -c . "$T/report.log")" = 1 ] || { cat "$T/report.log"; cat "$IT"; false; }'
back 3 7300; run run
t "놀고 있음(대기열 빔): 2시간 뒤에도 안 보냄" '[ "$(grep -c . "$T/report.log")" = 1 ] || { cat "$T/report.log"; cat "$IT"; false; }'
lj box-1/x1:claude-code:a box-1/x2:claude-code:b; run run
t "놀고 있음: 자리가 차면 그 기계 기록을 지움(다시 비면 처음부터)" '! grep -q "^box-1	" "$IT" && grep -q "^box-2	" "$IT"'
lj; run run; back 2 660; rm -f "$T/report.log"; run run
t "놀고 있음(대기열 빔): 자리가 찼다가 다시 비면 그 기계만 다시 한 번" '[ "$(grep -c . "$T/report.log")" = 1 ] && grep -Fq "box-1 자리 2개 11분째 빔" "$T/report.log" && ! grep -Fq "box-2 자리" "$T/report.log" || { cat "$T/report.log"; cat "$IT"; false; }'
back 3 7300; rm -f "$T/report.log"; run run
t "놀고 있음(대기열 빔): 다시 보낸 뒤로도 한 번만" '[ ! -s "$T/report.log" ] || { cat "$T/report.log"; false; }'
idlek() { awk -F'\t' -v m="$1" '$1==m {print NF, $6}' "$IT"; }   # 칸 수 · 보낸 사유(eval 안의 "{a, b}"는 bash 3.2가 깨뜨려 함수로)
t "idle.tsv: 보낸 줄 여섯째 칸에 보낸 사유 none" '[ "$(idlek box-1)" = "6 none" ] || { cat "$IT"; false; }'
awk -F'\t' -v OFS='\t' '{NF = 5} 1' "$IT" >"$IT.x" && mv "$IT.x" "$IT"; back 3 7300; rm -f "$T/report.log"; run run
t "idle.tsv: 옛 5칸 줄(보낸 적 있음)도 대기열 빔을 다시 안 보냄, 6칸으로 채움" '[ ! -s "$T/report.log" ] && [ "$(idlek box-1)" = "6 none" ] || { cat "$T/report.log"; cat "$IT"; false; }'
# 2026-11-19 hcfill-idle-seats: 자리가 하나라도 차면(빈 자리 수가 줄면) 그 순간부터 다시 셈 — 레인이 떠 있는 기계를 새벽부터 553분째 빔으로 셌다
fresh
run run; back 2 660; lj box-1/y1:claude-code:a; rm -f "$T/report.log"; run run
t "놀고 있음: 빈 자리 수가 줄면(레인이 뜸) 처음부터 다시 셈 — 그 바퀴엔 알리지 않음" '! grep -Fq "box-1 자리" "$T/report.log" && grep -Fq "box-2 자리 2개 11분째 빔" "$T/report.log" && [ "$(awk -F"\t" "\$1==\"box-1\" {print \$5}" "$IT")" = 1 ] || { cat "$T/report.log"; cat "$IT"; printf "%s\n" "$out"; false; }'
back 2 660; rm -f "$T/report.log"; run run
t "놀고 있음: 다시 센 뒤 idle분 넘으면 그때 알림(box-1 자리 1개 11분째)" 'grep -Fq "box-1 자리 1개 11분째 빔" "$T/report.log" || { cat "$T/report.log"; cat "$IT"; false; }'
s1=$(awk -F'\t' '$1=="box-1" {print $2}' "$IT"); lj; run run
t "놀고 있음: 빈 자리 수가 늘면(레인이 끝남) 시작 시각은 그대로" '[ "$(awk -F"\t" "\$1==\"box-1\" {print \$2}" "$IT")" = "$s1" ] && [ "$(awk -F"\t" "\$1==\"box-1\" {print \$5}" "$IT")" = 2 ] || { cat "$IT"; false; }'
fresh
add w1 --after pa --no-early --only slow --coord tq1; add w2 --after pa --no-early --only slow --coord tq2
run run; back 2 700; rm -f "$T/report.log"; run run
t "놀고 있음: 대기 줄 주인 지휘들에게만(--to tq1,tq2)" 'grep -Fq -- "--to tq1,tq2" "$T/report.log" || { cat "$T/report.log"; false; }'
export HC_FAKE_COORD_GONE=tq1; unsent; rm -f "$T/report.log"; run run
t "놀고 있음: 사라진 주인은 빼고(--to tq2)" 'grep -Fq -- "--to tq2 " "$T/report.log" || { cat "$T/report.log"; false; }'
export HC_FAKE_COORD_GONE="tq1 tq2"; unsent; rm -f "$T/report.log"; run run
t "놀고 있음: 살아 있는 주인이 없으면 모든 지휘" '[ -s "$T/report.log" ] && ! grep -q -- --to "$T/report.log" || { cat "$T/report.log"; false; }'
unset HC_FAKE_COORD_GONE
add w3 --after pa --no-early --only slow; unsent; rm -f "$T/report.log"; run run
t "놀고 있음: 주인 없는 대기 줄이 있으면 모든 지휘" '[ -s "$T/report.log" ] && ! grep -q -- --to "$T/report.log" || { cat "$T/report.log"; false; }'
export HC_FAKE_REPORT_RC=1; unsent; rm -f "$T/report.log"; run run
t "놀고 있음: 보내기(report) 실패면 한 줄, 보낸 시각을 안 적어 다음 바퀴에 다시" 'printf "%s" "$out" | grep -Fq "놀고 있음 알림 실패" && unset HC_FAKE_REPORT_RC && run run && [ "$(grep -c . "$T/report.log")" = 2 ] || { cat "$T/report.log"; false; }'
unset HC_FAKE_REPORT_RC
fresh   # 감사 [낮음] 3: 세 번 보낸 뒤로는 2시간마다 — 되풀이는 사유가 먼저 띄움 상한일 때만(idle-once). ca가 살아 있어 바퀴마다 상한에 걸림
sed -i.bak 's/^early$/early 1/' "$T/conf/hc-fill.tsv"; add ca --after pa --only box-2; add cb --after pa --only box-2
run run; lj box-2/ca:claude-code:ca; back 2 700; run run; back 3 1900; run run; back 3 1900; run run
t "되풀이 상한: 30분마다 세 번" '[ "$(grep -c "먼저 띄움 상한" "$T/report.log")" = 3 ] || { cat "$T/report.log"; cat "$IT"; false; }'
back 3 1900; run run
t "되풀이 상한: 세 번 뒤로는 30분 지나도 안 보냄" '[ "$(grep -c "먼저 띄움 상한" "$T/report.log")" = 3 ]'
back 3 7300; run run
t "되풀이 상한: 2시간 지나면 다시" '[ "$(grep -c "먼저 띄움 상한" "$T/report.log")" = 4 ] || { cat "$IT"; false; }'
t "되풀이 상한: 넷 다 먼저 띄움 상한 사유" '[ "$(grep -c "먼저 띄움 상한(early 1)에 걸려 남김" "$T/report.log")" = 4 ] || { cat "$T/report.log"; false; }'
# 아스트라 2차 낮음 1: 상한 사유는 상한에 걸린 후보를 받을 기계(cb --only box-2)에만 — 다른 기계는 「띄울 것 없음」(한 번)
t "되풀이 상한: 상한 사유는 그 후보를 받을 box-2에만, box-1은 「대기열에 띄울 것 없음」 한 번" '! grep "먼저 띄움 상한" "$T/report.log" | grep -Fq "box-1 자리" && [ "$(grep "대기열에 띄울 것 없음" "$T/report.log" | grep -c "box-1 자리")" = 1 ] || { cat "$T/report.log"; false; }'
cp "$EX" "$T/conf/hc-fill.tsv"
fresh   # 사유: 먼저 띄움 상한에 걸려 남긴 자리면 그렇게
sed -i.bak 's/^early$/early 0/' "$T/conf/hc-fill.tsv"
run run; back 2 700; run run
t "사유: 먼저 띄움 상한에 안 걸렸으면 「대기열에 띄울 것 없음」" 'grep -Fq "대기열에 띄울 것 없음" "$T/report.log"'
sed -i.bak 's/^early 0$/early 1/' "$T/conf/hc-fill.tsv"
add ca --after pa --only box-1; add cb --after pa --only box-2; rm -f "$T/report.log"; run run   # 상한 사유는 cb를 받을 box-2에(2차 낮음 1)
t "사유: 대기열 빔으로 한 번 보낸 뒤 상한으로 바뀌면 30분 안이어도 그 사유로 다시 — 「먼저 띄움 상한(early 1)에 걸려 남김」" 'grep -Fq "먼저 띄움 상한(early 1)에 걸려 남김" "$T/report.log" && ! grep -Fq "대기열에 띄울 것 없음" "$T/report.log" || { cat "$T/report.log"; printf "%s\n" "$out"; false; }'
capn() { awk -F'\t' '$6=="cap" {n++; if ($4 != 1) bad++} END {print n + 0, bad + 0}' "$IT"; }   # 상한 사유 줄 수 · 보낸 횟수가 1이 아닌 줄 수
t "사유가 바뀌어 보내면 보낸 횟수는 1부터(감사 낮음 1 — 상한 사유도 30분 간격 세 번을 다 받게)" '[ "$(capn | cut -d" " -f2)" = 0 ] && [ "$(capn | cut -d" " -f1)" -ge 1 ] || { cat "$IT"; false; }'
cp "$EX" "$T/conf/hc-fill.tsv"
fresh; run run --dry-run
t "놀고 있음: dry-run은 기록·알림 없음" '[ ! -e "$IT" ] && [ ! -e "$T/report.log" ]'

# 10h. 자동 감사(2026-11-19 fill-auto-audit · 2026-11-20 fill-handoff-integrity): 쓰기 레인의 「완료 <sha>」 → 그 실행·그 커밋(40자)에 묶인
#      감사 차수(ID a…) → 긴급 감사 줄 audit-<ID> → 판정(6칸 40자 sha, 「감사 완료 <ID> …」)을 레인 기계 verdicts/<레인>/<ID>.md에 두고 깨움 →
#      레인이 hc-lane verdict로 수거(<ID>.ack). 가짜 events는 $T/events, 레인 실행 = <터미널>@<가짜 hc get created_ms>
AT=$HOME/.local/share/hc-fill/audit.tsv
LT=$HOME/.local/state/hc-lane/lanes.tsv   # hc-lane 레인 기록(가짜 hc-lane은 안 씀 — 시험이 적는다)
VD=$HOME/.local/share/hc-fill/verdicts
afresh() { fresh; rm -f "$T/events" "$T/events.log" "$T/send.log" "$LT"; cp "$EX" "$T/conf/hc-fill.tsv"; echo "audit    $EXA" >>"$T/conf/hc-fill.tsv"; ek=0; unset HC_FAKE_CREATED HC_FAKE_REVNONE HC_FAKE_SHA40 HC_FILL_CRASH_AT; }
ev() { ek=$((ek + 1)); printf '2026-11-%02dT%02d:%02d:00Z\tm\t%s\t%s\t%s\t%s\t%s\t1\n' $((19 + ek / 1440)) $((ek / 60 % 24)) $((ek % 60)) "$@" >>"$T/events"; }   # ev <터미널> <이름> <상태> <sha> <한 줄>
ev10() { ek=$((ek + 1)); printf '2026-11-%02dT%02d:%02d:00Z\tm\t%s\t%s\t%s\t%s\t%s\t1\t-\t%s\n' $((19 + ek / 1440)) $((ek / 60 % 24)) $((ek % 60)) "$@" >>"$T/events"; }   # … <발행 실행>(10칸)
col15() { awk -F'\t' -v n="$1" '$1==n {print $15}' "$Q"; }
s40() { printf '%s%0*d' "$1" $((40 - ${#1})) 0; }   # 가짜 git이 푸는 40자
aidof() { awk -F'\t' -v n="$1" 'NF >= 17 && $2==n {a=$1} END {print a}' "$AT" 2>/dev/null; }   # 그 레인의 마지막 차수
acol() { awk -F'\t' -v a="$1" -v c="$2" 'NF >= 17 && $1==a {v=$c} END {print v}' "$AT" 2>/dev/null; }
verd() { ev "$1" "audit-$2" 판정 "$(acol "$2" 5)" "감사 완료 $2 높음 $3·중간 $4·낮음 ${5:-0}"; }   # verd <감사 터미널> <ID> <높음> <중간> [<낮음>]
vkey() { awk -F'\t' -v k="$2" '$1==k {print $2}' "$VD/$1/$3.md" 2>/dev/null; }   # vkey <레인> <키> <ID>
nrep() { if [ -f "$T/report.log" ]; then grep -cF -- "$1" "$T/report.log" || true; else echo 0; fi; }   # 그 글이 든 사건 수
ABR=$HOME/.local/share/hc-fill/briefs/audit
mkdir -p "$W/app.wt/w1" "$W/app.wt/w2"

afresh
add w1 --only slow
run run
rf=$HOME/.local/share/hc-fill/runs/w1-run.md
t "자동 감사: 쓰기 레인 run 지시문 끝에 「감사·착지 (hc-fill 자동)」 절(완료 보고 · hc-lane verdict <ID> · land --verdict), 감사 칸 auto" '[ "$(where w1)" = "up slow slow/w1" ] && [ "$(col15 w1)" = auto ] && grep -Fq "## 감사·착지 (hc-fill 자동)" "$rf" && grep -Fq "hc-lane report 완료 <sha>" "$rf" && grep -Fq "hc-lane verdict <ID>" "$rf" && grep -Fq "hc-lane land <기본> --verdict <ID>" "$rf" || { printf "%s\n" "$out"; cat "$rf"; false; }'
t "자동 감사: 완료 사건 전에는 감사 줄 없음" '[ -z "$(awk -F"\t" "\$15 ~ /^audit=/" "$Q")" ] && [ ! -s "$AT" ]'
lj "slow/w1:claude-code:w1:done:$W/app.wt/w1"
ev slow/w1 w1 완료 abc1234 "끝"; ev slow/w1 w1 중간 - "완료 뒤 중간 보고"; ev - w1 막힘 - "hc-fill이 남긴 것(다른 터미널)"
run run
A1=$(aidof w1)
t "완료 뒤 중간 보고가 끼어도 그 실행의 최신 완료로 차수를 연다 → 긴급 감사 줄 audit-<ID>(주인·우선·감사 칸), 같은 바퀴에 띄움" 'case $A1 in a*-??????) true ;; *) false ;; esac && [ "$(where "audit-$A1")" = "up slow slow/audit-$A1" ] && [ "$(col15 "audit-$A1")" = audit=w1 ] && [ "$(prio_of "audit-$A1")" = urgent ] && printf "%s" "$out" | grep -Fq "감사 넣음 audit-$A1 ← w1 abc12340 (1번째, slow)" || { printf "%s\n" "$out"; cat "$Q" "$AT"; false; }'
t "감사 줄 띄우기: hc-lane --read, --repo는 그 레인 작업 트리, events는 그 기계 --on" 'grep -Fxq -- --read "$T/lane.log" && grep -Fxq /Users/other/src/app.wt/w1 "$T/lane.log" && grep -Fq "events --on slow" "$T/events.log" || { cat "$T/lane.log"; false; }'
ab=$ABR/$A1.md
t "감사 지시문에 재현 프로세스의 시간 제한·종료·회수 규칙을 넣음" 'grep -Fq "trap" "$ab" && grep -Fq "wait" "$ab" && grep -Fq "시간 제한" "$ab"'
t "감사 지시문 briefs/audit/<ID>.md: 틀 자리를 다 채움(40자 sha·차수 ID·보고서 reports/audit-<ID>.md·원 지시문)" 'grep -Fq "git -C /Users/other/src/app.wt/w1 diff origin/main...$(s40 abc1234)" "$ab" && grep -Fq "~/.local/share/hc-fill/runs/w1-run.md" "$ab" && grep -Fq "~/.local/share/hc-fill/reports/audit-$A1.md" "$ab" && grep -Fq "hc-lane report 판정 $(s40 abc1234) \"감사 완료 $A1 높음" "$ab" && ! grep -q "{[a-z_]*}" "$ab" || { cat "$ab"; false; }'
t "감사 줄의 run 지시문에는 감사·착지 절 없음(읽기 레인), 차수마다 따로" '[ -f "$HOME/.local/share/hc-fill/runs/audit-$A1-run.md" ] && ! grep -Fq "감사·착지 (hc-fill 자동)" "$HOME/.local/share/hc-fill/runs/audit-$A1-run.md"'
t "자리: 감사에 넘겨진 쓰기 레인(done)은 안 셈 — 자리 1인 slow에 감사가 뜸" 'printf "%s" "$out" | grep -Fq "자리 slow slow/w1 w1: done, 자동 감사 기다림(자리는 감사 줄이) → 안 셈"'
t "audit.tsv 차수 줄: 레인 차수 부모실행(<터미널>@<created_ms>) 40자 sha 기계 부모주소 queued" '[ "$(acol "$A1" 2) $(acol "$A1" 3) $(acol "$A1" 4) $(acol "$A1" 5) $(acol "$A1" 6) $(acol "$A1" 7) $(acol "$A1" 8)" = "w1 1 w1@1700000000000 $(s40 abc1234) slow slow/w1 queued" ] || { cat "$AT"; false; }'
run ls
t "ls: 감사 중 · 감사 대상" 'printf "%s\n" "$out" | grep "^w1 " | grep -Fq "감사 중(1번째)" && printf "%s\n" "$out" | grep "^audit-$A1 " | grep -Fq "감사 대상 w1" || { printf "%s\n" "$out"; false; }'
: >"$T/lane.log"; lj "slow/w1:claude-code:w1:done:$W/app.wt/w1" "slow/audit-$A1:claude-code:audit-$A1:working:$W/app.wt/w1"
run run
t "같은 실행·같은 sha는 두 번 열지 않음" '! grep -Fxq -- --name "$T/lane.log" && ! printf "%s" "$out" | grep -Fq "감사 넣음" && [ "$(awk -F"\t" "NF >= 17" "$AT" | grep -c .)" = 1 ]'
# 옛 꼴 판정(차수 ID 없음·6칸 -)과 다른 차수 ID 판정은 받지 않는다
lj "slow/w1:claude-code:w1:done:$W/app.wt/w1" "slow/audit-$A1:claude-code:audit-$A1:working:$W/app.wt/w1"
ev "slow/audit-$A1" "audit-$A1" 판정 - "감사 완료 abc1234 높음 0·중간 0·낮음 2"
ev "slow/audit-$A1" "audit-$A1" 판정 "$(s40 abc1234)" "감사 완료 azzzzzz-zzzzzz 높음 0·중간 0·낮음 2"
run run
t "옛 꼴 판정·다른 차수 ID 판정은 받지 않음(판정 파일·깨움 없음)" '[ ! -e "$VD/w1/$A1.md" ] && [ ! -e "$T/send.log" ] && [ "$(acol "$A1" 8)" = queued ]'
verd "slow/audit-$A1" "$A1" 0 0 2
lj "slow/w1:claude-code:w1:done:$W/app.wt/w1" "slow/audit-$A1:claude-code:audit-$A1:done:$W/app.wt/w1"
run run
t "판정 받음(통과) → 레인 기계 verdicts/w1/<ID>.md(aid·run·40자 sha·verdict·report), 깨움(hc-lane verdict <ID>), 상태 deliver" '[ "$(vkey w1 aid "$A1")" = "$A1" ] && [ "$(vkey w1 run "$A1")" = w1@1700000000000 ] && [ "$(vkey w1 sha "$A1")" = "$(s40 abc1234)" ] && [ "$(vkey w1 verdict "$A1")" = pass ] && [ "$(vkey w1 report "$A1")" = "~/.local/share/hc-fill/reports/audit-$A1.md" ] && grep -Fq "send slow/w1 hc-fill: 감사 판정이 왔다 — 이 작업 트리에서 hc-lane verdict $A1 를 돌리고" "$T/send.log" && [ "$(acol "$A1" 8)" = deliver ] && ! grep -Fq "감사 통과 (" "$T/send.log" || { printf "%s\n" "$out"; cat "$T/send.log" "$AT"; false; }'
run run
t "수거 전 다시 깨움은 wake_every 뒤에만(이번 바퀴 깨움 없음), 판정 파일은 그대로(ALREADY)" '[ "$(grep -c "hc-lane verdict $A1" "$T/send.log")" = 1 ] && [ "$(acol "$A1" 8)" = deliver ] || { printf "%s\n" "$out"; false; }'
run ls
t "ls: 감사 통과 — 수거 기다림" 'printf "%s\n" "$out" | grep "^w1 " | grep -Fq "감사 통과 — 수거 기다림"'
printf 'landing\t2026-11-20T00:00:00Z\tw1@1700000000000\tp\n' >"$VD/w1/$A1.ack"
run run
t "레인이 수거(<ID>.ack landing) → 차수 taken, 감사 터미널 정리 주소를 지휘에 한 번(키 hf:<ID>:cleanup), 감사 줄 뺌, 끄지 않음" '[ "$(acol "$A1" 8)" = taken ] && grep -Fq -- "--key hf:$A1:cleanup" "$T/report.log" && grep -Fq "감사 터미널 slow/audit-$A1 정리 필요" "$T/report.log" && [ -z "$(where "audit-$A1")" ] && ! grep -q "^kill" "$T/hc.log" 2>/dev/null || { printf "%s\n" "$out"; cat "$T/report.log"; false; }'
run run
t "끝난 차수는 다시 안 다룸(정리 사건 한 번), ls 「감사 통과 받음」" '[ "$(nrep "hf:$A1:cleanup")" = 1 ] && "$FILL" ls 2>&1 | grep "^w1 " | grep -Fq "감사 통과 받음"'

# 지적 → 수거(taken) → 고침 → 새 차수(2번째, 지적수 이어 감) → 지적 → 상한이면 주인 지휘에(키 hf:<ID>:limit)
afresh
"$FILL" add --name w2 --project app --brief "$(brief w2)" --only box-1 --coord tq1 >/dev/null 2>&1
run run
lj "box-1/w2:claude-code:w2:done:$W/app.wt/w2"; ev box-1/w2 w2 완료 aaaa111 "1차"
run run; B1=$(aidof w2)
t "주인 지휘가 있는 레인: 감사 줄 주인도 그 지휘" '[ "$(awk -F"\t" -v n="audit-$B1" "\$1==n {print \$12}" "$Q")" = tq1 ]'
verd "box-1/audit-$B1" "$B1" 1 0 0
lj "box-1/w2:claude-code:w2:done:$W/app.wt/w2" "box-1/audit-$B1:claude-code:audit-$B1:done:$W/app.wt/w2"
run run
t "지적 → 판정 파일 verdict fix·지적수 1, ls 「감사 지적 1번째 — 수거 기다림」" '[ "$(vkey w2 verdict "$B1")" = fix ] && [ "$(acol "$B1" 9)" = 1 ] && "$FILL" ls 2>&1 | grep "^w2 " | grep -Fq "감사 지적 1번째 — 수거 기다림" || { printf "%s\n" "$out"; cat "$AT"; false; }'
printf 'taken\t2026-11-20T00:00:00Z\tw2@1700000000000\n' >"$VD/w2/$B1.ack"; run run
t "지적 수거(taken) → 차수 taken" '[ "$(acol "$B1" 8)" = taken ]'
: >"$T/lane.log"; ev box-1/w2 w2 완료 bbbb222 "고침"; lj "box-1/w2:claude-code:w2:done:$W/app.wt/w2"
run run; B2=$(aidof w2)
t "새 sha 완료 → 새 차수(다른 ID, 2번째, 지적수 1 이어 감), 새 감사 줄" '[ "$B2" != "$B1" ] && [ "$(acol "$B2" 3)" = 2 ] && [ "$(acol "$B2" 9)" = 1 ] && [ "$(where "audit-$B2" | cut -d" " -f1)" = up ] && grep -Fq "$(s40 bbbb222)" "$ABR/$B2.md" || { printf "%s\n" "$out"; cat "$Q" "$AT"; false; }'
verd "box-1/audit-$B1" "$B1" 0 0 0   # 지난 차수의 판정이 늦게 와도
lj "box-1/w2:claude-code:w2:done:$W/app.wt/w2" "box-1/audit-$B2:claude-code:audit-$B2:done:$W/app.wt/w2"
verd "box-1/audit-$B2" "$B2" 0 1 3
run run
t "2번째 지적(지적수 1 < audit_max 2) → 지적수 2, 지난 차수 판정은 안 씀" '[ "$(acol "$B2" 9)" = 2 ] && [ "$(vkey w2 verdict "$B2")" = fix ] && [ "$(acol "$B1" 8)" = taken ] || { printf "%s\n" "$out"; cat "$AT"; false; }'
printf 'taken\tx\tw2@1700000000000\n' >"$VD/w2/$B2.ack"; run run
ev box-1/w2 w2 완료 cccc333 "또 고침"; lj "box-1/w2:claude-code:w2:done:$W/app.wt/w2"; run run; B3=$(aidof w2)
lj "box-1/w2:claude-code:w2:done:$W/app.wt/w2" "box-1/audit-$B3:claude-code:audit-$B3:done:$W/app.wt/w2"
verd "box-1/audit-$B3" "$B3" 2 0 0; run run
t "상한(지적 2번) → 주인 지휘에 hc-lane report 막힘 「<레인> 감사 3번째 … 지휘 판단」(--to 주인, 키 hf:<ID>:limit), 판정 파일 없음" 'grep -Fq "report 막힘 - w2 감사 3번째 높음 2·중간 0 — 지휘 판단" "$T/report.log" && grep -Fq -- "--name w2 --to tq1 --key hf:$B3:limit" "$T/report.log" && [ ! -e "$VD/w2/$B3.md" ] && [ "$(acol "$B3" 8)" = coord ] || { printf "%s\n" "$out"; cat "$T/report.log"; false; }'
run run
t "지휘로 넘긴 뒤 되풀이 없음, ls 「지휘로 넘김」" '[ "$(nrep "hf:$B3:limit")" = 1 ] && "$FILL" ls 2>&1 | grep "^w2 " | grep -Fq "지휘로 넘김"'

# 판정 줄을 못 읽으면 지휘로(주인 없음 → --to 없이 모든 지휘)
afresh; add w3 --only box-1; run run
lj "box-1/w3:claude-code:w3:done:/"; ev box-1/w3 w3 완료 dddd444 "끝"; run run; C1=$(aidof w3)
ev "box-1/audit-$C1" "audit-$C1" 판정 "$(s40 dddd444)" "감사 완료 $C1 끝 — 괜찮음"; run run
t "판정 줄을 못 읽음 → 모든 지휘에 막힘(--to 없음, 키 hf:<ID>:unreadable)" 'grep -Fq "report 막힘 - w3 감사 1번째 판정 줄을 못 읽음" "$T/report.log" && ! grep -q -- --to "$T/report.log" && grep -Fq -- "--key hf:$C1:unreadable" "$T/report.log" && [ ! -e "$T/send.log" ] || { printf "%s\n" "$out"; cat "$T/report.log"; false; }'

# 부모 실행: 같은 주소의 다른 실행(created_ms 다름)·없어진 부모의 판정은 받지 않고 지휘로
afresh; add p1 --only box-1; run run
lj "box-1/p1:claude-code:p1:done:/"; ev box-1/p1 p1 완료 1212aaa "끝"; run run; P1=$(aidof p1)
verd "box-1/audit-$P1" "$P1" 0 0 0; lj "box-1/p1:claude-code:p1:done:/" "box-1/audit-$P1:claude-code:audit-$P1:done:/"
export HC_FAKE_CREATED="box-1/p1=1700000999000"; run run; unset HC_FAKE_CREATED
t "부모 주소의 created_ms가 다름(주소 재사용) → 판정 안 씀, 지휘에 한 번(hf:<ID>:parent-ended)" '[ ! -e "$VD/p1/$P1.md" ] && [ "$(nrep "hf:$P1:parent-ended")" = 1 ] && [ "$(acol "$P1" 8)" = coord ] || { printf "%s\n" "$out"; cat "$AT"; false; }'
afresh; add p2 --only box-1; run run
lj "box-1/p2:claude-code:p2:done:/"; ev box-1/p2 p2 완료 1313bbb "끝"; run run; P2=$(aidof p2)
verd "box-1/audit-$P2" "$P2" 0 0 0; lj "box-1/audit-$P2:claude-code:audit-$P2:done:/"
export HC_FAKE_GONE=box-1/p2; run run; unset HC_FAKE_GONE
t "부모 터미널이 없어짐(not_found) → 판정 안 씀, 지휘에 한 번" '[ ! -e "$VD/p2/$P2.md" ] && [ "$(nrep "hf:$P2:parent-ended")" = 1 ]'
# 10칸 발행 실행이 다른 완료(같은 주소의 옛 실행)는 이 실행의 완료로 보지 않는다
afresh; add p3 --only box-1; run run
lj "box-1/p3:claude-code:p3:done:/"; ev10 box-1/p3 p3 완료 1414ccc "옛 실행" p3@1600000000000; run run
t "10칸 발행 실행이 지금 실행이 아닌 완료로는 차수를 열지 않음" '[ -z "$(aidof p3)" ]'
ev10 box-1/p3 p3 완료 1515ddd "이 실행" p3@1700000000000; run run
t "10칸이 지금 실행이면 연다" '[ "$(acol "$(aidof p3)" 5)" = "$(s40 1515ddd)" ]'

# S1 완료 → 감사 → S2 완료 → 중간 → S1 판정: 지난 차수는 superseded(지휘 알림 없음), S2 차수가 열린다
afresh; add sp --only box-1; run run
lj "box-1/sp:claude-code:sp:done:/"; ev box-1/sp sp 완료 2020aaa "S1"; run run; S1=$(aidof sp)
lj "box-1/sp:claude-code:sp:done:/" "box-1/audit-$S1:claude-code:audit-$S1:done:/"
ev box-1/sp sp 완료 2121bbb "S2"; ev box-1/sp sp 중간 - "중간"; verd "box-1/audit-$S1" "$S1" 0 0 0
run run; S2=$(aidof sp)
t "S1 판정이 S2 완료 뒤에 와도: S1 차수 superseded(판정 파일·지휘 사건 없음), S2 차수 열림(2번째)" '[ "$(acol "$S1" 8)" = superseded ] && [ ! -e "$VD/sp/$S1.md" ] && [ ! -s "$T/report.log" -o "$(nrep "hf:$S1:")" = 0 ] && [ "$S2" != "$S1" ] && [ "$(acol "$S2" 5)" = "$(s40 2121bbb)" ] && [ "$(acol "$S2" 3)" = 2 ] || { printf "%s\n" "$out"; cat "$AT"; false; }'
# 지난 감사가 아직 돌면 새 완료는 그 감사가 끝난 뒤에(감사 둘이 함께 안 돎)
afresh; add w6 --only box-1; run run
lj "box-1/w6:claude-code:w6:done:/"; ev box-1/w6 w6 완료 2222bbb "끝"; run run; G1=$(aidof w6)
lj "box-1/w6:claude-code:w6:done:/" "box-1/audit-$G1:claude-code:audit-$G1:working:/"; ev box-1/w6 w6 완료 3333ccc "더 고침"; run run
t "지난 감사가 아직 돌면 새 완료는 미룸" 'printf "%s" "$out" | grep -Fq "자동 감사 미룸 w6: 새 완료 3333ccc — 지난 감사($G1 2222bbb0)가 아직 돈다" && [ "$(aidof w6)" = "$G1" ] && [ "$(acol "$G1" 8)" = queued ] || { printf "%s\n" "$out"; false; }'

# 넘기기 멱등성: 깨우기 실패·판정 파일 쓰기 전후 중단·다른 내용·수거 전 새 완료·수거와 취소 경합·수거 기다림 상한
afresh; add d1 --only box-1; run run
lj "box-1/d1:claude-code:d1:done:/"; ev box-1/d1 d1 완료 3030aaa "끝"; run run; D1=$(aidof d1)
verd "box-1/audit-$D1" "$D1" 0 0 0; lj "box-1/d1:claude-code:d1:done:/" "box-1/audit-$D1:claude-code:audit-$D1:done:/"
export HC_FAKE_SEND_FAIL=box-1/d1; run run; unset HC_FAKE_SEND_FAIL
t "깨우기 hc send 실패: 판정 파일은 있음(영속 지시), 상태 deliver, 「깨우기 실패」" '[ -f "$VD/d1/$D1.md" ] && [ "$(acol "$D1" 8)" = deliver ] && printf "%s" "$out" | grep -Fq "깨우기 실패 d1 box-1/d1" || { printf "%s\n" "$out"; false; }'
run run
t "다음 바퀴에 다시 깨움(지난번 깨움이 실패라 기한 없음)" '[ "$(grep -c "hc-lane verdict $D1" "$T/send.log")" = 2 ] && [ "$(acol "$D1" 11)" = 1 ]'
afresh; add d2 --only box-1; run run
lj "box-1/d2:claude-code:d2:done:/"; ev box-1/d2 d2 완료 3131bbb "끝"; run run; D2=$(aidof d2)
verd "box-1/audit-$D2" "$D2" 0 0 0; lj "box-1/d2:claude-code:d2:done:/" "box-1/audit-$D2:claude-code:audit-$D2:done:/"
HC_FILL_CRASH_AT=after-consume run run
t "중단 after-consume: 판정을 기록(deliver)한 뒤 판정 파일 전에 죽음" 'grep -Fxq after-consume "$HOME/.local/share/hc-fill/crash.log" && [ "$(acol "$D2" 8)" = deliver ] && [ ! -e "$VD/d2/$D2.md" ]'
run run
t "다음 바퀴: 기록에서 판정 파일을 만들고 깨움(한 번)" '[ "$(vkey d2 verdict "$D2")" = pass ] && [ "$(grep -c "hc-lane verdict $D2" "$T/send.log")" = 1 ]'
afresh; add d3 --only box-1; run run
lj "box-1/d3:claude-code:d3:done:/"; ev box-1/d3 d3 완료 3232ccc "끝"; run run; D3=$(aidof d3)
verd "box-1/audit-$D3" "$D3" 0 0 0; lj "box-1/d3:claude-code:d3:done:/" "box-1/audit-$D3:claude-code:audit-$D3:done:/"
HC_FILL_CRASH_AT=after-verdict-file run run; i0=$(ino "$VD/d3/$D3.md"); run run
t "중단 after-verdict-file: 다음 바퀴는 같은 내용(ALREADY) — 파일 그대로, 깨움 한 번" '[ "$(ino "$VD/d3/$D3.md")" = "$i0" ] && [ "$(grep -c "hc-lane verdict $D3" "$T/send.log")" = 1 ] && [ "$(acol "$D3" 8)" = deliver ] || { printf "%s\n" "$out"; false; }'
afresh; add d4 --only box-1; run run
lj "box-1/d4:claude-code:d4:done:/"; ev box-1/d4 d4 완료 3333ddd "끝"; run run; D4=$(aidof d4)
mkdir -p "$VD/d4"; printf 'aid\t%s\nverdict\tfix\n' "$D4" >"$VD/d4/$D4.md"
verd "box-1/audit-$D4" "$D4" 0 0 0; lj "box-1/d4:claude-code:d4:done:/" "box-1/audit-$D4:claude-code:audit-$D4:done:/"; run run
t "판정 파일 자리에 다른 내용 → 덮지 않고 지휘에(hf:<ID>:conflict), 깨우지 않음" 'grep -q "verdict	fix" "$VD/d4/$D4.md" && [ "$(nrep "hf:$D4:conflict")" = 1 ] && [ ! -e "$T/send.log" ]'
afresh; add d5 --only box-1; run run
lj "box-1/d5:claude-code:d5:done:/"; ev box-1/d5 d5 완료 3434eee "끝"; run run; D5=$(aidof d5)
verd "box-1/audit-$D5" "$D5" 0 1 0; lj "box-1/d5:claude-code:d5:done:/" "box-1/audit-$D5:claude-code:audit-$D5:done:/"; run run
ev box-1/d5 d5 완료 3535fff "수거 전에 새 커밋"; run run; D5b=$(aidof d5)
t "수거 전 새 완료 → <ID>.ack에 void를 먼저 잡고 그 차수는 superseded, 새 차수" '[ "$(cut -f 1 "$VD/d5/$D5.ack")" = void ] && [ "$(acol "$D5" 8)" = superseded ] && [ "$D5b" != "$D5" ] && [ "$(acol "$D5b" 5)" = "$(s40 3535fff)" ] || { printf "%s\n" "$out"; cat "$AT"; false; }'
afresh; add d6 --only box-1; run run
lj "box-1/d6:claude-code:d6:done:/"; ev box-1/d6 d6 완료 3636aaa "끝"; run run; D6=$(aidof d6)
verd "box-1/audit-$D6" "$D6" 0 1 0; lj "box-1/d6:claude-code:d6:done:/" "box-1/audit-$D6:claude-code:audit-$D6:done:/"; run run
printf 'taken\tx\td6@1700000000000\n' >"$VD/d6/$D6.ack"; ev box-1/d6 d6 완료 3737bbb "수거 뒤 고침"; run run
t "레인이 먼저 수거(taken)했으면 void는 못 잡고 그 차수는 taken(지적이 전달됨), 새 완료는 새 차수" '[ "$(cut -f 1 "$VD/d6/$D6.ack")" = taken ] && [ "$(acol "$D6" 8)" = taken ] && [ "$(aidof d6)" != "$D6" ] || { printf "%s\n" "$out"; cat "$AT"; false; }'
afresh; echo "ack_max 0" >>"$T/conf/hc-fill.tsv"; add d7 --only box-1; run run
lj "box-1/d7:claude-code:d7:done:/"; ev box-1/d7 d7 완료 3838ccc "끝"; run run; D7=$(aidof d7)
verd "box-1/audit-$D7" "$D7" 0 0 0; lj "box-1/d7:claude-code:d7:done:/" "box-1/audit-$D7:claude-code:audit-$D7:done:/"; run run; run run
t "ack_max(연결된 동안 센 수거 기다림) 넘으면 지휘에 한 번(hf:<ID>:noack), 기다림은 계속(deliver)" '[ "$(nrep "hf:$D7:noack")" = 1 ] && [ "$(acol "$D7" 8)" = deliver ] || { printf "%s\n" "$out"; cat "$T/report.log"; false; }'
afresh; add d8 --only box-1; run run
lj "box-1/d8:claude-code:d8:done:/"; ev box-1/d8 d8 완료 3939ddd "끝"; run run; D8=$(aidof d8)
verd "box-1/audit-$D8" "$D8" 0 0 0; lj "box-1/d8:claude-code:other:done:/" "box-1/audit-$D8:claude-code:audit-$D8:done:/"; run run
t "레인 주소에 다른 이름 터미널이 서 있으면 깨우지 않음(「깨우기 미룸」), 판정 파일은 둠" '[ ! -e "$T/send.log" ] && [ -f "$VD/d8/$D8.md" ] && printf "%s" "$out" | grep -Fq "깨우기 미룸 audit-$D8" || { printf "%s\n" "$out"; false; }'

# 한 줄 전체가 판정 형식과 맞아야 한다. 잘린 줄과 상반된 숫자에서 통과를 만들지 않는다.
for wording in '높음 0·중간 0' '높음 1·중간 0·낮음 0 (예시 높음 0)'; do
  afresh; add verdict --only box-1; run run
  lj "box-1/verdict:claude-code:verdict:done:/"; ev box-1/verdict verdict 완료 abcd001 끝; run run; ai=$(aidof verdict)
  ev "box-1/audit-$ai" "audit-$ai" 판정 "$(acol "$ai" 5)" "감사 완료 $ai $wording"; run run
  t "모호한 판정은 공개하지 않고 지휘로 보냄($wording)" '[ ! -e "$VD/verdict/$ai.md" ] && [ "$(nrep "hf:$ai:unreadable")" = 1 ]'
done

# 소비 의도만 남긴 뒤 부모가 새 커밋을 완료하면 옛 통과 파일을 공개하지 않는다.
afresh; add newer --only box-1; run run
lj "box-1/newer:claude-code:newer:done:/"; ev box-1/newer newer 완료 abcd001 끝; run run; ai=$(aidof newer)
verd "box-1/audit-$ai" "$ai" 0 0; HC_FILL_CRASH_AT=after-consume run run
ev box-1/newer newer 완료 abcd002 다음; HC_FILL_CRASH_AT=after-verdict-file run run
t "재시작 때 부모 완료가 바뀌면 옛 통과 파일을 한순간도 공개하지 않음" '[ "$(acol "$ai" 8)" = superseded ] && [ ! -e "$VD/newer/$ai.md" ]'

# 차수 열기 중단: 지시문·기록·대기열 사이에서 죽어도 다음 바퀴가 같은 ID·같은 확정 지시문으로 감사 줄 하나
for cp_ in after-brief after-opening after-put; do
  afresh; add o1 --only box-1; run run
  lj "box-1/o1:claude-code:o1:done:/"; ev box-1/o1 o1 완료 4040aaa "끝"
  HC_FILL_CRASH_AT=$cp_ run run
  oa_=$(aidof o1); ob_=$(cat "$ABR/$oa_.md" 2>/dev/null)
  run run
  t "중단 $cp_ 뒤: 감사 줄 하나(같은 ID), 같은 확정 지시문, queued" '[ "$(awk -F"\t" "\$15 == \"audit=o1\"" "$Q" | grep -c .)" = 1 ] && [ -n "$oa_" ] && [ "$(aidof o1)" = "$oa_" ] && [ "$(find "$ABR" -name "*.md" | wc -l)" -eq 1 ] && { [ "$cp_" = after-opening ] || { [ -n "$ob_" ] && [ "$(cat "$ABR/$oa_.md")" = "$ob_" ]; }; } && [ "$(acol "$(aidof o1)" 8)" = queued ] || { printf "%s\n" "$out"; cat "$Q" "$AT"; false; }'
done
# 지휘 넘김 중단: 기록 뒤·사건 뒤 → 같은 키로 이어 감(hc-lane report --key가 한 번만 남긴다)
afresh; add e1 --only box-1; run run
lj "box-1/e1:claude-code:e1:done:/"; ev box-1/e1 e1 완료 4141bbb "끝"; run run; E1=$(aidof e1)
ev "box-1/audit-$E1" "audit-$E1" 판정 "$(s40 4141bbb)" "감사 완료 $E1 모름"; lj "box-1/e1:claude-code:e1:done:/" "box-1/audit-$E1:claude-code:audit-$E1:done:/"
HC_FILL_CRASH_AT=after-escalate-record run run
t "중단 after-escalate-record: 기록은 escalate:unreadable, 사건은 아직" '[ "$(acol "$E1" 8)" = escalate:unreadable ] && [ "$(nrep "hf:$E1:")" = 0 ]'
HC_FILL_CRASH_AT=after-escalate-event run run
t "중단 after-escalate-event: 사건 한 번(키 hf:<ID>:unreadable), 기록은 아직 escalate" '[ "$(nrep "hf:$E1:unreadable")" = 1 ] && [ "$(acol "$E1" 8)" = escalate:unreadable ]'
run run; run run
t "그 뒤 바퀴: 같은 키로 한 번 더 부르고(hc-lane이 같은 키는 안 더함) coord, 그 뒤로는 부르지 않음" '[ "$(nrep "hf:$E1:unreadable")" = 2 ] && [ "$(acol "$E1" 8)" = coord ]'
# 감사 줄을 rm하면(queued 차수) 지휘에 한 번, 다시 띄우지 않음
afresh; add closed --only box-1; run run
lj "box-1/closed:claude-code:closed:idle:/"; ev box-1/closed closed 완료 5858aaa "끝"; run run; CL=$(aidof closed)
awk -F'\t' -v OFS='\t' -v a="$CL" '$1==a {$8="taken"} 1' "$AT" >"$AT.t" && mv "$AT.t" "$AT"
cp "$Q" "$T/before-disconnect"; mj box-2; lj; run run
t "감사 정리: 기계가 끊기면 정리할 감사 줄을 보존" '[ -n "$(where "audit-$CL")" ]'
cp "$T/before-disconnect" "$Q"; mj box-1 box-2; lj "box-1/audit-$CL:claude-code:new-user:idle:/"; run run
t "감사 정리: 다른 터미널이 같은 주소를 쓰면 종료 안내를 보내지 않음" '[ "$(nrep "hf:$CL:cleanup")" = 0 ] && [ -z "$(where "audit-$CL")" ]'

afresh; add r2 --only box-1; run run
lj "box-1/r2:claude-code:r2:done:/"; ev box-1/r2 r2 완료 4242ccc "끝"; run run; R2=$(aidof r2)
"$FILL" rm "audit-$R2" >/dev/null; : >"$T/lane.log"; run run; run run
t "감사 줄 rm → 차수 coord, 지휘에 한 번(hf:<ID>:cancelled), 감사 다시 안 띄움" '[ "$(acol "$R2" 8)" = coord ] && [ "$(nrep "hf:$R2:cancelled")" = 1 ] && ! grep -Fxq "audit-$R2" "$T/lane.log" && [ "$(aidof r2)" = "$R2" ] || { printf "%s\n" "$out"; cat "$AT"; false; }'

# 판정 없이 감사 터미널이 끝나거나 사라지면 세 바퀴 뒤 지휘로, working이면 세지 않음
afresh; add v1 --only box-1; run run
lj "box-1/v1:claude-code:v1:done:/"; ev box-1/v1 v1 완료 6666fff "끝"; run run; V1=$(aidof v1)
lj "box-1/v1:claude-code:v1:done:/" "box-1/audit-$V1:claude-code:audit-$V1:done:/"; ev "box-1/audit-$V1" "audit-$V1" 완료 - "판정 대신 완료로 적음"
run run; run run
t "판정 없이 done 두 바퀴: 아직 지휘로 안 넘김" '[ "$(nrep "hf:$V1:")" = 0 ] && [ "$(acol "$V1" 8)" = queued ]'
run run
t "판정 없이 done 세 바퀴 → 「<레인> 감사 1번째 판정 없음 — 지휘 판단」(hf:<ID>:noverdict)" 'grep -Fq "report 막힘 - v1 감사 1번째 판정 없음 — 지휘 판단" "$T/report.log" && [ "$(nrep "hf:$V1:noverdict")" = 1 ] && [ "$(acol "$V1" 8)" = coord ] || { printf "%s\n" "$out"; cat "$AT"; false; }'
afresh; add v2 --only box-1; run run
lj "box-1/v2:claude-code:v2:done:/"; ev box-1/v2 v2 완료 7777aaa "끝"; run run; V2=$(aidof v2)
lj "box-1/v2:claude-code:v2:done:/"; run run; run run; run run
t "감사 터미널이 판정 없이 사라짐 세 바퀴 → 지휘로" '[ "$(nrep "hf:$V2:noverdict")" = 1 ] && [ "$(acol "$V2" 8)" = coord ] || { printf "%s\n" "$out"; cat "$AT"; false; }'
afresh; add v3 --only box-1; run run
lj "box-1/v3:claude-code:v3:done:/"; ev box-1/v3 v3 완료 8888bbb "끝"; run run; V3=$(aidof v3)
lj "box-1/v3:claude-code:v3:done:/" "box-1/audit-$V3:claude-code:audit-$V3:working:/"; run run; run run; run run
t "감사가 working이면 판정 없음으로 세지 않음" '[ ! -e "$T/report.log" ] && [ "$(acol "$V3" 8)" = queued ]'

# 완료 sha: 없음(-)·작업 트리에 없는 sha는 지휘에 한 번, 그 뒤 sha가 있는 완료는 그대로 감사
afresh; add s1 --only box-1; run run
lj "box-1/s1:claude-code:s1:done:/"; ev box-1/s1 s1 완료 - "sha 빠뜨림"; run run; run run
t "완료에 sha 없음 → 지휘에 「자동 감사 못 함 <레인>: 완료에 sha 없음」 한 번(차수 nosha)" '[ "$(grep -c "자동 감사 못 함 s1: 완료에 sha 없음" "$T/report.log" 2>/dev/null)" = 1 ] && [ "$(acol "$(aidof s1)" 8)" = nosha ] && [ -z "$(awk -F"\t" "\$15 == \"audit=s1\"" "$Q")" ] || { printf "%s\n" "$out"; cat "$T/report.log"; false; }'
ev box-1/s1 s1 완료 cccc999 "다시"; run run
t "그 뒤 sha가 있는 완료는 그대로 감사" '[ "$(where "audit-$(aidof s1)" | cut -d" " -f1)" = up ]'
afresh; add s2 --only box-1; run run
lj "box-1/s2:claude-code:s2:done:/"; ev box-1/s2 s2 완료 dead999 "없는 커밋"
export HC_FAKE_REVNONE=dead999; run run; run run; unset HC_FAKE_REVNONE
t "완료 sha가 작업 트리에 없음(rev-parse 종료 1, 출력 없음) → 지휘에 한 번, 감사 줄 없음" '[ "$(grep -c "완료 sha dead999 가 작업 트리" "$T/report.log" 2>/dev/null)" = 1 ] && [ -z "$(awk -F"\t" "\$15 == \"audit=s2\"" "$Q")" ] || { printf "%s\n" "$out"; cat "$T/report.log"; false; }'

# 같은 이름을 새로 띄운 새 실행(created_ms 다름)은 옛 실행의 차수를 물려받지 않음(지적수 0부터)
afresh; add r1 --only box-1; run run
lj "box-1/r1:claude-code:r1:done:/"; ev box-1/r1 r1 완료 9898aaa "옛 일"; run run; Q1=$(aidof r1)
verd "box-1/audit-$Q1" "$Q1" 0 1 0; lj "box-1/r1:claude-code:r1:done:/" "box-1/audit-$Q1:claude-code:audit-$Q1:done:/"; run run
printf 'taken\tx\tr1@1700000000000\n' >"$VD/r1/$Q1.ack"; run run
export HC_FAKE_CREATED="box-1/r1=1700000555000"
ev10 box-1/r1 r1 완료 9898aaa "같은 커밋을 새 실행에서 완료" r1@1700000555000; lj "box-1/r1:claude-code:r1:done:/"; run run; Q2=$(aidof r1)
t "새 실행의 첫 차수는 1번째·지적수 0(옛 실행 기록과 따로)" '[ "$Q2" != "$Q1" ] && [ "$(acol "$Q2" 3)" = 1 ] && [ "$(acol "$Q2" 9)" = 0 ] && [ "$(acol "$Q2" 4)" = r1@1700000555000 ] || { printf "%s\n" "$out"; cat "$AT"; false; }'
unset HC_FAKE_CREATED

# 옛 감사 기록(7칸) 옮기기: 끝 상태는 legacy(같은 sha로 다시 안 엶), queued는 지휘에 한 번 — 옛 감사 줄은 정리
afresh; add lg --only box-1; add lq --only box-1; run run
mkdir -p "$(dirname "$AT")"; printf 'lg\tabc7777\t1\t0\tpass\t0\t0\nlq\tdef8888\t1\t0\tqueued\t0\t0\n' >"$AT"
lj "box-1/lg:claude-code:lg:done:/" "box-1/lq:claude-code:lq:done:/"; ev box-1/lg lg 완료 abc7777 "옛 판정이 이미 간 완료"
run run --dry-run
t "옛 기록: dry-run은 옮기지 않음" '[ "$(awk -F"\t" "NF >= 17" "$AT" | grep -c .)" = 0 ]'
run run
t "옛 기록 옮김: legacy·escalate:legacy(결정적 ID al…), 같은 레인·같은 sha 완료로 다시 열지 않음, queued는 지휘에 한 번" '[ "$(acol "$(aidof lg)" 8)" = legacy ] && case $(aidof lg) in al????????) true ;; *) false ;; esac && [ -z "$(awk -F"\t" "\$15 == \"audit=lg\"" "$Q")" ] && [ "$(nrep "hf:$(aidof lq):legacy")" = 1 ] && [ "$(acol "$(aidof lq)" 8)" = coord ] || { printf "%s\n" "$out"; cat "$AT"; false; }'

# 옛 설치 틀({audit_id} 없음): 지시문 끝에 새 판정 줄 절이 붙는다
afresh; sed 's/{audit_id}/{sha}/' "$EXA" >"$T/oldtpl.md"; grep -v '^audit ' "$T/conf/hc-fill.tsv" >"$T/c.t"; echo "audit    $T/oldtpl.md" >>"$T/c.t"; mv "$T/c.t" "$T/conf/hc-fill.tsv"
add ot --only box-1; run run; lj "box-1/ot:claude-code:ot:done:/"; ev box-1/ot ot 완료 5050aaa "끝"; run run; O1=$(aidof ot)
t "옛 틀: 「## 판정 줄 (hc-fill — 이 절이 위 판정 줄보다 앞선다)」 절이 붙고 차수 ID·40자 sha가 든다" 'grep -Fq "## 판정 줄 (hc-fill — 이 절이 위 판정 줄보다 앞선다)" "$ABR/$O1.md" && grep -Fq "hc-lane report 판정 $(s40 5050aaa) \"감사 완료 $O1 높음" "$ABR/$O1.md" || { cat "$ABR/$O1.md"; false; }'

# 진짜 hc-lane report·verdict로 끝까지: 배포 틀 판정 줄 → 소비 → 판정 파일 → 레인이 hc-lane verdict → taken
afresh; RR=$T/realrepo; rm -rf "$RR"; mkdir -p "$RR" "$T/bin2"; ln -sf "$T/bin/hc" "$T/bin2/hc"
( cd "$RR" && env -u GIT_DIR /usr/bin/git init -q && /usr/bin/git -c user.name=t -c user.email=t@t commit -q --allow-empty -m one ) && RS=$(cd "$RR" && /usr/bin/git rev-parse HEAD)
add rl --only box-1; run run; lj "box-1/rl:claude-code:rl:done:$RR"; ev box-1/rl rl 완료 "${RS:0:8}" "끝"
export HC_FAKE_SHA40=$RS; run run; unset HC_FAKE_SHA40; RL=$(aidof rl)
LANEB=${HC_LANE_BIN:-${FILL%/*}/hc-lane}
RPATH=$T/bin2:${PATH#"$T/bin:"}   # 가짜 hc만(진짜 git으로 HEAD를 읽게)
( export PATH="$RPATH" XDG_STATE_HOME=$T/xdg-a HC_TERMINAL_ID=audit-$RL HC_LANE=audit-$RL HC_LANE_MACHINE=m; bash "$LANEB" report 판정 "$RS" "감사 완료 $RL 높음 1·중간 0·낮음 0" >/dev/null 2>&1 )
awk -F'\t' -v OFS='\t' '{ $3 = "box-1/" $3; print }' "$T/xdg-a/hc-lane/events.tsv" >>"$T/events"
lj "box-1/rl:claude-code:rl:done:$RR" "box-1/audit-$RL:claude-code:audit-$RL:done:/"; run run
vo=$(cd "$RR" && PATH="$RPATH" HC_TERMINAL_ID=rl HC_LANE=rl bash "$LANEB" verdict "$RL" 2>&1)
run run
t "진짜 report 판정(배포 틀 꼴) → 소비 → 판정 파일 → 진짜 hc-lane verdict가 지적을 수거(taken) → 차수 taken" '[ "$(vkey rl verdict "$RL")" = fix ] && printf "%s" "$vo" | grep -Fq "감사 지적" && [ "$(cut -f 1 "$VD/rl/$RL.ack")" = taken ] && [ "$(acol "$RL" 8)" = taken ] || { printf "%s\n%s\n" "$out" "$vo"; cat "$AT"; false; }'

# 감사 대상이 아닌 줄: --no-audit · 받은편지 · audit 키 없음
afresh
run add --name na --project app --brief "$(brief na)" --only box-1 --no-audit
t "add --no-audit: 감사 칸 no-audit, 알림에 「자동 감사 끔」" '[ $rc = 0 ] && [ "$(col15 na)" = no-audit ] && printf "%s" "$out" | grep -Fq "자동 감사 끔"'
run run; lj "box-1/na:claude-code:na:done:/"; ev box-1/na na 완료 abcdef7 "끝"; run run
t "--no-audit 줄: 감사·착지 절 없음, 완료해도 감사 줄 없음" '! grep -Fq "감사·착지" "$HOME/.local/share/hc-fill/runs/na-run.md" && [ -z "$(aidof na)" ] && [ "$(col15 na)" = no-audit ] || { printf "%s\n" "$out"; false; }'
ifresh; echo "audit    $EXA" >>"$T/conf/hc-fill.tsv"; rm -f "$T/events"
mj box-1 shared slow; req "$IS" inq-z app - slow; run run
lj "slow/inq-z:claude-code:inq-z:done:/"; printf '2026-11-19T01:00:00Z\tm\tslow/inq-z\tinq-z\t완료\t1234abc\t끝\t1\n' >"$T/events"; run run
t "받은편지 줄: 감사·착지 절 없음, 완료해도 감사 줄 없음" '[ "$(where inq-z | cut -d" " -f1)" = up ] && ! grep -Fq "감사·착지" "$HOME/.local/share/hc-fill/runs/inq-z-run.md" && [ -z "$(aidof inq-z)" ] || { printf "%s\n" "$out"; false; }'
fresh; rm -f "$T/events" "$T/events.log"; cp "$EX" "$T/conf/hc-fill.tsv"
add nk --only box-1; run run; lj "box-1/nk:claude-code:nk:done:/"; printf '2026-11-19T01:00:00Z\tm\tbox-1/nk\tnk\t완료\t1234abc\t끝\t1\n' >"$T/events"; run run
t "audit 키 없음: 절 없음, 감사 줄·기록 없음(events는 마지막 보고로 읽음)" '! grep -Fq "감사·착지" "$HOME/.local/share/hc-fill/runs/nk-run.md" && [ -z "$(awk -F"\t" "\$15 ~ /^audit=/" "$Q")" ] && [ ! -e "$AT" ] && [ "$(col15 nk)" != auto ]'

# 긴 이름 · dry-run
afresh; ln=abcdefghijklmnopqrstuvwxyz0123; add $ln --only box-1; run run
lj "box-1/$ln:claude-code:$ln:done:/"; ev box-1/$ln $ln 완료 1111aaa "끝"; run run
an=$(awk -F'\t' '$15 ~ /^audit=/ {print $1}' "$Q")
t "긴 레인 이름도 감사 줄 이름은 audit-<차수 ID>(32자 안), 감사 칸 audit=<레인>" '[ "${#an}" -le 32 ] && [ "$an" = "audit-$(aidof $ln)" ] && [ "$(col15 "$an")" = "audit=$ln" ] || { echo "이름: $an"; printf "%s\n" "$out"; false; }'
afresh; add w7 --only box-1; run run
lj "box-1/w7:claude-code:w7:done:/"; ev box-1/w7 w7 완료 4444ddd "끝"
before=$(cat "$Q"); run run --dry-run
t "dry-run: 「넣을 감사」만 찍고 대기열·기록·지시문 그대로" 'printf "%s" "$out" | grep -Fq "[dry-run] 넣을 감사 audit-<새 차수> ← w7 4444ddd0 (1번째, box-1)" && [ "$(cat "$Q")" = "$before" ] && [ ! -s "$AT" ] && [ ! -d "$ABR" ]'
run run; W7=$(aidof w7); verd "box-1/audit-$W7" "$W7" 0 0 1; lj "box-1/w7:claude-code:w7:done:/" "box-1/audit-$W7:claude-code:audit-$W7:done:/"
run run --dry-run
t "dry-run: 「넘길 판정」만 찍고 판정 파일·깨움 없음" 'printf "%s" "$out" | grep -Fq "[dry-run] 넘길 판정 audit-$W7 → w7 box-1/w7 (pass" && [ ! -e "$T/send.log" ] && [ ! -e "$VD/w7/$W7.md" ] && [ "$(acol "$W7" 8)" = queued ]'
# 레인상한 1인 기계: 감사 줄은 레인상한을 안 봄(교착 없음)
afresh; sed -i '' 's/^machine  box-1    2   -$/machine  box-1    2   1/' "$T/conf/hc-fill.tsv"
add l1 --only box-1; run run
lj "box-1/l1:claude-code:l1:done:/"; ev box-1/l1 l1 완료 5555eee "끝"; run run; L1=$(aidof l1)
t "레인상한 1인 기계에서 완료 → 감사 줄이 뜬다(감사 줄은 레인상한을 안 봄)" '[ "$(where "audit-$L1")" = "up box-1 box-1/audit-$L1" ] || { printf "%s\n" "$out"; cat "$Q"; false; }'
add l2 --only box-1; lj "box-1/l1:claude-code:l1:done:/" "box-1/audit-$L1:claude-code:audit-$L1:working:/"; run run
t "레인상한 1에서 감사를 기다리는 동안 다른 쓰기 줄은 대기" '[ "$(where l2)" = "wait - -" ] || { printf "%s\n" "$out"; false; }'
# 그 기계에서 상태가 안 잡혀 남긴(stuck) 감사 줄은 자리를 쥔 것으로 보지 않음 — 레인을 다시 셈
afresh; mkdir -p "$W/app.wt/q1"; add q1 --only box-1; run run
lj "box-1/q1:claude-code:q1:done:$W/app.wt/q1"; ev box-1/q1 q1 완료 eeee777 "끝"
HC_FILL_CRASH_AT=after-put run run; QA=$(aidof q1)
export HC_FAKE_NOSTATUS="box-1/audit-$QA"; run run; unset HC_FAKE_NOSTATUS
run run
t "stuck 감사 줄(그 레인 기계에 못 뜸): 감사 기다리는 레인을 다시 자리로 셈" '[ "$(where "audit-$QA")" = "wait - -" ] && printf "%s" "$out" | grep -Fq "자리 box-1 box-1/q1 q1: done, 작업 트리 있음 → 셈" || { printf "%s\n" "$out"; cat "$Q"; false; }'
t "자동 감사는 어떤 터미널도 끄지 않음(hc kill 없음)" '! grep -q "^kill" "$T/hc.log" 2>/dev/null'

# add·받은편지 이름 규칙(감사 줄 이름 자리·커밋 종류)
run add --name audit-x --project app --brief "$(brief ax)"
t "add: audit- 머리 이름은 거절(감사 줄 이름 자리)" '[ $rc = 1 ] && [ -z "$(where audit-x)" ] && printf "%s" "$out" | grep -Fq "audit-"'
fresh; cp "$EX" "$T/conf/hc-fill.tsv"; run add --name audit-y --project app --brief "$(brief ay)"
t "add: 감사 꺼짐(audit 키 없음)에서도 audit- 머리 이름 거절" '[ $rc = 1 ] && [ -z "$(where audit-y)" ]'
ifresh; mj box-1 shared slow; req "$IS" audit-zz app; run run
t "받은편지: audit- 머리 이름은 거절(rejected/<이름>.why, 감사 꺼짐에서도)" 'grep -Fq "audit-" "$IS/rejected/audit-zz.why" && [ -z "$(where audit-zz)" ] || { printf "%s\n" "$out"; false; }'
cp "$EX" "$T/conf/hc-fill.tsv"
# 커밋 종류와 같은 이름(fix·docs…)은 착지·선행 판정이 아무 「fix: …」 커밋에나 맞으니 거절, 그 낱말로 시작하는 이름은 받음
fresh; run add --name fix --project app --brief "$(brief kf)"
t "add: 커밋 종류와 같은 이름(fix) 거절" '[ $rc = 1 ] && [ -z "$(where fix)" ] && printf "%s" "$out" | grep -Fq "레인 이름이 커밋 종류(fix)와 같으면" || { printf "%s\n" "$out"; false; }'
run add --name fix-login --project app --brief "$(brief kl)"
t "add: 커밋 종류로 시작하는 이름(fix-login)은 받음" '[ $rc = 0 ] && [ "$(where fix-login | cut -d" " -f1)" = wait ] || { printf "%s\n" "$out"; false; }'
run add --name kd --project app --brief "$(brief kd)" --after fix-login,docs
t "add: --after에 커밋 종류 이름(docs) 거절" '[ $rc = 1 ] && [ -z "$(where kd)" ] && printf "%s" "$out" | grep -Fq "커밋 종류(docs)" || { printf "%s\n" "$out"; false; }'
ifresh; mj box-1 shared slow; req "$IS" fix app; req "$IS" kfa app docs; run run
t "받은편지: 커밋 종류 이름(fix)·after(docs) 거절, why에 사유" 'grep -Fq "레인 이름이 커밋 종류(fix)와 같으면 착지·선행 판정이 아무 fix: 커밋에나 맞는다" "$IS/rejected/fix.why" && grep -Fq "커밋 종류(docs)" "$IS/rejected/kfa.why" && [ -z "$(where fix)" ] && [ -z "$(where kfa)" ] || { printf "%s\n" "$out"; false; }'
cp "$EX" "$T/conf/hc-fill.tsv"

afresh; echo "audit    $T/없는틀.md" >>"$T/conf/hc-fill.tsv"; sed -i '' "/^audit    ${EXA//\//\\/}\$/d" "$T/conf/hc-fill.tsv"; add w8; run run
t "설정 audit 틀을 못 읽으면 종료 1, 띄우지 않음" '[ $rc = 1 ] && printf "%s" "$out" | grep -Fq "설정 audit 틀을 읽지 못함" && [ ! -e "$T/lane.log" ] || { printf "%s\n" "$out"; false; }'
cp "$EX" "$T/conf/hc-fill.tsv"

# 10h+. 최종 감사(2026-11-20 지휘 — 실수를 코드로): 최종 감사가 따로 있는데 레인이 hc-fill 감사 통과만 보고 착지했다.
#      설정 audit_final <안내>가 있으면 run 지시문 감사 절 3번·ls가 「최종 감사 통과 뒤에만 착지」. 없으면 그대로(위 10h)
cp "$T/conf/hc-fill.tsv" "$T/conf.keep"; mkdir -p "$W/app.wt/wf"
afresh; echo "audit_final 최종 감사(지휘 자동 감사기)" >>"$T/conf/hc-fill.tsv"
add wf --only slow
run run
rf=$HOME/.local/share/hc-fill/runs/wf-run.md
t "audit_final: run 지시문 감사 절 3번은 「감사 통과여도 착지하지 않는다 — 최종 감사(<안내>) 통과 알림을 기다린다」" 'grep -Fq "감사 통과 …」여도 착지하지 않는다 — 최종 감사(최종 감사(지휘 자동 감사기))의 통과 알림을 기다린다" "$rf" && ! grep -Fq "3. 출력이 「감사 통과 …」면 그 출력의 착지 줄" "$rf" || { printf "%s\n" "$out"; cat "$rf"; false; }'
lj "slow/wf:claude-code:wf:done:$W/app.wt/wf"; ev slow/wf wf 완료 abc1234 "끝"; run run; WF=$(aidof wf)
lj "slow/wf:claude-code:wf:done:$W/app.wt/wf" "slow/audit-$WF:claude-code:audit-$WF:done:$W/app.wt/wf"
verd "slow/audit-$WF" "$WF" 0 0 1; run run
printf 'landing\tx\twf@1700000000000\tp\n' >"$VD/wf/$WF.ack"; run run
t "audit_final: ls 비고 「감사 통과 받음(최종 대기)」" '"$FILL" ls 2>&1 | grep "^wf " | grep -Fq "감사 통과 받음(최종 대기)"'
cp "$T/conf.keep" "$T/conf/hc-fill.tsv"

# 10i. --then(2026-11-19 fill-then 사용자 「--then 추가 (추천)」): 같은 일의 다음 단계는 새 줄 없이 대기 중인 선행 줄 지시문에 이어 붙임
fresh
add p1
pb=$HOME/.local/share/hc-fill/briefs/added/p1.md
run add --name n1 --project app --brief "$(brief n1)" --then p1
t "--then: 종료 0, 대기열 줄 수 그대로, 출력 한 줄" '[ $rc = 0 ] && [ "$(grep -c . "$Q")" = 1 ] && printf "%s" "$out" | grep -Fxq "이어 붙임 n1 → p1 (새 레인 없음, 선행 지시문 $pb)" || { printf "%s\n" "$out"; cat "$Q"; false; }'
t "--then: 선행 지시문 = 원래 본문 + 「## 이어서: n1」 절·커밋 머리 약속·본문" 'head -n 2 "$pb" | grep -Fq "# 지시문 p1" && grep -Eq "^## 이어서: n1 \(hc-fill add --then, 20[0-9-]+T[0-9:]+\)$" "$pb" && grep -Fq "커밋 제목 머리는 \`n1:\`" "$pb" && grep -Fq "위 일의 완료 보고·착지는 하지 않는다 — 이 절까지 끝낸 뒤 한 번만 report 완료" "$pb" && [ "$(tail -n 2 "$pb" | head -n 1)" = "# 지시문 n1" ] && [ "$(tail -n 1 "$pb")" = 본문 ] || { cat "$pb"; false; }'
qsum() { cat "$Q" "$HOME/.local/share/hc-fill/briefs/added/"*.md 2>/dev/null | cksum; }
s0=$(qsum)
run add --name n1 --project app --brief "$T/n1.md" --then p1
t "--then: 같은 이름 두 번 → 종료 2, 그대로" '[ $rc = 2 ] && printf "%s" "$out" | grep -Fq "n1은 이미 p1에 이어 붙였다" && [ "$(qsum)" = "$s0" ] || { printf "%s\n" "$out"; false; }'
run add --name n2 --project app --brief "$(brief n2)" --then n1
t "--then: 이어 붙인 이름을 선행으로 → 종료 2 + 실제 줄 안내" '[ $rc = 2 ] && printf "%s" "$out" | grep -Fq "n1은 p1에 이어 붙였다 — --then p1" && [ "$(qsum)" = "$s0" ] || { printf "%s\n" "$out"; false; }'
run add --name n2 --project app --brief "$T/n2.md" --then nope
t "--then: 선행이 대기열에 없음 → 종료 2" '[ $rc = 2 ] && printf "%s" "$out" | grep -Fq "선행이 대기열에 없다: nope" && [ "$(qsum)" = "$s0" ]'
run add --name n1 --project app --brief "$T/n2.md"
t "add: 이어 붙인 이름을 새 줄로 → 거절, 그대로" '[ $rc != 0 ] && printf "%s" "$out" | grep -Fq "n1은 이미 p1에 이어 붙였다(--then)" && [ "$(qsum)" = "$s0" ] || { printf "%s\n" "$out"; false; }'
for o in "--after p1" "--only box-1" "--priority 높음" "--no-early" "--no-audit" "--coord t0"; do
  run add --name n3 --project app --brief "$(brief n3)" --then p1 $o
  t "--then과 $o 함께 → 종료 2, 그대로" '[ $rc = 2 ] && printf "%s" "$out" | grep -Fq -- "--then은 ${o%% *}와 함께 쓰지 않는다" && [ "$(qsum)" = "$s0" ] || { printf "%s\n" "$out"; false; }'
done
: >"$T/empty.md"
run add --name n3 --project app --brief "$T/empty.md" --then p1
t "--then: 빈 지시문 → 종료 2" '[ $rc = 2 ] && printf "%s" "$out" | grep -Fq "지시문을 읽지 못함(빈 파일)" && [ "$(qsum)" = "$s0" ]'
# 감사 [낮음] 2: 사본 폴더 밖 지시문(사본 기능 전에 넣은 옛 줄)은 원본을 고치지 않게 거절
cp "$Q" "$Q.keep"; awk -F'\t' -v OFS='\t' -v b="$T/p1.md" '$1=="p1" {$3 = b} 1' "$Q.keep" >"$Q"; s1=$(cksum <"$T/p1.md")
run add --name n3 --project app --brief "$T/n3.md" --then p1
t "--then: 선행 지시문이 사본 폴더 밖 → 종료 2, 원본 그대로" '[ $rc = 2 ] && printf "%s" "$out" | grep -Fq "p1: 지시문이 사본이 아니다($T/p1.md) — rm 뒤 다시 add" && [ "$(cksum <"$T/p1.md")" = "$s1" ] || { printf "%s\n" "$out"; false; }'
mv "$Q.keep" "$Q"
qb=${pb%/*}/q1.md; cp "$T/n1.md" "$qb"   # 다른 프로젝트(tools)의 대기 줄 q1
awk -F'\t' -v OFS='\t' -v b="$qb" '$1=="p1" {print "q1", "tools", b, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15} 1' "$Q" >"$Q.x" && mv "$Q.x" "$Q"
s0=$(qsum)
run add --name n3 --project app --brief "$T/n3.md" --then q1
t "--then: 프로젝트가 다름 → 종료 2" '[ $rc = 2 ] && printf "%s" "$out" | grep -Fq "q1: 프로젝트가 다르다(tools)" && [ "$(qsum)" = "$s0" ] || { printf "%s\n" "$out"; false; }'
for k in "up|6|이미 떴다(up)" "sent|6|이미 떴다(sent)" "box-1|inbox11|받은편지 줄에는" "audit=p9|15|자동 감사 줄에는" "box-1=box-1/p1|9|상태 안 잡힘으로 남긴 터미널이 있다"; do
  IFS='|' read -r v c why <<<"$k"
  cp "$Q" "$Q.keep"; awk -F'\t' -v OFS='\t' -v v="$v" -v c="$c" '$1=="p1" { if (c == 6) $6 = v; else if (c == 9) $9 = v; else if (c == "inbox11") $11 = v ":/in/done/p1.md"; else $15 = v } 1' "$Q.keep" >"$Q"
  s0=$(qsum)
  run add --name n4 --project app --brief "$(brief n4)" --then p1
  t "--then: 선행이 ${v}(${c}) → 종료 2 「${why}」, 그대로" '[ $rc = 2 ] && printf "%s" "$out" | grep -Fq "p1: ${why}" && [ "$(qsum)" = "$s0" ] || { printf "%s\n" "$out"; false; }'
  mv "$Q.keep" "$Q"
done
awk -F'\t' -v OFS='\t' '$1=="p1" {$6="up"} 1' "$Q" >"$Q.x" && mv "$Q.x" "$Q"
run add --name n4 --project app --brief "$T/n4.md" --then p1
t "--then: 떠 있는 선행 → 「이미 떴다(up) — --after p1」" 'printf "%s" "$out" | grep -Fq "이미 떴다(up) — --after p1으로 넣거나 그 레인에 hc send"'
awk -F'\t' -v OFS='\t' '$1=="p1" {$6="wait"} 1' "$Q" >"$Q.x" && mv "$Q.x" "$Q"
run ls
t "ls: 선행 줄 비고에 「이어서 n1」" 'printf "%s" "$out" | grep -E "^p1 .*이어서 n1" >/dev/null || { printf "%s\n" "$out"; false; }'
run add --name a1 --project app --brief "$(brief a1)" --after p1,q1
t "--after에 대기 중 같은 프로젝트 레인 → 알림 한 줄 + 정상 추가, 다른 프로젝트는 알림 없음" '[ $rc = 0 ] && [ -n "$(where a1)" ] && printf "%s" "$out" | grep -Fxq "알림: p1가 아직 대기 중 — 같은 일의 다음 단계면 --after 대신 --then p1로 한 레인에 이어 붙인다" && ! printf "%s" "$out" | grep -Fq "q1가 아직" || { printf "%s\n" "$out"; false; }'
awk -F'\t' -v OFS='\t' '$1=="p1" {$6="up"} 1' "$Q" >"$Q.x" && mv "$Q.x" "$Q"
run add --name a2 --project app --brief "$(brief a2)" --after p1
t "--after에 떠 있는 레인 → 알림 없음" '[ $rc = 0 ] && ! printf "%s" "$out" | grep -Fq "아직 대기 중"'
fresh; add p2; run add --name n5 --project app --brief "$(brief n5)" --then p2; run run
rf=$HOME/.local/share/hc-fill/runs/p2-run.md
t "run: 선행을 띄울 때 run 지시문에 이어 붙인 절이 들어감" '[ "$(where p2 | cut -d" " -f1)" = up ] && grep -Fq "## 이어서: n5 (hc-fill add --then" "$rf" && grep -Fq "# 지시문 n5" "$rf" && [ -z "$(where n5)" ] || { printf "%s\n" "$out"; cat "$rf"; false; }'
fresh; add p3
# 감사 [낮음] 5: 시간 비교 없이 순서로 — 배경이 run.lock을 잡은 뒤(표식) --then을 부르고, 배경이 잠금 안에서 남긴 줄이 --then 줄보다 먼저
mark=$T/then.mark ord=$T/then.ord; : >"$ord"
lockf -k "$HOME/.local/share/hc-fill/run.lock" sh -c ': >"$1"; sleep 1; echo run >>"$2"' _ "$mark" "$ord" &
lp=$!; i=0; while [ ! -e "$mark" ] && [ $i -lt 100 ]; do sleep 0.1; i=$((i + 1)); done
run add --name n6 --project app --brief "$(brief n6)" --then p3; echo then >>"$ord"; wait $lp
t "--then: 도는 run(run.lock)이 끝난 뒤에 붙인다 — run이 지시문을 읽고 up으로 바꾸는 사이에 끼지 않게" '[ -e "$mark" ] && [ $rc = 0 ] && [ "$(paste -sd , "$ord")" = run,then ] || { printf "%s\n" "$out"; cat "$ord"; false; }'

# 10h. 마지막 보고(2026-11-19 fill-ls-early-state, 사용자 「이제 이런 문제 없나?」): ls가 터미널 상태와 띄울 때 적은 글만 보여
# 「끝남」이 착지인지 실패인지, 「떠 있음(done)」이 감사 기다림인지 착지 뒤 쉼인지 몰랐다. run이 바퀴마다 레인 사건(hc-lane events,
# 자동 감사와 같은 한 번)에서 대기열 레인마다 마지막 사건을 $DATA/last.tsv에 적고, ls는 그 파일만 읽는다(원격 호출 없음)
LR=$HOME/.local/share/hc-fill/last.tsv
lrl() { printf "%s\n" "$out" | grep "^$1 "; }
fresh; rm -f "$T/events" "$T/events.log"; cp "$EX" "$T/conf/hc-fill.tsv"
add lr1 --only box-1; add lr2 --only slow; run run
lj "box-1/lr1:claude-code:lr1:done:/" "slow/lr2:claude-code:lr2:working:/"
printf '2026-11-19T08:30:00Z\tm\tbox-1/lr1\tlr1\t완료\t1372684aa\t다 됨\t1\n2026-11-19T08:38:00Z\tm\tbox-1/lr1\tlr1\t착지\t8bccc0d11\tmain에 넣음\t2\n2026-11-19T08:39:00Z\tm\tbox-1/other\tlr1\t막힘\t-\t남의 터미널\t1\n' >"$T/events"
rm -f "$T/events.log"; TZ=UTC run run
t "마지막 보고: run이 hc-lane events를 한 번만(기계마다 --on 묶어) 읽음" '[ "$(grep -c . "$T/events.log")" = 1 ] && grep -Fq -- "--on box-1" "$T/events.log" && grep -Fq -- "--on slow" "$T/events.log" || { cat "$T/events.log"; printf "%s\n" "$out"; false; }'
rm -f "$T/events.log" "$T/hc.log"; run ls
t "ls 마지막 보고: 그 터미널의 마지막 사건 착지 sha 8자·시각(남의 터미널 사건은 안 셈)" 'lrl lr1 | grep -Fq "착지 8bccc0d1 08:38 main에 넣음" || { printf "%s\n" "$out"; cat "$LR"; false; }'
t "ls 마지막 보고: 사건 없는 레인은 「보고 없음」" 'lrl lr2 | grep -Fq "보고 없음" || { printf "%s\n" "$out"; false; }'
t "ls는 원격 호출 안 함(hc-lane events·hc exec 없음)" '[ ! -e "$T/events.log" ] && ! grep -q "^exec" "$T/hc.log" 2>/dev/null || { cat "$T/events.log" "$T/hc.log" 2>/dev/null; false; }'
mj box-2 shared slow; rm -f "$T/events.log"; run run; run ls
t "연결 끊긴 기계: 지난 값을 두고 「(옛 값 HH:MM)」, 그 기계는 events에 --on 안 함" 'lrl lr1 | grep -Fq "착지 8bccc0d1 08:38" && lrl lr1 | grep -Eq "\(옛 값 [0-9]{2}:[0-9]{2}\)" && ! lrl lr2 | grep -Fq "옛 값" && ! grep -Fq -- "--on box-1" "$T/events.log" || { printf "%s\n" "$out"; cat "$LR" "$T/events.log"; false; }'
mj box-1 box-2 shared slow; run run; run ls
r_=$(awk -F'\t' '$1=="lr1" {print $17}' "$Q")
t "last.tsv 여덟째 칸 = 대기열 줄 ID" '[ -n "$r_" ] && awk -F"\t" -v r="$r_" "\$1==\"lr1\" && \$8==r {f=1} END {exit !f}" "$LR" || { cat "$LR"; false; }'
awk -F'\t' -v OFS='\t' '$1=="lr1" {$17 = "rnew"} 1' "$Q" >"$Q.t" && mv "$Q.t" "$Q"   # 같은 이름의 새 줄(rm·다시 add)
mj box-2 shared slow; run run; run ls
t "기존 6: 끊긴 기계의 지난 값은 줄 ID가 같을 때만 — 새 동명 줄에 옛 착지 sha가 안 붙음" '! lrl lr1 | grep -Fq "8bccc0d1" || { printf "%s\n" "$out"; cat "$LR"; false; }'
mj box-1 box-2 shared slow; run run; run ls
t "다시 연결되면 옛 값 표시 없음" 'lrl lr1 | grep -Fq "착지 8bccc0d1" && ! lrl lr1 | grep -Fq "옛 값" || { printf "%s\n" "$out"; false; }'
cp "$LR" "$T/lr.before"; printf '2026-11-19T09:00:00Z\tm\tslow/lr2\tlr2\t막힘\t-\t테스트 실패\t1\n' >>"$T/events"; run run --dry-run
t "dry-run은 last.tsv를 쓰지 않음" 'cmp -s "$LR" "$T/lr.before" || { cat "$LR"; false; }'
TZ=UTC run run; run ls
t "막힘은 sha 없이 「막힘 09:00 <앞부분>」" 'lrl lr2 | grep -Fq "막힘 09:00 테스트 실패" || { printf "%s\n" "$out"; false; }'

# 10j. 겹침(2026-11-19 lane-overlap-guard 사용자 「rebase 충돌을 근본적으로」, 지휘 검토 반영): 같은 프로젝트의 쓰기 소유 레인(대기열에서
#      띄운 쓰기 줄 — 터미널이 살아 있고 착지 전)과 겹치는 대기 줄. 예약 = 지시문의 「파일:」·「쓰기:」 줄의 경로(아직 없는 새 파일도)·FR ID,
#      언급 = 본문의 저장소 경로·FR ID. 설정 overlap: warn(기본 — 띄우고 경고만) · block(예약끼리 겹치면 대기, 언급은 경고) · off
OR=$T/orepo; mkdir -p "$OR/bin" "$OR/lib" "$OR/docs/frag" "$OR.wt/o1"; : >"$OR/bin/tool"; : >"$OR/lib/b.sh"; : >"$OR/docs/frag/x.md"; : >"$OR/docs/a.md"
# ofresh [warn|block|off|none]: 저장소 경로를 $OR로, overlap 줄을 그 값으로(none이면 줄을 뺌 — 설정을 안 적은 기존 설치). 사건 파일도 비움
ofresh() { fresh; rm -f "$T/events"; oe_="s|^overlap[[:space:]].*|overlap  ${1:-warn}|"; [ "${1-}" != none ] || oe_='/^overlap[[:space:]]/d'
  sed -E -e "s|^(repo[[:space:]]+app[[:space:]]+here[[:space:]]+).*|\1$OR|" -e "$oe_" "$EX" >"$T/conf/hc-fill.tsv"; }
t "겹침: 예시 설정의 overlap은 block(같은 파일을 고칠 레인이 떠 있으면 막음 — 2026-11-20 지휘 t1cj)" 'grep -Eq "^overlap[[:space:]]+block$" "$EX"'
oadd() { on_=$1 ob_=$2; shift 2; printf '# 지시문 %s\n%s\n' "$on_" "$ob_" >"$T/$on_.md"; "$FILL" add --name "$on_" --project app --brief "$T/$on_.md" "$@" >/dev/null 2>&1; }
up() { [ "$(where "$1" | cut -d" " -f1)" = up ]; }
OW=$HOME/.local/share/hc-fill/overlap-warn.log
ofresh
oadd o1 '파일: `bin/tool`'; oadd o2 '- 파일: bin/tool, docs/a.md'
run run
t "겹침 warn(기본): 예약이 겹쳐도 띄우고 경고 한 줄·경고 기록" 'up o1 && up o2 && printf "%s" "$out" | grep -Fq "겹침 경고 o2: o1 예약 (bin/tool)" && grep -q "	o2	o1	예약	bin/tool" "$OW" || { printf "%s\n" "$out"; cat "$OW"; false; }'
ofresh off
oadd o1 '파일: bin/tool'; oadd o2 '파일: bin/tool'
run run
t "겹침 off: 경고도 없음" 'up o2 && ! printf "%s" "$out" | grep -Fq "겹침" && [ ! -e "$OW" ]'
ofresh none
oadd o1 '파일: bin/tool'; oadd o2 '파일: bin/tool'
run run
t "겹침: 설정에 overlap 줄이 없으면(기존 설치) block — 같은 파일 예약은 막음(2026-11-20 지휘 t1cj)" '! grep -q "^overlap" "$T/conf/hc-fill.tsv" && up o1 && [ "$(where o2)" = "wait - -" ] && printf "%s" "$out" | grep -Fq "겹침 대기 o2: o1 (bin/tool)" || { printf "%s\n" "$out"; false; }'
ofresh block
oadd o1 '파일: `bin/tool`을 고친다'; oadd o2 '- 파일: bin/tool, docs/a.md'
run run
t "겹침 block: 같은 바퀴에 예약이 겹치는 두 줄은 앞 줄만 뜨고 뒤 줄은 대기, 사유 한 줄" 'up o1 && [ "$(where o2)" = "wait - -" ] && printf "%s" "$out" | grep -Fq "겹침 대기 o2: o1 (bin/tool)" || { printf "%s\n" "$out"; false; }'
run ls
t "겹침 ls: 비고에 「겹침 대기(<앞 레인>: <파일>)」" 'printf "%s\n" "$out" | grep "^o2 " | grep -Fq "겹침 대기(o1: bin/tool)" || { printf "%s\n" "$out"; false; }'
lj box-1/o1:claude-code:o1; echo "abc1234 o1: 옛 같은 이름 일" >"$T/commits"; run run
t "겹침 block: 앞 레인이 떠 있는 동안 다음 바퀴에도 대기 — 옛 같은 이름 커밋 제목은 소유를 풀지 않음(아스트라 높음 4)" '[ "$(where o2)" = "wait - -" ] || { printf "%s\n" "$out"; false; }'
printf '%s\tm\tbox-1/o1\to1\t착지\tabc12345\tmain에 넣음\t2\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$T/events"; run run
t "겹침 block: 앞 레인 터미널이 그 이름으로 착지 사건을 적으면(터미널이 남아도) 쓰기 소유를 풀어 뜸" 'up o2 || { printf "%s\n" "$out"; false; }'
ofresh block
oadd o1 '파일: bin/tool'; oadd o2 '파일: bin/tool'; run run; lj box-1/o1:claude-code:o1; run run
t "겹침 block(해제 시험 앞): 앞 레인 터미널이 있으면 대기" '[ "$(where o2)" = "wait - -" ] || { printf "%s\n" "$out"; false; }'
mj box-2 shared slow; lj; run run
t "겹침 block: 앞 레인 기계가 연결 안 됨이면(끝났는지 모름) 소유 유지 — 대기(아스트라 높음 2)" '[ "$(where o2)" = "wait - -" ] && printf "%s" "$out" | grep -Fq "o1 @box-1 연결 안 됨" || { printf "%s\n" "$out"; false; }'
mj box-1 box-2 shared slow; run run
t "겹침 block: 연결된 기계에서 앞 레인 터미널이 사라지면 같은 줄이 wait → up" 'up o2 || { printf "%s\n" "$out"; false; }'
# 병합기에 제출한 레인(2026-11-20 감사 land-merge-train-2 중간 1): 제출 뒤 터미널이 사라져도 원격 큐(refs/merge-queue/<초>-<레인>)에
# 남아 있는 동안은 쓰기 소유 유지 — 병합기가 넣기 전에 같은 파일 레인이 뜨면 병합기에서 충돌한다
ofresh block
oadd o1 '파일: bin/tool'; oadd o2 '파일: bin/tool'; run run; lj box-1/o1:claude-code:o1; run run
lj; HC_FAKE_MQ=o1 run run
t "겹침 block: 앞 레인이 병합기 큐에 있으면 터미널이 사라져도 소유 유지 — 대기" '[ "$(where o2)" = "wait - -" ] && printf "%s" "$out" | grep -Fq "o1 병합기 큐에 제출됨" || { printf "%s\n" "$out"; false; }'
run run
t "겹침 block: 큐에서 빠지면(병합기가 넣음) 풀려 뜸" 'up o2 || { printf "%s\n" "$out"; false; }'
# 병합기 1차 중간 5: 큐를 못 읽으면(모름) 소유를 쥔다 · 레인 터미널이 살아 있어도 병합기 기계가 그 이름으로 착지를 적으면 푼다
ofresh block
oadd o1 '파일: bin/tool'; oadd o2 '파일: bin/tool'; run run; lj box-1/o1:claude-code:o1; run run
lj; HC_FAKE_MQ_FAIL=1 run run
t "겹침 block: 병합기 큐를 못 읽으면 소유 유지 — 대기" '[ "$(where o2)" = "wait - -" ] && printf "%s" "$out" | grep -Fq "o1 병합기 큐를 못 읽음" || { printf "%s\n" "$out"; false; }'
ofresh block
oadd o1 '파일: bin/tool'; oadd o2 '파일: bin/tool'; run run; lj box-1/o1:claude-code:o1; run run
printf '2026-11-19T08:00:00Z\tm\tbox-1/o1\to1\t완료\tabc12345\t제출함\t2\n' >"$T/events"
HC_FAKE_MGM=box-2 run run
t "겹침 block: 제출한 레인 터미널이 살아 있고 병합기 착지 전이면 대기" '[ "$(where o2)" = "wait - -" ] || { printf "%s\n" "$out"; false; }'
printf '2026-11-19T09:00:00Z\tbox-2\tbox-2/t9\to1\t착지\tdef45678\t병합기 착지\t-\n' >>"$T/events"
HC_FAKE_MGM=box-2 run run
t "겹침 block: 병합기 기계가 그 이름으로 착지를 적으면 레인 터미널이 살아 있어도 풂, 병합기 기계 사건도 읽음" 'up o2 && grep "^events " "$T/events.log" | grep -Fq -- "--on box-2" || { printf "%s\n" "$out"; cat "$T/events.log"; false; }'
ofresh block; oadd old1 '파일: bin/tool'; oadd new1 '파일: bin/tool'; run run
lj box-1/old1:claude-code:old1
printf '2026-11-20T10:00:00Z\tm\tbox-1/old1\told1\t중간\t-\t작업 중\t1\n2026-11-20T10:02:00Z\tbox-2\tbox-2/t9\told1\t착지\tabcd1234\t제출 ID another\t-\n' >"$T/events"
HC_FAKE_MGM=box-2 run run
t "자기 제출이 없는 실행은 다른 제출 ID의 착지로 소유를 풀지 않음" '[ "$(where new1)" = "wait - -" ]'
# 병합기 1차 중간 10: 설정 한 줄 「merger <프로젝트> <기계>」가 원격 켬 표식을 켜고, 줄을 지우면 끈다(켠 적 있는 프로젝트만)
ofresh warn; echo "merger   app   box-2" >>"$T/conf/hc-fill.tsv"
run run
t "merger 줄: 켬 표식 refs/herdr/merger/box-2 push" 'grep "push" "$T/git.log" | grep -Fq "refs/remotes/origin/HEAD:refs/herdr/merger/box-2" && printf "%s" "$out" | grep -Fq "병합기 app: 켬 → box-2" || { printf "%s\n" "$out"; cat "$T/git.log"; false; }'
: >"$T/git.log"; HC_FAKE_MGM=box-2 run run
t "merger 줄: 이미 켜져 있으면 push 안 함" '! grep -q "push" "$T/git.log"'
sed -i.bak '/^merger/d' "$T/conf/hc-fill.tsv"; HC_FAKE_MGM=box-2 run run
t "merger 줄을 지우면 켬 표식을 지움" 'grep "push" "$T/git.log" | grep -Fq ":refs/herdr/merger/box-2" && printf "%s" "$out" | grep -Fq "병합기 app: 끔" || { printf "%s\n" "$out"; false; }'
# 켬 표식은 원격에 도착했는데 응답이 끊기고 다음 바퀴 전에 설정 줄이 빠졌다.
ofresh warn; echo "merger app box-2" >>"$T/conf/hc-fill.tsv"
HC_FAKE_PUSH_RC=128 run run
sed -i.bak '/^merger/d' "$T/conf/hc-fill.tsv"; : >"$T/git.log"; HC_FAKE_MGM=box-2 run run
t "병합기 켬 응답 유실 뒤 설정 삭제: 원격 표식을 끄고 관리 기록을 지움" 'grep "push" "$T/git.log" | grep -Fq "origin :refs/herdr/merger/box-2" && ! grep -q "^app" "${Q%/*}/merger.tsv"'
# 병합기 2차 중간 9: 대기열이 비고 idle 0이라 일찍 끝나는 바퀴에도 merger 줄을 원격에 옮긴다
ofresh warn; echo "merger   app   box-2" >>"$T/conf/hc-fill.tsv"; sed -i '' 's/^idle .*/idle     0/' "$T/conf/hc-fill.tsv"
run run
t "merger 줄: 빈 대기열·idle 0에도 켬 표식 push" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fq "대기열 비었음" && grep "push" "$T/git.log" | grep -Fq "refs/herdr/merger/box-2" || { printf "%s\n" "$out"; cat "$T/git.log"; false; }'
# 병합기 2차 중간 6: 새로 뜬 레인 터미널이 아직 사건을 하나도 안 적었으면 같은 이름의 옛 병합기 착지로 소유를 풀지 않는다
ofresh block
oadd o1 '파일: bin/tool'; oadd o2 '파일: bin/tool'; run run; lj box-1/o1:claude-code:o1; run run
printf '2026-11-19T09:00:00Z\tbox-2\tbox-2/t9\to1\t착지\tdef45678\t병합기 착지(지난 실행)\t-\n' >"$T/events"
HC_FAKE_MGM=box-2 run run
t "겹침 block: 새 터미널이 사건 없음 — 같은 이름의 옛 병합기 착지로 풀지 않고 대기" '[ "$(where o2)" = "wait - -" ] || { printf "%s\n" "$out"; false; }'
# 병합기 2차 중간 7: 원격이 없는 저장소는 큐가 없다(조회 실패로 읽지 않는다) — 앞 레인 터미널이 사라지면 풀려 뜬다
ofresh block
oadd o1 '파일: bin/tool'; oadd o2 '파일: bin/tool'; run run; lj box-1/o1:claude-code:o1; run run
lj; HC_FAKE_NOORIGIN=1 run run
t "겹침 block: 원격 없는 저장소는 큐 없음 — 앞 레인이 사라지면 풀림" 'up o2 && ! printf "%s" "$out" | grep -Fq "병합기 큐를 못 읽음" || { printf "%s\n" "$out"; false; }'
# 병합기 3차 중간 11: 마지막 사건이 「제출」인 레인은 그 제출 ID가 든 병합기 착지로 푼다 — 같은 초여도
ofresh block
oadd o1 '파일: bin/tool'; oadd o2 '파일: bin/tool'; run run; lj box-1/o1:claude-code:o1; run run
printf '2026-11-19T10:00:00Z\tm\tbox-1/o1\to1\t제출\tabc12345\tbox-1-7 → 큐\t2\n2026-11-19T10:01:00Z\tbox-2\tbox-2/t9\to1\t착지\tdef45678\t제출 ID box-1-70 (갈래점 x)\t-\n' >"$T/events"
HC_FAKE_MGM=box-2 run run
t "병합기 소유: 제출 뒤 시각이어도 ID가 다른 착지는 소유를 풀지 않음" '[ "$(where o2)" = "wait - -" ]'
printf '2026-11-19T10:00:00Z\tm\tbox-1/o1\to1\t제출\tabc12345\tbox-1-20261119T100000-7 → refs/merge-queue/1763546400-o1\t2\n2026-11-19T10:00:00Z\tbox-2\tbox-2/t9\to1\t착지\tdef45678\t병합기 착지 — 제출 abc12345, 제출 ID box-1-20261119T100000-7 (갈래점 x)\t-\n' >"$T/events"
HC_FAKE_MGM=box-2 run run
t "겹침 block: 같은 초라도 그 제출 ID의 병합기 착지면 풂" 'up o2 || { printf "%s\n" "$out"; false; }'
# 병합기 3차 중간 10: ls 마지막 보고 — 제출한 레인은 병합기 기계가 그 제출 ID로 적은 결과(착지·막힘)를 보인다
ofresh warn
add lr3 --only box-1; run run; lj "box-1/lr3:claude-code:lr3:done:/"
printf '2026-11-19T10:00:00Z\tm\tbox-1/lr3\tlr3\t제출\tabc12345\tbox-1-20261119T100000-8 → refs/merge-queue/1763546400-lr3\t2\n2026-11-19T10:05:00Z\tbox-2\tbox-2/t9\tlr3\t착지\tdef45678\t병합기 착지 — 제출 ID box-1-20261119T100000-8\t-\n' >"$T/events"
HC_FAKE_MGM=box-2 TZ=UTC run run; run ls
t "ls 마지막 보고: 제출한 레인은 병합기 착지(sha·시각)를 보임" 'printf "%s\n" "$out" | grep "^lr3 " | grep -Fq "착지 def45678 10:05" || { printf "%s\n" "$out"; cat "$HOME/.local/share/hc-fill/last.tsv"; false; }'
ofresh block
oadd o1 '파일: bin/tool' --only box-1; run run; lj box-1/o1:claude-code:o1; run run
lj; run run   # 연결된 동안 사라져 풀림(대기열 줄은 「끝남」 up 그대로)
mj box-2 shared slow; oadd o2 '파일: bin/tool'; run run
t "겹침 block: 연결된 동안 사라져 풀린 옛 up 줄은 기계가 끊겨도 소유로 안 되살아남(hc-fill 감사 중간 1)" '[ "$(where o1 | cut -d" " -f1)" = up ] && up o2 && ! printf "%s" "$out" | grep -Fq "o1 @box-1 연결 안 됨" || { printf "%s\n" "$out"; false; }'
mj box-1 box-2 shared slow
ofresh block
oadd o1 '파일: bin/tool' --only box-1; run run
lj box-1/o1:claude-code:other-lane; oadd o2 '파일: bin/tool'; run run
t "겹침 block: 대기열 주소를 다른 이름 터미널이 쓰면(주소 재사용) 그 레인은 끝남 — 소유 풂" '[ "$(where o1 | cut -d" " -f3)" = box-1/o1 ] && up o2 || { printf "%s\n" "$out"; cat "$Q"; false; }'
ofresh block
lj "box-2/w1:claude-code:w1:working:/home/me/src/app.wt/w1"; export HC_FAKE_LS_LANE='{"terminals":[{"address":"box-2/w1"}]}'; oadd w1 '파일: bin/tool'; oadd w2 '파일: bin/tool'
run run; unset HC_FAKE_LS_LANE
t "겹침 block: 대기열이 wait인데 같은 이름 터미널이 이미 일하면 그 예약도 소유(아스트라 높음 3)" 'printf "%s" "$out" | grep -Fq "이미 떠 있음 w1" && [ "$(where w2)" = "wait - -" ] && printf "%s" "$out" | grep -Fq "겹침 대기 w2: w1" || { printf "%s\n" "$out"; false; }'
ofresh block
oadd o1 '파일: bin/tool' --only box-1; run run
lj "box-1/o1:claude-code:o1:idle:$OR.wt/o1"; printf 'box-1/o1\t%s\n' $(( $(date +%s) - 7200 )) >"$HOME/.local/share/hc-fill/held.tsv"
oadd o2 '파일: bin/tool'; run run
t "겹침 block: 자리의 hold 만료(쉰 지 오래 — 자리는 안 셈)와 쓰기 소유는 따로 — 소유는 그대로라 대기" 'printf "%s" "$out" | grep -Fq "자리 box-1 box-1/o1 o1: idle, 쉰 지" && [ "$(where o2)" = "wait - -" ] || { printf "%s\n" "$out"; false; }'
ofresh block
oadd o1 '파일: bin/tool'; oadd o3 '파일: lib/b.sh'
run run
t "겹침 block: 안 겹치는 예약은 함께 뜸" 'up o1 && up o3 || { printf "%s\n" "$out"; false; }'
ofresh block
oadd m1 'bin/tool 을 고친다'; oadd m2 '`bin/tool`과 docs/a.md를 본다'
run run
t "겹침 block: 본문 언급끼리는 막지 않고 경고(언급)" 'up m1 && up m2 && printf "%s" "$out" | grep -Fq "겹침 경고 m2: m1 언급 (bin/tool)" || { printf "%s\n" "$out"; false; }'
ofresh block
oadd n1 '파일: bin/new-tool, FR-TRV-005'; oadd n2 '파일: ./bin/./new-tool'; oadd n3 "- 쓰기: \`FR-TRV-005\` 문장"; oadd n4 "파일: $OR/bin/new-tool"
oadd n5 '파일: /home/me/src/app.wt/other-lane/bin/new-tool'; oadd n6 '파일: lib/../bin//new-tool'; oadd n7 '파일: /home/me/src/app/bin/new-tool'
oadd r1 '파일: /Users/x/other.wt/l/bin/new-tool, ../bin/new-tool, /srv/elsewhere/bin/new-tool'
run run
t "겹침 block: 새 파일·FR ID도 예약, ./ · .. · 겹 / · local·원격 repo 줄 절대 경로 · 그 .wt 경로는 저장소 상대로(아스트라 중간 2)" 'up n1 && [ "$(for x in n2 n3 n4 n5 n6 n7; do where $x; done | sort -u)" = "wait - -" ] && printf "%s" "$out" | grep -Fq "겹침 대기 n3: n1 (FR-TRV-005)" || { printf "%s\n" "$out"; false; }'
t "겹침 block: 다른 프로젝트 .wt·저장소 밖(..)·모르는 절대 경로는 버림" 'up r1 || { printf "%s\n" "$out"; false; }'
ofresh block; : >"$OR/docs/안내.md"; : >"$OR/Makefile"
oadd k1 '파일: docs/안내.md (new), lib/c.sh (new)'; oadd k2 '파일: docs/a.md, lib/d.sh (new)'; oadd k3 '파일: `docs/안내.md`'
run run
t "겹침 block: 한글 파일명은 통째로, 뒤 설명 「(new)」은 키가 아님(아스트라 중간 3)" 'up k1 && up k2 && [ "$(where k3)" = "wait - -" ] && printf "%s" "$out" | grep -Fq "겹침 대기 k3: k1 (docs/안내.md)" || { printf "%s\n" "$out"; false; }'
oadd sx1 '- 파일: `bin/tool` (the run section only, see README), e.g. new docs'; oadd sx2 '쓰기: lib/b.sh (only new docs, see README)'
run run
t "겹침 block: 「파일:」 항목의 설명 낱말(see·only·docs·e.g)은 예약 키가 아님(hc-fill 감사 낮음 1)" 'up sx1 && up sx2 && ! printf "%s" "$out" | grep -Fq "겹침 대기 sx2" || { printf "%s\n" "$out"; false; }'
: >"$OR/README.md"; oadd rd1 '- 파일: lib/e.sh, README'; oadd rd2 '파일: `README.md`'
run run
t "겹침 block: 「파일:」의 맨 이름 README는 확장자 붙은 맨 위 파일 README.md로(2차 낮음 1)" 'up rd1 && [ "$(where rd2)" = "wait - -" ] && printf "%s" "$out" | grep -Fq "겹침 대기 rd2: rd1 (README.md)" || { printf "%s\n" "$out"; false; }'
oadd mk1 'Makefile를 고친다'; oadd mk2 '`Makefile` 을 본다'
run run
t "겹침 언급: 확장자 없는 맨 위 실제 파일(Makefile)도, 끝 조사는 뗌" 'up mk1 && up mk2 && printf "%s" "$out" | grep -Fq "겹침 경고 mk2: mk1 언급 (Makefile)" || { printf "%s\n" "$out"; false; }'
ofresh block
oadd d1 '파일: `docs/`'; oadd d2 '파일: docs/a.md'
run run
t "겹침 block: 폴더 예약은 그 아래 파일과 겹침" 'up d1 && [ "$(where d2)" = "wait - -" ] || { printf "%s\n" "$out"; false; }'
ofresh block; echo "overlap_ignore app docs/frag/* CHANGELOG.md" >>"$T/conf/hc-fill.tsv"
oadd g1 '파일: docs/frag/x.md'; oadd g2 '파일: docs/frag/x.md, CHANGELOG.md'
run run
t "겹침: 설정 overlap_ignore의 glob은 겹침으로 안 셈" 'up g1 && up g2 && ! printf "%s" "$out" | grep -Fq 겹침 || { printf "%s\n" "$out"; false; }'
ofresh block
oadd o1 '파일: bin/tool'; run run; lj box-1/o1:claude-code:o1
oadd hi '파일: bin/tool' --priority high; oadd lo '파일: lib/b.sh' --priority low
run run
t "겹침 block: 우선 높은 겹침 대기 줄이 낮은 안 겹치는 줄을 막지 않음" '[ "$(where hi)" = "wait - -" ] && up lo || { printf "%s\n" "$out"; false; }'
run add --name ok --project app --brief "$T/hi.md" --overlap-ok
t "add --overlap-ok: 알림에 「겹침 봄 끔」" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fq "겹침 봄 끔"'
run run
t "--overlap-ok: 겹쳐도 뜸" 'up ok && [ "$(where hi)" = "wait - -" ] || { printf "%s\n" "$out"; false; }'
ofresh block
oadd o1 '파일: bin/tool' --only box-1; run run; lj box-1/o1:claude-code:o1
oadd a1 '파일: bin/tool' --after o1 --only slow; oadd a2 '파일: bin/tool' --after pz --only box-2
run run
t "겹침+먼저 띄움: 선행(--after)이 떠 있으면 그 선행 예약도 막음(선행이 먼저 — 아스트라 높음 1), 다른 소유 레인과도" '[ "$(where a1)" = "wait - -" ] && [ "$(where a2)" = "wait - -" ] && printf "%s" "$out" | grep -Fq "겹침 대기 a1: o1" && printf "%s" "$out" | grep -Fq "겹침 대기 a2: o1" || { printf "%s\n" "$out"; false; }'
ofresh block
oadd e1 '파일: bin/tool' --after p9 --only slow; run run; lj slow/e1:claude-code:e1
t "겹침+먼저 띄움(앞): 선행이 대기열에 없으면 후속을 먼저 띄움" 'up e1 && [ "$(col14 e1)" = early ] || { printf "%s\n" "$out"; false; }'
oadd p9 '파일: bin/tool' --only box-1; run run
t "겹침+먼저 띄움: 나중에 넣은 선행은 그 선행을 기다리는 후속 예약에 막히지 않음(교착 없음)" 'up p9 && ! printf "%s" "$out" | grep -Fq "겹침 대기 p9" || { printf "%s\n" "$out"; false; }'
ofresh block
oadd o1 '파일: bin/tool' --only box-1; run run; lj box-1/o1:claude-code:o1
printf '# 감사\n파일: bin/tool, lib/b.sh\n' >"$T/aud.md"
printf 'audit-o1\tapp\t%s\t-\tbox-1\twait\t-\t-\t-\t2026-11-19T00:00\t-\t-\turgent\tno-early\taudit=o1\n' "$T/aud.md" >>"$Q"
oadd x1 '파일: lib/b.sh'
run run
t "겹침: 읽기 감사 줄은 겹침을 안 보고(그대로 뜸) 같은 바퀴의 쓰기 소유도 아님(아스트라 중간 4)" 'up audit-o1 && up x1 && ! printf "%s" "$out" | grep -Fq "겹침 대기" || { printf "%s\n" "$out"; false; }'
ofresh block
oadd o1 '파일: bin/tool' --only box-1; oadd o2 '파일: bin/tool' --only box-1; run run; lj box-1/o1:claude-code:o1
run run; back 2 700; rm -f "$T/report.log"; run run
t "놀고 있음: 「겹침 대기 n줄」 사유는 그 줄을 받을 수 있는 기계(box-1)에만, 다른 기계는 「띄울 것 없음」(아스트라 중간 5)" 'grep -F "box-1 자리 1개" "$T/report.log" | grep -Fq "(겹침 대기 1줄) — 겹침 대기" && grep -F "대기열에 띄울 것 없음" "$T/report.log" | grep -Fq "box-2 자리 2개" && ! grep -F "대기열에 띄울 것 없음" "$T/report.log" | grep -Fq "box-1 " && ! grep -F "겹침 대기" "$T/report.log" | grep -Fq "box-2 " || { cat "$T/report.log"; printf "%s\n" "$out"; false; }'

# 10k. 겹침 block의 남은 경계(2026-11-20 overlap-guard-2 — 아스트라 2차 감사 높음 2·중간 7·낮음 2, hc-fill 감사(99d653e) 낮음 2).
#      쓰기 소유는 실행 기록(own.tsv: 레인 프로젝트 기계 주소 시작시각 지시문사본 선행): 쓰기 터미널 주소를 받은 때 적고, 그 기계가 연결된 채
#      그 주소에 그 이름 터미널이 없거나 그 터미널이 시작 뒤 착지를 적으면 지운다
OWNF=$HOME/.local/share/hc-fill/own.tsv
ofresh block
export HC_FAKE_LANE_START="box-1/o1=5"
oadd o1 '파일: bin/tool' --only box-1; oadd o2 '파일: bin/tool'
run run; unset HC_FAKE_LANE_START
t "겹침 block(2차 높음 1): 상태가 안 잡혀 wait로 남은 레인도 주소를 받은 때부터 예약 — 같은 바퀴 다른 기계의 겹치는 줄은 대기" '[ "$(where o1 | cut -d" " -f1)" = wait ] && [ "$(where o2)" = "wait - -" ] && printf "%s" "$out" | grep -Fq "겹침 대기 o2: o1 (bin/tool)" || { printf "%s\n" "$out"; false; }'
mj box-2 shared slow; run run
t "겹침 block(2차 높음 1): 다음 바퀴에 그 기계가 끊겨도 남긴(stuck) 터미널의 예약 유지" '[ "$(where o2)" = "wait - -" ] && printf "%s" "$out" | grep -Fq "o1 @box-1 연결 안 됨" || { printf "%s\n" "$out"; false; }'
mj box-1 box-2 shared slow; run run
t "겹침 block(2차 높음 1): 다시 연결되고 그 주소에 터미널이 없으면(종료 확인) 풀려 뜸" 'up o2 || { printf "%s\n" "$out"; false; }'
ofresh block
oadd c '파일: bin/tool' --after b --only slow; run run; lj slow/c:claude-code:c
oadd b '파일: lib/b.sh' --after a; oadd a '파일: bin/tool'
run run
t "겹침+먼저 띄움(2차 높음 2): 먼저 뜬 c(--after b)의 예약은 b의 선행 a(사슬 끝)를 막지 않음 — 교착 없음" 'up c && up a && ! printf "%s" "$out" | grep -Fq "겹침 대기 a:" || { printf "%s\n" "$out"; false; }'
ofresh block
oadd d '파일: bin/tool' --after x2 --only slow; run run; lj slow/d:claude-code:d
oadd p '파일: bin/tool' --only box-1
"$FILL" add --name x2 --project app --brief "$(brief x2)" --then p >/dev/null 2>&1
run run
t "겹침+먼저 띄움(2차 높음 2): --then으로 p에 붙은 x2를 기다리는 d의 예약은 실제 레인 p를 막지 않음" 'up d && up p && ! printf "%s" "$out" | grep -Fq "겹침 대기 p:" || { printf "%s\n" "$out"; cat "$Q"; false; }'
ofresh block
echo "1234abcd pz: 선행" >"$T/commits"
oadd y1 '파일: lib/b.sh' --after pz
export HC_FAKE_FETCH_HOOK="\"$FILL\" rm y1 && printf '# 지시문 y1\n파일: bin/tool\n' >\"$T/y1.md\" && \"$FILL\" add --name y1 --project app --brief \"$T/y1.md\" --only box-2"
run run; unset HC_FAKE_FETCH_HOOK; rm -f "$T/fetch-hook.done"
t "겹침(2차 중간 1): 바퀴 도중 같은 이름 줄이 바뀌면(rm·add) 이번 바퀴엔 띄우지 않고 다음 바퀴에 다시 판정" '[ "$(where y1)" = "wait - -" ] && printf "%s" "$out" | grep -Fq "대기열 줄이 바뀌었다 y1" || { printf "%s\n" "$out"; cat "$Q"; false; }'
run run
ocopy() { awk -F'\t' -v n="$1" '$1==n {print $6; exit}' "$OWNF"; }   # 그 레인 실행 기록의 지시문 사본
t "겹침(2차 중간 1): 다음 바퀴에 새 줄(--only box-2)대로 뜨고, run 지시문 본문 = 검사한 지시문 사본(실행 기록)" '[ "$(where y1 | cut -d" " -f1,2)" = "up box-2" ] && c_=$(ocopy y1) && [ -f "$c_" ] && grep -Fq "bin/tool" "$c_" && head -c "$(wc -c <"$c_")" "$HOME/.local/share/hc-fill/runs/y1-run.md" | cmp -s - "$c_" || { printf "%s\n" "$out"; cat "$OWNF"; false; }'
ofresh block
oadd o1 '파일: bin/tool' --only box-1; run run; lj box-1/o1:claude-code:o1
printf '%s\tm\tbox-1/o1\to1\t착지\tabc12345\t옛 일\t2\n' "$(date -u -r $(( $(date +%s) - 86400 )) +%Y-%m-%dT%H:%M:%SZ)" >"$T/events"
oadd o2 '파일: bin/tool'; run run
t "겹침 block(2차 중간 2): 이름·주소를 다시 쓴 새 실행에 그 실행 전 시각의 착지 사건은 예약을 풀지 않음" '[ "$(where o2)" = "wait - -" ] || { printf "%s\n" "$out"; false; }'
ofresh block
oadd o1 '파일: bin/tool' --only box-1; run run; lj box-1/o1:claude-code:o1
"$FILL" rm o1 >/dev/null 2>&1; oadd o2 '파일: bin/tool'; run run
t "겹침 block(2차 중간 4): hc-fill rm 뒤에도 살아 있는 레인은 쓰기 소유 — 대기" '[ "$(where o2)" = "wait - -" ] && printf "%s" "$out" | grep -Fq "겹침 대기 o2: o1" || { printf "%s\n" "$out"; false; }'
ofresh block
oadd o1 '파일: bin/tool' --only box-1; run run; lj box-1/o1:claude-code:o1
awk -F'\t' -v OFS='\t' '$1=="o1" {$6 = "sent"} 1' "$Q" >"$Q.x" && mv "$Q.x" "$Q"
oadd o2 '파일: bin/tool'; run run
t "겹침 block(2차 중간 4): 결과를 돌려준(sent) 미착지 레인도 터미널이 살아 있으면 쓰기 소유 — 대기" '[ "$(where o2)" = "wait - -" ] || { printf "%s\n" "$out"; false; }'
ofresh block
mkdir -p "$HOME/.local/state/hc-lane"; printf 'box-2/w1\tw1\t-\tread\n' >"$HOME/.local/state/hc-lane/lanes.tsv"
lj "box-2/w1:claude-code:w1:working:/home/me/src/app.wt/w1"; export HC_FAKE_LS_LANE='{"terminals":[{"address":"box-2/w1"}]}'   # 채택은 hc-lane 레인·프로젝트 폴더일 때만
oadd w1 '파일: bin/tool' --only slow; oadd w2 '파일: bin/tool' --only box-1
run run; rm -f "$HOME/.local/state/hc-lane/lanes.tsv"; unset HC_FAKE_LS_LANE
t "겹침 block(2차 중간 4): 레인 기록이 read인 같은 이름 터미널은 쓰기 소유가 아님" 'up w2 && ! printf "%s" "$out" | grep -Fq "겹침 대기 w2" || { printf "%s\n" "$out"; false; }'
ofresh block
oadd f1 '파일: bin'; oadd f2 '파일: bin/tool'
oadd f3 '파일: main.c, .newconfig, NewTool'; oadd f4 '파일: `main.c`'; oadd f5 '쓰기: .newconfig'; oadd f6 '- 파일: NewTool'
run run
t "겹침 block(2차 중간 5): 「파일: bin」(끝 / 없는 맨 위 폴더)은 bin/ 예약 — 그 아래 파일 대기" 'up f1 && [ "$(where f2)" = "wait - -" ] && printf "%s" "$out" | grep -Fq "겹침 대기 f2: f1" || { printf "%s\n" "$out"; false; }'
t "겹침 block(2차 중간 5): 아직 없는 main.c(짧은 확장자)·.newconfig(점 파일)·NewTool(확장자 없음)도 예약" 'up f3 && [ "$(for x in f4 f5 f6; do where $x; done | sort -u)" = "wait - -" ] || { printf "%s\n" "$out"; false; }'
ofresh block
oadd h1 '파일: lib/x.sh (참고 `bin/tool`)'; oadd h2 '파일: bin/tool'; oadd h3 '파일: lib/x.sh'
run run
t "겹침 block(2차 중간 6): 항목 첫 경로가 쓰기 대상 — 뒤 괄호의 참고 경로는 언급(경고만)" 'up h1 && up h2 && printf "%s" "$out" | grep -Fq "겹침 경고 h2: h1 언급 (bin/tool)" || { printf "%s\n" "$out"; false; }'
t "겹침 block(2차 중간 6): 항목 첫 경로(아직 없는 lib/x.sh)는 예약" '[ "$(where h3)" = "wait - -" ] && printf "%s" "$out" | grep -Fq "겹침 대기 h3: h1 (lib/x.sh)" || { printf "%s\n" "$out"; false; }'
ofresh block
oadd j1 'bin/tool·lib/b.sh 를 본다'; oadd j2 '파일: lib/b.sh'
run run
t "겹침(2차 중간 6): 본문의 가운뎃점 목록(bin/tool·lib/b.sh)은 경로마다 언급" 'up j2 && printf "%s" "$out" | grep -Fq "겹침 경고 j2: j1 언급 (lib/b.sh)" || { printf "%s\n" "$out"; false; }'
ofresh block; : >"$OR/README.md"; echo "overlap_ignore app README.md" >>"$T/conf/hc-fill.tsv"
oadd ri1 '파일: README'; oadd ri2 '- 파일: lib/e.sh, README'
run run
t "겹침(hc-fill 감사 낮음): 맨 이름 README를 찾은 README.md도 overlap_ignore를 거침" 'up ri1 && up ri2 && ! printf "%s" "$out" | grep -Fq 겹침 || { printf "%s\n" "$out"; false; }'
ofresh block
oadd o1 '파일: bin/tool' --only box-1; run run; lj box-1/o1:claude-code:o1; run run
rm -f "$OWNF"; mj box-2 shared slow; lj; oadd o2 '파일: bin/tool'; run run
t "겹침 block(hc-fill 감사 낮음): own.tsv가 없으면(새로 깐 첫 바퀴) 연결 안 된 기계의 up 레인은 끝났는지 몰라 소유 유지" '[ "$(where o2)" = "wait - -" ] && printf "%s" "$out" | grep -Fq "o1 @box-1 연결 안 됨" || { printf "%s\n" "$out"; false; }'
mj box-1 box-2 shared slow
ofresh block
oadd o1 '파일: bin/tool' --only box-1; run run; lj box-1/o1:claude-code:o1; run run
sed -i.bak 's/^overlap .*/overlap  off/' "$T/conf/hc-fill.tsv"; lj; run run
sed -i.bak 's/^overlap .*/overlap  block/' "$T/conf/hc-fill.tsv"; mj box-2 shared slow; oadd o2 '파일: bin/tool'; run run
t "겹침 block(hc-fill 감사 낮음): overlap off 동안 끝난 레인은 다시 켜도 낡은 own.tsv로 되살아나지 않음" 'up o2 && ! printf "%s" "$out" | grep -Fq "o1 @box-1 연결 안 됨" || { printf "%s\n" "$out"; false; }'
mj box-1 box-2 shared slow
run nope
t "도움말(2차 낮음 2): --then 병용 금지 목록에 --overlap-ok" 'printf "%s" "$out" | grep -F -- "--no-audit·--coord" | grep -Fq -- "--overlap-ok"'
if [ -d "$here/../config.example" ]; then
  t "README(2차 낮음 2): --then 병용 금지 목록에 --overlap-ok, 한 값 키 목록에 overlap" 'grep -F -- "--no-audit\`·\`--coord" "$here/../README.md" | grep -Fq -- "--overlap-ok" && grep -F "한 값 키(" "$here/../README.md" | grep -Fq "\`overlap\`"'
fi
# 10l. 단독(2026-11-20 지휘 t1cj — conflict-hotspots §4 K: 파일을 가로지르는 이름 바꾸기·sweep 레인이 다른 레인과 겹쳐 modify/delete 충돌):
#      add --solo 줄은 그 프로젝트에 다른 쓰기 소유 레인이 있으면 기다리고, 단독이 떠 있으면 그 프로젝트의 다른 쓰기 줄을 띄우지 않는다.
#      기다리는 줄은 띄우지 않으니 자리를 쥐지 않는다
col16() { awk -F'\t' -v n="$1" '$1==n {print $16}' "$Q"; }
ofresh block
run add --name s1 --project app --brief "$(brief s1)" --solo
t "add --solo: 대기열 16번째 칸 solo, 알림에 「단독」" '[ $rc = 0 ] && [ "$(col16 s1)" = solo ] && printf "%s" "$out" | grep -Fq 단독 || { printf "%s\n" "$out"; false; }'
run add --name s2 --project app --brief "$T/s1.md" --solo --overlap-ok
t "add --solo: --overlap-ok와 함께면 거절" '[ $rc != 0 ] && [ -z "$(where s2)" ]'
run add --name s3 --project app --brief "$T/s1.md" --solo --then s1
t "add --solo: --then과 함께면 거절(종료 2)" '[ $rc = 2 ]'
ofresh warn
oadd o1 '파일: bin/tool' --only box-1; run run; lj box-1/o1:claude-code:o1
oadd s1 '파일: lib/b.sh' --solo; run run
t "단독: 같은 프로젝트에 쓰기 소유 레인(o1)이 떠 있으면 파일이 안 겹쳐도 대기(overlap warn이어도)" '[ "$(where s1)" = "wait - -" ] && printf "%s" "$out" | grep -Fq "단독 대기 s1: o1" || { printf "%s\n" "$out"; false; }'
run ls
t "단독 ls: 비고에 「단독」과 「겹침 대기(o1: 단독)」" 'printf "%s\n" "$out" | grep "^s1 " | grep -Fq "겹침 대기(o1: 단독)" && printf "%s\n" "$out" | grep "^s1 " | grep -Fq 단독 || { printf "%s\n" "$out"; false; }'
lj; run run
t "단독: 앞 레인이 끝나면(종료 확인) 단독이 뜸" 'up s1 || { printf "%s\n" "$out"; false; }'
lj "$(where s1 | cut -d" " -f3):claude-code:s1"
oadd o2 '본문만'; oadd o3 '파일: docs/a.md'; run run
t "단독: 단독이 떠 있으면 그 프로젝트의 다른 쓰기 줄은 파일이 안 겹쳐도 대기" '[ "$(where o2)" = "wait - -" ] && [ "$(where o3)" = "wait - -" ] && printf "%s" "$out" | grep -Fq "단독 대기 o2: s1" || { printf "%s\n" "$out"; false; }'
ofresh block
oadd o1 '파일: bin/tool' --only box-1; run run; lj box-1/o1:claude-code:o1
oadd s1 '파일: lib/b.sh' --solo --only slow --priority high; oadd lo '본문만' --only slow --priority low
"$FILL" add --name t1 --project tools --brief "$(brief t1)" --only here >/dev/null 2>&1
run run
t "단독: 기다리는 단독 줄은 자리를 안 쥠 — 자리 1개 기계(slow)를 뒤 줄이 받음, 다른 프로젝트는 상관없음" '[ "$(where s1)" = "wait - -" ] && [ "$(where lo | cut -d" " -f1,2)" = "up slow" ] && up t1 || { printf "%s\n" "$out"; false; }'
ofresh block
oadd c '파일: bin/tool' --after s1 --only slow; run run; lj slow/c:claude-code:c
oadd s1 '파일: lib/b.sh' --solo; run run
t "단독: 단독 줄을 --after로 기다리며 먼저 뜬 레인은 그 단독을 막지 않음(교착 없음)" 'up s1 || { printf "%s\n" "$out"; false; }'
cp "$EX" "$T/conf/hc-fill.tsv"

# 띄운 직후 중단되고 줄을 지운 뒤 기계까지 끊겨도 원래 실행의 쓰기 예약은 남아야 한다.
ofresh block; oadd writer '파일: bin/tool' --only box-1
HC_FILL_CRASH_AT=after-launch run run
run rm writer; mj box-2; oadd follower '파일: bin/tool' --only box-2; run run
t "띄운 직후 중단·rm·연결 끊김: 다른 기계의 겹치는 작업은 대기" '[ "$(where follower)" = "wait - -" ] && grep -q "^writer" "${Q%/*}/own.tsv"'
mj box-1 box-2; lj "box-1/ret:claude-code:writer:working:/home/me/src/app.wt/writer"; run run
t "재연결하면 대기 중인 쓰기 예약을 실제 주소에 연결" '[ "$(where follower)" = "wait - -" ] && grep -Fq "box-1/ret" "${Q%/*}/own.tsv"'
lj; run run
t "연결된 기계에서 이전 실행이 사라지면 새 작업을 띄움" '[ "$(where follower)" = "up box-2 box-2/follower" ]'
cp "$EX" "$T/conf/hc-fill.tsv"

# 10z. 착지 표식(2026-11-20 fill-dup-guard, 지휘 결정): 선행은 커밋 제목 머리 규칙 또는 그 선행 줄을 넣은 뒤의 착지 표식
# refs/hc-landed/<선행>/<UTC 초>(hc-lane land가 main과 같은 원자 push에)의 sha가 origin/<기본>의 조상일 때 들어감. git 오류면 아직
mark() { mkdir -p "$T/landed"; printf '%s\trefs/hc-landed/%s/%s\n' "$2" "$1" "$3" >>"$T/landed/$1"; }   # mark <레인> <sha> <UTC 초>
later=$(( $(date +%s) + 600 ))
# 실제 사례(carousel-save-race ← main-split): 선행 커밋 제목이 「feat(frontend): main-split-1 …」 꼴이라 제목 머리 규칙에 안 걸려 기한 초과
fresh; rm -rf "$T/landed"; add main-split; add carousel-save-race --after main-split --no-early
printf '%s\n' "aaa1111 feat(frontend): main-split-1 화면 나눔" "bbb2222 test(frontend): main-split-1 시험" >"$T/commits"; run run
t "표식: 제목 머리가 레인 이름이 아닌 커밋(main-split-1)만 있고 표식이 없으면 아직" '[ "$(where carousel-save-race)" = "wait - -" ] && printf "%s" "$out" | grep -q "선행 carousel-save-race ← main-split: 아직"'
mark main-split aaa11110000 1; run run
t "표식: 그 선행 줄을 넣기 전의 옛 표식(같은 이름의 옛 실행)은 안 셈 — 아직" '[ "$(where carousel-save-race)" = "wait - -" ]'
mark main-split aaa11112222 "$later"; export HC_FAKE_LSREMOTE_FAIL=1; run run; unset HC_FAKE_LSREMOTE_FAIL
t "표식: ls-remote git 오류면 판정 보류(풀지 않음)" '[ "$(where carousel-save-race)" = "wait - -" ] && printf "%s" "$out" | grep -q "선행 carousel-save-race ← main-split: 판정 못 함"'
export HC_FAKE_MB_RC=1; run run
t "표식: 표식 sha가 origin/main의 조상이 아니면(merge-base 1) 아직" '[ "$(where carousel-save-race)" = "wait - -" ]'
export HC_FAKE_MB_RC=128; run run; unset HC_FAKE_MB_RC
t "표식: merge-base git 오류(128)도 아직" '[ "$(where carousel-save-race)" = "wait - -" ]'
run run
t "표식: 제목이 「feat(frontend): main-split-1 …」이어도 줄을 넣은 뒤의 표식이 조상이면 들어감(carousel-save-race 사례)" '[ "$(where carousel-save-race | cut -d" " -f1)" = up ] && printf "%s" "$out" | grep -Fq "선행 carousel-save-race ← main-split: 들어감 (aaa11112 — 착지 표식)"'
fresh; rm -rf "$T/landed"; add qx --after zz8 --no-early; mark zz8 cccc0000 "$later"; run run
t "표식: 대기열에 없는 선행은 표식을 안 보고 커밋 제목 머리 규칙만(줄을 넣은 시각이 없다)" '[ "$(where qx)" = "wait - -" ]'
echo "dddd0000 zz8: 끝" >"$T/commits"; run run
t "표식: 제목 머리 규칙은 그대로(표식 없이도 들어감)" '[ "$(where qx | cut -d" " -f1)" = up ] && printf "%s" "$out" | grep -q "선행 qx ← zz8: 들어감 (dddd0000"'
# ls는 마지막 바퀴가 검증한 병합기 표식도 보인다. 진행 중이라는 go만으로는 풀지 않는다.
fresh; rm -rf "$T/landed"; add main-split --only box-1; add show-mark --after main-split --only slow; run run
lj "box-1/main-split:claude-code:main-split" "slow/show-mark:claude-code:show-mark"
run run; run ls
t "ls는 단순 진행 중 상태를 선행 착지로 표시하지 않음" 'printf "%s\n" "$out" | grep "^show-mark " | grep -q "선행 기다림 main-split"'
printf 'abcd1111 feat(frontend): main-split-1 구현\n' >"$T/commits"; mark main-split abcd1111 "$later"; run run; : >"$T/git.log"; run ls
t "ls는 조회한 원격 표식의 로컬 조상을 확인해 실제 착지를 표시" 'printf "%s\n" "$out" | grep "^show-mark " | grep -q "선행 들어감" && ! grep -q "ls-remote" "$T/git.log"'
# 표식 조회 실패는 선행 미착지와 다르다. 먼저 띄울 수 있는 줄도 그 바퀴에는 보류한다.
for fault in 목록 객체; do
  fresh; rm -rf "$T/landed"; add needs-mark --only box-1; add later-mark --after needs-mark --only slow
  if [ "$fault" = 목록 ]; then HC_FAKE_LSREMOTE_FAIL=1 run run
  else mark needs-mark abc12345 "$later"; HC_FAKE_MB_RC=128 run run; fi
  t "표식 $fault 조회 실패 때 먼저 띄우기를 보류" '[ "$(where later-mark)" = "wait - -" ]'
done
# 같은 분의 오래된 표식도 새 대기열 실행의 증거가 될 수 없다.
fresh; rm -rf "$T/landed"; add same-min; add wait-min --after same-min --no-early
mid=$(date -j -f '%Y-%m-%dT%H:%M:%S' '2026-11-20T10:00:00' +%s); rowsec=$((mid+45))
awk -F'\t' -v OFS='\t' -v r="r$rowsec-1-2" '$1=="same-min" {$10="2026-11-20T10:00"; $17=r} 1' "$Q" >"$Q.t" && mv "$Q.t" "$Q"
mark same-min abc1111 "$((rowsec-1))"; run run
t "새 줄을 넣기 1초 전의 같은 분 표식은 보류" '[ "$(where wait-min)" = "wait - -" ]'
mark same-min abc2222 "$rowsec"; run run
t "줄을 넣은 초부터의 표식으로 후속을 풂" '[ "$(where wait-min | cut -d" " -f1)" = up ]'
# 먼저 띄운 레인의 기다림 줄도 같은 판정 — 그 줄을 가짜 git 응답으로 실제로 돌린다(표식 있음·없음·옛 표식·ls-remote 오류·merge-base 1·128)
fresh; rm -rf "$T/landed"; add ea --only box-1; add eb --after ea,ec --only slow; run run
rf=$HOME/.local/share/hc-fill/runs/eb-run.md
t "먼저 띄움 기다림 줄: 대기열 선행(ea)은 제목 머리 또는 refs/hc-landed/ea/* 표식, 대기열 밖(ec)은 제목 머리만" 'grep -Fq "git ls-remote origin '\''refs/hc-landed/ea/*'\''" "$rf" && ! grep -Fq "refs/hc-landed/ec/" "$rf" || { cat "$rf"; false; }'
mkdir -p "$T/eg2"; printf '#!/bin/sh\ncase $1 in fetch) exit 0 ;; log) cat "$T_LOG/subjects" ;; ls-remote) [ ! -f "$T_LOG/lsrfail" ] || exit 128; cat "$T_LOG/lsr" 2>/dev/null; exit 0 ;; merge-base) exit "$(cat "$T_LOG/mbrc" 2>/dev/null || echo 0)" ;; esac\n' >"$T/eg2/git"; chmod +x "$T/eg2/git"
cond=$(awk '/^until /{f=1; sub(/^until /, "")} f && /^do /{exit} f' "$rf")
ew() {  # ew <표식 초|-> <mb 종료> <lsr 실패 1|-> <제목…>: 기다림 조건을 돌려 yes|no
  rm -f "$T/lsr" "$T/lsrfail" "$T/mbrc"; [ "$1" = - ] || printf '9999aaaa0000\trefs/hc-landed/ea/%s\n' "$1" >"$T/lsr"; [ "$2" = 0 ] || echo "$2" >"$T/mbrc"; [ "$3" = - ] || : >"$T/lsrfail"
  shift 3; printf '%s\n' "$@" >"$T/subjects"; PATH="$T/eg2:$PATH" sh -c "if $cond
then echo yes; else echo no; fi" 2>&1
}
t "먼저 띄움 기다림: ea 표식(줄을 넣은 뒤)이 조상이면 ea 제목이 레인 이름이 아니어도 풀림(+ ec 제목)" '[ "$(ew "$later" 0 - "feat(frontend): ea-1 다른 제목" "ec: 고침")" = yes ] || { printf "%s\n" "$cond"; false; }'
t "먼저 띄움 기다림: 표식이 없으면(빈 답·종료 0) 아직" '[ "$(ew - 0 - "feat(frontend): ea-1 다른 제목" "ec: 고침")" = no ]'
t "먼저 띄움 기다림: 줄을 넣기 전의 옛 표식은 안 셈" '[ "$(ew 1 0 - "ec: 고침")" = no ]'
t "먼저 띄움 기다림: ls-remote 오류면 아직" '[ "$(ew "$later" 0 1 "ec: 고침")" = no ]'
t "먼저 띄움 기다림: merge-base 1(조상 아님)·128(오류)이면 아직" '[ "$(ew "$later" 1 - "ec: 고침")" = no ] && [ "$(ew "$later" 128 - "ec: 고침")" = no ]'
t "먼저 띄움 기다림: 제목 머리 규칙은 그대로(ea: …, 표식 없이)" '[ "$(ew - 0 - "ea: 끝" "ec: 고침")" = yes ]'
# 2026-11-20 fill-early-wait-zsh: 표식 판정이 파이프 끝 { …; exit 1; }라 zsh(워커 Bash 도구의 셸)에서는 그 exit가 셸째 끝내
# 기다림이 바로 빠졌다. 블록 전체(until … done)를 zsh·bash로 돌린다 — 가짜 sleep이 한 바퀴 뒤 ea 제목을 넣어 다음 판정에서 풀린다
printf '#!/bin/sh\necho slept; printf "ea: 끝\\n" >>"$T_LOG/subjects"\n' >"$T/eg2/sleep"; chmod +x "$T/eg2/sleep"
blk=$(awk '/^until /{f=1} f; f && /^do /{exit}' "$rf")
eb() { rm -f "$T/lsr" "$T/lsrfail" "$T/mbrc"; printf '%s\n' "ec: 고침" >"$T/subjects"; PATH="$T/eg2:$PATH" "$1" -c "$blk
echo rc=\$?" 2>&1; }
for sh_ in zsh bash; do
  t "먼저 띄움 기다림 블록($sh_): 선행 안 들어감이면 셸이 끝나지 않고 기다림 계속 → 들어가면 풀림" '[ "$(eb $sh_ | paste -sd" " -)" = "slept rc=0" ] || { eb $sh_; printf "%s\n" "$blk"; false; }'
done

# 원격 없는 저장소의 로컬 master에서 선행을 판정하고 같은 참조로 기다린다.
fresh; cp "$EX" "$T/conf/hc-fill.tsv"; export HC_REAL_REPO="$T/로컬 저장소"
"$HC_REAL_GIT" init -qb master "$HC_REAL_REPO" || exit 1
local_commit() { "$HC_REAL_GIT" -C "$HC_REAL_REPO" -c user.name=fixture -c user.email=fixture@example.invalid commit -qm "$1" --allow-empty; }
local_commit 시작 || exit 1
sed -i.bak "s|/Users/me/src/app|$HC_REAL_REPO|" "$T/conf/hc-fill.tsv"
add wait-local --after prep --only here; add next-local --after prep --only here --no-early; run run
rf=$HOME/.local/share/hc-fill/runs/wait-local-run.md
cond=$(awk '/^until /{f=1; sub(/^until /, "")} f && /^do /{exit} f' "$rf" 2>/dev/null)
local_wait() { (cd "$HC_REAL_REPO" && PATH="${PATH#"$T/bin:"}" sh -c "$cond"); }
t "원격 없는 master: 준비만 띄우고 기다림 조건은 거짓" '[ -n "$cond" ] && ! local_wait && [ "$(where next-local)" = "wait - -" ]'
local_commit 'prep: 준비 완료' || exit 1; run run
t "원격 없는 master: 선행 착지 뒤 대기열과 생성한 기다림 조건 모두 풀림" '[ -n "$cond" ] && local_wait && [ "$(where next-local)" = "up here next-local" ] && grep -q "git rebase refs/heads/master" "$rf"'
fresh; echo "audit $EXA" >>"$T/conf/hc-fill.tsv"; add check-local --only here; run run
lj "check-local:claude-code:check-local:done:/"
printf '2026-11-20T10:00:00Z\there\tcheck-local\tcheck-local\t완료\tabc1234\t끝\t1\n' >"$T/events"; run run
t "원격 없는 master: 감사 지시문도 로컬 기준으로 대조" 'grep -Fq "diff refs/heads/master..." "${Q%/*}"/briefs/audit/*.md'
unset HC_REAL_REPO; cp "$EX" "$T/conf/hc-fill.tsv"

# Git이 제목 일부를 출력하고 128로 끝나면 대기열과 생성한 기다림 블록 모두 보류한다.
fresh; add partial --after need --only slow
echo "abcd1234 need: 제목" >"$T/commits"; HC_FAKE_PARTIAL_LOG=1 run run
t "선행 git log 부분 출력 뒤 실패: 본 작업과 먼저 띄우기를 모두 보류" '[ "$(where partial)" = "wait - -" ] && [ ! -s "$T/lane.log" ]'
fresh; add partial --after need --only slow; run run
rf=$HOME/.local/share/hc-fill/runs/partial-run.md
cond=$(awk '/^until /{f=1; sub(/^until /, "")} f && /^do /{exit} f' "$rf")
mkdir -p "$T/partial-bin"; printf '#!/bin/sh\ncase $1 in log) echo "need: 제목"; exit 128 ;; esac\n' >"$T/partial-bin/git"; chmod +x "$T/partial-bin/git"
t "생성한 기다림 조건: git log가 제목 뒤 실패하면 기다림" '[ -n "$cond" ] && ! PATH="$T/partial-bin:$PATH" sh -c "$cond"'

# 10k. 선행 사슬(2026-11-19 fill-dep-needs 사용자 「① 선행 사슬 기다림 (추천)」 — make-url-import 하나가 늦자 뒤 레인들이 3~5시간 기다렸고,
# 먼저 띄움 레인은 선행이 살아 진행 중이어도 기한에 「선행 대기 초과」로 멈춰 지휘를 불렀다)
# ① 기다리는 줄이 있는 선행을 같은 우선순위 안에서 먼저(많을수록 먼저), 그 수를 레인 환경 HERDR_LAND_PRIORITY로
cp "$EX" "$T/conf/hc-fill.tsv"
fresh
add x1 --only slow; add p1 --only slow; add q1 --after p1 --only slow --no-early; add q2 --after p1 --only slow --no-early
run run
t "선행 먼저: 기다리는 줄이 있는 p1이 먼저 넣은 x1보다 먼저" '[ "$(where p1)" = "up slow slow/p1" ] && [ "$(where x1 | cut -d" " -f1)" = wait ] || { printf "%s\n" "$out"; false; }'
t "HERDR_LAND_PRIORITY: 띄울 때 레인 환경에 기다리는 줄 수" 'grep -Fxq "p1 2" "$T/landprio.log" || { cat "$T/landprio.log"; false; }'
lj; : >"$T/landprio.log"; awk -F'\t' -v OFS='\t' '$1=="p1" {$6="sent"} 1' "$Q" >"$Q.t" && mv "$Q.t" "$Q"; run run
t "HERDR_LAND_PRIORITY: 기다리는 줄이 없으면 0" 'grep -Fxq "x1 0" "$T/landprio.log" || { cat "$T/landprio.log"; false; }'
fresh
add pa1 --only slow; add pb1 --only slow; add w1 --after pa1 --only slow --no-early; add w2 --after pb1 --only slow --no-early; add w3 --after pb1 --only slow --no-early
run run
t "선행 먼저: 기다리는 줄이 많은 pb1(2)이 pa1(1)보다 먼저" '[ "$(where pb1)" = "up slow slow/pb1" ] && [ "$(where pa1 | cut -d" " -f1)" = wait ] || { printf "%s\n" "$out"; false; }'
fresh
add pc1 --only slow; add wc --after pc1 --only slow --no-early; add hx --only slow --priority high
run run
t "선행 먼저는 같은 우선순위 안에서만: 높음 hx가 먼저" '[ "$(where hx)" = "up slow slow/hx" ] && [ "$(where pc1 | cut -d" " -f1)" = wait ] || { printf "%s\n" "$out"; false; }'
fresh   # 떠 있는 먼저 띄움 레인도 기다리는 줄로 센다
add z3 --only slow; add p3 --only slow; add e3 --after p3 --only box-1
awk -F'\t' -v OFS='\t' '$1=="e3" {$6="up"; $7="box-1"; $8="box-1/e3"; $14="early"} 1' "$Q" >"$Q.t" && mv "$Q.t" "$Q"
lj box-1/e3:claude-code:e3; run run
t "선행 먼저: 떠 있는 먼저 띄움 레인(e3)이 기다리는 p3이 먼저" '[ "$(where p3)" = "up slow slow/p3" ] && grep -Fxq "p3 1" "$T/landprio.log" || { printf "%s\n" "$out"; cat "$Q"; false; }'
fresh   # --then으로 이어 붙인 이름을 기다리는 줄은 그 이름을 품은 줄의 몫
add x4 --only slow; add p4 --only slow; "$FILL" add --name p4b --project app --brief "$(brief p4b)" --then p4 >/dev/null 2>&1
add w4 --after p4b --only slow --no-early
run run
t "선행 먼저: --then 이름(p4b)을 기다리는 줄이 있으면 그 이름을 품은 p4가 먼저" '[ "$(where p4)" = "up slow slow/p4" ] && [ "$(where x4 | cut -d" " -f1)" = wait ] || { printf "%s\n" "$out"; cat "$Q"; false; }'
fresh   # 자동 감사 줄도 그 선행의 차례(감사 줄끼리는 모두 긴급)
mkdir -p "$(dirname "$Q")"; brief ad-a >/dev/null; brief ad-b >/dev/null; brief c5 >/dev/null
printf 'audit-a5\tapp\t%s\t-\tslow\twait\t-\t-\t-\t2026-01-01T00:00\t-\t-\turgent\tno-early\taudit=a5\n' "$T/ad-a.md" >"$Q"
printf 'audit-b5\tapp\t%s\t-\tslow\twait\t-\t-\t-\t2026-01-01T00:00\t-\t-\turgent\tno-early\taudit=b5\n' "$T/ad-b.md" >>"$Q"
printf 'c5\tapp\t%s\tb5\tslow\twait\t-\t-\t-\t2026-01-01T00:00\t-\t-\tnormal\tno-early\t-\n' "$T/c5.md" >>"$Q"
run run
t "선행 먼저: 기다리는 줄이 있는 b5의 감사 줄이 a5의 감사 줄보다 먼저" '[ "$(where audit-b5)" = "up slow slow/audit-b5" ] && [ "$(where audit-a5 | cut -d" " -f1)" = wait ] || { printf "%s\n" "$out"; cat "$Q"; false; }'

# ② 먼저 띄움 레인 기계에 선행 상태(~/.local/share/hc-fill/after.tsv: 선행 go|stop 까닭 쓴시각)를 바퀴마다 써 준다 — 레인의 기다림
#    블록이 기한에 닿으면 hc-lane early-wait로 이 파일을 보고, 선행이 살아 진행 중이면 계속 기다린다
AF=$HOME/.local/share/hc-fill/after.tsv
afv() { awk -F'\t' -v n="$1" '$1==n {print $2 " " $3; exit}' "$AF" 2>/dev/null; }   # 선행의 판정 까닭
fresh
add pp --only box-1; add ee --after pp --only slow
run run
t "먼저 띄움 준비: pp는 box-1, ee는 slow에 먼저 띄움" '[ "$(where pp)" = "up box-1 box-1/pp" ] && [ "$(where ee)" = "up slow slow/ee" ] || { printf "%s\n" "$out"; false; }'
rm -f "$AF"; : >"$T/hc.log"; lj box-1/pp:claude-code:pp slow/ee:claude-code:ee; run run
t "선행 상태: 선행이 진행 중(working)이면 go, 먼저 띄움 레인 기계(slow)에 hc exec로 씀" 'afv pp | grep -q "^go 진행 중" && grep -q "^exec.*--on slow" "$T/hc.log" && [ "$(awk -F"\t" "{print \$4}" "$AF")" -gt 0 ] || { printf "%s\n" "$out"; cat "$AF" "$T/hc.log"; false; }'
lj box-1/pp:claude-code:pp:idle:/w/app.wt/pp slow/ee:claude-code:ee
printf '2026-11-19T09:00:00Z\tm\tbox-1/pp\tpp\t막힘\t-\t시험 실패\t1\n' >"$T/events"; run run
t "선행 상태: 선행이 쉬고 마지막 사건이 막힘이면 stop 「막힘: …」" 'afv pp | grep -q "^stop 막힘: 시험 실패" || { printf "%s\n" "$out"; cat "$AF"; false; }'
printf '2026-11-19T09:10:00Z\tm\tbox-1/pp\tpp\t완료\t1234abcd\t다 됨\t2\n' >>"$T/events"; run run
t "선행 상태: 쉬어도 마지막 사건이 완료면(감사·착지 기다림) go" 'afv pp | grep -q "^go " || { printf "%s\n" "$out"; cat "$AF"; false; }'
rm -f "$T/events"; run run
t "선행 상태: 사건 없이 막 쉬기 시작했으면 go(쉼)" 'afv pp | grep -q "^go 쉼" || { printf "%s\n" "$out"; cat "$AF"; false; }'
HELD=$HOME/.local/share/hc-fill/held.tsv
awk -F'\t' -v OFS='\t' '$1=="box-1/pp" {$2 -= 61 * 60} 1' "$HELD" >"$HELD.t" && mv "$HELD.t" "$HELD"; run run
t "선행 상태: 사건 없이 hold분 넘게 쉬면 stop" 'afv pp | grep -q "^stop " || { printf "%s\n" "$out"; cat "$AF" "$HELD"; false; }'
lj slow/ee:claude-code:ee; run run
t "선행 상태: 선행 터미널이 사라졌으면 stop 「터미널 없음」" 'afv pp | grep -q "^stop .*터미널 없음" || { printf "%s\n" "$out"; cat "$AF"; false; }'
echo "abcd1234 pp: 끝" >"$T/commits"; run run
t "선행 상태: 선행이 들어갔으면 go 「들어감」" 'afv pp | grep -q "^go 들어감" || { printf "%s\n" "$out"; cat "$AF"; false; }'
: >"$T/commits"; cp "$AF" "$T/af.before"; run run --dry-run
t "선행 상태: dry-run은 쓰지 않음" 'cmp -s "$AF" "$T/af.before"'
fresh   # 선행이 대기열에서 기다림(아직 안 뜸)이면 go, 대기열에 없으면 stop
add pw --only box-1 --after zz --no-early; add ew --after pw --only slow; add ex --after nope9 --only slow
awk -F'\t' -v OFS='\t' '$1=="ew" || $1=="ex" {$6="up"; $7="slow"; $8="slow/" $1; $14="early"} 1' "$Q" >"$Q.t" && mv "$Q.t" "$Q"
lj slow/ew:claude-code:ew slow/ex:claude-code:ex; run run
t "선행 상태: 대기열에서 기다리는 선행은 go, 대기열에 없는 선행은 stop" 'afv pw | grep -q "^go 대기열" && afv nope9 | grep -q "^stop 대기열에 없음" || { printf "%s\n" "$out"; cat "$AF"; false; }'

# ③ 기다림 블록을 그대로 돌린다(가짜 시계: date +%s가 기한 뒤, 가짜 sleep이 두 번째에 선행을 넣음, early-wait는 진짜 hc-lane)
HL=${HC_LANE_BIN:-${FILL%/*}/hc-lane}
fresh
add pp --only box-1; add ee --after pp --only slow; run run
rf=$HOME/.local/share/hc-fill/runs/ee-run.md
blk=$(awk '/^```sh$/ {f=1; next} f && /^```$/ {exit} f' "$rf")
t "먼저 띄움 절: 기한에 닿으면 hc-lane early-wait <선행>으로 선행 상태를 본다" 'printf "%s" "$blk" | grep -Fq "hc-lane early-wait pp" && grep -Fq "선행이 살아 진행 중" "$rf" || { cat "$rf"; false; }'
mkdir -p "$T/clk"; ln -sf "$HL" "$T/clk/hc-lane"
printf '#!/bin/sh\n[ "$1" = +%%s ] && { echo 9999999000; exit 0; }\nexec /bin/date "$@"\n' >"$T/clk/date"
printf '#!/bin/sh\nn=$(cat "$T_LOG/sleeps" 2>/dev/null || echo 0); n=$((n + 1)); echo $n >"$T_LOG/sleeps"\n[ $n -lt 2 ] || echo "pp: 끝" >>"$T_LOG/subjects"\n' >"$T/clk/sleep"
printf '#!/bin/sh\ncase $1 in fetch) exit 0 ;; log) cat "$T_LOG/subjects" ;; esac\n' >"$T/clk/git"; chmod +x "$T/clk/date" "$T/clk/sleep" "$T/clk/git"
blkrun() { rm -f "$T/sleeps"; : >"$T/subjects"; out=$(cd "$T" && PATH="$T/clk:$PATH" perl -e 'alarm 20; exec @ARGV' bash -c "$blk" 2>&1); }
mkdir -p "$(dirname "$AF")"; printf 'pp\tgo\t진행 중(working)\t9999998990\n' >"$AF"; blkrun
t "기다림 블록: 기한이 지나도 선행이 진행 중이면 멈추지 않고, 선행이 들어가면 끝남" '! printf "%s" "$out" | grep -Fq "선행 대기 초과" && [ "$(cat "$T/sleeps")" = 2 ] || { printf "%s\n" "$out"; printf "%s\n" "$blk"; false; }'
printf 'pp\tstop\t막힘: 시험 실패\t9999998990\n' >"$AF"; blkrun
t "기다림 블록: 기한에 선행이 막혔으면 까닭 한 줄과 「선행 대기 초과」로 멈춤" 'printf "%s" "$out" | grep -Fq "pp: 막힘: 시험 실패" && printf "%s" "$out" | grep -Fq "선행 대기 초과" && [ ! -e "$T/sleeps" ] || { printf "%s\n" "$out"; false; }'

# 능력이 있어도 프로젝트 경로나 쓰기 자리가 없는 기계에는 배정할 수 없다.
for invalid in 저장소없음 상한0; do
  fresh; cp "$EX" "$T/conf/hc-fill.tsv"
  case $invalid in 저장소없음) sed -i.bak '/^repo .*app .*slow /d' "$T/conf/hc-fill.tsv" ;;
    상한0) sed -i.bak -E 's/^(machine +slow +)1/\10/' "$T/conf/hc-fill.tsv" ;; esac
  echo 'machine-has slow xcode' >>"$T/conf/hc-fill.tsv"; printf '본문\n필요: xcode\n' >"$T/need.md"
  "$FILL" add --name need --project app --brief "$T/need.md" >/dev/null; run run
  t "능력 기계가 $invalid: 주인에게 막힘을 알림" '[ "$(where need)" = "wait - -" ] && grep -Fq "필요 xcode" "$T/report.log"'
done
cp "$EX" "$T/conf/hc-fill.tsv"

# ④ 기계 능력(fill-dep-needs 사용자 같은 결정 — railway CLI 없는 기계에 웹 배포 레인이 떠 막혔다): 지시문의 「필요: <능력>,…」 줄과
#    설정 machine-has <기계> <능력,…>을 맞춰 능력이 없는 기계에는 띄우지 않는다. 가진 기계가 하나도 없으면 막힘 한 줄·알림 한 번
nb() { printf '# 지시문 %s\n본문\n- 필요: %s\n' "$1" "$2" >"$T/$1.md"; "$FILL" add --name "$1" --project app --brief "$T/$1.md" "${@:3}" >/dev/null 2>&1; }
fresh
printf 'machine-has slow railway,xcode\nmachine-has box-2 xcode\n' >>"$T/conf/hc-fill.tsv"
nb nr railway; nb nx xcode; nb nk kube; add plain
run run
t "필요 railway: 가진 slow에만(앞 기계 box-1이 비어도)" '[ "$(where nr)" = "up slow slow/nr" ] || { printf "%s\n" "$out"; false; }'
t "필요 xcode: 없는 box-1은 건너뛰고 box-2" '[ "$(where nx)" = "up box-2 box-2/nx" ] || { printf "%s\n" "$out"; false; }'
t "필요 줄 없는 레인은 그대로 첫 기계" '[ "$(where plain)" = "up box-1 box-1/plain" ] || { printf "%s\n" "$out"; false; }'
t "필요 kube: 가진 기계 없음 → 대기 그대로, 막힘 한 줄, 주인 지휘에 막힘 사건 한 번" '[ "$(where nk | cut -d" " -f1)" = wait ] && printf "%s" "$out" | grep -q "막힘 nk.*kube" && [ "$(grep -c "막힘 .*--name nk" "$T/report.log")" = 1 ] || { printf "%s\n" "$out"; cat "$T/report.log"; false; }'
run run
t "필요 막힘 알림은 바퀴마다 되풀이하지 않음" '[ "$(grep -c "막힘 .*--name nk" "$T/report.log")" = 1 ] || { cat "$T/report.log"; false; }'
run ls
t "ls: 필요 막힘 비고" 'printf "%s" "$out" | grep "^nk " | grep -Fq "필요 kube" || { printf "%s\n" "$out"; false; }'
printf 'machine-has box-1 kube\n' >>"$T/conf/hc-fill.tsv"; run run
t "설정에 능력을 더하면 다음 바퀴에 뜸" '[ "$(where nk)" = "up box-1 box-1/nk" ] || { printf "%s\n" "$out"; false; }'
nb no railway --only box-1; run run
t "필요 railway + --only box-1: 가진 기계가 기계 제한 밖이면 막힘" '[ "$(where no | cut -d" " -f1)" = wait ] && printf "%s" "$out" | grep -q "막힘 no" || { printf "%s\n" "$out"; false; }'
printf '# 지시문 np\n본문\n필요: 설치와 확인(사람)\n' >"$T/np.md"; "$FILL" add --name np --project app --brief "$T/np.md" --only box-2 >/dev/null 2>&1; run run
t "필요: 뒤가 능력 이름 목록이 아니면(문장) 필요 줄로 안 봄" '[ "$(where np)" = "up box-2 box-2/np" ] || { printf "%s\n" "$out"; false; }'
cp "$EX" "$T/conf/hc-fill.tsv"

# ⑤ 빈 지시문(2026-11-16 travel-ta: 본문 없이 착지 부록만 든 run 지시문으로 레인이 떴다): 제목·부록 절 말고 글 줄이 없으면 거절
fresh
printf '\n### 착지·정리 (hc-lane이 채움)\n- 작업 폴더 /x (브랜치 x)\n1. hc-lane land main\n' >"$T/ap.md"
run add --name ap --project app --brief "$T/ap.md"
t "빈 지시문: 부록(착지·정리 절)만 든 지시문은 add 거절" '[ $rc != 0 ] && [ -z "$(where ap)" ] && printf "%s" "$out" | grep -Fq "본문 없음" || { printf "%s\n" "$out"; false; }'
printf '# 제목만\n\n## 감사·착지 (hc-fill 자동)\n1. x\n' >"$T/ho.md"
run add --name ho --project app --brief "$T/ho.md"
t "빈 지시문: 제목과 감사·착지 절만이면 거절" '[ $rc != 0 ] && [ -z "$(where ho)" ] || { printf "%s\n" "$out"; false; }'
add ok1 --only slow; cp "$T/ap.md" "$HOME/.local/share/hc-fill/briefs/added/ok1.md"; run run
t "빈 지시문: 대기열 사본이 부록만 되면 run도 띄우지 않음" '[ "$(where ok1 | cut -d" " -f1)" = wait ] && [ ! -e "$T/lane.log" ] && printf "%s" "$out" | grep -Fq "실패 ok1" || { printf "%s\n" "$out"; false; }'

# 10y. 기존 결함(2026-11-20 fill-existing-defects — fill-dup-guard 보고 「기존」·아스트라 2차·4차 B에서 넘김)
# 숫자 꼴 이름: awk의 ==는 양쪽이 숫자처럼 보이면 숫자로 견준다(10 == 010 == 1e1) — 이름은 글자로
fresh
add 010 --after zz9 --no-early
run add --name 10 --project app --brief "$(brief 10)"
t "숫자 이름: 010이 있어도 10을 넣음(글자 비교)" '[ $rc = 0 ] && [ "$(grep -c . "$Q")" = 2 ] || { printf "%s\n" "$out"; cat "$Q"; false; }'
run run
t "숫자 이름: 10을 띄워도 010 줄은 대기 그대로" '[ "$(where 010)" = "wait - -" ] && [ "$(where 10 | cut -d" " -f1)" = up ] || { printf "%s\n" "$out"; cat "$Q"; false; }'
run rm 1e1
t "숫자 이름: rm 1e1은 없는 레인(10·010과 다름)" '[ $rc = 1 ] && [ "$(where 010)" = "wait - -" ] && [ -n "$(where 10)" ]'
run rm 10
t "숫자 이름: rm 10은 010을 남김" '[ $rc = 0 ] && [ "$(where 010)" = "wait - -" ] && [ -z "$(where 10)" ] || { printf "%s\n" "$out"; cat "$Q"; false; }'
# 자리·레인 기록은 주소만이 아니라 주소+이름으로(주소는 재사용된다 — 운영 모델): lanes.tsv의 옛 read 줄이 같은 주소의 새 쓰기 레인을 안 세게
fresh
lj "box-1/k1:claude-code:k1:done:$W/app.wt/k1" "box-1/k2:claude-code:k2:idle:$W/app.wt/k2"
mkdir -p "${LT%/*}"; printf 'box-1/k1\taudit-old\tt0\tread\nbox-1/k2\tk2\tt0\tfill\n' >"$LT"
add fd
run run
t "자리: 레인 기록 read가 같은 주소의 다른 이름이면(주소 재사용) 읽기 레인으로 안 봄 → 셈(box-1 가득 → box-2)" '[ "$(where fd)" = "up box-2 box-2/fd" ] && printf "%s" "$out" | grep -Fq "자리 box-1 box-1/k1 k1: done, 작업 트리 있음 → 셈" || { printf "%s\n" "$out"; false; }'
rm -f "$LT"
fresh
lj box-1/a:claude-code:a box-1/b:claude-code:b box-2/a:claude-code:a2 box-2/b:claude-code:b2
add s1; add s2; run run
lj box-1/a:claude-code:a box-1/b:claude-code:b box-2/a:claude-code:a2 box-2/b:claude-code:b2 shared/s1:claude-code:other shared/s2:claude-code:s2
add s3; run run
t "레인상한: 대기열 주소에 다른 이름 터미널(주소 재사용)은 그 레인으로 안 셈 → shared에 셋째" '[ "$(where s3)" = "up shared shared/s3" ] || { printf "%s\n" "$out"; false; }'
fresh; sed -i.bak 's/^early$/early 1/' "$T/conf/hc-fill.tsv"
add ea --after pa --only box-1; add eb --after pa --only box-1; run run
lj box-1/ea:claude-code:other; run run
t "먼저 띄움 상한: 그 주소가 다른 이름이면(주소 재사용) 살아 있는 먼저 띄움으로 안 셈 → eb 띄움" '[ "$(where eb)" = "up box-1 box-1/eb" ] || { printf "%s\n" "$out"; false; }'
cp "$EX" "$T/conf/hc-fill.tsv"
# 7) 「이미 떠 있음」은 hc ls에 있는 기계만 본다 — 끊긴 기계에서 그 이름이 돌던 것은 끝났는지 모른다(목록에 없음 ≠ 끝남): 띄우지 않음
fresh
add hold1 --after zz9 --no-early   # 대기열이 비어 바퀴가 일찍 끝나지 않게
lj "box-1/k9:claude-code:dx:working:/home/me/src/app.wt/dx"; export HC_FAKE_LS_LANE='{"terminals":[{"address":"box-1/k9"}]}'
run run
add dx; mj box-2 shared slow; lj; unset HC_FAKE_LS_LANE
run run
t "끊긴 기계에 지난 바퀴 같은 이름 터미널이 있었으면 띄우지 않음(모름)" '[ "$(where dx)" = "wait - -" ] && printf "%s" "$out" | grep -q "dx.*끊긴 기계 box-1" || { printf "%s\n" "$out"; false; }'
mj box-1 box-2 shared slow; run run
t "다시 붙어 그 이름이 없으면 띄움" '[ "$(where dx | cut -d" " -f1)" = up ] || { printf "%s\n" "$out"; false; }'
# 4차 B 높음 7: run이 대기열을 읽은 뒤(띄우는 동안) 그 레인이 rm·다시 add되면 새 줄을 옛 실행 주소로 덮지 않는다(qset이 이름만 보고 씀)
fresh
add rr
export HC_FAKE_LANE_HOOK="'$FILL' rm rr; '$FILL' add --name rr --project app --brief '$(brief rr2)'"
run run; unset HC_FAKE_LANE_HOOK
t "띄우는 동안 rm·다시 add된 줄은 옛 실행으로 up이 되지 않음(새 줄 그대로 대기)" '[ "$(where rr)" = "wait - -" ] && grep -Fq "# 지시문 rr2" "$(awk -F"\t" "\$1==\"rr\" {print \$3}" "$Q")" || { printf "%s\n" "$out"; cat "$Q"; false; }'
t "줄 ID: add마다 17칸에 새 값(같은 이름 다시 add해도 다름)" '[ -n "$(awk -F"\t" "\$1==\"rr\" {print \$17}" "$Q")" ] && printf "%s" "$out" | grep -q "줄 바뀜 rr" || { printf "%s\n" "$out"; cat "$Q"; false; }'
lj "box-1/rr:claude-code:rr:working:/home/me/src/app.wt/rr"; export HC_FAKE_LS_LANE='{"terminals":[{"address":"box-1/rr"}]}'
run run
t "다음 바퀴에도 교체된 줄은 이전 시도의 터미널을 채택하지 않음" '[ "$(where rr)" = "wait - -" ]'
lj; unset HC_FAKE_LS_LANE; run run
t "옛 실행 정리 뒤 새 줄의 지시문으로 띄움" '[ "$(where rr)" = "up box-1 box-1/rr" ] && grep -Fq "# 지시문 rr2" "${Q%/*}/runs/rr-run.md"'
run rm rr; add rr; lj "box-1/rr:claude-code:rr:working:/home/me/src/app.wt/rr"; export HC_FAKE_LS_LANE='{"terminals":[{"address":"box-1/rr"}]}'
run run
t "띄우기를 마친 뒤 rm·add해도 옛 주소를 새 줄에 채택하지 않음" '[ "$(where rr)" = "wait - -" ]'
unset HC_FAKE_LS_LANE
fresh; add tidy; run run; run rm tidy; run run
t "여러 기계가 연결되면 끝난 시도만 정리" '[ ! -s "${Q%/*}/try.tsv" ] && ! printf "%s" "$out" | grep -q "awk:"'
# 기존 5(감사 줄의 남긴것을 덮으면 그 터미널이 살아나도 중복 위험을 못 본다): 감사 줄은 차수마다 새 이름(audit-<차수 ID>)이라
# 바꿔 넣지 않는다(_put 없음) — 같은 이름을 다시 넣으면 거절하고 옛 줄의 남긴것은 그대로
fresh; mkdir -p "${Q%/*}"
printf 'audit-ax\tapp\t/b\t-\tbox-1\twait\t-\t-\tbox-1=box-1/audit-ax\t2026-11-20T00:00\t-\t-\turgent\tno-early\taudit=x\n' >"$Q"
"$FILL" _add audit-ax app /b2 - box-1 wait - - - 2026-11-20T00:01 - - urgent no-early audit=x 2>/dev/null; rc=$?
t "감사 줄 다시 넣기: 같은 이름은 거절, 옛 줄의 남긴것 그대로" '[ $rc != 0 ] && [ "$(cut -f 9 "$Q")" = box-1=box-1/audit-ax ] && [ "$(cut -f 3 "$Q")" = /b ] && [ "$(grep -c . "$Q")" = 1 ] || { cat "$Q"; false; }'
# 바퀴가 강제 종료되어도 다음 바퀴 뒤에 임시 지시문이 남지 않는다.
fresh; add snap-cut --only box-1; mkdir -p "$T/snapshot-tmp"
TMPDIR="$T/snapshot-tmp" HC_FILL_CRASH_AT=after-launch run run
lj "box-1/snap-cut:claude-code:snap-cut:working:/home/me/src/app.wt/snap-cut"
HC_FAKE_LS_LANE='{"terminals":[{"address":"box-1/snap-cut"}]}' TMPDIR="$T/snapshot-tmp" run run
t "띄운 뒤 강제 종료하고 복구하면 임시 지시문도 회수" 'grep -Fxq after-launch "${Q%/*}/crash.log" && [ "$(where snap-cut | cut -d" " -f1)" = up ] && [ -z "$(find "$T/snapshot-tmp" -name snap-cut.md -print -quit)" ] && [ ! -e "${Q%/*}/.run-brief.tmp" ]'

# 아스트라 2차 ⑤⑥: 띄운 뒤 up을 적기 전에 run이 죽으면 다음 바퀴 채택이 먼저 띄움 표시를 잃고, 이번 실행이 쓴 받은편지 결과를 .old-로 치웠다
fresh
add ck --after pz --only box-1
awk -F'\t' -v OFS='\t' '$1=="ck" {$11 = "box-2:/tmp/x/done/ck.md"} 1' "$Q" >"$Q.t" && mv "$Q.t" "$Q"   # 받은편지 줄로
HC_FILL_CRASH_AT=after-launch run run
t "준비: 띄운 뒤 up 전에 run이 죽음 — 줄은 대기, 시도 기록이 남음" '[ "$(where ck)" = "wait - -" ] && grep -q "^ck	" "$HOME/.local/share/hc-fill/try.tsv" || { printf "%s\n" "$out"; cat "$Q"; false; }'
mkdir -p "$HOME/.local/share/hc-fill/reports"; echo "결과: 고침 abc1234" >"$HOME/.local/share/hc-fill/reports/ck.md"   # 그 실행이 쓴 결과
lj "box-1/ck:claude-code:ck:working:/home/me/src/app.wt/ck"; export HC_FAKE_LS_LANE='{"terminals":[{"address":"box-1/ck"}]}'
run run; unset HC_FAKE_LS_LANE
t "복구: 채택이 시도 기록의 먼저 띄움 표시(early)를 쓰고 실행 주소를 확정" '[ "$(where ck)" = "up box-1 box-1/ck" ] && [ "$(awk -F"\t" "\$1==\"ck\" {print \$14}" "$Q")" = early ] && [ "$(cut -f 6 "$HOME/.local/share/hc-fill/try.tsv")" = box-1/ck ] || { printf "%s\n" "$out"; cat "$Q"; false; }'
t "복구: 같은 실행의 받은편지 보고는 .old-로 치우지 않음" '[ -f "$HOME/.local/share/hc-fill/reports/ck.md" ] && ! ls "$HOME/.local/share/hc-fill/reports/" | grep -q "ck.md.old-" || { printf "%s\n" "$out"; ls "$HOME/.local/share/hc-fill/reports/"; false; }'
# 같은 주소와 이름이었던 지난 실행의 주차·제출 사건을 새 실행에 붙이지 않는다.
fresh; add same-park --after prep --only box-1; run run
lj "box-1/same-park:claude-code:same-park:working:/home/me/src/app.wt/same-park"
printf '2026-11-20T01:00:00Z\tm\tbox-1/same-park\tsame-park\t주차\tabc1234\t옛 주차\t1\t-\tsame-park@1600000000000\n' >"$T/events"
run run
t "옛 실행의 주차는 같은 주소·이름인 새 실행을 주차하지 않음" '[ "$(where same-park)" = "up box-1 box-1/same-park" ]'
ifresh; req "$IS" same-in app - slow; run run; ri=$(rid same-in)
printf '2026-11-20T01:00:00Z\tm\tslow/same-in\tsame-in\t제출\tabc1234\told-id → 큐\t1\t-\tsame-in@1600000000000\n' >"$T/events"
mkdir -p "$RP"; printf '결과: 넘김 현재 요청\n' >"$RP/same-in.$ri.md"
HC_FAKE_GONE=slow/same-in run run
t "터미널이 사라져도 새 요청을 옛 실행의 병합 결과로 막지 않음" '[ "$(lst "$ri")" = sent ] && cmp -s "$RP/same-in.$ri.md" "$IS/done/same-in.md"'
cp "$EX" "$T/conf/hc-fill.tsv"

# 주차(사용자 「노는 워크트리 없이」): 먼저 띄운 레인은 준비 뒤 hc-lane park로 가지를 원격에 두고 끝낸다 → hc-fill이 줄을 주차로 되돌려
# 자리를 비우고 → 선행이 들어가면 hc-lane --resume으로 그 가지에서 다시 띄운다
fresh; rm -f "$T/events"
cp "$EX" "$T/conf/hc-fill.tsv"; printf 'audit %s\naudit_final 최종 확인\n' "$EXA" >>"$T/conf/hc-fill.tsv"
add pk --after pz --only box-1; run run
rf=$HOME/.local/share/hc-fill/runs/pk-run.md
t "먼저 띄움 절: 준비 뒤 hc-lane park → clean → 턴 끝(기다리지 않음), 기다림 블록은 원격 없을 때만" '[ "$(where pk)" = "up box-1 box-1/pk" ] && grep -Fq "hc-lane park" "$rf" && grep -Fq "park가 종료 3(원격 없는 저장소)일 때만" "$rf" || { cat "$rf"; false; }'
lj "box-1/pk:claude-code:pk:working:/home/me/src/app.wt/pk"; export HC_FAKE_LS_LANE='{"terminals":[{"address":"box-1/pk"}]}'
printf '2026-11-20T01:00:00Z\tm\tbox-1/pk\tpk\t주차\tabcdef12\tlane/pk 준비\t1\n' >"$T/events"; run run
t "주차 사건: 원래 터미널이 아직 정리 중이어도 parked를 유지" '[ "$(where pk)" = "wait - -" ] && [ "$(awk -F"\t" "\$1==\"pk\" {print \$14}" "$Q")" = parked ] && printf "%s" "$out" | grep -Fq "주차 pk" && ! printf "%s" "$out" | grep -q "이미 떠 있음 pk" || { printf "%s\n" "$out"; cat "$Q"; false; }'
echo "aaaa1111 pz: 끝" >"$T/commits"; : >"$T/lane.log"; run run
t "선행이 들어가도 원래 주차 터미널이 정리 중이면 이어 띄우기를 미룸" '[ "$(where pk)" = "wait - -" ] && [ ! -s "$T/lane.log" ]'
: >"$T/commits"; lj "box-1/pk:claude-code:pk:idle:/home/me/src/app.wt/pk"
run ls
t "ls: 주차(선행 …)" 'printf "%s\n" "$out" | grep "^pk " | grep -Fq "주차(선행 pz" || { printf "%s\n" "$out"; false; }'
run run
t "주차 줄은 선행 전에 다시 먼저 띄우지 않음" '[ "$(where pk)" = "wait - -" ] || { printf "%s\n" "$out"; false; }'
rm -f "$T/lane.log"; echo "aaaa1111 pz: 끝" >"$T/commits"; lj; unset HC_FAKE_LS_LANE; run run
t "선행 착지 뒤: hc-lane --resume으로 다시 띄우고 「이어 가기」 절(먼저 띄움 절 없음), 먼저 칸 resumed" '[ "$(where pk)" = "up box-1 box-1/pk" ] && grep -Fxq -- --resume "$T/lane.log" && grep -Fq "## 이어 가기" "$rf" && ! grep -Fq "## 먼저 띄움" "$rf" && [ "$(awk -F"\t" "\$1==\"pk\" {print \$14}" "$Q")" = resumed ] || { printf "%s\n" "$out"; cat "$rf"; false; }'
t "이어 띄운 작업 지시문에도 자동 감사와 최종 감사 조건이 있음" 'grep -Fq "## 감사·착지 (hc-fill 자동)" "$rf" && grep -Fq "최종 감사(최종 확인)" "$rf"'
cp "$EX" "$T/conf/hc-fill.tsv"

# 11. 잠금: watch는 한 번에 하나, run도 겹치지 않음
fresh
ln -s "$FILL" "$T/hc-fill"
"$T/hc-fill" watch --every 1 >"$T/watch.out" 2>&1 &
bg=$!
i=0; while [ ! -e "$HOME/.local/share/hc-fill/watch.lock" ] && [ $i -lt 50 ]; do sleep 0.1; i=$((i + 1)); done; sleep 0.3
out=$("$T/hc-fill" watch --every 1 2>&1); rc=$?
t "watch: 이미 돌고 있으면 종료 75" '[ $rc = 75 ] && printf "%s" "$out" | grep -q "이미"'
t "watch: 띄운 뒤 lockf 안쪽까지 둘 이상 돈다(남은 watch 검사가 제 구실을 하는지)" '[ "$(leftw | grep -c .)" -ge 2 ]'
killtree "$bg"; wait "$bg" 2>/dev/null; bg=
i=0; while [ -n "$(leftw)" ] && [ $i -lt 30 ]; do sleep 0.1; i=$((i + 1)); done
out=$(ps -o pid,ppid,command -p "$(leftw | paste -sd, -)" 2>/dev/null)
t "watch를 멈춘 뒤 남은 watch 0(lockf가 띄운 안쪽 루프가 고아로 남지 않음)" '[ -z "$(leftw)" ]'
run ls
t "watch를 멈춘 뒤 ls 정상" '[ $rc = 0 ]'
run nope
t "모르는 명령은 종료 2, 사용법은 README와 같은 말(그 레인에서 빼고)" '[ $rc = 2 ] && printf "%s" "$out" | grep -q "그 기계를 그 레인에서 빼고"'

# SIGPIPE를 무시한 채 물려받은 셸(배경 실행기 등)에서도 newid가 끝난다 — 끝없는 /dev/urandom을 tr로 읽으면 head가 끝나도 tr이 안 죽어 run이 멈췄다
# (제 프로세스 그룹에서 돌리고 10초가 넘으면 그룹째 멈춘다 — 남은 tr이 시험을 붙잡거나 고아로 남지 않게. 출력은 파일로)
nf=$(mktemp "$T/newid.XXXXXX")
perl -e '$SIG{PIPE}="IGNORE"; setpgrp(0, 0); my $p = fork; if (!$p) { exec "bash", "-c", q{eval "$(grep -E "^(b36|newid)\(\)" "$1")"; newid a}, "-", $ARGV[0] }
  $SIG{ALRM} = sub { $SIG{TERM} = "IGNORE"; kill "TERM", -$$; exit 124 }; alarm 10; waitpid($p, 0); exit($? >> 8)' "${FILL%/*}/../lib/hc-fill/common.sh" >"$nf" 2>&1; rc=$?
out=$(cat "$nf")
t "newid: SIGPIPE를 무시해도 10초 안에 끝나고 a<시각>-<6글자>" '[ $rc = 0 ] && printf "%s" "$out" | grep -Eq "^a[0-9a-z]+-[0-9a-z]{6}$"'

echo "# $((n - fails))/$n 통과"
[ $fails = 0 ]
