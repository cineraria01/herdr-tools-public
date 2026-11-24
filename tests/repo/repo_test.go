package repo_test

// 저장소 싼 검사(몇 초): .claude/herdr-verify.sh가 무거운 시험보다 먼저 `go test ./tests/repo/`로 부른다(제출 전 사전 검사
// .claude/herdr-precheck.sh도 여기까지). 2026-11-22 go-shell-remove — 옛 tests/line-budget.test.sh·publish-secrets.test.sh와
// herdr-verify.sh의 문법·개인 값·싼 검사 빠짐(B28) 검사를 Go로. 판정·실패 글은 옛것 그대로다.
//  · 줄 수 예산(TestLineBudget) · 싼 검사 파일 빠짐(TestCheapKept, R-14) · gofmt(TestGofmt) · 셸 문법(TestShellSyntax)
//  · 셸 시험 파일 없음(TestNoShellTests)
//  · 개인 값(TestPrivateValues)·비밀 꼴(TestPublishSecrets) — secrets_test.go

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"herdr-tools/internal/clitest"
	"herdr-tools/internal/gitx"
)

// git: gitx.Out에 오류 글을 붙여서(실패한 명령의 반쪽 출력은 빈 값)
func git(dir string, args ...string) (string, int, string) {
	var eb bytes.Buffer
	out, rc := gitx.Out(dir, &eb, args...)
	return out, rc, strings.TrimSpace(eb.String())
}

// tracked: git ls-files -z [<꼴>…]. 실패는 오류(빈 저장소로 읽지 않는다)
func tracked(dir string, pats ...string) ([]string, error) {
	out, rc, e := git(dir, append([]string{"ls-files", "-z", "--"}, pats...)...)
	if rc != 0 {
		return nil, fmt.Errorf("git ls-files 실패(종료 %d): %s", rc, e)
	}
	var ps []string
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			ps = append(ps, p)
		}
	}
	return ps, nil
}

// ---- 줄 수 예산 ----
// 줄 수 톱니바퀴(2026-11-20 herdr-tools-split — 거대 파일 하나를 여러 레인이 함께 고쳐 서로 기다리고 부딪혔다).
// 시험 아닌 추적 파일(tests/·*/tests/·*.md 빼고)은 lineLimit을 넘지 않는다. 이미 넘은 파일은 budgetFile 「<줄 수> <경로>」에
// 적힌 줄 수까지만 — 늘면 실패(줄면 통과. 예산은 사람·지휘가 낮춘다). 새 거대 파일은 예산 없이 실패: 늘릴 일은 새 기능 파일로 나눈다.
// 레인 안(HC_LANE)에서는 예산 파일을 늘리지 못한다: origin/main과의 갈래점 판보다 예산 줄이 늘거나 새로 생기면 실패
// (줄이거나 지우기만은 된다. 갈래점에 예산 파일이 없으면 — 처음 만드는 판 — 안 본다).

const (
	lineLimit  = 600
	budgetFile = "tests/line-budget.tsv"
)

var budgetLine = regexp.MustCompile(`^([0-9]+)[ \t]+(.*)$`)

// readBudget: 「<줄 수> <경로>」 줄만(주석·빈 줄은 뺀다)
func readBudget(b string) map[string]int {
	m := map[string]int{}
	for _, l := range strings.Split(b, "\n") {
		if s := budgetLine.FindStringSubmatch(l); s != nil {
			n, err := strconv.Atoi(s[1])
			if err == nil {
				m[s[2]] = n
			}
		}
	}
	return m
}

