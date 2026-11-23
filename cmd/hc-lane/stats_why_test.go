package main

import (
	"strings"
	"testing"
)

// merger-journal-why(2026-11-24): 병합기 일지 착지·반송 줄의 까닭 칸 끝에 검증 길(「, 검증 건너뜀(추측 <아이디 8> · <기계>)」·
// 「, 직접 검증(<까닭>)」)이 붙어도 hc-lane stats 표는 그대로다(돌려보낸 까닭은 종료 코드로 묶고, 「이미 … 에 있음」 정리 판별도 그대로)
func TestStatsReportReadsJournalWhy(t *testing.T) {
	ev, jr := statsFixture()
	jr = append(jr, stJ("11:50", "r", "S7", "11:00", "1", "11:40", "11:50", "돌려보냄", "-", "착지 실패(종료 1)"))
	want := statsReport(ev, jr, "2026-11-22T00:00:00Z", "2026-11-23T00:00:00Z")
	for i, l := range jr {
		switch {
		case strings.Contains(l, "\t착지\t") && i%2 == 0:
			jr[i] = l + ", 검증 건너뜀(추측 0123abcd · v1)"
		case strings.Contains(l, "\t착지\t"):
			jr[i] = l + ", 직접 검증(묶음)"
		case strings.Contains(l, "\t돌려보냄\t") && !strings.Contains(l, "종료 3"):
			jr[i] = l + ", 직접 검증(추측 실패)"
		}
	}
	if got := statsReport(ev, jr, "2026-11-22T00:00:00Z", "2026-11-23T00:00:00Z"); got != want {
		t.Errorf("검증 길이 붙은 일지가 표를 바꿨다\n--- 받음\n%s--- 바람\n%s", got, want)
	}
	// 반송 까닭 묶음: 「착지 실패(종료 1), 직접 검증(추측 실패)」 → 「종료 1」
	if w := stWhy("착지 실패(종료 1), 직접 검증(추측 실패)"); w != "종료 1" {
		t.Errorf("반송 까닭 묶음: %q", w)
	}
}
