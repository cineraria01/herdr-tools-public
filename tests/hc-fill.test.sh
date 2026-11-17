#!/bin/bash
# hc-fill 시험: 가짜 hc·hc-lane·git·herdr-balance(PATH 앞)와 저장소의 예시 설정(config.example/hc-fill.tsv)으로.
# 실제 레인·기계·네트워크는 쓰지 않는다. 실행: bash tests/hc-fill.test.sh  (다른 사본은 HC_FILL=<경로>, 설정은 HC_FILL_EXAMPLE=<경로>)
set -u
here=$(cd "$(dirname "$0")" && pwd)
FILL=${HC_FILL:-$here/../bin/hc-fill}
[ -f "$FILL" ] || FILL=$here/../hc-fill
EX=${HC_FILL_EXAMPLE:-$here/../config.example/hc-fill.tsv}
[ -f "$EX" ] || EX=$here/../../share/hc-fill/hc-fill.example.tsv   # 설치된 사본(~/.local/bin/tests → ~/.local/share/hc-fill)
T=$(mktemp -d "${TMPDIR:-/tmp}/hc-fill-test.XXXXXX") && T=$(cd "$T" && pwd -P) || exit 1
bg=
tree() { for c in $(pgrep -P "$1"); do tree "$c"; done; echo "$1"; }   # 손자까지(lockf가 띄운 안쪽 watch가 고아로 남지 않게)
# watch는 $T 안 링크로 띄워 명령 줄에 $T가 들어간다 → 이 시험의 watch만 골라 세고, 끝에 남은 것이 있으면 마저 끈다
leftw() { pgrep -f "$T/hc-fill watch"; }
trap '[ -z "$bg" ] || kill $(tree "$bg") 2>/dev/null; pkill -f "$T/hc-fill watch" 2>/dev/null; rm -rf "$T"' EXIT
mkdir -p "$T/bin" "$T/conf"
cp "$EX" "$T/conf/hc-fill.tsv" || { echo "FAIL - 예시 설정이 없다: $EX"; exit 1; }

# 가짜 hc: ls·machine ls는 $HC_FAKE_LS·$HC_FAKE_MACHINES(JSON). get은 주소가 $HC_FAKE_NOSTATUS(공백 목록)에 있으면 status null.
# exec는 -- 뒤 명령을 그 자리에서 돌린다(받은 표준 입력 그대로). 그 밖의 호출은 $T/hc.log에 남긴다(kill 등이 없어야 한다).
cat >"$T/bin/hc" <<'EOF'
#!/bin/bash
case $1 in
  ls) [ -z "${HC_FAKE_LS_FAIL-}" ] || exit 3; printf '%s\n' "${HC_FAKE_LS:-{\"terminals\":[]\}}" ;;
  machine) printf '%s\n' "$HC_FAKE_MACHINES" ;;
  get) case " ${HC_FAKE_GONE-} " in *" $2 "*) echo '{"error":{"kind":"not_found"}}'; exit 3 ;; esac
       case " ${HC_FAKE_FINISHED-} " in *" $2 "*) echo "{\"address\":\"$2\",\"state\":\"finished\",\"status\":null}"; exit 0 ;; esac
       case " ${HC_FAKE_NOSTATUS-} " in
         *" $2 "*) echo "{\"address\":\"$2\",\"status\":null}" ;;
         *) echo "{\"address\":\"$2\",\"status\":{\"app\":\"claude-code\",\"state\":\"working\"}}" ;;
       esac ;;
  exec) echo "$*" >>"$T_LOG/hc.log"; case "$*" in *"${HC_FAKE_EXEC_FAIL:-@@없음@@}"*) exit 255 ;; esac; while [ "$1" != -- ]; do shift; done; shift; exec "$@" ;;
  *) echo "$*" >>"$T_LOG/hc.log" ;;
