package main

// 병합기 살아 있음(2026-11-24 merger-alive — 병합기는 그 기계의 hc 터미널 하나라 SIGKILL·재부팅·터미널 닫힘이면 막힘 사건 없이 줄이
// 소리 없이 섰다. stallMerger는 큐에 제출이 있으면 「병합기가 맡음」으로 본다).
// 재기: 바퀴마다 이미 읽은 hc ls(TERMS — 살아 있는 터미널만)에서 설정 줄 「merger <프로젝트> <기계>」의 기계에 이름 merger-<프로젝트>
// (또는 merger) 터미널이 있나와, 이미 읽은 원격 큐(MQS)의 제출 수·가장 오래된 제출 시각. 원격 호출을 늘리지 않는다.
// 병합기는 hc-lane merger를 exec한 터미널이라 터미널이 살아 있음 = 병합기 프로세스가 삶(긴 검증 중이어도 산다).
// 알림: 제출이 기다리는데 터미널이 merger_dead분(기본 5) 넘게 없으면 주인 지휘에 막힘 한 번, 모름(기계 연결 안 됨·큐 못 읽음·그 기계
// 목록 항목 모름)은 세지 않되 merger_unknown분(기본 30) 넘게 이어지면 따로 한 번, 알린 뒤 터미널이 다시 보이면 한 줄.
// 손잡이는 상태 폴더(DATA)의 표식 파일 하나씩(2026-11-24 지휘 — 공통 손잡이 저장으로 옮길 때까지 가장 단순한 꼴): merger_dead·
// merger_unknown(내용 = 분, 없으면 기본, 0이면 끔), merger_restart(있으면 켬). 바퀴마다 읽으니 바로 먹는다.
// 다시 띄우기: hc-fill merger restart <프로젝트>(지휘가 한 명령으로), merger_restart가 있으면 알릴 때 스스로 한 번(기본 끔 — 왜 죽었는지
// 모르고 다시 띄우면 같은 실패를 되풀이할 수 있다). 기록 DATA/merger-alive.tsv — status·ls가 읽는다.
// ponytail: 멈춰 선(살아 있는데 바퀴를 안 도는) 병합기는 못 잰다 — 바퀴 시각은 병합기 기계에만 있어 바퀴마다 원격 읽기가 는다

import (
	"fmt"
	"sort"
	"strings"

	"herdr-tools/internal/hcjson"
)

// maRow: DATA/merger-alive.tsv 한 줄 — 프로젝트 기계 상태(alive|gone|unknown) 그 상태 시작(초) 주소 큐수(?=모름) 가장 오래(초|-)
// 확인(초) 알림(,dead,unknown,restart, 목록 — 그 상태 흐름에서 이미 한 것) 까닭
type maRow struct {
	P, M, St        string
	Since           int64
	Addr, K         string
	Oldest, Checked int64
	Alerted, Why    string
}

func maFile() string { return DATA + "/merger-alive.tsv" }

// maKnob <이름> <기본>: 공통 손잡이(hc-fill knob, knobs-migrate)가 있으면 그 값, 없으면 옛 표식 파일 DATA/<이름>의 분(없으면 기본,
// 숫자가 아니면 알림 한 줄과 기본)
func maKnob(k string, def int) int {
	if v, ok := knobConf(k); ok {
		return atoi10(v)
	}
	b, ok := readFile(DATA + "/" + k)
	if !ok {
		return def
	}
	if v := strings.TrimSpace(b); isDigits(v) {
		return atoi10(v)
	}
	say("알림: 손잡이 %s/%s 는 분(숫자)이다, %d으로", DATA, k, def)
	return def
}

// maRestart: 스스로 다시 띄우기 — 공통 손잡이 merger_restart가 있으면 그 값(1 켬·0 끔), 없으면 옛 표식 파일 DATA/merger_restart가 있나
func maRestart() bool {
	if v, ok := knobConf("merger_restart"); ok {
		return v == "1"
	}
	return exists(DATA + "/merger_restart")
}

// maLegacy <이름>: knob ls가 보이는 옛 표식 파일 값(merger_dead·merger_unknown = 내용, merger_restart = 있으면 1). 없으면 빈 값
func maLegacy(k string) string {
	switch k {
	case "merger_restart":
		if exists(DATA + "/merger_restart") {
			return "1"
		}
	case "merger_dead", "merger_unknown":
		if b, ok := readFile(DATA + "/" + k); ok {
			return strings.TrimSpace(b)
		}
	}
	return ""
}

