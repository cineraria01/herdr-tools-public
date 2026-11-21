package main

// cotenant-guard의 셸 낱말 가르기 — 셸 판 AWK 프로그램(LC_ALL=C /usr/bin/awk, 바이트 단위)을 줄마다 옮겼다.
// 찾은 것마다 「KIND\037DIR\037ARG」 한 줄(ARG 안 줄바꿈은 \036). 함수 이름·변수 이름은 awk 원문 그대로다.

import (
	"regexp"
	"strings"
)

type cgTok struct {
	ch                []byte // ch[1..n]
	n, i              int
	mode, tok         string
	intok             bool
	tl                int
	t                 []string // t[1..nt]
	nt                int
	hd                []string // hd[1..nhd]
	nhd               int
	cwd, home         string
	subnext, viax, bt bool
	al                map[string]string
	opton             bool
	optcwd            string
	out               []string
}

// cgAnalyze: awk에 넘기던 입력(here-string이라 끝에 \n, RS=\001이라 \001은 빠진다)을 갈라 찾은 줄들
func cgAnalyze(cmd, cwd, home string) []string {
	src := strings.ReplaceAll(cmd, "\x01", "") + "\n"
	k := &cgTok{cwd: cwd, home: home, n: len(src)}
	k.ch = append([]byte{0}, src...)
	k.run()
	return k.out
}

// c: awk ch[i] — 범위 밖은 ""(여기서는 0)
func (k *cgTok) c(i int) byte {
	if i < 1 || i > k.n {
		return 0
	}
	return k.ch[i]
}

// tt: awk t[j] — 범위 밖은 ""
func (k *cgTok) tt(j int) string {
	if j < 1 || j > k.nt {
		return ""
	}
	return k.t[j]
}

var (
	awkDigitsDash = regexp.MustCompile(`^[0-9]*-?$`)
	awkDigits     = regexp.MustCompile(`^[0-9]+$`)
)

func (k *cgTok) word() {
	if !k.intok {
		return
	}
	switch k.mode {
	case "w":
		k.emit("WRITE", k.cwd, k.tilde(k.tok))
	case "d":
		if !awkDigitsDash.MatchString(k.tok) {
			k.emit("WRITE", k.cwd, k.tilde(k.tok))
		}
	case "h":
		k.nhd++
		k.hd = append(k.hd[:k.nhd], k.tok)
	case "s":
	default:
		k.nt++
		k.t = append(k.t[:k.nt], k.tok)
	}
	k.mode, k.tok, k.intok, k.tl = "", "", false, 0
}

func (k *cgTok) add(c byte) {
	if k.tl < 1024 {
		k.tok += string([]byte{c})
		k.tl++
	}
	k.intok = true
}

func (k *cgTok) emit(kind, d, a string) {
	k.out = append(k.out, kind+"\x1f"+d+"\x1f"+strings.ReplaceAll(a, "\n", "\x1e"))
}

func awkJoin(b, p string) string {
	if strings.HasPrefix(p, "/") {
		return p
	}
	return b + "/" + p
}

func (k *cgTok) tilde(p string) string {
	if p == "~" {
		return k.home
	}
	if strings.HasPrefix(p, "~/") {
		return k.home + p[1:]
	}
	return p
}

func (k *cgTok) redir() {
	c := k.c(k.i)
	if k.intok && k.mode == "" && awkDigits.MatchString(k.tok) {
		k.tok, k.intok, k.tl = "", false, 0
	} else {
		k.word()
	}
	switch {
	case c == '>':
		if k.c(k.i+1) == '>' || k.c(k.i+1) == '|' {
			k.i++
		}
		if k.c(k.i+1) == '&' {
			k.i++
			k.mode = "d"
		} else {
			k.mode = "w"
		}
	case k.c(k.i+1) == '<':
		k.i++
		if k.c(k.i+1) == '<' {
			k.i++
			k.mode = "s"
		} else {
			if k.c(k.i+1) == '-' {
				k.i++
			}
			k.mode = "h"
		}
	case k.c(k.i+1) == '&':
		k.i++
		k.mode = "s"
	case k.c(k.i+1) == '>':
		k.i++
		k.mode = "w"
	default:
		k.mode = "s"
	}
}

// skipto: p의 줄바꿈부터 본문 줄을 건너 w와 같은 줄까지 — 그 줄 끝 자리, 없으면 -1
func (k *cgTok) skipto(p int, w string) int {
	for {
		s := p + 1
		if s > k.n {
			return -1
		}
		e := s
		for e <= k.n && k.ch[e] != '\n' {
			e++
		}
		p = e
		if e-s <= len(w)+16 {
			line := strings.TrimRight(strings.TrimLeft(string(k.ch[s:e]), " \t"), " \t")
			if line == w {
				return p
			}
		}
	}
}

