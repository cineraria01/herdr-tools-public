package gitx

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"herdr-tools/internal/proc"
)

// dedupe-helpers ③: hc-lane(fetchrCmd — stderr nil이면 버림)·hc-merger(fetchr quiet) 두 옛 판의 꼴을 덮는 표
func TestFetchrTable(t *testing.T) {
	const lock = "error: cannot lock ref 'refs/x'\n"
	cases := []struct {
		name    string
		rcs     []int  // 시도마다 종료 코드
		errText string // 시도마다 오류 글
		quiet   bool
		calls   int
		rc      int
		out     string // stderr에 남는 것(quiet면 빈 값)
	}{
		{"성공 한 번", []int{0}, "", false, 1, 0, ""},
		{"경합 아닌 실패는 한 번", []int{128}, "fatal: x\n", false, 1, 128, "fatal: x\n"},
		{"경합이면 끝까지", []int{1, 1, 1, 1}, lock, false, 4, 1,
			strings.Repeat(lock+"hc-lane: git fetch 참조 잠금 경합(같은 클론의 다른 fetch·착지) — 0초 뒤 다시\n", 3) + lock},
		{"경합 뒤 성공", []int{1, 0}, lock, false, 2, 0, lock + "hc-lane: git fetch 참조 잠금 경합(같은 클론의 다른 fetch·착지) — 0초 뒤 다시\n"},
		{"시간 넘김은 다시 안 돎·반쪽 글 안 냄", []int{proc.TimeoutCode}, lock, false, 1, proc.TimeoutCode, ""},
		{"quiet(nil)도 경합이면 다시", []int{1, 1, 1, 1}, lock, true, 4, 1, ""},
	}
	t.Setenv("HC_LANE_FETCH_WAITS", "0 0 0")
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			var w io.Writer = &out
			if c.quiet {
				w = nil
			}
			n := 0
			rc := Fetchr(w, func(e io.Writer) int {
				r := c.rcs[n]
				n++
				if r != 0 {
					io.WriteString(e, c.errText)
				}
				return r
			})
			if rc != c.rc || n != c.calls {
				t.Fatalf("rc %d(바람 %d) 시도 %d(바람 %d)", rc, c.rc, n, c.calls)
			}
			if out.String() != c.out {
				t.Fatalf("오류 출력:\n%q\n바람:\n%q", out.String(), c.out)
			}
		})
	}
}

// 빈 HC_LANE_FETCH_WAITS는 기본(1 2 4), 끝 표지 「-」
func TestFetchWaits(t *testing.T) {
	t.Setenv("HC_LANE_FETCH_WAITS", "")
	if got := strings.Join(FetchWaits(), " "); got != "1 2 4 -" {
		t.Fatal(got)
	}
	t.Setenv("HC_LANE_FETCH_WAITS", " 0.5  3 ")
	if got := strings.Join(FetchWaits(), " "); got != "0.5 3 -" {
		t.Fatal(got)
	}
}
