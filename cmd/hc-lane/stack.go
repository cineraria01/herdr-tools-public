package main

// 쌓아 띄움(docs/hc-fill/stack.md): 레인 환경 HC_LANE_STACK=<선행>@<P 40자> — 선행이 감사 통과로 제출한 커밋 P 위에서 띄운 레인.
// 이 레인의 범위는 P..HEAD다: 승인 patch-id·제출 기록 fork=·제출 충돌 판정이 merge-base 대신 P를 쓴다. 값이 없으면 오늘 길 그대로.

import (
	"fmt"
	"os"
	"strings"
)

// stackEnv: HC_LANE_STACK 값(선행·P). 없으면 빈 값, 꼴이 틀리면 멈춘다(모르는 기준으로 범위를 정하지 않는다)
func stackEnv() (pred, p string) {
	v := os.Getenv("HC_LANE_STACK")
	if v == "" {
		return "", ""
	}
	pred, p, ok := strings.Cut(v, "@")
	if !ok || !laneNameOK(pred) || !sha40.MatchString(p) {
		die("HC_LANE_STACK 꼴은 <선행 레인>@<40자 SHA>: %s", v)
	}
	return pred, p
}

func laneNameOK(l string) bool {
	return l != "" && l[0] != '-' && len(l) <= 32 && strings.Trim(l, "abcdefghijklmnopqrstuvwxyz0123456789_-") == ""
}

// forkOf <커밋> <기준 참조>: 승인 범위의 시작 — 쌓인 레인이면 P(커밋의 조상이어야 한다), 아니면 merge-base. rc ≠ 0이면 모름
func forkOf(sha, ref string) (string, int) {
	if _, p := stackEnv(); p != "" {
		switch rc := gitRunQ("merge-base", "--is-ancestor", p, sha); rc {
		case 0:
			return p, 0
		case 1:
			eprintf("hc-lane: 쌓아 띄운 레인인데 선행 커밋 %s 가 %s 의 조상이 아니다(HC_LANE_STACK) — 선행 위로 rebase하지 않았다\n", cut(p, 8), cut(sha, 8))
			exit(2)
		default:
			return "", rc
		}
	}
	return gitOut("merge-base", sha, ref)
}

// stackSkipConflict <origin/<기본>>: 제출 때 충돌 판정(싼 사전 검사)을 건너뛰나 — 쌓인 제출이고 선행 커밋 P가 아직 기본 가지에 없을 때
// (지휘 t3yx 2026-11-27 안 A: 선행 없이 견주는 판정은 두 번 틀렸다). 그 충돌은 병합기가 후보를 지을 때(--onto <앞 후보> P) 검증을 맡기기
// 전에 본다. P가 들었으면 merge-base가 P라 오늘 판정 그대로 자기 커밋만 견준다. 조상 확인을 못 하면(git 실패) 판정 못 함 길처럼 건너뛴다
func stackSkipConflict(ob string) bool {
	_, p := stackEnv()
	return p != "" && gitRunQ("merge-base", "--is-ancestor", p, ob) != 0
}

// stackMeta: 제출 기록 메시지에 더할 칸(쌓인 레인만)
func stackMeta() string {
	if pred, p := stackEnv(); p != "" {
		return " stack=" + pred + "@" + p
	}
	return ""
}

// stackFrom: 띄울 때 선행의 제출 가지(lane/<선행>)를 받아 그 값이 P일 때 그 참조에서 시작한다(--resume이면 주차 가지에서 — 레인이 P 위로
// rebase한다, 지시문 절). 돌려준 값은 작업 트리 시작 참조. 쓰기 레인이 아니면 HC_LANE_STACK을 받지 않는다
func stackFrom(wfrom string) string {
	pred, p := stackEnv()
	if p == "" {
		return wfrom
	}
	if read {
		die("HC_LANE_STACK은 쓰기 레인에만")
	}
	if !remote {
		refuse("쌓아 띄움: 원격(origin)이 없는 저장소에는 선행 제출이 없다: %s", repo)
	}
	sref := "refs/hc-stack/" + name
	switch {
	case dry:
		fmt.Printf("[dry-run] 쌓아 띄움: 선행 %s 제출 %s 위에서\n", pred, cut(p, 12))
		return wfrom
	case xfetch(repo, "-q", "origin", "+refs/heads/lane/"+pred+":"+sref) != 0:
		die("쌓아 띄움: 선행 제출 가지 lane/%s 를 받지 못했다: %s", pred, repo)
	}
	if v, r := xout("git", "-C", repo, "rev-parse", "-q", "--verify", sref+"^{commit}"); r != 0 || v != p {
		refuse("쌓아 띄움: 선행 제출 가지 lane/%s 가 %s 가 아니다(%s) — 선행이 다시 냈거나 들어갔다, 다음 바퀴에: %s", pred, cut(p, 12), cut(v, 12), repo)
	}
	if resume {
		return wfrom
	}
	return sref
}
