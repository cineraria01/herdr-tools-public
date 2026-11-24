package repo_test

// tri-state-type(2026-11-24): 「모름」 가지를 빠뜨리지 못하게 하는 싼 검사 두 가지 — 저장소의 시험 아닌 Go 파일 전부를 구문으로만 본다.
// 저장소 전체를 읽으니 늘 도는 싼 단계(tests/repo)에 둔다 — 꾸러미 조각에 두면 검증 범위 판정이 그 조각을 건너뛸 수 있다(감사 1차).
//  ① 세 값 형(tri.State·gitx.State)이나 꾸러미 안 iota 판정 형(0값이 모름인 형)을 고르는 switch는 그 형의 값을 다 적거나 default를 둔다.
//  ② int를 돌려주는 함수의 문서 주석에 「2 모름」·「2(모름)」·「모름(2)」 꼴(안쪽 판정을 0·1·2 정수로)이 다시 생기지 않는다 —
//     바깥 종료 코드(「종료 2(모름)」)는 명령 계약이라 그대로 둔다.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"herdr-tools/internal/clitest"
)

// pkgFiles: 저장소의 꾸러미 폴더마다 시험 아닌 Go 파일(구문 나무)
func pkgFiles(t *testing.T) map[string][]*ast.File {
	t.Helper()
	root := clitest.Root(t)
	fs := token.NewFileSet()
	out := map[string][]*ast.File{}
	for _, top := range []string{"cmd", "internal", "tests"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == "testdata" {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(fs, p, nil, parser.ParseComments)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			fnames[f] = rel
			out[filepath.Dir(rel)] = append(out[filepath.Dir(rel)], f)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(out) == 0 {
		t.Fatal("Go 파일을 하나도 못 찾았다")
	}
	return out
}

// enumGroups: 꾸러미 안 판정 형의 상수 묶음 — 상수 이름 → 묶음(그 묶음의 모든 이름). 판정 형 = 「T = iota」로 시작하는 이름 붙은 형이고
// 0값(첫 상수)이 모름: 이름에 Unknown이 들었거나 주석에 「모름」이 있다(그 밖의 iota 묶음 — 종류·단계 이름 — 은 보지 않는다)
func enumGroups(files []*ast.File) map[string][]string {
	g := map[string][]string{}
	for _, f := range files {
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			var cur []string
			flush := func() {
				for _, n := range cur {
					g[n] = cur
				}
				cur = nil
			}
			for _, s := range gd.Specs {
				vs := s.(*ast.ValueSpec)
				if len(vs.Values) > 0 {
					flush()
					id, named := vs.Type.(*ast.Ident)
					if !named || id.Name == "int" || !isIota(vs.Values[0]) || !zeroUnknown(vs) {
						continue
					}
				} else if cur == nil {
					continue
				}
				for _, n := range vs.Names {
					if n.Name != "_" {
						cur = append(cur, n.Name)
					}
				}
			}
			flush()
		}
	}
	return g
}

// zeroUnknown: 묶음의 첫 상수(0값)가 모름인가 — 이름에 Unknown, 또는 그 줄·앞 주석에 「모름」
func zeroUnknown(vs *ast.ValueSpec) bool {
	if strings.Contains(vs.Names[0].Name, "Unknown") {
		return true
	}
	for _, c := range []*ast.CommentGroup{vs.Comment, vs.Doc} {
		if c != nil && strings.Contains(c.Text(), "모름") {
			return true
		}
	}
	return false
}

func isIota(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "iota"
}

var triNames = []string{"Unknown", "Yes", "No"}

// caseName: case 식이 세 값 형(tri.X·gitx.X)이거나 이 꾸러미 판정 형의 상수면 그 이름과 묶음
func caseName(e ast.Expr, local map[string][]string, inTri bool) (string, []string) {
	switch x := e.(type) {
	case *ast.SelectorExpr:
		if p, ok := x.X.(*ast.Ident); ok && (p.Name == "tri" || p.Name == "gitx") {
			for _, n := range triNames {
				if x.Sel.Name == n {
					return "tri." + n, []string{"tri.Unknown", "tri.Yes", "tri.No"}
				}
			}
		}
	case *ast.Ident:
		if grp, ok := local[x.Name]; ok {
			return x.Name, grp
		}
		if inTri {
			for _, n := range triNames {
				if x.Name == n {
					return "tri." + n, []string{"tri.Unknown", "tri.Yes", "tri.No"}
				}
			}
		}
	}
	return "", nil
}

// switchMisses: 한 꾸러미의 판정 형 switch 가운데 값을 빠뜨린 것(본 switch 수와 함께)
func switchMisses(files []*ast.File, inTri bool) (bad []string, checked int) {
	local := enumGroups(files)
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok || sw.Tag == nil {
				return true
			}
			seen := map[string]bool{}
			var grp []string
			hasDefault := false
			for _, s := range sw.Body.List {
				cc := s.(*ast.CaseClause)
				if cc.List == nil {
					hasDefault = true
				}
				for _, e := range cc.List {
					if nm, g := caseName(e, local, inTri); nm != "" {
						seen[nm], grp = true, g
					}
				}
			}
			if grp == nil {
				return true
			}
			checked++
			if hasDefault {
				return true
			}
			var miss []string
			for _, v := range grp {
				if !seen[v] {
					miss = append(miss, v)
				}
			}
			if len(miss) > 0 {
				bad = append(bad, fnames[f]+": switch에 "+strings.Join(miss, "·")+" 가지가 없다(다 적거나 default)")
			}
			return true
		})
	}
	return bad, checked
}

