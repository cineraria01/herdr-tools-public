//go:build !windows

package main

// 병합 검증 자리(ht-merger-gate-slot, 2026-11-26 — 업계의 「게이트 우선」): 손잡이 merger_gate@<기계> 1인 검증 기계는 레인 검증이 돌거나
// 부하가 높아도 그 기계에 병합 검증이 안 돌면 맡길 수 있다. 고르는 차례: 한가한 기계 먼저 → 없으면 병합 검증 자리가 빈 기계 가운데 코어당
// 부하 낮은 차례. 그 기계에 맡긴 검증에는 HERDR_VERIFY_GATE=1(검증 스크립트가 레인 검증과 다른 잠금을 쥔다), 손잡이 0인 기계에는 안 준다.
// --spec-idle 출력은 「… gates=<도는 병합 검증 수>」를 더 싣는다 — 그 칸이 없는 옛 꼴은 자리를 모름(자리로 안 고름, 한가함 판정은 그대로).
// 틀은 specFx·fakeHcIdle(verifierpick_test.go).

import (
	"os"
	"strings"
	"testing"

	"herdr-tools/internal/knob"
)

const (
	laneBusy = "load=7.00 cpus=8 verifies=1 gates=0" // 레인 검증만 돎·부하 높음 — 한가하지 않지만 병합 검증 자리는 빔
	laneLow  = "load=2.00 cpus=8 verifies=1 gates=0" // 레인 검증만 돎·부하 낮음
	gateBusy = "load=0.50 cpus=8 verifies=1 gates=1" // 병합 검증이 돎 — 자리 없음
)

// gateKnob: 이 시험의 손잡이 설정 파일에 merger_gate@<기계> 1(시험 폴더 — 운영 손잡이를 읽지 않는다)
func gateKnob(t *testing.T, machs ...string) {
	t.Helper()
	d := t.TempDir()
	t.Setenv("HERDR_TOOLS_CONFIG", d)
	var b strings.Builder
	for _, m := range machs {
		b.WriteString("merger_gate@" + m + "\t1\t# 시험\n")
	}
	if err := os.WriteFile(d+"/knobs.tsv", []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (s *sfx) gateEnvSent() bool {
	return strings.Contains(strings.Join(s.lines(s.hclog), "\n"), "HERDR_VERIFY_GATE=1")
}

// ① 손잡이 0(없음) → 지금과 같은 선택·값 없음: 레인 검증이 도는 기계뿐이면 직접 검증, 한가한 기계에 맡겨도 HERDR_VERIFY_GATE 없음
func TestGateKnobOffAsBefore(t *testing.T) {
	gateKnob(t)
	s := specFx(t)
	s.idle("v1", laneBusy)
	s.idle("v2", laneLow)
	s.sub(1000000001, "a", false)
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a" || len(s.dispatched()) != 0 {
		t.Fatalf("① 손잡이 없으면 지금처럼 직접 검증: 착지 %q 띄움 %v", got, s.dispatched())
	}
	s2 := specFx(t)
	s2.sub(1000000001, "a", false)
	s2.round(1)
	if d := s2.dispatched(); len(d) != 1 || d[0][0] != "v1" || s2.gateEnvSent() {
		t.Fatalf("① 한가한 v1에 값 없이: %v %v", d, s2.lines(s2.hclog))
	}
}

// ② 손잡이 1 · 레인 검증이 도는 기계만 있음 → 그 기계(병합 검증 자리)에 맡김 · HERDR_VERIFY_GATE=1 · 일지·--why에 「병합 검증 자리(<기계>)」
func TestGateSlotTakesLaneBusyMachine(t *testing.T) {
	gateKnob(t, "v2")
	s := specFx(t)
	s.idle("v1", laneBusy)
	s.idle("v2", laneBusy)
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || d[0][0] != "v2" || !s.gateEnvSent() || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("② 자리 기계 v2에 HERDR_VERIFY_GATE=1로: 띄움 %v 착지 %v\n%v", d, s.lines(s.landlog), s.lines(s.hclog))
	}
	t.Setenv("HERDR_VERIFY_GATE", "1")
	t.Setenv("HC_LANE_SPEC_BUSY_WAIT", "0")
	idleProbe = func() (idleInfo, bool) { return parseIdle(laneBusy) } // 일꾼: 레인 검증·부하로 「바쁨」 물러나지 않는다
	if rc := s.verifyOn(d[0]); rc != 0 {
		t.Fatalf("일꾼 종료 %d", rc)
	}
	if len(s.refs(specFails)) != 0 {
		t.Fatalf("② 자리 일꾼은 바쁨·실패 없이 검증: %v", s.refs(specFails))
	}
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|a" {
		t.Fatalf("② v2 기록으로 착지: %q\n%s", got, jdump())
	}
	if w := jrows(t, "추측 기다림"); len(w) != 1 || !strings.Contains(w[0][11], "병합 검증 자리(v2)") {
		t.Fatalf("② 일지 「병합 검증 자리(v2)」(한가한 기계와 갈라):\n%s", jdump())
	}
	if got := whySummary(strings.Split(jdump(), "\n"), 20); !strings.Contains(got, "병합 검증 자리 1") {
		t.Fatalf("② --why 요약: %s", got)
	}
}

// ③ 한가한 기계와 자리 기계가 함께 → 한가한 기계 먼저(목록 차례가 뒤여도)
func TestGateIdleMachineFirst(t *testing.T) {
	gateKnob(t, "v1")
	s := specFx(t)
	s.idle("v1", laneBusy)
	s.idle("v2", idleOut)
	s.sub(1000000001, "a", false)
	s.round(1)
	if d := s.dispatched(); len(d) != 1 || d[0][0] != "v2" {
		t.Fatalf("③ 한가한 v2 먼저: %v", d)
	}
}

