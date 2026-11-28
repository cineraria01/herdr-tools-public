// push.go: watch가 지휘에 밀어 주기 — emit(한 문 gate → 묶음 → wsend)·owner·evowner·wsend·wheld·wdialog·wcheck·watchLock
// (셸 판 lib/hc-lane/watch.sh와 같은 꼴: 같은 커서·sent·잠금 폴더·watch.log)
package main

import (
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"herdr-tools/internal/events"
	"herdr-tools/internal/lockx"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/tri"
)

// emit: 커서에 적고 찍는다. watch는 먼저 지휘 터미널에 한 줄로 밀어 주고(hc send), 보낸 뒤에만 커서에 적는다.
// 0 됨, 1 못 보냄(다음 바퀴에 다시), 3 이 지휘의 사건이 없음
func (e *ev) emit(lines []string) int {
	if e.ec == "watch" {
		lines = e.gate(lines) // 한 문(gatesend.go): 이 지휘가 받는 쪽인 줄만
		// 묶음(digest.go): 모아 두는 바퀴는 아무것도 안 보내고 커서에도 안 적는다 — 한가로 세지 않게 0
		dig, imm, hold := e.digest(lines)
		if hold {
			return 0
		}
		lines = append(dig, imm...)
		if len(lines) == 0 {
			return 3
		}
		var msg []string
		if len(dig) > 0 {
			msg = append(msg, e.digestText(dig))
		}
		if len(imm) > 0 {
			var parts []string
			for _, l := range imm {
				k := lineKey(l)
				parts = append(parts, e.rpre[k]+wpart(l)+e.rtag[k])
			}
			msg = append(msg, "hc-lane: "+strings.Join(parts, " ‖ "))
		}
		if !e.wsend(stripCtl(strings.Join(msg, " ‖ "))) {
			return 1
		}
		e.dg.sent(dig)
	}
	var ks []string
	for _, l := range lines {
		ks = append(ks, lineKey(l))
	}
	e.appendCur(ks...)
	if e.ec == "wait" {
		e.waitSent(lines)
	}
	// 「보냄」 표시(공용 — 누가 받았나)는 watch만, 한 명에게 간 사건만(받는 쪽 확인 못 해 모두에게 가는 것은 각자 커서로)
	if e.ec == "watch" {
		var sk []string
		for _, l := range lines {
			if v, ok := e.vd[lineKey(l)]; ok && !v.all {
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

// owner: 그 레인을 띄운 지휘 — 레인 기록의 마지막 줄, 없으면 hc ls parent, 없으면 -. 끝의 t…
// parent가 레인(hc ls 레인 목록에 있음)이면 그 레인의 주인으로 올라간다 — 레인은 지휘가 아니라 「주인 사라짐」으로 모든 지휘에게 가던 것
// (2026-11-27 ht-watch-child-job)
func (e *ev) owner(a string) string {
	ls, _ := readLines(e.ed + "/lanes.tsv")
	for range 8 { // 부모 사슬 상한(고리 막기)
		o := ""
		for _, x := range ls {
			f := strings.Split(x, tab)
			if f[0] == a {
				o = at(f, 2)
			}
		}
		if o != "" && o != "-" {
			return lastSeg(o)
		}
		p, lane := "", false
		for _, r := range e.pmap {
			if r[0] == a {
				p = r[1]
			}
		}
		if p == "" || p == "-" {
			return "-"
		}
		for _, r := range e.pmap {
			lane = lane || r[0] == p
		}
		if !lane {
			return lastSeg(p)
		}
		a = p
	}
	return "-"
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
	return e.laneOwner(x.Term)
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
	if c == tri.No && e.wpar {
		if _, err := os.Stat(coordFile(e.wto)); os.IsNotExist(err) {
			coordSet(e.wto, false)
			_, why, c = coordCheck(e.wto)
		}
	}
	if c == tri.Unknown {
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
	if c != tri.Yes {
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
