package main

// 레인 작업 트리 찾기·HEAD 읽기 — aOpen(감사할 sha 풀기)과 aDeliver(판정 뒤 새 커밋 가름)가 같이 쓴다

import (
	"regexp"
	"strings"
)

// laneCwd: 레인 작업 트리 — 레인 터미널(주소 la)의 cwd, 없으면 <repo>.wt/<레인> 짐작
func laneCwd(la, p, m, n string) string {
	for _, t := range TERMS {
		if t.Addr == la && t.Cwd != "" && t.Cwd != "-" {
			return t.Cwd
		}
	}
	return strings.TrimSuffix(repoOf(p, m), "/") + ".wt/" + n
}

// auLaneHead: 레인 작업 트리(laneCwd)의 HEAD 40자 — 못 읽거나 꼴이 틀리면 ""(모름 → 지금처럼 막힘)
func auLaneHead() string {
	d := laneCwd(auPa, aproj(auAid), auAm, auLn)
	if d == ".wt/"+auLn { // 터미널 cwd도 저장소도 모름
		return ""
	}
	o, rc := onm(nil, auAm, `git -C "$d" rev-parse --verify -q HEAD`, d)
	if h := sub(o); rc == 0 && sha40Re.MatchString(h) {
		return h
	}
	return ""
}

var sha40Re = regexp.MustCompile(`^[0-9a-f]{40}$`)
