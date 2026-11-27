package main

// 겹침 키 — 셸 판 lib/hc-fill/overlap.sh의 ktok·lkeys·ovcmp. 셸 판 awk는 바이트 단위 정규식이다: 문자 묶음은 모두 ASCII라 바이트로 보고,
// 여러 바이트 문장부호(「」『』。“”‘’，·、)는 글자열로 따로 다룬다

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// after_tick: 떠 있는 먼저 띄움 줄(up·early)의 기계마다 그 줄 선행의 판정(pred_state)을 ~/.local/share/hc-fill/after.tsv
// (선행 go|stop 까닭 쓴시각(초))로 원자 쓰기 — 레인 기계의 hc-lane early-wait가 이 파일을 본다. 확인한 착지 표식은
// DATA/after-landed.tsv에(ls가 쓴다). dry-run은 찍기만
func afterTick() {
	if !nonEmpty(Q) {
		return
	}
	qs := qRows()
	var markers strings.Builder
	atn := strconv.FormatInt(nowUnix(), 10)
	var ms []string
	for _, r := range qs {
		if r.F(qState) == "up" && rowMode(r) == mEarly && r.F(qMach) != "" {
			ms = append(ms, r.F(qMach))
		}
	}
	sort.Strings(ms)
	for i, am := range ms {
		if i > 0 && ms[i-1] == am {
			continue
		}
		if am != LOCAL && !connHas(am) {
			say("선행 상태 미룸 @%s: 연결 안 됨", am)
			continue
		}
		var ab []string // 「선행<TAB>go|stop<TAB>까닭<TAB>쓴시각」
		done := map[string]bool{}
		for _, r := range qs {
			if r.F(qState) != "up" || rowMode(r) != mEarly || r.F(qMach) != am || r.F(qAfter) == "-" {
				continue
			}
			ap := r.F(qProj)
			for _, x := range words(r.F(qAfter)) {
				if done[x] {
					continue
				}
				done[x] = true
				n, op := afterAt(x, ap) // 일반 배치와 같은 해석(재감사2 fill/R-217) — after.tsv 키는 줄에 적힌 그대로(x)
				predState(n, op)
				if LMS != "" {
					markers.WriteString(op + "\t" + n + "\t" + LMFROM + "\t" + LMS + "\n")
				}
				ab = append(ab, x+"\t"+PSV+"\t"+atn)
			}
		}
		if len(ab) == 0 {
			continue
		}
		if dry {
			for _, l := range ab {
				f := splitRow(l)
				say("%s선행 상태 @%s: %s", P, am, strings.Join(f[:min(3, len(f))], "\t"))
			}
			continue
		}
		if _, rc := onm(strings.NewReader(strings.Join(ab, "\n")+"\n"), am, `mkdir -p "${d%/*}" && cat >"$d.tmp" && mv -f "$d.tmp" "$d"`, "~/.local/share/hc-fill/after.tsv"); rc != 0 {
			say("선행 상태 쓰기 실패 @%s — 다음 바퀴에", am)
		}
	}
	if !dry {
		if err := writeAtomic(DATA+"/after-landed.tsv", markers.String()); err != nil {
			fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
		}
	}
}

var (
	ovParenRe    = regexp.MustCompile(`\([^()]*\)`)
	ovItemSepRe  = regexp.MustCompile(`,|·|、|;|，`)
	ovFridRe     = regexp.MustCompile(`^FR-[A-Z0-9]+(-[A-Z0-9]+)*`)
	ovAlnumRe    = regexp.MustCompile(`[A-Za-z0-9]`)
	ovParticleRe = regexp.MustCompile(`[A-Za-z0-9_./)-](에서|으로|까지|부터|처럼|에게|을|를|이|가|은|는|에|의|와|과|도|로|만)$`)
	ovExRe       = regexp.MustCompile(`^[[:space:]]*([-*][[:space:]]*)?(파일|쓰기):(.*)`)
	ovMSep       = strings.NewReplacer("`", " ", "·", " ", "、", " ", "，", " ", ",", " ", ";", " ")
)

