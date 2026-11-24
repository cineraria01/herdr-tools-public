package main

// add 때 비슷한 레인 알림(Go 판에만) — 같은 프로젝트의 대기·작업 중인 줄(모든 주인)과 견줘 비슷한 것을 알린다. 막지 않는다.
// 예약(「파일:」·「쓰기:」 줄)이 겹치면 늘, 제목 낱말이 겹치면(Jaccard) 본문 언급 경로 겹침을 곁들여. 본문 언급만 겹치는 것은
// 알리지 않는다 — 모든 지시문이 함께 적는 상용구(프로젝트 이름·<…> 자리표시·그 프로젝트 지시문 다수가 적는 흔한 경로)가
// 언급 겹침을 다 채워 오인만 났다(2026-11-22 dup-warn-noise). 가장 비슷한 3개만 + 「외 N」

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var (
	dupParenRe = regexp.MustCompile(`\([^()]*\)|（[^（）]*）`)
	dupLaneRe  = regexp.MustCompile(`^(레인|감사)[ \t]+[^ \t]+`)
)

const (
	dupShow      = 3 // 알림 줄 수 상한
	dupMinBriefs = 5 // 흔한 경로를 셀 최소 지시문 수 — 적으면 한두 지시문이 곧 「흔함」이 된다
)

// dupConf: 설정 dup_warn <낱말 비율> [<언급 경로 수>] [<흔한 경로 비율>] — 빼면 0.5 2 0.3, off면 끔(ok=false).
// 낱말 비율 0이면 제목 알림 끔(예약 겹침만), 흔한 경로 비율 0이면 흔한 경로를 안 거른다
func dupConf() (ratio float64, paths int, common float64, ok bool) {
	ratio, paths, common = 0.5, 2, 0.3
	f := fields(conf("dup_warn"))
	if len(f) > 0 && f[0] == "off" {
		return 0, 0, 0, false
	}
	frac := func(i int, def float64, what string) float64 {
		if len(f) <= i {
			return def
		}
		if v, err := strconv.ParseFloat(f[i], 64); err == nil && v >= 0 && v <= 1 {
			return v
		}
		say("알림: 설정 dup_warn %s은 0~1 수다, %g로: %s", what, def, f[i])
		return def
	}
	ratio = frac(0, ratio, "낱말 비율")
	if len(f) > 1 {
		if v, err := strconv.Atoi(f[1]); err == nil && v >= 0 {
			paths = v
		} else {
			say("알림: 설정 dup_warn 경로 수는 0 이상 정수다, 2로: %s", f[1])
		}
	}
	common = frac(2, common, "흔한 경로 비율")
	return ratio, paths, common, true
}

// dupTitle: 지시문 첫 「# 」 제목 줄의 낱말 집합 — 괄호 안(프로젝트·지휘·날짜), 머리 「레인 <이름>」, 한 글자·숫자만인 낱말은 뺀다
func dupTitle(brief string) map[string]bool {
	t := ""
	for _, l := range readLines(brief) {
		if strings.HasPrefix(l, "# ") {
			t = strings.TrimSpace(l[2:])
			break
		}
	}
	for {
		y := dupParenRe.ReplaceAllString(t, " ")
		if y == t {
			break
		}
		t = y
	}
	t = dupLaneRe.ReplaceAllString(t, "")
	ws := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(t), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len([]rune(w)) >= 2 && !isDigits(w) {
			ws[w] = true
		}
	}
	return ws
}

// dupJaccard: 낱말 집합 겹침 비율과 겹친 수
func dupJaccard(a, b map[string]bool) (float64, int) {
	n := 0
	for w := range a {
		if b[w] {
			n++
		}
	}
	u := len(a) + len(b) - n
	if u == 0 {
		return 0, 0
	}
	return float64(n) / float64(u), n
}

// dupKeys: lkeys에서 상용구를 뺀 키 — 프로젝트 이름 그 자체, <…> 자리표시가 든 경로
func dupKeys(prj, brief string) []Row {
	var o []Row
	for _, k := range lkeys(prj, brief) {
		if p := k.F(kyKey); strings.TrimSuffix(p, "/") != prj && !strings.ContainsAny(p, "<>") {
			o = append(o, k)
		}
	}
	return o
}

// dupCommon: 지시문들 가운데 share 이상(두 지시문 이상)이 적은 키 — 지시문이 dupMinBriefs 미만이거나 share 0이면 없음
func dupCommon(keys [][]Row, share float64) map[string]bool {
	c := map[string]bool{}
	if share <= 0 || len(keys) < dupMinBriefs {
		return c
	}
	n := map[string]int{}
	for _, ks := range keys {
		seen := map[string]bool{}
		for _, k := range ks {
			if p := k.F(kyKey); !seen[p] {
				seen[p] = true
				n[p]++
			}
		}
	}
	need := max(2, int(math.Ceil(share*float64(len(keys)))))
	for p, v := range n {
		if v >= need {
			c[p] = true
		}
	}
	return c
}

// dupPaths: 두 키 목록(dupKeys)에서 겹친 경로(폴더 키는 그 아래와 겹침, FR ID 포함) — 둘 다 예약(R)이면 res, 아니면 언급 men.
// 언급 겹침은 흔한 경로(common)를 뺀다
func dupPaths(a, b []Row, common map[string]bool) (res, men []string) {
	for _, x := range a {
		for _, y := range b {
			p, q := x.F(kyKey), y.F(kyKey)
			if p == "" || q == "" {
				continue
			}
			if p == q || strings.HasSuffix(p, "/") && strings.HasPrefix(q, p) || strings.HasSuffix(q, "/") && strings.HasPrefix(p, q) {
				switch {
				case x.F(kyKind) == "R" && y.F(kyKind) == "R":
					if !has(res, q) {
						res = append(res, q)
					}
				case !common[p] && !common[q] && !has(men, q):
					men = append(men, q)
				}
				break
			}
		}
	}
	return res, men
}

