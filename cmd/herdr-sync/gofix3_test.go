package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// treeRepo: 나무 폴더 t에 big(옛 값)을 커밋한 저장소와, 맥 쪽 새 값(src/big 131072바이트·src/small)을 담은 tar
func treeRepo(t *testing.T) (repo, src, tb string) {
	t.Helper()
	repo = newRepo(t)
	STATE, TMP = t.TempDir(), t.TempDir()
	if err := os.MkdirAll(repo+"/t", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(repo+"/t/big", []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "add", "t/big")
	gitT(t, repo, "commit", "-qm", "t")
	src = t.TempDir()
	if err := os.WriteFile(src+"/big", bytes.Repeat([]byte("0123456789abcde\n"), 8192), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src+"/small", []byte("s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tb = t.TempDir() + "/tree.tar"
	if out, err := exec.Command("tar", "-cf", tb, "-C", src, "big", "small").CombinedOutput(); err != nil {
		t.Fatalf("tar: %v %s", err, out)
	}
	return
}

// stageLeft: 저장소에 이번 모음의 준비 파일이 남았나(작업 트리 untracked·.git 아래 준비 폴더)
func stageLeft(t *testing.T, repo string) string {
	t.Helper()
	var left []string
	filepath.WalkDir(repo, func(p string, d os.DirEntry, err error) error {
		if err == nil && strings.Contains(d.Name(), "herdr-sync-stage") || strings.Contains(d.Name(), ".hsync.") {
			left = append(left, p)
		}
		return nil
	})
	return strings.Join(left, " ")
}

// ops/R-202: 나무 tar가 최종 경로에 일부만 쓴 채 끊기면(131,072바이트 중 39,424) 그 파일이 목표 서명과 다른 채 남아
// 다음 실행 두 번이 모두 1이었다. 같은 파일시스템의 준비 폴더에 풀고 파일마다 서명을 확인한 뒤 완성한 파일만 바꿔 넣는다
func TestCollectTreeStagesAndVerifies(t *testing.T) {
	repo, src, tb := treeRepo(t)
	want := map[string]string{"big": rsig(src+"/big", "t/big"), "small": rsig(src+"/small", "t/small")}
	rels := []string{"big", "small"}
	cut := exec.Command("sh", "-c", `head -c 39424 "$1"; exit 1`, "_", tb)
	if err := collectTree(repo, repo+"/t", "t", rels, want, cut); err == nil {
		t.Fatal("끊긴 tar인데 성공")
	}
	if b, _ := os.ReadFile(repo + "/t/big"); string(b) != "old\n" {
		t.Errorf("끊긴 tar가 최종 경로에 일부를 썼다: %d바이트", len(b))
	}
	if _, err := os.Lstat(repo + "/t/small"); err == nil {
		t.Error("끊긴 묶음의 다른 파일이 최종 경로에 생겼다")
	}
	if l := stageLeft(t, repo); l != "" {
		t.Errorf("준비 파일이 남았다: %s", l)
	}
	// tar는 끝까지 됐지만 서명이 다르다(그사이 맥 파일이 바뀜) — 바꿔 넣지 않는다
	bad := map[string]string{"big": "1.1.-", "small": want["small"]}
	if err := collectTree(repo, repo+"/t", "t", rels, bad, exec.Command("cat", tb)); err == nil {
		t.Fatal("서명이 다른데 성공")
	}
	if b, _ := os.ReadFile(repo + "/t/big"); string(b) != "old\n" {
		t.Error("서명이 다른 파일을 바꿔 넣었다")
	}
	if _, err := os.Lstat(repo + "/t/small"); err == nil {
		t.Error("서명 확인 전에 다른 파일을 바꿔 넣었다")
	}
	// 정상: 두 파일 모두 새 값, 준비 파일 없음
	if err := collectTree(repo, repo+"/t", "t", rels, want, exec.Command("cat", tb)); err != nil {
		t.Fatalf("정상 묶음: %v", err)
	}
	if rsig(repo+"/t/big", "t/big") != want["big"] || rsig(repo+"/t/small", "t/small") != want["small"] {
		t.Error("정상 묶음인데 서명이 다르다")
	}
	if l := stageLeft(t, repo); l != "" {
		t.Errorf("준비 파일이 남았다: %s", l)
	}
}
