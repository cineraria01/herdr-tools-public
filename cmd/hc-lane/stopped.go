// stopped.go: wait·watch의 멈춘 레인 판정(셸 판 lib/hc-lane/report.sh stopped·resting·psget·wtbusy·procbusy·pnorm·plive·palive·mine)
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"herdr-tools/internal/events"
	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/tri"
)

// round: 한 바퀴(stopped 한 번) 동안의 ps·hc get 기억
type round struct {
	e     *ev
	ps    map[string][][3]string // 기계 → (pid, ppid, command)
	psbad map[string]bool
	pl    map[string]pstate
	mls   string // " 이름 … " 또는 fail, 빈 값 = 아직
}

// mineOf: 이 지휘의 레인인가(거르지 않으면 늘 참, parent 모르면 참)
func (e *ev) mineOf(pa string) bool {
	if e.myp == "" {
		return true
	}
	return pa == "" || pa == "-" || pa == e.myp || strings.HasSuffix(pa, "/"+e.myp)
}

// lastIn: 그 터미널(주소·레인 이름)의 마지막 report 상태·turn과, 그 뒤에 정리됨이 있었나
func lastIn(evl []string, a, nm string) (last, lt string, sw bool) {
	r := newLastRep()
	have := false
	for _, l := range evl {
		x := events.Cols(l)
		if x.Term != a || x.Lane != nm {
			continue
		}
		if x.State == "정리됨" {
			sw = true
			continue
		}
		r.rep(x.State, x.SHA, turnOf(x), "x")
		have, sw = true, false
	}
	if !have {
		return "", "0", sw
	}
	return r.lst["x"], r.ltn["x"], sw
}

// endIn: 그 터미널(주소·레인 이름)의 중간을 뺀 마지막 report 상태(없으면 "")
func endIn(evl []string, a, nm string) string {
	end := ""
	for _, l := range evl {
		if x := events.Cols(l); x.Term == a && x.Lane == nm && x.State != "정리됨" && x.State != "멈춤" && x.State != "중간" {
			end = x.State
		}
	}
	return end
}

// turnAfter: report turn+1보다 뒤인가. 둘 중 하나라도 숫자가 아니면 unknown
func turnAfter(lt, tn string) (after, unknown bool) {
	if lt+tn == "" || !digits(lt+tn) {
		return false, true
	}
	return atoi(tn) > atoi(lt)+1, false
}

// handedOff: 마지막 report가 제출(병합기 큐에 넣음)·주차(원격 lane/<레인>에 두고 hc-fill이 다시 띄움)면 그 터미널은 할 일이 없다 —
// 닫히거나(hc-sweep·끝남) 차례를 끝내 쉬어도 지휘에 알릴 것이 없다(결과는 병합기·hc-fill이 따로 알린다). 2026-11-22 go-alert-quiet
func handedOff(last string) bool { return last == "제출" || last == "주차" }

// stopWhy: 멈춤 까닭(빈 값 = 알릴 것 없음). s = 터미널 state, ss = status, tn = 지금 turn, last·lt = 마지막 report 상태·turn,
// sw = 마지막 report 뒤 정리됨이 있었나(그 사건이 따로 간다), rd = 읽기·채움 레인(완료가 끝)
func stopWhy(s, ss, tn, last, lt string, sw, rd bool) string {
	if s != "live" {
		if sw || handedOff(last) {
			return ""
		}
		fin := last == "착지" || last == "판정" || rd && last == "완료"
		if after, unk := turnAfter(lt, tn); fin && !unk && !after {
			return ""
		}
		return "뜻밖의 종료(" + s + ")"
	}
	switch ss {
	case "blocked", "error":
		return ss
	case "done", "idle":
		switch {
		case handedOff(last):
			return ""
		case last == "완료" || last == "착지" || last == "막힘" || last == "판정":
			if after, unk := turnAfter(lt, tn); unk || !after {
				return ""
			}
		}
		return ss
	}
	return ""
}

