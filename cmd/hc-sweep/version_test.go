package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 재감사 base R-31(B25 다섯째 CLI): 릴리스 방식(-ldflags -X …version.Commit)으로 지은 hc-sweep의 --version이 그 커밋을 보인다
func TestVersionShowsCommit(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "hc-sweep")
	if b, err := exec.Command("go", "build", "-ldflags", "-X herdr-tools/internal/version.Commit=abc123def456", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("빌드: %v %s", err, b)
	}
	out, err := exec.Command(bin, "--version").Output()
	if err != nil || strings.TrimSpace(string(out)) != "hc-sweep (Go abc123def456)" {
		t.Fatalf("--version: %q %v", out, err)
	}
}
