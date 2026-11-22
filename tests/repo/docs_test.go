package repo_test

// 문서 목차(2026-11-22 docs-split — README.md가 충돌 1위(병합기 기록 11/20~22, 17번): 레인마다 자기 기능 설명을 README 한 파일의
// 같은 절에 고쳐 넣었다). 기능 설명은 docs/<도구>/<기능>.md 한 파일에 한 주제, README 「## 목차」에는 그 파일마다 링크 한 줄.
// 목차 줄은 경로 이름순이라 새 줄이 끝에 몰리지 않고(끝에 덧붙이면 레인끼리 같은 자리를 고친다), 파일과 목차가 어긋나면 실패한다.
// docs/audit/(감사 기록)는 목차 밖이다.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"herdr-tools/internal/clitest"
)

var tocLine = regexp.MustCompile(`^- \[[^\]]+\]\((docs/[^/)]+/[^/)]+\.md)\)$`)

// docsIndexProblems: README 목차와 docs/<도구>/<기능>.md를 견준 문제 목록(없으면 nil)
func docsIndexProblems(root string) []string {
	var bad []string
	b, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		return []string{"README.md를 못 읽음: " + err.Error()}
	}
	var listed []string
	in := false
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "## ") {
			in = l == "## 목차"
			continue
		}
		if !in || !strings.HasPrefix(l, "- ") {
			continue
		}
		m := tocLine.FindStringSubmatch(l)
		if m == nil {
			bad = append(bad, "목차 줄 꼴이 아님(「- [제목](docs/<도구>/<기능>.md)」): "+l)
			continue
		}
		listed = append(listed, m[1])
	}
	if len(listed) == 0 {
		bad = append(bad, "README에 「## 목차」 줄이 없음")
	}
	seen := map[string]bool{}
	for i, p := range listed {
		if i > 0 && p <= listed[i-1] {
			bad = append(bad, "목차가 이름순이 아님(또는 겹침): "+listed[i-1]+" 다음에 "+p+" — 끝에 덧붙이지 말고 이름순 자리에")
		}
		seen[p] = true
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			bad = append(bad, "목차의 파일이 없음: "+p)
		}
	}
	err = filepath.WalkDir(filepath.Join(root, "docs"), func(f string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(f, ".md") {
			return err
		}
		p, err := filepath.Rel(root, f)
		if err != nil {
			return err
		}
		p = filepath.ToSlash(p)
		switch n := strings.Count(p, "/"); {
		case strings.HasPrefix(p, "docs/audit/"):
		case n == 1 && docsTopKeep[p]:
		case n != 2:
			bad = append(bad, "docs/<도구>/<기능>.md 꼴이 아닌 문서: "+p+" — 기능 설명은 docs/<도구>/<기능>.md 한 단계에")
		case !seen[p]:
			bad = append(bad, "목차에 없는 문서: "+p+" — README 「## 목차」 이름순 자리에 한 줄")
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		bad = append(bad, "docs/를 못 걸음: "+err.Error())
	}
	return bad
}

// docsTopKeep: 목차 밖 맨 위 docs/*.md(기능 설명이 아닌 기록) — 새로 더하지 않는다
var docsTopKeep = map[string]bool{"docs/land-stats-baseline.md": true}

// readmeSectionRefs: README가 목차뿐이라 「README 「절」」 참조는 가리킬 곳이 없다 — 남은 파일:줄(시험·감사 기록 빼고)
func readmeSectionRefs(root string, paths []string) []string {
	var bad []string
	for _, p := range paths {
		if strings.HasPrefix(p, "tests/") || strings.Contains(p, "/tests/") || strings.HasSuffix(p, "_test.go") ||
			strings.Contains(p, "/testdata/") || strings.HasPrefix(p, "docs/audit/") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			bad = append(bad, "못 읽음: "+p+": "+err.Error())
			continue
		}
		for i, l := range strings.Split(string(b), "\n") {
			if strings.Contains(l, "README 「") {
				bad = append(bad, fmt.Sprintf("%s:%d: 없어진 README 절 참조 — docs/<도구>/<기능>.md 경로로", p, i+1))
			}
		}
	}
	return bad
}

func TestReadmeSectionRefs(t *testing.T) {
	root := clitest.Root(t)
	ps, err := tracked(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range readmeSectionRefs(root, ps) {
		t.Error(p)
	}
}

func TestReadmeSectionRefsSelf(t *testing.T) {
	R := t.TempDir()
	clitest.Put(t, R+"/a.plist", "<!-- 쓰는 법은 docs/x/y.md -->\n<!-- 쓰는 법은 README 「launchd 작업」 -->\n")
	clitest.Put(t, R+"/tests/x_test.go", "README 「x」\n")
	clitest.Put(t, R+"/docs/audit/r.md", "README 「x」\n")
	got := strings.Join(readmeSectionRefs(R, []string{"a.plist", "tests/x_test.go", "docs/audit/r.md"}), "\n")
	if !strings.Contains(got, "a.plist:2:") || strings.Contains(got, "a.plist:1:") || strings.Contains(got, "tests/") || strings.Contains(got, "audit") {
		t.Errorf("문제 %q", got)
	}
}

func TestDocsIndex(t *testing.T) {
	for _, p := range docsIndexProblems(clitest.Root(t)) {
		t.Error(p)
	}
}

func TestDocsIndexSelf(t *testing.T) {
	R := t.TempDir()
	clitest.Put(t, R+"/docs/a/x.md", "# x\n")
	clitest.Put(t, R+"/docs/b/y.md", "# y\n")
	clitest.Put(t, R+"/docs/audit/z.md", "# z\n")
	ok := "# t\n\n## 목차\n\n- [x](docs/a/x.md)\n- [y](docs/b/y.md)\n\n## 다른 절\n\n- 아무 줄\n"
	cases := []struct{ name, readme, want string }{
		{"맞음", ok, ""},
		{"끝에 덧붙임", "## 목차\n- [y](docs/b/y.md)\n- [x](docs/a/x.md)\n", "이름순이 아님"},
		{"빠짐", "## 목차\n- [x](docs/a/x.md)\n", "목차에 없는 문서: docs/b/y.md"},
		{"없는 파일", "## 목차\n- [x](docs/a/x.md)\n- [y](docs/b/y.md)\n- [z](docs/c/z.md)\n", "목차의 파일이 없음: docs/c/z.md"},
		{"꼴", "## 목차\n- x docs/a/x.md\n- [y](docs/b/y.md)\n", "꼴이 아님"},
		{"목차 없음", "# t\n", "목차」 줄이 없음"},
	}
	clitest.Put(t, R+"/docs/land-stats-baseline.md", "# 기록\n")
	for _, c := range []struct{ file, want string }{
		{"docs/new.md", "꼴이 아닌 문서: docs/new.md"},
		{"docs/a/b/c.md", "꼴이 아닌 문서: docs/a/b/c.md"},
	} {
		clitest.Put(t, R+"/README.md", ok)
		clitest.Put(t, R+"/"+c.file, "# x\n")
		if got := strings.Join(docsIndexProblems(R), "\n"); !strings.Contains(got, c.want) || strings.Contains(got, "land-stats") {
			t.Errorf("%s: 문제 %q, 기대 %q", c.file, got, c.want)
		}
		if err := os.Remove(R + "/" + c.file); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range cases {
		clitest.Put(t, R+"/README.md", c.readme)
		got := strings.Join(docsIndexProblems(R), "\n")
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%s: 문제 %q, 기대 %q", c.name, got, c.want)
		}
	}
}
