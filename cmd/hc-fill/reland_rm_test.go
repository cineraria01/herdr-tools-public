package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/journal"
)

// ht-fill-smalls-1128 ①: 지휘가 hc-fill rm으로 뺀 레인의 rebase 충돌 반송은 다시 맞춤(<레인>-reland)을 넣지 않는다
// (11-27 — rm --force 직후 같은 레인의 반송을 본 자동 다시 맞춤이 -reland를 넣어 일부러 닫은 레인이 되살아났다).
// 대기열에 줄이 있는 레인·대기열 밖 레인(줄도 rm 기록도 없음)은 지금처럼 넣는다.

func rmFix(t *testing.T) func(dir string, a ...string) {
	t.Helper()
	w, git := rlRepo(t)
	t.Setenv(journal.Env, DATA+"/journal.jsonl")
	for _, n := range []string{"100-l-rm", "110-l-rm2", "120-l-up", "130-l-direct", "140-l-race", "150-l-late"} {
		git(w, "push", "-q", "origin", "HEAD:refs/merge-failed/"+n)
	}
	return git
}

func rmRow(t *testing.T, lane string) {
	t.Helper()
	os.WriteFile(DATA+"/"+lane+".md", []byte("# "+lane+"\n"), 0o644)
	if err := appendLine(Q, pad16(lane, "app", DATA+"/"+lane+".md", "-", "-", "up", "here", "here/t5", "-", "2026-11-22T00:00", "-", "t9", "normal", "-", "auto").Line()); err != nil {
		t.Fatal(err)
	}
}

func rmLane(t *testing.T, lane string) {
	t.Helper()
	dupCapture(t, func() { rmCmd([]string{lane, "--force", "--why", "지휘가 닫음"}) })
}

const rmConflict = "착지 실패(종료 3, 충돌 a.go)"

func TestRelandSkipsRemovedLane(t *testing.T) {
	rmFix(t)
	// rm 뒤에 온 반송 · rm 앞에 온 반송(바퀴가 rm 뒤에 돎) — 둘 다 넣지 않고 한 줄
	rmRow(t, "l-rm")
	rmRow(t, "l-rm2")
	rmRow(t, "l-up")
	rmLane(t, "l-rm")
	rmLane(t, "l-rm2")
	after := time.Now().UTC().Add(time.Minute).Format("2006-01-02T15:04:05Z") // rm보다 뒤 · 아래 l-rm2는 rm보다 앞
	EV = []string{rlEv(after, "l-rm", "100-l-rm", rmConflict, "t9"),
		rlEv("2026-11-22T00:00:30Z", "l-rm2", "110-l-rm2", rmConflict, "t9")}
	// 둘째 바퀴 전에 같은 반송을 또 본다(사건 줄이 겹침)
	EV = append(EV, EV[0])
	out := dupCapture(t, relandTick)
	if qrow("l-rm-reland") != nil || qrow("l-rm2-reland") != nil || len(qRows()) != 1 {
		t.Fatalf("지운 레인은 다시 맞춤을 넣지 않는다: %v", qRows())
	}
	for _, l := range []string{"l-rm", "l-rm2"} {
		if strings.Count(out, "반송 다시 띄움 안 함 "+l+": 대기열에서 지워진 레인") != 1 {
			t.Fatalf("한 줄만: %q", out)
		}
	}
	rl, _ := readFile(DATA + "/reland.tsv")
	if !strings.Contains(rl, "100-l-rm\tl-rm\tl-rm\t줄 없음\t-") || !strings.Contains(rl, "110-l-rm2\tl-rm2\tl-rm2\t줄 없음\t-") {
		t.Fatalf("장부: %s", rl)
	}
	if rep, _ := readFile(DATA + "/reports"); rep != "" {
		t.Fatalf("지휘에 알림 없음: %q", rep)
	}
	// 두 번째 실행 — 같은 반송은 다시 보지 않는다
	if out := dupCapture(t, relandTick); strings.Contains(out, "l-rm") || len(qRows()) != 1 {
		t.Fatalf("두 번째 바퀴: %q %v", out, qRows())
	}
	// 줄이 있는 레인 · 대기열 밖 레인(줄도 rm 기록도 없음) — 지금처럼 -reland
	EV = append(EV, rlEv("2026-11-22T00:02:00Z", "l-up", "120-l-up", rmConflict, "t9"),
		rlEv("2026-11-22T00:03:00Z", "l-direct", "130-l-direct", rmConflict, "t9"))
	relandTick()
	if qrow("l-up-reland") == nil || qrow("l-direct-reland") == nil {
		t.Fatalf("줄 있는 레인·대기열 밖 레인은 지금처럼: %v", qRows())
	}
	// 순서 반대: 다시 맞춤을 넣은 뒤 원래 줄을 빼도 넣은 -reland는 그대로(넣음 알림을 받은 지휘가 뺀다), 더 넣지 않는다
	rmLane(t, "l-up")
	n := len(qRows())
	relandTick()
	if qrow("l-up-reland") == nil || len(qRows()) != n {
		t.Fatalf("순서 반대: %v", qRows())
	}
}

// rm이 바퀴의 첫 확인 뒤·넣기 전에 끼어듦 — 대기열 잠금 안에서 다시 보고 넣지 않는다
func TestRelandRemovedInsideLock(t *testing.T) {
	rmFix(t)
	rmRow(t, "l-race")
	done, inLock := false, false
	qRowsWHook = func() {
		if !done {
			done, inLock = true, qHeld // 바퀴의 첫 확인(잠금 밖)은 지난 뒤 — 넣기 직전 잠금 안의 다시 읽기에서
			rmCmd([]string{"l-race", "--force"})
		}
	}
	t.Cleanup(func() { qRowsWHook = nil })
	EV = []string{rlEv("2026-11-22T00:01:00Z", "l-race", "140-l-race", rmConflict, "t9")}
	out := dupCapture(t, relandTick)
	if !done || !inLock || qrow("l-race-reland") != nil || len(qRows()) != 0 || isFile(DATA+"/briefs/added/l-race-reland.md") {
		t.Fatalf("잠금 안에서 다시 봄: %v %q", qRows(), out)
	}
	if !strings.Contains(out, "반송 다시 띄움 안 함 l-race: 대기열에서 지워진 레인") {
		t.Fatalf("한 줄: %q", out)
	}
	rl, _ := readFile(DATA + "/reland.tsv")
	if !strings.Contains(rl, "140-l-race\tl-race\tl-race\t줄 없음\t-") {
		t.Fatalf("장부: %s", rl)
	}
}

// 일지를 못 읽음(모름) — 지웠는지 모르니 넣지도 「줄 없음」으로 적지도 않고 다음 바퀴에
func TestRelandRemovedUnknown(t *testing.T) {
	rmFix(t)
	os.MkdirAll(DATA+"/jdir", 0o755)
	t.Setenv(journal.Env, DATA+"/jdir") // 폴더 — 읽기 실패
	EV = []string{rlEv("2026-11-22T00:01:00Z", "l-late", "150-l-late", rmConflict, "t9")}
	out := dupCapture(t, relandTick)
	rl, _ := readFile(DATA + "/reland.tsv")
	if len(qRows()) != 0 || strings.Contains(rl, "150-l-late") || !strings.Contains(out, "다시 맞춤 보류 l-late") {
		t.Fatalf("모름: %v %q %s", qRows(), out, rl)
	}
}
