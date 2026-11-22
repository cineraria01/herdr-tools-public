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

// aclOf: ls -led의 ACL 항목(「N: 」 번호를 뺀 줄들). 주체는 macOS가 이름이나 UUID로 보이니 글자를 박지 않고 붙인 직후 값과 견준다(R-29)
func aclOf(p string) string {
	out, _ := exec.Command("/bin/ls", "-led", p).Output()
	var r []string
	for _, l := range strings.Split(string(out), "\n")[1:] {
		if _, e, ok := strings.Cut(strings.TrimSpace(l), ": "); ok {
			r = append(r, e)
		}
	}
	return strings.Join(r, "\n")
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
	acl := aclOf(e.S())
	if acl == "" || !strings.Contains(acl, "allow read") {
		t.Fatalf("시험 준비: ACL이 붙지 않았다: %q", acl)
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
		if got := aclOf(p); got != acl {
			t.Errorf("ACL이 바뀌었다: %s\n%q, 붙인 값 %q", p, got, acl)
		}
	}
}
