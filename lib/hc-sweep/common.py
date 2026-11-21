"""hc-sweep 셸 판 공용: hc 부르기·closed.log 기록·설정 읽기 (sweep.py·orphans.py·index.py가 읽는다)."""
import argparse
import json
import os
import pathlib
import re
import signal
import socket
import sqlite3
import stat
import subprocess
import sys
import time

BIN = os.path.join(os.path.dirname(os.path.dirname(os.path.dirname(os.path.realpath(__file__)))), "bin")  # 저장소 bin/, 깔린 곳 ~/.local/bin
TASKS = os.path.expanduser("~/.claude-tasks")
SHELLS = {"zsh", "bash", "sh", "fish", "dash", "ksh"}
SESSIONS = os.path.expanduser("~/.claude/sessions")
MIN = 60 * 1000
BUSY = re.compile(r"still running|·\s*\d+\s+(?:shell|monitor)s?\b")
LANE_DIR = os.path.join(os.environ.get("XDG_STATE_HOME") or os.path.expanduser("~/.local/state"), "hc-lane")


def hc(*args):
    out = subprocess.run(["hc", *args, "--json"], capture_output=True, text=True, stdin=subprocess.DEVNULL)
    if out.returncode:
        raise RuntimeError((out.stderr.strip().splitlines() or ["hc 실패"])[0][:200])
    data = json.loads(out.stdout) if out.stdout.strip() else {}
    if isinstance(data, dict):
        data["shell"] = is_shell_pane(data.pop("argv", None) or [""])  # argv는 판정에만 쓰고 버린다
        data.pop("env", None)
        data.pop("foreground", None)
    return data


def is_shell_pane(argv):
    """사람이 연 셸 칸인가. -c(-lc 등)나 스크립트 인자를 받은 셸은 명령 실행기라 아니다."""
    args = argv[1:]
    return (os.path.basename(argv[0]).lstrip("-") in SHELLS
            and all(a.startswith("-") and (a.startswith("--") or "c" not in a) for a in args))


def record(t, why, sid):
    os.makedirs(TASKS, exist_ok=True)
    fields = [time.strftime("%Y-%m-%dT%H:%M:%S%z"), t["address"], t.get("name") or "-", why,
              t.get("cwd") or "-", sid, t.get("title") or "-"]
    with open(os.path.join(TASKS, "closed.log"), "a") as f:
        f.write("\t".join(str(x).replace("\t", " ").replace("\n", " ") for x in fields) + "\n")


def conf(key, default, tool=None, run=subprocess.run):
    """설정 값(herdr-tools-config get). 설정 도구가 곁에 없으면(맥 밖) 기본값, 있는데 실패하면 None(끄지 않음)."""
    tool = tool or os.path.join(BIN, "herdr-tools-config")
    if not os.access(tool, os.X_OK):
        return default
    try:
        out = run([tool, "get", key, default], capture_output=True, text=True, stdin=subprocess.DEVNULL, timeout=20)
    except Exception:
        return None
    return out.stdout.strip() if out.returncode == 0 else None


def exists(path):
    """지워졌으면 False. 그 밖 읽기 오류는 True(있다고 보고 터미널로 판정)."""
    try:
        os.stat(path)
        return True
    except FileNotFoundError:
        return False
    except OSError:
        return True
