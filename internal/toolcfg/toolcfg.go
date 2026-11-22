// Package toolcfg: 개인 설정 폴더($HERDR_TOOLS_CONFIG, 기본 ~/.config/herdr-tools) 읽기와 개인 값 검사(lint)·sync-lint.
// herdr-tools-config·herdr-tools-publish·herdr-sync Go 판이 함께 쓴다. 꼴·출력·종료 코드는 셸 판 bin/herdr-tools-config와 같다.
package toolcfg

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"herdr-tools/internal/proc"
)

const me = "herdr-tools-config"

// KeyRe: 설정 키 꼴 <영역>.<이름>
var KeyRe = regexp.MustCompile(`^[a-z]+\.[a-z0-9_]+$`)

// Dir: 설정 폴더
func Dir() string {
	if d := os.Getenv("HERDR_TOOLS_CONFIG"); d != "" {
		return d
	}
	return filepath.Join(os.Getenv("HOME"), ".config/herdr-tools")
}

// ErrDup: 같은 키가 두 번 — Msg는 셸 판이 표준 오류에 찍는 줄 그대로(앞머리 「herdr-tools-config: 」 포함)
type ErrDup struct{ Msg string }

func (e *ErrDup) Error() string { return e.Msg }

// Get: dir/config에서 key 값. 파일이 없거나 키가 없으면 found=false. 같은 키 두 번은 *ErrDup, 못 읽으면 그 밖의 오류.
// 같은 키 두 번은 묻는 키와 상관없이 파일 차례로 처음 걸린 것에서 멈춘다(셸 판 awk와 같음)
func Get(dir, key string) (val string, found bool, err error) {
	f := dir + "/config"
	if !isFile(f) {
		return "", false, nil
	}
	data, err := os.ReadFile(f)
	if err != nil {
		return "", false, err
	}
	seen := map[string]int{}
	for i, l := range splitLines(data) {
		t := strings.TrimLeft(strings.TrimSuffix(string(l), "\r"), " \t")
		if t == "" || t[0] == '#' {
			continue
		}
		k := t
		if j := strings.IndexAny(t, " \t"); j >= 0 {
			k = t[:j]
		}
		if !KeyRe.MatchString(k) {
			continue
		}
		if p, ok := seen[k]; ok {
			return "", false, &ErrDup{fmt.Sprintf("%s: 같은 키 두 번 %s (%s:%d·%d)", me, k, f, p, i+1)}
		}
		seen[k] = i + 1
		if k == key {
			val, found = strings.Trim(t[len(k):], " \t"), true
		}
	}
	return val, found, nil
}

// Lookup: Get + 기본값(셸 판 `get <키> <기본값>`). 같은 키 두 번이면 그 줄을 stderr에 찍고 오류
func Lookup(dir, key, def string, stderr io.Writer) (string, error) {
	v, ok, err := Get(dir, key)
	if err != nil {
		var d *ErrDup
		if errors.As(err, &d) {
			fmt.Fprintln(stderr, d.Msg)
		}
		return "", err
	}
	if !ok {
		return def, nil
	}
	return v, nil
}

// HasPatterns: 패턴 파일에 주석·빈 줄이 아닌 줄이 있나(publish·sync가 검사 전에 보는 것 — 없으면 검사 못 함)
func HasPatterns(file string) bool {
	data, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	for _, l := range splitLines(data) {
		t := strings.TrimLeft(string(l), " \t\r\v\f")
		if t != "" && t[0] != '#' {
			return true
		}
	}
	return false
}

type pattern struct {
	text string
	re   *regexp.Regexp
}

// Lint: 셸 판 `lint [--strict] [<파일>…]`. 걸린 줄은 stdout에 「파일:줄: 패턴」. 종료 코드 0 깨끗 · 1 걸림 · 2 검사 못 함 · 3 strict인데 패턴 없음.
// 패턴은 셸 판(LC_ALL=C grep -E -i)처럼 바이트 단위로 읽는다(ereToGo) — Go가 못 읽는 패턴은 틀린 패턴(종료 2, 검사 없이 통과하지 않게)
func Lint(dir string, strict bool, files []string, stdout, stderr io.Writer) int {
	return LintAt("", dir, strict, files, stdout, stderr)
}

