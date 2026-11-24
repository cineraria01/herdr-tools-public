//go:build !windows

package main

// 범위(2026-11-24 verify-scope): 조각(go test 하나)마다 「그 결과가 기댈 수 있는 것」의 내용 지문을 내고, 통과한 지문을 이 기계의
// 사용자 캐시(herdr-tools/verify-pass/<지문>)에 남긴다. 같은 지문의 기록이 있으면 그 조각은 건너뛸 수 있다.
//
// 지문의 입력(넓게 — 덜 건너뛰는 것은 느릴 뿐, 잘못 건너뛰는 것은 깨진 것이 main에 든다):
//   - 꾸러미의 파일: 꾸러미 폴더 아래 추적 파일(안쪽 꾸러미 폴더는 뺌) — .go·embed·스크립트·help, 조각이 도는 꾸러미는 시험 파일
//     (_test.go·testdata)까지. 가져오거나 도구로 지어 돌리는 꾸러미의 시험 파일은 바이너리에 안 들어 뺀다.
//   - (가) 조각이 도는 꾸러미와 그 시험 짓기의 모듈 안 의존 전부(go list -test의 <꾸러미>.test Deps).
//   - (나) 글자로 가리킨 꾸러미: 위 꾸러미들의 Go 글자 상수(주석은 아님)·Go 아닌 파일에 폴더 상대 경로(cmd/hc-lane, internal/proc)가
//     나오거나, cmd/<이름>의 「<이름>」이 명령 꼴 낱말(앞뒤가 빈칸·/·따옴표·셸 기호·끝 — exec.Command("hc-lane"), "bin/hc-lane land")로
//     나오면 그 꾸러미와 그 짓기 의존 — 그 짓는 파일에서 또 찾는다(hc-fill → hc-lane → hc-merger …). 조각이 도는 꾸러미는 시험 파일에서도
//     찾는다. 「hc-sweep이 정리한다」「hc-sweep.log」 같은 문장·다른 이름은 명령이 아니다. 손으로 적은 표가 없다.
//   - (다) 공통: 이 판·go 판·OS·아키텍처·go 환경 몇, 어느 꾸러미 폴더에도 안 든 추적 파일 전부(go.mod·go.sum·.claude/·config.example/·
//     launchd/·tests/line-budget.tsv …, 문서는 빼고), cmd/*/OWNER, cmd/ 아래 폴더 이름 목록(디스크 그대로), 검증 본문(tests/repo/verify의 시험 아닌 소스),
//     Go 글자 상수가 가리키는 문서(아래), 조각의 argv·env.
//   - 문서(docs/ 아래, 그 밖 꾸러미 폴더 밖의 *.md)는 입력이 아니다 — 단 Go 글자 상수(tests/repo 밖)에 그 경로(docs/…)가 나오거나,
//     뿌리의 *.md 이름이 저장소 뿌리를 읽는 시험 파일(clitest Repo(t)·Root(t), "../..")의 글자 상수에 나오면 공통 입력이다. 추적 파일 전부를
//     훑는 TestTrackedNoSecret(cmd/herdr-tools-publish)은 싼 단계(tests/repo TestPublishSecrets)가 늘 같은 저장소에 돌리므로 문서를 넣지 않는다.
//
// 기록은 작업 트리가 깨끗할 때만(추적 파일 변경 없음 + 문서 밖 추적 안 된 파일 없음) 읽고 쓴다 — 그래서 내용은 git blob 해시로 본다.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"go/scanner"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const scopeVer = "verify-scope 2" // 지문 짓는 법이 바뀌면 올린다(옛 기록이 저절로 안 맞게)

// rootRead: 저장소 뿌리를 읽는 시험 파일의 꼴 — clitest의 Repo(t)·Root(t)(rlRepo(t) 같은 가짜 저장소 도우미는 아님), "../.."
var rootRead = regexp.MustCompile(`(^|[^A-Za-z0-9_])(Repo|Root)\(t\)|"\.\./\.\.`)

