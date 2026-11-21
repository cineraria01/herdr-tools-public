#!/usr/bin/env python3
"""끝났는데 안 닫힌 hc 에이전트 터미널을 닫고, 레인이 남긴 고아 프로세스를 끈다 (launchd <사용자>.hc-sweep, 5분마다).

hc-sweep              규칙에 걸린 터미널을 닫고 고아를 끄고 ~/.claude-tasks/closed.log에 남긴다
hc-sweep --dry-run    닫을 것·끌 것과 이유만 출력
hc-sweep --self-test  가짜 기록으로 규칙 판정 확인(hc를 부르지 않는다)

대상: 이 기계(주소에 "/" 없음)의 live 터미널 중 status.app이 있고 state가 idle·done인 것.
닫지 않음: 이름에 loop, -proxy로 끝남, hc-lane 감시(이름 lane-watch-* 또는 라벨 hc-lane-watch=1 — 스스로 끝난다), ~/.claude-tasks/keep(한 줄에 이름·주소 하나),
사람이 연 칸(parent 없는 where 라벨 칸, 또는 터미널 자체 프로그램이 셸 — kill하면 칸이 사라진다).
worker = parent가 있거나 라벨 hc-lane=1(hc-fill이 띄운 레인은 parent가 없다 — 2026-11-17 원격 레인이 몇 시간 남음). where 라벨이 있어도 worker면 닫을 수 있다(herdr-balance가 붙이며 라벨을 단다).
R1 안 쓴 세션: worker 아님, turn 0, 시작부터 줄곧 idle(status_ms-created_ms<60초), 만든 지 30분 이상
R2 일 끝난 worker: 마지막 상태 변화(status_ms)로부터 30분 이상(마지막 hc-lane 사건이 「제출」 — 병합기에 넣고 끝남 — 이면 바로). 단 다음이면 닫지 않는다
   (a) cwd가 git 작업 트리이고 git cherry origin/main HEAD에 + 커밋이 있음(착지 전 레인, fetch 안 함)
   (b) 화면 마지막 줄들에 배경 실행 흔적(BUSY: "still running", 상태 줄 "· N shell(s)"·"· N monitor(s)")
   (c) 이 기계 hc-fill 대기열(${HC_FILL_DATA:-~/.local/share/hc-fill}/queue.tsv)에 같은 이름·주소의 받은편지 레인이
       아직 「떠 있음」(up — 결과를 요청한 기계에 되돌려 주기 전). 되돌려 줌(sent)·손으로 넣은 줄은 보통 판정.
       대기열은 hc-fill이 도는 기계에만 있어 원격 레인 기계에는 이 보호가 없다. 그래도 잃는 것은 없다: 보고 파일은 터미널과
       따로 남고, hc-fill은 터미널이 사라진(gone) 레인도 그 보고를 되돌려 준다
R2·R3 공통 보호: cwd가 아직 있는 링크된 git 워크트리(그 폴더나 위 폴더의 .git이 파일)면 닫지 않는다. 레인은 착지 뒤 hc-lane clean으로 워크트리를 지우니, 남아 있으면 일이 안 끝났다. git을 부르지 않는다.
   단 worker이고(쉰 지 30분 또는 제출 뒤) 그 레인이 착지했고(git cherry origin/main HEAD에 + 없음 + 착지 증거: 이 레인 이름의
   hc-lane 사건 착지·제출·완료·판정, 없으면 원격 refs/hc-landed/<가지>/*) 작업 트리가 깨끗하면(status --porcelain 빔) 보호를 풀고,
   닫은 뒤 hc-lane clean --repo <메인 저장소> --branch <가지>(teardown → worktree remove → branch -D, hc exec로)를 부른다 —
   hc-sweep은 작업 트리 때문에, hc-lane gc는 터미널 때문에 서로 기다리던 교착(2026-11-21 mini-m6-1 16시간). 결과는 closed.log에.
--dry-run은 남기는 터미널마다 「남김: <주소> <이름> — <까닭>」 한 줄을 찍는다.
판정은 모두 fail-closed: 예외·시간 초과·읽기 실패·모르는 값은 「닫지 않음」이다. R3 오래 쉰 세션: status_ms로부터 6시간 이상
닫는 터미널이 hc-lane 레인(라벨 hc-lane=1)이면 닫기 전에 hc-lane events.tsv에 「정리됨」 한 줄(hc-lane wait는 그 레인의
앞 report가 착지·판정 그 턴이면 조용히, 아니면 「뜻밖의 종료(정리됨)」로 알린다). 그 줄을 못 쓰면 닫지 않는다.
다시 열기: closed.log의 cwd·sessionId로 cd <cwd> && claude --resume <sessionId>. 터미널의 argv·환경 변수는 출력하지도 기록하지도 않는다(자격증명이 섞여 있다).

고아 쓸기: 이 사용자의 PPID 1 프로세스 가운데 명령 줄의 절대 경로나 cwd가 레인 작업 트리(<저장소><접미사>/<레인>,
접미사는 설정 sweep.wt_suffix, 기본 .wt) 아래이고 만든 지 --min-age분(기본 10) 이상인 것. 실행 파일 이름이 설정
sweep.orphan_keep(기본 codebase-memory-mcp herdr-core — 여러 세션이 함께 쓰는 데몬·코어)에 있으면 보지 않는다.
  남김: 그 작업 트리 아래를 cwd로 쓰는 live hc 터미널이 있음(경로는 실제 경로로 견줌)
  보류(찍기만): 작업 트리가 있는데 본 저장소(<저장소>) 아래를 쓰는 터미널이 있음(감사 스냅숏 꼴), 작업 트리가 있는데 cwd로만 걸림, 자손에 live 터미널 프로세스가 있음, 터미널 pid를 못 읽음
  끔: 그 밖(작업 트리가 지워졌거나 쓰는 터미널 없음) — 그 프로세스와 자손에 TERM, 5초 뒤 남으면 KILL.
끄기는 설정 sweep.orphan_kill on일 때만이다. 기본 off는 찍고 closed.log에 「고아 관찰」로 한 번만 남긴다. 끈 것마다 pid·나이·명령 앞 80자를 closed.log에 남긴다. 작업 트리 밖(launchd 작업·로그인 항목·앱)은 보지 않는다.
hc ls가 실패하거나 terminals 목록이 아니거나, cwd 모르는 이 기계 터미널이 있거나, 설정을 못 읽으면 아무것도 끄지 않는다 (2026-11-16 시험이 남긴 hc-fill watch 고아 18개가 작업 트리를 지운 뒤에도 55분 돌았다).

색인 쓸기: codebase-memory 색인 폴더(${XDG_CACHE_HOME:-~/.cache}/codebase-memory-mcp) 바로 아래 <이름>.db(_config.db 빼고)
가운데 projects 표의 root_path(빈 값 빼고)가 모두 지워진 것을 -wal·-shm·-journal과 함께 지우고 closed.log에 이름·root·크기를 남긴다.
지운 워크트리의 색인이 기계마다 수십 GB 쌓였다(2026-11-18 — teardown이 없는 옛 워크트리, 손으로 지운 워크트리, teardown 뒤
끝난 배경 색인). 건너뜀: mtime(.db·-journal·비지 않은 -wal)이 --min-age분 안(색인 중), 읽기 전용으로 못 열거나 표·줄 없음, root가 /Volumes/
아래(안 붙은 디스크 — 링크를 풀어서도 본다), root가 하나라도 있거나 읽기 오류(TCC 등 — exists()와 같은 뜻). 터미널·고아 쓸기와 따로 돈다: hc ls가 실패해도 돌고, 이 쓸기의 오류는 나머지를 막지 않는다.

디스크 쓸기(임시 찌꺼기 go-build*·Test* 지우기, 디스크 부족 알림 hc-disk-low): lib/hc-sweep/disk.py 머리글. 색인 쓸기처럼 따로 돈다.

두 판 공통 잠금: --dry-run이 아니면 ${XDG_STATE_HOME:-~/.local/state}/hc-sweep/sweep.lock을 flock 배타로 잡는다(Go 판과 같은 파일·같은 뜻).
남이 쥐면 한 줄 찍고 종료 75(다음 바퀴에), 잠금 파일을 못 만들면 경고하고 돈다.

알려진 차이(2026-11-21 Go 전환 감사 base — Go 판만 고쳤다, 셸 판은 곧 걷는다): Go 판은 닫기 직전 hc get으로 같은 실행·같은 상태인지
다시 보고(B04), kill 뒤 hc get이 not_found가 아닌 실패면 닫힘이 아니라 모름(B05 — 작업 트리 정리 안 함), keep을 못 읽으면 그 바퀴
닫지 않고(E12), 빈 화면은 모름(E08), 폴더가 있는데 git cherry가 실패하면 모름(E07), 착지 증거는 이 실행 키·이 터미널 뒤의 사건·원격
표식만(B03), 고아는 시작 시각까지 견주고(E04) 기록에 명령 인자를 남기지 않으며(E05), lsof·ps 출력이 온전하지 않으면 그 정리를
건너뛰고(E13·B22), 디스크 알림은 대기 표식 → 보고 → 보냄 순서로 같은 키를 다시 쓴다(E06). 자식은 두 판 모두 쓸기와 같은 무리다(B19).
"""
import sys

