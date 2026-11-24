// watch.go: hc-lane events·wait·watch — 레인 사건 읽기·기다리기·지휘에 밀어 주기
// (셸 판 lib/hc-lane/report.sh ev_cmd·pick·evread·inseen, lib/hc-lane/watch.sh emit·wfilter·prevseen·owner·evowner·wsend·wheld·
// wdialog·wcheck·watch_lock·fillq와 같은 꼴: 같은 커서·sent·잠금 폴더·지휘 정보 파일·출력·종료 코드)
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/events"
	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/selfexec"
	"herdr-tools/internal/tri"
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
	lanes        []string // --lane(events·wait): 이 레인들의 사건만(evargs.go laneOK)
	kind         string   // --kind(events·wait): 이 종류(ops·lane)의 사건만(evargs.go laneOK)
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
	wu0          int64 // 지휘 확인 모름이 이어진 첫 시각(wcheck)
	wuLogged     bool
	self         *selfexec.Self // watch가 뜰 때의 실행 파일 — 새 판이 깔리면 바퀴 사이에 그 판으로 이어 돈다
	wpar         bool
	tl           string
	tls          hcjson.List // tl을 hcjson.ParseList로 읽은 것(lsok일 때만)
	lsok         bool
	pmap         [][]string
	lk           string
	lrepo        map[string]string    // 이 바퀴 사건의 레인 이름 → 13칸 저장소(laneRepos)
	occ          map[string]tri.State // 이 바퀴 wfilter의 지휘 확인(coordCheck) 결과
	seenc        map[string]seenCache
	rgrace       time.Duration        // 남의 저장소 주인 없는 사건을 미루는 유예(repoSend)
	rdefer       map[string]time.Time // 미룬 열쇠 → 이 watch가 처음 미룬 때(바퀴를 넘어 남음)
	dg           digestState          // 지휘 알림 묶기(digest.go)
}

