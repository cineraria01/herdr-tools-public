package main

// run 한 바퀴 뒷부분: 쓰기 소유 맞추기·대기 줄 띄우기·먼저 띄움 — 셸 판 bin/hc-fill 「run: 한 바퀴」

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func launchRows() {
	ov := conf("overlap")
	if ov == "" {
		ov = "block"
	}
	switch ov {
	case "warn", "block", "off":
	default:
		say("알림: 설정 overlap은 warn|block|off다, block으로: %s", ov)
		ov = "block"
	}
	OVM, OWN, OVWAIT = ov, nil, nil
	LOADS, MWHY = map[string]mLoad{}, map[string]string{} // 부하(loadsteer.go) — actTick이 같은 호출로 읽은 기계부터 채운다
	tickMark("owners")
	syncOwners()
	tickMark("load")
	loadTick()
	NDOLD, NDNOW = readLines(DATA+"/need.tsv"), nil
	ELIST, UPNOW, ECAPHIT, ECAPM = "", "", false, ""
	SEENOFF = nil
	for _, r := range readRows(DATA + "/seen.tsv") {
		if r.F(snMach) != "" && r.F(snMach) != LOCAL && !connHas(r.F(snMach)) {
			SEENOFF = append(SEENOFF, Row{r.F(snMach), r.F(snName), r.F(snAddr)})
		}
	}
	if !dry {
		var b strings.Builder
		for _, t := range TERMS {
			if t.Name != "-" && t.Name != "" {
				b.WriteString(t.M + "\t" + t.Name + "\t" + t.Addr + "\n")
			}
		}
		b.WriteString(rowsText(SEENOFF))
		if err := writeAtomic(DATA+"/seen.tsv", b.String()); err != nil {
			fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
		}
	}
	tickMark("launch")
	LSSNAP = false // 이제 띄운다 — 새 주소는 표에 없으니 pget이 다시 hc get으로 본다
	for _, r := range ROWS {
		waitRow(r)
	}
	tickMark("early")
	earlyRows()
	if !dry {
		for _, w := range []struct {
			f string
			d string
		}{{"overlap.tsv", rowsText(OVWAIT)}, {"need.tsv", strings.Join(NDNOW, "")}} {
			if err := writeAtomic(DATA+"/"+w.f, w.d); err != nil {
				fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
			}
		}
	}
	loadEnd(time.Now())
	tickMark("ownfinish")
	ownFinish()
	tickMark("watchown")
	if !dry {
		watchOwners()
	}
	tickMark("after")
	afterTick()
	tickMark("idle")
	idleTick()
	tickMark("gc")
	gcTick()
}

