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
	"herdr-tools/internal/opsstate"
)

func init() {
	register("machine", machineCmd)
	register("machines", machineCmd)
}

// mLoad: 한 기계의 1분 부하와 코어 수. OK가 거짓이면 모름(한가함이 아니다). At은 읽은 때(status 표시용, 0이면 표시 안 함).
// Ver는 같은 때 읽은 도는 저장소 검증 수(VerOK 거짓이면 모름) — 보이는 글만, 배정·치우침 판정에는 안 쓴다(ht-skew-verify-cause)
type mLoad struct {
	L1    float64
	Cores int
	OK    bool
	At    int64
	Ver   int
	VerOK bool
	// Gates: Ver 가운데 병합기 검증(추측 일꾼·착지 실행 — --spec-idle의 gates=, ht-merger-gate-slot). 보이는 글만 — 자리·레인상한에는
	// 안 든다(병합기 터미널은 hc-lane 라벨 없는 env 터미널이라 countSeats가 안 센다, ht-fill-gate-seat). GatesOK 거짓이면 모름(옛 판 워커)
	Gates   int
	GatesOK bool
}

// Per: 코어당 부하(모름이면 0 — 부르는 쪽은 OK를 먼저 본다)
func (l mLoad) Per() float64 {
	if !l.OK || l.Cores <= 0 {
		return 0
	}
	return l.L1 / float64(l.Cores)
}

// verPtr: 도는 검증 수(모름이면 nil — 상태 한 장 칸)
func (l mLoad) verPtr() *int {
	if !l.VerOK {
		return nil
	}
	v := l.Ver
	return &v
}

// gatesPtr: 도는 병합기 검증 수(모름이면 nil)
func (l mLoad) gatesPtr() *int {
	if !l.VerOK || !l.GatesOK {
		return nil
	}
	g := l.Gates
	return &g
}

// loadSnip: 「load <1분 부하> <코어 수>」 한 줄(리눅스 /proc/loadavg·getconf, macOS sysctl). 실패해도 조각을 실패로 만들지 않는다 —
// 꼴이 틀린 줄은 loadParse가 모름으로 읽는다. 한 줄이다(끝 줄바꿈 없음) — 혼자 부를 때는 끝 표식 없이 그대로(잘린 출력은 꼴이 틀려 모름).
// 이어 그 기계의 도는 저장소 검증 수를 병합기 검증 기계 고르기와 같은 명령(hc-lane merger --spec-idle — 「load=… cpus=… verifies=n」)으로
// 한 줄 더(ht-skew-verify-cause — 레인 0인 바쁜 기계의 까닭이 병합기 추측 검증임을 보이게). 못 부르면 그 줄이 없어 검증 수만 모름.
// 같은 호출에 그 기계 repo 줄 폴더들의 읽기 점검(--repo-read — cordon.go). 그 인자를 모르는 옛 판은 거절하니 인자 없이 한 번 더(점검만 모름)
func loadSnip(m string) string {
	rr := ""
	for _, r := range repoPaths(m) {
		rr += " " + shq(r.path)
	}
	return `{ l=$(cut -d' ' -f1 /proc/loadavg 2>/dev/null) || l=$(sysctl -n vm.loadavg | awk '{print $2}'); c=$(getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu); printf 'load %s %s\n' "$l" "$c"; hc-lane merger --spec-idle --repo-read` +
		rr + ` || hc-lane merger --spec-idle; } 2>/dev/null || :`
}

