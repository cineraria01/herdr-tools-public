// alertroute.go: 주인 없는·주인 사라진 사건을 밀어 넣을 지휘 고르기 — wfilter가 repoSend 대신 부르는 alertSend 한 곳
// (ht-alert-route, docs/hc-fill/alert-route.md). 손잡이 alert_route all(기본)이면 repoSend 그대로
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/events"
	"herdr-tools/internal/tri"
)

type repPick struct {
	addr string
	st   tri.State
}

// alertSend: 이 watch가 그 사건을 보내나. one이면 그 저장소(모르면 전역) 대표만 보내고, 대표를 못 정하면 오늘처럼 repoSend.
// 대표가 아닌 watch는 대표가 본(커서 seen.<대표>) 뒤에야 놓는다 — 대표 커서에 없으면 repoSend처럼 유예(rgrace) 뒤 보낸다(아무도 안 받음은 없다).
// 막힘은 대표 커서에 적힌 뒤 alert_escalate_min분 동안 대표 턴(repSaw)·풀림(같은 레인 뒤 사건이 막힘 아님)이 없으면 한 번 보낸다(켜기 전 사건은 빼고)
func (e *ev) alertSend(l string) bool {
	if v, _ := e.knob("alert_route"); v != "one" {
		os.Unsetenv(oneEnv) // 다시 켜면 그때부터 센다
		return e.repoSend(l)
	}
	oneSince()
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
	if n <= 0 || err != nil || at.Before(oneSince()) || e.resolved(x, at) {
		e.appendCur(k)
		return false
	}
	if time.Since(at) < time.Duration(n)*time.Minute {
		return false
	}
	if repSaw(p.addr, at) {
		e.appendCur(k)
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

// repSaw: 대표가 그 사건 뒤 턴을 돌렸나 — hc get의 에이전트 상태가 사건 뒤 바뀌었고(status_ms) 지금 막힘·오류가 아님. 놓침 막기가 막는 것은
// 「대표가 못 봄」이지 「보고 손 안 댐」이 아니다(ht-alert-stale-escalate — 일지 밖 처리·손댈 것 없음이 30분 뒤 모두에게 갔다). 못 읽으면 아님 — 올려 보내는 쪽으로.
// shortcut: 기준이 보낸 때가 아니라 사건 시각이라 사건과 보냄(한 바퀴·유예) 사이의 상태 바뀜도 봄으로 친다(그 뒤 붙여 넣은 글은 살아 있는 에이전트가 이어 읽는다) — 보낸 시각이 커서에 적히는 날 그것으로
func repSaw(rep string, at time.Time) bool {
	t, found, err := hcGet(10, rep)
	if err != nil || !found || t.State != "live" || t.Status != "idle" && t.Status != "working" && t.Status != "done" {
		return false
	}
	ms, err := strconv.ParseInt(string(t.Raw["status_ms"]), 10, 64)
	return err == nil && ms > at.UnixMilli()
}

// oneEnv: 이 watch가 alert_route one을 처음 본 때(유닉스 초) — 새 판 넘겨줌(selfexec, os.Environ 그대로 exec)에도 남게 프로세스 환경에 둔다
const oneEnv = "HC_LANE_ALERT_ONE_SINCE"

// oneSince: 기능이 켜진 때 — 그 앞 사건은 놓침 막기로 세지 않는다(옛 사건이 켜자마자 「n분 조치 없음」으로 올라갔다).
// shortcut: watch를 새로 띄우면(넘겨줌 아님) 그때부터 — 띄우기 전 막힘은 이 watch가 올려 보내지 않는다, 켠 때를 일지에서 읽게 되면 그것으로
func oneSince() time.Time {
	if s, err := strconv.ParseInt(os.Getenv(oneEnv), 10, 64); err == nil {
		return time.Unix(s, 0)
	}
	now := time.Now()
	os.Setenv(oneEnv, strconv.FormatInt(now.Unix(), 10)) // 열쇠가 상수라 실패 없음 — 실패해도 다음 바퀴에 다시 적을 뿐
	return now
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
