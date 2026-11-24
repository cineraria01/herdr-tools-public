package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 착지를 시작한 쪽(fill-auto-submit): 같은 실행 ID라도 대기열(HC_LANE_LAND_BY=fill)과 에이전트는 서로의 시작 표시를 잇지 않는다
func TestLandByOwner(t *testing.T) {
	for _, c := range []struct {
		fifth string
		fill  bool
		want  int
	}{
		{"", false, 0},     // 에이전트가 시작한 것을 에이전트가 잇는다(지금 그대로)
		{"fill", true, 0},  // 대기열이 시작한 것을 대기열이 잇는다
		{"fill", false, 1}, // 대기열이 맡음 — 에이전트는 아무것도 안 한다
		{"", true, 2},      // 에이전트가 시작함 — 대기열은 손대지 않는다
	} {
		if got := lvOwner(c.fifth, c.fill); got != c.want {
			t.Fatalf("lvOwner(%q, %v) = %d, 기대 %d", c.fifth, c.fill, got, c.want)
		}
	}
	t.Setenv("HC_LANE_LAND_BY", "fill")
	if lvByTag() != "\tfill" {
		t.Fatalf("대기열 시작 표시 칸: %q", lvByTag())
	}
	t.Setenv("HC_LANE_LAND_BY", "")
	if lvByTag() != "" {
		t.Fatalf("에이전트 시작 표시는 넷째 칸까지(지금 꼴): %q", lvByTag())
	}
}

// hc-lane verdict: 대기열이 시작한 착지(다섯째 칸 fill)를 에이전트가 열면 「아무것도 하지 않는다」, 에이전트 것은 지금처럼 「같은 착지 줄을 다시」
func TestVerdictLandingByFill(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HC_FILL_VERDICTS", d)
	t.Setenv("HC_LANE_JOB", "t1@5")
	t.Setenv("HC_LANE_LAND_BY", "")
	ld := filepath.Join(d, "lane")
	if err := os.MkdirAll(ld, 0o777); err != nil {
		t.Fatal(err)
	}
	body := "aid\ta1\nrun\tt1@5\nsha\t" + strings.Repeat("ab", 20) + "\nverdict\tpass\nreport\t-\nround\t1\n"
	if err := os.WriteFile(filepath.Join(ld, "a1.md"), []byte(body), 0o666); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ ack, by, want string }{
		{"landing\tT\tt1@5\tpid\tfill\n", "", "대기열(hc-fill)이 레인 대신 제출하고 있다 — 아무것도 하지 않는다"},
		{"landing\tT\tt1@5\tpid\n", "", "착지를 시작했다 — 같은 착지 줄을 다시"},
		{"landing\tT\tt1@5\tpid\tfill\n", "fill", "착지를 시작했다 — 같은 착지 줄을 다시"},
	} {
		if err := os.WriteFile(filepath.Join(ld, "a1.ack"), []byte(c.ack), 0o666); err != nil {
			t.Fatal(err)
		}
		t.Setenv("HC_LANE_LAND_BY", c.by)
		out, rc := lbCapture(t, func() int { return verdictCmd([]string{"--name", "lane", "a1"}) })
		if rc != 0 || !strings.Contains(out, c.want) {
			t.Fatalf("표시 %q(부른 쪽 %q): 종료 %d 출력 %q, 기대 %q", c.ack, c.by, rc, out, c.want)
		}
	}
	if f := ackFifth(filepath.Join(ld, "없음.ack")); f != "" {
		t.Fatalf("없는 표시의 다섯째 칸: %q", f)
	}
}

func lbCapture(t *testing.T, f func() int) (string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	rc := f()
	os.Stdout = old
	w.Close()
	b, _ := io.ReadAll(r)
	return string(b), rc
}
