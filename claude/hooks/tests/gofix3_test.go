package tests

// gofix3-hooks(2026-11-22 Go 전환 재감사 아스트라 2바퀴 지적): 고치기 전 코드에서 실패하는 시험. 지적 ID는 재감사 보고서의 것(R-H2xx·R-M2xx).
// 기대값은 지적 원문과 사건 단일성 계약으로 정했다. 시험마다 저장소·에이전트를 따로 만든다.
// 이 레인의 몫은 R-H203·R-M201·R-M210(Mythos 판정, 지휘 2026-11-23) — 나머지 지적의 재현 시험은 고침과 함께 뺐다.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// R-H203: 옛 판이 적은 철자(대소문자·NFD)의 임대도 두 조회 경로(정리자의 경로 반환, 바뀐 경로 조회) 모두에서 같은 파일로 찾는다
func TestCotenantLegacyKeyLookup(t *testing.T) {
	t.Run("정리자 경로는 색인 철자", func(t *testing.T) {
		c := newCG(t)
		R, A, _ := repoAB(c)
		c.rec(c.leaseDirs(R), "SUB/C.TXT", A, procStart(c, A), false)
		c.appendf(R+"/sub/c.txt", "y\n")
		c.run("sb", "Bash", "git add sub")
		if c.RC != 2 {
			t.Errorf("rc=%d(want 2 — A의 옛 철자 임대 sub/c.txt) err=[%s]", c.RC, c.ERR)
		}
	})
	t.Run("정리자를 못 잡을 때 옛 철자", func(t *testing.T) {
		c := newCG(t)
		R, A, _ := repoAB(c)
		L := c.leaseDirs(R)
		c.rec(L, "SUB/C.TXT", A, procStart(c, A), false)
		c.write(L+"/.cleaning", "") // 다른 훅이 지금 정리 중 — 바뀐 경로의 git 철자 열쇠(sub/c.txt)만 보면 옛 철자 임대를 놓친다
		c.appendf(R+"/sub/c.txt", "y\n")
		c.run("sb", "Bash", "git add -A")
		if c.RC != 2 {
			t.Errorf("rc=%d(want 2 — A의 옛 철자 임대 sub/c.txt) err=[%s]", c.RC, c.ERR)
		}
	})
	t.Run("뿌리를 다른 대소문자로 적은 편집도 옛 열쇠", func(t *testing.T) {
		c := newCG(t)
		R, A, _ := repoAB(c)
		c.rec(c.leaseDirs(R), "A.TXT", A, procStart(c, A), false) // 산 A가 방금 손댄(깨끗한) a.txt — 옛 판이 적은 철자 A.TXT
		up := filepath.Join(filepath.Dir(R), strings.ToUpper(filepath.Base(R)))
		c.run("sb", "Edit", up+"/A.TXT")
		if c.RC != 2 || exists(c.leasefile(R, "a.txt")) {
			t.Errorf("rc=%d(want 2 — A의 옛 열쇠 A.TXT), a.txt 새 임대 %v(want 없음) err=[%s]", c.RC, exists(c.leasefile(R, "a.txt")), c.ERR)
		}
	})
}

// R-M201: 여러 대상 Bash에서 둘째 대상이 거절되면 첫 대상의 인계(끝난 주인·멈춘 첫 획득)를 세대 확인 뒤 이전 상태로 되돌린다
func TestCotenantBatchRecovery(t *testing.T) {
	setup := func(t *testing.T) (*cg, string) {
		c := newCG(t)
		R, _, _ := repoAB(c)
		// b.txt는 다른 훅이 지금 잡는 중(주인 없는 새 임대 폴더) — 분석 단계의 확인(check)은 지나고, 둘째 대상 임대에서 거절된다
		c.run("sb", "Edit", R+"/d.txt")
		c.fatal(os.Mkdir(c.leaseDirs(R)+"/"+md5hex("b.txt"), 0o755))
		return c, R
	}
	t.Run("끝난 주인의 임대", func(t *testing.T) {
		c, R := setup(t)
		D := c.agent()
		st := procStart(c, D)
		ld := c.rec(c.leaseDirs(R), "a.txt", D, st, true)
		c.killAgent(D)
		before, err := os.ReadFile(ld + "/owner")
		c.fatal(err)
		c.run("sa", "Bash", "tee a.txt b.txt </dev/null")
		after, _ := os.ReadFile(ld + "/owner")
		if c.RC != 2 || string(after) != string(before) {
			t.Errorf("rc=%d(want 2), a.txt 주인 %q(want 옛 주인 그대로 %q) err=[%s]", c.RC, after, before, c.ERR)
		}
	})
	t.Run("멈춘 첫 획득", func(t *testing.T) {
		c, R := setup(t)
		ld := c.leaseDirs(R) + "/" + md5hex("a.txt")
		c.fatal(os.Mkdir(ld, 0o755))
		c.touchOld(ld)
		c.run("sa", "Bash", "tee a.txt b.txt </dev/null")
		if c.RC != 2 || exists(ld+"/owner") {
			t.Errorf("rc=%d(want 2), a.txt 주인 %v(want 없음) err=[%s]", c.RC, exists(ld+"/owner"), c.ERR)
		}
	})
}
