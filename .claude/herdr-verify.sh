#!/bin/bash
# herdr-tools 검증: 문법 검사 + tests/*.test.sh·claude/hooks/tests/*.test.sh 전부. hc-lane land가 이것을 부른다.
# 고아 검사: 시험마다 따로 프로세스 무리(pgid)로 돌린다. 시험이 끝난 뒤 그 무리에 남은 프로세스, 또는 검증 중 새로 생겨
# 부모가 1이 된 프로세스 가운데 명령 줄이나 cwd에 이 폴더 경로가 든 것(과 그 자손)은 고아다: 목록을 찍고 끄고(TERM, 3초 뒤 KILL)
# 검증 실패. 그래서 배경 프로세스를 남기는 시험은 착지 전에 걸린다(2026-11-16 hc-fill 시험이 watch 고아 18개를 남김).
set -u
cd "$(dirname "$0")/.." || exit 1
fail=0
for f in bin/* install.sh .claude/*.sh; do
  [ -f "$f" ] || continue
  case "$(head -1 "$f")" in
    *python*) python3 -c 'import ast,sys; ast.parse(open(sys.argv[1]).read())' "$f" || { echo "문법 실패 $f"; fail=1; } ;;
    *bash*|*/sh*) bash -n "$f" || { echo "문법 실패 $f"; fail=1; } ;;
  esac
done
# 개인 값 검사: git ls-files에서 개인 값 패턴(개인 설정 private-patterns.txt, 없으면 ~/.local/share/herdr-tools/)에 걸린 줄.
# --strict: 패턴 파일·패턴 줄이 없으면 실패(검사 없이 착지하지 않게). 걸려도 실패.
# 패턴이 없는 게 정당한 곳(공개 사본을 받아 돌리는 쪽)은 HERDR_VERIFY_NO_LINT=1로만 건너뛴다.
# 레인 안(HC_LANE)에서는 무시한다: 무인 레인이 그 값으로 막힘을 우회해 착지하지 않게
if [ "${HERDR_VERIFY_NO_LINT-}" = 1 ] && [ -n "${HC_LANE-}" ]; then echo "레인 안이라 HERDR_VERIFY_NO_LINT 무시 (HC_LANE=$HC_LANE)"; fi
if [ "${HERDR_VERIFY_NO_LINT-}" = 1 ] && [ -z "${HC_LANE-}" ]; then echo "건너뜀 개인 값 검사 (HERDR_VERIFY_NO_LINT=1)"
elif bash bin/herdr-tools-config lint --strict; then echo "통과 개인 값 검사"
else echo "실패 개인 값 검사 (위 줄을 범용 값으로 바꾸거나 그 파일을 개인 저장소로. 패턴 파일 없음이면 herdr-sync로 맞춘다)"; fail=1; fi
tmp=$(mktemp -d "${TMPDIR:-/tmp}/herdr-tools-verify.XXXXXX") || exit 1
ps -x -o pid= >"$tmp/before"
pg=
# 검증이 끊기면(Ctrl-C·TERM·터미널 닫힘) 돌던 시험 무리도 같이 끈다. 따로 된 무리라 신호가 저절로 가지 않는다
stop() { [ -n "$pg" ] && kill -TERM -- "-$pg" 2>/dev/null; rm -rf "$tmp"; exit "$1"; }
trap 'stop 130' INT
trap 'stop 143' TERM
trap 'stop 129' HUP
for t in tests/*.test.sh claude/hooks/tests/*.test.sh; do
  [ -f "$t" ] || continue
  HC_LANE_BIN="$PWD/bin/hc-lane" python3 -c 'import os,sys; os.setpgid(0, 0); os.execvp("bash", ["bash"] + sys.argv[1:])' "$PWD/$t" \
    >"$tmp/log" 2>&1 </dev/null &
  pg=$!
  # 실패면 FAIL 줄을 먼저(끝 40줄에 잘려 흔들린 시험 번호가 안 보였다 — lane-push 4차 감사 [낮음] 3)
  if wait $pg; then echo "통과 $t"; else echo "실패 $t"; grep -E '^(FAIL|not ok)' "$tmp/log"; tail -40 "$tmp/log"; fail=1; fi
  python3 - "$pg" "$tmp/before" "$PWD" "$(pwd -P)" <<'PY' || { echo "실패 $t: 고아를 남김(위 목록, 껐음)"; fail=1; }
import os, signal, subprocess, sys, time
pg, before, here, me = int(sys.argv[1]), set(open(sys.argv[2]).read().split()), sys.argv[3:], os.getpid()
procs = {}
for line in subprocess.run(["ps", "-x", "-o", "pid=,pgid=,ppid=,command="], capture_output=True, text=True).stdout.splitlines():
    f = line.split(None, 3)
    if len(f) >= 3 and f[0].isdigit():
        procs[int(f[0])] = (int(f[1]), int(f[2]), f[3] if len(f) > 3 else "")
new = [p for p, (g, pp, cmd) in procs.items() if pp == 1 and str(p) not in before]
cwd, pid = {}, None  # 새 PPID 1 프로세스의 cwd
if new:
    for line in subprocess.run(["lsof", "-a", "-d", "cwd", "-p", ",".join(map(str, new)), "-Fpn"],
                               capture_output=True, text=True).stdout.splitlines():
        if line[:1] == "p":
            pid = int(line[1:])
        elif line[:1] == "n" and pid:
            cwd[pid] = line[1:] + "/"
bad = {p for p, (g, pp, cmd) in procs.items() if p != me and (g == pg or (
    p in new and any(h + "/" in cmd or cwd.get(p, "").startswith(h + "/") for h in here)))}
while True:  # 자손도 함께
    kids = {p for p, (g, pp, cmd) in procs.items() if pp in bad and p != me} - bad
    if not kids:
        break
    bad |= kids
if not bad:
    sys.exit(0)
for p in sorted(bad):
    print("  고아 pid %d: %s" % (p, procs[p][2][:120]))
def send(pids, sig):
    for p in pids:
        try:
            os.kill(p, sig)
        except OSError:
            pass
def alive(p):
    try:
        os.kill(p, 0)
        return True
    except OSError:  # 끝났거나 이제 남의 pid
        return False
send(bad, signal.SIGTERM)
for _ in range(30):
    left = [p for p in bad if alive(p)]
    if not left:
        break
    time.sleep(0.1)
send(left, signal.SIGKILL)
sys.exit(1)
PY
done
rm -rf "$tmp"
exit $fail
