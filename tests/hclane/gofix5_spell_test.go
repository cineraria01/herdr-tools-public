package hclane_test

// gofix5-sweep(2026-11-23 Go 전환 재감사 바퀴 4): lane/R-4A02 — 「그 작업 트리를 살아 있는 터미널이 쓰는가」를 cwd 글자로만 견줬다.
// macOS 기본 파일 시스템은 대소문자·한글 정규화를 가리지 않아 다른 철자가 같은 폴더다(hc는 띄울 때 준 글자 그대로 돌려준다).
// git이 등록한 철자와 다르게 띄운 live 터미널이 있어도 gc·clean이 「아무도 안 씀」으로 읽고 그 작업 트리를 지웠다

import (
	"fmt"
	"strings"
	"testing"

	"herdr-tools/internal/clitest"
)

func TestGofix5LaneOtherSpelling(t *testing.T) {
	withK(t, func(k *K) {
		k.mk("g5sp")
		k.seed(".gitignore", ".omc/\n.qa/")
		k.git(k.R, "pull", "-q", "--ff-only")
		ls := func(addr, cwd string) string {
			return fmt.Sprintf(`HC_FAKE_LS={"terminals":[{"address":"%s","agent":"claude-code","cwd":"%s","state":"live"}]}`, addr, cwd)
		}
		for _, c := range []struct {
			br  string
			nfd bool
		}{{"g-case", false}, {"g-한글", true}} {
			s19gw(k, c.br)
			wt := k.R + ".wt/" + c.br
			k.runE([]string{ls("t5", clitest.AltSpelling(k.t, wt, c.nfd))}, "gc", "--repo", k.R, "--min-age", "0")
			k.check("R-4A02 gc "+c.br+": 다른 철자로 띄운 live 터미널 → 작업 트리 남김(주소)", k.rc == 0 && s19gwt(k, c.br) && isDir(wt) &&
				s19lineHas(k, "남김 "+wt, "t5"))
		}
		s19gw(k, "g-same")
		k.runE([]string{ls("t5", k.R+".wt/g-same")}, "gc", "--repo", k.R, "--min-age", "0")
		k.check("R-4A02 gc: 대조 — 같은 철자도 남김", s19gwt(k, "g-same") && s19lineHas(k, "남김 "+k.R+".wt/g-same", "t5"))

		k.land("c-case")
		wt := k.R + ".wt/c-case"
		k.runE([]string{ls("t7", clitest.AltSpelling(k.t, wt, false))}, "clean", "--repo", k.R, "--branch", "c-case")
		k.check("R-4A02 clean: 다른 철자로 띄운 live 터미널 → 종료 1·그대로", k.rc == 1 && k.kept("c-case") && k.regd("c-case") &&
			strings.Contains(k.errf(), "t7"))
		k.runE([]string{ls("t7", "/nonexistent-g5/c-case")}, "clean", "--repo", k.R, "--branch", "c-case")
		k.check("R-4A02 clean: 대조 — 다른 폴더 터미널은 막지 않음", k.rc == 0 && k.gone("c-case"))
	})
}
