package main

// argerr.go: 인자 오류(모르는 인자·값 없음·꼴 틀림 — 사람·에이전트가 잘못 부름)로 끝내기(2026-11-24 cli-usage-version).
// 표준 오류 글·종료 코드는 die 그대로(계약)이고, 운영 기록만 die가 아니라 usage 줄이다 — 자가 점검이 도구 이상으로 줍지 않게.
// 모르는 인자가 아는 인자와 가까우면 둘째 줄에 권한다(레인이 events --name을 20초마다 되풀이했다 → --lane)

import (
	"fmt"
	"os"
	"strings"

	"herdr-tools/internal/oplog"
)

// dieArg: 「hc-lane: <글>」 한 줄 뒤 종료 1, 운영 기록은 usage
func dieArg(format string, a ...any) { argFail(1, fmt.Sprintf(format, a...)) }

// argFail: 「hc-lane: <글>」(글에 줄바꿈이 있으면 여러 줄 그대로) 뒤 종료 rc, 운영 기록은 usage
func argFail(rc int, msg string) {
	fmt.Fprintf(os.Stderr, "hc-lane: %s\n", msg)
	oplog.Fatal("hc-lane", "", oplog.BadArg, rc, msg)
	if dieHook != nil { // clean은 파싱 전에 건다 — 인자 오류에도 결과 JSON 「stopped」(die와 같게)
		dieHook(msg)
	}
	exit(rc)
}

// unknownArg: 「알 수 없는 인자: <x><tail>」, 가까운 아는 인자가 있으면 둘째 줄에 권함
// 가까운 인자가 없으면 「다음:」 한 줄로 안내를 가리킨다(agent-skill-guides — 짐작한 인자를 되풀이하지 않게)
func unknownArg(x, tail string, known ...string) {
	h := argHint(x, known)
	if h == "" {
		h = "\nhc-lane: 다음: hc-lane --skill(에이전트 안내)이나 hc-lane --help(사용법)로 인자를 본다"
	}
	dieArg("알 수 없는 인자: %s%s%s", x, tail, h)
}

// argHint: x와 가장 가까운 아는 인자를 「\nhc-lane: <x> 대신 <k>?」로(없으면 ""). 「--」를 뺀 이름의 편집 거리가 2 이하이고
// 이름 길이보다 작아야 가깝다(짧은 오타가 아무 인자에나 붙지 않게)
// ponytail: 편집 거리 하나뿐 — 뜻이 같은 다른 이름(--dir → --repo 등)은 못 짚는다. 자주 틀리는 짝이 나오면 여기에 표를 더한다
func argHint(x string, known []string) string {
	n := strings.TrimLeft(x, "-")
	best, bd := "", 3
	for _, k := range known {
		if d := editDist(n, strings.TrimLeft(k, "-")); d < bd && d < len(n) {
			best, bd = k, d
		}
	}
	if best == "" {
		return ""
	}
	return "\nhc-lane: " + x + " 대신 " + best + "?"
}

// editDist: 바이트 단위 레벤슈타인 거리
func editDist(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			c := prev[j-1]
			if a[i-1] != b[j-1] {
				c++
			}
			cur[j] = min(c, prev[j]+1, cur[j-1]+1)
		}
		prev = cur
	}
	return prev[len(b)]
}
