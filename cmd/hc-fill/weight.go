package main

// 느린 기계는 가벼운 레인만(2026-11-26 ht-slow-machine-light, docs/hc-fill/load-steer.md 「가벼운 일만」 — 사용자 「느린 m1 에어 같은것들은
// 좀 빼고 다른기기 우선으로 하는 기능도 필요할듯」 「간단한 작업들만 할수 있도록」). 느린 기계는 일을 늦게 끝내 자주 한가해 보여
// 부하 차례로는 무거운 레인을 받았다. 손잡이 light_only@<기계> 1이면 그 기계에는 가벼운 레인만 띄우고, 가벼운 레인은 그 기계를 먼저 쓴다
// (빠른 기계 자리를 아끼게). 레인 무게는 대기열 18칸(지휘가 정함) — 비었으면 지시문 「파일:」 줄로 추정(이 파일 한 곳)

import (
	"regexp"
	"strings"
)

var (
	LIGHT    bool   // 이번 줄이 가벼운 레인인가(waitRow가 laneWeight로 — place가 읽는다)
	LIGHTWHY string // 그 근거(「가벼움(지정)」 등)
)

// machLight: 그 기계가 가벼운 일만 받는가(손잡이 light_only@<기계> 1)
func machLight(m string) bool {
	v, ok := knobConf("light_only@" + m)
	return ok && v == "1"
}

var (
	fileLineRe = regexp.MustCompile(`^[[:space:]]*([-*][[:space:]]*)?(\*\*)?파일(\*\*)?:(\*\*)?`)
	fileSepRe  = regexp.MustCompile("[\\s·,、;+()\\[\\]「」`'\"]+")
	pathHeadRe = regexp.MustCompile(`^[A-Za-z0-9_.*/~-]+`)
)

// laneWeight <대기열 줄>: 가벼운 레인인가와 그 근거. 18칸 light·normal이 앞서고, 비었으면 지시문 「파일:」 줄(여러 줄이면 모두)의
// 낱말이 모두 문서(*.md·docs/ 아래)일 때만 가벼움 — 파일 줄이 없거나 코드 경로·도구 이름·설명 글이 하나라도 섞이면 보통(모르면 보통).
// 낱말 끝에 붙은 조사(「README.md와」)는 경로 뒤라 뗀다
func laneWeight(r Row) (bool, string) {
	switch r.F(qWeight) {
	case "light":
		return true, "가벼움(지정)"
	case "normal":
		return false, "보통(지정)"
	}
	n := 0
	for _, l := range readLines(r.F(qBrief)) {
		loc := fileLineRe.FindStringIndex(l)
		if loc == nil {
			continue
		}
		for _, w := range fileSepRe.Split(l[loc[1]:], -1) {
			if w == "" {
				continue
			}
			p := strings.TrimRight(pathHeadRe.FindString(w), ".,-")
			if p == "" || !(strings.HasSuffix(p, ".md") || p == "docs" || strings.HasPrefix(p, "docs/") || strings.Contains(p, "/docs/")) {
				return false, "보통(추정)"
			}
			n++
		}
	}
	if n == 0 {
		return false, "보통(추정)"
	}
	return true, "가벼움(파일 줄 문서뿐)"
}

// weightArg: add·edit의 --light·--weight를 하나로 — ok에 없는 값·서로 다른 값이면 거절
func weightArg(cur, v string, ok ...string) string {
	if !has(ok, v) {
		die("--weight는 %s: %s", strings.Join(ok, "|"), v)
	}
	if cur != "" && cur != v {
		die("--light·--weight: 서로 다른 무게를 함께 쓰지 않는다(%s · %s)", cur, v)
	}
	return v
}

var weightLabel = map[string]string{"light": "가벼움", "normal": "보통", "-": "추정"}

// setWeight: edit --light|--weight — 16칸까지 채우고(우선은 기본 보통) 18칸에 무게
func setWeight(x Row, wt string) Row {
	for i := x.NF() + 1; i < qRID; i++ {
		x = x.Set(i, map[bool]string{true: "normal", false: "-"}[i == qPrio])
	}
	if x.F(qRID) == "" { // 줄 ID 없는 옛 줄 — 빈 17칸을 두면 탭 나누기가 18칸을 17칸으로 읽는다
		x = x.Set(qRID, rowid())
	}
	return x.Set(qWeight, wt)
}

// lightFirst: 가벼운 레인의 후보(부하 차례)에서 가벼운 일만 받는 기계를 앞으로 — 그 사이·나머지 사이 차례는 그대로
func lightFirst(cs []Machine) []Machine {
	var a, b []Machine
	for _, m := range cs {
		if machLight(m.Name) {
			a = append(a, m)
		} else {
			b = append(b, m)
		}
	}
	return append(a, b...)
}
