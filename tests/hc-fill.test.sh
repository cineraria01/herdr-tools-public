#!/bin/bash
# hc-fill 시험: 가짜 hc·hc-lane·git·herdr-balance(PATH 앞)와 저장소의 예시 설정(config.example/hc-fill.tsv)으로.
# 실제 레인·기계·네트워크는 쓰지 않는다. 실행: bash tests/hc-fill.test.sh  (다른 사본은 HC_FILL=<경로>, 설정은 HC_FILL_EXAMPLE=<경로>)
set -u
here=$(cd "$(dirname "$0")" && pwd)
FILL=${HC_FILL:-$here/../bin/hc-fill}
[ -f "$FILL" ] || FILL=$here/../hc-fill
EX=${HC_FILL_EXAMPLE:-$here/../config.example/hc-fill.tsv}
[ -f "$EX" ] || EX=$here/../../share/hc-fill/hc-fill.example.tsv   # 설치된 사본(~/.local/bin/tests → ~/.local/share/hc-fill)
EXA=${HC_FILL_AUDIT_EXAMPLE:-$here/../config.example/hc-fill-audit.md}   # 자동 감사 예시 틀
[ -f "$EXA" ] || EXA=$here/../../share/hc-fill/hc-fill-audit.example.md
T=$(mktemp -d "${TMPDIR:-/tmp}/hc-fill-test.XXXXXX") && T=$(cd "$T" && pwd -P) || exit 1
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
      case " $* " in *" --label hc-lane=1 "*) e='{"terminals":[]}'; printf '%s\n' "${HC_FAKE_LS_LANE:-$e}"; exit 0 ;; esac   # 라벨 hc-lane=1 터미널만
      printf '%s\n' "${HC_FAKE_LS:-{\"terminals\":[]\}}" ;;
  machine) printf '%s\n' "$HC_FAKE_MACHINES" ;;
  get) case " ${HC_FAKE_GONE-} " in *" $2 "*) echo '{"error":{"kind":"not_found"}}'; exit 3 ;; esac
       case " ${HC_FAKE_FINISHED-} " in *" $2 "*) echo "{\"address\":\"$2\",\"state\":\"finished\",\"status\":null}"; exit 0 ;; esac
       case " ${HC_FAKE_NOSTATUS-} " in
         *" $2 "*) echo "{\"address\":\"$2\",\"status\":null}" ;;
         *) echo "{\"address\":\"$2\",\"status\":{\"app\":\"claude-code\",\"state\":\"working\"}}" ;;
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
if [ "${1-}" = events ]; then echo "$*" >>"$T_LOG/events.log"; cat "$T_LOG/events" 2>/dev/null; exit 0; fi   # 사건: $T/events 그대로
if [ "${1-}" = gc ]; then echo "$*" >>"$T_LOG/gc.log"; echo "지움 0 · 남김 0"; exit "${HC_FAKE_GC_RC:-0}"; fi   # gc는 따로 기록
{ echo "---"; for a in "$@"; do echo "$a"; done; } >>"$T_LOG/lane.log"
[ "${1-}" = --print-steps ] || [ "${2-}" = --print-steps ] || [ "${3-}" = --print-steps ] || echo "${HC_LANE_COORD:-없음}" >>"$T_LOG/coord.log"   # 띄울 때 넘긴 지휘(watch 대상)
[ "${1-}" = --print-steps ] || [ "${2-}" = --print-steps ] || [ "${3-}" = --print-steps ] || echo "${HC_LANE_FILL:-없음}" >>"$T_LOG/fill.log"   # hc-fill 레인 표시(lanes.tsv 넷째 칸 fill)
[ "${1-}" = --print-steps ] || [ "${2-}" = --print-steps ] || [ "${3-}" = --print-steps ] || echo "${HC_LANE_OWNER:-없음}" >>"$T_LOG/owner.log"   # 레인 주인(lanes.tsv 셋째 칸)
on= name=
while [ $# -gt 0 ]; do case $1 in --on) on=$2; shift 2 ;; --name) name=$2; shift 2 ;; --print-steps) steps=1; shift ;; --) break ;; *) shift ;; esac; done
if [ -n "${steps-}" ]; then echo "### 착지 STEPS $name ${on:-local}"; exit 0; fi
case " ${HC_FAKE_LANE_FAIL-} " in *" ${on:-here}/$name "*) echo "hc-lane: 가짜 실패" >&2; exit 1 ;; esac
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
for a in "$@"; do case $a in
  fetch) [ -z "${HC_FAKE_FETCH_FAIL-}" ] || exit 1; exit 0 ;;
  symbolic-ref) echo origin/main; exit 0 ;;
  --grep=*) grep -E -- "${a#--grep=}" "$T_LOG/commits" 2>/dev/null | head -n 1; exit 0 ;;
esac; done
case " $* " in *" log "*) cat "$T_LOG/commits" ;; esac
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
fresh() { rm -rf "$HOME/.local/share/hc-fill" "$T/report.log" "$T/lane.log" "$T/hc.log" "$T/balance.log" "$T/git.log" "$T/gc.log" "$T/coord.log" "$T/owner.log" "$T/coordset.log" "$T/coordcheck.log"; : >"$T/commits"; mj box-1 box-2 shared slow; lj; unset HC_FAKE_LS_LANE HC_FAKE_NOSTATUS HC_FAKE_LANE_FAIL HC_FAKE_FETCH_FAIL HC_FAKE_GONE HC_FAKE_EXEC_FAIL HC_FAKE_LANE_START HC_FAKE_FINISHED HC_FAKE_COORD_GONE HC_FAKE_REPORT_RC HC_FAKE_SEND_FAIL; }
brief() { printf '# 지시문 %s\n본문\n' "$1" >"$T/$1.md"; echo "$T/$1.md"; }
add() { "$FILL" add --name "$1" --project app --brief "$(brief "$1")" "${@:2}" >/dev/null 2>&1; }
where() { awk -F'\t' -v n="$1" '$1==n {print $6, $7, $8}' "$Q"; }   # 상태 기계 주소
launched() { grep -Fxc -- "$1" "$T/lane.log" 2>/dev/null; }          # hc-lane 띄우기 인자 줄 수

