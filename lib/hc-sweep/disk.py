"""hc-sweep 디스크 쓸기 (bin/hc-sweep가 바퀴마다 부른다. 터미널·고아 쓸기와 따로 돈다).

임시 찌꺼기: 임시 폴더($TMPDIR, 없으면 getconf DARWIN_USER_TEMP_DIR, 없으면 tempfile) 바로 아래 이름이 go-build*·Test*인
  폴더(링크 아님, 내 것) 가운데 그 아래 모든 항목의 mtime이 설정 sweep.tmp_hours시간(기본 3, 0이면 끔) 넘게 지났고
  살아 있는 어떤 프로세스도 열고 있지 않은 것(lsof 한 번 — 열린 파일·cwd·실행 파일. lsof가 실패하면 하나도 안 지움)만 지운다.
  go test가 끝나도 남긴 go-build*·t.TempDir(Test*)가 기계마다 GB씩 쌓여 검증이 ENOSPC로 멈췄다(2026-11-21 워커 맥 여유 1GB).
  다른 이름은 건드리지 않는다. 지웠으면 closed.log에 「임시 찌꺼기 지움」 한 줄(개수·크기).
디스크 부족 알림: 홈이 있는 볼륨 여유가 설정 sweep.disk_free_gb(기본 10, 0이면 끔) 아래면 hc-lane report 중간 --name hc-disk-low
  사건 하나(주인 없는 사건이라 살아 있는 지휘 watch가 모두 받는다. 터미널 밖이라 watch 줄 끝 괄호에 기계 이름). 한 줄 칸에
  여유·볼륨 사용률(df처럼 사용/(사용+여유))과 임시 폴더·~/Library/Caches/go-build·~/.cache/go-build 크기. 알린 표식(…/hc-sweep/disk-low)이 있으면 다시 알리지 않고, 여유가 문턱의 1.5배를 넘으면 표식을 지워
  다음 부족 때 또 알린다(문턱 언저리에서 오르내릴 때 되풀이하지 않게). 보내기 실패면 표식 없이 다음 바퀴에 다시.
  알림 자체는 Go 캐시를 지우지 않고 크기만 보인다 — ~/Library/Caches/go-build는 macOS 기본 GOCACHE라 GOCACHE를 옮긴 저장소가
  있어도 다른 go가 다시 채울 수 있다(2026-11-21 지운 뒤 다시 1.5GB). 줄이는 것은 아래 상한뿐이다(캐시라 지워도 다시 지을 뿐).
Go 빌드 캐시 상한: 여유가 문턱의 1.5배 아래면(문턱 0이면 끔) GOCACHE(없으면 ~/.cache/go-build — 병합기 기본과 같음)의
  <두 자리 16진>/ 아래 항목을 mtime 오래된 것부터 지워 설정 sweep.gocache_gb(기본 6, 0이면 끔) 아래로 줄이고, 옛 자리
  ~/Library/Caches/go-build가 GOCACHE와 다른 곳이면 통째로 비운다. go 명령(ps comm go)이 돌면 건너뛴다. 한 일은 closed.log에
  「Go 캐시 줄임」 한 줄(2026-11-21 mini-m4 여유 9GB: ~/.cache/go-build가 하루 13.4GB, 옛 자리 1.5GB).
"""
import os
import re
import shutil
import stat
import subprocess
import tempfile
import time

GB = 1e9
SHOW = ("~/Library/Caches/go-build", "~/.cache/go-build")


def tmp_dir(run=subprocess.run):
    if os.environ.get("TMPDIR"):
        return os.environ["TMPDIR"]
    try:  # launchd 작업에는 TMPDIR가 없을 수 있다. go는 터미널의 TMPDIR(= 이 값)를 쓴다
        out = run(["getconf", "DARWIN_USER_TEMP_DIR"], capture_output=True, text=True, timeout=10)
        if out.returncode == 0 and out.stdout.strip().startswith("/"):
            return out.stdout.strip()
    except Exception:
        pass
    return tempfile.gettempdir()


def fail(err):
    raise err


def scan(path, cutoff):
    """(바이트, 오래됨). 하나라도 cutoff보다 새로우면 오래됨 False로 바로 끝. 못 읽는 곳은 OSError(건너뜀)."""
    size = 0
    for root, dirs, files in os.walk(path, onerror=fail):
        for name in [""] + dirs + files:
            st = os.lstat(os.path.join(root, name))
            if st.st_mtime > cutoff:
                return size, False
            size += st.st_blocks * 512
    return size, True


def remove(path):
    """go 모듈 캐시를 품은 t.TempDir는 폴더가 0555라 rmtree가 못 지운다: 쓰기 막힌 폴더를 u+rwx로 연 뒤 지운다."""
    for root, dirs, _ in os.walk(path):
        for d in [root] + [os.path.join(root, x) for x in dirs]:
            st = os.lstat(d)
            if stat.S_ISDIR(st.st_mode) and not st.st_mode & stat.S_IWUSR:
                os.chmod(d, st.st_mode | stat.S_IRWXU)
    shutil.rmtree(path)


