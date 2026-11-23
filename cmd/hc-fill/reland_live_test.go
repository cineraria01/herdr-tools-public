package main

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// 원래 레인이 살아 있으면 다시 맞춤 대신 그 터미널에 보냄(reland-live-lane): 살아 있음 → 보냄·다시 맞춤 없음, 닫힘·작업 트리 없음 →
// 다시 맞춤, 기계 연결 안 됨(모름) → 다시 맞춤, 같은 반송은 두 번 보내지 않음, 보낸 뒤 reland_wait분 안에 안 냄 → 다시 맞춤,
// 그 안에 다시 냄(제출 사건) → 다시 냄으로 끝
// rlRepo: 새 DATA에 원격(o.git)과 작업 클론(w), 가짜 hc-lane(report)·hc(send) — 인자를 한 줄로 reports·sends에
func rlRepo(t *testing.T) (string, func(dir string, a ...string)) {
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
	for _, b := range []struct{ name, out string }{{"hc-lane", "reports"}, {"hc", "sends"}} {
		os.WriteFile(DATA+"/"+b.name, []byte("#!/bin/sh\nprintf '%s|' \"$@\" >>"+DATA+"/"+b.out+"\necho >>"+DATA+"/"+b.out+"\n"), 0o755)
	}
	t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
	confLines = lines("local here\nrepo app here " + w + "\nrepo app far /far/w\nmerger app box\n")
	LOCAL, MGM, dry, EVBAD, CONN = "here", " box", false, false, nil
	MQP, MQS, MQBAD = "", nil, ""
	os.WriteFile(DATA+"/reland.tsv", []byte("#since\t2026-11-22T00:00:00Z\n"), 0o644)
	return w, git
}

// 병합기가 그 레인의 새 제출을 지금 착지 중이면(refs/herdr/landing/<큐> = 큐 SHA) 다시 맞춤도 보내기도 안 함(「착지 중」),
// 큐에 서 있기만 해도 새 제출이라 안 함(「다시 냄」). 남은 표식(큐 SHA와 다름)은 착지 중이 아님
func TestRelandLanding(t *testing.T) {
	w, git := rlRepo(t)
	conflict := "착지 실패(종료 3, 충돌 README.md)"
	EV = nil
	for i, l := range []string{"l-land", "l-q", "l-stale"} {
		n := string(rune('1'+i)) + "00-" + l
		git(w, "push", "-q", "origin", "HEAD:refs/merge-failed/"+n)
		EV = append(EV, rlEv("2026-11-22T00:0"+string(rune('1'+i))+":00Z", l, n, conflict, "t9"))
	}
	git(w, "push", "-q", "origin", "HEAD:refs/merge-queue/1795317520-l-land", "HEAD:refs/herdr/landing/1795317520-l-land", "HEAD:refs/merge-queue/1795317530-l-q")
	git(w, "commit", "-q", "--allow-empty", "-m", "old")
	git(w, "push", "-q", "origin", "HEAD:refs/herdr/landing/1795317540-l-stale")
	OREC, TERMS = nil, []Term{{"here", "here/t1", "claude", "l-land", "idle", "-"}}
	os.MkdirAll(w+".wt/l-land", 0o755) // 원래 레인이 살아 있어도 착지 중이면 보내지 않는다
	relandTick()
	if inQ := qrow("l-stale-reland") != nil; !inQ || qrow("l-land-reland") != nil || qrow("l-q-reland") != nil {
		t.Fatalf("착지 중·큐에 섬은 안 넣고, 남은 표식은 넣는다: %v", qRows())
	}
	if s, _ := readFile(DATA + "/sends"); s != "" {
		t.Fatalf("착지 중인 레인에 보냄: %q", s)
	}
	rl, _ := readFile(DATA + "/reland.tsv")
	for _, s := range []string{"100-l-land\tl-land\tl-land\t착지 중\t-", "200-l-q\tl-q\tl-q\t다시 냄\t-", "300-l-stale\tl-stale\tl-stale\t넣음"} {
		if !strings.Contains(rl, s) {
			t.Errorf("기록에 %q 없음:\n%s", s, rl)
		}
	}
}

