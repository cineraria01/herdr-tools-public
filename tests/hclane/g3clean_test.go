package hclane_test

// gofix3-lane-clean(2026-11-22 Go 전환 재감사 2바퀴 — 아스트라 6.2 max, 감사 HEAD 1eccd73): hc-lane clean 지적(R-207)을
// 고치기 전 Go 판에서 실패하는 시험. 기대값은 지적 원문과 계약(모름은 상태를 안 바꾼다 — 지우지 않고 멈춘다)으로 정했다.
// R-201·203·204·205·219·220은 재감사 확인에서 뺀 지적(한계 목록으로)이라 시험도 두지 않는다.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestG3LaneClean(t *testing.T) {
	withK(t, func(k *K) { g3CleanRecheck(k) })
}

// R-207: 처음 조회에는 터미널이 없다가 .qa 보관(복사) 중에 그 작업 트리에 터미널이 다시 붙는다 — 제거 직전 재조회로 보고 남긴다
func g3CleanRecheck(k *K) {
	k.mk("g3207")
	appendf(k.t, k.R+"/.git/info/exclude", ".qa/\n")
	k.land("g3207l")
	os.Remove(k.wt + "/.omc/state/s.json")
	write(k.t, k.wt+"/.qa/r/R.md", "r\n")
	live := fmt.Sprintf(`{"machines":[],"terminals":[{"address":"t7rc","agent":"claude-code","cwd":"%s","state":"live"}]}`, k.wt)
	lf := filepath.Join(k.T, "g3207-ls")
	write(k.t, lf, live+"\n")
	cpp, err := exec.LookPath("cp")
	if err != nil {
		k.t.Fatal(err)
	}
	os.Remove(k.HCLOG + ".lsfile")
	// 가짜 cp는 진짜 cp를 경로로 부른다(이름으로 부르면 자기 자신 — 끝없는 재귀)
	p := k.gfBin(map[string]string{"cp": sq(cpp) + ` ` + sq(lf) + ` "$HCLOG.lsfile" 2>/dev/null; exec ` + sq(cpp) + ` "$@"`})
	k.runE([]string{p}, "clean", "--repo", k.R, "--branch", "g3207l")
	k.check("R-207 준비: 보관 복사(cp)가 돌아 그 뒤 목록에 터미널이 보인다", exists(k.HCLOG+".lsfile"))
	os.Remove(k.HCLOG + ".lsfile")
	k.check("R-207 clean: 보관 중 다시 붙은 터미널을 제거 직전에 보고 멈춘다(작업 트리 보존)", k.rc != 0 && k.kept("g3207l") && isDir(k.wt))
}
