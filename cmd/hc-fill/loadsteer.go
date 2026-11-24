package main

// 기계 부하 보고 고르기(2026-11-24 fill-load-steer, docs/hc-fill/load-steer.md — 사용자 「기계 자리 설정을 일정 기계에 몰지 말고
// 부하를 확인해서 분산하자」 「기계적으로 딱 고정으로 박아두지 말고 전체적인 판단을 지휘에서」).
// 도구는 재고(1분 부하·코어 수) · 손잡이(load_*·skew_*·seats@<기계>·pause@<기계> — 공통 손잡이 저장) · 알림(치우침 사건 한 번)만 준다.
// 기본 배정은 자리가 남는 기계 가운데 코어당 부하가 낮은 쪽부터, 모르거나 비슷하면 설정 machine 차례. 떠 있는 레인은 건드리지 않는다

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"herdr-tools/internal/knob"
)

func init() {
	register("machine", machineCmd)
	register("machines", machineCmd)
}

// mLoad: 한 기계의 1분 부하와 코어 수. OK가 거짓이면 모름(한가함이 아니다). At은 읽은 때(status 표시용, 0이면 표시 안 함)
type mLoad struct {
	L1    float64
	Cores int
	OK    bool
	At    int64
}

// Per: 코어당 부하(모름이면 0 — 부르는 쪽은 OK를 먼저 본다)
func (l mLoad) Per() float64 {
	if !l.OK || l.Cores <= 0 {
		return 0
	}
	return l.L1 / float64(l.Cores)
}

// loadSnip: 「load <1분 부하> <코어 수>」 한 줄(리눅스 /proc/loadavg·getconf, macOS sysctl). 실패해도 조각을 실패로 만들지 않는다 —
// 꼴이 틀린 줄은 loadParse가 모름으로 읽는다. 한 줄이다(끝 줄바꿈 없음) — 혼자 부를 때는 끝 표식 없이 그대로(잘린 출력은 꼴이 틀려 모름)
const loadSnip = `{ l=$(cut -d' ' -f1 /proc/loadavg 2>/dev/null) || l=$(sysctl -n vm.loadavg | awk '{print $2}'); c=$(getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu); printf 'load %s %s\n' "$l" "$c"; } 2>/dev/null || :`

// loadParse: 출력에서 「load」로 시작하는 첫 줄. 꼴이 틀리거나 없으면 모름
func loadParse(out string) mLoad {
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) == 0 || f[0] != "load" {
			continue
		}
		if len(f) != 3 {
			return mLoad{}
		}
		l1, err := strconv.ParseFloat(f[1], 64)
		c, err2 := strconv.Atoi(f[2])
		if err != nil || err2 != nil || l1 < 0 || l1 != l1 || c <= 0 {
			return mLoad{}
		}
		return mLoad{L1: l1, Cores: c, OK: true}
	}
	return mLoad{}
}

// LOADS: 이번 바퀴에 읽은 부하(기계 → 값). 없는 기계는 모름
var (
	LOADS  map[string]mLoad
	loadMu sync.Mutex
	MWHY   map[string]string // 기계 → 이번 바퀴에 그 기계에 안 띄운 까닭(마지막 것)
	PADDED string            // place 직전 줄의 넣은 때(대기열 10칸) — 모두 바쁨 관문의 기다린 시간
)

func loadPut(m string, l mLoad) {
	loadMu.Lock()
	defer loadMu.Unlock()
	if LOADS == nil {
		LOADS = map[string]mLoad{}
	}
	LOADS[m] = l
}

// loadTick: 기다리는 줄이 있으면, 자리가 남는 후보 기계 가운데 이번 바퀴에 아직 부하를 못 읽은 기계를 기계마다 한 번 동시에 읽는다.
// 실제 변경 조회(actTick — 살아 있는 쓰기 레인이 있는 기계)가 부른 기계는 같은 호출에서 읽었다. 한가한 기계는 그 호출이 없어
// 여기서만 읽힌다(바퀴당 추가 hc exec는 「자리 남음·못 읽음」 기계 수만큼). 자리 참·멈춤·연결 안 됨·repo 줄 없는 기계는 안 읽는다(모름)
func loadTick() {
	wait := false
	for _, r := range ROWS {
		if r.F(6) == "wait" && !strings.HasPrefix(r.F(15), "audit=") {
			wait = true
		}
	}
	if !wait || dry { // dry-run은 hc exec를 하지 않는다 — 미리 보기는 actTick이 읽은 기계 말고는 모름(설정 차례)
		return
	}
	var wg sync.WaitGroup
	for _, mc := range machines() {
		m := mc.Name
		loadMu.Lock()
		_, have := LOADS[m]
		loadMu.Unlock()
		if have || (m != LOCAL && !connHas(m)) || !hasRepoOn(m) || has(AGUNK, m) || machPaused(m) || int(num(mc.Cap))-agentsOn(m) <= 0 {
			continue
		}
		wg.Add(1)
		go func(m string) {
			defer wg.Done()
			if out, rc := onm(nil, m, loadSnip, "-"); rc == 0 {
				if l := loadParse(out); l.OK {
					loadPut(m, l)
				}
			}
		}(m)
	}
	wg.Wait()
}

