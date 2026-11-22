// watch.go: hc-lane events·wait·watch — 레인 사건 읽기·기다리기·지휘에 밀어 주기
// (셸 판 lib/hc-lane/report.sh ev_cmd·pick·evread·inseen, lib/hc-lane/watch.sh emit·wfilter·prevseen·owner·evowner·wsend·wheld·
// wdialog·wcheck·watch_lock·fillq와 같은 꼴: 같은 커서·sent·잠금 폴더·지휘 정보 파일·출력·종료 코드)
package main

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func init() {
	for _, c := range []string{"events", "wait", "watch"} {
		c := c
		register(c, thenExit(func(a []string) int { return evCmd(c, a) }), noHelp)
	}
}

// ev: 한 번의 events·wait·watch 실행 상태(셸 판의 전역 변수들)
type ev struct {
	ec, ed       string
	all          bool
	ons          []string
	since, sts   string
	mine         bool
	wto, cur     string
	wsf          string
	myp          string
	ems, unk     []string
	pbad         bool
	wgrace       int64
	wbmax        int64
	wbusy, wline *regexp.Regexp
	wbusyOn      bool
	wprocs       string
	wdlg         *regexp.Regexp
	whmax        int64
	wh0          int64
	whLogged     bool
	wpar         bool
	tl           string
	lsok         bool
	pmap         [][]string
	lk           string
}

func evCmd(ec string, a []string) int {
	e := &ev{ec: ec, ed: evdir()}
	to := "110"
	for i := 0; i < len(a); i++ {
		switch a[i] {
		case "--all":
			e.all = true
		case "--on":
			v := val(a, i)
			if v == "" || strings.ContainsAny(v, "/ ") {
				die("--on 기계 이름이 이상하다: %s", v)
			}
			e.ons = append(e.ons, v)
			i++
		case "--since":
			e.since = val(a, i)
			i++
		case "--timeout":
			if ec != "wait" {
				die("--timeout은 wait에서만 쓴다")
			}
			to = val(a, i)
			i++
		case "--state":
			if ec == "events" {
				die("--state는 wait·watch에서만 쓴다")
			}
			e.sts = val(a, i)
			i++
		case "--mine":
			if ec == "events" {
				die("--mine은 wait·watch에서만 쓴다")
			}
			e.mine = true
		case "--to":
			if ec != "watch" {
				die("--to는 watch에서만 쓴다")
			}
			e.wto = val(a, i)
			i++
		default:
			die("알 수 없는 인자: %s", a[i])
		}
	}
	if !digits(to) {
		die("--timeout은 분(0 이상 정수): %s", to)
	}
	if ec == "watch" && (e.wto == "" || strings.HasPrefix(e.wto, "-") || !onlyChars(e.wto, alnum+"_./-")) {
		die("watch: --to <지휘 터미널 주소>가 없거나 이상하다: %s", e.wto)
	}
	for _, x := range strings.Fields(strings.ReplaceAll(e.sts, ",", " ")) {
		if !inWords(evstates+" 멈춤", x) {
			die("--state는 %s 멈춤 가운데: %s", evstates, x)
		}
	}
	e.since = sinceArg(e.since)
	if ec == "wait" && stdoutIsNull() {
		fmt.Fprintln(os.Stderr, "hc-lane wait: 출력이 /dev/null이라 사건을 읽을 사람이 없습니다 — seen에 적지 않고 끝냅니다. 백그라운드로 기다리려면 출력을 파일로 받으세요")
		return 2
	}
	if ec == "events" {
		e.pick()
		var tl, mu []string
		for _, l := range e.evread(0) {
			if strings.HasPrefix(l, "모름 ") {
				mu = append(mu, l)
			} else if strings.Contains(l, tab) && strings.SplitN(l, tab, 2)[0] >= e.since {
				tl = append(tl, l)
			}
		}
		sortByTime(tl)
		for _, l := range append(tl, mu...) {
			fmt.Println(l)
		}
		return 0
	}
	return e.loop(atoi(to))
}

// sinceArg: --since 시각 그대로, 또는 숫자에 s·m·h·d(없으면 시간) → 그만큼 전의 시각
func sinceArg(s string) string {
	if s == "" || !onlyChars(s, "0123456789smhd") || strings.IndexAny(s[:1], "smhd") == 0 {
		return s
	}
	k, u := s, ""
	if strings.ContainsAny(s[len(s)-1:], "smhd") {
		k, u = s[:len(s)-1], s[len(s)-1:]
	}
	if !digits(k) {
		die("--since는 시각(UTC 2026-11-16T02:00:00Z) 또는 숫자에 s·m·h·d: %s", s)
	}
	n := atoi(k)
	switch u {
	case "s":
	case "m":
		n *= 60
	case "d":
		n *= 86400
	default:
		n *= 3600
	}
	return utcAt(time.Now().Unix() - n)
}