// scopeEnv: 시험 짓기·실행을 바꾸는 go 환경 — 지문에 든다
var scopeEnv = []string{"GOFLAGS", "GOEXPERIMENT", "CGO_ENABLED", "GOAMD64", "GOARM64", "GODEBUG", "GOTOOLCHAIN"}

type scopeIdx struct {
	root   string
	files  map[string][]string // 꾸러미 → 「모드 blob 경로」 줄
	tdeps  map[string][]string // 꾸러미 → 시험 짓기의 모듈 안 의존
	bdeps  map[string][]string // 꾸러미 → 짓기의 모듈 안 의존
	refs   map[string][]string // 꾸러미 → 그 짓는 파일에 글자로 나온 다른 꾸러미
	trefs  map[string][]string // 꾸러미 → 그 시험 파일(_test.go·testdata)에 글자로 나온 다른 꾸러미 — 조각이 도는 꾸러미만 본다
	byDir  map[string]string   // 뿌리 기준 폴더 → 꾸러미
	common string              // (다)의 해시
	tree   string              // 색인 때 HEAD^{tree}
	dirty  string              // 깨끗하지 않은 까닭(깨끗하면 "")
}

// scopeIndex: 저장소 하나의 색인 — git ls-files·git status·go list 한 번씩과 꾸러미 파일 읽기
func scopeIndex(root string) (*scopeIdx, error) {
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	x := &scopeIdx{root: root, files: map[string][]string{}, tdeps: map[string][]string{}, bdeps: map[string][]string{},
		refs: map[string][]string{}, trefs: map[string][]string{}, byDir: map[string]string{}}
	if err := x.goList(); err != nil {
		return nil, err
	}
	ls, err := scopeGit(root, "ls-files", "-s", "-z")
	if err != nil {
		return nil, err
	}
	owner := x.owner
	var commons, docs []string
	texts := map[string][][]byte{}  // 꾸러미 → 짓는 파일 내용(글자 찾기 — Go는 글자 상수만)
	ttexts := map[string][][]byte{} // 꾸러미 → 시험 파일(_test.go·testdata) 내용
	var rootReaders [][]byte        // 저장소 뿌리를 읽는 Go 시험 파일
	var goText [][]byte             // tests/repo 밖 Go 소스·config.example
	for _, e := range strings.Split(strings.TrimSuffix(ls, "\x00"), "\x00") {
		meta, p, ok := strings.Cut(e, "\t") // 「모드 blob 단계\t경로」
		f := strings.Fields(meta)
		if !ok || len(f) != 3 {
			return nil, fmt.Errorf("git ls-files 줄을 못 읽음: %q", e)
		}
		line := f[0] + " " + f[1] + " " + p
		k := owner(p)
		if strings.HasPrefix(p, "cmd/") && filepath.Base(p) == "OWNER" { // internal/toolcfg가 cmd/*/OWNER를 글롭으로 읽는다 — 공통
			k = ""
		}
		switch {
		case k != "":
			x.files[k] = append(x.files[k], line)
			b, err := os.ReadFile(filepath.Join(root, p))
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			s := b
			if strings.HasSuffix(p, ".go") {
				s = goStrings(b)
			}
			if scopeTestFile(p) {
				ttexts[k] = append(ttexts[k], s)
			} else {
				texts[k] = append(texts[k], s)
			}
			if strings.HasSuffix(p, ".go") && !strings.HasPrefix(p, "tests/repo/") { // 문서 경로는 글자 상수만(주석의 「docs/… 참고」는 아님)
				goText = append(goText, s)
				if strings.HasSuffix(p, "_test.go") && rootRead.Match(b) {
					rootReaders = append(rootReaders, s)
				}
			}
		case scopeDoc(p):
			docs = append(docs, line)
		default:
			commons = append(commons, line)
			if strings.HasPrefix(p, "config.example/") {
				b, err := os.ReadFile(filepath.Join(root, p))
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					return nil, err
				}
				goText = append(goText, b)
			}
		}
	}
	for _, l := range docs { // Go 소스가 가리키는 문서는 공통 입력
		p := l[strings.LastIndex(l, " ")+1:]
		hit := false
		if strings.Contains(p, "/") {
			hit = anyContains(goText, p)
		} else {
			hit = anyContains(rootReaders, p)
		}
		if hit {
			commons = append(commons, l)
		}
	}
	// 글자 찾기: 폴더 상대 경로(앞뒤가 낱말 글자가 아닌 자리), cmd/<이름>은 「<이름>」이 명령 꼴 낱말로(앞뒤가 빈칸·/·따옴표·셸 기호·끝 —
	// 「hc-sweep이 …」 같은 문장이나 hc-sweep.log 같은 다른 이름은 아님)
	find := func(bs [][]byte, k string) []string {
		var out []string
		for d, q := range x.byDir {
			if q == k {
				continue
			}
			c := strings.HasPrefix(d, "cmd/") && !strings.Contains(d[4:], "/")
			if anyWord(bs, d) || c && anyCmd(bs, d[4:]) {
				out = append(out, q)
			}
		}
		sort.Strings(out)
		return out
	}
	for k, bs := range texts {
		x.refs[k] = find(bs, k)
	}
	for k, bs := range ttexts {
		x.trefs[k] = find(bs, k)
	}
	ents, err := os.ReadDir(filepath.Join(root, "cmd"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var cmds []string
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".") { // 세션 훅이 남기는 무시 폴더(cmd/.omc 등) — 명령이 아니다(감사 낮음)
			continue
		}
		cmds = append(cmds, e.Name())
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\n%s %s/%s\n", scopeVer, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	for _, k := range scopeEnv {
		fmt.Fprintf(h, "env %s=%s\n", k, os.Getenv(k))
	}
	fmt.Fprintf(h, "cmd/ %s\n", strings.Join(cmds, " "))
	sort.Strings(commons)
	fmt.Fprintf(h, "common\n%s\n", strings.Join(commons, "\n"))
	if k := x.byDir["tests/repo/verify"]; k != "" { // 검증 본문의 소스(시험 파일은 그 꾸러미 조각의 입력일 뿐)
		for _, l := range x.files[k] {
			if !scopeTestFile(l[strings.LastIndex(l, " ")+1:]) {
				fmt.Fprintf(h, "verify %s\n", l)
			}
		}
	}
	x.common = hex.EncodeToString(h.Sum(nil))
	if x.tree, err = scopeGit(root, "rev-parse", "HEAD^{tree}"); err != nil {
		return nil, err
	}
	if x.dirty, err = x.dirtyNow(); err != nil {
		return nil, err
	}
	return x, nil
}

