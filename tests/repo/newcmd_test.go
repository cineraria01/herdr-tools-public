package repo_test

// 새 명령을 더할 때 함께 고칠 곳(2026-11-24 newcmd-cheap — 「cmd/*마다 sync.list 바이너리 줄」 확인이 무거운 조각의 herdr-sync 계약
// 시험에서만 돌아, 새 명령을 더한 레인이 병합기 검증 뒤에야 반송됐다). 싼 단계에서 파일만 읽어(빌드·실행 없음) cmd/<도구>마다 본다:
// ① config.example/sync.list 바이너리 줄 ② README 도구 표(「## 목차」 앞)의 `<도구>` ③ docs/<도구>/ — 없으면 docsElsewhere
// ④ --version 꼴 「<도구> (Go …)」(internal/version.String) — 아니면 versionOwn. 원래 계약 시험의 확인은 그대로 둔다(겹친다).

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"herdr-tools/internal/clitest"
)

// docsElsewhere: docs/<도구>/ 폴더 없이 다른 문서가 설명하는 도구 → 그 문서(있어야 하고 도구 이름이 나와야 한다).
// 새 도구는 되도록 docs/<도구>/<기능>.md를 만들고 여기 더하지 않는다
var docsElsewhere = map[string]string{
	"hc-core-relaunch":     "docs/go/small-tools.md",
	"hc-merger":            "docs/hc-lane/merger.md", // hc-lane의 곁 도구
	"herdr-balance":        "docs/go/small-tools.md",
	"herdr-settings-merge": "docs/go/ops-tools.md", // herdr-sync가 부른다
	"ovis-split":           "docs/go/small-tools.md",
	"worker-mac-check":     "docs/go/small-tools.md",
	"worker-mac-setup":     "docs/go/small-tools.md",
}

// versionOwn: --version이 version.String 꼴이 아닌 도구 → 까닭(출력은 바꾸지 않는다 — 바꾸는 레인이 여기서 지운다)
var versionOwn = map[string]string{
	"herdr-hooks": "고정 「herdr-hooks 1」(const version)",
}

// newCmdProblems: cmd/<도구>마다 빠진 곳(없으면 nil). 글은 「<무엇>이 빠졌다 — <어느 파일>에 <어떤 줄>을 더하라」
func newCmdProblems(root string) []string {
	var bad []string
	read := func(p string) string {
		b, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			bad = append(bad, p+"를 못 읽음: "+err.Error())
		}
		return string(b)
	}
	list := read("config.example/sync.list")
	readme := read("README.md")
	if i := strings.Index(readme, "\n## 목차"); i >= 0 {
		readme = readme[:i]
	}
	ents, err := os.ReadDir(filepath.Join(root, "cmd"))
	if err != nil {
		return append(bad, "cmd/를 못 읽음: "+err.Error())
	}
	var tools []string
	for _, e := range ents {
		// 점 폴더(세션 훅이 남긴 .omc 등)는 도구가 아니다 — 계약 시험과 같다
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			tools = append(tools, e.Name())
		}
	}
	for _, t := range tools {
		q := regexp.QuoteMeta(t)
		if !regexp.MustCompile(`(?m)\s\.local/bin/` + q + `\s+tools:cmd/` + q + `\s`).MatchString(list) {
			bad = append(bad, "cmd/"+t+"의 바이너리 줄이 빠졌다 — config.example/sync.list에 「도구  .local/bin/"+t+"  tools:cmd/"+t+"  macos」 줄을 더하라(대상은 도는 곳대로 all·macos·local)")
		}
		if !strings.Contains(readme, "`"+t+"`") {
			bad = append(bad, "cmd/"+t+"의 README 도구 줄이 빠졌다 — README.md 도구 표(「## 목차」 앞)에 「| `"+t+"` | <하는 일> | <도는 곳> |」 줄을 더하라")
		}
		if d, ok := docsElsewhere[t]; ok {
			if b, err := os.ReadFile(filepath.Join(root, d)); err != nil || !strings.Contains(string(b), t) {
				bad = append(bad, "cmd/"+t+"의 문서가 빠졌다 — 예외 목록(tests/repo/newcmd_test.go docsElsewhere)의 "+d+"가 없거나 "+t+"를 말하지 않는다: 그 문서에 "+t+" 설명 줄을 더하거나 docs/"+t+"/<기능>.md를 만들라")
			}
		} else if fi, err := os.Stat(filepath.Join(root, "docs", t)); err != nil || !fi.IsDir() {
			bad = append(bad, "cmd/"+t+"의 문서가 빠졌다 — docs/"+t+"/<기능>.md를 만들고 README.md 「## 목차」 이름순 자리에 「- [<제목>](docs/"+t+"/<기능>.md)」 줄을 더하라(다른 문서가 설명하면 tests/repo/newcmd_test.go docsElsewhere에 「\""+t+"\": \"<그 문서>\"」)")
		}
		if _, ok := versionOwn[t]; !ok && !versionForm(root, t) {
			bad = append(bad, "cmd/"+t+"의 --version 꼴이 빠졌다 — cmd/"+t+"/main.go에 「if len(os.Args) > 1 && os.Args[1] == \"--version\" { fmt.Println(version.String(\""+t+"\")); return }」를 더하라(꼴 「"+t+" (Go …)」, internal/version)")
		}
	}
	return bad
}

