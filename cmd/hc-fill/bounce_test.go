package main

import (
	"os"
	"strings"
	"testing"
)

// bnRepo: rlRepo 위에 — 원격에 main(base)을 올리고, 가짜 hc는 exec를 이 기계에서 그대로 돌린다(병합기 기계 box의 로그·레인 기계의
// coord-notes를 읽는 길까지 시험). 반송 사건의 로그는 DATA/logs/<레인>.log
func bnRepo(t *testing.T) (string, func(dir string, a ...string)) {
	w, git := rlRepo(t)
	t.Setenv("HOME", DATA)
	git(w, "push", "-q", "origin", "HEAD:refs/heads/main")
	git(w, "fetch", "-q", "origin")
	git(w, "remote", "set-head", "origin", "main")
	os.WriteFile(DATA+"/hc", []byte("#!/bin/sh\nprintf '%s|' \"$@\" >>"+DATA+"/sends\necho >>"+DATA+"/sends\n"+
		"if [ \"$1\" = exec ]; then while [ \"$1\" != -- ]; do shift; done; shift; exec \"$@\"; fi\n"), 0o755)
	CONN = []string{"box"}
	OREC, TERMS = nil, nil
	return w, git
}

// bnEv: 병합기 검증 실패 반송 사건(cmd/hc-merger fail()의 글 꼴, 로그는 DATA/logs/<레인>.log)
func bnEv(tm, lane, n string) string {
	return strings.Replace(rlEv(tm, lane, n, "착지 실패(종료 1), 직접 검증(맡길 한가한 기계 없음)", "t9"), "/s/merger-logs/"+lane+".log", DATA+"/logs/"+lane+".log", 1)
}

// bnRow: 떠 있는(up) 자동 감사 대기열 줄
func bnRow(t *testing.T, lane string) Row {
	t.Helper()
	os.WriteFile(DATA+"/"+lane+".md", []byte("# "+lane+"\n본 작업\n"), 0o644)
	r := pad16(lane, "app", DATA+"/"+lane+".md", "-", "-", "up", "here", "here/t5", "-", "2026-11-22T00:00", "-", "t9", "normal", "-", "auto")
	if err := appendLine(Q, r.Line()); err != nil {
		t.Fatal(err)
	}
	return r
}

// 검증 실패 반송 → 그 줄을 같은 이름·같은 줄 ID로 「반송됨」 대기(감사 칸 그대로), 꼬리 지시문(반송 커밋으로 옮기기·실패 줄·지휘 메모),
// 주인 알림 한 번, status 단계 「반송됨 — 다시 띄움 대기」. 두 번째 바퀴는 아무것도 더 안 함
func TestBounceRelaunch(t *testing.T) {
	w, git := bnRepo(t)
	git(w, "commit", "-q", "--allow-empty", "-m", "l-v: 일")
	git(w, "push", "-q", "origin", "HEAD:refs/merge-failed/700-l-v")
	row := bnRow(t, "l-v")
	os.MkdirAll(DATA+"/logs", 0o755)
	os.WriteFile(DATA+"/logs/l-v.log", []byte("앞 줄\nhc-lane land: 실패 줄(로그에서, 최대 40줄):\n✗ smoke-tablet 실패 (0초)\n✖ worker-errors.test.mjs\n"), 0o644)
	os.MkdirAll(DATA+"/.local/share/hc-fill/verdicts/l-v", 0o755)
	os.WriteFile(DATA+"/.local/share/hc-fill/verdicts/l-v/coord-notes.md", []byte("## 2026-11-22T00:00:00Z 지휘\nREADME는 건드리지 말 것\n"), 0o644)
	EV = []string{bnEv("2026-11-22T00:01:00Z", "l-v", "700-l-v")}
	relandTick()
	r := qrow("l-v")
	if r == nil || r.F(qState) != "wait" || r.F(qEarly) != "bounced" || r.F(qAudit) != "auto" || r.F(qRID) != row.F(qRID) || r.F(qMach) != "-" {
		t.Fatalf("반송된 줄은 같은 줄 ID로 반송됨 대기: %v", r)
	}
	if len(qRows()) != 1 {
		t.Fatalf("새 줄(-reland)을 넣지 않는다: %v", qRows())
	}
	tail := bounceSec("l-v")
	for _, s := range []string{"git fetch origin refs/merge-failed/700-l-v ", "git reset --keep FETCH_HEAD", "git rebase origin/main",
		"✗ smoke-tablet 실패", "✖ worker-errors.test.mjs", "README는 건드리지 말 것", "hc-lane report 막힘"} {
		if !strings.Contains(tail, s) {
			t.Errorf("꼬리에 %q 없음:\n%s", s, tail)
		}
	}
	rep, _ := readFile(DATA + "/reports")
	if !strings.Contains(rep, "report|중간|-|반송 다시 띄움 l-v") || !strings.Contains(rep, "--key|hf:reland:700-l-v|") {
		t.Fatalf("주인 알림: %q", rep)
	}
	if s := stStage(stIn{State: r.F(qState), Early: r.F(qEarly)}); s != "반송됨 — 다시 띄움 대기" {
		t.Fatalf("status 단계: %q", s)
	}
	MQP, MQS, MQBAD = "", nil, ""
	relandTick()
	if rep2, _ := readFile(DATA + "/reports"); rep2 != rep {
		t.Fatalf("같은 반송을 두 번 처리: %q", rep2)
	}
	rl, _ := readFile(DATA + "/reland.tsv")
	if !strings.Contains(rl, "700-l-v\tl-v\tl-v\t다시 띄움\tl-v\t") {
		t.Fatalf("장부: %s", rl)
	}
}