// loadParse: 출력에서 「load」로 시작하는 첫 줄(부하). 꼴이 틀리거나 없으면 모름. --spec-idle 줄이 있으면 그 verifies= 를 검증 수로
func loadParse(out string) mLoad {
	var o mLoad
	seen := false
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if (len(f) == 3 || len(f) == 4) && strings.HasPrefix(f[0], "load=") && strings.HasPrefix(f[2], "verifies=") {
			if v, err := strconv.Atoi(strings.TrimPrefix(f[2], "verifies=")); err == nil && v >= 0 {
				o.Ver, o.VerOK = v, true
				// 새 꼴(ht-merger-gate-slot)은 넷째 칸 gates= — 고치기 전엔 4칸 줄을 통째로 버려 새 판 워커의 검증 수가 모름이었다
				if len(f) == 4 && strings.HasPrefix(f[3], "gates=") {
					if g, err := strconv.Atoi(strings.TrimPrefix(f[3], "gates=")); err == nil && g >= 0 && g <= v {
						o.Gates, o.GatesOK = g, true
					}
				}
			}
			continue
		}
		if seen || len(f) == 0 || f[0] != "load" {
			continue
		}
		seen = true
		if len(f) != 3 {
			return mLoad{}
		}
		l1, err := strconv.ParseFloat(f[1], 64)
		c, err2 := strconv.Atoi(f[2])
		if err != nil || err2 != nil || l1 < 0 || l1 != l1 || c <= 0 {
			return mLoad{}
		}
		o.L1, o.Cores, o.OK = l1, c, true
	}
	if !o.OK {
		return mLoad{}
	}
	return o
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
		if r.F(qState) == "wait" && !strings.HasPrefix(r.F(qAudit), "audit=") {
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
			if out, rc := onm(nil, m, loadSnip(m), "-"); rc == 0 {
				loadTake(m, out)
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

// loadLine: 기계 한 줄 — 이름 · 부하/코어(코어당) · 자리 쓴/상한(쓴 수 모르면 ?) · 빈 자리(그 바퀴 machRoom, 모르면 -1 — 안 적음) ·
// 자리 밖 수(who — loadRec.whoNote) · 실행 중 조정 · 이번 바퀴에 안 띄운 까닭
func loadLine(name string, l mLoad, used, cp, free int, who, adj, why string) string {
	ld := "부하 모름"
	if l.OK {
		ld = fmt.Sprintf("부하 %.1f/%d(코어당 %.2f)", l.L1, l.Cores, l.Per())
		ld += opsstate.VerNote(l.verPtr(), l.gatesPtr())
		if l.At > 0 {
			ld += " " + stDur(time.Now().Unix()-l.At) + " 전"
		}
	}
	u := "?"
	if used >= 0 {
		u = strconv.Itoa(used)
	}
	s := fmt.Sprintf("%s  %s  자리 %s/%d", name, ld, u, cp)
	if free >= 0 { // 레인상한·멈춤까지 본 띄울 수 있는 수 — advise 빈 자리와 같은 값
		s += fmt.Sprintf(" · 빈 %d", free)
	}
	s += who
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
	es, cf := knobEntries(), knobFile()
	now := time.Now()
	var o []string
	for _, k := range []string{"seats@", "pause@", "light_only@"} {
		e, ok := es[k+m]
		if !ok || !e.Live(now) { // 손잡이가 없으면 설정 파일에 정해 둔 값
			c, ok := cf[k+m]
			if !ok {
				continue
			}
			e = knob.Entry{Value: c.Value, By: "설정 파일", Why: c.Why}
		}
		if k == "light_only@" && e.Value != "1" {
			continue
		}
		u := "계속"
		if e.Until != 0 {
			u = stamp(e.Until) + "까지"
		}
		t := "자리 " + e.Value
		switch k {
		case "pause@":
			t = "멈춤"
		case "light_only@":
			t = "가벼운 일만"
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
	var pin []string      // 묶인 기계(--only)가 상한·멈춤으로 빠진 까닭 — 정적 후보가 모두 그렇면 ONLYFULL(pinned.go)
	var lightOut []string // 보통 레인이라 뺀 가벼운 일만 받는 기계
	defer func() { pinFinish(only, isa, nstatic, pin, cands) }()
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
		if !isa && !LIGHT && machLight(m) { // 가벼운 일만 받는 기계(weight.go) — 보통 레인의 정적 후보가 아니다(능력이 없는 기계처럼)
			mwhy(m, "가벼운 일만")
			lightOut = append(lightOut, m)
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
		if n, why := machRoom(mc, isa); n <= 0 { // 빈 자리 셈은 machRoom 하나(바퀴 기록·advise·gc와 같은 판정)
			switch why {
			case "연결 안 됨":
				if !inlist(m, OFF) {
					say("건너뜀 %s: 연결 안 됨", m)
					OFF += "," + m
				}
			case "멈춤(조정)":
				pin = append(pin, m+" 멈춤")
			case cordonWhy:
				pin = append(pin, m+" 격리")
			case "자리 참":
				full = m
				pin = append(pin, pinNote(m, "상한"))
			case "레인상한 참":
				pin = append(pin, pinNote(m, "상한"))
			}
			if why != "" {
				mwhy(m, why)
			}
			continue
		}
		cands, repos[m] = append(cands, mc), repo
	}
	if !isa {
		cands = steer(lane, cands, nstatic > 1)
		if LIGHT {
			cands = lightFirst(cands)
		}
	}
	if nstatic == 0 && len(lightOut) > 0 {
		say("기다림 %s: 가벼운 기계만 남음(%s — light_only) · 보통 레인이다. 가벼운 레인이면 hc-fill edit %s --light", lane, strings.Join(lightOut, ","), lane)
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

// loadEnd: 바퀴 끝 — 이번 바퀴 부하·자리·까닭을 load.tsv에, 치우침이 skew_min 이어지고 한가한 기계가 있으면 지휘에 사건 한 번
// (skew_every 간격). 이번 바퀴에 못 읽은 기계는 지난 값(읽은 때와 함께)을 남기고 바쁨 시작은 그대로 둔다(모름은 모름)
func loadEnd(now time.Time) {
	old := loadRecs()
	busy := float64(knobInt("load_busy")) / 100
	every := time.Duration(knobInt("skew_every")) * time.Minute
	smin := int64(knobInt("skew_min")) * 60
	k := 0
	for _, r := range ROWS {
		if r.F(qState) == "wait" && !strings.HasPrefix(r.F(qAudit), "audit=") && !inlist(r.F(qName), UPNOW) {
			k++
		}
	}
	var idle []string
	ms := machines()
	for _, mc := range ms {
		if l := LOADS[mc.Name]; l.OK && busy > 0 && l.Per() <= busy {
			if n, _ := machRoom(mc, false); n > 0 {
				idle = append(idle, mc.Name)
			}
		}
	}
	var b strings.Builder
	var mv []string // 바쁜 기계로만 갈 대기 줄 — 보낼 때 한 번만 센다
	mvOK := false
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
				if !mvOK {
					mv, mvOK = skewMovable(now, busy), true
				}
				// 옮길 일 없음(도는 레인뿐·대기 줄은 한가한 기계로 감)이면 사건 없이 load.tsv·상태 한 장에만(fill-polish-1124)
				msg := fmt.Sprintf("%s 코어당 부하 %.2f %d분째 · 대기 레인 %d · 한가한 기계 %s · 바쁜 기계로만 갈 대기 줄 %d(%s) · %s 도는 레인 %d — 조정은 지휘가(hc-fill machine)",
					m, l.Per(), (now.Unix()-x.hotSince)/60, k, strings.Join(idle, ","), len(mv), strings.Join(mv, ","), m, lanesOn(m))
				if len(mv) > 0 && hcLaneReport("중간", msg, "hc-fill-skew", "", "") == 0 {
					x.sent = now.Unix()
				}
			}
		}
		l1, c, at, ver, gates := "-", "-", "-", "-", "-"
		if x.l.OK {
			l1, c, at = strconv.FormatFloat(x.l.L1, 'f', 2, 64), strconv.Itoa(x.l.Cores), strconv.FormatInt(x.l.At, 10)
			if x.l.VerOK {
				ver = strconv.Itoa(x.l.Ver)
			}
			if g := x.l.gatesPtr(); g != nil {
				gates = strconv.Itoa(*g)
			}
		}
		x.free, _ = machRoom(mc, false) // 이 바퀴 띄우기 판정 그대로의 빈 자리 — 상태 한 장 free, advise는 이것만 읽는다
		x.freeOK = true
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%d\t%d\t%s\t%d\t%d\t%s\t%d\t%s\t%d\t%d\n", m, l1, c, at, x.used, x.cp, dash(flat1(x.why)), x.hotSince, x.sent, ver, x.free, gates,
			cnt(SREST, m), cnt(SOUT, m))
	}
	if dry {
		return
	}
	if err := writeAtomic(DATA+"/load.tsv", b.String()); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
	}
}

// skewMovable: 바퀴 끝 대기 줄 가운데 자리가 남는 후보(멈춤 아님·자리·레인상한)가 모두 바쁜(코어당 busy 넘음) 줄 —
// 판정은 advise와 같은 opsstate.SkewMovable. 이번 바퀴에 띄운 줄·자동 감사 줄은 뺀다
func skewMovable(now time.Time, busy float64) []string {
	var ws []opsstate.Wait
	for _, r := range ROWS {
		if r.F(qState) == "wait" && !strings.HasPrefix(r.F(qAudit), "audit=") && !inlist(r.F(qName), UPNOW) {
			ws = append(ws, adviseWait(r, now))
		}
	}
	free, hot := map[string]int{}, map[string]bool{}
	for _, mc := range machines() {
		m := mc.Name
		n, _ := machRoom(mc, false)
		if n <= 0 {
			continue
		}
		free[m] = n
		if l := LOADS[m]; l.OK && l.Per() > busy {
			hot[m] = true
		}
	}
	return opsstate.SkewMovable(ws, free, hot)
}

// machineLines: 기계마다 한 줄(설정 machine 차례) — 부하·자리는 마지막 바퀴 것, 조정은 지금 것. withEmpty면 기록이 없어도 찍는다
func machineLines(withEmpty bool) []string {
	recs := loadRecs()
	if len(recs) == 0 && !withEmpty {
		return nil
	}
	var o []string
	rn, cn := reachNotes(), cordonNotes() // 끊김(unreach.go)·격리(cordon.go)
	for _, mc := range machines() {
		x, ok := recs[mc.Name]
		used := x.used
		if !ok {
			used = -1
		}
		o = append(o, loadLine(mc.Name, x.l, used, int(num(mc.Cap)), x.freeN(), x.whoNote(), machAdj(mc.Name), x.why)+rn[mc.Name]+cn[mc.Name])
	}
	return o
}
