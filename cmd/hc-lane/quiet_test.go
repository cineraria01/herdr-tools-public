package main

// go-alert-quiet(2026-11-22): 제출·주차를 마친 레인 터미널은 닫히거나(hc-sweep 정리됨·끝남) 차례를 끝내 쉬어도 지휘에 안 알린다.
// 정말 뜻밖(작업 중·완료 전)만 멈춤으로.

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStopWhy(t *testing.T) {
	cases := []struct {
		name                string
		s, ss, tn, last, lt string
		sw, rd              bool
		want                string
	}{
		// 살아 있음 · done/idle
		{"제출 뒤 쉼", "live", "done", "9", "제출", "3", false, false, ""},
		{"주차 뒤 쉼", "live", "idle", "9", "주차", "3", false, false, ""},
		{"착지 바로 뒤 쉼", "live", "done", "4", "착지", "3", false, false, ""},
		{"착지 뒤 이어 받은 일 끝에 쉼", "live", "done", "6", "착지", "3", false, false, "done"},
		{"완료 바로 뒤 쉼(감사 기다림)", "live", "idle", "4", "완료", "3", false, false, ""},
		{"작업 중(중간) 뒤 쉼", "live", "done", "4", "중간", "3", false, false, "done"},
		{"report 없이 쉼", "live", "done", "2", "", "0", false, false, "done"},
		{"제출 뒤라도 blocked는 알림", "live", "blocked", "9", "제출", "3", false, false, "blocked"},
		{"working은 조용", "live", "working", "9", "중간", "3", false, false, ""},
		// 끝남(state가 live 아님)
		{"제출 뒤 끝남", "exited", "", "9", "제출", "3", false, false, ""},
		{"주차 뒤 끝남", "exited", "", "9", "주차", "3", false, false, ""},
		{"착지 바로 뒤 끝남", "exited", "", "4", "착지", "3", false, false, ""},
		{"착지 뒤 이어 받은 일 중 끝남", "exited", "", "6", "착지", "3", false, false, "뜻밖의 종료(exited)"},
		{"완료 전 끝남", "exited", "", "4", "중간", "3", false, false, "뜻밖의 종료(exited)"},
		{"쓰기 레인 완료 뒤 끝남(착지 전)", "exited", "", "4", "완료", "3", false, false, "뜻밖의 종료(exited)"},
		{"읽기 레인 완료 뒤 끝남", "exited", "", "4", "완료", "3", false, true, ""},
		{"정리됨이 따로 감", "exited", "", "4", "중간", "3", true, false, ""},
	}
	for _, c := range cases {
		if got := stopWhy(c.s, c.ss, c.tn, c.last, c.lt, c.sw, c.rd); got != c.want {
			t.Errorf("%s: stopWhy = %q, 기대 %q", c.name, got, c.want)
		}
	}
}

// hc-sweep이 적은 「정리됨」: 앞 report가 제출·주차면 turn과 상관없이 안 보냄, 착지는 바로 뒤만 조용, 작업 중이면 뜻밖의 종료
func TestCleanedEvent(t *testing.T) {
	cases := []struct {
		name, last, lt, tn string
		want               bool // 멈춤 사건을 보내나
	}{
		{"제출 바로 뒤", "제출", "3", "4", false},
		{"제출 뒤 차례가 더 지남(깨우기 글 등)", "제출", "3", "9", false},
		{"주차 뒤", "주차", "3", "9", false},
		{"착지 바로 뒤", "착지", "3", "4", false},
		{"착지 뒤 이어 받은 일 중", "착지", "3", "9", true},
		{"작업 중", "중간", "3", "4", true},
		{"완료 뒤(착지 전)", "완료", "3", "4", true},
	}
	for _, c := range cases {
		d := t.TempDir()
		e := &ev{ed: d, cur: filepath.Join(d, "cur")}
		cand := []string{
			strings.Join([]string{"2026-11-22T00:00:00Z", "m", "m/t1", "lane-a", c.last, "abc", "보고", c.lt}, tab),
			strings.Join([]string{"2026-11-22T00:30:00Z", "m", "m/t1", "lane-a", "정리됨", "-", "hc-sweep R2", c.tn}, tab),
		}
		got := false
		for _, l := range e.newEvents(cand, "") {
			if strings.Contains(l, tab+"멈춤"+tab) {
				got = true
				if !strings.Contains(l, "뜻밖의 종료(정리됨) hc-sweep R2") {
					t.Errorf("%s: 멈춤 글 %q", c.name, l)
				}
			}
		}
		if got != c.want {
			t.Errorf("%s: 멈춤 보냄 = %v, 기대 %v", c.name, got, c.want)
		}
	}
}