# 0. 도구와 설정
t "도구가 있다" '[ -f "$FILL" ]'
fresh
rm "$T/conf/hc-fill.tsv"; run run
t "설정 없음: 종료 1, 설정 경로와 예시 안내" '[ $rc = 1 ] && printf "%s" "$out" | grep -Fq "$T/conf/hc-fill.tsv" && printf "%s" "$out" | grep -Fq config.example'
cp "$EX" "$T/conf/hc-fill.tsv"

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
lj slow/k1:claude-code:dup
add dup
run run
t "같은 이름 살아 있음: hc-lane 안 부르고 떠 있음으로 표시" '[ ! -e "$T/lane.log" ] && [ "$(where dup)" = "up slow slow/k1" ] && printf "%s" "$out" | grep -q "이미 떠 있음 dup"'

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
IS=$T/in-slow IB=$T/in-box2 IH=$HOME/.local/share/hc-fill/inbox RP=$HOME/.local/share/hc-fill/reports
ifresh() { fresh; rm -rf "$IS" "$IB" "$RP"; cp "$EX" "$T/conf/hc-fill.tsv"; printf 'inbox slow %s\ninbox box-2 %s\ninbox shared ~/.local/share/hc-fill/inbox\n' "$IS" "$IB" >>"$T/conf/hc-fill.tsv"; }
req() {  # req <폴더> <이름> <project> [after] [only] — 지시문 먼저, .req는 마지막에 mv
  mkdir -p "$1"; printf '# 요청 %s\n' "$2" >"$1/$2.brief.md"
  { printf 'project\t%s\n' "$3"; [ -z "${4-}" ] || printf 'after\t%s\n' "$4"; [ -z "${5-}" ] || printf 'only\t%s\n' "$5"; } >"$1/$2.req.tmp" && mv "$1/$2.req.tmp" "$1/$2.req"; }
ifresh
mj box-1 shared slow
add dupl
req "$IS" inq-a app - slow; req "$IS" nopr nope; req "$IS" Bad.X app; req "$IS" dupl app; req "$IS" badonly app - box-9
req "$IS" nobrief app; rm "$IS/nobrief.brief.md"
req "$IS" emptyb app; printf ' \n\n' >"$IS/emptyb.brief.md"
req "$IH" inq-h app - box-1
req "$IB" inq-off app
run run --dry-run
t "받은편지 dry-run: 받을·거절할만 찍고 아무것도 옮기지 않음" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fq "[dry-run] 받을 slow inq-a (app)" && printf "%s" "$out" | grep -Fq "[dry-run] 거절할 slow nopr: 설정에 없는 project: nope" && [ -f "$IS/inq-a.req" ] && [ ! -e "$IS/rejected" ] && [ -z "$(where inq-a)" ]'
run run
t "받은편지: 받아 대기열에(되돌릴곳 = 요청 기계:폴더/done/이름.md), 같은 바퀴에 띄움" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fq "받음 slow inq-a → 대기열 (app, 우선 높음)" && [ "$(where inq-a)" = "up slow slow/inq-a" ] && [ "$(awk -F"\t" "\$1==\"inq-a\" {print \$11}" "$Q")" = "slow:$IS/done/inq-a.md" ]'
t "받은편지: 지시문을 briefs/<기계>/<이름>.md로 복사, 요청 쪽 둘은 taken/으로" 'grep -Fq "# 요청 inq-a" "$HOME/.local/share/hc-fill/briefs/slow/inq-a.md" && [ -f "$IS/taken/inq-a.req" ] && [ -f "$IS/taken/inq-a.brief.md" ] && [ ! -e "$IS/inq-a.req" ]'
t "받은편지: run 지시문 끝에 결과 보고 절(보고 경로·결과 줄 꼴)" 'f=$HOME/.local/share/hc-fill/runs/inq-a-run.md; grep -Fq "# 요청 inq-a" "$f" && grep -Fq "~/.local/share/hc-fill/reports/inq-a.md" "$f" && grep -Fq "결과: 고침 <sha>" "$f"'
t "받은편지 거절: 사유 한 줄 rejected/<이름>.why, 둘은 rejected/로" 'grep -Fxq "설정에 없는 project: nope" "$IS/rejected/nopr.why" && [ -f "$IS/rejected/nopr.req" ] && [ -f "$IS/rejected/nopr.brief.md" ] && [ ! -e "$IS/nopr.req" ] && [ -z "$(where nopr)" ]'
t "받은편지 거절: 이름 글자·같은 이름·없는 only 기계·지시문 없음·빈 지시문" 'grep -q "이름은" "$IS/rejected/Bad.X.why" && grep -Fxq "이미 대기열에 있는 이름" "$IS/rejected/dupl.why" && grep -Fq "box-9" "$IS/rejected/badonly.why" && grep -Fq "nobrief.brief.md가 없거나 비었다" "$IS/rejected/nobrief.why" && grep -Fq "emptyb.brief.md가 없거나 비었다" "$IS/rejected/emptyb.why" && [ -f "$IS/rejected/emptyb.req" ] && [ -z "$(where emptyb)" ] && [ "$(grep -c "^dupl	" "$Q")" = 1 ]'
t "받은편지: ~/ 는 그 기계 HOME으로(shared)" '[ -f "$IH/taken/inq-h.req" ] && [ "$(awk -F"\t" "\$1==\"inq-h\" {print \$11}" "$Q")" = "shared:~/.local/share/hc-fill/inbox/done/inq-h.md" ]'
t "받은편지: 연결 안 된 기계는 건너뜀, 요청은 그대로" 'printf "%s" "$out" | grep -Fq "받은편지 건너뜀 box-2: 연결 안 됨" && [ -f "$IB/inq-off.req" ] && [ -z "$(where inq-off)" ]'
t "되돌려 주기: 손으로 넣은 레인(dupl)의 run 지시문에는 결과 보고 절 없음" '! grep -Fq "결과 보고" "$HOME/.local/share/hc-fill/runs/dupl-run.md"'

