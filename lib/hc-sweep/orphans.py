"""hc-sweep 셸 판: 고아 쓸기 (머리글은 sweep.py)."""
from common import *


def secs(etime):
    """ps etime([[dd-]hh:]mm:ss) → 초."""
    days, _, rest = etime.rpartition("-")
    n = 0
    for part in rest.split(":"):
        n = n * 60 + int(part)
    return n + int(days or 0) * 86400


def processes(run=subprocess.run):
    """이 사용자의 프로세스 {pid: (ppid, 나이 초, 명령)}."""
    out = run(["ps", "-x", "-o", "pid=,ppid=,etime=,command="], capture_output=True, text=True,
              stdin=subprocess.DEVNULL, timeout=60)
    if out.returncode:
        raise RuntimeError("ps 실패")
    procs = {}
    for line in out.stdout.splitlines():
        f = line.split(None, 3)
        if len(f) >= 3 and f[0].isdigit():
            procs[int(f[0])] = (int(f[1]), secs(f[2]), f[3] if len(f) > 3 else "")
    return procs


def exes(run=subprocess.run):
    """{pid: argv[0]}(ps comm — 맥은 공백 든 경로도 그대로, 리눅스는 15자로 잘린 이름이라 "/" 든 것만 쓴다).
    processes() 뒤 pid가 재사용되면 keep으로 잘못 걸려 한 바퀴 남을 수 있다(끄는 쪽은 kill_tree가 다시 확인)."""
    out = run(["ps", "-x", "-o", "pid=,comm="], capture_output=True, text=True, stdin=subprocess.DEVNULL, timeout=60)
    if out.returncode:
        raise RuntimeError("ps 실패")
    res = {}
    for line in out.stdout.splitlines():
        f = line.strip().split(None, 1)
        if len(f) == 2 and f[0].isdigit() and "/" in f[1]:
            res[int(f[0])] = f[1]
    return res


def cwds(pids, run=subprocess.run):
    """{pid: cwd}. 못 읽은 pid는 빠진다(명령 줄로만 판정)."""
    if not pids:
        return {}
    try:
        out = run(["lsof", "-a", "-d", "cwd", "-p", ",".join(map(str, pids)), "-Fpn"], capture_output=True,
                  text=True, stdin=subprocess.DEVNULL, timeout=60).stdout
    except Exception:
        return {}
    res, pid = {}, None
    for line in out.splitlines():
        if line[:1] == "p":
            pid = int(line[1:])
        elif line[:1] == "n" and pid:
            res[pid] = line[1:]
    return res


def lane_root(path, suffix):
    """절대 경로가 레인 작업 트리 아래면 그 작업 트리(<…><접미사>/<레인>), 아니면 None."""
    if not isinstance(path, str) or not path.startswith("/"):
        return None
    parts = os.path.normpath(path).split("/")
    for i, p in enumerate(parts[:-1]):
        if p.endswith(suffix) and len(p) > len(suffix):
            return "/".join(parts[:i + 2])
    return None


def under(c, root):
    return c == root or c.startswith(root + "/")


def orphans(procs, cwd_of, term_cwds, suffix, min_age, me=None, only=None, exists=exists, keep=(), exe_of=None):
    """고아 후보 [(pid, 작업 트리, 나이 초, 명령, 까닭, 끔 여부)]. 그 작업 트리 아래를 쓰는 live 터미널이 있으면 빼고,
    작업 트리가 있는데 본 저장소 아래를 쓰는 터미널이 있거나 cwd로만 걸린 것은 보류(끔 False). 경로는 실제 경로로 견준다.
    keep은 실행 파일 이름: exe_of(ps comm)에 있으면 그 경로, 없으면 명령 첫 낱말(공백 든 경로는 잘린다)."""
    terms = [os.path.realpath(c) for c in term_cwds]
    found = []
    for pid, (ppid, age, cmd) in sorted(procs.items()):
        exe = (exe_of or {}).get(pid) or (cmd.split() or [""])[0]
        if ppid != 1 or pid == me or age < min_age or os.path.basename(exe) in keep:
            continue
        by_cmd = {r for r in (lane_root(x.split("=", 1)[-1], suffix) for x in cmd.split()) if r}
        roots = sorted(by_cmd | {r for r in [lane_root(cwd_of.get(pid), suffix)] if r})
        if only:
            roots = [r for r in roots if r.startswith(only.rstrip("/") + "/")]
            by_cmd &= set(roots)
        if not roots or any(exists(r) and any(under(c, os.path.realpath(r)) for c in terms) for r in roots):
            continue
        there = [r for r in roots if exists(r)]
        mains = [os.path.realpath(os.path.dirname(r)[:-len(suffix)]) for r in there]
        if any(under(c, m) for m in mains for c in terms):
            why, kill = "보류: 본 저장소 터미널 있음", False
        elif there and not by_cmd:
            why, kill = "보류: cwd로만 걸림, 작업 트리 있음", False
        else:
            why, kill = "쓰는 터미널 없음" if there else "작업 트리 지워짐", True
        found.append((pid, (there or roots)[0], age, cmd, why, kill))
    return found