func hasRepoOn(m string) bool {
	for _, f := range confRows("repo") {
		if F1(f, 2) == m {
			return true
		}
	}
	return false
}

// loadOrder: 부하를 아는 기계끼리만 코어당 부하 낮은 차례로 자리를 바꾼다 — 모르는 기계는 설정 차례 자리 그대로.
// 차이가 band 안이면 비슷함(설정 차례 유지): 앞 기계보다 band 넘게 가벼울 때만 앞으로
func loadOrder(ms []Machine, ld map[string]mLoad, band float64) []Machine {
	var idx []int
	var sorted []Machine
	for i, m := range ms {
		l, ok := ld[m.Name]
		if !ok || !l.OK {
			continue
		}
		idx = append(idx, i)
		j := len(sorted)
		for j > 0 && l.Per()+band < ld[sorted[j-1].Name].Per() {
			j--
		}
		sorted = append(sorted, Machine{})
		copy(sorted[j+1:], sorted[j:])
		sorted[j] = m
	}
	out := append([]Machine(nil), ms...)
	for k, i := range idx {
		out[i] = sorted[k]
	}
	return out
}

// loadGate: 후보가 모두 코어당 부하 busy를 넘으면(하나라도 모르거나 안 넘으면 그대로) 이번 바퀴는 안 띄움 — 다만 starve만큼
// 기다린 줄은 가장 덜 바쁜 기계 하나에(굶지 않게). busy 0이면 끔. 까닭이 빈 글이면 관문을 안 탔다.
// 그 순간 남은 후보가 하나여도(다른 기계는 자리 참·멈춤·연결 안 됨) 관문을 탄다 — 정적 후보가 하나뿐인 줄은 steer가 부르지 않는다
func loadGate(ms []Machine, ld map[string]mLoad, busy float64, waited, starve time.Duration) ([]Machine, string) {
	if busy <= 0 || len(ms) == 0 {
		return ms, ""
	}
	best := -1
	for i, m := range ms {
		l := ld[m.Name]
		if !l.OK || l.Per() <= busy {
			return ms, ""
		}
		if best < 0 || l.Per() < ld[ms[best].Name].Per() {
			best = i
		}
	}
	b := ms[best]
	bp := ld[b.Name].Per()
	if waited >= starve {
		return []Machine{b}, fmt.Sprintf("모두 바쁨(코어당 %.2f 넘음) — %d분 기다려 가장 덜 바쁜 %s(%.2f)에", busy, int(waited.Minutes()), b.Name, bp)
	}
	return nil, fmt.Sprintf("모두 바쁨(코어당 %.2f 넘음, 가장 덜 바쁜 %s %.2f) — %d분째 기다림, %d분이면 가장 덜 바쁜 곳에",
		busy, b.Name, bp, int(waited.Minutes()), int(starve.Minutes()))
}

// skewDue: 치우침 사건을 지금 보내도 되나(처음이거나 되풀이 간격이 지났다)
func skewDue(last int64, now time.Time, every time.Duration) bool {
	return last == 0 || now.Unix()-last >= int64(every.Seconds())
}

// loadLine: 기계 한 줄 — 이름 · 부하/코어(코어당) · 자리 쓴/상한(쓴 수 모르면 ?) · 실행 중 조정 · 이번 바퀴에 안 띄운 까닭
func loadLine(name string, l mLoad, used, cp int, adj, why string) string {
	ld := "부하 모름"
	if l.OK {
		ld = fmt.Sprintf("부하 %.1f/%d(코어당 %.2f)", l.L1, l.Cores, l.Per())
		if l.At > 0 {
			ld += " " + stDur(time.Now().Unix()-l.At) + " 전"
		}
	}
	u := "?"
	if used >= 0 {
		u = strconv.Itoa(used)
	}
	s := fmt.Sprintf("%s  %s  자리 %s/%d", name, ld, u, cp)
	if adj != "" {
		s += "  조정 " + adj
	}
	if why != "" {
		s += "  안 띄움 " + why
	}
	return s
}

