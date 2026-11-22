#!/bin/bash
# herdr-tools 검증: 문법·개인 값·싼 검사(CHEAP) 먼저 — 실패면 바로 끝 — 그다음 tests/*.test.sh 전부(훅 시험은 Go — claude/hooks/tests/*_test.go, tests/hc-merger-go.test.sh의 go test ./...가 두 판으로). hc-lane land가 이것을 부른다.
# --cheap-only: 문법·개인 값·싼 검사까지만(.claude/herdr-precheck.sh — 병합기 제출 직전).
# 무거운 시험은 동시에(2026-11-22 verify-parallel — 하나씩 돌면 병합기 착지 한 건이 45~50분, 그동안 12코어 부하 1.5~2):
#  · 동시 수 HERDR_VERIFY_JOBS(기본 CPU 수의 절반, 최소 2). 1이면 예전처럼 하나씩
#  · 시험 파일 머리에 「# herdr-verify: stages」 줄이 있으면 `bash <파일> --stages`가 찍는 단계(한 줄에 하나)를 따로 돌린다
#    (`bash <파일> --stage <이름>`). 「!」로 시작하는 단계는 홀로 — 맨 먼저, 다른 단계와 겹치지 않게(부하를 재는 시험)
#  · 결과는 원래 순서(파일 이름순)로 「통과 <파일>」·「실패 <파일>」 — 실패면 ^FAIL·not ok 줄, 로그 끝 40줄. 단계가 여럿이면 실패한 단계마다
#  · 하나가 실패해도 나머지는 끝까지. 단계마다 상한 HERDR_VERIFY_STEP_SECS(기본 2400초) — 넘으면 무리째 끄고 「실패 <파일>: 시간 넘김」
#  · 끝에 「검증 시간 …」 한 줄(합·동시 수·긴 단계)
#  · 셸 판 계약 시험(tests/*-bash.test.sh, 훅 시험의 셸 판 갈래)은 HERDR_TEST_BASH=1일 때만 — 아니면 「건너뜀 <파일>」
# 고아 검사: 시험마다 따로 프로세스 무리(pgid)로 돌린다. 시험이 끝난 뒤 그 무리에 남은 프로세스, 또는 검증 중 새로 생겨
# 부모가 1이 된 프로세스 가운데 명령 줄이나 cwd에 이 폴더 경로가 든 것(과 그 자손)은 고아다: 목록(pid·cwd·명령)을 찍고
# 끄고(TERM, 3초 뒤 KILL) 검증 실패. 단 cwd로만 걸리고 명령 줄에 /.claude/가 든 것(이 폴더에서 도는 Claude 세션의
# 훅·플러그인이 떼어 띄운 것)은 뺀다. 그래서 배경 프로세스를 남기는 시험은 착지 전에 걸린다(2026-11-16 hc-fill 시험이 watch 고아 18개를 남김).
# 부모가 1이 된 고아는 도는 시험이 하나도 없을 때 본다(동시에 도는 남의 시험이 떼어 띄운 것을 잘못 잡지 않게) — 그 사이 끝난 시험이 하나면
# 그 이름, 여럿이면 그 목록을 찍는다(HERDR_VERIFY_JOBS=1이면 늘 하나).
set -u
cd "$(dirname "$0")/.." || exit 1
# 추측 검증 기록(README 「추측 검증」): hc-lane land는 같은 트리·같은 이 스크립트 blob의 원격 기록 refs/herdr/verified/<트리>가 있을 때만
# HERDR_VERIFIED_TREE=<트리>를 넘긴다. 지금 HEAD^{tree}가 그 값이고, 추적 파일이 깨끗하고, 원격에 그 기록이 아직 있을 때만 건너뛴다
# (--cheap-only는 늘 돈다). 하나라도 아니면 그대로 검증한다
if [ -n "${HERDR_VERIFIED_TREE-}" ] && [ "${1-}" != --cheap-only ]; then
  vt_=$(git rev-parse -q --verify 'HEAD^{tree}' 2>/dev/null) || vt_=
  if [ "$vt_" = "$HERDR_VERIFIED_TREE" ] && vs_=$(git status --porcelain --untracked-files=no 2>/dev/null) && [ -z "$vs_" ] &&
    vr_=$(git ls-remote origin "refs/herdr/verified/$vt_" 2>/dev/null) && [ -n "$vr_" ]; then
    echo "건너뜀 검증 — 이 트리($vt_)는 이미 통과(refs/herdr/verified)"
    exit 0
  fi
  echo "HERDR_VERIFIED_TREE가 지금 트리·작업 트리·원격 기록과 맞지 않다 — 그대로 검증한다"
