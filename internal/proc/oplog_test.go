//go:build !windows

package proc

// 운영 기록 길목(2026-11-23 ht-oplog): 시간 넘김 줄이 표준 오류에 그대로 나가고, 운영 기록에 timeout 줄로도 남는다

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestTimeoutGoesToOpLog(t *testing.T) {
	fakeGit(t)
	t.Setenv("HERDR_TIMEOUT_GIT", "1")
	ops := t.TempDir() + "/ops.tsv"
	t.Setenv("HERDR_TOOLS_OPLOG", ops)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	done := make(chan string)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	rc := Code(Run(exec.Command("git", "-C", "/nowhere", "remote", "get-url", "origin")))
	os.Stderr = old
	w.Close()
	se := <-done
	if rc != TimeoutCode || se != Who+": 시간 넘김 git -C /nowhere remote get-url 1\n" {
		t.Fatalf("표준 오류·종료 코드 그대로여야: rc=%d err=%q", rc, se)
	}
	b, err := os.ReadFile(ops)
	if err != nil {
		t.Fatalf("운영 기록이 없다: %v", err)
	}
	c := strings.Split(strings.TrimSuffix(string(b), "\n"), "\t")
	if len(c) != 8 || c[1] != Who || c[4] != "timeout" || c[5] != "-" || c[7] != "시간 넘김 git -C /nowhere remote get-url 1" {
		t.Fatalf("timeout 줄 하나: %q", c)
	}
}