// knobInt: 등록 표의 정수 손잡이 — 손잡이 > 설정 > 표 기본. 범위 밖 설정 값은 알리고 기본으로
func knobInt(name string) int {
	d, _ := knob.Find(name)
	v := conf(name)
	if v == "" {
		v = d.Default
	}
	if err := d.Check(v); err != nil {
		say("알림: 설정 %s: %v — 기본 %s로", name, err, d.Default)
		v = d.Default
	}
	n, _ := strconv.Atoi(v)
	return n
}

func machPaused(m string) bool {
	v, ok := knobConf("pause@" + m)
	return ok && v == "1"
}

// machAdj: 그 기계의 살아 있는 조정(자리·멈춤) 글. 없으면 빈 글
func machAdj(m string) string {
	es := knobEntries()
	now := time.Now()
	var o []string
	for _, k := range []string{"seats@", "pause@"} {
		e, ok := es[k+m]
		if !ok || !e.Live(now) {
			continue
		}
		u := "계속"
		if e.Until != 0 {
			u = stamp(e.Until) + "까지"
		}
		t := "자리 " + e.Value
		if k == "pause@" {
			t = "멈춤"
		}
		o = append(o, fmt.Sprintf("%s(%s · %s · %s)", t, e.By, u, e.Why))
	}
	return strings.Join(o, " ")
}

func mwhy(m, w string) {
	if MWHY != nil {
		MWHY[m] = w
	}
}

// placeCands: place의 후보 — 설정 machine 차례에서 기계제한·능력·상태 안 잡힘(stuck)·저장소·연결·유형 모름·멈춤·자리·레인상한을
// 지난 기계와 그 저장소 경로(자동 감사는 그 레인 작업 트리), 자리 참으로 빠진 기계(full — 자동 감사 자리 기다림). 쓰기 줄은 steer를 지난다
func placeCands(lane, p, only string, isa bool) (cands []Machine, repos map[string]string, full string) {
	repos = map[string]string{}
	nstatic := 0
	for _, mc := range machines() {
		m := mc.Name
		if only != "-" && !inlist(m, only) {
			continue
		}
		if !mhas(m, NEED) { // 지시문 「필요:」 능력이 없는 기계
			continue
		}
		repo := repoOf(p, m)
		if repo == "" {
			continue
		}
		nstatic++ // 여기까지는 설정·지시문만 본 정적 후보(--only·능력·repo) — 아래는 그 순간의 상태
		stk := 0
		for _, s := range strings.Split(stuck, ",") {
			if strings.HasPrefix(s, m+"=") {
				stk++
			}
		}
		if stk != 0 {
			continue
		}
		if isa {
			repo = strings.TrimSuffix(repo, "/") + ".wt/" + AK[len("audit="):] // 자동 감사: 그 레인 작업 트리에서 --read
		}
		if m != LOCAL && !connHas(m) {
			if !inlist(m, OFF) {
				say("건너뜀 %s: 연결 안 됨", m)
				OFF += "," + m
			}
			mwhy(m, "연결 안 됨")
			continue
		}
		if has(AGUNK, m) { // 유형 모를 터미널이 있다 — 자리를 셀 수 없다(재감사2 fill/R-203)
			mwhy(m, "유형 모를 터미널")
			continue
		}
		if !isa && machPaused(m) { // 실행 중 멈춤(hc-fill machine --pause) — 떠 있는 레인의 감사는 그대로 띄운다
			mwhy(m, "멈춤(조정)")
			continue
		}
		own := 0
		if isa && RSV[lane] == m { // 자동 감사 줄은 제 몫으로 잡아 둔 자리를 쓴다
			own = 1
		}
		if c, err := strconv.Atoi(mc.Cap); err != nil || agentsOn(m)-own >= c {
			if err == nil {
				full = m
				mwhy(m, "자리 참")
			}
			continue
		}
		// 자동 감사 줄은 레인상한을 보지 않고 레인 수에도 안 든다
		if !isa && mc.LCap != "-" {
			if lc, err := strconv.Atoi(mc.LCap); err != nil || lanesOn(m) >= lc {
				mwhy(m, "레인상한 참")
				continue
			}
		}
		cands, repos[m] = append(cands, mc), repo
	}
	if !isa {
		cands = steer(lane, cands, nstatic > 1)
	}
	return cands, repos, full
}

