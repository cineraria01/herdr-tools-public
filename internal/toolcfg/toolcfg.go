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
		re, err := regexp.Compile(ereToGo(s, true))
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
	re, err := regexp.Compile(ereToGo(p, icase))
	if err != nil {
		return nil, err
	}
	return &ERE{re}, nil
}

// Match: 한 줄(줄바꿈 뺀 바이트)이 걸리는지
func (e *ERE) Match(line []byte) bool { return e.re.MatchString(byteRunes(line)) }

// ereToGo: ERE 패턴 한 줄을 셸 판(LC_ALL=C grep -E, icase면 -i)과 같게 거는 Go 정규식으로 — 바이트는 byteRunes처럼 옮기고,
// 괄호 식 안의 역슬래시는 ERE처럼 글자 그대로, \< \> [[:<:]] [[:>:]]는 \b.
// ponytail: \b는 낱말 시작·끝을 가리지 않아 셸보다 느슨하다(「tok\<x」 같은 꼴을 더 건다 — 개인 값 검사라 더 거는 쪽).
// 역참조 등 Go가 못 읽는 꼴은 컴파일 오류 → 종료 2(조용히 통과하지 않게)
func ereToGo(p string, icase bool) string {
	var b strings.Builder
	if icase {
		b.WriteString("(?i)")
	}
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

// SyncLint: `herdr-tools-config sync-lint <범용 저장소> <sync.list>` — herdr-sync도 맞추기 전에 부른다. 더할 줄은 stdout, 머리 줄·지울 줄은 stderr.
// 0 통과 · 1 걷힌 셸 판 줄이 남음·바이너리 줄이 틀림·딸린 줄이 모자람 · 2 sync.list 없음
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
	bad := 0
	// 1. 걷힌 셸 판 줄(2026-11-22 go-shell-remove): 저장소에 없는 tools:lib/…·tools:…*.sh·*.py 줄, cmd/<도구>가 있는데 저장소에 없는
	// tools:bin/<도구> 줄이 남아 있으면 herdr-sync가 맥에 남은 옛 파일을 저장소로 다시 모은다(맥이 정본) — 맞추기 전에 멈추고 바이너리 줄을 권한다
	var stale, repl []string
	for _, f := range rows {
		r, ok := strings.CutPrefix(f[2], "tools:")
		if !ok || strings.Contains(r, "*") {
			continue
		}
		x, isBin := strings.CutPrefix(r, "bin/")
		goTool := isBin && !strings.Contains(x, "/") && isDir(repo+"/cmd/"+x)
		shellish := goTool || strings.HasPrefix(r, "lib/") || strings.HasSuffix(r, ".sh") || strings.HasSuffix(r, ".py")
		if !shellish || exists(repo+"/"+strings.TrimSuffix(r, "/")) {
			continue
		}
		stale = append(stale, strings.Join(f[:4], "  "))
		if goTool {
			repl = append(repl, fmt.Sprintf("%s  %sbin/%s  tools:cmd/%s  %s", f[0], macRoot(f[1]), x, x, f[3]))
		}
	}
	if len(stale) > 0 {
		fmt.Fprintf(stderr, "%s: %s — 셸 판이 걷혔다(저장소에 없는 셸·파이썬·포장 줄). 이 줄을 지우고(기계에 남은 것은 「지움」 줄로):\n  %s\n",
			me, list, strings.Join(stale, "\n  "))
		if len(repl) > 0 {
			fmt.Fprintf(stderr, "%s: 도구는 바이너리 줄로 바꾼다. 이 줄을 더한다:\n", me)
			fmt.Fprintln(stdout, strings.Join(repl, "\n"))
		}
		bad = 1
	}
	if syncLintBins(repo, list, rows, stdout, stderr) {
		bad = 1
	}
	return bad
}

var cmdLineRe = regexp.MustCompile(`^tools:cmd/([^/]+)/?$`)