# 되돌려 주기
lj slow/inq-a:claude-code:inq-a:working box-1/inq-h:claude-code:inq-h:working
req "$IH" inq-h app - box-1
run run
t "되돌려 주기: 레인이 일하는 동안은 쓰지 않음" '[ ! -e "$IS/done/inq-a.md" ] && [ "$(where inq-a | cut -d" " -f1)" = up ]'
t "받은편지: 진행 중(taken/에 있음)인 이름으로 새 요청 → 거절 사유, 대기열 줄 하나" 'grep -Fq "이미 받아 진행 중인 이름" "$IH/rejected/inq-h.why" && [ ! -e "$IH/inq-h.req" ] && [ "$(grep -c "^inq-h	" "$Q")" = 1 ]'
lj slow/inq-a:claude-code:inq-a:done box-1/inq-h:claude-code:inq-h:working
run run
t "되돌려 주기: done인데 보고가 없으면 기다림" '[ ! -e "$IS/done/inq-a.md" ] && [ "$(where inq-a | cut -d" " -f1)" = up ]'
mkdir -p "$RP"; printf '고친 것 설명\n결과: 고침 abc12345\n' >"$RP/inq-a.md"
run run
t "되돌려 주기: done + 보고 → 요청 기계 done/<이름>.md에 그대로, 상태 sent" '[ "$(cat "$IS/done/inq-a.md")" = "$(cat "$RP/inq-a.md")" ] && [ "$(where inq-a | cut -d" " -f1)" = sent ] && printf "%s" "$out" | grep -Fq "되돌려 줌 inq-a → slow:$IS/done/inq-a.md (결과: 고침 abc12345)" && grep -q "^exec -i --on slow " "$T/hc.log"'
rm "$IS/done/inq-a.md"; run run
t "되돌려 주기: sent는 다시 쓰지 않음, ls에 되돌려줌" '[ ! -e "$IS/done/inq-a.md" ] && "$FILL" ls 2>&1 | grep -q "inq-a.*되돌려줌.*받은편지 slow"'
req "$IS" inq-a app - slow
run run
t "받은편지: 끝난(sent) 이름으로 새 요청 → 삼키지 않고 rejected/<이름>.why로 알림" 'grep -Fq "이미 끝난 이름" "$IS/rejected/inq-a.why" && [ -f "$IS/rejected/inq-a.req" ] && [ ! -e "$IS/inq-a.req" ] && [ "$(where inq-a | cut -d" " -f1)" = sent ]'
"$FILL" rm inq-a >/dev/null; rm -rf "$IS/rejected"; req "$IS" inq-a app - slow
run run
t "다시 띄움: 같은 이름의 옛 보고는 띄우기 전에 .old-<시각>으로 비켜 둠" '[ "$(where inq-a)" = "up slow slow/inq-a" ] && [ ! -e "$RP/inq-a.md" ] && ls "$RP" | grep -q "^inq-a.md.old-"'
lj slow/inq-a:claude-code:inq-a:done box-1/inq-h:claude-code:inq-h:working
run run
t "다시 띄움: 옛 보고를 새 레인 결과로 보내지 않음" '[ ! -e "$IS/done/inq-a.md" ] && [ "$(where inq-a | cut -d" " -f1)" = up ]'
mj shared slow
run run
t "되돌려 주기: 레인 기계가 연결 안 됨이면 사라진 것으로 보지 않고 미룸" '[ ! -e "$IH/done/inq-h.md" ] && [ "$(where inq-h | cut -d" " -f1)" = up ]'
mj box-1 shared slow; lj
run run
t "되돌려 주기: hc ls에 없어도 hc get이 not_found가 아니면 실패를 쓰지 않음" '[ ! -e "$IH/done/inq-h.md" ] && [ "$(where inq-h | cut -d" " -f1)" = up ] && printf "%s" "$out" | grep -q "되돌려 주기 미룸 inq-h"'
export HC_FAKE_FINISHED=box-1/inq-h
run run; unset HC_FAKE_FINISHED
t "되돌려 주기: 보고 없이 프로세스가 끝난(finished) 레인도 「결과: 실패 보고 없음」" '[ "$(cat "$IH/done/inq-h.md" 2>/dev/null)" = "결과: 실패 보고 없음" ] && [ "$(where inq-h | cut -d" " -f1)" = sent ]'
rm -f "$IH/done/inq-h.md"; qq=$(awk -F"\t" -v OFS="\t" '$1=="inq-h" {$6="up"} 1' "$Q") && printf '%s\n' "$qq" >"$Q"
export HC_FAKE_GONE=box-1/inq-h
run run
t "되돌려 주기: 보고 없이 터미널이 사라지면 「결과: 실패 보고 없음」" '[ "$(cat "$IH/done/inq-h.md")" = "결과: 실패 보고 없음" ] && [ "$(where inq-h | cut -d" " -f1)" = sent ]'

# 지시문 있음 확인이 한 번 실패해도 거절하지 않음
ifresh
req "$IS" inq-c app - slow
export HC_FAKE_EXEC_FAIL='then echo yes'
run run; unset HC_FAKE_EXEC_FAIL
t "받은편지: 지시문 확인 실패는 거절이 아니라 다음 바퀴에" '[ ! -e "$IS/rejected" ] && [ -f "$IS/inq-c.req" ] && printf "%s" "$out" | grep -q "inq-c: 지시문을 확인하지 못했다, 다음 바퀴에"'
run run
t "받은편지: 다음 바퀴에 받음" '[ -f "$IS/taken/inq-c.req" ] && [ "$(where inq-c | cut -d" " -f1)" = up ]'

# 넣은 뒤 taken/으로 옮기지 못한 요청: 다음 바퀴에 거절하지 않고 옮기기만
ifresh
req "$IS" inq-r app - slow
mkdir -p "$(dirname "$Q")"
printf 'inq-r\tapp\t%s\t-\tslow\twait\t-\t-\t-\t2026-01-01T00:00\tslow:%s/done/inq-r.md\n' "$(brief inq-r)" "$IS" >"$Q"
run run --dry-run
run run
t "받은편지: 이미 넣은 요청은 taken/으로 옮기기만(거절·중복 줄 없음)" '[ -f "$IS/taken/inq-r.req" ] && [ ! -e "$IS/rejected" ] && [ "$(grep -c "^inq-r	" "$Q")" = 1 ]'

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
t "되풀이 상한: 30분마다 세 번" '[ "$(grep -c . "$T/report.log")" = 3 ] || { cat "$T/report.log"; cat "$IT"; false; }'
back 3 1900; run run
t "되풀이 상한: 세 번 뒤로는 30분 지나도 안 보냄" '[ "$(grep -c . "$T/report.log")" = 3 ]'
back 3 7300; run run
t "되풀이 상한: 2시간 지나면 다시" '[ "$(grep -c . "$T/report.log")" = 4 ] || { cat "$IT"; false; }'
t "되풀이 상한: 넷 다 먼저 띄움 상한 사유" '[ "$(grep -c "먼저 띄움 상한(early 1)에 걸려 남김" "$T/report.log")" = 4 ] || { cat "$T/report.log"; false; }'
cp "$EX" "$T/conf/hc-fill.tsv"
fresh   # 사유: 먼저 띄움 상한에 걸려 남긴 자리면 그렇게
sed -i.bak 's/^early$/early 0/' "$T/conf/hc-fill.tsv"
run run; back 2 700; run run
t "사유: 먼저 띄움 상한에 안 걸렸으면 「대기열에 띄울 것 없음」" 'grep -Fq "대기열에 띄울 것 없음" "$T/report.log"'
sed -i.bak 's/^early 0$/early 1/' "$T/conf/hc-fill.tsv"
add ca --after pa --only box-1; add cb --after pa --only box-1; rm -f "$T/report.log"; run run
t "사유: 대기열 빔으로 한 번 보낸 뒤 상한으로 바뀌면 30분 안이어도 그 사유로 다시 — 「먼저 띄움 상한(early 1)에 걸려 남김」" 'grep -Fq "먼저 띄움 상한(early 1)에 걸려 남김" "$T/report.log" && ! grep -Fq "대기열에 띄울 것 없음" "$T/report.log" || { cat "$T/report.log"; printf "%s\n" "$out"; false; }'
capn() { awk -F'\t' '$6=="cap" {n++; if ($4 != 1) bad++} END {print n + 0, bad + 0}' "$IT"; }   # 상한 사유 줄 수 · 보낸 횟수가 1이 아닌 줄 수
t "사유가 바뀌어 보내면 보낸 횟수는 1부터(감사 낮음 1 — 상한 사유도 30분 간격 세 번을 다 받게)" '[ "$(capn | cut -d" " -f2)" = 0 ] && [ "$(capn | cut -d" " -f1)" -ge 1 ] || { cat "$IT"; false; }'
cp "$EX" "$T/conf/hc-fill.tsv"
fresh; run run --dry-run
t "놀고 있음: dry-run은 기록·알림 없음" '[ ! -e "$IT" ] && [ ! -e "$T/report.log" ]'