func maLoad() map[string]maRow {
	o := map[string]maRow{}
	for _, r := range readRows(maFile()) {
		al := r.F(9)
		if al == "-" {
			al = ""
		}
		o[r.F(1)] = maRow{r.F(1), r.F(2), r.F(3), int64(atoi10(r.F(4))), r.F(5), r.F(6), int64(atoi10(r.F(7))), int64(atoi10(r.F(8))), al, r.F(10)}
	}
	return o
}

func (x maRow) line() string {
	o := "-"
	if x.Oldest > 0 {
		o = fmt.Sprint(x.Oldest)
	}
	return strings.Join([]string{x.P, x.M, x.St, fmt.Sprint(x.Since), dash(x.Addr), x.K, o, fmt.Sprint(x.Checked), dash(x.Alerted), dash(x.Why)}, "\t")
}

// maNames: 병합기 터미널 이름(docs/hc-lane/merger.md 「준비·띄우기」)
func maNames(p string) []string { return []string{"merger-" + p, "merger"} }

// mqSnap: mergerTick이 이 바퀴에 읽은 큐(MQP·MQS·MQBAD) — 그 뒤 비운다
type mqSnap struct {
	p   string
	s   []Row
	bad string
}

var maSnap mqSnap

// maQueue <프로젝트>: 큐 제출 레인들과 가장 오래된 제출 시각(초). ok=false면 큐를 못 읽음. mergerTick이 이 바퀴에 읽었으면 그것을
func maQueue(p string) (lanes []string, oldest int64, ok bool) {
	if !inlist(p, maSnap.p) {
		mqload(p)
		maSnap.p += "," + p
		for _, r := range MQS {
			if r.F(1) == p {
				maSnap.s = append(maSnap.s, r)
			}
		}
		if inlist(p, MQBAD) {
			maSnap.bad += "," + p
		}
	}
	rows, bad := maSnap.s, maSnap.bad
	if inlist(p, bad) {
		return nil, 0, false
	}
	for _, r := range rows {
		if r.F(1) != p {
			continue
		}
		if loc := mgQueueRe.FindStringIndex(r.F(2)); loc != nil {
			lanes = append(lanes, r.F(2)[loc[1]:])
			s := int64(atoi10(strings.TrimPrefix(r.F(2)[:loc[1]-1], "refs/merge-queue/")))
			if oldest == 0 || s < oldest {
				oldest = s
			}
		}
	}
	return lanes, oldest, true
}

// maOwners: 큐 레인들의 주인 지휘(받은편지 레인·줄 없음은 「-」 = 모든 지휘)
func maOwners(lanes []string) []string {
	var o []string
	for _, n := range lanes {
		own := "-"
		if q := qrow(n); q != nil {
			own = dash(q.F(12))
			if _, _, inbox := replyOf(q.F(11)); inbox {
				own = "-"
			}
		}
		o = append(o, own)
	}
	if len(o) == 0 { // 큐를 못 읽음 — 누구 것인지 모른다
		return []string{"-"}
	}
	o = uniq(o)
	sort.Strings(o)
	return o
}