// LintAt: Lint와 같되 상대 경로 파일을 base 아래에서 읽는다(출력·경로 글자 검사는 준 글자 그대로 — 셸 판 `(cd base && lint …)`)
func LintAt(base, dir string, strict bool, files []string, stdout, stderr io.Writer) int {
	at := func(f string) string {
		if base == "" || filepath.IsAbs(f) {
			return f
		}
		return filepath.Join(base, f)
	}
	die2 := func(f string, a ...any) int { fmt.Fprintf(stderr, me+": "+f+"\n", a...); return 2 }
	pf := dir + "/private-patterns.txt"
	if !isFile(pf) {
		pf = os.Getenv("HOME") + "/.local/share/herdr-tools/private-patterns.txt"
	}
	if !isFile(pf) {
		p := "검사 건너뜀"
		if strict {
			p = "검사 못 함(--strict)"
		}
		fmt.Fprintf(stderr, "%s: 개인 값 패턴 파일 없음(%s/private-patterns.txt, ~/.local/share/herdr-tools/private-patterns.txt) — %s\n", me, dir, p)
		if strict {
			return 3
		}
		return 0
	}
	data, err := os.ReadFile(pf)
	if err != nil {
		return die2("패턴 파일을 읽지 못했다: %s", pf)
	}
	var pats []pattern
	for _, p := range splitLines(data) {
		s := strings.TrimSuffix(string(p), "\r")
		t := strings.TrimLeft(s, " \t\v\f\r")
		if t == "" || t[0] == '#' {
			continue
		}
		re, err := compileERE(s, true)
		if err != nil {
			return die2("틀린 패턴 %s (%s)", s, pf)
		}
		pats = append(pats, pattern{s, re})
	}
	if strict && len(pats) == 0 {
		fmt.Fprintf(stderr, "%s: 개인 값 패턴 줄 없음(%s) — 검사 못 함\n", me, pf)
		return 3
	}
	if len(files) == 0 {
		if proc.Run(exec.Command("git", "rev-parse", "--git-dir")) != nil {
			return die2("lint: 파일을 주거나 git 저장소 안에서 부른다")
		}
		// 지금 읽은 패턴 파일 하나만 뺀다(패턴 글자가 자기에게 걸림). 실제 경로(링크 풂)로 견준다
		self, _ := filepath.EvalSymlinks(pf)
		if self != "" {
			self, _ = filepath.Abs(self)
		}
		here, _ := os.Getwd()
		if h, err := filepath.EvalSymlinks(here); err == nil {
			here = h
		}
		out, err := proc.Output(exec.Command("git", "ls-files", "-z"))
		if err != nil {
			return die2("lint: git ls-files 실패 — 추적 파일을 검사하지 못했다")
		}
		for _, f := range strings.Split(string(out), "\x00") {
			if f == "" || here+"/"+f == self {
				continue
			}
			if _, err := os.Lstat(f); err == nil { // 작업 트리에서 지운 추적 파일은 뺀다
				files = append(files, f)
			}
		}
	}
	if len(pats) == 0 || len(files) == 0 {
		return 0
	}
	type hit struct {
		file string
		line int
		text string
	}
	var hits []hit
	add := func(f string, n int, why string) { hits = append(hits, hit{f, n, fmt.Sprintf("%s:%d: %s", f, n, why)}) }
	failed := false
	for _, f := range files {
		if fi, err := os.Lstat(at(f)); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			// 심볼릭 링크는 git에 대상 경로 글자로 들어간다: 그 글자를 견주고 대상 내용은 읽지 않는다
			if t, err := os.Readlink(at(f)); err == nil {
				t = byteRunes([]byte(t))
				for _, p := range pats {
					if p.re.MatchString(t) {
						add(f, 1, p.text)
					}
				}
			}
		} else if data, err := os.ReadFile(at(f)); err != nil {
			failed = true
		} else {
			// 이진·UTF-8 아닌 파일도 바이트로 본다
			for i, l := range splitLines(data) {
				m := byteRunes(l)
				for _, p := range pats {
					if p.re.MatchString(m) {
						add(f, i+1, p.text)
					}
				}
			}
		}
		if !strings.HasPrefix(f, "/") { // 경로 글자: 상대 경로로 준 것만(절대 경로는 임시 파일 같은 남의 자리)
			m := byteRunes([]byte(f))
			for _, p := range pats {
				if p.re.MatchString(m) {
					add(f, 0, "경로 "+p.text)
				}
			}
		}
	}
	if failed {
		return die2("검사 못 함 — 못 읽은 파일이 있다")
	}
	if len(hits) == 0 {
		return 0
	}
	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.file != b.file {
			return a.file < b.file
		}
		if a.line != b.line {
			return a.line < b.line
		}
		return a.text < b.text
	})
	w := bufio.NewWriter(stdout)
	for _, h := range hits {
		w.WriteString(h.text + "\n")
	}
	w.Flush()
	return 1
}