// ① switch가 판정 형의 값을 다 다룬다
func TestTriSwitchExhaustive(t *testing.T) {
	var bad []string
	checked := 0
	for dir, files := range pkgFiles(t) {
		b, c := switchMisses(files, dir == filepath.Join("internal", "tri") || dir == filepath.Join("internal", "gitx"))
		bad, checked = append(bad, b...), checked+c
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Error(b)
	}
	if checked == 0 {
		t.Fatal("판정 형 switch를 하나도 못 찾았다 — 검사가 아무것도 안 본다")
	}
}

// 검사 자신: 모름 가지를 빠뜨린 switch는 잡고, 다 적거나 default면 넘긴다. 모름이 0값이 아닌 iota 묶음은 보지 않는다
func TestTriSwitchGuardSelf(t *testing.T) {
	src := `package p
import "herdr-tools/internal/tri"
type ps int
const (
	pUnknown ps = iota // 모름
	pLive
	pGone
)
type kind int
const (
	kA kind = iota
	kB
)
func f(s tri.State, p ps, k kind) {
	switch s { case tri.Yes: case tri.No: }
	switch s { case tri.Yes: default: }
	switch s { case tri.Yes, tri.No, tri.Unknown: }
	switch p { case pLive: case pGone: }
	switch p { case pLive, pGone, pUnknown: }
	switch k { case kA: }
}`
	f, err := parser.ParseFile(token.NewFileSet(), "self.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	fnames[f] = "self.go"
	bad, checked := switchMisses([]*ast.File{f}, false)
	want := []string{"self.go: switch에 tri.Unknown 가지가 없다(다 적거나 default)", "self.go: switch에 pUnknown 가지가 없다(다 적거나 default)"}
	if checked != 5 || strings.Join(bad, "\n") != strings.Join(want, "\n") {
		t.Fatalf("검사 %d개 %q, 바람 5개 %q", checked, bad, want)
	}
}

// fnames: 구문 나무 → 저장소 기준 경로(pkgFiles가 채운다)
var fnames = map[*ast.File]string{}

// 안쪽 판정을 0·1·2 정수로 적은 문서 주석(바깥 종료 코드 「종료 2(모름)」은 부르는 쪽에서 미리 뺀다)
var reIntUnknown = regexp.MustCompile(`\b[1-3] ?모름|\b[1-3] ?\(모름\)|모름\([1-3]\)|\b[1-3] 그 밖\(모름\)`)

// ② int를 돌려주는 함수가 안쪽 판정을 「…2 모름」 정수로 적지 않는다
func TestNoIntTriState(t *testing.T) {
	var bad []string
	for dir, files := range pkgFiles(t) {
		for _, f := range files {
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || fd.Doc == nil || fd.Type.Results == nil {
					continue
				}
				hasInt := false
				for _, r := range fd.Type.Results.List {
					if id, ok := r.Type.(*ast.Ident); ok && id.Name == "int" {
						hasInt = true
					}
				}
				doc := strings.ReplaceAll(fd.Doc.Text(), "종료 2(모름)", "")
				if hasInt && reIntUnknown.MatchString(doc) {
					bad = append(bad, dir+": "+fd.Name.Name+" — 0·1·2 정수 판정(tri.State나 0값이 모름인 이름 붙은 형으로)")
				}
			}
		}
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Error(b)
	}
}