fi
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
rp=
# 검증이 끊기면(Ctrl-C·TERM·터미널 닫힘) 실행기에 TERM — 실행기가 돌던 시험 무리를 다 끈다(따로 된 무리라 신호가 저절로 가지 않는다)
stop() { [ -n "$rp" ] && kill -TERM "$rp" 2>/dev/null && wait "$rp"; rm -rf "$tmp"; exit "$1"; }
trap 'stop 130' INT
trap 'stop 143' TERM
trap 'stop 129' HUP
# 싼 검사(몇 초): 맨 앞에 돌리고, 문법·개인 값·싼 검사 가운데 하나라도 실패면 무거운 시험을 돌리지 않고 끝(종료 1).
# HERDR_VERIFY_ALL=1이면 실패해도 끝까지 다 돈다. 아래 루프는 이 목록을 다시 돌리지 않는다.
# 2026-11-21 줄 수 한도에 걸린 레인이 이름순으로 hc-lane 시험(20분+) 뒤에야 반송돼 병합 큐 전체가 기다렸다.
# 2026-11-21 publish-lint-go: 공개 내보내기의 비밀 꼴(가짜 SHA 40자 등)이 착지 뒤에야 걸려 릴리스가 막혔다 — 착지 전에 본다
CHEAP="tests/line-budget.test.sh tests/publish-secrets.test.sh"
# 먼저 띄울 긴 단계(나머지는 이름순 — 긴 것을 늦게 띄우면 끝이 그만큼 늦다). 출력 차례와는 상관없다.
# 2026-11-22 mini-m4 하나씩 실측(초): hc-lane-bash 746 · hc-lane-go 647 · hc-merger-go 589(hcfill 275·herdr-sync 136·묶음 90·훅 68) ·
# hc-fill-bash 581 · hc-lane-merger-bash 247 · hc-lane-merger 162 · 나머지 각 120 아래, 합 3290
FIRST="tests/hc-lane-go.test.sh tests/hc-merger-go.test.sh tests/hc-lane-merger.test.sh tests/hc-lane-bash.test.sh tests/hc-fill-bash.test.sh tests/hc-lane-merger-bash.test.sh"
# 실행기: run.py <단계 목록 파일> <동시 수> <시간 줄 1/0> <먼저 띄울 파일들> <$PWD> <pwd -P> — 목록 한 줄은 「파일<탭>단계<탭>홀로(1/0)」
cat >"$tmp/run.py" <<'PY'
import os, signal, subprocess, sys, time
tmp, before = os.path.dirname(sys.argv[0]), set(open(os.path.join(os.path.dirname(sys.argv[0]), "before")).read().split())
steps = [l.rstrip("\n").split("\t") for l in open(sys.argv[1]) if l.strip()]
jobs, limit = max(1, int(sys.argv[2])), int(os.environ.get("HERDR_VERIFY_STEP_SECS") or 2400)
times, first, here, me = sys.argv[3] == "1", sys.argv[4].split(), sys.argv[5:7], os.getpid()
env = dict(os.environ, HC_LANE_BIN=os.path.join(here[0], "bin/hc-lane"))
env.pop("HERDR_VERIFY_JOBS", None)  # 시험 속 검증(tests/herdr-verify.test.sh)이 바깥 값을 물려받지 않게 — 그 시험이 스스로 정한다
files = []  # 출력 차례(파일 처음 나온 차례)
for f, st, solo in steps:
    if f not in files:
        files.append(f)
