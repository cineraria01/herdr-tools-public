package main

// watch-idle-open 감사 atoza2r-r1pbkl 중간 1: 제출은 레인 터미널이, 그 착지·돌려보냄은 병합기 터미널이 쓰고, 주차한 레인은 새 터미널로
// 다시 뜬다 — 열쇠가 「터미널+레인」이면 착지가 와도 제출이 닫히지 않아 watch를 lane.watch_open_max까지 붙잡았다

import (
	"strings"
	"testing"
	"time"
)

func TestOpenHoldLaneKey(t *testing.T) {
	t.Setenv("HC_LANE_WATCH_OPEN_MAX", "3600")
	now := time.Now().Unix()
	ln := func(ago int64, term, lane, st, owner string) string {
		return strings.Join([]string{utcAt(now - ago), "m", term, lane, st, "-", "요약", "1", "-", "-", owner}, tab)
	}
	e := &ev{wto: "t9", ed: t.TempDir()}
	for _, c := range []struct {
		name string
		ls   []string
		want bool
	}{
		{"제출만 — 열림", []string{ln(60, "t5", "y", "제출", "t9")}, true},
		{"제출 뒤 중간 — 열림", []string{ln(60, "t5", "y", "제출", "t9"), ln(50, "t5", "y", "중간", "t9")}, true},
		{"병합기 터미널의 착지 — 닫힘", []string{ln(60, "t5", "y", "제출", "t9"), ln(50, "t1my", "y", "착지", "t9")}, false},
		{"병합기 터미널의 돌려보냄(막힘) — 닫힘", []string{ln(60, "t5", "y", "제출", "t9"), ln(50, "t1my", "y", "막힘", "t9")}, false},
		{"주차 → 새 터미널 완료·착지 — 닫힘", []string{ln(90, "t5", "y", "주차", "t9"), ln(70, "t7", "y", "완료", "t9"), ln(50, "t1my", "y", "착지", "t9")}, false},
		{"다른 기계 줄이 앞에 와도 시각 순 — 닫힘", []string{ln(50, "m2/t1my", "y", "착지", "t9"), ln(60, "t5", "y", "제출", "t9")}, false},
		{"돌려보낸 뒤 다시 제출 — 열림", []string{ln(90, "t5", "y", "제출", "t9"), ln(80, "t1my", "y", "막힘", "t9"), ln(60, "t6", "y", "제출", "t9")}, true},
		{"남의 지휘 제출 — 안 셈", []string{ln(60, "t5", "y", "제출", "t-other")}, false},
		{"상한보다 오래된 제출 — 셈", []string{ln(7200, "t5", "y", "제출", "t9")}, false},
	} {
		if got := e.openHold(append([]string(nil), c.ls...), now); got != c.want {
			t.Errorf("%s: openHold %v, 바람 %v", c.name, got, c.want)
		}
	}
}
