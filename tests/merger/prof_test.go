package merger_test

// 원래 PR 잇기(2026-11-24 fill-polish-1124, docs/hc-fill/pr.md 「다른 이름으로 다시 내는 레인」): 다른 이름(alpha-fix)으로 다시 낸 레인에
// 짝 참조 refs/herdr/pr-of/alpha-fix/alpha가 있으면 병합기가 원래 머리 가지 pr/alpha를 그 레인의 착지 커밋으로 올린다(원래 PR이 Merged).
// 고치기 전: 병합기가 pr/alpha-fix만 찾아 머리를 안 올렸고, 착지 글에 「PR 머리」가 없었다

import (
	"strings"
	"testing"
)

func TestPROfHead(t *testing.T) {
	k := newRK(t)

	t.Run("짝 참조 — 원래 머리 가지 pr/alpha가 착지 커밋을 담음", func(t *testing.T) {
		f := k.fixture("pr-of")
		f.enable()
		f.commitFile("alpha", "first")
		prMade(f, "alpha") // 원래 레인 alpha의 PR 머리
		// hc-fill이 alpha-fix 완료 때 레인 기계에서 올리는 짝
		f.w(`git push -q origin "HEAD:refs/herdr/pr-of/alpha-fix/alpha"`)
		r := f.submit("alpha-fix")
		k.check("제출 종료 0", r.RC == 0, clog(r))
		advance(f)
		prHook(f)
		r = f.tick()
		k.check("착지", f.og("show", "main:alpha") == "first", clog(r))
		k.check("pr/alpha가 기본 가지보다 먼저", recvAt(f, "refs/heads/pr/alpha") >= 0 && recvAt(f, "refs/heads/pr/alpha") < recvAt(f, "refs/heads/main"), read(f.home+"/recv.log"))
		k.check("pr/alpha = 착지 커밋", f.og("rev-parse", "refs/heads/pr/alpha") == f.og("rev-parse", "main"), f.og("for-each-ref"))
		k.check("pr/alpha-fix 안 만듦", !f.ogOK("rev-parse", "-q", "--verify", "refs/heads/pr/alpha-fix"), f.og("for-each-ref"))
		m := landMsg(f, "alpha-fix")
		k.check("착지 글에 「PR 머리 … 들어감」", strings.Contains(m, "PR 머리 ") && strings.Contains(m, " 들어감"), m)
	})

	t.Run("짝 참조 없음 — 전과 같다(머리 없음, 글에 PR 머리 없음)", func(t *testing.T) {
		f := k.fixture("pr-of-none")
		f.enable()
		f.commitFile("alpha", "first")
		prMade(f, "alpha")
		f.submit("alpha-fix")
		advance(f)
		prHook(f)
		r := f.tick()
		k.check("착지", f.og("show", "main:alpha") == "first", clog(r))
		k.check("pr 참조 push 없음", recvAt(f, "refs/heads/pr/alpha") < 0, read(f.home+"/recv.log"))
		k.check("착지 글에 PR 머리 없음", !strings.Contains(landMsg(f, "alpha-fix"), "PR 머리"), landMsg(f, "alpha-fix"))
	})
}
