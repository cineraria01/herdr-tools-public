package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// ht-seat-working-only(2026-11-27, 사용자 결정 「일하는 것만 센다」 — docs/hc-fill/slots.md): 자리는 대기열이 띄운 것 가운데 지금 일하는
// 에이전트만 센다. 감사·착지·다시 넣기를 기다리며 쉬는 레인 터미널, 닫힌 레인의 done 터미널, 사람이 연 세션·지휘 세션은 자리를 안 쓴다.
// 고치기 전: 자리 3인 기계에서 그것들이 「에이전트 7/3」을 채워 자동 감사가 30분 못 떴다.
// 차림: 기계 here 하나(자리 cap·레인상한 lcap), 대기열 줄은 swRow로, 터미널은 TERMS로 직접. 가짜 hc-lane은 report만 받는다
type swT struct {
	t *testing.T
	d string
	q []string
}

func swSetup(t *testing.T, cap, lcap string) *swT {
	setData(t)
	d := DATA
	HOME = d
	OWNF, OWND = d+"/own.tsv", d+"/own"
	LOCAL, dry, P, CONN, AGENT, EVBAD = "here", true, "", nil, "claude-code", false
	AGUNK, SEATS, NOW, NOWL, ROWS, OREC, LANEADDR, TERMS, EV = nil, nil, nil, nil, nil, nil, nil, nil, nil
	AK, AUD, NEED, RSRC, EARLYF = "-", d+"/audit.md", "", srcNew, ""
	RID, stuck, SB = "r1", "-", d+"/brief.md"
	confLines = []string{"machine here " + cap + " " + lcap, "repo p here " + d + "/r"}
	if err := os.MkdirAll(d+"/r.wt", 0o755); err != nil {
		t.Fatal(err)
	}
	rejW(t, d+"/brief.md", "# 일\n본문\n")
	rejW(t, d+"/hc-lane", "#!/bin/sh\ncase \"$1\" in report) exit 0 ;; esac\nexit 1\n")
	if err := os.Chmod(d+"/hc-lane", 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	t.Cleanup(func() { dry = false })
	return &swT{t: t, d: d}
}

// lane: 대기열에 떠 있는(up) 쓰기 레인과 그 터미널(상태 st). 작업 트리 폴더도 만든다
func (x *swT) lane(name, st string) {
	x.t.Helper()
	if err := os.MkdirAll(x.d+"/r.wt/"+name, 0o755); err != nil {
		x.t.Fatal(err)
	}
	x.q = append(x.q, swRow(name, x.d+"/brief.md", "-", "up", "here", "here/"+name, "normal", "auto", "q-"+name))
	x.term(name, "here/"+name, "claude-code", st, x.d+"/r.wt/"+name)
	LANEADDR = append(LANEADDR, "here/"+name)
}

// auditWait: 쓰기 레인 lane이 완료를 적고(차수 queued) 감사 줄이 아직 안 떴다(wait, 긴급)
func (x *swT) auditWait(id, lane string) {
	x.q = append(x.q, swRow("audit-"+id, x.d+"/brief.md", "here", "wait", "-", "-", "urgent", "audit="+lane, "q-"+id))
	f, _ := os.OpenFile(AT, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	fmt.Fprintln(f, strings.Join([]string{id, lane, "1", "run-" + lane, strings.Repeat("a", 40), "here", "here/" + lane, "queued", "0", "-", "0", "0", "-", "-", "0", "-", "-"}, "\t"))
	f.Close()
}

func (x *swT) waitLane(name, prio string) {
	x.q = append(x.q, swRow(name, x.d+"/brief.md", "-", "wait", "-", "-", prio, "auto", "q-"+name))
}

func (x *swT) term(name, addr, agent, st, cwd string) {
	TERMS = append(TERMS, Term{"here", addr, agent, name, st, cwd})
}

func swRow(name, brief, only, state, mach, addr, prio, audit, rid string) string {
	return strings.Join([]string{name, "p", brief, "-", only, state, mach, addr, "-", "t", "-", "t9", prio, "-", audit, "-", rid}, "\t")
}

// round: 한 바퀴의 자리 셈과, 바퀴 차례(orderRows)대로 기다리는 줄 놓기(dry-run). 돌려줌: 놓인 줄 이름
func (x *swT) round() []string {
	x.t.Helper()
	rejW(x.t, Q, strings.Join(x.q, "\n")+"\n")
	aseatSweep()
	orderRows()
	countSeats()
	NOW, NOWL, OFF = nil, nil, ""
	var got []string
	for _, r := range ROWS {
		if r.F(qState) != "wait" {
			continue
		}
		AK, RID, stuck = dash(r.F(qAudit)), r.F(qRID), r.F(qStuck)
		if AK == "auto" {
			AK = "-"
		}
		place(r.F(qName), "p", x.d+"/brief.md", dash(r.F(qOnly)), "-", "t9", "")
		if placed {
			got = append(got, r.F(qName))
		}
	}
	AK = "-"
	return got
}

func room() (int, string) { return machRoom(machines()[0], false) }

// ① 자리 3 · 감사를 기다리는 done 레인 터미널 3 → 감사 셋이 뜬다
func TestSeatWorkAuditsLaunch(t *testing.T) {
	x := swSetup(t, "3", "-")
	for _, l := range []string{"la", "lb", "lc"} {
		x.lane(l, "done")
		x.auditWait("a-"+l, l)
	}
	if got := strings.Join(x.round(), ","); got != "audit-a-la,audit-a-lb,audit-a-lc" {
		t.Fatalf("놓인 줄 %q, 바람 감사 셋", got)
	}
}

// ② 자리 3 · done 레인 3(감사 기다림) + 사람이 연 세션 여럿(일하는 중) + 닫힌 레인의 done 터미널 + 손으로 띄운 hc-lane 레인 → 감사가 뜬다.
// 사람 세션·지휘 세션·대기열 밖 레인은 자리를 안 쓰고, 쉬는 대기열 터미널도 안 쓴다
func TestSeatWorkHumansAndClosedNotCounted(t *testing.T) {
	x := swSetup(t, "3", "-")
	for _, l := range []string{"la", "lb", "lc"} {
		x.lane(l, "done")
		x.auditWait("a-"+l, l)
	}
	x.term("지휘", "here/c1", "claude-code", "working", x.d)
	x.term("-", "here/h1", "claude-code", "working", x.d+"/r")
	x.term("사람2", "here/h2", "codex", "working", x.d)
	x.term("닫힌", "here/old", "claude-code", "done", x.d+"/r.wt/old")   // 대기열 줄이 없는(닫힌) 레인의 done 터미널
	x.term("손레인", "here/m1", "claude-code", "working", x.d+"/r.wt/m1") // 지휘가 hc-lane으로 손으로 띄운 레인
	LANEADDR = append(LANEADDR, "here/m1")
	if got := strings.Join(x.round(), ","); got != "audit-a-la,audit-a-lb,audit-a-lc" {
		t.Fatalf("놓인 줄 %q, 바람 감사 셋(쉬는 레인·사람 세션·대기열 밖은 자리를 안 씀)", got)
	}
}

// ③ 일하는 레인 3 · 자리 3 → 새 것 안 뜸(자리 참)
func TestSeatWorkFullWhenWorking(t *testing.T) {
	x := swSetup(t, "3", "-")
	for _, l := range []string{"la", "lb", "lc"} {
		x.lane(l, "working")
	}
	x.waitLane("ld", "urgent")
	if got := x.round(); len(got) != 0 {
		t.Fatalf("자리 3/3인데 %q 놓임", got)
	}
	if n, why := room(); n != 0 || why != "자리 참" || agentsOn("here") != 3 {
		t.Fatalf("빈 자리 %d %q · 쓴 %d, 바람 0 자리 참 · 3", n, why, agentsOn("here"))
	}
}

// ④ 쉬던 레인이 감사 지적으로 다시 일함 → 그때부터 셈. 넘쳐도 그 레인은 그대로(도구는 일하는 레인을 건드리지 않음), 새 띄우기만 멈춤
func TestSeatWorkWokenLaneCounts(t *testing.T) {
	x := swSetup(t, "2", "-")
	x.lane("la", "working")
	x.lane("lb", "working")
	x.lane("lc", "done") // 감사 판정을 기다리며 쉼 — 자리 안 씀
	x.q = append(x.q, swRow("audit-a1", x.d+"/brief.md", "here", "up", "here", "here/au", "urgent", "audit=lc", "q-a1"))
	x.term("audit-a1", "here/au", "claude-code", "done", x.d+"/r.wt/lc") // 판정을 내고 쉼
	x.waitLane("ld", "normal")
	x.round()
	if n := agentsOn("here"); n != 2 {
		t.Fatalf("쉬는 lc·판정 낸 감사를 셌다: 쓴 %d, 바람 2", n)
	}
	TERMS[2].Status = "working" // 감사 지적을 받아 lc가 다시 일함
	if got := x.round(); len(got) != 0 {
		t.Fatalf("넘친 기계에 %q 놓임", got)
	}
	if n, why := room(); agentsOn("here") != 3 || n != 0 || why != "자리 참" {
		t.Fatalf("깨어난 뒤 쓴 %d · 빈 %d %q, 바람 3 · 0 자리 참", agentsOn("here"), n, why)
	}
	if !strings.Contains(strings.Join(SEATS, ","), "here,here,here") {
		t.Fatalf("SEATS %q", SEATS)
	}
}

// ⑤ 레인상한·새 쓰기 레인 띄우기는 같은 입력에서 고치기 전과 같은 판정(레인 수는 쉬는 쓰기 레인도 센다)
func TestSeatWorkLaneCapSame(t *testing.T) {
	for _, c := range []struct {
		name       string
		cap, lcap  string
		lanes      []string // 상태
		lanesOn    int
		wantPlaced bool
		wantWhy    string
	}{
		{"레인 2/2 둘 다 일함", "5", "2", []string{"working", "working"}, 2, false, "레인상한 참"},
		{"레인 2/2 둘 다 쉼(감사·착지 기다림)", "5", "2", []string{"done", "idle"}, 2, false, "레인상한 참"},
		{"레인 1/2", "5", "2", []string{"working"}, 1, true, ""},
		{"레인상한 없음·자리 남음", "3", "-", []string{"working"}, 1, true, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			x := swSetup(t, c.cap, c.lcap)
			for i, st := range c.lanes {
				x.lane(fmt.Sprintf("l%d", i), st)
			}
			x.waitLane("lw", "normal")
			got := x.round()
			if (len(got) == 1) != c.wantPlaced {
				t.Fatalf("놓임 %q, 바람 놓임=%v", got, c.wantPlaced)
			}
			NOW, NOWL = nil, nil
			if n := lanesOn("here"); n != c.lanesOn {
				t.Fatalf("lanesOn %d, 바람 %d", n, c.lanesOn)
			}
			if _, why := room(); why != c.wantWhy {
				t.Fatalf("까닭 %q, 바람 %q", why, c.wantWhy)
			}
		})
	}
}

// ⑥ 유형 모를 터미널(agent 칸 없음) → 지금처럼 그 기계 0
func TestSeatWorkUnknownTypeZero(t *testing.T) {
	x := swSetup(t, "3", "-")
	x.term("?", "here/u", "-", "-", x.d)
	x.waitLane("lw", "normal")
	if got := x.round(); len(got) != 0 {
		t.Fatalf("유형 모를 터미널이 있는데 %q 놓임", got)
	}
	if n, why := room(); n != 0 || why != "유형 모를 터미널" {
		t.Fatalf("빈 자리 %d %q", n, why)
	}
}

// ⑦ 운영 에이전트는 도는 동안(상태 파일의 실행 주소·이름 hc-fill-agent) 일하면 셈, 끝나 쉬면 안 셈.
// 띄웠지만 시작 확인이 안 된(남긴것) 터미널·미확정 시도가 받은 주소도 대기열이 띄운 것으로 안다
func TestSeatWorkOpsAgentAndStuck(t *testing.T) {
	x := swSetup(t, "3", "-")
	if err := os.MkdirAll(oaDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	rejW(t, oaStatePath(), `{"run":{"id":"x1","addr":"here/oa","mode":"act","started":1}}`)
	x.term(oaName, "here/oa", "claude-code", "working", x.d)
	x.q = append(x.q, strings.Join([]string{"ls", "p", x.d + "/brief.md", "-", "-", "wait", "-", "-", "here=here/st", "t", "-", "t9", "normal", "-", "auto", "-", "q-ls"}, "\t"))
	x.term("ls", "here/st", "claude-code", "working", x.d+"/r.wt/ls")
	rejW(t, DATA+"/try.tsv", strings.Join([]string{"lu", "q-lu", "here", "-", "-", "?", "-", "here/un", "1"}, "\t")+"\n")
	x.term("lu", "here/un", "claude-code", "blocked", x.d+"/r.wt/lu")
	x.round()
	if n := agentsOn("here"); n != 3 {
		t.Fatalf("운영 에이전트·남긴것·미확정 셋이 일하는데 쓴 %d, 바람 3", n)
	}
	TERMS[0].Status = "done"
	x.round()
	if n := agentsOn("here"); n != 2 {
		t.Fatalf("운영 에이전트가 끝나 쉬는데 쓴 %d, 바람 2", n)
	}
}

// ⑨ 병합기가 띄운 에이전트(충돌 풀이 merge-<레인>·풀이 감사 audit-merge-<레인>)도 대기열 쪽 도구가 띄운 것 — 병합기 기록 한 장(agents)의
// 주소+이름으로 알고, 일하면 셈·쉬면 안 셈(감사 atp4gp8-irb1dx 중간 — 고치기 전: 「대기열 밖 → 안 셈」이라 자리 2인 기계에 넷이 일함)
func TestSeatWorkMergerAgents(t *testing.T) {
	x := swSetup(t, "2", "-")
	x.lane("la", "working")
	rec := `{"v":1,"machine":"here","phase":"round","agents":[{"machine":"here","addr":"mg","name":"merge-lb","role":"풀이 resolve","at":1},` +
		`{"machine":"here","addr":"here/am","name":"audit-merge-lc","role":"풀이 audit","at":1}]}`
	ma := maRow{P: "p", M: "here", St: "alive", K: "0", Rec: rec}
	rejW(t, maFile(), ma.line()+"\n")
	x.term("merge-lb", "mg", "claude-code", "working", x.d)
	x.term("audit-merge-lc", "am", "claude-code", "working", x.d) // 이 기계 터미널 주소는 기계 없이(기록의 here/am → am, recAddr)
	x.term("merge-lb", "here/zz", "claude-code", "working", x.d)  // 같은 이름 다른 주소 — 기록에 없는 터미널은 대기열 밖
	x.waitLane("ld", "normal")
	if got := x.round(); len(got) != 0 {
		t.Fatalf("병합기 풀이 에이전트 둘이 일하는데 %q 놓임", got)
	}
	if n, out := agentsOn("here"), cnt(SOUT, "here"); n != 3 || out != 1 {
		t.Fatalf("쓴 %d · 대기열 밖 %d, 바람 3(la·풀이·풀이 감사) · 1(같은 이름 다른 주소)", n, out)
	}
	TERMS[1].Status, TERMS[2].Status = "done", "idle"
	x.round()
	if n := agentsOn("here"); n != 2 { // la + 이번 바퀴에 띄운 ld
		t.Fatalf("풀이 에이전트가 쉬는데 쓴 %d, 바람 2(la·ld)", n)
	}
}

// ⑧ 상태 한 장(바퀴 기록 load.tsv free·used)·advise가 읽는 빈 자리가 띄우기 판정과 같은 함수 — ② 차림에서
func TestSeatWorkStateSameAsPlace(t *testing.T) {
	x := swSetup(t, "3", "-")
	x.lane("la", "working")
	x.lane("lb", "done")
	x.term("지휘", "here/c1", "claude-code", "working", x.d)
	x.term("사람", "here/h1", "claude-code", "working", x.d)
	x.round()
	NOW, NOWL = nil, nil
	n, _ := room()
	LOADS = map[string]mLoad{"here": {L1: 1, Cores: 8, OK: true}}
	MWHY = map[string]string{}
	t.Cleanup(func() { LOADS, MWHY = nil, nil })
	dry = false
	loadEnd(time.Now())
	r := loadRecs()["here"]
	if n != 2 || !r.freeOK || r.free != n || r.used != 1 {
		t.Fatalf("띄우기 빈 자리 %d · 바퀴 기록 %+v, 바람 빈 2 · 쓴 1(일하는 대기열 레인 la만)", n, r)
	}
}
