package main

// seat-clean-detached(2026-11-23): 감사 스크립트가 `git worktree add --detach`로 직접 만든 읽기 전용 작업 트리의 끝난 터미널을
// 30분 뒤 닫고 그 작업 트리를 지운다. 변경·가지에 없는 커밋·가지 위·git 실패는 지금 규칙(작업 트리 남음)

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDetachedClean(t *testing.T) {
	G, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	R, W := G+"/repo", G+"/repo.wt/aud"
	j2Git(t, G, "init", "-q", "-b", "main", R)
	j2Git(t, R, "commit", "-q", "--allow-empty", "-m", "base")
	j2Git(t, R, "worktree", "add", "-q", "--detach", W)

	if got := detachedClean(W, j2HcExecGit); got != R {
		t.Fatalf("분리·깨끗: 메인 저장소 %q 아님(%q)", R, got)
	}
	// judge: worker가 아니어도(parent·hc-lane 라벨 없음) 30분 지난 done이면 R2, keep 목록·배경 실행이면 남김
	now := float64(time.Now().UnixMilli())
	term := map[string]any{"state": "live", "address": "t1", "name": "fable-audit-1", "cwd": W,
		"status": map[string]any{"app": "claude-code", "state": "done"}, "created_ms": now - 90*minute, "status_ms": now - 40*minute}
	quiet, busy := "끝", "still running"
	f := facts{worktree: no, unlanded: no, detached: true, tail: &quiet}
	if r, why := judge(term, f, now, nil); r != "R2" {
		t.Fatalf("분리·깨끗 40분: R2 아님(%q)", why)
	}
	if r, _ := judge(term, f, now, map[string]bool{"fable-audit-1": true}); r != "" {
		t.Fatal("keep 목록인데 닫음")
	}
	fb := f
	fb.tail = &busy
	if r, _ := judge(term, fb, now, nil); r != "" {
		t.Fatal("배경 실행 흔적인데 닫음")
	}
	if r, _ := judge(term, facts{worktree: no, unlanded: no, tail: &quiet}, now, nil); r != "" {
		t.Fatal("분리·깨끗 아닌 non-worker를 30분에 닫음(지금 규칙은 R3 6시간)")
	}

	// 변경 있음 → 아님, 지우기도 손대지 않음
	os.WriteFile(W+"/new.txt", []byte("x\n"), 0o644)
	if got := detachedClean(W, j2HcExecGit); got != "" {
		t.Fatalf("변경 있음인데 분리·깨끗(%q)", got)
	}
	if ok, msg := worktreeRemove(R, W, j2HcExecGit); ok || !isDir(W) {
		t.Fatalf("변경 있는 작업 트리를 지움(%v %s)", ok, msg)
	}
	os.Remove(W + "/new.txt")
	// 지우기: 깨끗하면 git worktree remove
	if ok, msg := worktreeRemove(R, W, j2HcExecGit); !ok || isDir(W) {
		t.Fatalf("분리·깨끗한 작업 트리를 못 지움(%v %s)", ok, msg)
	}

	// 가지에 없는 커밋 → 아님
	j2Git(t, R, "worktree", "add", "-q", "--detach", W)
	j2Git(t, W, "commit", "-q", "--allow-empty", "-m", "lane")
	if got := detachedClean(W, j2HcExecGit); got != "" {
		t.Fatalf("가지에 없는 커밋인데 분리·깨끗(%q)", got)
	}
	// 가지 위 → 아님
	j2Git(t, W, "switch", "-q", "-c", "aud-b")
	if got := detachedClean(W, j2HcExecGit); got != "" {
		t.Fatalf("가지 위인데 분리·깨끗(%q)", got)
	}
	// git 실패(저장소 아님) → 아님
	N := G + "/plain"
	os.MkdirAll(N, 0o755)
	if got := detachedClean(N, j2HcExecGit); got != "" {
		t.Fatalf("git 실패인데 분리·깨끗(%q)", got)
	}
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
