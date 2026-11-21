package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRelandName(t *testing.T) {
	for _, c := range []struct {
		base string
		k    int
		want string
	}{
		{"fill-gc", 1, "fill-gc-reland"},
		{"fill-gc", 2, "fill-gc-reland2"},
		{"abcdefghij-abcdefghij-abcdefghij", 1, "abcdefghij-abcdefghij-abc-reland"},
		{"abcdefghij-abcdefghij-abcd-efgh", 2, "abcdefghij-abcdefghij-ab-reland2"},
		{"abcdefghijabcdefghijabcd-xyzw", 1, "abcdefghijabcdefghijabcd-reland"}, // 자른 끝 - 는 뗀다
	} {
		got := relandName(c.base, c.k)
		if got != c.want || !okname(got) {
			t.Errorf("relandName(%q,%d) = %q, 바람 %q", c.base, c.k, got, c.want)
		}
	}
}

// rlEv: 병합기 기계 box가 적은 반송 사건(cmd/hc-merger fail()의 글 꼴)
func rlEv(tm, lane, n, why, to string) string {
	msg := "병합기: " + why + " — 원본 aaaaaaaa, 기준 bbbbbbbb, 제출 ID box-" + lane + ", refs/merge-failed/" + n + " 로 옮김(lane/" + lane + " 는 그대로), 로그 /s/merger-logs/" + lane + ".log"
	return strings.Join([]string{tm, "box", "-", lane, "막힘", "-", msg, "-", to, "-", "-", "-"}, "\t")
}

// relandTick: rebase 충돌 반송만 다시 맞춤 줄로(같은 프로젝트·주인·높음·감사 끔), 검증 실패·since 앞·원격에 없는 반송은 안 넣음,
// 같은 바탕 레인은 reland_max번까지 그 뒤 막힘, 같은 반송은 두 번 넣지 않음
func TestRelandTick(t *testing.T) {
	setData(t)
	git := func(dir string, a ...string) {
		t.Helper()
		c := exec.Command("git", a...)
		c.Dir = dir
		if o, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, o)
		}
	}
	for k, v := range map[string]string{"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_AUTHOR_NAME": "t",
		"GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t"} {
		t.Setenv(k, v)
	}
	o, w := DATA+"/o.git", DATA+"/w"
	git(DATA, "init", "-q", "--bare", "-b", "main", o)
	git(DATA, "init", "-q", "-b", "main", w)
	git(w, "commit", "-q", "--allow-empty", "-m", "base")
	git(w, "remote", "add", "origin", o)
	for _, n := range []string{"100-l-a", "300-l-a-reland", "400-l-a-reland2", "500-l-b"} {
		git(w, "push", "-q", "origin", "HEAD:refs/merge-failed/"+n)
	}
	// 가짜 hc-lane: report 인자를 한 줄로
	os.WriteFile(DATA+"/hc-lane", []byte("#!/bin/sh\nprintf '%s|' \"$@\" >>"+DATA+"/reports\necho >>"+DATA+"/reports\n"), 0o755)
	t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
	confLines = lines("local here\nrepo app here " + w + "\nmerger app box\n")
	LOCAL, MGM, dry, EVBAD = "here", " box", false, false
	conflict := "착지 실패(종료 3, 충돌 a.go b.go)"
	EV = []string{rlEv("2026-11-22T00:00:00Z", "l-a", "100-l-a", conflict, "t9")}
	relandTick() // 첫 바퀴: 기록 파일만 만든다(그 앞 반송은 안 봄)
	if q := qRows(); len(q) != 0 {
		t.Fatalf("첫 바퀴는 넣지 않는다: %v", q)
	}
	os.WriteFile(DATA+"/reland.tsv", []byte("#since\t2026-11-22T00:00:00Z\n"), 0o644)
	// 검증 실패 — 안 넣음 · 원격에 없음 — 기록만 · since 앞 — 안 봄 · 병합기 기계 아님 — 안 봄
	EV = append(EV,
		rlEv("2026-11-22T00:01:00Z", "l-c", "200-l-c", "착지 실패(종료 1)", "t9"),
		rlEv("2026-11-22T00:02:00Z", "l-d", "250-l-d", conflict, "t9"),
		rlEv("2026-11-21T23:00:00Z", "l-b", "500-l-b", conflict, "t9"),
		strings.Replace(rlEv("2026-11-22T00:03:00Z", "l-e", "260-l-e", conflict, "-"), "\tbox\t", "\tother\t", 1),
	)
	relandTick()
	r := qrow("l-a-reland")
	if r == nil || r.F(2) != "app" || r.F(6) != "wait" || r.F(12) != "t9" || r.F(13) != "high" || r.F(15) != "no-audit" {
		t.Fatalf("다시 맞춤 줄: %v", r)
	}
	if len(qRows()) != 1 {
		t.Fatalf("다른 반송은 넣지 않는다: %v", qRows())
	}
	b, _ := readFile(r.F(3))
	for _, s := range []string{"git fetch origin refs/merge-failed/100-l-a ", "충돌 파일(병합기가 3자 합침으로 본 것): a.go b.go", "/s/merger-logs/l-a.log", "머리 l-a:", "rebase --abort 금지"} {
		if !strings.Contains(b, s) {
			t.Errorf("지시문에 %q 없음:\n%s", s, b)
		}
	}
	rep, _ := readFile(DATA + "/reports")
	if !strings.Contains(rep, "report|중간|-|자동 다시 맞춤 넣음 l-a-reland — l-a") || !strings.Contains(rep, "--to|t9|--key|hf:reland:100-l-a|") {
		t.Fatalf("주인 알림: %q", rep)
	}
	relandTick() // 같은 반송을 다시 보지 않는다
	if rep, _ = readFile(DATA + "/reports"); len(qRows()) != 1 || strings.Count(rep, "\n") != 1 {
		t.Fatalf("두 번 넣음: %v", qRows())
	}
	// 다시 맞춤이 또 반송 → 둘째(-reland2), 그것도 반송 → 상한(2) 넘어 막힘
	os.WriteFile(Q, nil, 0o644)
	EV = append(EV, rlEv("2026-11-22T01:00:00Z", "l-a-reland", "300-l-a-reland", conflict, "t9"))
	relandTick()
	if qrow("l-a-reland2") == nil {
		t.Fatalf("둘째 다시 맞춤: %v", qRows())
	}
	EV = append(EV, rlEv("2026-11-22T02:00:00Z", "l-a-reland2", "400-l-a-reland2", conflict, "t9"))
	relandTick()
	rep, _ = readFile(DATA + "/reports")
	if len(qRows()) != 1 || !strings.Contains(rep, "report|막힘|-|l-a-reland2 병합기 rebase 충돌로 또 반송 — 자동 다시 맞춤 2번 넘음") {
		t.Fatalf("상한 막힘: %v %q", qRows(), rep)
	}
	rl, _ := readFile(DATA + "/reland.tsv")
	for _, s := range []string{"250-l-d\tl-d\tl-d\t없음", "100-l-a\tl-a\tl-a\t넣음\tl-a-reland", "400-l-a-reland2\tl-a-reland2\tl-a\t상한"} {
		if !strings.Contains(rl, s) {
			t.Errorf("기록에 %q 없음:\n%s", s, rl)
		}
	}
	// 끔(reland_max 0)
	confLines = append(confLines, "reland_max 0")
	EV = append(EV, rlEv("2026-11-22T03:00:00Z", "l-f", "600-l-f", conflict, "t9"))
	n := len(qRows())
	relandTick()
	if len(qRows()) != n {
		t.Fatal("reland_max 0이면 끔")
	}
}
