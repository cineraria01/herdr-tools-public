package main

import (
	"os"
	"strings"
	"testing"
)

// knob-test-leak ④: 2026-11-24 15:20 꼴의 검증 로그 — 통과한 병합기 계약 시험이 표준 출력에 가짜 검증 출력 「--- FAIL: TestBad」를
// 여러 번 찍고(다음 줄은 병합기 줄), 끝에 진짜 실패 넷(다음 줄은 본문)과 「FAIL <꾸러미>」. 이름표는 진짜 넷이다(고치기 전: TestBad)
const leakLog = `ok  	herdr-tools/internal/knob	0.4s
hc-lane merger: 착지 시작 — a+b+c (기준 53c85105, 묶음 3)
--- FAIL: TestBad (0.00s)
hc-lane merger: 묶음 3(a+b+c) 실패(종료 1, 로그 /x/a.log) — 마지막 c 빼고 다시
--- FAIL: TestBad (0.00s)
hc-lane merger: a 실패 — 착지 실패(종료 1), 직접 검증(추측 없음) → refs/merge-failed/1000000001-a
--- FAIL: TestPickSkipsLoadBusyMachine (1.94s)
    verifierpick_test.go:92: ① 부하 높은 v1을 건너뛰고 v2에: []
hc-lane merger: 검증 기계 한가함 — v1 한가함(load=7.00 cpus=8 verifies=0)
--- FAIL: TestBad (0.00s)
hc-lane merger: c 실패 — 착지 실패(종료 1) → refs/merge-failed/1000000003-c
--- FAIL: TestPickAllBusyVerifiesLocally (1.94s)
    verifierpick_test.go:104: ② 맡기지 않고 직접 검증 한 번
--- FAIL: TestWorkerBusyLeavesBusy (1.96s)
    verifierpick_test.go:198: v1에 띄우며 손잡이 busy_wait를 넘긴다
--- FAIL: TestPreSpecNeedsIdle (1.42s)
    verifierpick_test.go:252: ⑥ 한가한 기계가 없으면 미리 검증을 띄우지 않는다
FAIL
FAIL	herdr-tools/cmd/hc-merger	7.667s
ok  	herdr-tools/cmd/hc-sweep	3.1s
FAIL
`

func logFile(t *testing.T, s string) string {
	t.Helper()
	p := t.TempDir() + "/verify.log"
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFirstFailPicksRealFailures(t *testing.T) {
	p := logFile(t, leakLog)
	want := "herdr-tools/cmd/hc-merger: TestPickSkipsLoadBusyMachine, TestPickAllBusyVerifiesLocally, TestWorkerBusyLeavesBusy 외 1"
	for _, test := range []bool{true, false} {
		if got := scanFail(p, 0, test); got != want {
			t.Errorf("test=%v: %q, 바람 %q", test, got, want)
		}
	}
}

func TestFirstFailOtherShapes(t *testing.T) {
	for _, c := range []struct{ name, out, want string }{
		// 통과한 꾸러미만 있는데 가짜 줄이 찍혔다(고아 검사 등으로 끝 종료가 1) — 고를 이름이 없다
		{"통과 꾸러미의 가짜 줄", "--- FAIL: TestBad (0.00s)\nhc-lane merger: x\nok  \tm/a\t0.1s\n", "시험 실패(이름 못 고름 — 로그 끝 @)"},
		// 실패 꾸러미의 가짜 줄뿐(TestMain이 끝냄) — 꾸러미 줄
		{"실패 꾸러미 가짜 줄뿐", "--- FAIL: TestBad (0.00s)\nhc-lane merger: x\nFAIL\tm/a\t0.1s\nFAIL\n", "FAIL\tm/a\t0.1s"},
		// 두 꾸러미
		{"두 꾸러미", "--- FAIL: TestA (0.00s)\n    a_test.go:1: x\nFAIL\nFAIL\tm/a\t0.1s\n--- FAIL: TestB (0.00s)\n    --- FAIL: TestB/s (0.00s)\nFAIL\nFAIL\tm/b\t0.1s\nFAIL\n", "m/a: TestA · m/b: TestB"},
		// 통과한 꾸러미 뒤의 실패 꾸러미 — 앞 꾸러미의 줄은 안 섞인다
		{"앞 꾸러미 줄 안 섞임", "--- FAIL: TestBad (0.00s)\n    fake\nok  \tm/a\t0.1s\n--- FAIL: TestC (0.00s)\n    c_test.go:1: x\nFAIL\nFAIL\tm/c\t0.1s\n", "m/c: TestC"},
		// 결과 줄이 없는 로그(가짜 검증·bun)는 옛 규칙
		{"결과 줄 없음", "--- FAIL: TestBad (0.00s)\n", "--- FAIL: TestBad (0.00s)"},
	} {
		p := logFile(t, c.out)
		want := strings.ReplaceAll(c.want, "@", p)
		if got := scanFail(p, 0, true); got != flat(want) {
			t.Errorf("%s: %q, 바람 %q", c.name, got, flat(want))
		}
	}
}
