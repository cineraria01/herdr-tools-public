// stats.go: hc-lane stats — 착지 통계(go-land-stats, 2026-11-23). 셸 시절 park/land-stats 설계(구조화된 사건만, 모르는 것은 따로)를
// 지금 Go 판이 이미 남기는 기록만으로 다시 했다: 레인 사건(events.tsv — 완료·제출 submit:<제출 ID>·판정·착지·막힘·정리됨, 10칸 실행 ID)과
// 병합기 일지(merger-journal.tsv — 제출 ID·제출 시각·묶음 크기·착지 시작·끝·결과·후보 SHA·까닭). 새 기록 파일은 없다.
// 정의와 병합기 전 기준선은 docs/land-stats-baseline.md, 쓰는 법은 docs/hc-lane/stats.md
package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/events"
)

func init() { register("stats", thenExit(statsCmd), noHelp) }

const statsDef = "정의: 착지 1건 = 기본 브랜치가 한 번 옮겨 간 것 — 병합기 일지 「착지」 줄을 후보 SHA로 묶은 것과, 병합기 제출이 없는 실행이 보고한 「착지」. " +
	"착지 지표는 착지 시각, 검증 실행은 시작 시각, 정리는 정리 시각으로 기간에 넣는다. 작업 = 레인 터미널 실행(사건 10칸), 완료 → 착지는 직전 착지 뒤 그 작업의 첫 완료부터. " +
	"분위는 정렬 뒤 ⌈p·n⌉번째. 모르는 것은 숫자에 넣지 않고 「모름」에 따로. 병합기 전 기준선은 docs/land-stats-baseline.md"

// stRemote: 원격 수집(sh) — 두 파일을 임시 사본에서 읽어 줄 수를 맞춘다(읽는 사이 더해진 줄이 수를 어긋나게 하지 않게)
// ponytail: --since와 상관없이 파일 전부를 hc exec 출력 한 번으로 받는다(2026-11-23 기계마다 50~200KB, 사건 파일은 돌려 쓰지 않아 계속 큼).
// 출력이 잘리면 END 줄 수가 어긋나 그 기계를 통째로 빼고 종료 1 — 그렇게 되면 원격 sh에서 --since 이후 줄만 보내게 바꾼다
const stRemote = `d=${XDG_STATE_HOME:-$HOME/.local/state}/hc-lane; t=$(mktemp) || exit 1
for f in events.tsv merger-journal.tsv; do echo "== $f"; : > "$t" || exit 1
if [ -f "$d/$f" ]; then cat "$d/$f" > "$t" || exit 1; fi
awk 1 "$t" || exit 1; echo "END $f $(awk 'END{print NR}' "$t")"; done; rm -f "$t"`

func statsCmd(a []string) int {
	e := &ev{ec: "stats", ed: evdir()}
	until := ""
	for i := 0; i < len(a); i++ {
		switch a[i] {
		case "--all":
			e.all = true
		case "--on":
			v := val(a, i)
			if v == "" || strings.ContainsAny(v, "/ ") {
				die("--on 기계 이름이 이상하다: %s", v)
			}
			e.ons = append(e.ons, v)
			i++
		case "--since":
			e.since = sinceArg(val(a, i))
			i++
		case "--until":
			until = sinceArg(val(a, i))
			i++
		default:
			die("알 수 없는 인자: %s", a[i])
		}
	}
	if until == "" {
		until = utcAt(time.Now().Unix()) // 관측 마감은 모으기 전에 고정(그 뒤 사건은 세지 않음)
	}
	e.pick()
	var bad []string
	got, want := 1, 1
	ev, jr, ok := statsLocal(e.ed)
	if !ok {
		bad = append(bad, "이 기계(사건·일지 파일 못 읽음)")
		got = 0
	}
	if e.pbad {
		bad = append(bad, "기계 목록(hc machine ls 실패)")
	}
	for _, m := range e.unk {
		want++
		bad = append(bad, m+"(연결 안 됨)")
	}
	for _, m := range e.ems {
		o, rc := rexec(m, "sh", "-c", stRemote)
		if rc == 9 { // sh를 못 띄우는 기계(레인 기록이 없다) — 대수에 넣지 않는다
			bad = append(bad, m+"(sh 없음 — 대수 밖)")
			continue
		}
		want++
		if rc != 0 {
			bad = append(bad, fmt.Sprintf("%s(hc exec 종료 %d)", m, rc))
			continue
		}
		mev, mjr, err := statsParse(o)
		if err != nil {
			bad = append(bad, m+"("+err.Error()+")")
			continue
		}
		got++
		ev, jr = append(ev, mev...), append(jr, mjr...)
	}
	line := fmt.Sprintf("수집: %d/%d대", got, want)
	if len(bad) > 0 {
		line += " · 수집 못 한 기계: " + strings.Join(bad, " ") + " — 그 기계 것은 하나도 세지 않았다"
	}
	fmt.Println(line)
	fmt.Print(statsReport(ev, jr, e.since, until))
	if got < want || e.pbad {
		return 1
	}
	return 0
}

