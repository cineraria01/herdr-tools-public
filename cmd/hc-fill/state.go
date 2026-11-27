package main

// state 명령(2026-11-24 ops-state, docs/ops/state.md — 지휘가 전체를 보려고 명령 예닐곱 개를 손으로 맞췄다): 상태 한 장을 사람용·json으로.
// 읽기만 한다(잠금·상태 파일 쓰기 없음). 재료는 run 바퀴가 남긴 기록(load.tsv 기계 부하·자리·까닭, merger-alive.tsv 병합기, last.tsv
// 레인 마지막 사건, tick.tsv 바퀴 시각)을 다시 쓰고, 새로 부르는 것은 hc ls·hc machine ls 한 번씩(레인 살아 있음·기계 연결·감시)과
// 배포 밀림을 셀 git rev-list(기계 판마다 한 번)뿐이다. 묶고 고르는 것은 internal/opsstate.

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/journal"
	"herdr-tools/internal/opsstate"
	"herdr-tools/internal/resident"
)

func init() { register("state", stateCmd) }

// stateCmd: state [--json] [--all] — 기본은 추린 것(opsstate.Trim, 끝난 레인 창 손잡이 state_done), --all은 전체.
// 지문은 늘 추린 것으로(--all도 같은 지문), 눈에 띄는 것은 보이는 것으로
func stateCmd(args []string) {
	js, all := false, false
	for _, a := range args {
		switch {
		case a == "--json" && !js:
			js = true
		case a == "--all" && !all:
			all = true
		default:
			usage()
		}
	}
	s, f := stateCollectF(time.Now())
	th := stateThresholds()
	tr := opsstate.Trim(s, knobInt("state_done"))
	tr.Notable = stateNotable(tr, f, th)
	tr.Fingerprint = opsstate.Fingerprint(tr)
	if all {
		s.Notable, s.Fingerprint = stateNotable(s, f, th), tr.Fingerprint
	} else {
		s = tr
	}
	var err error
	if js {
		err = opsstate.JSON(os.Stdout, s)
	} else {
		err = opsstate.Human(os.Stdout, s)
	}
	if err != nil {
		die("%v", err)
	}
	exit(0)
}

// stateNotable: 눈에 띄는 것 = 지금 손댈 것(hc-fill advise, docs/ops/advise.md)의 요약 한 줄씩 — 같은 판정을 두 군데 두지 않는다.
// 이 기계 상주 판 점검 결과(hc-sweep이 쓴 한 장 — docs/hc-sweep/resident.md)는 f.Resident로 들어간다
func stateNotable(s opsstate.State, f opsstate.Facts, th opsstate.Thresholds) []string {
	return opsstate.Summaries(opsstate.Advise(s, f, th))
}

func stateThresholds() opsstate.Thresholds {
	return opsstate.Thresholds{StageMin: knobInt("state_stage"),
		SkewHi: float64(knobInt("state_hot")) / 100, SkewLo: float64(knobInt("state_cool")) / 100,
		IdleMin: knobInt("idle"), WaitMin: knobInt("advise_wait"), SlowMin: knobInt("advise_merge_slow"),
		PinMin: knobInt("advise_pinned")}
}

// stateUpLane: 떠 있는(up) 대기열 줄 하나 → 상태 한 장의 레인(stateCollectF의 한 단계). x는 last.tsv 줄(없으면 nil), live는 stLive,
// ma·maOK는 그 프로젝트 병합기 살아 있음 기록(바퀴가 읽은 큐 — 차례), bat은 도는 묶음 덧말(mqBatch, 아니면 빈 값)
func stateUpLane(r Row, x Row, live string, ma maRow, maOK bool, bat string, now time.Time) opsstate.Lane {
	name := r.F(qName)
	pos, known := maPos(ma, maOK, name)
	if !known {
		pos = -1 // stIn.Pos: −1 = 줄을 못 읽음
	}
	in := stIn{State: r.F(qState), Early: r.F(qEarly), Lane: name, Last: x, Pos: pos, Batch: bat, Bg: hangBgWait(name, r.F(qAddr)), Live: live}
	if r.F(qAudit) == "auto" {
		in.Aud = anote(name)
	}
	ln := opsstate.Lane{Name: name, Machine: r.F(qMach), Stage: stStage(in), Open: live != "",
		Queued: ma.St == "alive" && (pos > 0 || bat != "")}
	if pr, u := prOf(name); pr != "" { // PR 거울(pr.go)
		ln.PR = strings.TrimSpace(pr + " " + u)
	}
	if x != nil && x.F(lsState) != "-" {
		ln.LastEvent = strings.Join(strings.Fields(strings.ReplaceAll(x.F(lsState)+" "+x.F(lsSHA)+" "+x.F(lsMsg), " -", "")), " ")
		if a, ok := stateClockAge(x.F(lsWhen), now); ok {
			ln.StageAgeS = ptrI(a)
		}
		// 더 할 일 없음: 마지막 사건이 정리됨·착지이고 터미널이 살아 있지 않다(「?」 모름은 살아 있을 수 있어 끝남이 아니다)
		ln.Done = (x.F(lsState) == "정리됨" || x.F(lsState) == "착지") && live == ""
	}
	return ln
}