// steer: place의 후보(설정 차례)를 부하 차례로 세우고 모두 바쁨 관문을 지난다. 자동 감사 줄은 부르지 않는다(그 레인의 기계에)
// gate가 거짓이면(줄의 정적 후보 — --only·「필요:」 능력·repo — 가 하나뿐) 나눌 곳이 없어 모두 바쁨 관문을 안 탄다(지휘 지시·감사 atozgyf).
// 자리 참·멈춤·연결 안 됨으로 그 순간 하나 남은 것은 관문을 탄다
func steer(lane string, cs []Machine, gate bool) []Machine {
	band, busy := float64(knobInt("load_band"))/100, float64(knobInt("load_busy"))/100
	out := loadOrder(cs, LOADS, band)
	var a, b []string
	for i := range cs {
		a, b = append(a, cs[i].Name), append(b, out[i].Name)
	}
	if strings.Join(a, ",") != strings.Join(b, ",") {
		var d []string
		for _, m := range out {
			if l := LOADS[m.Name]; l.OK {
				d = append(d, fmt.Sprintf("%s(%.2f)", m.Name, l.Per()))
			} else {
				d = append(d, m.Name+"(모름)")
			}
		}
		say("부하 차례 %s: %s", lane, strings.Join(d, ","))
	}
	var waited time.Duration
	if t, err := time.ParseInLocation("2006-01-02T15:04", PADDED, time.Local); err == nil && time.Since(t) > 0 {
		waited = time.Since(t)
	}
	if !gate {
		return out
	}
	g, why := loadGate(out, LOADS, busy, waited, time.Duration(knobInt("load_starve"))*time.Minute)
	if why != "" {
		say("부하 %s: %s", lane, why)
		if len(g) == 0 {
			for _, m := range out {
				mwhy(m.Name, "모두 바쁨")
			}
		}
	}
	return g
}

// ---- 상태 파일 <DATA>/load.tsv: 기계 · 1분 부하 · 코어 · 읽은 때 · 쓴 자리 · 상한 · 안 띄운 까닭 · 바쁨 시작 · 치우침 보낸 때 ----

type loadRec struct {
	m              string
	l              mLoad
	used, cp       int
	why            string
	hotSince, sent int64
}

func loadRecs() map[string]loadRec {
	o := map[string]loadRec{}
	for _, r := range readRows(DATA + "/load.tsv") {
		if r.NF() < 9 {
			continue
		}
		x := loadRec{m: r.F(1), why: strings.TrimPrefix(r.F(7), "-")}
		x.l = loadParse("load " + r.F(2) + " " + r.F(3))
		x.l.At, _ = strconv.ParseInt(r.F(4), 10, 64)
		x.used, _ = strconv.Atoi(r.F(5))
		x.cp, _ = strconv.Atoi(r.F(6))
		x.hotSince, _ = strconv.ParseInt(r.F(8), 10, 64)
		x.sent, _ = strconv.ParseInt(r.F(9), 10, 64)
		o[x.m] = x
	}
	return o
}

