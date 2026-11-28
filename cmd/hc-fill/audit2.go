package main

// 자동 감사 차수 — 셸 판 lib/hc-fill/audit.sh의 a_round·a_queued·a_landed·a_deliver·a_cleanup·a_open·audit_tick.
// Go 판에서 더한 완료 뒤 멈춤 알림은 stall.go

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"herdr-tools/internal/rest"
)

// a_round: 차수 하나를 본다(상태별). 줄 칸은 au* 변수에
func aRound() {
	auAn = "audit-" + auAid
	switch {
	case auAst == "opening": // 지시문 확정 → 대기열 → queued. 중간에 죽었으면 남은 단계만
		if dry {
			fmt.Println(P + "이을 감사 " + auAn + " (opening)")
			return
		}
		pj := aproj(auAid)
		if !abrief(auAid, auLn, auAm, auSha, pj) || !aPut(auAid, auLn, auAm, pj) {
			return
		}
		aupd(auAid, cv{atState, "queued"})
		fmt.Println("감사 넣음 " + auAn + " ← " + auLn + " " + head(auSha, 8) + " (" + auRnd + "번째, " + auAm + ")")
	case auAst == "queued":
		aQueued()
	case auAst == "deliver":
		aDeliver()
	case strings.HasPrefix(auAst, "escalate:"):
		aSendEsc(auAid)
	default:
		aCleanup()
	}
}

// auVerdict <감사 줄 이름> <aid> <sha40>: 이 차수의 판정 사건 — 이름 · 「판정」 · 6칸이 이 차수 sha · 본문에 「감사 완료 <aid> 」
func auVerdict(an, aid, sha string) string {
	l := ""
	for _, e := range EV {
		r := splitRow(e)
		if r.F(evLane) == an && r.F(evState) == "판정" && r.F(evSHA) == sha && strings.Contains(r.F(evMsg)+" ", "감사 완료 "+aid+" ") {
			l = e
		}
	}
	return l
}

// auSameSha: 셸 판 「case $sha in "$ls"*) [ ${#ls} -ge 7 ] … *) ls=x」의 참(같은 커밋)
func auSameSha(sha, ls string) bool { return strings.HasPrefix(sha, ls) && len(ls) >= 7 }

