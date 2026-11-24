package main

import (
	"reflect"
	"testing"
)

// long-func-split: cleanMain에서 뗀 판정 단계 — 읽기·쓰기 없이 입력 → 결정
func TestCleanStepDecisions(t *testing.T) {
	wl := "worktree /r\nHEAD aaa\nbranch refs/heads/main\n\nworktree /r.wt/x\nHEAD bbb\nbranch refs/heads/x\nlocked\n\nworktree /r.wt/y\nHEAD ccc\ndetached\n"
	blk, reg := wtBlock(wl, "/r.wt/x")
	if !reg || blkVal(blk, "HEAD ") != "bbb" || blkVal(blk, "branch ") != "refs/heads/x" {
		t.Fatalf("wtBlock(x) = %q %v", blk, reg)
	}
	if _, reg := wtBlock(wl, "/r.wt/z"); reg {
		t.Error("wtBlock: 없는 폴더를 등록으로 읽었다")
	}
	if g := wtBlockProblem(blk, true, "x", "/r.wt/x"); g != "잠긴 워크트리다(git worktree unlock 뒤 다시): /r.wt/x" {
		t.Errorf("잠긴 작업 트리: %q", g)
	}
	if g := wtBlockProblem(blk, true, "other", "/r.wt/x"); g != "이 폴더는 다른 브랜치(x)의 워크트리다: /r.wt/x" {
		t.Errorf("다른 브랜치: %q", g)
	}
	yb, _ := wtBlock(wl, "/r.wt/y")
	if g := wtBlockProblem(yb, true, "y", "/r.wt/y"); g == "" {
		t.Error("분리 HEAD인데 브랜치가 따로 있으면 멈춰야")
	}
	if g := wtBlockProblem(yb, false, "y", "/r.wt/y"); g != "" {
		t.Errorf("분리 HEAD·브랜치 없음은 통과: %q", g)
	}

	lrr := []string{"s1\trefs/merge-queue/100-other", "s1\trefs/merge-queue/101-lane-a", "s1\trefs/merge-queue/102-lane-a", "s2\trefs/merge-queue/103-lane-a", "s1\trefs/park/lane-a"}
	if g := submitHit(lrr, "s1", "lane-a"); g != "refs/merge-queue/101-lane-a" {
		t.Errorf("submitHit = %q (이 레인 표식 가운데 첫 것)", g)
	}
	if g := submitHit(lrr, "s3", "lane-a"); g != "" {
		t.Errorf("다른 커밋은 표식이 아니다: %q", g)
	}

	if n, ok := deleteOnly(" D a\n D b"); n != 2 || !ok {
		t.Errorf("지움뿐: %d %v", n, ok)
	}
	if n, ok := deleteOnly(" D a\n M b"); n != 1 || ok {
		t.Errorf("고침이 섞임: %d %v", n, ok)
	}

	if g := mainWorktree(wl, "/r.wt/x"); g != "/r" {
		t.Errorf("mainWorktree = %q", g)
	}
	if g := mainWorktree(wl, "/r"); g != "" {
		t.Errorf("자기 자신이 메인이면 빈 값: %q", g)
	}
	if g := mainWorktree("worktree /b\nbare\n", "/x"); g != "" {
		t.Errorf("bare면 빈 값: %q", g)
	}

	ds, n, nc, ns, dq, bad := qaRead("c a/.qa/1\ns a/.qa/2\nd .qa/3\nf b/.qa/x/4\nn .qa/5")
	if bad != "" || n != 5 || nc != 1 || ns != 1 || !reflect.DeepEqual(ds, []string{".qa", "a/.qa", "b/.qa"}) ||
		!reflect.DeepEqual(dq, []string{"다름(메인 것 둠) .qa/3", "복사 실패 b/.qa/x/4"}) {
		t.Errorf("qaRead = %q %d %d %d %q %q", ds, n, nc, ns, dq, bad)
	}
	if _, _, _, _, _, bad := qaRead("c a/.qa/1\nzz"); bad != "zz" {
		t.Errorf("줄 꼴이 틀리면 그 줄: %q", bad)
	}
}
