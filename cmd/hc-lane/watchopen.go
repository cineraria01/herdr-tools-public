// watchopen.go: watch의 쉬는 시간에서 결과를 기다리는 제출·주차를 빼고, 끝날 때 까닭을 남긴다(watch-idle-open, 2026-11-24 —
// 마지막 제출에서 결과까지 60분 넘은 착지가 30%였는데 그사이 watch가 한가로 끝나 알림이 늦거나 다른 지휘에게 갔고, 끝난 까닭은
// --rm 터미널의 표준 오류와 함께 사라졌다)
package main

import (
	"fmt"
	"os"
	"strings"

	"herdr-tools/internal/events"
	"herdr-tools/internal/oplog"
)

// myLaneLive: 이 지휘의 살아 있는 레인(parent가 --to, 모르면 이 지휘 것으로 침)이 있나
func (e *ev) myLaneLive() bool {
	for _, t := range e.tls.Terminals {
		if t.State == "live" && (t.Parent == "" || t.Parent == e.wto || strings.HasSuffix(t.Parent, "/"+e.wto)) {
			return true
		}
	}
	return false
}

// fillq: hc-fill 대기열에 아직 안 띄운 요청(wait)이 있으면 참
func fillq() bool {
	d := os.Getenv("HC_FILL_DATA")
	if d == "" {
		d = os.Getenv("HOME") + "/.local/share/hc-fill"
	}
	ls, _ := readLines(d + "/queue.tsv")
	for _, l := range ls {
		if at(strings.Split(l, tab), 5) == "wait" {
			return true
		}
	}
	return false
}

// watchOpenMax: 결과를 기다리는 제출·주차를 한가로 세지 않는 상한(초) — lane.watch_open_max시간(기본 12, 0이면 늘 셈),
// 시험·손 조정은 HC_LANE_WATCH_OPEN_MAX(초). 쉬는 시간이 찰 때마다 읽으니 돌고 있는 watch에도 바로 먹는다
func watchOpenMax() int64 {
	if s := os.Getenv("HC_LANE_WATCH_OPEN_MAX"); s != "" {
		if !digits(s) {
			die("HC_LANE_WATCH_OPEN_MAX는 0 이상 정수(초): %s", s)
		}
		return atoi(s)
	}
	return cfgInt("lane.watch_open_max", "12", "시간") * 3600
}

// openHold: 이 지휘(--to)가 주인인 레인 가운데 제출·주차 뒤에 결과(착지·막힘·완료·판정)가 아직 없고 그 제출·주차 뒤 omax초가 안 지난
// 것이 있으면 참. 열쇠는 레인 이름만 — 제출은 레인 터미널이, 그 착지·돌려보냄은 병합기 터미널이 쓰고, 주차한 레인은 새 터미널로 다시
// 뜬다(감사 atoza2r-r1pbkl 중간 1). 여러 기계의 줄을 시각 순으로 맞춘 뒤 본다. 넘긴 뒤 중간은 결과가 아니다. 원격 호출 없음 —
// watch는 쉬는 시간이 찼을 때만 부르니 바퀴마다 드는 값은 없다. ls는 정렬한다(부르는 쪽이 사본을 준다).
// ponytail: 사건 창(기계마다 끝 evWin+evCtx줄) 밖으로 밀려난 제출은 못 본다 — 그만큼 쌓이면 한가로 세고, hc-fill 바퀴가 다시 띄운다
func (e *ev) openHold(ls []string, now int64) bool {
	omax := watchOpenMax()
	sortByTime(ls)
	open := map[string]string{}
	for _, l := range ls {
		x := events.Cols(l)
		if x.N < 7 {
			continue
		}
		switch x.State {
		case "제출", "주차":
			open[x.Lane] = l
		case "착지", "막힘", "완료", "판정":
			delete(open, x.Lane)
		}
	}
	me := lastSeg(e.wto)
	for _, l := range open {
		if t, ok := stT(events.Cols(l).Time); ok && now-t < omax && e.evowner(l) == me {
			return true
		}
	}
	return false
}

// wend: watch가 끝나는 까닭을 표준 오류·watch.log·운영 기록에 한 줄. 기록은 곁에 더할 뿐 — 못 써도 끝내기는 그대로
func (e *ev) wend(why, msg string) {
	l := e.wlog(fmt.Sprintf("끝냄(%s) %s", why, msg))
	oplog.Log("hc-lane", "watch", oplog.Warn, -1, l)
}

// wlog: 시각을 붙여 표준 오류와 watch.log에 한 줄(그 줄을 돌려준다)
func (e *ev) wlog(msg string) string {
	l := fmt.Sprintf("%s hc-lane watch: %s", utcNow(), msg)
	fmt.Fprintln(os.Stderr, l)
	_ = appendLines(e.ed+"/watch.log", l)
	return l
}