// loadEnd: 바퀴 끝 — 이번 바퀴 부하·자리·까닭을 load.tsv에, 치우침이 skew_min 이어지고 한가한 기계가 있으면 지휘에 사건 한 번
// (skew_every 간격). 이번 바퀴에 못 읽은 기계는 지난 값(읽은 때와 함께)을 남기고 바쁨 시작은 그대로 둔다(모름은 모름)
func loadEnd(now time.Time) {
	old := loadRecs()
	busy := float64(knobInt("load_busy")) / 100
	every := time.Duration(knobInt("skew_every")) * time.Minute
	smin := int64(knobInt("skew_min")) * 60
	k := 0
	for _, r := range ROWS {
		if r.F(6) == "wait" && !strings.HasPrefix(r.F(15), "audit=") && !inlist(r.F(1), UPNOW) {
			k++
		}
	}
	var idle []string
	ms := machines()
	for _, mc := range ms {
		if l := LOADS[mc.Name]; l.OK && busy > 0 && l.Per() <= busy && !machPaused(mc.Name) && int(num(mc.Cap))-agentsOn(mc.Name) > 0 {
			idle = append(idle, mc.Name)
		}
	}
	var b strings.Builder
	for _, mc := range ms {
		m := mc.Name
		x := old[m]
		x.m, x.used, x.cp, x.why = m, agentsOn(m), int(num(mc.Cap)), MWHY[m]
		if l, ok := LOADS[m]; ok && l.OK {
			x.l, x.l.At = l, now.Unix()
			switch {
			case busy <= 0 || l.Per() <= busy:
				x.hotSince = 0
			case x.hotSince == 0:
				x.hotSince = now.Unix()
			}
			if x.hotSince != 0 && now.Unix()-x.hotSince >= smin && len(idle) > 0 && every > 0 && skewDue(x.sent, now, every) && !dry {
				msg := fmt.Sprintf("%s 코어당 부하 %.2f %d분째 · 대기 레인 %d · 한가한 기계 %s — 조정은 지휘가(hc-fill machine)",
					m, l.Per(), (now.Unix()-x.hotSince)/60, k, strings.Join(idle, ","))
				if hcLaneReport("중간", msg, "hc-fill-skew", "", "") == 0 {
					x.sent = now.Unix()
				}
			}
		}
		l1, c, at := "-", "-", "-"
		if x.l.OK {
			l1, c, at = strconv.FormatFloat(x.l.L1, 'f', 2, 64), strconv.Itoa(x.l.Cores), strconv.FormatInt(x.l.At, 10)
		}
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%d\t%d\t%s\t%d\t%d\n", m, l1, c, at, x.used, x.cp, dash(flat1(x.why)), x.hotSince, x.sent)
	}
	if dry {
		return
	}
	if err := writeAtomic(DATA+"/load.tsv", b.String()); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
	}
}

// machineLines: 기계마다 한 줄(설정 machine 차례) — 부하·자리는 마지막 바퀴 것, 조정은 지금 것. withEmpty면 기록이 없어도 찍는다
func machineLines(withEmpty bool) []string {
	recs := loadRecs()
	if len(recs) == 0 && !withEmpty {
		return nil
	}
	var o []string
	for _, mc := range machines() {
		x, ok := recs[mc.Name]
		used := x.used
		if !ok {
			used = -1
		}
		o = append(o, loadLine(mc.Name, x.l, used, int(num(mc.Cap)), machAdj(mc.Name), x.why))
	}
	return o
}

// machine [<기계> (--seats <n>|- | --pause | --resume) [--for <분>] [--why <글>] [--force]] — 인자 없으면 기계 줄
func machineCmd(args []string) {
	if len(args) == 0 {
		for _, l := range machineLines(true) {
			fmt.Println(l)
		}
		exit(0)
	}
	m := args[0]
	if strings.HasPrefix(m, "-") {
		usage()
	}
	if !machineKnown(m) {
		die("설정에 없는 기계: %s — machine 줄의 이름만", m)
	}
	act, val, why, forM, force := "", "", "", "", false
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--seats" && i+1 < len(args) && act == "":
			act, val = "seats", args[i+1]
			i++
		case (a == "--pause" || a == "--resume") && act == "":
			act = a[2:]
		case (a == "--why" || a == "--for") && i+1 < len(args):
			if a == "--why" {
				why = args[i+1]
			} else {
				forM = args[i+1]
			}
			i++
		case a == "--force":
			force = true
		default:
			usage()
		}
	}
	if act == "" || (forM != "" && (act == "resume" || val == "-")) {
		usage()
	}
	now := time.Now()
	var err error
	k := map[string]string{"seats": "seats@" + m, "pause": "pause@" + m, "resume": "pause@" + m}[act]
	if act == "resume" || val == "-" {
		var a []string
		if a = []string{k}; force {
			a = append(a, "--force")
		}
		err = knobUnset(a, now)
	} else {
		v := val
		if act == "pause" {
			v = "1"
		}
		a := []string{k, v, "--why", why}
		if forM != "" {
			a = append(a, "--for", forM)
		}
		if force {
			a = append(a, "--force")
		}
		err = knobSet(a, now)
	}
	if err != nil {
		die("%v", err)
	}
	what := act
	if act == "seats" {
		what = "seats " + val
	}
	if e := appendLine(DATA+"/machine.log", strings.Join([]string{stampLog(), knobBy(), m, what, dash(flat1(forM)), dash(flat1(why))}, "\t")); e != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: 조정 기록을 남기지 못함: %v\n", e)
	}
	exit(0)
}