def tree(pids, procs):
    """pids와 그 자손."""
    kids = {}
    for p, (pp, _, _) in procs.items():
        kids.setdefault(pp, []).append(p)
    todo, out = list(pids), set()
    while todo:
        p = todo.pop()
        if p not in out:
            out.add(p)
            todo += kids.get(p, [])
    return out


def kill_tree(pids, procs, wait=5.0):
    """pids와 그 자손을 끈다(TERM, wait초 뒤 남으면 KILL). 지금 다시 본 ppid·명령이 같은 것만. KILL까지 간 pid 집합."""
    now = processes()
    todo = {p for p in tree(pids, procs) if p in now and (now[p][0], now[p][2]) == (procs[p][0], procs[p][2])}

    def send(ps, sig):
        for p in ps:
            try:
                os.kill(p, sig)
            except OSError:
                pass

    def alive(p):
        try:
            os.kill(p, 0)
            return True
        except OSError:
            return False

    send(todo, signal.SIGTERM)
    end = time.time() + wait
    while time.time() < end and any(alive(p) for p in todo):
        time.sleep(0.2)
    left = {p for p in todo if alive(p)}
    send(left, signal.SIGKILL)
    return left


def seen(pid, head, path=None):
    """closed.log에 이 pid·명령 앞부분의 관찰 기록이 이미 있나. 제목 「나이 N분 <명령>」의 나이는 견주지 않는다."""
    try:
        with open(path or os.path.join(TASKS, "closed.log")) as f:
            return any(x[1:2] == ["pid:%d" % pid] and x[6:7] and x[6].split(" ", 2)[2:] == [head] and "관찰" in x[3]
                       for x in (line.rstrip("\n").split("\t") for line in f))
    except OSError:
        return False


def sweep_orphans(terms, dry, min_age, only=None):
    """고아 쓸기 한 바퀴. 끌 것 수."""
    suffix = conf("sweep.wt_suffix", ".wt")
    keep = conf("sweep.orphan_keep", "codebase-memory-mcp herdr-core")
    mode = conf("sweep.orphan_kill", "off")
    if not suffix or "/" in suffix or keep is None or mode is None:
        print("고아 쓸기 건너뜀: 설정 sweep.wt_suffix·sweep.orphan_keep·sweep.orphan_kill을 못 읽음")
        return 0
    if mode not in ("on", "off"):
        print("고아 쓸기: sweep.orphan_kill은 on·off다(지금 %r) — 끄지 않는다" % mode)
        mode = "off"
    if not isinstance(terms, list) or not all(isinstance(t, dict) and isinstance(t.get("address"), str) for t in terms):
        print("고아 쓸기 건너뜀: hc ls에 terminals 목록이 없다")
        return 0
    live = [t for t in terms if t.get("state") == "live"]
    local = [t for t in live if "/" not in t["address"]]
    if any(not isinstance(t.get("cwd"), str) for t in local):
        print("고아 쓸기 건너뜀: cwd를 모르는 이 기계 터미널이 있다")
        return 0
    procs = processes()
    cwd_of = cwds([p for p, (pp, _, _) in procs.items() if pp == 1])
    found = orphans(procs, cwd_of, [t["cwd"] for t in live if isinstance(t.get("cwd"), str)], suffix, min_age,
                    os.getpid(), only, keep=set(keep.split()), exe_of=exes())
    term_pids = set()
    if any(f[5] for f in found):  # 자손에 live 터미널 프로세스(코어·에이전트)가 있으면 끄지 않는다
        for t in local:
            try:
                term_pids.add(int(hc("get", t["address"])["pid"]))
            except (RuntimeError, KeyError, TypeError, ValueError):
                term_pids = None
                break
    targets = 0
    for pid, root, age, cmd, why, kill in found:
        if kill and term_pids is None:
            why, kill = "보류: 터미널 pid를 못 읽음", False
        elif kill and tree([pid], procs) & term_pids:
            why, kill = "보류: 자손에 터미널 프로세스 있음", False
        line = "pid %d 나이 %d분 %s (%s) %s" % (pid, age // 60, root, why, cmd[:80])
        head = cmd[:80].replace("\t", " ").replace("\n", " ")
        title = "나이 %d분 %s" % (age // 60, head)
        if not kill:
            print("보류 고아:", line)
            continue
        targets += 1
        if dry:
            print("끌 고아:", line)
        elif mode == "off":
            print("고아(끄기 꺼짐, 기록만):", line)
            if not seen(pid, head):
                record({"address": "pid:%d" % pid, "name": "-", "cwd": root, "title": title}, "고아 관찰(%s)" % why, "-")
        else:
            hard = kill_tree([pid], procs)
            record({"address": "pid:%d" % pid, "name": "-", "cwd": root, "title": title},
                   "고아(%s)%s" % (why, " KILL" if hard else ""), "-")
            print("끈 고아:", line, "(KILL)" if hard else "")
    return targets