func TestRelandLiveLane(t *testing.T) {
	w, git := rlRepo(t)
	lanes := []string{"l-live", "l-gone", "l-far", "l-nowt", "l-back"}
	for i, l := range lanes {
		git(w, "push", "-q", "origin", "HEAD:refs/merge-failed/"+string(rune('1'+i))+"00-"+l)
	}
	for _, l := range []string{"l-live", "l-back", "l-gone"} { // l-gone은 작업 트리만 남고 터미널이 없다
		if err := os.MkdirAll(w+".wt/"+l, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	OREC = []Row{
		{"l-live", "app", "here", "here/t1", "2026-11-22T00:00:00Z", "-", ",-,", "-"},
		{"l-gone", "app", "here", "here/t2", "2026-11-22T00:00:00Z", "-", ",-,", "-"},
		{"l-far", "app", "far", "far/t3", "2026-11-22T00:00:00Z", "-", ",-,", "-"},
		{"l-back", "app", "here", "here/t5", "2026-11-22T00:00:00Z", "-", ",-,", "-"},
	}
	TERMS = []Term{ // l-nowt는 실행 기록 없이 터미널 표에만(작업 트리 없음)
		{"here", "here/t1", "claude", "l-live", "idle", "-"},
		{"here", "here/t4", "claude", "l-nowt", "idle", "-"},
		{"here", "here/t5", "claude", "l-back", "working", "-"},
	}
	conflict := "착지 실패(종료 3, 충돌 README.md)"
	EV = nil
	for i, l := range lanes {
		EV = append(EV, rlEv("2026-11-22T00:0"+string(rune('1'+i))+":00Z", l, string(rune('1'+i))+"00-"+l, conflict, "t9"))
	}
	tick := func() {
		MQP, MQS, MQBAD = "", nil, ""
		relandTick()
	}
	inQ := func(n string) bool { return qrow(n) != nil }

	tick()
	for _, n := range []string{"l-gone-reland", "l-far-reland", "l-nowt-reland"} {
		if !inQ(n) {
			t.Errorf("닫힘·모름·작업 트리 없음은 다시 맞춤: %s 없음 %v", n, qRows())
		}
	}
	if inQ("l-live-reland") || inQ("l-back-reland") {
		t.Fatalf("살아 있는 원래 레인은 다시 맞춤을 넣지 않는다: %v", qRows())
	}
	sends, _ := readFile(DATA + "/sends")
	if strings.Count(sends, "\n") != 2 || !strings.Contains(sends, "send|here/t1|병합기 반송(rebase 충돌 README.md, refs/merge-failed/100-l-live) — origin/main으로 rebase해 양쪽 살려 풀고 푼 파일 둘레의 빌드·시험만 돌린 뒤 hc-lane land main 다시(전체 검증은 병합기가 돈다)|") ||
		!strings.Contains(sends, "send|here/t5|") {
		t.Fatalf("원래 레인에 보냄: %q", sends)
	}
	rep, _ := readFile(DATA + "/reports")
	if !strings.Contains(rep, "report|중간|-|원래 레인에 보냄 l-live(here/t1) — 병합기 rebase 충돌 반송(refs/merge-failed/100-l-live") ||
		!strings.Contains(rep, "--to|t9|--key|hf:reland:100-l-live:sent|") {
		t.Fatalf("주인 알림: %q", rep)
	}
	rl, _ := readFile(DATA + "/reland.tsv")
	if !strings.Contains(rl, "100-l-live\tl-live\tl-live\t보냄\there/t1\t") {
		t.Fatalf("기록 보냄: %q", rl)
	}

	n := len(qRows())
	tick() // 같은 반송: 기다리는 동안 다시 보내지도 넣지도 않는다
	if s2, _ := readFile(DATA + "/sends"); s2 != sends || len(qRows()) != n {
		t.Fatalf("같은 반송을 두 번 처리: %q %v", s2, qRows())
	}

	// l-back은 보낸 뒤 다시 냄(제출 사건), 둘 다 보낸 지 30분 넘음
	EV = append(EV, strings.Join([]string{"2099-01-01T00:00:00Z", "here", "-", "l-back", "제출", "-", "1700-l-back", "-", "t9", "-", "-", "-"}, "\t"))
	// 보냄 줄은 보내기 전 시도 줄과 보낸 뒤 알림글 줄 둘(재감사2 fill/R-215) — 둘 다 옛 시각으로
	rl = regexp.MustCompile(`(\t보냄\there/t[15]\t)[0-9TZ:-]+`).ReplaceAllString(rl, "${1}2026-11-22T00:00:00Z")
	os.WriteFile(DATA+"/reland.tsv", []byte(rl), 0o644)
	tick()
	if !inQ("l-live-reland") {
		t.Fatalf("보낸 뒤 30분 무제출 → 다시 맞춤: %v", qRows())
	}
	if inQ("l-back-reland") {
		t.Fatalf("다시 낸 레인은 다시 맞춤 없음: %v", qRows())
	}
	rl, _ = readFile(DATA + "/reland.tsv")
	if !strings.Contains(rl, "500-l-back\tl-back\tl-back\t다시 냄\there/t5\t") || !strings.Contains(rl, "100-l-live\tl-live\tl-live\t넣음\tl-live-reland\t") {
		t.Fatalf("기록: %q", rl)
	}
	// 보냄 뒤 넣음 알림은 보냄과 다른 키(같은 키면 hc-lane report --key가 이미 있다고 삼킨다 — 작업 중 발견해 함께 고침)
	if rep, _ = readFile(DATA + "/reports"); !strings.Contains(rep, "자동 다시 맞춤 넣음 l-live-reland — l-live 병합기 rebase 충돌 반송(refs/merge-failed/100-l-live, 충돌 README.md)|--name|l-live|--to|t9|--key|hf:reland:100-l-live|") {
		t.Fatalf("보냄 뒤 넣음 알림 키: %q", rep)
	}
	n = len(qRows())
	tick()
	if s3, _ := readFile(DATA + "/sends"); s3 != sends || len(qRows()) != n {
		t.Fatalf("끝난 반송을 또 처리: %q %v", s3, qRows())
	}
}

// 이름을 바꿔 낸 제출(<원래>-fix가 HC_LANE=<원래>로): 원래 이름 터미널은 닫혔어도, 반송된 제출 기록(refs/merge-failed-meta)의
// host=·term=이 가리키는 낸 터미널이 살아 있고 그 작업 트리가 남았으면 다시 맞춤 대신 그쪽에 보낸다
func TestRelandRenamedSubmit(t *testing.T) {
	w, git := rlRepo(t)
	git(w, "push", "-q", "origin", "HEAD:refs/merge-failed/100-l-orig")
	git(w, "commit", "-q", "--allow-empty", "-m", "merge-queue lane=l-orig id=x orig=a fork=b base=main host=here term=t7 owner=t9 job=- created_ms=- priority=보통")
	git(w, "push", "-q", "origin", "HEAD:refs/merge-failed-meta/100-l-orig")
	if err := os.MkdirAll(w+".wt/l-orig-fix", 0o755); err != nil {
		t.Fatal(err)
	}
	OREC = []Row{{"l-orig", "app", "here", "here/t2", "2026-11-22T00:00:00Z", "-", ",-,", "-"}} // 원래 레인 터미널은 닫힘
	TERMS = []Term{{"here", "t7", "claude", "l-orig-fix", "idle", "-"}}
	EV = []string{rlEv("2026-11-22T00:01:00Z", "l-orig", "100-l-orig", "착지 실패(종료 3, 충돌 README.md)", "t9")}
	relandTick()
	if qrow("l-orig-reland") != nil {
		t.Fatalf("낸 터미널이 살아 있으면 다시 맞춤 없음: %v", qRows())
	}
	if s, _ := readFile(DATA + "/sends"); !strings.Contains(s, "send|t7|병합기 반송(rebase 충돌 README.md, refs/merge-failed/100-l-orig)") {
		t.Fatalf("낸 터미널에 보냄: %q", s)
	}
	if rl, _ := readFile(DATA + "/reland.tsv"); !strings.Contains(rl, "100-l-orig\tl-orig\tl-orig\t보냄\tt7\t") {
		t.Fatalf("기록 보냄: %q", rl)
	}
}
