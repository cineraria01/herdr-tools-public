// hc-split Go 판(개발용, 배포 안 함 — 2026-11-21 사용자 「모두 고로 옮기고 싶어」): dev/split/hc-split.py와 같은 출력·종료 코드.
//
//	go run ./dev/split split          # 작업 트리의 bin/<도구>를 나눈다(계획 dev/split/plan.tsv)
//	go run ./dev/split check <기준 rev> # 기준 rev의 원본과 지금 작업 트리를 대조
//
// 규칙은 hc-split.py 머리말 그대로다(아래 doc). 파이썬과 같게 하려고: 줄 나누기는 str.splitlines(keepends) 꼴, 읽기는 universal
// newlines(\r\n·\r → \n), 공백 판정은 str.strip/str.isspace, 정규식의 \s는 파이썬 유니코드 공백 집합으로 바꿔 쓴다.
// 다른 점: git show가 실패하면 파이썬은 예외 추적을 찍고 1, 여기는 「hc-split: git show … 실패」 한 줄로 1.
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"unicode"

	"herdr-tools/internal/gitx"
)

const bashPath = "/bin/bash"

var doc = "hc-split: bin/hc-lane·bin/hc-fill의 최상위 함수를 기능별 lib/<도구>/<모듈>.sh로 기계적으로 옮기고(split), 대조한다(check).\n" +
	"\n" +
	"  python3 -I dev/split/hc-split.py split          # 작업 트리의 bin/<도구>를 나눈다(계획 dev/split/plan.tsv)\n" +
	"  python3 -I dev/split/hc-split.py check <기준 rev> # 기준 rev의 원본과 지금 작업 트리를 대조\n" +
	"\n" +
	"나누기 규칙: 최상위 함수(첫 칸의 `이름()`)와 그 바로 위에 붙은 주석 줄(빈 줄 없이 붙은 첫 칸 `#` 줄)이 한 덩이다.\n" +
	"덩이 끝은 bash 파서로 정한다: 시작 줄, 그다음 첫 칸 `}` 줄들 가운데 처음으로 `bash -n`이 경고 없이 받는 곳.\n" +
	"덩이마다 다음을 bash로 증명한다(아니면 멈춤): 덩이 앞 원본 전체가 경고 없이 파싱된다(=덩이가 최상위, if·case·heredoc 밖),\n" +
	"덩이 하나만 source하면 정확히 그 이름의 함수 하나만 생긴다.\n" +
	"본체에는 덩이를 지운 나머지를 원래 순서 그대로 두고, 첫 덩이 자리에 lib를 읽는 몇 줄을 넣는다(지운 자리에 빈 줄이\n" +
	"겹치면 하나만 남긴다). lib 파일은 머리 주석 한 줄 + 그 모듈 덩이를 원래 순서대로.\n" +
	"\n" +
	"check(대조): 기준 원본에서 덩이를 다시 뽑아(위 증명 포함)\n" +
	"  1) 모든 함수가 lib 가운데 정확히 한 곳에 정확히 한 번, 본문 글자 그대로(덩이 글자 = lib 안 덩이 글자)\n" +
	"  2) lib 안 덩이 순서 = 원본 순서, lib에는 덩이 말고 머리 주석·빈 줄뿐\n" +
	"  3) 본체 = 원본에서 덩이를 지운 것(빈 줄 겹침만 다름) + 로더 줄\n" +
	"  4) bash -n 전부 통과, lib를 모두 source한 깨끗한 bash의 함수 목록·declare -f = 원본 덩이들의 것\n" +
	"  5) 원본에서 어떤 함수 이름이 그 정의보다 앞 줄에 명령 낱말로 나오면 목록으로 알린다(정의가 앞당겨져 뜻이 바뀌는지 사람이 본다)\n"