sys.dont_write_bytecode = True  # lib/hc-sweep에 __pycache__를 남기지 않게

from common import *  # noqa: E402
from index import sweep_index  # noqa: E402
from orphans import sweep_orphans  # noqa: E402


def is_worker(t):
    """지휘가 띄운 worker(parent 있음) 또는 hc-lane 레인(라벨 hc-lane=1 — hc-fill 레인은 parent가 없다)."""
    return bool(t.get("parent")) or (t.get("labels") or {}).get("hc-lane") == "1"


def rule(t, now, keep):
    """닫을 규칙(R1·R2·R3) 또는 None."""
    return judge(t, now, keep)[0]


def judge(t, now, keep):
    """(닫을 규칙 또는 None, 남기는 까닭 — --dry-run이 찍는다)."""
    status = t.get("status")
    name = t.get("name") or ""
    labels = t.get("labels") or {}
    worker = is_worker(t)
    for bad, why in ((t.get("state") != "live" or "/" in t["address"], "live 아님·다른 기계"),
                     (not isinstance(status, dict) or not status.get("app"), "에이전트 상태 없음"),
                     (isinstance(status, dict) and status.get("state") not in ("idle", "done"), "idle·done 아님"),
                     ("loop" in name or name.endswith("-proxy"), "보호: 이름(loop·-proxy)"),
                     (name in keep or t["address"] in keep, "보호: keep 목록"),
                     (name.startswith("lane-watch-") or labels.get("hc-lane-watch") == "1", "보호: hc-lane 감시"),
                     ((t.get("where") or labels.get("where")) and not worker, "보호: 사람이 연 칸(where)"),
                     (t.get("shell"), "보호: 셸 칸")):
        if bad:
            return None, why
    created = t.get("created_ms") or now
    changed = t.get("status_ms") or now
    if not worker and t.get("turn") == 0 and changed - created < MIN and now - created >= 30 * MIN:
        return "R1", ""
    if t.get("worktree") is not False:  # 남은 워크트리이거나 모름
        return None, "작업 트리 남음" + (" — " + t["wtwhy"] if t.get("wtwhy") else "")
    idle = (now - changed) // MIN
    if worker:
        for bad, why in ((now - changed < (0 if t.get("submitted") else 30 * MIN), "쉰 지 %d분(R2 30분 전)" % idle),
                         (t.get("unlanded") is not False, "착지 전 커밋(git cherry +) 또는 모름"),
                         (t.get("fill") == "up", "hc-fill이 결과를 되돌려 주기 전"),
                         (not isinstance(t.get("tail"), str) or BUSY.search(t["tail"]), "배경 실행 흔적·화면 못 읽음")):
            if bad:
                break
        else:
            return "R2", ""
    else:
        why = "worker 아님"
    if now - changed >= 6 * 60 * MIN:
        return "R3", ""
    return None, "%s, 쉰 지 %d분(R3 6시간 전)" % (why, idle)