func stdoutIsNull() bool {
	a, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	b, err := os.Stat("/dev/null")
	return err == nil && os.SameFile(a, b)
}

func sortByTime(ls []string) {
	sort.SliceStable(ls, func(i, j int) bool {
		return strings.SplitN(ls[i], tab, 2)[0] < strings.SplitN(ls[j], tab, 2)[0]
	})
}

func has(list []string, w string) bool {
	for _, x := range list {
		if x == w {
			return true
		}
	}
	return false
}

// pick: --all이면 hc machine ls에서 연결된 기계를 ems에, 아닌 기계를 unk에. --on으로 준 기계는 늘 ems에
func (e *ev) pick() {
	e.ems, e.unk, e.pbad = append([]string(nil), e.ons...), nil, false
	if !e.all {
		return
	}
	js, rc := hcRun(0, errDrop, "machine", "ls", "--json")
	if rc != 0 || !jlistOK(js, "machines", "name", "state") {
		fmt.Fprintf(os.Stderr, "hc-lane %s: hc machine ls 실패 — 이 기계만 본다\n", e.ec)
		e.pbad = true
		return
	}
	for _, r := range jlist(js, "machines", "name", "state") {
		if has(e.ems, r[0]) {
			continue
		}
		if r[1] == "connected" {
			e.ems = append(e.ems, r[0])
		} else {
			e.unk = append(e.unk, r[0])
		}
	}
}

// evrd: 원격에서 도는 사건 읽기(셸 판과 같은 글 — 원격 기계는 sh로 돈다)
const evrd = `f=${XDG_STATE_HOME:-$HOME/.local/state}/hc-lane/events.tsv; [ -f "$f" ] || exit 0; if [ "$1" = 0 ]; then cat "$f"; else tail -n "$1" "$f"; fi`

// nosh: 그 기계가 프로그램을 띄우지 못함(spawn_failed + 「cannot start」)
func nosh(s string) bool {
	return strings.Contains(s, `"spawn_failed"`) && strings.Contains(s, "cannot start ")
}

// rexec: hc exec --on(60초). 실패했는데 nosh면 종료 9
func rexec(m string, args ...string) (string, int) {
	var eb strings.Builder
	a := append([]string{"exec", "--on", m, "--timeout", "60", "--"}, args...)
	out, rc := captErr(90, &eb, "hc", a...) // --timeout 60은 원격 명령의 상한 — hc 자신이 멈춰도 끝나게(gofix M4)
	if rc != 0 && nosh(eb.String()) {
		rc = 9
	}
	return out, rc
}

func nf7(l string) bool { return l != "" && strings.Count(l, tab) >= 6 }

// evread: 이 기계와 ems 기계의 사건(7칸 이상 줄만). 원격 줄은 기계·터미널 칸을 hc 이름으로. 못 읽은 기계는 「모름 <기계>」
func (e *ev) evread(n int) []string {
	var out []string
	ls, _ := tailFileLines(e.ed+"/events.tsv", n)
	for _, l := range ls {
		if nf7(l) {
			out = append(out, l)
		}
	}
	for _, m := range e.unk {
		out = append(out, "모름 "+m)
	}
	for _, m := range e.ems {
		o, rc := rexec(m, "sh", "-c", evrd, "hc-lane", strconv.Itoa(n))
		if rc != 0 {
			if rc != 9 {
				out = append(out, "모름 "+m)
			}
			continue
		}
		for _, l := range strings.Split(o, "\n") {
			if !nf7(l) {
				continue
			}
			f := strings.Split(l, tab)
			f[1] = m
			if f[2] != "-" && !strings.Contains(f[2], "/") {
				f[2] = m + "/" + f[2]
			}
			out = append(out, strings.Join(f, tab))
		}
	}
	return out
}

// inseen: 그 파일들에 같은 사건(nk로 견줌)이 있으면 참
func inseen(k string, files ...string) bool {
	for _, p := range files {
		if p == "" {
			continue
		}
		ls, _ := readLines(p)
		for _, l := range ls {
			if nk(l) == k {
				return true
			}
		}
	}
	return false
}

func (e *ev) appendCur(lines ...string) {
	if err := appendLines(e.cur, lines...); err != nil {
		die("%s: 커서 파일을 쓰지 못했다", e.ec)
	}
}