// 지휘가 줄을 뺐으면(rm) 안 띄움 · 반송 커밋이 이미 main에 있으면(다른 이름으로 cherry-pick해 들어감) 안 띄움 · 상한이면 막힘
func TestBounceSkip(t *testing.T) {
	w, git := bnRepo(t)
	git(w, "checkout", "-q", "-b", "side")
	os.WriteFile(w+"/a.txt", []byte("a\n"), 0o644)
	git(w, "add", "a.txt")
	git(w, "commit", "-q", "-m", "l-x: a")
	git(w, "push", "-q", "origin", "HEAD:refs/merge-failed/800-l-x", "HEAD:refs/merge-failed/810-l-rm", "HEAD:refs/merge-failed/820-l-max")
	git(w, "checkout", "-q", "main")
	git(w, "cherry-pick", "side")
	git(w, "commit", "-q", "--allow-empty", "-m", "다른 커밋") // main 쪽 sha가 반송 커밋과 다르다
	git(w, "push", "-q", "origin", "HEAD:refs/heads/main")
	bnRow(t, "l-x")
	git(w, "checkout", "-q", "-b", "side2", "HEAD~2")
	os.WriteFile(w+"/b.txt", []byte("b\n"), 0o644)
	git(w, "add", "b.txt")
	git(w, "commit", "-q", "-m", "l-max: b")
	git(w, "push", "-q", "-f", "origin", "HEAD:refs/merge-failed/820-l-max")
	bnRow(t, "l-max")
	appendLine(DATA+"/reland.tsv", "1-l-max\tl-max\tl-max\t다시 띄움\tl-max\t2026-11-22T00:00:00Z")
	appendLine(DATA+"/reland.tsv", "2-l-max\tl-max\tl-max\t다시 띄움\tl-max\t2026-11-22T00:00:00Z")
	EV = []string{bnEv("2026-11-22T00:01:00Z", "l-x", "800-l-x"), bnEv("2026-11-22T00:02:00Z", "l-rm", "810-l-rm"),
		bnEv("2026-11-22T00:03:00Z", "l-max", "820-l-max")}
	relandTick()
	if r := qrow("l-x"); r == nil || r.F(qState) != "up" {
		t.Fatalf("이미 main에 든 커밋은 다시 띄우지 않는다: %v", r)
	}
	if r := qrow("l-max"); r == nil || r.F(qState) != "up" {
		t.Fatalf("상한이면 다시 띄우지 않는다: %v", r)
	}
	if len(qRows()) != 2 {
		t.Fatalf("줄을 더하지 않는다: %v", qRows())
	}
	rl, _ := readFile(DATA + "/reland.tsv")
	for _, s := range []string{"800-l-x\tl-x\tl-x\t이미 main\t-", "810-l-rm\tl-rm\tl-rm\t줄 없음\t-", "820-l-max\tl-max\tl-max\t상한\t-"} {
		if !strings.Contains(rl, s) {
			t.Errorf("장부에 %q 없음:\n%s", s, rl)
		}
	}
	if rep, _ := readFile(DATA + "/reports"); !strings.Contains(rep, "report|막힘|-|l-max 병합기 착지 실패(종료 1), 직접 검증(맡길 한가한 기계 없음)로 또 반송") {
		t.Fatalf("상한 막힘: %q", rep)
	}
}

