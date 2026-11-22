package main

// A11(2026-11-22 gofix-ops): macOS에서 settings.json을 바꿔 써도 원본 자리와 백업 모두 ACL·확장 속성이 남는다(셸 판 cp -p와 같다)

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	ct "herdr-tools/internal/clitest"
)

func xattrOf(p string) string {
	out, err := exec.Command("/usr/bin/xattr", "-p", "com.example.herdr-test", p).Output()
	if err != nil {
		return "!없음"
	}
	return strings.TrimSpace(string(out))
}

func aclOf(p string) string {
	out, _ := exec.Command("/bin/ls", "-le", p).Output()
	return string(out)
}

func TestMetadataKept(t *testing.T) {
	e := newMerge(t)
	ct.Put(t, e.S(), `{"model":"old"}`+"\n")
	if out, err := exec.Command("/usr/bin/xattr", "-w", "com.example.herdr-test", "t1", e.S()).CombinedOutput(); err != nil {
		t.Fatalf("xattr -w: %v %s", err, out)
	}
	if out, err := exec.Command("/bin/chmod", "+a", "everyone allow read", e.S()).CombinedOutput(); err != nil {
		t.Fatalf("chmod +a: %v %s", err, out)
	}
	if !strings.Contains(aclOf(e.S()), "everyone allow read") {
		t.Fatalf("시험 준비: ACL이 붙지 않았다: %s", aclOf(e.S()))
	}
	r := e.run(`{"values":{".model":"new"}}`, "", "--take", ".model")
	if r.RC != 0 || r.Out != "씀 .model" {
		t.Fatalf("씀이어야: rc=%d out=%q err=%s", r.RC, r.Out, r.Err)
	}
	baks, _ := filepath.Glob(e.ST() + "/bak/*/settings.json")
	if len(baks) != 1 {
		t.Fatalf("백업 하나: %v", baks)
	}
	for _, p := range []string{e.S(), baks[0]} {
		if xattrOf(p) != "t1" {
			t.Errorf("확장 속성을 잃었다: %s", p)
		}
		if !strings.Contains(aclOf(p), "everyone allow read") {
			t.Errorf("ACL을 잃었다: %s\n%s", p, aclOf(p))
		}
	}
}
