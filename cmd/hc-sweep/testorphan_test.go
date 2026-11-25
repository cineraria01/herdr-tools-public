package main

import (
	"strings"
	"testing"
	"time"
)

// 시험 고아 점검 — 가짜 ps. 부모 없는 시험 경로 프로세스가 문턱을 넘으면 사건 한 번(같은 pid·시작 시각에 되풀이 없음),
// 부모가 있거나 시험 경로가 아니거나 문턱 안이면 없음, ps 실패는 모름
func TestTestOrphanOnce(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().Truncate(time.Second)
	old := now.Add(-30 * time.Hour).Format("Mon Jan _2 15:04:05 2006")
	young := now.Add(-time.Minute).Format("Mon Jan _2 15:04:05 2006")
	f := &fakePS{out: "" +
		"  71     1 275.0 " + old + " /vol/herdr-verify-ram-9/t/herdr-test-bin.5.1/herdr-sync --check\n" + // 잡음
		"  72   500   0.0 " + old + " /x/T/herdr-test-bin.5.1/hc-merger --repo /x --tick 17\n" + // 부모 있음(시험 도는 중)
		"  73     1   0.0 " + old + " /u/.local/bin/hc-merger --repo /u/m main\n" + // 시험 경로 아님(진짜 상주)
		"  74     1   0.0 " + young + " /bin/sh /x/T/TestZ1/004/fake-agent -p x\n" + // 문턱 안
		"  75     1   1.5 " + old + " /bin/sh /x/T/TestZ1/004/fake-agent -p x\n"} // 잡음(인자에 시험 경로)
	var sent []string
	rep := func(msg, key string) bool { sent = append(sent, msg+"|"+key); return true }
	state := dir + "/sent"
	testOrphanCheck("box-1", state, false, f.run, rep, now, 30*time.Minute)
	testOrphanCheck("box-1", state, false, f.run, rep, now.Add(time.Minute), 30*time.Minute)
	if len(sent) != 2 || !strings.Contains(sent[0], "pid 71") || !strings.Contains(sent[0], "CPU 275.0%") || !strings.Contains(sent[0], "kill 71") ||
		!strings.Contains(sent[0], "|hc-test-orphan:box-1:71:") || !strings.Contains(sent[1], "pid 75") {
		t.Fatalf("71·75에 한 번씩이어야: %q", sent)
	}
	// 고아가 사라지면 기록에서 빠진다 — 같은 번호의 새 고아(시작 시각 다름)는 다시 알린다
	f.out = "  71     1   0.0 " + now.Add(-2*time.Hour).Format("Mon Jan _2 15:04:05 2006") + " /tmp/TestX1/001/hc-fill run\n"
	testOrphanCheck("box-1", state, false, f.run, rep, now, 30*time.Minute)
	if len(sent) != 3 || !strings.Contains(sent[2], "hc-fill run") {
		t.Fatalf("새 고아는 한 번 더: %q", sent)
	}
	if lines := testOrphanCheck("box-1", state, false, (&fakePS{rc: 1}).run, rep, now, 30*time.Minute); len(sent) != 3 || !strings.Contains(strings.Join(lines, "\n"), "모름") {
		t.Fatalf("ps 실패: 사건 %q 줄 %v", sent, lines)
	}
}
