package main

// 병합기 줄의 착지 실행(ht-merger-land-offload, docs/hc-lane/merger.md 「착지 실행 기계」): 병합기가 이 기계(LOCAL)에 있으면 이 기계 병합기
// 일지(<XDG_STATE_HOME, 없으면 ~/.local/state>/hc-lane/merger-journal.tsv)의 마지막 「착지 실행」 줄 까닭 칸 「<기계>(<까닭>)」(읽기만).
// 이 기계 병합기 프로젝트가 둘 이상이면 그 줄의 레인이 그 프로젝트 대기열 줄일 때만 — statemerge.go와 같은 한계. 다른 기계 병합기는 안 보임

import (
	"os"
	"strings"
)

// maLandOn <프로젝트> <이 기계 병합기 프로젝트 수>: 마지막 착지 실행, 없으면 빈 값
func maLandOn(p string, locals int) string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		dir = HOME + "/.local/state"
	}
	b, ok := readFile(dir + "/hc-lane/merger-journal.tsv")
	if !ok {
		return ""
	}
	ls := lines(b)
	for i := len(ls) - 1; i >= 0; i-- {
		f := strings.Split(ls[i], "\t")
		if len(f) < 12 || f[9] != "착지 실행" {
			continue
		}
		if q := qrow(f[2]); locals > 1 && (q == nil || q.F(qProj) != p) {
			continue
		}
		what, _, _ := strings.Cut(f[11], " — 구성원 ")
		return what
	}
	return ""
}
