package main

import (
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/resident"
)

// ⑥ 상태 한 장의 「눈에 띄는 것」에 이 기계 상주 판 점검 결과 줄(있을 때만)
func TestStateNotableResident(t *testing.T) {
	stFx(t, `{"machines":[{"name":"w","state":"connected"},{"name":"x","state":"connected"}]}`)
	t.Setenv("XDG_STATE_HOME", "")
	dir := resident.DirFor("", HOME)
	th := stateThresholds()
	s, fc := stateCollectF(time.Now())
	for _, n := range stateNotable(s, fc, th) {
		if strings.Contains(n, "상주") {
			t.Fatalf("결과가 없는데 상주 줄: %s", n)
		}
	}
	f := resident.Finding{Who: "hc-lane watch", Pid: 41, Start: time.Now().Add(-20 * time.Hour), State: "unknown", Have: "모름",
		Installed: "11-24 12:00", Term: "t9zz", Restart: "hc kill t9zz && hc-lane coord --watch t3yx"}
	if err := resident.SaveReport(resident.ReportPath(dir), resident.Report{Checked: time.Now(), Findings: []resident.Finding{f}}); err != nil {
		t.Fatal(err)
	}
	_, fc = stateCollectF(time.Now())
	got := strings.Join(stateNotable(s, fc, th), "\n")
	if !strings.Contains(got, "옛 판으로 도는 상주: hc-lane watch(t9zz pid 41") || !strings.Contains(got, "다시 띄우기: hc kill t9zz && hc-lane coord --watch t3yx") {
		t.Fatalf("눈에 띄는 것에 상주 줄이 없다:\n%s", got)
	}
}
