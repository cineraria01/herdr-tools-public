#!/usr/bin/env python3
"""Ovis(herdr-core 모드)에서 worker를 지휘 탭 옆 탭에 보이게 한다 (herdr-ovis 스킬).

ovis-split --main <지휘> [--add tX ...]   지휘와 같은 공간에 worker마다 탭 하나. --add가 없으면 지휘의 이름 붙은 자식 전부
ovis-split --main <지휘> --drop tX ...     공간에서 뺀다(Ovis의 Other terminals로). 지휘 자신과 그 자식만
ovis-split --self-test

Ovis는 herdr.space 라벨로 공간을 고르고, herdr.tab 라벨이 없으면 터미널마다 탭을 따로 준다(Ovis
server/herdr-core/snapshot.ts groups). 한 탭에 나란히 넣지 않는다: Ovis는 칸 크기를 각 터미널의 열·행 비율로
정하고 그린 칸 크기로 터미널을 다시 줄여, 쓰는 칸이 계속 작아진다(2026-11-05 지휘 t1v0가 26열×1행까지).
지휘에 라벨이 없으면 같은 폴더의 Herdr 앱 공간, 없으면 같은 폴더의 다른 지휘가 쓰는 공간, 그것도 없으면
새 공간에 지휘 탭을 준다.
"""
import argparse
import json
import os
import subprocess
import sys
import uuid

SPACE, TAB, WS_NAME, TAB_NAME = "herdr.space", "herdr.tab", "ovis.workspace", "ovis.tab"


def hc(*args):
    out = subprocess.run(["hc", *args], check=True, capture_output=True, text=True, stdin=subprocess.DEVNULL)
    return json.loads(out.stdout) if out.stdout.strip() else {}


def label(address, pairs):
    subprocess.run(["hc", "label", address, *(f"{k}={v}" for k, v in pairs.items())],
                   check=True, capture_output=True, stdin=subprocess.DEVNULL)


def place(main, candidates, space_of):
    """main 탭의 라벨과, 그 라벨을 main에 새로 붙여야 하는지.
    candidates: 공간을 빌려 올 후보(앞이 우선). space_of(주소)는 그 터미널의 herdr.space."""
    labels = main.get("labels") or {}
    if labels.get(SPACE) and labels.get(TAB):
        return {k: labels[k] for k in (SPACE, TAB, WS_NAME, TAB_NAME) if labels.get(k)}, False
    space = labels.get(SPACE)
    for other in candidates:
        if space:
            break
        # 같은 폴더의 터미널이 쓰는 공간에 새 탭으로. 자기 자신과 다른 기계 것은 제외.
        if (other["address"] != main["address"] and other.get("cwd") == main.get("cwd")
                and "/" not in other["address"]):
            space = space_of(other["address"])
    return {SPACE: space or f"ovis-{uuid.uuid4()}", TAB: labels.get(TAB) or f"ovis-{uuid.uuid4()}",
            WS_NAME: os.path.basename(main.get("cwd") or "") or "ovis",
            TAB_NAME: f"지휘 {main['address']}"}, True


def worker_labels(pairs, worker, main_address):
    """worker 라벨: 지휘의 공간, 탭 라벨은 지워 worker마다 탭 하나, 탭 이름은 'worker 이름 · 지휘 주소'."""
    return {SPACE: pairs[SPACE], TAB: "", WS_NAME: pairs.get(WS_NAME, ""),
            TAB_NAME: f"{worker.get('name') or worker['address']} · {main_address}"}


def droppable(target, main_address):
    """--drop이 받는 것: 지휘 자신이나 그 자식이고, Herdr 앱 칸(where 라벨)이 아닌 것."""
    if (target.get("labels") or {}).get("where"):
        return False
    return target.get("address") == main_address or target.get("parent") == main_address


