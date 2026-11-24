// Package trustq: Claude 폴더 신뢰 질문 화면 판정 — 한 곳(2026-11-24 knob-test-leak). hc-lane 띄운 직후 확인(cmd/hc-lane/start.go)과
// 병합기가 띄운 풀이·풀이 감사 터미널 감시(cmd/hc-merger/resolvewatch.go)가 함께 쓴다. 화면 글자는 Claude 2.1 실측
// (「Is this a project you created or one you trust?」 아래 「❯ No, exit」·「  Yes, I trust this folder」). 넘기는지는 부르는 쪽이
// 설정 lane.auto_trust로 정한다.
package trustq

import (
	"regexp"
	"strings"
)

var (
	trustQ    = regexp.MustCompile(`(?m)^ *(❯ | +)No, exit *$`)
	trustY    = regexp.MustCompile(`(?m)^ *(❯ | +)Yes, I trust this folder *$`)
	trustCurN = regexp.MustCompile(`(?m)^ *❯ No, exit *$`)
	trustCurY = regexp.MustCompile(`(?m)^ *❯ Yes, I trust this folder *$`)
)

// 판정
const (
	None     = iota // 신뢰 질문 화면이 아니다
	Keys            // 질문이다 — 보낼 키(Down·Enter 또는 Enter)
	NoCursor        // 질문 글자는 맞는데 커서(❯)가 어느 줄에도 안 보인다 — 건드리지 않는다
	Odd             // 신뢰 질문 같은데 글자가 다르다 — 건드리지 않는다
)

// Seen: 신뢰 질문으로 보이는 화면인가(글자가 정확한지는 Look)
func Seen(scr string) bool {
	return strings.Contains(scr, "Accessing workspace") || strings.Contains(scr, "trust this folder") ||
		strings.Contains(scr, "Do you trust") || strings.Contains(scr, "one you trust?")
}

// Look: 화면 판정과 보낼 키(hc keys 이름 — down·enter)
func Look(scr string) (int, []string) {
	if !Seen(scr) {
		return None, nil
	}
	if !strings.Contains(scr, "Is this a project you created or one you trust?") || !trustQ.MatchString(scr) || !trustY.MatchString(scr) {
		return Odd, nil
	}
	switch {
	case trustCurN.MatchString(scr):
		return Keys, []string{"down", "enter"}
	case trustCurY.MatchString(scr):
		return Keys, []string{"enter"}
	}
	return NoCursor, nil
}