esac
EOF
# 가짜 hc-lane: 받은 인자를 한 줄에 하나씩 $T/lane.log에. --print-steps는 착지 줄 대신 표식, 띄우기는 JSON(주소 = <기계>/<이름>).
cat >"$T/bin/hc-lane" <<'EOF'
#!/bin/bash
if [ "${1-}" = coord ]; then [ -n "${HC_FAKE_COORD-}" ] || { echo "hc-lane coord: 지휘 없음(가짜)" >&2; exit 1; }; echo "$HC_FAKE_COORD"; exit 0; fi   # 지휘 정보 확인
if [ "${1-}" = gc ]; then echo "$*" >>"$T_LOG/gc.log"; echo "지움 0 · 남김 0"; exit "${HC_FAKE_GC_RC:-0}"; fi   # gc는 따로 기록
{ echo "---"; for a in "$@"; do echo "$a"; done; } >>"$T_LOG/lane.log"
[ "${1-}" = --print-steps ] || [ "${2-}" = --print-steps ] || [ "${3-}" = --print-steps ] || echo "${HC_LANE_COORD:-없음}" >>"$T_LOG/coord.log"   # 띄울 때 넘긴 지휘(watch 대상)
[ "${1-}" = --print-steps ] || [ "${2-}" = --print-steps ] || [ "${3-}" = --print-steps ] || echo "${HC_LANE_FILL:-없음}" >>"$T_LOG/fill.log"   # hc-fill 레인 표시(lanes.tsv 넷째 칸 fill)
on= name=
while [ $# -gt 0 ]; do case $1 in --on) on=$2; shift 2 ;; --name) name=$2; shift 2 ;; --print-steps) steps=1; shift ;; --) break ;; *) shift ;; esac; done
if [ -n "${steps-}" ]; then echo "### 착지 STEPS $name ${on:-local}"; exit 0; fi
case " ${HC_FAKE_LANE_FAIL-} " in *" ${on:-here}/$name "*) echo "hc-lane: 가짜 실패" >&2; exit 1 ;; esac
# $HC_FAKE_LANE_START의 「<기계>/<이름>=<종료>」: 띄우고 터미널은 남긴 채 시작 확인 실패(5·6·7)로 끝난다(진짜 hc-lane처럼 JSON 끝 줄)
for x in ${HC_FAKE_LANE_START-}; do case $x in "${on:-here}/$name="*)
  echo "hc-lane: 시작 안 됨 ${on:+$on/}$name: 가짜 로그인 막힘" >&2; echo "{\"lane\":\"${on:+$on/}$name\",\"name\":\"$name\",\"start\":\"login\"}"; exit "${x#*=}" ;; esac; done
echo "레인 $name"
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
fresh() { rm -rf "$HOME/.local/share/hc-fill" "$T/lane.log" "$T/hc.log" "$T/balance.log" "$T/git.log" "$T/gc.log" "$T/coord.log"; : >"$T/commits"; mj box-1 box-2 shared slow; lj; unset HC_FAKE_NOSTATUS HC_FAKE_LANE_FAIL HC_FAKE_FETCH_FAIL HC_FAKE_GONE HC_FAKE_EXEC_FAIL HC_FAKE_LANE_START HC_FAKE_FINISHED; }
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

# 4. 선행 조건: origin/<기본>의 커밋 메시지에 레인 이름(단어 경계)
fresh
add pa; add pb --after pa,pc
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
t "받은편지: 받아 대기열에(되돌릴곳 = 요청 기계:폴더/done/이름.md), 같은 바퀴에 띄움" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fq "받음 slow inq-a → 대기열 (app)" && [ "$(where inq-a)" = "up slow slow/inq-a" ] && [ "$(awk -F"\t" "\$1==\"inq-a\" {print \$11}" "$Q")" = "slow:$IS/done/inq-a.md" ]'
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

# 11. 잠금: watch는 한 번에 하나, run도 겹치지 않음
fresh
ln -s "$FILL" "$T/hc-fill"
"$T/hc-fill" watch --every 1 >"$T/watch.out" 2>&1 &
bg=$!
i=0; while [ ! -e "$HOME/.local/share/hc-fill/watch.lock" ] && [ $i -lt 50 ]; do sleep 0.1; i=$((i + 1)); done; sleep 0.3
out=$("$T/hc-fill" watch --every 1 2>&1); rc=$?
t "watch: 이미 돌고 있으면 종료 75" '[ $rc = 75 ] && printf "%s" "$out" | grep -q "이미"'
t "watch: 띄운 뒤 lockf 안쪽까지 둘 이상 돈다(남은 watch 검사가 제 구실을 하는지)" '[ "$(leftw | grep -c .)" -ge 2 ]'
kill $(tree "$bg") 2>/dev/null; wait "$bg" 2>/dev/null; bg=
i=0; while [ -n "$(leftw)" ] && [ $i -lt 30 ]; do sleep 0.1; i=$((i + 1)); done
out=$(ps -o pid,ppid,command -p "$(leftw | paste -sd, -)" 2>/dev/null)
t "watch를 멈춘 뒤 남은 watch 0(lockf가 띄운 안쪽 루프가 고아로 남지 않음)" '[ -z "$(leftw)" ]'
run ls
t "watch를 멈춘 뒤 ls 정상" '[ $rc = 0 ]'
run nope
t "모르는 명령은 종료 2, 사용법은 README와 같은 말(그 레인에서 빼고)" '[ $rc = 2 ] && printf "%s" "$out" | grep -q "그 기계를 그 레인에서 빼고"'

echo "# $((n - fails))/$n 통과"
[ $fails = 0 ]
