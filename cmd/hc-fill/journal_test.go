package main

// 운영 조치 일지(journal.go): 조치 명령마다 한 줄(칸이 다 참) · journal --since·--by · 되풀이 알림 · 일지 쓰기 실패에도 조치는 됨

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"herdr-tools/internal/journal"
)

func jfix(t *testing.T) (*rejT, string) {
	t.Helper()
	x := rejSetup(t)
	p := filepath.Join(t.TempDir(), "journal.jsonl")
	t.Setenv(journal.Env, p)
	t.Setenv("HC_TERMINAL_ID", "w-j")
	knobOnce, knobES = sync.Once{}, nil
	t.Cleanup(func() { knobOnce, knobES = sync.Once{}, nil })
	return x, p
}

func errCapture(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	fn()
	os.Stderr = old
	w.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func jread(t *testing.T) []journal.Entry {
	t.Helper()
	es, bad, err := journal.Read(journal.Filter{})
	if err != nil || bad != 0 {
		t.Fatalf("일지 읽기 %v (깨진 줄 %d)", err, bad)
	}
	return es
}

// ① 조치마다 일지 한 줄 — 누가·무엇·대상·결과·되돌리기·상태가 다 차고, --why는 왜 칸으로
func TestJournalEachAction(t *testing.T) {
	x, _ := jfix(t)
	now := time.Now()
	if err := knobSet([]string{"idle", "40", "--why", "밤사이 조용히"}, now); err != nil {
		t.Fatal(err)
	}
	knobOnce, knobES = sync.Once{}, nil
	if err := knobUnset([]string{"idle", "--why", "아침"}, now); err != nil {
		t.Fatal(err)
	}
	if rc := withQ(func() { prioCmd([]string{"l1", "high", "--why", "급함"}) }); rc != 0 {
		t.Fatalf("prio 종료 %d", rc)
	}
	if rc := withQ(func() { editCmd([]string{"l1", "--only", "far", "--why", "here 바쁨"}) }); rc != 0 {
		t.Fatalf("edit 종료 %d", rc)
	}
	t.Setenv("HC_T_RC_far", "11")
	x.place()
	if rc := withQ(func() { retryCmd([]string{"l1", "--why", "가지 지움"}) }); rc != 0 {
		t.Fatalf("retry 종료 %d", rc)
	}
	dupCapture(t, func() { rmCmd([]string{"l1", "--why", "필요 없어짐"}) })

	es := jread(t)
	want := []struct{ act, why, undo string }{
		{"knob set", "밤사이 조용히", "hc-fill knob unset idle"},
		{"knob unset", "아침", "hc-fill knob set idle 40 --why 되돌림"},
		{"prio", "급함", "hc-fill prio l1 보통"},
		{"edit", "here 바쁨", "hc-fill edit l1 --only '-'"},
		{"retry", "가지 지움", "-"},
		{"rm", "필요 없어짐", "hc-fill add --name l1 --project 'p' --brief '" + x.d + "/brief.md' --priority 높음 --only 'far' --coord 't9'"},
	}
	if len(es) != len(want) {
		t.Fatalf("일지 %d줄, 바람 %d: %+v", len(es), len(want), es)
	}
	for i, w := range want {
		e := es[i]
		if e.Time.IsZero() || e.By != "w-j" || e.Tool != "hc-fill" || e.Action != w.act || e.Target == "" || e.Why != w.why ||
			e.Result != "됨" || e.Undo != w.undo || e.State == "" {
			t.Errorf("%d번째 줄 %+v, 바람 %s · 왜 %q · 되돌리기 %q", i, e, w.act, w.why, w.undo)
		}
	}
	// 아무것도 안 바뀐 부름(둔 값 없는 unset·멈추지 않은 줄 retry)은 조치가 아니다 — 줄을 더하지 않는다
	if err := knobUnset([]string{"hold"}, now); err != nil {
		t.Fatal(err)
	}
	if n := len(jread(t)); n != len(want) {
		t.Fatalf("할 것 없는 unset이 일지에: %d줄", n)
	}
}

// ② journal --since·--by(사람 출력과 --json)
func TestJournalCmdFilter(t *testing.T) {
	jfix(t)
	now := time.Now()
	for i, by := range []string{"a", "b", "a"} {
		if err := journal.Append(journal.Entry{Time: now.Add(time.Duration(i-3) * time.Hour), By: by, Tool: "hc-fill", Action: "prio", Target: "l" + by}); err != nil {
			t.Fatal(err)
		}
	}
	var got []journal.Entry
	out := dupCapture(t, func() { journalCmd([]string{"--json", "--since", "150m", "--by", "a"}) })
	if err := json.Unmarshal([]byte(out), &got); err != nil || len(got) != 1 || got[0].By != "a" || got[0].Time.Before(now.Add(-150*time.Minute)) {
		t.Fatalf("--since 150m --by a: %v %q", err, out)
	}
	out = dupCapture(t, func() { journalCmd([]string{"--by", "b"}) })
	if strings.Count(out, "\n") != 2 || !strings.Contains(out, "  b  hc-fill prio lb — ") || !strings.Contains(out, "되돌리기 -") {
		t.Fatalf("사람 출력 %q", out)
	}
	if out = dupCapture(t, func() { journalCmd([]string{"--json", "--by", "없음"}) }); strings.TrimSpace(out) != "[]" {
		t.Fatalf("빈 --json %q", out)
	}
	if _, err := parseSince("어제", now); err == nil {
		t.Fatal("틀린 --since를 받음")
	}
	if ts, err := parseSince("2026-11-24 09:00", now); err != nil || ts.Hour() != 9 {
		t.Fatalf("시각 --since %v %v", ts, err)
	}
}

// ③ 같은 대상에 같은 조치가 되풀이되면 조치 명령이 한 줄 알린다 — 막지 않는다
func TestJournalRepeatNotice(t *testing.T) {
	jfix(t)
	var errs []string
	for _, p := range []string{"high", "low", "urgent"} {
		errs = append(errs, errCapture(t, func() {
			if rc := withQ(func() { dupCapture(t, func() { prioCmd([]string{"l1", p}) }) }); rc != 0 {
				t.Fatalf("prio 종료 %d", rc)
			}
		}))
	}
	if errs[0] != "" || !strings.Contains(errs[2], "hc-fill: 최근 2시간에 같은 조치(prio l1) 3번째") {
		t.Fatalf("알림 %q", errs)
	}
	if r := qrow("l1"); r.F(13) != "urgent" {
		t.Fatalf("알림이 조치를 막음: 우선 %q", r.F(13))
	}
}

// ④ 일지를 못 써도 조치는 된다(한 줄 알림)
func TestJournalWriteFailKeepsAction(t *testing.T) {
	jfix(t)
	block := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(block, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(journal.Env, filepath.Join(block, "journal.jsonl"))
	msg := errCapture(t, func() {
		if rc := withQ(func() { dupCapture(t, func() { prioCmd([]string{"l1", "high"}) }) }); rc != 0 {
			t.Fatalf("prio 종료 %d", rc)
		}
	})
	if !strings.Contains(msg, "hc-fill: 일지를 남기지 못했다(조치는 그대로)") || qrow("l1").F(13) != "high" {
		t.Fatalf("알림 %q, 우선 %q", msg, qrow("l1").F(13))
	}
}
