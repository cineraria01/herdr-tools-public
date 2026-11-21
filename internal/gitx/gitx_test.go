package gitx

import (
	"os"
	"path/filepath"
	"testing"
)

// 가짜 git: 출력을 반쯤 내고 실패 — Out은 그 반쪽을 돌려주지 않아야 한다
func fakeGit(t *testing.T, script string) {
	t.Helper()
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "git"), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
}

func TestOutRejectsPartialOutput(t *testing.T) {
	fakeGit(t, `echo "refs/heads/half"; exit 128`)
	if o, rc := Out("", nil, "ls-remote", "origin"); rc != 128 || o != "" {
		t.Fatalf("실패면 빈 출력과 128: %q %d", o, rc)
	}
	if Probe(Out("", nil, "ls-remote", "origin")) != Unknown {
		t.Fatal("실패는 모름이어야")
	}
}

func TestOutAndProbe(t *testing.T) {
	fakeGit(t, `case $1 in has) printf 'a\nb\n\n' ;; empty) ;; verify) exit 1 ;; esac`)
	if o, rc := Out("", nil, "has"); rc != 0 || o != "a\nb" {
		t.Fatalf("끝 줄바꿈만 뺀다: %q %d", o, rc)
	}
	if s := Probe(Out("", nil, "has")); s != Yes {
		t.Fatalf("있음: %v", s)
	}
	if s := Probe(Out("", nil, "empty")); s != No {
		t.Fatalf("0과 빈 출력은 없음: %v", s)
	}
	o, rc := Out("", nil, "verify")
	if s := Probe(o, rc); s != Unknown {
		t.Fatalf("absent를 안 주면 1도 모름: %v", s)
	}
	if s := Probe(o, rc, 1); s != No {
		t.Fatalf("rev-parse -q --verify 종료 1은 없음: %v", s)
	}
	if Unknown.String() != "모름" || Yes.String() != "있음" || No.String() != "없음" {
		t.Fatal("State 글")
	}
}

func TestRunMissingGit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if rc := Run("", nil, nil, "status"); rc != 127 {
		t.Fatalf("git이 없으면 127: %d", rc)
	}
}
