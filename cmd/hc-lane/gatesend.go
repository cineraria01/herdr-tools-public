// gatesend.go: watch가 한 문(gate.go judge)을 부르는 자리 — 이 바퀴 재료(gworld)를 짓고, 보낼 줄을 고르고(gateSend), 밀어 넣지 않을·묵은
// 사건은 커서에만 적는다. 옛 wfilter·prevseen·alertSend·repoSend·resolved·repSaw(남의 커서와 이 watch의 유예 표로 정하던 갈래)를 접었다
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
	"herdr-tools/internal/proc"
	"herdr-tools/internal/tri"
)

// world: 이 바퀴 판정 재료 — 사건 창(e.win)·손잡이·지휘 확인(바퀴 캐시). 커서는 넣지 않는다.
// lines가 있으면 창에서 그 줄들의 레인(맨 앞 못 맡김은 그 맨 앞 레인도)의 사건만 푼다 — 판정은 같은 레인 사건만 보니(풀림·물려받기)
func (e *ev) world(lines []string) *gworld {
	if e.gfirst == nil {
		e.gfirst = map[string]time.Time{}
	}
	now := time.Now()
	for k, t := range e.gfirst {
		if now.Sub(t) > staleAge+time.Hour {
			delete(e.gfirst, k)
		}
	}
	w := &gworld{now: now, repo: e.lrepo, recv: e.recvOK, reps: e.repList, laneOwner: e.laneOwner,
		knob: func(n string) string { v, _ := e.knob(n); return v },
		first: func(k string) time.Time {
			if t, ok := e.gfirst[k]; ok {
				return t
			}
			e.gfirst[k] = now
			return now
		}}
	var need map[string]bool
	if lines != nil {
		need = map[string]bool{}
		for _, l := range lines {
			x := events.Cols(l)
			need[x.Lane] = true
			if m := reFront.FindStringSubmatch(x.Msg); m != nil {
				need[m[1]] = true
			}
		}
	}
	for _, l := range e.win {
		if need != nil && !need[laneField(l)] {
			continue
		}
		if x := events.Cols(l); x.N >= 7 {
			w.hist = append(w.hist, x)
		}
	}
	return w
}

// gate: 이 watch(--to)가 이 바퀴에 보낼 줄 — 판정은 e.vd, 받는 쪽 한 구절은 e.rtag, 묵은 사건의 시각 머리는 e.rpre.
// 밀어 넣지 않음·묵음(보내지 않는 것)은 커서에 적고, 이 지휘가 받는 쪽이던 묵은 사건은 watch.log에 한 줄
func (e *ev) gate(lines []string) []string {
	getMemo = map[string]getRes{}
	defer func() { getMemo = nil }()
	e.occ, e.rcv, e.reps, e.lown = map[string]tri.State{}, map[string]tri.State{}, map[string][]string{}, map[string]string{}
	e.vd, e.rtag, e.rpre = map[string]gverdict{}, map[string]string{}, map[string]string{}
	w := e.world(lines)
	me := lastSeg(e.wto)
	var out, cur, old []string
	for _, l := range lines {
		if l == "" {
			continue
		}
		x := events.Cols(l)
		if x.Lane == "hc-lane-wait" { // watch 스스로의 알림(hc ls 실패 등)
			out = append(out, l)
			continue
		}
		k := lineKey(l)
		v := w.judge(x)
		e.vd[k] = v
		if ok, tag := gateSend(v, me, w.now); ok {
			e.rtag[k] = tag
			if v.stale {
				e.rpre[k] = "[" + clock(v.at) + "] "
			}
			out = append(out, l)
			continue
		}
		if v.br == brMute || v.stale && !staleKeep(v, w.now) {
			cur = append(cur, k)
			if v.stale && v.br != brMute && v.to == me {
				over := ""
				if v.br == brNow && v.open {
					over = ", 상한 " + mins(staleMax) + " 넘음"
				}
				old = append(old, "안 보냄(묵음 "+mins(w.now.Sub(v.at))+over+"): "+digestCut(wpart(l), 160)+" — "+v.kind)
			}
		}
	}
	if len(cur) > 0 {
		e.appendCur(cur...)
	}
	// 묵은 것 기록: 바퀴에 셋까지 한 줄씩, 넘으면 한 줄로(옛 커서로 처음 도는 watch가 수천 줄을 남기지 않게)
	if len(old) > 3 {
		old = []string{strconv.Itoa(len(old)) + "건 " + old[0] + " 외"}
	}
	for _, l := range old {
		e.wlog(l)
	}
	return out
}