func (k *cgTok) heredocs(p int) int {
	for j := 1; j <= k.nhd; j++ {
		p = k.skipto(p, k.hd[j])
		if p < 0 {
			p = k.n
			break
		}
	}
	k.nhd = 0
	return p
}

func (k *cgTok) qheredoc(p int) int {
	j := p + 2
	if k.c(j) == '-' {
		j++
	}
	for k.c(j) == ' ' || k.c(j) == '\t' {
		j++
	}
	if k.c(j) == '\'' || k.c(j) == '"' {
		j++
	}
	w := ""
	for j <= k.n {
		c := k.ch[j]
		if c == ' ' || c == '\t' || c == '\n' || c == '\'' || c == '"' || strings.IndexByte("<>;&|()", c) >= 0 {
			break
		}
		w += string([]byte{c})
		j++
	}
	if w == "" {
		return -1
	}
	e := j
	for e <= k.n && k.ch[e] != '\n' {
		e++
	}
	if e > k.n {
		return -1
	}
	return k.skipto(e, w)
}

func (k *cgTok) nonopt(at int, last bool) {
	dd := false
	for j := at + 1; j <= k.nt; j++ {
		a := k.t[j]
		if !dd && a == "--" {
			dd = true
			continue
		}
		if !dd && strings.HasPrefix(a, "-") {
			continue
		}
		if last && j < k.nt {
			continue
		}
		k.emit("WRITE", k.cwd, k.tilde(a))
	}
}

// optend: 감싸는 명령의 옵션을 getopt처럼 걷는다 — 첫 피연산자(안쪽 명령) 자리
func (k *cgTok) optend(j int, shortv, longv string, plus bool) int {
	k.opton, k.optcwd = false, ""
	for ; j <= k.nt; j++ {
		o := k.t[j]
		if o == "--" {
			return j + 1
		}
		if !awkAnyOpt.MatchString(o) && !(plus && awkPlusOpt.MatchString(o)) {
			return j
		}
		if strings.HasPrefix(o, "--") {
			c := o
			if e := strings.IndexByte(c, '='); e >= 0 {
				c = c[:e]
			}
			if c == "--on" {
				k.opton = true
			}
			if c == "--cwd" {
				if strings.Contains(o, "=") {
					k.optcwd = o[6:]
				} else {
					k.optcwd = k.tt(j + 1)
				}
			}
			if !strings.Contains(o, "=") && strings.Contains(longv, " "+o+" ") {
				j++
			}
			continue
		}
		for q := 1; q < len(o); q++ {
			if strings.IndexByte(shortv, o[q]) >= 0 {
				if q == len(o)-1 {
					j++
				}
				break
			}
		}
	}
	return j
}

