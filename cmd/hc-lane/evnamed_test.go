package main

// events-named-cols(2026-11-24): hc-lane의 사건 칸 접근을 events.Cols 이름 칸으로 옮긴 뒤에도 열쇠·글·판정이 같은지 —
// 옮기기 전 코드(아래 old*, 칸 번호 접근 그대로 베낌)와 칸 수별 견본에서 견준다

import (
	"strings"
	"testing"

	"herdr-tools/internal/events"
)

var evCorpus = []string{
	"2026-11-21T00:00:00Z\tbox\t-\tl-a\t착지\t-\t글",
	"2026-11-24T04:09:01Z\tbox\tt20r\tl-a\t정리됨\t-\thc-sweep R2\t2",
	"2026-11-24T04:09:01Z\tm2\tm2/t20r\tl-a\t정리됨\t-\thc-sweep R2\t2",
	"2026-11-21T00:00:01Z\tbox\tt1\tl-a\t완료\tabcdef12\t글\t3\t-",
	"2026-11-21T00:00:02Z\tbox\tt1\tl-a\t완료\tabcdef12\t글\t3\t-\tt1@1795",
	"2026-11-21T00:00:03Z\tbox\tt1\tl-a\t중간\t-\t글\t3\t-\tt1@1795\tt9@1700",
	"2026-11-21T00:00:04Z\tbox\tt1\tl-a\t제출\tabcdef12\tbox-20261121T010203-42 → q\t0\t-\tt1@1795\tt9@1700\tsubmit:box-20261121T010203-42",
	"2026-11-21T00:00:05Z\tbox\tt1\tl-a\t완료\tabcdef12\t글\t0\t-\tt1@1795\tt9@1700\t-\tgithub.com/org/repo",
	"2026-11-21T00:00:05Z\tbox\tt2\tl-b\t주차\tabcdef1234\t주차함\t0\t-\tt2@1795\tt9@1700\t-\tgithub.com/org/other",
	"2026-11-21T00:00:06Z\tbox\tt1\tmerger\t막힘\t-\t병합기 멈춤\t0\t-\tt1@1795\t-\t-\t-\tops",
	"2026-11-21T00:00:07Z\tbox\tt1\tl-a\t중간\t-\t글\t0\t-\tt1@1795\t-\t-\tgithub.com/org/repo\tlane",
	"2026-11-21T00:00:08Z\tbox\tt1\tl-a\t중간\t-\t글\t0\t-\tt1@1795\t-\t-\t-\tlane\t덧\t칸",
	"2026-11-21T00:00:09Z\tbox\t-\thc-fill-idle\t중간\t-\t자리 빔\t-\t-\t-\t-\t-",
	"2026-11-21T00:00:10Z\tbox\t-\tmerger-proj\t막힘\t-\t멈춤\t-\t-\t-\t-\thf:merger:proj:dead:1:-",
	"2026-11-21T00:00:11Z\tbox\tt1\tmerger-alive\t중간\t-\t글\t-\t-\t-\t-\t-",
	"2026-11-21T00:00:12Z\tbox\tt1\tl-a\t멈춤\t-\tdone turn 4\t4\t-\t-\t-\t-",
	"2026-11-21T00:00:12Z\tbox\tbox/t1\tl-a\t멈춤\t-\tidle turn 5\t5",
	"2026-11-21T00:00:13Z\tbox\tt1\thc-lane-wait\t중간\t-\t글\t-\tt9,t8\t-\tt9@1700\t-",
	"2026-11-21T00:00:14Z\t\t\tl-b\t\t\t\t\t\t\t\t",
	"2026-11-21T00:00:15Z\tbox\tt2\tl-b\t제출\tfedcba98\tx\t1",
	"2026-11-21T00:00:16Z\tbox\tt2\tl-b\t주차\tfedcba9876\tx\t1",
	"모름 box", "#since\t2026", "멈춤\tbox/t1\tl-a\t쉼 turn 3", "멈춤\tt1\tl-a", "", "\t", "a\tb\tc",
}

func oldFld(f []string, i int) string {
	if i-1 < len(f) {
		return f[i-1]
	}
	return ""
}

func oldNk(l string) string {
	if l == "" {
		return l
	}
	f := strings.Split(l, tab)
	if f[0] == "멈춤" && len(f) == 4 {
		return "멈춤" + tab + lastSeg(f[1]) + tab + f[2] + tab + f[3]
	}
	if len(f) < 7 || strings.HasPrefix(f[0], "#") {
		return l
	}
	f[2] = lastSeg(f[2])
	return f[0] + tab + "*" + tab + strings.Join(f[2:], tab)
}

func oldEvKey(f []string) string {
	if oldFld(f, 5) != "멈춤" {
		return strings.Join(f, tab)
	}
	w := oldFld(f, 7)
	if strings.HasPrefix(w, "done ") || strings.HasPrefix(w, "idle ") {
		w = "쉼 " + w[5:]
	}
	return "멈춤" + tab + oldFld(f, 3) + tab + oldFld(f, 4) + tab + w
}