// 반송된 줄을 띄울 때 같은 이름의 옛 실행 터미널(hc-lane이 「같은 이름」으로 띄우기를 거절한다): 제출하고 끝내 쉬면 닫고,
// 돌거나 마지막 보고가 제출이 아니면 닫지 않는다(가짜 hc — parkclose_test의 pcSetup)
func TestBounceCloseOld(t *testing.T) {
	for _, c := range []struct {
		status, report string
		gone           bool
	}{{"idle", "제출", true}, {"done", "제출", true}, {"working", "제출", false}, {"idle", "완료", false}} {
		k := pcSetup(t, c.status, c.report, "wait", "bounced")
		gone, why := endClose("pk", "m/t1", "m", "제출")
		if gone != c.gone || k.pcKills() != map[bool]int{true: 1, false: 0}[c.gone] {
			t.Errorf("%s·%s: 닫힘 %v(%s) kill %d, 바람 %v", c.status, c.report, gone, why, k.pcKills(), c.gone)
		}
	}
}

// 한 바퀴에 같은 레인의 반송 둘(아직 안 뜬 반송됨 줄에 새 반송) — 절은 뒤 반송을 가리킨다(감사 낮음 1)
func TestBounceTwoInOneRound(t *testing.T) {
	w, git := bnRepo(t)
	git(w, "commit", "-q", "--allow-empty", "-m", "l-y: 하나")
	git(w, "push", "-q", "origin", "HEAD:refs/merge-failed/900-l-y")
	git(w, "commit", "-q", "--allow-empty", "-m", "l-y: 둘")
	git(w, "push", "-q", "origin", "HEAD:refs/merge-failed/910-l-y")
	bnRow(t, "l-y")
	EV = []string{bnEv("2026-11-22T00:01:00Z", "l-y", "900-l-y"), bnEv("2026-11-22T00:02:00Z", "l-y", "910-l-y")}
	relandTick()
	if s := bounceSec("l-y"); !strings.Contains(s, "refs/merge-failed/910-l-y ") || strings.Contains(s, "900-l-y") {
		t.Fatalf("절이 뒤 반송을 가리켜야: %s", s)
	}
	if r := qrow("l-y"); r == nil || r.F(qState) != "wait" || r.F(qEarly) != "bounced" {
		t.Fatalf("줄: %v", r)
	}
}

// 반송 까닭: 추측 검증 까닭은 괄호 안의 「 — 」까지 담는다(감사 낮음 2)
func TestBounceWhy(t *testing.T) {
	for msg, want := range map[string]string{
		"병합기: 착지 실패(종료 1, 추측 검증 — m1 에서 기준 abc 위 시험 실패, 첫 실패 「TestX」) — 원본 aaaa, 기준 bbbb": "착지 실패(종료 1, 추측 검증 — m1 에서 기준 abc 위 시험 실패, 첫 실패 「TestX」)",
		"병합기: 착지 실패(종료 1), 직접 검증(맡길 한가한 기계 없음) — 원본 aaaa":                                 "착지 실패(종료 1), 직접 검증(맡길 한가한 기계 없음)",
		"병합기: 착지 실패(종료 1) — 다른 꼴":                                                         "착지 실패(종료 1)",
	} {
		if got := bounceWhy(msg); got != want {
			t.Errorf("%q → %q, 바람 %q", msg, got, want)
		}
	}
}

// 살아 있는 원래 레인에 보내는 글: 충돌이 아닌 반송은 그 실패를 고쳐 다시 내라고
func TestBounceSendMsg(t *testing.T) {
	s := relandSendMsg("", "100-l")
	for _, w := range []string{"refs/merge-failed/100-l", "rebase", "그 실패", "hc-lane land main"} {
		if !strings.Contains(s, w) {
			t.Errorf("%q 없음: %s", w, s)
		}
	}
}
