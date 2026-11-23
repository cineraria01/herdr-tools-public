package main

// 제출의 사전 검사 건너뜀(submit-fast, 2026-11-24): 검증 본문(tests/repo/verify)이 이 작업 트리에서 전체 검증을 통과로 끝냈을 때 남긴
// 표식(이 작업 트리의 git 폴더 herdr-verified-tree: 「<트리> <유닉스 초>」)의 트리가 제출 커밋의 트리와 같으면 사전 검사를 다시 돌리지 않는다.
// 표식은 병합기·착지가 읽지 않는다(줄에 든 것은 그대로 병합기·미리 검증이 검증한다). 손으로 만든 표식으로 잃는 것은 「깨진 것이 줄에
// 들어가 병합기가 반송한다」뿐이다. HC_LANE_PRECHECK_ALWAYS=1이면 늘 돌린다

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var hexTree = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)

// verifiedTree <제출 커밋>: 표식이 그 커밋의 트리면 (통과 시각, true). 표식 없음·읽기 실패·깨짐·다른 트리·손잡이 켬이면 false
func verifiedTree(sh string) (string, bool) {
	if os.Getenv("HC_LANE_PRECHECK_ALWAYS") == "1" {
		return "", false
	}
	p, rc := gitOutQ("rev-parse", "--path-format=absolute", "--git-path", "herdr-verified-tree")
	if rc != 0 || p == "" {
		return "", false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", false
	}
	f := strings.Fields(string(b))
	if len(f) != 2 || !hexTree.MatchString(f[0]) {
		return "", false
	}
	secs, err := strconv.ParseInt(f[1], 10, 64)
	if err != nil {
		return "", false
	}
	t, rc := gitOutQ("rev-parse", "-q", "--verify", sh+"^{tree}")
	if rc != 0 || t != f[0] {
		return "", false
	}
	return time.Unix(secs, 0).Format("2006-01-02 15:04:05"), true
}
