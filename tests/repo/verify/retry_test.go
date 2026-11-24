//go:build !windows

package main

// 흔들림 다시 돌리기(retry.go)의 계약: 판정(경합·빌드·시간 넘김·이름 없음은 안 돎), 상한·끔, 한 번만, 기록·요약·--flaky,
// 그리고 verify() 전체에서(가짜 go) 다른 조각이 다 끝난 뒤 혼자 다시 돎·홀로 단계도 한 번 다시.

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestFailedTests(t *testing.T) {
	out := "=== RUN TestA\n--- FAIL: TestA (1.0s)\n    --- FAIL: TestA/s1 (0.5s)\n--- FAIL: TestB (0.1s)\n--- FAIL: TestA (1.0s)\nFAIL\n"
	if got := failedTests(out); !slices.Equal(got, []string{"TestA/s1", "TestB"}) {
		t.Errorf("잎만·처음 차례: %q", got)
	}
}

func TestNoRetry(t *testing.T) {
	fail := "--- FAIL: TestX (0.1s)\nFAIL\n"
	for out, want := range map[string]string{
		fail:                                         "",
		fail + "WARNING: DATA RACE\n":                "경합",
		"FAIL\therdr-tools/x [build failed]\n":       "빌드 실패",
		"FAIL\therdr-tools/x [setup failed]\n":       "[setup failed]",
		fail + "panic: test timed out after 40m0s\n": "시간 넘김",
		"  고아 pid 1 (cwd /x): sleep\n실패 고아를 남김\n":    "이름이 없음",
	} {
		if got := noRetry(out); (want == "") != (got == "") || !strings.Contains(got, want) {
			t.Errorf("noRetry(%q) = %q, %q가 들어야", out, got, want)
		}
	}
}

func flakyHome(t *testing.T) string {
	p := filepath.Join(t.TempDir(), "verify-flaky.log")
	old := flakyFile
	flakyFile = func() string { return p }
	t.Cleanup(func() { flakyFile = old })
	return p
}

func TestRetryPhase(t *testing.T) {
	flakyHome(t)
	fl := retryCand{i: 2, label: "[3/5] go test x", out: "--- FAIL: TestX (0.1s)\n"}
	race := retryCand{i: 4, label: "[5/5] go test y", out: "--- FAIL: TestY (0.1s)\nWARNING: DATA RACE\n"}
	var ran []int
	run := func(rc int) func(retryCand) (int, string) {
		return func(c retryCand) (int, string) { ran = append(ran, c.i); return rc, "" }
	}
	type tc struct {
		name  string
		cs    []retryCand
		limit int
		rc    int
		fail  bool
		ran   []int
		flaky []int
		say   string
	}
	for _, c := range []tc{
		{"흔들림", []retryCand{fl}, 3, 0, false, []int{2}, []int{2}, "흔들림: [3/5] go test x — TestX · 혼자 다시 돌려 통과"},
		{"두 번 실패", []retryCand{fl}, 3, 1, true, []int{2}, nil, "두 번 실패: [3/5] go test x — TestX"},
		{"경합이 하나라도 있으면 아무것도 안 돎", []retryCand{fl, race}, 3, 0, true, nil, nil, "다시 안 돎: [5/5] go test y — 경합"},
		{"상한 넘음", []retryCand{fl, fl}, 1, 0, true, nil, nil, "상한 1를 넘음"},
		{"끔", []retryCand{fl}, 0, 0, true, nil, nil, ""},
		{"끊김", []retryCand{fl}, 3, 143, true, []int{2}, nil, ""},
	} {
		ran = nil
		f, err := os.CreateTemp(t.TempDir(), "out")
		if err != nil {
			t.Fatal(err)
		}
		fail, flaky, rc := retryPhase(t.TempDir(), c.cs, c.limit, run(c.rc), f)
		b, _ := os.ReadFile(f.Name())
		f.Close()
		if fail != c.fail || !slices.Equal(ran, c.ran) || !slices.Equal(flaky, c.flaky) || (c.rc >= 128) != (rc == c.rc && rc != 0) ||
			!strings.Contains(string(b), c.say) {
			t.Errorf("%s: fail=%v ran=%v flaky=%v rc=%d\n%s", c.name, fail, ran, flaky, rc, b)
		}
		if c.limit == 0 && len(b) > 0 {
			t.Errorf("끔이면 아무것도 안 찍는다: %s", b)
		}
	}
}

