package main

// sweep-lsof-bound: 느린 lsof에 묶이지 않기 — 좁힌 인자, 상한 넘김은 건너뜀(지움 0), 잇단 건너뜀은 사건 한 번

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// 부른 것을 적는 가짜 runner
type call struct {
	timeout time.Duration
	name    string
	args    []string
}

func recRun(calls *[]call, res result) runner {
	return func(to time.Duration, _ []string, name string, args ...string) result {
		*calls = append(*calls, call{to, name, args})
		return res
	}
}

// opq: 출력 꼴만 보는 시험용
func opq(r runner) map[string]bool { return openPaths(r, time.Minute) }

// ⑤ 멈출 수 있는 stat을 피하고(-b) 내 프로세스만(-u) 한 번에 묻는다. 상한은 받은 값(설정 sweep.lsof_secs).
// ht-sweep-lsof: 후보만 묻는 +D는 -b와 못 쓰고 열거 비용이 같아(실측) 쓰지 않는다
func TestOpenPathsArgs(t *testing.T) {
	var calls []call
	openPaths(recRun(&calls, result{rc: 1}), 7*time.Second)
	if len(calls) != 1 || calls[0].timeout != 7*time.Second || filepath.Base(calls[0].name) != "lsof" {
		t.Fatalf("부름 %+v", calls)
	}
	a := " " + strings.Join(calls[0].args, " ") + " "
	for _, w := range []string{" -n ", " -P ", " -w ", " -b ", " -u " + strconv.Itoa(os.Getuid()) + " ", " -F fn "} {
		if !strings.Contains(a, w) {
			t.Errorf("인자에 %q 없음: %s", w, a)
		}
	}
	if strings.Contains(a, "+D") {
		t.Errorf("+D는 -b와 못 쓴다: %s", a)
	}
}

// ht-sweep-lsof-linux: 리눅스 lsof는 -F n에 n 줄만 내고 f 줄은 요청해야 낸다(macOS는 늘 낸다). 온전함 검사(레코드마다 f·n)는
// 그대로 두고 f를 요청한다 — ① f 줄 없는 꼴은 지금처럼 nil ② 요청한 필드만 내는 가짜(리눅스 꼴)에서 경로 집합이 나온다
func TestOpenPathsLinuxFields(t *testing.T) {
	me := strconv.Itoa(os.Getpid())
	if m := opq(fakeRun(0, "p"+me+"\nn/\np1\nn/held\n", "", nil)); m != nil {
		t.Fatalf("f 줄 없는 출력을 받아들임 %v", m)
	}
	linux := func(_ time.Duration, _ []string, _ string, args ...string) result {
		if len(args) < 2 || args[len(args)-2] != "-F" {
			return result{rc: 1}
		}
		fields, out := args[len(args)-1], ""
		for _, p := range [][2]string{{me, "/"}, {"1", "/held"}} {
			out += "p" + p[0] + "\n"
			if strings.Contains(fields, "f") {
				out += "fcwd\n"
			}
			if strings.Contains(fields, "n") {
				out += "n" + p[1] + "\n"
			}
		}
		return result{out: out}
	}
	if m := opq(linux); m == nil || !m["/held"] {
		t.Fatalf("리눅스 꼴(요청한 필드만 냄)에서 경로 집합이 없음 %v", m)
	}
}

// ht-sweep-lsof: sweepTmp가 받은 상한을 lsof에 그대로 넘기고, 쓰는 중인 후보는 남기고 안 쓰는 것만 지운다(lsof는 한 번)
func TestSweepTmpLimit(t *testing.T) {
	base := realpath(t.TempDir())
	old := time.Now().Add(-5 * time.Hour)
	for _, n := range []string{"go-build1", "Test2"} {
		d := filepath.Join(base, n, "sub")
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		os.Chtimes(d, old, old)
		os.Chtimes(filepath.Dir(d), old, old)
	}
	var calls []call
	out := "p" + strconv.Itoa(os.Getpid()) + "\nfcwd\nn/\np1\nfcwd\nn" + base + "/go-build1/sub\n"
	asked, failed := sweepTmp(base, 3, 9*time.Second, false, recRun(&calls, result{out: out}))
	if !asked || failed || len(calls) != 1 || calls[0].timeout != 9*time.Second {
		t.Fatalf("asked=%v failed=%v 부름 %+v", asked, failed, calls)
	}
	if !exists(base+"/go-build1") || exists(base+"/Test2") {
		t.Fatal("쓰는 중은 남기고 안 쓰는 후보만 지워야")
	}
}

