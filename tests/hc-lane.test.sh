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
  # $HC_FAKE_STATUS_AGE초 전에 status가 바뀐 쉬는 터미널(wait 유예 시험)
  [ -z "${HC_FAKE_STATUS_AGE-}" ] || { echo "{\"address\":\"$2\",\"state\":\"live\",\"status\":{\"state\":\"done\"},\"status_ms\":$((($(date +%s) - HC_FAKE_STATUS_AGE) * 1000)),\"turn\":1}"; exit 0; }
  [ -z "${HC_FAKE_GET_SLEEP-}" ] || exec sleep "$HC_FAKE_GET_SLEEP"   # 응답 없는 코어(alarm이 이 프로세스를 끝내 고아가 안 남게 exec)
  [ -z "${HC_FAKE_GET_TURN-}" ] || { echo "{\"address\":\"$2\",\"state\":\"live\",\"status\":{\"app\":\"x\",\"msg\":\"m\",\"state\":\"working\"},\"turn\":$HC_FAKE_GET_TURN}"; exit 0; }
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
  [ "$2" = "${HC_FAKE_LIVE-}" ] && { echo "{\"address\":\"t1aa\",\"name\":\"$2\",\"state\":\"live\"}"; exit 0; }
  [ "$2" = "${HC_FAKE_ID-}" ] && { echo "{\"address\":\"$2\",\"name\":\"other\",\"state\":\"live\"}"; exit 0; }
  echo '{"error":{"kind":"not_found"}}' >&2; exit 3
fi
if [ "$1" = ls ]; then   # $HC_FAKE_LS(JSON)를 답한다. $HC_FAKE_LS_FAIL이면 실패. 인자는 $HCLOG.ls에 한 줄로
  echo "$*" >>"$HCLOG.ls"
  [ -z "${HC_FAKE_LS_FAIL-}" ] || exit 3
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
f=$HCLOG; [ "$1" = exec ] && f=$HCLOG.exec
{ echo "--- $PWD"; for a in "$@"; do echo "$a"; done; } >>"$f"
case $1 in
  run) [ -n "${HC_FAKE_RUN_FAIL-}" ] && { echo '{"error":{"kind":"name_taken"}}' >&2; exit 1; }
       echo '{"address":"t9zz","agent":"x","state":"live"}' ;;
  exec) on=; while [ "$1" != -- ]; do [ "$1" != --on ] || on=$2; shift; done; shift
        # $HC_FAKE_MACH_HOME가 있으면 기계마다 HOME을 <그 폴더>/<기계>로(폴더가 없으면 모르는 기계, 종료 4)
        if [ -n "$on" ] && [ -n "${HC_FAKE_MACH_HOME-}" ]; then
          [ -d "$HC_FAKE_MACH_HOME/$on" ] || { echo '{"error":{"kind":"machine_unknown"}}' >&2; exit 4; }
          HOME=$HC_FAKE_MACH_HOME/$on exec "$@"
        fi
        exec "$@" ;;
esac
EOF
chmod +x "$T/bin/hc"
ln -s "$LANE" "$T/bin/hc-lane"   # 착지 절차의 hc-lane clean이 이 사본을 쓰게
export PATH="$T/bin:$PATH" HCLOG="$T/hc.log" HC_LANE_POLL=0.1
unset HC_LANE HC_TERMINAL_ID HC_LANE_MACHINE XDG_STATE_HOME   # 레인 안에서 돌려도 물려받은 값이 시험을 바꾸지 않게
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
run --name lane-s --repo "$R" -- true
t "setup: 성공 시 종료 0" '[ $rc = 0 ]'
t "setup: 워크트리 안에서 실행(LANE_REPO=메인 폴더·LANE_* 받음, WT_MAIN·HERDR_BASE 없음, 로컬은 ~/.zshrc 안 읽음)" '[ "$(cat "$R.wt/lane-s/setup-ran" 2>/dev/null)" = "$R|lane-s|main||none|none" ]'
t "setup: 성공 뒤 hc run" 'has run'
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
undo=$(sed -n 's/.*지우려면: //p' "$T/err")
t "--on setup 실패: 지우는 명령이 hc exec --on box 로" '[ $rc != 0 ] && printf "%s" "$undo" | grep -Fq "hc exec --on box -- git -C" && printf "%s" "$undo" | grep -Fq "&& hc exec --on box -- git -C"'
(cd "$T" && eval "$undo") >/dev/null 2>&1
t "--on setup 실패: 그 명령으로 정리됨" '[ ! -e "$R.wt/lane-o2" ] && ! git -C "$R" show-ref -q --verify refs/heads/lane-o2'

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
undo=$(sed -n 's/.*지우려면: //p' "$T/err")
t "hc run 실패: 오류에 지우는 명령" '[ $rc != 0 ] && [ -n "$undo" ]'
(cd "$T" && eval "$undo") >/dev/null 2>&1
t "hc run 실패: 알려 준 명령으로 워크트리·브랜치가 지워짐" '[ ! -e "$R.wt/lane-v" ] && ! git -C "$R" show-ref -q --verify refs/heads/lane-v'
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

