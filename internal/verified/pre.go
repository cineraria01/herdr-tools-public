package verified

import (
	"fmt"
	"strconv"
	"strings"
)

// 미리 줄(preverify, 2026-11-24): 레인이 「hc-lane report 완료 <sha>」를 적을 때 원격 참조 PreRefs+<레인> → 표식 커밋(부모 = 그 sha,
// 트리 = 그 sha의 트리, 제목 PreMsg)을 올린다. 병합기가 빈 검증 기계에 「기준 + 그 커밋」 추측을 미리 띄운다(cmd/hc-merger/specpre.go).
// 제출 큐(refs/merge-queue/*)와 다른 이름 공간이다 — 착지 후보로 읽는 길이 없다.
const (
	PreRefs = "refs/merge-pre/"
	preHead = "hc-lane-pre"
	// PreOffLine: 검증 기계 목록 커밋 메시지에 이 줄이 있으면 미리 줄을 쓰지 않는다(hc-fill 설정 줄 「merger_preverify <프로젝트> off」).
	// 탭이 없어 옛 ParseVerifiers는 건너뛴다
	PreOffLine = "#preverify=off"
)

// PreMsg: 표식 커밋 제목
func PreMsg(lane, base string, at int64) string {
	return fmt.Sprintf("%s lane=%s base=%s at=%d", preHead, lane, base, at)
}

// ParsePre: 표식 커밋 제목을 읽는다
func ParsePre(subject string) (lane, base string, at int64, ok bool) {
	f := strings.Fields(subject)
	if len(f) != 4 || f[0] != preHead {
		return "", "", 0, false
	}
	for _, kv := range f[1:] {
		k, v, _ := strings.Cut(kv, "=")
		switch k {
		case "lane":
			lane = v
		case "base":
			base = v
		case "at":
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return "", "", 0, false
			}
			at = n
		default:
			return "", "", 0, false
		}
	}
	return lane, base, at, lane != "" && base != ""
}

// PreOff: 검증 기계 목록 커밋 메시지가 미리 줄을 껐나
func PreOff(msg string) bool {
	for _, l := range strings.Split(msg, "\n") {
		if l == PreOffLine {
			return true
		}
	}
	return false
}
