package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVloadMark(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HC_FILL_VERDICTS", d)
	if err := os.MkdirAll(filepath.Join(d, "lane"), 0o777); err != nil {
		t.Fatal(err)
	}
	if _, rc := vload("lane", "a1"); rc != 1 {
		t.Fatalf("없는 판정 = %d", rc)
	}
	if _, rc := vload("la ne", "a1"); rc != 2 {
		t.Fatalf("이상한 이름 = %d", rc)
	}
	sha := strings.Repeat("ab", 20)
	body := "aid\ta1\nrun\tt1@5\nsha\t" + sha + "\nverdict\tfix\nreport\t/r.md\nround\t2\n"
	if err := os.WriteFile(filepath.Join(d, "lane", "a1.md"), []byte(body), 0o666); err != nil {
		t.Fatal(err)
	}
	vd, rc := vload("lane", "a1")
	if rc != 0 || vd.b != "main" || vd.run != "t1@5" || vd.round() != "2번째 " {
		t.Fatalf("vload = %+v %d", vd, rc)
	}
	if vd.state() != "none" {
		t.Fatalf("표시 없음 = %q", vd.state())
	}
	if vd.mark("taken\tT\tt1@5") != 0 || vd.mark("void") != 1 || vd.state() != "taken" {
		t.Fatalf("표시: %q", vd.state())
	}
	if err := os.WriteFile(filepath.Join(d, "lane", "a1.md"), []byte(strings.Replace(body, "fix", "maybe", 1)), 0o666); err != nil {
		t.Fatal(err)
	}
	if _, rc := vload("lane", "a1"); rc != 2 {
		t.Fatalf("틀린 판정 값 = %d", rc)
	}
}
