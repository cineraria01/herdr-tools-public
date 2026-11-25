package main

// ht-stall-spec-alive(2026-11-25): 병합 줄 칸의 재료 — 병합기가 맨 앞 추측 검증을 기다리는 중이면(병합기 기록 폴더 specwait) 그 검증
// 기계의 살아 있는 spec-<아이디8> 터미널과 그 로그·작업 트리가 손잡이 분 안에 갱신됐나. 고치기 전 코드에는 stateSpecWait가 없어
// 컴파일부터 실패한다.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/hcjson"
)

func TestStateSpecWait(t *testing.T) {
	old := LOCAL
	defer func() { LOCAL = old }()
	LOCAL = "here"
	d := t.TempDir()
	mj, clone := filepath.Join(d, "mj"), filepath.Join(d, "app")
	wt := clone + ".spec-wt/c9ea4788"
	for _, p := range []string{mj, wt + "/.qa", wt + "/node_modules"} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p, s string, at time.Time) {
		t.Helper()
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	long := now.Add(-time.Hour)
	wait := func(mach string, last time.Time) {
		write(mj+"/specwait", "1795500000 "+itoa(last.Unix())+" 30 "+mach+"\nrefs/merge-queue/1-a "+strings.Repeat("a", 40)+" a box\n", now)
	}
	term := func(addr string) []hcjson.Terminal {
		return []hcjson.Terminal{{Address: "t1", State: "live", Name: "lane-x", Cwd: clone},
			{Address: addr, State: "live", Name: "spec-c9ea4788", Cwd: clone}}
	}
	// 오래 멈춘 검증: 로그·.times 모두 한 시간 전, 갓 쓴 것은 node_modules 안뿐(빼고 본다)
	write(wt+".log", "", long)
	write(wt+"/.qa/go-test-1.log.times", "", long)
	write(wt+"/node_modules/x.js", "", now)

	// 기다림 아님(specwait 없음) — 빈 값
	if m, tm, f, why := stateSpecWait(mj, term("t2"), 10, now); m != "" || tm != "" || f || why != "" {
		t.Fatalf("specwait 없음: %q %q %v %q", m, tm, f, why)
	}
	// ② 살아 있으나 오래 멈춤(로컬 터미널) — 터미널은 있고 갱신 없음
	wait("here", now.Add(-30*time.Second))
	if m, tm, f, why := stateSpecWait(mj, term("t2"), 10, now); m != "here" || tm != "t2" || f || why != "" {
		t.Errorf("② 멈춤: %q %q %v %q", m, tm, f, why)
	}
	// ① 살아 있고 갱신 중(남은 묶음의 .times가 방금 늘어남) — 갱신
	write(wt+"/.qa/go-test-2.log.times", "app\tTestA\t1\n", now)
	if m, tm, f, _ := stateSpecWait(mj, term("t2"), 10, now); m != "here" || tm != "t2" || !f {
		t.Errorf("① 갱신 중: %q %q %v", m, tm, f)
	}
	// ③ 터미널 없음(끝났거나 죽음 — 끝난 터미널·다른 기계 터미널은 안 셈) — 기계만
	gone := []hcjson.Terminal{{Address: "t2", State: "finished", Name: "spec-c9ea4788", Cwd: clone},
		{Address: "box-2/t9", State: "live", Name: "spec-c9ea4788", Cwd: clone}}
	if m, tm, f, _ := stateSpecWait(mj, gone, 10, now); m != "here" || tm != "" || f {
		t.Errorf("③ 터미널 없음: %q %q %v", m, tm, f)
	}
	// 병합기가 더는 기다리지 않음(마지막 기다림 바퀴가 손잡이 분 밖) · 맡길 기계를 기다림(-) · 손잡이 0 — 빈 값(지금 그대로)
	wait("here", long)
	if m, _, f, _ := stateSpecWait(mj, term("t2"), 10, now); m != "" || f {
		t.Errorf("기다림 끝남: %q %v", m, f)
	}
	wait("-", now)
	if m, _, f, _ := stateSpecWait(mj, term("t2"), 10, now); m != "" || f {
		t.Errorf("맡길 기계 기다림: %q %v", m, f)
	}
	wait("here", now)
	if m, _, f, _ := stateSpecWait(mj, term("t2"), 0, now); m != "" || f {
		t.Errorf("손잡이 0: %q %v", m, f)
	}

	// ④ 다른 기계(원격 hc): hc exec --on box-2 -- sh … 로 그 기계에서 본다(가짜 hc는 부른 기계를 적고 로컬 sh로 돈다)
	bin := t.TempDir()
	rec := filepath.Join(bin, "on")
	sc := "#!/bin/sh\n[ \"$1\" = exec ] && [ \"$2\" = --on ] || exit 2\necho \"$3\" >> " + rec + "\n" +
		"[ \"$3\" = dead ] && exit 255\nshift 4\nexec \"$@\"\n"
	if err := os.WriteFile(bin+"/hc", []byte(sc), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	wait("box-2/box-1→box-2", now) // 바쁨으로 다시 고른 사슬이 붙은 꼴
	if m, tm, f, why := stateSpecWait(mj, term("box-2/t2h2"), 10, now); m != "box-2" || tm != "box-2/t2h2" || !f || why != "" {
		t.Errorf("④ 원격 갱신 중: %q %q %v %q", m, tm, f, why)
	}
	if b, _ := os.ReadFile(rec); strings.TrimSpace(string(b)) != "box-2" {
		t.Errorf("④ 원격으로 안 물음: %q", b)
	}
	// 원격을 못 읽음 — 갱신 아님(안 움직임 쪽)·까닭
	wait("dead", now)
	if m, tm, f, why := stateSpecWait(mj, term("dead/t3"), 10, now); m != "dead" || tm != "dead/t3" || f || !strings.Contains(why, "못 읽음") {
		t.Errorf("원격 못 읽음: %q %q %v %q", m, tm, f, why)
	}
}
