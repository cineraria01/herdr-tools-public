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

func runRound() {
	SB = DATA + "/.run-brief.tmp"
	exitHooks = append(exitHooks, func() {
		if !dry && !HELDW {
			if err := os.WriteFile(DATA+"/held.tsv", nil, 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
			}
		}
		os.Remove(SB)
	})
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
	inboxTick()
	mergerTick()
	specVerifiersTick()
	onlandTick() // 대기열이 비어도(착지한 줄이 빠진 뒤) 돈다
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
		if r.F(4) == "deliver" || r.F(4) == "open" {
			hpend = true
		}
	}
	for _, r := range readRows(AT) {
		if r.NF() >= 17 && !auditTerm[r.F(8)] {
			hpend = true
		}
	}
	if !nonEmpty(Q) && !hpend {
		say("대기열 비었음")
		if IDLE == 0 {
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
	readTables()
	aconf()
	EV, EVBAD, MGM = nil, false, ""
	OWNF, OWND = DATA+"/own.tsv", DATA+"/own"
	OREC, OSEED = nil, false
	if isFile(OWNF) {
		for _, r := range readRows(OWNF) {
			if r.NF() >= 7 {
				OREC = append(OREC, r)
			} else if r.NF() > 0 {
				OSEED = true
			}
		}
	} else {
		OSEED = true
	}
	tryPrune()
	evTick()
	parkTick()
	replyTick() // 사건(EV) 뒤 — 제출한 받은편지 레인은 병합기 결과를 보고 회신한다
	auditTick()
	lowsTick() // 감사 통과 차수의 [낮음] 후속 레인(lows.go)
	stallTick()
	relandTick() // 병합기 rebase 충돌 반송 → 다시 맞춤 줄(이 바퀴에 띄울 수 있게 orderRows 앞)
	auditSeats()
	orderRows()
	NOW, NOWL, OFF, FETCHED, FETCHBAD = nil, nil, "", "", ""
	ALLM = nil
	for _, m := range machines() {
		if m.All == "all" {
			ALLM = append(ALLM, m.Name)
		}
	}
	h := conf("hold")
	if h == "" {
		h = "60"
	}
	if !isDigits(h) {
		say("알림: 설정 hold는 분(숫자)이다, 쉰 레인도 셈: %s", h)
		h = "0"
	}
	HOLD = atoi10(h)
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
	raw, rc := hcOut("ls", "--json")
	if rc != 0 || sub(raw) == "" {
		die("hc ls 실패, 이번 바퀴는 띄우지 않았다")
	}
	var ok bool
	if TERMS, ok = parseTerms(raw); !ok {
		die("hc ls --json 출력을 읽지 못했다, 이번 바퀴는 띄우지 않았다")
	}
	raw, rc = hcOut("ls", "--label", "hc-lane=1", "--json")
	if rc == 0 {
		LANEADDR, ok = jsonList(raw, "terminals", "address", nil)
	}
	if rc != 0 || !ok {
		die("hc ls --label hc-lane=1 실패, 이번 바퀴는 띄우지 않았다")
	}
	raw, rc = hcOut("machine", "ls", "--json")
	if rc != 0 || sub(raw) == "" {
		die("hc machine ls 실패, 이번 바퀴는 띄우지 않았다")
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
	if CONN, ok = jsonList(raw, "machines", "name", isConnected); !ok {
		die("hc machine ls --json 출력을 읽지 못했다, 이번 바퀴는 띄우지 않았다")
	}
}

// auditSeats: 자리에서 뺄 자동 감사 레인 이름(AUDN)과, 감사 줄이 자리를 쥔 동안의 쓰기 레인 주소(AWT)
func auditSeats() {
	AUDN, AWT = nil, nil
	if AUD == "" || !isFile(Q) {
		return
	}
	rows := qRows()
	hold := ","
	for _, r := range rows {
		if !strings.HasPrefix(r.F(15), "audit=") {
			continue
		}
		AUDN = append(AUDN, r.F(1))
		switch r.F(6) {
		case "wait":
			if strings.Contains(","+r.F(9)+",", ","+r.F(5)+"=") {
				continue
			}
		case "up":
			if s := alive(r.F(8)); s == "gone" || s == "done" || s == "idle" {
				continue
			}
		default:
			continue
		}
		hold += strings.TrimPrefix(r.F(15), "audit=") + ","
	}
	w := map[string]bool{}
	for _, a := range readRows(AT) {
		if a.NF() >= 17 && (a.F(8) == "queued" || a.F(8) == "opening") {
			w[a.F(2)] = true
		}
	}
	for _, r := range rows {
		if r.F(6) == "up" && r.F(15) == "auto" && w[r.F(1)] && strings.Contains(hold, ","+r.F(1)+",") {
			AWT = append(AWT, r.F(8))
		}
	}
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
		for _, l := range readLines(r.F(3)) {
			if m := talRe.FindStringSubmatch(l); m != nil {
				TAL[m[1]] = r.F(1)
			}
		}
	}
	for _, r := range rows {
		if !(r.F(6) == "wait" || (r.F(6) == "up" && r.F(14) == "early")) || r.F(4) == "" || r.F(4) == "-" {
			continue
		}
		seen := map[string]bool{}
		for _, x := range strings.Split(r.F(4), ",") {
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
		switch r.F(13) {
		case "urgent":
			p = 0
		case "high":
			p = 1
		case "low":
			p = 3
		}
		k := r.F(1)
		if strings.HasPrefix(r.F(15), "audit=") {
			k = r.F(15)[6:]
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

// countSeats: 자리를 차지한 에이전트(SEATS)와 쉬기 시작한 때(held.tsv)
func countSeats() {
	lk := os.Getenv("XDG_STATE_HOME")
	if lk == "" {
		lk = HOME + "/.local/state"
	}
	kind := map[string]string{}
	for _, r := range readRows(lk + "/hc-lane/lanes.tsv") {
		kind[r.F(1)+"\t"+r.F(2)] = r.F(4)
	}
	LKIND = map[string]bool{}
	for k, v := range kind {
		if v == "read" {
			LKIND[k] = true
		}
	}
	HELD = map[string]string{}
	for _, r := range readRows(DATA + "/held.tsv") {
		if _, ok := HELD[r.F(1)]; !ok {
			HELD[r.F(1)] = r.F(2)
		}
	}
	HNEW, HNOW, SEATS = nil, nowUnix(), nil
	for _, t := range TERMS {
		tm, ta, tn, ts, tc := t.M, t.Addr, t.Name, t.Status, t.Cwd
		pre := fmt.Sprintf("자리 %s %s %s: ", tm, ta, tn)
		if t.Agent != AGENT { // 상태 모르는 hc-lane 레인(띄운 직후·훅 안 보냄)은 살아 있는 에이전트로 셈
			if ts == "-" && has(LANEADDR, ta) {
				say("%s%s, 상태 모름(hc-lane 레인) → 셈", pre, t.Agent)
				SEATS = append(SEATS, tm)
			}
			continue
		}
		if ts != "done" && ts != "idle" {
			SEATS = append(SEATS, tm)
			continue
		}
		hs := HELD[ta]
		if !isDigits(hs) {
			hs = strconv.FormatInt(HNOW, 10)
		}
		HNEW = append(HNEW, ta+"\t"+hs)
		hsn, _ := strconv.ParseInt(hs, 10, 64)
		switch {
		case has(AUDN, tn):
			say("%s%s, 자동 감사 레인 → 안 셈", pre, ts)
			continue
		case has(AWT, ta):
			say("%s%s, 자동 감사 기다림(자리는 감사 줄이) → 안 셈", pre, ts)
			continue
		case has(ALLM, tm):
			SEATS = append(SEATS, tm)
			continue
		}
		if !strings.Contains(tc, ".wt/") {
			say("%s%s, 읽기 전용(작업 트리 아님) → 안 셈", pre, ts)
			continue
		}
		if LKIND[ta+"\t"+tn] {
			say("%s%s, 읽기 레인(레인 기록) → 안 셈", pre, ts)
			continue
		}
		tw := "작업 트리 확인 못 함"
		switch wtThere(tm, tc) {
		case "yes":
			tw = "작업 트리 있음"
		case "no":
			say("%s%s, 작업 트리 없음 → 안 셈", pre, ts)
			continue
		}
		switch {
		case submitted(tn, ta):
			say("%s%s, 제출함(병합기 큐 — 레인은 끝) → 안 셈", pre, ts)
		case laneLanded(tn, tc):
			say("%s%s, 착지함(%s) → 안 셈", pre, ts, strings.SplitN(LHIT, " ", 2)[0])
		case HOLD != 0 && HNOW-hsn >= int64(HOLD*60):
			say("%s%s, 쉰 지 %d분(hold %d) → 안 셈", pre, ts, (HNOW-hsn)/60, HOLD)
		default:
			say("%s%s, %s → 셈", pre, ts, tw)
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
	HELDW = true
}

// asecText: 자동 감사 쓰기 레인의 run 지시문 끝 절(셸 판 ASEC 그대로)
const asecText = "\n## 감사·착지 (hc-fill 자동)\n" +
	"이 절이 위 지시문 본문의 감사·착지 문구보다 앞선다. 감사는 hc-fill이 띄운다(지휘가 따로 띄우지 않는다).\n" +
	"1. 구현·검증이 끝나면 착지하지 말고 `hc-lane report 완료 <sha> \"<한 줄> (감사 hc-fill 자동)\"`을 부른 뒤 턴을 끝내고 기다린다. 판정이 올 때까지 다른 hc-lane report는 하지 않는다.\n" +
	"2. hc-fill이 「감사 판정이 왔다 — … hc-lane verdict <ID> …」로 깨우면 이 작업 트리에서 `hc-lane verdict <ID>`를 돌리고 그 출력대로만 한다(판정은 도구가 정한다 — 글로 짐작하지 않는다). 같은 글이 또 와도 같은 줄을 다시 돌리면 된다(판정은 한 번만 받힌다).\n" +
	"3. 출력이 「감사 통과 …」면 그 출력의 착지 줄 `hc-lane land <기본> --verdict <ID>`로(위 착지 절차의 land 줄 대신 — 정리 절차는 그대로) main에 넣고 `hc-lane report 착지 <sha>`. 병합기가 켜진 저장소면 착지 줄이 큐에 제출만 하고 끝난다 — 그때 착지는 병합기가 알리니 report 착지 없이 위 정리 절차의 `hc-lane clean` 줄을 바로 돌리고 끝낸다(제출 뒤 이 레인이 할 일은 없다).\n" +
	"4. 출력이 「감사 지적 … <보고서>」면 그 보고서를 읽고 높음·중간을 고친 뒤 다시 검증하고 `hc-lane report 완료 <새 sha> \"<한 줄>\"`, 그리고 다시 기다린다. 출력이 「지금 커밋으로 hc-lane report 완료 …」면 그 줄을 부르고 기다린다.\n"
