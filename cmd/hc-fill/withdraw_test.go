package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// wdFx: 병합기 프로젝트 app(기계 box)의 원격 하나와 가짜 hc-lane(부른 인자를 DATA/args에) — 제출 하나 = 큐 표식·기록·lane 가지(제출 쪽 꼴)
func wdFx(t *testing.T) (git func(a ...string) string, sub func(lane, n, owner string) (string, string), refs func() string) {
	setData(t)
	run := func(dir string, a ...string) string {
		t.Helper()
		c := exec.Command("git", a...)
		c.Dir = dir
		o, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, o)
		}
		return strings.TrimSpace(string(o))
	}
	o, w := DATA+"/o.git", DATA+"/w"
	run(DATA, "init", "-q", "--bare", "-b", "main", o)
	run(DATA, "init", "-q", "-b", "main", w)
	run(w, "commit", "-q", "--allow-empty", "-m", "base")
	run(w, "remote", "add", "origin", o)
	run(w, "push", "-q", "origin", "main")
	git = func(a ...string) string { t.Helper(); return run(w, a...) }
	sub = func(lane, n, owner string) (string, string) {
		t.Helper()
		git("checkout", "-q", "-B", "x-"+lane, "main")
		git("commit", "-q", "--allow-empty", "-m", lane)
		s := git("rev-parse", "HEAD")
		rec := git("commit-tree", s+"^{tree}", "-p", s, "-m", "merge-queue lane="+lane+" id=box-"+lane+" orig="+s+" base=main owner="+owner+" verdict=v1 patch=-")
		git("push", "-q", "origin", s+":refs/merge-queue/"+n, rec+":refs/merge-queue-meta/"+n, s+":refs/heads/lane/"+lane)
		return s, rec
	}
	refs = func() string { return git("ls-remote", "origin", "refs/merge-*", "refs/hc-landed/*") }
	os.WriteFile(DATA+"/hc-lane", []byte("#!/bin/sh\nfor a in \"$@\"; do printf '%s|' \"$a\"; done >>"+DATA+"/args\necho >>"+DATA+"/args\n"), 0o755)
	t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
	t.Setenv("HERDR_TOOLS_JOURNAL", DATA+"/journal.jsonl")
	t.Setenv("HC_TERMINAL_ID", "t9")
	t.Setenv("HC_LANE", "")
	confLines = lines("local here\nrepo app here " + w + "\nmerger app box /srv/clones/app\n")
	LOCAL = "here"
	pcRef = map[string]pcRes{}
	return
}

// ① 줄에 선 제출을 거둠 → 큐 표식·기록이 같은 이름의 반송 참조로(같은 커밋·같은 기록), 주인에게 막힘 사건(반송 다시 띄움이 받는 꼴), 일지.
// dry-run은 아무것도 안 바꿈 · --json은 한 줄 객체
func TestWithdraw(t *testing.T) {
	_, sub, refs := wdFx(t)
	s, rec := sub("la", "1000000001-la", "here/t9")
	before := refs()
	out := dupCapture(t, func() {
		if rc := withdraw("la", "진짜 충돌", false, true, false); rc != 0 {
			t.Errorf("dry-run rc %d", rc)
		}
	})
	if refs() != before || !strings.Contains(out, "거둘 수 있음 la") {
		t.Fatalf("dry-run이 바꿈 또는 글: %q\n%s", out, refs())
	}
	if a, _ := os.ReadFile(DATA + "/args"); len(a) != 0 {
		t.Fatalf("dry-run이 사건을 냄: %s", a)
	}
	out = dupCapture(t, func() {
		if rc := withdraw("la", "진짜 충돌", false, false, true); rc != 0 {
			t.Errorf("거두기 rc %d", rc)
		}
	})
	var js map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &js); err != nil || js["result"] != "거둠" || js["queue"] != "1000000001-la" || js["sha"] != s {
		t.Fatalf("--json 한 줄: %v %q", err, out)
	}
	after := refs()
	if strings.Contains(after, "refs/merge-queue/1000000001-la") || strings.Contains(after, "refs/merge-queue-meta/1000000001-la") ||
		!strings.Contains(after, s+"\trefs/merge-failed/1000000001-la") || !strings.Contains(after, rec+"\trefs/merge-failed-meta/1000000001-la") {
		t.Fatalf("큐 → 반송 참조(같은 이름·같은 커밋·같은 기록):\n%s", after)
	}
	a, _ := os.ReadFile(DATA + "/args")
	ev := string(a)
	for _, want := range []string{"report|막힘|-|지휘가 거둠: 진짜 충돌 — 원본 " + s[:8] + ", 제출 ID box-la, refs/merge-failed/1000000001-la 로 옮김",
		"|--name|la|--to|here/t9|--key|hf:withdraw:1000000001-la|"} {
		if !strings.Contains(ev, want) {
			t.Fatalf("사건에 %q 없음:\n%s", want, ev)
		}
	}
	msg := strings.Split(ev, "|")[3]
	if !wdEvRe.MatchString(msg) || bounceWhy(msg) != "지휘가 거둠: 진짜 충돌" || !relandQRe.MatchString(msg) || !bounceFrom(map[string]bool{"box": true}, "here", msg) {
		t.Fatalf("반송 다시 띄움·PR 거울이 받는 꼴이 아님: %q (까닭 %q)", msg, bounceWhy(msg))
	}
	j, _ := os.ReadFile(DATA + "/journal.jsonl")
	if !strings.Contains(string(j), `"action":"withdraw","target":"la","why":"진짜 충돌","result":"됨: refs/merge-queue/1000000001-la → refs/merge-failed/1000000001-la`) {
		t.Fatalf("일지:\n%s", j)
	}
	// 다시 부름 → 이미 반송됨(아무것도 안 바꿈)
	out = dupCapture(t, func() {
		if rc := withdraw("la", "또", false, false, false); rc != 1 {
			t.Errorf("두 번째 rc %d, 바람 1", rc)
		}
	})
	if !strings.Contains(out, "이미 반송됨") || refs() != after {
		t.Fatalf("두 번째: %q", out)
	}
}

