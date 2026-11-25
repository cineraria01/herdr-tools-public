//go:build !windows

package selfexec

// 시험(ht-resident-reexec, 2026-11-25 — 배포가 「됐다」고 했는데 상주 몇이 옛 판으로 돌았다): 넘어가기 전에 새 판이 실제로 뜨는지
// (--version이 판 문자열을 내는지) 보고, 같은 판 문자열이면 다시 실행하지 않고 판 표시만 새 파일로, 손잡이 resident_reexec=0이면 넘어가지 않는다

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/knob"
	"herdr-tools/internal/resident"
	"herdr-tools/internal/version"
)

// 새 파일 깨짐(--version 실패) → 갈아타지 않고 계속 돈다, 같은 깨진 파일로 매 바퀴 띄우지 않는다(기록 한 줄), 다시 깔리면 그 판으로
func TestBrokenNewBinaryKeepsRunning(t *testing.T) {
	s, p, n, log := fake(t, nil)
	probes := 0
	s.probe = func(string) (string, error) { probes++; return "", errors.New("exec format error") }
	put(t, p, "v2-깨짐")
	for range 5 {
		s.Check()
	}
	if *n != 0 {
		t.Fatalf("깨진 새 판으로 exec했다(%d번)", *n)
	}
	if probes != 1 || strings.Count(log.String(), "새 판이 뜨지 않는다") != 1 {
		t.Fatalf("깨진 같은 파일을 %d번 띄워 봤다, 기록 %q", probes, log.String())
	}
	time.Sleep(10 * time.Millisecond)
	s.probe = func(string) (string, error) { return "tool (Go fixed)", nil }
	put(t, p, "v3-고친 판")
	s.Check()
	s.Check()
	if *n != 1 {
		t.Fatalf("고친 새 판으로 넘어가지 않았다(%d번)", *n)
	}
}

// 실제 파일 — 실행이 안 되는 새 판(판 문자열이 아님·실패 종료)은 probe가 거른다
func TestProbeReal(t *testing.T) {
	d := t.TempDir()
	for _, c := range []struct{ body, want string }{
		{"#!/bin/sh\necho 'tool (Go abc123)'\n", "tool (Go abc123)"},
		{"#!/bin/sh\necho 'tool (Go)'\n", "tool (Go)"},
		{"#!/bin/sh\nexit 3\n", ""},
		{"#!/bin/sh\necho hello\n", ""},
		{"\x7fELF반쯤", ""},
	} {
		p := filepath.Join(d, "tool")
		put(t, p, c.body)
		got, err := probe(p)
		if got != c.want || (c.want == "") != (err != nil) {
			t.Errorf("%q: %q, %v — 바람 %q", c.body, got, err, c.want)
		}
	}
}

// 실행 파일만 다시 쓰임(내용·판 문자열 같음) → 다시 실행하지 않고, 판 표시를 새 파일로(상주 판 점검이 옛 판으로 세지 않게)
func TestSameVersionRewriteNoExec(t *testing.T) {
	st := t.TempDir()
	t.Setenv("XDG_STATE_HOME", st)
	old := version.Commit
	version.Commit = "abc123def456"
	defer func() { version.Commit = old }()
	s, p, n, log := fake(t, nil)
	s.probe = func(string) (string, error) { return "hc-lane (Go abc123def456)", nil }
	time.Sleep(10 * time.Millisecond)
	put(t, p, "v1")
	s.Check()
	s.Check()
	if *n != 0 || !strings.Contains(log.String(), "같은 판") {
		t.Fatalf("같은 판 문자열인데 exec %d번, 기록 %q", *n, log.String())
	}
	fi, _ := os.Stat(p)
	h, _ := os.UserHomeDir()
	ms, _ := resident.Load(resident.DirFor(st, h))
	if m, ok := ms[os.Getpid()]; !ok || !m.Mtime.Equal(fi.ModTime()) || m.Path != p {
		t.Fatalf("판 표시가 새 파일로 안 바뀜: %+v (있음 %v)", m, ok)
	}
	s.Check()
	if *n != 0 {
		t.Fatalf("받아들인 뒤 또 exec했다(%d번)", *n)
	}
}

// 손잡이 resident_reexec=0 → 바뀌어도 넘어가지 않는다(끄기 전과 같음), 손잡이를 지우면 다음 바퀴에 넘어간다
func TestKnobOff(t *testing.T) {
	h := t.TempDir()
	t.Setenv("HOME", h)
	now := time.Now()
	if err := knob.Set(knob.Path(h), knob.Entry{Name: "resident_reexec", Value: "0", By: "시험", Why: "끔", At: now.Unix()}, true, now); err != nil {
		t.Fatal(err)
	}
	s, p, n, log := fake(t, nil)
	put(t, p, "v2")
	for range 4 {
		s.Check()
	}
	if *n != 0 || strings.Count(log.String(), "resident_reexec") != 1 {
		t.Fatalf("손잡이 끔인데 exec %d번, 기록 %q", *n, log.String())
	}
	if _, err := knob.Unset(knob.Path(h), "resident_reexec", true, now); err != nil {
		t.Fatal(err)
	}
	s.Check()
	if *n != 1 {
		t.Fatalf("손잡이를 지웠는데 넘어가지 않았다(%d번)", *n)
	}
}
