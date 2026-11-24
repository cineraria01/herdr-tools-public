package events

// events-named-cols(2026-11-24) ⑤: 사건 칸을 번호로 읽는 꼴이 도구에 다시 들어오지 못하게 — 사건 줄은 Cols·Parse의 이름 칸으로 읽는다.
// 싼 검사 둘(시험 파일 빼고 cmd/의 모든 .go):
//   - 1부터 세는 칸 도우미 fld(…) 부름(0부터 세는 f[n]과 한 줄에 섞였다)
//   - 상태 칸 자리의 숫자 칸(x[4])을 사건 상태 글(착지·제출 …)과 견주는 것(==·!=·switch case) — 상태 칸을 번호로 읽는 꼴

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var evStates = map[string]bool{"중간": true, "완료": true, "막힘": true, "착지": true, "제출": true, "주차": true, "판정": true, "정리됨": true, "멈춤": true}

// numIndex: x[4] — 사건 상태 칸의 자리(0부터 4, awk $5). 다른 표(병합기 일지 f[9] 「착지」 등)는 잡지 않는다
func numIndex(e ast.Expr) bool {
	ix, ok := e.(*ast.IndexExpr)
	if !ok {
		return false
	}
	b, ok := ix.Index.(*ast.BasicLit)
	return ok && b.Kind == token.INT && b.Value == "4"
}

func stateLit(e ast.Expr) bool {
	b, ok := e.(*ast.BasicLit)
	if !ok || b.Kind != token.STRING {
		return false
	}
	s, err := strconv.Unquote(b.Value)
	return err == nil && evStates[s]
}

func TestNoNumberedEventColumns(t *testing.T) {
	var files []string
	err := filepath.WalkDir("../../cmd", func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
			files = append(files, p)
		}
		return err
	})
	if err != nil || len(files) == 0 {
		t.Fatalf("cmd 파일을 못 읽음: %v", err)
	}
	fs := token.NewFileSet()
	for _, p := range files {
		f, err := parser.ParseFile(fs, p, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		bad := func(n ast.Node, why string) {
			t.Errorf("%s: %s — 사건 줄은 events.Cols(줄)의 이름 칸으로 읽는다", fs.Position(n.Pos()), why)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				if id, ok := n.Fun.(*ast.Ident); ok && id.Name == "fld" {
					bad(n, "1부터 세는 칸 도우미 fld")
				}
			case *ast.BinaryExpr:
				if (n.Op == token.EQL || n.Op == token.NEQ) && (numIndex(n.X) && stateLit(n.Y) || numIndex(n.Y) && stateLit(n.X)) {
					bad(n, "숫자 칸을 사건 상태와 견줌")
				}
			case *ast.SwitchStmt:
				if n.Tag == nil || !numIndex(n.Tag) {
					return true
				}
				for _, s := range n.Body.List {
					for _, e := range s.(*ast.CaseClause).List {
						if stateLit(e) {
							bad(n, "숫자 칸의 switch에 사건 상태")
							return true
						}
					}
				}
			}
			return true
		})
	}
}
