#!/bin/bash
# hc-sweep 디스크 쓸기(lib/hc-sweep/disk.py): 가짜 $TMPDIR에 오래된·새·열린·다른 이름·링크·쓰기 막힌 항목을 두고
# bin/hc-sweep 한 바퀴(가짜 hc — 터미널 없음)가 정리 대상만 지우는지, 디스크 부족 알림(hc-lane report → 시험 HOME의
# events.tsv)이 같은 상태에서 한 번만 나가고 회복 뒤 다시 나가는지 본다. 실행: bash tests/hc-sweep-disk.test.sh
set -u
here=$(cd "$(dirname "$0")" && pwd)
SWEEP=$here/../bin/hc-sweep
T=$(mktemp -d "${TMPDIR:-/tmp}/hcsweep-disk-test.XXXXXX") && T=$(cd "$T" && pwd -P) || exit 1
. "$here/isolate.sh" || exit 1
unset HC_TERMINAL_ID
pid=
cleanup() { [ -n "$pid" ] && kill "$pid" 2>/dev/null; chmod -R u+w "$T" 2>/dev/null; rm -rf "$T"; }
trap cleanup EXIT
fail=0
ok() { echo "ok - $*"; }
bad() { echo "FAIL - $*"; fail=1; }

mkdir -p "$T/bin" "$T/badbin" "$T/home" "$T/cfg" "$T/tmp" "$T/outside/keep"
printf '#!/bin/sh\n[ "$1" = ls ] && { echo %s; exit 0; }\nexit 1\n' "'{\"terminals\": []}'" >"$T/bin/hc"
printf '#!/bin/sh\nexit 1\n' >"$T/badbin/lsof"
chmod +x "$T/bin/hc" "$T/badbin/lsof"
export PATH="$T/bin:$PATH" HOME="$T/home" HERDR_TOOLS_CONFIG="$T/cfg" TMPDIR="$T/tmp"
cfg() { printf '%s\n' "$@" >"$T/cfg/config"; }
sweep() { out=$(python3 "$SWEEP" --only-under "$T" "$@" 2>&1); }
EV=$T/home/.local/state/hc-lane/events.tsv
LOG=$T/home/.claude-tasks/closed.log
old=$(date -v-5H +%Y%m%d%H%M.%S)
age() { find "$@" -exec touch -h -t "$old" {} +; }

X=$T/tmp
mkdir -p "$X/go-build111/b001" "$X/TestFoo222/001" "$X/TestRO333/001/mod" "$X/go-buildnew/b001" \
  "$X/TestOpen444/001" "$X/other-old" "$X/TestMixed555/001" "$X/node-compile-cache"
for d in go-build111/b001 TestFoo222/001 TestRO333/001/mod go-buildnew/b001 TestOpen444/001 other-old TestMixed555/001 node-compile-cache; do
  head -c 4096 /dev/zero >"$X/$d/f"
done
printf x >"$X/TestFile.txt"
ln -s "$T/outside/keep" "$X/go-buildlink"
age "$X/go-build111" "$X/TestFoo222" "$X/TestRO333" "$X/TestOpen444" "$X/other-old" "$X/TestMixed555" \
  "$X/node-compile-cache" "$X/TestFile.txt" "$X/go-buildlink" "$T/outside"
chmod 0555 "$X/TestRO333/001/mod" "$X/TestRO333/001" "$X/TestOpen444/001"   # go 모듈 캐시 꼴(쓰기 막힘)
touch "$X/TestMixed555/001/f"                          # 안쪽 하나만 새로움
(cd "$X/TestOpen444/001" && exec sleep 120) &
pid=$!
sleep 0.3
for d in TestOpen444 TestRO333; do age "$X/$d"; done   # 위 chmod·cd가 바꾼 시각을 다시 오래되게

# lsof 실패면 아무것도 안 지움
cfg "sweep.disk_free_gb 0"
out=$(PATH="$T/badbin:$PATH" python3 "$SWEEP" --only-under "$T" 2>&1)
[ -d "$X/go-build111" ] && [ -d "$X/TestFoo222" ] && case $out in *"lsof 실패"*) true ;; *) false ;; esac &&
  ok "lsof 실패면 건너뜀" || bad "lsof 실패인데 지움 또는 안내 없음: $out"

