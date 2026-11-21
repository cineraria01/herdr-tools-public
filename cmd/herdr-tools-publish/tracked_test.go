package main

// 착지 전 비밀 꼴 검사(publish-lint-go, 2026-11-21): 추적 파일(git ls-files)의 내용·경로를 공개 내보내기와 같은 scan으로 본다.
// 공개 내보내기는 착지 뒤 herdr-sync 끝에서야 돌아, 가짜 SHA 40자 같은 시험 값이 착지한 뒤에야 걸리고 릴리스가 막혔다.
// .claude/herdr-verify.sh의 싼 검사(tests/publish-secrets.test.sh)가 이 시험을 부른다. 다른 저장소는 HERDR_SCAN_ROOT=<폴더>.
// 설정 publish.secrets(개인 꼴)는 보지 않는다 — 그것은 개인 저장소 몫이다.

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestTrackedNoSecret(t *testing.T) {
	root := os.Getenv("HERDR_SCAN_ROOT")
	if root == "" {
		root = "../.."
	}
	out, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	tmp := t.TempDir()
	var files []string
	for _, n := range bytes.Split(bytes.TrimRight(out, "\x00"), []byte{0}) {
		f := filepath.Join(root, string(n))
		st, err := os.Lstat(f)
		switch {
		case os.IsNotExist(err): // 작업 트리에서 지운 파일
			continue
		case err != nil:
			t.Fatalf("%s: %v", f, err)
		case st.Mode()&os.ModeSymlink != 0: // 링크는 대상 글자를 본다(공개 내보내기와 같이)
			to, err := os.Readlink(f)
			if err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			lf := filepath.Join(tmp, fmt.Sprintf("link%d-%s", len(files), filepath.Base(f)))
			if err := os.WriteFile(lf, []byte(to), 0o600); err != nil {
				t.Fatal(err)
			}
			f = lf
		case !st.Mode().IsRegular(): // 서브모듈 등
			continue
		}
		files = append(files, f)
	}
	names := filepath.Join(tmp, "paths")
	if err := os.WriteFile(names, bytes.ReplaceAll(out, []byte{0}, []byte{'\n'}), 0o600); err != nil {
		t.Fatal(err)
	}
	hits, ok := scan(append(files, names), "")
	if !ok {
		t.Fatal("비밀 꼴 검사 못 함")
	}
	for _, h := range hits {
		t.Error(h)
	}
	if len(hits) > 0 {
		t.Log("가짜 값은 비밀 꼴에 걸리지 않게 끊거나 실행 중에 만든다(예: \"abc\" + strings.Repeat(\"0\", 37))")
	}
}