func aQueued() {
	qr := qrow(auAn)
	if qr == nil {
		aEsc(auAid, "cancelled", auLn+" 감사 "+auRnd+"번째("+auAid+", "+head(auSha, 8)+") 감사 줄이 대기열에서 빠졌다(hc-fill rm) — 지휘 판단")
		return
	}
	qs, qm, qa := qr.F(qState), qr.F(qMach), qr.F(qAddr)
	if !auOK(qm) {
		return
	}
	rep := "~/.local/share/hc-fill/reports/" + auAn + ".md"
	v := auVerdict(auAn, auAid, auSha)
	if v == "" {
		ats := alive(qa)
		if qs != "up" {
			ats = "-"
		}
		switch ats {
		case "gone", "done", "idle":
			if auRebound("판정 없음 버림") { // 레인과 감사 터미널이 함께 죽은 재시동 — 이어 뜬 실행의 새 완료로 새 차수(감사 atp6fql M1)
				return
			}
			n := atoi10(auAnv) + 1
			auAnv = strconv.Itoa(n)
			aupd(auAid, cv{atNoVerdict, auAnv})
			if n >= 3 {
				lx := ""
				for _, e := range EV {
					if r := splitRow(e); r.F(evLane) == auAn {
						lx = r.F(evState) + " " + r.F(evMsg)
					}
				}
				if lx == "" {
					lx = "없음"
				}
				aEsc(auAid, "noverdict", auLn+" 감사 "+auRnd+"번째 판정 없음 — 지휘 판단 (감사 터미널 "+qa+" "+ats+", 마지막 사건 「"+lx+
					"」 — 옛 꼴 판정(차수 ID·40자 sha 없음)은 받지 않는다, "+auAn+" @"+qm+" "+rep+")")
			} else {
				fmt.Printf("감사 판정 없음 %s %s %s — %d/3 바퀴\n", auAn, qa, ats, n)
			}
		default:
			if auAnv != "" && auAnv != "0" {
				aupd(auAid, cv{atNoVerdict, "0"})
			}
		}
		return
	}
	// 부모 실행이 그대로인가(같은 주소·같은 created_ms). 끝남 확정이면 지휘로, 모르면 다음 바퀴
	pr := pget(auPa)
	if pr == pUnknown {
		fmt.Println("감사 판정 미룸 " + auAn + ": 부모 " + auPa + " 를 확인하지 못했다(hc get)")
		return
	}
	if pr == pDone || PRUN != auPrun {
		if auRebound("판정 버림") {
			return
		}
		aEsc(auAid, "parent-ended", auLn+" 감사 "+auRnd+"번째 판정이 왔는데 부모 실행("+auPrun+")이 끝났다 — 지휘 판단 ("+head(auSha, 8)+", "+auAn+" @"+qm+" "+rep+")")
		return
	}
	le := runlast(auLn, auPa, auPrun, "완료")
	if le == "" {
		fmt.Println("감사 판정 미룸 " + auAn + ": 부모 실행의 완료 사건을 못 찾았다")
		return
	}
	if !auSameSha(auSha, evF(le, evSHA)) {
		aupd(auAid, cv{atState, "superseded"})
		fmt.Println("판정 버림 " + auAn + ": " + auLn + " 이 새 완료(" + evF(le, evSHA) + ")를 적었다 — 새 차수로")
		return
	}
	msg := evF(v, evMsg)
	mm := regexp.MustCompile(`^감사 완료 ` + regexp.QuoteMeta(auAid) + ` 높음 ([0-9]+)·중간 ([0-9]+)·낮음 ([0-9]+)$`).FindStringSubmatch(msg)
	if mm == nil {
		aEsc(auAid, "unreadable", auLn+" 감사 "+auRnd+"번째 판정 줄을 못 읽음(「"+msg+"」) — 지휘 판단 ("+auAn+" @"+qm+" "+rep+")")
		return
	}
	h, md, lw := mm[1], mm[2], mm[3]
	hml := "hml=" + h + "." + md + "." + lw // 다음 차수 감사 틀의 {prev_verdict}(prevround.go)
	switch {
	case strings.Trim(h+md, "0") == "":
		auAv = "pass"
	case atoi10(auAfx) < AMAX:
		auAv, auAfx = "fix", strconv.Itoa(atoi10(auAfx)+1)
	default:
		aupd(auAid, cv{atFlags, addFlag(auAfl, hml)})
		aEsc(auAid, "limit", fmt.Sprintf("%s 감사 %s번째 높음 %s·중간 %s — 지휘 판단 (지적 %s번, 상한 audit_max %d, %s @%s %s)", auLn, auRnd, h, md, auAfx, AMAX, auAn, qm, rep))
		return
	}
	if dry {
		fmt.Println(P + "넘길 판정 " + auAn + " → " + auLn + " " + auPa + " (" + auAv + ", 높음 " + h + "·중간 " + md + "·낮음 " + lw + ")")
		return
	}
	auAfl = addFlag(auAfl, hml)
	if auAv == "pass" && atoi10(lw) > 0 { // 낮음 후속(lows.go lowsTick)
		auAfl = addFlag(auAfl, "lows="+strconv.Itoa(atoi10(lw)))
	}
	aupd(auAid, cv{atState, "deliver"}, cv{atFixes, auAfx}, cv{atVerdict, auAv}, cv{atFlags, auAfl}, cv{atNote, rep})
	crash("after-consume")
	fmt.Println("판정 받음 " + auAn + " → " + auLn + " (" + auAv + ", 높음 " + h + "·중간 " + md + "·낮음 " + lw + ")")
	auAst, auAlc, auAlw = "deliver", "", ""
	aDeliver()
}

// auLowAF: tr A-F a-f
func auLowAF(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'F' {
			return r + 'a' - 'A'
		}
		return r
	}, s)
}

// a_landed: 이 차수의 부모 실행이 감사 sha로 「제출」이나 「착지」를 적었나(수거 없이 그 커밋 그대로 넣은 레인).
// ponytail: 사건만 본다 — report 없이 손으로 push한 착지는 못 본다(그때는 noack가 지휘에)
func aLanded() bool {
	for _, s := range []string{"제출", "착지"} {
		h := evF(runlast(auLn, auPa, auPrun, s), evSHA)
		if len(h) < 7 {
			continue
		}
		if strings.HasPrefix(auLowAF(auSha), auLowAF(h)) {
			return true
		}
	}
	return false
}