// statsLocal: 이 기계 두 파일(없으면 빈 것). 하나라도 못 읽으면 둘 다 버리고 false — 반쪽만 세지 않는다
func statsLocal(dir string) (ev, jr []string, ok bool) {
	ev, err := readLines(dir + "/events.tsv")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, false
	}
	jr, err = readLines(dir + "/merger-journal.tsv")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, false
	}
	return ev, jr, true
}

// statsParse: 원격 수집 출력 → 사건 줄·일지 줄. 두 파일의 「END <파일> <줄 수>」가 있고 받은 줄 수가 맞아야 한다(출력 상한에 잘린 것도 걸린다)
func statsParse(o string) (ev, jr []string, err error) {
	var cur *[]string
	name, done := "", 0
	for _, l := range strings.Split(strings.TrimSuffix(o, "\n"), "\n") {
		switch {
		case cur == nil && (l == "== events.tsv" || l == "== merger-journal.tsv"):
			name = l[3:]
			cur = &ev
			if name != "events.tsv" {
				cur = &jr
			}
		case cur != nil && strings.HasPrefix(l, "END "+name+" "):
			n, e := strconv.Atoi(strings.TrimPrefix(l, "END "+name+" "))
			if e != nil || n != len(*cur) {
				return nil, nil, fmt.Errorf("%s 줄 수가 맞지 않음 — 받음 %d, 그 기계 %s", name, len(*cur), strings.TrimPrefix(l, "END "+name+" "))
			}
			cur, done = nil, done+1
		case cur != nil:
			*cur = append(*cur, l)
		}
	}
	if done != 2 || cur != nil {
		return nil, nil, errors.New("수집 출력이 끝까지 오지 않음")
	}
	return ev, jr, nil
}

// stEv: 사건 한 줄(모자란 칸은 「-」)
type stEv struct{ t, term, lane, st, sha, msg, run, key string }

// stLand: 레인 하나의 착지(병합기 일지 「착지」 줄 하나, 또는 레인 직접 착지 보고 하나)
type stLand struct {
	at, subAt  int64
	run, why   string // 작업(실행 ID) · 그 작업을 모르는 까닭
	sub        string // 병합기: 제출 SHA(감사 대조) — 레인 직접이면 빈 값
	merger     bool
	group, key string // 착지 묶음 열쇠(후보 SHA·레인/sha) — 착지 수는 이것으로 한 번
	batch      int
}

func stT(s string) (int64, bool) {
	t, err := time.Parse("2006-01-02T15:04:05Z", s)
	return t.Unix(), err == nil
}

func stCut(f []string, i int) string {
	if i < len(f) && f[i] != "" {
		return f[i]
	}
	return "-"
}

// stDur: 「n · 중앙 · 상위 10%」(초 목록, 분으로)
func stDur(xs []int64) string {
	if len(xs) == 0 {
		return "n 0"
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })
	q := func(p float64) int64 {
		k := int(float64(len(xs))*p + 0.999999)
		if k < 1 {
			k = 1
		}
		return xs[k-1]
	}
	return fmt.Sprintf("n %d · 중앙 %d분 · 상위 10%% %d분", len(xs), (q(0.5)+30)/60, (q(0.9)+30)/60)
}

