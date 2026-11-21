#!/bin/bash
# herdr-tools 검증: 문법·개인 값·싼 검사(CHEAP) 먼저 — 실패면 바로 끝 — 그다음 tests/*.test.sh 전부(훅 시험은 Go — claude/hooks/tests/*_test.go, tests/hc-merger-go.test.sh의 go test ./...가 두 판으로). hc-lane land가 이것을 부른다.
# --cheap-only: 문법·개인 값·싼 검사까지만(.claude/herdr-precheck.sh — 병합기 제출 직전).
# 고아 검사: 시험마다 따로 프로세스 무리(pgid)로 돌린다. 시험이 끝난 뒤 그 무리에 남은 프로세스, 또는 검증 중 새로 생겨
# 부모가 1이 된 프로세스 가운데 명령 줄이나 cwd에 이 폴더 경로가 든 것(과 그 자손)은 고아다: 목록(pid·cwd·명령)을 찍고
# 끄고(TERM, 3초 뒤 KILL) 검증 실패. 단 cwd로만 걸리고 명령 줄에 /.claude/가 든 것(이 폴더에서 도는 Claude 세션의
# 훅·플러그인이 떼어 띄운 것)은 뺀다. 그래서 배경 프로세스를 남기는 시험은 착지 전에 걸린다(2026-11-16 hc-fill 시험이 watch 고아 18개를 남김).
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
for f in lib/*.sh lib/*/*.sh; do   # 도구가 source하는 함수 파일(첫 줄이 #!가 아니다 — lib/go-run.sh 포함)
  [ -f "$f" ] || continue
  bash -n "$f" || { echo "문법 실패 $f"; fail=1; }
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
# 싼 검사(몇 초): 맨 앞에 돌리고, 문법·개인 값·싼 검사 가운데 하나라도 실패면 무거운 시험을 돌리지 않고 끝(종료 1).
# HERDR_VERIFY_ALL=1이면 실패해도 끝까지 다 돈다. 아래 루프는 이 목록을 다시 돌리지 않는다.
# 2026-11-21 줄 수 한도에 걸린 레인이 이름순으로 hc-lane 시험(20분+) 뒤에야 반송돼 병합 큐 전체가 기다렸다.
# 2026-11-21 publish-lint-go: 공개 내보내기의 비밀 꼴(가짜 SHA 40자 등)이 착지 뒤에야 걸려 릴리스가 막혔다 — 착지 전에 본다
CHEAP="tests/line-budget.test.sh tests/publish-secrets.test.sh"
run() {
  t=$1
  [ -f "$t" ] || return 0
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
cwd = {}  # pid -> cwd + "/"
def getcwd(pids):
    pid = None
    if pids:
        for line in subprocess.run(["lsof", "-a", "-d", "cwd", "-p", ",".join(map(str, pids)), "-Fpn"],
                                   capture_output=True, text=True).stdout.splitlines():
            if line[:1] == "p":
                pid = int(line[1:])
            elif line[:1] == "n" and pid:
                cwd[pid] = line[1:] + "/"
getcwd(new)
# cwd로만 걸린 것 가운데 명령 줄에 /.claude/(plugins·hooks·shell-snapshots)가 든 것은 이 폴더에서 도는
# Claude 세션의 훅·플러그인이 떼어 띄운 것이라 뺀다(2026-11-19 mini-m4 레인 착지가 OMC node를 잡아 끔).
# 명령 줄에 이 폴더 경로가 들면 /.claude/가 있어도 잡는다(저장소 .claude/ 스크립트)
def stray(cmd, c):
    return any(h + "/" in cmd or (c.startswith(h + "/") and "/.claude/" not in cmd) for h in here)
bad = {p for p, (g, pp, cmd) in procs.items() if p != me and (g == pg or (
    p in new and stray(cmd, cwd.get(p, ""))))}
while True:  # 자손도 함께
    kids = {p for p, (g, pp, cmd) in procs.items() if pp in bad and p != me} - bad
    if not kids:
        break
    bad |= kids
if not bad:
    sys.exit(0)
getcwd([p for p in bad if p not in cwd])
for p in sorted(bad):  # cwd도 찍는다: 이 폴더의 다른 Claude 세션 소속인지 사람이 가리게
    print("  고아 pid %d (cwd %s): %s" % (p, cwd.get(p, "?/")[:-1], procs[p][2][:120]))
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
}
for t in $CHEAP; do run "$t"; done
# --cheap-only: 싼 검사까지만(제출 전 사전 검사 .claude/herdr-precheck.sh — go-followups)
if [ "${1-}" = --cheap-only ]; then rm -rf "$tmp"; exit $fail; fi
if [ $fail != 0 ] && [ "${HERDR_VERIFY_ALL-}" != 1 ]; then
  echo "실패 싼 검사 — 무거운 시험을 건너뛰고 끝 (끝까지 돌리려면 HERDR_VERIFY_ALL=1)"
  rm -rf "$tmp"
  exit 1
fi
for t in tests/*.test.sh; do
  case " $CHEAP " in *" $t "*) continue ;; esac
  run "$t"
done
rm -rf "$tmp"
exit $fail