def landed_clean(t, run=subprocess.run, events=None):
    """링크된 작업 트리(cwd)의 레인이 착지했고 작업 트리가 깨끗하면 (메인 저장소, 가지, ""), 아니면 (None, None, 까닭).
    착지 = git cherry origin/main HEAD에 + 없음(fetch 안 함) + 착지 증거(이 레인 이름의 hc-lane 사건 착지·제출·완료·판정,
    없으면 원격 표식 refs/hc-landed/<가지>/*). 깨끗함 = git status --porcelain --untracked-files=all이 빔. git은 hc exec로(TCC).
    hc-sweep(작업 트리가 남아 터미널을 남김)과 hc-lane gc(터미널이 그 폴더를 써서 작업 트리를 남김)가 서로 기다리던 교착을 푼다
    (2026-11-21 mini-m6-1 land-metrics 16시간·land-stale-lows 14시간). 모르는 값은 남김."""
    cwd, name = t.get("cwd"), t.get("name") or ""

    def git(*a, timeout=20):
        out = run(["hc", "exec", "--cwd", cwd, "--env", "LC_ALL=C", "--timeout", str(timeout), "--", "git", *a],
                  capture_output=True, text=True, stdin=subprocess.DEVNULL, timeout=timeout + 20)
        if out.returncode:
            raise RuntimeError("git %s 실패" % a[0])
        return out.stdout

    try:
        if git("status", "--porcelain", "--untracked-files=all").strip():
            return None, None, "커밋 안 된 변경"
        branch = git("symbolic-ref", "--short", "-q", "HEAD").strip()
        main = os.path.dirname(git("rev-parse", "--path-format=absolute", "--git-common-dir").strip())
        if any(x.startswith("+") for x in git("cherry", "origin/main", "HEAD").splitlines()):
            return None, None, "착지 전 커밋(git cherry +)"
        try:
            with open(events or os.path.join(LANE_DIR, "events.tsv")) as f:
                said = any(len(r) > 4 and r[3] == name and r[4] in ("착지", "제출", "완료", "판정")
                           for r in (x.rstrip("\n").split("\t") for x in f))
        except FileNotFoundError:
            said = False
        if not said and not git("ls-remote", "origin", "refs/hc-landed/%s/*" % branch, timeout=30).strip():
            return None, None, "착지 증거 없음(사건·원격 표식)"
    except Exception as err:
        return None, None, "모름(%s)" % str(err)[:80]
    if not branch or not main.startswith("/"):
        return None, None, "가지·메인 저장소 모름"
    return main, branch, ""