// owner: p가 든 가장 깊은 꾸러미 폴더의 꾸러미(없으면 "")
func (x *scopeIdx) owner(p string) string {
	for d := filepath.Dir(p); d != "."; d = filepath.Dir(d) {
		if k, ok := x.byDir[d]; ok {
			return k
		}
	}
	return ""
}

// dirtyNow: 지금 작업 트리가 깨끗하지 않은 까닭(깨끗하면 "") — 추적 파일 변경, 문서 밖 추적 안 된 파일, 색인 때와 다른 HEAD 트리
func (x *scopeIdx) dirtyNow() (string, error) {
	st, err := scopeGit(x.root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return "", err
	}
	for _, e := range strings.Split(strings.TrimSuffix(st, "\x00"), "\x00") {
		if len(e) < 4 {
			continue
		}
		if e[:2] != "??" || x.owner(e[3:]) != "" || !scopeDoc(e[3:]) {
			return e, nil
		}
	}
	if t, err := scopeGit(x.root, "rev-parse", "HEAD^{tree}"); err != nil || t != x.tree {
		return "HEAD 트리가 바뀜", err
	}
	return "", nil
}

// goList: 꾸러미 폴더와 의존(go list -test 한 번 — 「P.test」의 Deps가 시험 짓기 전부)
func (x *scopeIdx) goList() error {
	c := exec.Command("go", "list", "-test", "-f", "{{.ImportPath}}\t{{.Dir}}\t{{join .Deps \",\"}}", "./...")
	c.Dir = x.root
	var eb bytes.Buffer
	c.Stderr = &eb
	out, err := c.Output()
	if err != nil {
		return fmt.Errorf("go list -test: %v %s", err, strings.TrimSpace(eb.String()))
	}
	in := func(ds string) []string {
		var r []string
		seen := map[string]bool{}
		for _, d := range strings.Split(ds, ",") {
			d, _, _ = strings.Cut(d, " ")
			d = strings.TrimSuffix(d, "_test")
			if strings.HasPrefix(d, mod) && !strings.HasSuffix(d, ".test") && !seen[d] {
				seen[d] = true
				r = append(r, d)
			}
		}
		return r
	}
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Split(l, "\t")
		if len(f) != 3 {
			return fmt.Errorf("go list 줄을 못 읽음: %q", l)
		}
		ip := f[0]
		switch {
		case strings.HasSuffix(ip, ".test"):
			x.tdeps[strings.TrimSuffix(ip, ".test")] = in(f[2])
		case !strings.Contains(ip, " ") && !strings.HasSuffix(ip, "_test"):
			d, err := filepath.EvalSymlinks(f[1])
			if err != nil {
				d = f[1]
			}
			rel, err := filepath.Rel(x.root, d)
			if err != nil || strings.HasPrefix(rel, "..") {
				return fmt.Errorf("꾸러미 %s의 폴더 %s가 저장소 밖", ip, f[1])
			}
			if rel != "." { // 뿌리 꾸러미는 폴더 소유를 안 한다(그 파일은 공통 입력)
				x.byDir[rel] = ip
			}
			x.bdeps[ip] = in(f[2])
		}
	}
	return nil
}

