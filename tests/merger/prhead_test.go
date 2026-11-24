package merger_test

// PR 머리 가지(2026-11-24 pr-surface, docs/hc-fill/pr.md): hc-fill PR 거울이 연 PR의 머리 가지 refs/heads/pr/<레인>이 있으면 병합기
// 착지가 기본 가지를 올리기 **앞에** rebase된 그 구성원의 커밋을 머리 가지에 올린다(원격이 받은 차례로 본다 — bare 원격의 update 훅
// 기록). 머리 push가 실패해도 착지는 그대로, 착지 사건 글이 「PR 머리 안 들어감」을 알린다. 머리 가지가 없으면 전과 같다

import (
	"os"
	"strings"
	"testing"
)

// prHook: origin update 훅 — 받은 참조 이름을 차례로 recv.log에, reject-pr 파일이 있으면 refs/heads/pr/*를 거절
func prHook(f *F) {
	f.t.Helper()
	h := "#!/bin/sh\necho \"$1\" >>'" + f.home + "/recv.log'\ncase $1 in refs/heads/pr/*) [ ! -f '" + f.home + "/reject-pr' ] || exit 1 ;; esac\nexit 0\n"
	if err := os.WriteFile(f.origin+"/hooks/update", []byte(h), 0o755); err != nil {
		f.t.Fatal(err)
	}
	os.Remove(f.home + "/recv.log")
}

// prMade <레인>: hc-fill이 완료 때 하는 것처럼 writer HEAD를 원격 머리 가지 pr/<레인>으로
func prMade(f *F, lane string) {
	f.t.Helper()
	f.w(`git push -q origin "HEAD:refs/heads/pr/$1"`, lane)
}

// advance: 제출 뒤 원격 main을 겹치지 않는 커밋 하나 앞당긴다 — 착지가 rebase해 새 커밋을 만들게(머리를 다시 올려야 Merged)
func advance(f *F) {
	f.t.Helper()
	f.w(`b=$(git rev-parse --abbrev-ref HEAD) && git checkout -q main && git pull -q origin main && echo m >moved && git add moved &&
git commit -qm moved && git push -q origin main && git checkout -q "$b"`)
}

// recvAt: recv.log에서 그 참조가 처음 나온 줄 번호(없으면 -1)
func recvAt(f *F, ref string) int {
	for i, l := range lines(read(f.home + "/recv.log")) {
		if l == ref {
			return i
		}
	}
	return -1
}

func landMsg(f *F, lane string) string {
	m := ""
	for _, r := range f.events() {
		if col(r, 4) == lane && col(r, 5) == "착지" {
			m = col(r, 7)
		}
	}
	return m
}

