package main

// run 한 바퀴 앞부분: 설정·터미널 표·사건·감사·자리 세기 — 셸 판 bin/hc-fill 「run: 한 바퀴」. 띄우기 고리는 run2.go

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var auditTerm = map[string]bool{"taken": true, "superseded": true, "coord": true, "nosha": true, "legacy": true}

// runHooks: 바퀴의 끝내기 훅을 단다 — run의 신호 고루틴을 띄우기 전에(훅 목록·SB를 그 고루틴이 동기화 없이 읽지 않게, R-20).
// 훅은 바퀴가 바꾸는 전역을 읽지 않는다: 경로·dry는 단 때 값, 자리 셈 끝은 HELDW(atomic)
func runHooks() {
	SB = DATA + "/.run-brief.tmp"
	sb, d, held := SB, dry, DATA+"/held.tsv"
	exitHooks = append(exitHooks, func() {
		if !d && !HELDW.Load() {
			if err := os.WriteFile(held, nil, 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
			}
		}
		os.Remove(sb)
	})
}

func runRound() {
	tickMark("migrate")
	if !dry {
		sweepTmp()
		if withQ(func() {
			if r := rmigrate(); r != 0 {
				exit(r)
			}
		}) != 0 {
			die("대기열 복구(받은편지 줄 옮기기) 실패, 이번 바퀴는 띄우지 않았다")
		}
	}
	tickMark("inbox")
	inboxTick()
	tickMark("merger")
	mergerTick()
	tickMark("specverifiers")
	specVerifiersTick()
	tickMark("knobs")
	knobsRemoteTick()
	tickMark("onland")
	onlandTick() // 대기열이 비어도(착지한 줄이 빠진 뒤) 돈다
	tickMark("conf")
	idle := conf("idle")
	if idle == "" {
		idle = "10"
	}
	if !isDigits(idle) {
		say("알림: 설정 idle은 분(숫자)이다, 놀고 있음 알림 끔: %s", idle)
		idle = "0"
	}
	IDLE = atoi10(idle)
	// 넘기기가 남았으면(장부 open·deliver, 끝나지 않은 감사 차수) 대기열이 비어도 바퀴를 돈다
	hpend := false
	for _, r := range readRows(LED) {
		if r.F(ldState) == "deliver" || r.F(ldState) == "open" {
			hpend = true
		}
	}
	for _, r := range readRows(AT) {
		if r.NF() >= atNF && !auditTerm[r.F(atState)] {
			hpend = true
		}
	}
	if !nonEmpty(Q) && !hpend {
		say("대기열 비었음")
		// idle 0은 놀고 있음 알림만 끈다 — 자동 다시 맞춤이 켜졌으면 병합기 반송을 읽고 처리하러 바퀴를 끝까지 돈다(R-16)
		if IDLE == 0 && !relandOn() && len(prProjects()) == 0 {
			tickMark("gc")
			gcTick()
			exit(0)
		}
	}
	TASK, LAUNCH = conf("task"), conf("launch")
	if COORD = os.Getenv("HC_FILL_COORD"); COORD == "" {
		COORD = conf("coord")
	}
	if TASK == "" {
		TASK = "{run}"
	}
	if !strings.Contains(LAUNCH, "{task}") {
		die("설정 launch 줄에 {task} 자리가 없다: %s", CONF)
	}
	if !strings.Contains(TASK, "{run}") {
		die("설정 task 줄에 {run} 자리가 없다: %s", CONF)
	}
	AUD = conf("audit")
	if strings.HasPrefix(AUD, "~/") {
		AUD = HOME + "/" + AUD[2:]
	}
	AFIN = conf("audit_final")
	am := conf("audit_max")
	if am == "" {
		am = "2"
	}
	if !isDigits(am) {
		say("알림: 설정 audit_max는 횟수(숫자)다, 2로: %s", am)
		am = "2"
	}
	AMAX = atoi10(am)
	if AUD != "" {
		if why, b := briefBad(AUD); b {
			die("설정 audit 틀을 읽지 못함(%s): %s", why, AUD)
		}
	}
	tickMark("tables")
	readTables()
	tickMark("conf")
	aconf()
	EV, EVBAD, MGM = nil, false, ""
	OWNF, OWND = DATA+"/own.tsv", DATA+"/own"
	OREC, OSEED = nil, false
	if isFile(OWNF) {
		for _, r := range readRows(OWNF) {
			if r.NF() >= owAfter {
				OREC = append(OREC, r)
			} else if r.NF() > 0 {
				OSEED = true
			}
		}
	} else {
		OSEED = true
	}
	tickMark("prune")
	tryPrune()
	tickMark("events")
	evTick()
	tickMark("park")
	parkTick()
	tickMark("reply")
	replyTick() // 사건(EV) 뒤 — 제출한 받은편지 레인은 병합기 결과를 보고 회신한다
	tickMark("audit")
	auditTick()
	tickMark("lows")
	lowsTick() // 감사 통과 차수의 [낮음] 후속 레인(lows.go)
	tickMark("stall")
	stallTick()
	tickMark("stack")
	stackTick() // 병합기가 주차한 쌓인 제출 다시 쌓기(restack.go) — 다시 맞춤·반송 다시 띄움 앞
	tickMark("reland")
	relandTick() // 병합기 rebase 충돌 반송 → 다시 맞춤 줄(이 바퀴에 띄울 수 있게 orderRows 앞)
	tickMark("pr")
	prTick() // PR 거울(pr.go — 손잡이 pr, 꺼져 있으면 아무것도 안 함). 사건·감사·다시 맞춤 뒤
	tickMark("order")
	aseatSweep()
	orderRows()
	NOW, NOWL, OFF, FETCHED, FETCHBAD = nil, nil, "", "", ""
	h := conf("hold")
	if h == "" {
		h = "60"
	}
	if !isDigits(h) {
		say("알림: 설정 hold는 분(숫자)이다, 쉰 앞 레인 멈춤 판정 끔: %s", h)
		h = "0"
	}
	HOLD = atoi10(h)
	tickMark("seats")
	countSeats()
	ASEC = asecText
	if AFIN != "" {
		afinSec()
	}
	launchRows()
	exit(0)
}