def self_test():
    app = {"address": "t1", "cwd": "/r/proj", "where": "proj / 1", "state": "live"}
    remote = {"address": "w1/t2", "cwd": "/r/proj", "where": "proj / 1", "state": "live"}
    me = {"address": "t9", "cwd": "/r/proj", "labels": {}}
    pairs, new = place(me, [remote, app], lambda address: "sp1" if address == "t1" else "spX")
    assert new and pairs[SPACE] == "sp1" and pairs[TAB].startswith("ovis-"), pairs
    assert pairs[WS_NAME] == "proj" and pairs[TAB_NAME] == "지휘 t9", pairs
    pairs, new = place(me, [], lambda _: None)
    assert new and pairs[SPACE].startswith("ovis-"), pairs
    peer = {"address": "t3", "cwd": "/r/proj", "state": "live"}  # 같은 폴더의 다른 지휘
    assert place(me, [peer], lambda _: "ovis-peer")[0][SPACE] == "ovis-peer"
    assert place(me, [dict(me)], lambda _: "self")[0][SPACE] != "self"  # 자기 자신은 후보가 아니다
    placed = {"address": "t9", "cwd": "/r/proj", "labels": {SPACE: "sp1", TAB: "tb1", "where": "proj / 1"}}
    assert place(placed, [app], lambda _: "x") == ({SPACE: "sp1", TAB: "tb1"}, False)
    labels = worker_labels({SPACE: "sp1", TAB: "tb1"}, {"address": "t5", "name": "lane-a"}, "t9")
    assert labels == {SPACE: "sp1", TAB: "", WS_NAME: "", TAB_NAME: "lane-a · t9"}, labels
    assert worker_labels({SPACE: "sp1", TAB: "tb1"}, {"address": "t6"}, "t9")[TAB_NAME] == "t6 · t9"
    assert droppable({"address": "t9"}, "t9") and droppable({"address": "t5", "parent": "t9"}, "t9")
    assert not droppable({"address": "t7", "parent": "t8"}, "t9")
    assert not droppable({"address": "t5", "parent": "t9", "labels": {"where": "proj / 1"}}, "t9")
    print("ok")


def run():
    # 셸 판(되돌리기, bin/ovis-split이 OVIS_SPLIT_IMPL=bash면 부름) — 이름은 bin 그대로
    parser = argparse.ArgumentParser(prog="ovis-split", description="Ovis(herdr-core 모드)에서 worker를 지휘 탭 옆 탭에 보이게 한다")
    parser.add_argument("--main", help="지휘 터미널 주소(보통 $HC_TERMINAL_ID)")
    parser.add_argument("--add", nargs="+", default=[], metavar="TERMINAL")
    parser.add_argument("--drop", nargs="+", default=[], metavar="TERMINAL")
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        return self_test()
    if not args.main:
        parser.error("--main이 필요하다")
    main = hc("get", "--json", args.main)
    if args.drop:
        dropped = []
        for name in args.drop:
            target = hc("get", "--json", name)
            if not droppable(target, main["address"]):
                print(f"건너뜀 {name}: 지휘나 그 자식이 아니거나 Herdr 앱 칸이다", file=sys.stderr)
                continue
            label(target["address"], dict.fromkeys((SPACE, TAB, WS_NAME, TAB_NAME), ""))
            dropped.append(target["address"])
        print(json.dumps({"main": main["address"], "dropped": dropped}, ensure_ascii=False))
        return
    live = [t for t in hc("ls", "--json").get("terminals", []) if t.get("state") == "live"]
    folder = os.path.basename(main.get("cwd") or "") or "ovis"
    peers = hc("ls", "--json", "--label", f"{WS_NAME}={folder}").get("terminals", [])
    candidates = [t for t in live if t.get("where")] + [t for t in peers if t.get("state") == "live"]
    pairs, new = place(main, candidates, lambda address: (hc("get", "--json", address).get("labels") or {}).get(SPACE))
    if new:
        label(main["address"], pairs)
    named = {t["address"]: t for t in live}
    # 이름 없는 자식은 HUD·배경 실행이라 건드리지 않는다(herdr-balance와 같은 기준)
    workers = args.add or [t["address"] for t in live if t.get("parent") == main["address"] and t.get("name")]
    for worker in workers:
        label(worker, worker_labels(pairs, named.get(worker) or {"address": worker}, main["address"]))
    print(json.dumps({"main": main["address"], "space": pairs[SPACE], "tab": pairs[TAB], "added": workers},
                     ensure_ascii=False))


if __name__ == "__main__":
    run()
