package hclane_test

// gofix4-lane(2026-11-23 Go 전환 재감사 바퀴 3): R-3A20 — gc도 clean과 같은 기준(어느 깊이든 .qa 폴더 + check-ignore)으로 무시된 보관물을 찾는다.

import (
	"strings"
	"testing"
)

func TestGofix4Qa(t *testing.T) {
	withK(t, func(k *K) {
		k.mk("g4qa")
		k.seed(".gitignore", ".omc/\n.qa/\nbuild/")
		k.git(k.R, "pull", "-q", "--ff-only")
		for _, b := range []string{"q-nest", "q-direct", "q-plain"} {
			s19gw(k, b)
		}
		write(k.t, k.R+".wt/q-nest/build/.qa/result.txt", "n\n") // 무시된 폴더 안 — git status --ignored 는 build/ 하나로 접는다
		write(k.t, k.R+".wt/q-direct/.qa/result.txt", "d\n")
		write(k.t, k.R+".wt/q-plain/build/out.txt", "p\n") // .qa 없는 무시 파일은 같이 지운다
		k.run("gc", "--repo", k.R, "--min-age", "0")
		k.check("R-3A20 gc: 무시된 폴더 안 .qa → 작업 트리·파일 남김과 「무시된 보관물」", k.rc == 0 && s19gwt(k, "q-nest") &&
			s13cat(k.R+".wt/q-nest/build/.qa/result.txt") == "n" && s19lineHas(k, "남김 "+k.R+".wt/q-nest", "무시된 보관물 build/.qa"))
		k.check("R-3A20 gc: 대조 — 바로 아래 .qa 도 남김", s19gwt(k, "q-direct") && s13cat(k.R+".wt/q-direct/.qa/result.txt") == "d" &&
			s19lineHas(k, "남김 "+k.R+".wt/q-direct", "무시된 보관물 .qa"))
		k.check("R-3A20 gc: .qa 없는 무시 폴더만이면 지움", !s19gwt(k, "q-plain") && !exists(k.R+".wt/q-plain"))
		// 탐색 실패는 모름 — 지우지 않고 종료 1
		p := k.gfBin(map[string]string{"find": `case $1 in */.qa) exit 1 ;; esac; exec /usr/bin/find "$@"`})
		k.runE([]string{p}, "gc", "--repo", k.R, "--min-age", "0")
		k.check("R-3A20 gc: .qa 탐색 실패면 남기고 종료 1", k.rc == 1 && s19gwt(k, "q-nest") && s19gwt(k, "q-direct") &&
			s19lineHas(k, "남김 "+k.R+".wt/q-nest", "확인하지 못함") && !strings.Contains(k.out, "지움 "+k.R))
	})
}
