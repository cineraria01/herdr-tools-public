package main

// gofix4-hooks(2026-11-23 Go 전환 재감사 바퀴 3) 단위 시험 — 지적 ID는 재감사 보고서의 것.

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// cgCall: 훅 안의 exit(cgExit)를 종료 코드로 — 끝까지 가면 0
func cgCall(f func()) (code int) {
	defer func() {
		if r := recover(); r != nil {
			code = r.(cgExit).code
		}
	}()
	f()
	return 0
}

// R-M205: 산 주인의 임대 시각을 못 읽으면(그 사이 주인 파일이 치워짐) 모름 — 종료 1(설정 onFailure: block으로 막힘), 새 임대를 쓰지 않는다
func TestRefuseLiveUnreadableTime(t *testing.T) {
	R := t.TempDir()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range [][]string{{"init", "-q"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "i"}} {
		if out, err := exec.Command(git, append([]string{"-C", R}, a...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
	}
	ld := R + "/leases/x"
	if err := os.MkdirAll(ld, 0o755); err != nil {
		t.Fatal(err)
	}
	hookBudget(4 * time.Second)
	var stderr bytes.Buffer
	g := &cg{stderr: &stderr, root: R, git: git, psc: map[string]string{}, aok: map[string]bool{}}
	code := cgCall(func() { g.refuseLive(ld, "a.txt") })
	if code != 1 || !strings.Contains(stderr.String(), "시각을 읽지 못했다") {
		t.Errorf("종료 %d(want 1 — 모름), stderr=%q(want 「시각을 읽지 못했다」)", code, stderr.String())
	}
	if es, err := os.ReadDir(ld); err != nil || len(es) != 0 {
		t.Errorf("임대 폴더에 새 파일 %v(want 없음) %v", es, err)
	}
}