// key: 조각(argv·env)의 지문. argv의 꾸러미(가져오기 경로나 ./폴더)를 모르면 오류 — 그 조각은 늘 돈다
func (x *scopeIdx) key(argv, env []string) (string, error) {
	ks, own, err := x.deps(argv)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\nargv %q\nenv %q\n", x.common, argv, env)
	for _, k := range ks {
		fmt.Fprintf(h, "pkg %s\n", k)
		for _, l := range x.files[k] { // 조각이 도는 꾸러미는 파일 전부, 가져오거나 지어 돌리는 꾸러미는 짓는 파일만
			if own[k] || !scopeTestFile(l[strings.LastIndex(l, " ")+1:]) {
				fmt.Fprintf(h, "%s\n", l)
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// scopeTestFile: 시험에만 쓰는 파일(_test.go·testdata 아래) — 꾸러미를 짓거나 도구로 돌릴 때는 안 든다
func scopeTestFile(p string) bool {
	return strings.HasSuffix(p, "_test.go") || strings.Contains("/"+p, "/testdata/")
}

// deps: argv의 꾸러미가 기대는 꾸러미 전부(이름순)와 그중 조각이 도는 꾸러미 — (가) 시험 짓기 의존, (나) 글자로 가리킨 꾸러미와
// 그 짓기 의존을 닫힐 때까지
func (x *scopeIdx) deps(argv []string) ([]string, map[string]bool, error) {
	set := map[string]bool{}
	var scan []string
	add := func(k string, deps []string) {
		for _, d := range append([]string{k}, deps...) {
			if !set[d] {
				set[d], scan = true, append(scan, d)
			}
		}
	}
	own := map[string]bool{} // 조각이 도는 꾸러미 — 그 시험 파일의 글자도 본다
	for _, a := range argv {
		k := ""
		switch {
		case strings.HasPrefix(a, mod):
			k = a
		case strings.HasPrefix(a, "./"):
			k = x.byDir[strings.TrimSuffix(filepath.Clean(a), "/")]
			if k == "" {
				return nil, nil, fmt.Errorf("꾸러미를 모름: %s", a)
			}
		default:
			continue
		}
		if _, ok := x.bdeps[k]; !ok {
			return nil, nil, fmt.Errorf("꾸러미를 모름: %s", a)
		}
		td, ok := x.tdeps[k]
		if !ok {
			td = x.bdeps[k]
		}
		add(k, td)
		own[k] = true
	}
	if len(own) == 0 {
		return nil, nil, errors.New("argv에 꾸러미가 없다")
	}
	// 글자로 가리킨 꾸러미(도구)와 그 짓기 의존 — 그 짓는 파일에서 또 찾는다(도구로 지어 돌리는 꾸러미의 시험 파일은 입력이 아니다)
	for i := 0; i < len(scan); i++ {
		qs := x.refs[scan[i]]
		if own[scan[i]] {
			qs = append(append([]string{}, qs...), x.trefs[scan[i]]...)
		}
		for _, q := range qs {
			add(q, x.bdeps[q])
		}
	}
	ks := make([]string, 0, len(set))
	for k := range set {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks, own, nil
}

// scopeDoc: 문서 — docs/ 아래, 그 밖의 *.md(꾸러미 폴더 안 것은 부르는 쪽이 먼저 꾸러미 파일로 친다). config.example/·.claude/ 아래는
// .md여도 문서가 아니다(설정 틀·검증 규칙 — 시험·도구가 읽는다, 감사 atozaxx-e9aphf 중간)
func scopeDoc(p string) bool {
	if strings.HasPrefix(p, "config.example/") || strings.HasPrefix(p, ".claude/") {
		return false
	}
	return strings.HasPrefix(p, "docs/") || strings.HasSuffix(p, ".md")
}

// goStrings: Go 소스의 글자 상수(문자열·문자)만 이어 붙인다 — 주석의 도구 이름은 기대는 것이 아니다. 못 읽으면 소스 그대로(넓게)
func goStrings(src []byte) []byte {
	var s scanner.Scanner
	bad := false
	s.Init(token.NewFileSet().AddFile("", -1, len(src)), src, func(token.Position, string) { bad = true }, 0)
	var out bytes.Buffer
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.STRING || tok == token.CHAR {
			if u, err := strconv.Unquote(lit); err == nil { // 「a\tb」의 \t가 빈칸으로 — 낱말 경계를 바로 본다
				lit = u
			}
			out.WriteString(lit)
			out.WriteByte('\n')
		}
	}
	if bad {
		return src
	}
	return out.Bytes()
}

func anyContains(bs [][]byte, s string) bool {
	for _, b := range bs {
		if bytes.Contains(b, []byte(s)) {
			return true
		}
	}
	return false
}

// anyWord: s가 앞뒤로 [A-Za-z0-9_-]가 아닌 자리에 나오는지(UTF-8 글자도 경계로 친다 — 경로는 넓게)
func anyWord(bs [][]byte, s string) bool {
	w := func(c byte) bool {
		return c == '_' || c == '-' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
	}
	edge := func(c byte) bool { return !w(c) }
	return anyAt(bs, s, edge, edge)
}

// anyCmd: s가 명령 꼴 낱말로 나오는지 — 앞은 처음·빈칸·/·따옴표·( = ; | &, 뒤는 끝·빈칸·따옴표·) ; | &
// (「exec.Command("hc-lane", …)」「bin/hc-lane land」「"hc-lane land main"」은 맞고, 「hc-sweep이 …」「hc-sweep.log」「hc-lane:」는 아니다)
func anyCmd(bs [][]byte, s string) bool {
	return anyAt(bs, s, func(c byte) bool { return strings.IndexByte(" \t\n\r/\"'`(=;|&", c) >= 0 },
		func(c byte) bool { return strings.IndexByte(" \t\n\r\"'`);|&", c) >= 0 })
}

// anyAt: s가 앞 글자가 before, 뒤 글자가 after를 만족하는(처음·끝은 늘 만족) 자리에 나오는지
func anyAt(bs [][]byte, s string, before, after func(byte) bool) bool {
	for _, b := range bs {
		for off := 0; ; {
			i := bytes.Index(b[off:], []byte(s))
			if i < 0 {
				break
			}
			i += off
			if (i == 0 || before(b[i-1])) && (i+len(s) == len(b) || after(b[i+len(s)])) {
				return true
			}
			off = i + 1
		}
	}
	return false
}

func scopeGit(root string, args ...string) (string, error) {
	c := exec.Command("git", args...)
	c.Dir = root
	var eb bytes.Buffer
	c.Stderr = &eb
	out, err := c.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %v %s", args[0], err, strings.TrimSpace(eb.String()))
	}
	return string(out), nil
}

// scopeRun: 한 검증의 범위 판정. items[0]은 홀로 도는 cotenant 단계, 그 뒤는 조각
type scopeRun struct {
	mode string    // observe·on·off
	x    *scopeIdx // nil이면 지문 없음(off·지문 실패·깨끗하지 않음) — 전부 돌고 기록을 안 쓴다
	keys []string  // "" = 지문 없음(늘 돈다)
	can  []bool    // 통과 기록과 같은 지문
	skip []bool    // 실제로 건너뜀
	secs []int     // can이면 기록의 초
	put  []passRec // 끝에 쓸 통과 기록
}

type passRec struct {
	key, label string
	secs       int
}

// scopePlan: HERDR_VERIFY_SCOPE(없음·observe / on / off)대로 판정하고 알림 줄을 찍는다. 지문 실패는 한 줄 알림 뒤 전부 돈다.
// observe: 전부 돌되 건너뛸 수 있는 조각을 알린다 — 단 전부가 기록과 같으면(문서만 고친 변경 등) 전부 건너뛴다. on: 기록과 같은 조각은 건너뛴다.
// HERDR_VERIFY_ALL=1이면 어떤 경우에도 전부 돈다(기록은 쓴다)
func scopePlan(root string, items []shard, w io.Writer) *scopeRun {
	n := len(items)
	r := &scopeRun{mode: os.Getenv("HERDR_VERIFY_SCOPE"), keys: make([]string, n), can: make([]bool, n), skip: make([]bool, n), secs: make([]int, n)}
	switch r.mode {
	case "", "observe":
		r.mode = "observe"
	case "on", "off":
	default:
		fmt.Fprintf(w, "verify: HERDR_VERIFY_SCOPE=%q 는 observe·on·off가 아니라 observe로 본다\n", r.mode)
		r.mode = "observe"
	}
	if r.mode == "off" {
		return r
	}
	t0 := time.Now()
	x, err := scopeIndex(root)
	switch {
	case err != nil:
		fmt.Fprintf(w, "알림 범위 지문 실패(%v) — 조각을 전부 돌리고 통과 기록을 안 쓴다\n", err)
		return r
	case x.dirty != "":
		fmt.Fprintf(w, "알림 범위: 작업 트리가 깨끗하지 않다(%s) — 통과 기록을 안 읽고 안 쓰고 전부 돈다\n", strings.TrimSpace(x.dirty))
		return r
	}
	r.x = x
	all := true
	for i, it := range items {
		if k, err := x.key(it.argv, it.env); err == nil {
			r.keys[i] = k
			r.secs[i], r.can[i] = passSecs(k)
		}
		all = all && r.can[i]
	}
	for i := range items {
		r.skip[i] = os.Getenv("HERDR_VERIFY_ALL") != "1" && r.can[i] && (r.mode == "on" || all)
	}
	fmt.Fprintf(w, "범위 지문 %.1f초 (HERDR_VERIFY_SCOPE=%s)\n", time.Since(t0).Seconds(), r.mode)
	return r
}

// note: 띄우는 줄에 덧붙일 말 — 건너뛸 수 있었는데 도는 조각
func (r *scopeRun) note(i int) string {
	if r.can[i] && !r.skip[i] {
		return " — 건너뛸 수 있었음(입력이 통과 기록과 같음)"
	}
	return ""
}

// done: 조각 i가 끝남. 통과면 기록을 미뤄 두고(끝에 트리가 그대로일 때만 쓴다), 건너뛸 수 있었던 조각이 실패면 놓친 입력
func (r *scopeRun) done(i int, label string, rc, secs int, w io.Writer) {
	if r.keys[i] == "" {
		return
	}
	if rc == 0 {
		r.put = append(r.put, passRec{r.keys[i], label, secs})
		return
	}
	if r.can[i] {
		fmt.Fprintf(w, "놓친 입력: %s — 건너뛸 조각이 실패 (지문 %s, 기록 %s)\n", label, r.keys[i][:12], filepath.Join(scopeCache(), "verify-scope-misses.log"))
		head, _ := scopeGit(r.x.root, "rev-parse", "HEAD")
		if err := missLog(strings.TrimSpace(head), r.keys[i], label); err != nil {
			fmt.Fprintf(w, "경고 놓친 입력 기록을 못 썼다: %v\n", err)
		}
	}
}

// finish: 요약 한 줄, 작업 트리가 처음 그대로면 미뤄 둔 통과 기록을 쓰고 오래된 기록을 정리한다
func (r *scopeRun) finish(w io.Writer) {
	if r.x == nil {
		return
	}
	k, j, s := 0, 0, 0
	for i := range r.can {
		if r.can[i] {
			k, s = k+1, s+r.secs[i]
		}
		if r.skip[i] {
			j++
		}
	}
	fmt.Fprintf(w, "범위(%s): 조각 %d개 중 %d개 건너뛸 수 있었음(약 %d초), 건너뜀 %d개\n", r.mode, len(r.can), k, s, j)
	if len(r.put) == 0 {
		return
	}
	if d, err := r.x.dirtyNow(); err != nil || d != "" {
		fmt.Fprintf(w, "알림 범위: 검증 중 작업 트리가 바뀌었다(%s %v) — 통과 기록을 안 쓴다\n", strings.TrimSpace(d), err)
		return
	}
	for _, p := range r.put {
		if err := passPut(p.key, p.label, p.secs); err != nil {
			fmt.Fprintf(w, "경고 통과 기록을 못 썼다: %v\n", err)
			return
		}
	}
	passPrune(time.Now())
}

// 통과 기록: 사용자 캐시 herdr-tools/verify-pass/<지문>(「초\t이름표」 한 줄), 놓친 입력은 herdr-tools/verify-scope-misses.log
const (
	passMax = 3000                // 기록 개수 상한(넘으면 오래된 것부터)
	passAge = 30 * 24 * time.Hour // 이보다 오래된 기록은 지운다
)

// scopeCache: 기록 폴더의 부모(시험이 바꾼다)
var scopeCache = func() string {
	d, err := os.UserCacheDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "herdr-tools")
}

func passHas(key string) bool {
	_, ok := passSecs(key)
	return ok
}

// passSecs: 기록이 있으면 그때 걸린 초(첫 칸, 못 읽으면 0)
func passSecs(key string) (int, bool) {
	b, err := os.ReadFile(filepath.Join(scopeCache(), "verify-pass", key))
	if err != nil {
		return 0, false
	}
	f, _, _ := strings.Cut(string(b), "\t")
	n, _ := strconv.Atoi(f)
	return n, true
}

// passPut: 기록 「초\t이름표」
func passPut(key, label string, secs int) error {
	d := filepath.Join(scopeCache(), "verify-pass")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d, key), fmt.Appendf(nil, "%d\t%s\n", secs, label), 0o644)
}

// passPrune: passAge보다 오래된 기록과, passMax를 넘는 오래된 기록을 지운다
func passPrune(now time.Time) {
	d := filepath.Join(scopeCache(), "verify-pass")
	ents, err := os.ReadDir(d)
	if err != nil {
		return
	}
	type rec struct {
		n string
		t time.Time
	}
	var rs []rec
	for _, e := range ents {
		if fi, err := e.Info(); err == nil {
			rs = append(rs, rec{e.Name(), fi.ModTime()})
		}
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].t.After(rs[j].t) })
	for i, r := range rs {
		if i >= passMax || now.Sub(r.t) > passAge {
			os.Remove(filepath.Join(d, r.n)) //nolint:errcheck // 못 지우면 다음에
		}
	}
}

// missLog: 놓친 입력(건너뛸 수 있었던 조각이 실패) 한 줄 — 「시각\tHEAD\t지문\t이름표」
func missLog(head, key, label string) error {
	if err := os.MkdirAll(scopeCache(), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(scopeCache(), "verify-scope-misses.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\t%s\t%s\t%s\n", time.Now().Format(time.RFC3339), head, key, label)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}
