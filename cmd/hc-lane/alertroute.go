// alertroute.go: 주인 없는·주인 사라진 사건을 밀어 넣을 지휘 고르기 — wfilter가 repoSend 대신 부르는 alertSend 한 곳
// (ht-alert-route, docs/hc-fill/alert-route.md). 손잡이 alert_route all(기본)이면 repoSend 그대로
package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/events"
	"herdr-tools/internal/journal"
	"herdr-tools/internal/opsstate"
	"herdr-tools/internal/tri"
)

type repPick struct {
	addr string
	st   tri.State
}

// alertSend: 이 watch가 그 사건을 보내나. one이면 그 저장소(모르면 전역) 대표만 보내고, 대표를 못 정하면 오늘처럼 repoSend.
// 대표가 아닌 watch는 대표가 본(커서 seen.<대표>) 뒤에야 놓는다 — 대표 커서에 없으면 repoSend처럼 유예(rgrace) 뒤 보낸다(아무도 안 받음은 없다).
// 막힘은 대표가 본 뒤 alert_escalate_min분 동안 조치함(대표의 일지 줄)·풀림(같은 레인 뒤 사건이 막힘 아님)이 없으면 한 번 보낸다
func (e *ev) alertSend(l string) bool {
	if v, _ := e.knob("alert_route"); v != "one" {
		return e.repoSend(l)
	}
	r := e.evrepo(l)
	p := e.alertRep(r)
	if p.st != tri.Yes {
		return e.repoSend(l)
	}
	k := lineKey(l)
	if e.rtag == nil {
		e.rtag = map[string]string{}
	}
	if lastSeg(p.addr) == lastSeg(e.wto) {
		e.rtag[k] = " → 대표 지휘 " + p.addr
		return true
	}
	s, sr := e.seenOf(p.addr)
	if !s[k] {
		if sr == tri.Unknown {
			return true // 대표 커서를 못 읽음 — 받았는지 모르니 보낸다
		}
		t0, ok := e.rdefer[k]
		if !ok {
			t0 = time.Now()
			e.rdefer[k] = t0
		}
		if time.Since(t0) < e.rgrace {
			return false
		}
		delete(e.rdefer, k)
		e.rtag[k] = " → 모든 지휘(대표 " + p.addr + " 커서에 없음)"
		return true
	}
	delete(e.rdefer, k)
	x := events.Cols(l)
	if x.State != "막힘" {
		e.appendCur(k)
		return false
	}
	m, _ := e.knob("alert_escalate_min")
	n, _ := strconv.Atoi(m)
	at, err := time.Parse(time.RFC3339, x.Time)
	subs := []string{x.Lane}
	if r != "" {
		subs = append(subs, lastSeg(r))
	}
	if n <= 0 || err != nil || e.resolved(x, at) || acted(p.addr, at, subs) {
		e.appendCur(k)
		return false
	}
	if time.Since(at) < time.Duration(n)*time.Minute {
		return false
	}
	e.rtag[k] = fmt.Sprintf(" → 모든 지휘(대표 %s %d분 조치 없음)", p.addr, n)
	return true
}

// resolved: 이 바퀴 사건(문맥 포함)에 같은 레인의 그 뒤 사건이 막힘이 아니면 풀림
func (e *ev) resolved(x events.Event, at time.Time) bool {
	for _, w := range e.win {
		y := events.Cols(w)
		if y.Lane != x.Lane || y.State == "막힘" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, y.Time); err == nil && t.After(at) {
			return true
		}
	}
	return false
}

// acted: 대표가 그 사건 뒤 그 레인·프로젝트(subs)에 조치를 남겼나 — advise_claim의 「조치함」과 같은 재료(opsstate.ActedOn: 조치 동작,
// 실패·거절 아님). 못 읽으면 아님 — 올려 보내는 쪽으로
func acted(rep string, at time.Time, subs []string) bool {
	es, _, err := journal.Read(journal.Filter{Since: at})
	if err != nil {
		return false
	}
	for _, j := range es {
		if lastSeg(j.By) == lastSeg(rep) && opsstate.ActedOn(j, subs, at) {
			return true
		}
	}
	return false
}

// alertRep: 저장소 r(빈 값 = 전역)의 대표 지휘 — 바퀴마다 다시 고른다(e.arep, wfilter가 비움).
// 손잡이 alert_coord@<r 마지막 마디>(없으면 alert_coord)의 앞에서부터 살아 있는 것, 없으면 r을 맡는다고 적힌(전역이면 모든) 살아 있는
// 지휘 가운데 터미널이 가장 먼저 뜬 것(created_ms, 같으면 주소). 후보 하나라도 모름이면 모름 — 오늘처럼 모두에게
func (e *ev) alertRep(r string) repPick {
	if p, ok := e.arep[r]; ok {
		return p
	}
	p := e.pickRep(r)
	if e.arep == nil {
		e.arep = map[string]repPick{}
	}
	e.arep[r] = p
	return p
}

func (e *ev) pickRep(r string) repPick {
	kn := "alert_coord"
	if r != "" {
		kn += "@" + lastSeg(r)
	}
	if v, err := e.knob(kn); err == nil && v != "-" {
		for _, a := range strings.Split(v, ",") {
			switch e.coordOK(a) {
			case tri.Yes:
				return repPick{a, tri.Yes}
			case tri.Unknown:
				return repPick{"", tri.Unknown}
			case tri.No: // 죽은 지휘 — 다음 것
			}
		}
	}
	cfs, _ := filepath.Glob(e.ed + "/coord.*")
	type cand struct {
		addr string
		born int64
	}
	var cs []cand
	for _, cf := range cfs {
		if strings.Contains(strings.TrimPrefix(filepath.Base(cf), "coord."), ".") {
			continue // 쓰는 중인 coord.<주소>.<pid>
		}
		c, ok := readCoord(cf)
		if !ok {
			continue
		}
		if r != "" {
			if rs, known := coordRepos(c[0]); !known || !rs[r] {
				continue
			}
		}
		switch e.coordOK(c[0]) {
		case tri.No:
			continue
		case tri.Unknown:
			return repPick{"", tri.Unknown}
		case tri.Yes:
		}
		b, ok := terminalCreatedMs(c[0])
		if !ok {
			return repPick{"", tri.Unknown}
		}
		n, _ := strconv.ParseInt(b, 10, 64)
		cs = append(cs, cand{c[0], n})
	}
	if len(cs) == 0 {
		return repPick{"", tri.No}
	}
	sort.Slice(cs, func(i, j int) bool {
		return cs[i].born < cs[j].born || cs[i].born == cs[j].born && cs[i].addr < cs[j].addr
	})
	return repPick{cs[0].addr, tri.Yes}
}

// coordOK: 이 바퀴 지휘 확인(coordCheck) — repoSend와 같은 e.occ
func (e *ev) coordOK(a string) tri.State {
	if e.occ == nil {
		e.occ = map[string]tri.State{}
	}
	if r, ok := e.occ[lastSeg(a)]; ok {
		return r
	}
	_, _, r := coordCheck(a)
	e.occ[lastSeg(a)] = r
	return r
}
