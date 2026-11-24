package repo_test

// agent-skill-guides(2026-11-24): 도구마다 에이전트가 읽는 안내(cmd/<도구>/skill.md, `<도구> --skill`)가 낡지 않게 — 파일만 읽는다
// (빌드·실행 없음, 실행 확인은 cmd/<도구>/skill_test.go). ① 안내가 있고 정한 절이 다 있다 ② 안내에 나온 `<도구> <하위 명령>`이
// 등록돼 있다(register("…") — 등록표가 있는 도구만) ③ 등록된 하위 명령이 안내에 있다(아니면 skillOmit에 까닭) ④ 안내에 나온
// 인자(--…)가 그 도구 코드(시험 빼고)에 글자로 있다

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"herdr-tools/internal/clitest"
)

var skillTools = []string{"hc-fill", "hc-lane", "hc-merger", "herdr-sync", "hc-sweep"}

// skillSections: 안내마다 있어야 하는 절(차례대로)
var skillSections = []string{"## 하는 일", "## 흔한 흐름", "## 상태 보기", "## 조정과 되돌리기", "## 사건을 받으면", "## 사람에게 물을 것", "## 하지 않을 것", "## 더 읽을 곳"}

// skillMaxLines: 「한 화면 남짓」 — 더 깊은 것은 문서 경로로
const skillMaxLines = 90

// skillOmit: 등록됐지만 안내에서 뺀 하위 명령(「<도구> <하위 명령>」) → 까닭. 본 작업에서 채운다
var skillOmit = map[string]string{
	"hc-fill machines":   "machine과 같은 명령(다른 이름)",
	"hc-lane launch":     "첫 인자가 하위 명령이 아니면 띄우기다 — 안내는 `hc-lane --name …` 꼴로 적는다",
	"hc-lane early-wait": "hc-fill이 먼저 띄운 레인의 지시문 기다림 블록에 적어 부른다 — 에이전트가 고를 명령이 아니다",
}

var (
	reRegister = regexp.MustCompile(`register\("([a-z][a-z0-9-]*)"`)
	// 이름 목록을 돌며 register(c, …)로 넣는 꼴(hc-lane events·wait·watch)
	reRegLoop = regexp.MustCompile(`(?s)range \[\]string\{([^}]*)\} \{[^{}]*?register\(`)
	reQuoted  = regexp.MustCompile(`"([a-z][a-z0-9-]*)"`)
	reTick    = regexp.MustCompile("`([^`\n]+)`")
	reFlag    = regexp.MustCompile(`--[a-z][a-z0-9-]*`)
)

// toolSrc: cmd/<도구>의 시험 아닌 Go 파일을 이은 글
func toolSrc(t *testing.T, root, tool string) string {
	t.Helper()
	fs, err := filepath.Glob(filepath.Join(root, "cmd", tool, "*.go"))
	if err != nil || len(fs) == 0 {
		t.Fatalf("cmd/%s/*.go를 못 읽음: %v", tool, err)
	}
	var b strings.Builder
	for _, f := range fs {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		s, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(s)
	}
	return b.String()
}

