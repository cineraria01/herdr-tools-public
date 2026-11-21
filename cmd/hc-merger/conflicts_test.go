package main

import (
	"os"
	"testing"
)

// conflicts: 기준과 부딪히는 파일만 이름으로(hc-fill 다시 맞춤 지시문에 들어간다), 깨끗이 합쳐지면 「파일 모름(로그)」
func TestConflictsNamesFiles(t *testing.T) {
	f := newFx(t)
	f.git(f.seed, "checkout", "-q", "-B", "l-a", "main")
	os.WriteFile(f.seed+"/base.txt", []byte("lane\n"), 0o666)
	os.WriteFile(f.seed+"/new.txt", []byte("new\n"), 0o666)
	f.git(f.seed, "add", "-A")
	f.git(f.seed, "commit", "-qm", "l-a")
	lane := f.git(f.seed, "rev-parse", "HEAD")
	f.git(f.seed, "checkout", "-q", "main")
	os.WriteFile(f.seed+"/base.txt", []byte("main\n"), 0o666)
	f.git(f.seed, "commit", "-qam", "main moves")
	f.git(f.seed, "push", "-q", "origin", "main")
	f.git(f.seed, "push", "-q", "origin", "l-a")
	f.git(f.clone, "fetch", "-q", "origin", "main", "l-a")
	m := f.merger()
	m.B = f.git(f.clone, "rev-parse", "origin/main")
	if got := m.conflicts(lane); got != "base.txt" {
		t.Fatalf("충돌 파일: %q", got)
	}
	clean := f.git(f.clone, "rev-parse", "origin/main~1") // 기준의 조상 — 부딪힘 없음
	if got := m.conflicts(clean); got != "파일 모름(로그)" {
		t.Fatalf("깨끗한 합침: %q", got)
	}
}