// staleExceptions: 예외 목록에 있는데 cmd/에 없는 도구(지운 도구·오타)
func staleExceptions(root string) []string {
	var bad []string
	for _, m := range []map[string]string{docsElsewhere, versionOwn} {
		var ks []string
		for k := range m {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		for _, k := range ks {
			if fi, err := os.Stat(filepath.Join(root, "cmd", k)); err != nil || !fi.IsDir() {
				bad = append(bad, "예외 목록의 "+k+"가 cmd/에 없다 — tests/repo/newcmd_test.go에서 그 줄을 지우라")
			}
		}
	}
	return bad
}

// versionForm: cmd/<t>의 시험 아닌 Go 파일이 version.String("<t>")를 부르거나, version.String(me)에 me = "<t>"
func versionForm(root, t string) bool {
	fs, _ := filepath.Glob(filepath.Join(root, "cmd", t, "*.go"))
	var src strings.Builder
	for _, f := range fs {
		if !strings.HasSuffix(f, "_test.go") {
			if b, err := os.ReadFile(f); err == nil {
				src.Write(b)
			}
		}
	}
	s, q := src.String(), regexp.QuoteMeta(`"`+t+`"`)
	return regexp.MustCompile(`version\.String\(`+q+`\)`).MatchString(s) ||
		(strings.Contains(s, "version.String(me)") && regexp.MustCompile(`\bme\s*=\s*`+q).MatchString(s))
}

func TestNewCmd(t *testing.T) {
	root := clitest.Root(t)
	for _, p := range append(newCmdProblems(root), staleExceptions(root)...) {
		t.Error(p)
	}
}

func TestNewCmdSelf(t *testing.T) {
	R := t.TempDir()
	main := func(tool string) string {
		return "package main\n\nfunc main() {\n\tif len(os.Args) > 1 && os.Args[1] == \"--version\" {\n\t\tfmt.Println(version.String(\"" + tool + "\"))\n\t}\n}\n"
	}
	readme := "# t\n\n| 도구 | 하는 일 | 도는 곳 |\n|---|---|---|\n| `old` | 옛것 | 맥 |\n\n## 목차\n\n- [x](docs/old/x.md)\n- `new` 목차 안에만\n"
	clitest.Put(t, R+"/README.md", readme)
	clitest.Put(t, R+"/config.example/sync.list", "도구  .local/bin/old  tools:cmd/old  macos\n")
	clitest.Put(t, R+"/cmd/old/main.go", main("old"))
	clitest.Put(t, R+"/docs/old/x.md", "# x\n")
	clitest.Put(t, R+"/cmd/.omc/x", "점 폴더는 도구가 아님\n")
	if got := newCmdProblems(R); len(got) != 0 {
		t.Fatalf("다 채운 트리인데 문제 %q", got)
	}

	// ① cmd/new만 더함 → 빠진 곳 셋(바이너리 줄·README·문서), 실패 글은 더할 파일과 줄을 말한다
	clitest.Put(t, R+"/cmd/new/main.go", main("new"))
	got := newCmdProblems(R)
	all := strings.Join(got, "\n")
	for _, w := range []string{
		"cmd/new의 바이너리 줄이 빠졌다 — config.example/sync.list에 「도구  .local/bin/new  tools:cmd/new  macos」",
		"cmd/new의 README 도구 줄이 빠졌다 — README.md 도구 표",
		"cmd/new의 문서가 빠졌다 — docs/new/<기능>.md를 만들고",
	} {
		if !strings.Contains(all, w) {
			t.Errorf("① 빠진 글 %q, 문제 %q", w, all)
		}
	}
	if len(got) != 3 {
		t.Errorf("① 문제 %d개(바람 3): %q", len(got), all)
	}

	// --version 꼴이 다르면 넷째
	clitest.Put(t, R+"/cmd/new/main.go", "package main\n\nconst version = \"new 1\"\n")
	if all := strings.Join(newCmdProblems(R), "\n"); !strings.Contains(all, "cmd/new의 --version 꼴이 빠졌다 — cmd/new/main.go에") {
		t.Errorf("--version: 문제 %q", all)
	}
	clitest.Put(t, R+"/cmd/new/main_test.go", "package main\n\nvar _ = version.String(\"new\")\n") // 시험 파일은 안 친다
	if all := strings.Join(newCmdProblems(R), "\n"); !strings.Contains(all, "--version 꼴이 빠졌다") {
		t.Errorf("--version(시험 파일만): 문제 %q", all)
	}
	clitest.Put(t, R+"/cmd/new/main.go", "package main\n\nconst me = \"new\"\n\nvar _ = version.String(me)\n")

	// ② 다 채움 → 통과
	clitest.Put(t, R+"/config.example/sync.list", "도구  .local/bin/old  tools:cmd/old  macos\n맥도구  .local/bin/new  tools:cmd/new  local\n")
	clitest.Put(t, R+"/README.md", strings.Replace(readme, "| `old` | 옛것 | 맥 |\n", "| `old` | 옛것 | 맥 |\n| `new` | 새것 | 맥 |\n", 1))
	clitest.Put(t, R+"/docs/new/usage.md", "# new\n")
	if got := newCmdProblems(R); len(got) != 0 {
		t.Errorf("② 다 채웠는데 문제 %q", got)
	}

	// ③ 문서 없는 도구: 예외 목록(docsElsewhere)에 없으면 실패, 있으면 그 문서가 도구를 말해야 통과
	clitest.Put(t, R+"/cmd/hc-merger/main.go", main("hc-merger"))
	clitest.Put(t, R+"/config.example/sync.list", "도구  .local/bin/old  tools:cmd/old  macos\n맥도구  .local/bin/new  tools:cmd/new  local\n도구  .local/bin/hc-merger  tools:cmd/hc-merger  macos\n도구  .local/bin/lone  tools:cmd/lone  macos\n")
	clitest.Put(t, R+"/README.md", strings.Replace(readme, "| `old` | 옛것 | 맥 |\n", "| `old` | 옛것 | 맥 |\n| `new` · `hc-merger` · `lone` | 새것 | 맥 |\n", 1))
	clitest.Put(t, R+"/cmd/lone/main.go", main("lone"))
	all = strings.Join(newCmdProblems(R), "\n")
	if !strings.Contains(all, "cmd/lone의 문서가 빠졌다 — docs/lone/<기능>.md") {
		t.Errorf("③ 예외 밖 문서 없음: 문제 %q", all)
	}
	if !strings.Contains(all, "cmd/hc-merger의 문서가 빠졌다 — 예외 목록") {
		t.Errorf("③ 예외 문서가 없음: 문제 %q", all)
	}
	clitest.Put(t, R+"/docs/hc-lane/merger.md", "# hc-merger 병합기\n")
	all = strings.Join(newCmdProblems(R), "\n")
	if strings.Contains(all, "cmd/hc-merger") || !strings.Contains(all, "cmd/lone의 문서") {
		t.Errorf("③ 예외 문서가 있음: 문제 %q", all)
	}
	// 예외 목록의 다른 도구가 cmd/에 없음(임시 트리라서) → 지우라는 글
	if all := strings.Join(staleExceptions(R), "\n"); strings.Contains(all, "hc-merger") || !strings.Contains(all, "예외 목록의 ovis-split가 cmd/에 없다") || !strings.Contains(all, "예외 목록의 herdr-hooks가 cmd/에 없다") {
		t.Errorf("없는 예외: 문제 %q", all)
	}
}
