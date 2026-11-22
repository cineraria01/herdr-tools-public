package main

// 나무 모음의 준비 폴더(ops/R-202): 맥 나무 파일 묶음(tar)을 저장소 최종 경로에 바로 풀면, 풀다 끊긴(SIGKILL·잘린 묶음) 파일이
// 목표 서명과 다른 채 남아 일지 되돌림이 「남이 바꿈」으로 멈췄다. 같은 파일시스템(그 저장소 git 폴더 아래)에 풀고 파일마다 서명을
// 확인한 뒤 완성한 파일만 rename으로 바꿔 넣는다 — 최종 경로의 파일은 언제나 옛 값이나 새 값 둘 가운데 하나다(일지가 둘 다 알아본다)

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// stageDir: 저장소 repo의 모음 준비 폴더(git 폴더 아래 — 작업 트리 상태에 안 보이고 같은 파일시스템). 앞 실행이 남긴 것은 비운다
// (herdr-sync는 맥 잠금 안에서 하나만 돈다 — 남은 것은 끝난 자기 실행의 것뿐이다)
func stageDir(repo string) (string, error) {
	gd, rc := gitq("-C", repo, "rev-parse", "--absolute-git-dir")
	if rc != 0 || gd == "" {
		return "", fmt.Errorf("git 폴더를 읽지 못했다 (git rev-parse 종료 %d — 모름)", rc)
	}
	d := gd + "/herdr-sync-stage"
	if err := os.RemoveAll(d); err != nil {
		return "", err
	}
	return d, os.MkdirAll(d, 0o700)
}

// collectTree: 나무 하나의 맥 파일 묶음(produce가 내는 tar, 나무 안 상대 경로 rels)을 저장소 폴더 dst에 옮긴다.
// want = 상대 경로 → 쓸 값의 서명(일지와 같은 값), tkey = 맥 나무 키(서명 계산용). 묶음·서명·바꿔 넣기 가운데 하나라도 실패면 오류 —
// 그때까지 바꿔 넣은 파일은 새 값 그대로다(부른 쪽이 일지로 되돌린다)
func collectTree(repo, dst, tkey string, rels []string, want map[string]string, produce *exec.Cmd) error {
	st, err := stageDir(repo)
	if err != nil {
		return err
	}
	defer os.RemoveAll(st)
	x := exec.Command("tar", "-xf", "-")
	x.Dir, x.Stderr = st, os.Stderr
	if p, c := pipeline(nil, produce, x, nil); p != 0 || c != 0 {
		return fmt.Errorf("묶음이 끝까지 오지 않았다 (보냄 %d · 받음 %d)", p, c)
	}
	for _, r := range rels { // 다 확인한 뒤에야 하나라도 바꿔 넣는다
		if w, s := want[r], rsig(st+"/"+r, tkey+"/"+r); w == "" || s != w {
			return fmt.Errorf("%s 서명이 맥 값과 다르다 (%s ≠ %s)", r, s, w)
		}
	}
	for _, r := range rels {
		if err := os.MkdirAll(filepath.Dir(dst+"/"+r), 0o755); err != nil {
			return err
		}
		if err := os.Rename(st+"/"+r, dst+"/"+r); err != nil {
			return err
		}
	}
	return nil
}
