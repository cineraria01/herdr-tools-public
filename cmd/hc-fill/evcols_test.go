package main

// events-named-cols(2026-11-24): hc-fill의 사건 칸 이름(cols.go ev…)이 사건 줄 한 곳의 형(internal/events.Cols)과 같은 칸을
// 가리킨다 — 사건에 칸을 더하거나 옮기면 이 시험이 hc-fill 쪽도 맞추라고 알린다

import (
	"testing"

	"herdr-tools/internal/events"
)

func TestEvColsMatchEvents(t *testing.T) {
	for _, l := range []string{
		"2026-11-21T00:00:00Z\tbox\t-\tl-a\t착지\t-\t글",
		"2026-11-24T04:09:01Z\tbox\tt20r\tl-a\t정리됨\t-\thc-sweep R2\t2",
		"2026-11-21T00:00:02Z\tbox\tt1\tl-a\t완료\tabcdef12\t글\t3\tt9\tt1@1795",
		"2026-11-21T00:00:04Z\tbox\tt1\tl-a\t제출\tabcdef12\tid → q\t0\t-\tt1@1795\tt9@1700\tsubmit:id",
		"2026-11-21T00:00:05Z\tbox\tt1\tl-a\t완료\tabcdef12\t글\t0\t-\tt1@1795\tt9@1700\t-\tgithub.com/org/repo\tlane",
		"모름 box",
	} {
		e, r := events.Cols(l), splitRow(l)
		for _, c := range []struct {
			col       int
			got, want string
		}{
			{evTime, r.F(evTime), e.Time}, {evMach, r.F(evMach), e.Machine}, {evTerm, r.F(evTerm), e.Term}, {evLane, r.F(evLane), e.Lane},
			{evState, r.F(evState), e.State}, {evSHA, r.F(evSHA), e.SHA}, {evMsg, r.F(evMsg), e.Msg}, {evTo, r.F(evTo), e.To}, {evRun, r.F(evRun), e.Run},
		} {
			if c.got != c.want {
				t.Errorf("%q: hc-fill 칸 %d = %q, internal/events %q", l, c.col, c.got, c.want)
			}
		}
		if r.NF() != e.N {
			t.Errorf("%q: 칸 수 %d, internal/events %d", l, r.NF(), e.N)
		}
	}
}
