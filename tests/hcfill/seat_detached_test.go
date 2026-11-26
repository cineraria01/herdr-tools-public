package hcfill_test

// seat-clean-detached(2026-11-23): 감사 스크립트가 `git worktree add --detach`로 직접 만든 읽기 전용 작업 트리(hc-lane --read 아님)의
// done 터미널 여섯이 다섯 시간 hc-fill 자리를 셌다. 2026-11-27 ht-seat-working-only부터 자리는 대기열이 띄운 것 가운데 지금 일하는
// 에이전트만이라(docs/hc-fill/slots.md), 대기열 기록에 없는 그 터미널은 작업 트리 꼴(분리·변경·가지)과 상관없이 「대기열 밖 → 안 셈」이고
// git을 부르지 않는다. 기계 줄에는 「대기열 밖 n」으로 따로 보인다.

import "strings"

func init() { reg(101, "seatdetached", secSeatDetached) }

func secSeatDetached(k *K) {
	T := k.T
	R, W := T+"/sdr", T+"/sdr.wt/aud"
	g := func(dir string, a ...string) {
		if o, rc := k.exe(dir, nil, k.REALGIT, append([]string{"-c", "user.name=t", "-c", "user.email=t@t.invalid"}, a...)...); rc != 0 {
			k.t.Fatal("git: " + o)
		}
	}
	g(T, "init", "-qb", "main", R)
	g(R, "commit", "-qm", "base", "--allow-empty")
	g(R, "worktree", "add", "-q", "--detach", W)
	seat := func(cwd, st string) string {
		k.fresh()
		k.cpConf()
		k.set("HC_REAL_REPO=" + cwd)
		k.lj("box-1/a1:claude-code:fable-audit-1:"+st+":"+cwd, "box-1/a2:claude-code:fable-audit-2:"+st+":"+cwd)
		k.add("sd")
		k.run("run")
		for _, l := range lines(k.out) {
			if strings.HasPrefix(l, "자리 box-1 box-1/a1 ") {
				return l
			}
		}
		return k.out
	}
	l := seat(W, "done")
	k.check("분리·깨끗한 작업 트리의 done(대기열이 안 띄움) → 대기열 밖 → 안 셈", strings.HasSuffix(l, "done, 대기열 밖 → 안 셈"), l)
	write(k.t, W+"/new.txt", "x\n")
	l = seat(W, "working")
	k.check("변경 있는 작업 트리에서 일하는 중이어도 대기열 밖 → 안 셈(자리 2인 box-1에 sd가 뜸)",
		strings.HasSuffix(l, "working, 대기열 밖 → 안 셈") && k.where("sd") == "up box-1 box-1/sd", l)
	k.run("machine")
	k.check("기계 줄에 자리 밖 「대기열 밖 2」", strings.Contains(k.out, "· 대기열 밖 2"), k.out)
	k.unset("HC_REAL_REPO")
	k.cpConf()
}
