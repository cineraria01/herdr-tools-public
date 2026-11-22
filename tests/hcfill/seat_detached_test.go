package hcfill_test

// seat-clean-detached(2026-11-23): 감사 스크립트가 `git worktree add --detach`로 직접 만든 읽기 전용 작업 트리(hc-lane --read 아님)의
// done 터미널 여섯이 다섯 시간 hc-fill 자리를 셌다. 분리 HEAD·변경 0·어느 가지(로컬·원격)에도 없는 커밋 0이면 읽기 전용으로 안 센다.
// git이 실패하면(모름) 지금처럼 센다.

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
	seat := func(cwd string) string {
		k.fresh()
		k.cpConf()
		k.set("HC_REAL_REPO=" + cwd)
		k.lj("box-1/a1:claude-code:fable-audit-1:done:" + cwd)
		k.add("sd")
		k.run("run")
		for _, l := range lines(k.out) {
			if strings.HasPrefix(l, "자리 box-1 box-1/a1 ") {
				return l
			}
		}
		return k.out
	}
	l := seat(W)
	k.check("분리·깨끗한 작업 트리 → 읽기 전용(분리·깨끗) → 안 셈", strings.HasSuffix(l, "done, 읽기 전용(분리·깨끗) → 안 셈"), l)
	write(k.t, W+"/new.txt", "x\n")
	l = seat(W)
	k.check("분리·변경 있음(추적 안 된 파일) → 셈", strings.HasSuffix(l, "done, 작업 트리 있음 → 셈"), l)
	rm(W + "/new.txt")
	g(W, "commit", "-qm", "lane", "--allow-empty")
	l = seat(W)
	k.check("분리·어느 가지에도 없는 커밋 → 셈", strings.HasSuffix(l, "done, 작업 트리 있음 → 셈"), l)
	g(W, "switch", "-qc", "sd-lane")
	l = seat(W)
	k.check("가지 위 작업 트리 → 지금 규칙(작업 트리 있음 → 셈)", strings.HasSuffix(l, "done, 작업 트리 있음 → 셈"), l)
	nr := T + "/sdn.wt/x" // 저장소 아님 — git 실패(모름)
	write(k.t, nr+"/f", "x\n")
	l = seat(nr)
	k.check("git 실패(저장소 아님) → 셈", strings.HasSuffix(l, "done, 작업 트리 있음 → 셈"), l)
	k.unset("HC_REAL_REPO")
	k.cpConf()
}
