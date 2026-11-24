package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dedupe-helpers ③: appendLine이 공용 atomic.Append로 옮긴 뒤에도 옛 꼴 그대로 — 줄바꿈을 붙이고, 못 열면 「열기 <경로>: …」
func TestAppendLineShape(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "f.tsv")
	if err := appendLine(p, "a\tb"); err != nil {
		t.Fatal(err)
	}
	if err := appendLine(p, "c"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "a\tb\nc\n" {
		t.Fatalf("%q", b)
	}
	bad := filepath.Join(d, "없음", "f")
	if err := appendLine(bad, "x"); err == nil || !strings.HasPrefix(err.Error(), "열기 "+bad+": ") {
		t.Fatalf("못 열면 「열기 <경로>: …」: %v", err)
	}
}
