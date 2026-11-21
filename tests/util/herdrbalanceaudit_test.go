package util_test

// 2026-11-21 Go 전환 전체 감사(go-final-audit base) 가운데 herdr-balance 지적의 계약 시험. 이름 앞 [ID]는 보고서 지적 ID.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestHerdrBalanceAudit(t *testing.T) { eachImpl(t, "herdr-balance", testHerdrBalanceAudit) }

func testHerdrBalanceAudit(t *testing.T, T string, e Env) {
	mkdirs(t, T+"/bin", T+"/home", T+"/py", T+"/fake")
	os.Symlink(filepath.Join(repo, "bin", "herdr-tools-config"), T+"/bin/herdr-tools-config")
	PY, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("python3 없음")
	}
	os.Symlink(PY, T+"/py/python3")
	HB := filepath.Join(repo, "bin", "herdr-balance")
	iv := implVar("herdr-balance")
	impl := e[iv]
	ex := filepath.Join(repo, "config.example")
	P := T + "/fake:" + T + "/py:" + T + "/bin:/usr/bin:/bin"
	env := func(kv ...string) Env {
		return Env{"HOME": T + "/home", "PATH": P, "HERDR_TOOLS_CONFIG": ex, "FAKE_HA_STATE": T + "/state.json",
			"FAKE_HA_LOG": T + "/ha.log", iv: impl}.With(kv...)
	}
	// 가짜 herdr-app 앞에 ls 조작 포장: FAKE_LS_EMPTY_AT=n이면 n번째 ls부터 {}(부분 목록), FAKE_LS_SLEEP=s면 ls가 s초 멈춤
	script(t, T+"/fake/herdr-app.py", T, fakeHerdrApp)
	script(t, T+"/fake/herdr-app", T, `#!/bin/bash
if [ "$1" = ls ]; then
  n=$(( $(cat "@T@/lsn" 2>/dev/null || echo 0) + 1 )); echo $n >"@T@/lsn"
  [ -n "$FAKE_LS_EMPTY_AT" ] && [ "$n" -ge "$FAKE_LS_EMPTY_AT" ] && { echo '{}'; exit 0; }
  [ -n "$FAKE_LS_SLEEP" ] && { echo $$ >"@T@/sleep.pid"; exec sleep "$FAKE_LS_SLEEP"; }
fi
if [ "$1 $2" = "do terminal.park" ] && [ -n "$FAKE_PARK_SLEEP" ]; then
  : >"@T@/park.started"; sleep "$FAKE_PARK_SLEEP"; "@T@/fake/herdr-app.py" "$@"; rc=$?; : >"@T@/park.done"; exit $rc
fi
exec "@T@/fake/herdr-app.py" "$@"
`)
	t.Cleanup(func() {
		if p, err := strconv.Atoi(strings.TrimSpace(read(T + "/sleep.pid"))); err == nil && p > 0 {
			syscall.Kill(p, syscall.SIGKILL)
		}
	})
	state := func(terms string) {
		write(t, T+"/state.json", `{"n": 0, "focused": "p0", "err_on": "", "parked": {}, "tabs": [{"id": "tab1", "terminals": [`+terms+`]}]}`+"\n")
		write(t, T+"/ha.log", "")
		os.Remove(T + "/lsn")
	}
	TAB1 := T + "/home/.local/state/herdr-balance/tab1.json"
	two := `{"terminal": "t0", "pane": "p0", "name": "lead", "app": "claude"}, {"terminal": "t1", "pane": "p1", "name": "lane-a", "app": "claude"}`

	// [B01] 이미 깔린 launchd가 「python3 …/herdr-balance」로 부른다(셸 판 시절 진입점) — 그래도 같은 결과로 돌아야 한다
	o1, _, rc1 := run2(T, env(), HB, "--print-remote")
	o2, se2, rc2 := run2(T, env(), PY, HB, "--print-remote")
	if rc1 != rc2 || o1 != o2 {
		t.Errorf("FAIL - [B01] python3로 불러도 같음: %d %q / %d %q %s", rc1, o1, rc2, o2, se2)
	}

	// [B12] 맞춤 중 목록이 부분({} — spaces 없음)이면 worker 없음으로 읽고 성공·상태 저장하지 않는다
	state(two)
	out, se, rc := run2(T, env("FAKE_LS_EMPTY_AT=2"), HB, "--main", "t0", "--drop", "t1")
	if rc == 0 || exists(TAB1) {
		t.Errorf("FAIL - [B12] 부분 목록: 실패·상태 파일 없음이어야 함 rc=%d tab=%v out=%q err=%q", rc, exists(TAB1), out, se)
	}
	os.RemoveAll(T + "/home/.local/state")

	// [B11] --watch가 herdr-app ls에 멈춰 있어도 TERM이면 곧 끝나고, 종료 코드는 셸 판과 같은 143
	state(two)
	w := exec.Command(HB, "--watch")
	w.Dir, w.Env = T, env("FAKE_LS_SLEEP=20").list()
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- w.Wait() }()
	t.Cleanup(func() {
		w.Process.Kill()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	})
	for i := 0; i < 50 && !exists(T+"/sleep.pid"); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	w.Process.Signal(syscall.SIGTERM)
	var werr error
	ended := false
	select {
	case werr = <-done:
		ended = true
		done <- werr
	case <-time.After(5 * time.Second):
	}
	wrc := -1
	var ee *exec.ExitError
	if errors.As(werr, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			wrc = 128 + int(ws.Signal())
		} else {
			wrc = ee.ExitCode()
		}
	} else if werr == nil && ended {
		wrc = 0
	}
	if !ended || wrc != 143 {
		t.Errorf("FAIL - [B11] ls에 멈춘 watch: TERM 5초 안 143 (끝남=%v rc=%d)", ended, wrc)
	}

	// [B11] worker를 떼는 중(park)에 TERM: 종료 코드는 두 판 143. Go 판은 그 맞춤을 마치고 끝난다(worker가 떼인 채 남지 않게 —
	// 셸 판은 바로 죽어 떼인 채 남는다: 알려진 차이, 셸 판은 곧 걷는다)
	state(two)
	os.Remove(T + "/park.started")
	os.Remove(T + "/park.done")
	m := exec.Command(HB, "--main", "t0")
	m.Dir, m.Env = T, env("FAKE_PARK_SLEEP=1").list()
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	mdone := make(chan error, 1)
	go func() { mdone <- m.Wait() }()
	t.Cleanup(func() { m.Process.Kill() })
	for i := 0; i < 100 && !exists(T+"/park.started"); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	m.Process.Signal(syscall.SIGTERM)
	mrc := -1
	select {
	case err := <-mdone:
		mrc = exitCode(err)
	case <-time.After(30 * time.Second):
	}
	for i := 0; i < 50 && !exists(T+"/park.done"); i++ { // 셸 판이 죽어도 띄운 park는 마저 돈다 — 그 뒤에 본다
		time.Sleep(100 * time.Millisecond)
	}
	parked := strings.Contains(read(T+"/state.json"), `"parked": {"t1"`)
	if mrc != 143 || (impl == "go" && parked) {
		t.Errorf("FAIL - [B11] park 중 TERM: 143·(Go 판) 다시 붙임 (rc=%d 떼인 채=%v)", mrc, parked)
	}

	// [M2] --watch 두 판 공통 잠금: 다른 판이 돌고 있으면 한 줄 찍고 75. 쥔 쪽은 INT면 130
	state(two)
	os.Remove(TAB1)
	other := map[string]string{"go": "bash", "bash": "go"}[impl]
	h := exec.Command(HB, "--watch")
	h.Dir, h.Env = T, env(iv+"="+other).list()
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	hdone := make(chan error, 1)
	go func() { hdone <- h.Wait() }()
	t.Cleanup(func() { h.Process.Kill() })
	for i := 0; i < 100 && !exists(TAB1); i++ { // 첫 회차(2초 뒤)가 상태를 적으면 잠금은 이미 쥐었다
		time.Sleep(100 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // 잠금이 없으면 끝나지 않는다 — 10초에 끊음
	defer cancel()
	r := exec.CommandContext(ctx, HB, "--watch")
	var rse strings.Builder
	r.Dir, r.Env, r.Stderr, r.WaitDelay = T, env().list(), &rse, 5*time.Second
	lrc := exitCode(r.Run())
	if lse := rse.String(); lrc != 75 || grepc(lse, "이미 돈다") != 1 {
		t.Errorf("FAIL - [M2] %s 판이 돌 때 %s 판 --watch: 75·한 줄 (rc=%d %q)", other, impl, lrc, lse)
	}
	h.Process.Signal(syscall.SIGINT)
	hrc := -1
	select {
	case err := <-hdone:
		hrc = exitCode(err)
	case <-time.After(10 * time.Second):
	}
	if hrc != 130 {
		t.Errorf("FAIL - [B11] %s 판 watch INT 종료 코드 130 (rc=%d)", other, hrc)
	}
}

// exitCode: Wait 결과를 셸 종료 코드로(신호면 128+신호)
func exitCode(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return ee.ExitCode()
	}
	if err != nil {
		return -1
	}
	return 0
}