func TestFlakyLog(t *testing.T) {
	p := flakyHome(t)
	R := t.TempDir()
	if err := flakyRecord(R, "[1/2] go test a", []string{"TestA", "TestB/s1"}); err != nil {
		t.Fatal(err)
	}
	if err := flakyRecord(R, "[2/2] go test b", []string{"TestA"}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().AddDate(0, 0, -flakyDays-1).Format(time.RFC3339)
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(old + "\t?\t[1/1] x\tTestOld\tload ?\n")
	f.Close()
	b, _ := os.ReadFile(p)
	if l := strings.Split(strings.TrimSpace(string(b)), "\n"); len(l) != 3 || len(strings.Split(l[0], "\t")) != 5 || !strings.Contains(l[0], "\t?\t") {
		t.Errorf("기록 줄: 시각·트리(저장소 밖이면 ?)·조각·시험·부하\n%s", b)
	}
	o, _ := os.CreateTemp(t.TempDir(), "out")
	flakySummary(o)
	if rc := flakyReport(o); rc != 0 {
		t.Errorf("--flaky 종료 %d", rc)
	}
	s, _ := os.ReadFile(o.Name())
	o.Close()
	out := string(s)
	if !strings.Contains(out, "최근 7일 흔들림: TestA 2번, TestB/s1 1번 (") || strings.Contains(strings.SplitN(out, "\n", 2)[0], "TestOld") {
		t.Errorf("요약: 최근 기간만·많은 것부터\n%s", out)
	}
	if !strings.Contains(out, "   2  TestA") || !strings.Contains(out, "   1  TestOld") {
		t.Errorf("--flaky: 기록 전부 시험별\n%s", out)
	}
	flakyFile = func() string { return filepath.Join(t.TempDir(), "none") }
	o, _ = os.CreateTemp(t.TempDir(), "out")
	flakySummary(o)
	s, _ = os.ReadFile(o.Name())
	o.Close()
	if len(s) != 0 {
		t.Errorf("기록이 없으면 요약 줄 없음: %s", s)
	}
}

// fakeVerify: 가짜 go로 verify() — 조각 셋(나머지 a, tests/util, cmd/herdr-tools-publish)과 홀로 단계. FAKE_FAIL(정규식)에 맞는
// go test는 FAKE_MODE대로 실패한다: flaky 첫 번만 · hard 늘 · race 늘·DATA RACE. FAKE_SLOW에 맞는 것은 1초 돈다(겹침을 만든다).
// 두 번째 이후 실행은 그때 함께 돌던 가짜 go 수를 B/again에 적는다. 돌려주는 것: 종료 코드, 출력, 가짜 go 호출 기록, again
func fakeVerify(t *testing.T, env map[string]string, args ...string) (int, string, string, string) {
	t.Helper()
	R, B := tmp(t), t.TempDir()
	fake := `#!/bin/sh
echo "$*" >>"$B/log"
case "$*" in
  'list ./...') printf 'herdr-tools/a\nherdr-tools/tests/util\nherdr-tools/cmd/herdr-tools-publish\n'; exit 0 ;;
  vet*|*' ./tests/repo/') exit 0 ;;
esac
mkdir -p "$B/run" && : >"$B/run/$$"
n=$(ls "$B/run" | wc -l | tr -d ' ')
if [ -n "${FAKE_SLOW:-}" ] && printf '%s' "$*" | grep -Eq "$FAKE_SLOW"; then sleep 1; fi
rc=0
if [ -n "${FAKE_FAIL:-}" ] && printf '%s' "$*" | grep -Eq "$FAKE_FAIL"; then
  k=$(printf '%s' "$*" | cksum | cut -d' ' -f1)
  c=$(cat "$B/cnt.$k" 2>/dev/null || echo 0); c=$((c+1)); echo "$c" >"$B/cnt.$k"
  [ "$c" -gt 1 ] && echo "$n $*" >>"$B/again"
  if [ "$FAKE_MODE" != flaky ] || [ "$c" = 1 ]; then
    [ "$FAKE_MODE" = race ] && echo 'WARNING: DATA RACE'
    echo '--- FAIL: TestShaky (0.10s)'; echo FAIL; rc=1
  fi
fi
rm "$B/run/$$"
exit $rc
`
	if err := os.WriteFile(B+"/go", []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", B+":"+os.Getenv("PATH"))
	t.Setenv("B", B)
	t.Setenv("HOME", B) // 흔들림 기록·검증 잠금이 이 시험의 폴더에
	t.Setenv("XDG_CACHE_HOME", B+"/.cache")
	for _, k := range []string{"HERDR_VERIFY_ALL", "HERDR_VERIFY_RETRY", "FAKE_FAIL", "FAKE_MODE", "FAKE_SLOW"} {
		t.Setenv(k, "")
	}
	t.Setenv("HERDR_VERIFY_NO_LOCK", "1")
	t.Setenv("HERDR_VERIFY_JOBS", "6")
	for k, v := range env {
		t.Setenv(k, v)
	}
	p := filepath.Join(B, "flaky.log")
	old := flakyFile
	flakyFile = func() string { return p }
	t.Cleanup(func() { flakyFile = old })
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rc := verify(R, args, make(chan os.Signal), f)
	o, _ := os.ReadFile(f.Name())
	l, _ := os.ReadFile(B + "/log")
	a, _ := os.ReadFile(B + "/again")
	rec, _ := os.ReadFile(p)
	return rc, string(o) + "\n#flaky.log\n" + string(rec), string(l), string(a)
}

// ①⑥ 한 조각이 첫 번 실패·다시 통과 → 전체 통과, 흔들림 줄·기록, 다시 돌 때 함께 도는 것은 자기 하나(느린 조각이 끝난 뒤)
func TestRetryFlakyShardPasses(t *testing.T) {
	rc, out, _, again := fakeVerify(t, map[string]string{"FAKE_FAIL": "tests/util$", "FAKE_MODE": "flaky", "FAKE_SLOW": "herdr-tools-publish$"})
	if rc != 0 || !strings.Contains(out, "흔들림: [") || !strings.Contains(out, "herdr-tools/tests/util — TestShaky · 혼자 다시 돌려 통과") ||
		!strings.Contains(out, "\tTestShaky\tload ") || !strings.Contains(out, "최근 7일 흔들림: TestShaky 1번") {
		t.Errorf("흔들림: 통과·줄·기록·요약이어야 rc=%d\n%s", rc, out)
	}
	if strings.Count(again, "\n") != 1 || !strings.HasPrefix(again, "1 ") {
		t.Errorf("다시 돌기는 한 번, 혼자(함께 도는 가짜 go 1): %q", again)
	}
}

// ② 두 번 다 실패 → 실패, 「두 번 실패」, 기록 없음
func TestRetryHardFails(t *testing.T) {
	rc, out, _, again := fakeVerify(t, map[string]string{"FAKE_FAIL": "tests/util$", "FAKE_MODE": "hard"})
	if rc != 1 || !strings.Contains(out, "두 번 실패: [") || strings.Contains(out, "흔들림: [") || strings.Contains(out, "TestShaky\tload") ||
		strings.Count(again, "\n") != 1 {
		t.Errorf("두 번 실패: 1·한 번만 다시·기록 없음이어야 rc=%d again=%q\n%s", rc, again, out)
	}
}

// ③ DATA RACE → 다시 안 돎
func TestRetryRaceNotRetried(t *testing.T) {
	rc, out, _, again := fakeVerify(t, map[string]string{"FAKE_FAIL": "tests/util$", "FAKE_MODE": "race"})
	if rc != 1 || again != "" || !strings.Contains(out, "다시 안 돎: [") || !strings.Contains(out, "경합") {
		t.Errorf("경합: 다시 안 돎이어야 rc=%d again=%q\n%s", rc, again, out)
	}
}

// ④ 실패한 조각이 상한을 넘음 → 다시 안 돎
func TestRetryOverLimit(t *testing.T) {
	rc, out, _, again := fakeVerify(t, map[string]string{"FAKE_FAIL": "(tests/util|herdr-tools-publish)$", "FAKE_MODE": "flaky", "HERDR_VERIFY_RETRY": "1"})
	if rc != 1 || again != "" || !strings.Contains(out, "실패 2개가 상한 1를 넘음") {
		t.Errorf("상한 넘음: 다시 안 돎이어야 rc=%d again=%q\n%s", rc, again, out)
	}
}

// ⑤ 끔(HERDR_VERIFY_RETRY=0) → 지금처럼 실패, 다시 안 돎
func TestRetryOff(t *testing.T) {
	rc, out, _, again := fakeVerify(t, map[string]string{"FAKE_FAIL": "tests/util$", "FAKE_MODE": "flaky", "HERDR_VERIFY_RETRY": "0"})
	if rc != 1 || again != "" || strings.Contains(out, "다시") {
		t.Errorf("끔: 실패·다시 안 돎이어야 rc=%d again=%q\n%s", rc, again, out)
	}
}

// ⑧ 홀로 단계(TestCotenantGuard) 실패도 한 번 다시 — 조각이 다 끝난 뒤
func TestRetryCotenantStep(t *testing.T) {
	rc, out, calls, again := fakeVerify(t, map[string]string{"FAKE_FAIL": "run \\^TestCotenantGuard", "FAKE_MODE": "flaky"})
	if rc != 0 || !strings.Contains(out, "흔들림: ") || !strings.Contains(out, "TestShaky · 혼자 다시 돌려 통과") || strings.Count(again, "\n") != 1 {
		t.Errorf("홀로 단계 흔들림: 통과·한 번 다시이어야 rc=%d again=%q\n%s", rc, again, out)
	}
	l := strings.Split(strings.TrimSpace(calls), "\n")
	if len(l) == 0 || !strings.Contains(l[len(l)-1], "TestCotenantGuard") {
		t.Errorf("홀로 단계의 다시 돌기는 조각이 다 끝난 뒤(마지막 호출):\n%s", calls)
	}
}

// 실패 없는 실행: 다시 돌기·흔들림 줄 없음(시간이 늘지 않는다)
func TestRetryNothingWhenGreen(t *testing.T) {
	rc, out, _, again := fakeVerify(t, nil)
	if rc != 0 || again != "" || strings.Contains(out, "다시") || strings.Contains(out, "흔들림") {
		t.Errorf("실패 없음: 다시 안 돎이어야 rc=%d\n%s", rc, out)
	}
}

// verify --flaky: 기록을 시험별로 보이고 끝(검증은 안 돈다)
func TestVerifyFlakyArg(t *testing.T) {
	rc, out, calls, _ := fakeVerify(t, nil, "--flaky")
	if rc != 0 || calls != "" || !strings.Contains(out, "흔들림 기록 없음") {
		t.Errorf("--flaky: 0·go 안 부름·기록 없음 줄이어야 rc=%d calls=%q\n%s", rc, calls, out)
	}
}