func aDeliver() {
	if !auOK(auAm) { // 끊긴 동안은 기다린 시간을 세지 않는다
		if auAlc != "" && auAlc != "-" {
			aupd(auAid, cv{atChecked, "-"})
		}
		return
	}
	if auAv == "pass" && aLanded() {
		if dry {
			fmt.Println(P + "수거된 것으로 닫을 " + auAid + " (" + auLn + " 이 수거 없이 제출·착지 " + head(auSha, 8) + ")")
			return
		}
		aupd(auAid, cv{atState, "taken"})
		fmt.Println("판정 수거됨 " + auAn + " → " + auLn + " (수거 없이 제출·착지 " + head(auSha, 8) + ")")
		aCleanupNow()
		return
	}
	if dry {
		fmt.Println(P + "둘 판정 " + auAid + " → " + auAm + ":verdicts/" + auLn + "/")
		return
	}
	auRep = auRest(arow(auAid), atNote)
	// 파일 공개 전에 부모 실행·최신 완료를 재확인한다. 이미 수거했으면 a_void가 그 표시를 보존한다
	pr := pget(auPa)
	if pr == pUnknown {
		return
	}
	if pr == pDone || PRUN != auPrun {
		if aVoid(auAid, auLn, auAm) == avVoid && !auRebound("판정 취소") && !aHeldEnd() {
			aEsc(auAid, "parent-ended", auLn+" 감사 "+auRnd+"번째 판정("+auAv+")을 수거하기 전에 부모 실행("+auPrun+")이 끝났다 — 지휘 판단 ("+auAn+" "+auRep+")")
		}
		return
	}
	le := runlast(auLn, auPa, auPrun, "완료")
	if le == "" {
		fmt.Println("판정 두기 미룸 " + auAid + ": 부모 완료 사건을 확인하지 못했다")
		return
	}
	if !auSameSha(auSha, evF(le, evSHA)) {
		if aVoid(auAid, auLn, auAm) == avVoid {
			aupd(auAid, cv{atState, "superseded"})
			fmt.Println("판정 취소 " + auAn + ": " + auLn + " 이 수거 전에 새 완료를 적었다 — 새 차수로")
		}
		return
	}
	unk, stop := aGate() // 그 완료가 아직 서 있나(stands.go) — 아니면 판정을 두지 않음, 모르면 두되 이 바퀴는 제출·깨우기 없이
	if stop {
		return
	}
	ref, ok := abase(aproj(auAid), auAm)
	if !ok {
		fmt.Println("판정 두기 미룸 " + auAid + ": 기본 가지 조회 실패")
		return
	}
	branch := strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), "origin/")
	vb := sub(fmt.Sprintf("aid\t%s\nlane\t%s\nrun\t%s\nsha\t%s\nverdict\t%s\nreport\t%s\nround\t%s\nbase\t%s\n", auAid, auLn, auPrun, auSha, auAv, auRep, auRnd, branch)) + "\n"
	vo, rc := onm(strings.NewReader(vb), auAm, sVput, "~/.local/share/hc-fill/verdicts/"+auLn, auAid, sha256hex([]byte(vb)), strconv.Itoa(len(vb)))
	if rc != 0 {
		fmt.Println("판정 두기 미룸 " + auAid + " → " + auAm + ": 원격 쓰기 실패, 다음 바퀴에")
		return
	}
	vo = sub(vo)
	crash("after-verdict-file")
	switch {
	case vo == "CONFLICT":
		aEsc(auAid, "conflict", auLn+" 감사 "+auRnd+"번째 판정 파일 자리에 다른 내용이 있다("+auAm+" ~/.local/share/hc-fill/verdicts/"+auLn+"/"+auAid+".md) — 지휘 판단")
		return
	case vo == "BADINPUT":
		fmt.Println("판정 두기 미룸 " + auAid + " → " + auAm + ": 받은 내용이 잘렸다(크기·해시 다름), 다음 바퀴에 다시")
		return
	case strings.HasPrefix(vo, "WROTE"), strings.HasPrefix(vo, "ALREADY"):
	default:
		fmt.Println("판정 두기 미룸 " + auAid + " → " + auAm + ": 알 수 없는 응답 「" + vo + "」")
		return
	}
	mark := F1(strings.Fields(vo), 1)
	asNote, asRest = "", ""
	switch mark {
	case "taken", "landing":
		aupd(auAid, cv{atState, "taken"})
		fmt.Println("판정 수거됨 " + auAn + " → " + auLn + " (" + mark + ")")
		aCleanupNow()
		return
	case "void":
		aupd(auAid, cv{atState, "superseded"})
		return
	case "landing+fill": // 대기열이 시작한 제출이 끝나지 않았다(앞 바퀴가 도중에 죽음) — 에이전트에 넘기고 바로 깨운다(autosubmit.go)
		if F1(strings.Fields(vo), 2) == "yes" { // 끝 표시(.done)가 있다 — 그 land는 끝났다(report 착지는 했는지 모름)
			asDone("앞 바퀴의 대기열 제출이 끝 표시를 남겼다", "?")
			return
		}
		if !asHand() {
			return
		}
		asNote = "대기열이 시작한 제출이 끝나지 않았다(앞 바퀴) — 에이전트에 넘김"
	case "none":
		if !unk && aAutoSubmit(branch) {
			return
		}
	}
	now := nowUnix()
	awt := int64(atoi10(auAwt))
	if isDigits(auAlc) { // 한 바퀴 몫은 10분까지
		d := now - int64(atoi10(auAlc))
		if d > 600 {
			d = 600
		}
		awt += d
	}
	auAwt = strconv.FormatInt(awt, 10)
	aupd(auAid, cv{atWaited, auAwt}, cv{atChecked, strconv.FormatInt(now, 10)})
	alw := int64(0)
	if isDigits(auAlw) {
		alw = int64(atoi10(auAlw))
	}
	if !unk && (now-alw >= int64(WAKE*60) || asNote != "") {
		wn := ""
		if n := asNote + asRest; n != "" { // 둘은 한 바퀴에 함께 서지 않는다
			wn = " — " + n
		}
		if !lalive(auPa, auLn) {
			fmt.Println("깨우기 미룸 " + auAn + ": 레인 터미널 " + auPa + " 이 이 바퀴 목록에 없다")
		} else if auSend(auPa, "hc-fill: 감사 판정이 왔다 — 이 작업 트리에서 hc-lane verdict "+auAid+" 를 돌리고 그 출력대로 한다 (판정 "+auAid+")"+wn) {
			k := strconv.Itoa(atoi10(auAwk) + 1)
			aupd(auAid, cv{atWakes, k}, cv{atWoke, strconv.FormatInt(now, 10)})
			fmt.Println("깨움 " + auLn + " " + auPa + " — 판정 " + auAid + " (" + auAv + ", " + k + "번째)")
		} else {
			fmt.Println("깨우기 실패 " + auLn + " " + auPa + " — 다음 바퀴에(판정은 파일에 있다)")
		}
	}
	if awt >= int64(ACKMAX*60) && !inlist("noack", dash(auAfl)) {
		h := ""
		if alive(auPa) == "working" { // 쉬는 레인은 새 커밋이 있어도 보고 없이 멈춘 것 — 지금처럼 막힘(감사 atp2nmt-bisrf4 낮음 1)
			h = auLaneHead()
		}
		if h != "" && h != auSha { // 통과·지적 뒤 새 커밋을 만들어 검증 중 — 레인은 hc-lane verdict 「지금 커밋으로 완료」대로 기다린다
			if nf := "newhead:" + head(h, 8); !inlist(nf, dash(auAfl)) { // 상태 줄은 sha마다 한 번(낮음 2)
				fmt.Println(auLn + " 새 커밋 " + head(h, 8) + " 검증 중 — 다시 감사 기다림 (판정 " + auAid + " 는 " + head(auSha, 8) + " 것)")
				var fl []string
				for _, x := range words(auAfl) {
					if x != "-" && !strings.HasPrefix(x, "newhead:") {
						fl = append(fl, x)
					}
				}
				auAfl = strings.Join(append(fl, nf), ",")
				aupd(auAid, cv{atFlags, auAfl})
			}
		} else if aesc(auLn, qown(auLn), fmt.Sprintf("%s 감사 %s번째 판정(%s)을 %d분 넘게 수거하지 않는다 — 지휘 판단 (%s: hc-lane verdict %s)", auLn, auRnd, auAv, ACKMAX, auPa, auAid), "hf:"+auAid+":noack") == 0 {
			f := "noack"
			if auAfl != "" {
				f = auAfl + ",noack"
			}
			aupd(auAid, cv{atFlags, f})
			auAfl = f
		}
	}
	aCloseDelivered() // 판정 파일을 둔 뒤 — 끝난 감사 터미널이 레인 작업 트리를 쥐지 않게(auditclose.go)
}

