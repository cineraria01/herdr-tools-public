//go:build !windows

package main

import (
	"os/exec"
	"strings"
	"testing"

	"herdr-tools/internal/verified"
)

// preverify(2026-11-24): 「merger_preverify <프로젝트> off」 → 목록 커밋에 #preverify=off 한 줄(목록은 그대로), 줄을 지우면 다시 켬
func TestSpecVerifiersPreOff(t *testing.T) {
	d := t.TempDir()
	for k, v := range map[string]string{"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null"} {
		t.Setenv(k, v)
	}
	g := func(args ...string) string {
		o, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, o)
		}
		return strings.TrimSpace(string(o))
	}
	g("init", "-q", "--bare", d+"/o.git")
	g("clone", "-q", d+"/o.git", d+"/r")
	DATA, LOCAL, dry = d, "here", false
	defer func() { DATA, LOCAL = "", "" }()
	base := []string{"repo app here " + d + "/r", "repo app box-1 /srv/app/", "merger_verifiers app box-1"}
	msg := func() string { return g("--git-dir="+d+"/o.git", "log", "-1", "--format=%B", verified.VerifiersRef) }
	confLines = append(base, "merger_preverify app off")
	specVerifiersTick()
	if m := msg(); !verified.PreOff(m) || len(verified.ParseVerifiers(m)) != 1 {
		t.Fatalf("끔: %q", m)
	}
	confLines = base
	specVerifiersTick()
	if m := msg(); verified.PreOff(m) || len(verified.ParseVerifiers(m)) != 1 {
		t.Fatalf("줄을 지우면 켬: %q", m)
	}
}
