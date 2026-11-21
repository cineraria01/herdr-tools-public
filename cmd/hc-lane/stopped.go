// stopped.go: wait·watch의 멈춘 레인 판정(셸 판 lib/hc-lane/report.sh stopped·resting·psget·wtbusy·procbusy·pnorm·plive·palive·mine)
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// round: 한 바퀴(stopped 한 번) 동안의 ps·hc get 기억
type round struct {
	e     *ev
	ps    map[string][][3]string // 기계 → (pid, ppid, command)
	psbad map[string]bool
	pl    map[string]int
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
		f := strings.Split(l, tab)
		if fld(f, 3) != a || fld(f, 4) != nm {
			continue
		}
		if f[4] == "정리됨" {
			sw = true
			continue
		}
		r.rep(f[4], f[5], turnOf(f), "x")
		have, sw = true, false
	}
	if !have {
		return "", "0", sw
	}
	return r.lst["x"], r.ltn["x"], sw
}

// turnAfter: report turn+1보다 뒤인가. 둘 중 하나라도 숫자가 아니면 unknown
func turnAfter(lt, tn string) (after, unknown bool) {
	if lt+tn == "" || !digits(lt+tn) {
		return false, true
	}
	return atoi(tn) > atoi(lt)+1, false
}

func (e *ev) stopped(evl, mu []string) []string {
	if !e.lsok {
		fmt.Fprintln(os.Stderr, "hc-lane wait: hc ls 실패 — 이번 바퀴는 멈춘 레인을 못 봤다")
		return []string{"#hc-ls-fail"}
	}
	rd := &round{e: e, ps: map[string][][3]string{}, psbad: map[string]bool{}, pl: map[string]int{}}
	me := machineName()
	km := e.lkinds()
	var out []string
	for _, r := range jlist(e.tl, "terminals", "address", "name", "state", "status", "turn", "parent", "reason", "cwd") {
		a, nm, s, ss, tn, pa, rs, cw := r[0], r[1], r[2], r[3], r[4], r[5], r[6], r[7]
		if !e.mineOf(pa) {
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
		why := ""
		fin := " 착지 판정 제출 주차 "
		if k := km[[2]string{lastSeg(a), nm}]; k == "read" || k == "fill" {
			fin = " 착지 판정 제출 주차 완료 "
		}
		if s != "live" {
			if !sw {
				if strings.Contains(fin, " "+last+" ") && last != "" {
					if after, unk := turnAfter(lt, tn); unk || after {
						why = "뜻밖의 종료(" + s + ")"
					}
				} else {
					why = "뜻밖의 종료(" + s + ")"
				}
			}
			// killed가 지휘의 hc kill인지: parent가 없거나 아직 살아 있을 때만 조용히 넘긴다
			kq := false
			if why != "" && rs == "killed" {
				kq = pa == "" || pa == "-" || rd.palive(pa) == 0
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
		} else {
			switch ss {
			case "blocked", "error":
				why = ss
			case "done", "idle":
				switch last {
				case "완료", "착지", "막힘", "판정":
					if after, unk := turnAfter(lt, tn); !unk && after {
						why = ss
					}
				default:
					why = ss
				}
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

// procbusy: 그 pid의 자손 가운데 명령 줄에 lane.wait_procs 글자가 든 것이 살아 있나. 0 있음, 1 없음, 2 모름
func (rd *round) procbusy(a, pid string) int {
	pm, ok := rd.psget(a)
	if !ok {
		return 2
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
				return 0
			}
			q = par[q]
		}
	}
	return 1
}

// resting: 쉬는 레인이 정말 멈췄나(유예·배경 작업·화면 표시)
func (rd *round) resting(a, k, cw string) bool {
	e := rd.e
	g, _ := hcRun(10, errDrop, "get", a, "--json")
	m := jparse(g)
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
	if e.wprocs != "" && tp != "" && rd.procbusy(a, tp) == 0 {
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
		if rc == 0 && strings.Contains(mj, `"machines"`) {
			var ns []string
			for _, r := range jlist(mj, "machines", "name", "state") {
				ns = append(ns, r[0])
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

// plive: 그 터미널이 지금 살아 있나. 0 살아 있음, 3 그 주소의 기계를 모름(machine_unknown), 1 그 밖
func (rd *round) plive(a string) int {
	if r, ok := rd.pl[a]; ok {
		return r
	}
	g, _ := hcRun(10, errMerge, "get", a, "--json")
	r := 1
	if m := jparse(g); m != nil && m["state"] == "live" {
		r = 0
	} else if strings.Contains(g, "machine_unknown") {
		r = 3
	}
	rd.pl[a] = r
	return r
}

// palive: parent를 그대로 먼저 묻고, machine_unknown일 때만 앞부분을 떼고 다시
func (rd *round) palive(pa string) int {
	r := rd.plive(pa)
	if r != 3 {
		return r
	}
	q := rd.pnorm(pa)
	if q == pa {
		return 1
	}
	return rd.plive(q)
}