def lane_clean(main, branch, run=subprocess.run):
    """닫은 레인의 작업 트리·가지를 hc-lane clean으로(teardown → worktree remove → branch -D). (성공, 한 줄)."""
    try:
        out = run(["hc", "exec", "--cwd", main, "--timeout", "300", "--", os.path.join(BIN, "hc-lane"), "clean",
                   "--repo", main, "--branch", branch], capture_output=True, text=True, stdin=subprocess.DEVNULL, timeout=340)
    except Exception as err:
        return False, str(err)[:200]
    lines = [x for x in (out.stderr + out.stdout).splitlines() if x.strip()]
    return out.returncode == 0, (lines[0] if lines else "-")[:200]


def linked_worktree(cwd):
    """cwd가 아직 있는 링크된 git 워크트리인가(.git이 파일). cwd가 없어졌거나 일반 폴더·메인 저장소면 False.
    모르면(cwd 없음·읽기 오류) True(닫지 않는 쪽). 파일 검사만 한다(launchd의 git은 TCC에 멈춘다)."""
    if not isinstance(cwd, str) or not cwd.startswith("/"):
        return True
    try:
        os.stat(cwd)
    except FileNotFoundError:
        return False  # 착지 뒤 지운 워크트리
    except OSError:
        return True
    d = os.path.normpath(cwd)
    while True:
        try:
            st = os.lstat(os.path.join(d, ".git"))
            return not stat.S_ISDIR(st.st_mode)
        except FileNotFoundError:
            pass
        except OSError:
            return True
        if d == "/":
            return False
        d = os.path.dirname(d)