// a_cleanup: 끝난 차수의 감사 줄을 대기열에서 뺀다 — 안 뜬 줄은 바로, 뜬 줄은 그 터미널이 쉬거나 없으면(정리 주소를 지휘에 한 번)
func aCleanupNow() { auAst = "taken"; aCleanup() }

func aCleanup() {
	cn := auAn
	if auPrun == "-" { // 옮긴 옛 기록은 옛 이름 줄
		cn = audname(auLn)
	}
	qr := qrow(cn)
	if qr == nil {
		return
	}
	qs, qm, qa := qr.F(qState), qr.F(qMach), qr.F(qAddr)
	if qr.F(qAudit) != "audit="+auLn {
		return
	}
	if dry {
		fmt.Println(P + "정리할 감사 줄 " + cn + " (" + qs + ")")
		return
	}
	if qs == "up" {
		if !auOK(qm) {
			return
		}
		if lalive(qa, cn) {
			switch alive(qa) {
			case "done", "idle":
				if aCloseTerm(cn, qa) {
					break
				}
				// 내기 직전에 다시 본다 — 바퀴 앞 목록 뒤에 끝났으면(없음·finished·주소를 다른 이름이 씀) 정리할 것이 없다(ht-alert-sources)
				if st := pget(qa); st == pDone || st == pLive && PNAME != cn {
					fmt.Println("감사 터미널 이미 끝남 " + qa + " (" + cn + ") — 정리 알림 없이 줄을 뺀다")
					break
				}
				if aesc(auLn, qown(auLn), "감사 터미널 "+qa+" 정리 필요("+cn+", 차수 "+auAid+" "+auAst+") — 정리할 때 이름과 차수를 다시 확인한다", "hf:"+auAid+":cleanup") != 0 {
					return
				}
			default:
				return
			}
		}
	}
	rc := 1
	if withQ(func() { rc = qRmx(cn, qs) }) == 0 && rc == 0 {
		fmt.Println("감사 줄 뺌 " + cn + " (차수 " + auAid + " " + auAst + ")")
	}
}