// sweepTmp: 강제 종료 뒤 남은 임시 파일(10분 넘은 .x.tmp*·x.tmp·x.tmp.<pid>)을 거둔다 — run.lock 안이라 바퀴 쓰기와 겹치지 않고,
// add·qlock이 쓰는 중인 것은 10분 안이라 건드리지 않는다
func sweepTmp() {
	for _, d := range []string{DATA, DATA + "/replies", DATA + "/briefs/added", DATA + "/briefs/audit"} {
		es, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range es {
			n := e.Name()
			if !e.Type().IsRegular() || !(strings.HasSuffix(n, ".tmp") || strings.Contains(n, ".tmp.") || (strings.HasPrefix(n, ".") && strings.Contains(n, ".tmp"))) {
				continue
			}
			if st, err := e.Info(); err == nil && time.Since(st.ModTime()) > 10*time.Minute {
				os.Remove(filepath.Join(d, n))
			}
		}
	}
}

// readTables: 이번 바퀴 터미널 표·hc-lane 레인 주소·연결된 기계. hc가 실패하면 멈춘다(빈 표로 읽어 다 빈 것처럼 띄우지 않게)
func readTables() {
	// hc의 오류 글은 버리지 않는다(셸 판처럼 로그에 — Fable L9)
	hcOut := func(args ...string) (string, int) { return Cmd{Name: "hc", Args: args, Err: errPass}.Run() }
	machinesUp := func() []string {
		raw, rc := hcOut("machine", "ls", "--json")
		if rc != 0 || sub(raw) == "" {
			die("hc machine ls 실패, 이번 바퀴는 띄우지 않았다")
		}
		c, ok := connectedOf(raw)
		if !ok {
			die("hc machine ls --json 출력을 읽지 못했다, 이번 바퀴는 띄우지 않았다")
		}
		return c
	}
	// 연결 확인은 목록 앞뒤 두 번 — 목록을 읽는 동안 끊겼다 다시 붙은 기계의 터미널은 목록에 없을 수 있다. 두 번 다 연결된 기계만 연결로
	// 본다(그 기계의 「터미널 없음」을 이번 목록으로 정하지 않게 — 재감사 R-02)
	before := machinesUp()
	raw, rc := hcOut("ls", "--json")
	if rc != 0 || sub(raw) == "" {
		die("hc ls 실패, 이번 바퀴는 띄우지 않았다")
	}
	var ok bool
	if TERMS, ok = parseTerms(raw); !ok {
		die("hc ls --json 출력을 읽지 못했다, 이번 바퀴는 띄우지 않았다")
	}
	// 라벨 목록도 전체 목록과 같은 기준(주소가 빈 글이 아닌 항목만 — 하나라도 빠지면 못 읽음, 재감사 R-04)
	raw, rc = hcOut("ls", "--label", "hc-lane=1", "--json")
	var lt []Term
	if rc == 0 {
		lt, ok = parseTerms(raw)
	}
	if rc != 0 || !ok {
		die("hc ls --label hc-lane=1 실패, 이번 바퀴는 띄우지 않았다")
	}
	LANEADDR = nil
	all := map[string]bool{}
	for _, t := range TERMS {
		all[t.Addr] = true
	}
	for _, t := range lt {
		LANEADDR = append(LANEADDR, t.Addr)
		// 전체 목록 뒤에 읽은 라벨 목록에만 있다 — 그 사이에 뜬 터미널: 표에 넣는다(「없음」으로 읽혀 미확정 시도를 안 떴다고 확정하지 않게 — 재감사3 fill/R-202)
		if !all[t.Addr] {
			TERMS = append(TERMS, t)
		}
	}
	found := false
	for _, t := range TERMS {
		if t.Agent == AGENT {
			found = true
		}
	}
	if !found {
		die("경고: hc ls에 %s 종류가 하나도 없다(지휘 자신도 그 종류라 늘 하나는 있어야 한다). 설정 agent 값이나 hc의 이름이 바뀌었는지 본다. 이번 바퀴는 띄우지 않았다", AGENT)
	}
	CONN = nil
	for _, m := range machinesUp() {
		if has(before, m) {
			CONN = append(CONN, m)
		}
	}
	LSSNAP = true // 띄우기 전까지 「표에 없음 = 터미널 없음」(lsGone, tick.go)
}

