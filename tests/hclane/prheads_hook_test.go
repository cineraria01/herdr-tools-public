package hclane_test

// PR 머리 정보를 저장소 착지 스크립트에(2026-11-24 pr-land-hook, docs/hc-fill/pr.md 「착지 스크립트 계약」): 병합기 안 착지가
// .claude/herdr-land.sh에 맡기면 HERDR_LAND_PR_HEADS를 그대로 넘긴다(스크립트가 머리 가지를 올린다). 병합기 밖 착지와 내장 길의
// 검증 자식에는 넘기지 않는다. 도우미 hc-lane land-prheads는 내장 길과 같은 판정으로 push 인자를 찍는다.
// stale_land_test.go의 머리(stlNewK·stlMk·stlSd·stlLane·stlLnd)를 그대로 쓴다.

import (
	"strings"
	"testing"
)

// 스크립트가 받은 값을 찍고 stlLandNew처럼 착지한다.
const phLand = `echo "pr-heads ${HERDR_LAND_PR_HEADS-없음}"
` + stlLandNew

func TestLandPRHeadsHook(t *testing.T) {
	k := stlNewK(t)
	phRun(k)
	phHelper(k)
	t.Logf("%d개 중 실패 %d개", k.n, k.fails)
}

// phGot: 출력에서 pre 로 시작하는 줄의 나머지. 없으면 「(줄 없음)」.
func (k *K) phGot(pre string) string {
	for _, l := range strings.Split(k.out, "\n") {
		if v, ok := strings.CutPrefix(l, pre); ok {
			return v
		}
	}
	return "(줄 없음)"
}

func phRun(k *K) {
	// 1. 병합기 안 착지·저장소 착지 스크립트: 값을 그대로(구성원 둘, 빈칸으로)
	k.stlMk("a")
	k.stlSd(".claude/herdr-verify.sh", "echo verify-ran")
	k.stlSd(".claude/herdr-land.sh", phLand)
	k.stlLane()
	sub := k.git(k.D+"/l", "rev-parse", "HEAD")
	seen := k.git(k.D+"/o.git", "rev-parse", "main")
	v := "x=" + sub + "=" + seen + " y=" + seen + "=" + sub
	k.stlLnd("HERDR_MERGER=1", "HERDR_LAND_PR_HEADS="+v)
	k.check("병합기 안 착지 스크립트: HERDR_LAND_PR_HEADS 그대로", k.rc == 0 && k.phGot("pr-heads ") == v)

	// 2. 병합기 밖 직접 착지(스크립트): 넘기지 않는다
	k.stlMk("b")
	k.stlSd(".claude/herdr-land.sh", phLand)
	k.stlLane()
	sub = k.git(k.D+"/l", "rev-parse", "HEAD")
	k.stlLnd("HERDR_LAND_PR_HEADS=x=" + sub + "=" + sub)
	k.check("병합기 밖 착지 스크립트: HERDR_LAND_PR_HEADS 없음", k.rc == 0 && k.phGot("pr-heads ") == "없음")

	// 3. 내장 길(병합기 안): 지금처럼 읽고 지운다 — 검증 자식에 없음
	k.stlMk("c")
	k.stlSd(".claude/herdr-verify.sh", `echo "verify-heads ${HERDR_LAND_PR_HEADS-없음}"`)
	k.stlLane()
	sub = k.git(k.D+"/l", "rev-parse", "HEAD")
	seen = k.git(k.D+"/o.git", "rev-parse", "main")
	k.stlLnd("HERDR_MERGER=1", "HERDR_LAND_PR_HEADS=x="+sub+"="+seen)
	k.check("내장 길: 검증 자식에 HERDR_LAND_PR_HEADS 없음", k.rc == 0 && k.phGot("verify-heads ") == "없음")
}

// phHelper: land-prheads — 구성원 둘이 rebase로 sha가 바뀐 후보에서 두 줄·본 값 lease, 못 찾은 구성원은 건너뜀(오류 출력),
// 이미 그 값인 구성원은 줄 없음. 값이 없으면 줄 없이 0, 후보를 못 읽으면 1.
func phHelper(k *K) {
	h := k.T + "/h"
	k.sh(k.T, `git init -q -b main h && cd h && echo 0 >b && git add b && git commit -qm b0 &&
git checkout -qb other && echo o >o && git add o && git commit -qm other &&
git checkout -q main && git checkout -qb sub && echo 1 >m1 && git add m1 && git commit -qm m1 && echo 2 >m2 && git add m2 && git commit -qm m2`)
	sub1, sub2 := k.git(h, "rev-parse", "sub~1"), k.git(h, "rev-parse", "sub")
	miss, seen := k.git(h, "rev-parse", "other"), k.git(h, "rev-parse", "main")
	k.sh(h, `git checkout -q main && echo 1 >b && git commit -qam b1 && git checkout -q sub && git rebase -q main`)
	new1, new2 := k.git(h, "rev-parse", "HEAD~1"), k.git(h, "rev-parse", "HEAD")
	v := "a=" + sub1 + "=" + seen + " b=" + sub2 + "=" + seen + " c=" + miss + "=" + seen + " d=" + sub2 + "=" + new2
	k.runIn(h, []string{"HERDR_LAND_PR_HEADS=" + v}, "land-prheads")
	want := new1 + ":refs/heads/pr/a --force-with-lease=refs/heads/pr/a:" + seen + "\n" +
		new2 + ":refs/heads/pr/b --force-with-lease=refs/heads/pr/b:" + seen
	k.check("land-prheads: rebase된 끝 커밋 두 줄·본 값 lease", k.rc == 0 && new1 != sub1 && k.out == want)
	k.check("land-prheads: 못 찾은 구성원은 건너뜀(오류 출력 한 줄)", strings.Contains(read(k.T+"/err"), "pr/c 건너뜀"))

	k.runIn(h, nil, "land-prheads")
	k.check("land-prheads: 값 없음 → 줄 없이 0", k.rc == 0 && k.out == "")
	k.runIn(h, []string{"HERDR_LAND_PR_HEADS=" + v}, "land-prheads", "no-such-rev")
	k.check("land-prheads: 후보를 못 읽음 → 1(모름)", k.rc == 1 && k.out == "")
}
