#!/bin/bash
# ovis-split 시험: --self-test, 가짜 hc(PATH 앞)로 --main(라벨 없는 지휘·이미 붙은 지휘)·--add·--drop 한 바퀴씩 JSON 출력과 hc label 인자.
# 실제 hc·Ovis는 쓰지 않는다. 기본은 Go 판, OVIS_SPLIT_IMPL=bash면 셸 판(tests/ovis-split-bash.test.sh) — 두 판이 같은 기대값을 지난다.
# 실행: bash tests/ovis-split.test.sh
set -u
here=$(cd "$(dirname "$0")/.." && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/ovis-split-test.XXXXXX") && T=$(cd "$T" && pwd -P) || exit 1
trap 'rm -rf "$T"' EXIT
mkdir -p "$T/bin" "$T/home"
PY=$(command -v python3) || { echo "python3 없음"; exit 1; }
ln -s "$PY" "$T/bin/python3"
cat >"$T/bin/hc" <<'EOF'
#!/bin/bash
printf '%s\n' "$*" >>"$FAKE_HC_LOG"
[ -z "${FAKE_HC_FAIL-}" ] || { echo "hc 꺼짐" >&2; exit 4; }
case "$*" in
  "get --json t9")
    if [ "$FAKE_MAIN" = placed ]; then echo '{"address": "t9", "cwd": "/r/proj", "labels": {"herdr.space": "sp9", "herdr.tab": "tb9", "ovis.workspace": "ws9"}}'
    else echo '{"address": "t9", "cwd": "/r/proj", "labels": {}}'; fi ;;
  "get --json t1") echo '{"address": "t1", "labels": {"herdr.space": "sp1"}}' ;;
  "get --json t5") echo '{"address": "t5", "parent": "t9", "name": "lane-a"}' ;;
  "get --json t6") echo '{"address": "t6", "parent": "t9", "labels": {"where": "proj / 1"}}' ;;
  "get --json t7") echo '{"address": "t7", "parent": "t8"}' ;;
  "ls --json") echo '{"terminals": [
    {"address": "w1/t2", "cwd": "/r/proj", "where": "proj / 1", "state": "live"},
    {"address": "t1", "cwd": "/r/proj", "where": "proj / 1", "state": "live"},
    {"address": "t9", "cwd": "/r/proj", "state": "live"},
    {"address": "t5", "parent": "t9", "name": "lane-a", "state": "live"},
    {"address": "t4", "parent": "t9", "state": "live"},
    {"address": "t8", "parent": "t9", "name": "old", "state": "done"}]}' ;;
  "ls --json --label ovis.workspace=proj") echo '{"terminals": []}' ;;
  label\ *) ;;
  *) echo "모르는 hc 인자: $*" >&2; exit 9 ;;
esac
EOF
chmod +x "$T/bin/hc"
IMPL=${OVIS_SPLIT_IMPL:+OVIS_SPLIT_IMPL=$OVIS_SPLIT_IMPL}
os() { : >"$T/hc.log"; env -i HOME="$T/home" PATH="$T/bin:/usr/bin:/bin" FAKE_HC_LOG="$T/hc.log" FAKE_MAIN="${FAKE_MAIN-bare}" \
  ${FAKE_HC_FAIL:+FAKE_HC_FAIL=1} $IMPL "$here/bin/ovis-split" "$@" 2>"$T/err"; }
U='s/ovis-[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}/ovis-UUID/g'
n=0 fails=0
t() { n=$((n + 1)); if [ "$2" = "$3" ]; then echo "ok $n - $1"; else fails=$((fails + 1)); echo "FAIL $n - $1: [$2] != [$3] err=[$(cat "$T/err")]"; fi; }

out=$(os --self-test); rc=$?
t "--self-test는 ok" "$rc $out" "0 ok"
out=$(os --main t9); rc=$?
t "--main(라벨 없음): 같은 폴더 앱 칸 공간, 새 탭, 이름 붙은 산 자식" "$rc $(printf %s "$out" | sed -E "$U")" '0 {"main": "t9", "space": "sp1", "tab": "ovis-UUID", "added": ["t5"]}'
t "--main(라벨 없음): hc 부른 순서·label 인자" "$(sed -E "$U" "$T/hc.log")" "get --json t9
ls --json
ls --json --label ovis.workspace=proj
get --json t1
label t9 herdr.space=sp1 herdr.tab=ovis-UUID ovis.workspace=proj ovis.tab=지휘 t9
label t5 herdr.space=sp1 herdr.tab= ovis.workspace=proj ovis.tab=lane-a · t9"
out=$(FAKE_MAIN=placed os --main t9 --add t5 t42); rc=$?
t "--add(이미 붙은 지휘): 지휘 라벨 그대로, 준 worker만" "$rc $out" '0 {"main": "t9", "space": "sp9", "tab": "tb9", "added": ["t5", "t42"]}'
t "--add: label 인자(살아 있지 않은 worker는 주소로 이름)" "$(grep '^label' "$T/hc.log")" "label t5 herdr.space=sp9 herdr.tab= ovis.workspace=ws9 ovis.tab=lane-a · t9
label t42 herdr.space=sp9 herdr.tab= ovis.workspace=ws9 ovis.tab=t42 · t9"
out=$(os --main t9 --drop t5 t7 t6); rc=$?
t "--drop: 자식만 빼고 JSON" "$rc $out" '0 {"main": "t9", "dropped": ["t5"]}'
t "--drop: 라벨 넷을 비움" "$(grep '^label' "$T/hc.log")" "label t5 herdr.space= herdr.tab= ovis.workspace= ovis.tab="
t "--drop: 남의 자식·앱 칸은 건너뜀 알림" "$(cat "$T/err")" "건너뜀 t7: 지휘나 그 자식이 아니거나 Herdr 앱 칸이다
건너뜀 t6: 지휘나 그 자식이 아니거나 Herdr 앱 칸이다"
out=$(os); rc=$?
t "인자 없음: usage·종료 2" "$rc|$(cat "$T/err")" "2|usage: ovis-split [-h] [--main MAIN] [--add TERMINAL [TERMINAL ...]]
                  [--drop TERMINAL [TERMINAL ...]] [--self-test]
ovis-split: error: --main이 필요하다"
out=$(os --main t9 --add); rc=$?
t "--add 값 없음: 종료 2" "$rc|$(tail -n 1 "$T/err")" "2|ovis-split: error: argument --add: expected at least one argument"
out=$(FAKE_HC_FAIL=1 os --main t9); rc=$?
t "hc 실패: 종료 1·라벨 안 붙임" "$rc $out $(grep -c '^label' "$T/hc.log")" "1  0"

echo "${n}개 중 실패 ${fails}개"
[ $fails = 0 ]