def open_paths(run=subprocess.run):
    """살아 있는 프로세스가 연 경로(실제 경로) 집합. 실패면 None."""
    # lsof는 /usr/sbin에 있는데 launchd 작업의 PATH에는 /usr/sbin이 없다(감사 중간 1). PATH가 먼저(시험의 가짜 lsof)
    lsof = shutil.which("lsof", path=os.environ.get("PATH", "") + ":/usr/sbin:/sbin") or "lsof"
    try:
        out = run([lsof, "-n", "-P", "-w", "-F", "n"], capture_output=True, text=True, timeout=60)
    except Exception:
        return None
    if out.returncode != 0 or not out.stdout:
        return None
    return {line[1:] for line in out.stdout.splitlines() if line.startswith("n/")}


def sweep_tmp(base, hours, dry, record, run=subprocess.run):
    """임시 찌꺼기 한 바퀴. 지운(dry면 지울) (개수, 바이트)."""
    try:
        names = sorted(os.listdir(base))
    except OSError:
        return 0, 0
    base, cutoff, uid = os.path.realpath(base), time.time() - hours * 3600, os.getuid()
    old = []
    for name in names:
        p = os.path.join(base, name)
        try:
            st = os.lstat(p)
            if not name.startswith(("go-build", "Test")) or not stat.S_ISDIR(st.st_mode) or st.st_uid != uid:
                continue
            size, aged = scan(p, cutoff)
        except OSError:
            continue
        if aged:
            old.append((p, size))
    if not old:
        return 0, 0
    held = open_paths(run)
    if held is None:
        print("임시 찌꺼기: lsof 실패 — 건너뜀")
        return 0, 0
    n = total = 0
    for p, size in old:
        if any(h == p or h.startswith(p + "/") for h in held):
            continue
        if not dry:
            try:
                remove(p)
            except OSError as err:
                print("임시 찌꺼기 못 지움: %s %s" % (p, err))
                continue
        n, total = n + 1, total + size
        print("지울 임시:" if dry else "지운 임시:", p)
    if n and not dry:
        record({"address": "임시:" + base, "name": "-", "cwd": base, "title": "%d개 %.1fGB" % (n, total / GB)},
               "임시 찌꺼기 지움(%s시간 넘음)" % hours, "-")
    if n:
        print("임시 찌꺼기: %s %d개(%.1fGB)" % ("지울 것" if dry else "지움", n, total / GB))
    return n, total


def du(path, run=subprocess.run):
    try:
        out = run(["du", "-sk", path], capture_output=True, text=True, timeout=300)
        return "%.1fGB" % (int(out.stdout.split()[0]) * 1024 / GB)
    except Exception:
        return "?" if os.path.exists(path) else "없음"


def disk_alert(usage, limit_gb, state, base, dry, lane, run=subprocess.run):
    """부족 알림 한 바퀴(usage = shutil.disk_usage). 보냈으면(dry면 보낼 것이면) 그 한 줄, 아니면 None."""
    free = usage.free
    if free >= limit_gb * 1.5 * GB:
        if os.path.exists(state) and not dry:
            os.remove(state)
        return None
    if free >= limit_gb * GB or os.path.exists(state):
        return None
    home = os.path.expanduser("~")
    short = lambda p: "~" + p[len(home):] if p.startswith(home + "/") else p
    sizes = " · ".join("%s %s" % (short(p.rstrip("/")), du(p, run))
                       for p in [base] + [os.path.expanduser(x) for x in SHOW])
    pct = 100 * usage.used / ((usage.used + free) or 1)
    msg = ("디스크 여유 %.1fGB(사용 %.0f%%) < 문턱 %sGB — %s (임시 찌꺼기는 hc-sweep이 지움, Go 캐시는 쓰는 곳이 없을 때만 손으로"
           " — ~/Library/Caches/go-build는 macOS 기본 GOCACHE)") % (free / GB, pct, limit_gb, sizes)
    if dry:
        return msg
    # 표식을 먼저 쓴다. 디스크가 차서 못 쓰면 날짜 키로 보내 hc-lane이 그날 두 번 적지 않게 한다(감사 낮음 2)
    try:
        os.makedirs(os.path.dirname(state), exist_ok=True)
        with open(state, "w") as f:
            f.write(time.strftime("%Y-%m-%dT%H:%M:%S%z") + "\t" + msg + "\n")
        key = []
    except OSError:
        key = ["--key", "hc-disk-low:" + time.strftime("%Y-%m-%d")]
    env = {k: v for k, v in os.environ.items() if k not in ("HC_TERMINAL_ID", "HC_LANE", "HC_LANE_OWNER")}
    try:
        out = run([lane if os.access(lane, os.X_OK) else "hc-lane", "report", "중간", "-", msg, "--name", "hc-disk-low"] + key,
                  capture_output=True, text=True, stdin=subprocess.DEVNULL, timeout=60, env=env)
        ok = out.returncode == 0
    except Exception:
        ok = False
    if not ok:
        print("디스크 부족 알림 실패(hc-lane report) — 다음 바퀴에 다시")
        if not key:
            os.remove(state)
        return None
    return msg


