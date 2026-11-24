package repo_test

// 시험이 있는 꾸러미는 모두 시험 격리(internal/clitest/sandbox)를 거친다(2026-11-24 knob-test-leak — 병합기 시험이 운영 손잡이를 읽어
// 검증이 거짓으로 실패했다). 싼 단계에서 구문만 본다(빌드·실행 없음): 폴더마다 *_test.go 가운데 하나가 그 꾸러미를 가져와야 한다.

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"herdr-tools/internal/clitest"
)

const sandboxPkg = "herdr-tools/internal/clitest/sandbox"

// sandboxExempt: 격리를 거치지 않는 시험 꾸러미 → 까닭
var sandboxExempt = map[string]string{
	"tests/repo":               "저장소 검사 — 이 기계의 개인 값 패턴(설정 폴더)과 레인 표식(HC_LANE)을 일부러 읽는다",
	"internal/clitest/sandbox": "격리 꾸러미 자신(시험이 같은 꾸러미라 init이 돈다)",
}

// unsandboxed: root 아래 시험이 있는데 격리를 가져오지 않는 폴더(점 폴더·testdata는 건너뜀)
func unsandboxed(root string, exempt map[string]string) ([]string, error) {
	need := map[string]bool{}
	has := map[string]bool{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, filepath.Dir(p))
		rel = filepath.ToSlash(rel)
		need[rel] = true
		f, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, im := range f.Imports {
			if v, _ := strconv.Unquote(im.Path.Value); v == sandboxPkg {
				has[rel] = true
			}
		}
		return nil
	})
	var bad []string
	for d := range need {
		if !has[d] && exempt[d] == "" {
			bad = append(bad, d)
		}
	}
	sort.Strings(bad)
	return bad, err
}

func TestTestsSandboxed(t *testing.T) {
	bad, err := unsandboxed(clitest.Root(t), sandboxExempt)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range bad {
		t.Errorf("%s: 시험이 격리를 거치지 않는다 — 실제 손잡이·대기열·일지·운영 환경 값을 읽을 수 있다. 그 폴더에 sandbox_test.go(TestSandboxed — 다른 꾸러미의 것을 본떠)를 두라 (docs/repo/tests.md 「시험의 집」)", d)
	}
}

// 일부러 격리를 안 거치는 시험을 둔 사본에서 잡힌다
func TestTestsSandboxedCatches(t *testing.T) {
	r := clitest.TempDir(t)
	for p, s := range map[string]string{
		"a/a_test.go": "package a\n\nimport _ \"" + sandboxPkg + "\"\n",
		"a/b_test.go": "package a\n",
		"b/b_test.go": "package b\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) {}\n",
		"c/c_test.go": "package c\n",
		"d/d.go":      "package d\n",
	} {
		clitest.Put(t, filepath.Join(r, p), s)
	}
	bad, err := unsandboxed(r, map[string]string{"c": "까닭"})
	if err != nil || strings.Join(bad, " ") != "b" {
		t.Fatalf("격리 안 거친 b만 잡혀야: %v %v", bad, err)
	}
	if err := os.WriteFile(filepath.Join(r, "b/z_test.go"), []byte("package b\n\nimport \""+sandboxPkg+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if bad, err := unsandboxed(r, map[string]string{"c": "까닭"}); err != nil || len(bad) != 0 {
		t.Fatalf("가져온 뒤에는 없어야: %v %v", bad, err)
	}
}
