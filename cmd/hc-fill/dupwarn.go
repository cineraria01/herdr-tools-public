package main

// add 때 비슷한 레인 알림(Go 판에만) — 같은 프로젝트의 대기·떠 있는 줄(모든 주인)과 지시문 제목 낱말 겹침(Jaccard)이나
// 지시문이 적은 저장소 경로(lkeys) 겹침을 견줘 한 줄씩 알린다. 막지 않는다

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	dupParenRe = regexp.MustCompile(`\([^()]*\)|（[^（）]*）`)
	dupLaneRe  = regexp.MustCompile(`^(레인|감사)[ \t]+[^ \t]+`)
)

// dupConf: 설정 dup_warn <낱말 비율> [<경로 수>] — 빼면 0.5 2, off·0이면 끔(ok=false)
func dupConf() (ratio float64, paths int, ok bool) {
	ratio, paths = 0.5, 2
	f := fields(conf("dup_warn"))
	if len(f) > 0 {
		if f[0] == "off" {
			return 0, 0, false
		}
		if v, err := strconv.ParseFloat(f[0], 64); err == nil && v >= 0 && v <= 1 {
			ratio = v
		} else {
			say("알림: 설정 dup_warn 낱말 비율은 0~1 수다, 0.5로: %s", f[0])
		}
	}
	if len(f) > 1 {
		if v, err := strconv.Atoi(f[1]); err == nil && v >= 0 {
			paths = v
		} else {
			say("알림: 설정 dup_warn 경로 수는 0 이상 정수다, 2로: %s", f[1])
		}
	}
	return ratio, paths, ratio > 0 || paths > 0
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

// dupPaths: 두 키 목록(lkeys)에서 겹친 경로(폴더 키는 그 아래와 겹침, FR ID 포함)
func dupPaths(a, b []Row) []string {
	var o []string
	for _, x := range a {
		for _, y := range b {
			p, q := x.F(2), y.F(2)
			if p == "" || q == "" {
				continue
			}
			if p == q || strings.HasSuffix(p, "/") && strings.HasPrefix(q, p) || strings.HasSuffix(q, "/") && strings.HasPrefix(p, q) {
				if !has(o, q) {
					o = append(o, q)
				}
				break
			}
		}
	}
	return o
}

// dupWarn <레인> <프로젝트> <지시문>: 비슷한 레인마다 「알림: 비슷한 레인 …」 한 줄. 감사 줄·착지한 줄은 안 본다
func dupWarn(name, prj, brief string) {
	ratio, np, ok := dupConf()
	if !ok || !isFile(Q) {
		return
	}
	mt, mk := dupTitle(brief), lkeys(prj, brief)
	logKnown := false
	if FR = repoOf(prj, LOCAL); FR != "" {
		FB, logKnown = projectRef(FR)
	}
	for _, r := range qRows() {
		n, st := r.F(1), r.F(6)
		if n == name || r.F(2) != prj || (st != "wait" && st != "up") || strings.HasPrefix(r.F(15), "audit=") || !isFile(r.F(3)) {
			continue
		}
		var why []string
		if ratio > 0 {
			if j, c := dupJaccard(mt, dupTitle(r.F(3))); c >= 2 && j >= ratio {
				why = append(why, "제목 낱말 "+strconv.Itoa(int(j*100+0.5))+"% 겹침")
			}
		}
		if np > 0 {
			if ps := dupPaths(mk, lkeys(prj, r.F(3))); len(ps) >= np {
				why = append(why, "경로 "+strconv.Itoa(len(ps))+"개 겹침("+strings.Join(ps[:min(3, len(ps))], ",")+")")
			}
		}
		if len(why) == 0 {
			continue
		}
		s := "대기"
		if st == "up" {
			if logKnown {
				if l, ok := landed(n); ok && l != "" {
					continue
				}
			}
			s = "떠 있음"
			if a := r.F(8); a != "" && a != "-" {
				s += " " + a
			}
		}
		own := dash(r.F(12))
		if own == "-" {
			own = "없음"
		}
		say("알림: 비슷한 레인 %s (주인 %s, %s) — %s", n, own, s, strings.Join(why, ", "))
	}
}
