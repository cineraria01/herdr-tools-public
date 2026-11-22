//go:build !windows

package main

// gofix3-shx lane/R-206: 미등록 작업 폴더 검사(leftoverScript)에서 find 열거가 실패하면 뒤의 경로 변환(sed) 성공이 그것을 가려
// 「무시되지 않는 것 없음」으로 읽고 clean이 폴더를 지웠다. 열거 실패는 모름(ok=false)이어야 한다

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLeftoverFindFails(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	d := t.TempDir()
	main, w := filepath.Join(d, "app"), filepath.Join(d, "app.wt", "l1")
	for _, a := range [][]string{{"init", "-q", main}} {
		if out, err := exec.Command("git", a...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
	}
	if err := os.MkdirAll(w, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w, "고유.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	common := filepath.Join(main, ".git")
	// 정상: 고유 파일이 보인다
	if left, ok := leftover(main, w, common); !ok || left != "고유.txt" {
		t.Fatalf("정상 검사: (%q, %v), want (\"고유.txt\", true)", left, ok)
	}
	// find만 실패(아무것도 안 찍고 1) — 모름이어야 한다
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "find"), []byte("#!/bin/sh\necho 'find: 읽지 못함' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if left, ok := leftover(main, w, common); ok {
		t.Fatalf("find 실패인데 검사 성공으로 읽었다: (%q, %v) — clean이 고유 파일을 지운다", left, ok)
	}
}
