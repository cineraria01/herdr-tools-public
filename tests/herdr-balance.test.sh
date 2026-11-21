#!/bin/bash
# herdr-balance 시험: balance.remote 읽기(설정 있음·파일 없음·키 없음·도우미 없음), 가짜 herdr-app(PATH 앞, 상태는 임시 JSON)로
# 한 탭 맞춤(--add·--drop·HUD 안 건드림·포커스 되돌림·상태 파일)·오류 꼴·--watch 한 바퀴. 실제 herdr-app·설정은 쓰지 않는다.
# 기본은 Go 판, HERDR_BALANCE_IMPL=bash면 셸 판(tests/herdr-balance-bash.test.sh) — 두 판이 같은 기대값을 지난다.
# 실행: bash tests/herdr-balance.test.sh
set -u
here=$(cd "$(dirname "$0")/.." && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/hb-test.XXXXXX") && T=$(cd "$T" && pwd -P) || exit 1
wpid=
trap '[ -z "$wpid" ] || { kill "$wpid" 2>/dev/null; wait "$wpid" 2>/dev/null; }; rm -rf "$T"' EXIT
mkdir -p "$T/bin" "$T/home" "$T/conf" "$T/py" "$T/fake"
ln -s "$here/bin/herdr-tools-config" "$T/bin/herdr-tools-config"
PY=$(command -v python3) || { echo "python3 없음"; exit 1; }
ln -s "$PY" "$T/py/python3"   # 셸 판이 부를 python3(PATH를 좁혀도 — 도우미를 찾는 길은 그대로 시험)
IMPL=${HERDR_BALANCE_IMPL:+HERDR_BALANCE_IMPL=$HERDR_BALANCE_IMPL}
hb() { env -i HOME="$T/home" PATH="$1" HERDR_TOOLS_CONFIG="$2" FAKE_HA_STATE="$T/state.json" FAKE_HA_LOG="$T/ha.log" \
  ${FAKE_HA_FAIL:+FAKE_HA_FAIL=$FAKE_HA_FAIL} $IMPL "$here/bin/herdr-balance" "${@:3}"; }
# remote <PATH> <설정 폴더>: remote_prefixes()의 결과를 「a|b」로, 경고는 $T/err
remote() { hb "$T/py:$1" "$2" --print-remote 2>"$T/err"; }
n=0 fails=0
t() { n=$((n + 1)); if [ "$2" = "$3" ]; then echo "ok $n - $1"; else fails=$((fails + 1)); echo "FAIL $n - $1: [$2] != [$3] err=[$(cat "$T/err")]"; fi; }

t "예시 설정: 목록" "$(remote "$T/bin:/usr/bin:/bin" "$here/config.example")" "worker-a/|worker-b/"
t "예시 설정: 경고 없음" "$(cat "$T/err")" ""
t "설정 파일 없음: 빈 목록" "$(remote "$T/bin:/usr/bin:/bin" "$T/conf")" ""
t "설정 파일 없음: 경고 한 줄" "$(grep -c '설정 파일 없음' "$T/err")" 1
printf 'sync.prebuild true\n' >"$T/conf/config"
t "키만 없음: 빈 목록" "$(remote "$T/bin:/usr/bin:/bin" "$T/conf")" ""
t "키만 없음: 경고 없음" "$(cat "$T/err")" ""
printf 'balance.remote a/\nbalance.remote b/\n' >"$T/conf/config"
t "같은 키 두 번: 빈 목록" "$(remote "$T/bin:/usr/bin:/bin" "$T/conf")" ""
t "같은 키 두 번: 경고 한 줄" "$(grep -c 'balance.remote 못 읽음' "$T/err")" 1
t "도우미가 PATH·~/.local/bin에 없음: 빈 목록" "$(remote "/usr/bin:/bin" "$here/config.example")" ""
t "도우미 없음: 경고 한 줄" "$(grep -c 'herdr-tools-config 못 부름' "$T/err")" 1
mkdir -p "$T/home/.local/bin" && ln -s "$here/bin/herdr-tools-config" "$T/home/.local/bin/herdr-tools-config"
t "PATH에 없어도 ~/.local/bin의 도우미(launchd)" "$(remote "/usr/bin:/bin" "$here/config.example")" "worker-a/|worker-b/"

# 가짜 herdr-app: ls·status·do(terminal.park·pane.focus·terminal.keep). keep은 포커스된 칸 바로 뒤에 새 칸으로 넣는다.
# 부른 인자는 한 줄씩 ha.log. FAKE_HA_FAIL=1이면 종료 3, do 인자 JSON의 terminal이 err_on과 같으면 {"error": "nope"}
cat >"$T/fake/herdr-app" <<'EOF'
#!/usr/bin/env python3
import json, os, sys
a = sys.argv[1:]
with open(os.environ["FAKE_HA_LOG"], "a") as f:
    f.write(" ".join(a) + "\n")
if os.environ.get("FAKE_HA_FAIL"):
    print("boom", file=sys.stderr); sys.exit(3)
p = os.environ["FAKE_HA_STATE"]; st = json.load(open(p))
def find(key, val):
    for tab in st["tabs"]:
        for i, x in enumerate(tab["terminals"]):
            if x[key] == val:
                return tab, i
    return None, None
if a[0] == "ls":
    print(json.dumps({"spaces": [{"name": "s", "tabs": st["tabs"]}]}))
elif a[0] == "status":
    print(json.dumps({"focused_pane": st["focused"]}))
elif a[0] == "do":
    kw = json.loads(a[2])
    if kw.get("terminal") and kw.get("terminal") == st.get("err_on"):
        print(json.dumps({"error": "nope"})); sys.exit(0)
    if a[1] == "terminal.park":
        tab, i = find("terminal", kw["terminal"]); st["parked"][kw["terminal"]] = tab["terminals"].pop(i)
    elif a[1] == "pane.focus":
        st["focused"] = kw["pane"] if "pane" in kw else find("terminal", kw["terminal"])[0]["terminals"][find("terminal", kw["terminal"])[1]]["pane"]
    elif a[1] == "terminal.keep":
        x = st["parked"].pop(kw["terminal"], None) or {"terminal": kw["terminal"], "name": None, "app": None}
        st["n"] += 1; x["pane"] = "n%d" % st["n"]
        tab, i = find("pane", st["focused"]); tab["terminals"].insert(i + 1, x)
    print("{}")
    json.dump(st, open(p, "w"))
EOF
chmod +x "$T/fake/herdr-app"
P=$T/fake:$T/py:$T/bin:/usr/bin:/bin
term() { printf '{"terminal": "%s", "pane": "%s", "name": %s, "app": %s}' "$@"; }
state() { printf '{"n": 0, "focused": "%s", "err_on": "%s", "parked": {"t9": %s}, "tabs": [{"id": "%s", "terminals": [%s]}]}\n' \
  "$1" "$2" "$(term t9 - '"lane-d"' null)" "$3" "$4" >"$T/state.json"; }
TABS="$(term t0 p0 '"lead"' '"claude"'), $(term t1 p1 '"lane-a"' '"claude"'), $(term t2 p2 null null), $(term t3 p3 '"lane-b"' null), $(term t4 p4 '"lane-c"' null)"
state p2 "" "w9/tab1" "$TABS"; : >"$T/ha.log"
out=$(hb "$P" "$here/config.example" --main t0 --add t9 --drop t3 2>"$T/err"); rc=$?
t "맞춤: 출력(한글 그대로·순서)" "$rc $out" '0 {"tab": "w9/tab1", "main": "t0", "workers": ["t1", "t4", "t9"]}'
t "맞춤: 상태 파일(/는 __)" "$(cat "$T/home/.local/state/herdr-balance/w9__tab1.json")" '["t1", "t4", "t9"]'
t "맞춤: 칸 순서(가짜는 포커스 칸 뒤에 넣음 — t0 다음 t1·t4·t9, HUD t2는 안 뗌)" "$("$PY" -c 'import json,sys; print(" ".join(x["terminal"] for x in json.load(open(sys.argv[1]))["tabs"][0]["terminals"]))' "$T/state.json")" "t0 t1 t4 t9 t2"
t "맞춤: herdr-app 부른 순서(park 다음 반씩 keep, 끝에 보던 칸으로)" "$(grep '^do' "$T/ha.log")" "$(cat <<'EOF'
do terminal.park {"terminal": "t1"}
do terminal.park {"terminal": "t4"}
do terminal.park {"terminal": "t3"}
do pane.focus {"pane": "p0"}
do terminal.keep {"terminal": "t1", "place": "right"}
do pane.focus {"pane": "n1"}
do terminal.keep {"terminal": "t4", "place": "down"}
do pane.focus {"pane": "n2"}
do terminal.keep {"terminal": "t9", "place": "down"}
do pane.focus {"terminal": "t2"}
EOF
)"
t "맞춤: stderr 없음" "$(cat "$T/err")" ""
state p0 "" tab1 "$TABS"
out=$(hb "$P" "$here/config.example" --main t404 2>"$T/err"); rc=$?
t "화면에 없는 지휘: 종료 1·문구" "$rc $out|$(cat "$T/err")" "1 |t404이(가) 화면의 어느 탭에도 없음"
out=$(FAKE_HA_FAIL=1 hb "$P" "$here/config.example" --main t0 2>"$T/err"); rc=$?
t "herdr-app 실패: 종료 1·출력을 붙인 문구(그 stderr 줄바꿈까지)" "$rc|$(cat "$T/err"; echo .)" "1|herdr-app ls 실패: boom

