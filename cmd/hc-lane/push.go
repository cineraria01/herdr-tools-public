// push.go: watch가 지휘에 밀어 주기 — emit·wfilter·prevseen·owner·evowner·wsend·wheld·wdialog·wcheck·watchLock
// (셸 판 lib/hc-lane/watch.sh와 같은 꼴: 같은 커서·sent·잠금 폴더·watch.log)
package main

import (
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"herdr-tools/internal/events"
	"herdr-tools/internal/lockx"
	"herdr-tools/internal/proc"
)

// emit: 커서에 적고 찍는다. watch는 먼저 지휘 터미널에 한 줄로 밀어 주고(hc send), 보낸 뒤에만 커서에 적는다.
// 0 됨, 1 못 보냄(다음 바퀴에 다시), 3 이 지휘의 사건이 없음
func (e *ev) emit(lines []string) int {
	if e.ec == "watch" {
		lines = e.wfilter(lines)
		if len(lines) == 0 {
			return 3
		}
		var parts []string
		for _, l := range lines {
			parts = append(parts, wpart(l))
		}
		if !e.wsend(stripCtl("hc-lane: " + strings.Join(parts, " ‖ "))) {
			return 1
		}
	}
	var ks []string
	for _, l := range lines {
		ks = append(ks, lineKey(l))
	}
	e.appendCur(ks...)
	// 「보냄」 표시는 watch만, 이 watch가 주인인 사건만
	if e.ec == "watch" {
		var sk []string
		for _, l := range lines {
			if e.evowner(l) == lastSeg(e.wto) {
				sk = append(sk, lineKey(l))
			}
		}
		if appendLines(e.wsf, sk...) != nil {
			fmt.Fprintf(os.Stderr, "hc-lane %s: 보냄 표시를 쓰지 못했다: %s\n", e.ec, e.wsf)
		}
	}
	for _, l := range lines {
		fmt.Println(l)
	}
	return 0
}

// wpart: watch가 지휘에 보내는 사건 하나의 글 「<레인> <상태> [sha] <한 줄> (<어디>)」.
// 운영 사건(events.KindOf)이면 머리 「운영: 」, 이름 표에 있으면 끝 「 → <다음 명령>」(ops-events — 받는 쪽이 레인 일과 가르고 바로 볼 명령)
func wpart(l string) string {
	x := events.Cols(l)
	s := x.Lane + " " + x.State
	if x.SHA != "-" {
		s += " " + x.SHA
	}
	// 끝 괄호는 터미널 주소, 터미널 밖 사건(주소 -)이면 기계 칸(disk-low-machine — 「(-)」로 와 어느 기계인지 몰랐다)
	where := x.Term
	if where == "-" {
		where = x.Machine
	}
	s += " " + x.Msg + " (" + where + ")"
	if x.KindOf() != events.KindOps {
		return s
	}
	if o, ok := events.OpOf(x.Lane, x.Key); ok {
		s += " → " + o.Next
	}
	return "운영: " + s
}