// syncOwners: 쓰기 소유 실행 기록(OREC) 채움·지움·입양, 기다림 사슬(QEDGES·THENMAP), 소유 키(OWN)
func syncOwners() {
	rows := qRows()
	notAudit := func(r Row) bool { return !strings.HasPrefix(r.F(qAudit), "audit=") }
	if OSEED { // own.tsv가 없거나 옛 꼴이면 대기열 쓰기 up 줄 주소와 끊긴 기계에 남긴 주소
		for _, r := range rows {
			if (r.F(qState) != "up" && r.F(qState) != "wait") || !notAudit(r) || r.F(qName) == "" {
				continue
			}
			qn, qp, qb, qa, qst, qm, qad, qsk, qad0, qo := r.F(qName), r.F(qProj), r.F(qBrief), r.F(qAfter), r.F(qState), r.F(qMach), r.F(qAddr), r.F(qStuck), r.F(qAdded), dash(r.F(qOverlap))
			if qst == "up" && qad != "-" {
				ownAdd(qn, qp, qm, qad, utcOf(qad0), qb, qa, qo)
			}
			if qsk != "-" {
				for _, e := range words(qsk) {
					m, a, _ := strings.Cut(e, "=")
					if m != LOCAL && !connHas(m) {
						ownAdd(qn, qp, m, a, utcOf(qad0), qb, qa, qo)
					}
				}
			}
		}
	}
	// 지움
	for _, o := range append([]Row(nil), OREC...) {
		rn, rp, rm, ra, rs := o.F(owLane), o.F(owProj), o.F(owMach), o.F(owAddr), o.F(owStart)
		if rn == "" {
			continue
		}
		if rm != LOCAL && !connHas(rm) {
			if OVM != "off" {
				say("겹침: %s @%s 연결 안 됨 — 끝났는지 몰라 쓰기 소유 유지", rn, rm)
			}
			continue
		}
		if ownPending(rn, rm, ra) {
			continue
		}
		if lalive(ra, rn) {
			if !ownDone(rn, ra, rs) {
				continue
			}
		} else {
			switch mqueued(rp, rn) {
			case 0:
				if OVM != "off" {
					say("겹침: %s 병합기 큐에 제출됨 — 들어가기 전이라 쓰기 소유 유지", rn)
				}
				continue
			case 2:
				if OVM != "off" {
					say("겹침: %s 병합기 큐를 못 읽음 — 들어갔는지 몰라 쓰기 소유 유지", rn)
				}
				continue
			}
		}
		ownRel(rn, ra)
	}
	// 입양: 대기열 쓰기 줄과 같은 이름의 살아 있는 터미널(기록이 없고 그 줄을 넣은 뒤 착지 전인 것)
	for _, r := range rows {
		st := r.F(qState)
		if (st != "up" && st != "wait" && st != "sent") || !notAudit(r) || r.F(qName) == "" {
			continue
		}
		qn, qp, qb, qa, qad0, qo := r.F(qName), r.F(qProj), r.F(qBrief), r.F(qAfter), r.F(qAdded), dash(r.F(qOverlap))
		for _, t := range TERMS {
			if t.Name != qn {
				continue
			}
			if LKIND[t.Addr+"\t"+qn] {
				continue
			}
			rec := false
			for _, o := range OREC {
				if o.F(owLane) == qn && o.F(owAddr) == t.Addr {
					rec = true
				}
			}
			if rec {
				continue
			}
			xs := utcOf(qad0)
			if ownDone(qn, t.Addr, xs) {
				continue
			}
			ownAdd(qn, qp, t.M, t.Addr, xs, qb, qa, qo)
		}
	}
	QEDGES, THENMAP = nil, nil
	for _, r := range rows {
		if r.NF() == 0 || !notAudit(r) {
			continue
		}
		QEDGES = append(QEDGES, Row{r.F(qName), "," + r.F(qAfter) + ","})
		for _, n := range thenNames(r.F(qBrief)) {
			if strings.TrimSpace(n) != "" {
				THENMAP = append(THENMAP, Row{n, r.F(qName)})
			}
		}
	}
	okey := false
	if OVM != "off" {
		for _, r := range ROWS {
			if r.F(qState) == "wait" && notAudit(r) && r.F(qOverlap) != "overlap-ok" && r.F(qOverlap) != "solo" {
				okey = true
			}
		}
	}
	ACT, ACTPARK = nil, nil
	if okey {
		actTick(rows) // 실제 변경(키 A) — 못 읽은 레인은 지시문 키만
		for _, o := range append([]Row(nil), OREC...) {
			if o.F(owLane) == "" {
				continue
			}
			for _, k := range append(lkeys(o.F(owProj), o.F(owBrief)), actKeys(o.F(owProj), o.F(owLane))...) {
				OWN = append(OWN, Row{o.F(owProj), o.F(owLane), k.F(kyKind), k.F(kyKey)})
			}
		}
	}
}

// curRid <이름>: 지금 대기열 그 줄의 줄 ID(17칸)
func curRid(n string) string {
	if r := qrow(n); r != nil {
		return r.F(qRID)
	}
	return ""
}

func stuckAlive(sa string) bool {
	out, _ := run("hc", "get", sa, "--json")
	vs, ok := decodeAll(out)
	if !ok || len(vs) == 0 {
		return false
	}
	_, isObj := jget(vs[len(vs)-1], "status").(map[string]any)
	return isObj
}