// 거두기 사건(이 기계 here) → 반송 다시 띄움이 병합기 반송처럼 받는다: 줄을 같은 이름 「반송됨」 대기로, 절에 까닭(병합기 로그 대신).
// 다른 기계의 같은 글은 받지 않는다(병합기 기계·이 기계만)
func TestWithdrawBounceRelaunch(t *testing.T) {
	w, git := bnRepo(t)
	git(w, "commit", "-q", "--allow-empty", "-m", "l-w: 일")
	git(w, "push", "-q", "origin", "HEAD:refs/merge-failed/750-l-w", "HEAD:refs/merge-failed/760-l-o")
	bnRow(t, "l-w")
	bnRow(t, "l-o")
	wd := func(mach, lane, n string) string {
		msg := "지휘가 거둠: 먼저 든 변경과 뜻이 부딪힘 — 원본 aaaaaaaa, 제출 ID box-" + lane + ", refs/merge-failed/" + n + " 로 옮김(lane/" + lane + " 는 그대로, 거둔 쪽 t9) — 고쳐 hc-lane land main 다시"
		return strings.Join([]string{"2026-11-22T00:01:00Z", mach, "-", lane, "막힘", "-", msg, "-", "t9", "-", "-", "-"}, "\t")
	}
	EV = []string{wd("here", "l-w", "750-l-w"), wd("far", "l-o", "760-l-o")}
	relandTick()
	if r := qrow("l-w"); r == nil || r.F(qState) != "wait" || r.F(qEarly) != "bounced" {
		t.Fatalf("거둔 레인은 반송됨 대기로: %v", r)
	}
	if r := qrow("l-o"); r == nil || r.F(qState) != "up" {
		t.Fatalf("다른 기계의 거두기 꼴 글은 받지 않는다: %v", r)
	}
	tail := bounceSec("l-w")
	for _, s := range []string{"지휘가 거둠: 먼저 든 변경과 뜻이 부딪힘로 refs/merge-failed/750-l-w ", "병합기 로그 없음 — 지휘가 거뒀다", "git fetch origin refs/merge-failed/750-l-w "} {
		if !strings.Contains(tail, s) {
			t.Errorf("절에 %q 없음:\n%s", s, tail)
		}
	}
}

// 운영 에이전트는 거두지 않는다(사람·주인 지휘에게 넘김), 미리 보기는 읽기로 본다
func TestWithdrawAgentGate(t *testing.T) {
	if oaHandoff("withdraw", []string{"la", "--why", "x"}) == "" {
		t.Fatal("운영 에이전트의 withdraw는 사람에게 넘겨야")
	}
	if !oaReadOnly("withdraw", []string{"la", "--why", "x", "--dry-run"}) || oaReadOnly("withdraw", []string{"la", "--why", "x"}) {
		t.Fatal("withdraw --dry-run만 읽기")
	}
}