// awkSqWord: awk sq() — 다시 가를 수 있게 한 낱말을 따옴표로
func awkSqWord(s string) string {
	if awkSq.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (k *cgTok) cmd() {
	k.mode = ""
	at := 1
	for at <= k.nt {
		w := k.t[at]
		if awkSkipWord.MatchString(w) || awkAssign.MatchString(w) {
			at++
			continue
		}
		if w == "env" {
			at++
			for at <= k.nt && (strings.HasPrefix(k.t[at], "-") || awkAssign.MatchString(k.t[at])) {
				if x := k.t[at]; x == "-u" || x == "-C" || x == "-S" || x == "-P" {
					at++
				}
				at++
			}
			continue
		}
		if w == "timeout" || w == "gtimeout" {
			at++
			for at <= k.nt && strings.HasPrefix(k.t[at], "-") {
				if k.t[at] == "-s" || k.t[at] == "-k" {
					at++
				}
				at++
			}
			at++
			continue
		}
		if w == "nice" {
			at++
			for at <= k.nt && strings.HasPrefix(k.t[at], "-") {
				if k.t[at] == "-n" {
					at++
				}
				at++
			}
			continue
		}
		if w == "xargs" {
			at++
			for at <= k.nt && strings.HasPrefix(k.t[at], "-") {
				if awkXargsV.MatchString(k.t[at]) {
					at++
				}
				at++
			}
			k.viax = true
			continue
		}
		break
	}
	if at <= k.nt {
		name := k.t[at]
		if e := strings.LastIndexByte(name, '/'); e >= 0 {
			name = name[e+1:]
		}
		switch {
		case name == "cd" || name == "pushd":
			if at == k.nt {
				k.cwd = k.home
			} else if !strings.HasPrefix(k.t[at+1], "-") {
				k.cwd = awkJoin(k.cwd, k.tilde(k.t[at+1]))
			}
		case name == "git":
			k.git(at)
		case name == "sed":
			k.sedi(at)
		case name == "cp":
			k.nonopt(at, true)
		case name == "mv" || name == "rm" || name == "rmdir" || name == "unlink" || name == "tee":
			k.nonopt(at, false)
		case awkShells.MatchString(name):
			j := k.optend(at+1, "oO", " --rcfile --init-file ", true)
			for q := at + 1; q < j; q++ {
				if awkShellC.MatchString(k.tt(q)) { // 값 없는 마지막 -o·-O면 optend가 nt+2 — awk t[nt+1]은 ""
					if j <= k.nt {
						k.emit("SUB", k.cwd, k.t[j])
					}
					break
				}
			}
		case name == "eval":
			x := ""
			for j := at + 1; j <= k.nt; j++ {
				if j > at+1 {
					x += " "
				}
				x += k.t[j]
			}
			k.emit("SUB", k.cwd, x)
		case name == "hc" && (k.tt(at+1) == "exec" || k.tt(at+1) == "run" || (k.tt(at+1) == "--json" && (k.tt(at+2) == "exec" || k.tt(at+2) == "run"))):
			if k.tt(at+1) == "--json" {
				at++
			}
			j := k.optend(at+2, "", " --on --cwd --env --timeout --name --label --parent ", false)
			x := ""
			if j > k.nt {
			} else if k.tt(at+1) == "run" && j == k.nt {
				x = k.t[j]
			} else {
				for q := j; q <= k.nt; q++ {
					if q > j {
						x += " "
					}
					x += awkSqWord(k.t[q])
				}
			}
			if x != "" {
				kind, d := "SUB", k.cwd
				if k.opton {
					kind = "RSUB"
				}
				if k.optcwd != "" {
					d = awkJoin(k.cwd, k.tilde(k.optcwd))
				}
				k.emit(kind, d, x)
			}
		case name == "ssh":
			j := k.optend(at+1, "BbcDEeFIiJLlmOoPpQRSWw", "", false) + 1
			if k.tt(j) == "--" {
				j++
			}
			x := ""
			for q := j; q <= k.nt; q++ {
				if q > j {
					x += " "
				}
				x += k.t[q]
			}
			if x != "" {
				k.emit("RSUB", k.cwd, x)
			}
		}
	}
	k.nt, k.viax = 0, false
	k.t = k.t[:1]
}

func (k *cgTok) run() {
	k.t = []string{""}
	k.hd = []string{""}
	n := k.n
	for k.i = 1; k.i <= n; k.i++ {
		c := k.ch[k.i]
		switch {
		case c == '\\':
			if k.i < n {
				k.i++
				if k.ch[k.i] != '\n' {
					k.add(k.ch[k.i])
				}
			}
		case c == '\'':
			k.i++
			for k.i <= n && k.ch[k.i] != '\'' {
				k.add(k.ch[k.i])
				k.i++
			}
			k.intok = true
		case c == '"':
			k.i++
			dsub := false
			for k.i <= n && k.ch[k.i] != '"' {
				if k.ch[k.i] == '\\' && k.i < n {
					k.i++
					k.add(k.ch[k.i])
					k.i++
					continue
				}
				if k.ch[k.i] == '$' && k.c(k.i+1) == '(' {
					dsub = true
				}
				if dsub && k.ch[k.i] == '<' && k.c(k.i+1) == '<' && k.c(k.i+2) != '<' {
					if r := k.qheredoc(k.i); r > 0 {
						k.i = r
						continue
					}
				}
				k.add(k.ch[k.i])
				k.i++
			}
			k.intok = true
		case c == '$' && k.c(k.i+1) == '(':
			k.word()
			k.subnext = true
			k.cmd()
			k.subnext = false
			k.i++
		case c == '$' && k.c(k.i+1) == '{':
			for k.i <= n && k.ch[k.i] != '}' {
				k.add(k.ch[k.i])
				k.i++
			}
			k.add('}')
		case c == ' ' || c == '\t':
			k.word()
		case c == '#' && !k.intok:
			for k.i < n && k.ch[k.i+1] != '\n' {
				k.i++
			}
		case c == '\n':
			k.word()
			k.cmd()
			if k.nhd > 0 {
				k.i = k.heredocs(k.i)
			}
		case c == '>' || c == '<':
			k.redir()
		case c == '&' && k.c(k.i+1) == '>':
			k.word()
			k.i++
			if k.c(k.i+1) == '>' {
				k.i++
			}
			k.mode = "w"
		case c == '`':
			k.word()
			k.bt = !k.bt
			k.subnext = k.bt
			k.cmd()
			k.subnext = false
		case c == ';' || c == '&' || c == '|' || c == '(' || c == ')':
			k.word()
			k.cmd()
		default:
			k.add(c)
		}
	}
	k.word()
	k.cmd()
}