def fill_state(name, address, path=None):
    """hc-fill 대기열에서 이 이름·주소의 받은편지 레인 상태(up·sent 등). 줄이 없거나 되돌릴 곳(11째 칸)이 없으면 None.
    파일이 있는데 못 읽으면 "up"(닫지 않는 쪽)."""
    path = path or os.path.join(os.environ.get("HC_FILL_DATA") or os.path.expanduser("~/.local/share/hc-fill"), "queue.tsv")
    try:
        with open(path) as f:
            rows = [line.rstrip("\n").split("\t") for line in f]
    except FileNotFoundError:
        return None
    except (OSError, UnicodeDecodeError):
        return "up"
    for r in rows:
        if len(r) >= 11 and r[0] == name and r[7] == address and r[10] not in ("", "-"):
            return r[5]
    return None


def submitted(t, path=None):
    """그 터미널(주소·이름, 실행 키 칸이 있으면 <주소>@<created_ms>)의 마지막 hc-lane 사건(정리됨 빼고)이 「제출」인가(못 읽으면 False).
    제출 뒤 레인이 할 일은 없다 — 결과는 병합기 사건(2026-11-21 fill-submit-release, 유예가 자리를 61~231분 쥠)."""
    try:
        with open(path or os.path.join(LANE_DIR, "events.tsv")) as f:
            ev = [r[4] for r in (x.rstrip("\n").split("\t") for x in f) if len(r) > 4 and r[2] == t["address"] and r[4] != "정리됨"
                  and r[3] == (t.get("name") or "-") and (len(r) < 10 or r[9] in ("-", "%s@%s" % (t["address"], t.get("created_ms"))))]
    except (OSError, UnicodeDecodeError):
        return False
    return ev[-1:] == ["제출"]


def unlanded(cwd, run=subprocess.run):
    """cwd 저장소에 origin/main에 없는 커밋이 있나(로컬 origin/main 기준, fetch 안 함).
    cwd가 없거나 git 저장소가 아니면 False. 그 밖에 판정이 안 되면 True(닫지 않는 쪽).
    git은 hc exec로 코어가 돌린다: launchd가 띄운 git은 ~/Documents에서 macOS 개인정보 보호(TCC)에
    걸려 멈춘다(2026-11-13 워커 맥, 시간 초과를 「착지함」으로 읽어 레인 t134를 닫았다)."""
    if not cwd:
        return False
    try:
        out = run(["hc", "exec", "--cwd", cwd, "--env", "LC_ALL=C", "--timeout", "20", "--",
                   "git", "cherry", "origin/main", "HEAD"], capture_output=True, text=True,
                  stdin=subprocess.DEVNULL, timeout=40)
    except Exception:
        return True
    if out.returncode == 0:
        return any(line.startswith("+") for line in out.stdout.splitlines())
    return not ("not a git repository" in out.stderr or "No such file or directory" in out.stderr)


def screen_tail(address, run=subprocess.run):
    """화면 마지막 비지 않은 6줄. 못 읽으면 BUSY에 걸리는 값(닫지 않는 쪽)."""
    try:
        out = run(["hc", "read", address], capture_output=True, text=True, stdin=subprocess.DEVNULL, timeout=40)
    except Exception:
        out = None
    if not out or out.returncode:
        return "still running (화면 못 읽음)"
    return "\n".join([line for line in out.stdout.splitlines() if line.strip()][-6:])