// mergerAliveTick: stallTick 머리에서 부른다(TERMS·CONN을 이 바퀴에 읽은 뒤 — readTables·mergerTick 다음). 사건이 끊기면 그 알림만
// 다음 바퀴에 다시(hc-lane이 키 「hf:merger:<프로젝트>:<종류>:<그 상태 시작>:<주인>」로 한 번만 보낸다)
func mergerAliveTick() {
	limDead, limUnk := maKnob("merger_dead", 5), maKnob("merger_unknown", 30)
	prev, now := maLoad(), nowUnix()
	var out []maRow
	for _, f := range confRows("merger") {
		if len(f) < 3 {
			continue
		}
		p, m := f[1], f[2]
		lanes, oldest, qok := maQueue(p)
		x := maRow{P: p, M: m, Addr: "-", K: "?", Checked: now, Oldest: oldest}
		if qok {
			x.K = fmt.Sprint(len(lanes))
		}
		unkTerm := false
		for _, t := range TERMS {
			if t.M != m {
				continue
			}
			if has(maNames(p), t.Name) {
				x.Addr = t.Addr
			}
			unkTerm = unkTerm || t.Cwd == "-" // hcjson 모름 항목(살아 있되 칸을 못 읽음 — 병합기일 수 있다)
		}
		switch {
		case x.Addr != "-":
			x.St = "alive"
		case m != LOCAL && !connHas(m):
			x.St, x.Why = "unknown", "기계 연결 안 됨"
		case unkTerm:
			x.St, x.Why = "unknown", "그 기계 터미널 목록 항목 모름"
		case !qok:
			x.St, x.Why = "unknown", "큐 못 읽음"
		default:
			x.St = "gone"
		}
		pv, seen := prev[p]
		x.Since = now
		if seen && pv.St == x.St && pv.M == m {
			x.Since = pv.Since
		}
		if seen && pv.M == m && x.St != "alive" {
			x.Alerted = pv.Alerted
		}
		mins := int((now - x.Since) / 60)
		send := func(state, msg, kind string) bool {
			ok := true
			for _, own := range maOwners(lanes) {
				key := fmt.Sprintf("hf:merger:%s:%s:%d:%s", p, kind, x.Since, own)
				if state == "막힘" {
					ok = aesc("merger-"+p, own, msg, key) == 0 && ok
					continue
				}
				to := own
				if to == "-" {
					to = ""
				}
				if dry {
					fmt.Println(P + "지휘로 넘길 merger-" + p + ": " + msg)
				} else if hcLaneReport(state, msg, "merger-"+p, to, key) != 0 {
					ok = false
				}
			}
			return ok
		}
		qtxt := fmt.Sprintf("큐 %s건", x.K)
		if oldest > 0 {
			qtxt += "(가장 오래 " + stDur(now-oldest) + ")"
		}
		switch {
		case x.St == "alive" && seen && pv.Alerted != "":
			msg := fmt.Sprintf("병합기 %s(%s) 다시 돎 — 터미널 %s, %s", p, m, x.Addr, qtxt)
			say("%s", msg)
			if !send("중간", msg, "up") {
				x.Alerted = pv.Alerted // 다음 바퀴에 다시
			}
		case x.St == "gone" && len(lanes) > 0 && mins >= limDead && limDead > 0 && !inlist("dead", x.Alerted):
			msg := fmt.Sprintf("병합기 %s(%s) %d분째 돌지 않음 — %s. 다시 띄우기: hc-fill merger restart %s", p, m, mins, qtxt, p)
			if maRestart() && !inlist("restart", x.Alerted) && !dry {
				x.Alerted += ",restart"
				rc, why := mergerLaunch(p)
				res := "됨: " + why
				if rc == 0 {
					msg += " — 스스로 다시 띄움: " + why
				} else {
					msg += " — 스스로 다시 띄우기 실패: " + why
					res = "실패: " + why
				}
				jotBy("hc-fill 스스로", "merger restart", p, fmt.Sprintf("%d분째 돌지 않음(merger_restart 켬)", mins), res, "-", qtxt)
			}
			if send("막힘", msg, "dead") {
				x.Alerted += ",dead"
			}
		case x.St == "unknown" && (!qok || len(lanes) > 0) && mins >= limUnk && limUnk > 0 && !inlist("unknown", x.Alerted):
			msg := fmt.Sprintf("병합기 %s(%s) 상태 모름 %d분째(%s) — %s", p, m, mins, x.Why, qtxt)
			if send("막힘", msg, "unknown") {
				x.Alerted += ",unknown"
			}
		}
		out = append(out, x)
	}
	if dry {
		return
	}
	var b strings.Builder
	for _, x := range out {
		b.WriteString(x.line() + "\n")
	}
	if len(out) == 0 && !exists(maFile()) {
		return
	}
	if err := writeAtomic(maFile(), b.String()); err != nil {
		say("병합기 살아 있음 기록을 못 씀: %v", err)
	}
}

// maPrint: status·ls 끝에 프로젝트별 병합기 한 줄(run 바퀴가 적은 기록 — 이 명령은 원격을 더 읽지 않는다)
func maPrint() {
	for _, l := range maLines() {
		fmt.Println(l)
	}
}