// byteRunes: 바이트 하나를 글자 하나로 — 0x00~0x7F는 그대로, 0x80~0xFF는 대소문자 접기가 없는 사용자 영역 글자 U+E080~U+E0FF.
// 셸 판 LC_ALL=C처럼 「.」 하나가 바이트 하나, 대소문자 무시는 ASCII만, 0x80 이상은 낱말 글자가 아니다(패턴도 같게 옮긴다)
func byteRunes(b []byte) string {
	ascii := true
	for _, c := range b {
		if c >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return string(b)
	}
	var s strings.Builder
	for _, c := range b {
		writeByte(&s, c)
	}
	return s.String()
}

func writeByte(s *strings.Builder, c byte) {
	if c < 0x80 {
		s.WriteByte(c)
	} else {
		s.WriteRune(0xE000 + rune(c))
	}
}

// ERE: 셸 판 LC_ALL=C grep -E(icase면 -i)처럼 한 줄을 바이트 단위로 거는 정규식 — 설정으로 ERE를 받는 도구가 쓴다(publish.secrets 등)
type ERE struct{ re *regexp.Regexp }

// CompileERE: ERE 한 줄을 ereToGo로 옮겨 컴파일. Go가 못 읽는 꼴(역참조 등)은 오류 — 부른 쪽은 「검사 못 함」으로 멈춘다
func CompileERE(p string, icase bool) (*ERE, error) {
	re, err := compileERE(p, icase)
	if err != nil {
		return nil, err
	}
	return &ERE{re}, nil
}

func compileERE(p string, icase bool) (*regexp.Regexp, error) {
	g, err := ereToGo(p, icase)
	if err != nil {
		return nil, err
	}
	return regexp.Compile(g)
}

// Match: 한 줄(줄바꿈 뺀 바이트)이 걸리는지
func (e *ERE) Match(line []byte) bool { return e.re.MatchString(byteRunes(line)) }

