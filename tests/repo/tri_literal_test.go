package repo_test

// tri-state-type 감사 1차 중간 1: 판정 형은 int 바탕이라 옛 꼴 숫자(`pget(a) == 0`, `st != 1`)가 그대로 컴파일되는데 숫자의 뜻이
// 바뀌었다(옛 0 「살아 있음」 → 새 0 「모름」, 옛 1 「없음」 → 새 1 tri.Yes). 옛 기본 가지 위에서 쓴 코드가 들어오면 소리 없이 뜻이 뒤집힌다.
//  ③ 판정 형(tri.State·gitx.State, 꾸러미 안 0값이 모름·보류인 iota 형)의 자리에 정수 리터럴을 쓰지 않는다 — 비교·case·대입·return·
//     인자 모두. go/types가 형 없는 상수에 붙여 둔 최종 형으로 본다(표준 라이브러리만, 익스포트 자료는 go list가 짓는다).

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"herdr-tools/internal/clitest"
)

const triPath = "herdr-tools/internal/tri"

// judgeTypes: 꾸러미 안 판정 형 이름(enumGroups와 같은 기준 — iota로 시작하고 0값이 모름·보류)
func judgeTypes(files []*ast.File) map[string]bool {
	out := map[string]bool{}
	for _, f := range files {
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, s := range gd.Specs {
				vs := s.(*ast.ValueSpec)
				if id, named := vs.Type.(*ast.Ident); named && id.Name != "int" && len(vs.Values) > 0 && isIota(vs.Values[0]) && zeroUnknown(vs) {
					out[id.Name] = true
				}
			}
		}
	}
	return out
}

// literalMisuse: 타입 검사한 한 꾸러미에서 판정 형 자리에 쓴 정수 리터럴(파일:줄 형)
func literalMisuse(fset *token.FileSet, files []*ast.File, pkgPath string, local map[string]bool, info *types.Info) []string {
	var bad []string
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.INT {
				return true
			}
			tv, ok := info.Types[lit]
			if !ok {
				return true
			}
			nt, ok := types.Unalias(tv.Type).(*types.Named)
			if !ok || nt.Obj().Pkg() == nil {
				return true
			}
			p, nm := nt.Obj().Pkg().Path(), nt.Obj().Name()
			if (p == triPath && nm == "State") || (p == pkgPath && local[nm]) {
				pos := fset.Position(lit.Pos())
				bad = append(bad, fnames[f]+":"+strconv.Itoa(pos.Line)+": "+nm+" 자리에 정수 "+lit.Value+" — 이름 붙은 값으로")
			}
			return true
		})
	}
	return bad
}

// checkPkg: 한 꾸러미를 타입 검사해 리터럴 오용을 찾는다. 타입 검사가 실패하면 그 오류(검사가 아무것도 안 본 채 통과하지 않게)
func checkPkg(imp types.Importer, fset *token.FileSet, files []*ast.File, pkgPath string) ([]string, error) {
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	conf := types.Config{Importer: imp}
	if _, err := conf.Check(pkgPath, fset, files, info); err != nil {
		return nil, err
	}
	return literalMisuse(fset, files, pkgPath, judgeTypes(files), info), nil
}

// exportImporter: 모듈 안 꾸러미·표준 라이브러리의 익스포트 자료(go list -export가 지어 둔 것)를 읽는 가져오기
func exportImporter(t *testing.T, fset *token.FileSet, root string) types.Importer {
	t.Helper()
	c := exec.Command("go", "list", "-export", "-deps", "-f", "{{.ImportPath}}\t{{.Export}}", "./...")
	c.Dir = root
	out, err := c.Output()
	if err != nil {
		t.Fatalf("go list -export: %v", err)
	}
	exp := map[string]string{}
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if p, f, ok := strings.Cut(l, "\t"); ok && f != "" {
			exp[p] = f
		}
	}
	return importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
		f, ok := exp[path]
		if !ok {
			return nil, fmt.Errorf("익스포트 자료 없음: %s", path)
		}
		return os.Open(f)
	})
}

// ③ 저장소에서 판정 형을 쓰는 꾸러미(tri·gitx를 가져오거나 판정 형을 둔 곳)를 모두 타입 검사한다
func TestTriNoIntLiteral(t *testing.T) {
	if testing.Short() {
		t.Skip("꾸러미마다 타입 검사(go list -export)")
	}
	root := clitest.Root(t)
	fset := token.NewFileSet()
	imp := exportImporter(t, fset, root)
	dirs := map[string][]*ast.File{}
	for _, top := range []string{"cmd", "internal"} {
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
			if ok, err := build.Default.MatchFile(filepath.Dir(p), d.Name()); err != nil || !ok {
				return err // 이 기계 빌드에 안 드는 파일(_windows.go·빌드 태그)
			}
			f, err := parser.ParseFile(fset, p, nil, parser.ParseComments)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			fnames[f] = rel
			dirs[filepath.Dir(rel)] = append(dirs[filepath.Dir(rel)], f)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	var bad []string
	checked := 0
	for dir, files := range dirs {
		uses := len(judgeTypes(files)) > 0 || dir == filepath.Join("internal", "tri")
		for _, f := range files {
			for _, im := range f.Imports {
				if v := strings.Trim(im.Path.Value, `"`); v == triPath || v == "herdr-tools/internal/gitx" {
					uses = true
				}
			}
		}
		if !uses {
			continue
		}
		b, err := checkPkg(imp, fset, files, "herdr-tools/"+filepath.ToSlash(dir))
		if err != nil {
			t.Fatalf("%s 타입 검사 실패 — 검사가 이 꾸러미를 못 본다: %v", dir, err)
		}
		bad = append(bad, b...)
		checked++
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Error(b)
	}
	if checked < 5 {
		t.Fatalf("판정 형을 쓰는 꾸러미를 %d개만 봤다 — 검사가 거의 아무것도 안 본다", checked)
	}
}

// 검사 자신: 옛 꼴 숫자 비교·case·대입·return은 잡고, 이름 붙은 값·판정 형이 아닌 정수·바깥 종료 코드 int는 넘긴다
func TestTriNoIntLiteralSelf(t *testing.T) {
	if testing.Short() {
		t.Skip("타입 검사(go list -export)")
	}
	src := `package p
import "herdr-tools/internal/tri"
type ps int
const (
	pUnknown ps = iota // 모름
	pLive
)
func get() ps { return 0 }
func own() (tri.State, string) { return 1, "" }
func f() int {
	if get() == 0 { return 1 }
	st, _ := own()
	if st != 1 { return 2 }
	var p ps = 1
	switch p { case 2: }
	if get() == pLive && st == tri.Yes { return 0 }
	n := 3
	return n
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "self.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	fnames[f] = "self.go"
	bad, err := checkPkg(exportImporter(t, fset, clitest.Root(t)), fset, []*ast.File{f}, "herdr-tools/p")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"self.go:8: ps 자리에 정수 0 — 이름 붙은 값으로",
		"self.go:9: State 자리에 정수 1 — 이름 붙은 값으로",
		"self.go:11: ps 자리에 정수 0 — 이름 붙은 값으로",
		"self.go:13: State 자리에 정수 1 — 이름 붙은 값으로",
		"self.go:14: ps 자리에 정수 1 — 이름 붙은 값으로",
		"self.go:15: ps 자리에 정수 2 — 이름 붙은 값으로",
	}
	if strings.Join(bad, "\n") != strings.Join(want, "\n") {
		t.Fatalf("잡은 것\n%s\n바람\n%s", strings.Join(bad, "\n"), strings.Join(want, "\n"))
	}
}