# 띄우는 차례: 홀로 단계 먼저(하나씩), 그다음 FIRST 차례, 나머지 목록 차례
order = sorted(range(len(steps)), key=lambda i: (steps[i][2] != "1", first.index(steps[i][0]) if steps[i][0] in first else len(first), i))
run, res, since, printed, t0 = {}, {}, [], 0, time.time()

def procs():
    out = {}
    for line in subprocess.run(["ps", "-x", "-o", "pid=,pgid=,ppid=,command="], capture_output=True, text=True).stdout.splitlines():
        f = line.split(None, 3)
        if len(f) >= 3 and f[0].isdigit():
            out[int(f[0])] = (int(f[1]), int(f[2]), f[3] if len(f) > 3 else "")
    return out

def getcwd(pids, cwd):
    pid = None
    if pids:
        for line in subprocess.run(["lsof", "-a", "-d", "cwd", "-p", ",".join(map(str, pids)), "-Fpn"],
                                   capture_output=True, text=True).stdout.splitlines():
            if line[:1] == "p":
                pid = int(line[1:])
            elif line[:1] == "n" and pid:
                cwd[pid] = line[1:] + "/"

def alive(p):
    try:
        os.kill(p, 0)
        return True
    except OSError:  # 끝났거나 이제 남의 pid
        return False

def send(pids, sig):
    for p in pids:
        try:
            os.kill(p, sig)
        except OSError:
            pass

def orphans(pg):
    """pg가 있으면 그 무리에 남은 것, 없으면 검증 중 새로 생겨 부모가 1이 된 이 폴더 프로세스. 자손까지 찍고 끈다. 찍은 줄 목록"""
    ps, cwd = procs(), {}
    new = [p for p, (g, pp, cmd) in ps.items() if pp == 1 and str(p) not in before]
    if pg is None:
        getcwd(new, cwd)
    # cwd로만 걸린 것 가운데 명령 줄에 /.claude/(plugins·hooks·shell-snapshots)가 든 것은 이 폴더에서 도는
    # Claude 세션의 훅·플러그인이 떼어 띄운 것이라 뺀다(2026-11-19 mini-m4 레인 착지가 OMC node를 잡아 끔).
    # 명령 줄에 이 폴더 경로가 들면 /.claude/가 있어도 잡는다(저장소 .claude/ 스크립트)
    def stray(cmd, c):
        return any(h + "/" in cmd or (c.startswith(h + "/") and "/.claude/" not in cmd) for h in here)
    bad = {p for p, (g, pp, cmd) in ps.items() if p != me and (g == pg if pg is not None else (p in new and stray(cmd, cwd.get(p, ""))))}
    while True:  # 자손도 함께
        kids = {p for p, (g, pp, cmd) in ps.items() if pp in bad and p != me} - bad
        if not kids:
            break
        bad |= kids
    if not bad:
        return []
    getcwd([p for p in bad if p not in cwd], cwd)
    # cwd도 찍는다: 이 폴더의 다른 Claude 세션 소속인지 사람이 가리게
    lines = ["  고아 pid %d (cwd %s): %s" % (p, cwd.get(p, "?/")[:-1], ps[p][2][:120]) for p in sorted(bad)]
    send(bad, signal.SIGTERM)
    for _ in range(30):
        left = [p for p in bad if alive(p)]
        if not left:
            break
        time.sleep(0.1)
    send(left, signal.SIGKILL)
    return lines