// aCloseTerm <감사 이름> <주소>: 판정을 읽은 차수의 감사 터미널을 스스로 닫는다 — 그 주소의 지금 터미널 이름이 이 차수 이름
// (audit-<차수 ID>)이고 실행(<주소>@<created_ms>)이 띄울 때 try.tsv에 확정한 것과 같을 때만, 그 정확한 주소로 hc kill. 다르거나
// 모르거나 kill이 실패하면 false(부른 쪽이 지금처럼 지휘에 알린다). 2026-11-22 지휘가 하루 세 번 손으로 닫았다
func aCloseTerm(cn, qa string) bool {
	run := attemptRun(cn, qa)
	if run == "" || pget(qa) != pLive || PNAME != cn || PRUN != run {
		return false
	}
	if v := rest.Ask(restRun, qa, LOCAL); v.State != rest.Quiet { // 상태 done·idle은 턴 끝일 뿐(ht-rest-one-gate)
		fmt.Println("감사 터미널 남김 " + qa + " (" + cn + "): " + rest.Why(v))
		return false
	}
	if _, rc := (Cmd{Name: "hc", Args: []string{"kill", qa}, Err: errMerge}).Run(); rc != 0 {
		return false
	}
	fmt.Println("감사 터미널 닫음 " + qa + " (" + cn + ", 차수 " + auAid + " " + auAst + ", 실행 " + run + ")")
	return true
}

// auSameRound: 차수 줄의 sha가 완료 sha s로 시작하나(s가 -이면 차수 sha도 -)
func auSameRound(r Row, s string) bool {
	if s == "-" {
		return r.F(atSHA) == "-"
	}
	return strings.HasPrefix(r.F(atSHA), s)
}

