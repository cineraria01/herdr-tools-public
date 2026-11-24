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

// ⑤ 멈출 수 있는 stat을 피하고(-b) 내 프로세스만(-u) 묻는다. 상한은 그대로 60초
func TestOpenPathsArgs(t *testing.T) {
	var calls []call
	openPaths(recRun(&calls, result{rc: 1}))
	if len(calls) != 1 || calls[0].timeout != 60*time.Second || filepath.Base(calls[0].name) != "lsof" {
		t.Fatalf("부름 %+v", calls)
	}
	a := " " + strings.Join(calls[0].args, " ") + " "
	for _, w := range []string{" -n ", " -P ", " -w ", " -b ", " -u " + strconv.Itoa(os.Getuid()) + " ", " -F n "} {
		if !strings.Contains(a, w) {
			t.Errorf("인자에 %q 없음: %s", w, a)
		}
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
	asked, failed := sweepTmp(base, 3, false, fakeRun(-1, "", "", errors.New("lsof: 시간 초과")))
	if !asked || !failed {
		t.Fatalf("건너뜀으로 안 알림 asked=%v failed=%v", asked, failed)
	}
	if !exists(d) {
		t.Fatal("lsof를 모르는데 지움")
	}
	// 후보가 없으면 lsof를 안 묻는다(건너뜀 셈에 안 듦)
	var calls []call
	if asked, _ := sweepTmp(t.TempDir(), 3, false, recRun(&calls, result{})); asked || len(calls) != 0 {
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