var (
	root   string
	here   string
	fnLine = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_-]*)\(\)`)
)

func die(msg string) {
	fmt.Fprintln(os.Stderr, "hc-split: "+msg)
	os.Exit(1)
}

// 파이썬 str.isspace 집합(Go unicode.IsSpace + \x1c-\x1f)
func pySpace(r rune) bool { return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f) }

func strip(s string) string { return strings.TrimFunc(s, pySpace) }

// 파이썬 정규식 \s(유니코드 str)
const pyS = `[\t\n\v\f\r \x1c-\x1f\x{85}\x{a0}\p{Z}]`

// splitlines: 파이썬 str.splitlines(keepends=True)
func splitlines(s string) []string {
	var out []string
	start := 0
	rs := []rune(s)
	b := 0 // 바이트 자리
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		n := len(string(r))
		switch r {
		case '\n', '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
			out = append(out, s[start:b+n])
			start = b + n
		case '\r':
			if i+1 < len(rs) && rs[i+1] == '\n' {
				i++
				n++
			}
			out = append(out, s[start:b+n])
			start = b + n
		}
		b += n
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// universal newlines(파이썬 텍스트 읽기)
func uni(b []byte) string {
	return strings.ReplaceAll(strings.ReplaceAll(string(b), "\r\n", "\n"), "\r", "\n")
}

func readText(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		die(fmt.Sprintf("읽기 실패: %v", err))
	}
	return uni(b)
}

func parses(text string) bool {
	var e bytes.Buffer
	c := exec.Command(bashPath, "-n")
	c.Stdin, c.Stderr = strings.NewReader(text), &e
	return c.Run() == nil && strip(e.String()) == ""
}

// pyRepr: 파이썬 repr(str)
func pyRepr(s string) string {
	q := "'"
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		q = `"`
	}
	var b strings.Builder
	b.WriteString(q)
	for _, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case string(r) == q:
			b.WriteString(`\` + q)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r > 0x7f && !unicode.IsPrint(r) && r != ' ':
			switch {
			case r <= 0xff:
				fmt.Fprintf(&b, `\x%02x`, r)
			case r <= 0xffff:
				fmt.Fprintf(&b, `\%s%04x`, "u", r)
			default:
				fmt.Fprintf(&b, `\U%08x`, r)
			}
		default:
			b.WriteRune(r)
		}
	}
	b.WriteString(q)
	return b.String()
}

func reprList(xs []string) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = pyRepr(x)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func intList(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = fmt.Sprint(x)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

type planT struct {
	script, lib, pfx string
	opens            [][2]string // (함수, 모듈)
}

func readPlan() []*planT {
	var out []*planT
	var cur *planT
	for _, ln := range splitlines(readText(filepath.Join(here, "plan.tsv"))) {
		f := strings.FieldsFunc(ln, pySpace)
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		if f[0] == "=" {
			cur = &planT{script: f[1], lib: f[2], pfx: f[3]}
			out = append(out, cur)
		} else {
			cur.opens = append(cur.opens, [2]string{f[1], f[0]})
		}
	}
	return out
}

type block struct {
	s, i, e int
	name    string
}

func join(lines []string) string { return strings.Join(lines, "") }

// blocks: [(시작 줄(주석 포함), 함수 줄, 끝 줄(포함), 이름)] — 0부터 센 줄 번호. bash로 증명
func blocks(lines []string, what string) []block {
	var out []block
	for i := 0; i < len(lines); {
		m := fnLine.FindStringSubmatch(lines[i])
		if m == nil {
			i++
			continue
		}
		name, end := m[1], -1
		cands := []int{i}
		for k := i + 1; k < len(lines); k++ {
			if strings.HasPrefix(lines[k], "}") {
				cands = append(cands, k)
			}
		}
		for _, e := range cands {
			if parses(join(lines[i : e+1])) {
				end = e
				break
			}
		}
		if end < 0 {
			die(fmt.Sprintf("%s:%d %s 끝을 못 찾았다", what, i+1, name))
		}
		s := i
		for s > 0 && strings.HasPrefix(lines[s-1], "#") && !strings.HasPrefix(lines[s-1], "#!") &&
			(len(out) == 0 || s-1 > out[len(out)-1].e) {
			s--
		}
		out = append(out, block{s, i, end, name})
		i = end + 1
	}
	for _, b := range out {
		if !parses(join(lines[:b.s])) {
			die(fmt.Sprintf("%s:%d %s 앞이 최상위로 끝나지 않는다(if·case·heredoc 안?)", what, b.i+1, b.name))
		}
		if got := definedNames(join(lines[b.s : b.e+1])); len(got) != 1 || got[0] != b.name {
			die(fmt.Sprintf("%s:%d 덩이가 함수 %s 하나가 아니다: %s", what, b.i+1, b.name, reprList(got)))
		}
	}
	count := map[string]int{}
	for _, b := range out {
		count[b.name]++
	}
	var dup []string
	for n, c := range count {
		if c > 1 {
			dup = append(dup, n)
		}
	}
	if len(dup) > 0 {
		sort.Strings(dup)
		die(fmt.Sprintf("%s: 같은 이름 함수가 두 번: %s", what, reprList(dup)))
	}
	return out
}

func defined(text string, body bool) string {
	f, err := os.CreateTemp("", "*.sh")
	if err != nil {
		die(fmt.Sprintf("임시 파일: %v", err))
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(text); err != nil {
		die(fmt.Sprintf("임시 파일 쓰기: %v", err))
	}
	if err := f.Close(); err != nil {
		die(fmt.Sprintf("임시 파일 닫기: %v", err))
	}
	what := "compgen -A function"
	if body {
		what = "declare -f"
	}
	var o, e bytes.Buffer
	c := exec.Command("env", "-i", "PATH=/usr/bin:/bin", bashPath, "--norc", "--noprofile", "-c",
		`. "$1" >/dev/null 2>&1 </dev/null || exit 9; `+what, "x", f.Name())
	c.Stdout, c.Stderr = &o, &e
	if err := c.Run(); err != nil {
		code := -1
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		die(fmt.Sprintf("source 실패(%d): %s", code, strip(e.String())))
	}
	return o.String()
}

func definedNames(text string) []string { return strings.FieldsFunc(defined(text, false), pySpace) }

func assign(p *planT, bl []block, what string) ([]string, []string) {
	opens := map[string]string{}
	for _, o := range p.opens {
		opens[o[0]] = o[1]
	}
	mod, mods := "", []string(nil)
	for _, b := range bl {
		if m, ok := opens[b.name]; ok {
			mod = m
		}
		if mod == "" {
			die(fmt.Sprintf("%s: 첫 함수 %s의 모듈이 계획에 없다", what, b.name))
		}
		mods = append(mods, mod)
	}
	have := map[string]bool{}
	for _, b := range bl {
		have[b.name] = true
	}
	var missing []string
	for f := range opens {
		if !have[f] {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		die(fmt.Sprintf("%s: 계획의 여는 함수가 원본에 없다: %s", what, reprList(missing)))
	}
	var order []string
	for _, m := range mods {
		if !contains(order, m) {
			order = append(order, m)
		}
	}
	return mods, order
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func loader(p *planT, order []string) []string {
	tool, v := filepath.Base(p.script), p.pfx+"_"
	return []string{
		fmt.Sprintf("# 함수는 기능별 파일 %s/*.sh에 있다(2026-11-20 herdr-tools-split — 여러 레인이 한 파일을 고쳐 부딪혀서).\n", p.lib),
		fmt.Sprintf("# 깔린 곳은 이 파일 옆 ../lib/%s(~/.local/lib/%s). 링크로 불려도 링크가 가리키는 실제 파일 옆을 본다\n", tool, tool),
		fmt.Sprintf("%ss=$0\n", v),
		fmt.Sprintf(`while [ -L "$%[1]ss" ]; do %[1]sl=$(readlink "$%[1]ss"); case $%[1]sl in /*) %[1]ss=$%[1]sl ;; *) %[1]ss=$(dirname "$%[1]ss")/$%[1]sl ;; esac; done`+"\n", v),
		fmt.Sprintf(`%[1]sd=$(dirname "$%[1]ss")/../lib/%[2]s`+"\n", v, tool),
		fmt.Sprintf("for %sf in %s; do\n", v, strings.Join(order, " ")),
		fmt.Sprintf(`  . "$%[1]sd/$%[1]sf.sh" || { printf '%[2]s: 함수 파일을 못 읽었다: %%s (herdr-sync가 lib도 까는지)\n' "$%[1]sd/$%[1]sf.sh" >&2; exit 1; }`+"\n", v, tool),
		"done\n",
	}
}

type gen struct {
	main  []string
	order []string          // 모듈 순서
	libs  map[string]string // 모듈 → 파일 글자
	bl    []block
	mods  []string
	lines []string
}

func generate(p *planT, src string) gen {
	lines := splitlines(src)
	if !strings.HasSuffix(lines[len(lines)-1], "\n") {
		die(p.script + ": 끝 줄바꿈 없음")
	}
	bl := blocks(lines, p.script)
	mods, order := assign(p, bl, p.script)
	gone := map[int]block{}
	for _, b := range bl {
		for k := b.s; k <= b.e; k++ {
			gone[k] = b
		}
	}
	var main []string
	put := false
	for k := 0; k < len(lines); {
		if b, ok := gone[k]; ok {
			if !put {
				main = append(main, loader(p, order)...)
				put = true
			}
			k = b.e + 1
			// 지운 자리에 빈 줄이 겹치면 하나만(덩이는 최상위라 뒤 빈 줄도 최상위 — heredoc 안이 아니다)
			for k < len(lines) && !isGone(gone, k) && strip(lines[k]) == "" && len(main) > 0 && strip(main[len(main)-1]) == "" {
				k++
			}
			continue
		}
		main = append(main, lines[k])
		k++
	}
	tool := filepath.Base(p.script)
	libs := map[string]string{}
	for _, m := range order {
		body := []string{fmt.Sprintf("# %s 함수: %s (%s가 읽는다. 함수는 %s 안 원래 정의 순서 그대로)\n", tool, m, p.script, p.script)}
		var prev *block
		for j := range bl {
			if mods[j] != m {
				continue
			}
			b := bl[j]
			// 원본에서 바로 붙어 있던 덩이는 붙여, 아니면 빈 줄 하나를 두고
			if prev == nil || prev.e+1 != b.s {
				body = append(body, "\n")
			}
			body = append(body, lines[b.s:b.e+1]...)
			prev = &bl[j]
		}
		libs[m] = join(body)
	}
	return gen{main, order, libs, bl, mods, lines}
}

func isGone(g map[int]block, k int) bool { _, ok := g[k]; return ok }

func split() {
	for _, p := range readPlan() {
		path := filepath.Join(root, p.script)
		g := generate(p, readText(path))
		d := filepath.Join(root, p.lib)
		if err := os.MkdirAll(d, 0o777); err != nil {
			die(fmt.Sprintf("폴더 만들기: %v", err))
		}
		for _, m := range g.order {
			if err := os.WriteFile(filepath.Join(d, m+".sh"), []byte(g.libs[m]), 0o666); err != nil {
				die(fmt.Sprintf("쓰기: %v", err))
			}
		}
		if err := os.WriteFile(path, []byte(join(g.main)), 0o666); err != nil {
			die(fmt.Sprintf("쓰기: %v", err))
		}
		fmt.Printf("%s: 함수 %d개 → %s/{%s}.sh, 본체 %d줄\n", p.script, len(g.bl), p.lib, strings.Join(g.order, ","), len(g.main))
	}
}

func main() {
	_, file, _, ok := runtime.Caller(0)
	if ok && filepath.IsAbs(file) {
		here = filepath.Dir(file)
	} else { // -trimpath로 지었으면 지금 폴더에서 위로 dev/split/plan.tsv를 찾는다
		wd, err := os.Getwd()
		if err != nil {
			die(fmt.Sprintf("지금 폴더: %v", err))
		}
		for d := wd; ; d = filepath.Dir(d) {
			if _, err := os.Stat(filepath.Join(d, "dev/split/plan.tsv")); err == nil {
				here = filepath.Join(d, "dev/split")
				break
			}
			if d == filepath.Dir(d) {
				die("dev/split/plan.tsv를 못 찾았다")
			}
		}
	}
	root = filepath.Dir(filepath.Dir(here))
	a := os.Args[1:]
	switch {
	case len(a) >= 1 && a[0] == "split":
		split()
	case len(a) == 2 && a[0] == "check":
		os.Exit(check(a[1]))
	default:
		fmt.Fprintln(os.Stderr, doc)
		os.Exit(1)
	}
}

// gitShow: git -C root show rev:path — 글자 그대로(끝 줄바꿈 포함, universal newlines)
func gitShow(rev, path string) string {
	var o, e bytes.Buffer
	if rc := gitx.Run(root, &o, &e, "show", rev+":"+path); rc != 0 {
		die(fmt.Sprintf("git show %s:%s 실패(%d): %s", rev, path, rc, strip(e.String())))
	}
	return uni(o.Bytes())
}
