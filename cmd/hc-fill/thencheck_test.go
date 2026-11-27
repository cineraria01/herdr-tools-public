package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// ht-revive-cmd(2026-11-28 지휘 실수): --then으로 단계를 이어 붙인 레인에 「1단계를 먼저 완료로 보고하라」고 잘못 시켜 1단계만 착지하고
// 레인이 끝났다 — 뒤 두 단계가 안 돈 채 사라졌고 지휘는 14분 뒤 우연히 알았다. 착지한 줄의 지시문 사본에 「## 이어서: <이름>」 절이 있는데
// 기본 가지의 접수 뒤 커밋에 제목 머리 「<이름>:」이 없으면 주인 지휘에 막힘 한 번

// tcFx: 프로젝트 app(이 기계 m, 원격 없는 저장소 — 기본 가지 refs/heads/main)·레인 w1(up, 착지 사건)·이어 붙인 절 then
func tcFx(t *testing.T, then []string, subjects ...string) (ev func() string) {
	t.Helper()
	setData(t)
	ol, od, oc, ocf, oe := LOCAL, dry, CONN, confLines, EVBAD
	t.Cleanup(func() { LOCAL, dry, CONN, confLines, EVBAD = ol, od, oc, ocf, oe })
	LOCAL, dry, CONN, EVBAD = "m", false, nil, false
	FETCHED, FETCHBAD = "", ""
	repo := DATA + "/repo"
	git := func(a ...string) {
		c := exec.Command("git", append([]string{"-C", repo}, a...)...)
		c.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2030-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2030-01-01T00:00:00Z")
		if o, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", a, err, o)
		}
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	git("init", "-q", "-b", "main")
	git("-c", "user.name=t", "-c", "user.email=t@e", "commit", "-q", "--allow-empty", "-m", "seed")
	for _, s := range subjects {
		git("-c", "user.name=t", "-c", "user.email=t@e", "commit", "-q", "--allow-empty", "-m", s)
	}
	confLines = []string{"repo app m " + repo}
	brief := DATA + "/briefs/added/w1.md"
	b := "# 본 일\n"
	for _, n := range then {
		b += "\n## 이어서: " + n + " (hc-fill add --then, 2026-11-28T03:36)\n이어서 한다\n"
	}
	if err := os.MkdirAll(DATA+"/briefs/added", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(brief, []byte(b), 0o644); err != nil {
		t.Fatal(err)
	}
	// 줄 ID 「r<접수 초>-…」 — 접수 2029년(커밋 시각 2030년이 그 뒤)
	q := strings.Join([]string{"w1", "app", brief, "-", "-", "up", "m", "m/t1", "-", "t", "-", "t0", "normal", "-", "auto", "-", "r1861920000-1-1"}, "\t") + "\n" +
		strings.Join([]string{"nx", "app", DATA + "/nx.md", "w1-b", "-", "wait", "-", "-", "-", "t", "-", "t0", "normal", "-", "auto", "-", "r1861920001-1-2"}, "\t") + "\n"
	if err := os.WriteFile(Q, []byte(q), 0o644); err != nil {
		t.Fatal(err)
	}
	EV = []string{auTEv("2030-01-01T00:00:01Z", "m/t1", "w1", "착지", "abcd1234", "main에 넣음", "-")}
	if err := os.WriteFile(DATA+"/hc-lane", []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >>'"+DATA+"/ev.log'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
	if err := os.WriteFile(DATA+"/then.tsv", nil, 0o644); err != nil { // 이 판이 이미 돈 뒤(첫 바퀴의 옛 줄 적기는 따로 본다)
		t.Fatal(err)
	}
	return func() string { b, _ := os.ReadFile(DATA + "/ev.log"); return string(b) }
}

// 이어 붙인 절 둘 가운데 하나만 착지 → 막힘 한 번(되풀이 없음), 그 이름을 --after로 기다리는 줄도 적는다
func TestThenMissOnce(t *testing.T) {
	ev := tcFx(t, []string{"w1-a", "w1-b"}, "w1: 본 일", "w1-a: 둘째 단계")
	thenCheckTick()
	thenCheckTick()
	e := ev()
	if strings.Count(e, "report 막힘") != 1 || !strings.Contains(e, "이어 붙인 단계 w1-b이 착지에 없다") || strings.Contains(e, "w1-a,") ||
		!strings.Contains(e, "nx") || !strings.Contains(e, "--to t0") || !strings.Contains(e, "--key hf:w1:then-miss:") {
		t.Fatalf("빠진 단계 w1-b로 막힘 한 번이어야: %q", e)
	}
}

// 다 착지 → 알림 없음 · 이어 붙인 절 없음 → 그대로
func TestThenAllLandedOrNone(t *testing.T) {
	ev := tcFx(t, []string{"w1-a"}, "w1: 본 일", "w1-a: 둘째 단계")
	thenCheckTick()
	if e := ev(); e != "" {
		t.Fatalf("다 착지했으면 알림 없음: %q", e)
	}
	ev = tcFx(t, nil, "w1: 본 일")
	thenCheckTick()
	if e := ev(); e != "" {
		t.Fatalf("이어 붙인 절이 없으면 그대로: %q", e)
	}
}

// 옛 줄(이 판 전에 착지 — then.tsv가 아직 없는 첫 바퀴) → 알림 없이 옛으로 적고, 다음 바퀴에도 알리지 않는다
func TestThenOldRowQuiet(t *testing.T) {
	ev := tcFx(t, []string{"w1-a"}, "w1: 본 일")
	os.Remove(DATA + "/then.tsv")
	thenCheckTick()
	thenCheckTick()
	if e := ev(); e != "" {
		t.Fatalf("옛 줄은 알리지 않는다: %q", e)
	}
	if b, _ := os.ReadFile(DATA + "/then.tsv"); !strings.Contains(string(b), "w1\tr1861920000-1-1\t옛") {
		t.Fatalf("옛 줄로 적어야: %q", b)
	}
}
