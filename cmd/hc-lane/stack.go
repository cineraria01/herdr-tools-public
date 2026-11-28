package main

// 쌓아 띄움(docs/hc-fill/stack.md): 레인 환경 HC_LANE_STACK=<선행>@<P 40자> — 선행이 감사 통과로 제출한 커밋 P 위에서 띄운 레인.
// 이 레인의 범위는 P..HEAD다: 승인 patch-id·제출 기록 fork=·제출 충돌 판정이 merge-base 대신 P를 쓴다. 값이 없으면 오늘 길 그대로.

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"herdr-tools/internal/gitx"
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

// 「지금도 쌓인 레인인가」 한 물음(docs/hc-fill/stack.md 「쌓임이 끝나는 때」) — 승인 범위 시작(forkOf)·제출 충돌 판정 건너뛰기
// (stackSkipConflict)·제출 기록 stack= 칸(stackMeta)이 모두 이 답을 쓴다. 한 실행에 한 번만 묻는다(같은 답·알림 한 줄).
// 띄울 때(stackFrom)는 작업 트리가 P에서 시작하니 늘 쌓임이다
var stk struct {
	key, p string
	rc     int
}

// stackNow <커밋> <기준 참조>: P(지금도 쌓임 — 범위 P..커밋) 또는 ""(환경 없음·선행이 이미 착지해 쌓임이 끝남). P가 커밋의 조상이
// 아니면 선행이 P를 만든 뒤 기본 가지에 들었을 때만 쌓임을 풀고(알림 한 줄), 아직이면 거절(2 — 선행 커밋이 merge-base 범위에 섞인다),
// 모르면 75. rc ≠ 0 = 조상 확인 git 실패(그때 p는 환경 값 그대로)
func stackNow(sha, ref string) (string, int) {
	key := os.Getenv("HC_LANE_STACK") + " " + sha
	if stk.key == key {
		return stk.p, stk.rc
	}
	pred, p := stackEnv()
	rc := 0
	if p != "" {
		if rc = gitRunQ("merge-base", "--is-ancestor", p, sha); rc == 1 {
			if !predLanded(pred, p, ref) {
				eprintf("hc-lane: 쌓아 띄운 레인인데 선행 커밋 %s 가 %s 의 조상이 아니고 선행 %s 이 아직 착지하지 않았다(HC_LANE_STACK) — 이대로 내면 선행 커밋이 이 범위에 섞인다. "+
					"선행 위로 다시 맞추거나(git rebase %s) 선행 착지를 기다린다(착지 표식 refs/hc-landed/%s/<초> 가 원격에 보이면 같은 줄을 다시)\n", cut(p, 8), cut(sha, 8), pred, cut(p, 8), pred)
				exit(2)
			}
			if top := predMixed(sha, p, ref); top != "" {
				eprintf("hc-lane: 쌓아 띄운 레인 — 선행 %s 은 착지했지만 선행 %s 의 커밋이 이 범위에 다시 얹혔다(plain rebase — 착지 때 바뀐 커밋을 git이 건너뛰지 않는다, 가장 위 %s). "+
					"git rebase --onto origin/%s %s 로 그 위 자기 커밋만 옮긴 뒤 같은 줄을 다시(자기 커밋이 선행 커밋 사이에 끼었으면 git rebase -i로 선행 커밋을 drop)\n",
					pred, pred, cut(top, 8), base, cut(top, 8))
				exit(2)
			}
			fmt.Printf("hc-lane: 선행 %s 이 이미 착지해 쌓임을 풀고 기본 가지 기준으로 낸다(선행 커밋 %s 는 이 커밋의 조상이 아님)\n", pred, cut(p, 8))
			p, rc = "", 0
		}
	}
	stk.key, stk.p, stk.rc = key, p, rc
	return p, rc
}

// predLanded <선행> <P> <기준 참조>: 선행이 P를 만든 뒤 기본 가지에 들었나 — 선행 레인 표식 refs/hc-landed/<선행>/<초>(착지 push 뒤
// 병합기·land가 두는, 그 착지 커밋을 가리키는 표식) 가운데 기준에 들었고, P의 조상이 아니고(P를 만들 때 기본 가지에 없었다),
// 표식 시각이 P 커밋 시각 이후인 것이 있으면 예. 같은 이름의 옛 착지는 P의 조상이거나 P보다 앞선 시각이라 세지 않는다.
// 원격·객체를 못 읽으면 모름 — 「들었다」로 읽지 않고 land의 모름 규칙대로 75(같은 줄을 다시)
func predLanded(pred, p, ref string) bool {
	pre := "refs/hc-landed/" + pred + "/"
	ms, st := gitx.LsRemote("", "origin", os.Stderr, pre+"*")
	pt, rc := gitOut("show", "-s", "--format=%ct", p)
	ptime, err := strconv.ParseInt(pt, 10, 64)
	unknown := st == gitx.Unknown || rc != 0 || err != nil
	for n, c := range ms {
		sec, err := strconv.ParseInt(strings.TrimPrefix(n, pre), 10, 64)
		if unknown || err != nil || sec < ptime {
			continue
		}
		in, old := gitRunQ("merge-base", "--is-ancestor", c, ref), gitRunQ("merge-base", "--is-ancestor", c, p)
		if in == 0 && old == 1 {
			return true
		}
		unknown = unknown || in > 1 || old > 1
	}
	if unknown {
		eprintf("hc-lane: 쌓아 띄운 레인 — 선행 %s 이 착지했는지 모른다(착지 표식·객체를 읽지 못함). 큐는 그대로 — 같은 줄을 다시\n", pred)
		exit(75)
	}
	return false
}

