package main

// add 때 비슷한 레인 알림(dupwarn.go) — 제목 낱말 겹침·경로 겹침, 겹치는 쌍과 안 겹치는 쌍

import (
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
)

func dupBrief(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := dir + "/" + name + ".md"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// 괄호 안(프로젝트·지휘·날짜)·머리 「레인 <이름>」·한 글자·숫자는 낱말이 아니다 — 지휘·날짜가 같다고 비슷해지지 않게
func TestDupTitlePairs(t *testing.T) {
	d := t.TempDir()
	a := dupTitle(dupBrief(t, d, "a", "# 레인 land-claim-net — 착지 원격 조회 실패를 75로 (app, 지휘 t30x, 2026-11-21)\n본문\n"))
	if got := strings.Join(sortedKeys(a), ","); got != "75로,실패를,원격,조회,착지" {
		t.Fatalf("낱말 %q", got)
	}
	cases := []struct {
		x, y string
		dup  bool
	}{
		{"# 레인 land-claim-net — 착지 원격 조회 실패를 75로 (app, 지휘 t30x, 2026-11-21)",
			"# 레인 land-query-75 — 착지 원격 조회 실패를 75로 고치기 (app, 지휘 t1cj, 2026-11-21)", true},
		{"# 레인 util-go — herdr-tools 전부 Go로 (herdr-tools, 지휘 t1cj, 2026-11-21)",
			"# 레인 hooks-go — herdr-tools 전부 Go로 (herdr-tools, 지휘 t1cj, 2026-11-21)", true},
		// 괄호 안만 같다(같은 프로젝트·지휘·날짜) → 아님
		{"# 레인 go-dup-warn — 같은 일을 두 번 넣지 않게 비슷한 레인 알려 주기 (herdr-tools, 지휘 t30x, 2026-11-22)",
			"# 레인 go-tailscale-heal — 기계의 Tailscale이 꺼지면 스스로 다시 켜기 (herdr-tools, 지휘 t30x, 2026-11-22)", false},
		// 낱말 하나만 겹침(비율이 높아도 두 낱말 미만) → 아님
		{"# 레인 a — 흔들림", "# 레인 b — 흔들림", false},
		// 제목 없음 → 아님
		{"본문만", "본문만", false},
	}
	for i, c := range cases {
		j, n := dupJaccard(dupTitle(dupBrief(t, d, "x", c.x+"\n")), dupTitle(dupBrief(t, d, "y", c.y+"\n")))
		if got := n >= 2 && j >= 0.5; got != c.dup {
			t.Errorf("%d: 비슷함 %v(비율 %.2f 겹 %d), 바람 %v", i, got, j, n, c.dup)
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	var o []string
	for k := range m {
		o = append(o, k)
	}
	sort.Strings(o)
	return o
}

func TestDupPaths(t *testing.T) {
	a := []Row{{"M", "cmd/x/add.go"}, {"R", "docs/"}, {"M", "FR-AB-1"}}
	b := []Row{{"M", "cmd/x/add.go"}, {"M", "docs/a.md"}, {"M", "lib/y.go"}}
	if got := strings.Join(dupPaths(a, b), ","); got != "cmd/x/add.go,docs/a.md" {
		t.Fatalf("겹침 %q", got)
	}
	if got := dupPaths(a, []Row{{"M", "lib/y.go"}, {"M", ""}}); len(got) != 0 {
		t.Fatalf("안 겹침이어야: %q", got)
	}
}

// dupWarn: 같은 프로젝트의 대기·떠 있는 줄만(모든 주인), 다른 프로젝트·감사 줄·끝난 상태·착지한 줄은 안 봄, 설정 off면 조용
func TestDupWarnAdd(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 없음")
	}
	setData(t)
	repo := t.TempDir()
	for _, f := range []string{"cmd/fill", "docs"} {
		if err := os.MkdirAll(repo+"/"+f, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"cmd/fill/add.go", "cmd/fill/run.go", "docs/a.md"} {
		if err := os.WriteFile(repo+"/"+f, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "landed-one: 착지함"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, c...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", c, out)
		}
	}
	LOCAL = "here"
	base := "local here\nrepo app here " + repo + "\nrepo other here " + repo + "\n"
	confLines = lines(base)
	d := t.TempDir()
	title := "# 레인 %s — 착지 원격 조회 실패를 75로 (app, 지휘 t1, 2026-11-21)\n"
	same := func(n string) string { return dupBrief(t, d, n, strings.Replace(title, "%s", n, 1)+"본문\n") }
	paths := dupBrief(t, d, "paths", "# 레인 paths — 전혀 다른 일\ncmd/fill/add.go·cmd/fill/run.go 를 고친다\n")
	onep := dupBrief(t, d, "onep", "# 레인 onep — 또 다른 일\ncmd/fill/add.go 만\n")
	far := dupBrief(t, d, "far", "# 레인 far — 화면 글꼴 바꾸기\ndocs/a.md\n")
	row := func(n, p, b, st, addr, own, k15 string) string {
		return strings.Join(pad16(n, p, b, "-", "-", st, "here", addr, "-", "x", "-", own, "normal", "-", k15, "-"), "\t") + "\n"
	}
	q := row("wait-a", "app", same("wait-a"), "wait", "-", "t2", "-") +
		row("up-b", "app", same("up-b"), "up", "here/w3", "-", "-") +
		row("paths", "app", paths, "up", "here/w4", "t1", "-") +
		row("onep", "app", onep, "wait", "-", "t1", "-") +
		row("far", "app", far, "wait", "-", "t1", "-") +
		row("other-prj", "other", same("other-prj"), "wait", "-", "t1", "-") +
		row("audit-x", "app", same("audit-x"), "up", "here/w5", "t1", "audit=a1") +
		row("sent-x", "app", same("sent-x"), "sent", "-", "t1", "-") +
		row("landed-one", "app", same("landed-one"), "up", "here/w6", "t1", "-")
	if err := os.WriteFile(Q, []byte(q), 0o644); err != nil {
		t.Fatal(err)
	}
	mine := dupBrief(t, d, "new", strings.Replace(title, "%s", "new", 1)+"cmd/fill/add.go 와 cmd/fill/run.go 를 고친다\n")

	got := dupCapture(t, func() { dupWarn("new", "app", mine) })
	want := "알림: 비슷한 레인 wait-a (주인 t2, 대기) — 제목 낱말 100% 겹침\n" +
		"알림: 비슷한 레인 up-b (주인 없음, 떠 있음 here/w3) — 제목 낱말 100% 겹침\n" +
		"알림: 비슷한 레인 paths (주인 t1, 떠 있음 here/w4) — 경로 2개 겹침(cmd/fill/add.go,cmd/fill/run.go)\n"
	if got != want {
		t.Fatalf("알림\n got %q\nwant %q", got, want)
	}
	// 안 겹치는 지시문은 조용
	quiet := dupBrief(t, d, "new2", "# 레인 new2 — 소리 크기 맞추기\n본문\n")
	if got := dupCapture(t, func() { dupWarn("new2", "app", quiet) }); got != "" {
		t.Fatalf("조용해야: %q", got)
	}
	// 문턱 올리면(경로 3개) 경로 알림이 빠지고, off면 다 조용
	confLines = lines(base + "dup_warn 0.5 3\n")
	if got := dupCapture(t, func() { dupWarn("new", "app", mine) }); strings.Contains(got, "paths") || !strings.Contains(got, "wait-a") {
		t.Fatalf("경로 문턱 3: %q", got)
	}
	confLines = lines(base + "dup_warn 0 2\n")
	if got := dupCapture(t, func() { dupWarn("new", "app", mine) }); strings.Contains(got, "wait-a") || !strings.Contains(got, "paths") {
		t.Fatalf("낱말 끔: %q", got)
	}
	confLines = lines(base + "dup_warn off\n")
	if got := dupCapture(t, func() { dupWarn("new", "app", mine) }); got != "" {
		t.Fatalf("off: %q", got)
	}
}

func dupCapture(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	os.Stdout = old
	w.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