// stateLoadFresh: 기계 부하를 읽은 때가 마지막 바퀴 끝에서 이 초 안이면 그 바퀴 값
const stateLoadFresh = 300

func ptrI(i int) *int         { return &i }
func ptrF(f float64) *float64 { return &f }
func ptrB(b bool) *bool       { return &b }

// stateCollect: 모든 칸을 모은다. 재료마다 sources 한 줄(나이·호출 수·초·못 읽은 까닭)
func stateCollect(now time.Time) opsstate.State {
	s, _ := stateCollectF(now)
	return s
}

// stateCollectF: stateCollect와 손댈 것 재료(대기 줄마다 까닭·레인 주소·멈춘 기계·상주 판 점검) — 같은 읽기에서
func stateCollectF(now time.Time) (opsstate.State, opsstate.Facts) {
	f := opsstate.Facts{Addr: map[string]string{}, Paused: map[string]bool{}, Known: adviseKnown(), Pred: map[string]opsstate.Lane{},
		Resident: resident.Show(resident.DirFor(os.Getenv("XDG_STATE_HOME"), HOME), now)}
	s := opsstate.State{Schema: opsstate.Schema, At: now, Queue: opsstate.Queue{Counts: map[string]int{}},
		Machines: []opsstate.Machine{}, Lanes: []opsstate.Lane{}, Merge: []opsstate.MergeLine{}, Deploy: []opsstate.Deploy{},
		Watch: []opsstate.Watch{}, Knobs: []opsstate.Knob{}, Journal: []string{}, Events: []string{}}

	// 바퀴 기록의 나이(tick.tsv 마지막 줄의 끝 초)
	tick := opsstate.Source{Name: "tick"}
	end := stateTickEnd()
	if end > 0 {
		tick.AgeS = ptrI(int(now.Unix() - end))
	} else {
		tick.Why = "바퀴 기록 없음(tick.tsv) — run이 아직 안 돌았거나 다른 기계"
	}

	// 이번에 부르는 것: hc ls·hc machine ls
	hc := opsstate.Source{Name: "hc", AgeS: ptrI(0)}
	t0 := time.Now()
	var terms []lsTerm
	termsOK := false
	var hterms []hcjson.Terminal // 같은 표(병합기 기록의 기다리는 대상 주소가 살아 있나 — mergerrec.go stateRec)
	htermsOK := false
	hc.Calls++
	if out, rc := run("hc", "ls", "--json"); rc == 0 && sub(out) != "" {
		terms, termsOK = lsTerms(sub(out))
		if l, err := hcjson.ParseList([]byte(out)); err == nil {
			hterms, htermsOK = l.Terminals, true
		}
	}
	var conn []string
	connKnown := false
	hc.Calls++
	if raw, rc := run("hc", "machine", "ls", "--json"); rc == 0 {
		conn, connKnown = jsonList(raw, "machines", "name", isConnected)
	}
	hc.Secs = time.Since(t0).Seconds()
	var bad []string
	if !termsOK {
		bad = append(bad, "hc ls 못 읽음")
	}
	if !connKnown {
		bad = append(bad, "hc machine ls 못 읽음")
	}
	hc.Why = strings.Join(bad, " · ")

	// 대기열·레인
	var rows []Row
	for _, l := range readLines(Q) {
		if f := Row(lsSplit(l, qNF)); !strings.HasPrefix(f.F(qAudit), "audit=") {
			rows = append(rows, f)
		}
	}
	last := map[string]Row{}
	for _, r := range readRows(DATA + "/last.tsv") {
		last[r.F(lsName)] = r
	}
	laneOn := map[string]int{}
	var oldest *opsstate.Waiting
	mr := maLoad()                         // 병합기 살아 있음 기록 — 레인 차례(줄)와 아래 병합 줄
	bats := map[string]map[string]string{} // 프로젝트 → 도는 묶음 구성원 덧말(mqBatch, 프로젝트마다 한 번)
	for _, r := range rows {
		name, st := r.F(qName), r.F(qState)
		s.Queue.Counts[st]++
		if st != "up" {
			if st == "wait" {
				if a, ok := stateAddedAge(r.F(qAdded), now); ok && (oldest == nil || a > oldest.AgeS) {
					oldest = &opsstate.Waiting{Lane: name, AgeS: a, Why: stateWaitWhy(r)}
				}
				f.Waits = append(f.Waits, adviseWait(r, now))
			}
			predQueued(f.Pred, predWaitLane(r, now), r.F(qProj), false)
			continue
		}
		f.Addr[name] = r.F(qAddr)
		live := stLive(terms, termsOK, conn, connKnown, r.F(qAddr), name)
		if live != "" && !strings.HasPrefix(live, "?") {
			laneOn[r.F(qMach)]++
		}
		p := r.F(qProj)
		ma, maOK := mr[p]
		if _, ok := bats[p]; !ok {
			bats[p] = mqBatch(ma, now)
		}
		ln := stateUpLane(r, last[name], live, ma, maOK, bats[p][name], now)
		s.Lanes = append(s.Lanes, ln)
		predQueued(f.Pred, ln, r.F(qProj), true)
	}
	s.Queue.Oldest = oldest
	predLanded(&f, now) // 선행 단계 재료(statepred.go)

	// 기계: 설정 machine 차례, 부하·쓴 자리·까닭은 바퀴 기록, 조정은 지금 손잡이
	recs := loadRecs()
	for _, mc := range machines() {
		m := mc.Name
		steer := machAdj(m)
		f.Paused[m] = machPaused(m)
		switch {
		case m != LOCAL && !connKnown:
			x := opsstate.MachineUnknown(m, "연결 모름(hc machine ls 못 읽음)")
			x.Steer = steer
			s.Machines = append(s.Machines, x)
			continue
		case m != LOCAL && !has(conn, m):
			x := opsstate.MachineUnknown(m, "연결 안 됨")
			x.Steer = steer
			s.Machines = append(s.Machines, x)
			continue
		}
		x := opsstate.Machine{Name: m, Reach: "ok", Steer: steer}
		if c, err := strconv.Atoi(mc.Cap); err == nil {
			x.Seats = ptrI(c)
		}
		if c, err := strconv.Atoi(mc.LCap); err == nil {
			x.LaneCap = ptrI(c)
		}
		if termsOK {
			x.Lanes = ptrI(laneOn[m])
		}
		if rec, ok := recs[m]; ok {
			x.Live, x.SkipWhy = ptrI(rec.used), rec.why
			x.Resting, x.Outside = ptrN(rec.rest), ptrN(rec.out) // 자리에 안 드는 것(쉬는 대기열 터미널·대기열 밖 에이전트)
			if rec.freeOK {
				x.Free = ptrI(rec.free) // advise의 빈 자리 — 그 바퀴 띄우기 판정(machRoom) 그대로
			}
			// 마지막 바퀴에 그 기계 부하를 못 읽었으면(loadEnd가 지난 값을 읽은 때와 함께 남긴다) 모름 — 지난 값을 지금 값으로 적지 않는다(감사 atozipa 낮음 2)
			// ponytail: 「그 바퀴」는 바퀴 끝 5분 안에 읽은 것 — 바퀴가 5분 넘게 걸리면 그 바퀴 값도 모름이 된다
			if rec.l.OK && rec.l.At+stateLoadFresh >= end {
				x.Load1, x.Cores, x.LoadPerCore, x.Verifies = ptrF(rec.l.L1), ptrI(rec.l.Cores), ptrF(rec.l.Per()), rec.l.verPtr()
				x.MergeVerifies = rec.l.gatesPtr()
			}
		}
		s.Machines = append(s.Machines, x)
	}
	cds := cordonOnRows() // 격리(cordon.go — 점검 여럿이면 가장 이른 시작·까닭을 「; 」로) — 닿지 않는 기계도 칸은 싣는다
	for i, x := range s.Machines {
		if cs := cds[x.Name]; len(cs) > 0 {
			cd := &opsstate.Cordon{SinceS: max(0, now.Unix()-cs[0].Since), Why: cs[0].Why}
			for _, c := range cs[1:] {
				cd.SinceS, cd.Why = max(cd.SinceS, now.Unix()-c.Since), cd.Why+"; "+c.Why
			}
			s.Machines[i].Cordon = cd
		}
	}

	// 병합 줄: run 바퀴의 병합기 살아 있음 기록과 그 줄에 받아 둔 병합기 기록 한 장(지금 상태 — mergerrec.go). 검증 시간·건너뜀
	// 이력은 병합기 기계 일지에만 — 이 기계 병합기면 로컬 일지에서 읽고(statemerge.go), 다른 기계면 모름
	var ps []string
	for p := range mr {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	mj := stateMergeJournal(mr, rows, hterms, htermsOK)
	for _, p := range ps {
		x := mr[p]
		g := opsstate.MergeLine{Project: p, Merger: x.St}
		if n, err := strconv.Atoi(x.K); err == nil {
			g.Length = ptrI(n)
		}
		if x.Oldest > 0 && g.Length != nil && *g.Length > 0 { // 큐 참조 이름의 초 — status의 「가장 오래」와 같은 값
			g.OldestAgeS, g.OldestLane = ptrI(max(0, int(now.Unix()-x.Oldest))), x.OldLane
		}
		mj(&g, x, now)
		if x.Why != "" && x.Why != "-" {
			g.Why = strings.TrimSuffix(x.Why+" · "+g.Why, " · ")
		}
		s.Merge = append(s.Merge, g)
	}

	s.Watch = stateWatch(rows, terms, termsOK, conn, connKnown)

	// 손잡이(살아 있는 것)
	es := knobEntries()
	var kn []string
	for k, e := range es {
		if e.Live(now) {
			kn = append(kn, k)
		}
	}
	sort.Strings(kn)
	for _, k := range kn {
		e := es[k]
		u := ""
		if e.Until != 0 {
			u = stamp(e.Until)
		}
		s.Knobs = append(s.Knobs, opsstate.Knob{Name: k, Value: e.Value, By: e.By, Why: e.Why, Until: u})
	}

	// 최근 사건(마지막 보고 10)
	jr := stateJournal(&s, &f, now)
	s.Events = stateEvents(last)
	dep := stateDeploy(&s, now)
	f.DeployHold = deployHoldIf(s.Deploy, now)
	s.Sources = []opsstate.Source{tick, hc, dep, jr}
	return s, f
}

// stateJournal: 최근 조치 — 일지 24시간을 f.Acts(손댈 것의 조치함)에, 꼬리 5줄을 s.Journal에. 못 읽으면 재료 journal의 까닭
func stateJournal(s *opsstate.State, f *opsstate.Facts, now time.Time) opsstate.Source {
	jr := opsstate.Source{Name: "journal", AgeS: ptrI(0)}
	js, _, err := journal.Read(journal.Filter{Since: now.Add(-24 * time.Hour)})
	if err != nil {
		jr.Why = err.Error()
		return jr
	}
	f.Acts = js
	for i := max(0, len(js)-5); i < len(js); i++ {
		e := js[i]
		s.Journal = append(s.Journal, e.Time.Local().Format("01-02 15:04")+" "+e.By+" "+e.Tool+" "+e.Action+" "+e.Target+" — "+e.Result)
	}
	return jr
}

// stateWatch: 열린 줄이 있는 주인 지휘마다 그 감시 터미널(hc-lane이 띄우는 이름 lane-watch-<주인>)이 hc ls에 있나.
// Need(손댈 것 「감시 안 돎」만 씀): 열린 줄(watchOwners와 같은 watchOpen — 착지·판정·정리됨·터미널 끝남은 닫힘)이 있고 지휘 터미널이
// hc ls에 있다. 지휘가 사라진 흔적은 그 사건을 다른 살아 있는 지휘의 감시가 받는다(hc-lane watch wfilter, advise-fixes). 지휘 터미널이 있어도
// hc-lane이 「지휘 아님」으로 거절했을 때의 지문 그대로면(watchDead — 에이전트가 끝나 셸만 남음 등) 사라진 흔적과 같다(watch-need-live)
func stateWatch(rows []Row, terms []lsTerm, termsOK bool, conn []string, connKnown bool) []opsstate.Watch {
	ws := []opsstate.Watch{}
	seen, open := map[string]bool{}, map[string]bool{}
	last, dead := watchLast(), watchDead()
	for _, r := range rows {
		o := r.F(qOwner)
		if o == "" || o == "-" || !(r.F(qState) == "up" || r.F(qState) == "wait" && r.F(qEarly) == "parked") {
			continue
		}
		live := ""
		if r.F(qState) == "up" {
			live = stLive(terms, termsOK, conn, connKnown, r.F(qAddr), r.F(qName))
		}
		if watchOpen(r, last[[2]string{r.F(qName), r.F(qRID)}], live) {
			open[o] = true
		}
		if seen[o] {
			continue
		}
		seen[o] = true
		w := opsstate.Watch{Owner: o}
		if termsOK {
			w.Alive = ptrB(false)
			for _, t := range terms {
				if t.Name == watchName(o) {
					w.Alive = ptrB(true)
				}
			}
		}
		ws = append(ws, w)
	}
	for i, w := range ws {
		for _, t := range terms { // 이 기계 터미널은 주소에 기계가 안 붙는다
			ws[i].Need = ws[i].Need || open[w.Owner] && ownerAddrIs(t.Addr, w.Owner) && !ownerNotAgent(dead, w.Owner, t)
		}
	}
	return ws
}

// watchName: hc-lane이 지휘 주소로 짓는 감시 터미널 이름(cmd/hc-lane/launchaux.go watchFor·hcx.go ckey와 같은 규칙)
func watchName(co string) string {
	n := strings.ToLower(ckeyOf(co))
	if len(n) > 21 {
		n = n[:21]
	}
	return "lane-watch-" + n
}

func stateTickEnd() int64 {
	var end int64
	for _, l := range readLines(DATA + "/tick.tsv") {
		if _, _, _, _, ok := tickParse(l); ok {
			end, _ = strconv.ParseInt(strings.SplitN(l, "\t", 2)[0], 10, 64)
		}
	}
	return end
}

// stateAddedAge: 대기열 10칸(이 기계 분 「2006-01-02T15:04」) → 기다린 초
func stateAddedAge(added string, now time.Time) (int, bool) {
	t, err := time.ParseInLocation("2006-01-02T15:04", added, time.Local)
	if err != nil {
		return 0, false
	}
	return int(now.Sub(t).Seconds()), true
}

// stateClockAge: last.tsv 사건 시각(이 기계 「15:04」, 날짜 없음) → 지난 초. 앞날이면 어제로
// ponytail: 날짜가 없어 하루 넘게 머문 단계는 하루 미만으로 읽힌다 — last.tsv에 날짜가 생기면 그것으로
func stateClockAge(hm string, now time.Time) (int, bool) {
	t, err := time.ParseInLocation("15:04", hm, time.Local)
	if err != nil {
		return 0, false
	}
	at := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
	if at.After(now) {
		at = at.Add(-24 * time.Hour)
	}
	return int(now.Sub(at).Seconds()), true
}

// stateWaitWhy: 기다리는 까닭 — 거절 · 겹침 · 필요 능력 · 선행 · 그 밖은 자리·차례(기계별 안 띄운 까닭은 machines[].skip_why)
func stateWaitWhy(r Row) string {
	name := r.F(qName)
	if w := rejWhy(name, r.F(qRID)); w != "" {
		return "거절 " + w
	}
	if x := lsFirst(DATA+"/overlap.tsv", name); x != nil {
		if x.F(wtQueue) == "줄" { // 앞선 대기 줄 뒤(overlap_fair.go)
			return "겹침 앞선 대기 줄 " + x.F(wtFront)
		}
		return "겹침 " + x.F(wtFront)
	}
	if x := lsFirst(DATA+"/need.tsv", name); x != nil {
		return "필요 " + x.F(ndNeed)
	}
	if w, _ := pinOf(name, r.F(qRID)); w != "" {
		return "묶인 기계 " + w
	}
	if a := r.F(qAfter); a != "" && a != "-" {
		return "선행 " + a
	}
	return "자리·차례"
}

// stateEvents: 마지막 보고(last.tsv — run 바퀴가 적음) 가운데 사건 있는 것, 늦은 시각 먼저 10개
// ponytail: last.tsv 시각은 시:분만이라 자정을 넘으면 23:50 사건이 00:10 사건보다 앞에 선다(stateClockAge와 같은 뿌리) — 날짜가 생기면 그것으로
func stateEvents(last map[string]Row) []string {
	var o []string
	for n, x := range last {
		if x.F(lsState) == "-" || x.F(lsWhen) == "?" {
			continue
		}
		o = append(o, strings.Join(strings.Fields(strings.ReplaceAll(x.F(lsWhen)+" "+n+" "+x.F(lsState)+" "+x.F(lsSHA)+" "+x.F(lsMsg), " -", "")), " "))
	}
	sort.Sort(sort.Reverse(sort.StringSlice(o)))
	if len(o) > 10 {
		o = o[:10]
	}
	if o == nil {
		o = []string{}
	}
	return o
}
