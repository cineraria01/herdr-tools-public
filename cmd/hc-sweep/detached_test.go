package main

// seat-clean-detached(2026-11-23): 감사 스크립트가 `git worktree add --detach`로 직접 만든 읽기 전용 작업 트리의 끝난 터미널을
// 30분 뒤 닫고 그 작업 트리를 지운다. 변경·가지에 없는 커밋·가지 위·git 실패는 지금 규칙(작업 트리 남음)

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withLs: hc ls는 ls를 돌려주고(빈 글이면 실패) hc exec git은 진짜로 돌리는 실행기
func withLs(ls string) runner {
	return func(d time.Duration, env []string, name string, args ...string) result {
		if name == "hc" && len(args) > 0 && args[0] == "ls" {
			if ls == "" {
				return result{rc: 1, errs: "hc ls 실패"}
			}
			return result{out: ls}
		}
		return j2HcExecGit(d, env, name, args...)
	}
}

const noTerms = `{"terminals":[]}`

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
	if ok, msg := worktreeRemove(R, W, "t1", withLs(noTerms)); ok || !isDir(W) {
		t.Fatalf("변경 있는 작업 트리를 지움(%v %s)", ok, msg)
	}
	os.Remove(W + "/new.txt")
	// 지우기: 깨끗하면 git worktree remove
	if ok, msg := worktreeRemove(R, W, "t1", withLs(noTerms)); !ok || isDir(W) {
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

// seat-clean-fix(2026-11-23): 분리 작업 트리를 닫고 지우기 전에 hc ls로 그 폴더(또는 그 아래)를 cwd로 쓰는 다른 live 터미널을 본다.
// 11-23 끝난 Mythos 감사 터미널 여섯을 닫으며 같은 폴더를 쓰던 확인 세션들의 작업 트리를 지운 일 뒤
func TestDetachedInUse(t *testing.T) {
	G, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	R, W := G+"/repo", G+"/repo.wt/aud"
	j2Git(t, G, "init", "-q", "-b", "main", R)
	j2Git(t, R, "commit", "-q", "--allow-empty", "-m", "base")
	j2Git(t, R, "worktree", "add", "-q", "--detach", W)
	os.MkdirAll(W+"/sub", 0o755) // 빈 폴더는 git status에 안 보인다
	os.MkdirAll(W+"2", 0o755)
	os.Symlink(W, G+"/link") // 다른 터미널이 심볼릭 링크로 들어가 있어도 같은 폴더

	term := func(addr, cwd string) string {
		return `{"address":"` + addr + `","state":"live","status":"working","cwd":"` + cwd + `"}`
	}
	list := func(ts ...string) string { return `{"terminals":[` + strings.Join(ts, ",") + `]}` }
	owner := term("t1", W)

	for _, c := range []struct {
		name, ls string
		remove   bool
	}{
		{"hc ls 실패", "", false},
		{"다른 live 터미널이 그 아래 cwd", list(owner, term("t2", W+"/sub")), false},
		{"다른 live 터미널이 링크로 같은 폴더", list(owner, term("t2", G+"/link")), false},
		{"이 기계 모름 항목", list(owner, `{"address":"t3","state":"live","cwd":7}`), false},
		{"끝난 터미널·다른 기계·형제 폴더 이름 접두만", list(owner, `{"address":"t4","state":"finished","cwd":"`+W+`"}`,
			term("m1/t5", W), term("t6", W+"2")), true},
	} {
		r := withLs(c.ls)
		if why := dirInUse(W, "t1", r); (why == "") != c.remove {
			t.Fatalf("%s: dirInUse %q", c.name, why)
		}
		ok, msg := worktreeRemove(R, W, "t1", r)
		if ok != c.remove || isDir(W) == c.remove {
			t.Fatalf("%s: 지움 %v(기대 %v) — %s", c.name, ok, c.remove, msg)
		}
	}
}
