#!/bin/bash
# hc-lane land: 가지의 .claude/herdr-land.sh가 origin/<기본>보다 낡았으면 먼저 rebase해 새 판으로 착지하는지(2026-11-20 land-stale-script —
# 한 레인이 문서 조각 접기 전 판 스크립트로 착지해 조각이 main에 남았다). 임시 bare origin + clone, 가짜 hc(ls만).
# 실행: bash tests/hc-lane-stale-land.test.sh   (다른 사본은 HC_LANE_BIN=<경로>)
set -u
here=$(cd "$(dirname "$0")" && pwd)
LANE=${HC_LANE_BIN:-$here/../bin/hc-lane}
T=$(mktemp -d "${TMPDIR:-/tmp}/hc-lane-stale.XXXXXX") && T=$(cd "$T" && pwd -P) || exit 1
trap 'rm -rf "$T"' EXIT
. "$here/isolate.sh"
mkdir -p "$T/bin"
printf '#!/bin/bash\n[ "$1" = ls ] && echo %s\nexit 0\n' "'{\"terminals\":[]}'" >"$T/bin/hc"; chmod +x "$T/bin/hc"
export PATH="$T/bin:$PATH" HOME="$T" GIT_CONFIG_NOSYSTEM=1 \
  GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
unset HC_LANE HC_TERMINAL_ID XDG_STATE_HOME HC_LANE_OWNER HC_LANE_COORD HC_LANE_FILL
n=0 fails=0
t() { n=$((n + 1)); if eval "$2"; then echo "ok $n - $1"; else fails=$((fails + 1)); echo "FAIL $n - $1"; cat "$T/err"; fi; }
# 착지 스크립트 판: 이름을 찍고 rebase·push(이름이 「old」면 push 없이 0 — 옛 판이 무언가를 빠뜨린 꼴)
LAND_NEW='git fetch -q origin && git rebase -q "origin/$1" && HERDR_LAND=1 git push -q origin "HEAD:$1" && echo land-new'
LAND_OLD='echo land-old'

# mk <이름>: origin(bare) + 시드 + 레인 클론(가지 lane, 커밋 하나)
mk() {
  D=$T/$1
  git init -q --bare -b main "$D/o.git"
  git clone -q "$D/o.git" "$D/s" 2>/dev/null
}
sd() { (cd "$D/s" && git pull -q --rebase origin main 2>/dev/null; mkdir -p "$(dirname "$1")" && printf '%s\n' "$2" >"$1" && git add -- "$1" && git commit -qm "$1" && git push -q origin HEAD:main); }
lane() { git clone -q "$D/o.git" "$D/l" && (cd "$D/l" && git checkout -qb lane && printf '%s\n' "${1:-x}" >"${2:-x.txt}" && git add -A && git commit -qm lane); }
lnd() { out=$(cd "$D/l" && "$LANE" land main 2>"$T/err"); rc=$?; }
otree() { git -C "$D/o.git" log --format=%s main; }

# 1. 낡은 판: 가지는 옛 판, origin에 새 판 → 먼저 rebase하고 새 판으로 착지
mk a; sd .claude/herdr-land.sh "$LAND_OLD"; lane
sd .claude/herdr-land.sh "$LAND_NEW"
lnd
t "낡은 가지: 새 판 스크립트로 착지(옛 판을 돌리지 않음)" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fxq land-new && ! printf "%s" "$out" | grep -Fq land-old && otree | grep -Fxq lane'
t "낡은 가지: 먼저 rebase한다고 한 줄 찍음" 'grep -Fq "가 origin/main 보다 낡음 → 먼저 rebase" "$T/err"'

# 2. 가지가 스크립트를 스스로 바꿈 → 그 판 그대로(먼저 rebase 없음)
mk b; sd .claude/herdr-land.sh "$LAND_NEW"; lane 'echo land-lane' .claude/herdr-land.sh
sd y y
lnd
t "스크립트를 바꾼 가지: 가지 판을 돌림" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fxq land-lane && ! grep -Fq "먼저 rebase" "$T/err" && ! otree | grep -Fxq lane'

# 3. 스크립트는 그대로, origin만 앞섬 → 먼저 rebase 없이 가지 판(=origin 판)을 돌림
mk c; sd .claude/herdr-land.sh "$LAND_NEW"; lane
sd y y
lnd
t "스크립트가 같으면: 먼저 rebase 없이 착지" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fxq land-new && ! grep -Fq "먼저 rebase" "$T/err" && otree | grep -Fxq lane'

# 4. 가지에 없던 스크립트를 origin이 새로 둠 → 먼저 rebase해 그 스크립트로
mk d; sd .claude/herdr-verify.sh 'echo verify-ran'; lane
sd .claude/herdr-land.sh "$LAND_NEW"
lnd
t "가지에 없던 스크립트: 먼저 rebase해 새 스크립트로 착지" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fxq land-new && ! printf "%s" "$out" | grep -Fq verify-ran && otree | grep -Fxq lane'

# 5. 먼저 rebase가 충돌 → 종료 3, rebase는 멈춘 채(버리지 않음), 옛 판은 돌지 않음
mk e; sd .claude/herdr-land.sh "$LAND_OLD"; sd x.txt base; lane mine
sd x.txt theirs; sd .claude/herdr-land.sh "$LAND_NEW"
lnd
t "먼저 rebase 충돌: 종료 3, rebase 멈춘 채, 안내 줄" '[ $rc = 3 ] && git -C "$D/l" rev-parse -q --verify REBASE_HEAD >/dev/null && grep -Fq "git rebase --continue 한 뒤 착지 줄(hc-lane land main)을 다시" "$T/err" && ! printf "%s" "$out" | grep -Fq land-old'

# 6. 커밋 안 된 변경이 있으면 먼저 rebase하지 않고 멈춤(옛 판도 안 돌림)
mk f; sd .claude/herdr-land.sh "$LAND_OLD"; lane
sd .claude/herdr-land.sh "$LAND_NEW"
echo dirty >>"$D/l/x.txt"
lnd
t "커밋 안 된 변경: 멈춤, 옛 판 안 돌림" '[ $rc = 1 ] && grep -Fq "커밋 안 된 변경" "$T/err" && ! printf "%s" "$out" | grep -Fq land-old'

# 7. 병합기 안 착지(HERDR_MERGER=1): 4와 같은 꼴이면 먼저 rebase가 들인 스크립트에 맡기지 않고 종료 7(병합기가 반송)
#    — 감사 7c010d9 중간 1: 맡기면 큐 lease(HERDR_LAND_PUSH_EXTRA)를 모르는 스크립트가 main을 올렸다
mk g; sd .claude/herdr-verify.sh 'echo verify-ran'; lane
sd .claude/herdr-land.sh "$LAND_NEW"
out=$(cd "$D/l" && HERDR_MERGER=1 HERDR_LAND_PUSH_EXTRA=':refs/merge-queue/x' "$LANE" land main 2>"$T/err"); rc=$?
t "병합기 안 착지: rebase로 들인 전용 착지 스크립트는 안 돌리고 종료 7, main 그대로" '[ $rc = 7 ] && ! printf "%s" "$out" | grep -Fq land-new && grep -Fq "전용 착지 스크립트" "$T/err" && ! otree | grep -Fxq lane'

echo "1..$n (실패 $fails)"
[ $fails = 0 ]