# 10h. 자동 감사(2026-11-19 fill-auto-audit): 설정 audit 틀이 있으면 쓰기 레인의 「완료 <sha>」 → 그 기계에 긴급 감사 줄 →
#      판정(높음·중간 0)이면 레인에 「감사 통과」, 아니면 「감사 지적」(audit_max번까지), 넘으면 주인 지휘에 막힘. 가짜 events는 $T/events
AT=$HOME/.local/share/hc-fill/audit.tsv
LT=$HOME/.local/state/hc-lane/lanes.tsv   # hc-lane 레인 기록(가짜 hc-lane은 안 씀 — 시험이 적는다)
afresh() { fresh; rm -f "$T/events" "$T/events.log" "$T/send.log" "$LT";cp "$EX" "$T/conf/hc-fill.tsv"; echo "audit    $EXA" >>"$T/conf/hc-fill.tsv"; ek=0; }
ev() { ek=$((ek + 1)); printf '2026-11-19T%02d:00:00Z\tm\t%s\t%s\t%s\t%s\t%s\t1\n' "$ek" "$@" >>"$T/events"; }   # ev <터미널> <이름> <상태> <sha> <한 줄>
col15() { awk -F'\t' -v n="$1" '$1==n {print $15}' "$Q"; }
arow() { awk -F'\t' -v n="$1" '$1==n {print $2, $3, $4, $5, $6}' "$AT" 2>/dev/null; }   # sha 몇번째 지적수 상태 실패
ABR=$HOME/.local/share/hc-fill/briefs/audit
mkdir -p "$W/app.wt/w1" "$W/app.wt/w2"

