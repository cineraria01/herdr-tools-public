package hclane_test

// tests/hc-lane.test.sh 1~7절: 레인 띄우기(기본·setup·--read·충돌·master·--dry-run·--print-steps).

import (
	"os"
	"os/exec"
	"strings"
)

func init() { reg(10, "s01-07", startBasics) }

func startBasics(k *K) {
	// 1. 기본 모드
	k.mk("t1")
	k.run("--name", "lane-a", "--repo", k.R, "--", "claude", "-p", "짧은 일 하나")
	wt := k.R + ".wt/lane-a"
	k.check("기본: 종료 0", k.rc == 0)
	k.check("기본: 워크트리가 <repo>.wt/<슬러그>", isDir(wt))
	k.check("기본: fetch한 origin/main 최신 커밋 기준", k.git(wt, "rev-parse", "HEAD") == k.git(k.D+"/origin.git", "rev-parse", "main"))
	k.check("기본: 브랜치 이름 = 레인 이름", k.git(wt, "symbolic-ref", "--short", "HEAD") == "lane-a")
	k.check("기본: hc run --bg --name", k.has("run") && k.has("--bg") && k.pair("--name", "lane-a"))
	k.check("기본: hc run --cwd = 워크트리", k.pair("--cwd", wt))
	k.check("기본: LANE_* 환경을 env로 넘김", k.has("env") && k.has("LANE_REPO="+k.R) && k.has("LANE_WORKTREE="+wt) && k.has("LANE_BRANCH=lane-a") && k.has("LANE_BASE=main"))
	k.check("기본: 실행 명령 인자 그대로", k.has("claude") && k.has("-p") && k.has("짧은 일 하나"))
	k.check("기본: 순서는 env → LANE_* → 실행 명령", k.line("env") < k.line("LANE_REPO="+k.R) && k.line("LANE_BASE=main") < k.line("claude"))
	k.check("기본: --on 없으면 hc exec 안 씀", !exists(k.HCLOG+".exec"))
	k.check("기본: 마지막 줄 JSON", strings.Contains(lastLine(k.out), `"lane":"t9zz","name":"lane-a"`))
	k.check("기본: 착지 절차에 경로·기준", strings.Contains(k.out, "cd "+sq(wt)+" && hc-lane land main") &&
		strings.Contains(k.out, "cd "+sq(k.R)+" && hc-lane clean --repo "+sq(k.R)+" --branch lane-a"))

	// 2. herdr-setup.sh 성공·실패
	k.mk("t2")
	k.seed(".claude/herdr-setup.sh", `echo "$LANE_REPO|$LANE_BRANCH|$LANE_BASE|${HL_ZSHRC-}|${WT_MAIN-none}|${HERDR_BASE-none}" > setup-ran`)
	k.runE([]string{"HC_TERMINAL_ID=t1zz"}, "--name", "lane-s", "--repo", k.R, "--", "true")
	k.check("setup: 성공 시 종료 0", k.rc == 0)
	k.check("setup: 워크트리 안에서 실행(LANE_REPO=메인 폴더·LANE_* 받음, WT_MAIN·HERDR_BASE 없음, hc 터미널 안 로컬은 ~/.zshrc 안 읽음)",
		strings.TrimRight(read(k.R+".wt/lane-s/setup-ran"), "\n") == k.R+"|lane-s|main||none|none")
	k.check("setup: 성공 뒤 hc run", k.has("run"))
	k.check("setup: hc 터미널 안 로컬은 그 자리에서(hc exec 안 씀)", !strings.Contains(read(k.HCLOG+".exec"), "herdr-setup.sh"))
	os.Remove(k.HCLOG + ".exec")
	k.run("--name", "lane-sc", "--repo", k.R, "--", "true")
	want := ""
	if _, err := exec.LookPath("zsh"); err == nil {
		want = "read"
	}
	ex := k.hclog(".exec")
	k.check("setup: hc 터미널 밖 로컬(launchd hc-fill)은 hc 코어 문맥으로(hc exec -- , --on 없이 — TCC로 bash가 ~/Documents를 못 읽음)",
		k.rc == 0 && strings.Contains(read(k.HCLOG+".exec"), ".claude/herdr-setup.sh") && !contains(ex, "--on"))
	k.check("setup: 코어 문맥이면 ~/.zshrc를 읽고 돈다(코어 PATH)", strings.TrimRight(read(k.R+".wt/lane-sc/setup-ran"), "\n") == k.R+"|lane-sc|main|"+want+"|none|none")
	k.mk("t2o")
	k.seed(".claude/wt-setup.sh", "echo old > old-setup-ran")
	k.run("--name", "lane-so", "--repo", k.R, "--", "true")
	k.check("setup: 옛 이름 wt-setup.sh는 부르지 않음(옛 이름 갈래 없음)", k.rc == 0 && !exists(k.R+".wt/lane-so/old-setup-ran") && k.has("run"))
	k.mk("t3")
	k.seed(".claude/herdr-setup.sh", "echo boom-setup; exit 7")
	k.run("--name", "lane-f", "--repo", k.R, "--", "true")
	k.check("setup 실패: 종료 0 아님", k.rc != 0)
	k.check("setup 실패: hc run 안 부름", !exists(k.HCLOG))
	k.check("setup 실패: 오류에 워크트리 경로와 기록 끝", strings.Contains(k.errf(), k.R+".wt/lane-f") && strings.Contains(k.errf(), "boom-setup"))
	k.check("setup 실패: 방금 만든 워크트리·브랜치를 지움(고유 커밋 없음)", !exists(k.R+".wt/lane-f") && !k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/lane-f") && strings.Contains(k.errf(), "지웠다"))
	k.seed(".claude/herdr-setup.sh", "true")
	k.run("--name", "lane-f", "--repo", k.R, "--", "true")
	k.check("setup 실패 뒤 고쳐서 같은 명령: 「브랜치가 이미 있다」로 안 막힘", k.rc == 0 && isDir(k.R+".wt/lane-f"))
	k.seed(".claude/herdr-setup.sh", "git commit -q --allow-empty -m setup-commit; exit 3")
	k.run("--name", "lane-fk", "--repo", k.R, "--", "true")
	k.check("setup 실패(고유 커밋 생김): 워크트리·브랜치를 남기고 지우는 명령 안내", k.rc != 0 && isDir(k.R+".wt/lane-fk") &&
		k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/lane-fk") && strings.Contains(k.errf(), "지우려면:"))

	// 3. --read
	k.mk("t4")
	k.run("--read", "--name", "lane-r", "--repo", k.R, "--", "echo", "hi")
	k.check("--read: 종료 0", k.rc == 0)
	k.check("--read: cwd = 저장소", k.pair("--cwd", k.R))
	k.check("--read: 워크트리·브랜치 안 만듦", !exists(k.R+".wt") && !k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/lane-r"))
	k.check("--read: LANE_REPO만, LANE_WORKTREE 없음", k.has("LANE_REPO="+k.R) && !strings.Contains(read(k.HCLOG), "LANE_WORKTREE"))

	// 4. 충돌은 오류, 아무것도 덮지 않음
	k.mk("t5")
	k.git(k.R, "branch", "lane-x")
	before := k.git(k.R, "rev-parse", "lane-x")
	k.run("--name", "lane-x", "--repo", k.R, "--", "true")
	k.check("충돌(로컬 브랜치): 오류와 --branch 안내", k.rc != 0 && strings.Contains(k.errf(), "--branch"))
	k.check("충돌(로컬 브랜치): 브랜치 그대로, 폴더·hc 없음", k.git(k.R, "rev-parse", "lane-x") == before && !exists(k.R+".wt/lane-x") && !exists(k.HCLOG))
	k.sh(k.D+"/seed", "git push -q origin HEAD:refs/heads/lane-y")
	k.run("--name", "lane-y", "--repo", k.R, "--", "true")
	k.check("충돌(원격 브랜치): 오류, 로컬 브랜치·폴더 안 만듦", k.rc != 0 && !k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/lane-y") && !exists(k.R+".wt/lane-y"))
	write(k.t, k.R+".wt/lane-z/f", "keep\n")
	k.run("--name", "lane-z", "--repo", k.R, "--", "true")
	k.check("충돌(폴더): 오류, 폴더 내용 그대로, 브랜치 안 만듦", k.rc != 0 && read(k.R+".wt/lane-z/f") == "keep\n" &&
		!k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/lane-z") && !exists(k.HCLOG))
	k.run("--name", "lane-z", "--branch", "feat/lane-z2", "--repo", k.R, "--", "true")
	k.check("--branch로 피함: 폴더는 / 를 - 로", k.rc == 0 && k.git(k.R+".wt/feat-lane-z2", "symbolic-ref", "--short", "HEAD") == "feat/lane-z2")

	// 5. 기본 브랜치가 master(origin/HEAD)
	k.mk("t6", "master")
	k.run("--name", "lane-m", "--repo", k.R, "--", "true")
	k.check("master: 종료 0, LANE_BASE=master", k.rc == 0 && k.has("LANE_BASE=master"))
	k.check("master: origin/master 최신 커밋 기준", k.git(k.R+".wt/lane-m", "rev-parse", "HEAD") == k.git(k.D+"/origin.git", "rev-parse", "master"))
	k.check("master: 착지 절차도 master", strings.Contains(k.out, "hc-lane land master"))

	// 6. --dry-run은 아무것도 만들지 않음
	k.mk("t7")
	ref := k.git(k.R, "rev-parse", "origin/main")
	k.run("--dry-run", "--name", "lane-d", "--repo", k.R, "--", "echo", "hi")
	k.check("dry-run: 종료 0", k.rc == 0)
	k.check("dry-run: 폴더·브랜치·hc 호출 없음, fetch도 안 함", !exists(k.R+".wt") && !k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/lane-d") &&
		!exists(k.HCLOG) && k.git(k.R, "rev-parse", "origin/main") == ref)
	k.check("dry-run: 할 일·착지 절차·JSON 출력", strings.Contains(k.out, "[dry-run] hc run") && strings.Contains(k.out, "hc-lane land main") &&
		strings.Contains(k.out, "herdr-setup.sh") && strings.Contains(lastLine(k.out), `"dry_run":true`))
	k.git(k.R, "branch", "lane-d2")
	k.run("--dry-run", "--name", "lane-d2", "--repo", k.R, "--", "echo", "hi")
	k.check("dry-run: 충돌도 알림", k.rc != 0)

	// 7. --print-steps
	k.run("--print-steps", "--name", "lane-p", "--repo", k.R)
	wp := k.R + ".wt/lane-p"
	k.check("print-steps: 종료 0, 경로·검증·정리 줄", k.rc == 0 && strings.Contains(k.out, sq(wp)) && strings.Contains(k.out, "herdr-verify.sh") &&
		strings.Contains(k.out, "cd "+sq(k.R)+" && hc-lane clean --repo "+sq(k.R)+" --branch lane-p"))
	var steps []string
	for _, l := range strings.Split(k.out, "\n") {
		if grep(l, `^[0-9]\. `) {
			steps = append(steps, l)
		}
	}
	k.check("print-steps: 착지는 hc-lane land 한 줄, 옛 이름·건너뜀 없음", contains(steps, "1. cd "+sq(wp)+" && hc-lane land main") &&
		!strings.Contains(k.out, "wt-") && !strings.Contains(k.out, "건너뜀)"))
	k.check("print-steps: 끝내기 전 작업 트리 배경 프로세스 확인 줄(끝 /, 자기 셸 빼기)", strings.Contains(k.out, "pgrep -fl "+sq(wp+"/")) && strings.Contains(k.out, "자기 셸·에이전트는 빼고"))
	k.check("print-steps: 아무것도 만들지 않음", !exists(wp) && !exists(k.HCLOG))
}

// contains: 줄 목록에 그 줄이 그대로 있는지(grep -Fx).
func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}