func oldWpart(l string, kindOf func([]string) string, opNext func(lane, key string) (string, bool)) string {
	f := strings.Split(l, tab)
	s := oldFld(f, 4) + " " + oldFld(f, 5)
	if oldFld(f, 6) != "-" {
		s += " " + oldFld(f, 6)
	}
	where := oldFld(f, 3)
	if where == "-" {
		where = oldFld(f, 2)
	}
	s += " " + oldFld(f, 7) + " (" + where + ")"
	if kindOf(f) != "ops" {
		return s
	}
	if n, ok := opNext(oldFld(f, 4), oldFld(f, 12)); ok {
		s += " → " + n
	}
	return "운영: " + s
}

func oldLaneRepos(ls []string) map[string]string {
	m := map[string]string{}
	for _, l := range ls {
		f := strings.Split(l, tab)
		ln, r := oldFld(f, 4), oldFld(f, 13)
		if ln == "" || ln == "-" || !repoIDok(r) {
			continue
		}
		if p, ok := m[ln]; ok && p != r {
			r = ""
		}
		m[ln] = r
	}
	return m
}

func oldLastParkIs(ls []string, lane, sha string) bool {
	ok := false
	for _, l := range ls {
		f := strings.Split(l, "\t")
		if len(f) < 6 || f[3] != lane || (f[4] != "주차" && f[4] != "제출") {
			continue
		}
		ok = f[4] == "주차" && len(f[5]) >= 7 && strings.HasPrefix(sha, f[5])
	}
	return ok
}

func oldTurnOf(f []string) string {
	if len(f) >= 8 {
		return f[7]
	}
	return "-"
}

func oldLastIn(evl []string, a, nm string) (last, lt string, sw bool) {
	r := newLastRep()
	have := false
	for _, l := range evl {
		f := strings.Split(l, tab)
		if oldFld(f, 3) != a || oldFld(f, 4) != nm {
			continue
		}
		if f[4] == "정리됨" {
			sw = true
			continue
		}
		r.rep(f[4], f[5], oldTurnOf(f), "x")
		have, sw = true, false
	}
	if !have {
		return "", "0", sw
	}
	return r.lst["x"], r.ltn["x"], sw
}

// ③ 줄 열쇠(커서·보냄 표시)가 한 글자도 안 바뀐다 ① 글·판정이 같다
func TestEventNamedColsSameAsBefore(t *testing.T) {
	kindOf := events.KindOf // 옮기기 전과 같음은 internal/events cols_test가 본다
	opNextOf := func(lane, key string) (string, bool) { o, ok := events.OpOf(lane, key); return o.Next, ok }
	for _, l := range evCorpus {
		if got, want := nk(l), oldNk(l); got != want {
			t.Errorf("nk(%q) = %q, 전 %q", l, got, want)
		}
		if got, want := lineKey(l), oldNk(oldEvKey(strings.Split(l, tab))); got != want {
			t.Errorf("lineKey(%q) = %q, 전 %q", l, got, want)
		}
		if !strings.Contains(l, tab) || !nf7(l) {
			continue
		}
		if got, want := wpart(l), oldWpart(l, kindOf, opNextOf); got != want {
			t.Errorf("wpart(%q) = %q, 전 %q", l, got, want)
		}
	}
	got, want := laneRepos(evCorpus), oldLaneRepos(evCorpus)
	if len(got) != len(want) {
		t.Fatalf("laneRepos %v, 전 %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("laneRepos[%s] = %q, 전 %q", k, got[k], v)
		}
	}
	for _, c := range [][2]string{{"l-b", "fedcba9876aa"}, {"l-b", "fedcba98"}, {"l-a", "abcdef12"}, {"없음", "x"}} {
		for n := 1; n <= len(evCorpus); n++ {
			if a, b := lastParkIs(evCorpus[:n], c[0], c[1]), oldLastParkIs(evCorpus[:n], c[0], c[1]); a != b {
				t.Errorf("lastParkIs(앞 %d줄, %v) = %v, 전 %v", n, c, a, b)
			}
		}
	}
	for _, c := range [][2]string{{"t1", "l-a"}, {"t20r", "l-a"}, {"t2", "l-b"}, {"box/t1", "l-a"}, {"", "l-b"}} {
		for n := 1; n <= len(evCorpus); n++ {
			l1, t1, s1 := lastIn(evCorpus[:n], c[0], c[1])
			l2, t2, s2 := oldLastIn(evCorpus[:n], c[0], c[1])
			if l1 != l2 || t1 != t2 || s1 != s2 {
				t.Errorf("lastIn(앞 %d줄, %v) = %q %q %v, 전 %q %q %v", n, c, l1, t1, s1, l2, t2, s2)
			}
		}
	}
}