// waitRow: 대기열 줄 하나(셸 판 run 고리 본문)
func waitRow(r Row) {
	name, prj, brief, after, only, state := r.F(qName), r.F(qProj), r.F(qBrief), r.F(qAfter), r.F(qOnly), r.F(qState)
	a0, reply, owner, ef, ak, ovk, rid := r.F(qAddr), r.F(qReply), r.F(qOwner), r.F(qEarly), r.F(qAudit), r.F(qOverlap), r.F(qRID)
	RID, proj, stuck = rid, prj, r.F(qStuck)
	// 상태가 안 잡혀 남긴 터미널이 살아났으면 같은 레인이 둘 돈다: 알리고 그 레인은 더 띄우지 않는다(지휘가 정리)
	dupr := false
	if stuck != "-" {
		for _, sa := range words(stuck) {
			_, sa, _ = strings.Cut(sa, "=")
			found := false
			for _, t := range TERMS {
				if t.Addr == sa {
					found = true
				}
			}
			if !found || !stuckAlive(sa) {
				continue
			}
			w := ""
			if state != "wait" {
				w = " 지금 떠 있는 " + a0 + " 와 같은 레인이 둘 돈다."
			}
			say("중복 위험 %s %s: 상태가 안 잡혀 남긴 터미널이 살아났다.%s 지휘가 하나를 정리한다", name, sa, w)
			dupr = true
		}
	}
	if state != "wait" {
		return
	}
	if dupr {
		say("건너뜀 %s: 중복 위험이라 더 띄우지 않음", name)
		return
	}
	if _, rr, ok := replyOf(dash(reply)); ok && lget(rr).F(ldState) != "open" {
		say("건너뜀 %s: 받은편지 요청 %s 결과가 이미 고정됐다", name, rr)
		return
	}
	for _, a := range readRows(AT) {
		if a.NF() >= atNF && a.F(atLane) == name && !auditTerm[a.F(atState)] {
			say("건너뜀 %s: 감사 %s 넘기기 중 — 끝난 뒤에", name, a.F(atID))
			return
		}
	}
	NEED = ""
	if !strings.HasPrefix(dash(ak), "audit=") {
		NEED = needs(brief)
	}
	if tryOldBusy(name, rid) {
		say("옛 실행 정리 기다림 %s", name)
		return
	}
	var live *Term
	parkbusy := false
	for i := range TERMS {
		t := &TERMS[i]
		if t.Name != name || t.Addr == "" || inlist(t.M+"="+t.Addr, stuck) {
			continue
		}
		if strings.HasPrefix(dash(ak), "audit=") && (t.Status == "done" || t.Status == "idle") {
			continue
		}
		if !has(LANEADDR, t.Addr) || !inproj(prj, t.M, t.Cwd) {
			say("알림 %s: 같은 이름 터미널 %s 는 hc-lane 레인(%s 폴더)이 아니라 채택하지 않음", name, t.Addr, prj)
			continue
		}
		if dash(ef) == "parked" {
			switch parkCandidate(name, rid, t.M, t.Addr, t.Status) {
			case 1:
				continue
			case 2:
				parkbusy = true
				continue
			}
		}
		live = t
		break
	}
	if live != nil {
		adopt(name, rid, dash(reply), live)
		return
	}
	if parkbusy {
		say("주차 정리 기다림 %s", name)
		return
	}
	if tryUnsure(name, rid) {
		return
	}
	if ndBlock(name, only, dash(owner), prj) {
		return
	}
	for _, s := range SEENOFF {
		if s.F(snName) == name {
			say("건너뜀 %s: 끊긴 기계 %s에 지난 바퀴 같은 이름 터미널(%s)이 있었다 — 끝났는지 몰라 띄우지 않음(다시 붙으면 본다)", name, s.F(snMach), s.F(snAddr))
			return
		}
	}
	if after != "-" {
		if ar := afterOK(name, prj, after); ar != 0 {
			if ar == 1 && dash(ef) == "-" {
				ELIST += "," + name + "=" + bBranch
			}
			return
		}
	}
	if why, b := briefBad(brief); b {
		say("실패 %s: 지시문을 읽지 못함(%s): %s", name, why, brief)
		return
	}
	if !snap(name, brief) || !ovlCheck(name, prj, SB, after, dash(ak), dash(ovk), only) {
		return
	}
	if curRid(name) != rid {
		say("건너뜀 %s: 대기열 줄이 바뀌었다(rm·다시 add) — 다음 바퀴에", name)
		return
	}
	RESUME, RSEC = false, ""
	if dash(ef) == "parked" {
		RESUME, RSEC = true, resumeSec(name, after, bBranch)
	}
	AK, PAFT, PSOLO, PRIO, PADDED = dash(ak), after, dash(ovk), r.F(qPrio), r.F(qAdded)
	place(name, prj, SB, only, dash(reply), dash(owner), "")
	RESUME, RSEC = false, ""
	if placed || GOTADDR {
		ovlPlaced(name, prj, SB, after, dash(ak))
	}
	if !placed && !bad {
		say("자리 없음 %s: 대기", name)
	}
}

// adopt: 같은 이름이 살아 있으면(지휘가 손으로 띄웠거나 지난 바퀴의 것) 다시 띄우지 않고 떠 있음으로 표시
func adopt(name, rid, reply string, t *Term) {
	say("이미 떠 있음 %s: %s — 떠 있음으로 표시", name, t.Addr)
	if dry {
		return
	}
	tg, got := tryget(name, rid, t.M)
	te, tu := "", ""
	if got {
		te, tu, _ = strings.Cut(tg, "\t")
		if te == "-" {
			te = ""
		}
		if tu == "-" {
			tu = ""
		}
	} else if _, _, isReq := replyOf(reply); reply != "-" && !isReq {
		if oldReport(t.M, name) != 0 {
			say("건너뜀 %s: %s 의 옛 보고를 비켜 두지 못했다", name, t.M)
			return
		}
	}
	if pget(t.Addr) != pLive {
		PRUN = "-"
	}
	if tryput(name, rid, t.M, te, tu, t.Addr, PRUN) == 0 {
		qset(true, rid, name, "up", t.M, t.Addr, stuck, te, tu)
	}
	if got {
		e, u := te, tu
		if e == "" {
			e = "아님"
		}
		if u == "" {
			u = "아님"
		}
		say("이미 떠 있음 %s: 띄운 뒤 멈춘 이번 시도 — 시도 기록대로(먼저 %s·감사 %s)", name, e, u)
	}
}

