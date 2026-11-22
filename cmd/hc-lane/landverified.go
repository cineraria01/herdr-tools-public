package main

// 추측 검증 기록 건너뛰기(merger-spec — docs/hc-lane/merger-spec-verify.md). 병합기가 맨 앞 추측이 통과한 트리를 HERDR_SPEC_TREE로 넘기면, 착지가
// 그 트리의 원격 기록(refs/herdr/verified/<트리>, 같은 검증 스크립트 blob)이 있을 때만 HERDR_VERIFIED_TREE=<트리>를 검증 스크립트에 넘긴다.
// 건너뛸지는 검증 스크립트가 정한다(지금 HEAD^{tree}가 그 값이고 작업 트리가 깨끗할 때만). 병합기 밖(HERDR_MERGER≠1)에서는 쓰지 않는다

import (
	"fmt"
	"os"

	"herdr-tools/internal/gitx"
	"herdr-tools/internal/verified"
)

var specTree string // 병합기가 넘긴 추측 트리(이 착지만 — 자식에 물려주지 않는다)

// specTreeTake: 착지 시작에. 물려받은 HERDR_VERIFIED_TREE는 늘 지운다(기록을 본 착지만 넘긴다)
func specTreeTake(merger bool) {
	specTree = ""
	if merger {
		specTree = os.Getenv("HERDR_SPEC_TREE")
	}
	os.Unsetenv("HERDR_SPEC_TREE")
	os.Unsetenv("HERDR_VERIFIED_TREE")
}

// verifiedEnv: 검증을 부르기 직전에. cand가 있으면(내장 길) 그 트리가 추측 트리와 같을 때만 본다. 기록이 있으면 HERDR_VERIFIED_TREE를 둔다
func verifiedEnv(cand string) {
	os.Unsetenv("HERDR_VERIFIED_TREE")
	if specTree == "" {
		return
	}
	if cand != "" {
		if t, rc := gitOut("rev-parse", "-q", "--verify", cand+"^{tree}"); rc != 0 || t != specTree {
			fmt.Printf("hc-lane land: 추측 트리(%s)와 rebase한 트리가 다르다 — 검증을 그대로 돈다\n", cut(specTree, 12))
			return
		}
	}
	switch _, st := verified.Lookup(".", specTree, os.Stderr); st {
	case gitx.Yes:
		fmt.Printf("hc-lane land: 이 트리는 이미 검증 통과(%s%s) — HERDR_VERIFIED_TREE를 검증에 넘긴다\n", verified.Prefix, cut(specTree, 12))
		os.Setenv("HERDR_VERIFIED_TREE", specTree)
	case gitx.No:
		fmt.Printf("hc-lane land: 추측 트리 %s 의 검증 기록이 없다(또는 검증 스크립트가 다름) — 검증을 그대로 돈다\n", cut(specTree, 12))
	default:
		fmt.Println("hc-lane land: 검증 기록을 못 읽었다(모름) — 검증을 그대로 돈다")
	}
}