// ② 그새 착지됨(큐 표식 없음·착지 표식) → 「이미 착지」·무변경. 줄에 없고 흔적도 없음 → 줄에 없음
func TestWithdrawAlreadyLanded(t *testing.T) {
	git, sub, refs := wdFx(t)
	s, _ := sub("lb", "1000000002-lb", "here/t9")
	git("push", "-q", "origin", ":refs/merge-queue/1000000002-lb", ":refs/merge-queue-meta/1000000002-lb", s+":refs/hc-landed/lb/1000000003")
	before := refs()
	out := dupCapture(t, func() {
		if rc := withdraw("lb", "x", false, false, false); rc != 1 {
			t.Errorf("rc %d, 바람 1", rc)
		}
	})
	if !strings.Contains(out, "이미 착지") || refs() != before {
		t.Fatalf("이미 착지: %q\n%s", out, refs())
	}
	out = dupCapture(t, func() {
		if rc := withdraw("lz", "x", false, false, false); rc != 1 {
			t.Errorf("없는 레인 rc %d", rc)
		}
	})
	if !strings.Contains(out, "줄에 없다") {
		t.Fatalf("없는 레인: %q", out)
	}
}

// 착지 push와 겨룸: 거두기가 읽은 뒤 병합기가 넣으며 큐 표식을 지웠으면 lease가 져 「이미 착지」·무변경(이중 착지 없음)
func TestWithdrawLosesToLanding(t *testing.T) {
	git, sub, refs := wdFx(t)
	s, rec := sub("lc", "1000000004-lc", "here/t9")
	wdBeforePush = func() { // 거두기가 원격을 읽은 뒤·push 전에 병합기가 착지
		git("push", "-q", "origin", s+":refs/heads/main", ":refs/merge-queue/1000000004-lc", ":refs/merge-queue-meta/1000000004-lc",
			rec+":refs/merge-done/lc", s+":refs/hc-landed/box-lc")
	}
	t.Cleanup(func() { wdBeforePush = func() {} })
	before := ""
	out := dupCapture(t, func() {
		before = refs()
		if rc := withdraw("lc", "x", false, false, false); rc != 1 {
			t.Errorf("rc %d, 바람 1", rc)
		}
	})
	if !strings.Contains(out, "이미 착지") || strings.Contains(refs(), "refs/merge-failed/") {
		t.Fatalf("착지가 이겨야: %q\n전 %s\n후 %s", out, before, refs())
	}
	if a, _ := os.ReadFile(DATA + "/args"); len(a) != 0 {
		t.Fatalf("진 거두기가 사건을 냄: %s", a)
	}
}

// ⑤ 주인 지휘가 아니면 거절(무변경) · --force면 됨(일지에 --force) · 주인 없는 제출도 --force만 · 레인 안에서는 거절(종료 2)
func TestWithdrawOwner(t *testing.T) {
	_, sub, refs := wdFx(t)
	sub("ld", "1000000005-ld", "here/t5")
	sub("le", "1000000006-le", "-")
	before := refs()
	for _, l := range []string{"ld", "le"} {
		out := dupCapture(t, func() {
			if rc := withdraw(l, "x", false, false, false); rc != 1 {
				t.Errorf("%s rc %d, 바람 1", l, rc)
			}
		})
		if !strings.Contains(out, "--force") || refs() != before {
			t.Fatalf("%s 주인 아님: %q", l, out)
		}
	}
	t.Setenv("HC_LANE", "ld")
	if rc := withdraw("ld", "x", true, false, false); rc != 2 || refs() != before {
		t.Fatalf("레인 안 rc %d, 바람 2·무변경", rc)
	}
	t.Setenv("HC_LANE", "")
	if rc := withdraw("ld", "다른 지휘가 대신", true, false, false); rc != 0 || strings.Contains(refs(), "refs/merge-queue/1000000005-ld") {
		t.Fatalf("--force rc %d\n%s", rc, refs())
	}
	j, _ := os.ReadFile(DATA + "/journal.jsonl")
	if !strings.Contains(string(j), `"target":"ld","why":"다른 지휘가 대신","result":"됨(--force, 주인 here/t5): `) {
		t.Fatalf("일지에 --force·주인:\n%s", j)
	}
}