// 자리 기계 여럿 → 코어당 부하 낮은 차례
func TestGateSlotLeastLoaded(t *testing.T) {
	gateKnob(t, "v1", "v2")
	s := specFx(t)
	s.idle("v1", laneBusy)
	s.idle("v2", laneLow)
	s.sub(1000000001, "a", false)
	s.round(1)
	if d := s.dispatched(); len(d) != 1 || d[0][0] != "v2" || !s.gateEnvSent() {
		t.Fatalf("부하 낮은 자리 기계 v2: %v", d)
	}
}

// ④ 그 기계에 병합 검증이 이미 돎 → 안 고름(다른 자리 기계, 없으면 지금 길 — 직접 검증, 까닭 「맡길 한가한 기계 없음」)
func TestGateSlotTakenNotChosen(t *testing.T) {
	gateKnob(t, "v1", "v2")
	s := specFx(t)
	s.idle("v1", gateBusy)
	s.idle("v2", laneBusy)
	s.sub(1000000001, "a", false)
	s.round(1)
	if d := s.dispatched(); len(d) != 1 || d[0][0] != "v2" {
		t.Fatalf("④ 병합 검증이 도는 v1은 빼고 v2: %v", d)
	}
	s2 := specFx(t)
	s2.idle("v1", gateBusy)
	s2.idle("v2", gateBusy)
	s2.sub(1000000001, "a", false)
	s2.round(1)
	if got := strings.Join(s2.lines(s2.landlog), " "); got != "verify|a" || len(s2.dispatched()) != 0 {
		t.Fatalf("④ 자리가 다 찼으면 직접 검증: 착지 %q 띄움 %v", got, s2.dispatched())
	}
	if l := jrows(t, "착지"); len(l) == 0 || !strings.Contains(l[len(l)-1][11], "직접 검증(맡길 한가한 기계 없음)") {
		t.Fatalf("④ 정말 자리가 없을 때만 「맡길 한가한 기계 없음」:\n%s", jdump())
	}
}

// ⑤ 옛 꼴 --spec-idle(gates 칸 없음 — 옛 판 워커) → 병합 검증 자리는 모름(안 고름). 한가함 판정은 그대로(옛 꼴 verifies=0은 한가함)
func TestGateOldIdleFormatUnknown(t *testing.T) {
	gateKnob(t, "v1", "v2")
	s := specFx(t)
	s.idle("v1", "load=0.50 cpus=8 verifies=1")
	s.idle("v2", "load=7.00 cpus=8 verifies=0")
	s.sub(1000000001, "a", false)
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a" || len(s.dispatched()) != 0 {
		t.Fatalf("⑤ 옛 꼴은 자리로 안 고름: 착지 %q 띄움 %v", got, s.dispatched())
	}
	if i, ok := parseIdle(laneBusy); !ok || i.verifies != 1 {
		t.Fatalf("⑤ 새 꼴(gates 칸)을 읽는다: %v %v", i, ok)
	}
}

// 일꾼(busy_wait): 자리 일꾼(HERDR_VERIFY_GATE=1)도 다른 병합 검증이 자리를 쥐고 있으면 「바쁨」
func TestGateWorkerBusyOnlyForGate(t *testing.T) {
	gateKnob(t, "v1")
	s := specFx(t)
	t.Setenv("HC_LANE_SPEC_BUSY_WAIT", "0")
	s.idle("v1", laneBusy)
	s.idle("v2", laneBusy)
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || d[0][0] != "v1" {
		t.Fatalf("자리 기계 v1에: %v", d)
	}
	t.Setenv("HERDR_VERIFY_GATE", "1")
	idleProbe = func() (idleInfo, bool) { return parseIdle(gateBusy) }
	if rc := s.verifyOn(d[0]); rc != 0 {
		t.Fatalf("바쁨 일꾼 종료 %d", rc)
	}
	msg := s.git(s.dir, "--git-dir="+s.origin, "log", "-1", "--format=%B", specRefs+d[0][1])
	if !strings.Contains(msg, "state=busy") {
		t.Fatalf("다른 병합 검증이 자리를 쥠 → 바쁨:\n%s", msg)
	}
}

// 감사 atp3qsv 낮음 3: 손잡이 바뀜 알림 줄에 기계별 묶음 손잡이도 나온다
func TestGateKnobLine(t *testing.T) {
	if got := knobLine(knob.Remote{"merger_batch": "2", "merger_gate@v2": "1"}); got != "merger_batch=2 merger_gate@v2=1" {
		t.Fatalf("knobLine = %q", got)
	}
}

// 워커 착지 실행(merger_land_on auto)도 같은 기준: 검증 자리 남은 워커가 없으면 병합 검증 자리가 빈 워커(레인 검증 중·부하 높아도)를
// 코어당 부하가 더 낮은 보통 워커보다 먼저 — 일지 까닭 「병합 검증 자리」
func TestGateLandOnSlot(t *testing.T) {
	gateKnob(t, "v1")
	s, runs := landFx(t)
	s.idle("v1", "load=12.00 cpus=8 verifies=1 gates=0")
	s.idle("v2", "load=4.00 cpus=8 verifies=2 gates=0")
	s.sub(1000000001, "a", false)
	s.round(1)
	if r := runs(); len(r) != 1 || r[0] != "v1" || !s.onMain("a.txt") {
		t.Fatalf("자리 기계 v1에서 착지: %v\n%s", r, jdump())
	}
	if l := landOnRows(t); len(l) != 1 || !strings.HasPrefix(l[0], "v1(병합 검증 자리") {
		t.Fatalf("착지 실행 까닭: %q", l)
	}
}