// ereToGo: ERE 패턴 한 줄을 셸 판(LC_ALL=C grep -E, icase면 -i)과 같게 거는 Go 정규식으로 — 바이트는 byteRunes처럼 옮기고,
// 괄호 식 안의 역슬래시는 ERE처럼 글자 그대로, \< \> [[:<:]] [[:>:]]는 \b.
// C 로캘 ERE와 같은 뜻을 보장 못 하는 꼴은 오류(재감사 R-18·R-09 — 맥 BSD grep은 \d를 숫자, \v·\A·\z를 그 글자, \b를 아무 데나로 읽어 Go와 어긋난다):
// 글자·숫자 앞 역슬래시, 괄호 식 안 동등 클래스 [=…=]·정렬 요소 [.….]·POSIX 12개 밖 문자 부류, Go 전용 「(?」·「{,」,
// 낱말 글자가 늘 붙지 않는 \< \> [[:<:]] [[:>:]](\<는 바로 뒤 원소가, \>는 바로 앞 원소가 빠질 수 없는 낱말 글자
// — 낱말 글자 하나나 낱말 글자만 든 괄호 식, 수량자는 없거나 최소 1 — 일 때만: 그때만 Go \b가 낱말 시작·끝과 같다).
// 역참조 등 Go가 못 읽는 꼴은 컴파일 오류. 부른 쪽은 둘 다 종료 2(조용히 다르게 걸지 않게)
func ereToGo(p string, icase bool) (string, error) {
	bad := func(why string) (string, error) {
		return "", fmt.Errorf("C 로캘 ERE와 뜻이 같다고 보장 못 하는 꼴(%s): %q", why, p)
	}
	var b strings.Builder
	if icase {
		b.WriteString("(?i)")
	}
	lastWord := false // 바로 앞 원소(수량자까지)가 늘 낱말 글자로 끝나는가 — \>를 받는가
	for i := 0; i < len(p); i++ {
		c := p[i]
		word := false
		switch {
		case strings.HasPrefix(p[i:], "[[:<:]]"), strings.HasPrefix(p[i:], "[[:>:]]"):
			if (p[i+3] == '<' && !wordAtom(p, i+7)) || (p[i+3] == '>' && !lastWord) {
				return bad("낱말 글자가 늘 붙지는 않는 낱말 경계")
			}
			b.WriteString(`\b`)
			i += 6
		case c == '\\' && i+1 < len(p):
			switch n := p[i+1]; {
			case n == '<' && !wordAtom(p, i+2), n == '>' && !lastWord:
				return bad("낱말 글자가 늘 붙지는 않는 낱말 경계")
			case n == '<' || n == '>':
				b.WriteString(`\b`)
			case n >= 0x80: // Go는 ASCII 아닌 글자의 역슬래시를 못 읽는다 — 글자 그대로(BSD grep도 그 바이트)
				writeByte(&b, n)
			case isWordByte(n) && n != '_':
				return bad(`글자·숫자 앞 역슬래시 \` + string(n))
			default:
				b.WriteByte('\\')
				b.WriteByte(n)
			}
			i++
		case c == '(' && i+1 < len(p) && p[i+1] == '?', c == '{' && i+1 < len(p) && p[i+1] == ',':
			return bad("Go 전용 " + p[i:i+2])
		case c == '[':
			j := bracketEnd(p, i)
			if j < 0 {
				b.WriteByte(c) // 닫히지 않은 괄호 — Go도 틀린 패턴으로 본다
				continue
			}
			if why := bracketUnsure(p[i : j+1]); why != "" {
				return bad(why)
			}
			word = bracketWordOnly(p[i : j+1])
			for ; i <= j; i++ {
				if p[i] == '\\' {
					b.WriteString(`\\`)
				} else {
					writeByte(&b, p[i])
				}
			}
			i = j
		case c == '{':
			if n, e, ok := braceMin(p, i); ok {
				word = lastWord && n >= 1
				b.WriteString(p[i : e+1])
				i = e
			} else {
				b.WriteByte(c)
			}
		case c == '+':
			word = lastWord
			b.WriteByte(c)
		default:
			word = isWordByte(c)
			writeByte(&b, c)
		}
		lastWord = word
	}
	return b.String(), nil
}

// wordAtom: p[k]에서 시작하는 원소가 빠질 수 없는 낱말 글자인가 — 낱말 글자 하나나 낱말 글자만 든 괄호 식, 뒤 수량자는 없거나 최소 1
func wordAtom(p string, k int) bool {
	var e int
	switch {
	case k < len(p) && isWordByte(p[k]):
		e = k + 1
	case k < len(p) && p[k] == '[':
		j := bracketEnd(p, k)
		if j < 0 || !bracketWordOnly(p[k:j+1]) {
			return false
		}
		e = j + 1
	default:
		return false
	}
	if e >= len(p) {
		return true
	}
	switch p[e] {
	case '*', '?':
		return false
	case '{':
		n, _, ok := braceMin(p, e)
		return ok && n >= 1
	}
	return true
}

// braceMin: p[i]의 { 로 여는 수량자 {n} {n,} {n,m}의 최소 n과 닫는 }의 자리. 꼴이 아니면 ok 거짓
func braceMin(p string, i int) (n, end int, ok bool) {
	k := i + 1
	for k < len(p) && p[k] >= '0' && p[k] <= '9' {
		n = n*10 + int(p[k]-'0')
		if n > 1000 {
			return 0, 0, false
		}
		k++
	}
	if k == i+1 {
		return 0, 0, false
	}
	if k < len(p) && p[k] == ',' {
		k++
		for k < len(p) && p[k] >= '0' && p[k] <= '9' {
			k++
		}
	}
	if k < len(p) && p[k] == '}' {
		return n, k, true
	}
	return 0, 0, false
}

// bracketWordOnly: 괄호 식 [ … ](부정 아님)이 낱말 글자만 받는가 — 낱말 글자, 같은 갈래(숫자·소문자·대문자) 안의 범위, alnum·alpha·digit·upper·lower·xdigit 부류
func bracketWordOnly(s string) bool {
	in := s[1 : len(s)-1]
	if in == "" || in[0] == '^' {
		return false
	}
	for k := 0; k < len(in); {
		if strings.HasPrefix(in[k:], "[:") {
			e := strings.Index(in[k+2:], ":]")
			if e < 0 {
				return false
			}
			switch in[k+2 : k+2+e] {
			case "alnum", "alpha", "digit", "upper", "lower", "xdigit":
			default:
				return false
			}
			k += 2 + e + 2
			continue
		}
		if k+2 < len(in) && in[k+1] == '-' {
			lo, hi := in[k], in[k+2]
			if !((isDigit(lo) && isDigit(hi)) || (lo >= 'a' && hi <= 'z' && lo <= hi) || (lo >= 'A' && hi <= 'Z' && lo <= hi)) {
				return false
			}
			k += 3
			continue
		}
		if !isWordByte(in[k]) {
			return false
		}
		k++
	}
	return true
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// isWordByte: C 로캘 낱말 글자(ASCII 글자·숫자·_)
func isWordByte(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// posixClasses: C 로캘과 Go가 같게 읽는 문자 부류(ASCII)
var posixClasses = map[string]bool{"alnum": true, "alpha": true, "blank": true, "cntrl": true, "digit": true, "graph": true,
	"lower": true, "print": true, "punct": true, "space": true, "upper": true, "xdigit": true}

// bracketUnsure: 괄호 식 하나(bracketEnd로 찾은 [ … ])에 Go가 C 로캘과 다르게 읽는 꼴이 있으면 그 까닭, 없으면 ""
func bracketUnsure(s string) string {
	k := 1
	if k < len(s) && s[k] == '^' {
		k++
	}
	if k < len(s) && s[k] == ']' {
		k++
	}
	for ; k < len(s)-1; k++ {
		if s[k] != '[' || strings.IndexByte(":.=", s[k+1]) < 0 {
			continue
		}
		d := s[k+1]
		e := strings.Index(s[k+2:], string(d)+"]")
		if e < 0 {
			return "닫히지 않은 [" + string(d)
		}
		if d != ':' {
			return "동등 클래스·정렬 요소 [" + string(d) + "…" + string(d) + "]"
		}
		if name := s[k+2 : k+2+e]; !posixClasses[name] {
			return "POSIX 밖 문자 부류 [:" + name + ":]"
		}
		k += 2 + e + 1
	}
	return ""
}

// bracketEnd: p[i]의 [로 여는 괄호 식을 닫는 ]의 자리(없으면 -1). 맨 앞 ^ 다음의 ]는 글자, [:…:] [.….] [=…=]는 통째로 건너뛴다
func bracketEnd(p string, i int) int {
	k := i + 1
	if k < len(p) && p[k] == '^' {
		k++
	}
	if k < len(p) && p[k] == ']' {
		k++
	}
	for ; k < len(p); k++ {
		if p[k] == '[' && k+1 < len(p) && strings.IndexByte(":.=", p[k+1]) >= 0 {
			e := strings.Index(p[k+2:], string(p[k+1])+"]")
			if e < 0 {
				return -1
			}
			k += 2 + e + 1
			continue
		}
		if p[k] == ']' {
			return k
		}
	}
	return -1
}

// splitLines: grep처럼 줄로 나눈다(끝 줄바꿈 뒤의 빈 조각은 줄이 아니다)
func splitLines(b []byte) [][]byte {
	l := bytes.Split(b, []byte("\n"))
	if len(l) > 0 && len(l[len(l)-1]) == 0 {
		l = l[:len(l)-1]
	}
	return l
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// ListFields: sync.list 줄 하나를 칸으로(awk 기본 나누기, 줄 끝 CR 뺌). 주석·빈 줄은 nil
func ListFields(l string) []string {
	l = strings.TrimSuffix(l, "\r")
	t := strings.TrimLeft(l, " \t")
	if t == "" || t[0] == '#' {
		return nil
	}
	return strings.Fields(l)
}

// macRoot: sync.list 맥 경로의 bin 위 폴더(.local/bin/x → .local/). 셸 판 awk sub 두 번과 같다
func macRoot(p string) string {
	i := strings.LastIndexByte(p, '/')
	if i < 0 {
		return ""
	}
	q := p[:i]
	if j := strings.LastIndexByte(q, '/'); j >= 0 {
		return q[:j+1]
	}
	return ""
}

func firstLine(f string) string {
	data, _ := os.ReadFile(f)
	s := string(data)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}
