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
		re, err := regexp.Compile(ereToGo(s))
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

// ereToGo: ERE 패턴 한 줄을 셸 판(LC_ALL=C grep -E -i)과 같게 거는 Go 정규식으로 — 바이트는 byteRunes처럼 옮기고,
// 괄호 식 안의 역슬래시는 ERE처럼 글자 그대로, \< \> [[:<:]] [[:>:]]는 \b.
// ponytail: \b는 낱말 시작·끝을 가리지 않아 셸보다 느슨하다(「tok\<x」 같은 꼴을 더 건다 — 개인 값 검사라 더 거는 쪽).
// 역참조 등 Go가 못 읽는 꼴은 컴파일 오류 → 종료 2(조용히 통과하지 않게)
func ereToGo(p string) string {
	var b strings.Builder
	b.WriteString("(?i)")
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case strings.HasPrefix(p[i:], "[[:<:]]"), strings.HasPrefix(p[i:], "[[:>:]]"):
			b.WriteString(`\b`)
			i += 6
		case c == '\\' && i+1 < len(p):
			i++
			switch n := p[i]; {
			case n == '<' || n == '>':
				b.WriteString(`\b`)
			case n >= 0x80: // Go는 ASCII 아닌 글자의 역슬래시를 못 읽는다 — 글자 그대로
				writeByte(&b, n)
			default:
				b.WriteByte('\\')
				b.WriteByte(n)
			}
		case c == '[':
			j := bracketEnd(p, i)
			if j < 0 {
				b.WriteByte(c) // 닫히지 않은 괄호 — Go도 틀린 패턴으로 본다
				continue
			}
			for ; i <= j; i++ {
				if p[i] == '\\' {
					b.WriteString(`\\`)
				} else {
					writeByte(&b, p[i])
				}
			}
			i = j
		default:
			writeByte(&b, c)
		}
	}
	return b.String()
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

// SyncLint: 셸 판 `sync-lint <범용 저장소> <sync.list>`. 더할 줄은 stdout, 머리 줄은 stderr. 0 통과 · 1 줄이 모자람 · 2 sync.list 없음
func SyncLint(repo, list string, stdout, stderr io.Writer) int {
	data, err := os.ReadFile(list)
	if !isFile(list) || err != nil {
		fmt.Fprintf(stderr, "%s: sync.list 없음: %s\n", me, list)
		return 2
	}
	var rows [][]string
	for _, l := range strings.Split(string(data), "\n") {
		if f := ListFields(l); len(f) >= 4 {
			rows = append(rows, f)
		}
	}
	// 도구가 함께 쓰는 lib: lib/<이름>/OWNER 첫 줄이 그 lib를 쓰는 도구
	dep := map[string][]string{}
	owners, _ := filepath.Glob(repo + "/lib/*/OWNER")
	for _, o := range owners {
		if isFile(o) {
			w := firstLine(o)
			dep[w] = append(dep[w], filepath.Base(filepath.Dir(o)))
		}
	}
	isLib := regexp.MustCompile(`^tools:lib/[^/]+/?$`)
	type need struct{ b, dst, tool, t string }
	var needs []need
	lib := map[string]bool{}
	for _, f := range rows {
		if binTool(f[2]) != "" {
			x := binTool(f[2])
			for _, tl := range append([]string{x}, dep[x]...) {
				needs = append(needs, need{f[0], macRoot(f[1]) + "lib/" + tl + "/**", tl, f[3]})
			}
			continue
		}
		if isLib.MatchString(f[2]) && strings.HasSuffix(f[1], "/**") {
			lib[strings.TrimSuffix(f[2][10:], "/")+"\x00"+f[1]+"\x00"+f[3]] = true
		}
	}
	bad := 0
	for _, n := range needs {
		if lib[n.tool+"\x00"+n.dst+"\x00"+n.t] || lib[n.tool+"\x00"+n.dst+"\x00all"] || !isDir(repo+"/lib/"+n.tool) {
			continue
		}
		if bad == 0 {
			fmt.Fprintf(stderr, "%s: %s — lib 나무 줄이 없다(bin만 깔리면 그 도구가 멈춘다). 이 줄을 더한다:\n", me, list)
		}
		fmt.Fprintf(stdout, "%s  %s  tools:lib/%s/  %s\n", n.b, n.dst, n.tool, n.t)
		bad = 1
	}
	if syncLintGo(repo, list, rows, stdout, stderr) {
		bad = 1
	}
	return bad
}

var binRe = regexp.MustCompile(`^tools:bin/[^/]+$`)

func binTool(s string) string {
	if binRe.MatchString(s) {
		return s[10:]
	}
	return ""
}

// syncLintGo: Go 판 도구(cmd/<이름>/, 그 bin 도구는 OWNER 첫 줄 — 없으면 <이름>)의 bin 줄마다 Go 소스 나무 줄 넷이
// 그 대상을 덮는 대상(같음·all, local은 macos도)으로 있어야 한다. 모자라면 참
func syncLintGo(repo, list string, rows [][]string, stdout, stderr io.Writer) bool {
	if _, err := os.Stat(repo + "/go.mod"); err != nil {
		return false
	}
	own := map[string]bool{}
	cmds, _ := filepath.Glob(repo + "/cmd/*")
	for _, c := range cmds {
		if !isDir(c) {
			continue
		}
		o := filepath.Base(c)
		if isFile(c + "/OWNER") {
			o = firstLine(c + "/OWNER")
		}
		own[o] = true
	}
	needs := [4][2]string{{"lib/herdr-go/go.mod", "tools:go.mod"}, {"lib/herdr-go/cmd/**", "tools:cmd/"},
		{"lib/herdr-go/internal/**", "tools:internal/"}, {"lib/go-run.sh", "tools:lib/go-run.sh"}}
	type bin struct{ b, root, t string }
	var bins []bin
	have := map[string]bool{}
	for _, f := range rows {
		if x := binTool(f[2]); x != "" && own[x] {
			bins = append(bins, bin{f[0], macRoot(f[1]), f[3]})
			continue
		}
		have[f[1]+"\t"+f[2]+"\t"+f[3]] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, b := range bins {
		for _, p := range needs {
			key := b.root + p[0] + "\t" + p[1] + "\t"
			if have[key+b.t] || have[key+"all"] || (b.t == "local" && have[key+"macos"]) || seen[key+b.t] {
				continue
			}
			seen[key+b.t] = true
			out = append(out, fmt.Sprintf("%s  %s  %s  %s", b.b, b.root+p[0], p[1], b.t))
		}
	}
	if len(out) == 0 {
		return false
	}
	fmt.Fprintf(stderr, "%s: %s — Go 소스 나무 줄이 없다(Go 판 도구가 셸 판으로 떨어진다). 이 줄을 더한다:\n", me, list)
	fmt.Fprintln(stdout, strings.Join(out, "\n"))
	return true
}
