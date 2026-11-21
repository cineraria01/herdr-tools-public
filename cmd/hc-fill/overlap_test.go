package main

// W3 단위 시험: 겹침 키(ktok·lkeys)·ovcmp 기대값은 셸 판(lib/hc-fill/overlap.sh)을 실제로 돌려 기록한 골든이다(손으로 쓴 값 아님).
// 지휘 기계 파일 붙이기(lcPasteScan·lcPasteHome)는 Go 판에만 있다

import (
	"os"
	"strings"
	"testing"
)

// 붙일 폴더(gofix-fill F07·F4): 기본 ~/.claude-tasks 아래만, 링크를 푼 실제 경로가 밖이면(~/.netrc를 가리키는 링크·..) 붙이지 않음
func TestLcPasteDirs(t *testing.T) {
	old, oc := HOME, confLines
	defer func() { HOME, confLines = old, oc }()
	HOME, confLines = t.TempDir(), nil
	for name, s := range map[string]string{".netrc": "machine x login me password hunter2hunter2\n", ".ssh/config": "Host x\n",
		".claude-tasks/a/report.md": "보고 원문\n", "notes/n.md": "메모\n"} {
		if err := os.MkdirAll(HOME+"/"+name[:strings.LastIndexByte(name, '/')+1], 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(HOME+"/"+name, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(HOME+"/.netrc", HOME+"/.claude-tasks/a/link.md"); err != nil {
		t.Fatal(err)
	}
	body := "~/.netrc ~/.ssh/config ~/notes/n.md ~/.claude-tasks/a/link.md ~/.claude-tasks/../.netrc ~/.claude-tasks/a/report.md"
	got, ok := lcPasteHome(body, "-", "ln")
	if want := "\n## (붙임) 지휘 기계 파일 원문 — ~/.claude-tasks/a/report.md\n보고 원문\n"; !ok || got != want {
		t.Errorf("기본 폴더\n got %q (ok %v)\nwant %q", got, ok, want)
	}
	confLines = []string{"paste_dirs ~/notes ~/.claude-tasks /etc"}
	if got, _ := lcPasteHome(body, "-", "ln"); strings.Count(got, "원문 — ") != 2 || !strings.Contains(got, "메모") || strings.Contains(got, "hunter2") {
		t.Errorf("설정 폴더: %q", got)
	}
}

// ovGlob은 글자 단위(gofix-fill F13 — 한글 무시 패턴이 바이트마다 다시 적혀 안 맞았다)
func TestOvGlobUnicode(t *testing.T) {
	for _, c := range []struct {
		pat, s string
		want   bool
	}{{"문서/*", "문서/안내.md", true}, {"?.md", "안.md", true}, {"\\문서/*", "문서/x", true}, {"[가나]*", "나무", true},
		{"[!가]*", "가지", false}, {"문서/*", "자료/안내.md", false}} {
		if got := ovGlob(c.pat, c.s); got != c.want {
			t.Errorf("ovGlob(%q, %q) = %v, 기대 %v", c.pat, c.s, got, c.want)
		}
	}
}

// rowsBar: 줄을 「칸\t칸|」 꼴로(기록한 골든과 같은 꼴)
func rowsBar(rs []Row) string {
	var b strings.Builder
	for _, r := range rs {
		b.WriteString(r.Line() + "|")
	}
	return b.String()
}

func TestKtokGolden(t *testing.T) {
	kp := []string{"/home/me/src/app", "/Users/me/src/app"}
	cases := []struct{ kind, in, want string }{
		// 괄호 안 설명(겹 괄호) 빼기 · 백틱 항목은 그 안 · 뒤 설명이 붙은 항목은 q · FR ID · 가운뎃점 나눔
		{"R", "bin/tool (설명 (겹)), `docs/a b.md` 뒤 설명, lib/x.sh 참고, FR-AB-12·README",
			"R\tp\tbin/tool|R\tp\tdocs/a b.md|R\tq\tlib/x.sh|R\tid\tFR-AB-12|R\tp\tREADME|"},
		// 절대 경로 접두 떼기 · .wt/<레인>/ 접두 떼기 · 레인 폴더만이면 버림 · 모르는 절대 경로·저장소 밖(..) 버림 · ./ 정리 · ，·、 나눔
		{"R", "/home/me/src/app/bin/t, /home/me/src/app.wt/ln/lib/u.go; /home/me/src/app.wt/ln, /srv/x, ../a、./b/./c，FR-X",
			"R\tp\tbin/t|R\tp\tlib/u.go|R\tp\tb/c|R\tid\tFR-X|"},
		// 백틱 뒤 조사 · 괄호 뒤 조사 · 「」 안(꼬리 」를 은 셸 판처럼 남음) · FR ID 머리 · 조사 떼기
		{"M", "`bin/tool`을 고친다 (lib/b.sh)에서 「docs/a.md」를 FR-TRV-005의 Makefile만",
			"M\tm\tbin/tool\tbin/tool|M\tm\tlib/b.sh)에서\tlib/b.sh|M\tm\tdocs/a.md」를\tdocs/a.md」를|M\tid\tFR-TRV-005|M\tm\tMakefile만\tMakefile|"},
		// 조사 둘 글자 · 끝 마침표 · ~/ · URL · 문장부호만 · 절대 .wt 경로와 쉼표
		{"M", "x/y.go으로 a/b.까지 ~/z http://a/b ._/ /Users/me/src/app.wt/o1/lib/b.sh, e.g.",
			"M\tm\tx/y.go으로\tx/y.go|M\tm\ta/b.까지\ta/b|M\tm\tlib/b.sh\tlib/b.sh|M\tm\te.g\te.g|"},
	}
	for _, c := range cases {
		if got := rowsBar(ktok(c.kind, []string{c.in}, kp)); got != c.want {
			t.Errorf("ktok %s %q\n got %q\nwant %q", c.kind, c.in, got, c.want)
		}
	}
}

func TestLkeysGolden(t *testing.T) {
	d := t.TempDir()
	or := d + "/orepo"
	for _, x := range []string{"bin", "lib", "docs/frag"} {
		if err := os.MkdirAll(or+"/"+x, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(or+".wt/o1", 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"bin/tool", "lib/b.sh", "docs/frag/x.md", "docs/a.md", "docs/안내.md", "Makefile", "README.md"} {
		if err := os.WriteFile(or+"/"+f, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	confLines = lines("local here\nrepo app here " + or + "/\nrepo app box /home/me/src/app\noverlap_ignore app docs/frag/* CHANGELOG.md\n")
	LOCAL = "here"
	cases := []struct{ brief, want string }{
		{"파일: `bin/tool`을 고친다\n- 파일: bin/tool, docs/a.md\n", "R\tbin/tool|R\tdocs/a.md|"},
		{"파일: " + or + "/bin/new-tool, /home/me/src/app.wt/other-lane/lib/n.go, /srv/e/bin/n, ../bin/n\n", "R\tbin/new-tool|R\tlib/n.go|"},
		// 뒤 설명 「(new)」·설명 낱말(see·only·docs·e.g)은 예약 키가 아님
		{"파일: docs/안내.md (new), lib/c.sh (new)\n- 파일: `bin/tool` (the run section only, see README), e.g. new docs\n",
			"R\tdocs/안내.md|R\tlib/c.sh|R\tbin/tool|"},
		// 맨 이름 README → README.md · 폴더 예약은 끝 / · 맨 위 실제 파일(Makefile) 언급
		{"- 파일: lib/e.sh, README\nMakefile를 고친다 `Makefile` 을 본다\n파일: `docs/`\n파일: bin\n",
			"R\tlib/e.sh|R\tREADME.md|R\tdocs/|R\tbin/|M\tMakefile|"},
		// 아직 없는 새 파일 예약 · 가운뎃점 언급 · overlap_ignore · 첫 폴더가 있는 새 경로만 언급
		{"파일: main.c, .newconfig, NewTool\nbin/tool·lib/b.sh 를 본다 docs/frag/x.md CHANGELOG.md lib/new/x.go nolib/y.go\n",
			"R\tmain.c|R\t.newconfig|R\tNewTool|M\tbin/tool|M\tlib/b.sh|M\tlib/new/x.go|"},
	}
	for i, c := range cases {
		bf := d + "/b" + string(rune('0'+i)) + ".md"
		if err := os.WriteFile(bf, []byte(c.brief), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := rowsBar(lkeys("app", bf)); got != c.want {
			t.Errorf("lkeys %q\n got %q\nwant %q", c.brief, got, c.want)
		}
	}
}

func TestOvcmpGolden(t *testing.T) {
	own := []Row{{"o1", "R", "bin/tool"}, {"o1", "M", "docs/a.md"}, {"o2", "M", "bin/tool"}, {"o3", "R", "docs/"},
		{"o4", "R", "lib/x.sh"}, {"o4", "R", "lib/y.sh"}, {"o4", "R", "lib/z.sh"}, {"o5", "R", ""}}
	cand := []Row{{"R", "bin/tool"}, {"M", "docs/a.md"}, {"R", "docs/b.md"}, {"M", "lib/x.sh"}, {"M", "lib/y.sh"}, {"M", "lib/z.sh"}, {"R", ""}}
	// 예약 = 둘 다 R(폴더 예약은 그 아래와 겹침), 언급은 키 둘까지, 빈 키는 겹침 아님
	want := "o1\t예약\tbin/tool|o2\t언급\tbin/tool|o3\t예약\tdocs/b.md|o4\t언급\tlib/x.sh,lib/y.sh|"
	if got := rowsBar(ovcmp(own, cand)); got != want {
		t.Errorf("ovcmp\n got %q\nwant %q", got, want)
	}
}

func TestLcPaste(t *testing.T) {
	old, oc := HOME, confLines
	defer func() { HOME, confLines = old, oc }()
	HOME = t.TempDir()
	confLines = []string{"paste_dirs ~/"} // 이 시험은 HOME 아래 어디든(붙일 폴더 한정은 TestLcPasteDirs)
	w := func(name, s string) {
		if err := os.MkdirAll(HOME+"/"+name[:strings.LastIndexByte(name, '/')+1], 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(HOME+"/"+name, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w("d/a.md", "에이\n")
	w("d/b.md", "비\n")
	w("big.md", strings.Repeat("x", lcPasteFileMax+1))
	w("sec.md", "설정\nAPI_TOKEN=abcdef0123456789xyz\n")
	w("doc.md", "문서: API_KEY·token= 자리는 설정에 둔다\n")
	for i := 0; i < 5; i++ {
		w("m/"+string(rune('0'+i))+".md", strings.Repeat("y", 60<<10))
	}

	// 붙임: ~/ 꼴과 HOME 절대 꼴, 백틱·괄호·조사 뗌, 같은 경로는 한 번, HOME 밖·없는 파일·폴더·HOME 밖으로 나가는 .. 는 건너뜀
	body := "`~/d/a.md`를 읽고 (" + HOME + "/d/b.md)에서 고친다. 다시 ~/d/a.md 와 /etc/hosts ~/없음.md ~/d ~/../x.md ~/d/../d/b.md"
	got, ok := lcPasteHome(body, "-", "ln")
	want := "\n## (붙임) 지휘 기계 파일 원문 — ~/d/a.md\n에이\n\n## (붙임) 지휘 기계 파일 원문 — " + HOME + "/d/b.md\n비\n"
	if !ok || got != want {
		t.Errorf("붙임\n got %q (ok %v)\nwant %q", got, ok, want)
	}
	// 로컬 경로가 아님(HOME 아래가 아님)이면 붙일 것 없음. hc-fill 데이터 폴더(레인 기계 자기 runs·reports)도 붙이지 않음
	w(".local/share/hc-fill/runs/ln-run.md", "옛 run\n")
	if got, ok := lcPasteHome("/srv/x.md /etc/hosts 본문만 ~/.local/share/hc-fill/runs/ln-run.md", "-", "ln"); !ok || got != "" {
		t.Errorf("HOME 밖·데이터 폴더: %q %v", got, ok)
	}
	// 비밀 이름만 든 문서(값 없음)는 붙인다
	if got, ok := lcPasteHome("~/doc.md", "-", "ln"); !ok || !strings.Contains(got, "API_KEY") {
		t.Errorf("문서 속 이름만: %q %v", got, ok)
	}
	// 비밀 꼴은 막힘(까닭과 경로)
	if p, shown, abs, why := lcPasteScan("「~/sec.md」"); p != "" || shown != "~/sec.md" || abs != HOME+"/sec.md" || !strings.Contains(why, "비밀 꼴(이름=값)") {
		t.Errorf("막힘 비밀: shown %q abs %q why %q", shown, abs, why)
	}
	// 파일 상한·합계 상한·바이너리는 막지 않고 경로만(2026-11-21 go-followups-2 — 5.5MB 바이너리 경로에 레인이 아예 안 떴다)
	only := func(c string) string {
		return "\n## (붙임) 지휘 기계 파일 — " + c + "\n(붙이지 않음: 큼/바이너리 — 경로만)\n"
	}
	w("bin.dat", "a\x00b")
	if got, ok := lcPasteHome("~/big.md 와 ~/bin.dat 를 본다", "-", "ln"); !ok || got != only("~/big.md")+only("~/bin.dat") {
		t.Errorf("큼·바이너리: %q %v", got, ok)
	}
	got, ok = lcPasteHome("~/m/0.md ~/m/1.md ~/m/2.md ~/m/3.md ~/m/4.md", "-", "ln")
	if !ok || strings.Count(got, "지휘 기계 파일 원문 — ") != 4 || !strings.HasSuffix(got, only("~/m/4.md")) {
		t.Errorf("합계 상한: 넷은 원문, 다섯째는 경로만이어야 (ok %v, 원문 %d)", ok, strings.Count(got, "지휘 기계 파일 원문 — "))
	}
}