def start(i):
    f, st, solo = steps[i]
    log = open(os.path.join(tmp, "log.%d" % i), "w")
    p = subprocess.Popen(["bash", os.path.join(here[0], f)] + (["--stage", st] if st else []), stdin=subprocess.DEVNULL,
                         stdout=log, stderr=subprocess.STDOUT, env=env, preexec_fn=lambda: os.setpgid(0, 0))
    log.close()
    run[i] = (p, time.time())

def finish(i, rc, over):
    p, s = run.pop(i)
    res[i] = (rc, over, orphans(p.pid), time.time() - s)
    since.append(steps[i][0])

def stopall(sig, _):
    for p, _s in run.values():
        send([-p.pid], signal.SIGTERM)
    sys.exit(128 + sig)

for s in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP):
    signal.signal(s, stopall)

def show(f):
    """한 파일의 결과(단계 모두 끝난 뒤). 실패면 True"""
    idx = [i for i in range(len(steps)) if steps[i][0] == f]
    bad = [i for i in idx if res[i][0] != 0 or res[i][1]]
    out = ["통과 " + f] if not bad else ["실패 " + f]
    for i in bad:
        rc, over, _o, d = res[i]
        if len(idx) > 1 or steps[i][1]:
            out.append("  단계 %s: %s" % (steps[i][1] or "-", "시간 넘김(%d초) — 무리째 끔" % limit if over else "실패(종료 %d)" % rc))
        elif over:
            out.append("실패 %s: 시간 넘김(%d초) — 무리째 끔" % (f, limit))
        # 실패면 FAIL 줄을 먼저(끝 40줄에 잘려 흔들린 시험 번호가 안 보였다 — lane-push 4차 감사 [낮음] 3)
        log = open(os.path.join(tmp, "log.%d" % i), errors="replace").read().splitlines()
        out += [l for l in log if l.startswith("FAIL") or l.startswith("not ok")] + log[-40:]
    for i in idx:
        if res[i][2]:
            out += res[i][2] + ["실패 %s: 고아를 남김(위 목록, 껐음)" % f]
    print("\n".join(out), flush=True)
    return bool(bad) or any(res[i][2] for i in idx)

fail, nxt = 0, 0
while nxt < len(order) or run:
    while nxt < len(order) and len(run) < jobs:
        i = order[nxt]
        if run and (steps[i][2] == "1" or any(steps[j][2] == "1" for j in run)):
            break  # 홀로 단계는 다른 것과 겹치지 않게
        start(i)
        nxt += 1
    time.sleep(0.1)
    for i in list(run):
        p, s = run[i]
        rc = p.poll()
        if rc is not None:
            finish(i, rc, False)
        elif time.time() - s > limit:
            send([-p.pid], signal.SIGTERM)
            for _ in range(30):
                if p.poll() is not None:
                    break
                time.sleep(0.1)
            send([-p.pid], signal.SIGKILL)
            finish(i, p.wait(), True)
    while printed < len(files) and all(i in res for i in range(len(steps)) if steps[i][0] == files[printed]):
        fail |= show(files[printed])
        printed += 1
    if not run and since:
        lines = orphans(None)
        if lines:
            who = sorted(set(since))
            print("\n".join(lines + ["실패 %s: 고아를 남김(위 목록, 껐음)" % who[0] if len(who) == 1 else
                                     "실패 고아를 남김(위 목록, 껐음) — 동시에 돈 시험 가운데: %s (HERDR_VERIFY_JOBS=1로 다시 돌리면 이름이 나온다)" % " ".join(who)]), flush=True)
            fail = 1
        since = []
if times:
    top = sorted(res, key=lambda i: -res[i][3])[:5]
    print("검증 시간 %d초 · 동시 %d — 긴 단계: %s" % (time.time() - t0, jobs,
          " · ".join("%s%s %d초" % (steps[i][0], " " + steps[i][1] if steps[i][1] else "", res[i][3]) for i in top)), flush=True)