func maLines() []string {
	rs := maLoad()
	var ps []string
	for p := range rs {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	now := nowUnix()
	var o []string
	for _, p := range ps {
		x := rs[p]
		st := map[string]string{"alive": "살아 있음(" + x.Addr + ")", "gone": "터미널 없음", "unknown": "모름"}[x.St]
		if st == "" {
			st = x.St
		}
		if x.Why != "" && x.Why != "-" {
			st += "(" + x.Why + ")"
		}
		l := fmt.Sprintf("병합기 %s %s: %s %s째 · 큐 %s건", p, x.M, st, stDur(now-x.Since), x.K)
		if x.Oldest > 0 {
			l += "(가장 오래 " + stDur(now-x.Oldest) + ")"
		}
		l += " · 확인 " + stDur(now-x.Checked) + " 전"
		o = append(o, l)
	}
	return o
}

func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// mergerLaunch <프로젝트>: 지금 띄우는 길 그대로(docs/hc-lane/merger.md 「준비·띄우기」) 한 번 — hc run --bg [--on <기계>] --name <이름>
// --cwd <클론> --no-parent -- zsh -lc 'export HC_LANE_MACHINE=<기계>; exec hc-lane merger --repo <클론>'. 클론·이름은 설정 줄 넷째 칸
// 이후(클론)가 있으면 그것, 없으면 그 기계의 가장 최근 끝난 병합기 터미널(hc ls --all)의 폴더·이름. 살아 있으면 띄우지 않는다.
// --no-parent: 부른 지휘 터미널이 끝나도 병합기가 함께 끝나지 않게(기본은 부모와 함께 끝난다)
func mergerLaunch(p string) (int, string) {
	var row []string
	for _, f := range confRows("merger") {
		if len(f) >= 3 && f[1] == p {
			row = f
			break
		}
	}
	if row == nil {
		return 1, "설정에 merger " + p + " 줄이 없다(병합기가 꺼져 있음)"
	}
	m := row[2]
	clone, name, last := "", "merger-"+p, ""
	for _, l := range confLines {
		if f := fields(l); len(f) >= 4 && f[0] == "merger" && f[1] == p {
			clone = restAfter(l, 3)
			break
		}
	}
	raw, rc := Cmd{Name: "hc", Args: []string{"ls", "--all", "--json"}, Err: errMerge}.Run()
	l, err := hcjson.ParseList([]byte(raw))
	if rc != 0 || err != nil {
		return 1, "hc ls --all을 못 읽음 — 살아 있는지 모르니 띄우지 않는다"
	}
	for _, t := range l.Terminals {
		tm := LOCAL
		if i := strings.IndexByte(t.Address, '/'); i >= 0 {
			tm = t.Address[:i]
		}
		if tm != m {
			continue
		}
		if t.Unknown != "" && t.State == "live" {
			return 1, "그 기계 터미널 " + t.Address + "을 못 읽음 — 살아 있는지 모르니 띄우지 않는다"
		}
		if !has(maNames(p), t.Name) {
			continue
		}
		if t.State == "live" {
			return 1, "이미 돈다: " + t.Address + "(멈출 때는 그 터미널의 정확한 pid로)"
		}
		if t.CwdText != "" && (clone == "" || clone == t.CwdText) {
			name, last = t.Name, t.CwdText // 목록 뒤쪽(나중에 띄운) 것이 이긴다
		}
	}
	if clone == "" {
		clone = last
	}
	if clone == "" {
		return 1, "클론을 모름 — 끝난 병합기 터미널 기록이 없다. 설정 줄을 「merger " + p + " " + m + " <클론>」으로"
	}
	a := []string{"run", "--bg"}
	if m != LOCAL {
		a = append(a, "--on", m)
	}
	a = append(a, "--name", name, "--cwd", clone, "--no-parent", "--", "zsh", "-lc",
		"export HC_LANE_MACHINE="+shq(m)+"; exec hc-lane merger --repo "+shq(clone))
	out, rc := Cmd{Name: "hc", Args: a, Err: errMerge}.Run()
	if rc != 0 {
		return 1, fmt.Sprintf("hc run 실패(종료 %d): %s", rc, flat1(sub(out)))
	}
	return 0, fmt.Sprintf("%s %s(클론 %s)", name, flat1(sub(out)), clone)
}

// mergerRestart: hc-fill merger restart <프로젝트>의 몸통. 0 띄움 · 1 안 띄움
func mergerRestart(p string) int { return mergerRestartWhy(p, "") }

func mergerRestartWhy(p, jwhy string) int {
	rc, why := mergerLaunch(p)
	res := "됨: " + why
	if rc == 0 {
		say("병합기 %s 다시 띄움: %s", p, why)
	} else {
		say("병합기 %s 다시 띄우지 않음: %s", p, why)
		res = "안 띄움: " + why
	}
	jot("merger restart", p, jwhy, res, "-", "")
	return rc
}

func init() { register("merger", mergerCmd) }

// merger restart <프로젝트> [--why <까닭>]
func mergerCmd(args []string) {
	args, why := takeWhy(args)
	if len(args) != 2 || args[0] != "restart" || args[1] == "" {
		usage()
	}
	exit(mergerRestartWhy(args[1], why))
}
