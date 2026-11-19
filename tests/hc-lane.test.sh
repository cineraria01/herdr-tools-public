#!/bin/bash
# hc-lane 시험: 임시 bare origin + clone, 가짜 hc(PATH 앞, 받은 인자를 기록). 실제 hc·네트워크는 쓰지 않는다.
# 실행: bash ~/.local/bin/tests/hc-lane.test.sh   (다른 사본은 HC_LANE_BIN=<경로>. 같은 폴더의 hc-lane이 있으면 그것)
# (HC_LANE은 레인 이름이기도 해서 시험할 사본은 HC_LANE_BIN으로 준다. 옛 HC_LANE=<경로>는 파일일 때만 받는다)
set -u
here=$(cd "$(dirname "$0")" && pwd)
LANE=${HC_LANE_BIN:-}
[ -n "$LANE" ] || { [ -f "${HC_LANE:-}" ] && LANE=$HC_LANE; }
[ -n "$LANE" ] || LANE=$here/hc-lane
[ -f "$LANE" ] || LANE=$here/../bin/hc-lane   # 저장소(tests/ 옆 bin/)
[ -f "$LANE" ] || LANE=$here/../hc-lane       # 깔린 사본(~/.local/bin/tests/)
T=$(mktemp -d "${TMPDIR:-/tmp}/hc-lane-test.XXXXXX") && T=$(cd "$T" && pwd -P) || exit 1
trap 'kill $(jobs -p) 2>/dev/null; rm -rf "$T"' EXIT
W="$T/공백 있는 폴더"   # 모든 저장소를 공백·한글 경로 아래에 둔다
mkdir -p "$T/bin" "$W"
# 가짜 hc: run 인자는 $HCLOG, exec 인자는 $HCLOG.exec에 한 줄씩. exec는 -- 뒤 명령을 그 자리에서 돌린다(--on 시험용).
# get은 기록하지 않고, 이름이 $HC_FAKE_LIVE면 살아 있는 터미널로 답한다. $HC_FAKE_RUN_FAIL이면 run이 실패한다.
cat >"$T/bin/hc" <<'EOF'
#!/bin/bash
if [ "$1" = get ]; then   # $HC_FAKE_GET_TURN이면 그 turn을 가진 터미널로 답한다(report의 turn 칸)
  echo "$2" >>"$HCLOG.get"
  [ ! -e "$HCLOG.getdown" ] || { echo "error: cannot reach the core" >&2; exit 1; }   # 코어 연결 실패(「없음」이 아님)
  # 주소가 $HC_FAKE_GET_ADDR이면 $HC_FAKE_GET_BODY(JSON 한 줄)를 그대로(메인 폴더 앞당김의 status 없는 터미널 판정 시험)
  [ -z "${HC_FAKE_GET_ADDR-}" ] || [ "$2" != "$HC_FAKE_GET_ADDR" ] || { printf '%s\n' "$HC_FAKE_GET_BODY"; exit 0; }
  # $HC_FAKE_STATUS_AGE초 전에 status가 바뀐 쉬는 터미널(wait 유예 시험)
  # $HC_FAKE_PID가 있으면 그 터미널 프로그램 pid도(배경 작업 프로세스 판정 시험). 앞의 foreground pid는 다른 값
  [ -z "${HC_FAKE_STATUS_AGE-}" ] || { echo "{\"address\":\"$2\",\"foreground\":{\"pid\":1},\"state\":\"live\",\"status\":{\"state\":\"done\"},\"status_ms\":$((($(date +%s) - HC_FAKE_STATUS_AGE) * 1000)),\"turn\":1${HC_FAKE_PID:+,\"pid\":$HC_FAKE_PID}}"; exit 0; }
  [ -z "${HC_FAKE_GET_SLEEP-}" ] || exec sleep "$HC_FAKE_GET_SLEEP"   # 응답 없는 코어(alarm이 이 프로세스를 끝내 고아가 안 남게 exec)
  # $HC_FAKE_GET_NAME이면 맨 위 name도(앞의 foreground.name은 다른 값 — 진짜 hc 꼴)
  [ -z "${HC_FAKE_GET_TURN-}" ] || { echo "{\"address\":\"$2\",\"foreground\":{\"name\":\"node\",\"pid\":1}${HC_FAKE_GET_NAME:+,\"name\":\"$HC_FAKE_GET_NAME\"},\"state\":\"live\",\"status\":{\"app\":\"x\",\"msg\":\"m\",\"state\":\"working\"},\"turn\":$HC_FAKE_GET_TURN}"; exit 0; }
  # 띄운 터미널(t9zz): 기본은 status가 잡힘. HC_FAKE_NOSTATUS=1 늘 null, =untilkeys 키를 받기 전까지 null, =finished 끝남
  case $2 in *t9zz)
    case ${HC_FAKE_NOSTATUS-} in
      '') s='{"state":"working"}' ;;
      untilkeys) s=null; [ -e "$HCLOG.keys" ] && s='{"state":"working"}' ;;
      finished) echo "{\"address\":\"$2\",\"state\":\"finished\",\"status\":null}"; exit 0 ;;
      *) s=null ;;
    esac
    echo "{\"address\":\"$2\",\"name\":\"x\",\"state\":\"live\",\"status\":$s,\"turn\":0}"; exit 0 ;;
  esac
  # 살아 있는 지휘 여럿: $HCLOG.live 파일의 「주소 pid」 줄(배경 watch가 도중에 바뀌게 파일로). 이름은 주소와 같게
  if [ -f "$HCLOG.live" ]; then p_=$(awk -v a="$2" '$1 == a { print $2 }' "$HCLOG.live"); [ -z "$p_" ] || { echo "{\"address\":\"$2\",\"foreground\":{\"name\":\"node\",\"pid\":7},\"name\":\"$2\",\"pid\":$p_,\"state\":\"live\"}"; exit 0; }; fi
  # 「mac/…」는 이 기계 자신의 이름이 붙은 주소 꼴: 진짜 hc처럼 machine_unknown
  case $2 in mac/*) echo "{\"error\":{\"kind\":\"machine_unknown\",\"message\":\"unknown machine \\\"mac\\\"\"}}" >&2; exit 4 ;; esac
  # 살아 있는 터미널(앞의 foreground pid·name은 다른 값 — 진짜 hc 꼴). pid는 $HC_FAKE_LIVE_PID(기본 4242)
  [ "$2" = "${HC_FAKE_LIVE-}" ] && { echo "{\"address\":\"t1aa\",\"foreground\":{\"name\":\"node\",\"pid\":7},\"name\":\"$2\",\"pid\":${HC_FAKE_LIVE_PID:-4242},\"state\":\"live\",\"status\":{\"state\":\"idle\"}}"; exit 0; }
  # 레인 터미널(라벨 hc-lane=1): 지휘 정보로 적지 않아야 한다
  [ "$2" = "${HC_FAKE_LANETERM-}" ] && { echo "{\"address\":\"$2\",\"labels\":{\"hc-lane\":\"1\"},\"name\":\"lane-x\",\"pid\":5555,\"state\":\"live\"}"; exit 0; }
  [ "$2" = "${HC_FAKE_ID-}" ] && { echo "{\"address\":\"$2\",\"name\":\"other\",\"state\":\"live\"}"; exit 0; }
  echo '{"error":{"kind":"not_found"}}' >&2; exit 3
fi
if [ "$1" = ls ]; then   # $HC_FAKE_LS(JSON)를 답한다. $HC_FAKE_LS_FAIL이면 실패. 인자는 $HCLOG.ls에 한 줄로
  echo "$*" >>"$HCLOG.ls"
  [ -z "${HC_FAKE_LS_FAIL-}" ] || exit 3
  [ ! -e "$HCLOG.lsfail" ] || exit 3
  [ ! -f "$HCLOG.lsfile" ] || { cat "$HCLOG.lsfile"; exit 0; }   # 배경 watch가 도중에 바뀌는 목록을 보게 파일이 앞
  if [ -n "${HC_FAKE_LS-}" ]; then printf '%s\n' "$HC_FAKE_LS"; else echo '{"terminals":[]}'; fi; exit 0
fi
if [ "$1" = machine ]; then   # machine ls --json: $HC_FAKE_MACHINES(JSON)
  if [ -n "${HC_FAKE_MACHINES-}" ]; then printf '%s\n' "$HC_FAKE_MACHINES"; else echo '{"machines":[]}'; fi; exit 0
fi
if [ "$1" = read ]; then   # 화면: $HC_FAKE_SCREEN 파일(없으면 빈 화면). 주소는 $HCLOG.read에
  echo "$2" >>"$HCLOG.read"
  [ -z "${HC_FAKE_READ_FAIL-}" ] || exit 3
  [ -z "${HC_FAKE_SCREEN-}" ] || cat "$HC_FAKE_SCREEN"; exit 0
fi
if [ "$1" = keys ]; then shift; echo "$*" >>"$HCLOG.keys"; exit 0; fi
if [ "$1" = send ]; then   # 「<터미널><탭><글>」을 $HCLOG.send에. $HCLOG.sendfail 파일이 있으면 실패(배경 watch가 도중에 바뀌게 파일로)
  [ ! -e "$HCLOG.sendfail" ] || exit 1
  printf '%s\t%s\n' "$2" "$3" >>"$HCLOG.send"; exit 0
fi
f=$HCLOG; [ "$1" = exec ] && f=$HCLOG.exec
{ echo "--- $PWD"; for a in "$@"; do echo "$a"; done; } >>"$f"
case $1 in
  run) [ -n "${HC_FAKE_RUN_FAIL-}" ] && { echo '{"error":{"kind":"name_taken"}}' >&2; exit 1; }
       echo "{\"address\":\"${HC_FAKE_RUN_ADDR:-t9zz}\",\"agent\":\"x\",\"state\":\"live\"}" ;;
  exec) on=; while [ "$1" != -- ]; do [ "$1" != --on ] || on=$2; shift; done; shift
        # $HC_FAKE_MACH_HOME가 있으면 기계마다 HOME을 <그 폴더>/<기계>로(폴더가 없으면 모르는 기계, 종료 4)
        if [ -n "$on" ] && [ -n "${HC_FAKE_MACH_HOME-}" ]; then
          [ -d "$HC_FAKE_MACH_HOME/$on" ] || { echo '{"error":{"kind":"machine_unknown"}}' >&2; exit 4; }
          HOME=$HC_FAKE_MACH_HOME/$on exec env -u XDG_STATE_HOME "$@"   # 그 기계 집의 상태 폴더(이 기계 XDG_STATE_HOME 말고)
        fi
        exec "$@" ;;
esac
EOF
chmod +x "$T/bin/hc"
# 가짜 ps: $HC_FAKE_PS 파일을 답하고(인자는 $HCLOG.ps), $HC_FAKE_PS_FAIL이면 실패. 둘 다 없으면 진짜 ps
cat >"$T/bin/ps" <<'EOF'
#!/bin/bash
[ -z "${HC_FAKE_PS_FAIL-}" ] || exit 1
[ -n "${HC_FAKE_PS-}" ] || exec /bin/ps "$@"
echo "$*" >>"$HCLOG.ps"; cat "$HC_FAKE_PS"
EOF
chmod +x "$T/bin/ps"
# 가짜 herdr-app: 인자는 $HCLOG.app에. $HC_FAKE_APP이 없으면 창 없음(종료 1 — 진짜 창은 늘 안 건드린다).
# ls: $HCLOG.appls가 있으면 그것, 없고 hc run이 herdr.space=lanes를 받았으면 창이 그 터미널을 본 꼴(첫 터미널 이름의 box/lanes 공간 — 진짜 ls에는 named 칸이 없다).
# do space.rename --id <id> --name <이름>: $HCLOG.appls를 그 공간이 이름 붙은 꼴로
cat >"$T/bin/herdr-app" <<'EOF'
#!/bin/bash
echo "$*" >>"$HCLOG.app"
[ -n "${HC_FAKE_APP-}" ] || { echo '{"error":"herdr-app is not running"}' >&2; exit 1; }
case $1 in
  ls) if [ -f "$HCLOG.appls" ]; then cat "$HCLOG.appls"
      elif grep -Fxq herdr.space=lanes "$HCLOG" 2>/dev/null; then echo '{"spaces":[{"id":"box/lanes","name":"lane-a","machine":"box"}]}'
      else echo '{"spaces":[{"id":"sp1","name":"here","machine":null}]}'; fi ;;
  do) [ "$2" = space.rename ] && printf '{"spaces":[{"id":"%s","name":"%s","machine":"box"}]}\n' "$4" "$6" >"$HCLOG.appls" ;;
esac
EOF
chmod +x "$T/bin/herdr-app"
ln -s "$LANE" "$T/bin/hc-lane"   # 착지 절차의 hc-lane clean이 이 사본을 쓰게
export PATH="$T/bin:$PATH" HCLOG="$T/hc.log" HC_LANE_POLL=0.1
unset HC_LANE HC_TERMINAL_ID HC_LANE_MACHINE XDG_STATE_HOME HC_LANE_OWNER HC_LANE_COORD HC_LANE_FILL   # 레인 안에서 돌려도 물려받은 값이 시험을 바꾸지 않게
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t GIT_CONFIG_NOSYSTEM=1 HOME="$T"
printf 'export HL_ZSHRC=read\ncd /\n' >"$T/.zshrc"   # --on 설정 단계만 읽어야 하고, 안의 cd가 setup 위치를 바꾸면 안 된다
printf 'export HL_ZPROFILE=read\n' >"$T/.zprofile"     # --on은 로그인 셸(.zprofile의 brew shellenv 등)도

n=0 fails=0
t() { n=$((n + 1)); if eval "$2"; then echo "ok $n - $1"; else fails=$((fails + 1)); echo "FAIL $n - $1"; fi; }
conf() { mkdir -p "$T/.config/herdr-tools"; printf '%s\n' "$@" >"$T/.config/herdr-tools/config"; }
noconf() { rm -f "$T/.config/herdr-tools/config"; }
has() { grep -Fxq -- "$1" "$HCLOG" 2>/dev/null; }                     # hc run이 받은 인자 한 줄
pair() { grep -Fx -A1 -- "$1" "$HCLOG" 2>/dev/null | grep -Fxq -- "$2"; } # 옵션 다음 값

# mk <이름> [기본 브랜치]: origin.git + clone. clone 뒤 origin에 커밋을 하나 더 올려 fetch가 필요한 상태로 둔다.
mk() {
  D="$W/$1" B=${2:-main}
  rm -f "$HCLOG" "$HCLOG.exec" "$HCLOG.keys"
  git init -q --bare -b "$B" "$D/origin.git"
  git clone -q "$D/origin.git" "$D/seed" 2>/dev/null
  seed a a
  git clone -q "$D/origin.git" "$D/저장소 한글"
  seed b b
  R="$D/저장소 한글"
}
seed() { (cd "$D/seed" && mkdir -p "$(dirname "$1")" && printf '%s\n' "$2" >"$1" && git add "$1" && git commit -qm "$1" && git push -q origin "HEAD:$B"); }
run() { out=$("$LANE" "$@" 2>"$T/err"); rc=$?; }

# 1. 기본 모드
mk t1
run --name lane-a --repo "$R" -- claude -p "짧은 일 하나"
wt="$R.wt/lane-a"
t "기본: 종료 0" '[ $rc = 0 ]'
t "기본: 워크트리가 <repo>.wt/<슬러그>" '[ -d "$wt" ]'
t "기본: fetch한 origin/main 최신 커밋 기준" '[ "$(git -C "$wt" rev-parse HEAD)" = "$(git -C "$D/origin.git" rev-parse main)" ]'
t "기본: 브랜치 이름 = 레인 이름" '[ "$(git -C "$wt" symbolic-ref --short HEAD)" = lane-a ]'
t "기본: hc run --bg --name" 'has run && has --bg && pair --name lane-a'
t "기본: hc run --cwd = 워크트리" 'pair --cwd "$wt"'
t "기본: LANE_* 환경을 env로 넘김" 'has env && has "LANE_REPO=$R" && has "LANE_WORKTREE=$wt" && has LANE_BRANCH=lane-a && has LANE_BASE=main'
t "기본: 실행 명령 인자 그대로" 'has claude && has -p && has "짧은 일 하나"'
line() { grep -n -Fx -- "$1" "$HCLOG" | head -n 1 | cut -d: -f1; }
t "기본: 순서는 env → LANE_* → 실행 명령" '[ "$(line env)" -lt "$(line "LANE_REPO=$R")" ] && [ "$(line LANE_BASE=main)" -lt "$(line claude)" ]'
t "기본: --on 없으면 hc exec 안 씀" '[ ! -e "$HCLOG.exec" ]'
t "기본: 마지막 줄 JSON" 'printf "%s\n" "$out" | tail -n 1 | grep -Fq "\"lane\":\"t9zz\",\"name\":\"lane-a\""'
t "기본: 착지 절차에 경로·기준" 'printf "%s" "$out" | grep -Fq "cd '\''$wt'\'' && hc-lane land main" && printf "%s" "$out" | grep -Fq "cd '\''$R'\'' && hc-lane clean --repo '\''$R'\'' --branch lane-a"'

# 2. herdr-setup.sh 성공·실패
mk t2
seed .claude/herdr-setup.sh 'echo "$LANE_REPO|$LANE_BRANCH|$LANE_BASE|${HL_ZSHRC-}|${WT_MAIN-none}|${HERDR_BASE-none}" > setup-ran'
HC_TERMINAL_ID=t1zz run --name lane-s --repo "$R" -- true
t "setup: 성공 시 종료 0" '[ $rc = 0 ]'
t "setup: 워크트리 안에서 실행(LANE_REPO=메인 폴더·LANE_* 받음, WT_MAIN·HERDR_BASE 없음, hc 터미널 안 로컬은 ~/.zshrc 안 읽음)" '[ "$(cat "$R.wt/lane-s/setup-ran" 2>/dev/null)" = "$R|lane-s|main||none|none" ]'
t "setup: 성공 뒤 hc run" 'has run'
t "setup: hc 터미널 안 로컬은 그 자리에서(hc exec 안 씀)" '! grep -Fq herdr-setup.sh "$HCLOG.exec" 2>/dev/null'
rm -f "$HCLOG.exec"
run --name lane-sc --repo "$R" -- true
if command -v zsh >/dev/null 2>&1; then want=read; else want=; fi
t "setup: hc 터미널 밖 로컬(launchd hc-fill)은 hc 코어 문맥으로(hc exec -- , --on 없이 — TCC로 bash가 ~/Documents를 못 읽음)" '[ $rc = 0 ] && grep -Fq .claude/herdr-setup.sh "$HCLOG.exec" && ! grep -Fxq -- --on "$HCLOG.exec"'
t "setup: 코어 문맥이면 ~/.zshrc를 읽고 돈다(코어 PATH)" '[ "$(cat "$R.wt/lane-sc/setup-ran" 2>/dev/null)" = "$R|lane-sc|main|$want|none|none" ]'
mk t2o
seed .claude/wt-setup.sh 'echo old > old-setup-ran'
run --name lane-so --repo "$R" -- true
t "setup: 옛 이름 wt-setup.sh는 부르지 않음(옛 이름 갈래 없음)" '[ $rc = 0 ] && [ ! -e "$R.wt/lane-so/old-setup-ran" ] && has run'
mk t3
seed .claude/herdr-setup.sh 'echo boom-setup; exit 7'
run --name lane-f --repo "$R" -- true
t "setup 실패: 종료 0 아님" '[ $rc != 0 ]'
t "setup 실패: hc run 안 부름" '[ ! -e "$HCLOG" ]'
t "setup 실패: 오류에 워크트리 경로와 기록 끝" 'grep -Fq "$R.wt/lane-f" "$T/err" && grep -q boom-setup "$T/err"'
t "setup 실패: 방금 만든 워크트리·브랜치를 지움(고유 커밋 없음)" '[ ! -e "$R.wt/lane-f" ] && ! git -C "$R" show-ref -q --verify refs/heads/lane-f && grep -Fq "지웠다" "$T/err"'
seed .claude/herdr-setup.sh 'true'
run --name lane-f --repo "$R" -- true
t "setup 실패 뒤 고쳐서 같은 명령: 「브랜치가 이미 있다」로 안 막힘" '[ $rc = 0 ] && [ -d "$R.wt/lane-f" ] || { cat "$T/err"; false; }'
seed .claude/herdr-setup.sh 'git commit -q --allow-empty -m setup-commit; exit 3'
run --name lane-fk --repo "$R" -- true
t "setup 실패(고유 커밋 생김): 워크트리·브랜치를 남기고 지우는 명령 안내" '[ $rc != 0 ] && [ -d "$R.wt/lane-fk" ] && git -C "$R" show-ref -q --verify refs/heads/lane-fk && grep -Fq "지우려면:" "$T/err"'

# 3. --read
mk t4
run --read --name lane-r --repo "$R" -- echo hi
t "--read: 종료 0" '[ $rc = 0 ]'
t "--read: cwd = 저장소" 'pair --cwd "$R"'
t "--read: 워크트리·브랜치 안 만듦" '[ ! -e "$R.wt" ] && ! git -C "$R" show-ref -q --verify refs/heads/lane-r'
t "--read: LANE_REPO만, LANE_WORKTREE 없음" 'has "LANE_REPO=$R" && ! grep -q LANE_WORKTREE "$HCLOG"'

# 4. 충돌은 오류, 아무것도 덮지 않음
mk t5
git -C "$R" branch lane-x
before=$(git -C "$R" rev-parse lane-x)
run --name lane-x --repo "$R" -- true
t "충돌(로컬 브랜치): 오류와 --branch 안내" '[ $rc != 0 ] && grep -Fq -- --branch "$T/err"'
t "충돌(로컬 브랜치): 브랜치 그대로, 폴더·hc 없음" '[ "$(git -C "$R" rev-parse lane-x)" = "$before" ] && [ ! -e "$R.wt/lane-x" ] && [ ! -e "$HCLOG" ]'
(cd "$D/seed" && git push -q origin HEAD:refs/heads/lane-y)
run --name lane-y --repo "$R" -- true
t "충돌(원격 브랜치): 오류, 로컬 브랜치·폴더 안 만듦" '[ $rc != 0 ] && ! git -C "$R" show-ref -q --verify refs/heads/lane-y && [ ! -e "$R.wt/lane-y" ]'
mkdir -p "$R.wt/lane-z" && echo keep >"$R.wt/lane-z/f"
run --name lane-z --repo "$R" -- true
t "충돌(폴더): 오류, 폴더 내용 그대로, 브랜치 안 만듦" '[ $rc != 0 ] && [ "$(cat "$R.wt/lane-z/f")" = keep ] && ! git -C "$R" show-ref -q --verify refs/heads/lane-z && [ ! -e "$HCLOG" ]'
run --name lane-z --branch feat/lane-z2 --repo "$R" -- true
t "--branch로 피함: 폴더는 / 를 - 로" '[ $rc = 0 ] && [ "$(git -C "$R.wt/feat-lane-z2" symbolic-ref --short HEAD)" = feat/lane-z2 ]'

# 5. 기본 브랜치가 master(origin/HEAD)
mk t6 master
run --name lane-m --repo "$R" -- true
t "master: 종료 0, LANE_BASE=master" '[ $rc = 0 ] && has LANE_BASE=master'
t "master: origin/master 최신 커밋 기준" '[ "$(git -C "$R.wt/lane-m" rev-parse HEAD)" = "$(git -C "$D/origin.git" rev-parse master)" ]'
t "master: 착지 절차도 master" 'printf "%s" "$out" | grep -Fq "hc-lane land master"'

# 6. --dry-run은 아무것도 만들지 않음
mk t7
ref=$(git -C "$R" rev-parse origin/main)
run --dry-run --name lane-d --repo "$R" -- echo hi
t "dry-run: 종료 0" '[ $rc = 0 ]'
t "dry-run: 폴더·브랜치·hc 호출 없음, fetch도 안 함" '[ ! -e "$R.wt" ] && ! git -C "$R" show-ref -q --verify refs/heads/lane-d && [ ! -e "$HCLOG" ] && [ "$(git -C "$R" rev-parse origin/main)" = "$ref" ]'
t "dry-run: 할 일·착지 절차·JSON 출력" 'printf "%s" "$out" | grep -Fq "[dry-run] hc run" && printf "%s" "$out" | grep -Fq "hc-lane land main" && printf "%s" "$out" | grep -Fq "herdr-setup.sh" && printf "%s\n" "$out" | tail -n 1 | grep -Fq "\"dry_run\":true"'
git -C "$R" branch lane-d2
run --dry-run --name lane-d2 --repo "$R" -- echo hi
t "dry-run: 충돌도 알림" '[ $rc != 0 ]'

# 7. --print-steps
run --print-steps --name lane-p --repo "$R"
t "print-steps: 종료 0, 경로·검증·정리 줄" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fq "'\''$R.wt/lane-p'\''" && printf "%s" "$out" | grep -Fq "herdr-verify.sh" && printf "%s" "$out" | grep -Fq "cd '\''$R'\'' && hc-lane clean --repo '\''$R'\'' --branch lane-p"'
t "print-steps: 착지는 hc-lane land 한 줄, 옛 이름·건너뜀 없음" 'printf "%s\n" "$out" | grep -E "^[0-9]\. " | grep -Fxq "1. cd '\''$R.wt/lane-p'\'' && hc-lane land main" && ! printf "%s" "$out" | grep -Fq "wt-" && ! printf "%s" "$out" | grep -Fq "건너뜀)"'
t "print-steps: 끝내기 전 작업 트리 배경 프로세스 확인 줄(끝 /, 자기 셸 빼기)" 'printf "%s" "$out" | grep -Fq "pgrep -fl '\''$R.wt/lane-p/'\''" && printf "%s" "$out" | grep -q "자기 셸·에이전트는 빼고"'
t "print-steps: 아무것도 만들지 않음" '[ ! -e "$R.wt/lane-p" ] && [ ! -e "$HCLOG" ]'

# 8. --on: git·설정은 hc exec --on, 실행은 hc run --on (가짜 hc exec가 그 자리에서 돌림)
mk t8
seed .claude/herdr-setup.sh 'echo "${HL_ZSHRC-none}|${HL_ZPROFILE-none}" > setup-ran'
run --on box --name lane-o --repo "$R" -- echo hi
t "--on: 종료 0" '[ $rc = 0 ]'
t "--on: git·설정을 hc exec --on box로" 'grep -Fx -A1 -- --on "$HCLOG.exec" | grep -Fxq box && grep -Fxq worktree "$HCLOG.exec" && [ -e "$R.wt/lane-o/setup-ran" ]'
if command -v zsh >/dev/null 2>&1; then want='read|read'; else want='none|none'; fi
t "--on: 설정 단계는 로그인 셸(~/.zprofile)과 ~/.zshrc를 읽고 돈다(zsh 있으면)" '[ "$(cat "$R.wt/lane-o/setup-ran")" = "$want" ]'
t "--on: hc run --on box --cwd 워크트리" 'pair --on box && pair --cwd "$R.wt/lane-o"'
t "--on: JSON에 기계" 'printf "%s\n" "$out" | tail -n 1 | grep -Fq "\"on\":\"box\""'
seed .claude/herdr-setup.sh 'exit 5'
run --on box --name lane-o2 --repo "$R" -- echo hi
t "--on setup 실패: 방금 만든 워크트리·브랜치를 hc exec --on box로 지움" '[ $rc != 0 ] && [ ! -e "$R.wt/lane-o2" ] && ! git -C "$R" show-ref -q --verify refs/heads/lane-o2 && grep -Fxq remove "$HCLOG.exec"'
seed .claude/herdr-setup.sh 'git commit -q --allow-empty -m setup-commit; exit 3'
run --on box --name lane-o3 --repo "$R" -- echo hi
undo=$(sed -n 's/.*지우려면: //p' "$T/err")
t "--on setup 실패(고유 커밋): 남기고 안내하는 명령이 hc exec --on box 꼴, 그 명령으로 정리됨(감사 [낮음] 4)" '[ $rc != 0 ] && [ -d "$R.wt/lane-o3" ] && printf "%s" "$undo" | grep -Fq "hc exec --on box -- git -C" && printf "%s" "$undo" | grep -Fq "&& hc exec --on box -- git -C" && (cd "$T" && eval "${undo/branch -d/branch -D}") >/dev/null 2>&1 && [ ! -e "$R.wt/lane-o3" ] && ! git -C "$R" show-ref -q --verify refs/heads/lane-o3'

# 8b. 다른 기계 레인은 맥 Herdr 창의 그 기계 공간(이름 = 기계, 기계마다 하나)에 탭으로(hc run --label herdr.space=<id>)
mk t8b
rm -f "$HCLOG.app" "$HCLOG.appls"
HC_FAKE_APP=1 run --on box --name lane-a --repo "$R" -- echo hi
t "원격 표시: 창에 그 기계 공간이 없으면 고정 id lanes로 붙인다" '[ $rc = 0 ] && pair --label herdr.space=lanes || { cat "$T/err"; false; }'
t "원격 표시: 창이 만든 공간에 기계 이름을 붙인다(space.rename box/lanes → box)" 'grep -Fxq "do space.rename --id box/lanes --name box" "$HCLOG.app"'
rm -f "$HCLOG" "$HCLOG.app"
HC_FAKE_APP=1 run --read --on box --name lane-b --repo "$R" -- echo hi
t "원격 표시: 같은 기계 두 번째 레인(--read 감사도)은 같은 공간(lanes)에, 이름은 다시 안 붙임" '[ $rc = 0 ] && pair --label herdr.space=lanes && ! grep -q rename "$HCLOG.app"'
rm -f "$HCLOG" "$HCLOG.app"
printf '{"spaces":[{"id":"box/sp9","name":"box","machine":"box"},{"id":"other/sp9","name":"box","machine":"other"}]}\n' >"$HCLOG.appls"
HC_FAKE_APP=1 run --on box --name lane-c --repo "$R" -- echo hi
t "원격 표시: 그 기계에 「<기계>」 공간이 있으면 그 id(창이 보이는 「box/」 접두는 뗀다 — 붙이면 새 공간)" '[ $rc = 0 ] && pair --label herdr.space=sp9 && ! grep -q rename "$HCLOG.app"'
rm -f "$HCLOG" "$HCLOG.app" "$HCLOG.appls"
run --on box --name lane-d --repo "$R" -- echo hi
t "원격 표시: 창이 없으면 띄우기는 성공, 라벨은 lanes, 알림 한 줄" '[ $rc = 0 ] && pair --label herdr.space=lanes && grep -q "Herdr 창이 없어" "$T/err"'
rm -f "$HCLOG"
HC_FAKE_APP=1 run --name lane-e --repo "$R" -- echo hi
t "원격 표시: 로컬 레인은 herdr.space 라벨 없음(지휘 옆 분할은 hc-fill의 herdr-balance)" '[ $rc = 0 ] && ! grep -q "^herdr.space=" "$HCLOG"'
run --dry-run --on box --name lane-g --repo "$R" -- echo hi
t "원격 표시: dry-run 줄에 라벨" 'printf "%s" "$out" | grep -Fq -- "--label herdr.space=lanes"'

# 9. 인자 오류
rm -f "$HCLOG"
run --name Bad_Name --repo "$R" -- true
t "인자: 대문자 이름 거절" '[ $rc != 0 ]'
run --name lane-q --repo "$R"
t "인자: 실행 명령 없으면 거절" '[ $rc != 0 ]'
run --name lane-q --repo "$T/없는 폴더" -- true
t "인자: git 저장소 아니면 거절" '[ $rc != 0 ] && [ ! -e "$HCLOG" ]'
run --name lane-q --branch 'a;id' --repo "$R" -- true
t "인자: 브랜치 이름의 특수 문자 거절" '[ $rc != 0 ] && [ ! -e "$HCLOG" ] && [ ! -e "$R.wt/a;id" ]'

# 10. 출력된 착지·정리 절차를 그대로 돌린다(branch.autoSetupMerge=false여도 branch -d까지 통해야 한다)
mk t10
seed .gitignore '.omc/'
seed .claude/herdr-verify.sh 'echo herdr-verify-ran'
git -C "$R" config branch.autoSetupMerge false
run --name lane-l --repo "$R" -- true
wt="$R.wt/lane-l"
(cd "$wt" && echo land >land && git add -- land && git commit -qm land -- land)
mkdir -p "$wt/.omc/state" && echo s >"$wt/.omc/state/s.json"   # 레인 세션이 남기는 무시 폴더
seed c c   # 그 사이 origin이 앞서 나가 rebase가 필요하게
printf '%s\n' "$out" | sed -n 's/^[0-9]\. //p' >"$T/steps.sh"
(cd "$T" && bash -e steps.sh) >"$T/steps.out" 2>&1; src=$?
t "착지 절차: 그대로 실행해 종료 0" '[ $src = 0 ] || { cat "$T/steps.out"; false; }'
t "착지 절차: origin에 레인 커밋이 앞 커밋 위로" '[ "$(git -C "$D/origin.git" log -1 --format=%s main)" = land ] && [ "$(git -C "$D/origin.git" log -2 --format=%s main | tail -n 1)" = c ]'
t "착지 절차: 워크트리·브랜치 정리됨(무시 폴더가 남아도)" '[ ! -e "$wt" ] && ! git -C "$R" show-ref -q --verify refs/heads/lane-l'
t "착지 절차: herdr-verify.sh를 돌리고 착지" 'grep -Fq herdr-verify-ran "$T/steps.out" && grep -Fq "hc-lane land: 착지" "$T/steps.out"'

# 10b. 검증이 실패하면 push가 나가지 않는다: 출력된 줄을 멈추지 않고(bash -e 없이) 차례로 다 돌려도
mk t10b
seed .claude/herdr-verify.sh 'echo verify-failed; exit 7'
run --name lane-vf --repo "$R" -- true
wt="$R.wt/lane-vf"
(cd "$wt" && echo vf >vf && git add -- vf && git commit -qm vf -- vf)
printf '%s\n' "$out" | sed -n 's/^[0-9]\. //p' >"$T/steps-vf.sh"
(cd "$T" && bash steps-vf.sh) >"$T/steps-vf.out" 2>&1
t "착지 절차: 검증이 실패하면 줄을 다 돌려도 push가 안 나감" '[ "$(git -C "$D/origin.git" log -1 --format=%s main)" != vf ] && grep -Fq verify-failed "$T/steps-vf.out"'
t "착지 절차: 검증이 실패하면 정리도 멈춰 워크트리·브랜치가 남음" '[ -e "$wt/vf" ] && git -C "$R" show-ref -q --verify refs/heads/lane-vf'
t "착지 절차: 착지는 hc-lane land 한 줄, 이어 쓰지 말라는 안내" 'printf "%s\n" "$out" | grep -E "^[0-9]\. " | grep -Fq "&& hc-lane land main" && printf "%s" "$out" | grep -Fq "세미콜론으로 이어 쓰지 말 것"'
t "착지 절차: 번호 줄은 명령만, 안내 # 줄에는 ; > | & 없음(대화형 zsh에 붙여 넣어도 명령이 안 됨)" '! printf "%s\n" "$out" | grep -E "^[0-9]\. " | grep -Fq "#" && ! printf "%s\n" "$out" | grep -E "^   #" | grep -q "[;>|&]"'

# 10c. rebase가 충돌로 멈추면 반쯤 된 HEAD가 push되지 않는다(줄을 멈추지 않고 다 돌려도)
mk t10c
seed x base
seed .claude/herdr-verify.sh 'exit 0'
run --name lane-rc --repo "$R" -- true
wt="$R.wt/lane-rc"
(cd "$wt" && echo l1 >l1 && git add -- l1 && git commit -qm l1 -- l1 && echo lane >x && git commit -qm l2 -- x)
seed x other   # origin의 x가 바뀌어 l2가 충돌한다
printf '%s\n' "$out" | sed -n 's/^[0-9]\. //p' >"$T/steps-rc.sh"
(cd "$T" && bash steps-rc.sh) >"$T/steps-rc.out" 2>&1
t "착지 절차: rebase 충돌이면 줄을 다 돌려도 origin은 그대로(앞 커밋 l1만 나가지 않음)" '[ "$(git -C "$D/origin.git" log -1 --format=%s main)" = x ] && ! git -C "$D/origin.git" log --format=%s main | grep -qx l1'
git -C "$wt" rebase --abort 2>/dev/null

# 11. 아무것도 만들기 전에 막거나, 반쯤 만든 것은 지우는 명령을 알린다
mk t11
HC_FAKE_LIVE=lane-u run --name lane-u --repo "$R" -- true
t "이름 사용 중: 오류, 워크트리·브랜치·hc run 없음" '[ $rc != 0 ] && grep -q "lane-u" "$T/err" && [ ! -e "$R.wt/lane-u" ] && ! git -C "$R" show-ref -q --verify refs/heads/lane-u && [ ! -e "$HCLOG" ]'
HC_FAKE_ID=t12 run --name t12 --repo "$R" -- true
t "ID가 같은 다른 이름의 터미널은 막지 않음" '[ $rc = 0 ]'
HC_FAKE_RUN_FAIL=1 run --name lane-v --repo "$R" -- true
t "hc run 실패: 방금 만든 워크트리·브랜치를 지움(다음 시도가 막히지 않게)" '[ $rc != 0 ] && grep -Fq "지웠다" "$T/err" && [ ! -e "$R.wt/lane-v" ] && ! git -C "$R" show-ref -q --verify refs/heads/lane-v'
git -C "$R" remote set-head origin -d
run --name lane-w --repo "$R" -- true
t "origin/HEAD 없음: main으로" '[ $rc = 0 ] && has LANE_BASE=main'
mk t12 master
git -C "$R" remote set-head origin -d
run --name lane-k --repo "$R" -- true
t "origin/HEAD 없고 main도 없음: 오류, 아무것도 안 만듦" '[ $rc != 0 ] && grep -q "origin/main" "$T/err" && [ ! -e "$R.wt" ] && [ ! -e "$HCLOG" ]'

# 13. hc-lane clean: 무시된 파일만 남았고 브랜치가 origin에 들어갔을 때만 지운다. 아니면 아무것도 지우지 않는다
mk t13
seed .gitignore '.omc/'
seed .claude/herdr-teardown.sh 'echo "$PWD" >"$HOME/teardown-ran"'
# land <레인>: 레인을 띄우고 커밋 하나를 origin/main에 넣은 뒤, 레인 세션처럼 무시 폴더 .omc/를 남긴다
land() {
  run --name "$1" --repo "$R" -- true
  wt="$R.wt/$1"
  (cd "$wt" && echo "$1" >"$1.txt" && git add -- "$1.txt" && git commit -qm "$1" -- "$1.txt" && git push -q origin HEAD:main)
  mkdir -p "$wt/.omc/state" && echo s >"$wt/.omc/state/s.json"
}
gone() { [ ! -e "$R.wt/$1" ] && ! git -C "$R" show-ref -q --verify "refs/heads/$1" && ! git -C "$R" worktree list --porcelain | grep -Fq "$R.wt/$1"; }
kept() { [ -d "$R.wt/$1" ] && git -C "$R" show-ref -q --verify "refs/heads/$1"; }
reg() { git -C "$R" worktree list --porcelain | grep -Fxq "worktree $R.wt/$1"; }
last() { printf '%s\n' "$out" | tail -n 1 | grep -Fq -- "$1"; }

land c-ok
rm -f "$T/teardown-ran"
run clean --repo "$R" --branch c-ok
t "clean: 무시 폴더만 남은 워크트리 → 종료 0" '[ $rc = 0 ] || { cat "$T/err"; false; }'
t "clean: 폴더·등록·브랜치 정리" 'gone c-ok'
t "clean: teardown은 워크트리 안에서" '[ "$(cat "$T/teardown-ran" 2>/dev/null)" = "$R.wt/c-ok" ]'
t "clean: 결과 JSON(done, teardown ran)" 'last "\"clean\":\"done\"" && last "\"teardown\":\"ran\""'
t "clean: 같이 지우는 무시 파일을 알림" 'printf "%s" "$out" | grep -Fq "무시된 것도 같이 지운다: .omc/"'
run clean --repo "$R" --branch c-ok
t "clean: 다시 돌리면 정리할 것 없음, 종료 0" '[ $rc = 0 ] && last "\"clean\":\"nothing\""'

land c-untracked
echo keep >"$R.wt/c-untracked/note.txt"
rm -f "$T/teardown-ran"
run clean --repo "$R" --branch c-untracked
t "clean: 무시되지 않은 untracked → 멈추고 목록" '[ $rc != 0 ] && grep -Fq note.txt "$T/err" && last "\"clean\":\"stopped\""'
t "clean: 그때 아무것도 안 지움(파일·무시 폴더·등록·브랜치, teardown도 안 돌림)" 'kept c-untracked && reg c-untracked && [ -f "$R.wt/c-untracked/note.txt" ] && [ -f "$R.wt/c-untracked/.omc/state/s.json" ] && [ ! -e "$T/teardown-ran" ]'

land c-tracked
echo changed >>"$R.wt/c-tracked/a"
run clean --repo "$R" --branch c-tracked
t "clean: 추적 파일 변경 → 멈추고 아무것도 안 지움" '[ $rc != 0 ] && grep -Fq "M a" "$T/err" && kept c-tracked && reg c-tracked && [ "$(tail -n 1 "$R.wt/c-tracked/a")" = changed ]'

run --name c-unpushed --repo "$R" -- true
(cd "$R.wt/c-unpushed" && echo u >u && git add -- u && git commit -qm u -- u)
run clean --repo "$R" --branch c-unpushed
t "clean: origin/main에 안 들어간 브랜치 → 멈추고 아무것도 안 지움" '[ $rc != 0 ] && grep -Fq origin/main "$T/err" && kept c-unpushed && reg c-unpushed && [ -f "$R.wt/c-unpushed/u" ]'
# sweep-rename 4(2026-11-17): 다른 곳에서 rebase돼 착지한 가지는 SHA·패치가 달라 안 들어간 것으로 보인다(2026-11-16 워커 맥 레인 — 지휘가 손으로
# refs/integ/ 에 push 뒤 지움). --archive(명시할 때만): 작업 폴더가 깨끗하면 원격 refs/archive/<가지>-<sha8>에 보관한 뒤 지운다
run --name c-arch --repo "$R" -- true
(cd "$R.wt/c-arch" && echo r >r && git add -- r && git commit -qm r -- r)
ca=$(git -C "$R" rev-parse c-arch)
printf '#!/bin/sh\nwhile read o n r; do case $r in refs/archive/*) echo no-archive >&2; exit 1 ;; esac; done\n' >"$D/origin.git/hooks/pre-receive"
chmod +x "$D/origin.git/hooks/pre-receive"
run clean --repo "$R" --branch c-arch --archive
t "clean --archive: push가 거절되면 아무것도 안 지움" '[ $rc != 0 ] && kept c-arch && reg c-arch && [ -f "$R.wt/c-arch/r" ] || { cat "$T/err"; false; }'
rm -f "$D/origin.git/hooks/pre-receive"
echo dirt >"$R.wt/c-arch/dirt.txt"
run clean --repo "$R" --branch c-arch --archive
t "clean --archive: 작업 폴더가 깨끗하지 않으면 보관·삭제 없이 멈춤" '[ $rc != 0 ] && kept c-arch && ! git -C "$D/origin.git" for-each-ref refs/archive | grep -q . || { cat "$T/err"; false; }'
rm -f "$R.wt/c-arch/dirt.txt"
run clean --repo "$R" --branch c-arch --archive
t "clean --archive: 원격 refs/archive/<가지>-<sha8>에 보관하고 정리" '[ $rc = 0 ] && gone c-arch && [ "$(git -C "$D/origin.git" rev-parse "refs/archive/c-arch-$(printf %.8s "$ca")" 2>/dev/null)" = "$ca" ] || { cat "$T/err"; printf "%s\n" "$out"; false; }'

# 메인 폴더 뒤처짐: 레인을 띄우면 메인 폴더가 앞당겨지니(main-folder-ff), 앞당김을 건너뛰는 꼴(다른 가지 — .gitignore 전 첫 커밋)로 만든다
git -C "$R" checkout -q -b behind "$(git -C "$R" rev-list --max-parents=0 HEAD)"
land c-left   # 오늘 상황: remove가 등록은 지웠고 무시 폴더(와 지워진 등록을 가리키는 .git 파일)만 남음
git -C "$R" worktree remove --force "$R.wt/c-left"
mkdir -p "$R.wt/c-left/.omc/state" && echo s >"$R.wt/c-left/.omc/state/s.json"
printf 'gitdir: %s\n' "$R/.git/worktrees/c-left" >"$R.wt/c-left/.git"
run clean --repo "$R" --branch c-left
t "clean: 메인 폴더에 그 .gitignore가 아직 없으면(뒤처짐) 남은 폴더는 멈추고 그대로(안전 쪽)" '[ $rc != 0 ] && [ -f "$R.wt/c-left/.omc/state/s.json" ] && git -C "$R" show-ref -q --verify refs/heads/c-left'
git -C "$R" checkout -q main && git -C "$R" branch -q -D behind
git -C "$R" pull -q --ff-only
run clean --repo "$R" --branch c-left
t "clean: 등록 없이 무시 폴더만 남은 폴더 → 폴더·브랜치 정리" '[ $rc = 0 ] && gone c-left || { cat "$T/err"; false; }'
land c-left2
git -C "$R" worktree remove --force "$R.wt/c-left2"
mkdir -p "$R.wt/c-left2/.omc" && echo s >"$R.wt/c-left2/.omc/s" && echo keep >"$R.wt/c-left2/mine.txt"
run clean --repo "$R" --branch c-left2
t "clean: 등록 없는 폴더에 무시되지 않은 파일 → 멈추고 그대로" '[ $rc != 0 ] && grep -Fq mine.txt "$T/err" && [ -f "$R.wt/c-left2/mine.txt" ] && [ -f "$R.wt/c-left2/.omc/s" ] && git -C "$R" show-ref -q --verify refs/heads/c-left2'
mkdir -p "$R.wt/c-left3" && git init -q "$R.wt/c-left3" && mkdir -p "$R.wt/c-left3/.omc" && echo s >"$R.wt/c-left3/.omc/s"
run clean --repo "$R" --branch c-left3
t "clean: 등록 없는 폴더가 다른 git 저장소면 멈추고 그대로" '[ $rc != 0 ] && grep -Fq .git "$T/err" && [ -d "$R.wt/c-left3/.git" ]'

land c-cwd
(cd "$R.wt/c-cwd" && "$LANE" clean --repo "$R" --branch c-cwd >"$T/cwd.out" 2>&1); rc=$?
t "clean: 워크트리 안에서 불러도 정리하고 cd 안내" '[ $rc = 0 ] && gone c-cwd && grep -Fq "cd '\''$R'\''" "$T/cwd.out"'

land c-on
rm -f "$HCLOG.exec"
run clean --on box --repo "$R" --name c-on
t "clean --on: git·파일 일은 hc exec --on box로, 정리됨" '[ $rc = 0 ] && gone c-on && grep -Fx -A1 -- --on "$HCLOG.exec" | grep -Fxq box && grep -Fxq worktree "$HCLOG.exec" && grep -Fxq branch "$HCLOG.exec" && last "\"on\":\"box\""'

land c-dry
rm -f "$T/teardown-ran"
ref=$(git -C "$R" rev-parse origin/main)
run clean --dry-run --repo "$R" --branch c-dry
t "clean --dry-run: 종료 0, 아무것도 안 지우고 teardown·fetch도 안 함" '[ $rc = 0 ] && kept c-dry && reg c-dry && [ -f "$R.wt/c-dry/.omc/state/s.json" ] && [ ! -e "$T/teardown-ran" ] && [ "$(git -C "$R" rev-parse origin/main)" = "$ref" ]'
t "clean --dry-run: 할 일과 JSON" 'printf "%s" "$out" | grep -Fq "[dry-run] git -C" && last "\"dry_run\":true"'
run clean --dry-run --repo "$R" --branch c-unpushed
t "clean --dry-run: 안 들어간 브랜치는 dry-run도 멈춤" '[ $rc != 0 ] && kept c-unpushed'

run clean --repo "$R" --branch main
t "clean: 기본 브랜치는 거절(그때도 JSON 한 줄)" '[ $rc != 0 ] && git -C "$R" show-ref -q --verify refs/heads/main && last "\"clean\":\"stopped\""'
run clean --repo "$R"
t "clean: --branch·--name 없으면 거절" '[ $rc != 0 ]'

# 14. 검토 반영(2026-11-05): 설정이 숨긴 untracked, 폴더 이름만 같은 다른 브랜치, 살아 있는 터미널,
#     메인 .gitignore 로컬 변경, 무시 규칙에도 걸리는 추적 파일, 폴더 없는 분리 HEAD
land c-hide
git -C "$R" config status.showUntrackedFiles no
echo keep >"$R.wt/c-hide/note.txt"
run clean --repo "$R" --branch c-hide
git -C "$R" config --unset status.showUntrackedFiles
t "clean: status.showUntrackedFiles=no여도 untracked를 보고 멈춤" '[ $rc != 0 ] && grep -Fq note.txt "$T/err" && [ -f "$R.wt/c-hide/note.txt" ] && kept c-hide'

run --name c-fx --branch feat/x --repo "$R" -- true
run clean --repo "$R" --branch feat-x
t "clean: 폴더 이름만 같은 다른 브랜치(feat/x)의 워크트리는 멈추고 그대로" '[ $rc != 0 ] && grep -Fq feat/x "$T/err" && reg feat-x && git -C "$R" show-ref -q --verify refs/heads/feat/x'

land c-live
lsj() { printf '{"machines":[],"terminals":[{"address":"%s","agent":"claude-code","cwd":"%s","state":"live"}]}' "$1" "$2"; }
HC_FAKE_LS=$(lsj t7 "$R.wt/c-live/sub") run clean --repo "$R" --branch c-live
t "clean: 살아 있는 다른 터미널이 그 폴더에서 돌면 멈추고 그대로" '[ $rc != 0 ] && grep -Fq t7 "$T/err" && kept c-live && reg c-live'
HC_FAKE_LS_FAIL=1 run clean --repo "$R" --branch c-live
t "clean: hc ls가 실패하면 멈춤(확인 못 한 것을 없음으로 읽지 않음)" '[ $rc != 0 ] && grep -Fq "hc ls" "$T/err" && kept c-live'
HC_TERMINAL_ID=t7 HC_FAKE_LS=$(lsj t7 "$R.wt/c-live") run clean --repo "$R" --branch c-live
t "clean: 그 터미널이 부른 자신이면(착지 절차의 레인) 정리" '[ $rc = 0 ] && gone c-live'
land c-live2
HC_FAKE_LS=$(lsj box/t7 "$R.wt/c-live2") run clean --repo "$R" --branch c-live2
t "clean: 다른 기계 터미널의 같은 경로는 막지 않음" '[ $rc = 0 ] && gone c-live2'
land c-live3
HC_FAKE_LS=$(lsj box/t7 "$R.wt/c-live3") run clean --on box --repo "$R" --branch c-live3
t "clean --on: 그 기계 터미널이 폴더에서 돌면 멈춤" '[ $rc != 0 ] && grep -Fq box/t7 "$T/err" && kept c-live3'
HC_FAKE_LS='{"terminals":[{"cwd":"/x","address":"t8"}]}' run clean --repo "$R" --branch c-live3
t "clean: hc ls 형식을 못 읽으면(cwd가 address 앞) 멈춤" '[ $rc != 0 ] && grep -Fq "hc ls --json 출력을 읽지 못했다" "$T/err" && kept c-live3'
HC_FAKE_LS="{\"terminals\":[{\"address\" : \"t7\", \"cwd\" : \"$R.wt/c-live3\"}]}" run clean --repo "$R" --branch c-live3
t "clean: 콜론 뒤 공백 꼴도 읽고 멈춤" '[ $rc != 0 ] && grep -Fq t7 "$T/err" && kept c-live3'
git -C "$R" worktree add -q --detach "$R.wt/c-dh" origin/main && git -C "$R" branch -q --no-track c-dh origin/main
run clean --repo "$R" --branch c-dh
t "clean: 분리 HEAD 폴더인데 같은 이름 브랜치가 따로 있으면 멈추고 그대로" '[ $rc != 0 ] && grep -Fq "분리 HEAD" "$T/err" && reg c-dh && git -C "$R" show-ref -q --verify refs/heads/c-dh'

land c-gi
git -C "$R" worktree remove --force "$R.wt/c-gi"
mkdir -p "$R.wt/c-gi/.omc" && echo s >"$R.wt/c-gi/.omc/s"
echo '*.tmp' >>"$R/.gitignore"
run clean --repo "$R" --branch c-gi
t "clean: 메인 .gitignore에 로컬 변경이 있으면 남은 폴더 판정을 멈춤" '[ $rc != 0 ] && grep -Fq .gitignore "$T/err" && [ -f "$R.wt/c-gi/.omc/s" ]'
git -C "$R" checkout -q -- .gitignore
run clean --repo "$R" --branch c-gi
t "clean: 되돌리면 정리" '[ $rc = 0 ] && gone c-gi'

(cd "$D/seed" && git pull -q --rebase origin main && mkdir -p .omc && echo k >.omc/keep && git add -f .omc/keep && git commit -qm keep && git push -q origin HEAD:main) && git -C "$R" pull -q --ff-only
land c-trk
git -C "$R" worktree remove --force "$R.wt/c-trk"
mkdir -p "$R.wt/c-trk/.omc" && echo changed >"$R.wt/c-trk/.omc/keep"
run clean --repo "$R" --branch c-trk
t "clean: 등록 없는 폴더의 추적 파일(무시 규칙에도 걸림)은 멈추고 그대로" '[ $rc != 0 ] && grep -Fq .omc/keep "$T/err" && [ "$(cat "$R.wt/c-trk/.omc/keep")" = changed ]'

git -C "$R" worktree add -q --detach "$R.wt/c-det" origin/main
(cd "$R.wt/c-det" && echo d >d && git add -- d && git commit -qm d -- d)
rm -rf "$R.wt/c-det"
run clean --repo "$R" --branch c-det
t "clean: 폴더가 없어도 등록의 분리 HEAD가 origin에 없으면 멈춤" '[ $rc != 0 ] && grep -Fq "워크트리 HEAD" "$T/err" && reg c-det'

# clean-qa(2026-11-19): 무시된 보관물 .qa는 메인 폴더의 같은 자리로 복사(덮지 않음)한 뒤 지운다 — 한 레인이 손으로 지우다 출력을 잃었다
mkdir -p "$R/.git/info" && echo '.qa/' >>"$R/.git/info/exclude"
land c-qa
mkdir -p "$R.wt/c-qa/.qa/d1/run" "$R.wt/c-qa/sub/.qa" "$R/.qa/d1"
echo new >"$R.wt/c-qa/.qa/d1/run/out 1.txt"; echo same >"$R.wt/c-qa/.qa/d1/s.txt"; echo same >"$R/.qa/d1/s.txt"; echo deep >"$R.wt/c-qa/sub/.qa/x"
run clean --dry-run --repo "$R" --branch c-qa
t "clean .qa --dry-run: 옮길 것만 알리고 복사·삭제 없음" '[ $rc = 0 ] && [ ! -e "$R/.qa/d1/run" ] && kept c-qa && printf "%s" "$out" | grep -Fq "[dry-run] 무시된 보관물" || { cat "$T/err"; printf "%s\n" "$out"; false; }'
run clean --repo "$R" --branch c-qa
t "clean .qa: 메인 폴더 같은 자리로 복사하고 정리(하위 폴더 .qa도)" '[ $rc = 0 ] && gone c-qa && [ "$(cat "$R/.qa/d1/run/out 1.txt")" = new ] && [ "$(cat "$R/sub/.qa/x")" = deep ] || { cat "$T/err"; printf "%s\n" "$out"; false; }'
t "clean .qa: 옮긴 경로를 한 줄로" 'printf "%s\n" "$out" | grep -F "옮김 $R.wt/c-qa/.qa → $R/.qa" | grep -Fq "파일 3개"'
land c-qa2
mkdir -p "$R.wt/c-qa2/.qa/d1" && echo mine >"$R.wt/c-qa2/.qa/d1/s.txt" && echo extra >"$R.wt/c-qa2/.qa/d1/e.txt"
run clean --repo "$R" --branch c-qa2
t "clean .qa: 메인에 같은 이름의 다른 파일 → 덮지 않고 그 파일을 알리고 멈춤(안 지움)" '[ $rc != 0 ] && [ "$(cat "$R/.qa/d1/s.txt")" = same ] && [ "$(cat "$R.wt/c-qa2/.qa/d1/s.txt")" = mine ] && kept c-qa2 && reg c-qa2 && grep -Fq "다름(메인 것 둠) .qa/d1/s.txt" "$T/err" || { cat "$T/err"; false; }'
rm "$R/.qa/d1/s.txt"
run clean --repo "$R" --branch c-qa2
t "clean .qa: 비교·정리한 뒤 다시 돌리면 옮기고 정리" '[ $rc = 0 ] && gone c-qa2 && [ "$(cat "$R/.qa/d1/s.txt")" = mine ] && [ "$(cat "$R/.qa/d1/e.txt")" = extra ] || { cat "$T/err"; false; }'
land c-qa3
mkdir -p "$R.wt/c-qa3/.qa/on" && echo far >"$R.wt/c-qa3/.qa/on/f"
run clean --on box --repo "$R" --branch c-qa3
t "clean --on .qa: 그 기계 안에서 메인 폴더로 복사하고 정리" '[ $rc = 0 ] && gone c-qa3 && [ "$(cat "$R/.qa/on/f")" = far ] || { cat "$T/err"; false; }'

mk t14
seed .gitignore '.omc/'
seed .claude/herdr-teardown.sh 'echo td-boom; exit 3'
land c-td
run clean --repo "$R" --branch c-td
t "clean: teardown이 실패해도 알리고 정리는 계속(herdr-teardown.sh 약속)" '[ $rc = 0 ] && gone c-td && grep -Fq td-boom "$T/err" && last "\"teardown\":\"failed\""'

# 15. hc-lane land: 검증 스크립트가 없으면 실패·push 없음, 있으면 rebase 뒤 검증하고 push, herdr-land.sh 우선, 거절이면 다시
lnd() { d=$1; shift; out=$(cd "$d" && "$LANE" land "$@" 2>"$T/err"); rc=$?; }
mklane() { run --name "$1" --repo "$R" -- true; wt="$R.wt/$1"; (cd "$wt" && echo "$1" >"$1.txt" && git add -- "$1.txt" && git commit -qm "$1" -- "$1.txt"); }
otop() { git -C "$D/origin.git" log -1 --format=%s "${1:-main}"; }
seedp() { (cd "$D/seed" && git pull -q --rebase origin "$B") && seed "$@"; }
mk t15
mklane l-nov
seed c c
lnd "$wt"
t "land: herdr-verify.sh가 없으면 실패, push 안 함" '[ $rc = 1 ] && [ "$(otop)" = c ] && grep -Fq herdr-verify.sh "$T/err"'
seed .claude/wt-verify.sh 'exit 0'
lnd "$wt"
t "land: 옛 wt-verify.sh만 있어도 실패(옛 이름 갈래 없음)" '[ $rc = 1 ] && [ "$(otop)" = .claude/wt-verify.sh ]'
seed .claude/herdr-verify.sh 'echo "verify $(git log -1 --format=%s) HERDR_LAND=${HERDR_LAND-}" >>"$HOME/verify.log"; echo verify-said-hi'
rm -f "$T/verify.log"
lnd "$wt"
t "land-log: 통과면 화면은 그대로(실패 줄·로그 경로 없음), 로그만 더 생김" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fxq verify-said-hi && ! grep -Fq "전체 로그" "$T/err" && grep -Fxq verify-said-hi "$(ls -t "$T/.local/state/hc-lane/land-logs/l-nov"-*.log | head -n 1)"'
t "land: 검증이 있으면 rebase 뒤(그 HEAD로) 검증하고 push" '[ $rc = 0 ] && [ "$(otop)" = l-nov ] && [ "$(git -C "$D/origin.git" log -2 --format=%s main | tail -n 1)" = .claude/herdr-verify.sh ] && grep -Fxq "verify l-nov HERDR_LAND=" "$T/verify.log"'
t "land: 착지 줄 출력" 'printf "%s" "$out" | grep -Fq "hc-lane land: 착지"'
run --name l-vf --repo "$R" -- true; wt="$R.wt/l-vf"
(cd "$wt" && printf 'echo vfail; exit 9\n' >.claude/herdr-verify.sh && git commit -qm vf -- .claude/herdr-verify.sh)
lnd "$wt"
t "land: 검증이 실패하면 멈추고 push 안 함" '[ $rc = 1 ] && [ "$(otop)" = l-nov ] && printf "%s" "$out" | grep -Fq vfail && grep -Fq "검증 실패" "$T/err"'
# 15a. land-log(2026-11-17 senior-web: 끝 15줄에 패키지 없는 FAIL만 남아 어느 시험인지 몰랐다): 검증 출력을 화면 그대로 + 로그 파일,
# 실패면 실패 줄(최대 40)과 로그 경로, 레인 안이면 「중간」 사건에도 로그 경로
LL=$T/.local/state/hc-lane/land-logs tab=$(printf '\t')
llog() { ls -t "$LL/$1"-*.log 2>/dev/null | head -n 1; }
run --name l-vlog --repo "$R" -- true; wt="$R.wt/l-vlog"
cat >"$wt/.claude/herdr-verify.sh" <<'EOF'
i=0; while [ $i -lt 3000 ]; do echo "noise-a $i"; i=$((i + 1)); done
echo '--- FAIL: TestFlaky (0.01s)'; printf 'FAIL\texample.com/pkg/x\t0.2s\n'
i=0; while [ $i -lt 3000 ]; do echo "noise-b $i"; i=$((i + 1)); done
echo '(fail) bun thing [3ms]'; echo 'not ok 4 - tap'; echo '✗ mocha it' >&2; echo 'panic: boom'; echo FAIL; exit 1
EOF
(cd "$wt" && git commit -qm vlog -- .claude/herdr-verify.sh)
HC_LANE=l-vlog lnd "$wt"; lg=$(llog l-vlog)
t "land-log: 검증 실패면 상태 폴더 land-logs/<레인>-*.log에 전체 출력(오류 출력 포함)" '[ $rc = 1 ] && [ -n "$lg" ] && grep -Fxq "noise-a 0" "$lg" && grep -Fxq "noise-b 2999" "$lg" && grep -Fxq "✗ mocha it" "$lg" || { ls -R "$T/.local/state" | head; false; }'
t "land-log: 화면에도 그대로(로그 tee)" 'printf "%s\n" "$out" | grep -Fxq "noise-b 2999"'
t "land-log: 긴 출력에서도 실패 줄을 뽑아 찍음(--- FAIL·FAIL<탭>패키지·(fail)·not ok·✗·panic)" 'grep -Fxq -- "--- FAIL: TestFlaky (0.01s)" "$T/err" && grep -Fxq "FAIL${tab}example.com/pkg/x${tab}0.2s" "$T/err" && grep -Fxq "(fail) bun thing [3ms]" "$T/err" && grep -Fxq "not ok 4 - tap" "$T/err" && grep -Fxq "✗ mocha it" "$T/err" && grep -Fxq "panic: boom" "$T/err" && ! grep -q noise "$T/err" || { cat "$T/err"; false; }'
t "land-log: 실패면 로그 경로 한 줄, push 안 함" '[ "$(otop)" = l-nov ] && grep -Fq "전체 로그 $lg" "$T/err"'
t "land-log: 레인 안(HC_LANE)이면 중간 사건에 로그 경로" 'tail -n 1 "$T/.local/state/hc-lane/events.tsv" | grep -F "${tab}l-vlog${tab}중간${tab}" | grep -Fq "$lg"'
run --name l-vcap --repo "$R" -- true; wt="$R.wt/l-vcap"
printf 'i=0; while [ $i -lt 100 ]; do echo "--- FAIL: TCap$i"; i=$((i + 1)); done; exit 1\n' >"$wt/.claude/herdr-verify.sh"
(cd "$wt" && git commit -qm vcap -- .claude/herdr-verify.sh)
lnd "$wt"
t "land-log: 실패 줄은 최대 40줄, 사건은 레인 밖이면 안 적음" '[ $rc = 1 ] && [ "$(grep -c -- "^--- FAIL: TCap" "$T/err")" = 40 ] && ! tail -n 1 "$T/.local/state/hc-lane/events.tsv" | grep -Fq l-vcap'
rm -f "$T/.local/state/hc-lane/events.tsv"   # 뒤 report 시험은 빈 events.tsv에서 시작한다
mklane l-dirty
echo more >>"$wt/l-dirty.txt"
lnd "$wt"
t "land: 커밋 안 된 변경이 있으면 멈춤" '[ $rc = 1 ] && grep -Fq "커밋 안 된" "$T/err" && [ "$(otop)" = l-nov ]'
lnd "$wt" main extra
t "land: 남는 인자는 거절" '[ $rc != 0 ] && grep -Fq "알 수 없는 인자: extra" "$T/err"'
(cd "$wt" && "$LANE" land --on box) >/dev/null 2>"$T/err"; rc=$?
t "land: 옵션(--on 등)은 거절" '[ $rc != 0 ] && grep -Fq "hc-lane land [<기본 브랜치>]" "$T/err"'
lnd "$T"
t "land: git 폴더 밖이면 거절" '[ $rc != 0 ] && grep -Fq "git 작업 폴더" "$T/err"'

mk t15r   # 거절 재시도: 첫 검증 동안 다른 쪽이 main에 먼저 넣는다
seed .claude/herdr-verify.sh 'n=$(cat "$HOME/race" 2>/dev/null || echo 0); echo $((n + 1)) >"$HOME/race"; [ "$n" -lt "$RACE_N" ] || exit 0; cd "$RACE_SEED" && git pull -q --rebase origin main && echo "$n" >"race$n" && git add "race$n" && git commit -qm "race$n" && HERDR_LAND=1 git push -q origin HEAD:main'
mklane l-race
RACE_SEED="$D/seed" RACE_N=1 lnd "$wt"
t "land: 거절되면 fetch·rebase·검증부터 다시 해 착지" '[ $rc = 0 ] && [ "$(otop)" = l-race ] && [ "$(git -C "$D/origin.git" log -2 --format=%s main | tail -n 1)" = race0 ] && [ "$(cat "$T/race")" = 2 ] && grep -Fq "거절됨" "$T/err"'
mklane l-race2
HERDR_LAND_TRIES=2 RACE_SEED="$D/seed" RACE_N=99 lnd "$wt"
t "land: 거절이 HERDR_LAND_TRIES번이면 종료 4, 레인 커밋은 안 나감" '[ $rc = 4 ] && [ "$(cat "$T/race")" = 4 ] && ! git -C "$D/origin.git" log --format=%s main | grep -qx l-race2'
mklane l-pf
git -C "$R" config remote.origin.pushurl "$T/없는 원격.git"
RACE_SEED="$D/seed" RACE_N=0 lnd "$wt"
git -C "$R" config --unset remote.origin.pushurl
t "land: 거절이 아닌 push 실패는 다시 하지 않고 멈춤" '[ $rc = 1 ] && grep -Fq "다른 쪽 push가 앞선 것이 아니다" "$T/err" && ! grep -Fq 거절됨 "$T/err"'

mk t15c   # rebase 충돌
seed x base
seed .claude/herdr-verify.sh 'exit 0'
run --name l-c --repo "$R" -- true; wt="$R.wt/l-c"
(cd "$wt" && echo lane >x && git commit -qm l-c -- x)
seed x other
lnd "$wt" main
t "land: rebase 충돌이면 종료 3, push 안 함" '[ $rc = 3 ] && [ "$(otop)" = x ] && grep -Fq "rebase --continue" "$T/err"'
git -C "$wt" rebase --abort 2>/dev/null

mk t15l   # .claude/herdr-land.sh가 있으면 그것에 맡긴다
seed .claude/herdr-verify.sh 'echo should-not-run; exit 9'
seed .claude/herdr-land.sh 'echo "land-script $1 HERDR_LAND=${HERDR_LAND-} $(basename "$PWD")"; exit 6'
mklane l-ls
lnd "$wt/.claude"
t "land: herdr-land.sh에 기본 브랜치·HERDR_LAND=1을 주고 맡김(하위 폴더에서 불러도 작업 폴더 최상위, 종료 코드 그대로)" '[ $rc = 6 ] && printf "%s" "$out" | grep -Fxq "land-script main HERDR_LAND=1 l-ls" && ! printf "%s" "$out" | grep -Fq should-not-run'
t "land-log: herdr-land.sh 출력도 로그로, 실패면 경로" 'lg=$(llog l-ls) && grep -Fxq "land-script main HERDR_LAND=1 l-ls" "$lg" && grep -Fq "전체 로그 $lg" "$T/err"'
mk t15q   # herdr-land.sh가 자기 검증 로그(.qa/<날짜>/herdr-verify-*)를 남기는 저장소: 그 폴더와 그 안 실패 줄도
seed .claude/herdr-land.sh 'd=.qa/$(date +%F)/herdr-verify-000000-1; mkdir -p "$d"; printf "x\n--- FAIL: TestQaDir (0.1s)\n" >"$d/go-test.log"; echo "──── go 로그 끝"; exit 1'
mklane l-qa
lnd "$wt"
t "land-log: herdr-land.sh가 남긴 이번 검증 로그 폴더와 그 안 실패 줄도 찍음" '[ $rc = 1 ] && grep -Fq "검증 로그 폴더 $wt/.qa/" "$T/err" && grep -Fxq -- "--- FAIL: TestQaDir (0.1s)" "$T/err" || { cat "$T/err"; false; }'

mk t15m master
seed .claude/herdr-verify.sh 'exit 0'
mklane l-m
lnd "$wt"
t "land: 기본 브랜치를 안 주면 origin/HEAD(master)" '[ $rc = 0 ] && [ "$(otop master)" = l-m ]'

# 16. hc-lane guard: 공용 git 폴더 pre-push 관문
g() { out=$("$LANE" guard "$@" 2>"$T/err"); rc=$?; }
mk t16
hookf="$R/.git/hooks/pre-push"
g status --repo "$R"
t "guard status: 없으면 종료 1" '[ $rc = 1 ] && printf "%s" "$out" | grep -Fq 없음'
g install --repo "$R"
t "guard install: 공용 git 폴더 hooks/pre-push, 실행 가능, 기본 브랜치 main" '[ $rc = 0 ] && [ -x "$hookf" ] && grep -Fxq "base=main" "$hookf"'
g status --repo "$R"
t "guard status: 깔리면 종료 0" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fq "기본 브랜치 main"'
g install --repo "$R"
t "guard install: 다시 해도 됨(자기 것은 새로 씀)" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fq 새로'
(cd "$R" && git pull -q --ff-only && echo m >m && git add -- m && git commit -qm m -- m)
before=$(otop)
git -C "$R" push -q origin HEAD:main 2>"$T/push.err"; prc=$?
t "guard: main으로 가는 손 push는 거절, 새 착지 줄을 알림" '[ $prc != 0 ] && grep -Fq "새 착지 줄: hc-lane land main" "$T/push.err" && [ "$(otop)" = "$before" ]'
run --name g-wt --repo "$R" -- true
(cd "$R.wt/g-wt" && echo w >w && git add -- w && git commit -qm w -- w && git push -q origin HEAD:main) 2>"$T/push.err"; prc=$?
t "guard: 레인 워크트리에서도 거절(공용 훅)" '[ $prc != 0 ] && grep -Fq "hc-lane land main" "$T/push.err" && [ "$(otop)" = "$before" ]'
g status --repo "$R.wt/g-wt"
t "guard status: 워크트리 경로로 물어도 같은 훅" '[ $rc = 0 ]'
git -C "$R" push -q origin HEAD:refs/heads/feat/x HEAD:refs/integ/x HEAD:refs/claims/x 2>"$T/push.err"; prc=$?
t "guard: 다른 브랜치·refs/integ·refs/claims는 통과" '[ $prc = 0 ] && for r in refs/heads/feat/x refs/integ/x refs/claims/x; do git -C "$D/origin.git" show-ref -q --verify $r || exit 1; done'
git -C "$R" push -q origin HEAD:refs/heads/feat/y HEAD:main 2>"$T/push.err"; prc=$?
t "guard: 여러 참조 가운데 main이 섞이면 전부 거절" '[ $prc != 0 ] && [ "$(otop)" = "$before" ] && ! git -C "$D/origin.git" show-ref -q --verify refs/heads/feat/y'
git -C "$R" push -q origin :main 2>"$T/push.err"; prc=$?
t "guard: main 지우기 push도 거절" '[ $prc != 0 ] && [ "$(otop)" = "$before" ]'
HERDR_LAND=1 git -C "$R" push -q origin HEAD:main 2>"$T/push.err"; prc=$?
t "guard: HERDR_LAND=1이면 main push 통과" '[ $prc = 0 ] && [ "$(otop)" = m ]'
seedp .claude/herdr-verify.sh 'exit 0'
lnd "$R.wt/g-wt"
t "guard + land: hc-lane land는 관문을 지나 착지" '[ $rc = 0 ] && [ "$(otop)" = w ] || { cat "$T/err"; false; }'
g remove --repo "$R"
t "guard remove: 자기 것을 지움" '[ $rc = 0 ] && [ ! -e "$hookf" ]'
(cd "$R" && git pull -q --ff-only && echo m2 >m2 && git add -- m2 && git commit -qm m2 -- m2 && git push -q origin HEAD:main) 2>"$T/push.err"; prc=$?
t "guard remove: 그 뒤 손 push 통과" '[ $prc = 0 ] && [ "$(otop)" = m2 ]'
g remove --repo "$R"
t "guard remove: 없으면 지울 것 없음, 종료 0" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fq "지울 것 없음"'
g install --repo "$R" --branch release
(cd "$R" && git push -q origin HEAD:main HEAD:refs/heads/next) 2>"$T/push.err"; prc=$?
git -C "$R" push -q origin HEAD:refs/heads/release 2>"$T/push2.err"; prc2=$?
t "guard install --branch: 그 브랜치만 막음" '[ $prc = 0 ] && [ $prc2 != 0 ] && grep -Fq "hc-lane land release" "$T/push2.err"'
g remove --repo "$R"

mk t16o   # 남의 pre-push 훅·core.hooksPath는 건드리지 않는다
hookf="$R/.git/hooks/pre-push"
printf '#!/bin/sh\necho mine\n' >"$hookf"; chmod +x "$hookf"
g install --repo "$R"
t "guard install: 다른 pre-push 훅이 있으면 덮지 않고 멈춤" '[ $rc != 0 ] && grep -Fq "다른 pre-push" "$T/err" && [ "$(sed -n 2p "$hookf")" = "echo mine" ]'
g status --repo "$R"
t "guard status: 다른 훅이면 종료 3" '[ $rc = 3 ]'
g remove --repo "$R"
t "guard remove: 남의 훅은 지우지 않음" '[ $rc != 0 ] && [ -f "$hookf" ]'
rm -f "$hookf"
git -C "$R" config core.hooksPath .githooks
g install --repo "$R"
t "guard install: core.hooksPath가 있으면 멈추고 아무것도 안 씀" '[ $rc != 0 ] && grep -Fq core.hooksPath "$T/err" && [ ! -e "$hookf" ] && [ ! -e "$R/.githooks/pre-push" ]'
git -C "$R" config --unset core.hooksPath
g bogus --repo "$R"
t "guard: 모르는 동작은 사용법(종료 2)" '[ $rc = 2 ]'
g install
t "guard: --repo 없으면 사용법(종료 2)" '[ $rc = 2 ]'

# 17. 옛 이름은 찾지도 부르지도 않는다
mk t17
seed .gitignore '.omc/'
seed .claude/wt-teardown.sh 'echo old >"$HOME/old-td"'
land c-old
run clean --repo "$R" --branch c-old
t "clean: 옛 이름 wt-teardown.sh는 부르지 않음" '[ $rc = 0 ] && gone c-old && [ ! -e "$T/old-td" ] && last "\"teardown\":\"none\""'
t "hc-lane 본문에 옛 이름 갈래(.claude/wt-*·WT_*·wt-land/verify/setup/teardown) 없음" '! grep -nE "\.claude/wt-|WT_[A-Z]|wt-(land|verify|setup|teardown)" "$LANE"'

# 18. 원격(origin)이 없는 저장소(맥의 game·hwanhon): 로컬 <기본>이 기준, 착지는 메인 클론의 로컬 <기본>을 fast-forward
D="$W/t18" B=main
rm -f "$HCLOG" "$HCLOG.exec"
R="$D/로컬 저장소"
git init -q -b main "$R"
lc() { (cd "$R" && mkdir -p "$(dirname "$1")" && printf '%s\n' "$2" >"$1" && git add -- "$1" && git commit -qm "$1" -- "$1"); }   # 메인 폴더에 커밋
ltop() { git -C "$R" log -1 --format=%s "${1:-main}"; }
lc .gitignore '.omc/'
lc .claude/herdr-setup.sh 'echo "$LANE_REPO|$LANE_BASE" > setup-ran'
run --name o-a --repo "$R" -- true
wt="$R.wt/o-a"
t "원격 없음: 레인 만들기 종료 0, 로컬 main 기준, 준비 훅 돎" '[ $rc = 0 ] && [ "$(git -C "$wt" rev-parse HEAD)" = "$(git -C "$R" rev-parse main)" ] && [ "$(cat "$wt/setup-ran")" = "$R|main" ] && has LANE_BASE=main || { cat "$T/err"; false; }'
t "원격 없음: 착지 줄은 hc-lane land main, 기준 로컬 main" 'printf "%s" "$out" | grep -Fq "&& hc-lane land main" && printf "%s" "$out" | grep -Fq "기준 로컬 main"'
rm -f "$wt/setup-ran"
(cd "$wt" && echo o-a >o-a.txt && git add -- o-a.txt && git commit -qm o-a -- o-a.txt)
lc m1 m1
lnd "$wt"
t "원격 없음 land: herdr-verify.sh가 없으면 실패, 로컬 main 그대로" '[ $rc = 1 ] && [ "$(ltop)" = m1 ] && grep -Fq herdr-verify.sh "$T/err"'
lc .claude/herdr-verify.sh 'echo "verify $(git log -1 --format=%s)" >>"$HOME/lverify.log"; n=$(cat "$HOME/lrace" 2>/dev/null || echo 0); echo $((n + 1)) >"$HOME/lrace"; [ "$n" -lt "${RACE_N:-0}" ] || exit 0; cd "$RACE_MAIN" && echo "$n" >"lrace$n" && git add "lrace$n" && git commit -qm "lrace$n"'
lnd "$wt"
t "원격 없음 land: 로컬 main 위로 rebase·검증 뒤 메인 클론의 main을 fast-forward(작업 트리도)" '[ $rc = 0 ] && [ "$(ltop)" = o-a ] && [ "$(git -C "$R" log -2 --format=%s main | tail -n 1)" = .claude/herdr-verify.sh ] && [ -f "$R/o-a.txt" ] && [ -z "$(git -C "$R" status --porcelain)" ] && grep -Fxq "verify o-a" "$T/lverify.log" || { cat "$T/err"; false; }'
run clean --repo "$R" --branch o-a
t "원격 없음 clean: fetch 없이 로컬 main 기준으로 정리" '[ $rc = 0 ] && gone o-a || { cat "$T/err"; false; }'
run --name o-b --repo "$R" -- true; wt="$R.wt/o-b"
(cd "$wt" && echo o-b >o-b.txt && git add -- o-b.txt && git commit -qm o-b -- o-b.txt)
run clean --repo "$R" --branch o-b
t "원격 없음 clean: 착지 전 브랜치는 멈춤(로컬 main 안내)" '[ $rc != 0 ] && grep -Fq "로컬 main" "$T/err" && kept o-b'
RACE_MAIN=$R RACE_N=2 lnd "$wt"   # 앞 착지에서 검증이 한 번 돌아 셈은 1부터
t "원격 없음 land: 검증 사이 로컬 main이 앞서 나가면 처음부터 다시 해 착지" '[ $rc = 0 ] && [ "$(ltop)" = o-b ] && [ "$(git -C "$R" log -2 --format=%s main | tail -n 1)" = lrace1 ] && grep -Fq 거절됨 "$T/err" || { cat "$T/err"; false; }'
run --name o-c --repo "$R" -- true; wt="$R.wt/o-c"
(cd "$wt" && echo lane >o-b.txt && git commit -qm o-c -- o-b.txt)
echo dirty >"$R/o-b.txt"
lnd "$wt"
t "원격 없음 land: 메인 폴더에 겹치는 커밋 안 된 변경이 있으면 멈추고 main 그대로" '[ $rc = 1 ] && [ "$(ltop)" = o-b ] && [ "$(cat "$R/o-b.txt")" = dirty ] && grep -Fq "fast-forward로 올리지 못했다" "$T/err"'
git -C "$R" checkout -q -- o-b.txt
git -C "$R" checkout -q -b side
lnd "$wt"
t "원격 없음 land: main이 아무 데도 체크아웃 안 됐으면 update-ref로 올림(메인 폴더 브랜치는 그대로)" '[ $rc = 0 ] && [ "$(ltop)" = o-c ] && [ "$(git -C "$R" symbolic-ref --short HEAD)" = side ] && [ "$(ltop side)" = o-b ] || { cat "$T/err"; false; }'
git -C "$R" checkout -q main
git -C "$R" branch -q -m main master
run --name o-m --repo "$R" -- true
t "원격 없음: main이 없고 master가 있으면 master 기준" '[ $rc = 0 ] && has LANE_BASE=master && printf "%s" "$out" | grep -Fq "hc-lane land master"'
git -C "$R" branch -q -m master main
g install --repo "$R"
t "원격 없음 guard install: 설치할 것 없음(종료 0), 훅을 만들지 않음" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fq "설치할 것 없음" && [ ! -e "$R/.git/hooks/pre-push" ]'
g status --repo "$R"
t "원격 없음 guard status: 설치할 것 없음(종료 0)" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fq "설치할 것 없음"'

# 19. hc-lane gc: main에 다 들어갔고(조상·rebase 둘 다) 깨끗하고 쓰는 터미널이 없는 작업 폴더·가지만 지운다.
#     --integ: main에 같은 내용이 있고 하루 지난 원격 refs/integ/*만 지운다. 나머지는 「남김」 한 줄
mk t19
seed .gitignore "$(printf '.omc/\n.qa/')"
seed .claude/herdr-teardown.sh 'echo "$LANE_BRANCH" >>"$HOME/gc-td"'
git -C "$R" pull -q --ff-only
old="$(( $(date +%s) - 172800 )) +0000"   # 이틀 전
gw() { git -C "$R" worktree add -q -b "$1" "$R.wt/$1" origin/main; }          # gw <가지>: origin/main 기준 작업 폴더
gcm() { (cd "$R.wt/$1" && echo "$1" >"$1.txt" && git add -- "$1.txt" && git commit -qm "$1" -- "$1.txt"); }
gw g-ff; gcm g-ff; git -C "$R.wt/g-ff" push -q origin HEAD:main            # fast-forward로 들어감
mkdir -p "$R.wt/g-ff/.omc" && echo s >"$R.wt/g-ff/.omc/s"                     # 무시 폴더는 같이 지운다
gw g-rb; gcm g-rb
(cd "$D/seed" && git pull -q --rebase origin main && seed z z && git fetch -q "$R" g-rb && git cherry-pick FETCH_HEAD >/dev/null && git push -q origin HEAD:main)  # rebase로 들어감(다른 sha)
gw g-un; gcm g-un                                                           # 안 들어감
gw g-dirty; echo changed >>"$R.wt/g-dirty/a"                                # 추적 파일 변경
gw g-untr; echo keep >"$R.wt/g-untr/note.txt"                               # 무시 안 된 untracked
gw g-qa; mkdir -p "$R.wt/g-qa/.qa/x" && echo r >"$R.wt/g-qa/.qa/x/REPORT.md"   # 무시된 보관물
gw g-live
git -C "$R" worktree add -q --detach "$R.wt/g-det" origin/main              # 분리 HEAD, 들어감
git -C "$R" fetch -q origin
gstate() { { git -C "$R" worktree list --porcelain; git -C "$R" for-each-ref; git -C "$D/origin.git" for-each-ref; } | cksum; }
gwt() { git -C "$R" worktree list --porcelain | grep -Fxq "worktree $R.wt/$1"; }
gbr() { git -C "$R" show-ref -q --verify "refs/heads/$1"; }
line_has() { printf '%s\n' "$out" | grep -F -- "$1" | grep -Fq -- "$2"; }
liveLS=$(printf '{"terminals":[{"address":"t5","agent":"claude-code","cwd":"%s","state":"live"}]}' "$R.wt/g-live/sub")
before=$(gstate)
HC_FAKE_LS=$liveLS run gc --repo "$R" --dry-run --min-age 0
t "gc --dry-run: 종료 0, 아무것도 안 바뀜" '[ $rc = 0 ] && [ "$(gstate)" = "$before" ] && [ ! -e "$T/gc-td" ] || { cat "$T/err"; false; }'
t "gc --dry-run: 지울 것·남길 것과 요약" 'line_has "$R.wt/g-ff" 지움 && line_has "$R.wt/g-un" 남김 && printf "%s\n" "$out" | tail -n 1 | grep -q "지움 3 · 남김 5"'
HC_FAKE_LS=$liveLS run gc --repo "$R" --min-age 0
t "gc: 종료 0, 요약 줄이 마지막" '[ $rc = 0 ] && printf "%s\n" "$out" | tail -n 1 | grep -q "^지움 3 · 남김 5$" || { cat "$T/err"; printf "%s\n" "$out"; false; }'
t "gc: fast-forward로 들어간 가지 → 폴더·가지 지움(무시 폴더도)" '! gwt g-ff && [ ! -e "$R.wt/g-ff" ] && ! gbr g-ff'
t "gc: rebase로 들어간 가지(git cherry) → 지움" '! gwt g-rb && [ ! -e "$R.wt/g-rb" ] && ! gbr g-rb'
t "gc: 분리 HEAD가 들어간 폴더 → 지움" '! gwt g-det && [ ! -e "$R.wt/g-det" ]'
t "gc: 지울 때 teardown을 돌림" 'grep -Fxq g-ff "$T/gc-td" && grep -Fxq g-rb "$T/gc-td"'
t "gc: 안 들어간 가지 → 남김" 'gwt g-un && gbr g-un && line_has "$R.wt/g-un" 남김'
t "gc: 추적 변경·무시 안 된 untracked → 남김" 'gwt g-dirty && [ "$(tail -n 1 "$R.wt/g-dirty/a")" = changed ] && gwt g-untr && [ -f "$R.wt/g-untr/note.txt" ] && line_has "$R.wt/g-dirty" 남김 && line_has "$R.wt/g-untr" 남김'
t "gc: 무시된 보관물(.qa) → 남김과 보고" 'gwt g-qa && [ -f "$R.wt/g-qa/.qa/x/REPORT.md" ] && line_has "$R.wt/g-qa" .qa'
t "gc: 살아 있는 터미널 cwd → 남김(주소)" 'gwt g-live && gbr g-live && line_has "$R.wt/g-live" t5'
t "gc: 메인 폴더·기본 가지는 그대로" '[ -d "$R/.git" ] && gbr main && [ "$(git -C "$R" symbolic-ref --short HEAD)" = main ] && ! printf "%s\n" "$out" | grep -Fxq "지움 $R"'
gw g-new
run gc --repo "$R"
t "gc: 기본 --min-age(24시간)면 방금 만든 폴더는 남김" '[ $rc = 0 ] && gwt g-new && line_has "$R.wt/g-new" 새것'
HC_FAKE_LS_FAIL=1 run gc --repo "$R" --min-age 0
t "gc: hc ls 실패면 아무것도 안 지우고 멈춤" '[ $rc != 0 ] && grep -Fq "hc ls" "$T/err" && gwt g-new'
# 감사 반영: 병합 커밋이 낀 가지·일부만 들어간 가지·잠긴 폴더·멈춘 rebase는 남김, 묵은 폴더는 status가 index를 다시 써도 새것이 아님
gw g-merge; gcm g-merge
(cd "$R.wt/g-merge" && git merge -q --no-ff --no-edit origin/main >/dev/null && git push -q origin HEAD~1:refs/heads/tmp-m) &&
  (cd "$D/seed" && git fetch -q origin && git checkout -q --detach origin/main && git cherry-pick origin/tmp-m >/dev/null && git push -q origin HEAD:main HEAD:refs/heads/x-sink :refs/heads/tmp-m && git checkout -q -B main origin/main)
gw g-part; gcm g-part; (cd "$R.wt/g-part" && echo p2 >p2 && git add -- p2 && git commit -qm p2 -- p2)
(cd "$D/seed" && git fetch -q origin && git fetch -q "$R" g-part && git checkout -q --detach origin/main && git cherry-pick FETCH_HEAD~1 >/dev/null && git push -q origin HEAD:main && git checkout -q -B main origin/main)
gw g-lock; git -C "$R" worktree lock "$R.wt/g-lock"
gw g-reb; (cd "$R.wt/g-reb" && GIT_SEQUENCE_EDITOR="sed -i.b 1s/^/break\\n/" git rebase -q -i HEAD~1 >/dev/null 2>&1)
git -C "$R" fetch -q origin
gw g-old; touch -t 202601010000 "$R.wt/g-old" "$R.wt/g-old/"* "$R.wt/g-old/".??* "$(git -C "$R.wt/g-old" rev-parse --absolute-git-dir)"/* "$(git -C "$R.wt/g-old" rev-parse --absolute-git-dir)"/logs/HEAD 2>/dev/null
gidx() { stat -f %m "$(git -C "$R.wt/g-old" rev-parse --absolute-git-dir)/index" 2>/dev/null || stat -c %Y "$(git -C "$R.wt/g-old" rev-parse --absolute-git-dir)/index"; }
i0=$(gidx)
run gc --repo "$R" --dry-run
t "gc --dry-run: index를 다시 쓰지 않음, 묵은 폴더는 새것이 아님" '[ "$(gidx)" = "$i0" ] && line_has "$R.wt/g-old" 지움 || { printf "%s\n" "$out"; false; }'
run gc --repo "$R" --min-age 0
t "gc: 병합 커밋이 낀 가지 → 남김" 'gwt g-merge && line_has "$R.wt/g-merge" "안 들어감"'
t "gc: 커밋 일부만 들어간 가지 → 남김" 'gwt g-part && line_has "$R.wt/g-part" "안 들어감"'
t "gc: 잠긴 작업 폴더 → 남김" 'gwt g-lock && line_has "$R.wt/g-lock" 잠긴'
t "gc: 멈춘 rebase가 있는 폴더 → 남김" 'gwt g-reb && line_has "$R.wt/g-reb" "진행 중 rebase-merge"'
t "gc: rebase 중인 작업 폴더의 가지(porcelain엔 분리 HEAD)는 가지로도 안 건드림, 종료 0" '[ $rc = 0 ] && gbr g-reb && ! printf "%s\n" "$out" | grep -Fq "가지 g-reb" || { cat "$T/err"; printf "%s\n" "$out"; false; }'
t "gc: 묵은 폴더는 지움" '! gwt g-old && ! gbr g-old'
run gc --repo "$R" --branch x
t "gc: --branch·--name은 받지 않음" '[ $rc != 0 ]'
run clean --repo "$R" --branch g-un --integ
t "gc 밖에서는 --integ를 받지 않음" '[ $rc != 0 ]'

# 19b. --integ
ic() {  # ic <참조> <파일> [이틀 전]: seed에서 origin/main 위 커밋을 그 참조로 올린다(main에는 안 넣음, R에는 객체 없음)
  (cd "$D/seed" && git fetch -q origin && git checkout -q --detach origin/main && echo "$2" >"$2" && git add -- "$2" &&
    if [ -n "${3-}" ]; then GIT_COMMITTER_DATE=$old git commit -qm "$2" -- "$2"; else git commit -qm "$2" -- "$2"; fi &&
    git push -q origin "HEAD:$1")
}
ic refs/integ/i-old-in i1 old; (cd "$D/seed" && git push -q origin HEAD:main)                 # 들어감(조상) + 오래됨
ic refs/integ/i-rb i2 old; s2=$(git -C "$D/seed" rev-parse HEAD)
(cd "$D/seed" && git checkout -q --detach origin/main && git pull -q --rebase origin main && seed z2 z2 && git cherry-pick "$s2" >/dev/null && git push -q origin HEAD:main)   # rebase로 들어감 + 오래됨
ic refs/integ/i-new i3; (cd "$D/seed" && git push -q origin HEAD:main)                         # 들어감 + 새것
ic refs/integ/i-out i4 old                                                                    # 안 들어감 + 오래됨
ic refs/claims/i-claim i5 old; (cd "$D/seed" && git push -q origin HEAD:refs/heads/refs/integ/i-trap HEAD:refs/tags/v1)  # 건드리면 안 되는 것
(cd "$D/seed" && git checkout -q -B main origin/main)
oref() { git -C "$D/origin.git" show-ref -q --verify "$1"; }
before=$(git -C "$D/origin.git" for-each-ref | cksum)
run gc --repo "$R" --integ --dry-run --min-age 0
t "gc --integ --dry-run: 원격 참조 그대로" '[ $rc = 0 ] && [ "$(git -C "$D/origin.git" for-each-ref | cksum)" = "$before" ] || { cat "$T/err"; false; }'
run gc --repo "$R" --integ
t "gc --integ: 종료 0" '[ $rc = 0 ] || { cat "$T/err"; false; }'
t "gc --integ: 들어감+오래됨 → 지우고 참조·sha를 찍음" '! oref refs/integ/i-old-in && printf "%s\n" "$out" | grep -E "^지움 refs/integ/i-old-in [0-9a-f]{40}$" >/dev/null'
t "gc --integ: rebase로 들어감+오래됨 → 지움" '! oref refs/integ/i-rb'
t "gc --integ: 들어감+새것 → 남김" 'oref refs/integ/i-new && line_has refs/integ/i-new 남김'
t "gc --integ: 안 들어감 → 남김" 'oref refs/integ/i-out && line_has refs/integ/i-out 남김'
t "gc --integ: refs/claims·태그·기본 가지·이름 끝만 같은 가지는 안 건드림" 'oref refs/claims/i-claim && oref refs/tags/v1 && oref refs/heads/main && oref refs/heads/refs/integ/i-trap'
run gc --repo "$R" --integ=refs/heads/
t "gc --integ: 접두가 refs/heads·tags·claims면 거절" '[ $rc != 0 ] && oref refs/heads/main'

# 19c. 작업 폴더 없이 남은 가지: 로컬 refs/heads는 늘, 원격 refs/heads는 --remote-branches일 때만.
#      들어갔고(조상·rebase) 오래됐고 남길 목록(gc.keep_branches)에 없으면 지움, 안 들어간 것은 「남김 … 안 들어감 n개 · 마지막 날짜」
mk t19c
git -C "$R" pull -q --ff-only
bo() {  # bo <가지> [old]: origin/main 위 커밋 하나인 가지(작업 폴더 없음). old면 커밋·reflog 시각을 이틀 전으로
  (if [ -n "${2-}" ]; then export GIT_COMMITTER_DATE=$old; fi
   cd "$R" && git fetch -q origin && git checkout -q -b "$1" origin/main && echo "$1" >"$1.txt" && git add -- "$1.txt" &&
   git commit -qm "$1" -- "$1.txt" && git checkout -q main)
}
bp() { git -C "$R" push -q origin "$1:main"; }   # fast-forward로 들어감
bo b-old old; bp b-old
bo b-ff; bp b-ff
bo b-new; bp b-new
bo b-keep; bp b-keep
bo b-head; bp b-head
bo b-rb; (cd "$D/seed" && git pull -q --rebase origin main && seed z3 z3 && git fetch -q "$R" b-rb && git cherry-pick FETCH_HEAD >/dev/null && git push -q origin HEAD:main)   # rebase로 들어감
bo b-un
bo b-merge; seed z4 z4
(cd "$R" && git fetch -q origin && git checkout -q b-merge && git merge -q --no-ff --no-edit origin/main >/dev/null && git checkout -q main)
(cd "$D/seed" && git fetch -q "$R" b-merge && git cherry-pick FETCH_HEAD^1 >/dev/null && git push -q origin HEAD:main)   # 내용은 들어갔지만 병합 커밋이 낌
(cd "$D/seed" && git fetch -q origin && git push -q origin origin/main:refs/heads/r-in origin/main:refs/heads/r-keep)
git -C "$R" push -q origin b-un:refs/heads/r-out
git -C "$R" fetch -q origin
git -C "$R" checkout -q b-head                                                # 지금 HEAD 가지(들어감)
mkdir -p "$T/.config/herdr-tools"; echo 'gc.keep_branches b-keep r-k*' >"$T/.config/herdr-tools/config"
before=$(gstate)
run gc --repo "$R" --dry-run --min-age 0 --remote-branches
t "gc 가지 --dry-run: 종료 0, 아무것도 안 바뀜" '[ $rc = 0 ] && [ "$(gstate)" = "$before" ] || { cat "$T/err"; printf "%s\n" "$out"; false; }'
t "gc 가지 --dry-run: 지울 가지·원격 가지를 찍음" 'line_has "가지 b-ff" 지움 && line_has "가지 b-old" 지움 && line_has refs/heads/r-in 지움 || { printf "%s\n" "$out"; false; }'
run gc --repo "$R"
t "gc 가지: 기본 --min-age면 오래된 들어간 가지만 지우고 sha를 찍음" '[ $rc = 0 ] && ! gbr b-old && printf "%s\n" "$out" | grep -E "^지움 가지 b-old [0-9a-f]{40}$" >/dev/null || { cat "$T/err"; printf "%s\n" "$out"; false; }'
t "gc 가지: 새것(--min-age 안) → 남김" 'gbr b-new && gbr b-ff && line_has "가지 b-new" 새것'
t "gc 가지: --remote-branches 없으면 원격 가지 그대로" 'oref refs/heads/r-in && oref refs/heads/r-out && ! printf "%s\n" "$out" | grep -Fq refs/heads/r-'
run gc --repo "$R" --min-age 0
t "gc 가지: 종료 0" '[ $rc = 0 ] || { cat "$T/err"; printf "%s\n" "$out"; false; }'
t "gc 가지: fast-forward·rebase로 들어간 가지 → 지움" '! gbr b-ff && ! gbr b-rb && ! gbr b-new'
t "gc 가지: 안 들어감 → 남김(개수·마지막 날짜)" 'gbr b-un && printf "%s\n" "$out" | grep -Eq "^남김 가지 b-un 안 들어감 1개 · 마지막 [0-9]{4}-[0-9]{2}-[0-9]{2}$" || { printf "%s\n" "$out"; false; }'
t "gc 가지: 병합 커밋이 낀 가지 → 남김" 'gbr b-merge && line_has "가지 b-merge" "안 들어감"'
t "gc 가지: 남길 목록 → 남김" 'gbr b-keep && line_has "가지 b-keep" 남길'
t "gc 가지: 기본·현재 HEAD 가지 → 그대로" 'gbr main && gbr b-head && [ "$(git -C "$R" symbolic-ref --short HEAD)" = b-head ]'
run gc --repo "$R" --min-age 0 --remote-branches
t "gc --remote-branches: 들어간 원격 가지 → 지우고 sha를 찍음" '[ $rc = 0 ] && ! oref refs/heads/r-in && printf "%s\n" "$out" | grep -E "^지움 refs/heads/r-in [0-9a-f]{40}$" >/dev/null || { cat "$T/err"; printf "%s\n" "$out"; false; }'
t "gc --remote-branches: 안 들어감 → 남김(개수)" 'oref refs/heads/r-out && line_has refs/heads/r-out "안 들어감 1개"'
t "gc --remote-branches: 남길 목록(무늬)·기본 가지 → 그대로" 'oref refs/heads/r-keep && oref refs/heads/main'
run clean --repo "$R" --branch b-un --remote-branches
t "gc 밖에서는 --remote-branches를 받지 않음" '[ $rc != 0 ]'
# 감사 반영: dry-run은 로컬에 없는 원격 가지 커밋을 받아도 원격 추적 참조(origin/main 포함)를 바꾸지 않는다
(cd "$D/seed" && git fetch -q origin && git checkout -q --detach origin/main && echo m1 >m1 && git add -- m1 && git commit -qm m1 -- m1 &&
  git push -q origin HEAD:main HEAD:refs/heads/r-m1 && git checkout -q -B main origin/main)
before=$(gstate)
run gc --repo "$R" --dry-run --min-age 0 --remote-branches
t "gc --remote-branches --dry-run: 커밋만 받고 참조는 그대로(옛 origin/main으로 판정)" '[ $rc = 0 ] && [ "$(gstate)" = "$before" ] && line_has refs/heads/r-m1 남김 || { cat "$T/err"; printf "%s\n" "$out"; false; }'
# 감사 반영: patch-id는 공백을 무시한다 — 공백만 다른 내용이 main에 들어간 가지는 안 들어감(지우지 않음)
git -C "$R" checkout -q main
(cd "$R" && git fetch -q origin && git checkout -q -b b-ws origin/main && echo 'z = 3' >w.txt && git add -- w.txt && git commit -qm ws -- w.txt && git checkout -q main)
(cd "$D/seed" && git fetch -q origin && git checkout -q --detach origin/main && echo '    z = 3' >w.txt && git add -- w.txt && git commit -qm ws -- w.txt &&
  git push -q origin HEAD:main && git checkout -q -B main origin/main)
run gc --repo "$R" --min-age 0
t "gc 가지: 공백만 다른 내용이 들어간 가지 → 남김(안 들어감)" '[ $rc = 0 ] && gbr b-ws && line_has "가지 b-ws" "안 들어감 1개" || { cat "$T/err"; printf "%s\n" "$out"; false; }'
rm -f "$T/.config/herdr-tools/config"

# 20. 레인 사건: hc-lane report·events·wait (지휘는 사건만 읽는다)
mk t20
EV=$T/.local/state/hc-lane/events.tsv
tab=$(printf '\t')
fld() { printf '%s\n' "$1" | cut -f"$2"; }   # fld <줄> <칸>
HC_LANE_MACHINE=mac HC_TERMINAL_ID=t5 HC_LANE=lane-q run report 완료 abc1234 "$(printf '요약\t탭\n줄')"
ln1=$(cat "$EV" 2>/dev/null)
t "report: 종료 0, events.tsv에 한 줄" '[ $rc = 0 ] && [ "$(grep -c . "$EV")" = 1 ] || { cat "$T/err"; false; }'
t "report: 여덟 칸(시각 UTC·기계·터미널·레인·상태·sha·한 줄·turn)" '[ "$(printf "%s\n" "$ln1" | awk -F"\t" "{print NF}")" = 8 ] && fld "$ln1" 1 | grep -Eq "^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$" && [ "$(fld "$ln1" 2)|$(fld "$ln1" 3)|$(fld "$ln1" 4)|$(fld "$ln1" 5)|$(fld "$ln1" 6)" = "mac|t5|lane-q|완료|abc1234" ]'
t "report: 한 줄의 탭·줄바꿈은 지움" '[ "$(fld "$ln1" 7)" = "요약 탭 줄" ]'
t "report: 화면에도 같은 사건" 'printf "%s" "$out" | grep -Fq "lane-q 완료" && printf "%s" "$out" | grep -Fq abc1234'
HC_LANE=lane-q run report 막힘 "이유만 있음"
t "report: sha 없이 한 줄만 주면 sha는 -" '[ $rc = 0 ] && [ "$(tail -n 1 "$EV" | cut -f5-7)" = "막힘${tab}-${tab}이유만 있음" ]'
HC_LANE=lane-q run report 판정 - "감사 완료"
t "report: 판정 - <한 줄>" '[ $rc = 0 ] && [ "$(tail -n 1 "$EV" | cut -f5-7)" = "판정${tab}-${tab}감사 완료" ]'
HC_LANE=lane-q run report 끝남 abc
t "report: 모르는 상태는 거절, 아무것도 안 씀" '[ $rc = 2 ] && [ "$(grep -c . "$EV")" = 3 ]'
HC_LANE=lane-q run report 완료 abc --name lane-n
t "report: 레인 이름은 --name이 \$HC_LANE보다 앞" '[ "$(tail -n 1 "$EV" | cut -f4)" = lane-n ]'
HC_LANE=lane-q run report 착지 6D295B6F 끝
t "report: 대문자 sha는 소문자로 적음(한 줄 칸으로 보내지 않음 — 감사 wait-landed [낮음] 3)" '[ $rc = 0 ] && [ "$(tail -n 1 "$EV" | cut -f5-7)" = "착지${tab}6d295b6f${tab}끝" ] || { tail -n 1 "$EV"; false; }'
HC_LANE=lane-q run report 막힘 DB 연결 실패
t "report: 7자 미만 16진 낱말은 한 줄 칸에 대문자 그대로(재감사 [낮음] 1)" '[ $rc = 0 ] && [ "$(tail -n 1 "$EV" | cut -f5-7)" = "막힘${tab}-${tab}DB 연결 실패" ] || { tail -n 1 "$EV"; false; }'
git -C "$R" checkout -q -b lane-br
out=$(cd "$R" && "$LANE" report 중간 2>"$T/err"); rc=$?
t "report: --name·\$HC_LANE 없으면 작업 트리 가지 이름" '[ $rc = 0 ] && [ "$(tail -n 1 "$EV" | cut -f4)" = lane-br ]'
git -C "$R" checkout -q main
out=$(cd "$R" && HC_LANE=lane-h "$LANE" report 중간 2>"$T/err")
t "report: \$HC_LANE이 가지 이름보다 앞" '[ "$(tail -n 1 "$EV" | cut -f4)" = lane-h ]'
t "report: 기계 이름 기본은 hostname" '[ "$(tail -n 1 "$EV" | cut -f2)" = "$(hostname -s)" ]'
# 작업 중 발견해 함께 고침(지휘 실측: 워커 기계 레인 t10 v5-lb → 「main 착지」): hc-lane clean으로 작업 트리를 지운 뒤 report하면
# 지금 폴더 가지(main)가 레인 칸에 찍혔다. 레인 이름은 가지보다 hc 터미널 이름이 먼저, 기본 가지 이름은 레인으로 쓰지 않는다
out=$(cd "$R" && HC_TERMINAL_ID=t10 HC_FAKE_GET_TURN=3 HC_FAKE_GET_NAME=v5-lb "$LANE" report 착지 abc1234 2>"$T/err"); rc=$?
t "report: --name·\$HC_LANE 없으면 가지(main)보다 hc 터미널 이름" '[ $rc = 0 ] && [ "$(tail -n 1 "$EV" | cut -f4)" = v5-lb ] || { tail -n 1 "$EV"; cat "$T/err"; false; }'
git -C "$R" checkout -q -b lane-br2
out=$(cd "$R" && HC_TERMINAL_ID=t10 HC_FAKE_GET_TURN=3 HC_FAKE_GET_NAME=v5-lb "$LANE" report 중간 2>"$T/err")
t "report: hc 터미널 이름이 가지 이름보다 앞" '[ "$(tail -n 1 "$EV" | cut -f4)" = v5-lb ]'
git -C "$R" checkout -q main
out=$(cd "$R" && "$LANE" report 중간 2>"$T/err"); rc=$?
t "report: 이름이 없고 가지가 기본 가지(main)면 레인 칸은 -" '[ $rc = 0 ] && [ "$(tail -n 1 "$EV" | cut -f4)" = - ] || { tail -n 1 "$EV"; false; }'

# 감사 반영 1·7: 긴 줄을 여럿이 함께 report해도 줄이 섞이지 않는다(줄 전체 1000바이트 안, 끝 반쪽 UTF-8 버림)
: >"$EV"
long=$(printf '가%.0s' $(seq 1 450))
for p_ in 1 2 3 4; do ( for i_ in $(seq 1 25); do HC_LANE=lane-c$p_ "$LANE" report 중간 - "$long" >/dev/null; done ) & done
wait
t "report: 긴 줄 동시 100건이 모두 8칸 한 줄" '[ "$(grep -c . "$EV")" = 100 ] && [ -z "$(awk -F"\t" "NF != 8" "$EV")" ]'
t "report: 줄은 1000바이트 안, 끝 UTF-8 글자 온전" '[ -z "$(LC_ALL=C awk "length > 1000" "$EV")" ] && iconv -f UTF-8 -t UTF-8 "$EV" >/dev/null 2>&1 && [ "$(LC_ALL=C tail -n 1 "$EV" | cut -f7 | LC_ALL=C wc -c | tr -d " ")" -gt 600 ]'
HC_LANE=$(printf 'n%.0s' $(seq 1 300)) HC_LANE_MACHINE=$(printf 'm%.0s' $(seq 1 300)) run report 완료 abc1234 "$long"
t "report: 긴 이름·기계 칸도 잘려 8칸·1000바이트 안" '[ $rc = 0 ] && [ "$(tail -n 1 "$EV" | awk -F"\t" "{print NF}")" = 8 ] && [ "$(LC_ALL=C tail -n 1 "$EV" | LC_ALL=C wc -c | tr -d " ")" -le 1001 ] && [ "$(tail -n 1 "$EV" | cut -f6)" = abc1234 ]'

# events: 이 기계 + 연결된 기계를 시각 순으로, 연결 안 된 기계는 「모름」
: >"$EV"
MH=$T/mh; export HC_FAKE_MACH_HOME=$MH
mkdir -p "$MH/m1/.local/state/hc-lane" "$MH/m2/.local/state/hc-lane"
ev() { printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$@"; }
ev8() { printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$@"; }   # 여덟째 칸 turn
ev 2026-11-16T02:00:00Z here t1 lane-l 중간 - 로컬 >>"$EV"
{ ev 2026-11-16T02:00:03Z host1 t7 lane-a 완료 aaaa1111 일; ev 2026-11-16T02:00:01Z host1 t8 lane-b 중간 - 둘; } >"$MH/m1/.local/state/hc-lane/events.tsv"
ev 2026-11-16T02:00:02Z host2 t9 lane-c 착지 cccc2222 셋 >"$MH/m2/.local/state/hc-lane/events.tsv"
export HC_FAKE_MACHINES='{"machines":[{"host":"h1","name":"m1","state":"connected"},{"name":"m2","host":"h2","state":"connected"},{"name":"m3","state":"connected"},{"name":"m4","state":"disconnected"}]}'
run events --all
evs=$(printf '%s\n' "$out" | grep "$tab")
t "events --all: 종료 0, 네 줄" '[ $rc = 0 ] && [ "$(printf "%s\n" "$evs" | grep -c .)" = 4 ] || { printf "%s\n" "$out"; cat "$T/err"; false; }'
t "events --all: 시각 순" '[ "$(printf "%s\n" "$evs" | cut -f4 | tr "\n" " ")" = "lane-l lane-b lane-c lane-a " ]'
t "events --all: 원격 줄은 hc 기계 이름과 <기계>/<터미널> 주소" 'printf "%s\n" "$evs" | grep -Fq "${tab}m1${tab}m1/t7${tab}lane-a${tab}" && printf "%s\n" "$evs" | grep -Fq "${tab}m2${tab}m2/t9${tab}"'
t "events --all: 연결 안 된 기계·exec 실패는 「모름 <기계>」 한 줄" 'printf "%s\n" "$out" | grep -Fxq "모름 m3" && printf "%s\n" "$out" | grep -Fxq "모름 m4"'
run events --on m2
t "events --on: 이 기계와 그 기계만" '[ $rc = 0 ] && [ "$(printf "%s\n" "$out" | cut -f4 | tr "\n" " ")" = "lane-l lane-c " ]'
run events --all --since 2026-11-16T02:00:02Z
t "events --since: 그 시각부터" '[ "$(printf "%s\n" "$out" | grep "$tab" | cut -f4 | tr "\n" " ")" = "lane-c lane-a " ]'
t "events: 아무것도 안 바꿈(seen 없음)" '[ ! -e "$T/.local/state/hc-lane/seen" ]'

# wait: 본 적 없는 사건을 찍고 0, 같은 사건은 다시 안 알림, 상한 75, 원격은 바퀴마다 기계당 한 번
export HC_FAKE_LS='{"terminals":[]}'
rm -f "$HCLOG.exec" "$HCLOG.ls"
: >"$T/.local/state/hc-lane/seen"   # 이미 있는 커서(바닥 없음): 고정 시각 사건을 그대로 본다
run wait --all --timeout 0
t "wait: 쌓인 새 사건을 찍고 종료 0" '[ $rc = 0 ] && [ "$(printf "%s\n" "$out" | grep -c "$tab")" = 4 ] || { printf "%s\n" "$out"; cat "$T/err"; false; }'
t "wait: 원격은 기계당 hc exec 한 번, 연결 안 된 기계는 안 부름" '[ "$(grep -Fx -A1 -- --on "$HCLOG.exec" | grep -cFx m1)" = 1 ] && [ "$(grep -Fx -A1 -- --on "$HCLOG.exec" | grep -cFx m4)" = 0 ]'
t "wait: 레인 터미널은 hc ls 한 번(--all·라벨)" '[ "$(grep -c . "$HCLOG.ls")" = 1 ] && grep -Fq -- "--label hc-lane=1" "$HCLOG.ls" && grep -Fq -- --all "$HCLOG.ls"'
run wait --all --timeout 0
t "wait: 같은 사건은 두 번 알리지 않음, 상한이면 75" '[ $rc = 75 ] && [ -z "$(printf "%s\n" "$out" | grep "$tab")" ]'
HC_LANE=lane-q run report 중간 - 반쯤
run wait --state 완료,착지 --timeout 0
t "wait --state: 고른 상태만, 아니면 75" '[ $rc = 75 ]'
run wait --timeout 0
t "wait: --state로 거른 사건은 다른 wait에 남음" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "lane-q${tab}중간${tab}-${tab}반쯤"'
HC_LANE_WAIT_TICK=1 "$LANE" wait --timeout 1 >"$T/wout" 2>"$T/werr" &
wpid=$!
sleep 2
HC_LANE=lane-q "$LANE" report 착지 dddd4444 끝 >/dev/null
wait $wpid; wrc=$?
t "wait: 기다리다 새 사건이 생기면 그 줄을 찍고 0" '[ $wrc = 0 ] && grep -Fq "lane-q${tab}착지${tab}dddd4444" "$T/wout" || { cat "$T/wout" "$T/werr"; false; }'
t "wait: 끝난 뒤 남은 프로세스 없음" '! kill -0 $wpid 2>/dev/null && [ -z "$(pgrep -P $wpid)" ] && [ -z "$(jobs -p)" ]'
run wait --timeout 0
t "wait: 그 사건도 다시 안 알림" '[ $rc = 75 ]'

# wait: 출력이 /dev/null이면 읽을 사람이 없으니 seen을 건드리지 않고 종료 2(2026-11-17 지휘의 >/dev/null 배경 wait가 사건을 삼킴)
cp "$EV" "$T/ev.bak"; HC_LANE=lane-q "$LANE" report 착지 ffff6666 끝 >/dev/null
seen0=$(cksum <"$T/.local/state/hc-lane/seen"); "$LANE" wait --timeout 0 >/dev/null 2>"$T/err"; nrc=$?
t "wait >/dev/null: 종료 2, 한국어 오류, seen 안 바뀜" '[ $nrc = 2 ] && grep -Fq "/dev/null" "$T/err" && [ "$(cksum <"$T/.local/state/hc-lane/seen")" = "$seen0" ]'
"$LANE" wait --timeout 0 >"$T/wout" 2>"$T/err"; frc=$?
t "wait >파일: 그 사건은 아직 남아 있어 찍는다" '[ $frc = 0 ] && grep -Fq "ffff6666" "$T/wout"'
cp "$T/ev.bak" "$EV"   # 뒤 시험이 events.tsv 줄 수를 센다

# 멈춤: report 없이 끝났거나 막힌 레인 터미널(hc-lane이 띄운 것 = 라벨 hc-lane=1)
ev8 2026-11-16T02:10:00Z here t43 lane-landed 착지 eeee5555 끝 3 >>"$EV"
run wait --timeout 0   # 위 착지 줄을 먼저 본다
export HC_FAKE_LS='{"terminals":[{"address":"t40","name":"lane-dead","state":"finished"},{"address":"m1/t41","name":"lane-blk","state":"live","status":"blocked"},{"address":"t42","name":"lane-ok","state":"live","status":"working"},{"address":"t43","name":"lane-landed","state":"finished","turn":4}]}'
run wait --timeout 0
t "wait 멈춤: --all 없으면 이 기계 레인만" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}t40${tab}lane-dead${tab}멈춤${tab}-${tab}뜻밖의 종료(finished)" && ! printf "%s" "$out" | grep -Fq lane-blk || { printf "%s\n" "$out"; false; }'
t "wait 멈춤: 일하는 레인·착지를 알린 레인은 아님" '! printf "%s" "$out" | grep -Fq lane-ok && ! printf "%s" "$out" | grep -Fq "lane-landed${tab}멈춤"'
run wait --all --timeout 0
t "wait 멈춤 --all: 다른 기계의 막힌 레인" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "m1/t41${tab}lane-blk${tab}멈춤${tab}-${tab}blocked" && ! printf "%s" "$out" | grep -Fq lane-dead'
run wait --all --timeout 0
t "wait 멈춤: 두 번 알리지 않음" '[ $rc = 75 ]'
t "wait: seen 말고는 events.tsv도 안 바꿈" '[ "$(grep -c . "$EV")" = 4 ]'

# 감사 반영 2·4·5: report 없이 done·idle로 쉬는 레인, turn이 바뀌면 다시, 같은 주소의 다른 레인
ev 2026-11-16T02:20:00Z here t51 lane-fin 완료 ffff6666 끝 >>"$EV"
ev 2026-11-16T02:20:01Z here t52 lane-mid 중간 - 반 >>"$EV"
ev 2026-11-16T02:20:02Z here t60 old-lane 착지 abcd7777 옛 >>"$EV"
lsI() { printf '{"terminals":[{"address":"t50","name":"lane-idle","state":"live","status":"done","turn":%s},{"address":"t51","name":"lane-fin","state":"live","status":"done","turn":2},{"address":"t52","name":"lane-mid","state":"live","status":"idle","turn":1},{"address":"t60","name":"new-lane","state":"finished"},{"address":"t70","name":"lane-b2","state":"live","status":"blocked","turn":%s}]}' "$1" "$2"; }
HC_FAKE_LS=$(lsI 3 1) run wait --timeout 0
t "멈춤: report 없이 done으로 쉬는 레인(turn 포함)" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}t50${tab}lane-idle${tab}멈춤${tab}-${tab}done turn 3" || { printf "%s\n" "$out"; false; }'
t "멈춤: 마지막 사건이 중간뿐이면 idle도" 'printf "%s\n" "$out" | grep -Fq "${tab}t52${tab}lane-mid${tab}멈춤${tab}-${tab}idle"'
t "멈춤: 완료를 알리고 쉬는 레인은 아님" '! printf "%s\n" "$out" | grep -Fq "lane-fin${tab}멈춤"'
t "멈춤: 같은 주소의 옛 레인 착지는 새 레인을 면제하지 않음" 'printf "%s\n" "$out" | grep -Fq "${tab}t60${tab}new-lane${tab}멈춤${tab}-${tab}뜻밖의 종료(finished)"'
HC_FAKE_LS=$(lsI 3 1) run wait --timeout 0
t "멈춤: turn이 그대로면 다시 안 알림" '[ $rc = 75 ]'
HC_FAKE_LS=$(lsI 4 5) run wait --timeout 0
t "멈춤: 다시 쉬거나(turn 4) 다시 막히면(turn 5) 또 알림" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "lane-idle${tab}멈춤${tab}-${tab}done turn 4" && printf "%s\n" "$out" | grep -Fq "lane-b2${tab}멈춤${tab}-${tab}blocked turn 5"'

# 재감사 A: report가 그때의 hc turn을 적고, 그 turn+1보다 뒤에 쉬면(이어 받은 일을 report 없이 끝냄) 멈춤
HC_FAKE_GET_TURN=5 HC_TERMINAL_ID=t80 HC_LANE=lane-t run report 완료 abc1234 첫 일
t "report: 여덟째 칸에 그때의 hc turn" '[ $rc = 0 ] && [ "$(tail -n 1 "$EV" | cut -f8)" = 5 ]'
t "report: 터미널 밖이면 turn -" '[ "$(HC_LANE=x "$LANE" report 중간 >/dev/null; tail -n 1 "$EV" | cut -f8)" = - ]'
lsT() { printf '{"terminals":[{"address":"t80","name":"lane-t","state":"live","status":"%s","turn":%s}]}' "$1" "$2"; }
HC_FAKE_LS=$(lsT done 6) run wait --timeout 0
t "멈춤 turn: 완료 report한 그 턴이 끝나 쉬는 것(turn+1)은 아님" '! printf "%s\n" "$out" | grep -Fq "lane-t${tab}멈춤"'
HC_FAKE_LS=$(lsT done 8) run wait --timeout 0
t "멈춤 turn: 완료 뒤 turn이 오른 done은 멈춤" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}t80${tab}lane-t${tab}멈춤${tab}-${tab}done turn 8" || { printf "%s\n" "$out"; false; }'
HC_FAKE_LS=$(lsT idle 8) run wait --timeout 0
t "멈춤 turn: 같은 turn에서 done→idle은 다시 안 알림(쉼으로 묶음)" '[ $rc = 75 ]'
HC_FAKE_LS='{"terminals":[{"address":"t81","name":"lane-nr","state":"live","status":"done","turn":3}]}' run wait --timeout 0
t "멈춤 쉼: report 없는 레인 done turn 3은 알림" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "lane-nr${tab}멈춤${tab}-${tab}done turn 3"'
HC_FAKE_LS='{"terminals":[{"address":"t81","name":"lane-nr","state":"live","status":"idle","turn":3}]}' run wait --timeout 0
t "멈춤 쉼: 같은 turn에서 idle로 바뀌어도 다시 안 알림" '[ $rc = 75 ]'
HC_FAKE_GET_TURN=7 HC_FAKE_GET_SLEEP=10.4258 HC_TERMINAL_ID=t82 HC_LANE=lane-slow run report 중간 - 느린 코어
t "report: hc get이 응답하지 않으면 3초 안에 turn -로 적음" '[ $rc = 0 ] && [ "$(tail -n 1 "$EV" | cut -f4,8)" = "lane-slow${tab}-" ]'
t "report: 느린 hc get을 끊은 뒤 남은 프로세스 없음(이 시험만의 표지 — 다른 검증의 sleep 10을 안 잡게)" '[ -z "$(pgrep -fx "sleep 10.4258")" ]'
# 재감사 낮음 2: 이번 바퀴에 못 읽은 기계의 레인은 멈춤으로 보지 않는다(그 기계 사건을 모르니)
mkdir -p "$T/mh2/m1"
HC_FAKE_MACH_HOME=$T/mh2 HC_FAKE_MACHINES='{"machines":[{"name":"m1","state":"connected"},{"name":"m3","state":"connected"},{"name":"m4","state":"disconnected"}]}' \
  HC_FAKE_LS='{"terminals":[{"address":"m1/t91","name":"lane-r1","state":"live","status":"done","turn":2},{"address":"m3/t92","name":"lane-r3","state":"live","status":"done","turn":2},{"address":"m4/t93","name":"lane-r4","state":"finished"},{"address":"m3/t94","name":"lane-r5","state":"live","status":"blocked","turn":3}]}' \
  run wait --all --timeout 0
t "멈춤: 읽은 기계 레인은 알리고, 모름 기계 레인은 판정 대신 「연결 안 됨」(하루 한 번)" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "m1/t91${tab}lane-r1${tab}멈춤" && printf "%s\n" "$out" | grep -Fq "m3/t92${tab}lane-r3${tab}멈춤${tab}-${tab}연결 안 됨 m3" && printf "%s\n" "$out" | grep -Fq "m4/t93${tab}lane-r4${tab}멈춤${tab}-${tab}연결 안 됨 m4" && ! printf "%s\n" "$out" | grep -Eq "lane-r[34]${tab}멈춤${tab}-${tab}(done|뜻밖)" || { printf "%s\n" "$out"; cat "$T/err"; false; }'
t "멈춤: 모름 기계 레인이라도 blocked는 그대로 알림(감사 4차 [낮음] 1)" 'printf "%s\n" "$out" | grep -Fq "m3/t94${tab}lane-r5${tab}멈춤${tab}-${tab}blocked turn 3"'

# 작업 중 발견해 함께 고침: macOS awk는 UTF-8 로캘에서 문자열 비교가 strcoll이라 "착지" == "멈춤"이 참이었다
# — 같은 한 줄로 완료→착지를 알리면 착지가 삼켜지고, --state 한 상태 거름이 꺼졌다. 비교는 LC_ALL=C로 한다
HC_LANE=lane-s HC_TERMINAL_ID=t99 run report 완료 abc1234 같은 줄
run wait --timeout 0
HC_LANE=lane-s HC_TERMINAL_ID=t99 run report 착지 abc1234 같은 줄
LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8 run wait --timeout 0
t "wait: 같은 레인·같은 한 줄이라도 완료 뒤 착지를 알림(한글 상태 비교)" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "lane-s${tab}착지${tab}abc1234" || { printf "%s\n" "$out"; false; }'
HC_LANE=lane-s run report 중간 - 반쯤 둘
LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8 run wait --state 멈춤 --timeout 0
t "wait --state 멈춤: 중간 사건은 거름(한 상태만 골라도)" '[ $rc = 75 ] && ! printf "%s\n" "$out" | grep -Fq "반쯤 둘"'
run wait --timeout 0

# wait-grace(2026-11-16 실측: 배경 셸 둘이 도는 동안 턴을 끝낸 Claude 레인을 멈춤으로 알렸다):
# done·idle은 화면 끝에 배경 작업 표시가 없고 그 status가 된 지 lane.wait_grace분(기본 10)이 지나야 멈춤. blocked는 바로
GS="$T/gscreen"
lsG() { printf '{"terminals":[{"address":"%s","name":"lane-g","state":"live","status":"%s","turn":%s}]}' "$1" "$2" "$3"; }
printf '%s\n' '● 검증을 배경으로 돌린다' '' '✻ Churned for 3m · done 12:33 PM · 2 shells still running' '────' '❯ ' '────' '  footer' >"$GS"
rm -f "$HCLOG.get" "$HCLOG.read"
HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=1200 HC_FAKE_LS=$(lsG tg1 done 2) run wait --timeout 0
t "유예: 화면 끝에 배경 셸 표시가 있으면 20분 쉬어도 멈춤 아님" '[ $rc = 75 ] && ! printf "%s\n" "$out" | grep -Fq "lane-g${tab}멈춤" && grep -Fxq tg1 "$HCLOG.read" || { printf "%s\n" "$out"; cat "$T/err"; false; }'
printf '%s\n' '✻ Worked · done · 1 monitor still running' '❯ ' >"$GS"
HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=1200 HC_FAKE_LS=$(lsG tg1 done 2) run wait --timeout 0
t "유예: monitor still running도 배경 작업" '[ $rc = 75 ]'
printf '%s\n' '✻ Churned for 3m · done 12:33 PM' '❯ ' >"$GS"
rm -f "$HCLOG.read"
HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=300 HC_FAKE_LS=$(lsG tg2 idle 2) run wait --timeout 0
t "유예: 표시가 없어도 쉰 지 10분 안이면 멈춤 아님(화면도 안 읽음)" '[ $rc = 75 ] && ! printf "%s\n" "$out" | grep -Fq "lane-g${tab}멈춤" && ! grep -Fxq tg2 "$HCLOG.read" 2>/dev/null'
HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=660 HC_FAKE_LS=$(lsG tg2 idle 2) run wait --timeout 0
t "유예: 표시 없고 10분 넘게 쉬면 멈춤" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}tg2${tab}lane-g${tab}멈춤${tab}-${tab}idle turn 2" || { printf "%s\n" "$out"; cat "$T/err"; false; }'
rm -f "$HCLOG.get"
HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=660 HC_FAKE_LS=$(lsG tg2 done 2) run wait --timeout 0
t "유예: 이미 알린 쉼은 다시 안 알리고 hc get도 안 부름" '[ $rc = 75 ] && ! grep -Fxq tg2 "$HCLOG.get" 2>/dev/null'
printf '%s\n' '✻ done · 2 shells still running' >"$GS"
rm -f "$HCLOG.get"
HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=5 HC_FAKE_LS=$(lsG tg3 blocked 2) run wait --timeout 0
t "유예 없음: blocked는 배경 표시·10분 안이어도 바로 멈춤(hc get 안 부름)" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}tg3${tab}lane-g${tab}멈춤${tab}-${tab}blocked turn 2" && ! grep -Fxq tg3 "$HCLOG.get" 2>/dev/null'
# 감사 반려 [중간] 1: 배경 표시는 마지막 턴 끝 줄(✻ 줄)에서만 보고, 쉼이 lane.wait_busy_max분(기본 30)을 넘으면 표시와 상관없이 멈춤
tail14() { for i_ in $(seq 1 14); do echo "  꼬리 $i_"; done; }
{ echo '✻ Crunched for 13m · done 12:33 PM · 2 shells still running'; echo '⏺ 배경 검증 통과. 끝.'; echo '✻ Cooked for 9s · done 12:48 PM'; echo '────'; echo '❯ '; tail14; } >"$GS"
HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=660 HC_FAKE_LS=$(lsG th1 done 3) run wait --timeout 0
t "유예: 옛 턴 끝 줄의 배경 표시는 무시 — 마지막 ✻ 줄에 없으면 10분 뒤 멈춤" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}th1${tab}lane-g${tab}멈춤${tab}-${tab}done turn 3" || { printf "%s\n" "$out"; cat "$T/err"; false; }'
{ echo '⏺ 검증을 배경으로'; echo '✻ Churned for 3m · done 12:33 PM · 2 shells still running'; echo '────'; echo '❯ '; tail14; } >"$GS"
HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=1500 HC_FAKE_LS=$(lsG th2 done 3) run wait --timeout 0
t "유예: 마지막 ✻ 줄에 배경 표시면 30분 안은 멈춤 아님" '[ $rc = 75 ]'
HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=1860 HC_FAKE_LS=$(lsG th2 done 3) run wait --timeout 0
t "유예 상한: 배경 표시가 있어도 30분 넘게 쉬면 멈춤" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}th2${tab}lane-g${tab}멈춤${tab}-${tab}done turn 3" || { printf "%s\n" "$out"; cat "$T/err"; false; }'
# 감사 [낮음] 3: hc read 실패는 배경 작업 없음으로(멈춤), hc get 실패·status_ms 없음은 유예 없이 멈춤
HC_FAKE_READ_FAIL=1 HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=700 HC_FAKE_LS=$(lsG th3 done 3) run wait --timeout 0
t "실패: hc read가 실패하면 배경 작업으로 숨기지 않고 멈춤" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}th3${tab}lane-g${tab}멈춤"'
HC_FAKE_SCREEN=$GS HC_FAKE_LS=$(lsG th4 done 3) run wait --timeout 0
t "실패: hc get이 실패하면(status_ms 모름) 배경 표시가 있어도 바로 멈춤" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}th4${tab}lane-g${tab}멈춤"'
# 감사 [낮음] 2: 같은 turn에서 done→idle로 바뀌어 status_ms가 새로 찍혀도 처음 본 쉼 시각부터 센다
printf '%s\n' '✻ Churned · done · 1 shell still running' '❯ ' >"$GS"
HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=650 HC_FAKE_LS=$(lsG th5 done 3) run wait --timeout 0
t "쉼 시각: 배경 셸 표시로 붙든 done(11분)은 멈춤 아님" '[ $rc = 75 ]'
printf '%s\n' '✻ Churned · done' '❯ ' >"$GS"
HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=5 HC_FAKE_LS=$(lsG th5 idle 3) run wait --timeout 0
t "쉼 시각: 같은 turn에서 idle로 바뀌어 status_ms가 새로 찍혀도 처음 쉼부터 세어 멈춤" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}th5${tab}lane-g${tab}멈춤${tab}-${tab}idle turn 3" || { printf "%s\n" "$out"; false; }'
conf 'lane.wait_grace 0' 'lane.wait_busy'
HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=5 HC_FAKE_LS=$(lsG tg4 done 2) run wait --timeout 0
t "설정: lane.wait_grace 0·lane.wait_busy 빈 값이면 유예 끔(바로 멈춤)" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}tg4${tab}lane-g${tab}멈춤"'
conf 'lane.wait_grace 1' 'lane.wait_busy compiling'
printf '%s\n' '✻ done · compiling' >"$GS"
HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=120 HC_FAKE_LS=$(lsG tg5 done 2) run wait --timeout 0
t "설정: 표시 글자 목록과 분을 바꿀 수 있음" '[ $rc = 75 ]'
conf 'lane.wait_grace 열'
HC_FAKE_LS=$(lsG tg6 done 2) run wait --timeout 0
t "설정: lane.wait_grace가 숫자가 아니면 오류" '[ $rc != 0 ] && [ $rc != 75 ] && grep -Fq lane.wait_grace "$T/err"'
noconf

# quiet-alerts 1: 배경 작업은 화면이 아니라 프로세스로 — 터미널 pid 아래 Claude Bash 도구 셸(명령 줄에 shell-snapshots)이 살아 있으면
# 배경 작업 중. MCP 서버처럼 늘 사는 자손·남의 셸은 안 셈. ps가 실패하면 화면 판정, 30분 상한은 그대로
PSF="$T/fakeps"
snap='/bin/zsh -c source /Users/u/.claude/shell-snapshots/snapshot-zsh-1.sh >/dev/null 2>&1 && eval '"'"'bash verify.sh'"'"''
{ echo " 4000     1 node agent-runner.js"; echo " 4001  4000 claude --model x"; echo " 4002  4001 $snap"
  echo " 4003  4001 node /x/mcp-server.js"; echo " 5001     1 $snap"; } >"$PSF"
printf '%s\n' '✻ Churned for 3m · done 12:33 PM' '❯ ' >"$GS"   # 화면에는 배경 표시가 없다(화면 판정이면 멈춤)
rm -f "$HCLOG.ps"
HC_FAKE_PID=4000 HC_FAKE_PS=$PSF HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=660 HC_FAKE_LS=$(lsG tp1 done 2) run wait --timeout 0
t "프로세스: 터미널 pid 자손에 shell-snapshots 셸이 살아 있으면 화면 표시가 없어도 멈춤 아님" '[ $rc = 75 ] && grep -Fq -- "-A -o pid=,ppid=,command=" "$HCLOG.ps" || { printf "%s\n" "$out"; cat "$T/err"; false; }'
{ echo " 4000     1 node agent-runner.js"; echo " 4001  4000 claude --model x"; echo " 4003  4001 node /x/mcp-server.js"
  echo " 5001     1 $snap"; echo " 5002  4100 $snap"; } >"$PSF"
printf '%s\n' '✻ Churned for 3m · done 12:33 PM · 2 shells still running' '❯ ' >"$GS"   # 화면은 배경 표시(옛 글자)
HC_FAKE_PID=4000 HC_FAKE_PS=$PSF HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=660 HC_FAKE_LS=$(lsG tp2 done 2) run wait --timeout 0
t "프로세스 없음이어도 화면에 배경 표시가 있으면 멈춤 아님(셸 없는 배경 작업 — 감사 [낮음] 4)" '[ $rc = 75 ] || { printf "%s\n" "$out"; cat "$T/err"; false; }'
printf '%s\n' '✻ Churned for 3m · done 12:33 PM' '❯ ' >"$GS"
HC_FAKE_PID=4000 HC_FAKE_PS=$PSF HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=660 HC_FAKE_LS=$(lsG tp2b done 2) run wait --timeout 0
t "프로세스: 자손이 MCP 서버 꼴뿐이고(남의 셸은 안 셈) 화면 표시도 없으면 유예 뒤 멈춤" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}tp2b${tab}lane-g${tab}멈춤${tab}-${tab}done turn 2" || { printf "%s\n" "$out"; cat "$T/err"; false; }'
printf '%s\n' '✻ Churned for 3m · done 12:33 PM · 2 shells still running' '❯ ' >"$GS"
HC_FAKE_PID=4000 HC_FAKE_PS=$PSF HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=300 HC_FAKE_LS=$(lsG tp3 done 2) run wait --timeout 0
t "프로세스: 유예(10분) 안이면 자손이 없어도 멈춤 아님" '[ $rc = 75 ]'
HC_FAKE_PS_FAIL=1 HC_FAKE_PID=4000 HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=660 HC_FAKE_LS=$(lsG tp4 done 2) run wait --timeout 0
t "프로세스: ps가 실패하면 화면 판정(배경 표시 → 멈춤 아님)" '[ $rc = 75 ]'
{ echo " 4000     1 node agent-runner.js"; echo " 4002  4000 $snap"; } >"$PSF"
HC_FAKE_PID=4000 HC_FAKE_PS=$PSF HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=1860 HC_FAKE_LS=$(lsG tp5 done 2) run wait --timeout 0
t "프로세스 상한: 배경 셸이 살아 있어도 30분 넘게 쉬면 멈춤" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}tp5${tab}lane-g${tab}멈춤" || { printf "%s\n" "$out"; false; }'
rm -f "$HCLOG.exec" "$HCLOG.ps"
HC_FAKE_MACHINES='{"machines":[{"name":"m1","state":"connected"}]}' HC_FAKE_PID=4000 HC_FAKE_PS=$PSF HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=660 \
  HC_FAKE_LS='{"terminals":[{"address":"m1/tq1","name":"lane-q1","state":"live","status":"done","turn":2},{"address":"m1/tq2","name":"lane-q2","state":"live","status":"idle","turn":2}]}' \
  run wait --all --timeout 0
t "프로세스 원격: 기계당 hc exec --on 한 번(ps), 두 레인 다 배경 작업 중" '[ $rc = 75 ] && [ "$(grep -c . "$HCLOG.ps")" = 1 ] && [ "$(grep -Fxc ps "$HCLOG.exec")" = 1 ] || { printf "%s\n" "$out"; cat "$T/err" "$HCLOG.exec"; false; }'
conf 'lane.wait_procs'
printf '%s\n' '✻ Churned for 3m · done 12:33 PM' '❯ ' >"$GS"
HC_FAKE_PID=4000 HC_FAKE_PS=$PSF HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=660 HC_FAKE_LS=$(lsG tp6 done 2) run wait --timeout 0
t "설정: lane.wait_procs 빈 값이면 프로세스 판정 끔(화면만)" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}tp6${tab}lane-g${tab}멈춤"'
noconf

# quiet-alerts 2·3(감사 반려 [높음] 1·2): 조용히 넘기는 것은 마지막 report가 착지·판정일 때의 끝남·정리됨뿐.
# 그 밖(완료·막힘·중간·report 없음)의 끝남은 「뜻밖의 종료(<상태>)」, 정리됨(hc-sweep이 닫고 hc rm — 목록에서 사라짐)은 그 사건 자체를
# 「뜻밖의 종료(정리됨)」 멈춤으로 알린다(wait가 안 돌던 틈에 닫혀도 다음 wait에서 나오게)
ev 2026-11-16T03:00:00Z here tf1 lane-f1 완료 abcd1234 끝 >>"$EV"
ev 2026-11-16T03:00:01Z here tf3 lane-f3 중간 - 반 >>"$EV"
ev 2026-11-16T03:00:02Z here tf3 lane-f3 정리됨 - "hc-sweep R2" >>"$EV"
ev8 2026-11-16T03:00:03Z here tf4 lane-f4 판정 - 감사 3 >>"$EV"
ev8 2026-11-16T03:00:04Z here tf4 lane-f4 정리됨 - "hc-sweep R2" 4 >>"$EV"
ev 2026-11-16T03:00:05Z here tf7 lane-f7 막힘 - 질문 >>"$EV"
ev8 2026-11-16T03:00:06Z here tf8 lane-f8 착지 abcd1234 끝 2 >>"$EV"
ev 2026-11-16T03:00:07Z here tf10 lane-f10 판정 - turn 모름 >>"$EV"
ev 2026-11-16T03:00:08Z here tf11 lane-f11 착지 abcd1234 turn 모름 >>"$EV"
ev 2026-11-16T03:00:09Z here tf11 lane-f11 정리됨 - "hc-sweep R2" >>"$EV"
HC_FAKE_LS='{"terminals":[{"address":"tf1","name":"lane-f1","state":"finished"},{"address":"tf2","name":"lane-f2","state":"finished"},{"address":"tf3","name":"lane-f3","state":"finished"},{"address":"tf4","name":"lane-f4","state":"finished"},{"address":"tf7","name":"lane-f7","state":"finished"},{"address":"tf8","name":"lane-f8","state":"finished","turn":3},{"address":"tf10","name":"lane-f10","state":"finished","turn":3}]}' run wait --timeout 0
t "끝남·정리됨: report turn을 모르면 착지·판정 뒤라도 알림(감사 3차 [낮음] 3)" 'printf "%s\n" "$out" | grep -Fq "${tab}tf10${tab}lane-f10${tab}멈춤${tab}-${tab}뜻밖의 종료(finished)" && printf "%s\n" "$out" | grep -Fq "${tab}tf11${tab}lane-f11${tab}멈춤${tab}-${tab}뜻밖의 종료(정리됨)" || { printf "%s\n" "$out"; false; }'
t "끝남: report 없이 끝나면 「뜻밖의 종료」" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}tf2${tab}lane-f2${tab}멈춤${tab}-${tab}뜻밖의 종료(finished)" || { printf "%s\n" "$out"; false; }'
t "끝남: 완료(착지 전) 뒤 끝남은 알림(옛 판 회귀 — 감사 [높음] 1)" 'printf "%s\n" "$out" | grep -Fq "${tab}tf1${tab}lane-f1${tab}멈춤${tab}-${tab}뜻밖의 종료(finished)"'
t "끝남: 막힘 뒤 끝남도 알림" 'printf "%s\n" "$out" | grep -Fq "${tab}tf7${tab}lane-f7${tab}멈춤${tab}-${tab}뜻밖의 종료(finished)"'
t "끝남: 착지·판정 뒤 끝남은 안 알림" '! printf "%s\n" "$out" | grep -Eq "lane-f[48]${tab}멈춤${tab}-${tab}뜻밖의 종료\(finished"'
t "정리됨: 중간 뒤 정리됨은 「뜻밖의 종료(정리됨)」, 그 끝남은 두 번 안 알림" 'printf "%s\n" "$out" | grep -Fq "${tab}tf3${tab}lane-f3${tab}멈춤${tab}-${tab}뜻밖의 종료(정리됨) hc-sweep R2" && ! printf "%s\n" "$out" | grep -Fq "lane-f3${tab}멈춤${tab}-${tab}뜻밖의 종료(finished)"'
t "정리됨: 판정 뒤 정리됨은 안 알림, 정리됨 줄 그대로는 안 찍음" '! printf "%s\n" "$out" | grep -Fq "lane-f4${tab}멈춤" && ! printf "%s\n" "$out" | grep -Fq "${tab}정리됨${tab}"'
run events
t "정리됨: hc-lane events에는 보임" 'printf "%s\n" "$out" | grep -Fq "lane-f3${tab}정리됨"'
HC_FAKE_LS='{"terminals":[{"address":"tf5","name":"lane-f5","state":"live","status":"done","turn":3}]}' run wait --timeout 0
# 감사 [높음] 2 재현 꼴: wait가 안 도는 사이에 report 없이 쉬던 레인을 hc-sweep이 닫고 지움(hc ls에 없음) → 다음 wait --state 멈춤에서 나옴
ev "$(date -u +%Y-%m-%dT%H:%M:%SZ)" here tf9 lane-f9 중간 - 반 >>"$EV"
ev "$(date -u +%Y-%m-%dT%H:%M:%SZ)" here tf9 lane-f9 정리됨 - "hc-sweep R2" >>"$EV"
HC_FAKE_LS='{"terminals":[]}' run wait --state 멈춤 --timeout 0
t "정리됨: 틈에 닫혀 목록에서 사라진 보고 없는 레인도 다음 wait에서 멈춤" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}tf9${tab}lane-f9${tab}멈춤${tab}-${tab}뜻밖의 종료(정리됨)" || { printf "%s\n" "$out"; cat "$T/err"; false; }'
HC_FAKE_LS='{"terminals":[]}' run wait --state 멈춤 --timeout 0
t "정리됨: 같은 정리됨은 두 번 안 알림" '[ $rc = 75 ]'
ev "$(date -u +%Y-%m-%dT%H:%M:%SZ)" here tf6 lane-f6 완료 abcd1234 끝 >>"$EV"
ev "$(date -u +%Y-%m-%dT%H:%M:%SZ)" here tf6 lane-f6 정리됨 - "hc-sweep R2" >>"$EV"
run wait --timeout 0
HC_FAKE_LS='{"terminals":[{"address":"tf6","name":"lane-f6","state":"live","status":"done","turn":1}]}' run wait --timeout 0
t "정리됨: 쉬는 레인은 정리됨 앞의 마지막 report(완료)로 판정(닫기 실패해도 거짓 멈춤 없음)" '[ $rc = 75 ] || { printf "%s\n" "$out"; false; }'
# 재감사 [높음] 1: 착지·판정 뒤 이어 받은 일(turn이 오름)에서 끝나거나 정리되면 알린다 — 쉼 판정과 같은 turn 규칙(report turn+1 이하만 조용)
nw=$(date -u +%Y-%m-%dT%H:%M:%SZ)
{ ev8 "$nw" here ta1 lane-ta1 판정 - 감사 3; ev8 "$nw" here ta2 lane-ta2 판정 - 감사 3
  ev8 "$nw" here tb1 lane-tb1 착지 abcd1234 끝 2; ev8 "$nw" here tb1 lane-tb1 정리됨 - "hc-sweep R2" 7
  ev8 "$nw" here tb2 lane-tb2 판정 - 감사 3; ev8 "$nw" here tb2 lane-tb2 정리됨 - "hc-sweep R2" 4; } >>"$EV"
HC_FAKE_LS='{"terminals":[]}' run wait --state 판정,착지 --timeout 0   # report 줄은 먼저 소비
HC_FAKE_LS='{"terminals":[{"address":"ta1","name":"lane-ta1","state":"finished","turn":6},{"address":"ta2","name":"lane-ta2","state":"finished","turn":4}]}' \
  run wait --state 멈춤 --timeout 0
t "turn: 판정 뒤 turn이 오른(이어 받은 일) 끝남은 알림" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}ta1${tab}lane-ta1${tab}멈춤${tab}-${tab}뜻밖의 종료(finished)" || { printf "%s\n" "$out"; cat "$T/err"; false; }'
t "turn: 착지 뒤 turn이 오른 정리됨은 알림" 'printf "%s\n" "$out" | grep -Fq "${tab}tb1${tab}lane-tb1${tab}멈춤${tab}-${tab}뜻밖의 종료(정리됨)"'
t "turn: 판정 그 턴(turn+1)의 끝남·정리됨은 조용" '! printf "%s\n" "$out" | grep -Eq "lane-t[ab]2${tab}멈춤"'
# 재감사 [낮음] 2: 쉼(done turn n)으로 이미 알린 레인을 hc-sweep이 같은 turn에 닫으면 다시 알리지 않음
HC_FAKE_LS='{"terminals":[{"address":"tc1","name":"lane-tc1","state":"live","status":"done","turn":5}]}' run wait --state 멈춤 --timeout 0
t "쉼 알림 먼저(준비)" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "lane-tc1${tab}멈춤${tab}-${tab}done turn 5"'
ev8 "$(date -u +%Y-%m-%dT%H:%M:%SZ)" here tc1 lane-tc1 정리됨 - "hc-sweep R2" 5 >>"$EV"
HC_FAKE_LS='{"terminals":[]}' run wait --state 멈춤 --timeout 0
t "정리됨: 같은 turn에 쉼으로 이미 알린 레인은 다시 안 알림" '[ $rc = 75 ] || { printf "%s\n" "$out"; false; }'
# wait-landed(2026-11-16 실측): 착지 뒤 같은 sha의 「완료」를 그 턴에 한 번 더 report하고 끝나면 착지 뒤 종료다.
# 다른 sha의 완료·중간이 뒤에 오거나, 착지와 완료 사이·그 뒤에 turn이 오르면(이어 받은 일), sha가 -면(판정 포함) 여전히 알린다
nw=$(date -u +%Y-%m-%dT%H:%M:%SZ)
{ ev8 "$nw" here td1 lane-td1 착지 6d295b6f 착지 2; ev8 "$nw" here td1 lane-td1 완료 6d295b6f 완료 2
  ev8 "$nw" here td2 lane-td2 착지 6d295b6f 착지 2; ev8 "$nw" here td2 lane-td2 완료 abcd1234 새일 2
  ev8 "$nw" here td3 lane-td3 착지 6d295b6f 착지 2; ev8 "$nw" here td3 lane-td3 완료 6d295b6f 완료 4
  ev8 "$nw" here td4 lane-td4 착지 6d295b6f 착지 2; ev8 "$nw" here td4 lane-td4 완료 6d295b6f 완료 2; ev8 "$nw" here td4 lane-td4 중간 - 반 2
  ev8 "$nw" here td5 lane-td5 판정 - 감사 2; ev8 "$nw" here td5 lane-td5 완료 - 끝 2
  ev8 "$nw" here td6 lane-td6 착지 - 착지 2; ev8 "$nw" here td6 lane-td6 완료 - 새일 2
  ev8 "$nw" here td7 lane-td7 착지 6d295b6f 착지 2; ev8 "$nw" here td7 lane-td7 완료 6d295b6f 완료 5
  ev8 "$nw" here td8 lane-td8 착지 6d295b6 착지 2; ev8 "$nw" here td8 lane-td8 완료 "6d295b6f00112233445566778899""aabbccddeeff" 완료 2
  ev8 "$nw" here td9 lane-td9 착지 6d295b6f 착지 2; ev8 "$nw" here td9 lane-td9 완료 6d295b6f 완료 3
  ev8 "$nw" here te1 lane-te1 착지 6d295b6f 착지 2; ev8 "$nw" here te1 lane-te1 완료 6d295b6f 완료 2; ev8 "$nw" here te1 lane-te1 정리됨 - "hc-sweep R2" 3
  ev8 "$nw" here te2 lane-te2 착지 6d295b6f 착지 2; ev8 "$nw" here te2 lane-te2 완료 abcd1234 새일 2; ev8 "$nw" here te2 lane-te2 정리됨 - "hc-sweep R2" 3; } >>"$EV"
HC_FAKE_LS='{"terminals":[]}' run wait --state 착지,판정,완료,중간 --timeout 0   # report 줄은 먼저 소비
HC_FAKE_LS='{"terminals":[{"address":"td1","name":"lane-td1","state":"finished","turn":3},{"address":"td2","name":"lane-td2","state":"finished","turn":3},{"address":"td3","name":"lane-td3","state":"finished","turn":7},{"address":"td4","name":"lane-td4","state":"finished","turn":3},{"address":"td5","name":"lane-td5","state":"finished","turn":3},{"address":"td6","name":"lane-td6","state":"finished","turn":3},{"address":"td7","name":"lane-td7","state":"finished","turn":6},{"address":"td8","name":"lane-td8","state":"finished","turn":3},{"address":"td9","name":"lane-td9","state":"finished","turn":4}]}' \
  run wait --state 멈춤 --timeout 0
t "착지 뒤 같은 sha 완료: 그 턴에 끝남·정리됨은 조용(실측 꼴, 짧은 쪽 길이 앞자리 비교)" '[ $rc = 0 ] && ! printf "%s\n" "$out" | grep -Eq "lane-t(d1|d8|e1)${tab}멈춤" || { printf "%s\n" "$out"; cat "$T/err"; false; }'
t "sha가 -면 같은 sha로 치지 않음: 판정·착지 - 뒤 완료 - 의 끝남은 알림(감사 wait-landed [높음] 1)" 'printf "%s\n" "$out" | grep -Fq "${tab}td5${tab}lane-td5${tab}멈춤${tab}-${tab}뜻밖의 종료(finished)" && printf "%s\n" "$out" | grep -Fq "${tab}td6${tab}lane-td6${tab}멈춤${tab}-${tab}뜻밖의 종료(finished)" || { printf "%s\n" "$out"; false; }'
t "착지와 같은 sha 완료 사이에 turn이 오르면(이어 받은 일) 끝남은 알림(감사 wait-landed [높음] 1)" 'printf "%s\n" "$out" | grep -Fq "${tab}td7${tab}lane-td7${tab}멈춤${tab}-${tab}뜻밖의 종료(finished)"'
t "착지 다음 턴(turn+1)의 같은 sha 완료도 새 일: 끝남은 알림(재감사 [낮음] 2)" 'printf "%s\n" "$out" | grep -Fq "${tab}td9${tab}lane-td9${tab}멈춤${tab}-${tab}뜻밖의 종료(finished)"'
t "착지 뒤 다른 sha 완료 뒤 끝남·정리됨은 알림" 'printf "%s\n" "$out" | grep -Fq "${tab}td2${tab}lane-td2${tab}멈춤${tab}-${tab}뜻밖의 종료(finished)" && printf "%s\n" "$out" | grep -Fq "${tab}te2${tab}lane-te2${tab}멈춤${tab}-${tab}뜻밖의 종료(정리됨)" || { printf "%s\n" "$out"; false; }'
t "착지 뒤 같은 sha 완료여도 turn이 오른 뒤 끝남은 알림" 'printf "%s\n" "$out" | grep -Fq "${tab}td3${tab}lane-td3${tab}멈춤${tab}-${tab}뜻밖의 종료(finished)"'
t "착지·완료 뒤 중간이 오면 끝남은 알림" 'printf "%s\n" "$out" | grep -Fq "${tab}td4${tab}lane-td4${tab}멈춤${tab}-${tab}뜻밖의 종료(finished)"'
# 지휘 요청(감사 3차 [높음] 1로 기본 꺼짐): wait는 기본으로 모든 레인 사건, --mine이면 레인의 hc parent가 이 지휘($HC_TERMINAL_ID —
# 원격 레인은 <기계>/<t…>로 본다)인 사건만. parent를 모르면(hc ls에 없음) 보여 준다(fail-open).
# 기본이 거르면 지휘가 바뀌거나(옛 parent는 죽은 터미널) wait를 다른 터미널에서 부를 때 사건이 다 가려진다
nw=$(date -u +%Y-%m-%dT%H:%M:%SZ)
{ ev8 "$nw" here tw1 lane-own 완료 abcd1234 내것 1; ev8 "$nw" m1 m1/tw2 lane-own2 완료 abcd1234 내것원격 1
  ev8 "$nw" here tw3 lane-other 완료 abcd1234 남의것 1; ev8 "$nw" here tw4 lane-gone 완료 abcd1234 모름 1; } >>"$EV"
lsP='{"terminals":[{"address":"tw1","name":"lane-own","parent":"tW","state":"live","status":"working","turn":1},{"address":"m1/tw2","name":"lane-own2","parent":"mac/tW","state":"live","status":"working","turn":1},{"address":"tw3","name":"lane-other","parent":"mac/tX","state":"live","status":"working","turn":1}]}'
HC_TERMINAL_ID=tNEW HC_FAKE_LS=$lsP run wait --state 완료 --timeout 0
t "parent 기본: 거르지 않음 — 다른(옛) 지휘가 parent인 레인 사건도 보임" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "lane-own${tab}완료" && printf "%s\n" "$out" | grep -Fq "lane-other${tab}완료" || { printf "%s\n" "$out"; cat "$T/err"; false; }'
HC_TERMINAL_ID=tW HC_FAKE_LS=$lsP run wait --mine --state 완료 --timeout 0
t "parent --mine: 내 레인(로컬·원격 꼴)과 parent 모르는 레인만, 남의 지휘 레인은 거름" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "lane-own${tab}완료" && printf "%s\n" "$out" | grep -Fq "lane-own2${tab}완료" && printf "%s\n" "$out" | grep -Fq "lane-gone${tab}완료" && ! printf "%s\n" "$out" | grep -Fq lane-other || { printf "%s\n" "$out"; cat "$T/err"; false; }'
run wait --everyone --timeout 0
t "--everyone은 없음(기본이 전부)" '[ $rc = 1 ] && grep -Fq -- --everyone "$T/err"'
# 감사 [낮음] 5: hc ls가 바퀴마다 실패하면 3바퀴째에 멈춤꼴 사건 한 줄로 끝낸다(배경 wait의 stderr는 끝나기 전엔 안 보인다)
HC_LANE_WAIT_TICK=0 HC_FAKE_LS_FAIL=1 run wait --state 멈춤 --timeout 1
t "hc ls 실패 3바퀴: 사건 한 줄로 알리고 종료 0" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "멈춤${tab}-${tab}hc ls 실패 3바퀴" || { printf "%s\n" "$out"; cat "$T/err"; false; }'

# quiet-alerts 2: lane.notify no면 hc-lane이 띄우는 레인·감사(--read)에 hc run --no-notify
rm -f "$HCLOG"
run --name lane-n1 --repo "$R" -- true
t "notify 기본(yes): --no-notify 없음" '[ $rc = 0 ] && has run && ! has --no-notify'
conf 'lane.notify no'
rm -f "$HCLOG"
run --name lane-n2 --repo "$R" -- true
t "notify no: 레인 hc run에 --no-notify" '[ $rc = 0 ] && has run && has --no-notify || { cat "$T/err"; false; }'
rm -f "$HCLOG"
run --read --name lane-n3 --repo "$R" -- true
t "notify no: --read(감사)에도 --no-notify" '[ $rc = 0 ] && has --no-notify'
run --dry-run --name lane-n4 --repo "$R" -- true
t "notify no: --dry-run 줄에도 --no-notify" '[ $rc = 0 ] && printf "%s\n" "$out" | grep "hc run --bg" | grep -Fq -- --no-notify'
conf 'lane.notify maybe'
run --name lane-n5 --repo "$R" -- true
t "notify: yes·no가 아니면 띄우기 전에 오류" '[ $rc != 0 ] && grep -Fq lane.notify "$T/err" && [ ! -e "$R.wt/lane-n5" ]'
noconf

# 감사 반영 3·6: 커서는 지휘(HC_TERMINAL_ID)마다 따로, 새 커서는 2시간 전부터
export HC_FAKE_LS='{"terminals":[]}'
ev 2000-01-01T00:00:00Z here t1 old-ev 완료 - 옛날 >>"$EV"
HC_LANE=lane-k run report 완료 abcdef1 둘이 받을 것
HC_TERMINAL_ID=tA run wait --timeout 0; oA=$out rA=$rc
HC_TERMINAL_ID=tB run wait --timeout 0; oB=$out rB=$rc
t "커서: 지휘 둘이 같은 사건을 각자 받음" '[ $rA = 0 ] && [ $rB = 0 ] && printf "%s" "$oA" | grep -Fq "lane-k${tab}완료" && printf "%s" "$oB" | grep -Fq "lane-k${tab}완료" && [ -f "$T/.local/state/hc-lane/seen.tA" ] && [ -f "$T/.local/state/hc-lane/seen.tB" ]'
t "커서: 새 커서는 2시간 전 사건부터(옛 사건을 쏟지 않음)" '! printf "%s" "$oA" | grep -Fq old-ev'
HC_TERMINAL_ID=tA run wait --timeout 0
t "커서: 같은 지휘에게는 다시 안 알림" '[ $rc = 75 ]'
unset HC_FAKE_MACH_HOME HC_FAKE_MACHINES HC_FAKE_LS

# print-steps·띄우기: report 줄, HC_LANE·라벨
run --print-steps --name lane-p --repo "$R"
t "print-steps: 쓰기 레인에 hc-lane report 줄(중간·완료·착지·막힘)" 'printf "%s" "$out" | grep -F "hc-lane report" | grep -Fq 완료 && printf "%s" "$out" | grep -F "hc-lane report" | grep -Fq 착지'
run --print-steps --read --name lane-pr --repo "$R"
t "print-steps --read: 판정 줄은 hc-lane report 판정 -" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fq "hc-lane report 판정 - " && ! printf "%s" "$out" | grep -Fq "hc-lane land" || { cat "$T/err"; false; }'
rm -f "$HCLOG"
run --name lane-e --repo "$R" -- true
t "띄우기: HC_LANE=<이름>을 넘기고 라벨 hc-lane=1" '[ $rc = 0 ] && has HC_LANE=lane-e && pair --label hc-lane=1'
rm -f "$HCLOG"
run --read --name lane-er --repo "$R" -- true
t "띄우기 --read: HC_LANE·라벨, 판정 줄 안내" '[ $rc = 0 ] && has HC_LANE=lane-er && pair --label hc-lane=1 && printf "%s" "$out" | grep -Fq "hc-lane report 판정 - "'
# 21. --brief(지시문 사본·{brief}), 신뢰 질문 자동 넘김, 시작 확인(종료 5·6·7), fetch(로그인 셸 꼴)
mk t18
BD="$W/지시 폴더"; mkdir -p "$BD"; printf '# 할 일\n본문 줄\n' >"$BD/BRIEF.md"
run --brief "$BD/BRIEF.md" --name lane-b --repo "$R" -- echo "{brief} 를 읽어라"
t "--brief: 종료 0" '[ $rc = 0 ] || { cat "$T/err"; false; }'
t "--brief: 사본은 지시문 옆 BRIEF-run.md, 본문+착지 절차(작업 폴더)" 'grep -Fxq "본문 줄" "$BD/BRIEF-run.md" && grep -Fq "### 착지·정리" "$BD/BRIEF-run.md" && grep -Fq "$R.wt/lane-b" "$BD/BRIEF-run.md"'
t "--brief: 실행 명령의 {brief} 자리에 사본 경로" 'has "$BD/BRIEF-run.md 를 읽어라"'
t "--brief: 레인 환경 HC_LANE·라벨도 그대로" 'has HC_LANE=lane-b && pair --label hc-lane=1'
t "--brief: 지시문 원본은 그대로" '[ "$(cat "$BD/BRIEF.md")" = "$(printf "# 할 일\n본문 줄")" ]'
conf 'lane.brief_dir ~/briefs'
run --on box --brief "$BD/BRIEF.md" --name lane-bo --repo "$R" -- echo "{brief}"
t "--brief --on: 설정 lane.brief_dir(그 기계 ~)에 <레인>-run.md, hc exec -i --on box로" '[ $rc = 0 ] && grep -Fq "본문 줄" "$T/briefs/lane-bo-run.md" && grep -Fx -A1 -- --on "$HCLOG.exec" | grep -Fxq box && grep -Fxq -- -i "$HCLOG.exec"'
t "--brief --on: 실행 명령에 그 기계 경로" 'has "$T/briefs/lane-bo-run.md"'
conf 'lane.run printf "%s\n" "읽고 진행: {brief}" x'
run --brief "$BD/BRIEF.md" --name lane-br --repo "$R"
t "--brief: -- 뒤 명령이 없으면 설정 lane.run 틀" '[ $rc = 0 ] && has "읽고 진행: $BD/BRIEF-run.md" && has "%s\n" && has x || { cat "$T/err"; false; }'
noconf
run --brief "$BD/BRIEF.md" --name lane-bn --repo "$R"
t "--brief: 명령도 lane.run도 없으면 오류, 아무것도 안 만듦" '[ $rc != 0 ] && grep -Fq lane.run "$T/err" && [ ! -e "$R.wt/lane-bn" ]'
run --brief "$BD/BRIEF.md" --name lane-bx --repo "$R" -- echo hi
t "--brief: 실행 명령에 {brief} 자리가 없으면 오류, 아무것도 안 만듦" '[ $rc != 0 ] && grep -Fq "{brief}" "$T/err" && [ ! -e "$R.wt/lane-bx" ]'
run --name lane-by --repo "$R" -- echo "{brief}"
t "{brief}가 있는데 --brief가 없으면 오류" '[ $rc != 0 ] && grep -Fq -- --brief "$T/err" && [ ! -e "$R.wt/lane-by" ]'
run --brief "$BD/없음.md" --name lane-bz --repo "$R" -- echo "{brief}"
t "--brief: 지시문 파일이 없으면 오류" '[ $rc != 0 ] && [ ! -e "$R.wt/lane-bz" ]'
printf '# 감사\n' >"$BD/AUDIT.md"
run --read --brief "$BD/AUDIT.md" --name lane-ba --repo "$R" -- echo "{brief}"
t "--read --brief: 사본에 착지 절차 대신 판정 알림, 경로가 명령에" '[ $rc = 0 ] && grep -Fxq "# 감사" "$BD/AUDIT-run.md" && ! grep -Fq "착지·정리" "$BD/AUDIT-run.md" && grep -Fq "판정 알림" "$BD/AUDIT-run.md" && has "$BD/AUDIT-run.md"'
printf '# 마른\n' >"$BD/DRY.md"
run --dry-run --brief "$BD/DRY.md" --name lane-bd --repo "$R" -- echo "{brief}"
t "--brief --dry-run: 사본을 쓰지 않고 할 일만" '[ $rc = 0 ] && [ ! -e "$BD/DRY-run.md" ] && printf "%s\n" "$out" | grep -Fq "지시문 사본" && [ ! -e "$R.wt/lane-bd" ]'

# 신뢰 질문(2026-11-16 실제 Claude 2.1 화면 글자)
SC="$T/screen"
trust_screen() { printf '%s\n' ' Accessing workspace:' ' /x' ' Quick safety check: Is this a project you created or one you trust? (Like your own code)' \
  ' Security guide' " $1 No, exit" " $2 Yes, I trust this folder" ' Enter to confirm · Esc to cancel' >"$SC"; }
trust_screen '❯' ' '
rm -f "$HCLOG.keys"; HC_FAKE_SCREEN=$SC HC_FAKE_NOSTATUS=untilkeys run --name lane-tr --repo "$R" -- claude
t "신뢰 질문: 아래·엔터로 넘기고 status가 잡혀 종료 0" '[ $rc = 0 ] && [ "$(cat "$HCLOG.keys")" = "t9zz down enter" ] && printf "%s\n" "$out" | grep -Fq "신뢰 질문 넘김" || { cat "$T/err"; false; }'
trust_screen ' ' '❯'
rm -f "$HCLOG.keys"; HC_FAKE_SCREEN=$SC HC_FAKE_NOSTATUS=untilkeys run --name lane-tr2 --repo "$R" -- claude
t "신뢰 질문: 이미 신뢰에 커서면 엔터만" '[ $rc = 0 ] && [ "$(cat "$HCLOG.keys")" = "t9zz enter" ]'
trust_screen ' ' ' '
rm -f "$HCLOG.keys"; HC_FAKE_SCREEN=$SC HC_FAKE_NOSTATUS=untilkeys run --start-wait 1 --name lane-tr3 --repo "$R" -- claude
t "신뢰 질문: 어느 줄에도 커서(❯)가 없으면 키를 안 보내고 「모름」(감사 [낮음] 3)" '[ $rc = 5 ] && [ ! -e "$HCLOG.keys" ] && grep -Fq "모름 — 시작 화면 확인" "$T/err"'
HC_FAKE_NOSTATUS=1 run --start-wait 0 --name lane-sw0 --repo "$R" -- ./script.sh
t "--start-wait 0: 이 호출만 시작 확인 끔(설정보다 앞섬, status 없는 명령용)" '[ $rc = 0 ] && printf "%s\n" "$out" | tail -n 1 | grep -Fq "\"start\":\"off\""'
run --start-wait 1분 --name lane-swx --repo "$R" -- claude
t "--start-wait가 숫자가 아니면 띄우기 전에 오류(알 수 없는 인자가 아니라 값 검사)" '[ $rc != 0 ] && grep -Fq -- "--start-wait는 0 이상 정수" "$T/err" && [ ! -e "$R.wt/lane-swx" ]'
conf 'lane.start_wait 1'
printf '%s\n' ' Accessing workspace:' ' Do you trust the files in this folder?' ' ❯ 1. Yes, proceed' '   2. No' >"$SC"
rm -f "$HCLOG.keys"; HC_FAKE_SCREEN=$SC HC_FAKE_NOSTATUS=1 run --name lane-tu --repo "$R" -- claude
t "신뢰 질문 글자가 다르면 안 건드리고 「모름 — 시작 화면 확인」, 종료 5" '[ $rc = 5 ] && [ ! -e "$HCLOG.keys" ] && grep -Fq "모름 — 시작 화면 확인" "$T/err"'
t "시작 실패에도 JSON 끝 줄에 주소와 start" 'printf "%s\n" "$out" | tail -n 1 | grep -Fq "\"lane\":\"t9zz\"" && printf "%s\n" "$out" | tail -n 1 | grep -Fq "\"start\":\"no_status\""'
printf 'Welcome\n> \n' >"$SC"
rm -f "$HCLOG.keys"; HC_FAKE_SCREEN=$SC HC_FAKE_NOSTATUS=1 run --name lane-ns --repo "$R" -- claude
t "status가 안 잡히면 종료 5, 키는 안 보냄" '[ $rc = 5 ] && [ ! -e "$HCLOG.keys" ] && grep -Fq "status" "$T/err"'
printf 'Claude Code\n\n  Login expired · Please run /login\n\n' >"$SC"
HC_FAKE_SCREEN=$SC HC_FAKE_NOSTATUS=1 run --on box --name lane-lg --repo "$R" -- claude
t "로그인 만료: 종료 6, 그 줄을 알림" '[ $rc = 6 ] && grep -Fq "로그인" "$T/err" && grep -Fq "Login expired" "$T/err" && printf "%s\n" "$out" | tail -n 1 | grep -Fq "\"start\":\"login\""'
printf 'security: SecKeychainCopyDefault: User interaction is not allowed.\nNot logged in\n' >"$SC"
HC_FAKE_SCREEN=$SC HC_FAKE_NOSTATUS=1 run --name lane-kc --repo "$R" -- claude
t "키체인: 종료 7" '[ $rc = 7 ] && grep -Fq "키체인" "$T/err"'
HC_FAKE_NOSTATUS=finished run --name lane-fn --repo "$R" -- claude
t "터미널이 끝나면 기다리지 않고 종료 5" '[ $rc = 5 ] && grep -Fq "끝났다" "$T/err"'
conf 'lane.start_wait 1' 'lane.auto_trust off'
trust_screen '❯' ' '
rm -f "$HCLOG.keys"; HC_FAKE_SCREEN=$SC HC_FAKE_NOSTATUS=1 run --name lane-to --repo "$R" -- claude
t "lane.auto_trust off: 신뢰 질문을 안 건드리고 종료 5" '[ $rc = 5 ] && [ ! -e "$HCLOG.keys" ] && grep -Fq "신뢰 질문" "$T/err"'
# 외부 가져오기 질문(2026-11-17 Claude 2.1.326 화면 글자: 경로 줄은 두 칸 들여쓰기, 기본 커서는 No)
imp_screen() {   # imp_screen <No 줄 앞> <Yes 줄 앞> <경로 줄…>
  c1=$1 c2=$2; shift 2
  { printf '%s\n' ' Allow external CLAUDE.md file imports?' '' \
      " This project's CLAUDE.md or .claude/rules imports files outside the current working directory. Never allow this for third-party repositories." \
      '' ' External imports:'
    for l; do printf '   %s\n' "$l"; done
    printf '%s\n' '' ' Important: Only use Claude Code with files you trust. Accessing untrusted files may pose security risks' '' \
      " $c1 No, disable external imports" " $c2 Yes, allow external imports"; } >"$SC"
}
conf 'lane.start_wait 1'
imp_screen '❯' ' ' "$R.wt/lane-im/AGENTS.md" "$R.wt/lane-im/docs/x.md"
rm -f "$HCLOG.keys"; HC_FAKE_SCREEN=$SC HC_FAKE_NOSTATUS=untilkeys run --name lane-im --repo "$R" -- claude
t "외부 가져오기: 경로가 모두 작업 폴더 저장소 안이면 아래·엔터로 허용, 종료 0" '[ $rc = 0 ] && [ "$(cat "$HCLOG.keys")" = "t9zz down enter" ] && printf "%s\n" "$out" | grep -Fq "외부 가져오기 허용" || { cat "$T/err"; false; }'
imp_screen ' ' '❯' "~/${R#"$T"/}.wt/lane-im2/AGENTS.md"
rm -f "$HCLOG.keys"; HC_FAKE_SCREEN=$SC HC_FAKE_NOSTATUS=untilkeys run --name lane-im2 --repo "$R" -- claude
t "외부 가져오기: ~ 줄임 경로도 펼쳐 보고, 이미 Yes에 커서면 엔터만" '[ $rc = 0 ] && [ "$(cat "$HCLOG.keys")" = "t9zz enter" ] || { cat "$T/err"; false; }'
imp_screen '❯' ' ' "$R.wt/lane-im3/AGENTS.md" "$R/AGENTS.md"
rm -f "$HCLOG.keys"; HC_FAKE_SCREEN=$SC HC_FAKE_NOSTATUS=1 run --name lane-im3 --repo "$R" -- claude
t "외부 가져오기: 밖 경로가 하나라도 있으면 안 고르고 「모름 — 외부 가져오기 질문(<경로>)」, 종료 5" '[ $rc = 5 ] && [ ! -e "$HCLOG.keys" ] && grep -Fq "모름 — 외부 가져오기 질문($R/AGENTS.md)" "$T/err" && ! grep -F "외부 가져오기 질문(" "$T/err" | grep -Fq "lane-im3/AGENTS.md"'
imp_screen '❯' ' ' "$R.wt/lane-im4/AGENTS.md" '… +3 imports not shown.' 'Yes covers those too, plus any this project adds later.'
rm -f "$HCLOG.keys"; HC_FAKE_SCREEN=$SC HC_FAKE_NOSTATUS=1 run --name lane-im4 --repo "$R" -- claude
t "외부 가져오기: 다 보이지 않는 목록(… +N not shown)은 안 고름, 종료 5" '[ $rc = 5 ] && [ ! -e "$HCLOG.keys" ] && grep -Fq "모름 — 외부 가져오기 질문(" "$T/err"'
imp_screen '❯' ' ' "$R.wt/lane-im5/../../../etc/x.md"
rm -f "$HCLOG.keys"; HC_FAKE_SCREEN=$SC HC_FAKE_NOSTATUS=1 run --name lane-im5 --repo "$R" -- claude
t "외부 가져오기: .. 로 빠져나가는 경로는 밖, 안 고름" '[ $rc = 5 ] && [ ! -e "$HCLOG.keys" ]'
conf 'lane.start_wait 1' 'lane.auto_trust off'
imp_screen '❯' ' ' "$R.wt/lane-im6/AGENTS.md"
rm -f "$HCLOG.keys"; HC_FAKE_SCREEN=$SC HC_FAKE_NOSTATUS=1 run --name lane-im6 --repo "$R" -- claude
t "외부 가져오기: lane.auto_trust off면 안 고름, 종료 5" '[ $rc = 5 ] && [ ! -e "$HCLOG.keys" ] && grep -Fq "외부 가져오기 질문(lane.auto_trust off)" "$T/err"'
conf 'lane.start_wait 1'
# --read: 저장소 안 하위 폴더(.qa/…)·저장소가 아닌 폴더도 그 폴더에서, 시작 확인도 받는다
mkdir -p "$R/.qa/감사 1"
imp_screen '❯' ' ' "$R/AGENTS.md"
rm -f "$HCLOG" "$HCLOG.keys"; HC_FAKE_SCREEN=$SC HC_FAKE_NOSTATUS=untilkeys run --read --name lane-iq --repo "$R/.qa/감사 1" -- claude
t "--read 하위 폴더: 그 폴더에서 띄우고 저장소 루트 가져오기는 허용" '[ $rc = 0 ] && pair --cwd "$R/.qa/감사 1" && has "LANE_REPO=$R" && [ "$(cat "$HCLOG.keys")" = "t9zz down enter" ] || { cat "$T/err"; false; }'
P="$T/저장소 아닌 폴더"; mkdir -p "$P"
rm -f "$HCLOG" "$HCLOG.keys"; run --read --name lane-ip --repo "$P" -- claude
t "--read 저장소 아닌 폴더: 그 폴더에서 띄움, LANE_BASE 빈 값, 종료 0" '[ $rc = 0 ] && pair --cwd "$P" && has "LANE_REPO=$P" && has LANE_BASE= || { cat "$T/err"; false; }'
imp_screen '❯' ' ' "$P/AGENTS.md"
rm -f "$HCLOG.keys"; HC_FAKE_SCREEN=$SC HC_FAKE_NOSTATUS=1 run --read --name lane-ip2 --repo "$P" -- claude
t "--read 저장소 아닌 폴더: 가져오기 질문은 안 고름, 종료 5" '[ $rc = 5 ] && [ ! -e "$HCLOG.keys" ] && grep -Fq "저장소 아님" "$T/err"'
run --read --name lane-ix --repo "$T/없는 폴더" -- claude
t "--read: 없는 폴더는 띄우기 전에 오류" '[ $rc = 1 ] && grep -Fq "폴더가 없다" "$T/err"'
conf 'lane.start_wait 0'
HC_FAKE_NOSTATUS=1 run --name lane-s0 --repo "$R" -- claude
t "lane.start_wait 0: 시작 확인 끔, 종료 0" '[ $rc = 0 ] && printf "%s\n" "$out" | tail -n 1 | grep -Fq "\"start\":\"off\""'
conf 'lane.start_wait 다섯'
run --name lane-sx --repo "$R" -- claude
t "lane.start_wait가 숫자가 아니면 띄우기 전에 오류" '[ $rc != 0 ] && grep -Fq lane.start_wait "$T/err" && [ ! -e "$R.wt/lane-sx" ]'
noconf

# fetch: 그 기계 로그인 셸(zsh·~/.zshrc) 꼴로 git fetch
mk t18f
old=$(git -C "$R" rev-parse origin/main)
run fetch --on box --repo "$R" main
t "fetch --on: 종료 0, origin/main이 새로 받혀짐" '[ $rc = 0 ] && [ "$(git -C "$R" rev-parse origin/main)" = "$(git -C "$D/origin.git" rev-parse main)" ] && [ "$old" != "$(git -C "$R" rev-parse origin/main)" ] || { cat "$T/err"; false; }'
t "fetch --on: hc exec --on box, ~/.zshrc를 읽는 셸" 'grep -Fx -A1 -- --on "$HCLOG.exec" | grep -Fxq box && grep -Fq zshrc "$HCLOG.exec" && grep -Fxq fetch "$HCLOG.exec"'
run fetch --repo "$R" "refs/heads/main:refs/remotes/origin/fetched"
t "fetch: --on 없으면 그 자리에서, 참조 그대로" '[ $rc = 0 ] && git -C "$R" rev-parse -q --verify refs/remotes/origin/fetched >/dev/null'
run fetch --on box --repo "$W/없는 저장소"
t "fetch: 실패면 종료 1" '[ $rc = 1 ]'
run fetch --on box
t "fetch: --repo 없으면 사용법" '[ $rc = 2 ]'
run fetch --repo "$R" main "--upload-pack=touch $T/pwned; git-upload-pack"
t "fetch: 둘째 참조부터도 - 로 시작하면 거절, 아무것도 안 돎(감사 [중간] 2)" '[ $rc != 0 ] && [ ! -e "$T/pwned" ] && grep -Fq upload-pack "$T/err"'

# watch(lane-push, 2026-11-16 실측: 지휘가 wait를 사건마다 다시 걸다 한 번 빠뜨리자 완료를 34·14분 늦게 봤다):
# 늘 도는 감시가 wait와 같은 판정으로 새 사건을 지휘 터미널에 hc send로 밀어 준다. 커서는 wait와 같은 지휘별 커서
export XDG_STATE_HOME=$T/wst HC_FAKE_LS='{"terminals":[]}' HC_LANE_WAIT_TICK=1
unset HC_FAKE_MACHINES
WD=$T/wst/hc-lane
mkdir -p "$WD"; printf 't7cc\t4242\tt7cc\t0\n' >"$WD/coord.t7cc"   # 지휘마다 지휘 정보(주소·pid·이름·마지막 레인 띄운 시각) — 보낼 때마다 hc get과 맞춰 본다
rm -f "$HCLOG.send" "$HCLOG.sendfail"
sends() { grep -c . "$HCLOG.send" 2>/dev/null || true; }
upto() { i_=0; while [ $i_ -lt 80 ]; do eval "$1" && return 0; sleep 0.1; i_=$((i_ + 1)); done; return 1; }   # 8초까지 기다림
alive() { kill -0 "$1" 2>/dev/null; }
stopw() { kill -TERM "$1" 2>/dev/null; wait "$1" 2>/dev/null; }
HC_FAKE_LIVE=t7cc "$LANE" watch --to t7cc >"$T/wo" 2>"$T/we" &
w1=$!
upto '[ -d "$WD/watch.t7cc" ]'
HC_LANE=lane-w "$LANE" report 완료 abcd1234 끝냄 >/dev/null
upto '[ "$(sends)" = 1 ]'
t "watch: 새 사건은 지휘 터미널에 hc send 한 번(한 줄)" '[ "$(sends)" = 1 ] && [ "$(cut -f1 "$HCLOG.send")" = t7cc ] && grep -Fq "lane-w 완료 abcd1234 끝냄" "$HCLOG.send" || { cat "$HCLOG.send" "$T/we"; false; }'
sleep 2.5
t "watch: 같은 사건은 다시 안 보냄" '[ "$(sends)" = 1 ]'
run watch --to t7cc
t "watch: 같은 지휘 주소로 두 번째는 종료 75" '[ $rc = 75 ] && alive $w1 || { cat "$T/err"; false; }'
HC_TERMINAL_ID=t7cc run wait --timeout 0
t "watch: wait와 커서를 함께 씀(watch가 보낸 사건을 wait가 다시 안 알림)" '[ $rc = 75 ]'
touch "$HCLOG.sendfail"
HC_LANE=lane-w "$LANE" report 막힘 - 막혔음 >/dev/null
sleep 2.5
t "watch: hc send가 실패하면 커서에 안 적음" '[ "$(sends)" = 1 ] && ! grep -Fq 막혔음 "$WD/seen.t7cc"'
rm -f "$HCLOG.sendfail"
upto '[ "$(sends)" = 2 ]'
t "watch: 다음 바퀴에 다시 보냄(한 번)" '[ "$(sends)" = 2 ] && grep -Fq "lane-w 막힘 막혔음" "$HCLOG.send" || cat "$HCLOG.send"'
stopw $w1
# 남은 프로세스는 이 시험의 출력 파일 $T/wo를 쥔 것으로 본다(pgrep -f "watch --to t7cc"는 같은 시험을 동시에 도는 다른 레인의 watch도 잡아 흔들렸다 — 2026-11-18 land-log 착지)
# 끈 watch의 자식(배경 sleep·hc 호출)도 $T/wo를 물려받아 TERM 뒤 끝나기까지 잠깐 걸린다 — 부하가 크면 바로 본 lsof에 잡혀 흔들렸다
# (2026-11-18 hcfill-early 착지). 8초까지 기다리고, 그래도 남으면 무엇이 쥐었는지 찍는다. $T는 이 시험만의 폴더라 남의 watch는 안 셈
wgone() { ! alive $w1 && [ ! -e "$WD/watch.t7cc" ] && [ -z "$(lsof -t "$T/wo" 2>/dev/null)" ]; }
upto wgone
t "watch: 끄면 잠금을 지우고 프로세스를 안 남김" 'wgone || { ls -d "$WD/watch.t7cc" 2>&1; lsof "$T/wo" 2>&1; false; }'
HC_LANE=lane-w "$LANE" report 중간 - 반쯤 >/dev/null
HC_TERMINAL_ID=t7cc run wait --timeout 0
t "watch 꺼진 사이 사건은 wait가 받음" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq 반쯤'
mkdir -p "$WD/watch.t7cc" && echo 1 >"$WD/watch.t7cc/pid"   # 남은 잠금(pid가 hc-lane이 아님)
HC_FAKE_LIVE=t7cc "$LANE" watch --to t7cc >"$T/wo" 2>"$T/we" &
w2=$!
HC_LANE=lane-w "$LANE" report 착지 abcd1234 끝 >/dev/null
upto '[ "$(sends)" = 3 ]'
t "watch: 남은 잠금은 넘겨받음, wait가 받은 사건은 안 보냄" 'alive $w2 && [ "$(sends)" = 3 ] && tail -n 1 "$HCLOG.send" | grep -Fq "lane-w 착지 abcd1234" && ! grep -Fq 반쯤 "$HCLOG.send" || { cat "$HCLOG.send" "$T/we"; false; }'
stopw $w2
# 지휘가 사라지면(hc get 실패 3번) 스스로 끝남
"$LANE" watch --to t7cc >"$T/wo" 2>"$T/we" &
w3=$!
upto '! alive $w3'
wait $w3; wrc=$?
t "watch: 지휘 터미널이 사라지면 스스로 종료 0, 잠금 지움" '[ $wrc = 0 ] && grep -Fq 사라짐 "$T/we" && [ ! -e "$WD/watch.t7cc" ] || { cat "$T/we"; false; }'
# 원격 기계 연결 실패: 「연결 안 됨」 한 번, 다음 바퀴에 다시 보고 붙으면 그 사건
export HC_FAKE_MACHINES='{"machines":[{"name":"m9","state":"connected"}]}' HC_FAKE_MACH_HOME=$MH
rm -rf "$MH/m9"
HC_FAKE_LIVE=t7cc "$LANE" watch --all --to t7cc >"$T/wo" 2>"$T/we" &
w4=$!
upto 'grep -Fq "연결 안 됨 m9" "$HCLOG.send" 2>/dev/null'
sleep 2.5
t "watch: 원격 연결 실패는 「연결 안 됨」 한 번만" '[ "$(grep -c "연결 안 됨 m9" "$HCLOG.send")" = 1 ] || { cat "$HCLOG.send" "$T/we"; false; }'
mkdir -p "$MH/m9/.local/state/hc-lane"
ev "$(date -u +%Y-%m-%dT%H:%M:%SZ)" h9 t3 lane-r 완료 eeee9999 원격 >"$MH/m9/.local/state/hc-lane/events.tsv"
upto 'grep -Fq "lane-r 완료 eeee9999" "$HCLOG.send" 2>/dev/null'
t "watch: 다음 바퀴에 다시 붙으면 그 기계 사건을 보냄" 'grep -Fq "lane-r 완료 eeee9999 원격 (m9/t3)" "$HCLOG.send" || { cat "$HCLOG.send" "$T/we"; false; }'
stopw $w4
unset HC_FAKE_MACHINES HC_FAKE_MACH_HOME
# 지휘가 손으로 걸 일 없게: 레인·감사를 띄우면 그 지휘($HC_TERMINAL_ID)의 watch가 없을 때 저절로 하나(사용자 「코드적으로 처리」)
mk t21
rm -f "$WD/coord.t7cc"
HC_FAKE_LIVE=t7cc HC_TERMINAL_ID=t7cc run --name lane-a1 --repo "$R" -- claude
t "지휘 터미널에서 레인을 띄우면 지휘 정보(주소·pid·이름·띄운 시각)를 coord.<주소>에 남기고, 레인 기록에 그 지휘" '[ "$(cut -f1-3 "$WD/coord.t7cc")" = "t7cc${tab}4242${tab}t7cc" ] && [ "$(cut -f4 "$WD/coord.t7cc")" -gt 0 ] && grep -Fxq "t9zz${tab}lane-a1${tab}t7cc" "$WD/lanes.tsv" || { cat "$WD/coord.t7cc" "$WD/lanes.tsv"; false; }'
t "레인을 띄우면 지휘의 watch를 hc run --bg로 띄움(--all --to 지휘, 라벨 hc-lane-watch=1, --no-notify)" '[ $rc = 0 ] && pair --name lane-watch-t7cc && has watch && has --all && pair --to t7cc && pair --label hc-lane-watch=1 && has --no-notify && printf "%s\n" "$out" | grep -Fq "감시 lane-watch-t7cc 띄움" || { cat "$HCLOG" "$T/err"; false; }'
t "watch 터미널은 레인 라벨(hc-lane=1)이 아님" '[ "$(grep -c -Fx -- --label=hc-lane=1 "$HCLOG")" = 0 ] && [ "$(grep -A1 -Fx -- --label "$HCLOG" | grep -cFx hc-lane=1)" = 1 ]'
t "레인 JSON 끝 줄은 그대로 레인 주소" 'printf "%s\n" "$out" | tail -n 1 | grep -Fq "\"lane\":\"t9zz\""'
HC_FAKE_LIVE=t7cc "$LANE" watch --to t7cc >"$T/wo" 2>"$T/we" &
w5=$!
upto '[ -s "$WD/watch.t7cc/pid" ]'
rm -f "$HCLOG"
HC_TERMINAL_ID=t7cc run --read --name lane-a2 --repo "$R" -- claude
t "둘째 레인(--read 포함)은 watch가 돌고 있으면 안 띄움" '[ $rc = 0 ] && ! has lane-watch-t7cc && [ "$(grep -cFx run "$HCLOG")" = 1 ] || { cat "$HCLOG"; false; }'
stopw $w5
rm -f "$HCLOG"
run --name lane-a3 --repo "$R" -- claude
t "지휘가 없으면(\$HC_TERMINAL_ID 없음 — hc-fill 등) watch를 안 띄움" '[ $rc = 0 ] && ! has watch && [ "$(grep -cFx run "$HCLOG")" = 1 ]'
rm -f "$HCLOG"
HC_FAKE_LIVE=t7cc HC_TERMINAL_ID=t7cc run --dry-run --name lane-a4 --repo "$R" -- claude
t "--dry-run은 watch도 띄우지 않고 할 일만" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "[dry-run] hc run --bg --no-notify --rm --name lane-watch-t7cc" && [ ! -e "$HCLOG" ]'
# 레인이 다 끝나고 lane.watch_idle분(시험은 초) 동안 사건도 없으면 watch가 스스로 끝남
printf '%s\n' '{"terminals":[{"address":"t60","name":"lane-z","parent":"mac/t7cc","state":"live","status":"working","turn":1}]}' >"$HCLOG.lsfile"
HC_LANE_WATCH_IDLE=2 HC_FAKE_LIVE=t7cc "$LANE" watch --to t7cc >"$T/wo" 2>"$T/we" &
w6=$!
sleep 3.5
t "watch: 이 지휘의 살아 있는 레인이 있으면 쉬어도 안 끝남" 'alive $w6 || { cat "$T/we"; false; }'
printf '%s\n' '{"terminals":[{"address":"t60","name":"lane-z","parent":"mac/t7cc","state":"finished","turn":1},{"address":"t61","name":"lane-o","parent":"mac/t7","state":"live","status":"working","turn":1}]}' >"$HCLOG.lsfile"
upto '! alive $w6'
wait $w6; wrc=$?
t "watch: 이 지휘의 레인이 다 끝나고 그 시간이 지나면 종료 0(남의 지휘 레인은 안 셈), 잠금 지움" '[ $wrc = 0 ] && grep -Fq "끝낸다" "$T/we" && [ ! -e "$WD/watch.t7cc" ] || { cat "$T/we"; false; }'
rm -f "$HCLOG.lsfile"
# 감사 [중간] 1: hc-fill(launchd, $HC_TERMINAL_ID 없음)이 띄운 레인도 지휘(HC_LANE_COORD — hc-fill 설정 coord)의 watch를 띄움
rm -f "$HCLOG"
HC_FAKE_LIVE=t7cc HC_LANE_COORD=t7cc run --name lane-a5 --repo "$R" -- claude
t "HC_LANE_COORD(터미널 밖)면 그 지휘의 watch를 띄움" '[ $rc = 0 ] && pair --name lane-watch-t7cc && pair --to t7cc && has --rm || { cat "$HCLOG" "$T/err"; false; }'
rm -f "$HCLOG"
HC_FAKE_LIVE=t7cc HC_LANE_COORD=t7cc HC_TERMINAL_ID=t3zz run --name lane-a6 --repo "$R" -- claude
t "HC_LANE_COORD가 \$HC_TERMINAL_ID보다 앞" '[ $rc = 0 ] && pair --to t7cc && ! has t3zz'
# 쉼 종료는 hc-fill 대기열에 걸린 요청(wait·up)이 없을 때만
mkdir -p "$T/fq"
printf 'lane-q1\tp\tb\t-\t-\twait\t-\t-\t-\t-\t-\n' >"$T/fq/queue.tsv"
HC_FILL_DATA=$T/fq HC_LANE_WATCH_IDLE=2 HC_FAKE_LIVE=t7cc "$LANE" watch --to t7cc >"$T/wo" 2>"$T/we" &
w7=$!
sleep 3.5
t "watch: hc-fill 대기열에 기다리는 요청이 있으면 레인이 없어도 안 끝남" 'alive $w7 || { cat "$T/we"; false; }'
printf 'lane-q1\tp\tb\t-\t-\tsent\tm\tm/t1\t-\t-\t-\n' >"$T/fq/queue.tsv"
upto '! alive $w7'
wait $w7; wrc=$?
t "watch: 대기열이 비면(sent만) 그 시간 뒤 끝남" '[ $wrc = 0 ] && grep -Fq "끝낸다" "$T/we" || { cat "$T/we"; false; }'
# 2차 감사 [중간] A: 끝난 up 줄(hc-fill add 줄은 rm할 때까지 up으로 남음)은 세지 않는다 — 살아 있는 up 레인은 hc ls가 센다
printf 'lane-q1\tp\tb\t-\t-\tup\tm\tm/t1\t-\t-\t-\n' >"$T/fq/queue.tsv"
HC_FILL_DATA=$T/fq HC_LANE_WATCH_IDLE=2 HC_FAKE_LIVE=t7cc "$LANE" watch --to t7cc >"$T/wo" 2>"$T/we" &
w7=$!
upto '! alive $w7' || kill $w7
wait $w7; wrc=$?
t "watch: 대기열의 끝난 up 줄은 세지 않고 그 시간 뒤 끝남" '[ $wrc = 0 ] && grep -Fq "끝낸다" "$T/we" || { cat "$T/we"; false; }'
# 감사 [중간] 2: hc ls 실패는 실패 시작·회복 때 한 번씩만 보냄(같은 상태 되풀이는 안 보냄)
rm -f "$HCLOG.send"; touch "$HCLOG.lsfail"
HC_FAKE_LIVE=t7cc "$LANE" watch --to t7cc >"$T/wo" 2>"$T/we" &
w8=$!
upto 'grep -Fq "hc ls 실패" "$HCLOG.send" 2>/dev/null'
sleep 7
t "watch: hc ls가 계속 실패해도 알림은 한 번" '[ "$(grep -c "hc ls 실패" "$HCLOG.send")" = 1 ] || { cat "$HCLOG.send"; false; }'
rm -f "$HCLOG.lsfail"
upto 'grep -Fq "hc ls 회복" "$HCLOG.send" 2>/dev/null'
sleep 2.5
t "watch: 되살아나면 「회복」 한 번" '[ "$(grep -c "hc ls 회복" "$HCLOG.send")" = 1 ] || { cat "$HCLOG.send"; false; }'
touch "$HCLOG.lsfail"
upto '[ "$(grep -c "hc ls 실패" "$HCLOG.send")" = 2 ]'
t "watch: 다시 실패하면 한 번 더" '[ "$(grep -c "hc ls 실패" "$HCLOG.send")" = 2 ] || { cat "$HCLOG.send"; false; }'
rm -f "$HCLOG.lsfail"
stopw $w8
# 감사 5: 지휘 hc get이 연결 실패면 끝내지 않고 계속 돈다(「없음」일 때만 끝냄)
touch "$HCLOG.getdown"
HC_FAKE_LIVE=t7cc "$LANE" watch --to t7cc >"$T/wo" 2>"$T/we" &
w9=$!
sleep 5
t "watch: 지휘 hc get 연결 실패는 사라짐이 아님(계속 돈다)" 'alive $w9 || { cat "$T/we"; false; }'
rm -f "$HCLOG.getdown"
# 감사 [낮음] 1: 보내는 글에서 제어 문자를 지움
rm -f "$HCLOG.send"
HC_LANE=lane-w "$LANE" report 중간 - "$(printf 'esc\033[2Jx\001y')" >/dev/null
upto '[ -s "$HCLOG.send" ]'
t "watch: hc send 글에 제어 문자가 없음" 'grep -Fq "esc[2Jxy" "$HCLOG.send" && ! LC_ALL=C grep -q "$(printf "[\001-\010\033]")" "$HCLOG.send" || { cat -v "$HCLOG.send"; false; }'
stopw $w9
# 2차 감사 [중간] B: 지휘 주소를 설정에 박지 않는다. coord 파일의 pid·이름이 지금 그 주소의 터미널과 다르면(코어가 다시 떠
# 주소가 남에게 감) 보내지 않고 끝내며, 사건은 커서에 안 적혀 events에 남는다(지휘가 돌아와 wait/watch를 걸면 받음)
rm -f "$HCLOG.send"
HC_LANE=lane-w "$LANE" report 완료 cafe1234 남의터미널 >/dev/null
HC_FAKE_LIVE=t7cc HC_FAKE_LIVE_PID=9999 "$LANE" watch --to t7cc >"$T/wo" 2>"$T/we" &
w10=$!
upto '! alive $w10' || kill $w10
wait $w10; wrc=$?
t "watch: 그 주소의 pid가 coord와 다르면 hc send 0번, 「지휘 없음」 남기고 종료 0" '[ $wrc = 0 ] && [ ! -s "$HCLOG.send" ] && grep -Fq "지휘 없음" "$T/we" && grep -Fq "지휘 없음" "$WD/watch.log" || { cat "$T/we" "$HCLOG.send"; false; }'
HC_TERMINAL_ID=t7cc HC_FAKE_LIVE=t7cc run wait --timeout 0
t "watch가 못 보낸 사건은 커서에 안 적혀 지휘의 wait가 받음" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq 남의터미널 || { printf "%s\n" "$out"; false; }'
rm -f "$WD/coord.t7cc"
HC_FAKE_LIVE=t7cc "$LANE" watch --to t7cc >"$T/wo" 2>"$T/we" &
w11=$!
HC_LANE=lane-w "$LANE" report 중간 - 파일없음 >/dev/null
upto '! alive $w11' || kill $w11
wait $w11; wrc=$?
t "watch: coord 파일이 없어도 보내지 않고 「지휘 없음」으로 끝남" '[ $wrc = 0 ] && [ ! -s "$HCLOG.send" ] && grep -Fq "지휘 없음" "$T/we" || { cat "$T/we"; false; }'
HC_TERMINAL_ID=t7cc HC_FAKE_LIVE=t7cc run wait --timeout 0
t "지휘가 wait를 걸면 지휘 정보를 coord.<주소>에 남김(그 사건도 받음)" '[ "$(cut -f1-3 "$WD/coord.t7cc")" = "t7cc${tab}4242${tab}t7cc" ] && printf "%s\n" "$out" | grep -Fq 파일없음 || { cat "$WD/coord.t7cc"; false; }'
HC_FAKE_LIVE=t7cc run coord
t "coord: 파일과 hc get(pid·이름)이 같으면 주소, 종료 0" '[ $rc = 0 ] && [ "$out" = t7cc ] || { cat "$T/err"; false; }'
HC_FAKE_LIVE=t7cc HC_FAKE_LIVE_PID=9999 run coord
t "coord: pid가 다르면 종료 1, 까닭 한 줄" '[ $rc = 1 ] && [ -z "$out" ] && grep -Fq "지휘 없음" "$T/err"'
rm -f "$HCLOG"
HC_FAKE_LIVE=t7cc HC_FAKE_LIVE_PID=9999 HC_LANE_COORD=t7cc run --name lane-a7 --repo "$R" -- claude
t "레인 띄우기: HC_LANE_COORD가 coord와 안 맞으면 watch를 안 띄우고 한 줄(레인은 뜸)" '[ $rc = 0 ] && ! has watch && grep -Fq "지휘 없음" "$T/err" || { cat "$HCLOG" "$T/err"; false; }'
cp "$WD/coord.t7cc" "$T/coord.bak"
HC_FAKE_LANETERM=t5ln HC_TERMINAL_ID=t5ln run --name lane-a8 --repo "$R" -- claude
t "레인 터미널(라벨 hc-lane=1)에서 띄우면 지휘 정보로 안 적고 watch도 안 띄움" '[ "$(cat "$WD/coord.t7cc")" = "$(cat "$T/coord.bak")" ] && [ ! -e "$WD/coord.t5ln" ] && ! pair --to t5ln'
# 3차 감사 [중간] 1: 지휘 정보는 지휘마다(coord.<주소>), 사건은 그 레인을 띄운 지휘의 watch만 보낸다. 같은 기계에서 지휘 둘이
# 레인을 띄워도 각자 자기 사건만 받고, 한쪽이 hc-lane을 써도 다른 쪽 watch는 계속 돈다. 주인을 모르는 레인(-)은 살아 있는
# 모든 지휘 watch가 한 번씩 보낸다(hcfill-owner 2026-11-17 — 한 곳만 넘겨받던 판은 넣은 지휘가 못 받았다), 없으면 events에 남긴다
rm -f "$WD"/coord.* "$WD/lanes.tsv" "$HCLOG.send" "$HCLOG"
printf 'ta1 1111\ntb2 2222\n' >"$HCLOG.live"
HC_TERMINAL_ID=ta1 HC_FAKE_RUN_ADDR=t9aa run --name lane-ma --repo "$R" -- claude
sleep 1.1   # 띄운 시각(초)이 달라야 「가장 최근」이 갈린다
HC_TERMINAL_ID=tb2 HC_FAKE_RUN_ADDR=t9bb run --name lane-mb --repo "$R" -- claude
t "지휘 둘: 지휘마다 지휘 정보 파일, 레인 기록에 띄운 지휘" '[ "$(cut -f1-3 "$WD/coord.ta1")" = "ta1${tab}1111${tab}ta1" ] && [ "$(cut -f1-3 "$WD/coord.tb2")" = "tb2${tab}2222${tab}tb2" ] && grep -Fxq "t9aa${tab}lane-ma${tab}ta1" "$WD/lanes.tsv" && grep -Fxq "t9bb${tab}lane-mb${tab}tb2" "$WD/lanes.tsv" || { cat "$WD"/coord.* "$WD/lanes.tsv"; false; }'
t "지휘 둘: 각자 자기 watch(lane-watch-<주소>)를 띄움" 'pair --to ta1 && pair --to tb2 && pair --name lane-watch-ta1 && pair --name lane-watch-tb2'
"$LANE" watch --to ta1 >"$T/woa" 2>"$T/wea" &
wa=$!
"$LANE" watch --to tb2 >"$T/wob" 2>"$T/web" &
wb=$!
upto '[ -s "$WD/watch.ta1/pid" ] && [ -s "$WD/watch.tb2/pid" ]'
HC_TERMINAL_ID=t9aa HC_LANE=lane-ma "$LANE" report 완료 aaaa1111 에이것 >/dev/null
HC_TERMINAL_ID=t9bb HC_LANE=lane-mb "$LANE" report 완료 bbbb2222 비것 >/dev/null
HC_TERMINAL_ID=t9cc HC_LANE=lane-fill "$LANE" report 완료 cccc3333 필것 >/dev/null   # 띄운 지휘가 없는 레인(hc-fill 꼴)
upto 'grep -Fq 에이것 "$HCLOG.send" 2>/dev/null && grep -Fq 비것 "$HCLOG.send" && grep -Fq 필것 "$HCLOG.send"'
sleep 2.5
t "지휘 둘: 각자 자기 레인 사건만 받음(남의 사건 send 0)" '[ "$(grep -F 에이것 "$HCLOG.send" | cut -f1 | sort -u)" = ta1 ] && [ "$(grep -F 비것 "$HCLOG.send" | cut -f1 | sort -u)" = tb2 ] && [ "$(grep -c 에이것 "$HCLOG.send")" = 1 ] || { cat "$HCLOG.send" "$T/wea" "$T/web"; false; }'
t "지휘 둘: 주인을 모르는 레인 사건은 두 지휘 모두 한 번씩(넘겨받지 않음, sent에 안 적음)" '[ "$(grep -F 필것 "$HCLOG.send" | cut -f1 | sort | tr "\n" " ")" = "ta1 tb2 " ] && ! awk -F"\t" "\$1 == \"t9cc\"" "$WD/lanes.tsv" | grep -q . || { cat "$HCLOG.send" "$WD/lanes.tsv"; false; }'
HC_TERMINAL_ID=tb2 run wait --timeout 0   # 다른 지휘가 hc-lane을 써도
HC_TERMINAL_ID=t9aa HC_LANE=lane-ma "$LANE" report 착지 aaaa1111 에이착지 >/dev/null
upto 'grep -Fq 에이착지 "$HCLOG.send" 2>/dev/null'
t "지휘 둘: 다른 지휘가 wait를 걸어도 앞 지휘 watch는 계속 돌고 자기 사건을 받음" 'alive $wa && [ "$(grep -F 에이착지 "$HCLOG.send" | cut -f1)" = ta1 ] || { cat "$HCLOG.send" "$T/wea"; false; }'
# 4차 감사 [중간] 1: 「보냄」은 사건 하나에 한 번 — 끝난 지휘가 이미 받은 사건은 남은 지휘에게 다시 안 감, 못 받은 것만 넘어감
touch "$HCLOG.sendfail"
HC_TERMINAL_ID=t9bb HC_LANE=lane-mb "$LANE" report 막힘 - 비못받음 >/dev/null   # tb2가 보내려다 실패(커서·보냄 표시 없음)
sleep 2.5
printf 'ta1 1111\n' >"$HCLOG.live"   # tb2가 사라짐
rm -f "$HCLOG.sendfail"
HC_TERMINAL_ID=t9cc HC_LANE=lane-fill "$LANE" report 착지 cccc3333 필착지 >/dev/null
upto 'grep -Fq 필착지 "$HCLOG.send" 2>/dev/null && grep -Fq 비못받음 "$HCLOG.send"'
sleep 2.5
t "지휘 둘: 한 지휘가 사라지면 주인 모르는 레인 사건은 살아 있는 지휘(ta1)만" '[ "$(grep -F 필착지 "$HCLOG.send" | cut -f1 | tr "\n" " ")" = "ta1 " ] || { cat "$HCLOG.send"; false; }'
t "지휘 둘: 끝난 지휘(tb2)가 이미 받은 사건(비것)은 ta1에게 다시 안 감, 주인 모르는 필것은 ta1도 한 번만" '! grep "^ta1" "$HCLOG.send" | grep -q 비것 && [ "$(grep "^ta1" "$HCLOG.send" | grep -c 필것)" = 1 ] || { cat "$HCLOG.send"; false; }'
t "지휘 둘: tb2가 못 받은 사건(보내기 실패)은 ta1이 한 번 받음" '[ "$(grep -F 비못받음 "$HCLOG.send" | cut -f1 | tr "\n" " ")" = "ta1 " ] || { cat "$HCLOG.send"; false; }'
stopw $wa; stopw $wb
: >"$HCLOG.live"   # 지휘가 아무도 없음
rm -f "$HCLOG.send"
HC_TERMINAL_ID=t9cc HC_LANE=lane-fill "$LANE" report 중간 - 아무도없음 >/dev/null
run coord
t "coord: 살아 있는 지휘가 없으면 종료 1(hc-fill은 watch를 안 띄움)" '[ $rc = 1 ] && grep -Fq "지휘 없음" "$T/err"'
printf 'ta1 1111\n' >"$HCLOG.live"
HC_TERMINAL_ID=ta1 run wait --timeout 0
t "보내지 못한 사건은 events에 남아 지휘가 wait를 걸면 받음" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq 아무도없음'
# wait로 받은 사건도 「보냄」: 그 지휘가 끝난 뒤 다른 지휘의 watch가 다시 보내지 않음
printf 'ta1 1111\ntb2 2222\n' >"$HCLOG.live"
# 이 시험은 tb2가 주인인 새 레인으로 — tb2가 새 레인을 띄움
HC_TERMINAL_ID=tb2 HC_FAKE_RUN_ADDR=t9ee run --name lane-me --repo "$R" -- claude
HC_TERMINAL_ID=t9ee HC_LANE=lane-me "$LANE" report 중간 - 비웨이트 >/dev/null
HC_TERMINAL_ID=tb2 run wait --timeout 0
t "wait로 받음(tb2)" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq 비웨이트'
printf 'ta1 1111\n' >"$HCLOG.live"; rm -f "$HCLOG.send"
"$LANE" watch --to ta1 >"$T/woa" 2>"$T/wea" &
wa=$!
HC_TERMINAL_ID=t9ee HC_LANE=lane-me "$LANE" report 중간 - 비새것 >/dev/null
upto 'grep -Fq 비새것 "$HCLOG.send" 2>/dev/null'
t "지휘가 wait로 받은 사건은 그 지휘가 끝난 뒤 남은 지휘 watch가 다시 안 보냄(새 것만)" 'grep -Fq 비새것 "$HCLOG.send" && ! grep -Fq 비웨이트 "$HCLOG.send" || { cat "$HCLOG.send" "$T/wea"; false; }'
stopw $wa
# 4차 감사 [낮음] 1: 레인 기록이 -(띄울 때 지휘 확인 실패)여도 hc parent가 있으면 그 지휘로(가장 최근 지휘 tb2가 아니라)
printf 'ta1 1111\ntb2 2222\n' >"$HCLOG.live"; rm -f "$HCLOG.send"
printf 't9dd\tlane-md\t-\n' >>"$WD/lanes.tsv"
printf '%s\n' '{"terminals":[{"address":"t9dd","name":"lane-md","parent":"mac/ta1","state":"live","status":"working","turn":1}]}' >"$HCLOG.lsfile"
"$LANE" watch --to ta1 >"$T/woa" 2>"$T/wea" &
wa=$!
"$LANE" watch --to tb2 >"$T/wob" 2>"$T/web" &
wb=$!
HC_TERMINAL_ID=t9dd HC_LANE=lane-md "$LANE" report 완료 dddd4444 디것 >/dev/null
upto 'grep -Fq 디것 "$HCLOG.send" 2>/dev/null'
sleep 2.5
t "레인 기록이 -여도 hc parent(ta1)가 있으면 그 지휘에게만" '[ "$(grep -F 디것 "$HCLOG.send" | cut -f1 | tr "\n" " ")" = "ta1 " ] || { cat "$HCLOG.send"; false; }'
stopw $wa; stopw $wb
rm -f "$HCLOG.lsfile"
# 5차 감사 [중간] 1: sent는 watch만 쓴다. 남의 지휘(tb2)의 wait가 ta1 레인 사건을 봐도 ta1 watch는 그 사건을 보낸다
printf 'ta1 1111\ntb2 2222\n' >"$HCLOG.live"; rm -f "$HCLOG.send"
HC_TERMINAL_ID=t9aa HC_LANE=lane-ma "$LANE" report 중간 - 에이늦음 >/dev/null
HC_TERMINAL_ID=tb2 run wait --timeout 0
t "남의 지휘(tb2)의 wait는 ta1 레인 사건을 보기만 함" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq 에이늦음 && ! grep -Fq 에이늦음 "$WD/sent" 2>/dev/null'
"$LANE" watch --to ta1 >"$T/woa" 2>"$T/wea" &
wa=$!
upto 'grep -Fq 에이늦음 "$HCLOG.send" 2>/dev/null'
t "그 뒤에도 주인 ta1의 watch가 자기 레인 사건을 보냄" '[ "$(grep -F 에이늦음 "$HCLOG.send" | cut -f1 | tr "\n" " ")" = "ta1 " ] || { cat "$HCLOG.send" "$T/wea"; false; }'
touch "$HCLOG.sendfail"
HC_TERMINAL_ID=t9aa HC_LANE=lane-ma "$LANE" report 막힘 - 에이남음 >/dev/null   # ta1이 보내려다 실패
sleep 2.5
printf 'tb2 2222\n' >"$HCLOG.live"   # ta1이 끝남
upto '! alive $wa' || kill $wa
wait $wa 2>/dev/null
rm -f "$HCLOG.sendfail"
"$LANE" watch --to tb2 >"$T/wob" 2>"$T/web" &
wb=$!
upto 'grep -Fq 에이남음 "$HCLOG.send" 2>/dev/null'
sleep 2.5
t "ta1이 끝난 뒤 tb2 watch는 ta1이 안 보낸 것(에이남음)만 받음 — 보낸 것(에이늦음)은 다시 안 감" '[ "$(grep -F 에이남음 "$HCLOG.send" | cut -f1 | tr "\n" " ")" = "tb2 " ] && [ "$(grep -c 에이늦음 "$HCLOG.send")" = 1 ] || { cat "$HCLOG.send" "$T/web"; false; }'
t "주인이 사라진 레인 사건을 보낸 watch도 주인이 되지 않음(레인 기록 그대로 ta1 — 감사 [중간] 1)" '[ "$(awk -F"\t" "\$1 == \"t9aa\" { o = \$3 } END { print o }" "$WD/lanes.tsv")" = ta1 ] || { cat "$WD/lanes.tsv"; false; }'
stopw $wb
# 6차 감사 [중간] 1(R6): 「보낸 것」 = sent ∪ 그 레인의 옛 주인 seen. 옛 주인이 wait로 받은 사건은 주인이 사라진 뒤에도 다시 안 감
printf 'ta1 1111\ntb2 2222\n' >"$HCLOG.live"; rm -f "$HCLOG.send"
HC_TERMINAL_ID=ta1 HC_FAKE_RUN_ADDR=t9ff run --name lane-mf --repo "$R" -- claude
HC_TERMINAL_ID=t9ff HC_LANE=lane-mf "$LANE" report 완료 ffff6666 에프하나 >/dev/null
HC_TERMINAL_ID=ta1 run wait --timeout 0
t "R6: ta1이 wait로 에프하나를 받음" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq 에프하나'
HC_TERMINAL_ID=t9ff HC_LANE=lane-mf "$LANE" report 중간 - 에프둘 >/dev/null
printf 'tb2 2222\n' >"$HCLOG.live"   # ta1이 끝남
"$LANE" watch --to tb2 >"$T/wob" 2>"$T/web" &
wb=$!
upto 'grep -Fq 에프둘 "$HCLOG.send" 2>/dev/null'
sleep 4
t "R6: 주인 ta1이 사라진 뒤 tb2는 에프둘만 받고, ta1이 wait로 받은 에프하나는 다시 안 받음" 'grep -Fq 에프둘 "$HCLOG.send" && ! grep -Fq 에프하나 "$HCLOG.send" || { cat "$HCLOG.send" "$T/web"; false; }'
stopw $wb
# hcfill-owner(2026-11-17): hc-fill 대기열에 넣은 지휘(tq1)의 레인 사건이 다른 지휘(tq2, 가장 최근에 레인을 띄움)로만 갔다.
# hc-fill add는 hc-lane coord --set으로 지휘 정보를 남기고, 띄울 때 HC_LANE_OWNER로 그 지휘를 레인 기록 주인으로 넘긴다
printf 'ta1 1111\ntb2 2222\n' >"$HCLOG.live"; rm -f "$HCLOG.send" "$HCLOG"
rm -f "$WD/coord.ta1"
HC_TERMINAL_ID=ta1 run coord --set
t "coord --set: 부른 지휘의 지휘 정보를 남기고 주소를 찍음" '[ $rc = 0 ] && [ "$out" = ta1 ] && [ "$(cut -f1-3 "$WD/coord.ta1")" = "ta1${tab}1111${tab}ta1" ] || { cat "$T/err"; false; }'
HC_FAKE_LANETERM=t5ln run coord --set t5ln
t "coord --set: 레인 터미널은 지휘 정보로 안 적고 종료 1" '[ $rc = 1 ] && [ ! -e "$WD/coord.t5ln" ]'
sleep 1.1
HC_TERMINAL_ID=tb2 HC_FAKE_RUN_ADDR=t9gg run --start-wait 0 --name lane-mg --repo "$R" -- claude   # tb2가 가장 최근에 레인을 띄움
HC_LANE_OWNER=ta1 HC_LANE_COORD=ta1 HC_LANE_FILL=1 HC_FAKE_RUN_ADDR=t9hh run --start-wait 0 --name lane-mh --repo "$R" -- claude   # hc-fill(터미널 밖)
t "HC_LANE_OWNER: 레인 기록 주인이 그 지휘(fill), 레인 환경에도 HC_LANE_OWNER" '[ $rc = 0 ] && grep -Fxq "t9hh${tab}lane-mh${tab}ta1${tab}fill" "$WD/lanes.tsv" && has HC_LANE_OWNER=ta1 || { cat "$WD/lanes.tsv" "$T/err"; false; }'
printf 't9ii\tlane-mi\t-\tfill\n' >>"$WD/lanes.tsv"   # 주인 없는 옛 hc-fill 레인
"$LANE" watch --to ta1 >"$T/woa" 2>"$T/wea" &
wa=$!
"$LANE" watch --to tb2 >"$T/wob" 2>"$T/web" &
wb=$!
upto '[ -s "$WD/watch.ta1/pid" ] && [ -s "$WD/watch.tb2/pid" ]'
HC_TERMINAL_ID=t9hh HC_LANE=lane-mh "$LANE" report 완료 hhhh8888 넣은지휘것 >/dev/null
HC_TERMINAL_ID=t9ii HC_LANE=lane-mi "$LANE" report 완료 iiii9999 주인없는것 >/dev/null
upto 'grep -Fq 넣은지휘것 "$HCLOG.send" 2>/dev/null && [ "$(grep -c 주인없는것 "$HCLOG.send")" = 2 ]'
sleep 2.5
t "대기열 레인: 넣은 지휘(ta1)만 받음 — 가장 최근 지휘(tb2)가 아님" '[ "$(grep -F 넣은지휘것 "$HCLOG.send" | cut -f1 | tr "\n" " ")" = "ta1 " ] || { cat "$HCLOG.send" "$T/wea" "$T/web"; false; }'
t "주인 없는 옛 레인: 두 지휘 모두 한 번씩 받고 주인은 그대로 -" '[ "$(grep -F 주인없는것 "$HCLOG.send" | cut -f1 | sort | tr "\n" " ")" = "ta1 tb2 " ] && [ "$(awk -F"\t" "\$1 == \"t9ii\" { o = \$3 } END { print o }" "$WD/lanes.tsv")" = - ] || { cat "$HCLOG.send" "$WD/lanes.tsv"; false; }'
# hcfill-early(2026-11-18): report --to <지휘>는 받을 지휘를 정한다(아홉째 칸) — 주인 없는 사건이어도 그 지휘 watch만(hc-fill 놀고 있음 알림)
env -u HC_TERMINAL_ID -u HC_LANE "$LANE" report 중간 - 받는이정함 --name hc-fill-idle --to tb2 >/dev/null
upto 'grep -Fq 받는이정함 "$HCLOG.send" 2>/dev/null'
sleep 2.5
t "report --to tb2: tb2 watch만 받음(주인 모르는 사건이어도 ta1은 안 받음)" '[ "$(grep -F 받는이정함 "$HCLOG.send" | cut -f1 | tr "\n" " ")" = "tb2 " ] || { cat "$HCLOG.send" "$T/wea" "$T/web"; false; }'
run report 중간 - x --to 't1;rm'
t "report --to: 주소 글자가 아니면 종료 2" '[ $rc = 2 ]'
stopw $wa; stopw $wb
# 감사 [중간] 1(b): 주인 지휘(ta1)가 레인이 뜬 뒤 사라지면 그 사건은 살아 있는 모든 지휘(tb2·tc3)가 한 번씩 받고, 누구도 넘겨받지
# 않는다(가장 최근 지휘 한 곳이 영구 주인이 되던 판은 그 레인을 이어받은 다른 지휘가 못 받았다)
rm -f "$HCLOG.send"
printf 'ta1 1111\ntb2 2222\ntc3 3333\n' >"$HCLOG.live"
HC_TERMINAL_ID=tc3 run coord --set
HC_LANE_OWNER=ta1 HC_LANE_COORD=ta1 HC_LANE_FILL=1 HC_FAKE_RUN_ADDR=t9jj run --start-wait 0 --name lane-mj --repo "$R" -- claude
printf 'tb2 2222\ntc3 3333\n' >"$HCLOG.live"   # ta1이 끝남
"$LANE" watch --to tb2 >"$T/wob" 2>"$T/web" &
wb=$!
"$LANE" watch --to tc3 >"$T/woc" 2>"$T/wec" &
wc=$!
upto '[ -s "$WD/watch.tb2/pid" ] && [ -s "$WD/watch.tc3/pid" ]'
HC_TERMINAL_ID=t9jj HC_LANE=lane-mj "$LANE" report 중간 - 주인사라짐1 >/dev/null
upto '[ "$(grep -c 주인사라짐1 "$HCLOG.send" 2>/dev/null)" = 2 ]'
HC_TERMINAL_ID=t9jj HC_LANE=lane-mj "$LANE" report 완료 jjjj0000 주인사라짐2 >/dev/null
upto '[ "$(grep -c 주인사라짐2 "$HCLOG.send" 2>/dev/null)" = 2 ]'
sleep 2.5
t "주인이 사라진 레인: 살아 있는 두 지휘 모두 한 번씩, 다음 사건도 둘 다(넘겨받지 않음)" '[ "$(grep -F 주인사라짐1 "$HCLOG.send" | cut -f1 | sort | tr "\n" " ")" = "tb2 tc3 " ] && [ "$(grep -F 주인사라짐2 "$HCLOG.send" | cut -f1 | sort | tr "\n" " ")" = "tb2 tc3 " ] || { cat "$HCLOG.send" "$T/web" "$T/wec"; false; }'
t "주인이 사라진 레인: 레인 기록 주인은 그대로 ta1(한 지휘가 영구히 넘겨받지 않음)" '[ "$(awk -F"\t" "\$1 == \"t9jj\" { o = \$3 } END { print o }" "$WD/lanes.tsv")" = ta1 ] || { cat "$WD/lanes.tsv"; false; }'
stopw $wb; stopw $wc
printf 'ta1 1111\n' >"$HCLOG.live"
run coord --check ta1
t "coord --check: 살아 있고 지휘 정보와 맞으면 0, 아무것도 안 씀" '[ $rc = 0 ]'
run coord --check tb2
t "coord --check: 사라진 지휘는 1" '[ $rc = 1 ] && grep -Fq "coord --check" "$T/err"'
HC_TERMINAL_ID=ta1 run coord --set   # 아래 coord 시험: 가장 최근은 ta1
run coord
t "coord: 살아 있는 지휘 가운데 가장 최근에 레인을 띄운 곳(지금은 ta1)" '[ $rc = 0 ] && [ "$out" = ta1 ] || { cat "$T/err"; false; }'
rm -f "$HCLOG.live"
unset XDG_STATE_HOME HC_LANE_WAIT_TICK
run watch
t "watch: --to 없으면 오류" '[ $rc != 0 ] && [ $rc != 75 ] && grep -Fq -- --to "$T/err"'
run watch --to t7cc --timeout 5
t "watch: --timeout은 wait만" '[ $rc != 0 ] && [ $rc != 75 ]'

# sweep-rename 2(2026-11-17): hc 기계 이름을 바꾸면(oldm → newm) 원격 사건의 기계·주소 칸이 새 이름으로 읽혀
# 이미 본 사건·멈춤이 다시 쏟아졌다. 열쇠에서 기계 이름을 빼고, 옛 꼴 커서 줄도 같은 사건으로 읽는다
MH=$T/mh; mkdir -p "$MH/oldm/.local/state/hc-lane"
nw=$(date -u +%Y-%m-%dT%H:%M:%SZ)
{ ev8 "$nw" x t5 lane-rn 완료 abcd1234 이름바꿈 2; } >"$MH/oldm/.local/state/hc-lane/events.tsv"
conf 'lane.wait_grace 0' 'lane.wait_busy' 'lane.wait_procs'
lsR() { printf '{"terminals":[{"address":"%s/t6","name":"lane-rn2","state":"live","status":"blocked","turn":2},{"address":"%s/t7","name":"lane-rn3","state":"live","status":"done","turn":2}]}' "$1" "$1"; }
HC_TERMINAL_ID=tR HC_FAKE_MACH_HOME=$MH HC_FAKE_MACHINES='{"machines":[{"name":"oldm","state":"connected"}]}' HC_FAKE_LS=$(lsR oldm) run wait --all --since "$nw" --timeout 0
t "이름 바꿈(준비): 옛 이름으로 사건·멈춤 셋을 받음" '[ $rc = 0 ] && [ "$(printf "%s\n" "$out" | grep -c "${tab}lane-rn")" = 3 ] || { printf "%s\n" "$out"; cat "$T/err"; false; }'
mv "$MH/oldm" "$MH/newm"
HC_TERMINAL_ID=tR HC_FAKE_MACH_HOME=$MH HC_FAKE_MACHINES='{"machines":[{"name":"newm","state":"connected"}]}' HC_FAKE_LS=$(lsR newm) run wait --all --timeout 0
t "이름 바꿈: 기계 이름만 다른 같은 사건·멈춤은 다시 안 나옴" '! printf "%s\n" "$out" | grep -q "${tab}lane-rn" || { printf "%s\n" "$out"; false; }'
# 옮김: 옛 판이 적은 커서(기계 이름이 든 줄 그대로·멈춤 열쇠)도 같은 사건으로 읽는다
SD=$T/.local/state/hc-lane
printf '#since\t%s\n%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n%s\n%s\n' "$nw" "$nw" oldm oldm/t5 lane-rn 완료 abcd1234 이름바꿈 2 \
  "멈춤${tab}oldm/t6${tab}lane-rn2${tab}blocked turn 2" "멈춤${tab}oldm/t7${tab}lane-rn3${tab}쉼 turn 2" >"$SD/seen.tM"
HC_TERMINAL_ID=tM HC_FAKE_MACH_HOME=$MH HC_FAKE_MACHINES='{"machines":[{"name":"newm","state":"connected"}]}' HC_FAKE_LS=$(lsR newm) run wait --all --timeout 0
t "이름 바꿈 옮김: 옛 꼴 커서 줄(옛 기계 이름)과 같은 사건은 안 나옴" '! printf "%s\n" "$out" | grep -q "${tab}lane-rn" || { printf "%s\n" "$out"; false; }'
noconf

# sweep-rename 5(2026-11-17 지휘): 지휘가 hc kill로 닫은 레인(reason killed)과, --read·hc-fill 레인이 「완료」 뒤 그 턴에 끝난 것은
# 「뜻밖의 종료」가 아니다(01:3x 끝난 레인 여덟을 닫자 여덟 줄). 「완료」 뒤 감사를 기다리는 구현 레인이 혼자 끝나면 그대로 멈춤
HC_FAKE_RUN_ADDR=tk1 run --read --start-wait 0 --name lane-k1 --repo "$R" -- claude
t "read 레인 기록(lanes.tsv 넷째 칸 read)" '[ $rc = 0 ] && grep -Fxq "tk1${tab}lane-k1${tab}-${tab}read" "$SD/lanes.tsv" || { cat "$SD/lanes.tsv" "$T/err"; false; }'
HC_FAKE_LIVE=t7cc HC_LANE_COORD=t7cc HC_FAKE_RUN_ADDR=tk2x run --start-wait 0 --name lane-k2x --repo "$R" -- claude
t "HC_LANE_COORD만 물려받은 구현 레인은 fill이 아님(재감사 [낮음] 4 — fill은 hc-fill이 HC_LANE_FILL=1로)" '[ $rc = 0 ] && grep -Fxq "tk2x${tab}lane-k2x${tab}-" "$SD/lanes.tsv" || { cat "$SD/lanes.tsv" "$T/err"; false; }'
HC_FAKE_LIVE=t7cc HC_LANE_COORD=t7cc HC_LANE_FILL=1 HC_FAKE_RUN_ADDR=tk2 run --start-wait 0 --name lane-k2 --repo "$R" -- claude
t "hc-fill 레인 기록(넷째 칸 fill)" '[ $rc = 0 ] && grep -Fxq "tk2${tab}lane-k2${tab}-${tab}fill" "$SD/lanes.tsv" || { cat "$SD/lanes.tsv" "$T/err"; false; }'
HC_FAKE_RUN_ADDR=tk5 run --start-wait 0 --name lane-k5 --repo "$R" -- claude
nw=$(date -u +%Y-%m-%dT%H:%M:%SZ)
{ ev8 "$nw" here tk1 lane-k1 완료 - 조사끝 2; ev8 "$nw" here tk2 lane-k2 완료 abcd1234 고침 2
  ev8 "$nw" here tk3 lane-k3 완료 abcd1234 감사기다림 2; ev8 "$nw" here tk4 lane-k4 중간 - 반 2
  ev8 "$nw" here tk5 lane-k5 완료 abcd1234 구현 2
  ev8 "$nw" here tk6 lane-k6 완료 abcd1234 손 2; ev8 "$nw" here tk6 lane-k6 정리됨 - "hc-sweep R2" 3; } >>"$EV"
{ printf 'tk6\tlane-k6\t-\tfill\n'; } >>"$SD/lanes.tsv"
HC_TERMINAL_ID=tK run wait --state 완료,중간 --timeout 0   # report 줄은 먼저 소비
HC_TERMINAL_ID=tK HC_FAKE_LS='{"terminals":[{"address":"tk1","name":"lane-k1","state":"finished","reason":"exited","turn":3},{"address":"tk2","name":"lane-k2","state":"finished","reason":"exited","turn":3},{"address":"tk3","name":"lane-k3","state":"finished","reason":"killed","signal":1,"turn":3},{"address":"tk4","name":"lane-k4","state":"finished","reason":"killed","signal":1,"turn":9},{"address":"tk5","name":"lane-k5","state":"finished","reason":"exited","turn":3}]}' \
  run wait --state 멈춤 --timeout 0
t "killed: 지휘가 닫은 레인은 완료·중간 뒤라도 안 알림" '! printf "%s\n" "$out" | grep -Eq "lane-k[34]${tab}멈춤" || { printf "%s\n" "$out"; false; }'
t "read·hc-fill 레인: 완료 그 턴의 끝남·정리됨은 조용" '! printf "%s\n" "$out" | grep -Eq "lane-k[126]${tab}멈춤" || { printf "%s\n" "$out"; false; }'
t "구현 레인: 완료(감사 기다림) 뒤 혼자 끝나면 그대로 뜻밖의 종료" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}tk5${tab}lane-k5${tab}멈춤${tab}-${tab}뜻밖의 종료(finished)" || { printf "%s\n" "$out"; cat "$T/err"; false; }'
# 재감사 [중간] M1: 부모(지휘)가 죽어 die_with_parent로 함께 killed된 레인은 지휘가 닫은 것이 아니다 — 그대로 알림.
# killed를 조용히 하는 것은 parent가 없거나(hc-fill 레인 — 누가 hc kill로 닫음) parent가 아직 살아 있을 때만
printf 'tPlive 3333\n' >"$HCLOG.live"
nw=$(date -u +%Y-%m-%dT%H:%M:%SZ)
{ ev8 "$nw" here tk8 lane-k8 완료 abcd1234 부모죽음 2; ev8 "$nw" here tk9 lane-k9 완료 abcd1234 부모살아 2; } >>"$EV"
HC_TERMINAL_ID=tK run wait --state 완료 --timeout 0
HC_TERMINAL_ID=tK HC_FAKE_LS='{"terminals":[{"address":"tk8","name":"lane-k8","parent":"mac/tPdead","state":"finished","reason":"killed","signal":1,"turn":3},{"address":"tk9","name":"lane-k9","parent":"tPlive","state":"finished","reason":"killed","signal":1,"turn":3}]}' \
  run wait --state 멈춤 --timeout 0
t "killed M1: 부모(지휘)가 죽어 함께 끝난 레인은 그대로 뜻밖의 종료" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}tk8${tab}lane-k8${tab}멈춤${tab}-${tab}뜻밖의 종료(finished)" || { printf "%s\n" "$out"; cat "$T/err"; false; }'
t "killed M1: 부모가 살아 있으면(지휘가 직접 닫음) 조용" '! printf "%s\n" "$out" | grep -Fq "lane-k9${tab}멈춤"'
printf 'tPlive 3333\n' >"$HCLOG.live"
# 3차 감사 [중간] N1: 원격 레인의 parent는 그 기계가 부르는 이름 「<지휘 기계>/t…」라 지휘 기계에서 그대로 hc get하면 안 풀린다.
# 앞부분이 hc machine ls에 없는 이름(= 이 기계 자신)이면 떼고 로컬로 본다. 낮음 1: 조용히 넘긴 killed는 커서에 적어 부모가 끝난 뒤에도 다시 안 옴
MH3=$T/mh3; mkdir -p "$MH3/m1/.local/state/hc-lane"
nw=$(date -u +%Y-%m-%dT%H:%M:%SZ)
{ ev8 "$nw" x tkR lane-kR 완료 abcd1234 원격살아 2; ev8 "$nw" x tkS lane-kS 완료 abcd1234 원격죽음 2; } >"$MH3/m1/.local/state/hc-lane/events.tsv"
lsN='{"terminals":[{"address":"m1/tkR","name":"lane-kR","parent":"mac/tPlive","state":"finished","reason":"killed","signal":1,"turn":3},{"address":"m1/tkS","name":"lane-kS","parent":"mac/tPdead","state":"finished","reason":"killed","signal":1,"turn":3}]}'
HC_TERMINAL_ID=tN HC_FAKE_MACH_HOME=$MH3 HC_FAKE_MACHINES='{"machines":[{"name":"m1","state":"connected"}]}' run wait --all --since "$nw" --state 완료 --timeout 0
HC_TERMINAL_ID=tN HC_FAKE_MACH_HOME=$MH3 HC_FAKE_MACHINES='{"machines":[{"name":"m1","state":"connected"}]}' HC_FAKE_LS=$lsN run wait --all --state 멈춤 --timeout 0
t "killed N1: 원격 레인 parent가 「<이 기계>/t…」여도 살아 있으면 조용" '! printf "%s\n" "$out" | grep -Fq "lane-kR${tab}멈춤" || { printf "%s\n" "$out"; cat "$T/err"; false; }'
t "killed N1: 원격 레인 parent가 죽었으면 그대로 뜻밖의 종료" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "m1/tkS${tab}lane-kS${tab}멈춤${tab}-${tab}뜻밖의 종료(finished)" || { printf "%s\n" "$out"; cat "$T/err"; false; }'
# 3차 감사 [낮음] L1: 들어온 연결의 상대(워커 맥에서 본 지휘 맥)는 machine ls에 없다 — 그대로 풀리는 parent는 떼지 않는다
printf 'peer/tPeer 4444\n' >>"$HCLOG.live"
nw=$(date -u +%Y-%m-%dT%H:%M:%SZ)
ev8 "$nw" here tkL lane-kL 완료 abcd1234 들어온연결 2 >>"$EV"
HC_TERMINAL_ID=tN run wait --since "$nw" --state 완료 --timeout 0
HC_TERMINAL_ID=tN HC_FAKE_LS='{"terminals":[{"address":"tkL","name":"lane-kL","parent":"peer/tPeer","state":"finished","reason":"killed","signal":1,"turn":3}]}' run wait --state 멈춤 --timeout 0
t "killed L1: machine ls에 없지만 그대로 풀리는 parent(들어온 연결의 지휘)가 살아 있으면 조용" '! printf "%s\n" "$out" | grep -Fq "lane-kL${tab}멈춤" || { printf "%s\n" "$out"; false; }'
rm -f "$HCLOG.live"
HC_TERMINAL_ID=tN HC_FAKE_MACH_HOME=$MH3 HC_FAKE_MACHINES='{"machines":[{"name":"m1","state":"connected"}]}' HC_FAKE_LS=$lsN run wait --all --state 멈춤 --timeout 0
t "killed 낮음 1: 조용히 넘긴 killed는 부모(지휘)가 끝난 뒤에도 다시 안 옴" '! printf "%s\n" "$out" | grep -Fq "lane-kR${tab}멈춤" || { printf "%s\n" "$out"; false; }'
rm -f "$HCLOG.live"
# watch-parent(2026-11-17 지휘 실측): hc는 원격 레인의 parent를 묻는 쪽 기준 주소로 준다 — 레인 기계에선 「<지휘 기계>/t7」,
# 지휘 기계에선 앞부분 없이 「t7」. 그 레인 기계를 붙여(m1/t7) 물으면 「없음」이라 지휘가 닫은 레인이 뜻밖의 종료로 왔다
printf 'tPlive 3333\n' >"$HCLOG.live"
nw=$(date -u +%Y-%m-%dT%H:%M:%SZ)
{ ev8 "$nw" x tkT lane-kT 완료 abcd1234 접두없음살아 2; ev8 "$nw" x tkU lane-kU 완료 abcd1234 접두없음죽음 2; } >>"$MH3/m1/.local/state/hc-lane/events.tsv"
lsP2='{"terminals":[{"address":"m1/tkT","name":"lane-kT","parent":"tPlive","die_with_parent":true,"state":"finished","reason":"killed","signal":1,"turn":3},{"address":"m1/tkU","name":"lane-kU","parent":"tPdead","die_with_parent":true,"state":"finished","reason":"killed","signal":1,"turn":3}]}'
HC_TERMINAL_ID=tN HC_FAKE_MACH_HOME=$MH3 HC_FAKE_MACHINES='{"machines":[{"name":"m1","state":"connected"}]}' run wait --all --since "$nw" --state 완료 --timeout 0
HC_TERMINAL_ID=tN HC_FAKE_MACH_HOME=$MH3 HC_FAKE_MACHINES='{"machines":[{"name":"m1","state":"connected"}]}' HC_FAKE_LS=$lsP2 run wait --all --state 멈춤 --timeout 0
t "killed watch-parent: 원격 레인 parent가 앞부분 없는 「t…」(지휘 기계 기준)이고 살아 있으면 조용" '! printf "%s\n" "$out" | grep -Fq "lane-kT${tab}멈춤" || { printf "%s\n" "$out"; cat "$T/err"; false; }'
t "killed watch-parent: 앞부분 없는 parent가 죽었으면 그대로 뜻밖의 종료" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "m1/tkU${tab}lane-kU${tab}멈춤${tab}-${tab}뜻밖의 종료(finished)" || { printf "%s\n" "$out"; cat "$T/err"; false; }'
rm -f "$HCLOG.live"
# sweep-rename 6(2026-11-17 지휘): 배경 검증을 기다리며 차례만 끝낸 레인이 「멈춤 done」으로 왔다(네 건). 그 레인 작업 트리 경로가
# 명령 줄에 든 프로세스(터미널 자손이 아니어도 — 떨어져 나간 배경 실행)가 살아 있으면 쉬는 중이 아니라 바쁨
printf '%s\n' '✻ Churned for 3m · done 12:33 PM' '❯ ' >"$GS"
lsB='{"terminals":[{"address":"tb9","name":"lane-b9","cwd":"/w/app.wt/lane-b9","state":"live","status":"done","turn":2}]}'
{ echo " 4000     1 node agent-runner.js"; echo " 4001  4000 claude --model x"
  echo " 6001     1 bash /w/app.wt/lane-b9/.claude/herdr-verify.sh"; echo " 6002     1 bash /w/app.wt/lane-b99/x"; } >"$PSF"
HC_TERMINAL_ID=tK HC_FAKE_PID=4000 HC_FAKE_PS=$PSF HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=660 HC_FAKE_LS=$lsB run wait --state 멈춤 --timeout 0
t "배경: 작업 트리 아래 프로세스가 살아 있으면 쉬는 레인을 멈춤으로 안 알림" '[ $rc = 75 ] || { printf "%s\n" "$out"; cat "$T/err"; false; }'
{ echo " 4000     1 node agent-runner.js"; echo " 4001  4000 claude --model x"; echo " 6002     1 bash /w/app.wt/lane-b99/x"; } >"$PSF"
HC_TERMINAL_ID=tK HC_FAKE_PID=4000 HC_FAKE_PS=$PSF HC_FAKE_SCREEN=$GS HC_FAKE_STATUS_AGE=660 HC_FAKE_LS=$lsB run wait --state 멈춤 --timeout 0
t "배경: 그 프로세스가 끝났으면(이름만 비슷한 옆 작업 트리는 안 셈) 멈춤 done" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}tb9${tab}lane-b9${tab}멈춤${tab}-${tab}done turn 2" || { printf "%s\n" "$out"; cat "$T/err"; false; }'

# send --all(2026-11-17 지휘 「실수는 바로 코드로」): 지휘가 모든 레인에 알릴 마감 연장을 원격 레인에만 보내 맥 레인이 멈췄다.
# 이 지휘가 띄운(레인 기록 마지막 지휘, 없으면 hc parent) 살아 있는 레인 전부(원격 포함)에 보낸다. 감사(read·audit-*)는 --audits로만
SD=$T/.local/state/hc-lane; mkdir -p "$SD"; rm -f "$HCLOG.send" "$HCLOG.sendfail"
printf '%s\n' "tA1${tab}impl-a${tab}tC" "mini/tA2${tab}impl-remote${tab}tC" "tA3${tab}audit-x${tab}tC${tab}read" "tA4${tab}other-lane${tab}tO" \
  "tA5${tab}dead-lane${tab}tC" "tA6${tab}fill-lane${tab}-${tab}fill" "tA8${tab}moved${tab}tC" "tA8${tab}moved${tab}tO" >"$SD/lanes.tsv"
lsS='{"terminals":[{"address":"tA1","name":"impl-a","parent":"mac/tC","state":"live"},{"address":"mini/tA2","name":"impl-remote","parent":"mac/tC","state":"live"},
{"address":"tA3","name":"audit-x","parent":"mac/tC","state":"live"},{"address":"tA4","name":"other-lane","parent":"mac/tO","state":"live"},
{"address":"tA5","name":"dead-lane","parent":"mac/tC","state":"finished"},{"address":"tA6","name":"fill-lane","parent":"mac/tC","state":"live"},
{"address":"mini/tA7","name":"audit-y","parent":"mac/tC","state":"live"},{"address":"tA8","name":"moved","parent":"mac/tC","state":"live"},
{"address":"tA9","name":"no-rec","parent":"tC","state":"live"}]}'
HC_TERMINAL_ID=tC HC_FAKE_LS=$lsS run send --all "마감 07:30으로 연장"
sent() { cut -f1 "$HCLOG.send" 2>/dev/null | sort | tr '\n' ' '; }
t "send --all: 이 지휘의 살아 있는 구현 레인 전부(원격·기록 없는 parent·fill 포함), 감사·남의 지휘·끝난 레인은 뺌" '[ $rc = 0 ] && [ "$(sent)" = "mini/tA2 tA1 tA6 tA9 " ] || { echo "[$(sent)]"; printf "%s\n" "$out"; cat "$T/err"; false; }'
t "send --all: 글 그대로, 보낸 주소를 한 줄씩" 'grep -Fxq "tA1${tab}마감 07:30으로 연장" "$HCLOG.send" && printf "%s\n" "$out" | grep -Fq "보냄 mini/tA2 impl-remote" && printf "%s\n" "$out" | grep -Fq "보냄 4 · 실패 0"'
rm -f "$HCLOG.send"; HC_TERMINAL_ID=tC HC_FAKE_LS=$lsS run broadcast --audits "x"
t "broadcast --audits: 감사 레인(read 기록·audit- 이름)도 포함" '[ $rc = 0 ] && [ "$(sent)" = "mini/tA2 mini/tA7 tA1 tA3 tA6 tA9 " ] || { echo "[$(sent)]"; false; }'
rm -f "$HCLOG.send"; touch "$HCLOG.sendfail"; HC_TERMINAL_ID=tC HC_FAKE_LS=$lsS run send --all "x"; rm -f "$HCLOG.sendfail"
t "send --all: 보내기 실패는 한 줄씩 알리고 종료 1" '[ $rc = 1 ] && printf "%s\n" "$out" | grep -Fq "실패 tA1 impl-a" && printf "%s\n" "$out" | grep -Fq "보냄 0 · 실패 4"'
HC_TERMINAL_ID=tC HC_FAKE_LS_FAIL=1 run send --all "x"
t "send --all: hc ls 실패면 아무에게도 안 보내고 종료 1" '[ $rc = 1 ] && [ ! -s "$HCLOG.send" ] && grep -Fq "hc ls" "$T/err"'
HC_TERMINAL_ID=tC HC_FAKE_LS=$lsS run send "x"
t "send: --all 없으면 종료 2(일부에만 보내는 꼴은 없다)" '[ $rc = 2 ] && [ ! -s "$HCLOG.send" ]'
HC_FAKE_LS=$lsS run send --all "x"
t "send --all: 지휘 주소(HC_TERMINAL_ID·--from)가 없으면 종료 2" '[ $rc = 2 ] && [ ! -s "$HCLOG.send" ]'
HC_FAKE_LS=$lsS run send --all --from tO "x"
t "send --all --from: 그 지휘의 레인에만" '[ $rc = 0 ] && [ "$(sent)" = "tA4 tA8 " ] || { echo "[$(sent)]"; false; }'
rm -f "$HCLOG.send"; HC_TERMINAL_ID=tC HC_FAKE_LS=$lsS run send --all -- --all --audits 끝
t "send --all --: 뒤 낱말은 옵션 꼴이어도 글(감사 L4)" '[ $rc = 0 ] && grep -Fxq "tA1${tab}--all --audits 끝" "$HCLOG.send" && ! grep -q "^tA3" "$HCLOG.send" || { cat "$HCLOG.send"; false; }'
rm -f "$HCLOG.send" "$SD/lanes.tsv"

# lane-fetch-race: 같은 클론에서 다른 레인의 착지·fetch가 origin/main을 함께 고쳐 git fetch가 참조 잠금 경합으로 실패하면
# 짧게 쉬고 최대 3번 다시 한다(2026-11-18 워커 기계 실측). 다른 실패는 그대로 실패. 가짜 git이 fetch를 처음
# $GITRACE번 실패시킨다(메시지 $GITRACE_MSG). fetch 횟수는 $GITRACE.n
mkdir -p "$T/racebin"
cat >"$T/racebin/git" <<EOF
#!/bin/bash
for a; do
  [ "\$a" = fetch ] || continue
  echo f >>"\$GITRACE.n"
  c=\$(cat "\$GITRACE" 2>/dev/null || echo 0)
  [ "\$c" -gt 0 ] && { echo \$((c - 1)) >"\$GITRACE"; printf '%s\n' "\$GITRACE_MSG" >&2; exit 1; }
  break
done
exec $(command -v git) "\$@"
EOF
chmod +x "$T/racebin/git"
race() { echo "$1" >"$T/race"; rm -f "$T/race.n"; }
fn() { wc -l <"$T/race.n" 2>/dev/null | tr -d ' '; }
rmsg="error: cannot lock ref 'refs/remotes/origin/main': is at 6323151 but expected 45c382a"
export HC_LANE_FETCH_WAITS="0 0 0"
mk tfr
race 1
GITRACE=$T/race GITRACE_MSG=$rmsg PATH="$T/racebin:$PATH" run --name fr-start --repo "$R" -- true
t "fetch 경합: 시작 fetch가 잠금 경합이면 다시 해 레인을 띄운다" '[ $rc = 0 ] && [ -d "$R.wt/fr-start" ] && [ "$(fn)" = 2 ] && grep -Fq "다시" "$T/err" || { cat "$T/err"; fn; false; }'
race 1
GITRACE=$T/race GITRACE_MSG="error: unable to update local ref 'refs/remotes/origin/main'" PATH="$T/racebin:$PATH" run fetch --repo "$R"
t "fetch 경합: hc-lane fetch도 unable to update local ref면 다시" '[ $rc = 0 ] && [ "$(fn)" = 2 ] || { cat "$T/err"; fn; false; }'
race 1
GITRACE=$T/race GITRACE_MSG="error: 로컬 레퍼런스를 업데이트할 수 없습니다" PATH="$T/racebin:$PATH" run fetch --repo "$R"
t "fetch 경합: 한국어 git 메시지(로컬 레퍼런스를 업데이트할 수 없습니다)도 다시" '[ $rc = 0 ] && [ "$(fn)" = 2 ] || { cat "$T/err"; fn; false; }'
# 한국어 git 카탈로그(git 2.56.1 ko): "Unable to create '%s.lock': %s" → "'%s.lock'을 만들 수 없습니다: %s"
race 1
GITRACE=$T/race GITRACE_MSG="error: '$R/.git/refs/remotes/origin/main.lock'을 만들 수 없습니다: File exists." PATH="$T/racebin:$PATH" run fetch --repo "$R"
t "fetch 경합: 한국어 .lock 만들기 실패('….lock'을 만들 수 없습니다)도 다시" '[ $rc = 0 ] && [ "$(fn)" = 2 ] || { cat "$T/err"; fn; false; }'
race 1
GITRACE=$T/race GITRACE_MSG="fatal: could not read Username for 'https://github.com'" PATH="$T/racebin:$PATH" run --name fr-auth --repo "$R" -- true
t "fetch 경합: 다른 실패는 다시 하지 않고 그대로 실패" '[ $rc != 0 ] && [ ! -d "$R.wt/fr-auth" ] && [ "$(fn)" = 1 ] && grep -Fq "could not read Username" "$T/err" || { cat "$T/err"; fn; false; }'
race 9
GITRACE=$T/race GITRACE_MSG=$rmsg PATH="$T/racebin:$PATH" run --name fr-many --repo "$R" -- true
t "fetch 경합: 다시 하기는 3번까지(모두 4번), 그래도 실패면 실패" '[ $rc != 0 ] && [ ! -d "$R.wt/fr-many" ] && [ "$(fn)" = 4 ] || { cat "$T/err"; fn; false; }'
seed .claude/herdr-verify.sh 'exit 0'
mklane fr-land
seedp c2 c2
race 1
out=$(cd "$wt" && GITRACE=$T/race GITRACE_MSG=$rmsg PATH="$T/racebin:$PATH" "$LANE" land 2>"$T/err"); rc=$?
t "fetch 경합: land의 fetch도 다시 해 착지" '[ $rc = 0 ] && [ "$(otop)" = fr-land ] && [ "$(fn)" = 2 ] || { cat "$T/err"; fn; false; }'
race 1
GITRACE=$T/race GITRACE_MSG=$rmsg PATH="$T/racebin:$PATH" run clean --repo "$R" --branch fr-land
t "fetch 경합: clean의 fetch도 다시 해 정리" '[ $rc = 0 ] && [ ! -e "$R.wt/fr-land" ] && [ "$(fn)" = 2 ] || { cat "$T/err"; fn; false; }'
unset HC_LANE_FETCH_WAITS

# 메인 폴더 앞당김(2026-11-18 main-folder-ff: 워커 기계 메인 폴더가 이틀 동안 main보다 375~497커밋 뒤라 --read 감사가 옛
# CLAUDE.md·계약을 읽음): 레인·읽기 레인 띄울 때 fetch 뒤와 land 착지 뒤, 메인 폴더가 깨끗한 기본 브랜치일 때만 --ff-only
mhead() { git -C "$R" rev-parse HEAD; }
otip() { git -C "$D/origin.git" rev-parse "${1:-main}"; }
mk t-ff
run --name ff-ok --repo "$R" -- true
t "메인 폴더: 깨끗한 main이면 fetch 뒤 origin/main으로 앞당김" '[ $rc = 0 ] && [ "$(mhead)" = "$(otip)" ] && printf "%s" "$out" | grep -Fq "메인 폴더 앞당김" || { cat "$T/err"; false; }'
seed f1 f1
run --name ff-rd --read --repo "$R" -- true
t "메인 폴더: 읽기 레인도 fetch 뒤 앞당김" '[ $rc = 0 ] && [ "$(mhead)" = "$(otip)" ] && printf "%s" "$out" | grep -Fq "메인 폴더 앞당김" || { cat "$T/err"; false; }'
seed f2 f2
h0=$(mhead); echo dirty >>"$R/a"
run --name ff-dirty --repo "$R" -- true
t "메인 폴더: 추적 파일 변경이 있으면 건너뜀·변경 보존, 레인은 뜸" '[ $rc = 0 ] && [ "$(mhead)" = "$h0" ] && grep -Fxq dirty "$R/a" && grep -Fq "메인 폴더 앞당김 건너뜀" "$T/err" && pair --name ff-dirty'
git -C "$R" checkout -q -- a
git -C "$R" checkout -qb side
run --name ff-side --repo "$R" -- true
t "메인 폴더: 기본 브랜치가 아니면 건너뜀" '[ $rc = 0 ] && [ "$(mhead)" = "$h0" ] && [ "$(git -C "$R" symbolic-ref --short HEAD)" = side ] && grep -Fq "메인 폴더 앞당김 건너뜀" "$T/err"'
git -C "$R" checkout -q main
seed u from-origin
echo mine-untracked >"$R/u"
run --name ff-ut --repo "$R" -- true
t "메인 폴더: 추적 안 된 파일이 앞당김과 부딪히면 git이 거절 → 건너뜀·파일 보존" '[ $rc = 0 ] && [ "$(mhead)" = "$h0" ] && grep -Fxq mine-untracked "$R/u" && grep -Fq "메인 폴더 앞당김 건너뜀" "$T/err"'
rm -f "$R/u"; echo other >"$R/keep-untracked"
run --name ff-ut2 --on box --repo "$R" -- true
t "메인 폴더: 부딪히지 않는 추적 안 된 파일은 괜찮음(--on도 hc exec로 그 기계에서)" '[ $rc = 0 ] && [ "$(mhead)" = "$(otip)" ] && grep -Fxq other "$R/keep-untracked" && printf "%s" "$out" | grep -Fq "메인 폴더 앞당김 " && grep -Fq -- "--ff-only" "$HCLOG.exec"'
run --name ff-dry --dry-run --repo "$R" -- true
t "메인 폴더: dry-run은 할 일만 찍고 건드리지 않음" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fq "[dry-run] 메인 폴더 앞당김"'
seed .claude/herdr-verify.sh 'exit 0'
mklane ff-land
h1=$(mhead)
seedp f3 f3
lnd "$wt"
t "메인 폴더: land 착지 뒤 앞당김(착지 커밋까지)" '[ $rc = 0 ] && [ "$(otop)" = ff-land ] && [ "$(mhead)" = "$(otip)" ] && [ "$(mhead)" != "$h1" ] && printf "%s" "$out" | grep -Fq "메인 폴더 앞당김" || { cat "$T/err"; false; }'
echo dirty >>"$R/a"
mklane ff-land2
lnd "$wt"
t "메인 폴더: land 착지 뒤에도 더러우면 건너뜀(착지는 성공)" '[ $rc = 0 ] && [ "$(otop)" = ff-land2 ] && [ "$(mhead)" != "$(otip)" ] && grep -Fxq dirty "$R/a" && grep -Fq "메인 폴더 앞당김 건너뜀" "$T/err"'
git -C "$R" checkout -q -- a
seedp .claude/herdr-land.sh 'HERDR_LAND=1 git push -q origin HEAD:main && echo land-script-ok'
mklane ff-land3
lnd "$wt"
t "메인 폴더: herdr-land.sh 착지 뒤에도 앞당김" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fxq land-script-ok && [ "$(otop)" = ff-land3 ] && [ "$(mhead)" = "$(otip)" ] && printf "%s" "$out" | grep -Fq "메인 폴더 앞당김" || { cat "$T/err"; false; }'
mk t-ffr   # rebase 중인 메인 폴더(따로 된 클론 — 내 커밋을 지우지 않고 끝낸다)
seed a a2
(cd "$R" && echo mine >a && git commit -qm mine -- a && git fetch -q origin && ! git rebase -q origin/main >/dev/null 2>&1)
run --name ff-rb --repo "$R" -- true
t "메인 폴더: rebase 중이면 건너뜀(진행 중 표시 그대로)" '[ $rc = 0 ] && [ -d "$R/.git/rebase-merge" ] && grep -Fq "메인 폴더 앞당김 건너뜀" "$T/err" && grep -Fq "진행 중" "$T/err" || { echo "rc=$rc"; cat "$T/err"; false; }'
git -C "$R" rebase --abort
# 재감사 고침(audit-main-folder-ff): M1 무시된 파일 덮지 않음, M2 메인 폴더에서 일하는 세션이면 건너뜀, L1 빈 갈래, L4 skip-worktree
mk t-ffi
seed .gitignore .env
run --name ffi-0 --repo "$R" -- true
echo mine >"$R/.env"
(cd "$D/seed" && git pull -q --rebase origin main && git rm -q .gitignore && git commit -qm unignore && HERDR_LAND=1 git push -q origin HEAD:main)
seed .env upstream
h2=$(mhead)
run --name ffi-1 --repo "$R" -- true
t "메인 폴더(M1): 무시된 로컬 파일을 앞당김이 추적하게 되면 덮지 않고 건너뜀·파일 보존" '[ $rc = 0 ] && [ "$(mhead)" = "$h2" ] && grep -Fxq mine "$R/.env" && grep -Fq "메인 폴더 앞당김 건너뜀: git이 거절" "$T/err" || { cat "$T/err"; false; }'
rm -f "$R/.env"
seed m1 m1
h2=$(mhead)
ls1() { printf '{"terminals":[{"address":"%s","agent":"claude-code","cwd":"%s","state":"live","status":"%s","turn":3}]}' "$1" "$2" "$3"; }
HC_FAKE_LS=$(ls1 t5aa "$R/sub" working) run --name ffb-1 --repo "$R" -- true
t "메인 폴더(M2): 그 폴더(아래)를 cwd로 쓰는 working 터미널이 있으면 건너뜀(주소를 알림)" '[ $rc = 0 ] && [ "$(mhead)" = "$h2" ] && grep -Fq "메인 폴더 앞당김 건너뜀: 메인 폴더에서 일하는 세션 t5aa" "$T/err" || { cat "$T/err"; false; }'
HC_FAKE_LS=$(ls1 t5aa "$R" blocked) run --name ffb-2 --read --repo "$R" -- true
t "메인 폴더(M2): blocked도 일하는 중(읽기 레인)" '[ $rc = 0 ] && [ "$(mhead)" = "$h2" ] && grep -Fq "일하는 세션 t5aa" "$T/err"'
HC_FAKE_LS=$(ls1 t5aa "$R.wt/other" working) run --name ffb-3 --repo "$R" -- true
t "메인 폴더(M2): 레인 작업 트리(<저장소>.wt/…)의 working은 상관없음" '[ $rc = 0 ] && [ "$(mhead)" = "$(otip)" ] || { cat "$T/err"; false; }'
seed m2 m2
HC_FAKE_LS=$(ls1 t5aa "$R" idle) run --name ffb-4 --repo "$R" -- true
t "메인 폴더(M2): 쉬는(idle) 세션 밑에서는 앞당김" '[ $rc = 0 ] && [ "$(mhead)" = "$(otip)" ] || { cat "$T/err"; false; }'
seed m3 m3
HC_TERMINAL_ID=t5aa HC_FAKE_LS=$(ls1 t5aa "$R" working) run --name ffb-5 --repo "$R" -- true
t "메인 폴더(M2): 부른 터미널 자신은 빼고 앞당김" '[ $rc = 0 ] && [ "$(mhead)" = "$(otip)" ] || { cat "$T/err"; false; }'
seed m4 m4; h2=$(mhead)
HC_TERMINAL_ID=t5aa HC_FAKE_LS=$(ls1 box/t5aa "$R" working) run --name ffb-6 --on box --repo "$R" -- true
t "메인 폴더(M2): --on이면 그 기계 터미널(<기계>/<주소>)로 판정, 부른 쪽 주소와 같아도 뺌 없음" '[ $rc = 0 ] && [ "$(mhead)" = "$h2" ] && grep -Fq "일하는 세션 box/t5aa" "$T/err" || { cat "$T/err"; false; }'
HC_FAKE_LS_FAIL=1 run --name ffb-7 --repo "$R" -- true
t "메인 폴더(M2): hc ls 실패면 건너뜀(모르면 안 건드림)" '[ $rc = 0 ] && [ "$(mhead)" = "$h2" ] && grep -Fq "건너뜀: hc ls 실패" "$T/err"'
# 재감사 R-M1: 셸에서 실행기로 띄운 대화형 지휘는 hc ls에 status가 없다(agent zsh) — hc get foreground가 셸이 아니면 일하는 중
lsz='{"terminals":[{"address":"t5bb","agent":"zsh","cwd":"'"$R"'","state":"live","turn":9}]}'
fgj() { printf '{"address":"t5bb","foreground":{"argv":["%s"],"name":"%s","pid":%s},"pid":1045,"state":"live","status":null}' "$1" "$1" "$2"; }
HC_FAKE_LS=$lsz HC_FAKE_GET_ADDR=t5bb HC_FAKE_GET_BODY=$(fgj Python 12534) run --name ffz-1 --repo "$R" -- true
t "메인 폴더(R-M1): status 없는 터미널이 셸 위에서 무언가 돌리면(하단 줄 지휘) 건너뜀" '[ $rc = 0 ] && [ "$(mhead)" = "$h2" ] && grep -Fq "일하는 세션 t5bb" "$T/err" || { cat "$T/err"; false; }'
HC_FAKE_LS=$lsz HC_FAKE_GET_ADDR=t5bb HC_FAKE_GET_BODY='{"error":{"kind":"not_found"}}' run --name ffz-2 --repo "$R" -- true
t "메인 폴더(R-M1): status 없는 터미널을 판정 못 하면 일하는 중(건너뜀)" '[ $rc = 0 ] && [ "$(mhead)" = "$h2" ] && grep -Fq "일하는 세션 t5bb" "$T/err"'
HC_FAKE_LS='{"error":"bad"}' run --name ffz-3 --repo "$R" -- true
t "메인 폴더(R-L1): hc ls 꼴을 못 읽으면 건너뜀" '[ $rc = 0 ] && [ "$(mhead)" = "$h2" ] && grep -Fq "건너뜀: hc ls 실패" "$T/err"'
HC_FAKE_LS=$lsz HC_FAKE_GET_ADDR=t5bb HC_FAKE_GET_BODY=$(fgj zsh 1045) run --name ffz-4 --repo "$R" -- true
t "메인 폴더(R-M1): status 없는 터미널이 셸만 떠 있으면 앞당김" '[ $rc = 0 ] && [ "$(mhead)" = "$(otip)" ] || { cat "$T/err"; false; }'
seed m5 m5; h2=$(mhead)
git -C "$R" checkout -q --detach
run --name ffl-1 --repo "$R" -- true
t "메인 폴더(L1): 분리된 HEAD면 건너뜀" '[ $rc = 0 ] && [ "$(mhead)" = "$h2" ] && grep -Fq "분리된 HEAD" "$T/err"'
git -C "$R" checkout -q main
git -C "$R" update-index --skip-worktree a; echo local >"$R/a"
run --name ffl-2 --repo "$R" -- true
t "메인 폴더(L4): skip-worktree 표시 파일이 있으면 건너뜀·파일 보존" '[ $rc = 0 ] && [ "$(mhead)" = "$h2" ] && grep -Fxq local "$R/a" && grep -Fq skip-worktree "$T/err"'
git -C "$R" update-index --no-skip-worktree a; git -C "$R" checkout -q -- a
ou=$(git -C "$R" remote get-url origin)
git -C "$R" remote set-url origin "$T/없는 원격.git"
run --name ffl-3 --read --repo "$R" -- true
git -C "$R" remote set-url origin "$ou"
t "메인 폴더(L1): 읽기 레인 fetch 실패면 경고만, 레인은 뜸" '[ $rc = 0 ] && pair --name ffl-3 && grep -Fq "건너뜀: git fetch 실패" "$T/err"'
(cd "$R" && echo own >own && git add -- own && git commit -qm own -- own)
run --name ffl-4 --repo "$R" -- true
t "메인 폴더(L1): 로컬에만 있는 커밋이 있으면 건너뜀" '[ $rc = 0 ] && [ "$(git -C "$R" log -1 --format=%s)" = own ] && grep -Fq "origin/main 에 없는 커밋" "$T/err"'

echo "${n}개 중 실패 ${fails}개"
[ $fails = 0 ]
