package tests

// gofix4-hooks(2026-11-23 Go 전환 재감사 바퀴 3): 고치기 전 코드에서 실패하는 시험. 지적 ID는 재감사 보고서의 것.

import "testing"

// R-3A01: 대소문자만 바꾼 이름 변경 뒤에도 옛 철자 글롭이 산 주인 A의 미커밋 파일(새 철자·옛 철자 둘 다)에 맞아 막힌다
func TestCotenantCaseRenameGlob(t *testing.T) {
	for _, cmd := range []string{"git add 'N*.txt'", "git commit -m x -- 'Docs/*.md'", "git add 'D*'"} {
		t.Run(cmd, func(t *testing.T) {
			c := newCG(t)
			R, A, _ := repoAB(c)
			c.write(R+"/N.txt", "n\n")
			c.write(R+"/Docs/x.md", "x\n")
			c.git(R, "add", "N.txt", "Docs/x.md")
			c.git(R, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "docs")
			L := c.leaseDirs(R)
			st := procStart(c, A)
			c.rec(L, "N.txt", A, st, false)
			c.rec(L, "Docs/x.md", A, st, false)
			c.git(R, "mv", "N.txt", "n.txt")   // A가 대소문자만 바꿔 이름을 바꿨다(미커밋)
			c.git(R, "mv", "Docs", "docs.tmp") // 대소문자를 무시하는 파일시스템은 폴더 이름을 한 번에 못 바꾼다
			c.git(R, "mv", "docs.tmp", "docs")
			c.run("sb", "Bash", cmd)
			if c.RC != 2 {
				t.Errorf("rc=%d(want 2 — A의 미커밋 이름 변경) err=[%s]", c.RC, c.ERR)
			}
		})
	}
}