var talRe = regexp.MustCompile(`^## 이어서: ([a-z0-9_-]+) `)

// orderRows: 기다리는 줄 수(WC)와 이번 바퀴 줄 차례(ROWS — 우선순위, 기다리는 줄 수, 넣은 차례)
func orderRows() {
	TAL, WC, ROWS = map[string]string{}, map[string]int{}, nil
	if !isFile(Q) {
		return
	}
	rows := qRows()
	for _, r := range rows {
		if r.NF() == 0 {
			continue
		}
		for _, l := range readLines(r.F(qBrief)) {
			if m := talRe.FindStringSubmatch(l); m != nil {
				TAL[m[1]] = r.F(qName)
			}
		}
	}
	for _, r := range rows {
		if !(r.F(qState) == "wait" || (r.F(qState) == "up" && rowMode(r) == mEarly)) || r.F(qAfter) == "" || r.F(qAfter) == "-" {
			continue
		}
		seen := map[string]bool{}
		for _, x := range strings.Split(r.F(qAfter), ",") {
			if t, ok := TAL[x]; ok {
				x = t
			}
			if !seen[x] {
				seen[x] = true
				WC[x]++
			}
		}
	}
	type ord struct {
		p, w, n int
		r       Row
	}
	var os_ []ord
	for i, r := range rows {
		if r.NF() == 0 {
			continue
		}
		p := 2
		switch r.F(qPrio) {
		case "urgent":
			p = 0
		case "high":
			p = 1
		case "low":
			p = 3
		}
		k := r.F(qName)
		if strings.HasPrefix(r.F(qAudit), "audit=") {
			k = r.F(qAudit)[6:]
			p = -1 // 자동 감사 줄은 차례 맨 앞 — 같은 바퀴의 대기 레인(긴급 포함)보다 먼저 빈 자리를 쓴다(감사 몫 예약 대신, slots.md)
		}
		os_ = append(os_, ord{p, WC[k], i, r})
	}
	sort.SliceStable(os_, func(i, j int) bool {
		a, b := os_[i], os_[j]
		if a.p != b.p {
			return a.p < b.p
		}
		if a.w != b.w {
			return a.w > b.w
		}
		return a.n < b.n
	})
	for _, o := range os_ {
		ROWS = append(ROWS, o.r)
	}
}

// AGUNK: 이번 바퀴 목록에 유형(agent) 모를 터미널이 있는 기계 — 새 일을 놓지 않는다(countSeats가 채움)
var AGUNK []string