sys.exit(fail)
PY
# runall <동시 수> <시간 줄 1/0> <파일…>: 단계 목록을 만들어 실행기로. 단계를 못 읽으면 그 파일은 실패
runall() {
  local j=$1 tm=$2 t st; shift 2
  : >"$tmp/steps"
  for t in "$@"; do
    [ -f "$t" ] || continue
    if grep -q '^# herdr-verify: stages$' "$t"; then
      st=$(bash "$t" --stages </dev/null) && [ -n "$st" ] || { echo "실패 $t: 단계 목록(--stages)을 못 읽음"; fail=1; continue; }
      printf '%s\n' "$st" | while IFS= read -r s; do
        case "$s" in '!'*) printf '%s\t%s\t1\n' "$t" "${s#!}" ;; *) printf '%s\t%s\t0\n' "$t" "$s" ;; esac
      done >>"$tmp/steps"
    else printf '%s\t\t0\n' "$t" >>"$tmp/steps"; fi
  done
  python3 "$tmp/run.py" "$tmp/steps" "$j" "$tm" "$FIRST" "$PWD" "$(pwd -P)" &
  rp=$!
  wait "$rp" || fail=1
  rp=
}
# 싼 검사 파일이 레인에서 사라지면 조용히 빠지지 않게(B28): 기준 브랜치 origin/main에 있던 파일이 없으면 이름을 찍고 실패.
# 기준 브랜치가 없는 저장소(원격 없는 시험용 가짜 저장소)는 그대로 건너뜀
cheap=
for t in $CHEAP; do
  if [ ! -f "$t" ] && git rev-parse -q --verify origin/main >/dev/null 2>&1; then
    if ! on=$(git ls-tree --name-only origin/main -- "$t" 2>&1); then   # 못 읽음은 「없었음」이 아니다
      echo "실패 $t: 없고, 기준 브랜치(origin/main)에 있었는지 못 읽음: $on"; fail=1; continue
    elif [ -n "$on" ]; then
      echo "실패 $t: 기준 브랜치(origin/main)에 있는 싼 검사 파일이 없음"; fail=1; continue
    fi
  fi
  cheap="$cheap $t"
done
# shellcheck disable=SC2086 # 파일 목록은 낱말로
runall 1 0 $cheap
# --cheap-only: 싼 검사까지만(제출 전 사전 검사 .claude/herdr-precheck.sh — go-followups)
if [ "${1-}" = --cheap-only ]; then rm -rf "$tmp"; exit $fail; fi
if [ $fail != 0 ] && [ "${HERDR_VERIFY_ALL-}" != 1 ]; then
  echo "실패 싼 검사 — 무거운 시험을 건너뛰고 끝 (끝까지 돌리려면 HERDR_VERIFY_ALL=1)"
  rm -rf "$tmp"
  exit 1
fi
heavy=()
for t in tests/*.test.sh; do
  case " $CHEAP " in *" $t "*) continue ;; esac
  # 셸 판 계약 시험(tests/*-bash.test.sh)은 HERDR_TEST_BASH=1일 때만(2026-11-22 verify-parallel — 셸 판은 곧 걷는다, 지휘 t1cj).
  # 훅 시험의 셸 판 갈래도 같은 값으로(claude/hooks/tests impls)
  case "$t:${HERDR_TEST_BASH-}" in *-bash.test.sh:1) ;; *-bash.test.sh:*) echo "건너뜀 $t (셸 판 — HERDR_TEST_BASH=1일 때만)"; continue ;; esac
  heavy+=("$t")
done
jobs=${HERDR_VERIFY_JOBS:-$(( $(sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo 4) / 2 ))}
case "$jobs" in ''|*[!0-9]*) jobs=2 ;; esac
[ -n "${HERDR_VERIFY_JOBS-}" ] || [ "$jobs" -ge 2 ] || jobs=2
runall "$jobs" 1 ${heavy[@]+"${heavy[@]}"}   # bash 3.2: 빈 배열은 set -u에 걸린다
rm -rf "$tmp"
exit $fail