// stUnk: 「 · 모름 n(까닭 a · 까닭 b)」 — 0인 까닭은 빼고, 모두 0이면 빈 글. 까닭은 주어진 차례대로
func stUnk(order []string, c map[string]int) string {
	n, parts := 0, []string{}
	for _, k := range order {
		if c[k] > 0 {
			n += c[k]
			parts = append(parts, fmt.Sprintf("%s %d", k, c[k]))
		}
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(" · 모름 %d(%s)", n, strings.Join(parts, " · "))
}

var stExit = regexp.MustCompile(`종료 ([0-9]+)`)

// stMergerMsg: 병합기 결과 사건 글(cmd/hc-merger result.go done — 「<까닭> — 제출 <sha8>, <제출 ID x|큐 x>…」)
var stMergerMsg = regexp.MustCompile(` — 제출 [0-9a-f]{8}, (제출 ID |큐 )`)

// stWhy: 돌려보낸 까닭 — 종료 코드(착지 스크립트 약속: 3 충돌 · 5 문서 겹침), 코드가 없으면 글 앞 30바이트
func stWhy(s string) string {
	m := stExit.FindStringSubmatch(s)
	switch {
	case m == nil:
		return stHead(s)
	case m[1] == "3":
		return "충돌(종료 3)"
	case m[1] == "5":
		return "문서 겹침(종료 5)"
	}
	return "종료 " + m[1]
}

func stHead(s string) string {
	r := []rune(s)
	if len(r) > 30 {
		r = r[:30]
	}
	return string(r)
}

// statsReport: 사건 줄·일지 줄 → 표. since가 비면 처음부터, until 미만
func statsReport(evl, jrl []string, since, until string) string {
	in := func(s string) bool { return s != "-" && s >= since && s < until }
	var evs []stEv
	seen := map[string]bool{}
	for _, l := range evl {
		f := strings.Split(l, tab)
		if len(f) < 7 || seen[l] {
			continue
		}
		seen[l] = true
		term := f[2][strings.LastIndex(f[2], "/")+1:] // 원격 줄의 「<기계>/<t…>」도 터미널만
		evs = append(evs, stEv{f[0], f[1] + "/" + term, f[3], f[4], f[5], f[6], stCut(f, 9), stCut(f, 11)})
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].t < evs[j].t })
	subEv := map[string]stEv{}   // 제출 ID → 그 제출 사건
	hasSub := map[string]bool{}  // 제출한 적 있는 작업
	dones := map[string][]stEv{} // 작업 → 완료 사건(시각 순)
	for _, x := range evs {
		switch x.st {
		case "제출":
			// 제출 ID가 없는 손 report 제출은 제출로 치지 않는다(그 실행의 직접 착지를 빼지 않게 — events.SubmitID)
			if id := (events.Event{State: x.st, Msg: x.msg, Key: x.key}).SubmitID(); id != "" {
				subEv[id] = x
				hasSub[x.run] = true
			}
		case "완료":
			dones[x.run] = append(dones[x.run], x)
		}
	}
	// 병합기 일지: 같은 큐·제출 ID·결과는 한 줄(jonce와 같은 열쇠 — 같은 파일을 두 번 받아도)
	type run struct {
		t0, t1 int64
		landed bool
	}
	var lands []stLand
	runs := map[string]*run{}
	runUnk, already, whys, whyOrder := 0, 0, map[string]int{}, []string{}
	jseen := map[string]bool{}
	for _, l := range jrl {
		f := strings.Split(l, tab)
		// 착지·돌려보냄 밖의 바퀴 줄(묶음 실패·lease 바뀜·일시 실패·맨 앞 충돌·추측 기다림 — merger-journal-rounds)은 이 표에 넣지 않는다
		if len(f) < 12 || (f[9] != "착지" && f[9] != "돌려보냄") || jseen[f[1]+tab+f[3]+tab+f[9]] {
			continue
		}
		jseen[f[1]+tab+f[3]+tab+f[9]] = true
		// 「제출이 이미 <기본> 에 있음」·「제출 내용이 이미 … 에 있음」(병합기 existing): 착지를 부르지 않고 정리만 — 기본 브랜치가 안 옮겨 갔다
		if f[9] == "착지" && strings.Contains(f[11], "이미 ") && strings.Contains(f[11], " 에 있음") {
			if lt := orDefault(strings.Trim(f[8], "-"), f[0]); in(lt) {
				already++
			}
			continue
		}
		if f[7] == "-" || f[8] == "-" {
			if (f[9] == "착지" || f[9] == "돌려보냄") && in(f[0]) {
				runUnk++
			}
		} else if in(f[7]) {
			k := f[7] + tab + f[8]
			if runs[k] == nil {
				t0, _ := stT(f[7])
				t1, _ := stT(f[8])
				runs[k] = &run{t0: t0, t1: t1}
			}
			if f[9] == "착지" {
				runs[k].landed = true
			} else if f[9] == "돌려보냄" {
				w := stWhy(f[11])
				if whys[w] == 0 {
					whyOrder = append(whyOrder, w)
				}
				whys[w]++
			}
		}
		if f[9] != "착지" {
			continue
		}
		lt := f[8]
		if lt == "-" {
			lt = f[0]
		}
		if !in(lt) {
			continue
		}
		at, _ := stT(lt)
		sa, ok := stT(f[5])
		if !ok {
			sa = -1
		}
		b, _ := strconv.Atoi(f[6])
		ld := stLand{at: at, subAt: sa, merger: true, group: f[10], batch: b, why: "제출 사건 없음"}
		if x, ok := subEv[f[3]]; ok {
			ld.run, ld.sub, ld.why = x.run, x.sha, ""
			if x.run == "-" {
				ld.why = "작업 ID 없음"
			}
		}
		lands = append(lands, ld)
	}
	// 레인 직접 착지: 제출한 적 없는 작업의 「착지」 보고. 병합기가 보낸 착지(「<까닭> — 제출 <sha8>, 제출 ID …|큐 …」 — 10칸은
	// 병합기 터미널 실행이라 hasSub에 안 걸린다)는 까닭과 상관없이 일지로 센다(감사 atoxryx-kxtet1 중간)
	for _, x := range evs {
		if x.st != "착지" || !in(x.t) || stMergerMsg.MatchString(x.msg) || (x.run != "-" && hasSub[x.run]) {
			continue
		}
		at, _ := stT(x.t)
		ld := stLand{at: at, run: x.run, group: x.lane + "/" + x.sha, batch: 1}
		if x.run == "-" {
			ld.why = "작업 ID 없음"
		}
		lands = append(lands, ld)
	}
	sort.SliceStable(lands, func(i, j int) bool { return lands[i].at < lands[j].at })

	var b strings.Builder
	p := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	p("%s", statsDef)
	p("기간: %s ~ %s", orDefault(since, "(처음)"), until)
	p("== 착지")
	groups, nm, nd := map[string]int{}, 0, 0
	var gorder []string
	for _, ld := range lands {
		if _, ok := groups[ld.group]; !ok {
			gorder = append(gorder, ld.group)
			if ld.merger {
				nm++
			} else {
				nd++
			}
		}
		groups[ld.group]++
	}
	bs := []int64{}
	for _, g := range gorder {
		bs = append(bs, int64(groups[g]))
	}
	sort.Slice(bs, func(i, j int) bool { return bs[i] < bs[j] })
	bat := ""
	if len(bs) > 0 {
		bat = fmt.Sprintf(" · 묶음 크기 중앙 %d · 최대 %d", bs[(len(bs)+1)/2-1], bs[len(bs)-1])
	}
	if already > 0 {
		bat += fmt.Sprintf(" · 이미 들어 있음 %d", already)
	}
	p("착지 %d건 (병합기 %d · 레인 직접 %d)%s", nm+nd, nm, nd, bat)
	var subL, doneL, audL []int64
	order := []string{"제출 사건 없음", "작업 ID 없음", "완료 없음"}
	du, au, noAud, noSubT := map[string]int{}, map[string]int{}, 0, 0
	last := map[string]int64{} // 작업 → 직전 착지 시각
	for _, ld := range lands {
		if ld.merger && ld.subAt >= 0 {
			subL = append(subL, ld.at-ld.subAt)
		} else if ld.merger {
			noSubT++
		}
		if ld.why != "" {
			du[ld.why]++
			au[ld.why]++
			continue
		}
		var first *stEv
		audited := map[string]bool{ld.sub: ld.sub != ""}
		for i, x := range dones[ld.run] {
			xt, _ := stT(x.t)
			if xt > ld.at {
				break
			}
			if xt > last[ld.run] {
				if first == nil {
					first = &dones[ld.run][i]
				}
				if !ld.merger {
					audited[x.sha] = true
				}
			}
		}
		lo := last[ld.run]
		if first == nil {
			du["완료 없음"]++
		} else {
			lo, _ = stT(first.t)
			doneL = append(doneL, ld.at-lo)
		}
		// 승인 = 이 작업 구간(첫 완료 — 없으면 직전 착지 — 뒤, 착지 전)의 마지막 통과 판정(높음 0·중간 0). 같은 SHA의 옛 작업 판정은 잇지 않는다
		at := int64(-1)
		for _, x := range evs {
			xt, _ := stT(x.t)
			if xt > ld.at {
				break
			}
			if x.st == "판정" && audited[x.sha] && x.sha != "-" && strings.Contains(x.msg, "높음 0·중간 0") && xt >= lo {
				at = xt
			}
		}
		if at < 0 {
			noAud++
		} else {
			audL = append(audL, ld.at-at)
		}
		last[ld.run] = ld.at
	}
	p("제출 → 착지: %s%s", stDur(subL), stUnk([]string{"제출 시각 없음"}, map[string]int{"제출 시각 없음": noSubT}))
	p("완료 → 착지: %s%s", stDur(doneL), stUnk(order, du))
	p("감사 통과 → 착지: %s · 판정 없음 %d%s", stDur(audL), noAud, stUnk(order, au))
	peak := func(ts []int64) string {
		c, best, bh := map[string]int{}, 0, ""
		for _, t := range ts {
			h := utcAt(t)[:13]
			c[h]++
			if c[h] > best || (c[h] == best && h < bh) {
				best, bh = c[h], h
			}
		}
		if best == 0 {
			return "0건"
		}
		return fmt.Sprintf("%d건(%s)", best, bh)
	}
	var lt, dt []int64
	gs := map[string]bool{}
	for _, ld := range lands {
		if !gs[ld.group] {
			gs[ld.group] = true
			lt = append(lt, ld.at)
		}
	}
	stuck := 0
	for _, x := range evs {
		if !in(x.t) {
			continue
		}
		t, _ := stT(x.t)
		switch x.st {
		case "완료":
			dt = append(dt, t)
		case "막힘":
			stuck++
		}
	}
	p("시간당 피크: 착지 %s · 완료 %s", peak(lt), peak(dt))

	p("== 병합기 검증 실행 (착지 한 번 부름 = 무거운 검증 한 번)")
	nl, tot := 0, int64(0)
	var rl []int64
	for _, r := range runs {
		if r.landed {
			nl++
		}
		rl = append(rl, r.t1-r.t0)
		tot += r.t1 - r.t0
	}
	per := "-"
	if nl > 0 {
		per = fmt.Sprintf("%.2f", float64(len(runs))/float64(nl))
	}
	p("실행 %d · 착지로 끝남 %d · 돌려보냄 %d · 착지 건당 실행 %s%s", len(runs), nl, len(runs)-nl, per, stUnk([]string{"시작·끝 시각 없음"}, map[string]int{"시작·끝 시각 없음": runUnk}))
	p("실행 시간: %s · 합 %.1f시간(돌려보낸 것 포함)", stDur(rl), float64(tot)/3600)
	wl := []string{}
	for _, w := range whyOrder {
		wl = append(wl, fmt.Sprintf("%s %d", w, whys[w]))
	}
	p("돌려보낸 까닭: %s", orDefault(strings.Join(wl, " · "), "없음"))

	p("== 레인 자리")
	p("막힘 %d", stuck)
	var occ, held []int64
	ou := 0
	termRun := map[string]string{} // 터미널 → 그때까지 본 마지막 실행 ID(정리됨 줄에는 10칸이 없다 — 시각 순으로 따라간다)
	for _, x := range evs {
		if x.run != "-" {
			termRun[x.term] = x.run
		}
		if x.st != "정리됨" || !in(x.t) {
			continue
		}
		ct, _ := stT(x.t)
		r := termRun[x.term]
		_, ms, ok := strings.Cut(r, "@")
		born, err := strconv.ParseInt(ms, 10, 64)
		if !ok || err != nil || born/1000 > ct {
			ou++
			continue
		}
		occ = append(occ, ct-born/1000)
		if d := dones[r]; len(d) > 0 {
			ft, _ := stT(d[0].t)
			held = append(held, ct-ft)
		}
	}
	p("자리 점유(터미널 생김 → 정리됨): %s%s", stDur(occ), stUnk([]string{"실행 ID 없음"}, map[string]int{"실행 ID 없음": ou}))
	p("완료 → 정리됨: %s", stDur(held))
	return b.String()
}
