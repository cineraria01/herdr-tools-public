package main

// 검증 임시 폴더의 고아(2026-11-26 ht-test-orphan-leak — 시험 바이너리가 끊겨 죽으면 새 무리로 띄운 자식이 부모 1로 남는데, 고아 검사는
// 저장소 경로만 보아 임시 폴더 아래 것을 놓쳤고, 메모리 디스크를 -force로 떼자 그 위 실행 파일이 며칠씩 CPU를 태웠다):
// 조각이 이 검증의 임시 폴더(TMPDIR) 아래 경로로 떼어 띄운 프로세스는 끝날 때(끊겨도) 찍고 끈다.

import (
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// leakVerify: 나머지 조각(가짜 go)이 TMPDIR 아래 인자로 도우미를 떼어 띄우는 verify. hold면 조각이 끝나지 않고 기다린다
func leakVerify(t *testing.T, hold bool) (R, pidFile string) {
	R, B := tmp(t), t.TempDir()
	pidFile = B + "/leak.pid"
	fake := "#!/bin/sh\ncase \"$*\" in\n  'list ./...') printf 'herdr-tools/a\\n' ;;\n" +
		"  *'-skip ^TestCotenantGuard$'*) env VERIFY_HELPER=detach VERIFY_DIR=\"$TMPDIR\" VERIFY_PID='" + pidFile + "' VERIFY_ARG=\"${TMPDIR%/}/leak\" '" + os.Args[0] + "' || exit 9\n" +
		"    [ -z \"$VHOLD\" ] || exec sleep 300 ;;\nesac\nexit 0\n"
	if err := os.WriteFile(B+"/go", []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", B+":"+os.Getenv("PATH"))
	t.Setenv("HOME", B)
	t.Setenv("XDG_CACHE_HOME", B+"/.cache")
	t.Setenv("HERDR_VERIFY_NO_LOCK", "1")
	t.Setenv("HERDR_VERIFY_ALL", "")
	t.Setenv("HERDR_VERIFY_JOBS", "2")
	t.Setenv("VHOLD", map[bool]string{true: "1"}[hold])
	t.Cleanup(func() { // 고치기 전 판이면 남는다 — 이 시험이 띄운 그 pid만
		if b, err := os.ReadFile(pidFile); err == nil {
			if p, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && p > 1 && alive(p) {
				syscall.Kill(p, syscall.SIGKILL) //nolint:errcheck // 시험이 띄운 도우미
			}
		}
	})
	return R, pidFile
}

func TestReapVerifyTmp(t *testing.T) {
	R, pf := leakVerify(t, false)
	f, read := outFile(t)
	rc := verify(R, nil, make(chan os.Signal), f)
	p := readPid(t, pf)
	out := read()
	if rc != 1 || !strings.Contains(out, "고아 pid "+strconv.Itoa(p)) || !strings.Contains(out, "검증 임시 폴더") || !gone(p) {
		t.Errorf("임시 폴더 아래 떼어 띄운 것: 실패·목록·끔이어야 rc=%d alive=%v\n%s", rc, alive(p), out)
	}
}

// 끊겨도(TERM) 임시 폴더 아래 것을 끄고 나서 돌아온다(메모리 디스크를 떼기 전)
func TestReapVerifyTmpOnStop(t *testing.T) {
	R, pf := leakVerify(t, true)
	f, read := outFile(t)
	stop := make(chan os.Signal, 1)
	rcc := make(chan int, 1)
	go func() { rcc <- verify(R, nil, stop, f) }()
	p := readPid(t, pf)
	stop <- syscall.SIGTERM
	select {
	case rc := <-rcc:
		if rc != 143 || !gone(p) {
			t.Errorf("끊김: 143·임시 폴더 고아 끔이어야 rc=%d alive=%v\n%s", rc, alive(p), read())
		}
	case <-time.After(60 * time.Second):
		t.Fatal("끊김 뒤 돌아오지 않음")
	}
}