// ovUnwrap: 앞뒤 괄호·따옴표·문장부호를 뗌(awk unwrap)
func ovUnwrap(t string) string {
	for {
		if t != "" && strings.IndexByte("`\"'([{<", t[0]) >= 0 {
			t = t[1:]
			continue
		}
		if p := ovAffix(t, true, "「", "『", "“", "‘"); p > 0 {
			t = t[p:]
			continue
		}
		if t != "" && strings.IndexByte("`\"')]}>,;:!?", t[len(t)-1]) >= 0 {
			t = t[:len(t)-1]
			continue
		}
		if s := ovAffix(t, false, "」", "』", "。", "”", "’", "，"); s > 0 {
			t = t[:len(t)-s]
			continue
		}
		if n := len(t); n >= 2 && t[n-1] == '.' && t[n-2] != '.' && !strings.HasSuffix(t, "/.") {
			t = t[:n-1]
			continue
		}
		return t
	}
}

// ovAffix: 그 머리(pre)·꼬리 가운데 맞는 것의 바이트 길이(없으면 0)
func ovAffix(t string, pre bool, xs ...string) int {
	for _, x := range xs {
		if pre && strings.HasPrefix(t, x) || !pre && strings.HasSuffix(t, x) {
			return len(x)
		}
	}
	return 0
}

func ovFrid(t string) string { return ovFridRe.FindString(t) }

// ovNorm: 경로 정규화(. · .. · 겹 /, 저장소 밖으로 나가면 버림). 절대 경로는 그 프로젝트 repo 줄 경로나 그 .wt/<레인>/ 아래만 받아 뗀다
func ovNorm(t string, kp []string) string {
	if t == "" || strings.HasPrefix(t, "~") || strings.Contains(t, "://") {
		return ""
	}
	abs, dir := t[0] == '/', strings.HasSuffix(t, "/")
	var b []string
	for _, a := range strings.Split(t, "/") {
		switch a {
		case "", ".":
		case "..":
			if len(b) == 0 {
				return ""
			}
			b = b[:len(b)-1]
		default:
			b = append(b, a)
		}
	}
	r := strings.Join(b, "/")
	if abs {
		r = "/" + r
		hit := false
		for _, p := range kp {
			if p == "" {
				continue
			}
			if strings.HasPrefix(r+"/", p+"/") {
				r, hit = ovSubstr(r, len(p)+1), true
				break
			}
			if strings.HasPrefix(r, p+".wt/") {
				r = ovSubstr(r, len(p)+4)
				i := strings.IndexByte(r, '/')
				if i <= 0 { // sub(/^[^\/]+\//, "") 이 안 맞음
					return ""
				}
				r, hit = r[i+1:], true
				break
			}
		}
		if !hit {
			return ""
		}
	}
	if r == "" || strings.Trim(r, "-._/") == "" {
		return ""
	}
	if dir {
		r += "/"
	}
	return r
}

func ovSubstr(s string, i int) string {
	if i >= len(s) {
		return ""
	}
	return s[i:]
}

func ovSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\n' }

// ktok <R|M>: 글 줄에서 키 후보 「R|M<TAB>id<TAB>FR ID」 · 「R<TAB>p|q<TAB>경로」 · 「M<TAB>m<TAB>경로<TAB>조사 뗀 경로」(kp = kpre 출력)
func ktok(kind string, ls []string, kp []string) []Row {
	var o []Row
	for _, l := range ls {
		if kind == "R" {
			// 예약: 괄호 안을 먼저 지우고(겹 괄호는 되풀이) 항목으로 나눈 뒤, 항목 첫 경로만 — 백틱으로 시작하면 그 안, 아니면 첫 낱말.
			// 낱말 하나뿐인 항목은 p(새 파일도 받음), 뒤에 설명이 붙은 항목은 q
			x0 := l
			for {
				y := ovParenRe.ReplaceAllString(x0, "")
				if y == x0 {
					break
				}
				x0 = y
			}
			for _, it := range ovItemSepRe.Split(x0, -1) {
				x := strings.Trim(it, " \t")
				if x == "" {
					continue
				}
				ty := "p"
				if strings.HasPrefix(x, "`") && strings.IndexByte(x[1:], '`') > 0 {
					x = x[1 : 1+strings.IndexByte(x[1:], '`')]
				} else {
					w := strings.FieldsFunc(x, func(r rune) bool { return r == ' ' || r == '\t' })
					if len(w) != 1 {
						ty = "q"
					}
					x = ovUnwrap(w[0])
				}
				if f := ovFrid(x); f != "" && f == x {
					o = append(o, Row{"R", "id", f})
					continue
				}
				if p := ovNorm(x, kp); p != "" {
					o = append(o, Row{"R", ty, p})
				}
			}
			continue
		}
		for _, fld := range strings.FieldsFunc(ovMSep.Replace(l), ovSpace) { // `경로`을 → 경로 · 을, 가운뎃점·쉼표 목록은 낱말마다
			x := ovUnwrap(fld)
			if f := ovFrid(x); f != "" {
				o = append(o, Row{"M", "id", f})
				continue
			}
			if !ovAlnumRe.MatchString(x) {
				continue
			}
			s := x
			if loc := ovParticleRe.FindStringIndex(s); loc != nil {
				s = ovUnwrap(s[:loc[0]+1])
			}
			p, q := ovNorm(x, kp), ovNorm(s, kp)
			if q == "" {
				q = p
			}
			if p != "" || q != "" {
				f := p
				if p == "" {
					f = q
				}
				o = append(o, Row{"M", "m", f, q})
			}
		}
	}
	return o
}