."
state p0 t4 tab1 "$TABS"
out=$(hb "$P" "$here/config.example" --main t0 2>"$T/err"); rc=$?
t "do 오류: 종료 1·문구" "$rc|$(cat "$T/err")" "1|terminal.park 실패: nope"
out=$(hb "$P" "$here/config.example" 2>"$T/err"); rc=$?
t "인자 없음: usage·종료 2" "$rc|$(cat "$T/err")" "2|usage: herdr-balance [-h] [--main MAIN] [--add [ADD ...]] [--drop [DROP ...]]
                     [--watch]
herdr-balance: error: --main 또는 --watch가 필요"

# --watch: 처음 보는 탭은 지금 배치를 적고, 이름 붙은 worker가 늘어 4초 그대로면 맞춘다. TERM이면 끝난다
rm -rf "$T/home/.local/state"
state p0 "" tab1 "$(term t0 p0 '"lead"' '"claude"'), $(term t1 p1 '"lane-a"' null)"
# 함수(hb)를 거치지 않고 띄운다: env → 포장 → 판이 모두 exec라 $!가 watch 그 프로세스(함수면 서브셸이 TERM을 받고 watch가 고아로 남는다)
env -i HOME="$T/home" PATH="$P" HERDR_TOOLS_CONFIG="$here/config.example" FAKE_HA_STATE="$T/state.json" FAKE_HA_LOG="$T/ha.log" \
  $IMPL "$here/bin/herdr-balance" --watch >"$T/watch.out" 2>"$T/err" &