// BinLine: sync.list 바이너리 줄(저장소 경로 tools:cmd/<도구> — 나무 줄 아님)이면 그 도구 이름. herdr-sync가 그 도구의 Go 바이너리를
// 맥 경로(<뿌리>bin/<도구>, 보통 .local/bin/<도구>)에 원자 교체로 깐다
func BinLine(mac, repoPath string) string {
	if strings.HasSuffix(mac, "/**") || strings.Contains(mac, "*") {
		return ""
	}
	if m := cmdLineRe.FindStringSubmatch(repoPath); m != nil {
		return m[1]
	}
	return ""
}

// covers: 대상 s가 대상 t를 덮나(같음·all, local은 macos도)
func covers(s, t string) bool { return s == t || s == "all" || (t == "local" && s == "macos") }

// syncLintBins: 바이너리 줄마다 — 맥 경로가 <뿌리>bin/<도구>이고 cmd/<도구>가 있어야 하고, 그 대상을 덮는 Go 소스 나무 줄 셋
// (그 기계에서 지을 때 — 릴리스를 못 받을 때)과, 곁 도구(cmd/<곁>/OWNER 첫 줄이 이 도구 — 예: hc-merger는 hc-lane이 부른다)의
// 바이너리 줄이 있어야 한다. 모자라면 참
func syncLintBins(repo, list string, rows [][]string, stdout, stderr io.Writer) bool {
	type bin struct{ b, root, tool, t string }
	var bins []bin
	have := map[string][]string{} // 맥 경로\t저장소 경로 → 대상들
	bad := false
	for _, f := range rows {
		x := BinLine(f[1], f[2])
		if x == "" {
			have[f[1]+"\t"+f[2]] = append(have[f[1]+"\t"+f[2]], f[3])
			continue
		}
		if !isDir(repo+"/cmd/"+x) || !strings.HasSuffix(f[1], "bin/"+x) {
			fmt.Fprintf(stderr, "%s: %s — 바이너리 줄이 틀렸다(맥 경로는 <뿌리>bin/%s, 저장소에 cmd/%s가 있어야): %s\n", me, list, x, x, strings.Join(f[:4], "  "))
			bad = true
			continue
		}
		bins = append(bins, bin{f[0], macRoot(f[1]), x, f[3]})
	}
	has := func(k, t string) bool {
		for _, s := range have[k] {
			if covers(s, t) {
				return true
			}
		}
		return false
	}
	hasBin := func(tool, t string) bool {
		for _, b := range bins {
			if b.tool == tool && covers(b.t, t) {
				return true
			}
		}
		return false
	}
	var out []string
	seen := map[string]bool{}
	more := func(l string) {
		if !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	srcs := [3][2]string{{"lib/herdr-go/go.mod", "tools:go.mod"}, {"lib/herdr-go/cmd/**", "tools:cmd/"}, {"lib/herdr-go/internal/**", "tools:internal/"}}
	cmds, _ := filepath.Glob(repo + "/cmd/*/OWNER")
	for _, b := range bins {
		for _, p := range srcs {
			if !has(b.root+p[0]+"\t"+p[1], b.t) {
				more(fmt.Sprintf("%s  %s  %s  %s", b.b, b.root+p[0], p[1], b.t))
			}
		}
		for _, o := range cmds {
			c := filepath.Base(filepath.Dir(o))
			if c != b.tool && firstLine(o) == b.tool && !hasBin(c, b.t) {
				more(fmt.Sprintf("%s  %sbin/%s  tools:cmd/%s  %s", b.b, b.root, c, c, b.t))
			}
		}
	}
	if len(out) == 0 {
		return bad
	}
	fmt.Fprintf(stderr, "%s: %s — 바이너리 줄에 딸린 줄이 없다(Go 소스 나무 — 그 기계에서 지을 때, 곁 도구 바이너리). 이 줄을 더한다:\n", me, list)
	fmt.Fprintln(stdout, strings.Join(out, "\n"))
	return true
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }
