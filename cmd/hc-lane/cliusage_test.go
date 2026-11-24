package main

// cli-usage-version(2026-11-24): --version에 커밋 · 인자 오류는 운영 기록에 사용법 줄(usage — 종료 코드·표준 오류 글은 그대로) ·
// events·wait의 레인 이름 거름 --lane · 모르는 인자에 가장 가까운 인자 권함

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type laneRun struct {
	out, err string
	rc       int
}

// runBin: 바이너리를 env(+이 프로세스 환경)로 돌린다. 표준 출력은 관(/dev/null이 아니게 — wait가 거절한다)
func runBin(t *testing.T, bin string, env []string, args ...string) laneRun {
	t.Helper()
	c := exec.Command(bin, args...)
	c.Env = append(os.Environ(), env...)
	var o, e strings.Builder
	c.Stdout, c.Stderr = &o, &e
	err := c.Run()
	rc := 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("%q: %v", args, err)
		}
		rc = ee.ExitCode()
	}
	return laneRun{o.String(), e.String(), rc}
}

// cuEnv: 상태·기록·설정을 임시 폴더에. ops는 운영 기록 파일
func cuEnv(t *testing.T) (d, ops string, env []string) {
	d = t.TempDir()
	ops = d + "/ops.tsv"
	if err := os.MkdirAll(d+"/state/hc-lane", 0o755); err != nil {
		t.Fatal(err)
	}
	return d, ops, []string{"HOME=" + d, "XDG_STATE_HOME=" + d + "/state", "HERDR_TOOLS_CONFIG=" + d + "/conf",
		"HERDR_TOOLS_OPLOG=" + ops, "HC_TERMINAL_ID=", "HC_LANE_WAIT_TICK=0"}
}

func opKinds(t *testing.T, ops string) []string {
	t.Helper()
	b, err := os.ReadFile(ops)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var ks []string
	for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		c := strings.Split(l, "\t")
		if len(c) != 8 {
			t.Fatalf("8칸이어야: %q", l)
		}
		ks = append(ks, c[4]+" "+c[5])
	}
	return ks
}

// ① --version은 다른 도구처럼 「hc-lane (Go <커밋>)」
func TestCliVersionCommit(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "hc-lane")
	if out, err := exec.Command("go", "build", "-ldflags", "-X herdr-tools/internal/version.Commit=abcdef012345", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("빌드: %v %s", err, out)
	}
	if r := runBin(t, bin, nil, "--version"); r.rc != 0 || r.out != "hc-lane (Go abcdef012345)\n" {
		t.Fatalf("--version: %+v", r)
	}
}

// ② 모르는 인자 — 종료 코드·표준 오류 글은 그대로, 운영 기록은 die가 아니라 사용법 줄(usage) 하나.
// parseArgs 길(land)은 옛 판에서 기록이 아예 없었고, die 길(events·stats·logs)은 die 1로 남았다
func TestCliArgErrorIsUsageInOpLog(t *testing.T) {
	bin := buildLane(t, t.TempDir())
	for _, c := range []struct {
		args []string
		err  string
	}{
		{[]string{"events", "--bogus"}, "hc-lane: 알 수 없는 인자: --bogus\n"},
		{[]string{"land", "main", "--bogus"}, "hc-lane: 알 수 없는 인자: --bogus\n"},
		{[]string{"events", "--since"}, ""}, // 값 없음도 인자 오류(글은 지금 그대로 — 아래에서 rc·기록만 본다)
	} {
		_, ops, env := cuEnv(t)
		r := runBin(t, bin, env, c.args...)
		// 첫 줄 그대로(agent-skill-guides가 가까운 인자가 없을 때 「다음:」 둘째 줄을 더했다)
		if r.rc != 1 || (c.err != "" && !strings.HasPrefix(r.err, c.err)) {
			t.Errorf("%q: 종료 1·글 그대로여야: %+v", c.args, r)
		}
		if ks := opKinds(t, ops); len(ks) != 1 || ks[0] != "usage 1" {
			t.Errorf("%q: 운영 기록은 usage 1 한 줄이어야: %q", c.args, ks)
		}
	}
}