func sortedKeys(m map[string]int) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// checkBudget: 위반마다 한 줄. 추적 목록·줄 수를 못 읽으면 오류. lane이면 갈래점 예산 base와도 견준다
func checkBudget(dir string, budget, base map[string]int, lane bool) ([]string, error) {
	files, err := tracked(dir)
	if err != nil {
		return nil, err
	}
	var v []string
	seen := map[string]bool{}
	for _, p := range files {
		// 문서·시험·기록 자료(testdata/ — 셸 판 골든처럼 다시 쓰지 않는 기록)는 줄 예산 밖
		if strings.HasSuffix(p, ".md") || strings.HasPrefix(p, "tests/") || strings.Contains(p, "/tests/") || strings.Contains(p, "/testdata/") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			return nil, fmt.Errorf("줄 수 읽기 실패: %v", err)
		}
		n := bytes.Count(b, []byte{'\n'})
		seen[p] = true
		if bn, ok := budget[p]; ok {
			if n > bn {
				v = append(v, fmt.Sprintf("늘었다 %s %d줄 (예산 %d) — 늘릴 것은 새 기능 파일로", p, n, bn))
			}
		} else if n > lineLimit {
			v = append(v, fmt.Sprintf("새 거대 파일 %s %d줄 (한도 %d) — 기능별 파일로 나눈다", p, n, lineLimit))
		}
	}
	for _, p := range sortedKeys(budget) {
		if !seen[p] {
			v = append(v, fmt.Sprintf("예산에 없는 파일 %s — 예산 줄을 지운다", p))
		}
	}
	if lane {
		for _, p := range sortedKeys(budget) {
			if o, ok := base[p]; !ok {
				v = append(v, fmt.Sprintf("레인이 예산을 늘렸다 %s %d (갈래점 없음)", p, budget[p]))
			} else if budget[p] > o {
				v = append(v, fmt.Sprintf("레인이 예산을 늘렸다 %s %d (갈래점 %d)", p, budget[p], o))
			}
		}
	}
	return v, nil
}

func TestLineBudget(t *testing.T) {
	R := clitest.Root(t)
	bb, err := os.ReadFile(filepath.Join(R, budgetFile))
	if err != nil {
		t.Fatalf("예산 파일이 없다: %s (%v)", budgetFile, err)
	}
	var base map[string]int
	lane := os.Getenv("HC_LANE") != ""
	if lane {
		mb, rc, e := git(R, "merge-base", "HEAD", "origin/main")
		if rc != 0 {
			t.Fatalf("갈래점 예산 조회 실패(merge-base 종료 %d): %s", rc, e)
		}
		entry, rc, e := git(R, "ls-tree", mb, "--", budgetFile)
		if rc != 0 {
			t.Fatalf("갈래점 예산 조회 실패(ls-tree 종료 %d): %s", rc, e)
		}
		if entry == "" {
			lane = false // 처음 만드는 판
		} else {
			s, rc, e := git(R, "show", mb+":"+budgetFile)
			if rc != 0 {
				t.Fatalf("갈래점 예산 읽기 실패(종료 %d): %s", rc, e)
			}
			base = readBudget(s)
		}
	}
	v, err := checkBudget(R, readBudget(string(bb)), base, lane)
	if err != nil {
		t.Fatalf("추적 파일 목록 또는 줄 수 읽기 실패: %v", err)
	}
	for _, l := range v {
		t.Error(l)
	}
	if len(v) == 0 {
		t.Logf("이 저장소: 한도 %d줄·예산 %s 지킴(레인: %v)", lineLimit, budgetFile, lane)
	}
}

