package hclane_test

// 착지 훅 판 넘기기(repo-go-plan §2 R0, 지휘 t1cj 2026-11-22): hc-lane land가 저장소 착지 스크립트(.claude/herdr-land.sh)를 부를 때
// HERDR_LAND_HOOK_REV=<지금 실행하는 그 스크립트 blob이 나온 커밋>을 넘긴다 — 직접 착지는 HEAD(먼저 rebase했으면 그 뒤 HEAD),
// 병합기 안 착지는 기본 브랜치 커밋. 저장소 착지 스크립트는 이 커밋의 트리로 도구를 짓는다(작업 트리 편집이 착지 판에 안 섞이게).
// 호출자 환경에 같은 이름이 있어도 덮어쓴다. stale_land_test.go의 머리(stlNewK·stlMk·stlSd·stlLane)를 그대로 쓴다.

import (
	"strings"
	"testing"
)

// 스크립트가 받은 값과 자기 blob을 찍고 stlLandNew처럼 착지한다.
const hrLand = `echo "hook-rev ${HERDR_LAND_HOOK_REV-없음} $(git hash-object "$0")"
` + stlLandNew

func TestLandHookRev(t *testing.T) {
	k := stlNewK(t)
	hrRun(k)
	t.Logf("%d개 중 실패 %d개", k.n, k.fails)
}

// hrGot: 출력의 「hook-rev <값> <blob>」 줄. 없으면 빈 값.
func (k *K) hrGot() (rev, blob string) {
	for _, l := range strings.Split(k.out, "\n") {
		if f := strings.Fields(l); len(f) == 3 && f[0] == "hook-rev" {
			return f[1], f[2]
		}
	}
	return "", ""
}

// hrOK: 받은 값이 want 커밋이고, 그 커밋의 .claude/herdr-land.sh blob이 실제로 돈 스크립트와 같다.
func (k *K) hrOK(want string) bool {
	rev, blob := k.hrGot()
	return k.rc == 0 && rev == want && blob != "" && k.git(k.D+"/o.git", "rev-parse", "-q", "--verify", rev+":.claude/herdr-land.sh") == blob
}

func hrRun(k *K) {
	// 1. 직접 착지: 값은 HEAD(호출자 환경의 같은 이름은 덮어씀)
	k.stlMk("a")
	k.stlSd(".claude/herdr-land.sh", hrLand)
	k.stlLane()
	head := k.git(k.D+"/l", "rev-parse", "HEAD")
	k.stlLnd("HERDR_LAND_HOOK_REV=junk")
	k.check("직접 착지: HERDR_LAND_HOOK_REV = HEAD, 그 커밋의 스크립트가 돈 판", k.hrOK(head))

	// 2. 직접 착지·낡은 스크립트: 먼저 rebase한 뒤 HEAD(옛 HEAD가 아니다)
	k.stlMk("b")
	k.stlSd(".claude/herdr-land.sh", stlLandOld)
	k.stlLane()
	old := k.git(k.D+"/l", "rev-parse", "HEAD")
	k.stlSd(".claude/herdr-land.sh", hrLand)
	k.stlLnd()
	rev, _ := k.hrGot()
	k.check("먼저 rebase한 직접 착지: 값은 rebase 뒤 HEAD", k.hrOK(rev) && rev != old && k.git(k.D+"/l", "rev-parse", "HEAD") == rev)

	// 3. 병합기 안 착지: 값은 기본 브랜치 커밋 — 가지가 고친 스크립트의 커밋이 아니다
	k.stlMk("c")
	k.stlSd(".claude/herdr-verify.sh", "echo verify-ran")
	k.stlSd(".claude/herdr-land.sh", hrLand)
	k.stlLane("echo land-lane", ".claude/herdr-land.sh")
	base := k.git(k.D+"/o.git", "rev-parse", "main")
	k.stlLnd("HERDR_MERGER=1", "HERDR_LAND_HOOK_REV=junk")
	k.check("병합기 안 착지: HERDR_LAND_HOOK_REV = 기본 브랜치 커밋, 그 판 스크립트", k.hrOK(base) && !strings.Contains(k.out, "land-lane"))

	// 4. 전용 스크립트 없는 저장소(내장 길): 검증 스크립트에 호출자 값을 물려주지 않는다(이 값을 안 읽는 저장소는 그대로)
	k.stlMk("d")
	k.stlSd(".claude/herdr-verify.sh", `echo "verify-rev ${HERDR_LAND_HOOK_REV-없음}"`)
	k.stlLane()
	k.stlLnd("HERDR_LAND_HOOK_REV=junk")
	k.check("내장 길: 검증 스크립트에 HERDR_LAND_HOOK_REV 없음", k.rc == 0 && contains(strings.Split(k.out, "\n"), "verify-rev 없음"))
}