// stripCtl: 제어 문자(ESC·Ctrl-C 등)는 지휘 입력에 넣지 않는다(tr -d '\000-\037\177')
func stripCtl(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// wfilter: 이 watch(--to 지휘)가 보낼 줄만 — 주인이 이 지휘·모름·사라짐이면 보냄, 다른 지휘가 살아 있거나 모르면 안 보냄
func (e *ev) wfilter(lines []string) []string {
	w0 := lastSeg(e.wto)
	recipientRun := ""
	occ := map[string]int{}
	e.occ, e.seenc = occ, map[string]seenCache{}
	e.pruneDefer()
	orc := map[string]string{}
	var out []string
	for _, l := range lines {
		if l == "" {
			continue
		}
		x := events.Cols(l)
		a := x.Term
		if x.Lane == "hc-lane-wait" {
			out = append(out, l)
			continue
		}
		if t := x.To; t != "" && t != "-" {
			if !strings.Contains(","+t+",", ","+w0+",") && !strings.Contains(","+t+",", "/"+w0+",") {
				continue
			}
			if intended := x.Owner; strings.HasPrefix(intended, t+"@") {
				if recipientRun == "" {
					if r, ok := terminalCreatedMs(e.wto); ok {
						recipientRun = r
					} else {
						recipientRun = "unknown"
					}
				}
				if !digits(recipientRun) || intended != t+"@"+recipientRun {
					continue
				}
			}
			out = append(out, l)
			continue
		}
		o := e.evowner(l)
		if o == "-" {
			if e.repoSend(l) {
				out = append(out, l)
			}
			continue
		}
		om := x.Owner
		if i := strings.LastIndex(om, "@"); i >= 0 {
			om = om[i+1:]
		} else {
			om = ""
		}
		if om != "" {
			oc, ok := orc[o]
			if !ok {
				// 주인의 지금 실행(created_ms): 확인된 끝남·완전한 not_found만 gone, 모름은 unk(hcjson.ParseGet)
				t, found, err := hcGet(10, o)
				oc = "unk"
				switch {
				case err != nil:
				case !found || t.State != "live":
					oc = "gone"
				case t.Created > 0:
					oc = strconv.FormatInt(t.Created, 10)
				}
				orc[o] = oc
			}
			if oc == "unk" {
				continue
			}
			if oc != om {
				if !e.prevseen(a, l, o) && e.repoSend(l) {
					out = append(out, l)
				}
				continue
			}
		}
		if o == w0 {
			if !e.prevseen(a, l, o) {
				out = append(out, l)
			}
			continue
		}
		r, ok := occ[o]
		if !ok {
			_, _, r = coordCheck(o)
			occ[o] = r
		}
		if r != 1 {
			continue
		}
		if !e.prevseen(a, l, o) && e.repoSend(l) {
			out = append(out, l)
		}
	}
	return out
}

// prevseen: 그 레인의 옛 주인이 자기 wait로 이미 본 사건이면 참 — 이 watch 커서에도 적는다
func (e *ev) prevseen(a, l, o string) bool {
	k := lineKey(l)
	set := map[string]bool{lastSeg(o): true}
	ls, _ := readLines(e.ed + "/lanes.tsv")
	for _, x := range ls {
		f := strings.Split(x, tab)
		if f[0] == a {
			set[lastSeg(at(f, 2))] = true
		}
	}
	var ps []string
	for p := range set {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	w0 := lastSeg(e.wto)
	for _, p := range ps {
		if p == "" || p == "-" || p == w0 {
			continue
		}
		if inseen(k, e.ed+"/seen."+ckey(p)) {
			e.appendCur(k)
			return true
		}
	}
	return false
}

// owner: 그 레인을 띄운 지휘 — 레인 기록의 마지막 줄, 없으면 hc ls parent, 없으면 -. 끝의 t…
func (e *ev) owner(a string) string {
	o := ""
	ls, _ := readLines(e.ed + "/lanes.tsv")
	for _, x := range ls {
		f := strings.Split(x, tab)
		if f[0] == a {
			o = at(f, 2)
		}
	}
	if o == "" || o == "-" {
		o = ""
		for _, r := range e.pmap {
			if r[0] == a {
				o = r[1]
				break
			}
		}
	}
	if o == "" || o == "-" {
		return "-"
	}
	return lastSeg(o)
}

// evowner: 사건의 주인 지휘 — 11칸(* = 주인 없음 → -), 없으면 레인 기록(주인이 둘 이상이면 -)
func (e *ev) evowner(l string) string {
	x := events.Cols(l)
	switch e11 := x.Owner; e11 {
	case "*":
		return "-"
	case "", "-":
	default:
		if i := strings.Index(e11, "@"); i >= 0 {
			e11 = e11[:i]
		}
		return lastSeg(e11)
	}
	a := x.Term
	own := map[string]bool{}
	ls, _ := readLines(e.ed + "/lanes.tsv")
	for _, r := range ls {
		g := strings.Split(r, tab)
		if g[0] == a && at(g, 2) != "-" && at(g, 2) != "" {
			own[g[2]] = true
		}
	}
	if len(own) > 1 {
		return "-"
	}
	return e.owner(a)
}

// wsend: 지휘 터미널에 붙여 넣고 Enter(60초 상한). 지휘 확인·질문 창 보류를 먼저
func (e *ev) wsend(m string) bool {
	if !e.wcheck() || e.wheld() {
		return false
	}
	if _, rc := hcRun(60, errDrop, "send", e.wto, m); rc != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane watch: hc send %s 실패 — 다음 바퀴에 다시\n", e.wto)
		return false
	}
	return true
}

var (
	reRule   = regexp.MustCompile(`^[ \t]*(─|╌)(─|╌)(─|╌)`)
	rePrompt = regexp.MustCompile(`^[ \t]*❯`)
)

// wheld: 지휘가 질문·권한 창을 띄웠으면 참(보내지 않음). status blocked는 lane.watch_hold_max 뒤 보내고, 화면으로 잡은 창은 늘 보류
func (e *ev) wheld() bool {
	if e.whmax <= 0 {
		return false
	}
	// 조회를 못 하면 보류하지 않는다 — 지휘 확인(wcheck)이 먼저 모름을 막는다
	t, found, err := hcGet(10, e.wto)
	_, hasSt := t.Raw["status"]
	hw := ""
	if err == nil && found && t.Status == "blocked" {
		hw = "blocked"
	} else if err == nil && found && hasSt && t.Status == "" {
		if scr, _ := hcRun(10, errDrop, "read", e.wto); e.wdialog(scr) {
			hw = "화면"
		}
	}
	if hw == "" {
		e.wh0, e.whLogged = 0, false
		return false
	}
	n := time.Now().Unix()
	if e.wh0 == 0 {
		e.wh0, e.whLogged = n, false
		fmt.Fprintf(os.Stderr, "hc-lane watch: 지휘 %s 질문·권한 창(%s) — 보내지 않고 창이 닫히면 한 번에 보낸다\n", e.wto, hw)
	}
	if n-e.wh0 < e.whmax {
		return true
	}
	if hw == "화면" {
		if !e.whLogged {
			e.whLogged = true
			l := fmt.Sprintf("%s hc-lane watch: %s 질문 창 %d분째 — 보류 중(화면으로 잡은 창은 보내지 않는다, 사건은 hc-lane wait로도 받음)", utcNow(), e.wto, (n-e.wh0)/60)
			fmt.Fprintln(os.Stderr, l)
			_ = appendLines(e.ed+"/watch.log", l) // 알림 기록일 뿐 — 못 써도 보류는 그대로
		}
		return true
	}
	if !e.whLogged {
		e.whLogged = true
		fmt.Fprintf(os.Stderr, "hc-lane watch: 지휘 %s 질문·권한 창이 %d분 넘게 — 보낸다(blocked면 hc가 거절해 창이 닫힐 때까지 다시)\n", e.wto, e.whmax/60)
	}
	return false
}

// wdialog: 화면이 Claude Code 질문·권한 창이면 참 — 마지막 가로줄 아래만 본다(평소 입력 칸이면 아님)
func (e *ev) wdialog(scr string) bool {
	ls := strings.Split(scr, "\n")
	lr := -1
	box := false
	for i, l := range ls {
		if reRule.MatchString(l) {
			box = lr >= 0 && lr+1 < i && rePrompt.MatchString(ls[lr+1])
			lr = i
		}
	}
	if box || lr < 0 {
		return false
	}
	for _, l := range ls[lr+1:] {
		if e.wdlg.MatchString(l) {
			return true
		}
	}
	return false
}

// wcheck: 이 watch의 지휘 정보가 맞나. 다르거나 없으면 「지휘 없음」을 남기고 끝낸다(종료 0), 모르면 거짓, 맞으면 참
func (e *ev) wcheck() bool {
	_, why, c := coordCheck(e.wto)
	if c == 1 && e.wpar {
		if _, err := os.Stat(coordFile(e.wto)); os.IsNotExist(err) {
			coordSet(e.wto, false)
			_, why, c = coordCheck(e.wto)
		}
	}
	if c == 2 {
		fmt.Fprintf(os.Stderr, "hc-lane watch: 지휘 %s 확인 못 함(%s) — 다음 바퀴에 다시\n", e.wto, why)
		// 모름이 lane.watch_hold_max 넘게 이어지면 watch.log에 한 번 — 보내지도 끝내지도 않는다(모름을 있음·없음으로 읽지 않는다)
		n := time.Now().Unix()
		if e.wu0 == 0 {
			e.wu0 = n
		}
		if !e.wuLogged && n-e.wu0 >= e.whmax {
			e.wuLogged = true
			l := fmt.Sprintf("%s hc-lane watch: 지휘 %s 확인 못 함 %d분째 — 보내지 않고 계속 본다(%s, 사건은 hc-lane wait로도 받음)", utcNow(), e.wto, (n-e.wu0)/60, why)
			fmt.Fprintln(os.Stderr, l)
			_ = appendLines(e.ed+"/watch.log", l) // 알림 기록일 뿐
		}
		return false
	}
	e.wu0, e.wuLogged = 0, false
	if c != 0 {
		w := "지휘 없음"
		if strings.Contains(why, "에이전트") {
			w = "에이전트 없음"
		}
		e.wend(w, fmt.Sprintf("지휘 없음 %s — %s. 보내지 않고 끝낸다(사건은 events에 남음, 지휘가 wait/watch를 걸면 받음)", e.wto, why))
		exit(0)
	}
	return true
}

// watchLock: 같은 지휘 주소로 watch는 하나만(뒤 것은 종료 75). 잠금 폴더 <evdir>/watch.<주소>/pid. 남은 잠금은 넘겨받는다.
// 끝날 때(지휘 사라짐·TERM·INT·HUP) 자기 잠금만 지운다
func (e *ev) watchLock() {
	e.lk = e.ed + "/watch." + strings.TrimPrefix(e.cur, e.ed+"/seen.")
	// 커널 잠금이 먼저: 쥔 watch가 살아 있는 동안 다른 watch는 잠금 폴더를 건드리지 못한다(죽으면 커널이 푼다). 남은 폴더 넘겨받기도
	// 이 잠금 안에서만 하니 두 watch가 서로의 잠금을 지우지 않는다(재감사 R-20). 쥔 파일은 프로세스가 끝날 때까지 연다
	lf, busy, err := lockx.TryLock(e.lk + ".lock")
	if err != nil || busy {
		fmt.Fprintf(os.Stderr, "hc-lane watch: %s 감시가 이미 돈다(잠금 %s.lock)\n", e.wto, e.lk)
		exit(75)
	}
	watchLockFile = lf
	if os.Mkdir(e.lk, 0o777) != nil {
		// 자기 pid의 잠금 폴더는 새 판으로 exec한 이 watch 자신의 것(selfexec) — 넘겨받는다
		if op, ok := watchAlive(e.lk); ok && op != strconv.Itoa(os.Getpid()) {
			fmt.Fprintf(os.Stderr, "hc-lane watch: %s 감시가 이미 돈다(pid %s)\n", e.wto, op)
			exit(75)
		}
		if err := os.RemoveAll(e.lk); err != nil || os.Mkdir(e.lk, 0o777) != nil {
			fmt.Fprintf(os.Stderr, "hc-lane watch: %s 감시가 막 떴다\n", e.wto)
			exit(75)
		}
	}
	me := strconv.Itoa(os.Getpid())
	if os.WriteFile(e.lk+"/pid", []byte(me+"\n"), 0o666) != nil {
		die("watch: 잠금을 쓰지 못했다: %s", e.lk)
	}
	lk := e.lk
	cleanups = append(cleanups, func() {
		if b, err := os.ReadFile(lk + "/pid"); err == nil && strings.TrimSpace(string(b)) == me {
			os.RemoveAll(lk)
		}
	})
	proc.TakeSignals() // 이 처리기 하나만(재감사 R-H04 — proc 처리기가 함께 받아 정리 전에 끝내지 않게)
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		s := <-sc
		code := map[os.Signal]int{syscall.SIGTERM: 143, syscall.SIGINT: 130, syscall.SIGHUP: 129}[s]
		proc.StopCurrent(proc.Grace) // 상한으로 따로 무리가 된 자식(hc ls 등)도 함께 거둔다
		exit(code)
	}()
}

// watchAlive: 그 잠금의 pid가 살아 있는 hc-lane watch면 참(그 pid)
func watchAlive(lk string) (string, bool) {
	b, err := os.ReadFile(lk + "/pid")
	if err != nil {
		return "", false
	}
	op := strings.TrimSpace(string(b))
	if !digits(op) {
		return op, false
	}
	pid, _ := strconv.Atoi(op)
	if !proc.Alive(pid) {
		return op, false
	}
	out, rc := capt(0, errDrop, "ps", "-p", op, "-o", "command=")
	if rc != 0 || strings.TrimSpace(out) == "" {
		// ps를 못 읽음(권한·시간 넘김·0인데 빈 출력) — 살아 있는 pid의 소유자를 모르면 잠금을 넘겨받지 않는다(gofix E05, 재감사 R-20).
		// 그사이 끝났으면 넘겨받는다
		return op, proc.Alive(pid)
	}
	return op, reWatchCmd.MatchString(out)
}

var (
	reWatchCmd    = regexp.MustCompile(`hc-lane.*watch`)
	watchLockFile *os.File // watchLock이 쥔 커널 잠금(닫히면 풀리니 프로세스가 끝날 때까지 잡아 둔다)
)
