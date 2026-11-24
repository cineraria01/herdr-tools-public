package merger_test

// pr-mirror-gaps ⑤: 거울 표식(refs/herdr/pr-src/<레인>) 없이 사람이 만든 같은 이름 가지 pr/<레인>은 병합기가 건드리지 않는다 —
// 거울이 꺼져 있어도 그 가지를 착지 커밋으로 덮어 잃었다. 거울이 만든 가지(대조)는 tests/merger/prhead_test.go

import "testing"

func TestPRHeadHumanBranch(t *testing.T) {
	k := newRK(t)
	f := k.fixture("pr-human")
	f.enable()
	f.commitFile("alpha", "first")
	old := f.g("rev-parse", "HEAD")
	f.w(`git push -q origin "HEAD:refs/heads/pr/alpha"`) // 사람이 쓰는 가지 — 거울 표식 없음
	r := f.submit("alpha")
	k.check("제출 종료 0", r.RC == 0, clog(r))
	advance(f)
	prHook(f)
	r = f.tick()
	k.check("착지", f.og("show", "main:alpha") == "first" && f.og("show", "main:moved") == "m", clog(r))
	k.check("pr 참조 push 없음", recvAt(f, "refs/heads/pr/alpha") < 0, read(f.home+"/recv.log"))
	k.check("사람 가지 그대로", f.og("rev-parse", "refs/heads/pr/alpha") == old, f.og("for-each-ref"))
}
