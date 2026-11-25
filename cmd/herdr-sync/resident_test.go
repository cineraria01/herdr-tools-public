package main

// 배포 뒤 상주 확인(ht-resident-reexec): 기계마다 그 기계 실행 파일로 도는 상주 가운데 옛 판·판 모름을 센다 — 같은 판 문자열(커밋)이면
// 파일만 다시 쓰인 것이라 세지 않고, 그 기계 HOME 밖 실행 파일(다른 사용자·시험 밖 실제 프로세스)은 보지 않는다

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/resident"
)

func TestResidentStale(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	inst := now.Add(-time.Hour)
	ps := func(pid int, start time.Time, cmd string) string {
		return fmt.Sprintf("P\t%5d %s %s\n", pid, start.Format("Mon Jan _2 15:04:05 2006"), cmd)
	}
	mark := func(pid int, path, commit string, mt time.Time) string {
		b, _ := json.Marshal(resident.Mark{Who: "x", Pid: pid, Path: path, Commit: commit, Size: 10, Mtime: mt.Add(123 * time.Millisecond), At: now.Add(-2 * time.Hour)})
		return "M\t" + string(b) + "\n"
	}
	day := now.Add(-3 * time.Hour)
	out := "H\t/h\n" +
		ps(11, day, "/h/.local/bin/hc-lane watch --all --to t1") + mark(11, "/h/.local/bin/hc-lane", "old111", inst.Add(-24*time.Hour)) + // 옛 판
		ps(12, day, "/h/.local/bin/hc-job-server a b c d") + mark(12, "/h/.local/bin/hc-job-server", "new222", inst) + // 깔린 판 그대로(초 단위로 견줌)
		ps(13, day, "/h/.local/bin/hc-fill watch --every 60") + mark(13, "/h/.local/bin/hc-fill", "cur333", inst.Add(-time.Hour)) + // 같은 판 문자열 — 파일만 다시 쓰임
		ps(14, day, "/h/.local/bin/herdr-balance --watch") + // 판 표시 없음 → 판 모름
		ps(15, day, "/other/.local/bin/hc-lane watch --all --to t2") + // 이 HOME 밖
		ps(16, day, "/h/.local/bin/hc-lane land main") + // 상주 아님
		fmt.Sprintf("B\t/h/.local/bin/hc-lane\t10 %d\n", inst.Unix()) +
		fmt.Sprintf("B\t/h/.local/bin/hc-job-server\t10 %d\n", inst.Unix()) +
		fmt.Sprintf("B\t/h/.local/bin/hc-fill\t10 %d\n", inst.Unix()) +
		fmt.Sprintf("B\t/h/.local/bin/herdr-balance\t10 %d\n", inst.Unix()) + "END\n"
	p, ok := residentParse(out)
	if !ok || p.home != "/h" {
		t.Fatalf("풀기 실패: %+v", p)
	}
	fs := residentStale(p, now, 5*time.Minute, "cur333")
	var got []string
	for _, f := range fs {
		got = append(got, fmt.Sprintf("%d:%s", f.Pid, f.State))
	}
	if strings.Join(got, " ") != "11:old 14:unknown" {
		t.Fatalf("옛 판·판 모름 %v, 바람 [11:old 14:unknown]", got)
	}
	// 방금 깔린 판은 유예 안 — 배포 직후(유예 0)에는 넘어가는 중으로 센다
	if fs := residentStale(p, inst.Add(time.Minute), 5*time.Minute, "cur333"); len(fs) != 0 {
		t.Fatalf("유예 안인데 %v", fs)
	}
	if fs := residentStale(p, inst.Add(time.Minute), 0, "cur333"); len(fs) != 2 {
		t.Fatalf("유예 0인데 %d개", len(fs))
	}
	for _, bad := range []string{"", "H\t/h\nP\tx\n", "H\t/h\n" + ps(1, day, "/h/a") + "BAD\nEND\n", "H\t/h\n" + ps(1, day, "/h/a") + "B\t/h/x\t\nEND\n"} {
		if _, ok := residentParse(bad); ok {
			t.Errorf("온전하지 않은 출력을 받았다: %q", bad)
		}
	}
}

// 조회 조각이 dash·sh에서 돌고 끝까지(END) 내는지 — 표시·실행 파일이 없는 빈 HOME
func TestResidentShRuns(t *testing.T) {
	h := t.TempDir()
	os.MkdirAll(h+"/.local/bin", 0o755)
	os.WriteFile(h+"/.local/bin/hc-lane", []byte("x"), 0o755)
	c := exec.Command("sh", append([]string{"-c", residentSh, "_"}, residentBins()...)...)
	c.Env = append(os.Environ(), "HOME="+h, "XDG_STATE_HOME=")
	out, err := c.Output()
	if err != nil {
		t.Fatal(err)
	}
	p, ok := residentParse(string(out))
	if !ok || p.home != h || len(p.procs) == 0 || len(p.bins) != 1 {
		t.Fatalf("조회: ok %v home %q 프로세스 %d 실행 파일 %v", ok, p.home, len(p.procs), p.bins)
	}
	// 리눅스(GNU stat): -f는 파일 시스템 정보를 여러 줄 찍고 실패한다 — 그 출력이 B 줄에 섞이면 못 셈이 된다(감사 atp2cjf 중간 1)
	fb := t.TempDir()
	os.WriteFile(fb+"/stat", []byte("#!/bin/sh\ncase $1 in\n -f) echo \"  File: \\\"$3\\\"\"; echo '    ID: 4d48 Namelen: 255'; exit 1 ;;\n -c) echo '1 1795619206' ;;\nesac\n"), 0o755)
	c = exec.Command("sh", append([]string{"-c", residentSh, "_"}, residentBins()...)...)
	c.Env = append(os.Environ(), "HOME="+h, "XDG_STATE_HOME=", "PATH="+fb+":"+os.Getenv("PATH"))
	if out, err = c.Output(); err != nil {
		t.Fatal(err)
	}
	p, ok = residentParse(string(out))
	if b := p.bins[h+"/.local/bin/hc-lane"]; !ok || b.Size != 1 || b.Mtime.Unix() != 1795619206 {
		t.Fatalf("GNU stat: ok %v 실행 파일 %v\n%s", ok, p.bins, out)
	}
}
