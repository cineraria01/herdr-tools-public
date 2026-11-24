package main

import (
	"strings"
	"testing"
)

// dedupe-helpers ②: hc get 종료 3이라도 완전한 not_found 오류 객체 하나가 아니면 「없음」이 아니라 모름 — 띄우지 않는다
// (공용 판독기 hcjson.ParseGet과 같은 기준. 옛 판은 종료 3만 보고 없음으로 읽어 루프를 겹쳐 띄울 수 있었다)
func TestGetRC3NotNotFoundIsUnknown(t *testing.T) {
	cases := []struct{ name, get string }{
		{"빈 출력", ""},
		{"다른 오류", `{"error":{"kind":"unreachable","message":"x"}}`},
		{"잘림", `{"error":{"kind":"not_fo`},
		{"오류 말고 칸이 더", `{"error":{"kind":"not_found"},"address":"t9"}`},
		{"JSON 둘", `{"error":{"kind":"not_found"}} {"error":{"kind":"not_found"}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if log := tick(t, c.get, 3); strings.Contains(log, "run ") || strings.Contains(log, "rm ") {
				t.Fatalf("모름이면 띄우지도 지우지도 않는다:\n%s", log)
			}
		})
	}
	if log := tick(t, `{"error":{"hint":"see hc ls --all","kind":"not_found","message":"no terminal"}}`, 3); !strings.Contains(log, "run ") {
		t.Fatalf("완전한 not_found면 띄운다:\n%s", log)
	}
	// 실제 hc 0.1.22 꼴: 표준 출력은 비고 오류 객체는 표준 오류에, 종료 3
	nf := `{"error":{"hint":"see hc ls --all","kind":"not_found","message":"no terminal \"job1\""}}`
	if log := tick(t, "", 3, "T_GET_ERR="+nf); !strings.Contains(log, "run ") {
		t.Fatalf("표준 오류의 not_found도 없음 — 띄운다:\n%s", log)
	}
	if log := tick(t, "", 3, "T_GET_ERR=hc: core unreachable"); strings.Contains(log, "run ") {
		t.Fatalf("표준 오류가 not_found가 아니면 모름:\n%s", log)
	}
}