def go_busy(run=subprocess.run):
    """go 명령(빌드·시험)이 도는가(ps comm 이름이 go — 시험은 HC_SWEEP_GO_COMM으로 바꾼다). 못 보면 True(건드리지 않음)."""
    name = os.environ.get("HC_SWEEP_GO_COMM") or "go"
    try:
        out = run(["ps", "-x", "-o", "comm="], capture_output=True, text=True, timeout=30)
    except Exception:
        return True
    return out.returncode != 0 or any(os.path.basename(x.strip()) == name for x in out.stdout.splitlines())


def trim_gocache(cap_gb, dry, record, run=subprocess.run):
    """Go 빌드 캐시 줄이기 한 바퀴(부르는 쪽이 디스크가 문턱 가까울 때만). 지운(dry면 지울) 바이트."""
    cache = os.environ.get("GOCACHE") or os.path.expanduser("~/.cache/go-build")
    old = os.path.expanduser("~/Library/Caches/go-build")
    if go_busy(run):
        print("Go 캐시 줄이기: go가 도는 중 — 건너뜀")
        return 0
    files = []
    for root, _, names in os.walk(cache):
        if not re.fullmatch(r"[0-9a-f]{2}", os.path.basename(root)):  # 항목은 <캐시>/<두 자리 16진>/ 아래(README·trim.txt는 둔다)
            continue
        for n in names:
            try:
                st = os.lstat(os.path.join(root, n))
            except OSError:
                continue
            if stat.S_ISREG(st.st_mode):
                files.append((st.st_mtime, st.st_blocks * 512, os.path.join(root, n)))
    over, n, freed = sum(f[1] for f in files) - cap_gb * GB, 0, 0
    for _, size, p in sorted(files):  # go는 쓸 때 mtime을 새로 한다: 오래된 것부터
        if freed >= over:
            break
        if not dry:
            try:
                os.remove(p)
            except OSError:
                continue
        n, freed = n + 1, freed + size
    old_size = 0
    if os.path.isdir(old) and not os.path.islink(old) and os.path.realpath(old) != os.path.realpath(cache):
        try:
            old_size = scan(old, time.time() + 86400)[0]
            if not dry:
                remove(old)
        except OSError as err:
            print("Go 옛 캐시 못 비움: %s %s" % (old, err))
            old_size = 0
    if n or old_size:
        msg = "캐시 %s 항목 %d개 %.1fGB(상한 %gGB)%s" % (cache, n, freed / GB, cap_gb,
                                                 " · 옛 캐시 %s %.1fGB" % (old, old_size / GB) if old_size else "")
        print("Go 캐시 줄이기%s: %s" % (" (할 것)" if dry else "", msg))
        if not dry:
            record({"address": "go-cache:" + cache, "name": "-", "cwd": cache, "title": msg}, "Go 캐시 줄임(디스크 문턱 가까움)", "-")
    return freed + old_size


def run(dry, conf, record, lane):
    """bin/hc-sweep main이 부른다(lane = 곁의 hc-lane, 없으면 PATH). 설정을 못 읽으면(None) 그 쓸기는 건너뛴다."""
    def num(key, default):  # 숫자가 아니면 그 키만 끈다(감사 낮음 1)
        v = conf(key, default)
        try:
            return float(v) if v is not None else 0
        except ValueError:
            print("디스크 쓸기: %s 값이 숫자가 아님(%s) — 그 쓸기만 끔" % (key, v))
            return 0

    base = tmp_dir()
    hours = num("sweep.tmp_hours", "3")
    if hours > 0:
        sweep_tmp(base, hours, dry, record)
    limit = num("sweep.disk_free_gb", "10")
    if limit > 0:
        cap = num("sweep.gocache_gb", "6")
        if cap > 0 and shutil.disk_usage(os.path.expanduser("~")).free < limit * 1.5 * GB:
            trim_gocache(cap, dry, record)
        state = os.path.join(os.environ.get("XDG_STATE_HOME") or os.path.expanduser("~/.local/state"), "hc-sweep", "disk-low")
        msg = disk_alert(shutil.disk_usage(os.path.expanduser("~")), limit, state, base, dry, lane)
        if msg:
            print("디스크 부족 알림%s:" % (" (보낼 것)" if dry else ""), msg)
