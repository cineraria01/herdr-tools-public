package main

// 쓰기 소유·겹침 판정·능력 막힘·선행 상태 — 셸 판 lib/hc-fill/overlap.sh의 own_* · owners · ovl_* · nd_block · snap · pred_state · after_tick

import (
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ruleDocs: 모든 지시문이 규칙으로 적는 문서 — 본문 언급(M)은 겹침 근거로 세지 않는다(예약 R·실제 A는 그대로).
// 「ht-merger-spec-reuse 언급 (CONTRACT.md)」 같은 잡음만 나고 진짜 겹침은 못 봤다(ht-add-sibling-notice, 2026-11-25)
var ruleDocs = map[string]bool{"CLAUDE.md": true, "AGENTS.md": true, "CONTRACT.md": true}

// own_store: 쓰기 소유 실행 기록 OREC를 own.tsv에(dry-run은 안 씀)
func ownStore() {
	if dry {
		return
	}
	var keep []Row
	for _, r := range OREC {
		if strings.TrimSpace(r.Line()) != "" {
			keep = append(keep, r)
		}
	}
	if err := writeAtomic(OWNF, rowsText(keep)); err != nil {
		die("쓰기 소유 기록 저장 실패")
	}
}

// own_rel <레인> <주소>: 그 실행 기록을 뺀다
func ownRel(n, a string) {
	var o []Row
	for _, r := range OREC {
		if r.NF() > 0 && !(r.F(owLane) == n && r.F(owAddr) == a) {
			o = append(o, r)
		}
	}
	OREC = o
}

// own_bind <레인> <옛 주소> <새 주소>: 주소를 받은 실행 기록을 새 주소로(같은 새 주소의 옛 기록은 뺌) 그리고 저장
func ownBind(n, a, b string) {
	var o []Row
	for _, r := range OREC {
		if r.F(owLane) == n && r.F(owAddr) == b {
			continue
		}
		if r.F(owLane) == n && r.F(owAddr) == a {
			r = r.Set(owAddr, b)
		}
		if r.NF() > 0 {
			o = append(o, r)
		}
	}
	OREC = o
	ownStore()
}

var ownCopyRe = regexp.MustCompile(`^[a-z0-9_-]+\.[a-zA-Z0-9]{6}$`)

// own_finish: 저장하고, 기록이 가리키지 않는 지시문 사본(<레인>.<6글자>)을 지운다
func ownFinish() {
	if dry {
		return
	}
	ownStore()
	ents, err := os.ReadDir(OWND)
	if err != nil {
		return
	}
	text, ok := readFile(OWNF)
	if !ok {
		return // 기록을 못 읽으면 지우지 않는다(셸 판 awk 종료 2)
	}
	ref := map[string]bool{}
	for _, l := range lines(text) {
		ref[splitRow(l).F(owBrief)] = true
	}
	for _, e := range ents {
		f := OWND + "/" + e.Name()
		if !ownCopyRe.MatchString(e.Name()) || !isFile(f) || ref[f] {
			continue
		}
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
		}
	}
}

// own_pending <레인> <기계> <주소>: 주소를 받기 전 끊긴 띄우기 — 그 기계에 같은 이름의 실행이 보이면 저장해 둔 원래 지시문을 그 주소에 잇는다
func ownPending(n, m, a string) bool {
	if !strings.HasPrefix(a, "pending:") {
		return false
	}
	addr := ""
	for _, t := range TERMS {
		if t.M == m && t.Name == n {
			addr = t.Addr
			break
		}
	}
	if addr == "" {
		return false
	}
	ownBind(n, a, addr)
	return true
}

