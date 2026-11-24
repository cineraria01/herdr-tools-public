package main

import (
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/resident"
)

// ⑥ --status: 이 기계 상주 판 점검 결과가 있을 때만 한 줄
func TestStatusResident(t *testing.T) {
	dir := t.TempDir()
	if l := statusResident(dir, time.Now()); len(l) != 0 {
		t.Fatalf("결과가 없는데 줄: %v", l)
	}
	f := resident.Finding{Who: "herdr-balance", Pid: 7, Start: time.Now().Add(-30 * time.Hour), State: "old", Have: "11-23 16:02",
		Installed: "11-24 12:00", Restart: "launchctl kickstart -k gui/$(id -u)/com.example.balance"}
	if err := resident.SaveReport(resident.ReportPath(dir), resident.Report{Checked: time.Now(), Findings: []resident.Finding{f}}); err != nil {
		t.Fatal(err)
	}
	l := statusResident(dir, time.Now())
	if len(l) != 1 || !strings.HasPrefix(l[0], "맥 옛 판으로 도는 상주: herdr-balance(pid 7") || !strings.Contains(l[0], "다시 띄우기: launchctl kickstart") {
		t.Fatalf("상주 줄: %v", l)
	}
}