func TestPRHeadBeforeBase(t *testing.T) {
	k := newRK(t)

	t.Run("④ 하나 — 머리 가지가 기본 가지보다 먼저, 착지 커밋을 담음", func(t *testing.T) {
		f := k.fixture("pr-one")
		f.enable()
		f.commitFile("alpha", "first")
		prMade(f, "alpha") // hc-fill이 완료 때 만든 머리
		r := f.submit("alpha")
		k.check("제출 종료 0", r.RC == 0, clog(r))
		advance(f)
		prHook(f)
		r = f.tick()
		k.check("착지", f.og("show", "main:alpha") == "first" && f.og("show", "main:moved") == "m", clog(r))
		p, m := recvAt(f, "refs/heads/pr/alpha"), recvAt(f, "refs/heads/main")
		k.check("머리 push가 기본 push 앞", p >= 0 && m >= 0 && p < m, read(f.home+"/recv.log"))
		k.check("머리 = 착지 커밋(rebase된 것)", f.og("rev-parse", "refs/heads/pr/alpha") == f.og("rev-parse", "main"), f.og("for-each-ref"))
		k.check("착지 글에 「PR 머리 … 들어감」", strings.Contains(landMsg(f, "alpha"), "PR 머리 ") && strings.Contains(landMsg(f, "alpha"), " 들어감"), landMsg(f, "alpha"))
	})

	t.Run("④ 기준이 안 움직임 — 머리가 이미 착지 커밋이라 다시 올리지 않고 들어감", func(t *testing.T) {
		f := k.fixture("pr-ff")
		f.enable()
		f.commitFile("alpha", "first")
		prMade(f, "alpha")
		f.submit("alpha")
		prHook(f)
		r := f.tick()
		k.check("착지", f.og("show", "main:alpha") == "first", clog(r))
		k.check("pr 참조 push 없음", recvAt(f, "refs/heads/pr/alpha") < 0, read(f.home+"/recv.log"))
		k.check("착지 글에 「들어감」", strings.Contains(landMsg(f, "alpha"), " 들어감"), landMsg(f, "alpha"))
	})

	t.Run("④ 묶음 둘 — 둘 다 머리가 기본 가지보다 먼저", func(t *testing.T) {
		f := k.fixture("pr-batch")
		f.enable()
		f.commitFile("alpha", "first")
		prMade(f, "alpha")
		f.submit("alpha")
		f.w(`git checkout -q main && git checkout -qb second`)
		f.commitFile("zulu", "z")
		prMade(f, "zulu")
		f.submit("zulu")
		advance(f)
		prHook(f)
		r := f.tick()
		k.check("둘 다 착지", f.og("show", "main:alpha") == "first" && f.og("show", "main:zulu") == "z", clog(r))
		m := recvAt(f, "refs/heads/main")
		for _, l := range []string{"alpha", "zulu"} {
			p := recvAt(f, "refs/heads/pr/"+l)
			k.check(l+": 머리 push가 기본 push 앞", p >= 0 && m >= 0 && p < m, read(f.home+"/recv.log"))
			k.check(l+": 머리가 기본 가지에 듦", f.ogOK("merge-base", "--is-ancestor", "refs/heads/pr/"+l, "main"))
		}
		k.check("묶음 둘이 한 번에(기본 push 한 번)", strings.Count(read(f.home+"/recv.log"), "refs/heads/main\n") == 1, read(f.home+"/recv.log"))
	})

	t.Run("⑤ 머리 push 실패 — 착지는 됨, 착지 글이 「PR 머리 안 들어감」", func(t *testing.T) {
		f := k.fixture("pr-reject")
		f.enable()
		f.commitFile("alpha", "first")
		old := f.g("rev-parse", "HEAD")
		prMade(f, "alpha")
		f.submit("alpha")
		advance(f)
		prHook(f)
		if err := os.WriteFile(f.home+"/reject-pr", nil, 0o644); err != nil {
			t.Fatal(err)
		}
		r := f.tick()
		k.check("착지", f.og("show", "main:alpha") == "first", clog(r))
		k.check("머리 그대로", f.og("rev-parse", "refs/heads/pr/alpha") == old)
		k.check("착지 글에 「PR 머리 안 들어감」", strings.Contains(landMsg(f, "alpha"), "PR 머리 안 들어감"), landMsg(f, "alpha"))
	})

	t.Run("꺼짐 — 머리 가지가 없으면 pr 참조를 쓰지 않고 착지 글도 전과 같다", func(t *testing.T) {
		f := k.fixture("pr-off")
		f.enable()
		f.commitFile("alpha", "first")
		f.submit("alpha")
		prHook(f)
		r := f.tick()
		k.check("착지", f.og("show", "main:alpha") == "first", clog(r))
		k.check("pr 참조 push 없음", !strings.Contains(read(f.home+"/recv.log"), "refs/heads/pr/"), read(f.home+"/recv.log"))
		k.check("착지 글에 PR 없음", landMsg(f, "alpha") != "" && !strings.Contains(landMsg(f, "alpha"), "PR 머리"), landMsg(f, "alpha"))
	})
}
