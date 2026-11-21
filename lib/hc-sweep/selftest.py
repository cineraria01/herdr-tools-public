"""hc-sweep 셸 판 --self-test (sweep.py가 부른다)."""
import tempfile

from common import *
from orphans import *
from index import *
from sweep import *


def self_test():
    assert index_gone(["/a/x.wt/gone"], exists=lambda p: False) and index_gone(["/a/x.wt/g1", "/a/x.wt/g2"], exists=lambda p: False)
    assert not index_gone([], exists=lambda p: False)  # 빈 root만 있던 색인
    assert not index_gone(["/a/x.wt/g1", "/a/live"], exists=lambda p: p == "/a/live")
    assert not index_gone(["/Volumes/usb/repo"], exists=lambda p: False)
    with tempfile.TemporaryDirectory() as root:  # 링크를 거쳐 빠진 디스크(/Volumes)로 가면 남긴다(감사 [중간])
        os.symlink("/Volumes/hc-sweep-self-test-없음", os.path.join(root, "usb"))
        assert not index_gone([os.path.join(root, "usb", "repo")], exists=lambda p: False)
        assert not index_gone([os.path.join(root, "usb", "repo")])
    assert not index_gone(["rel/repo"], exists=lambda p: False) and index_roots("/nonexistent/x.db") == []
    assert not index_gone(["/Users/u/Documents/app/x"], exists=lambda p: True)  # TCC 권한 거부 = exists() 「있음」
    now, h = 1_000_000_000_000, 60 * MIN
    keep = {"keepme", "t9"}

    def t(**kw):
        base = {"address": "t1", "name": "claude-x", "state": "live", "status": {"app": "claude-code", "state": "idle"},
                "created_ms": now - 3 * h, "status_ms": now - 3 * h + 500, "turn": 0, "parent": None, "labels": {},
                "worktree": False, "unlanded": False, "tail": ""}
        base.update(kw)
        return base

    assert rule(t(), now, keep) == "R1"
    assert rule(t(created_ms=now - 20 * MIN, status_ms=now - 20 * MIN), now, keep) is None  # 아직 30분 안 됨
    assert rule(t(status_ms=now - 2 * h), now, keep) is None  # 한 번 일했다 쉰 지 2시간
    assert rule(t(turn=3, status_ms=now - 3 * h + 500), now, keep) is None
    assert rule(t(parent="t0", turn=2, status_ms=now - 31 * MIN), now, keep) == "R2" and rule(
        t(parent="t0", turn=2, status_ms=now - 10 * MIN), now, keep) is None
    assert rule(t(parent="t0", status={"app": "claude-code", "state": "done"}, status_ms=now - h), now, keep) == "R2"
    assert rule(t(turn=5, created_ms=now - 9 * h, status_ms=now - 7 * h), now, keep) == "R3" and rule(
        t(turn=5, created_ms=now - 9 * h, status_ms=now - 5 * h), now, keep) is None
    for state in ("working", "blocked", "error"):
        assert rule(t(status={"app": "claude-code", "state": state}), now, keep) is None
    for kw in (dict(status=None), dict(status={"state": "idle"}), dict(state="finished"), dict(address="w1/t4rn"),
               dict(name="d1-loop-w1"), dict(name="claude-proxy"), dict(name="keepme"), dict(address="t9"),
               dict(labels={"where": "proj / 1"})):
        assert rule(t(**kw), now, keep) is None, kw
    assert rule(t(name="lane-watch-t7cc", parent="t7cc", status_ms=now - 2 * h), now, keep) is None  # hc-lane 감시
    assert rule(t(name="w", labels={"hc-lane-watch": "1"}, parent="t7cc", status_ms=now - 7 * h), now, keep) is None
    done = {"app": "claude-code", "state": "done"}
    lane = dict(parent="t0", turn=4, status=done, status_ms=now - 31 * MIN, labels={"where": "proj / Claude Code"})
    assert rule(t(**lane), now, keep) == "R2"  # 지휘가 띄워 탭에 붙인 worker
    assert rule(t(**dict(lane, parent=None)), now, keep) is None  # 사람이 연 칸(R3 시간 전)
    assert rule(t(**dict(lane, shell=True)), now, keep) is None
    assert rule(t(**dict(lane, unlanded=True)), now, keep) is None  # 착지 전 커밋
    for tail in ("  ⏵⏵ bypass permissions on · 2 shells", "⎿ 1 monitor still running", "  ⏵⏵ bypass permissions on · 1 shell · 1 monitor"):
        assert rule(t(**dict(lane, tail=tail)), now, keep) is None, tail
    assert rule(t(**dict(lane, tail="  ⏵⏵ bypass permissions on (shift+tab to cycle)")), now, keep) == "R2"
    assert rule(t(**dict(lane, unlanded=True, status_ms=now - 7 * h)), now, keep) == "R3"  # R3는 보호 없음
    assert rule(t(shell=True, labels={"herdr.space": "sp1", "herdr.tab": "tb1"}), now, keep) is None  # Ovis 새 탭 셸
    assert rule(t(shell=False, labels={"herdr.space": "sp1", "herdr.tab": "tb1", "ovis.tab": "지휘"}), now, keep) == "R1"
    assert rule(t(status_ms=None, created_ms=None), now, keep) is None
    # 남은 워크트리·모르는 값은 R2·R3 모두 닫지 않음
    assert rule(t(**dict(lane, worktree=True)), now, keep) is rule(t(**dict(lane, worktree=True, status_ms=now - 7 * h)), now, keep) is None
    for missing in ("worktree", "unlanded", "tail"):
        x = t(**lane)
        del x[missing]
        assert rule(x, now, keep) is None, missing
    assert rule(t(**dict(lane, tail=None)), now, keep) is None
    assert rule(t(turn=None), now, keep) is None  # turn 모름이면 R1 아님
    # hc-fill이 띄운 레인: parent 없음 + 라벨 hc-lane=1(where 라벨이 붙어도) → worker로 R2 (2026-11-17 원격 레인이 몇 시간 남음)
    fill = dict(parent=None, turn=5, status=done, status_ms=now - 31 * MIN, labels={"hc-lane": "1"})
    assert rule(t(**fill), now, keep) == "R2"
    assert rule(t(**dict(fill, labels={"hc-lane": "1", "where": "proj / Claude Code"})), now, keep) == "R2"
    assert rule(t(**dict(fill, worktree=True)), now, keep) is None
    assert rule(t(**dict(fill, unlanded=True)), now, keep) is None
    assert rule(t(**dict(fill, tail="⎿ 1 monitor still running")), now, keep) is None
    assert rule(t(**dict(fill, fill="up")), now, keep) is None  # hc-fill이 아직 결과를 안 되돌려 줌
    assert rule(t(**dict(fill, fill="sent")), now, keep) == "R2"
    assert rule(t(**dict(fill, turn=0, status_ms=now - 3 * h + 500)), now, keep) == "R2"  # 레인은 R1(안 쓴 세션) 아님
    assert rule(t(**dict(fill, status_ms=now - MIN, submitted=True)), now, keep) == "R2"  # 병합기에 제출하고 끝남: 30분 안 기다림
    assert rule(t(**dict(fill, status_ms=now - MIN, submitted=True, worktree=True)), now, keep) is None
    with tempfile.TemporaryDirectory() as root:
        q = os.path.join(root, "queue.tsv")
        assert fill_state("x", "t5", q) is None  # 대기열 없음
        with open(q, "w") as f:
            f.write("x\tp\t/b\t-\t-\tup\tm\tt5\t-\t2026\tmini:/in/done/x.md\n"
                    "y\tp\t/b\t-\t-\tsent\tm\tt6\t-\t2026\tmini:/in/done/y.md\n"
                    "z\tp\t/b\t-\t-\tup\tm\tt7\t-\t2026\t-\n"
                    "o\tp\t/b\t-\t-\tup\tm\tt8\t-\t2026\n")
        assert fill_state("x", "t5", q) == "up"
        assert fill_state("x", "t9", q) is None  # 다른 주소(같은 이름의 옛 줄)
        assert fill_state("y", "t6", q) == "sent"
        assert fill_state("z", "t7", q) is None and fill_state("o", "t8", q) is None  # 되돌릴 곳 없는 줄(손으로 넣은 것)
        os.chmod(q, 0)
        try:
            assert fill_state("x", "t5", q) == "up"  # 못 읽으면 닫지 않는 쪽
        finally:
            os.chmod(q, 0o644)
    with tempfile.TemporaryDirectory() as root:
        root = os.path.realpath(root)
        main, wt = os.path.join(root, "repo"), os.path.join(root, "repo.wt", "lane")
        os.makedirs(os.path.join(main, ".git"))
        os.makedirs(os.path.join(main, "sub"))
        os.makedirs(os.path.join(wt, "frontend"))
        with open(os.path.join(wt, ".git"), "w") as f:
            f.write("gitdir: " + main + "/.git/worktrees/lane\n")
        assert linked_worktree(wt) is True
        assert linked_worktree(os.path.join(wt, "frontend")) is True
        assert linked_worktree(main) is False
        assert linked_worktree(os.path.join(main, "sub")) is False
        assert linked_worktree(root) is False
        assert linked_worktree(os.path.join(root, "repo.wt", "gone")) is False  # 지운 워크트리
        assert linked_worktree(None) is True and linked_worktree("") is True and linked_worktree("rel/x") is True
        os.chmod(os.path.join(root, "repo.wt"), 0)
        try:
            assert linked_worktree(wt) is True  # 읽기 오류는 닫지 않음
        finally:
            os.chmod(os.path.join(root, "repo.wt"), 0o755)

    def fake(rc=0, out="", err="", exc=None):
        def run(*a, **kw):
            if exc:
                raise exc
            return subprocess.CompletedProcess(a, rc, out, err)
        return run

    assert unlanded("/x", fake(out="+ abc\n")) is True and unlanded("/x", fake(out="- abc\n")) is unlanded("/x", fake(out="")) is False
    assert unlanded(None, fake(exc=AssertionError("부르면 안 됨"))) is False
    assert unlanded("/x", fake(rc=128, err="fatal: not a git repository (or any of the parent directories): .git")) is False
    assert unlanded("/x", fake(rc=1, err='{"error":{"message":"cannot start folder /x: No such file or directory"}}')) is False
    for bad in (fake(exc=subprocess.TimeoutExpired("hc", 40)), fake(exc=OSError("hc 없음")), fake(rc=124, err="timed out"),
                fake(rc=128, err="fatal: bad revision 'origin/main'")):
        assert unlanded("/x", bad) is True  # 판정 실패는 닫지 않음
    assert BUSY.search(screen_tail("t1", fake(rc=1)))
    assert BUSY.search(screen_tail("t1", fake(exc=subprocess.TimeoutExpired("hc", 40))))
    assert not BUSY.search(screen_tail("t1", fake(out="a\n\n  ⏵⏵ bypass permissions on (shift+tab to cycle)\n")))
    for argv in (["/bin/zsh", "-l"], ["-zsh"], ["bash"], ["zsh", "-i"], ["/bin/bash", "--login"]):
        assert is_shell_pane(argv), argv
    for argv in (["/bin/zsh", "-lc", "exec claude run"], ["zsh", "-c", "x"], ["bash", "/x/hc-loop-start"],
                 ["env", "claude"], ["claude", "run"]):
        assert not is_shell_pane(argv), argv
    sfx = ".wt"
    assert lane_root("/a/app.wt/l1/tests/../bin/x", sfx) == "/a/app.wt/l1" == lane_root("/a/app.wt/l1", sfx)
    assert lane_root("/a/app.wt", sfx) is None and lane_root("/a/.wt/l1/x", sfx) is None
    assert lane_root("rel/app.wt/l1", sfx) is None and lane_root(None, sfx) is None
    assert lane_root("/a/app.lanes/l1/x", ".lanes") == "/a/app.lanes/l1"
    assert secs("05:03") == 303 and secs("1:00:00") == 3600 and secs("2-01:00:00") == 2 * 86400 + 3600
    there = {"/a/app.wt/l1", "/a/app.wt/l2", "/a/app.wt/l3", "/a/main.wt/m1"}
    procs = {10: (1, 3600, "/usr/bin/python3 /a/app.wt/l1/tests/../bin/hc-fill watch --every 1"),
             11: (1, 3600, "sleep 300"),                       # cwd로 l2
             12: (1, 3600, "/a/app.wt/l3/bin/x"),              # l3은 터미널이 씀
             13: (1, 3600, "/Applications/Foo.app/Contents/MacOS/Foo"),
             14: (1, 3600, "/a/app.wt/gone/bin/x --root=/a/app.wt/gone"),
             15: (99, 3600, "/a/app.wt/l1/bin/y"),             # 부모가 살아 있음
             16: (1, 30, "/a/app.wt/l1/bin/z"),                # 너무 어림
             17: (1, 3600, "/usr/local/bin/agent"),            # launchd 작업, cwd가 작업 트리 밖
             18: (1, 3600, "/b/app.wt/l9/bin/w"),              # --only-under 밖
             19: (1, 3600, "/Users/u/.local/bin/cbm-mcp --daemon"),  # 여러 세션이 쓰는 데몬(keep), cwd가 작업 트리
             20: (1, 3600, "/a/main.wt/m1/bin/x"),             # 본 저장소 /a/main을 터미널이 씀(감사 스냅숏 꼴)
             22: (1, 3600, "/opt/h/herdr-core server")}        # 코어(keep)
    cwd_of = {11: "/a/app.wt/l2/sub", 17: "/Users/u", 19: "/a/app.wt/gone", 22: "/a/app.wt/gone"}
    got = orphans(procs, cwd_of, ["/a/app.wt/l3/frontend", "/a/app.wt/l2x", "/a/main/sub"], sfx, 600, only="/a",
                  exists=lambda p: p in there, keep={"cbm-mcp", "herdr-core"})
    assert [(g[0], g[1], g[4], g[5]) for g in got] == [
        (10, "/a/app.wt/l1", "쓰는 터미널 없음", True),
        (11, "/a/app.wt/l2", "보류: cwd로만 걸림, 작업 트리 있음", False),
        (14, "/a/app.wt/gone", "작업 트리 지워짐", True),
        (20, "/a/main.wt/m1", "보류: 본 저장소 터미널 있음", False)], got
    got = orphans(procs, cwd_of, [], sfx, 600, me=10, exists=lambda p: p in there)
    assert [g[0] for g in got if g[5]] == [12, 14, 18, 19, 20, 22] and [g[0] for g in got if not g[5]] == [11], got
    # keep은 공백 든 실행 경로도 ps comm으로 본다(H56 — 맥 코어 …/Herdr Core.app/Contents/MacOS/herdr-core)
    core = "/x/Herdr Core.app/Contents/MacOS/herdr-core"
    sp = {23: (1, 3600, core + " server"), 24: (1, 3600, "/x/My Tool.app/Contents/MacOS/tool")}
    got = orphans(sp, {23: "/a/app.wt/gone", 24: "/a/app.wt/gone"}, [], sfx, 600, exists=lambda p: False,
                  keep={"herdr-core"}, exe_of={23: core, 24: "/x/My Tool.app/Contents/MacOS/tool"})
    assert [g[0] for g in got] == [24], got
    assert [g[0] for g in orphans(sp, {23: "/a/app.wt/gone"}, [], sfx, 600, exists=lambda p: False,
                                  keep={"herdr-core"})] == [23]  # comm 없으면 첫 낱말(/x/Herdr)
    assert exes(fake(out="  23 %s\n 7 herdr-core\n x y\n" % core)) == {23: core}  # "/" 없는 이름(리눅스 잘린 comm)은 버림
    try:
        exes(fake(rc=1))
        raise AssertionError("ps 실패를 삼킴")
    except RuntimeError:
        pass
    assert tree([10], {10: (1, 0, ""), 11: (10, 0, ""), 12: (11, 0, ""), 13: (1, 0, "")}) == {10, 11, 12}
    assert conf("sweep.wt_suffix", ".wt", "/nonexistent/tool") == ".wt"
    with tempfile.TemporaryDirectory() as root:
        tool = os.path.join(root, "cfg")
        with open(tool, "w") as f:
            f.write("#!/bin/sh\nexit 2\n")
        os.chmod(tool, 0o755)
        assert conf("sweep.wt_suffix", ".wt", tool) is None  # 설정을 못 읽으면 끄지 않음
        log = os.path.join(root, "closed.log")
        with open(log, "w") as f:
            f.write("t\tpid:5\t-\t고아 관찰(쓰는 터미널 없음)\t/a.wt/l\t-\t나이 0분 sleep 300\n")
        assert seen(5, "sleep 300", log) and not seen(6, "sleep 300", log) and not seen(5, "sleep 30", log)
    print("ok")