// a_open: 쓰기 레인(up·auto)이 그 실행으로 적은 최신 「완료 <sha>」가 아직 감사되지 않았으면 새 차수(완료 뒤 중간 보고가 끼어도)
func aOpen(n, p, m, la string) {
	if lastev(n, la, "완료") == "" || pget(la) != pLive {
		return
	}
	lc := runlast(n, la, PRUN, "완료")
	if lc == "" {
		return
	}
	s := evF(lc, evSHA)
	var mine []Row
	for _, r := range readRows(AT) {
		if r.NF() >= atNF && r.F(atLane) == n {
			mine = append(mine, r)
		}
	}
	for _, r := range mine {
		if r.F(atRun) == PRUN && r.F(atAddr) == la && auSameRound(r, s) {
			return
		}
		// 옮긴 옛 기록이 같은 sha를 이미 다뤘으면 다시 열지 않는다(이미 보낸 판정)
		if s != "-" && r.F(atRun) == "-" && (strings.HasPrefix(r.F(atSHA), s) || strings.HasPrefix(s, r.F(atSHA))) {
			return
		}
	}
	// 이 레인의 끝나지 않은 차수: 감사가 아직 돌면 미룸, 아니면 그 차수를 끝낸다(deliver는 void를 먼저)
	for _, r := range mine {
		oa, osh, om, ost := r.F(atID), r.F(atSHA), r.F(atMach), r.F(atState)
		if oa == "" {
			continue
		}
		switch {
		case ost == "opening" || ost == "queued":
			q := qrow("audit-" + oa)
			if q.F(qState) == "up" {
				switch alive(q.F(qAddr)) {
				case "gone", "done", "idle":
				default:
					fmt.Println("자동 감사 미룸 " + n + ": 새 완료 " + s + " — 지난 감사(" + oa + " " + head(osh, 8) + ")가 아직 돈다")
					return
				}
			}
			if !dry {
				aupd(oa, cv{atState, "superseded"})
			}
		case ost == "deliver":
			if dry {
				continue
			}
			switch aVoid(oa, n, om) {
			case avVoid:
				aupd(oa, cv{atState, "superseded"})
			case avTaken:
			case avUnknown:
				fmt.Println("자동 감사 미룸 " + n + ": 지난 판정 " + oa + " 취소를 확인하지 못했다")
				return
			}
		case strings.HasPrefix(ost, "escalate:"):
			fmt.Println("자동 감사 미룸 " + n + ": 지난 차수 " + oa + " 를 지휘에 넘기는 중")
			return
		}
	}
	aid := newid("a")
	for arow(aid) != nil || exists(DATA+"/briefs/audit/"+aid+".md") {
		aid = newid("a")
	}
	k, f := 0, ""
	for _, r := range readRows(AT) {
		if r.NF() >= atNF && r.F(atLane) == n && r.F(atRun) == PRUN {
			k, f = k+1, r.F(atFixes)
		}
	}
	rnd, afx := strconv.Itoa(k+1), strconv.Itoa(int(num(f))) // 기록이 아직 없으면 0
	// 완료에 sha가 없으면 감사할 커밋을 모른다 — 지휘에 한 번
	if s == "-" {
		if dry {
			fmt.Println(P + "지휘로 넘길 " + n + ": 완료에 sha 없음")
			return
		}
		aadd(aid, n, rnd, PRUN, "-", m, la, "escalate:nosha", afx, "-", "0", "0", "-", "-", "0", "-", "자동 감사 못 함 "+n+": 완료에 sha 없음 — 지휘 판단 ("+la+")")
		aSendEsc(aid)
		return
	}
	// 40자 sha: 부모 기계·부모 작업 트리에서. 「없음」은 rev-parse 종료 1에 오류 출력이 없을 때만, 그 밖은 모름
	cw := laneCwd(la, p, m, n)
	var so string
	var rc int
	if m == LOCAL { // 이 기계: git을 바로(sh 안의 git은 상한을 못 받는다 — Fable F3·아스트라 E01). 같은 셋 가름
		d := cw
		if strings.HasPrefix(d, "~/") {
			d = HOME + d[1:]
		}
		e, r := Cmd{Name: "git", Args: []string{"-C", d, "rev-parse", "--verify", "--quiet", s + "^{commit}"}, Err: errMerge}.Run()
		switch e = strings.TrimSpace(e); {
		case r == 0:
			so = "OK " + e
		case r == 1 && e == "":
			so = "NONE"
		default:
			rc = 2
		}
	} else {
		so, rc = onm(nil, m, `e=$(git -C "$d" rev-parse --verify --quiet "$1^{commit}" 2>&1); r=$?; if [ $r = 0 ]; then echo "OK $e"; elif [ $r = 1 ] && [ -z "$e" ]; then echo NONE; else exit 2; fi`, cw, s)
	}
	if rc != 0 {
		fmt.Println("자동 감사 미룸 " + n + ": " + m + " 에서 완료 sha " + s + " 를 풀지 못했다")
		return
	}
	so = sub(so)
	var s40 string
	switch {
	case strings.HasPrefix(so, "OK "):
		s40 = strings.TrimPrefix(so, "OK ")
	case so == "NONE":
		if dry {
			return
		}
		aadd(aid, n, rnd, PRUN, s, m, la, "escalate:nosha", afx, "-", "0", "0", "-", "-", "0", "-", "자동 감사 못 함 "+n+": 완료 sha "+s+" 가 작업 트리("+cw+")에 없다 — 지휘 판단 ("+la+")")
		aSendEsc(aid)
		return
	default:
		fmt.Println("자동 감사 미룸 " + n + ": 완료 sha 응답 「" + so + "」")
		return
	}
	if len(s40) != 40 || strings.Trim(s40, "0123456789abcdef") != "" {
		fmt.Println("자동 감사 미룸 " + n + ": sha 꼴 「" + s40 + "」")
		return
	}
	if dry {
		fmt.Println(P + "넣을 감사 audit-<새 차수> ← " + n + " " + s40[:8] + " (" + rnd + "번째, " + m + ")")
		return
	}
	// 지시문을 확정하기 전에 차수를 기록해야 재시작해도 같은 ID로 이어 간다
	if appendLine(DATA+"/audit-proj.tsv", aid+"\t"+p) != nil {
		return
	}
	if aadd(aid, n, rnd, PRUN, s40, m, la, "opening", afx, "-", "0", "0", "-", "-", "0", "-", "-") != nil {
		return
	}
	crash("after-opening")
	if !abrief(aid, n, m, s40, p) {
		return
	}
	crash("after-brief")
	if !aPut(aid, n, m, p) {
		return
	}
	crash("after-put")
	aupd(aid, cv{atState, "queued"})
	fmt.Println("감사 넣음 audit-" + aid + " ← " + n + " " + s40[:8] + " (" + rnd + "번째, " + m + ")")
}

