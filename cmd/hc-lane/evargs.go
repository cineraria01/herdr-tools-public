package main

// evargs.go: events·wait·watch 인자 읽기(watch.go evCmd에서 옮김 — 동작 그대로) + 레인 이름 거름 --lane(2026-11-24 cli-usage-version:
// 레인 하나가 20초마다 events --name <레인>을 되풀이해 운영 기록 116줄 가운데 81줄이 「알 수 없는 인자」였다).
// 인자 오류는 dieArg(글·종료 코드는 die 그대로, 운영 기록은 usage)

import (
	"strings"
)

// evKnown: events·wait·watch가 받는 인자(모르는 인자 권함용)
var evKnown = []string{"--all", "--on", "--since", "--timeout", "--state", "--mine", "--to", "--lane"}

// args: 인자를 e에 읽고 wait의 --timeout(분) 글을 돌려준다
func (e *ev) args(a []string) string {
	ec, to := e.ec, "110"
	for i := 0; i < len(a); i++ {
		switch a[i] {
		case "--all":
			e.all = true
		case "--on":
			v := val(a, i)
			if v == "" || strings.ContainsAny(v, "/ ") {
				dieArg("--on 기계 이름이 이상하다: %s", v)
			}
			e.ons = append(e.ons, v)
			i++
		case "--since":
			e.since = val(a, i)
			i++
		case "--timeout":
			if ec != "wait" {
				dieArg("--timeout은 wait에서만 쓴다")
			}
			to = val(a, i)
			i++
		case "--state":
			if ec == "events" {
				dieArg("--state는 wait·watch에서만 쓴다")
			}
			e.sts = val(a, i)
			i++
		case "--mine":
			if ec == "events" {
				dieArg("--mine은 wait·watch에서만 쓴다")
			}
			e.mine = true
		case "--to":
			if ec != "watch" {
				dieArg("--to는 watch에서만 쓴다")
			}
			e.wto = val(a, i)
			i++
		case "--lane":
			if ec == "watch" {
				dieArg("--lane은 events·wait에서만 쓴다")
			}
			v := val(a, i)
			if v == "" || !laneChars(v) {
				dieArg("--lane 레인 이름은 a-z 0-9 - _ 만: %s", v)
			}
			e.lanes = append(e.lanes, v)
			i++
		default:
			unknownArg(a[i], "", evKnown...)
		}
	}
	if !digits(to) {
		dieArg("--timeout은 분(0 이상 정수): %s", to)
	}
	if ec == "watch" && (e.wto == "" || strings.HasPrefix(e.wto, "-") || !onlyChars(e.wto, alnum+"_./-")) {
		dieArg("watch: --to <지휘 터미널 주소>가 없거나 이상하다: %s", e.wto)
	}
	for _, x := range strings.Fields(strings.ReplaceAll(e.sts, ",", " ")) {
		if !inWords(evstates+" 멈춤", x) {
			dieArg("--state는 %s 멈춤 가운데: %s", evstates, x)
		}
	}
	e.since = sinceArg(e.since)
	return to
}

// laneOK: --lane이 없거나 그 사건(탭 칸)의 레인 칸(넷째)이 --lane 가운데 하나
func (e *ev) laneOK(f []string) bool {
	return len(e.lanes) == 0 || has(e.lanes, fld(f, 4))
}