// 상한 손잡이: 양수면 그 초, 없음·0·음수·숫자 아님이면 60초(상한 없이 돌지도, 쓸기를 끄지도 않는다)
func TestLsofLimit(t *testing.T) {
	for v, want := range map[string]time.Duration{"180": 180 * time.Second, "1.5": 1500 * time.Millisecond, "0": time.Minute, "-3": time.Minute, "3m": time.Minute, "": time.Minute} {
		if got := lsofLimit(v, true); got != want {
			t.Errorf("%q → %v, 바람 %v", v, got, want)
		}
	}
	if lsofLimit("", false) != time.Minute {
		t.Error("설정을 못 읽으면 60초")
	}
}

// launchd PATH(/usr/sbin 없음)에서도 /usr/sbin·/sbin의 lsof를 찾는다 — 찾기만, 띄우지 않는다
func TestLsofPathFallback(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	want := "lsof"
	for _, p := range []string{"/usr/sbin/lsof", "/sbin/lsof"} {
		if executable(p) {
			want = p
			break
		}
	}
	if got := lsofPath(); got != want {
		t.Fatalf("lsofPath %q, 바람 %q", got, want)
	}
}

// ① lsof가 상한을 넘기면 열린 경로를 모름 — 오래된 후보도 하나도 안 지우고 건너뜀으로 알린다
func TestSweepTmpLsofTimeout(t *testing.T) {
	base := t.TempDir()
	d := filepath.Join(base, "go-build1", "b001")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-5 * time.Hour)
	for _, p := range []string{d, filepath.Dir(d)} {
		os.Chtimes(p, old, old)
	}
	asked, failed := sweepTmp(base, 3, time.Minute, false, fakeRun(-1, "", "", errors.New("lsof: 시간 초과")))
	if !asked || !failed {
		t.Fatalf("건너뜀으로 안 알림 asked=%v failed=%v", asked, failed)
	}
	if !exists(d) {
		t.Fatal("lsof를 모르는데 지움")
	}
	// 후보가 없으면 lsof를 안 묻는다(건너뜀 셈에 안 듦)
	var calls []call
	if asked, _ := sweepTmp(t.TempDir(), 3, time.Minute, false, recRun(&calls, result{})); asked || len(calls) != 0 {
		t.Fatalf("후보 없는데 물음 %v %+v", asked, calls)
	}
}

// ① 잇단 건너뜀 n바퀴에 사건 한 번, 더 이어져도 되풀이 없음, lsof가 되면 새로 센다. 보내기 실패는 다음 바퀴에 같은 키로
func TestTmpSkipAlert(t *testing.T) {
	state := filepath.Join(t.TempDir(), "hc-sweep", "tmp-skip")
	var calls []call
	ok := recRun(&calls, result{})
	reports := func() (n int, keys []string) {
		for _, c := range calls {
			if strings.Contains(strings.Join(c.args, " "), "hc-sweep-tmp-skip") {
				n++
				keys = append(keys, c.args[len(c.args)-1])
			}
		}
		return
	}
	tmpSkipAlert(state, true, 3, false, "", ok)
	tmpSkipAlert(state, true, 3, false, "", ok)
	if n, _ := reports(); n != 0 {
		t.Fatalf("문턱 전에 보냄 %d", n)
	}
	tmpSkipAlert(state, true, 3, true, "", ok) // dry-run은 세지도 보내지도 않는다
	if n, _ := reports(); n != 0 {
		t.Fatal("dry-run이 보냄")
	}
	for i := 0; i < 3; i++ {
		tmpSkipAlert(state, true, 3, false, "", ok)
	}
	n, keys := reports()
	if n != 1 || !strings.HasPrefix(keys[0], "hc-sweep-tmp-skip:") {
		t.Fatalf("한 번만 보내야 함: %d %v", n, keys)
	}
	tmpSkipAlert(state, false, 3, false, "", ok) // lsof가 됨 → 새로 센다
	if exists(state) {
		t.Fatal("lsof가 됐는데 표식이 남음")
	}
	calls = nil
	bad := recRun(&calls, result{rc: 1})
	for i := 0; i < 3; i++ {
		tmpSkipAlert(state, true, 3, false, "", bad)
	}
	tmpSkipAlert(state, true, 3, false, "", ok)
	n, keys = reports()
	if n != 2 || keys[0] != keys[1] {
		t.Fatalf("실패 뒤 같은 키로 다시 보내야 함: %d %v", n, keys)
	}
	tmpSkipAlert(state, true, 3, false, "", ok)
	if n, _ := reports(); n != 2 {
		t.Fatal("보낸 뒤 되풀이")
	}
	calls = nil
	os.Remove(state)
	for i := 0; i < 5; i++ {
		tmpSkipAlert(state, true, 0, false, "", ok) // 0이면 끔
	}
	if len(calls) != 0 || exists(state) {
		t.Fatal("0인데 셈·보냄")
	}
}