func (e *ev) stopped(evl, mu []string) []string {
	if !e.lsok {
		fmt.Fprintln(os.Stderr, "hc-lane wait: hc ls 실패 — 이번 바퀴는 멈춘 레인을 못 봤다")
		return []string{"#hc-ls-fail"}
	}
	rd := &round{e: e, ps: map[string][][3]string{}, psbad: map[string]bool{}, pl: map[string]pstate{}}
	me := machineName()
	km := e.lkinds()
	var out []string
	for _, t := range e.tls.Terminals { // lsok — hcjson.ParseList로 통째 검증된 목록(watch.go lsLanes)
		if t.Unknown != "" { // 모름 항목(R-3M01) — 살아 있고 무엇을 하는지 모른다: 멈춤으로 알리지 않는다
			continue
		}
		a, nm, s, ss, tn, pa, rs, cw := t.Address, orHyph(t.Name), t.State, orHyph(t.Status), rawCell(t, "turn"), orHyph(t.Parent), rawCell(t, "reason"), orHyph(t.Cwd)
		lk, rec := km[[2]string{lastSeg(a), nm}]
		if !rec || !e.mineOf(pa) { // 레인 기록에 없는 터미널은 레인이 아니다(watch.go lkinds)
			continue
		}
		uk := false
		mn := me
		if i := strings.Index(a, "/"); i >= 0 {
			mn = a[:i]
			if !e.all && !has(e.ons, mn) {
				continue
			}
			uk = e.pbad || has(mu, mn)
		} else if mn == "" {
			mn = "-"
		}
		last, lt, sw := lastIn(evl, a, nm)
		why := stopWhy(s, ss, tn, last, lt, sw, lk == "read" || lk == "fill")
		if s != "live" && endQuiet(endIn(evl, a, nm)) { // 끝 보고·넘김·알린 막힘 뒤 사라짐은 알림이 아니다(한 문 — closeName과 같은 규칙)
			why = ""
		}
		if s != "live" {
			// killed가 지휘의 hc kill인지: parent가 없거나 아직 살아 있을 때만 조용히 넘긴다
			kq := false
			if why != "" && rs == "killed" {
				// 지휘가 살아 있음이 확인될 때만 조용히(그 지휘의 hc kill) — 모름·기계 모름은 알린다(숨기지 않는 쪽)
				kq = pa == "" || pa == "-" || rd.palive(pa) == pLive
			}
			if kq {
				k := "멈춤" + tab + lastSeg(a) + tab + flat(nm) + tab + why + " turn " + tn
				if !inseen(k, e.cur, e.wsf) {
					e.appendCur(k)
					if e.wsf != "" {
						if err := appendLines(e.wsf, k); err != nil {
							fmt.Fprintf(os.Stderr, "hc-lane %s: 보냄 표시를 쓰지 못했다: %s\n", e.ec, e.wsf)
						}
					}
				}
				why = ""
			}
		}
		if why == "" {
			continue
		}
		if uk && why != "blocked" && why != "error" {
			why = "연결 안 됨 " + mn + " " + time.Now().UTC().Format("2006-01-02")
		}
		if why == "done" || why == "idle" {
			k := "멈춤" + tab + lastSeg(a) + tab + flat(nm) + tab + "쉼 turn " + tn
			if inseen(k, e.cur, e.wsf) {
				continue
			}
			if !(e.wgrace == 0 && !e.wbusyOn && e.wprocs == "") && !rd.resting(a, k, cw) {
				continue
			}
		}
		out = append(out, strings.Join([]string{utcNow(), mn, a, flat(nm), "멈춤", "-", why + " turn " + tn}, tab))
	}
	return out
}

// psget: 그 주소 기계의 ps를 바퀴에 한 번. 참 = 있음, 거짓 = 모름(ps 실패)
func (rd *round) psget(a string) (string, bool) {
	pm := "."
	if i := strings.Index(a, "/"); i >= 0 {
		pm = a[:i]
	}
	if rd.psbad[pm] {
		return pm, false
	}
	if _, ok := rd.ps[pm]; ok {
		return pm, true
	}
	var o string
	var rc int
	if pm == "." {
		o, rc = capt(20, errDrop, "ps", "-A", "-o", "pid=,ppid=,command=")
	} else {
		o, rc = rexec(pm, "ps", "-A", "-o", "pid=,ppid=,command=")
	}
	if rc == 9 { // ps를 못 띄우는 기계(nosh): 레인 프로세스가 없는 것으로
		rd.ps[pm] = nil
		return pm, true
	}
	if rc != 0 || o == "" {
		rd.psbad[pm] = true
		return pm, false
	}
	var rows [][3]string
	for _, l := range strings.Split(o, "\n") {
		f := strings.Fields(l)
		if len(f) < 2 {
			rows = append(rows, [3]string{})
			continue
		}
		rows = append(rows, [3]string{f[0], f[1], psCommand(l)})
	}
	rd.ps[pm] = rows
	return pm, true
}

var rePsHead = regexp.MustCompile(`^[ \t]*[0-9]+[ \t]+[0-9]+[ \t]*`)

func psCommand(l string) string { return rePsHead.ReplaceAllString(l, "") }

var reWt = regexp.MustCompile(`^/.*/.+\.wt/.+$`)

// wtbusy: 레인 작업 트리 경로가 명령 줄에 든 프로세스가 그 기계에 살아 있나
func (rd *round) wtbusy(a, cw string) bool {
	if !reWt.MatchString(cw) {
		return false
	}
	pm, ok := rd.psget(a)
	if !ok {
		return false
	}
	c := strings.TrimSuffix(cw, "/")
	for _, p := range rd.ps[pm] {
		cmd := p[2] + " "
		if p[0] != "" && (strings.Contains(cmd, c+"/") || strings.Contains(cmd, c+" ")) {
			return true
		}
	}
	return false
}