// dupDone <대기열 줄> <프로젝트> <병합기 켬>: 떠 있어도 일은 끝난 레인 — 완료·제출·착지를 보고했거나(run이 바퀴마다 쓰는
// last.tsv, 같은 줄 ID) 병합기 큐에 냈다
func dupDone(r Row, prj string, merger bool) bool {
	for _, x := range readRows(DATA + "/last.tsv") {
		if x.F(lsName) == r.F(qName) && (x.F(lsRID) == "" || x.F(lsRID) == r.F(qRID)) {
			switch x.F(lsState) {
			case "완료", "제출", "착지":
				return true
			}
			break
		}
	}
	return merger && mqueued(prj, r.F(qName)) == 0
}

// dupLandedSince <landed 줄> <줄 넣은 시각>: 그 커밋이 줄을 넣은 뒤의 것(이번 실행의 착지)인가 — 같은 이름 옛 커밋이면 거짓(R-26).
// 넣은 시각이나 커밋 시각을 모르면 참(전처럼 뺀다 — 알림일 뿐이라 조용한 쪽)
func dupLandedSince(l, at string) bool {
	t, err := time.ParseInLocation("2006-01-02T15:04", at, time.Local)
	if err != nil {
		return true
	}
	o, rc := run("git", "-C", FR, "show", "-s", "--format=%ct", strings.Fields(l)[0])
	ct, err := strconv.ParseInt(sub(o), 10, 64)
	if rc != 0 || err != nil {
		return true
	}
	return ct >= t.Unix()
}

// dupWarn <레인> <프로젝트> <지시문>: 비슷한 레인마다 「알림: 비슷한 레인 …」 한 줄(가장 비슷한 dupShow개 + 「외 N」).
// 감사 줄·끝난 레인(완료 보고·병합기 큐·착지)은 안 본다 — 그건 겹침 막기(overlap)가 볼 일
func dupWarn(name, prj, brief string) {
	ratio, np, share, ok := dupConf()
	if !ok || !isFile(Q) {
		return
	}
	mt, mk := dupTitle(brief), dupKeys(prj, brief)
	var rows []Row
	all, kk := [][]Row{mk}, map[string][]Row{}
	for _, r := range qRows() {
		if r.F(qName) == name || r.F(qProj) != prj || strings.HasPrefix(r.F(qAudit), "audit=") || !isFile(r.F(qBrief)) {
			continue
		}
		ks := dupKeys(prj, r.F(qBrief)) // 흔한 경로는 끝난 줄까지 모든 지시문으로 센다
		all = append(all, ks)
		if st := r.F(qState); st == "wait" || st == "up" {
			rows, kk[r.F(qName)] = append(rows, r), ks
		}
	}
	common := dupCommon(all, share)
	logKnown, merger := false, false
	if FR = repoOf(prj, LOCAL); FR != "" {
		FB, logKnown = projectRef(FR)
	}
	for _, f := range confRows("merger") {
		merger = merger || F1(f, 1) == prj
	}
	type hit struct {
		line  string
		nr, n int
		j     float64
	}
	var hits []hit
	for _, r := range rows {
		n, st := r.F(qName), r.F(qState)
		res, men := dupPaths(mk, kk[n], common)
		j, c := dupJaccard(mt, dupTitle(r.F(qBrief)))
		var why []string
		if len(res) > 0 {
			why = append(why, "예약 "+strconv.Itoa(len(res))+"개 겹침("+strings.Join(res[:min(3, len(res))], ",")+")")
		}
		if ratio > 0 && c >= 2 && j >= ratio {
			why = append(why, "제목 낱말 "+strconv.Itoa(int(j*100+0.5))+"% 겹침")
			if np > 0 && len(men) >= np {
				why = append(why, "언급 경로 "+strconv.Itoa(len(men))+"개 겹침("+strings.Join(men[:min(3, len(men))], ",")+")")
			}
		}
		if len(why) == 0 {
			continue
		}
		s := "대기"
		if st == "up" {
			if logKnown {
				if l, ok := landed(n); ok && l != "" && dupLandedSince(l, r.F(qAdded)) {
					continue
				}
			}
			if dupDone(r, prj, merger) {
				continue
			}
			s = "떠 있음"
			if a := r.F(qAddr); a != "" && a != "-" {
				s += " " + a
			}
		}
		own := dash(r.F(qOwner))
		if own == "-" {
			own = "없음"
		}
		hits = append(hits, hit{"알림: 비슷한 레인 " + n + " (주인 " + own + ", " + s + ") — " + strings.Join(why, ", "), len(res), len(men), j})
	}
	sort.SliceStable(hits, func(a, b int) bool {
		x, y := hits[a], hits[b]
		if x.nr != y.nr {
			return x.nr > y.nr
		}
		if x.j != y.j {
			return x.j > y.j
		}
		return x.n > y.n
	})
	for i, h := range hits {
		if i == dupShow {
			say("알림: 비슷한 레인 외 %d개(가장 비슷한 %d개만)", len(hits)-dupShow, dupShow)
			break
		}
		say("%s", h.line)
	}
}
