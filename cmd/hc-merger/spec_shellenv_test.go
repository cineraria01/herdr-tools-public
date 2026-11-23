//go:build !windows

package main

// merger-journal-rounds B(2026-11-23 — 한 검증 기계에서 추측 준비 훅이 `bun: command not found`(종료 127)로 「모름」): 추측 일꾼은 hc
// 코어가 물려준 PATH로 돌았고, 같은 기계에서 hc-lane이 띄우는 레인의 준비 단계는 그 기계의 zsh·~/.zshrc를 읽고 돈다. 추측의 준비 훅·
// 검증도 그 셸 환경으로 — 코어 PATH에는 없고 ~/.zshrc가 PATH에 넣는 도구를 준비 훅이 부르면 통과(검증 기록)해야 한다.

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"herdr-tools/internal/verified"
)

func TestSpecSetupUsesShellEnv(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh 없음 — ~/.zshrc 셸 환경 길은 zsh가 있는 기계 몫(없으면 지금 PATH 그대로)")
	}
	s := specFx(t) // HOME = s.dir
	tools := s.dir + "/shell-only"
	os.MkdirAll(tools, 0o777)
	os.WriteFile(tools+"/only-in-shell", []byte("#!/bin/sh\necho \"$PATH\" >\"$HOME/setup-path\"\n"), 0o777)
	// 코드 색인 도구는 셸 환경에 있어도 준비 훅 PATH에서 뺀다(지금 동작 그대로)
	cbm := s.dir + "/cbm"
	os.MkdirAll(cbm, 0o777)
	os.WriteFile(cbm+"/codebase-memory-mcp", []byte("#!/bin/sh\n"), 0o777)
	os.WriteFile(s.dir+"/.zshrc", []byte("export PATH=\""+tools+":"+cbm+":$PATH\"\n"), 0o666)
	os.WriteFile(s.seed+"/.claude/herdr-setup.sh", []byte("#!/bin/bash\nonly-in-shell\n"), 0o777)
	s.git(s.seed, "add", "-A")
	s.git(s.seed, "commit", "-qm", "setup hook")
	s.git(s.seed, "push", "-q", "origin", "main")
	s.sub(1000000001, "a", false)
	s.sub(1000000002, "b", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 {
		t.Fatalf("B 띄움: %v", d)
	}
	if rc := s.verifyOn(d[0]); rc != 0 {
		t.Fatalf("일꾼 종료 %d", rc)
	}
	if v := s.refs(verified.Prefix); len(v) != 1 {
		lg, _ := os.ReadFile(s.dir + "/v1.spec-wt/" + d[0][1][:8] + ".log")
		t.Fatalf("셸 환경의 도구로 준비 훅이 돌아 통과해야(모름이면 기록 없음): %v\n%s", v, lg)
	}
	p, _ := os.ReadFile(s.dir + "/setup-path")
	if strings.Contains(string(p), cbm) {
		t.Fatalf("준비 훅 PATH에서 코드 색인 도구 폴더를 빼야: %s", p)
	}
}
