package main

// 쌓아 띄움(stack.go): HC_LANE_STACK=<선행>@<P>면 승인 범위의 시작(forkOf)이 merge-base가 아니라 P — 선행 커밋이 승인 patch-id에 섞이지
// 않는다. 없으면 오늘처럼 merge-base, 제출 기록 칸도 없음

import (
	"os/exec"
	"strings"
	"testing"
)

func TestStackForkOf(t *testing.T) {
	d := t.TempDir()
	g := func(a ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", d, "-c", "user.name=t", "-c", "user.email=t@example.invalid"}, a...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	g("init", "-q", "-b", "main")
	g("commit", "-q", "--allow-empty", "-m", "처음")
	m := g("rev-parse", "HEAD")
	g("commit", "-q", "--allow-empty", "-m", "A: 선행")
	p := g("rev-parse", "HEAD")
	g("commit", "-q", "--allow-empty", "-m", "B: 뒤")
	s := g("rev-parse", "HEAD")
	t.Chdir(d)

	t.Setenv("HC_LANE_STACK", "")
	if f, rc := forkOf(s, "main~2"); rc != 0 || f != m {
		t.Errorf("쌓지 않음: 갈래점 %s(rc %d), 바람 merge-base %s", f, rc, m)
	}
	if stackMeta() != "" {
		t.Error("쌓지 않은 제출에 stack 칸")
	}
	t.Setenv("HC_LANE_STACK", "a-1@"+p)
	if f, rc := forkOf(s, "main~2"); rc != 0 || f != p {
		t.Errorf("쌓음: 갈래점 %s(rc %d), 바람 P %s", f, rc, p)
	}
	if got := stackMeta(); got != " stack=a-1@"+p {
		t.Errorf("제출 기록 칸: %q", got)
	}
	if pred, pp := stackEnv(); pred != "a-1" || pp != p {
		t.Errorf("stackEnv: %s %s", pred, pp)
	}
}
