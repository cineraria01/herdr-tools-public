package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// 먼저 띄움 기다림 블록의 조건을 진짜 git으로 돌린다(2026-11-21 go-followups — 11/20 t30x 제보 land-stale-lows·carousel-save-race):
// 선행 커밋이 rebase로 SHA가 바뀌어 들어간 것(제목 머리)과 병합기 표식 refs/hc-landed/<선행>/<초>(제목이 레인 이름이 아님)을
// 둘 다 착지로 보고, 안 들어갔으면 아직. 이력이 파이프 버퍼(64KiB)보다 길고 pipefail이 켜져도 같게, zsh·bash 둘 다
func TestEarlyWaitRealGit(t *testing.T) {
	for _, b := range []string{"git", "zsh", "bash"} {
		if _, err := exec.LookPath(b); err != nil {
			t.Skip(b + " 없음")
		}
	}
	setData(t)
	EWAIT = 5
	st := time.Now().Unix() - 60
	row := pad16("pre", "app", "b", "-", "-", "up", "-", "-", "-", "t", "-", "-", "normal", "-", "-", "-").Set(qRID, fmt.Sprintf("r%d-1-1", st))
	if err := os.WriteFile(Q, []byte(row.Line()+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sec := earlySec("pre", "origin/main")
	cond := ""
	if i := strings.Index(sec, "\nuntil "); i >= 0 {
		cond, _, _ = strings.Cut(sec[i+len("\nuntil "):], "\ndo ")
	}
	if !strings.Contains(cond, "refs/hc-landed/pre/") {
		t.Fatalf("기다림 조건에 표식 판정이 없음:\n%s", sec)
	}

	d := t.TempDir()
	g := func(dir string, a ...string) string {
		t.Helper()
		c := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid"}, a...)...)
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	origin, w, lane := d+"/origin.git", d+"/w", d+"/lane"
	g(d, "init", "-q", "--bare", "-b", "main", origin)
	g(d, "clone", "-q", origin, w)
	// 이력 3000개(제목 합계 64KiB 넘음) — grep -q가 일찍 끝나 printf가 SIGPIPE를 받아도 판정이 같아야 한다
	var fi strings.Builder
	for i := 0; i < 3000; i++ {
		fmt.Fprintf(&fi, "commit refs/heads/main\ncommitter t <t@example.invalid> %d +0000\ndata <<E\nother-%d: 다른 레인의 아주 긴 제목 줄 채우기 채우기 채우기\nE\n", 1700000000+i, i)
		if i > 0 {
			fi.WriteString("\n")
		}
	}
	fe := exec.Command("git", "-C", w, "fast-import", "--quiet")
	fe.Stdin = strings.NewReader(fi.String())
	if out, err := fe.CombinedOutput(); err != nil {
		t.Fatalf("fast-import: %v\n%s", err, out)
	}
	g(w, "reset", "-q", "--hard", "main")
	g(w, "push", "-q", "origin", "main")
	g(d, "clone", "-q", origin, lane)

	run := func(sh string, pipefail bool) bool {
		t.Helper()
		pre := ""
		if pipefail {
			pre = "set -o pipefail\n"
		}
		c := exec.Command(sh, "-c", pre+"if "+cond+"\nthen echo yes; else echo no; fi")
		c.Dir = w
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", sh, err, out)
		}
		return strings.TrimSpace(string(out)) == "yes"
	}
	each := func(name string, want bool) {
		t.Helper()
		for _, sh := range []string{"zsh", "bash"} {
			for _, pf := range []bool{false, true} {
				if got := run(sh, pf); got != want {
					t.Errorf("%s(%s, pipefail %v): %v, 바람 %v", name, sh, pf, got, want)
				}
			}
		}
	}
	each("선행 안 들어감", false)

	// 선행 가지의 커밋(제목이 레인 이름 아님) — 병합기가 rebase해 C로 넣고 표식을 둔다
	g(lane, "commit", "-q", "--allow-empty", "-m", "feat(x): pre-1 다른 제목")
	g(w, "commit", "-q", "--allow-empty", "-m", "other-x: 그사이 들어간 남의 커밋")
	g(w, "push", "-q", "origin", "main")
	g(lane, "fetch", "-q")
	g(lane, "rebase", "-q", "origin/main")
	g(lane, "push", "-q", "origin", "HEAD:main")
	C := g(lane, "rev-parse", "HEAD")
	g(lane, "push", "-q", "origin", fmt.Sprintf("%s:refs/hc-landed/pre/%d", C, st-10)) // 줄을 넣기 전의 옛 표식
	each("제목 아님 + 옛 표식만", false)
	g(lane, "push", "-q", "origin", fmt.Sprintf("%s:refs/hc-landed/pre/%d", C, st+5))
	each("병합기 표식(조상)", true)

	// rebase로 SHA가 바뀐 커밋(git cherry로는 같은 패치) — 표식 없이 제목 머리로
	g(lane, "push", "-q", "origin", fmt.Sprintf(":refs/hc-landed/pre/%d", st+5))
	g(lane, "checkout", "-q", "-b", "pre2", "HEAD~1")
	g(lane, "commit", "-q", "--allow-empty", "-m", "pre: 본 작업")
	orig := g(lane, "rev-parse", "HEAD")
	g(w, "pull", "-q", "--rebase")
	g(w, "commit", "-q", "--allow-empty", "-m", "other-y: 또 남의 커밋")
	g(w, "push", "-q", "origin", "main")
	g(lane, "fetch", "-q")
	g(lane, "rebase", "-q", "origin/main")
	g(lane, "push", "-q", "origin", "HEAD:main")
	if g(lane, "rev-parse", "HEAD") == orig {
		t.Fatal("rebase가 SHA를 바꾸지 않음 — 시험 준비가 틀림")
	}
	each("rebase로 들어간 커밋(제목 머리)", true)
}