// earlyRows: 띄울 수 있는 것을 다 띄우고도 자리가 남으면 선행에만 묶인 레인을 같은 차례로(설정 early·early_wait)
func earlyRows() {
	ECAP = conf("early")
	ew := conf("early_wait")
	if ew == "" {
		ew = "5"
	}
	if isDigits(ew) {
		ew = fmt.Sprint(atoi10(ew))
	}
	if ECAP != "" && !isDigits(ECAP) {
		say("알림: 설정 early는 개수(숫자)다, 먼저 띄움 끔: %s", ECAP)
		ECAP = "0"
	}
	if !isDigits(ew) || ew == "0" {
		say("알림: 설정 early_wait는 1 이상 시간(숫자)이다, 5로: %s", ew)
		ew = "5"
	}
	EWAIT = atoi10(ew)
	var echain []Row // 이번 바퀴에 선행 예약과 겹친 채 먼저 띄운 줄 「레인 줄ID 앞레인 키」(ls 비고)
	defer func() { earlyChainSave(ROWS, echain) }()
	if ELIST == "" || ECAP == "0" {
		return
	}
	elive := 0
	for _, r := range ROWS {
		if r.F(qState) == "up" && r.F(qEarly) == "early" && lalive(r.F(qAddr), r.F(qName)) {
			elive++
		}
	}
	for _, r := range ROWS {
		name, prj, brief, after, only := r.F(qName), r.F(qProj), r.F(qBrief), r.F(qAfter), r.F(qOnly)
		reply, owner, ak, ovk, rid := r.F(qReply), r.F(qOwner), r.F(qAudit), r.F(qOverlap), r.F(qRID)
		RID, proj, stuck = rid, prj, r.F(qStuck)
		eb := ""
		for _, e := range strings.Split(ELIST, ",") {
			if v, ok := strings.CutPrefix(e, name+"="); ok {
				eb = v
			}
		}
		if eb == "" {
			continue
		}
		if curRid(name) != rid {
			say("먼저 띄움 건너뜀 %s: 대기열 줄이 바뀌었다(rm·다시 add)", name)
			continue
		}
		wq := ""
		for _, a := range words(after) {
			ra, st := a, ""
			inRows := false
			for _, x := range ROWS {
				if x.F(qName) == a {
					inRows = true
				}
			}
			if !inRows {
				ra = tmap(a)
			}
			if inlist(ra, UPNOW) {
				continue
			}
			for _, x := range ROWS {
				if x.F(qName) == ra {
					st = x.F(qState)
					break
				}
			}
			if st == "wait" {
				if wq != "" {
					wq += ","
				}
				wq += a
			}
		}
		if wq != "" {
			say("먼저 띄움 건너뜀 %s: 선행 %s 가 대기열에서 아직 안 떴다(그 선행이 자리를 먼저)", name, wq)
			continue
		}
		if ECAP != "" && elive >= atoi10(ECAP) {
			if !ECAPHIT {
				say("먼저 띄움 상한 %s개(살아 있는 것 %d) — 나머지는 대기", ECAP, elive)
			}
			ECAPHIT, ECAPM = true, ECAPM+","+only
			continue
		}
		if why, b := briefBad(brief); b {
			say("실패 %s: 지시문을 읽지 못함(%s): %s", name, why, brief)
			continue
		}
		OVEARLY, OVCHAIN = true, nil
		ok := snap(name, brief) && ovlCheck(name, prj, SB, after, dash(ak), dash(ovk), only)
		OVEARLY = false
		if !ok {
			continue
		}
		ESEC = earlySec(after, eb)
		NEED = ""
		if !strings.HasPrefix(dash(ak), "audit=") {
			NEED = needs(SB)
		}
		AK, PAFT, PSOLO, PRIO, PADDED = dash(ak), after, dash(ovk), r.F(qPrio), r.F(qAdded)
		place(name, prj, SB, only, dash(reply), dash(owner), after)
		ESEC = ""
		if placed || GOTADDR {
			ovlPlaced(name, prj, SB, after, dash(ak))
		}
		if placed {
			elive++
			for _, c := range OVCHAIN {
				echain = append(echain, Row{name, rid, c.F(ovLane), c.F(ovKeys)})
			}
		}
	}
}