afresh
add w1 --only slow
run run
rf=$HOME/.local/share/hc-fill/runs/w1-run.md
t "자동 감사: 쓰기 레인 run 지시문 끝에 「감사·착지 (hc-fill 자동)」 절, 감사 칸 auto" '[ "$(where w1)" = "up slow slow/w1" ] && [ "$(col15 w1)" = auto ] && grep -Fq "## 감사·착지 (hc-fill 자동)" "$rf" && grep -Fq "hc-lane report 완료 <sha>" "$rf" || { printf "%s\n" "$out"; cat "$rf"; false; }'
t "자동 감사: 완료 사건 전에는 감사 줄 없음" '[ -z "$(where audit-w1)" ] && [ ! -e "$AT" ]'
lj "slow/w1:claude-code:w1:done:$W/app.wt/w1"
ev slow/w1 w1 중간 - "반쯤"; ev - w1 막힘 - "hc-fill이 남긴 것(다른 터미널)"; ev slow/w1 w1 완료 abc1234 "끝"
run run
t "완료 → 그 기계에 긴급 감사 줄 audit-<레인>(주인·우선·감사 칸), 같은 바퀴에 띄움" '[ "$(where audit-w1)" = "up slow slow/audit-w1" ] && [ "$(col15 audit-w1)" = audit=w1 ] && [ "$(prio_of audit-w1)" = urgent ] && printf "%s" "$out" | grep -Fq "감사 넣음 audit-w1 ← w1 abc1234 (1번째, slow)" || { printf "%s\n" "$out"; cat "$Q"; false; }'
t "감사 줄 띄우기: hc-lane --read, --repo는 그 레인 작업 트리, events는 그 기계 --on" 'grep -Fxq -- --read "$T/lane.log" && grep -Fxq /Users/other/src/app.wt/w1 "$T/lane.log" && grep -Fq "events --on slow" "$T/events.log" || { cat "$T/lane.log"; false; }'
ab=$ABR/audit-w1.md
t "감사 지시문: 틀 자리를 다 채움(작업 트리·sha·기본·원 지시문·보고서)" 'grep -Fq "git -C /Users/other/src/app.wt/w1 diff origin/main...abc1234" "$ab" && grep -Fq "~/.local/share/hc-fill/runs/w1-run.md" "$ab" && grep -Fq "~/.local/share/hc-fill/reports/audit-w1.md" "$ab" && grep -Fq "감사 완료 abc1234 높음" "$ab" && ! grep -q "{[a-z]*}" "$ab" || { cat "$ab"; false; }'
t "감사 줄의 run 지시문에는 감사·착지 절 없음(읽기 레인)" '! grep -Fq "감사·착지 (hc-fill 자동)" "$HOME/.local/share/hc-fill/runs/audit-w1-run.md"'
t "자리: 감사에 넘겨진 쓰기 레인(done)은 안 셈 — 자리 1인 slow에 감사가 뜸" 'printf "%s" "$out" | grep -Fq "자리 slow slow/w1 w1: done, 자동 감사 기다림(자리는 감사 줄이) → 안 셈"'
t "audit.tsv: 레인 sha 1번째 지적0 queued" '[ "$(arow w1)" = "abc1234 1 0 queued 0" ] || { cat "$AT"; false; }'
run ls
t "ls: 감사 중 · 감사 대상" 'printf "%s\n" "$out" | grep "^w1 " | grep -Fq "감사 중(1번째)" && printf "%s\n" "$out" | grep "^audit-w1 " | grep -Fq "감사 대상 w1" || { printf "%s\n" "$out"; false; }'
: >"$T/lane.log"; lj "slow/w1:claude-code:w1:done:$W/app.wt/w1" "slow/audit-w1:claude-code:audit-w1:working:$W/app.wt/w1"
run run
t "같은 sha는 두 번 넣지 않음" '! grep -Fxq -- --name "$T/lane.log" && ! printf "%s" "$out" | grep -Fq "감사 넣음" && [ "$(grep -c "^audit-w1	" "$Q")" = 1 ]'
lj "slow/w1:claude-code:w1:done:$W/app.wt/w1" "slow/audit-w1:claude-code:audit-w1:done:$W/app.wt/w1"
ev slow/audit-w1 audit-w1 판정 - "감사 완료 abc1234 높음 0·중간 0·낮음 2"
run run
t "높음0·중간0 → 레인에 「감사 통과」 hc send, audit.tsv pass" 'grep -Fxq "send slow/w1 감사 통과 (abc1234, 낮음 2 — ~/.local/share/hc-fill/reports/audit-w1.md). run 지시문의 착지 절차대로 main에 넣어라." "$T/send.log" && [ "$(arow w1)" = "abc1234 1 0 pass 0" ] || { printf "%s\n" "$out"; cat "$T/send.log"; false; }'
t "자리: 판정 뒤 감사 레인(done)은 안 셈" 'printf "%s" "$out" | grep -Fq "자리 slow slow/audit-w1 audit-w1: done, 자동 감사 레인 → 안 셈"'
run run
t "통과는 한 번만 보냄, ls에 감사 통과 보냄" '[ "$(grep -c . "$T/send.log")" = 1 ] && "$FILL" ls 2>&1 | grep "^w1 " | grep -Fq "감사 통과 보냄"'
# 판정 보낸 뒤 감사 터미널 끄기(2026-11-19 지휘 추가 명세 — 감사 터미널이 레인 작업 트리를 cwd로 남아 hc-lane clean이 멈춤)
t "레인 기록 종류가 read가 아니면(기록 없음) 감사 터미널을 끄지 않음" '! grep -q "^kill" "$T/hc.log" 2>/dev/null'
mkdir -p "${LT%/*}"; printf 'slow/audit-w1\taudit-w1\t-\tread\nslow/w1\tw1\t-\tfill\n' >"$LT"
lj "slow/w1:claude-code:w1:done:$W/app.wt/w1" "slow/audit-w1:claude-code:audit-w1:working:$W/app.wt/w1"
run run
t "판정 뒤에도 감사 터미널이 working이면 끄지 않음(다음 바퀴에)" '! grep -q "^kill" "$T/hc.log" 2>/dev/null'
lj "slow/w1:claude-code:w1:done:$W/app.wt/w1" "slow/audit-w1:claude-code:audit-w1:done:$W/app.wt/w1"
run run --dry-run
t "dry-run: 「끌 감사 터미널」만 찍고 끄지 않음" 'printf "%s" "$out" | grep -Fq "[dry-run] 끌 감사 터미널 audit-w1 slow/audit-w1" && ! grep -q "^kill" "$T/hc.log" 2>/dev/null'
run run
t "판정 보낸 뒤 done 감사 터미널을 정확한 주소로 끔(레인 터미널은 안 끔)" '[ "$(grep "^kill" "$T/hc.log")" = "kill slow/audit-w1" ] && printf "%s" "$out" | grep -Fq "감사 터미널 끔 audit-w1 slow/audit-w1" || { printf "%s\n" "$out"; cat "$T/hc.log"; false; }'

# 지적 → 고침 → 다시 감사(새 sha, 줄 덮어 씀) → 지적 → 상한이면 주인 지휘에 막힘
afresh
"$FILL" add --name w2 --project app --brief "$(brief w2)" --only box-1 --coord tq1 >/dev/null 2>&1
run run
lj "box-1/w2:claude-code:w2:done:$W/app.wt/w2"; ev box-1/w2 w2 완료 aaaa111 "1차"
run run
t "주인 지휘가 있는 레인: 감사 줄 주인도 그 지휘" '[ "$(awk -F"\t" "\$1==\"audit-w2\" {print \$12}" "$Q")" = tq1 ]'
ev box-1/audit-w2 audit-w2 판정 - "감사 완료 aaaa111 높음 1·중간 0·낮음 0"
lj "box-1/w2:claude-code:w2:done:$W/app.wt/w2" "box-1/audit-w2:claude-code:audit-w2:done:$W/app.wt/w2"
run run
t "지적 → 「감사 지적」 보냄, 지적수 1, ls 「감사 지적 1번째」" 'grep -Fxq "send box-1/w2 감사 지적 (aaaa111, 높음 1·중간 0) — ~/.local/share/hc-fill/reports/audit-w2.md를 읽고 높음·중간을 고친 뒤 다시 검증하고 hc-lane report 완료 <새 sha>." "$T/send.log" && [ "$(arow w2)" = "aaaa111 1 1 fix 0" ] && "$FILL" ls 2>&1 | grep "^w2 " | grep -Fq "감사 지적 1번째" || { printf "%s\n" "$out"; cat "$T/send.log" "$AT"; false; }'
: >"$T/lane.log"; ev box-1/w2 w2 완료 bbbb222 "고침"
run run
t "새 sha 완료 → 같은 이름 감사 줄을 새 바퀴로 덮어 씀(2번째, 지난 감사 터미널을 떠 있음으로 잡지 않음)" '[ "$(grep -c "^audit-w2	" "$Q")" = 1 ] && [ "$(where audit-w2 | cut -d" " -f1)" = up ] && grep -Fxq audit-w2 "$T/lane.log" && [ "$(arow w2)" = "bbbb222 2 1 queued 0" ] && grep -Fq "bbbb222" "$ABR/audit-w2.md" || { printf "%s\n" "$out"; cat "$Q" "$AT"; false; }'
t "옛 바퀴의 판정(다른 sha)은 새 감사의 판정으로 안 봄" '[ "$(grep -c . "$T/send.log")" = 1 ]'
ev box-1/audit-w2 audit-w2 판정 - "감사 완료 bbbb222 높음 0·중간 1·낮음 3"
run run
t "2번째 지적(지적수 1 < audit_max 2) → 지적 보냄, 지적수 2" '[ "$(grep -c "감사 지적" "$T/send.log")" = 2 ] && [ "$(arow w2)" = "bbbb222 2 2 fix 0" ] || { printf "%s\n" "$out"; cat "$AT"; false; }'
ev box-1/w2 w2 완료 cccc333 "또 고침"; run run
ev box-1/audit-w2 audit-w2 판정 - "감사 완료 cccc333 높음 2·중간 0·낮음 0"
run run
t "상한(지적 2번) → 주인 지휘에 hc-lane report 막힘 「<레인> 감사 3번째 … 지휘 판단」, 보내지 않음" 'grep -Fq "report 막힘 - w2 감사 3번째 높음 2·중간 0 — 지휘 판단" "$T/report.log" && grep -Fq -- "--name w2 --to tq1" "$T/report.log" && [ "$(grep -c . "$T/send.log")" = 2 ] && [ "$(arow w2 | cut -d" " -f4)" = coord ] || { printf "%s\n" "$out"; cat "$T/report.log"; false; }'
run run
t "지휘로 넘긴 뒤 되풀이 없음, ls 「지휘로 넘김」" '[ "$(grep -c . "$T/report.log")" = 1 ] && "$FILL" ls 2>&1 | grep "^w2 " | grep -Fq "지휘로 넘김"'

