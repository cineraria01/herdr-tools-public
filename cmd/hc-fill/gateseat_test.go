package main

import (
	"strings"
	"testing"
	"time"
)

// ht-fill-gate-seat(2026-11-26): 병합기 검증 터미널(추측 일꾼 spec-<아이디>·착지 실행 land-<차례> — agent env, hc-lane=1 라벨 없음)은
// 손잡이 merger_gate@<기계>와 상관없이 자리·레인상한에 안 든다(병합 검증은 검증 잠금이 막는다 — slots.md). 기계 줄은 그 수를
// 「검증 n(병합 g)」로 따로 보여 「검증 1 · 빈 자리 0」이 병합기 탓으로 읽히지 않게 한다.
func TestGateSeatMergerNotCounted(t *testing.T) {
	lpFix(t)
	defer func(o bool) { dry = o }(dry)
	confLines = []string{"local a", "machine w 3 3", "repo p w /r"}
	CONN, AGENT, ALLM, AUDN, AWT, EVBAD = []string{"w"}, "claude-code", nil, nil, nil, false
	LANEADDR = []string{"w/a1", "w/a2"}
	lanes := []Term{
		{"w", "w/a1", "claude-code", "l1", "working", "/r.wt/l1"},
		{"w", "w/a2", "claude-code", "l2", "working", "/r.wt/l2"},
	}
	spec := Term{"w", "w/s1", "env", "spec-1234abcd", "-", "/r"}
	land := Term{"w", "w/s2", "env", "land-1234abcd", "-", "/r"}
	for _, c := range []struct {
		name  string
		extra []Term
	}{
		{"병합 검증 1", []Term{spec}},
		{"병합 검증 2(넘침 — 자리 아니라 기계 줄 병합 수로 보인다)", []Term{spec, land}},
		{"병합 검증 0", nil},
	} {
		TERMS = append(append([]Term(nil), lanes...), c.extra...)
		dry = true
		countSeats()
		dry = false
		ROWS = []Row{pad16("l1", "p", "/b.md", "-", "-", "up", "w", "w/a1"), pad16("l2", "p", "/b.md", "-", "-", "up", "w", "w/a2")}
		if n, why := machRoom(Machine{Name: "w", Cap: "3", LCap: "3"}, false, 0); n != 1 || why != "" {
			t.Fatalf("%s: 빈 자리 %d %q, 바람 1(레인 2/3)", c.name, n, why)
		}
		// 상태 한 장·advise는 바퀴 기록(load.tsv)의 빈 자리만 읽는다 — 띄우기와 같은 값
		LOADS = map[string]mLoad{"w": {L1: 4, Cores: 8, OK: true, Ver: len(c.extra), VerOK: true, Gates: len(c.extra), GatesOK: true}}
		loadEnd(time.Now())
		r := loadRecs()["w"]
		if !r.freeOK || r.free != 1 || r.used != 2 {
			t.Fatalf("%s: 바퀴 기록 %+v, 바람 빈 1 · 쓴 2", c.name, r)
		}
		if !r.l.GatesOK || r.l.Gates != len(c.extra) {
			t.Fatalf("%s: 바퀴 기록 병합 검증 %+v", c.name, r.l)
		}
	}
}

// --spec-idle 새 꼴(「load= cpus= verifies= gates=」, ht-merger-gate-slot)도 검증 수로 읽고 병합 검증 수를 따로 둔다.
// 고치기 전: 3칸만 받아 새 판 워커의 검증 수가 모름이 됐다
func TestLoadParseGates(t *testing.T) {
	for _, c := range []struct {
		in             string
		ver, gates     int
		verOK, gatesOK bool
	}{
		{"load 2.0 8\nload=2.00 cpus=8 verifies=2 gates=1\n", 2, 1, true, true},
		{"load 2.0 8\nload=2.00 cpus=8 verifies=1 gates=0\n", 1, 0, true, true},
		{"load 2.0 8\nload=2.00 cpus=8 verifies=1\n", 1, 0, true, false}, // 옛 판 워커 — 병합 수 모름
		{"load 2.0 8\nload=2.00 cpus=8 verifies=1 gates=x\n", 1, 0, true, false},
		{"load 2.0 8\nload=2.00 cpus=8 verifies=1 gates=2\n", 1, 0, true, false}, // 병합 > 전체는 틀린 꼴
	} {
		l := loadParse(c.in)
		if !l.OK || l.Ver != c.ver || l.VerOK != c.verOK || l.Gates != c.gates || l.GatesOK != c.gatesOK {
			t.Errorf("%q: %+v", c.in, l)
		}
	}
	l := mLoad{L1: 8, Cores: 8, OK: true, Ver: 2, VerOK: true, Gates: 1, GatesOK: true}
	if s := loadLine("a", l, 2, 3, 1, "", ""); !strings.Contains(s, " · 검증 2(병합 1)") {
		t.Errorf("기계 줄에 병합 검증 없음: %s", s)
	}
	l.Gates = 0
	if s := loadLine("a", l, 2, 3, 1, "", ""); !strings.Contains(s, " · 검증 2 ") || strings.Contains(s, "병합") {
		t.Errorf("병합 0이면 지금 글 그대로: %s", s)
	}
}