// countSeats: 자리를 쓰는 에이전트(SEATS) — 대기열이 띄운 것 가운데 지금 일하는 것(docs/hc-fill/slots.md, 2026-11-27 사용자 결정
// 「일하는 것만 센다」) — 와 쉬기 시작한 때(held.tsv). 쉬는 레인·사람이 연 세션은 자리를 안 쓴다. ROWS(orderRows)를 읽는다
func countSeats() {
	lk := os.Getenv("XDG_STATE_HOME")
	if lk == "" {
		lk = HOME + "/.local/state"
	}
	kind := map[string]string{}
	for _, r := range readRows(lk + "/hc-lane/lanes.tsv") {
		kind[r.F(lnAddr)+"\t"+r.F(lnName)] = r.F(lnKind)
	}
	LKIND = map[string]bool{}
	for k, v := range kind {
		if v == "read" {
			LKIND[k] = true
		}
	}
	HELD = map[string]string{}
	for _, r := range readRows(DATA + "/held.tsv") {
		if _, ok := HELD[r.F(hdAddr)]; !ok {
			HELD[r.F(hdAddr)] = r.F(hdSince)
		}
	}
	ours := fillTerms()
	HNEW, HNOW, SEATS, AGUNK, SREST, SOUT = nil, nowUnix(), nil, nil, nil, nil
	for _, t := range TERMS {
		tm, ta, ts := t.M, t.Addr, t.Status
		pre := fmt.Sprintf("자리 %s %s %s: ", tm, ta, t.Name)
		rest := ts == "done" || ts == "idle"
		if t.Agent == AGENT && rest { // 쉬기 시작한 때 — 선행 판정(쉰 앞 레인, overlap2.go predState)이 읽는다
			hs := HELD[ta]
			if !isDigits(hs) {
				hs = strconv.FormatInt(HNOW, 10)
			}
			HNEW = append(HNEW, ta+"\t"+hs)
		}
		mine := ours[ta+"\t"+t.Name]
		switch {
		case t.Agent == "-": // agent 칸 없음·null — 유형 모름: 그 기계에는 이번 바퀴 새 일을 놓지 않는다(재감사2 fill/R-203)
			AGUNK = append(AGUNK, tm)
			if mine {
				say("%s유형 모름(대기열이 띄움) → 셈, 이 기계에 새 일 안 놓음", pre)
				SEATS = append(SEATS, tm)
			} else {
				say("%s유형 모름 → 이 기계에 새 일 안 놓음", pre)
			}
		case !mine: // 사람이 연 세션·지휘 세션·손으로 띄운 레인·루프 — 상태를 보내는 에이전트만 알리고 센다(셸·병합기 검증 터미널은 말없이)
			if ts != "-" {
				say("%s%s, 대기열 밖 → 안 셈", pre, ts)
				SOUT = append(SOUT, tm)
			}
		case rest:
			say("%s%s, 쉼 → 안 셈", pre, ts)
			SREST = append(SREST, tm)
		case ts == "-" && laneEnded(t):
			say("%s%s, 끝난 레인(hc keep이 셸로 바꿈) → 안 셈", pre, t.Agent)
		case ts == "-":
			say("%s%s, 상태 모름(뜨는 중) → 셈", pre, t.Agent)
			SEATS = append(SEATS, tm)
		default:
			say("%s%s → 셈", pre, ts)
			SEATS = append(SEATS, tm)
		}
	}
	if !dry {
		var b strings.Builder
		for _, l := range HNEW {
			b.WriteString(l + "\n")
		}
		if err := writeAtomic(DATA+"/held.tsv", b.String()); err != nil {
			fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
		}
	}
	HELDW.Store(true)
}

// asecText: 자동 감사 쓰기 레인의 run 지시문 끝 절(셸 판 ASEC 그대로)
const asecText = "\n## 감사·착지 (hc-fill 자동)\n" +
	"이 절이 위 지시문 본문의 감사·착지 문구보다 앞선다. 감사는 hc-fill이 띄운다(지휘가 따로 띄우지 않는다).\n" +
	"1. 구현·검증이 끝나면 착지하지 말고 `hc-lane report 완료 HEAD \"<한 줄> (감사 hc-fill 자동)\"`을(HEAD는 hc-lane이 이 작업 트리의 전체 해시로 푼다 — 해시를 손으로 베끼지 않는다) 부른 뒤 턴을 끝내고 기다린다. 판정이 올 때까지 다른 hc-lane report는 하지 않는다.\n" +
	"2. hc-fill이 「감사 판정이 왔다 — … hc-lane verdict <ID> …」로 깨우면 이 작업 트리에서 `hc-lane verdict <ID>`를 돌리고 그 출력대로만 한다(판정은 도구가 정한다 — 글로 짐작하지 않는다). 같은 글이 또 와도 같은 줄을 다시 돌리면 된다(판정은 한 번만 받힌다).\n" +
	"3. 출력이 「감사 통과 …」면 그 출력의 착지 줄 `hc-lane land <기본> --verdict <ID>`로(위 착지 절차의 land 줄 대신 — 정리 절차는 그대로) main에 넣고 `hc-lane report 착지 <sha>`. 병합기가 켜진 저장소면 착지 줄이 큐에 제출만 하고 끝난다 — 그때 착지는 병합기가 알리니 report 착지 없이 위 정리 절차의 `hc-lane clean` 줄을 바로 돌리고 끝낸다(제출 뒤 이 레인이 할 일은 없다).\n" +
	"4. 출력이 「감사 지적 … <보고서>」면 그 보고서를(이 기계에 없으면 `hc-lane audit-report <ID>`로) 읽고 높음·중간을 고친 뒤 다시 검증하고 `hc-lane report 완료 HEAD \"<한 줄>\"`, 그리고 다시 기다린다. 출력이 「지금 커밋으로 hc-lane report 완료 …」면 그 줄을 부르고 기다린다.\n"
