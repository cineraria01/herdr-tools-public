"""hc-sweep 셸 판: 색인 쓸기 (머리글은 sweep.py)."""
from common import *


def index_gone(roots, exists=exists):
    """색인의 원본이 모두 사라졌나. 빈 목록·상대 경로·/Volumes/ 아래·하나라도 있음(읽기 오류 포함)이면 False.
    /Volumes는 실제 경로로도 본다: 링크를 거쳐 빠진 디스크로 가면 stat이 FileNotFoundError라 지울 뻔했다(감사 [중간])."""
    return (bool(roots) and all(isinstance(r, str) and r.startswith("/") and not r.startswith("/Volumes/")
                                and not os.path.realpath(r).startswith("/Volumes/") for r in roots)
            and not any(exists(r) for r in roots))


def index_roots(db):
    """색인 DB의 root_path들(빈 값 빼고). 열기·읽기 실패·표 없음은 [](건너뜀)."""
    try:
        con = sqlite3.connect(pathlib.Path(db).as_uri() + "?mode=ro", uri=True, timeout=2)
        try:
            return [r[0] for r in con.execute("SELECT root_path FROM projects WHERE root_path <> ''")]
        finally:
            con.close()
    except Exception:
        return []


def sweep_index(dry, min_age):
    """색인 쓸기 한 바퀴. 지운(dry면 지울) 수."""
    d = os.path.join(os.environ.get("XDG_CACHE_HOME") or os.path.expanduser("~/.cache"), "codebase-memory-mcp")
    try:
        names = sorted(os.listdir(d))
    except FileNotFoundError:
        return 0
    n = size = 0
    for f in names:
        db = os.path.join(d, f)
        if not f.endswith(".db") or f == "_config.db":
            continue
        try:
            mb, roots = sweep_one(db, dry, min_age)
        except Exception as err:  # 이 DB만 건너뛴다(다음 바퀴에 다시)
            print("색인 쓸기 실패: %s %s" % (f[:-3], err))
            continue
        if roots is not None:
            print("지울 색인:" if dry else "지운 색인:", "%s %s 크기 %.1fMB" % (f[:-3], " ".join(roots), mb))
            n += 1
            size += mb
    if n or dry:  # 0개면 조용히(기계마다 하루 288줄)
        print("색인 쓸기: %s %d개(%.1fMB)" % ("지울 것" if dry else "지움", n, size))
    return n


def sweep_one(db, dry, min_age):
    """색인 DB 하나. 지웠으면(dry면 지울 것이면) (MB, roots), 아니면 (0, None).
    나이는 열기 전에 .db·-journal·비지 않은 -wal로 본다. -shm과 빈 -wal은 뺀다: 읽기 전용으로 열기만 해도 SQLite가
    -shm을 다시 쓰고 빈 -wal을 만들어서, 넣으면 매 바퀴 「최근」이 된다. 곁 파일은 연 뒤 다시 모으고 .db를 맨 끝에 지운다
    (중간에 실패하면 .db가 남아 다음 바퀴에 다시 본다)."""
    side = lambda: [x for x in (db + "-wal", db + "-shm", db + "-journal") if os.path.lexists(x)]
    if not os.path.isfile(db):
        return 0, None
    aged = [db] + [x for x in side() if not x.endswith("-shm") and not (x.endswith("-wal") and os.path.getsize(x) == 0)]
    if time.time() - max(os.stat(x).st_mtime for x in aged) < min_age:
        return 0, None
    roots = index_roots(db)
    if not index_gone(roots):
        return 0, None
    files = side() + [db]
    mb = sum(os.path.getsize(x) for x in files if os.path.isfile(x)) / 1e6
    if not dry:
        for x in files:
            try:
                os.remove(x)
            except FileNotFoundError:
                pass
        record({"address": "색인:" + os.path.basename(db)[:-3], "name": "-", "cwd": " ".join(roots),
                "title": "크기 %.1fMB" % mb}, "색인 지움(원본 없음)", "-")
    return mb, roots