func (e *ev) loop(to int64) int {
	me := e.wto
	if me == "" {
		me = os.Getenv("HC_TERMINAL_ID")
	}
	e.cur = e.ed + "/seen"
	if me != "" {
		e.cur += "." + ckey(me)
	}
	if e.ec == "watch" {
		e.wsf = e.ed + "/sent"
	}
	if err := os.MkdirAll(e.ed, 0o777); err != nil {
		die("wait: 폴더를 만들지 못했다: %s", e.ed)
	}
	if e.ec == "watch" {
		e.watchLock()
	}
	if tid := os.Getenv("HC_TERMINAL_ID"); tid != "" {
		if e.ec == "wait" {
			coordSet(tid, false)
		} else {
			out, _ := hcRun(10, errDrop, "get", tid, "--json")
			pa, _ := jget(jparse(out), "parent")
			if pa != "" && (pa == e.wto || strings.HasSuffix(pa, "/"+e.wto)) {
				e.wpar = true
				coordSet(e.wto, false)
			}
		}
	}
	// 새 커서는 바닥 시각(--since, 없으면 2시간 전)을 첫 줄에: 처음 wait가 옛 사건을 쏟지 않게
	if _, err := os.Lstat(e.cur); os.IsNotExist(err) {
		fl := e.since
		if fl == "" {
			fl = utcAt(time.Now().Unix() - 7200)
		}
		if os.WriteFile(e.cur, []byte("#since"+tab+fl+"\n"), 0o666) != nil {
			die("wait: 커서 파일을 쓰지 못했다: %s", e.cur)
		}
	}
	e.wgrace = cfgInt("lane.wait_grace", "10", "분")
	wb := cfg("lane.wait_busy", "shells? still running|monitors? still running")
	e.wbmax = cfgInt("lane.wait_busy_max", "30", "분")
	e.wprocs = cfg("lane.wait_procs", "shell-snapshots")
	wl := cfg("lane.wait_line", "^[[:space:]]*(✻|✶|✳|✢|✽)")
	if wl == "" {
		wl = "$.^"
	}
	e.wbusyOn = wb != ""
	e.wbusy, e.wline = mustRe("lane.wait_busy", wb, true), mustRe("lane.wait_line", wl, true)
	var tick, widle string
	var whmaxS string
	if e.ec == "watch" {
		tick = cfg("lane.watch_tick", "30")
		w := cfgInt("lane.watch_idle", "60", "분")
		widle = strconv.FormatInt(w*60, 10)
		if s := os.Getenv("HC_LANE_WATCH_IDLE"); s != "" {
			widle = s
		}
		h := cfgInt("lane.watch_hold_max", "30", "분")
		whmaxS = strconv.FormatInt(h*60, 10)
		if s := os.Getenv("HC_LANE_WATCH_HOLD_MAX"); s != "" {
			whmaxS = s
		}
		e.whmax = atoi(whmaxS)
		e.wdlg = mustRe("lane.watch_dialog", cfg("lane.watch_dialog", `Tab/Arrow keys to navigate|Chat about this|Type something|Do you want to proceed\?|Would you like to proceed\?|tell Claude what to do differently`), false)
	} else {
		tick = "60"
	}
	if s := os.Getenv("HC_LANE_WAIT_TICK"); s != "" {
		tick = s
	}
	if !digits(tick) {
		die("바퀴 간격(lane.watch_tick·HC_LANE_WAIT_TICK)은 0 이상 정수(초): %s", tick)
	}
	tk := time.Duration(atoi(tick)) * time.Second
	now := func() int64 { return time.Now().Unix() }
	end := now() + to*60
	idle0 := now()
	var told []string
	lsf, lf0, lsa := 0, "", false
	if e.mine {
		e.myp = me
	}
	gone := 0
	wsleep := func() { time.Sleep(tk) }
	for {
		if e.ec == "watch" {
			g, hrc := hcRun(10, errMerge, "get", e.wto, "--json")
			// 확인된 없음·끝남만 센다 — 잘린 not_found·state 없는 응답은 모름이라 세지 않는다(재감사 R-13)
			if _, st := getState(g, hrc); st == 0 {
				gone = 0
				if !e.wcheck() {
					wsleep()
					continue
				}
			} else if st == 1 {
				gone++
				if gone >= 3 {
					fmt.Fprintf(os.Stderr, "hc-lane watch: 지휘 터미널 %s 사라짐(hc get %d바퀴 없음·끝남) — 끝낸다\n", e.wto, gone)
					return 0
				}
				wsleep()
				continue
			}
		}
		e.pick()
		// ponytail: 바퀴마다 기계마다 끝 500줄만 본다 — 한 바퀴 사이에 한 기계에 500건 넘게 쌓이면 앞의 것을 놓친다
		out := e.evread(500)
		var mu, evl []string
		for _, l := range out {
			if m, ok := strings.CutPrefix(l, "모름 "); ok {
				mu = append(mu, m)
			} else if strings.Contains(l, tab) {
				evl = append(evl, l)
			}
		}
		var t2 []string
		for _, m := range told {
			if has(mu, m) {
				t2 = append(t2, m)
			}
		}
		told = t2
		for _, m := range mu {
			if has(told, m) {
				continue
			}
			fmt.Fprintf(os.Stderr, "hc-lane %s: 모름 %s (연결되면 그때 본다)\n", e.ec, m)
			if e.ec == "watch" && !e.wsend("hc-lane watch: 연결 안 됨 "+m+" — 다음 바퀴에 다시 본다") {
				continue
			}
			told = append(told, m)
		}
		cand := append([]string(nil), evl...)
		var st []string
		e.tl, e.lsok, e.pmap = "", true, nil
		wantStop := strings.Contains(","+orDefault(e.sts, "멈춤")+",", ",멈춤,")
		if e.myp != "" || e.ec == "watch" || wantStop {
			var rc int
			e.tl, rc = hcRun(0, errDrop, "ls", "--all", "--json", "--label", "hc-lane=1")
			e.lsok = rc == 0 && jlistOK(e.tl, "terminals", "address", "state") // 부실한 목록은 「레인 없음」이 아니라 실패(gofix K2, R-14)
			if e.myp != "" || e.ec == "watch" {
				e.pmap = jlist(e.tl, "terminals", "address", "parent")
			}
		}
		if wantStop {
			st = e.stopped(evl, mu)
			cand = append(cand, st...)
		}
		if e.ec == "watch" && (!e.lsok || e.myLaneLive()) || fillq() {
			idle0 = now()
		}
		if has(st, "#hc-ls-fail") {
			lsf++
			if lsf == 1 {
				lf0 = utcNow()
			}
			if lsf >= 3 {
				l := strings.Join([]string{utcNow(), machineName(), "-", "hc-lane-wait", "멈춤", "-",
					fmt.Sprintf("hc ls 실패 %d바퀴(%s부터) — 레인 멈춤·끝남을 못 봄", lsf, lf0), "-"}, tab)
				if !lsa && e.emit([]string{l}) == 0 {
					if e.ec != "watch" {
						return 0
					}
					lsa = true
				}
			}
		} else {
			lsf = 0
			if lsa && e.wsend("hc-lane watch: hc ls 회복 — 레인 멈춤·끝남을 다시 본다") {
				lsa = false
			}
		}
		born := ""
		if e.myp != "" && strings.Contains(strings.Join(cand, "\n"), "@") {
			born, _ = terminalCreatedMs(e.myp)
		}
		nw := e.newEvents(cand, born)
		if len(nw) > 0 && e.emit(nw) == 0 {
			if e.ec != "watch" {
				return 0
			}
			idle0 = now()
		}
		if e.ec == "wait" && now() >= end {
			fmt.Fprintf(os.Stderr, "hc-lane wait: %d분 동안 새 사건 없음\n", to)
			return 75
		}
		if e.ec == "watch" && now()-idle0 >= atoi(widle) {
			fmt.Fprintf(os.Stderr, "hc-lane watch: %s 의 살아 있는 레인·새 사건 없이 %d분 — 끝낸다\n", e.wto, atoi(widle)/60)
			return 0
		}
		wsleep()
	}
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// myLaneLive: 이 지휘의 살아 있는 레인(parent가 --to, 모르면 이 지휘 것으로 침)이 있나
func (e *ev) myLaneLive() bool {
	for _, r := range jlist(e.tl, "terminals", "address", "state", "parent") {
		if r[1] == "live" && (r[2] == "-" || r[2] == e.wto || strings.HasSuffix(r[2], "/"+e.wto)) {
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
		if fld(strings.Split(l, tab), 6) == "wait" {
			return true
		}
	}
	return false
}

// lkinds: 레인 기록에서 (주소 끝의 t…, 레인) → 종류(넷째 칸 read·fill만)
func (e *ev) lkinds() map[[2]string]string {
	k := map[[2]string]string{}
	ls, _ := readLines(e.ed + "/lanes.tsv")
	for _, l := range ls {
		f := strings.Split(l, tab)
		if len(f) >= 4 && (f[3] == "read" || f[3] == "fill") {
			k[[2]string{lastSeg(f[0]), f[1]}] = f[3]
		}
	}
	return k
}