// waitSent: 지휘가 wait로 받은 사건 가운데 한 문이 그 지휘를 받는 쪽으로 정한 것은 공용 보냄 기록에도 적는다 — 「누가 받았나」는 sent
// 한 곳이라, 그 지휘가 끝난 뒤 다음 받는 쪽 watch가 다시 보내지 않는다(옛 prevseen이 옛 주인 커서로 하던 일). 남의 사건을 본 wait는 안 적는다
func (e *ev) waitSent(lines []string) {
	me := os.Getenv("HC_TERMINAL_ID")
	if me == "" || len(lines) == 0 {
		return
	}
	getMemo = map[string]getRes{}
	defer func() { getMemo = nil }()
	e.wto, e.occ, e.rcv, e.reps, e.lown = me, map[string]tri.State{}, map[string]tri.State{}, map[string][]string{}, map[string]string{}
	defer func() { e.wto = "" }()
	w := e.world(lines)
	var sk []string
	for _, l := range lines {
		if x := events.Cols(l); x.N >= 7 && x.Lane != "hc-lane-wait" {
			// 자기가 주인으로, 또는 주인 후보가 아예 없는 사건을 대표로 받은 것만 — 주인 watch가 잠깐 안 돌아 대표로 정해진 남의 사건은
			// 그 주인 watch 몫으로 남긴다
			if v := w.judge(x); !v.all && v.to == lastSeg(me) && (v.owner == v.to || !v.hadOwner) && v.br != brMute && v.br != brHold {
				sk = append(sk, lineKey(l))
			}
		}
	}
	if err := appendLines(e.ed+"/sent", sk...); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane wait: 보냄 표시를 쓰지 못했다: %s/sent\n", e.ed)
	}
}

// gateSend: 이 watch(me)가 그 판정의 사건을 지금 보내나와 글 끝 한 구절 — 순수.
// 받는 쪽이 나 · 받는 쪽 확인 못 한 판단 필요(모두) · 놓침 막기(받는 쪽이 nextAt까지 안 받았고 다음 사람이 나)만. 묵은 것은 staleKeep만
func gateSend(v gverdict, me string, now time.Time) (bool, string) {
	switch {
	case v.br == brMute || v.br == brHold:
		return false, ""
	case v.stale && !staleKeep(v, now):
		return false, ""
	case v.all, v.to == me:
		return true, v.tag
	case v.next == me && !v.stale && !now.Before(v.nextAt): // 묵은 것은 받는 쪽 한 명에게만(시각 붙여)
		return true, " → 놓침 막기(" + v.to + " " + mins(now.Sub(v.at)) + " 안 받음)"
	}
	return false, ""
}

// staleKeep: 묵은 사건의 예외 — 안 풀린 판단 필요이고 staleMax 안(origin 규칙에 나이 상한 하나 — 지휘 결정: 이미 판정한 사건은 커서에
// 있어 다시 문에 안 들어오니, 예외가 먹는 것은 늦게 들어온 사건(끊겼던 기계 — 가야 함)과 새 커서(상한이 막음)뿐이다)
func staleKeep(v gverdict, now time.Time) bool {
	return v.br == brNow && v.open && now.Sub(v.at) <= staleMax
}

// recvOK: 그 지휘가 지금 받을 수 있나 — 지휘 확인(coordCheck)·실행(run이 있으면 created_ms)·그 지휘의 watch가 돎(이 watch 자신은 늘).
// watch가 없는 지휘에게 정하면 아무도 안 밀어 넣으니 그 지휘는 건너뛴다(다음 사람 — 늦을 뿐 놓치지 않음). 바퀴마다 한 번(e.rcv)
func (e *ev) recvOK(addr, run string) tri.State {
	a := lastSeg(addr)
	k := a + "@" + run
	if r, ok := e.rcv[k]; ok {
		return r
	}
	r := tri.Yes // 이 watch 자신은 바퀴 앞 지휘 확인(wcheck)을 지났다
	if a != lastSeg(e.wto) {
		r = e.coordOK(a)
	}
	if r == tri.Yes && run != "" {
		if c, ok := liveRun(a); !ok {
			r = tri.Unknown
		} else if c != run {
			r = tri.No
		}
	}
	if r == tri.Yes && a != lastSeg(e.wto) && !watchUp(e.ed, a) {
		r = tri.No
	}
	if e.rcv == nil {
		e.rcv = map[string]tri.State{}
	}
	e.rcv[k] = r
	return r
}

// liveRun: 살아 있는 터미널의 지금 실행(created_ms) — hcGet(바퀴 안에서는 getMemo로 지휘 확인과 한 번), 모르면 false
func liveRun(a string) (string, bool) {
	t, found, err := hcGet(10, a)
	if err != nil || !found || t.State != "live" || t.Created <= 0 {
		return "", false
	}
	return strconv.FormatInt(t.Created, 10), true
}