land c-left   # 오늘 상황: remove가 등록은 지웠고 무시 폴더(와 지워진 등록을 가리키는 .git 파일)만 남음
git -C "$R" worktree remove --force "$R.wt/c-left"
mkdir -p "$R.wt/c-left/.omc/state" && echo s >"$R.wt/c-left/.omc/state/s.json"
printf 'gitdir: %s\n' "$R/.git/worktrees/c-left" >"$R.wt/c-left/.git"
run clean --repo "$R" --branch c-left
t "clean: 메인 폴더에 그 .gitignore가 아직 없으면(뒤처짐) 남은 폴더는 멈추고 그대로(안전 쪽)" '[ $rc != 0 ] && [ -f "$R.wt/c-left/.omc/state/s.json" ] && git -C "$R" show-ref -q --verify refs/heads/c-left'
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
seed .claude/herdr-verify.sh 'echo "verify $(git log -1 --format=%s) HERDR_LAND=${HERDR_LAND-}" >>"$HOME/verify.log"'
rm -f "$T/verify.log"
lnd "$wt"
t "land: 검증이 있으면 rebase 뒤(그 HEAD로) 검증하고 push" '[ $rc = 0 ] && [ "$(otop)" = l-nov ] && [ "$(git -C "$D/origin.git" log -2 --format=%s main | tail -n 1)" = .claude/herdr-verify.sh ] && grep -Fxq "verify l-nov HERDR_LAND=" "$T/verify.log"'
t "land: 착지 줄 출력" 'printf "%s" "$out" | grep -Fq "hc-lane land: 착지"'
run --name l-vf --repo "$R" -- true; wt="$R.wt/l-vf"
(cd "$wt" && printf 'echo vfail; exit 9\n' >.claude/herdr-verify.sh && git commit -qm vf -- .claude/herdr-verify.sh)
lnd "$wt"
t "land: 검증이 실패하면 멈추고 push 안 함" '[ $rc = 1 ] && [ "$(otop)" = l-nov ] && printf "%s" "$out" | grep -Fq vfail && grep -Fq "검증 실패" "$T/err"'
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
git -C "$R" checkout -q -b lane-br
out=$(cd "$R" && "$LANE" report 중간 2>"$T/err"); rc=$?
t "report: --name·\$HC_LANE 없으면 작업 트리 가지 이름" '[ $rc = 0 ] && [ "$(tail -n 1 "$EV" | cut -f4)" = lane-br ]'
git -C "$R" checkout -q main
out=$(cd "$R" && HC_LANE=lane-h "$LANE" report 중간 2>"$T/err")
t "report: \$HC_LANE이 가지 이름보다 앞" '[ "$(tail -n 1 "$EV" | cut -f4)" = lane-h ]'
t "report: 기계 이름 기본은 hostname" '[ "$(tail -n 1 "$EV" | cut -f2)" = "$(hostname -s)" ]'

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

# 멈춤: report 없이 끝났거나 막힌 레인 터미널(hc-lane이 띄운 것 = 라벨 hc-lane=1)
ev 2026-11-16T02:10:00Z here t43 lane-landed 착지 eeee5555 끝 >>"$EV"
run wait --timeout 0   # 위 착지 줄을 먼저 본다
export HC_FAKE_LS='{"terminals":[{"address":"t40","name":"lane-dead","state":"finished"},{"address":"m1/t41","name":"lane-blk","state":"live","status":"blocked"},{"address":"t42","name":"lane-ok","state":"live","status":"working"},{"address":"t43","name":"lane-landed","state":"finished"}]}'
run wait --timeout 0
t "wait 멈춤: --all 없으면 이 기계 레인만" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "${tab}t40${tab}lane-dead${tab}멈춤${tab}-${tab}finished" && ! printf "%s" "$out" | grep -Fq lane-blk || { printf "%s\n" "$out"; false; }'
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
t "멈춤: 같은 주소의 옛 레인 착지는 새 레인을 면제하지 않음" 'printf "%s\n" "$out" | grep -Fq "${tab}t60${tab}new-lane${tab}멈춤${tab}-${tab}finished"'
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
HC_FAKE_GET_TURN=7 HC_FAKE_GET_SLEEP=10 HC_TERMINAL_ID=t82 HC_LANE=lane-slow run report 중간 - 느린 코어
t "report: hc get이 응답하지 않으면 3초 안에 turn -로 적음" '[ $rc = 0 ] && [ "$(tail -n 1 "$EV" | cut -f4,8)" = "lane-slow${tab}-" ]'
t "report: 느린 hc get을 끊은 뒤 남은 프로세스 없음" '[ -z "$(pgrep -f "sleep 10")" ]'
# 재감사 낮음 2: 이번 바퀴에 못 읽은 기계의 레인은 멈춤으로 보지 않는다(그 기계 사건을 모르니)
mkdir -p "$T/mh2/m1"
HC_FAKE_MACH_HOME=$T/mh2 HC_FAKE_MACHINES='{"machines":[{"name":"m1","state":"connected"},{"name":"m3","state":"connected"},{"name":"m4","state":"disconnected"}]}' \
  HC_FAKE_LS='{"terminals":[{"address":"m1/t91","name":"lane-r1","state":"live","status":"done","turn":2},{"address":"m3/t92","name":"lane-r3","state":"live","status":"done","turn":2},{"address":"m4/t93","name":"lane-r4","state":"finished"}]}' \
  run wait --all --timeout 0
t "멈춤: 읽은 기계 레인은 알리고, 모름 기계 레인은 건너뜀" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -Fq "m1/t91${tab}lane-r1${tab}멈춤" && ! printf "%s\n" "$out" | grep -Fq lane-r3 && ! printf "%s\n" "$out" | grep -Fq lane-r4 || { printf "%s\n" "$out"; cat "$T/err"; false; }'

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

echo "${n}개 중 실패 ${fails}개"
[ $fails = 0 ]
