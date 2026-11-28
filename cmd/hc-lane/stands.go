package main

// 그 완료가 아직 서 있는가 — 제출 직전의 마지막 막음(ht-submit-still-stands, 2026-11-28 — 레인이 완료 뒤 막힘·주차를 보고했는데 앞서 돌던
// 감사의 통과로 대기열이 그 커밋을 제출했다). 대기열 쪽 물음은 cmd/hc-fill/stands.go, 여기는 이 기계의 지금 사건과 원격 refs/park로 본다:
// 판정 착지(--verdict)는 그 판정 실행의 마지막 완료·막힘·주차가 그 커밋의 완료여야 하고(lvStands), 어떤 제출이든 원격 refs/park/<레인>이
// 그 커밋이고 그 레인의 마지막 완료·주차가 그 커밋의 주차면 거절한다(parkGate — 이어 띄운 레인이 rebase 뒤 같은 커밋이면 새 완료가 그 뒤에 있다)

import (
	"errors"
	"io/fs"
	"strings"

	"herdr-tools/internal/events"
)

// laneStand <레인> <실행|""> <상태…>: 이 기계 사건에서 그 레인(실행을 주면 10칸이 그 실행인 것만)이 그 상태로 적은 마지막 사건.
// ok=false는 사건 파일을 못 읽음(없음은 사건 없음)
func laneStand(lane, run string, states ...string) (events.Event, bool) {
	ls, err := readLines(evdir() + "/events.tsv")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return events.Event{}, false
	}
	var last events.Event
	for _, l := range ls {
		e := events.Cols(l)
		if e.N >= 6 && e.Lane == lane && (run == "" || e.Run == run) && has(states, e.State) {
			last = e
		}
	}
	return last, true
}

// sameSha: 사건 sha(7자 이상 머리)가 그 40자 커밋인가
func sameSha(full, ev string) bool { return len(ev) >= 7 && strings.HasPrefix(full, ev) }

// lvStands: 판정 lvd의 완료(lvs, 실행 lvme)가 이 기계의 지금 사건에서 아직 서 있나 — 아니면 착지하지 않는다(시작 표시를 두기 전)
func lvStands(lane string) {
	e, ok := laneStand(lane, lvme, "완료", "막힘", "주차")
	if !ok {
		eprintf("hc-lane land: 사건(events.tsv)을 읽지 못해 판정 %s 의 완료가 아직 서 있는지 모른다 — 착지하지 않았다, 같은 줄을 다시\n", lvd)
		exit(75)
	}
	if e.State == "" || e.State == "완료" && sameSha(lvs, e.SHA) {
		return
	}
	eprintf("hc-lane land: 판정 %s 의 완료(%s)가 서 있지 않다 — 이 실행이 그 뒤 「%s %s」를 보고했다. 착지하지 않았다. 그래도 낼 거면 같은 커밋으로 hc-lane report 완료 %s 를 부르고 판정을 기다린다\n",
		lvd, cut(lvs, 8), e.State, e.SHA, cut(lvs, 8))
	exit(2)
}

// parkGate <제출할 커밋>: 원격 refs/park/<레인>이 그 커밋이고 이 레인의 마지막 완료·주차가 그 커밋의 주차면 거절(종료 2). 원격을 못 읽으면 lsq의 모름(75)
func parkGate(sh string) {
	pk := "refs/park/" + sln
	if lsq("주차 가지를 확인하지 못했다", pk)[pk] != sh {
		return
	}
	e, ok := laneStand(sln, "", "완료", "주차")
	if !ok {
		eprintf("hc-lane land(제출): 사건(events.tsv)을 읽지 못해 %s 가 주차된 커밋인지 모른다 — 제출하지 않았다, 같은 줄을 다시\n", pk)
		exit(75)
	}
	if e.State == "주차" && sameSha(sh, e.SHA) {
		eprintf("hc-lane land(제출): 이 커밋(%s)은 주차돼 있다(%s, 마지막 보고 「주차」) — 제출하지 않았다. 선행이 들어가 hc-fill이 이어 띄운 뒤, 또는 주차를 풀려면 hc-lane report 완료 %s 를 부른 뒤 같은 줄을 다시\n",
			cut(sh, 8), pk, cut(sh, 8))
		exit(2)
	}
}
