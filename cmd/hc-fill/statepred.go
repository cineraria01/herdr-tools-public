package main

// 선행 단계 재료(ht-advise-stage-lookup, 2026-11-26): advise가 선행·겹침 앞 레인의 단계를 대기열 전체(주인·상태 상관없이)에서,
// 대기열에 없으면 이 기계 그 프로젝트 저장소 기본 가지의 착지(fetch 없이 — 바퀴가 fetch한 그대로)에서 찾게 opsstate.Facts.Pred를 채운다.
// 전에는 추린 떠 있는 레인에서만 찾아, 착지해 빠진 선행·대기 줄이 「단계 모름(떠 있는 레인에 없음)」으로 보였다

import (
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/opsstate"
)

// predQueued: 대기열 줄 하나의 단계를 「이름@프로젝트」 열쇠로(같은 이름의 다른 프로젝트 줄과 섞지 않는다 — 감사 atp306v 낮음 1).
// 같은 열쇠가 여럿이면 떠 있는(up) 줄이 이긴다
func predQueued(p map[string]opsstate.Lane, l opsstate.Lane, proj string, up bool) {
	k := opsstate.PredKey(l.Name, proj)
	if _, ok := p[k]; !ok || up {
		p[k] = l
	}
}

// predThen <대기열 줄>: --then 이어 붙인 단계 이름 → 받은 줄(run 바퀴의 THENMAP과 같은 읽기 — 지시문의 「## 이어서: <이름>」, 선행 판정
// overlap2.go는 그 이름이 대기열 줄이 아닐 때만 받은 줄로 읽는다). advise가 이어서 단계를 「대기열에 없음 → 풀 수 있음」으로 권했다(ht-advise-noise)
func predThen(rows []Row) map[string]string {
	in, m := map[string]bool{}, map[string]string{}
	for _, r := range rows {
		in[r.F(qName)] = true
	}
	for _, r := range rows {
		for _, n := range thenNames(r.F(qBrief)) {
			if n = strings.TrimSpace(n); n != "" && !in[n] {
				if _, ok := m[n]; !ok { // thenof처럼 대기열 차례로 첫 줄
					m[n] = r.F(qName)
				}
			}
		}
	}
	return m
}

// predWaitLane: 대기·되돌려줌 줄의 단계(나이 = 넣은 뒤)
func predWaitLane(r Row, now time.Time) opsstate.Lane {
	pin, _ := pinOf(r.F(qName), r.F(qRID))
	l := opsstate.Lane{Name: r.F(qName), Stage: stStage(stIn{State: r.F(qState), Early: r.F(qEarly), Lane: r.F(qName), Pin: pin})}
	if a, ok := stateAddedAge(r.F(qAdded), now); ok {
		l.StageAgeS = ptrI(a)
	}
	return l
}

// predLanded: 대기열에 없거나 대기열 줄이 끝난(착지 표시 없이 「끝남」 등 — 착지 뒤 정리됨으로 끝난 up 줄이 「끝남」으로 보였다, 감사
// atp306v 중간 1) 선행·겹침 앞 레인 → hc-fill ls와 같은 출처(afterIn — 제목 머리 커밋·착지 표식)로 기본 가지에 들어갔으면
// 「착지 <sha8>」(나이 = 커밋 뒤)로 덮는다. 대기열에 없는데 저장소 줄·기본 가지·git을 못 읽으면 「대기열에 없음(착지 확인 못 함)」,
// 착지 없으면 그대로(대기열 줄 단계, 없으면 advise가 「대기열에 없음」)
func predLanded(f *opsstate.Facts, now time.Time) {
	look := func(a, p string) {
		n, _ := afterAt(a, p)
		k := opsstate.PredKey(a, p) // 다른 프로젝트 꼴(이름@프로젝트)은 그 꼴 그대로 — afterIn도 그 프로젝트 저장소에서 찾는다
		q, inQ := f.Pred[k]
		if n == "" || inQ && !(opsstate.Finished(q) && !strings.HasPrefix(q.Stage, "착지")) {
			return
		}
		ofr, ofb := FR, FB // 바퀴가 쓰는 저장소·가지는 그대로
		sha, ok := afterIn(p, a)
		repo := FR
		FR, FB = ofr, ofb
		switch {
		case !ok && !inQ:
			f.Pred[k] = opsstate.Lane{Name: n, Stage: "대기열에 없음(착지 확인 못 함)"}
		case ok && sha != "":
			l := opsstate.Lane{Name: n, Machine: q.Machine, Stage: "착지 " + sha, Done: true} // %h는 7자 — 아래에서 sha8로
			if o, rc := run("git", "-C", repo, "log", "-1", "--format=%H %ct", sha); rc == 0 {
				full, ts, _ := strings.Cut(strings.TrimSpace(o), " ")
				l.Stage = "착지 " + head(full, 8)
				if ct, err := strconv.ParseInt(ts, 10, 64); err == nil {
					l.StageAgeS = ptrI(int(now.Unix() - ct))
				}
			}
			f.Pred[k] = l
		}
	}
	for _, w := range f.Waits {
		for _, a := range w.After {
			look(a, w.Project)
		}
		if w.Front != "" {
			look(w.Front, w.Project)
		}
	}
}
