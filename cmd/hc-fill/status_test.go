package main

import (
	"strings"
	"testing"
	"time"
)

// status 단계: 상태 조합마다 한 줄(go-lane-status)
func TestStatusStage(t *testing.T) {
	last := func(st, sha, msg string) Row { return Row{"l", st, sha, "12:00", msg, "12:01", "fresh", "r1"} }
	cases := []struct {
		in   stIn
		want string
	}{
		{stIn{State: "wait"}, "대기"},
		{stIn{State: "wait", Early: "parked"}, "대기(주차 — 선행 기다림)"},
		{stIn{State: "sent"}, "되돌려줌"},
		{stIn{State: "up", Live: "working"}, "작업 중(working)"},
		{stIn{State: "up", Live: "?연결 안 됨"}, "모름(연결 안 됨)"},
		{stIn{State: "up", Live: "?연결 확인 못 함"}, "모름(연결 확인 못 함)"},
		{stIn{State: "up", Live: "?hc ls 못 읽음"}, "모름(hc ls 못 읽음)"},
		{stIn{State: "up"}, "끝남"},
		{stIn{State: "up", Live: "idle", Last: last("완료", "abcd1234", "x")}, "완료 abcd1234"},
		{stIn{State: "up", Live: "idle", Last: last("완료", "abcd1234", "x"), Aud: "자동 감사(완료 기다림)"}, "완료 abcd1234"},
		{stIn{State: "up", Live: "idle", Last: last("완료", "abcd1234", "x"), Aud: "감사 중(1번째)"}, "감사 중(1번째)"},
		{stIn{State: "up", Live: "working", Last: last("중간", "-", "x"), Aud: "감사 지적 2번째"}, "감사 지적 2번째"},
		{stIn{State: "up", Live: "idle", Last: last("제출", "abcd1234", "x"), Pos: 1}, "제출됨(병합기 줄 맨 앞)"},
		{stIn{State: "up", Live: "idle", Last: last("제출", "abcd1234", "x"), Pos: 3}, "제출됨(병합기 줄 3번째)"},
		{stIn{State: "up", Last: last("제출", "abcd1234", "x"), Pos: -1}, "제출됨(병합기 줄 모름)"},
		{stIn{State: "up", Last: last("제출", "abcd1234", "x")}, "제출됨(줄에서 빠짐 — 결과 기다림)"},
		{stIn{State: "up", Last: last("착지", "beef0001", "x"), Aud: "감사 통과 받음"}, "착지 beef0001"},
		{stIn{State: "up", Live: "idle", Last: last("막힘", "-", "선행 대기 초과")}, "막힘: 선행 대기 초과"},
		{stIn{State: "up", Last: last("주차", "abcd1234", "x")}, "주차"},
	}
	for _, c := range cases {
		if got := stStage(c.in); got != c.want {
			t.Errorf("%+v: %q, want %q", c.in, got, c.want)
		}
	}
}

// 병합기 줄은 제출 시각 순, 착지 표식은 <레인>/<초>만(제출 ID 표식은 뺌)
func TestStatusRemote(t *testing.T) {
	out := "s1\trefs/merge-queue/1795000200-b\n" +
		"s2\trefs/merge-queue/1795000100-a\n" +
		"s3\trefs/merge-queue/bad\n" +
		"s4\trefs/hc-landed/a/1795000900\n" +
		"s5\trefs/hc-landed/b/1795000300\n" +
		"s6\trefs/hc-landed/mini-1795000000-123\n"
	q, l := stRemote(out)
	if strings.Join(q, ",") != "a,b" {
		t.Errorf("줄 %v", q)
	}
	if len(l) != 2 || l[0] != 1795000300 || l[1] != 1795000900 {
		t.Errorf("착지 %v", l)
	}
}

