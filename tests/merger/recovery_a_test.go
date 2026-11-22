package merger_test

// 복구 시험 사례(옛 hc-lane-merger-recovery.test.sh) — 사례 함수 하나가 Test 하나

import "testing"

// untracked_hooks: 커밋 안 된 검증·착지 훅은 돌리지 않고 착지하지 않는다
func TestRecoveryUntrackedHooks(t *testing.T) {
	k := newRK(t)
	for _, hook := range []string{"herdr-verify", "herdr-land"} {
		f := k.fixture(hook)
		f.w(`git rm -q .claude/herdr-verify.sh && git commit -qm '검증 훅 제거' && mkdir -p .claude && echo 'touch "$HOME/ran"' >".claude/$1.sh"`, hook)
		r := f.lane(f.writer, nil, "land", "main")
		k.check(hook+": land 실패", r.RC != 0, r.Out, r.Err)
		k.check(hook+": 훅 안 돎", !exists(f.home+"/ran"))
		k.check(hook+": main 그대로", f.og("rev-parse", "main") == f.g("rev-parse", "main"))
	}
}

// batch_base: 묶음은 각 구성원의 갈래점이 아니라 지금 main 위에 얹는다
func TestRecoveryBatchBase(t *testing.T) {
	k := newRK(t)
	f := k.fixture("batch")
	f.enable()
	f.commitFile("alpha", "first")
	f.submit("alpha")
	f.w(`git checkout -q main`)
	f.commitFile("shared", "advanced")
	f.w(`git push -q origin main && git checkout -qb second`)
	f.commitFile("shared", "base")
	f.submit("zulu")
	r := f.tick()
	k.check("alpha 착지", f.og("show", "main:alpha") == "first", r.Out, r.Err)
	k.check("shared = base", f.og("show", "main:shared") == "base")
	k.check("큐 빔", f.refs("refs/merge-queue/") == "")
}