// predMixed <커밋> <P> <기준 참조>: 이번 범위(merge-base(커밋, 기준)..커밋)에 다시 얹힌 선행 자신의 커밋(merge-base(P, 기준)..P) 가운데
// 가장 위 것의 sha(없으면 "") — rebase가 바꾸지 않는 작성자 메일·작성 시각·제목으로 견준다(patch-id는 착지 때 문서 접기로 달라져 못 쓴다,
// 감사 atp88r8 중간 1). 그 sha가 거절 글의 --onto 기준이다(P 기준으로는 다시 얹힌 커밋이 안 빠진다, 감사 atp89wu 중간 1). 모르면 75
func predMixed(sha, p, ref string) string {
	rows := func(c string) []string {
		mb, rc := gitOut("merge-base", c, ref)
		lg, rc2 := gitOut("log", "--format=%H %ae %at %s", mb+".."+c)
		if rc != 0 || rc2 != 0 {
			eprintf("hc-lane: 쌓아 띄운 레인 — 범위의 커밋을 읽지 못했다(git 실패). 큐는 그대로 — 같은 줄을 다시\n")
			exit(75)
		}
		return strings.Split(lg, "\n")
	}
	var own []string
	for _, r := range rows(p) {
		if _, k, ok := strings.Cut(r, " "); ok {
			own = append(own, k)
		}
	}
	for _, r := range rows(sha) { // log는 위(새것)부터
		if h, k, ok := strings.Cut(r, " "); ok && slices.Contains(own, k) {
			return h
		}
	}
	return ""
}

// forkOf <커밋> <기준 참조>: 승인 범위의 시작 — 지금도 쌓인 레인이면 P, 아니면 merge-base. rc ≠ 0이면 모름
func forkOf(sha, ref string) (string, int) {
	p, rc := stackNow(sha, ref)
	if rc != 0 {
		return "", rc
	}
	if p != "" {
		return p, 0
	}
	return gitOut("merge-base", sha, ref)
}

// stackSkipConflict <origin/<기본>> <제출 커밋>: 제출 때 충돌 판정(싼 사전 검사)을 건너뛰나 — 지금도 쌓인 제출이고 선행 커밋 P가 아직
// 기본 가지에 없을 때(지휘 t3yx 2026-11-27 안 A: 선행 없이 견주는 판정은 두 번 틀렸다). 그 충돌은 병합기가 후보를 지을 때(--onto <앞 후보> P)
// 검증을 맡기기 전에 본다. P가 들었으면 merge-base가 P라 오늘 판정 그대로 자기 커밋만 견준다. 조상 확인을 못 하면(git 실패) 판정 못 함 길처럼 건너뛴다
func stackSkipConflict(ob, sh string) bool {
	p, _ := stackNow(sh, ob)
	return p != "" && gitRunQ("merge-base", "--is-ancestor", p, ob) != 0
}

// stackMeta <제출 커밋> <기준 참조>: 제출 기록 메시지에 더할 칸(지금도 쌓인 레인만)
func stackMeta(sh, ob string) string {
	if p, _ := stackNow(sh, ob); p != "" {
		pred, _ := stackEnv()
		return " stack=" + pred + "@" + p
	}
	return ""
}

// stackFrom: 띄울 때 선행의 제출 가지(lane/<선행>)를 받아 그 값이 P인지 보고 P(커밋)를 작업 트리 시작으로 돌려준다(--resume이면 주차 가지에서
// — 레인이 P 위로 rebase한다, 지시문 절). 받은 refs/hc-stack/<레인>은 확인에만 쓰고 곧바로 지운다(어느 결과로 끝나도 남기지 않는다 —
// 작업 트리는 커밋 P에서 만들고, 그 뒤 범위는 레인 환경 HC_LANE_STACK이 말한다). 쓰기 레인이 아니면 HC_LANE_STACK을 받지 않는다
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
	if dry {
		fmt.Printf("[dry-run] 쌓아 띄움: 선행 %s 제출 %s 위에서(원격 lane/%s 가 그 커밋일 때만)\n", pred, cut(p, 12), pred)
	} else {
		sref := "refs/hc-stack/" + name
		fr := xfetch(repo, "-q", "origin", "+refs/heads/lane/"+pred+":"+sref)
		v, r := xout("git", "-C", repo, "rev-parse", "-q", "--verify", sref+"^{commit}")
		xrun(nil, nil, "git", "-C", repo, "update-ref", "-d", sref) // 확인에만 썼다 — 받지 못해 없으면 지울 것도 없다
		if fr != 0 {
			die("쌓아 띄움: 선행 제출 가지 lane/%s 를 받지 못했다: %s", pred, repo)
		}
		if r != 0 || v != p {
			refuse("쌓아 띄움: 선행 제출 가지 lane/%s 가 %s 가 아니다(%s) — 선행이 다시 냈거나 들어갔다, 다음 바퀴에: %s", pred, cut(p, 12), cut(v, 12), repo)
		}
	}
	if resume {
		return wfrom
	}
	return p
}