// watchUp: 그 지휘의 watch가 이 기계에서 도나(잠금 폴더 watch.<주소>의 pid가 hc-lane watch) — 시험이 바꿔 끼운다
// shortcut: 한 번 hc-lane watch로 확인한 잠금 pid는 살아 있는 동안(kill 0) ps 없이 그대로 친다 — 그 pid가 끝나고 다른 프로세스에 다시
// 쓰이면 한 바퀴 틀릴 수 있다(잠금 pid가 바뀌거나 그 pid가 끝나면 다시 ps), 잦아지면 pid 시작 시각까지 견준다
var watchUp = func(ed, a string) bool {
	lk := ed + "/watch." + ckey(a)
	if b, err := os.ReadFile(lk + "/pid"); err == nil {
		if p := strings.TrimSpace(string(b)); p != "" && wupPid[lk] == p && digits(p) && proc.Alive(int(atoi(p))) {
			return true
		}
	}
	op, ok := watchAlive(lk)
	if ok {
		wupPid[lk] = op
	} else {
		delete(wupPid, lk)
	}
	return ok
}

var wupPid = map[string]string{} // 잠금 폴더 → 확인한 hc-lane watch pid(watchUp)

// coordOK: 이 바퀴 지휘 확인(coordCheck) — e.occ
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

// repList: 저장소 r(빈 값 = 전역)의 대표 후보 차례 — 손잡이 alert_coord@<r 마지막 마디>(없으면 alert_coord)의 차례, 그다음 r을 맡는다고
// 적힌(전역이면 모든) 지휘를 터미널이 먼저 뜬 차례(created_ms, 같으면 주소, 모르면 뒤로). 받을 수 없음(recvOK 없음)은 빼고 모름은 넣는다
func (e *ev) repList(r string) []string {
	if l, ok := e.reps[r]; ok { // 바퀴마다 저장소 하나에 한 번(지휘마다 hc get — 사건마다 부르면 바퀴가 초 단위가 된다)
		return l
	}
	if e.reps == nil {
		e.reps = map[string][]string{}
	}
	l := e.repListNow(r)
	e.reps[r] = l
	return l
}

func (e *ev) repListNow(r string) []string {
	var out []string
	kn := "alert_coord"
	if r != "" {
		kn += "@" + lastSeg(r)
	}
	if v, err := e.knob(kn); err == nil && v != "-" {
		for _, a := range strings.Split(v, ",") {
			if a = strings.TrimSpace(a); a != "" && e.recvOK(a, "") != tri.No && !has(out, lastSeg(a)) {
				out = append(out, lastSeg(a))
			}
		}
	}
	type born struct {
		addr string
		ms   int64
	}
	var cs []born
	cfs, _ := filepath.Glob(e.ed + "/coord.*")
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
		if e.recvOK(c[0], "") == tri.No {
			continue
		}
		n, ok := e.born[c[0]+tab+c[1]] // 뜬 시각은 같은 pid 동안 그대로 — 바퀴를 넘어 기억(지휘마다 hc get 한 번 덜)
		if !ok {
			n = int64(1 << 62)
			if b, live := liveRun(c[0]); live {
				n, _ = strconv.ParseInt(b, 10, 64)
				if e.born == nil {
					e.born = map[string]int64{}
				}
				e.born[c[0]+tab+c[1]] = n
			}
		}
		cs = append(cs, born{lastSeg(c[0]), n})
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].ms < cs[j].ms || cs[i].ms == cs[j].ms && cs[i].addr < cs[j].addr })
	for _, c := range cs {
		if !has(out, c.addr) {
			out = append(out, c.addr)
		}
	}
	return out
}

// laneOwner: 11칸이 없는 사건의 주인 — 레인 기록에 그 터미널 주인이 둘 넘게 적혔으면 모름(-), 아니면 owner(레인 기록·hc parent 사슬)
func (e *ev) laneOwner(term string) string {
	if o, ok := e.lown[term]; ok { // 바퀴마다 터미널 하나에 한 번(레인 기록 파일을 사건마다 읽지 않게)
		return o
	}
	if e.lown == nil {
		e.lown = map[string]string{}
	}
	o := e.laneOwnerNow(term)
	e.lown[term] = o
	return o
}

func (e *ev) laneOwnerNow(term string) string {
	own := map[string]bool{}
	ls, _ := readLines(e.ed + "/lanes.tsv")
	for _, r := range ls {
		g := strings.Split(r, tab)
		if g[0] == term && at(g, 2) != "-" && at(g, 2) != "" {
			own[g[2]] = true
		}
	}
	if len(own) > 1 {
		return "-"
	}
	return e.owner(term)
}

// laneRepos: 사건들에서 레인 이름 → 13칸 저장소(둘 이상이면 "" — 모름)
func laneRepos(ls []string) map[string]string {
	m := map[string]string{}
	for _, l := range ls {
		x := events.Cols(l)
		ln, r := x.Lane, x.Repo
		if ln == "" || ln == "-" || !repoIDok(r) {
			continue
		}
		if p, ok := m[ln]; ok && p != r {
			r = ""
		}
		m[ln] = r
	}
	return m
}