// ownTemp: mktemp <폴더>/<레인>.XXXXXX 꼴(6글자 영숫자 — own_finish가 이 꼴만 지운다)
func ownTemp(dir, lane string) (*os.File, error) {
	const a = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var err error
	for try := 0; try < 100; try++ {
		b := make([]byte, 6)
		for i := range b {
			k, rerr := rand.Int(rand.Reader, big.NewInt(int64(len(a))))
			if rerr != nil {
				return nil, fmt.Errorf("난수: %w", rerr)
			}
			b[i] = a[k.Int64()]
		}
		f, e := os.OpenFile(dir+"/"+lane+"."+string(b), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if e == nil {
			return f, nil
		}
		if err = e; !os.IsExist(e) {
			break
		}
	}
	return nil, err
}

// ownCopy: 지시문을 사본으로. 사본 경로(만들었으면 — 실패해도)와 성공
func ownCopy(lane, brief string) (string, bool) {
	if os.MkdirAll(OWND, 0o755) != nil {
		return "", false
	}
	f, err := ownTemp(OWND, lane)
	if err != nil {
		return "", false
	}
	c := f.Name()
	src, err := os.Open(brief)
	if err != nil {
		f.Close()
		return c, false
	}
	_, err = io.Copy(f, src)
	src.Close()
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return c, err == nil
}

// own_add <레인> <프로젝트> <기계> <주소> <시작UTC> <지시문> <선행|-> <겹침칸>: 지시문을 OWND에 복사해 기록(dry-run은 복사 없이)
func ownAdd(lane, p, m, addr, start, brief, after, ovk string) {
	c := brief
	if !dry {
		cp, ok := ownCopy(lane, brief)
		switch {
		case ok:
			c = cp
		case strings.HasPrefix(addr, "pending:"):
			if cp != "" {
				os.Remove(cp) // 셸 판은 남겨 두고 own_finish가 치웠다
			}
			die("띄우기 보류: 쓰기 소유 %s 지시문 사본을 못 만듦", lane)
		default:
			if cp != "" {
				os.Remove(cp)
			}
			say("알림: 쓰기 소유 %s 지시문 사본을 못 만듦 — 원본으로 봄(%s)", lane, brief)
		}
	}
	ownRel(lane, addr)
	OREC = append(OREC, Row{lane, p, m, addr, start, c, "," + after + ",", dash(ovk)})
}

// own_done <레인> <주소> <시작UTC>: 그 실행이 착지했으면 참. 사건을 못 읽었으면 거짓(유지)
func ownDone(n, a, start string) bool {
	if EVBAD {
		return false
	}
	lx := joblast(n, a, "")
	if lcCut(lx, evState) == "착지" && !(lcCut(lx, evTime) < start) {
		return true
	}
	// 그 터미널이 아직 사건을 하나도 안 적었으면 같은 이름의 옛 착지로 풀지 않는다. 마지막 사건이 「제출」이면 그 제출 ID가 든 병합기
	// 착지로도 푼다 — 시각과 상관없이
	if sm := sublast(n, a); sm != "" {
		return lcCut(mgres(n, evSid(sm)), evState) == "착지"
	}
	// 제출 ID가 없던 옛 형식은 레인의 완료·제출 보고와 ID 없는 병합기 착지만 잇는다
	f57 := lx
	if strings.Contains(lx, "\t") {
		r := splitRow(lx)
		f57 = r.F(evState)
		if r.NF() >= evMsg {
			f57 += "\t" + r.F(evMsg)
		}
	}
	if !strings.HasPrefix(f57, "완료\t제출") {
		return false
	}
	t := lcCut(lx, evTime)
	mg := mgSet() // 이름 바꾼 기계의 옛 이름도(rename.go)
	for _, l := range EV {
		e := splitRow(l)
		if t != "" && e.F(evLane) == n && e.F(evState) == "착지" && mg[e.F(evMach)] && !strings.Contains(e.F(evMsg), "제출 ID ") && e.F(evTime) > t {
			return true
		}
	}
	return false
}

// tmap <이름>: 이어 붙인 이름이면 받은 줄
func tmap(n string) string {
	for _, r := range THENMAP {
		if r.F(tmName) == n {
			return r.F(tmInto)
		}
	}
	return n
}

// owners <후보> <프로젝트>: 같은 프로젝트의 쓰기 소유 레인 가운데 자기 아니고 이 후보를 (--after 사슬·--then을 건너서라도) 기다리지 않는 것
// 「레인<TAB>겹침칸」
func owners(c, p string) []Row {
	A, T := qGraph()
	var R []Row
	for _, r := range OREC {
		if r.NF() >= owAfter {
			R = append(R, r)
		}
	}
	waits := func(o string) bool { return reach(A, T, o)[c] }
	S := map[string]bool{}
	var out []Row
	for _, f := range R {
		if f.F(owProj) != p || f.F(owLane) == c || S[f.F(owLane)] {
			continue
		}
		S[f.F(owLane)] = true
		if !waits(f.F(owLane)) {
			out = append(out, Row{f.F(owLane), f.F(owOverlap)})
		}
	}
	return out
}

// ovl_check <레인> <프로젝트> <지시문> <선행|-> <감사칸> <겹침칸> <기계제한>: 참 띄워도 됨(경고는 OVWARN에), 거짓 겹침 대기
func ovlCheck(name, p, brief, after, ak, ovk, only string) bool {
	OVWARN, OVKEYS = nil, nil
	if strings.HasPrefix(dash(ak), "audit=") { // 읽기 감사는 통과
		return true
	}
	ovo := owners(name, p)
	so := ""
	for _, r := range ovo {
		if dash(ovk) == "solo" && r.NF() > 0 || r.F(orOverlap) == "solo" {
			so = r.F(orLane)
			break
		}
	}
	if so != "" {
		say("단독 대기 %s: %s — 앞 레인이 끝나면 뜬다", name, so)
		OVWAIT = append(OVWAIT, Row{name, so, "단독", dash(only), p})
		ovqAdd(name, p, brief, ovk, only, nil, []string{so})
		return false
	}
	// 쓰기 소유와 안 겹쳐도 줄 차례가 앞선 겹침 대기 줄과 겹치면 그 뒤(overlap_fair.go) — --overlap-ok 줄은 안 본다
	pass := func() bool { return dash(ovk) == "overlap-ok" || !ovqHold(name, p, brief, ovk, only) }
	if OVM == "off" || dash(ovk) != "-" || len(ovo) == 0 {
		return pass()
	}
	ok := map[string]bool{}
	for _, r := range ovo {
		if r.F(orLane) != "" {
			ok[r.F(orLane)] = true
		}
	}
	var ow []Row
	for _, r := range OWN {
		if r.F(onProj) == p && ok[r.F(onLane)] {
			ow = append(ow, Row{r.F(onLane), r.F(onKind), r.F(onKey)})
		}
	}
	if len(ow) == 0 {
		return pass()
	}
	OVKEYS = candKeys(p, name, brief)
	ov := chainCut(name, ovcmp(ow, OVKEYS)) // 먼저 띄움이면 자기 선행 사슬의 예약은 세지 않는다(overlap_early.go)
	if len(ov) == 0 {
		return pass()
	}
	if OVM == "block" {
		for _, r := range ov {
			if r.F(ovGrade) == "예약" || r.F(ovGrade) == "실제" {
				why := r.F(ovKeys) // 예약은 예전 꼴 그대로(시험·지휘가 읽는 줄), 실제 변경만 「실제 」를 붙인다
				if r.F(ovGrade) == "실제" {
					why = "실제 " + why
				}
				if ACTPARK[p+"\t"+name] { // 주차해 둔 가지: 앞 레인을 먼저 착지시키고 그 위로 이어 띄운다
					say("재개 미룸 %s: %s (%s) — 앞 레인이 착지한 뒤 주차한 가지에서 이어 띄운다", name, r.F(ovLane), why)
				} else {
					say("겹침 대기 %s: %s (%s) — 앞 레인이 착지하면 뜬다", name, r.F(ovLane), why)
				}
				OVWAIT = append(OVWAIT, Row{name, r.F(ovLane), r.F(ovKeys), dash(only), p})
				var by []string
				for _, x := range ov {
					if x.F(ovGrade) != "언급" {
						by = append(by, x.F(ovLane))
					}
				}
				ovqAdd(name, p, brief, ovk, only, OVKEYS, by)
				return false
			}
		}
	}
	if !pass() {
		return false
	}
	OVWARN = ov
	return true
}

// ovl_placed <레인> <프로젝트> <지시문> <선행|-> <감사칸>: 주소를 받은 쓰기 줄의 키를 소유에 더하고(감사 줄은 아님) 경고를 찍고 적는다
func ovlPlaced(name, p, brief, after, ak string) {
	if OVM == "off" || strings.HasPrefix(dash(ak), "audit=") {
		return
	}
	if len(OVKEYS) == 0 {
		OVKEYS = candKeys(p, name, brief)
	}
	for _, k := range OVKEYS {
		OWN = append(OWN, append(Row{p, name}, k...))
	}
	for _, w := range OVWARN {
		say("겹침 경고 %s: %s %s (%s)", name, w.F(ovLane), w.F(ovGrade), w.F(ovKeys))
		if !dry {
			if err := appendLine(DATA+"/overlap-warn.log", strings.Join([]string{nowMin(), name, w.F(ovLane), w.F(ovGrade), w.F(ovKeys)}, "\t")); err != nil {
				fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
			}
		}
	}
}

// nd_block <레인> <기계제한|-> <주인|-> <프로젝트>: 지시문 「필요:」 능력(NEED)을 가진 기계가 (기계 제한 안에) 하나도 없으면 「막힘」 한 줄을
// 찍고 그 레인 주인 지휘에 막힘 사건 한 번(need.tsv — 같은 레인·능력이면 다시 안 보냄)을 보내고 참(대기 그대로)
func ndBlock(lane, only, owner, p string) bool {
	if NEED == "" {
		return false
	}
	for _, mc := range machines() {
		if !(num(mc.Cap) > 0 && (mc.LCap == "-" || num(mc.LCap) > 0)) {
			continue
		}
		if only != "-" && !inlist(mc.Name, only) || repoOf(p, mc.Name) == "" {
			continue
		}
		if mhas(mc.Name, NEED) {
			return false
		}
	}
	nl := "그 능력을 가진 기계가 없다(프로젝트 저장소·쓰기 상한·설정 machine-has)"
	if only != "-" {
		nl = "기계 제한 " + only + " 안에 " + nl
	}
	say("막힘 %s: 필요 %s — %s. 대기로 둠", lane, NEED, nl)
	ns := "0"
	if has(NDOLD, lane+"\t"+NEED+"\t1") {
		ns = "1"
	} else if !dry && aesc(lane, owner, lane+": 필요 "+NEED+" — "+nl+". 설정에 능력을 더하거나 지시문의 「필요:」 줄을 고친다(hc-fill ls)", "") == 0 {
		ns = "1"
	}
	NDNOW = append(NDNOW, lane+"\t"+NEED+"\t"+ns+"\n") // 줄마다 줄바꿈(셸 판과 같은 꼴 — 없으면 둘째 레인부터 바퀴마다 다시 알렸다, Fable F1)
	return true
}

// ovCut15: cut -f 1-5,10- (탭 없는 줄은 통째로, 줄이 없으면 빈 값)
func ovCut15(r Row) string {
	if r == nil {
		return ""
	}
	if r.NF() <= qName {
		return r.Line()
	}
	o := append(Row(nil), r[:min(qOnly, r.NF())]...)
	if r.NF() > qStuck {
		o = append(o, r[qAdded-1:]...)
	}
	return o.Line()
}

// snap <레인> <지시문>: 바퀴 처음에 읽은 줄(ROWS)과 지금 대기열 줄을 견줘(1-5칸·10칸부터) 같으면 지시문을 SB에 복사한다. 바뀌었으면 거짓
func snap(lane, brief string) bool {
	var o, c Row
	for _, r := range ROWS {
		if r.F(qName) == lane {
			o = r
			break
		}
	}
	for _, r := range qRows() {
		if r.F(qName) == lane {
			c = r
			break
		}
	}
	if ovCut15(o) != ovCut15(c) {
		say("대기열 줄이 바뀌었다 %s — 다음 바퀴에 다시 판정", lane)
		return false
	}
	ok := false
	if b, err := os.ReadFile(brief); err == nil {
		if f, err := os.OpenFile(SB, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600); err == nil {
			_, werr := f.Write(b)
			ok = f.Close() == nil && werr == nil && strings.TrimLeft(string(b), " \t\n\v\f\r") != ""
		}
	}
	if !ok {
		say("실패 %s: 지시문 사본을 못 뜸: %s", lane, brief)
	}
	return ok
}

// pred_state <선행> <기다리는 줄 프로젝트>: PSV=「go|stop<TAB>까닭」
func predState(pred, p string) {
	LMS = ""
	switch lcFetch(p, true) {
	case 0:
		switch landingHit(pred) {
		case 0:
			PSV = "go\t들어감(" + strings.SplitN(hit, " ", 2)[0] + ")"
			return
		case 2:
			PSV = "go\t모름(착지 조회 실패)"
			return
		}
	case 2: // fetch 실패·시간 넘김: 기본 가지를 모른다 — 빠진 선행을 「착지 안 하고 끝남」으로 정하지 않는다(gofix-fill F05)
		PSV = "go\t모름(기본 가지 조회 실패)"
		return
	}
	r := qrow(pred)
	if r == nil {
		if tf := thenof(pred); tf != "" { // --then 이름
			r = qrow(tf)
		}
	}
	if r == nil {
		PSV = "stop\t대기열에 없음(착지 안 하고 끝남)"
		return
	}
	pn, pq, pm, pa := r.F(qName), r.F(qState), r.F(qMach), r.F(qAddr)
	switch pq {
	case "wait":
		PSV = "go\t대기열에서 기다림(아직 안 뜸)"
		return
	case "up":
	default:
		PSV = "stop\t대기열 상태 " + pq + "(떠 있지 않음)"
		return
	}
	if pm != LOCAL && !connHas(pm) {
		PSV = "go\t모름(" + pm + " 연결 안 됨)"
		return
	}
	if !lalive(pa, pn) {
		PSV = "stop\t터미널 없음(" + pa + ")"
		return
	}
	if ts := alive(pa); ts != "done" && ts != "idle" {
		PSV = "go\t진행 중(" + ts + ")"
		return
	}
	// 그 레인의 마지막 감사 차수(audit.tsv 17칸 줄): 상태·판정
	s := ""
	for _, a := range readRows(AT) {
		if a.NF() >= atNF && a.F(atLane) == pn {
			s = a.F(atState) + " " + a.F(atVerdict)
		}
	}
	switch {
	case strings.HasPrefix(s, "opening ") || strings.HasPrefix(s, "queued "):
		PSV = "go\t자동 감사 중"
		return
	case s == "deliver pass" || s == "taken pass":
		PSV = "go\t감사 통과 — 착지 중"
		return
	case s == "deliver fix" || s == "taken fix":
		PSV = "go\t감사 지적 고치는 중"
		return
	}
	if !EVBAD {
		l := lastev(pn, pa, "")
		switch e := lcCut(l, evState); e {
		case "완료":
			PSV = "go\t완료 보고(감사·착지 기다림)"
			return
		case "착지", "제출":
			PSV = "go\t" + e + " 보고"
			return
		case "막힘":
			PSV = "stop\t막힘: " + lcCut(l, evMsg)
			return
		}
	}
	hs := HNOW
	for _, h := range HNEW {
		if r := splitRow(h); r.F(hdAddr) == pa {
			if v := r.F(hdSince); isDigits(v) {
				if n, err := strconv.ParseInt(v, 10, 64); err == nil {
					hs = n
				}
			}
			break
		}
	}
	hm := (HNOW - hs) / 60
	if HOLD != 0 && HNOW-hs >= int64(HOLD)*60 {
		PSV = fmt.Sprintf("stop\t쉰 지 %d분 — 사건 없음(hold %d분)", hm, HOLD)
	} else {
		PSV = fmt.Sprintf("go\t쉼(%d분, 사건 없음)", hm)
	}
}