// 판정 자체: 임시 저장소에서 늘어남·새 거대 파일·예산 밖·레인의 예산 올림이 걸리는지(옛 셸 시험의 자기 시험 그대로)
func TestLineBudgetSelf(t *testing.T) {
	T := clitest.TempDir(t)
	clitest.Isolate(t, T)
	R := T + "/r"
	lines := func(n int) string { return strings.Repeat("x\n", n) }
	clitest.Put(t, R+"/big", lines(700))
	clitest.Put(t, R+"/small", lines(10))
	clitest.Put(t, R+"/tests/huge.test.sh", lines(900))
	clitest.Put(t, R+"/doc.md", lines(900))
	clitest.MustGit(t, R, "init", "-q", ".")
	clitest.MustGit(t, R, "add", ".")
	b := map[string]int{"big": 700}
	has := func(v []string, s string) bool {
		for _, l := range v {
			if strings.Contains(l, s) {
				return true
			}
		}
		return false
	}
	chk := func(budget, base map[string]int, lane bool) []string {
		t.Helper()
		v, err := checkBudget(R, budget, base, lane)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	clitest.Put(t, T+"/notgit/f", lines(1))
	if _, err := checkBudget(T+"/notgit", map[string]int{}, nil, false); err == nil {
		t.Error("git ls-files 실패가 빈 저장소로 통과")
	}
	if v := chk(b, nil, false); len(v) != 0 {
		t.Errorf("예산 그대로·시험·문서 큰 파일은 통과여야: %v", v)
	}
	clitest.Put(t, R+"/big", lines(701))
	if !has(chk(b, nil, false), "늘었다 big 701줄") {
		t.Error("예산보다 늘면 실패여야")
	}
	clitest.Put(t, R+"/big", lines(650))
	if v := chk(b, nil, false); len(v) != 0 {
		t.Errorf("예산보다 줄면 통과여야: %v", v)
	}
	clitest.Put(t, R+"/new", lines(601))
	clitest.MustGit(t, R, "add", "new")
	if !has(chk(b, nil, false), "새 거대 파일 new 601줄") {
		t.Error("예산 없는 새 거대 파일은 실패여야")
	}
	clitest.MustGit(t, R, "rm", "-q", "--cached", "new")
	if !has(chk(map[string]int{"big": 700, "gone": 10}, nil, false), "예산에 없는 파일 gone") {
		t.Error("사라진 파일의 예산 줄은 실패여야")
	}
	o := map[string]int{"big": 700}
	if !has(chk(map[string]int{"big": 800}, o, true), "레인이 예산을 늘렸다 big 800") {
		t.Error("레인의 예산 올림은 실패여야")
	}
	if v := chk(map[string]int{"big": 800}, o, false); len(v) != 0 {
		t.Errorf("레인 밖(지휘·사람)은 예산을 올릴 수 있어야: %v", v)
	}
	if v := chk(map[string]int{"big": 650}, o, true); len(v) != 0 {
		t.Errorf("레인이 예산을 낮추는 것은 통과여야: %v", v)
	}
	if !has(chk(map[string]int{"big": 700, "x": 600}, o, true), "레인이 예산을 늘렸다 x 600 (갈래점 없음)") {
		t.Error("레인이 새 예산 줄을 더하면 실패여야")
	}
	if got := readBudget("# 주석 12 a\n700 big\n12\tsp ace\n"); len(got) != 2 || got["big"] != 700 || got["sp ace"] != 12 {
		t.Errorf("예산 줄 읽기: %v", got)
	}
}

// ---- 싼 검사 파일 빠짐(B28·R-14) ----
// 이 꾸러미(tests/repo)의 파일이 레인에서 사라지면 그 검사가 조용히 빠진다. 이 가지가 origin/main에서 갈라진 자리(cheapBase)에
// 있던 tests/repo 파일이 작업 트리에 없으면 이름을 찍고 실패. 원격 origin이 없는 저장소(시험용 가짜 저장소)만 「기준 없음」으로 건너뛴다.
// R-14(2026-11-22 재감사): 옛 셸 판은 `git rev-parse -q --verify origin/main`의 실패를 모두 「기준 없음」으로 읽어, 기준 조회가
// 깨진 저장소(128)에서 빠진 검사를 못 보고 0으로 끝났다. 이제 원격이 있는데 기준을 못 읽으면 검사 실패다.

const cheapDir = "tests/repo/"

// missingCheap: 기준에 있던 싼 검사 파일 가운데 작업 트리에 없는 것. 원격 origin이 없으면 (nil, nil). 조회가 깨지면 오류
func missingCheap(dir string) (string, []string, error) {
	if _, rc, e := git(dir, "rev-parse", "--git-dir"); rc != 0 {
		return "", nil, fmt.Errorf("git 저장소를 못 읽음(종료 %d): %s", rc, e)
	}
	switch _, rc, e := git(dir, "config", "--get", "remote.origin.url"); rc {
	case 0:
	case 1: // 설정 없음 — 원격 없는 저장소
		return "", nil, nil
	default:
		return "", nil, fmt.Errorf("원격 origin 설정을 못 읽음(종료 %d): %s", rc, e)
	}
	ref, at := cheapBase(dir)
	out, rc, e := git(dir, "ls-tree", "-r", "--name-only", ref, "--", cheapDir)
	if rc != 0 {
		return "", nil, fmt.Errorf("기준 브랜치(origin/main)에 있던 싼 검사 파일을 못 읽음(종료 %d): %s", rc, e)
	}
	var miss []string
	for _, p := range strings.Split(out, "\n") {
		if p == "" {
			continue
		}
		if _, err := os.Lstat(filepath.Join(dir, p)); errors.Is(err, fs.ErrNotExist) {
			miss = append(miss, p)
		} else if err != nil {
			return "", nil, fmt.Errorf("%s: %v", p, err)
		}
	}
	return at, miss, nil
}

// cheapBase: 견줄 기준 = 이 트리(HEAD)가 origin/main에서 갈라진 자리(2026-11-24 cheapkept-base — 갈라진 뒤 다른 레인이 더한 파일은
// 아직 받지 않은 것이지 지운 것이 아니다). 병합기·미리 검증은 기본 가지 끝 위에 rebase한 트리라 갈라진 자리 = 끝, 관문 보호는 같다.
// 못 구하면(얕은 클론 — 잘린 역사로 더 옛 자리를 고를 수 있다 · 공통 조상 없음 · git 실패) 기본 가지 끝과 견준다(at "" — 엄한 쪽)
func cheapBase(dir string) (ref, at string) {
	const tip = "refs/remotes/origin/main"
	if sh, rc, _ := git(dir, "rev-parse", "--is-shallow-repository"); rc != 0 || sh != "false" {
		return tip, ""
	}
	mb, rc, _ := git(dir, "merge-base", "HEAD", tip)
	if rc != 0 || len(mb) < 8 {
		return tip, ""
	}
	return mb, mb[:8]
}

func TestCheapKept(t *testing.T) {
	at, miss, err := missingCheap(clitest.Root(t))
	if err != nil {
		t.Fatalf("싼 검사 파일 빠짐을 못 봄: %v", err)
	}
	for _, p := range miss {
		if at == "" {
			t.Errorf("기준 브랜치(origin/main)에 있는 싼 검사 파일이 없음: %s — 지운 것이면 되살린다", p)
		} else {
			t.Errorf("이 가지가 갈라진 자리(%s)에 있던 싼 검사 파일이 없음: %s — 지운 것이면 되살린다", at, p)
		}
	}
}

func TestCheapKeptSelf(t *testing.T) {
	T := clitest.TempDir(t)
	clitest.Isolate(t, T)
	for _, kv := range []string{"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"} {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	R := T + "/r"
	clitest.Put(t, R+"/tests/repo/a_test.go", "package a\n")
	clitest.Put(t, R+"/tests/repo/b_test.go", "package a\n")
	clitest.MustGit(t, T, "init", "-q", "--bare", "-b", "main", T+"/o.git")
	clitest.MustGit(t, R, "init", "-q", "-b", "main")
	clitest.MustGit(t, R, "add", ".")
	clitest.MustGit(t, R, "commit", "-qm", "base")
	if _, m, err := missingCheap(R); err != nil || m != nil {
		t.Errorf("원격 없는 저장소는 기준 없음(건너뜀)이어야: %v %v", m, err)
	}
	clitest.MustGit(t, R, "remote", "add", "origin", T+"/o.git")
	clitest.MustGit(t, R, "push", "-q", "origin", "HEAD:main")
	clitest.MustGit(t, R, "fetch", "-q", "origin")
	if _, m, err := missingCheap(R); err != nil || len(m) != 0 {
		t.Errorf("준비 확인: 다 있으면 빠짐 없음이어야: %v %v", m, err)
	}
	clitest.MustGit(t, R, "rm", "-q", "--", "tests/repo/b_test.go")
	if _, m, err := missingCheap(R); err != nil || len(m) != 1 || m[0] != "tests/repo/b_test.go" {
		t.Errorf("[B28] 기준에 있던 싼 검사 파일이 없으면 그 이름: %v %v", m, err)
	}
	// R-14: 원격은 있는데 기준 ref가 깨짐(없는 객체를 가리킴 — rev-parse -q --verify는 1, ls-tree는 128).
	// 옛 꼴(조회 실패 = 기준 없음)이면 빠짐을 못 보고 통과했다
	gd, _, _ := git(R, "rev-parse", "--absolute-git-dir")
	clitest.Put(t, gd+"/refs/remotes/origin/main", strings.Repeat("0", 39)+"1\n")
	if _, rc, _ := git(R, "rev-parse", "-q", "--verify", "refs/remotes/origin/main^{commit}"); rc == 0 {
		t.Fatal("준비 확인: 기준 ref가 깨져 있어야")
	}
	if _, m, err := missingCheap(R); err == nil {
		t.Errorf("[R-14] 기준 조회가 깨지면(128) 검사 실패여야 — 「없음」으로 읽어 통과함: %v", m)
	}
	clitest.Put(t, T+"/plain/f", "x\n")
	if _, _, err := missingCheap(T + "/plain"); err == nil {
		t.Error("[R-14] 저장소 밖(조회 128)은 「원격 없음」이 아니라 검사 실패여야")
	}
}

// ---- gofmt·셸 문법 ----

func TestGofmt(t *testing.T) {
	R := clitest.Root(t)
	files, err := tracked(R, "*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range files {
		b, err := os.ReadFile(filepath.Join(R, p))
		if errors.Is(err, fs.ErrNotExist) { // 작업 트리에서 지운 파일
			continue
		} else if err != nil {
			t.Fatal(err)
		}
		f, err := format.Source(b)
		if err != nil {
			t.Errorf("gofmt: %s: %v", p, err)
		} else if !bytes.Equal(f, b) {
			t.Errorf("gofmt: %s 가 gofmt 꼴이 아니다(gofmt -w %s)", p, p)
		}
	}
}

// 셸 시험 파일(*.test.sh)은 두지 않는다: 검증은 go test만 돌려(옛 검증은 tests/*.test.sh를 다 돌렸다) 두면 아무도 안 돌리는 시험이 된다.
// 시험은 Go(_test.go)로 쓴다
func TestNoShellTests(t *testing.T) {
	files, err := tracked(clitest.Root(t), "*.test.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range files {
		t.Errorf("검증이 돌리지 않는 셸 시험: %s — Go 시험으로 옮기고 지운다", p)
	}
}

// 남은 셸(저장소 간 계약 .claude/*.sh, 원격에서 도는 내장 조각 cmd/*/*.sh)은 bash -n
func TestShellSyntax(t *testing.T) {
	R := clitest.Root(t)
	files, err := tracked(R, "*.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range files {
		if !clitest.Exists(filepath.Join(R, p)) {
			continue
		}
		c := exec.Command("bash", "-n", p)
		c.Dir = R
		if out, err := c.CombinedOutput(); err != nil {
			t.Errorf("문법 실패 %s: %v\n%s", p, err, out)
		}
	}
}