// skillProblems: 한 도구 안내의 어긋남(없으면 nil)
func skillProblems(t *testing.T, root, tool string) []string {
	b, err := os.ReadFile(filepath.Join(root, "cmd", tool, "skill.md"))
	if err != nil {
		return []string{"cmd/" + tool + "/skill.md가 없다 — 에이전트용 안내를 쓰고 main.go에서 --skill로 내라"}
	}
	doc := string(b)
	var bad []string
	if n := strings.Count(doc, "\n"); n > skillMaxLines {
		bad = append(bad, tool+": 안내가 너무 길다("+strconv.Itoa(n)+"줄 > "+strconv.Itoa(skillMaxLines)+") — 깊은 것은 「## 더 읽을 곳」의 문서로")
	}
	at := 0
	for _, s := range skillSections {
		i := strings.Index(doc[at:], "\n"+s+"\n")
		if i < 0 {
			bad = append(bad, tool+": 절 「"+s+"」이 없거나 차례가 어긋났다")
			continue
		}
		at += i + 1
	}
	src := toolSrc(t, root, tool)
	reg := map[string]bool{}
	for _, m := range reRegister.FindAllStringSubmatch(src, -1) {
		reg[m[1]] = true
	}
	for _, m := range reRegLoop.FindAllStringSubmatch(src, -1) {
		for _, q := range reQuoted.FindAllStringSubmatch(m[1], -1) {
			reg[q[1]] = true
		}
	}
	said := map[string]bool{}
	for _, m := range reTick.FindAllStringSubmatch(doc, -1) {
		w := strings.Fields(m[1])
		if len(w) == 0 || w[0] != tool {
			continue
		}
		if len(w) > 1 && len(reg) > 0 && !strings.HasPrefix(w[1], "-") && !strings.HasPrefix(w[1], "<") {
			said[w[1]] = true
			if !reg[w[1]] {
				bad = append(bad, tool+": 안내의 `"+m[1]+"` — 하위 명령 "+w[1]+"이 등록돼 있지 않다")
			}
		}
		for _, f := range reFlag.FindAllString(m[1], -1) {
			if !strings.Contains(src, `"`+f+`"`) && !strings.Contains(src, `"`+f+`=`) && !strings.Contains(src, `"`+f[2:]+`"`) {
				bad = append(bad, tool+": 안내의 `"+m[1]+"` — 인자 "+f+"가 cmd/"+tool+" 코드에 없다")
			}
		}
	}
	var names []string
	for n := range reg {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if strings.HasPrefix(n, "_") || said[n] {
			continue
		}
		if _, ok := skillOmit[tool+" "+n]; !ok {
			bad = append(bad, tool+": 등록된 하위 명령 "+n+"이 안내에 없다 — cmd/"+tool+"/skill.md에 `"+tool+" "+n+" …`을 적거나 tests/repo/skill_test.go skillOmit에 까닭과 함께")
		}
	}
	return bad
}

func TestSkillGuides(t *testing.T) {
	root := clitest.Root(t)
	for _, tool := range skillTools {
		for _, p := range skillProblems(t, root, tool) {
			t.Error(p)
		}
	}
}

// 대조가 실제로 잡는지: 등록 안 된 하위 명령·없는 인자·빠진 절·빠진 등록 명령
func TestSkillGuidesSelf(t *testing.T) {
	root := t.TempDir()
	put := func(p, s string) {
		clitest.Put(t, filepath.Join(root, p), s)
	}
	put("cmd/hc-fill/main.go", `package main
func init() { register("add", nil); register("ls", nil); register("_tick", nil) }
func init() {
	for _, c := range []string{"wait"} {
		register(c, nil)
	}
}
var _, _ = "--json", "--name"`)
	good := "# hc-fill\n"
	for _, s := range skillSections {
		good += "\n" + s + "\n\n- `hc-fill add --name x` · `hc-fill ls --json` · `hc-fill wait`\n"
	}
	put("cmd/hc-fill/skill.md", good)
	if p := skillProblems(t, root, "hc-fill"); len(p) != 0 {
		t.Fatalf("맞는 안내가 걸렸다: %q", p)
	}
	bad := strings.Replace(good, "## 하지 않을 것\n", "", 1)
	bad = strings.Replace(bad, "`hc-fill ls --json`", "`hc-fill lsx --yaml`", -1)
	put("cmd/hc-fill/skill.md", bad)
	got := strings.Join(skillProblems(t, root, "hc-fill"), "\n")
	for _, w := range []string{"「## 하지 않을 것」", "하위 명령 lsx", "인자 --yaml", "등록된 하위 명령 ls이"} {
		if !strings.Contains(got, w) {
			t.Errorf("%q를 못 잡았다:\n%s", w, got)
		}
	}
}