# 판정 줄을 못 읽으면 지휘로(주인 없음 → --to 없이 모든 지휘)
afresh; add w3 --only box-1; run run
lj "box-1/w3:claude-code:w3:done:/"; ev box-1/w3 w3 완료 dddd444 "끝"; run run
ev box-1/audit-w3 audit-w3 판정 - "감사 끝 — 괜찮음"; run run
t "판정 줄을 못 읽음 → 모든 지휘에 막힘(--to 없음)" 'grep -Fq "report 막힘 - w3 감사 1번째 판정 줄을 못 읽음" "$T/report.log" && ! grep -q -- --to "$T/report.log" && [ ! -e "$T/send.log" ] || { printf "%s\n" "$out"; cat "$T/report.log"; false; }'

# 보내기 실패 → 다음 바퀴에 다시, 세 번이면 지휘로
afresh; add w4 --only box-1; run run
lj "box-1/w4:claude-code:w4:done:/"; ev box-1/w4 w4 완료 eeee555 "끝"; run run
ev box-1/audit-w4 audit-w4 판정 - "감사 완료 eeee555 높음 0·중간 0·낮음 0"
export HC_FAKE_SEND_FAIL=box-1/w4; run run; unset HC_FAKE_SEND_FAIL
t "hc send 실패 → 상태 그대로, 실패 1, 한 줄" '[ "$(arow w4)" = "eeee555 1 0 queued 1" ] && printf "%s" "$out" | grep -Fq "감사 판정 보내기 미룸 w4: hc send 실패 — 다음 바퀴에 다시(1/3)" || { printf "%s\n" "$out"; cat "$AT"; false; }'
run run
t "다음 바퀴에 다시 보냄" '[ "$(arow w4)" = "eeee555 1 0 pass 0" ] && grep -Fq "send box-1/w4 감사 통과" "$T/send.log"'
afresh; add w5 --only box-1; run run
lj "box-1/w5:claude-code:w5:done:/"; ev box-1/w5 w5 완료 ffff666 "끝"; run run
ev box-1/audit-w5 audit-w5 판정 - "감사 완료 ffff666 높음 0·중간 0·낮음 0"
lj; run run; run run; run run
t "레인 터미널이 사라짐 세 바퀴 → 지휘로 넘김" '[ ! -e "$T/send.log" ] && grep -Fq "report 막힘 - w5 감사 판정(높음 0·중간 0)을 레인에 세 번 못 보냄(레인 터미널 box-1/w5 없음)" "$T/report.log" && [ "$(arow w5 | cut -d" " -f4)" = coord ] || { printf "%s\n" "$out"; cat "$AT" "$T/report.log"; false; }'

# 감사 대상이 아닌 줄: --no-audit · 받은편지 · audit 키 없음
afresh
run add --name na --project app --brief "$(brief na)" --only box-1 --no-audit
t "add --no-audit: 감사 칸 no-audit, 알림에 「자동 감사 끔」" '[ $rc = 0 ] && [ "$(col15 na)" = no-audit ] && printf "%s" "$out" | grep -Fq "자동 감사 끔"'
run run; lj "box-1/na:claude-code:na:done:/"; ev box-1/na na 완료 abcdef7 "끝"; run run
t "--no-audit 줄: 감사·착지 절 없음, 완료해도 감사 줄 없음" '! grep -Fq "감사·착지" "$HOME/.local/share/hc-fill/runs/na-run.md" && [ -z "$(where audit-na)" ] && [ "$(col15 na)" = no-audit ] || { printf "%s\n" "$out"; false; }'
ifresh; echo "audit    $EXA" >>"$T/conf/hc-fill.tsv"; rm -f "$T/events"
mj box-1 shared slow; req "$IS" inq-z app - slow; run run
lj "slow/inq-z:claude-code:inq-z:done:/"; printf '2026-11-19T01:00:00Z\tm\tslow/inq-z\tinq-z\t완료\t1234abc\t끝\t1\n' >"$T/events"; run run
t "받은편지 줄: 감사·착지 절 없음, 완료해도 감사 줄 없음" '[ "$(where inq-z | cut -d" " -f1)" = up ] && ! grep -Fq "감사·착지" "$HOME/.local/share/hc-fill/runs/inq-z-run.md" && [ -z "$(where audit-inq-z)" ] || { printf "%s\n" "$out"; false; }'
fresh; rm -f "$T/events" "$T/events.log"; cp "$EX" "$T/conf/hc-fill.tsv"
add nk --only box-1; run run; lj "box-1/nk:claude-code:nk:done:/"; printf '2026-11-19T01:00:00Z\tm\tbox-1/nk\tnk\t완료\t1234abc\t끝\t1\n' >"$T/events"; run run
t "audit 키 없음: 절 없음, events 안 봄, 감사 줄·기록 없음" '! grep -Fq "감사·착지" "$HOME/.local/share/hc-fill/runs/nk-run.md" && [ ! -e "$T/events.log" ] && [ -z "$(where audit-nk)" ] && [ ! -e "$AT" ] && [ "$(col15 nk)" != auto ]'

