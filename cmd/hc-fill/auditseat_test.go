package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fill-audit-seat(2026-11-24): 완료를 적고 자동 감사를 기다리는 레인의 자리는 그 감사가 뜰 때까지 감사 몫으로 잡아 둔다 —
// 완료한 레인이 비운 자리를 같은 바퀴의 다른 대기 레인이 먼저 가져가 감사가 「자리 없음」으로 굶던 일(긴급 레인 감사 20분·17분).
// 차림: 기계 here 하나(상한 cap), 쓰기 레인 la(done, 자동 감사 차수 a1 queued, 감사 줄 audit-a1 wait)·lb(working),
// 대기 레인 lc(긴급 — 차례가 감사 줄보다 앞). 가짜 hc-lane은 report(사건)를 reports에 적는다
type asT struct {
	t *testing.T
	d string
}

func asSetup(t *testing.T, cap string, laAuto bool) *asT {
	setData(t)
	d := DATA
	HOME = d
	OWNF, OWND = d+"/own.tsv", d+"/own"
	LOCAL, dry, P, CONN, AGENT, EVBAD = "here", true, "", nil, "claude-code", false
	AGUNK, SEATS, NOW, NOWL, ROWS, OREC, ALLM, AUDN, AWT, HELD = nil, nil, nil, nil, nil, nil, nil, nil, nil, nil
	AK, AUD, NEED, RESUME, EARLYF, HOLD = "-", d+"/audit.md", "", false, "", 60
	RID, stuck, SB = "r1", "-", d+"/brief.md"
	confLines = []string{"machine here " + cap + " -", "repo p here " + d + "/r"}
	for _, w := range []string{"/r.wt/la", "/r.wt/lb", "/r.wt/lc"} {
		if err := os.MkdirAll(d+w, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	rejW(t, d+"/brief.md", "# 일\n본문\n")
	rejW(t, d+"/hc-lane", "#!/bin/sh\ncase \"$1\" in report) printf '%s\\n' \"$*\" >>\"$HC_T/reports\"; exit 0 ;; esac\nexit 1\n")
	if err := os.Chmod(d+"/hc-lane", 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HC_T", d)
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	ak := "auto"
	if !laAuto {
		ak = "-"
	}
	row := func(f ...string) string { return strings.Join(f, "\t") }
	q := []string{
		row("la", "p", d+"/brief.md", "-", "-", "up", "here", "h/a", "-", "t", "-", "t9", "normal", "-", ak, "-", "q1"),
		row("lb", "p", d+"/brief.md", "-", "-", "up", "here", "h/b", "-", "t", "-", "t9", "normal", "-", "auto", "-", "q2"),
		row("lc", "p", d+"/brief.md", "-", "-", "wait", "-", "-", "-", "t", "-", "t9", "urgent", "-", "auto", "-", "q3"),
	}
	if laAuto {
		q = append(q, row("audit-a1", "p", d+"/brief.md", "-", "here", "wait", "-", "-", "-", nowMin(), "-", "t9", "urgent", "no-early", "audit=la", "-", "q4"))
		rejW(t, AT, row("a1", "la", "1", "run1", strings.Repeat("a", 40), "here", "h/a", "queued", "0", "-", "0", "0", "-", "-", "0", "-", "-")+"\n")
	}
	rejW(t, Q, strings.Join(q, "\n")+"\n")
	TERMS = []Term{
		{"here", "h/a", "claude-code", "la", "done", d + "/r.wt/la"},
		{"here", "h/b", "claude-code", "lb", "working", d + "/r.wt/lb"},
	}
	LANEADDR = []string{"h/a", "h/b"}
	return &asT{t, d}
}

// round: 한 바퀴의 자리 셈과, 차례대로 lc(긴급 대기 레인) → audit-a1 놓기. 돌려줌: lc 놓임, 감사 놓임
func (x *asT) round() (lc, au bool) {
	auditSeats()
	countSeats()
	NOW, NOWL, OFF = nil, nil, ""
	AK, RID = "-", "q3"
	place("lc", "p", x.d+"/brief.md", "-", "-", "t9", "")
	lc = placed
	if qrow("audit-a1") != nil {
		AK, RID = "audit=la", "q4"
		place("audit-a1", "p", x.d+"/brief.md", "here", "-", "t9", "")
		au = placed
	}
	AK = "-"
	return
}

func (x *asT) reports() string {
	b, _ := os.ReadFile(x.d + "/reports")
	return string(b)
}

// ① 자리 2·작업 중 2, 하나가 완료하고 감사를 기다림 → 같은 바퀴에 대기 레인이 그 자리를 못 가져가고 감사가 뜬다
// ② 감사가 뜬 뒤 자리 셈 = 작업 중 1 + 감사 1(겹쳐 세지 않음) — 같은 바퀴에도, 감사 터미널이 도는 다음 바퀴에도
func TestAuditSeatHandover(t *testing.T) {
	x := asSetup(t, "2", true)
	lc, au := x.round()
	if lc || !au {
		t.Fatalf("lc 놓임 %v·감사 놓임 %v, 바람 lc 안 놓임·감사 놓임(완료한 레인 자리는 감사 몫)", lc, au)
	}
	if n := agentsOn("here"); n != 2 {
		t.Fatalf("감사를 놓은 뒤 agentsOn %d, 바람 2(lb + 감사 — la 자리를 겹쳐 세지 않음)", n)
	}
	// 다음 바퀴: 감사 줄이 up·working
	rows := qRows()
	for i, r := range rows {
		if r.F(1) == "audit-a1" {
			rows[i] = r.Set(6, "up").Set(7, "here").Set(8, "h/au")
		}
	}
	rejW(t, Q, rowsText(rows))
	TERMS = append(TERMS, Term{"here", "h/au", "claude-code", "audit-a1", "working", x.d + "/r.wt/la"})
	LANEADDR = append(LANEADDR, "h/au")
	lc, _ = x.round()
	if lc {
		t.Fatal("감사가 도는 동안 lc가 놓였다(자리 2 = lb + 감사)")
	}
	if got := strings.Join(SEATS, ","); got != "here,here" {
		t.Fatalf("SEATS %q, 바람 here,here(lb·감사, la는 안 셈)", got)
	}
}

// ③ 자리 상한이 줄어 이미 꽉 참(상한 1, lb 작업 중 + la 감사 몫) → 감사 대기, N바퀴(audit_seat_rounds) 넘으면 지휘에 사건 한 번, 되풀이 없음.
// 도구는 상한을 넘겨 띄우지 않는다
func TestAuditSeatFullAlertsOnce(t *testing.T) {
	x := asSetup(t, "1", true)
	dry = false
	confLines = append(confLines, "audit_seat_rounds 2")
	for i := 1; i <= 6; i++ {
		lc, au := x.round()
		if lc || au {
			t.Fatalf("%d바퀴: lc %v·감사 %v — 꽉 찬 기계에 띄웠다", i, lc, au)
		}
		n := strings.Count(x.reports(), "자리 없어")
		switch {
		case i <= 2 && n != 0:
			t.Fatalf("%d바퀴: 사건 %d, 바람 0(2바퀴까지는 기다림)\n%s", i, n, x.reports())
		case i > 2 && n != 1:
			t.Fatalf("%d바퀴: 사건 %d, 바람 1(한 번만)\n%s", i, n, x.reports())
		}
	}
	r := x.reports()
	if !strings.Contains(r, "감사 a1(la)가 here 자리 없어 ") || !strings.Contains(r, "분 대기") || !strings.Contains(r, "--name la") {
		t.Fatalf("사건 글 %q", r)
	}
}

// ④ 자동 감사가 꺼진 레인(--no-audit) 완료 → 지금 그대로: 감사 몫으로 잡지 않는다(쉰 지 hold 넘으면 자리 빔 → lc가 뜬다)
// ⑥ 감사 포기(지휘로 넘김) 뒤 감사 몫이 풀려 지금 규칙으로 — 같은 조건에서 lc가 뜬다
func TestAuditSeatReleases(t *testing.T) {
	old := strconv.FormatInt(time.Now().Add(-2*time.Hour).Unix(), 10)
	for _, c := range []struct {
		name string
		auto bool
		st   string
	}{{"no-audit", false, ""}, {"포기", true, "escalate:noverdict"}, {"넘김 끝", true, "coord"}} {
		t.Run(c.name, func(t *testing.T) {
			x := asSetup(t, "2", c.auto)
			if c.auto {
				rows := readRows(AT)
				rows[0] = rows[0].Set(8, c.st)
				rejW(t, AT, rowsText(rows))
			}
			rejW(t, DATA+"/held.tsv", "h/a\t"+old+"\n")
			if lc, _ := x.round(); !lc {
				t.Fatalf("lc 안 놓임 — %s 뒤에도 la 자리를 잡아 둠", c.name)
			}
		})
	}
	// 대조: 같은 조건(쉰 지 hold 넘음)이어도 감사를 기다리는 동안은 잡아 둔다
	x := asSetup(t, "2", true)
	rejW(t, DATA+"/held.tsv", "h/a\t"+old+"\n")
	if lc, au := x.round(); lc || !au {
		t.Fatalf("감사 기다림: lc %v·감사 %v, 바람 lc 안 놓임·감사 놓임", lc, au)
	}
}

// ⑤ ls·status 비고: 뜬 감사는 「감사 중(n번째)」, 자리 기다리는 감사는 「감사 대기(자리 없음 <분>분)」
func TestAuditSeatNote(t *testing.T) {
	x := asSetup(t, "1", true)
	dry = false
	x.round()
	if got := anote("la"); !strings.HasPrefix(got, "감사 대기(자리 없음 ") || !strings.HasSuffix(got, "분)") {
		t.Fatalf("자리 기다리는 감사 비고 %q", got)
	}
	rows := qRows()
	for i, r := range rows {
		if r.F(1) == "audit-a1" {
			rows[i] = r.Set(6, "up").Set(7, "here").Set(8, "h/au")
		}
	}
	rejW(t, Q, rowsText(rows))
	if got := anote("la"); got != "감사 중(1번째)" {
		t.Fatalf("뜬 감사 비고 %q, 바람 감사 중(1번째)", got)
	}
}