// ovGlob: 셸 case 패턴 맞춤(* 는 / 도 넘는다, ?, [...] 와 [!...])
func ovGlob(pattern, s string) bool {
	pat := []rune(pattern) // 글자 단위(한글 등 UTF-8 여러 바이트 글자를 바이트마다 다시 적지 않게 — gofix-fill F13)
	var b strings.Builder
	b.WriteString(`(?s)^`)
	for i := 0; i < len(pat); i++ {
		c := pat[i]
		switch c {
		case '*':
			b.WriteString(`.*`)
		case '?':
			b.WriteString(`.`)
		case '\\':
			if i+1 < len(pat) {
				i++
				b.WriteString(regexp.QuoteMeta(string(pat[i])))
			} else {
				b.WriteString(`\\`)
			}
		case '[':
			j := i + 1
			if j < len(pat) && (pat[j] == '!' || pat[j] == '^') {
				j++
			}
			if j < len(pat) && pat[j] == ']' {
				j++
			}
			for j < len(pat) && pat[j] != ']' {
				j++
			}
			if j >= len(pat) { // 닫히지 않은 [ 는 글자 그대로
				b.WriteString(`\[`)
				continue
			}
			body := string(pat[i+1 : j])
			neg := strings.HasPrefix(body, "!") || strings.HasPrefix(body, "^")
			if neg {
				body = body[1:]
			}
			b.WriteByte('[')
			if neg {
				b.WriteByte('^')
			}
			b.WriteString(strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`, `^`, `\^`).Replace(body))
			b.WriteByte(']')
			i = j
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteByte('$')
	re, err := regexp.Compile(b.String())
	return err == nil && re.MatchString(s)
}

// ovGlobEsc: 파일 이름 펼치기에서 글자 그대로 쓸 부분(셸 판 "$lr_/$v_".* 의 따옴표 안)
func ovGlobEsc(s string) string {
	return strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`).Replace(s)
}

// lkeys <프로젝트> <지시문>: 그 레인의 키 「R|M<TAB>키」(같은 키는 앞의 것 — R이 앞)
func lkeys(p, brief string) []Row {
	lr := strings.TrimSuffix(repoOf(p, LOCAL), "/")
	kp := kpre(p)
	igs := ovIgnores(p)
	bl := readLines(brief)
	var ex []string
	for _, l := range bl {
		if m := ovExRe.FindStringSubmatch(l); m != nil {
			ex = append(ex, m[3])
		}
	}
	seen := map[string]bool{}
	var o []Row
	for _, t := range append(ktok("R", ex, kp), ktok("M", bl, kp)...) {
		k, y, v, w := t.F(tkKind), t.F(tkType), t.F(tkVal), t.F(tkRaw)
		var vs []string // 맨 이름이 확장자 붙은 맨 위 파일로 풀리면 그 이름들(README → README.md)
		switch y {
		case "p", "q":
			// 예약 경로: / 가 있으면 그대로, 맨 위 실제 파일·폴더면 그것(폴더면 끝 /), 없으면 확장자 붙은 맨 위 파일, 그것도 없으면
			// p는 새 파일, q는 확장자나 점 파일 꼴일 때만
			if !strings.Contains(v, "/") && (lr == "" || !lcE(lr+"/"+v)) {
				if lr != "" {
					ms, _ := filepath.Glob(ovGlobEsc(lr+"/"+v) + ".*")
					for _, f := range ms {
						if isFile(f) {
							vs = append(vs, filepath.Base(f))
						}
					}
				}
				if len(vs) == 0 && y != "p" && !ovGlob(`??*.[A-Za-z0-9]*`, v) && !ovGlob(`.[A-Za-z0-9]?*`, v) {
					continue
				}
			}
			if lr != "" && isDir(lr+"/"+v) {
				v = strings.TrimSuffix(v, "/") + "/"
			}
		case "m":
			// 언급: 있는 것(조사 붙은 그대로 → 뗀 것), 없으면 첫 폴더가 있는 새 경로. 맨 위 낱말은 실제 파일만(Makefile)
			if lr == "" {
				continue
			}
			if lcE(lr + "/" + v) {
			} else if lcE(lr + "/" + w) {
				v = w
			} else {
				v = w
				if v == "" || !strings.Contains(v[:len(v)-1], "/") || !isDir(lr+"/"+v[:strings.IndexByte(v, '/')]) {
					continue
				}
			}
			if !strings.Contains(v, "/") && !isFile(lr+"/"+v) {
				continue
			}
			if isDir(lr + "/" + v) {
				v = strings.TrimSuffix(v, "/") + "/"
			}
		}
		if len(vs) == 0 {
			vs = []string{v}
		}
		for _, x := range vs {
			if x == "" || k == "M" && ruleDocs[filepath.Base(x)] {
				continue
			}
			if !ovIgnored(igs, x) && !seen[x] {
				seen[x] = true
				o = append(o, Row{k, x})
			}
		}
	}
	return o
}

// ovIgnores <프로젝트>: 설정 overlap_ignore 패턴
func ovIgnores(p string) []string {
	var igs []string
	for _, f := range confRows("overlap_ignore") {
		if len(f) >= 2 && f[1] == p {
			igs = append(igs, f[2:]...)
		}
	}
	return igs
}

func ovIgnored(igs []string, x string) bool {
	for _, g := range igs {
		if ovGlob(g, x) {
			return true
		}
	}
	return false
}

// ovcmp <소유 줄(레인 R|M|A 키)> <후보 키(R|M|A 키)>: 앞 레인마다 「앞레인<TAB>실제|예약|언급<TAB>키 둘까지」 — 예약 = 둘 다 R,
// 실제 = 한쪽이 실제 변경(A)이고 다른 쪽이 A나 R(A와 M은 언급). 한 앞 레인에 여럿이면 실제 > 예약 > 언급
func ovcmp(own, cand []Row) []Row {
	hitf := func(a, b string) bool {
		return a == b || strings.HasSuffix(a, "/") && strings.HasPrefix(b, a) || strings.HasSuffix(b, "/") && strings.HasPrefix(a, b)
	}
	type rk struct {
		r int // 0 언급 · 1 예약 · 2 실제
		o string
	}
	strong := func(k string) bool { return k == "R" || k == "A" }
	var ord []string
	seenO, res := map[string]bool{}, map[string]int{}
	ks, kc := map[rk][]string{}, map[rk]int{}
	for _, f := range own {
		if f.NF() < okKey || f.F(okKey) == "" {
			continue
		}
		for _, c := range cand {
			if c.F(kyKey) == "" || !hitf(f.F(okKey), c.F(kyKey)) { // 빈 키는 겹침이 아니다
				continue
			}
			o := f.F(okLane)
			if !seenO[o] {
				seenO[o] = true
				ord = append(ord, o)
			}
			r := 0
			switch {
			case f.F(okKind) == "R" && c.F(kyKind) == "R":
				r = 1
			case strong(f.F(okKind)) && strong(c.F(kyKind)):
				r = 2
			}
			res[o] = max(res[o], r)
			k := rk{r, o}
			if !has(ks[k], c.F(kyKey)) && kc[k] < 2 {
				ks[k] = append(ks[k], c.F(kyKey))
				kc[k]++
			}
		}
	}
	var out []Row
	for _, o := range ord {
		out = append(out, Row{o, []string{"언급", "예약", "실제"}[res[o]], strings.Join(ks[rk{res[o], o}], ",")})
	}
	return out
}
