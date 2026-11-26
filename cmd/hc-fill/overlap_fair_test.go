package main

// ht-overlap-fair 시험: 겹치는 대기 줄끼리 줄 차례를 지킨다 — 줄 차례(orderRows)가 앞이고 겹침만으로 못 뜬 대기 줄 A와 겹치는 뒤 줄 B는
// 기다린다. A가 자리·멈춤으로도 못 뜨면 B를 막지 않고, 우선순위·--overlap-ok·먼저 띄움 선행 사슬은 지금 규칙 그대로.
// 고치기 전 코드는 B를 띄웠다(떠 있는 레인과만 견줌)

import (
	"os"
	"strings"
	"testing"
	"time"
)

type ofLane struct{ name, prio, brief, after, only, ovk string }

// ofSetup: ecSetup + 대기열 파일 + 기계 here(저장소 있음, 자리 2)·w2(저장소 없음). 쓰기 소유 X가 cmd/x.go를 쥐고 있다
func ofSetup(t *testing.T) func(n, s string) string {
	t.Helper()
	bf := ecSetup(t)
	setData(t)
	or, oq, os_, on, orid, ost, oh, oe := ROWS, OVQ, SEATS, NOW, RID, stuck, OVWARN, OVEARLY
	t.Cleanup(func() { ROWS, OVQ, SEATS, NOW, RID, stuck, OVWARN, OVEARLY = or, oq, os_, on, orid, ost, oh, oe })
	confLines = append(confLines, "machine here 2 -", "machine w2 2 -")
	SEATS, NOW = nil, nil
	OREC = []Row{{"X", "app", "here", "t1", "s", "b", ",-,", "-"}}
	OWN = []Row{{"app", "X", "R", "cmd/x.go"}}
	return bf
}