# 긴 이름 · 지난 감사가 도는 중의 새 완료 · dry-run · 틀 못 읽음
afresh; ln=abcdefghijklmnopqrstuvwxyz0123; add $ln --only box-1; run run
lj "box-1/$ln:claude-code:$ln:done:/"; ev box-1/$ln $ln 완료 1111aaa "끝"; run run
an=$(awk -F'\t' '$15 ~ /^audit=/ {print $1}' "$Q")
t "32자 넘는 감사 이름은 audit-<앞 21자>-<4자리 16진>(32자)" '[ "${#an}" = 32 ] && case $an in audit-abcdefghijklmnopqrstu-????) true ;; *) false ;; esac && [ "$(col15 "$an")" = "audit=$ln" ] || { echo "이름: $an"; printf "%s\n" "$out"; false; }'
afresh; add w6 --only box-1; run run
lj "box-1/w6:claude-code:w6:done:/" "box-1/audit-w6:claude-code:audit-w6:working:/"; ev box-1/w6 w6 완료 2222bbb "끝"; run run
ev box-1/w6 w6 완료 3333ccc "더 고침"; run run
t "지난 감사가 아직 돌면 새 완료는 판정 뒤에(감사 둘이 함께 안 돎)" 'printf "%s" "$out" | grep -Fq "자동 감사 미룸 w6: 새 완료 3333ccc — 지난 감사(2222bbb)가 아직 돈다" && [ "$(arow w6)" = "2222bbb 1 0 queued 0" ]'
ev box-1/audit-w6 audit-w6 판정 - "감사 완료 2222bbb 높음 0·중간 0·낮음 0"
lj "box-1/w6:claude-code:w6:done:/" "box-1/audit-w6:claude-code:audit-w6:done:/"; run run
t "옛 sha의 판정은 보내지 않고 새 sha로 다시 감사(2번째)" '[ ! -e "$T/send.log" ] && [ "$(arow w6)" = "3333ccc 2 0 queued 0" ] || { printf "%s\n" "$out"; cat "$AT"; false; }'
afresh; add w7 --only box-1; run run
lj "box-1/w7:claude-code:w7:done:/"; ev box-1/w7 w7 완료 4444ddd "끝"
before=$(cat "$Q"); run run --dry-run
t "dry-run: 「넣을 감사」만 찍고 대기열·기록·지시문 그대로" 'printf "%s" "$out" | grep -Fq "[dry-run] 넣을 감사 audit-w7 ← w7 4444ddd (1번째, box-1)" && [ "$(cat "$Q")" = "$before" ] && [ ! -e "$AT" ] && [ ! -e "$ABR/audit-w7.md" ]'
run run; ev box-1/audit-w7 audit-w7 판정 - "감사 완료 4444ddd 높음 0·중간 0·낮음 1"; lj "box-1/w7:claude-code:w7:done:/" "box-1/audit-w7:claude-code:audit-w7:done:/"
run run --dry-run
t "dry-run: 「보낼 …」만 찍고 보내지 않음" 'printf "%s" "$out" | grep -Fq "[dry-run] 보낼 w7 box-1/w7: 감사 통과 (4444ddd" && [ ! -e "$T/send.log" ] && [ "$(arow w7 | cut -d" " -f4)" = queued ]'
# 재감사 지적(1차 감사 반려 5bc7134 높음 1·중간 2·낮음 4)
# [높음 1] 레인상한 1인 기계: 감사를 기다리는 레인이 레인상한을 채워 자기 감사 줄을 못 띄우던 교착
afresh; sed -i '' 's/^machine  box-1    2   -$/machine  box-1    2   1/' "$T/conf/hc-fill.tsv"
add l1 --only box-1; run run
lj "box-1/l1:claude-code:l1:done:/"; ev box-1/l1 l1 완료 5555eee "끝"; run run
t "레인상한 1인 기계에서 완료 → 감사 줄이 뜬다(감사 줄은 레인상한을 안 봄)" '[ "$(where audit-l1)" = "up box-1 box-1/audit-l1" ] || { printf "%s\n" "$out"; cat "$Q"; false; }'
add l2 --only box-1; lj "box-1/l1:claude-code:l1:done:/" "box-1/audit-l1:claude-code:audit-l1:working:/"; run run
t "레인상한 1에서 감사를 기다리는 동안 다른 쓰기 줄은 대기(감사 기다리는 레인은 레인 수에 듦 — 지적 뒤 둘이 함께 돌지 않게, 재재감사 [중간] 1)" '[ "$(where l2)" = "wait - -" ] || { printf "%s\n" "$out"; false; }'
# [재감사 중간 2] 감사 터미널이 끝났는데(판정을 못 보냄·판정 없음) 상태가 queued면 레인을 다시 자리로 센다
afresh; mkdir -p "$W/app.wt/m1"; add m1 --only box-1; run run
lj "box-1/m1:claude-code:m1:done:$W/app.wt/m1"; ev box-1/m1 m1 완료 dddd777 "끝"; run run
ev box-1/audit-m1 audit-m1 판정 - "감사 완료 dddd777 높음 1·중간 0·낮음 0"
lj "box-1/m1:claude-code:m1:done:$W/app.wt/m1" "box-1/audit-m1:claude-code:audit-m1:done:$W/app.wt/m1"
add m2 --only box-1; add m3 --only box-1
export HC_FAKE_SEND_FAIL=box-1/m1; run run; unset HC_FAKE_SEND_FAIL
t "보내기 실패 바퀴: 감사가 끝났으면 레인을 다시 셈 — 기계 상한 2에 m2 하나만 뜸(m3 대기)" '[ "$(arow m1 | cut -d" " -f4,5)" = "queued 1" ] && [ "$(where m2 | cut -d" " -f1)" = up ] && [ "$(where m3)" = "wait - -" ] || { printf "%s\n" "$out"; cat "$Q"; false; }'
# [중간 2] 감사 터미널이 판정 없이 끝나거나 사라지면 몇 바퀴 뒤 지휘로
afresh; add v1 --only box-1; run run
lj "box-1/v1:claude-code:v1:done:/"; ev box-1/v1 v1 완료 6666fff "끝"; run run
lj "box-1/v1:claude-code:v1:done:/" "box-1/audit-v1:claude-code:audit-v1:done:/"; ev box-1/audit-v1 audit-v1 완료 - "판정 대신 완료로 적음"
run run; run run
t "판정 없이 done 두 바퀴: 아직 지휘로 안 넘김" '[ ! -e "$T/report.log" ] && [ "$(arow v1 | cut -d" " -f4)" = queued ]'
run run
t "판정 없이 done 세 바퀴 → 「<레인> 감사 1번째 판정 없음 — 지휘 판단」, 지휘로 넘김" 'grep -Fq "report 막힘 - v1 감사 1번째 판정 없음 — 지휘 판단" "$T/report.log" && [ "$(arow v1 | cut -d" " -f4)" = coord ] || { printf "%s\n" "$out"; cat "$AT"; false; }'
afresh; add v2 --only box-1; run run
lj "box-1/v2:claude-code:v2:done:/"; ev box-1/v2 v2 완료 7777aaa "끝"; run run
lj "box-1/v2:claude-code:v2:done:/"; run run; run run; run run
t "감사 터미널이 판정 없이 사라짐 세 바퀴 → 지휘로" 'grep -Fq "report 막힘 - v2 감사 1번째 판정 없음" "$T/report.log" && [ "$(arow v2 | cut -d" " -f4)" = coord ] || { printf "%s\n" "$out"; cat "$AT"; false; }'
afresh; add v3 --only box-1; run run
lj "box-1/v3:claude-code:v3:done:/"; ev box-1/v3 v3 완료 8888bbb "끝"; run run
lj "box-1/v3:claude-code:v3:done:/" "box-1/audit-v3:claude-code:audit-v3:working:/"; run run; run run; run run
t "감사가 working이면 판정 없음으로 세지 않음" '[ ! -e "$T/report.log" ] && [ "$(arow v3 | cut -d" " -f4)" = queued ]'
# [중간 3] 같은 이름을 다시 넣으면 audit.tsv 옛 기록(지적수)을 물려받지 않음
afresh
"$FILL" add --name r1 --project app --brief "$(brief r1)" --only box-1 >/dev/null 2>&1; run run
mkdir -p "$(dirname "$AT")"; printf 'r1\tcccc333\t3\t2\tcoord\t0\n' >"$AT"   # 지난 일: 지적 2번 뒤 지휘로 넘김
"$FILL" rm r1 >/dev/null; "$FILL" add --name r1 --project app --brief "$(brief r1)" --only box-1 >/dev/null 2>&1; run run
t "같은 이름을 새로 띄우면 그 레인의 audit.tsv 옛 기록을 지움" '[ -z "$(arow r1)" ] && ! "$FILL" ls 2>&1 | grep "^r1 " | grep -Fq "지휘로 넘김" || { cat "$AT"; false; }'
lj "box-1/r1:claude-code:r1:done:/"; ev box-1/r1 r1 완료 9999ccc "새 일"; run run
ev box-1/audit-r1 audit-r1 판정 - "감사 완료 9999ccc 높음 0·중간 1·낮음 0"; lj "box-1/r1:claude-code:r1:done:/" "box-1/audit-r1:claude-code:audit-r1:done:/"; run run
t "새 일의 첫 감사는 1번째, 중간 1 → 지적(옛 지적수로 바로 지휘로 넘기지 않음)" 'grep -Fq "send box-1/r1 감사 지적 (9999ccc" "$T/send.log" && [ "$(arow r1)" = "9999ccc 1 1 fix 0" ] && [ ! -e "$T/report.log" ] || { printf "%s\n" "$out"; cat "$AT"; false; }'
# [3차 감사 낮음 2] 그 기계에서 상태가 안 잡혀 남긴(stuck) 감사 줄은 자리를 쥔 것으로 보지 않음 — 레인을 다시 셈
afresh; mkdir -p "$W/app.wt/q1"; add q1 --only box-1; run run
lj "box-1/q1:claude-code:q1:done:$W/app.wt/q1"; ev box-1/q1 q1 완료 eeee777 "끝"
export HC_FAKE_NOSTATUS=box-1/audit-q1; run run; unset HC_FAKE_NOSTATUS
run run
t "stuck 감사 줄(그 레인 기계에 못 뜸): 감사 기다리는 레인을 다시 자리로 셈" '[ "$(where audit-q1)" = "wait - -" ] && printf "%s" "$out" | grep -Fq "자리 box-1 box-1/q1 q1: done, 작업 트리 있음 → 셈" || { printf "%s\n" "$out"; cat "$Q"; false; }'
# [낮음 1] 그 주소의 이름이 레인 이름일 때만 보냄
afresh; add n1 --only box-1; run run
lj "box-1/n1:claude-code:n1:done:/"; ev box-1/n1 n1 완료 aaaa999 "끝"; run run
ev box-1/audit-n1 audit-n1 판정 - "감사 완료 aaaa999 높음 0·중간 0·낮음 0"
lj "box-1/n1:claude-code:other:done:/" "box-1/audit-n1:claude-code:audit-n1:done:/"; run run
t "레인 주소에 다른 이름 터미널이 서 있으면 보내지 않고 「레인 터미널 없음」" '[ ! -e "$T/send.log" ] && printf "%s" "$out" | grep -Fq "레인 터미널 box-1/n1 없음" || { printf "%s\n" "$out"; false; }'
# [낮음 2] 감사 터미널 끄기는 감사 줄 기계의 터미널만
afresh; add k1 --only box-1; run run
lj "box-1/k1:claude-code:k1:done:/"; ev box-1/k1 k1 완료 bbbb999 "끝"; run run
ev box-1/audit-k1 audit-k1 판정 - "감사 완료 bbbb999 높음 0·중간 0·낮음 0"
mkdir -p "${LT%/*}"; printf 'box-1/audit-k1\taudit-k1\t-\tread\nbox-2/tz\taudit-k1\t-\tread\n' >"$LT"
lj "box-1/k1:claude-code:k1:done:/" "box-1/audit-k1:claude-code:audit-k1:working:/" "box-2/tz:claude-code:audit-k1:done:/"; run run
t "다른 기계의 같은 이름 감사 터미널은 끄지 않음" '! grep -q "^kill" "$T/hc.log" 2>/dev/null || { cat "$T/hc.log"; false; }'
# [낮음 4] 완료에 sha 없음 → 지휘에 한 번 · add는 audit- 머리 이름 거절
afresh; add s1 --only box-1; run run
lj "box-1/s1:claude-code:s1:done:/"; ev box-1/s1 s1 완료 - "sha 빠뜨림"; run run; run run
t "완료에 sha 없음 → 지휘에 「자동 감사 못 함 <레인>: 완료에 sha 없음」 한 번" '[ "$(grep -c "자동 감사 못 함 s1: 완료에 sha 없음" "$T/report.log" 2>/dev/null)" = 1 ] && [ -z "$(where audit-s1)" ] || { printf "%s\n" "$out"; cat "$T/report.log"; false; }'
ev box-1/s1 s1 완료 cccc999 "다시"; run run
t "그 뒤 sha가 있는 완료는 그대로 감사" '[ "$(where audit-s1 | cut -d" " -f1)" = up ]'
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

echo "# $((n - fails))/$n 통과"
[ $fails = 0 ]