// audit_tick: 있는 차수를 잇고(1), 쓰기 줄(up·auto)의 새 완료에 차수를 연다(2)
func auditTick() {
	if AUD == "" || (!nonEmpty(Q) && !nonEmpty(AT)) {
		return
	}
	amigrate()
	if EVBAD { // 사건은 ev_tick이 읽었다
		fmt.Println("자동 감사 미룸: hc-lane events 실패, 다음 바퀴에")
		return
	}
	AUNK = evUnk()
	for _, r := range readRows(AT) {
		if r.NF() < atNF || r.F(atID) == "" {
			continue
		}
		auAid, auLn, auRnd, auPrun, auSha, auAm, auPa, auAst = r.F(atID), r.F(atLane), r.F(atRound), r.F(atRun), r.F(atSHA), r.F(atMach), r.F(atAddr), r.F(atState)
		auAfx, auAv, auAwk, auAwt, auAlc, auAlw, auAnv, auAfl = r.F(atFixes), r.F(atVerdict), r.F(atWakes), r.F(atWaited), r.F(atChecked), r.F(atWoke), r.F(atNoVerdict), r.F(atFlags)
		aRound()
	}
	for _, r := range qRows() {
		if r.F(qState) != "up" || r.F(qAudit) != "auto" || alive(r.F(qAddr)) == "gone" {
			continue
		}
		if !auOK(r.F(qMach)) {
			fmt.Println("자동 감사 미룸 @" + r.F(qMach) + ": 연결 안 됨")
			continue
		}
		aOpen(r.F(qName), r.F(qProj), r.F(qMach), r.F(qAddr))
	}
}
