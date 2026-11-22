//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"herdr-tools/internal/verified"
)

// merger_verifiers 줄 → 원격 표식(같은 목록이면 다시 안 올림), 줄을 지우면 표식을 지운다
func TestSpecVerifiersTick(t *testing.T) {
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
	confLines = []string{"repo app here " + d + "/r", "repo app box-1 /srv/app/", "merger_verifiers app box-1,nobox"}
	specVerifiersTick()
	msg := g("--git-dir="+d+"/o.git", "log", "-1", "--format=%B", verified.VerifiersRef)
	vs := verified.ParseVerifiers(msg)
	if len(vs) != 1 || vs[0].Machine != "box-1" || vs[0].Path != "/srv/app" {
		t.Fatalf("목록 = %v (%q)", vs, msg)
	}
	sha := g("--git-dir="+d+"/o.git", "rev-parse", verified.VerifiersRef)
	specVerifiersTick()
	if g("--git-dir="+d+"/o.git", "rev-parse", verified.VerifiersRef) != sha {
		t.Fatal("같은 목록이면 같은 SHA")
	}
	confLines = confLines[:2]
	specVerifiersTick()
	if exec.Command("git", "--git-dir="+d+"/o.git", "rev-parse", "-q", "--verify", verified.VerifiersRef).Run() == nil {
		t.Fatal("줄을 지우면 표식도 지운다")
	}
	if b, _ := os.ReadFile(d + "/spec-verifiers.tsv"); len(b) != 0 {
		t.Fatalf("관리 기록은 비어야: %q", b)
	}
}
