package repo_test

// ---- 함수 길이 예산 ----
// 줄 수 예산(TestLineBudget)은 파일만 보아, 크기로 파일만 나누고 몇백 줄짜리 함수는 그대로였다(2026-11-24 long-func-sync — 200줄 넘는
// 함수 15개가 고침 커밋이 몰린 자리와 겹쳤다). 시험 아닌 추적 Go 파일(*_test.go·tests/·*/tests/·*/testdata/ 빼고)의 함수·메서드는
// funcLimit 줄(func 줄부터 닫는 } 까지)을 넘지 않는다. 이미 넘은 함수는 funcBudgetFile 「<줄 수> <경로> <함수>」(메서드는 「<받는 형>.<이름>」)에
// 적힌 줄 수까지만 — 늘면 실패(줄면 통과). 새 긴 함수는 예산 없이 실패: 늘릴 일은 단계 함수로 나눈다. 사라진 함수의 예산 줄은 실패(지운다).
// 레인 안(HC_LANE)에서는 예산을 늘리지 못한다(줄 수 예산과 같은 규칙 — 갈래점 판보다 늘거나 새 줄이면 실패, 갈래점에 파일이 없으면 안 본다).

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"herdr-tools/internal/clitest"
)

const (
	funcLimit      = 200
	funcBudgetFile = "tests/func-budget.tsv"
)

var funcBudgetLine = regexp.MustCompile(`^([0-9]+)[ \t]+(\S+)[ \t]+(\S+)[ \t]*$`)

// readFuncBudget: 「<줄 수> <경로> <함수>」 줄만 → 「<경로> <함수>」 열쇠(주석·빈 줄은 뺀다)
func readFuncBudget(b string) map[string]int {
	m := map[string]int{}
	for _, l := range strings.Split(b, "\n") {
		if s := funcBudgetLine.FindStringSubmatch(l); s != nil {
			if n, err := strconv.Atoi(s[1]); err == nil {
				m[s[2]+" "+s[3]] = n
			}
		}
	}
	return m
}

// funcLens: 추적 Go 파일의 함수마다 줄 수(열쇠 「<경로> <함수>」). 목록·읽기·구문 분석 실패는 오류
func funcLens(dir string) (map[string]int, error) {
	files, err := tracked(dir, "*.go")
	if err != nil {
		return nil, err
	}
	m := map[string]int{}
	fset := token.NewFileSet()
	for _, p := range files {
		if strings.HasSuffix(p, "_test.go") || strings.HasPrefix(p, "tests/") || strings.Contains(p, "/tests/") || strings.Contains(p, "/testdata/") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, p), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("구문 분석 실패: %v", err)
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			name := fd.Name.Name
			if fd.Recv != nil && len(fd.Recv.List) == 1 {
				name = recvName(fd.Recv.List[0].Type) + "." + name
			}
			k := p + " " + name
			n := fset.Position(fd.End()).Line - fset.Position(fd.Pos()).Line + 1
			if n > m[k] { // 한 파일에 여럿일 수 있는 이름(init)은 가장 긴 것
				m[k] = n
			}
		}
	}
	return m, nil
}

// recvName: *T·T·T[P] → T
func recvName(e ast.Expr) string {
	for {
		switch x := e.(type) {
		case *ast.StarExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.IndexListExpr:
			e = x.X
		case *ast.Ident:
			return x.Name
		default:
			return "?"
		}
	}
}

// checkFuncBudget: 위반마다 한 줄. lane이면 갈래점 예산 base와도 견준다
func checkFuncBudget(lens, budget, base map[string]int, lane bool) []string {
	var v []string
	keys := make([]string, 0, len(lens))
	for k := range lens {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		n := lens[k]
		if bn, ok := budget[k]; ok {
			if n > bn {
				v = append(v, fmt.Sprintf("함수가 늘었다 %s %d줄 (예산 %d) — 늘릴 것은 단계 함수로 나눈다", k, n, bn))
			}
		} else if n > funcLimit {
			v = append(v, fmt.Sprintf("새 긴 함수 %s %d줄 (한도 %d) — 단계 함수로 나눈다", k, n, funcLimit))
		}
	}
	for _, k := range sortedKeys(budget) {
		if _, ok := lens[k]; !ok {
			v = append(v, fmt.Sprintf("예산에 없는 함수 %s — 예산 줄을 지운다", k))
		}
	}
	if lane {
		for _, k := range sortedKeys(budget) {
			if o, ok := base[k]; !ok {
				v = append(v, fmt.Sprintf("레인이 함수 예산을 늘렸다 %s %d (갈래점 없음)", k, budget[k]))
			} else if budget[k] > o {
				v = append(v, fmt.Sprintf("레인이 함수 예산을 늘렸다 %s %d (갈래점 %d)", k, budget[k], o))
			}
		}
	}
	return v
}

// laneBaseFile: 레인 안이면 갈래점(merge-base HEAD origin/main) 판의 file 내용. 갈래점에 파일이 없으면 ok=false(처음 만드는 판).
// git 실패는 시험 실패(「없음」으로 읽지 않는다)
func laneBaseFile(t *testing.T, R, file string) (string, bool) {
	t.Helper()
	mb, rc, e := git(R, "merge-base", "HEAD", "origin/main")
	if rc != 0 {
		t.Fatalf("갈래점 예산 조회 실패(merge-base 종료 %d): %s", rc, e)
	}
	entry, rc, e := git(R, "ls-tree", mb, "--", file)
	if rc != 0 {
		t.Fatalf("갈래점 예산 조회 실패(ls-tree 종료 %d): %s", rc, e)
	}
	if entry == "" {
		return "", false
	}
	s, rc, e := git(R, "show", mb+":"+file)
	if rc != 0 {
		t.Fatalf("갈래점 예산 읽기 실패(종료 %d): %s", rc, e)
	}
	return s, true
}

func TestFuncBudget(t *testing.T) {
	R := clitest.Root(t)
	bb, err := os.ReadFile(filepath.Join(R, funcBudgetFile))
	if err != nil {
		t.Fatalf("함수 예산 파일이 없다: %s (%v)", funcBudgetFile, err)
	}
	var base map[string]int
	lane := os.Getenv("HC_LANE") != ""
	if lane {
		s, ok := laneBaseFile(t, R, funcBudgetFile)
		lane = ok
		base = readFuncBudget(s)
	}
	lens, err := funcLens(R)
	if err != nil {
		t.Fatalf("함수 길이를 못 잼: %v", err)
	}
	v := checkFuncBudget(lens, readFuncBudget(string(bb)), base, lane)
	for _, l := range v {
		t.Error(l)
	}
	if len(v) == 0 {
		t.Logf("이 저장소: 함수 한도 %d줄·예산 %s 지킴(레인: %v)", funcLimit, funcBudgetFile, lane)
	}
}
