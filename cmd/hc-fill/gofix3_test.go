package main

// gofix3-fill-queue 재현 시험(2바퀴 재감사 지적, 감사 HEAD 1eccd73): 고치기 전 판에서 실패하던 시험. 기대값은 지적 원문과 계약으로 정했다

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"herdr-tools/internal/lockx"
)

// fill/R-208: run 바퀴가 도는 동안(run.lock — 줄·지시문 사본을 뜬 뒤 up으로 바꾸기 전) edit는 줄을 바꾸지 않는다.
// 바꾸면 대기열은 새 선행·새 지시문인데 실제 레인은 옛 사본으로 뜬다. 잠금을 못 얻으면 75, 대기열 그대로
func TestEditDuringRunRefused(t *testing.T) {
	setData(t)
	dry, P, confLines = false, "", []string{"repo app m " + DATA}
	nb := DATA + "/new.md"
	if err := os.WriteFile(nb, []byte("# 새\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	row := strings.Join([]string{"e1", "app", DATA + "/old.md", "-", "-", "wait", "-", "-", "-", "-", "-", "t1", "normal", "-", "-", "-", "r1"}, "\t") + "\n"
	if err := os.WriteFile(Q, []byte(row), 0o644); err != nil {
		t.Fatal(err)
	}
	f, busy, err := lockx.TryLock(DATA + "/run.lock")
	if err != nil || busy {
		t.Fatalf("run.lock: %v %v", busy, err)
	}
	defer f.Close()
	t.Setenv("HC_FILL_THEN_WAIT", "1")
	rc := withQ(func() { editCmd([]string{"e1", "--brief", nb, "--after", "blocker"}) })
	if rc != 75 {
		t.Fatalf("run이 도는 동안 edit 종료 %d(바람 75)", rc)
	}
	if got, _ := readFile(Q); got != row {
		t.Fatalf("run이 도는 동안 대기열이 바뀌었다:\n%s", got)
	}
	f.Close()
	if rc := withQ(func() { editCmd([]string{"e1", "--brief", nb}) }); rc != 0 {
		t.Fatalf("run이 끝난 뒤 edit 종료 %d", rc)
	}
	if r := qrow("e1"); r == nil || r.F(qBrief) != DATA+"/briefs/added/e1.md" {
		t.Fatalf("run이 끝난 뒤 edit가 안 먹었다: %v", qRows())
	}
}

// fill/R-215: 원래 레인에 hc send가 응답을 잃으면(255 — 받았는지 모름) 같은 바퀴에 다시 맞춤 레인을 넣지 않는다.
// 보낸 시도를 먼저 「보냄」으로 적고, 원래 레인이 reland_wait분 안에 다시 내는지 본 뒤에만 다시 맞춤
func TestRelandSendLost(t *testing.T) {
	w, git := rlRepo(t)
	git(w, "push", "-q", "origin", "HEAD:refs/merge-failed/100-l-live")
	if err := os.MkdirAll(w+".wt/l-live", 0o755); err != nil {
		t.Fatal(err)
	}
	// 받은 기록은 남기고 연결 오류 255
	if err := os.WriteFile(DATA+"/hc", []byte("#!/bin/sh\nprintf '%s|' \"$@\" >>"+DATA+"/sends\necho >>"+DATA+"/sends\nexit 255\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	OREC = []Row{{"l-live", "app", "here", "here/t1", "2026-11-22T00:00:00Z", "-", ",-,", "-"}}
	TERMS = []Term{{"here", "here/t1", "claude", "l-live", "idle", "-"}}
	EV = []string{rlEv("2026-11-22T00:01:00Z", "l-live", "100-l-live", "착지 실패(종료 3, 충돌 README.md)", "t9")}
	tick := func() {
		MQP, MQS, MQBAD = "", nil, ""
		relandTick()
	}
	tick()
	sends, _ := readFile(DATA + "/sends")
	if strings.Count(sends, "send|here/t1|") != 1 {
		t.Fatalf("원래 레인에 보낸 시도: %q", sends)
	}
	if qrow("l-live-reland") != nil {
		t.Fatalf("hc send 응답을 잃었는데 같은 바퀴에 다시 맞춤을 넣었다: %v", qRows())
	}
	rl, _ := readFile(DATA + "/reland.tsv")
	if !strings.Contains(rl, "100-l-live\tl-live\tl-live\t보냄\there/t1\t") {
		t.Fatalf("보낸 시도 기록(보냄) 없음: %q", rl)
	}
	tick() // 기다리는 동안: 다시 보내지도 넣지도 않는다
	if s2, _ := readFile(DATA + "/sends"); s2 != sends || qrow("l-live-reland") != nil {
		t.Fatalf("기다리는 동안 또 처리: %q %v", s2, qRows())
	}
	// reland_wait분이 지나도 다시 안 냄 → 그때 다시 맞춤 한 번
	old := regexp.MustCompile(`(\t보냄\there/t1\t)[0-9TZ:-]+`).ReplaceAllString(rl, "${1}2026-11-22T00:02:00Z")
	if err := os.WriteFile(DATA+"/reland.tsv", []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	tick()
	if qrow("l-live-reland") == nil {
		t.Fatalf("기다림이 지났는데 다시 맞춤을 안 넣었다: %v", qRows())
	}
	if s3, _ := readFile(DATA + "/sends"); s3 != sends {
		t.Fatalf("기다림 뒤 원래 레인에 또 보냄: %q", s3)
	}
}
