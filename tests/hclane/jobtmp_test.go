package hclane_test

// 작업별 임시 폴더(ht-job-tmpdir, docs/hc-sweep/job-tmp.md): hc-lane이 띄우는 레인·--read 감사는 env 뒤에 감싸개
// (sh -c <jobtmp> jobtmp <뿌리> <이름> exec <실행 명령…>)를 거친다. 감싸개 자체의 동작(TMPDIR·표식·길이)은 internal/jobtmp 시험

import (
	"strings"

	"herdr-tools/internal/jobtmp"
)

func init() { reg(96, "jobtmp", jobTmpLane) }

func jobTmpLane(k *K) {
	// 감싸개 줄(가짜 hc는 인자를 줄마다 적는다 — 감싸개는 한 줄)의 번호, 없으면 0
	head := func() int {
		for i, l := range k.hclog("") {
			if strings.HasPrefix(l, "r=$1 n=$2 m=$3; shift 3;") {
				return i + 1
			}
		}
		return 0
	}
	wrapped := func(name string) bool {
		h := head()
		return h > 0 && k.pair("jobtmp", jobtmp.Default) && k.pair(jobtmp.Default, name) && k.pair(name, "exec") && k.pair("exec", "claude") &&
			k.line("LANE_REPO="+k.R) < h && h < k.line("claude")
	}
	k.mk("j1")
	k.run("--name", "lane-j", "--repo", k.R, "--", "claude", "-p", "일")
	k.check("레인: env 뒤 감싸개(exec 꼴, 기본 뿌리·레인 이름) 다음 실행 명령", k.rc == 0 && wrapped("lane-j"))

	k.mk("j2")
	k.run("--read", "--name", "audit-j", "--repo", k.R, "--", "claude", "-p", "감사")
	k.check("--read 감사도 감싸개를 거친다", k.rc == 0 && wrapped("audit-j"))

	k.mk("j3")
	k.conf("jobtmp.root off")
	k.run("--name", "lane-off", "--repo", k.R, "--", "claude")
	k.check("jobtmp.root off면 지금 그대로(감싸지 않음)", k.rc == 0 && head() == 0 && !k.has("jobtmp") && k.has("claude"))
}
