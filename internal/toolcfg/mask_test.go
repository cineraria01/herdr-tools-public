package toolcfg

import (
	"os"
	"testing"
)

func TestMasker(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HOME", d)
	if _, err := Masker(d); err == nil {
		t.Fatal("무늬 파일이 없으면 오류(올리지 않게)")
	}
	if err := os.WriteFile(d+"/private-patterns.txt", []byte("# 주석만\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Masker(d); err == nil {
		t.Fatal("패턴 줄이 없으면 오류")
	}
	if err := os.WriteFile(d+"/private-patterns.txt", []byte("Secret-Project\nbox-[0-9]+\n/Users/alice/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Masker(d)
	if err != nil {
		t.Fatal(err)
	}
	for in, want := range map[string]string{
		"깨끗한 글":                      "깨끗한 글",
		"SECRET-project 의 box-12 에서": Hidden + " 의 " + Hidden + " 에서",
		"경로 /Users/alice/x — 한글 뒤":   "경로 " + Hidden + "x — 한글 뒤",
		"널\x00글자":                    "널글자",
	} {
		if got := m(in); got != want {
			t.Errorf("%q → %q, 기대 %q", in, got, want)
		}
	}
}