// procbusy: 그 pid의 자손 가운데 명령 줄에 lane.wait_procs 글자가 든 것이 살아 있나. 있음·없음·모름(ps 실패)
func (rd *round) procbusy(a, pid string) tri.State {
	pm, ok := rd.psget(a)
	if !ok {
		return tri.Unknown
	}
	par := map[string]string{}
	cmd := map[string]string{}
	for _, p := range rd.ps[pm] {
		if p[0] == "" {
			continue
		}
		par[p[0]], cmd[p[0]] = p[1], p[2]
	}
	ws := strings.Fields(rd.e.wprocs)
	for p, c := range cmd {
		hit := false
		for _, w := range ws {
			if strings.Contains(c, w) {
				hit = true
			}
		}
		if !hit {
			continue
		}
		q := par[p]
		for k := 0; q != "" && atoi(q) > 1 && k < 10000; k++ {
			if q == pid {
				return tri.Yes
			}
			q = par[q]
		}
	}
	return tri.No
}

// resting: 쉬는 레인이 정말 멈췄나(유예·배경 작업·화면 표시)
func (rd *round) resting(a, k, cw string) bool {
	e := rd.e
	g, rc := hcGetRaw(10, a)
	var m map[string]any
	if _, found, err := hcjson.ParseGet([]byte(g), rc, a); err == nil && found {
		m = jparse(g) // ParseGet이 통째 검증한 응답에서만 칸을 꺼낸다
	}
	// 조회를 못 하면(status_ms 모름) 바로 멈춤으로 알린다 — 알림을 숨기지 않는 쪽(지휘가 본다)
	sm := topNum(m, "status_ms", "")
	if sm == "" {
		return true
	}
	r := ""
	cl, _ := readLines(e.cur)
	for _, l := range cl {
		f := strings.Split(l, tab)
		if f[0] == "#rest" && len(f) >= 3 && nk(strings.Join(f[1:len(f)-1], tab)) == k {
			r = f[len(f)-1]
		}
	}
	if !digits(r) {
		e.appendCur("#rest" + tab + k + tab + sm)
		r = sm
	}
	if atoi(sm) < atoi(r) {
		r = sm
	}
	ag := time.Now().Unix() - atoi(r)/1000
	if ag < e.wgrace*60 {
		return false
	}
	if ag >= e.wbmax*60 {
		return true
	}
	if e.wprocs != "" && rd.wtbusy(a, cw) {
		return false
	}
	tp := topNum(m, "pid", "foreground")
	if tp == "0" || tp == "1" {
		tp = ""
	}
	if e.wprocs != "" && tp != "" && rd.procbusy(a, tp) == tri.Yes {
		return false
	}
	if !e.wbusyOn {
		return true
	}
	scr, _ := hcRun(10, errDrop, "read", a)
	lastHit := ""
	for _, l := range strings.Split(scr, "\n") {
		if e.wline.MatchString(l) || e.wbusy.MatchString(l) {
			lastHit = l
		}
	}
	return !(lastHit != "" && e.wbusy.MatchString(lastHit))
}

// pnorm: 「<기계>/t…」의 앞부분이 hc machine ls에 없는 이름(= 이 기계 자신)이면 뗀 주소
func (rd *round) pnorm(pa string) string {
	i := strings.Index(pa, "/")
	if i < 0 {
		return pa
	}
	if rd.mls == "" {
		mj, rc := hcRun(20, errDrop, "machine", "ls", "--json")
		if ms, err := hcjson.ParseMachines([]byte(mj)); rc == 0 && err == nil {
			var ns []string
			for _, m := range ms {
				ns = append(ns, m.Name)
			}
			rd.mls = " " + strings.Join(ns, " ") + " "
		} else {
			rd.mls = "fail"
		}
	}
	if rd.mls != "fail" && !strings.Contains(rd.mls, " "+pa[:i]+" ") {
		return pa[i+1:]
	}
	return pa
}

// pstate: plive 판정 — 0값이 모름
type pstate int

const (
	pUnknown   pstate = iota // 모름(조회 실패·잘린 응답)
	pLive                    // 살아 있음
	pGone                    // 확인된 없음·끝남
	pNoMachine               // 그 주소의 기계를 모름(machine_unknown) — palive가 앞부분을 떼고 다시 묻는다
)

// plive: 그 터미널이 지금 살아 있나(hcjson.ParseGet)
func (rd *round) plive(a string) pstate {
	if r, ok := rd.pl[a]; ok {
		return r
	}
	g, rc := hcGetRaw(10, a)
	t, found, err := hcjson.ParseGet([]byte(g), rc, a)
	r := pUnknown
	switch {
	case err == nil:
		switch getState(t, found, nil) {
		case tri.Yes:
			r = pLive
		case tri.No:
			r = pGone
		case tri.Unknown:
		}
	case strings.Contains(g, `"machine_unknown"`):
		r = pNoMachine
	}
	rd.pl[a] = r
	return r
}

// palive: parent를 그대로 먼저 묻고, machine_unknown일 때만 앞부분을 떼고 다시
func (rd *round) palive(pa string) pstate {
	r := rd.plive(pa)
	if r != pNoMachine {
		return r
	}
	q := rd.pnorm(pa)
	if q == pa {
		return pUnknown // 기계 목록으로 떼지 못했다 — 그 지휘가 사는지 모른다
	}
	return rd.plive(q)
}
