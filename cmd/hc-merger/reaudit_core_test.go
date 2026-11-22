package main

// gofix2(Go 전환 재감사 아스트라 1바퀴) — 고치기 전 코드에서 실패하던 시험

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// R-H07: hc get 응답은 정확히 한 JSON 객체이고, live면 이름·시작 시각이 있어야 「다른 실행」으로 확정한다.
// 칸이 빠졌거나 첫 객체 뒤에 잘린 글이 붙으면 해석 못 함(ok 거짓 — 보낼 함을 두고 다시)
func TestGofix2SameRunStrict(t *testing.T) {
	for _, c := range []struct {
		js       string
		same, ok bool
		why      string
	}{
		{`{"state":"live","name":"l","created_ms":123}`, true, true, "같은 실행"},
		{`{"state":"live","name":"other","created_ms":5}`, false, true, "다른 실행"},
		{`{"state":"live"}`, false, false, "live인데 이름·시작 시각 없음"},
		{`{"state":"live","name":"l"}`, false, false, "live인데 시작 시각 없음"},
		{`{"state":"live","name":"l","created_ms":123}{"x":"tru`, false, false, "첫 객체 뒤 잘린 글"},
		{`{"state":"live","name":"l","created_ms":123} junk`, false, false, "첫 객체 뒤 다른 글"},
		{`[1]`, false, false, "객체 아님"},
	} {
		same, ok := sameRun(c.js, "l", "123")
		if same != c.same || ok != c.ok {
			t.Errorf("%s: sameRun(%s) = %v,%v, 기대 %v,%v", c.why, c.js, same, ok, c.same, c.ok)
		}
	}
}

// fakeGitIn: PATH 맨 앞에 가짜 git — script가 처리하지 않으면 진짜 git으로
func fakeGitIn(t *testing.T, script string) {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "git"), []byte("#!/bin/sh\n"+script+"\nexec \""+real+"\" \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
}

// R-H03: ls-remote가 종료 0에 SHA 앞 12자만 준 줄은 「표식 없음」이 아니라 모름 — 일지를 두고 쉰다(proofload 거짓)
func TestGofix2ProofTruncLine(t *testing.T) { // 이름이 32자 넘는 대·소문자·숫자 섞인 낱말이면 추적 파일 비밀 꼴 검사에 걸린다
	f := newFx(t)
	m := f.merger()
	fakeGitIn(t, `[ "$1" = ls-remote ] && { printf 'abcdef012345\trefs/hc-landed/box-x\n'; exit 0; }`)
	if m.proofload([]string{"box-x"}) {
		t.Fatalf("잘린 ls-remote 줄을 받아들였다: lk2=%q", m.lk2)
	}
	fakeGitIn(t, `[ "$1" = ls-remote ] && { printf '%s\trefs/hc-landed/box-x\n' `+strings.Repeat("a", 40)+`; exit 0; }`)
	if !m.proofload([]string{"box-x"}) || refsha(m.lk2, "refs/hc-landed/box-x") != strings.Repeat("a", 40) {
		t.Fatalf("온전한 줄은 받아야: %q", m.lk2)
	}
	fakeGitIn(t, `[ "$1" = ls-remote ] && exit 2`)
	if !m.proofload([]string{"box-x"}) || m.lk2 != "" {
		t.Fatalf("--exit-code 종료 2(맞는 참조 없음)는 표식 없음: %q", m.lk2)
	}
	fakeGitIn(t, `[ "$1" = ls-remote ] && exit 0`)
	if m.proofload([]string{"box-x"}) {
		t.Fatalf("빈 출력·0은 모름(gofix3-gitref 계약): %q", m.lk2)
	}
}

// R-C07: fail/·out/ 목록을 못 읽으면 조용히 넘어가지 않는다 — 이 바퀴는 못 본 것이 있음(m.mer, --once 75)
func TestGofix2ListDirErrors(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root는 권한을 무시한다")
	}
	f := newFx(t)
	for _, sub := range []string{"fail", "out"} {
		m := f.merger()
		d := m.mj + "/" + sub
		if err := os.WriteFile(d+"/1-x", []byte("x\n"), 0o666); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(d, 0o100); err != nil { // 들어가기만 — 목록 읽기는 EACCES
			t.Fatal(err)
		}
		if sub == "fail" {
			m.fretry()
		} else {
			m.flush()
		}
		os.Chmod(d, 0o777)
		if !m.mer {
			t.Errorf("%s/ 목록 읽기 실패를 못 본 것으로 치지 않았다(m.mer 거짓)", sub)
		}
		if _, err := os.Stat(d + "/1-x"); err != nil {
			t.Errorf("%s/ 기록이 사라졌다: %v", sub, err)
		}
	}
}

// R-L01: 충돌 파일 이름은 git 따옴표·팔진 이스케이프 없이 실제 이름 그대로(다시 맞춤 지시문에 간다)
func TestGofix2ConflictPathKorean(t *testing.T) {
	f := newFx(t)
	name := "한글 문서.txt"
	os.WriteFile(f.seed+"/"+name, []byte("base\n"), 0o666)
	f.git(f.seed, "add", "-A")
	f.git(f.seed, "commit", "-qm", "ko base")
	f.git(f.seed, "push", "-q", "origin", "main")
	f.git(f.seed, "checkout", "-q", "-B", "l-k", "main")
	os.WriteFile(f.seed+"/"+name, []byte("lane\n"), 0o666)
	f.git(f.seed, "commit", "-qam", "l-k")
	lane := f.git(f.seed, "rev-parse", "HEAD")
	f.git(f.seed, "checkout", "-q", "main")
	os.WriteFile(f.seed+"/"+name, []byte("main\n"), 0o666)
	f.git(f.seed, "commit", "-qam", "main moves")
	f.git(f.seed, "push", "-q", "origin", "main")
	f.git(f.seed, "push", "-q", "origin", "l-k")
	f.git(f.clone, "fetch", "-q", "origin", "main", "l-k")
	m := f.merger()
	m.B = f.git(f.clone, "rev-parse", "origin/main")
	if got := m.conflicts(lane); got != name {
		t.Fatalf("충돌 파일 이름: %q, 기대 %q", got, name)
	}
}
