package main

// report의 sha 칸 확인(2026-11-22 lane-report-sha — 레인이 짧은 해시를 베끼다 오타(8b231930 ≠ 8b231938…)를 내 보고가 그대로
// 받아졌고, 몇 분 뒤 자동 감사가 「완료 sha 가 작업 트리에 없다」로 막혀 지휘가 손으로 찾았다).
// 완료·중간의 sha와 HEAD는 이 작업 트리에서 커밋으로 풀어 전체 해시로 적는다. 못 풀면 사건을 쓰지 않고 종료 2.

import (
	"fmt"
	"os"
	"strings"

	"herdr-tools/internal/gitx"
)

// reportSHA: sha 칸을 풀어 돌려준다(ok=false면 이미 오류 출력을 냈다 — 종료 2).
//   - "HEAD": 어느 상태든 이 작업 트리의 HEAD 커밋 전체 해시(git 밖이면 거절)
//   - 완료·중간의 16진 sha: git 작업 트리 안이면 <sha>^{commit}으로 풀어 전체 해시(없음·여럿에 걸림은 거절). 밖이면 그대로
//   - 완료인데 HEAD가 아니면 경고 한 줄(막지 않음)
func reportSHA(st, sha string) (string, bool) {
	isHead := sha == "HEAD"
	if !isHead && (sha == "-" || (st != "완료" && st != "중간")) {
		return sha, true
	}
	if in, _ := gitx.Out("", nil, "rev-parse", "--is-inside-work-tree"); in != "true" {
		if isHead {
			fmt.Fprintln(os.Stderr, "hc-lane report: HEAD를 풀 수 없다 — git 작업 트리 안에서 부른다")
			return "", false
		}
		return sha, true // 작업 트리 밖(도구가 --name으로 적는 사건) — 옛 꼴 그대로
	}
	head, _ := gitx.Out("", nil, "rev-parse", "-q", "--verify", "HEAD^{commit}")
	full, rc := gitx.Out("", nil, "rev-parse", "-q", "--verify", sha+"^{commit}")
	if rc != 0 || full == "" {
		h := head
		if h == "" {
			h = "(없음)"
		}
		fmt.Fprintf(os.Stderr, "hc-lane report: %s는 이 작업 트리에 없다 — HEAD는 %s\n", sha, h)
		return "", false
	}
	if st == "완료" && full != head {
		more := ""
		if n, rc := gitx.Out("", nil, "rev-list", "--count", full+".."+head); rc == 0 && n != "0" {
			more = ", 그 뒤 커밋 " + n + "개"
		}
		fmt.Fprintf(os.Stderr, "hc-lane report: 경고 — 완료 %s는 HEAD(%s)가 아니다%s. 마지막 커밋이 맞으면 hc-lane report 완료 HEAD …\n",
			cut(full, 12), cut(head, 12), more)
	}
	return strings.ToLower(full), true
}
