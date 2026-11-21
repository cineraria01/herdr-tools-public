package hclane_test

// tests/hc-lane.test.sh 21절 뒷부분(4): 메인 폴더 앞당김(main-folder-ff·재감사 M1·M2·R-M1·L1·L4), 대기열만(lane-queue-only·fill-no-parent).

import (
	"fmt"
	"os"
	"strings"
)

func init() { reg(113, "s21b4", s21b4) }

func s21b4(k *K) {
	k.unset("XDG_STATE_HOME")
	k.set(`HC_FAKE_LS={"terminals":[]}`)
	mhead := func() string { return k.git(k.R, "rev-parse", "HEAD") }
	otip := func() string { return k.git(k.D+"/origin.git", "rev-parse", "main") }
	has := func(s string) bool { return strings.Contains(k.out, s) }
	eh := func(s string) bool { return strings.Contains(k.errf(), s) }
	skip := "메인 폴더 앞당김 건너뜀"

	// 메인 폴더 앞당김(2026-11-18 main-folder-ff): 깨끗한 기본 브랜치일 때만 --ff-only
	k.mk("t-ff")
	k.run("--name", "ff-ok", "--repo", k.R, "--", "true")
	k.check("메인 폴더: 깨끗한 main이면 fetch 뒤 origin/main으로 앞당김", k.rc == 0 && mhead() == otip() && has("메인 폴더 앞당김"))
	k.seed("f1", "f1")
	k.run("--name", "ff-rd", "--read", "--repo", k.R, "--", "true")
	k.check("메인 폴더: 읽기 레인도 fetch 뒤 앞당김", k.rc == 0 && mhead() == otip() && has("메인 폴더 앞당김"))
	k.seed("f2", "f2")
	h0 := mhead()
	appendf(k.t, k.R+"/a", "dirty\n")
	k.run("--name", "ff-dirty", "--repo", k.R, "--", "true")
	k.check("메인 폴더: 추적 파일 변경이 있으면 건너뜀·변경 보존, 레인은 뜸", k.rc == 0 && mhead() == h0 && contains(s21bLines(k.R+"/a"), "dirty") && eh(skip) && k.pair("--name", "ff-dirty"))
	k.git(k.R, "checkout", "-q", "--", "a")
	k.git(k.R, "checkout", "-qb", "side")
	k.run("--name", "ff-side", "--repo", k.R, "--", "true")
	k.check("메인 폴더: 기본 브랜치가 아니면 건너뜀", k.rc == 0 && mhead() == h0 && k.git(k.R, "symbolic-ref", "--short", "HEAD") == "side" && eh(skip))
	k.git(k.R, "checkout", "-q", "main")
	k.seed("u", "from-origin")
	write(k.t, k.R+"/u", "mine-untracked\n")
	k.run("--name", "ff-ut", "--repo", k.R, "--", "true")
	k.check("메인 폴더: 추적 안 된 파일이 앞당김과 부딪히면 git이 거절 → 건너뜀·파일 보존", k.rc == 0 && mhead() == h0 && contains(s21bLines(k.R+"/u"), "mine-untracked") && eh(skip))
	os.Remove(k.R + "/u")
	write(k.t, k.R+"/keep-untracked", "other\n")
	k.run("--name", "ff-ut2", "--on", "box", "--repo", k.R, "--", "true")
	k.check("메인 폴더: 부딪히지 않는 추적 안 된 파일은 괜찮음(--on도 hc exec로 그 기계에서)", k.rc == 0 && mhead() == otip() && contains(s21bLines(k.R+"/keep-untracked"), "other") &&
		has("메인 폴더 앞당김 ") && strings.Contains(read(k.HCLOG+".exec"), "--ff-only"))
	k.run("--name", "ff-dry", "--dry-run", "--repo", k.R, "--", "true")
	k.check("메인 폴더: dry-run은 할 일만 찍고 건드리지 않음", k.rc == 0 && has("[dry-run] 메인 폴더 앞당김"))
	k.seed(".claude/herdr-verify.sh", "exit 0")
	k.mklane("ff-land")
	h1 := mhead()
	k.seedp("f3", "f3")
	k.lnd(k.wt)
	k.check("메인 폴더: land 착지 뒤 앞당김(착지 커밋까지)", k.rc == 0 && k.otop() == "ff-land" && mhead() == otip() && mhead() != h1 && has("메인 폴더 앞당김"))
	appendf(k.t, k.R+"/a", "dirty\n")
	k.mklane("ff-land2")
	k.lnd(k.wt)
	k.check("메인 폴더: land 착지 뒤에도 더러우면 건너뜀(착지는 성공)", k.rc == 0 && k.otop() == "ff-land2" && mhead() != otip() && contains(s21bLines(k.R+"/a"), "dirty") && eh(skip))
	k.git(k.R, "checkout", "-q", "--", "a")
	k.seedp(".claude/herdr-land.sh", "HERDR_LAND=1 git push -q origin HEAD:main && echo land-script-ok")
	k.mklane("ff-land3")
	k.lnd(k.wt)
	k.check("메인 폴더: herdr-land.sh 착지 뒤에도 앞당김", k.rc == 0 && contains(strings.Split(k.out, "\n"), "land-script-ok") && k.otop() == "ff-land3" && mhead() == otip() && has("메인 폴더 앞당김"))
	k.mk("t-ffr") // rebase 중인 메인 폴더(따로 된 클론 — 내 커밋을 지우지 않고 끝낸다)
	k.seed("a", "a2")
	k.sh(k.R, `echo mine >a && git commit -qm mine -- a && git fetch -q origin && ! git rebase -q origin/main >/dev/null 2>&1`)
	k.run("--name", "ff-rb", "--repo", k.R, "--", "true")
	k.check("메인 폴더: rebase 중이면 건너뜀(진행 중 표시 그대로)", k.rc == 0 && isDir(k.R+"/.git/rebase-merge") && eh(skip) && eh("진행 중"))
	k.git(k.R, "rebase", "--abort")
	// 재감사 고침(audit-main-folder-ff): M1 무시된 파일 덮지 않음, M2 메인 폴더에서 일하는 세션이면 건너뜀, L1 빈 갈래, L4 skip-worktree
	k.mk("t-ffi")
	k.seed(".gitignore", ".env")
	k.run("--name", "ffi-0", "--repo", k.R, "--", "true")
	write(k.t, k.R+"/.env", "mine\n")
	k.sh(k.D+"/seed", `git pull -q --rebase origin main && git rm -q .gitignore && git commit -qm unignore && HERDR_LAND=1 git push -q origin HEAD:main`)
	k.seed(".env", "upstream")
	h2 := mhead()
	k.run("--name", "ffi-1", "--repo", k.R, "--", "true")
	k.check("메인 폴더(M1): 무시된 로컬 파일을 앞당김이 추적하게 되면 덮지 않고 건너뜀·파일 보존", k.rc == 0 && mhead() == h2 && contains(s21bLines(k.R+"/.env"), "mine") && eh("메인 폴더 앞당김 건너뜀: git이 거절"))
	os.Remove(k.R + "/.env")
	k.seed("m1", "m1")
	h2 = mhead()
	ls1 := func(a, cwd, st string) string {
		return fmt.Sprintf(`{"terminals":[{"address":"%s","agent":"claude-code","cwd":"%s","state":"live","status":"%s","turn":3}]}`, a, cwd, st)
	}
	k.runE([]string{"HC_FAKE_LS=" + ls1("t5aa", k.R+"/sub", "working")}, "--name", "ffb-1", "--repo", k.R, "--", "true")
	k.check("메인 폴더(M2): 그 폴더(아래)를 cwd로 쓰는 working 터미널이 있으면 건너뜀(주소를 알림)", k.rc == 0 && mhead() == h2 && eh("메인 폴더 앞당김 건너뜀: 메인 폴더에서 일하는 세션 t5aa"))
	k.runE([]string{"HC_FAKE_LS=" + ls1("t5aa", k.R, "blocked")}, "--name", "ffb-2", "--read", "--repo", k.R, "--", "true")
	k.check("메인 폴더(M2): blocked도 일하는 중(읽기 레인)", k.rc == 0 && mhead() == h2 && eh("일하는 세션 t5aa"))
	k.runE([]string{"HC_FAKE_LS=" + ls1("t5aa", k.R+".wt/other", "working")}, "--name", "ffb-3", "--repo", k.R, "--", "true")
	k.check("메인 폴더(M2): 레인 작업 트리(<저장소>.wt/…)의 working은 상관없음", k.rc == 0 && mhead() == otip())
	k.seed("m2", "m2")
	k.runE([]string{"HC_FAKE_LS=" + ls1("t5aa", k.R, "idle")}, "--name", "ffb-4", "--repo", k.R, "--", "true")
	k.check("메인 폴더(M2): 쉬는(idle) 세션 밑에서는 앞당김", k.rc == 0 && mhead() == otip())
	k.seed("m3", "m3")
	k.runE([]string{"HC_TERMINAL_ID=t5aa", "HC_FAKE_LS=" + ls1("t5aa", k.R, "working")}, "--name", "ffb-5", "--repo", k.R, "--", "true")
	k.check("메인 폴더(M2): 부른 터미널 자신은 빼고 앞당김", k.rc == 0 && mhead() == otip())
	k.seed("m4", "m4")
	h2 = mhead()
	k.runE([]string{"HC_TERMINAL_ID=t5aa", "HC_FAKE_LS=" + ls1("box/t5aa", k.R, "working")}, "--name", "ffb-6", "--on", "box", "--repo", k.R, "--", "true")
	k.check("메인 폴더(M2): --on이면 그 기계 터미널(<기계>/<주소>)로 판정, 부른 쪽 주소와 같아도 뺌 없음", k.rc == 0 && mhead() == h2 && eh("일하는 세션 box/t5aa"))
	k.runE([]string{"HC_FAKE_LS_FAIL=1"}, "--name", "ffb-7", "--repo", k.R, "--", "true")
	k.check("메인 폴더(M2): hc ls 실패면 건너뜀(모르면 안 건드림)", k.rc == 0 && mhead() == h2 && eh("건너뜀: hc ls 실패"))
	// 재감사 R-M1: status 없는 대화형 지휘 — hc get foreground가 셸이 아니면 일하는 중
	lsz := `{"terminals":[{"address":"t5bb","agent":"zsh","cwd":"` + k.R + `","state":"live","turn":9}]}`
	fgj := func(n, pid string) string {
		return fmt.Sprintf(`{"address":"t5bb","foreground":{"argv":["%s"],"name":"%s","pid":%s},"pid":1045,"state":"live","status":null}`, n, n, pid)
	}
	z := func(body string) []string {
		return []string{"HC_FAKE_LS=" + lsz, "HC_FAKE_GET_ADDR=t5bb", "HC_FAKE_GET_BODY=" + body}
	}
	k.runE(z(fgj("Python", "12534")), "--name", "ffz-1", "--repo", k.R, "--", "true")
	k.check("메인 폴더(R-M1): status 없는 터미널이 셸 위에서 무언가 돌리면(하단 줄 지휘) 건너뜀", k.rc == 0 && mhead() == h2 && eh("일하는 세션 t5bb"))
	k.runE(z(`{"error":{"kind":"not_found"}}`), "--name", "ffz-2", "--repo", k.R, "--", "true")
	k.check("메인 폴더(R-M1): status 없는 터미널을 판정 못 하면 일하는 중(건너뜀)", k.rc == 0 && mhead() == h2 && eh("일하는 세션 t5bb"))
	k.runE([]string{`HC_FAKE_LS={"error":"bad"}`}, "--name", "ffz-3", "--repo", k.R, "--", "true")
	k.check("메인 폴더(R-L1): hc ls 꼴을 못 읽으면 건너뜀", k.rc == 0 && mhead() == h2 && eh("건너뜀: hc ls 실패"))
	k.runE(z(fgj("zsh", "1045")), "--name", "ffz-4", "--repo", k.R, "--", "true")
	k.check("메인 폴더(R-M1): status 없는 터미널이 셸만 떠 있으면 앞당김", k.rc == 0 && mhead() == otip())
	k.seed("m5", "m5")
	h2 = mhead()
	k.git(k.R, "checkout", "-q", "--detach")
	k.run("--name", "ffl-1", "--repo", k.R, "--", "true")
	k.check("메인 폴더(L1): 분리된 HEAD면 건너뜀", k.rc == 0 && mhead() == h2 && eh("분리된 HEAD"))
	k.git(k.R, "checkout", "-q", "main")
	k.git(k.R, "update-index", "--skip-worktree", "a")
	write(k.t, k.R+"/a", "local\n")
	k.run("--name", "ffl-2", "--repo", k.R, "--", "true")
	k.check("메인 폴더(L4): skip-worktree 표시 파일이 있으면 건너뜀·파일 보존", k.rc == 0 && mhead() == h2 && contains(s21bLines(k.R+"/a"), "local") && eh("skip-worktree"))
	k.git(k.R, "update-index", "--no-skip-worktree", "a")
	k.git(k.R, "checkout", "-q", "--", "a")
	ou := k.git(k.R, "remote", "get-url", "origin")
	k.git(k.R, "remote", "set-url", "origin", k.T+"/없는 원격.git")
	k.run("--name", "ffl-3", "--read", "--repo", k.R, "--", "true")
	k.git(k.R, "remote", "set-url", "origin", ou)
	k.check("메인 폴더(L1): 읽기 레인 fetch 실패면 경고만, 레인은 뜸", k.rc == 0 && k.pair("--name", "ffl-3") && eh("건너뜀: git fetch 실패"))
	k.sh(k.R, `echo own >own && git add -- own && git commit -qm own -- own`)
	k.run("--name", "ffl-4", "--repo", k.R, "--", "true")
	k.check("메인 폴더(L1): 로컬에만 있는 커밋이 있으면 건너뜀", k.rc == 0 && k.git(k.R, "log", "-1", "--format=%s") == "own" && eh("origin/main 에 없는 커밋"))

	// lane-queue-only(2026-11-19): 지휘 터미널이 쓰기 레인을 직접 띄우면 대기열(hc-fill add)로 돌려보낸다
	k.mk("tq")
	lanes := k.T + "/.local/state/hc-lane/lanes.tsv"
	q := func(extra ...string) []string {
		return append([]string{"HC_FAKE_LIVE=tQc", "HC_TERMINAL_ID=tQc"}, extra...)
	}
	k.runE(q(), "--start-wait", "0", "--name", "q-coord", "--repo", k.R, "--", "claude")
	k.check("대기열만: 지휘 터미널 + 쓰기 레인 → 종료 2, hc-fill add 안내", k.rc == 2 && eh("쓰기 레인은 hc-fill add로 대기열에 넣는다") && eh("--direct"))
	k.check("대기열만: 거절이면 워크트리·브랜치·hc run 없음", !exists(k.R+".wt/q-coord") && !k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/q-coord") && !k.has("run"))
	k.runE(q("HC_FAKE_RUN_ADDR=tq2"), "--start-wait", "0", "--direct", "--name", "q-direct", "--repo", k.R, "--", "claude")
	k.check("대기열만: --direct → 띄움, 「대기열 밖에서 띄움(--direct)」, 레인 기록 종류 direct", k.rc == 0 && isDir(k.R+".wt/q-direct") && eh("대기열 밖에서 띄움(--direct)") &&
		contains(s21bLines(lanes), "tq2\tq-direct\ttQc\tdirect"))
	k.check("직접 띄운 레인(--direct)은 부모를 그대로(--no-parent 없음)", !k.has("--no-parent"))
	os.Remove(k.HCLOG)
	k.runE(q("HC_LANE_FILL=1", "HC_FAKE_RUN_ADDR=tq3"), "--start-wait", "0", "--name", "q-fill", "--repo", k.R, "--", "claude")
	k.check("대기열만: HC_LANE_FILL=1(hc-fill) → 띄움, 종류 fill", k.rc == 0 && contains(s21bLines(lanes), "tq3\tq-fill\ttQc\tfill") && !eh("대기열 밖"))
	// 2026-11-19 fill-no-parent: 지휘 세션이 닫히자 대기열 레인이 함께 꺼졌다
	k.check("대기열 레인(HC_LANE_FILL=1)은 부모 없이(hc run --no-parent) — 대기열이 주인", k.has("run") && k.has("--no-parent"))
	os.Remove(k.HCLOG)
	k.runE(q("HC_LANE_FILL=1", "HC_FAKE_RUN_ADDR=tq3a"), "--start-wait", "0", "--read", "--name", "q-fill-audit", "--repo", k.R, "--", "claude")
	k.check("대기열 감사 줄(HC_LANE_FILL=1 --read)도 --no-parent", k.rc == 0 && k.has("--no-parent"))
	os.Remove(k.HCLOG)
	k.runE([]string{"HC_TERMINAL_ID=tQc", "HC_LANE_FILL=1"}, "--dry-run", "--on", "box", "--name", "q-fill-on", "--repo", k.R, "--", "claude")
	dry := false
	for _, l := range strings.Split(k.out, "\n") {
		dry = dry || strings.Contains(l, "hc run --bg") && strings.Contains(l, "--on box") && strings.Contains(l, "--no-parent")
	}
	k.check("대기열 레인 --on(원격)·--dry-run 줄에도 --no-parent", k.rc == 0 && dry)
	k.runE(q("HC_FAKE_RUN_ADDR=tq4"), "--start-wait", "0", "--read", "--name", "q-read", "--repo", k.R, "--", "claude")
	k.check("대기열만: --read → 띄움(종류 read)", k.rc == 0 && contains(s21bLines(lanes), "tq4\tq-read\ttQc\tread"))
	k.runE(append([]string{"HC_LANE=lane-up"}, q("HC_FAKE_RUN_ADDR=tq5")...), "--start-wait", "0", "--name", "q-inlane", "--repo", k.R, "--", "claude")
	k.check("대기열만: 레인 안(HC_LANE 있음) → 띄움", k.rc == 0 && isDir(k.R+".wt/q-inlane"))
	k.runE([]string{"HC_FAKE_LANETERM=tQl", "HC_TERMINAL_ID=tQl", "HC_FAKE_RUN_ADDR=tq6"}, "--start-wait", "0", "--name", "q-laneterm", "--repo", k.R, "--", "claude")
	k.check("대기열만: 레인 터미널(라벨 hc-lane=1) → 띄움", k.rc == 0 && isDir(k.R+".wt/q-laneterm"))
	k.runE([]string{"HC_FAKE_WATCHTERM=tQw", "HC_TERMINAL_ID=tQw", "HC_FAKE_RUN_ADDR=tq9"}, "--start-wait", "0", "--name", "q-watchterm", "--repo", k.R, "--", "claude")
	k.check("대기열만: 감시 터미널(라벨 hc-lane-watch=1) → 띄움(감사 [낮음] 3a)", k.rc == 0 && isDir(k.R+".wt/q-watchterm") && !eh("hc-fill add"))
	os.Remove(k.HCLOG)
	k.runE(q(), "--start-wait", "0", "--read", "--direct", "--name", "q-rd", "--repo", k.R, "--", "claude")
	k.check("대기열만: --read와 --direct를 함께 주면 거절(종료 2, 감사 [낮음] 2)", k.rc == 2 && eh("--read와 --direct는 함께 쓰지 않는다") && !k.has("run"))
	k.runE([]string{"HC_FAKE_RUN_ADDR=tq7"}, "--start-wait", "0", "--name", "q-outside", "--repo", k.R, "--", "claude")
	k.check("대기열만: 터미널 밖(HC_TERMINAL_ID 없음 — launchd·cron) → 띄움, 종류 칸 없음", k.rc == 0 && contains(s21bLines(lanes), "tq7\tq-outside\t-"))
	write(k.t, k.HCLOG+".getdown", "")
	k.runE(q("HC_FAKE_RUN_ADDR=tq8"), "--start-wait", "0", "--name", "q-unknown", "--repo", k.R, "--", "claude")
	os.Remove(k.HCLOG + ".getdown")
	k.check("대기열만: 판정 못 함(hc get 실패) → 띄움", k.rc == 0 && isDir(k.R+".wt/q-unknown"))
	k.runE(q(), "--print-steps", "--name", "q-steps", "--repo", k.R)
	k.check("대기열만: --print-steps는 영향 없음", k.rc == 0 && has("hc-lane land main") && !eh("hc-fill add"))
	k.runE(q(), "--dry-run", "--name", "q-dry", "--repo", k.R, "--", "claude")
	k.check("대기열만: --dry-run은 영향 없음(아무것도 안 만듦)", k.rc == 0 && !exists(k.R+".wt/q-dry") && !eh("hc-fill add"))
	k.run("land", "--direct")
	k.check("대기열만: --direct는 레인을 띄울 때만", k.rc == 1 && eh("--direct는 레인을 띄울 때만"))
}