// 살아 있음: 주소·이름이 함께 맞을 때만, 모르면 「?」(끝남으로 읽지 않음)
func TestStatusLive(t *testing.T) {
	ts := []lsTerm{{"t1", "working", "a"}, {"mini/t2", "idle", "b"}, {"t3", "idle", "남"}}
	cases := []struct {
		termsOK, connKnown bool
		conn               []string
		addr, name, want   string
	}{
		{true, true, nil, "t1", "a", "working"},
		{true, true, nil, "t3", "c", ""}, // 주소 재사용 — 남의 터미널
		{true, true, nil, "t9", "c", ""}, // 이 기계, 목록에 없음 = 끝남
		{false, true, nil, "t1", "a", "?hc ls 못 읽음"},
		{true, true, []string{"mini"}, "mini/t2", "b", "idle"},
		{true, true, nil, "mini/t2", "b", "?연결 안 됨"},
		{true, false, nil, "mini/t2", "b", "idle"},       // 연결 모름이어도 목록에 있으면
		{true, false, nil, "mini/t8", "x", "?연결 확인 못 함"}, // 연결 모름 + 목록에 없음 = 모름(ls와 같은 글)
		{true, false, nil, "t9", "c", ""},
	}
	for _, c := range cases {
		if got := stLive(ts, c.termsOK, c.conn, c.connKnown, c.addr, c.name); got != c.want {
			t.Errorf("%+v: %q, want %q", c, got, c.want)
		}
	}
}

func TestStatusEstimate(t *testing.T) {
	if s := stEstimate(2, []int64{100}); s != "" {
		t.Errorf("착지 하나면 빈 값: %q", s)
	}
	if s := stEstimate(0, []int64{0, 600}); s != "" {
		t.Errorf("줄에 없으면 빈 값: %q", s)
	}
	// 간격 10m·10m·10m·2h(쉰 틈)·10m → 중앙값 10m, 2번째면 ~20m. 오래된 착지(첫 값)는 6개 밖이라 안 셈
	l := []int64{0, 100000, 100600, 101200, 101800, 109000, 109600}
	want := "~20m (줄 2번째 → 착지 2번 × 최근 착지 간격 중앙값 10m, 최근 착지 6번·한 번에 최대 1개)"
	if s := stEstimate(2, l); s != want {
		t.Errorf("%q, want %q", s, want)
	}
	// 묶음 착지(감사 중간 1): 몇 초 사이 표식은 착지 한 번 — 간격 0으로 무너지지 않고, 3개씩 넣으니 4번째는 착지 2번 뒤
	b := []int64{1000, 1001, 1002, 2800, 2801, 2802}
	for pos, want := range map[int]string{
		4: "~1h00m (줄 4번째 → 착지 2번 × 최근 착지 간격 중앙값 30m, 최근 착지 2번·한 번에 최대 3개)",
		3: "~30m (줄 3번째 → 착지 1번 × 최근 착지 간격 중앙값 30m, 최근 착지 2번·한 번에 최대 3개)",
	} {
		if s := stEstimate(pos, b); s != want {
			t.Errorf("묶음 %d: %q, want %q", pos, s, want)
		}
	}
	if s := stEstimate(1, []int64{1000, 1001, 1100}); s != "" {
		t.Errorf("한 번의 묶음만이면 간격을 모름 — 빈 값: %q", s)
	}
	// 섞임: 600·601(묶음 2)·1200·1800 → 간격 10m 셋, 3번째는 착지 2번
	if s := stEstimate(3, []int64{0, 600, 601, 1200, 1800}); !strings.HasPrefix(s, "~20m (줄 3번째 → 착지 2번") {
		t.Errorf("섞임 %q", s)
	}
	if a, b := stPad("레인", 6), stPad("ab", 6); a != "레인  " || b != "ab    " || stPad("긴이름이다", 4) != "긴이름이다" {
		t.Errorf("stPad %q %q", a, b)
	}
	if stDur(3900) != "1h05m" || stDur(59) != "1m" {
		t.Errorf("stDur %s %s", stDur(3900), stDur(59))
	}
	now := time.Date(2026, 11, 22, 3, 0, 0, 0, time.Local)
	if a := stAge("2026-11-22T01:45", now); a != "1h15m" {
		t.Errorf("stAge %s", a)
	}
	if a := stAge("t", now); a != "-" {
		t.Errorf("stAge 못 읽음 %s", a)
	}
}