def session_id(pid):
    """pid와 그 자손 가운데 ~/.claude/sessions/<pid>.json이 있는 것의 sessionId."""
    children = {}
    ps = subprocess.run(["ps", "-A", "-o", "pid=,ppid="], capture_output=True, text=True).stdout
    for line in ps.splitlines():
        child, parent = line.split()
        children.setdefault(int(parent), []).append(int(child))
    todo = [pid] if pid else []
    while todo:
        p = todo.pop(0)
        try:
            with open(os.path.join(SESSIONS, "%d.json" % p)) as f:
                return json.load(f).get("sessionId") or "-"
        except (OSError, ValueError):
            todo += children.get(p, [])
    return "-"


def keep_list():
    try:
        with open(os.path.join(TASKS, "keep")) as f:
            return {line.strip() for line in f if line.strip() and not line.startswith("#")}
    except OSError:
        return set()


def lane_event(t, why):
    """레인 터미널(라벨 hc-lane=1)이면 hc-lane events.tsv에 「정리됨」 한 줄(hc-lane report와 같은 8칸, turn은 닫을 때 것).
    hc-lane wait는 앞 report가 착지·판정 그 턴(turn+1 이하)이면 조용히, 아니면 「뜻밖의 종료(정리됨)」로 알리고, 뒤따른
    끝남은 다시 알리지 않는다. kill 전에 적는다(그 사이 wait가 끝남을 먼저 「뜻밖의 종료(finished)」로 보지 않게).
    레인 아님·적음이면 True, 못 쓰면 False — 그때는 닫지 않는다: 닫으면 wait가 reason killed(parent 없음·살아 있음)를
    지휘의 hc kill로 보고 조용히 넘겨 뜻밖의 종료 알림이 사라진다."""
    if (t.get("labels") or {}).get("hc-lane") != "1":
        return True
    flat = lambda x: str(x).replace("\t", " ").replace("\r", " ").replace("\n", " ")
    fields = [time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
              os.environ.get("HC_LANE_MACHINE") or socket.gethostname().split(".")[0] or "-",
              t["address"], t.get("name") or "-", "정리됨", "-", "hc-sweep " + why, "-" if t.get("turn") is None else t["turn"]]
    try:
        os.makedirs(LANE_DIR, exist_ok=True)
        with open(os.path.join(LANE_DIR, "events.tsv"), "a") as f:
            f.write("\t".join(flat(x) for x in fields) + "\n")
    except OSError:
        return False
    return True


def close(t, why):
    address = t["address"]
    record(t, why, session_id(t.get("pid")))  # 먼저: closed.log를 못 쓰면 정리됨도 안 남기고 멈춘다(감사 [낮음] 2)
    if not lane_event(t, why):
        record(t, why + " 실패(정리됨 못 씀)", "-")
        return "실패(정리됨 못 씀)"
    try:
        hc("kill", address)
    except RuntimeError as err:
        record(t, why + " 실패(kill): " + str(err), "-")
        return "실패(kill)"
    for _ in range(30):
        try:
            if hc("get", address).get("state") == "finished":
                break
        except RuntimeError:
            return "닫힘(이미 지워짐)"  # 감싸는 실행기가 끝날 때 자기 터미널을 지우기도 한다
        time.sleep(1)
    else:
        record(t, why + " 실패(30초 안에 안 끝남)", "-")
        return "실패(안 끝남)"
    try:
        hc("rm", address)
    except RuntimeError as err:
        record(t, why + " 실패(rm): " + str(err), "-")
        return "실패(rm)"
    return "닫힘"


