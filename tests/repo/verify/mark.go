//go:build !windows

package main

// 통과 표식(submit-fast, 2026-11-24): 전체 검증(--cheap-only 아님)이 통과로 끝났고, 시작과 끝에 작업 트리가 깨끗하며(추적 파일 변경·추적
// 안 된 파일 없음) HEAD^{tree}가 같을 때만 이 작업 트리의 git 폴더 herdr-verified-tree에 「<트리> <유닉스 초>」를 남긴다(작업 트리마다 따로,
// 원격으로 안 나감). 읽는 곳은 제출(hc-lane land)의 사전 검사 건너뜀 하나 — 병합기·착지는 읽지 않는다. 저장소가 아니거나 git이 실패하면 안 남긴다

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const markName = "herdr-verified-tree"

// cleanTree: 작업 트리가 깨끗하면 HEAD^{tree}, 아니면(또는 git 실패) 빈 값
func cleanTree(root string) string {
	st, err := gitIn(root, "status", "--porcelain", "--untracked-files=all")
	if err != nil || st != "" {
		return ""
	}
	t, err := gitIn(root, "rev-parse", "-q", "--verify", "HEAD^{tree}")
	if err != nil {
		return ""
	}
	return t
}

// markPassed: 통과한 검증 끝에 — 시작 때 트리 t0이 비지 않았고 지금도 깨끗한 같은 트리면 표식을 tmp→rename으로 쓴다
func markPassed(root, t0 string, w *os.File) {
	if t0 == "" || cleanTree(root) != t0 {
		return
	}
	p, err := gitIn(root, "rev-parse", "--path-format=absolute", "--git-path", markName)
	if err != nil || p == "" {
		return
	}
	tmp := fmt.Sprintf("%s.%d", p, os.Getpid())
	if err := os.WriteFile(tmp, fmt.Appendf(nil, "%s %d\n", t0, time.Now().Unix()), 0o644); err != nil {
		fmt.Fprintf(w, "경고 통과 표식을 못 썼다(%v) — 제출 때 사전 검사를 그대로 돈다\n", err)
		return
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		fmt.Fprintf(w, "경고 통과 표식을 못 썼다(%v) — 제출 때 사전 검사를 그대로 돈다\n", err)
		return
	}
	fmt.Fprintf(w, "통과 표식 %s (%s — 이 트리를 제출하면 사전 검사를 건너뛴다)\n", t0[:12], filepath.Base(p))
}

func gitIn(root string, args ...string) (string, error) {
	c := exec.Command("git", args...)
	c.Dir = root
	out, err := c.Output()
	return strings.TrimSpace(string(out)), err
}