# 끔(sweep.tmp_hours 0), 숫자 아닌 값은 그 쓸기만 끔(알림은 돈다)
cfg "sweep.tmp_hours 0" "sweep.disk_free_gb 0"
sweep
[ -d "$X/go-build111" ] && ok "tmp_hours 0이면 끔" || bad "tmp_hours 0인데 지움: $out"
cfg "sweep.tmp_hours 3h" "sweep.disk_free_gb 1000000000"
sweep --dry-run
[ -d "$X/go-build111" ] && case $out in *"sweep.tmp_hours 값이 숫자가 아님"*"디스크 부족 알림 (보낼 것)"*) true ;; *) false ;; esac &&
  ok "숫자 아닌 tmp_hours는 그 쓸기만 끄고 알림은 돈다" || bad "숫자 아닌 값: $out"

# dry-run은 찍기만
cfg "sweep.disk_free_gb 0"
sweep --dry-run
[ -d "$X/go-build111" ] && case $out in *"지울 임시: $X/go-build111"*) true ;; *) false ;; esac &&
  ok "dry-run은 찍기만" || bad "dry-run: $out"

sweep
gone=1; for d in go-build111 TestFoo222 TestRO333; do [ -e "$X/$d" ] && { gone=0; bad "안 지움 $d"; }; done
[ $gone = 1 ] && ok "오래되고 안 열린 go-build*·Test*(쓰기 막힌 것 포함) 지움"
kept=1
for d in go-buildnew TestOpen444 other-old TestMixed555 node-compile-cache TestFile.txt go-buildlink outside/keep; do
  p=$X/$d; [ "$d" = outside/keep ] && p=$T/outside/keep
  [ -e "$p" ] || { kept=0; bad "지우면 안 되는 것을 지움 $d"; }
done
[ $kept = 1 ] && ok "새·열린·안쪽이 새·다른 이름·파일·링크(와 그 대상)는 남김"
[ "$(stat -f %Lp "$X/TestOpen444/001")" = 555 ] && ok "남긴 열린 폴더의 권한은 그대로" || bad "열린 폴더 권한이 바뀜"
grep -q '임시 찌꺼기 지움(3.0시간 넘음).*3개' "$LOG" 2>/dev/null && ok "closed.log에 한 줄(3개)" || bad "closed.log: $(cat "$LOG" 2>/dev/null)"
[ ! -s "$EV" ] && ok "문턱 0이면 알림 없음" || bad "끈 알림이 나감: $(cat "$EV")"

# 디스크 부족 알림: 문턱을 아주 크게 → 부족. 두 바퀴에도 한 번만, dry-run은 안 보냄
cfg "sweep.disk_free_gb 1000000000"
sweep --dry-run
[ ! -s "$EV" ] && case $out in *"보낼 것"*) true ;; *) false ;; esac && ok "dry-run은 알림을 찍기만" || bad "dry-run 알림: $out"
sweep; sweep
n=$(grep -c 'hc-disk-low' "$EV" 2>/dev/null)
[ "$n" = 1 ] && ok "같은 상태에서 한 번만 알림" || bad "알림 수 $n: $(cat "$EV" 2>/dev/null)"
line=$(grep 'hc-disk-low' "$EV" | tail -n 1)
case $line in *"	중간	"*"디스크 여유"*"문턱 1000000000"*"~/Library/Caches/go-build 없음"*"~/.cache/go-build 없음"*) ok "알림 줄: 중간·여유·세 곳 크기" ;;
  *) bad "알림 줄 꼴: $line" ;; esac
# 회복(여유 ≥ 문턱의 1.5배) 뒤 다시 부족이면 또 한 번
cfg "sweep.disk_free_gb 0.000001"
sweep
cfg "sweep.disk_free_gb 1000000000"
sweep
n=$(grep -c 'hc-disk-low' "$EV" 2>/dev/null)
[ "$n" = 2 ] && ok "회복 뒤 다시 부족이면 다시 알림" || bad "회복 뒤 알림 수 $n"

# launchd 작업과 같은 PATH(/usr/sbin 없음)에서도 lsof를 찾는다(감사 중간 1): 오래된 폴더는 지우고 열린 폴더는 남긴다
cfg "sweep.disk_free_gb 0"
mkdir -p "$X/go-buildlaunchd/b001"; age "$X/go-buildlaunchd"
py=$(command -v python3)
out=$(env PATH="$T/bin:/usr/bin:/bin" "$py" "$SWEEP" --only-under "$T" 2>&1)
[ ! -e "$X/go-buildlaunchd" ] && [ -d "$X/TestOpen444" ] && ok "launchd PATH에서도 lsof로 판정해 지움" ||
  bad "launchd PATH: $out"

kill "$pid" 2>/dev/null; wait "$pid" 2>/dev/null; pid=
exit $fail
