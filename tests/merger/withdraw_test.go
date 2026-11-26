package merger_test

// ht-merger-withdraw ⑥: 지휘가 거둔(hc-fill withdraw — 큐 표식·기록을 같은 이름의 반송 참조로) 레인이 고쳐 다시 낸 제출은 들어간다 —
// 제출 막기(같은 레인의 옛 커밋)에 안 걸리고, 병합기가 착지한다. 거둔 반송 참조는 그대로 남는다

import (
	"strings"
	"testing"
)

func TestWithdrawThenResubmit(t *testing.T) {
	k := newRK(t)
	f := k.fixture("withdraw")
	f.enable()
	f.commitFile("work", "broken")
	f.bok("submit", f.submit("lane"))
	n := strings.TrimPrefix(f.refs("--format=%(refname)", "refs/merge-queue/"), "refs/merge-queue/")
	if n == "" {
		t.Fatal("제출이 줄에 없다")
	}
	sha := f.og("rev-parse", "refs/merge-queue/"+n)
	rec := f.og("rev-parse", "refs/merge-queue-meta/"+n)
	f.w(`git push -q --atomic origin "$1:refs/merge-failed/$3" "$2:refs/merge-failed-meta/$3" ":refs/merge-queue/$3" ":refs/merge-queue-meta/$3"`, sha, rec, n)
	f.commitFile("work", "fixed")
	r := f.submit("lane")
	k.check("고쳐 다시 제출 접수", r.RC == 0 && f.refs("refs/merge-queue/") != "", r.Out, r.Err)
	r = f.tick()
	f.bok("tick", r)
	k.check("고친 제출 착지", f.og("show", "main:work") == "fixed", r.Out, r.Err)
	k.check("거둔 반송 참조는 그대로", f.og("rev-parse", "refs/merge-failed/"+n) == sha)
}