wpid=$!
i=0; until [ -f "$T/home/.local/state/herdr-balance/tab1.json" ] || [ $i -ge 100 ]; do sleep 0.1; i=$((i + 1)); done
t "watch: 처음 본 탭은 지금 배치를 기준으로" "$(cat "$T/home/.local/state/herdr-balance/tab1.json" 2>/dev/null)" '["t1"]'
"$PY" -c 'import json,sys; p=sys.argv[1]; s=json.load(open(p)); s["tabs"][0]["terminals"].append({"terminal": "t5", "pane": "p5", "name": "lane-e", "app": None}); json.dump(s, open(p + ".new", "w"))' "$T/state.json" &&
  mv "$T/state.json.new" "$T/state.json"
i=0; until grep -q '맞춤' "$T/watch.out" || [ $i -ge 200 ]; do sleep 0.1; i=$((i + 1)); done
t "watch: 바뀐 목록이 그대로면 맞춤 한 줄" "$(sed -E 's/^[0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2} //' "$T/watch.out")" '맞춤 {"tab": "tab1", "main": "t0", "workers": ["t1", "t5"]}'
kill -TERM "$wpid"; i=0; while kill -0 "$wpid" 2>/dev/null && [ $i -lt 50 ]; do sleep 0.1; i=$((i + 1)); done
t "watch: TERM이면 곧 끝남" "$(kill -0 "$wpid" 2>/dev/null && echo 돈다 || echo 끝)" 끝
wait "$wpid" 2>/dev/null; rc=$?; wpid=
# Go 판은 회차를 마치고 0, 셸 판은 TERM으로 죽는다(143) — 의도된 차이
if [ "${HERDR_BALANCE_IMPL-}" = bash ]; then t "watch: 셸 판 TERM 종료 코드" "$rc" 143; else t "watch: Go 판은 TERM에 0으로 끝남" "$rc" 0; fi
echo "${n}개 중 실패 ${fails}개"
[ $fails = 0 ]