// ofQueue: 대기열 파일을 쓰고 줄 차례(orderRows)로 ROWS를 세운다
func ofQueue(t *testing.T, ls ...ofLane) {
	t.Helper()
	var b strings.Builder
	for i, l := range ls {
		r := pad16(l.name, "app", l.brief, dash(l.after), dash(l.only), "wait", "-", "-", "-", "2026-11-27T0"+itoa(int64(i))+":00").
			Set(qPrio, l.prio).Set(qOverlap, dash(l.ovk))
		b.WriteString(r.Line() + "\n")
	}
	if err := os.WriteFile(Q, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	orderRows()
}

// ofRound: 한 바퀴의 겹침 판정 — 줄 차례대로 ovlCheck, 통과한 줄은 띄운 것처럼 쓰기 소유에 더한다. 「이름 up|wait」
func ofRound(early bool) string {
	OVWAIT, OVQ = nil, nil
	var o []string
	for _, r := range ROWS {
		n := r.F(qName)
		RID, stuck, OVKEYS, OVEARLY = r.F(qRID), "-", nil, early
		ok := ovlCheck(n, "app", r.F(qBrief), r.F(qAfter), "-", dash(r.F(qOverlap)), r.F(qOnly))
		OVEARLY = false
		if ok {
			OREC = append(OREC, Row{n, "app", "here", "t-" + n, "s", "b", ",-,", "-"})
			ovlPlaced(n, "app", r.F(qBrief), r.F(qAfter), "-")
			o = append(o, n+" up")
		} else {
			o = append(o, n+" wait")
		}
	}
	return strings.Join(o, ", ")
}

// ① A(앞·X와 겹쳐 대기, 예약 cmd/)·B(뒤·A와 겹침, X와는 안 겹침): B도 A 뒤에서 기다리고, X가 착지하면 A가 뜨고 B는 A를 기다린다
func TestOverlapFairQueue(t *testing.T) {
	bf := ofSetup(t)
	ofQueue(t, ofLane{name: "A", prio: "high", brief: bf("A", "파일: cmd\n")}, ofLane{name: "B", prio: "normal", brief: bf("B", "파일: cmd/y.go\n")})
	if got := ofRound(false); got != "A wait, B wait" {
		t.Fatalf("X가 떠 있는 바퀴: %s (고치기 전은 B up — 떠 있는 레인과만 견줌)", got)
	}
	if got := rowsBar(OVWAIT); got != "A\tX\tcmd/\t-\tapp|B\tA\tcmd/y.go\t-\tapp\t줄|" {
		t.Errorf("겹침 대기 기록: %q", got)
	}
	if got := ovWaitNote("B", OVWAIT); got != "겹침 대기(앞선 대기 줄 A: cmd/y.go)" {
		t.Errorf("B 비고: %q", got)
	}
	if got := ovWaitNote("A", OVWAIT); got != "겹침 대기(X: cmd/) 다음 차례(겹치는 뒤 줄 1개가 기다림)" {
		t.Errorf("A 비고: %q", got)
	}
	OREC, OWN = nil, nil // X 착지 — 쓰기 소유가 풀림
	if got := ofRound(false); got != "A up, B wait" {
		t.Fatalf("X 착지 뒤 바퀴: %s", got)
	}
	if got := rowsBar(OVWAIT); got != "B\tA\tcmd/y.go\t-\tapp|" {
		t.Errorf("착지 뒤 B는 떠 있는 A를 기다림: %q", got)
	}
}

// ② B가 A와 안 겹치면 바로 뜬다
func TestOverlapFairDisjoint(t *testing.T) {
	bf := ofSetup(t)
	ofQueue(t, ofLane{name: "A", prio: "normal", brief: bf("A", "파일: cmd\n")}, ofLane{name: "B", prio: "normal", brief: bf("B", "파일: lib/z.go\n")})
	if got := ofRound(false); got != "A wait, B up" {
		t.Errorf("안 겹치는 B: %s", got)
	}
}

// ③ A가 다른 까닭으로도 못 뜨면(묶인 기계에 저장소 없음 · 띄우기 거절로 멈춤 · 자리 참) B를 막지 않는다 — 굶음을 B로 옮기지 않게.
// 선행 미착지(afterOK)·능력 막힘(ndBlock)은 waitRow가 겹침 판정 전에 돌려보내 A가 기록조차 안 된다
func TestOverlapFairOtherReason(t *testing.T) {
	bf := ofSetup(t)
	a, b := bf("A", "파일: cmd\n"), bf("B", "파일: cmd/y.go\n")
	ofQueue(t, ofLane{name: "A", prio: "normal", brief: a, only: "w2"}, ofLane{name: "B", prio: "normal", brief: b})
	if got := ofRound(false); got != "A wait, B up" {
		t.Errorf("A 묶인 기계에 저장소 없음: %s", got)
	}
	OREC, OWN = OREC[:1], OWN[:1]
	ofQueue(t, ofLane{name: "A", prio: "normal", brief: a}, ofLane{name: "B", prio: "normal", brief: b})
	rid := ROWS[0].F(qRID)
	if err := os.WriteFile(DATA+"/reject.tsv", []byte(rid+"\tA\treject\t-\t-\t-\there\t입력 거절\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ofRound(false); got != "A wait, B up" {
		t.Errorf("A 띄우기 거절로 멈춤: %s", got)
	}
	OREC, OWN = OREC[:1], OWN[:1]
	if err := os.Remove(DATA + "/reject.tsv"); err != nil {
		t.Fatal(err)
	}
	SEATS = []string{"here", "here"}
	if got := ofRound(false); got != "A wait, B up" {
		t.Errorf("A 띄울 자리 없음: %s", got)
	}
}

// ④ 우선순위: 긴급 B가 보통 A와 겹치면 줄 차례(우선순위 먼저)대로 B가 앞 — B는 뜨고(X와 안 겹침) A는 X를 기다린다
func TestOverlapFairPriority(t *testing.T) {
	bf := ofSetup(t)
	ofQueue(t, ofLane{name: "A", prio: "normal", brief: bf("A", "파일: cmd\n")}, ofLane{name: "B", prio: "urgent", brief: bf("B", "파일: cmd/y.go\n")})
	if got := ofRound(false); got != "B up, A wait" {
		t.Errorf("긴급 B: %s", got)
	}
}

// ⑤ --overlap-ok 줄은 겹침을 안 본다(앞선 대기 줄도) · 앞 줄이 --overlap-ok면 겹침 대기가 없어 뒤 줄을 막지 않는다
func TestOverlapFairOverlapOK(t *testing.T) {
	bf := ofSetup(t)
	ofQueue(t, ofLane{name: "A", prio: "normal", brief: bf("A", "파일: cmd\n")}, ofLane{name: "B", prio: "normal", brief: bf("B", "파일: cmd/y.go\n"), ovk: "overlap-ok"})
	if got := ofRound(false); got != "A wait, B up" {
		t.Errorf("B --overlap-ok: %s", got)
	}
}

// 단독: 앞선 대기 줄이 단독이면 같은 프로젝트의 뒤 줄은 파일과 상관없이 그 뒤
func TestOverlapFairSolo(t *testing.T) {
	bf := ofSetup(t)
	ofQueue(t, ofLane{name: "A", prio: "normal", brief: bf("A", "본문\n"), ovk: "solo"}, ofLane{name: "B", prio: "normal", brief: bf("B", "파일: lib/z.go\n")})
	if got := ofRound(false); got != "A wait, B wait" {
		t.Fatalf("단독 A 뒤: %s", got)
	}
	if got := rowsBar(OVWAIT); got != "A\tX\t단독\t-\tapp|B\tA\t단독\t-\tapp\t줄|" {
		t.Errorf("단독 기록: %q", got)
	}
}

// 서로 기다리지 않게: A를 막은 앞 레인 X가 B를 기다리면(--after B — 먼저 띄운 X) B는 A 뒤에 서지 않는다
func TestOverlapFairNoCycle(t *testing.T) {
	bf := ofSetup(t)
	QEDGES = []Row{{"X", ",B,"}}
	ofQueue(t, ofLane{name: "A", prio: "normal", brief: bf("A", "파일: cmd\n")}, ofLane{name: "B", prio: "normal", brief: bf("B", "파일: cmd/y.go\n")})
	if got := ofRound(false); got != "A wait, B up" {
		t.Errorf("X가 B를 기다림: %s", got)
	}
}

// status 단계·advise: 앞선 대기 줄 뒤에 선 줄은 「대기(겹침 대기(앞선 대기 줄 …))」이고, advise는 그 줄에 --overlap-ok를 권하지 않는다
// (풀면 줄 차례가 깨진다 — 앞 줄 항목이 맡는다)
func TestOverlapFairShow(t *testing.T) {
	ws := []Row{{"lane-c", "lane-a", "docs/x.md", "-", "app", "줄"}}
	if got := stStage0(stIn{State: "wait", Ovl: ovWaitNote("lane-c", ws)}); got != "대기(겹침 대기(앞선 대기 줄 lane-a: docs/x.md))" {
		t.Errorf("status 단계: %q", got)
	}
	if got := stStage0(stIn{State: "wait", Ovl: ovWaitNote("lane-a", ws)}); got != "대기(다음 차례(겹치는 뒤 줄 1개가 기다림))" {
		t.Errorf("앞 줄 status 단계: %q", got)
	}
	advFx(t)
	if err := os.WriteFile(DATA+"/overlap.tsv", []byte("lane-c\tlane-a\tdocs/x.md\t-\tapp\t줄\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, it := range adviseNow(time.Now()).Items {
		if it.Key == "free-wait:lane-c" {
			t.Errorf("앞선 대기 줄 뒤에 선 줄에 겹침 풀기를 권함: %+v", it)
		}
	}
}

// ⑥ 먼저 띄움의 선행 사슬 겹침은 그대로: B(--after P, P가 cmd/y.go를 쥠)는 앞선 대기 줄 A와 안 겹치면 먼저 띄움으로 뜨고 사슬 겹침을 남긴다
func TestOverlapFairEarlyChain(t *testing.T) {
	bf := ofSetup(t)
	OREC = append(OREC, Row{"P", "app", "here", "t2", "s", "b", ",-,", "-"})
	OWN = append(OWN, Row{"app", "P", "R", "cmd/y.go"})
	QEDGES = []Row{{"B", ",P,"}}
	ofQueue(t, ofLane{name: "A", prio: "normal", brief: bf("A", "파일: cmd/x.go\n")}, ofLane{name: "B", prio: "normal", brief: bf("B", "파일: cmd/y.go\n"), after: "P"})
	OVCHAIN = nil
	if got := ofRound(true); got != "A wait, B up" {
		t.Errorf("먼저 띄움 B: %s", got)
	} else if got := rowsBar(OVCHAIN); got != "P\t예약\tcmd/y.go|" {
		t.Errorf("선행 사슬 겹침 기록: %q", got)
	}
}