// ⑥ 모르는 인자가 다른 인자와 한 끗 차이(또는 흔히 잘못 쓰는 짝)면 둘째 줄에 권함 — 첫 줄·종료 코드는 그대로
func TestCliUnknownArgSuggests(t *testing.T) {
	bin := buildLane(t, t.TempDir())
	_, _, env := cuEnv(t)
	r := runBin(t, bin, env, "events", "--name", "x")
	if r.rc != 1 || !strings.HasPrefix(r.err, "hc-lane: 알 수 없는 인자: --name\n") || !strings.Contains(r.err, "--lane") {
		t.Fatalf("--name → --lane 권함: %+v", r)
	}
	r = runBin(t, bin, env, "events", "--bogus")
	// 가까운 인자가 없으면 「대신 …?」 권함 없이, 안내를 가리키는 「다음:」 한 줄만(agent-skill-guides)
	if strings.Count(r.err, "\n") != 2 || strings.Contains(r.err, " 대신 ") || !strings.Contains(r.err, "\nhc-lane: 다음: ") {
		t.Fatalf("가까운 인자가 없으면 권함 줄 없이 「다음:」만: %q", r.err)
	}
}

func cuEvent(at time.Time, lane, st string) string {
	return strings.Join([]string{at.UTC().Format("2006-01-02T15:04:05Z"), "here", "t1-" + lane, lane, st, "-", "글 " + lane, "-"}, "\t") + "\n"
}

// ④ events --lane A(여럿 가능) → 그 레인 사건만
func TestCliEventsLane(t *testing.T) {
	bin := buildLane(t, t.TempDir())
	d, _, env := cuEnv(t)
	now := time.Now()
	ev := cuEvent(now.Add(-3*time.Second), "lane-a", "중간") + cuEvent(now.Add(-2*time.Second), "lane-b", "완료") +
		cuEvent(now.Add(-time.Second), "lane-c", "착지")
	if err := os.WriteFile(d+"/state/hc-lane/events.tsv", []byte(ev), 0o644); err != nil {
		t.Fatal(err)
	}
	r := runBin(t, bin, env, "events", "--lane", "lane-a")
	if r.rc != 0 || strings.Count(r.out, "\n") != 1 || !strings.Contains(r.out, "\tlane-a\t") {
		t.Fatalf("--lane lane-a: %+v", r)
	}
	r = runBin(t, bin, env, "events", "--lane", "lane-a", "--lane", "lane-c")
	if r.rc != 0 || strings.Count(r.out, "\n") != 2 || strings.Contains(r.out, "lane-b") {
		t.Fatalf("--lane 둘: %+v", r)
	}
	if r = runBin(t, bin, env, "events"); r.rc != 0 || strings.Count(r.out, "\n") != 3 {
		t.Fatalf("거름 없으면 셋 다: %+v", r)
	}
}

// ⑤ wait --lane A → A의 새 사건에만 깬다. 거른 사건은 커서에 적지 않는다(거름 없는 wait가 그대로 받는다)
func TestCliWaitLane(t *testing.T) {
	bin := buildLane(t, t.TempDir())
	d, _, env := cuEnv(t)
	evp := d + "/state/hc-lane/events.tsv"
	if err := os.WriteFile(evp, []byte(cuEvent(time.Now().Add(-time.Second), "lane-b", "완료")), 0o644); err != nil {
		t.Fatal(err)
	}
	// --state 완료(멈춤 판정 없음 — hc를 부르지 않는다) · --timeout 0(한 바퀴)
	w := []string{"wait", "--state", "완료", "--timeout", "0"}
	if r := runBin(t, bin, env, append(w, "--lane", "lane-a")...); r.rc != 75 || r.out != "" {
		t.Fatalf("B 사건에는 깨지 않는다(75): %+v", r)
	}
	f, err := os.OpenFile(evp, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(cuEvent(time.Now(), "lane-a", "완료"))
	f.Close()
	if r := runBin(t, bin, env, append(w, "--lane", "lane-a")...); r.rc != 0 || !strings.Contains(r.out, "\tlane-a\t") || strings.Contains(r.out, "lane-b") {
		t.Fatalf("A 사건에 깬다: %+v", r)
	}
	if r := runBin(t, bin, env, w...); r.rc != 0 || !strings.Contains(r.out, "\tlane-b\t") {
		t.Fatalf("거른 B 사건은 커서에 안 적혀 거름 없는 wait가 받는다: %+v", r)
	}
}