// orHyph: 빈 칸은 「-」(옛 jlist 칸 꼴)
func orHyph(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// rawCell: 항목의 그 칸 원문을 옛 jlist 칸 꼴로 — 없음·null은 「-」, 문자열은 그 글, 그 밖(숫자 등)은 JSON 그대로
func rawCell(t hcjson.Terminal, k string) string {
	r, ok := t.Raw[k]
	if !ok || string(r) == "null" {
		return "-"
	}
	var s string
	if json.Unmarshal(r, &s) == nil {
		return s
	}
	return string(r)
}

// lsLanes: hc ls --all --json --label hc-lane=1 을 엄격히 읽는다(hcjson.ParseList) — 실패·꼴이 아님이면 lsok 거짓(「레인 없음」이 아니다, gofix K2·R-14)
func (e *ev) lsLanes() {
	var rc int
	e.tl, rc = hcRun(0, errDrop, "ls", "--all", "--json", "--label", "hc-lane=1")
	l, err := hcjson.ParseList([]byte(e.tl))
	e.tls, e.lsok, e.pmap = l, rc == 0 && err == nil, nil
	if !e.lsok {
		e.tls = hcjson.List{}
		return
	}
	for _, t := range l.Terminals {
		e.pmap = append(e.pmap, []string{t.Address, orHyph(t.Parent)})
	}
}

func evCmd(ec string, a []string) int {
	e := &ev{ec: ec, ed: evdir()}
	to := e.args(a) // evargs.go
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
			} else if x := events.Cols(l); x.N > 1 && x.Time >= e.since && e.laneOK(x) {
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
		dieArg("--since는 시각(UTC 2026-11-16T02:00:00Z) 또는 숫자에 s·m·h·d: %s", s)
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

func nf7(l string) bool { return l != "" && strings.Count(l, tab) >= 6 }

// 바퀴 창: 새 사건 후보는 기계마다 끝 evWin줄, 그 앞 evCtx줄은 판정 문맥(마지막 report)으로만 — 착지가 창 밖으로 빠진
// 정리됨이 거짓 「멈춤 뜻밖의 종료(정리됨)」이 되지 않게(watch-window-stale-stop, 2026-11-23)
// ponytail: 문맥도 끝 evWin+evCtx줄까지 — 앞 report와 정리됨 사이가 evCtx줄을 넘으면 다시 거짓 멈춤. 바퀴마다 기계마다 최대 2000줄(줄 평균 약 210바이트 ≈ 420KB)
const evWin, evCtx = 500, 1500

// evread: 이 기계와 ems 기계의 사건(7칸 이상 줄만). 원격 줄은 기계·터미널 칸을 hc 이름으로. 못 읽은 기계는 「모름 <기계>」
func (e *ev) evread(n int) []string {
	_, w := e.evreadCtx(n, 0)
	return w
}

// evreadCtx: 기계마다 끝 n+c줄을 읽어 끝 n줄은 win, 그 앞(있는 만큼)은 ctx로. n이 0이면 전부 win
func (e *ev) evreadCtx(n, c int) (ctx, win []string) {
	nr := n
	if n > 0 {
		nr = n + c
	}
	add := func(raw []string, fix func(string) string) {
		k := 0
		if n > 0 && len(raw) > n {
			k = len(raw) - n
		}
		for i, l := range raw {
			if !nf7(l) {
				continue
			}
			if i < k {
				ctx = append(ctx, fix(l))
			} else {
				win = append(win, fix(l))
			}
		}
	}
	ls, _ := tailFileLines(e.ed+"/events.tsv", nr)
	add(ls, func(l string) string { return l })
	for _, m := range e.unk {
		win = append(win, "모름 "+m)
	}
	for _, m := range e.ems {
		o, rc := rexec(m, "sh", "-c", evrd, "hc-lane", strconv.Itoa(nr))
		if rc != 0 {
			if rc != 9 {
				win = append(win, "모름 "+m)
			}
			continue
		}
		add(strings.Split(strings.TrimRight(o, "\n"), "\n"), func(l string) string {
			x := events.Cols(l)
			x.Machine = m
			if x.Term != "-" && !strings.Contains(x.Term, "/") {
				x.Term = m + "/" + x.Term
			}
			return x.Raw()
		})
	}
	return ctx, win
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
		// 커서·보냄 표시는 파일이고 바퀴 사이(보내기 밖)에서만 넘어가니 같은 사건을 두 번 보내거나 건너뛰지 않는다. 메모리 상태(쉼·질문 창
		// 보류 시각, 「연결 안 됨」 알린 기계)는 새 판에서 처음부터 — 「연결 안 됨」이 한 번 더 갈 수 있다. 넘겨줌은 정상 동작이라
		// watch.log에만(운영 기록에는 안 남김 — 감사 낮음 1)
		e.self = selfexec.New("hc-lane watch", func(l string) { e.wlog(l) })
	}
	if tid := os.Getenv("HC_TERMINAL_ID"); tid != "" {
		if e.ec == "wait" {
			coordSet(tid, false)
		} else {
			t, found, err := hcGet(10, tid)
			if pa := t.Parent; err == nil && found && pa != "" && (pa == e.wto || strings.HasSuffix(pa, "/"+e.wto)) {
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
	if e.ec == "watch" {
		e.rgrace = repoGrace(atoi(tick))
	}
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
			e.self.Check()
			// 확인된 없음·끝남만 센다 — 잘린 not_found·state 없는 응답은 모름이라 세지 않는다(재감사 R-13, hcjson.ParseGet)
			if st := getState(hcGet(10, e.wto)); st == tri.Yes {
				gone = 0
				if !e.wcheck() {
					wsleep()
					continue
				}
			} else if st == tri.No {
				gone++
				if gone >= 3 {
					e.wend("지휘 사라짐", fmt.Sprintf("지휘 터미널 %s 사라짐(hc get %d바퀴 없음·끝남) — 끝낸다", e.wto, gone))
					return 0
				}
				wsleep()
				continue
			}
		}
		e.pick()
		// ponytail: 바퀴마다 기계마다 끝 evWin줄만 새 사건으로 본다 — 한 바퀴 사이에 한 기계에 evWin건 넘게 쌓이면 앞의 것을 놓친다
		ctx, out := e.evreadCtx(evWin, evCtx)
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
		if e.ec == "watch" {
			e.lrepo = laneRepos(append(append([]string(nil), ctx...), evl...))
		}
		cand := append([]string(nil), evl...)
		var st []string
		e.tl, e.tls, e.lsok, e.pmap = "", hcjson.List{}, true, nil
		wantStop := strings.Contains(","+orDefault(e.sts, "멈춤")+",", ",멈춤,")
		if e.myp != "" || e.ec == "watch" || wantStop {
			e.lsLanes()
		}
		if wantStop {
			st = e.stopped(append(append([]string(nil), ctx...), evl...), mu) // 마지막 report는 창 앞 문맥까지 보고
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
		nw := e.newEvents(ctx, cand, born)
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
		if e.ec == "watch" && now()-idle0 >= atoi(widle) && e.openHold(append(append([]string(nil), ctx...), evl...), now()) {
			idle0 = now()
		}
		if e.ec == "watch" && now()-idle0 >= atoi(widle) {
			e.wend("한가", fmt.Sprintf("%s 의 살아 있는 레인·결과 기다리는 제출·주차·새 사건 없이 %d분 — 끝낸다", e.wto, atoi(widle)/60))
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