def main():
    parser = argparse.ArgumentParser(description="끝났는데 안 닫힌 hc 에이전트 터미널을 닫는다")
    parser.add_argument("--dry-run", action="store_true", help="닫을 것과 이유만 출력")
    parser.add_argument("--self-test", action="store_true")
    parser.add_argument("--min-age", type=float, default=10, help="고아·색인 최소 나이(분, 기본 10)")
    parser.add_argument("--only-under", help="이 폴더 아래 작업 트리만(시험용)")
    args = parser.parse_args()
    if args.self_test:
        from selftest import self_test
        return self_test()
    if not args.dry_run:  # 두 판 공통 잠금(머리글) — 파이썬이 연 fd는 자식에 물리지 않는다
        import fcntl
        d = os.path.join(os.environ.get("XDG_STATE_HOME") or os.path.expanduser("~/.local/state"), "hc-sweep")
        try:
            os.makedirs(d, exist_ok=True)
            lock = open(os.path.join(d, "sweep.lock"), "a")
        except OSError as err:
            print("hc-sweep: 경고 — 잠금을 못 잡음(그대로 돈다):", err, file=sys.stderr)
        else:
            try:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError:
                print("hc-sweep: 다른 쓸기가 도는 중(sweep.lock) — 이번 바퀴는 건너뜀", file=sys.stderr)
                sys.exit(75)
            main.lock = lock  # 끝날 때까지 쥔다
    for what, sweep in (("색인", lambda: sweep_index(args.dry_run, args.min_age * 60)),
                        ("디스크", lambda: __import__("disk").run(args.dry_run, conf, record, os.path.join(BIN, "hc-lane")))):
        try:  # 색인·디스크 쓸기는 따로: hc ls 실패와 상관없이 돌고, 실패해도 서로·터미널·고아 쓸기를 막지 않는다
            sweep()
        except Exception as err:
            print(what, "쓸기 실패:", err)
    keep, now = keep_list(), time.time() * 1000
    terms = hc("ls").get("terminals")
    listed = [t for t in (terms if isinstance(terms, list) else []) if isinstance(t, dict)
              and t.get("state") == "live" and "/" not in t["address"] and t.get("status") in ("idle", "done")]
    closed = targets = 0
    for brief in listed:
        try:
            t = hc("get", brief["address"])
        except RuntimeError:
            if args.dry_run:
                print("남김: %s %s — hc get 실패" % (brief["address"], brief.get("name") or "-"))
            continue
        t["worktree"] = linked_worktree(t.get("cwd"))
        wt = None
        if is_worker(t) and t["worktree"] and (submitted(t) or now - (t.get("status_ms") or now) >= 30 * MIN):
            main_repo, branch, t["wtwhy"] = landed_clean(t)  # 착지한 깨끗한 작업 트리: 닫고 hc-lane clean(교착 풀기)
            if main_repo:
                t["worktree"], t["unlanded"], wt = False, False, (main_repo, branch)
        if is_worker(t) and not t["worktree"]:  # R2 보호에만 쓴다
            if not wt:
                t["unlanded"] = unlanded(t.get("cwd"))
            t["tail"] = screen_tail(t["address"])
            t["fill"], t["submitted"] = fill_state(t.get("name") or "", t["address"]), submitted(t)
        why, kept = judge(t, now, keep)
        if not why:
            if args.dry_run:
                print("남김: %s %s — %s" % (t["address"], t.get("name") or "-", kept))
            continue
        age = "만든 지 %d분·상태 그대로 %d분" % ((now - t["created_ms"]) // MIN, (now - t["status_ms"]) // MIN)
        line = "%s %s %s (%s, turn %s) %s" % (why, t["address"], t.get("name") or "-", age, t.get("turn"), t.get("cwd"))
        targets += 1
        if args.dry_run:
            print("닫을 것:", line + (" (닫은 뒤 hc-lane clean %s)" % wt[1] if wt else ""))
            continue
        result = close(t, why)
        closed += result.startswith("닫힘")
        print(result + ":", line)
        if wt and result.startswith("닫힘"):
            ok, msg = lane_clean(*wt)
            record({"address": t["address"], "name": t.get("name"), "cwd": wt[0], "title": msg},
                   "작업 트리 정리(hc-lane clean %s)%s" % (wt[1], "" if ok else " 실패"), "-")
            print("작업 트리 정리%s: %s %s — %s" % ("" if ok else " 실패", wt[0], wt[1], msg))
    print("%s 이 기계 idle·done %d개 확인, %s %d개" % (time.strftime("%Y-%m-%d %H:%M:%S"), len(listed),
          "닫을 것" if args.dry_run else "닫음", targets if args.dry_run else closed))
    sweep_orphans(terms, args.dry_run, args.min_age * 60, args.only_under)


if __name__ == "__main__":
    main()
