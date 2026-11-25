//go:build !windows

package main

// 통과 조각 이어 쓰기(2026-11-25 ht-merger-spec-reuse — 병합기 추측 일꾼이 HERDR_VERIFY_PARTS=<파일>을 준다, docs/hc-lane/merger-spec-verify.md
// 「통과 조각 이어 쓰기」): 조각 이름 = 범위 지문(scope.go — 입력이 같으면 같다). 파일 첫 줄 「<트리> <지문…>」의 트리가 지금 트리면 그
// 지문의 조각을 건너뛰고(HERDR_VERIFY_ALL=1이면 전부 돈다), 조각이 통과할 때마다 지금까지 통과한 전부로 파일을 새로 쓴다(임시 → 이름 바꿈 —
// 일꾼이 도는 중에 읽는다). 범위 지문이 없으면(HERDR_VERIFY_SCOPE=off·지문 실패·작업 트리가 깨끗하지 않음) 읽지도 쓰지도 않는다.

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

type partsFile struct {
	path, tree string
	have       []string
}

// partsLoad: HERDR_VERIFY_PARTS가 있으면 그 파일 — 같은 트리 줄의 지문만 읽는다(없으면 nil)
func partsLoad(tree string) *partsFile {
	p, tree := os.Getenv("HERDR_VERIFY_PARTS"), strings.TrimSpace(tree)
	if p == "" || tree == "" {
		return nil
	}
	pf := &partsFile{path: p, tree: tree}
	if b, err := os.ReadFile(p); err == nil {
		l, _, _ := strings.Cut(string(b), "\n")
		if f := strings.Fields(l); len(f) > 1 && f[0] == tree {
			pf.have = f[1:]
		}
	}
	return pf
}

// partsSkip: 앞 실행이 같은 트리에서 통과한 조각을 건너뛴다(한 줄 알림)
func (r *scopeRun) partsSkip(w io.Writer) {
	if r.parts == nil || os.Getenv("HERDR_VERIFY_ALL") == "1" {
		return
	}
	k := 0
	for i, key := range r.keys {
		if key != "" && slices.Contains(r.parts.have, key) {
			r.can[i], r.skip[i] = true, true
			k++
		}
	}
	if k > 0 {
		fmt.Fprintf(w, "이어 쓰기: 같은 트리 %.12s 에서 앞 실행이 통과한 조각 %d개 건너뜀(HERDR_VERIFY_PARTS)\n", r.parts.tree, k)
	}
}

// add: 조각 하나가 통과 — 파일을 새로 쓴다(못 쓰면 경고 한 줄, 검증 결과는 그대로)
func (p *partsFile) add(key string, w io.Writer) {
	if slices.Contains(p.have, key) {
		return
	}
	p.have = append(p.have, key)
	tmp := p.path + ".tmp"
	err := os.WriteFile(tmp, []byte(p.tree+" "+strings.Join(p.have, " ")+"\n"), 0o644)
	if err == nil {
		err = os.Rename(tmp, p.path)
	}
	if err != nil {
		fmt.Fprintf(w, "경고 통과 조각 파일을 못 썼다(%v) — 다시 맡은 실행이 이 조각을 또 돈다\n", err)
	}
}
