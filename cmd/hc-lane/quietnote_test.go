package main

// stall-after-submit-note(2026-11-23): 제출·주차 뒤 적은 「중간」 한 줄이 넘김을 지우지 않는다 — 넘긴 레인은 그 뒤 쉬거나 닫혀도 조용.
// 넘긴 뒤 다시 일을 맡아 완료·막힘·착지·판정을 적으면 지금 규칙, 넘기지 않은 레인의 중간 뒤 멈춤은 그대로 알림

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func noteRow(i int, st, tn string) string {
	return strings.Join([]string{fmt.Sprintf("2026-11-23T00:%02d:00Z", i), "m", "m/t1", "lane-a", st, "abc", "보고", tn}, tab)
}

func TestStopAfterHandoffNote(t *testing.T) {
	cases := []struct {
		name      string
		seq       [][2]string // (상태, turn)
		s, ss, tn string
		want      string
	}{
		{"완료→제출→중간 뒤 done", [][2]string{{"완료", "1"}, {"제출", "2"}, {"중간", "2"}}, "live", "done", "3", ""},
		{"완료→제출→중간 뒤 한 턴 더 idle", [][2]string{{"완료", "1"}, {"제출", "2"}, {"중간", "2"}}, "live", "idle", "4", ""},
		{"주차→중간 뒤 done", [][2]string{{"주차", "2"}, {"중간", "2"}}, "live", "done", "4", ""},
		{"완료→제출→중간 뒤 닫힘", [][2]string{{"완료", "1"}, {"제출", "2"}, {"중간", "2"}}, "exited", "", "4", ""},
		{"제출→중간→완료 바로 뒤 쉼", [][2]string{{"제출", "2"}, {"중간", "3"}, {"완료", "3"}}, "live", "done", "4", ""},
		{"제출→중간→완료 뒤 한참 쉼", [][2]string{{"제출", "2"}, {"중간", "3"}, {"완료", "3"}}, "live", "done", "6", "done"},
		{"제출→중간→막힘 뒤 쉼", [][2]string{{"제출", "2"}, {"중간", "3"}, {"막힘", "3"}}, "live", "idle", "4", ""},
		{"제출→중간→완료 뒤 닫힘(착지 전)", [][2]string{{"제출", "2"}, {"중간", "3"}, {"완료", "3"}}, "exited", "", "4", "뜻밖의 종료(exited)"},
		{"넘기지 않은 레인의 중간 뒤 done", [][2]string{{"완료", "1"}, {"중간", "2"}}, "live", "done", "3", "done"},
		{"넘기지 않은 레인의 중간 뒤 닫힘", [][2]string{{"중간", "2"}}, "exited", "", "3", "뜻밖의 종료(exited)"},
		{"제출→중간 뒤라도 blocked는 알림", [][2]string{{"제출", "2"}, {"중간", "2"}}, "live", "blocked", "3", "blocked"},
	}
	for _, c := range cases {
		var evl []string
		for i, r := range c.seq {
			evl = append(evl, noteRow(i, r[0], r[1]))
		}
		last, lt, sw := lastIn(evl, "m/t1", "lane-a")
		if got := stopWhy(c.s, c.ss, c.tn, last, lt, sw, false); got != c.want {
			t.Errorf("%s: stopWhy = %q(마지막 %s turn %s), 기대 %q", c.name, got, last, lt, c.want)
		}
	}
}

// 같은 차례를 hc-sweep이 정리됨으로 닫아도(newEvents) 제출 뒤 중간이면 조용, 넘기지 않은 중간이면 뜻밖의 종료
func TestCleanedAfterHandoffNote(t *testing.T) {
	cases := []struct {
		name string
		seq  []string
		want bool
	}{
		{"제출→중간→정리됨", []string{"제출", "중간"}, false},
		{"주차→중간→정리됨", []string{"주차", "중간"}, false},
		{"완료→중간→정리됨", []string{"완료", "중간"}, true},
		{"제출→중간→완료→정리됨(착지 전)", []string{"제출", "중간", "완료"}, true},
	}
	for _, c := range cases {
		d := t.TempDir()
		e := &ev{ed: d, cur: filepath.Join(d, "cur")}
		var cand []string
		for i, st := range c.seq {
			cand = append(cand, noteRow(i, st, "3"))
		}
		cand = append(cand, strings.Join([]string{"2026-11-23T00:30:00Z", "m", "m/t1", "lane-a", "정리됨", "-", "hc-sweep R2", "4"}, tab))
		got := false
		for _, l := range e.newEvents(nil, cand, "") {
			if strings.Contains(l, tab+"멈춤"+tab) {
				got = true
			}
		}
		if got != c.want {
			t.Errorf("%s: 멈춤 보냄 = %v, 기대 %v", c.name, got, c.want)
		}
	}
}
