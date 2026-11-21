package hclane_test

// tests/hc-lane.test.sh 15r절(rerere·승인 착지)과 15절 끝(herdr-land.sh·master) — s15_land_test.go의 landRerere에서 이어진다.

import (
	"os"
	"strings"
)

func landRerere2(k *K, evf string) {
	origin := func() string { return k.D + "/origin.git" }
	noLocalRR := func() bool { return k.git(k.R, "config", "--local", "--get-regexp", `^rerere\.`) == "" }
	showX := func() string { return strings.ReplaceAll(k.git(origin(), "show", "main:x"), "\n", ",") }
	inRebase := func(wt string) bool { return k.gitOK(wt, "rev-parse", "-q", "--verify", "REBASE_HEAD") }

	// 15r. rerere
	k.mk("t15rr")
	k.check("rerere: 처음엔 저장소에 값도 rr-cache도 없다(시험 전제)", !k.gitOK(k.R, "config", "--get", "rerere.enabled") && !s15rrc(k))
	k.run("--read", "--name", "l-rd", "--repo", k.R, "--", "true")
	k.check("rerere: 읽기 레인(--read)은 켜지 않음", k.rc == 0 && !k.gitOK(k.R, "config", "--get", "rerere.enabled") && !s15rrc(k))
	k.git(k.R, "config", "rerere.enabled", "false")
	k.run("--name", "l-rf", "--repo", k.R, "--", "true")
	k.check("rerere: 쓰기 레인을 만들어도 저장소 설정은 그대로(false 그대로, autoUpdate도 안 씀)", k.rc == 0 &&
		k.git(k.R, "config", "--get", "rerere.enabled") == "false" && !k.gitOK(k.R, "config", "--get", "rerere.autoUpdate"))
	k.mk("t15rc")
	k.seed("x", "base")
	k.seed(".claude/herdr-verify.sh", "exit 0")
	k.run("--name", "l-rr", "--repo", k.R, "--", "true")
	wt := k.R + ".wt/l-rr"
	k.check("rerere: 쓰기 레인 작업 트리를 만들면 rr-cache(정하지 않은 가지는 git이 켬), 저장소 설정에는 rerere.*를 안 씀", s15rrc(k) && noLocalRR())
	k.sh(wt, "echo lane >x && git commit -qm l-rr -- x")
	l0 := k.git(wt, "rev-parse", "HEAD")
	k.seed("x", "other")
	k.lnd(wt, "main")
	k.check("rerere: 처음 충돌은 그대로 종료 3", k.rc == 3 && k.otop() == "x")
	s15resolve(k, wt)
	k.git(wt, "reset", "-q", "--hard", l0) // 착지 전으로(다시 서는 착지가 같은 충돌을 다시 만나는 꼴)
	k.lnd(wt, "main")
	k.check("rerere: 같은 충돌은 rerere가 풀고 land가 rebase를 이어 가 착지", k.rc == 0 && k.otop() == "l-rr" && showX() == "lane,other" && strings.Contains(k.errf(), "rerere"))
	// 미병합(U)이 비었다는 것만으로 --continue 하지 않는다
	k.mk("t15rp")
	k.seed("x", "base")
	k.seed("y", "base")
	k.seed(".claude/herdr-verify.sh", "exit 0")
	k.run("--name", "l-rp", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/l-rp"
	k.sh(wt, "echo lane >x && echo lane-y >y && git commit -qm l-rp -- x y")
	l0 = k.git(wt, "rev-parse", "HEAD")
	k.seed("x", "other")
	k.lnd(wt, "main")
	s15resolve(k, wt)
	k.git(wt, "reset", "-q", "--hard", l0)
	k.seedp("y", "other-y") // x와 다른 충돌 내용
	k.lnd(wt, "main")
	k.check("rerere: 기록 풀이가 일부(x)만 적용되면 종료 3, rebase 멈춤 그대로(y 미병합)", k.rc == 3 && inRebase(wt) &&
		k.git(wt, "diff", "--name-only", "--diff-filter=U") == "y" && !strings.Contains(k.errf(), "이어 간다"))
	k.git(wt, "rebase", "--abort")
	k.mk("t15rh")
	k.seed(".claude/herdr-verify.sh", "exit 0")
	k.run("--name", "l-rh", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/l-rh"
	k.sh(wt, "echo lane >z && git add -- z && git commit -qm l-rh -- z")
	k.seed("x", "other")
	hook := k.R + "/.git/hooks/pre-rebase"
	write(k.t, hook, "#!/bin/sh\necho pre-rebase-refused >&2\nexit 1\n")
	if err := os.Chmod(hook, 0o755); err != nil {
		k.t.Fatal(err)
	}
	k.lnd(wt, "main")
	os.Remove(hook)
	k.check("rerere: 충돌이 아닌 중단(훅 실패)은 이어 가지 않고 종료 3, push 안 함", k.rc == 3 && k.otop() == "x" &&
		strings.Contains(k.errf(), "pre-rebase-refused") && !strings.Contains(k.errf(), "이어 간다"))
	// 미병합 확인 명령(git rerere remaining)이 실패하면(빈 출력·128) 이어 가지 않는다
	k.mk("t15rm")
	k.seed("x", "base")
	k.seed(".claude/herdr-verify.sh", "exit 0")
	k.run("--name", "l-rm", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/l-rm"
	k.sh(wt, "echo lane >x && git commit -qm l-rm -- x")
	l0 = k.git(wt, "rev-parse", "HEAD")
	k.seed("x", "other")
	k.lnd(wt, "main")
	s15resolve(k, wt)
	k.git(wt, "reset", "-q", "--hard", l0)
	s15fakegit(k, "gitfail", `[ "$1 $2" = "rerere remaining" ] && exit 128`)
	k.lndE([]string{"PATH=" + k.T + "/gitfail:" + k.env["PATH"]}, wt, "main")
	k.check("rerere: 확인 명령(rerere remaining)이 실패하면 이어 가지 않고 종료 3, rebase 멈춤 그대로", k.rc == 3 && k.otop() == "x" && inRebase(wt) && !strings.Contains(k.errf(), "이어 간다"))
	k.git(wt, "rebase", "--abort")
	// 전역 includeIf onbranch로 그 가지에만 준 false는 그대로
	k.mk("t15ri")
	write(k.t, k.T+"/rr-off.gitconfig", "[rerere]\n\tenabled = false\n")
	write(k.t, k.T+"/.gitconfig", "[includeIf \"onbranch:l-ri\"]\n\tpath = "+k.T+"/rr-off.gitconfig\n")
	k.run("--name", "l-ri", "--repo", k.R, "--", "true")
	os.Remove(k.T + "/.gitconfig")
	k.check("rerere: 조건부(includeIf onbranch) false는 보존 — 저장소 설정에 rerere.*를 안 씀", k.rc == 0 && noLocalRR())
	// 일반 레인을 먼저 띄운 뒤 조건부 false 가지 레인
	k.mk("t15ro")
	write(k.t, k.T+"/.gitconfig", "[includeIf \"onbranch:l-ro2\"]\n\tpath = "+k.T+"/rr-off.gitconfig\n")
	k.run("--name", "l-ro1", "--repo", k.R, "--", "true")
	k.run("--name", "l-ro2", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/l-ro2"
	k.check("rerere: 일반 레인 다음에 띄운 조건부 false 가지는 그 가지에서 false 그대로(앞 레인이 가리지 않음)", k.rc == 0 &&
		k.git(wt, "config", "--get", "rerere.enabled") == "false" && noLocalRR())
	os.Remove(k.T + "/.gitconfig")
	// 승인 착지(HERDR_LAND_PATCHID)에서도 rerere 자동 이어 가기
	k.mk("t15rv")
	k.seed("x", "base")
	k.seed("z", "base-z")
	k.seed(".claude/herdr-verify.sh", "exit 0")
	k.run("--name", "l-rv", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/l-rv"
	k.sh(wt, "echo lane >x && echo lane-z >z && git commit -qm l-rv -- x z")
	l0 = k.git(wt, "rev-parse", "HEAD")
	p0 := s15pidof(k, wt, l0)
	k.seed("x", "other")
	k.lnd(wt, "main") // 처음 충돌 — 사람이 풀어 rerere가 풀이를 적는다
	s15resolve(k, wt)
	k.git(wt, "reset", "-q", "--hard", l0)
	k.lndE([]string{"HERDR_LAND_PATCHID=" + p0, "HERDR_LAND_SHA=" + l0, "HERDR_LAND_VERDICT=ax"}, wt, "main")
	k.check("승인 착지 + rerere: 다른 곳이 rerere가 푼 경로(x)뿐이면 착지하고 「rerere로 바뀐 경로 1개」", k.rc == 0 && k.otop() == "l-rv" &&
		showX() == "lane,other" && strings.Contains(k.out+"\n"+k.errf(), "rerere로 바뀐 경로 1개"))
	k.sh(wt, "echo lane-z2 >z && git commit -qm l-rv2 -- z")
	o1 := k.git(origin(), "rev-parse", "main")
	k.lndE([]string{"HERDR_LAND_PATCHID=" + p0, "HERDR_LAND_SHA=" + l0}, wt, "main")
	k.check("승인 착지: HEAD가 제출 SHA가 아니면 종료 2, push 없음", k.rc == 2 && k.git(origin(), "rev-parse", "main") == o1)
	k.lndE([]string{"HERDR_LAND_PATCHID=" + p0}, wt, "main")
	k.check("승인 착지: 착지할 커밋이 승인 변경이 아니면(승인 뒤 커밋 섞임) 종료 2, push 없음", k.rc == 2 &&
		strings.Contains(k.errf(), "승인 변경이 아니다") && k.git(origin(), "rev-parse", "main") == o1)
	// --verdict(레인 경로): rerere가 x를 풀었고, 검증 실패 뒤 레인이 z를 고쳐 커밋 → 종료 2(z만 이름)
	k.mk("t15rw")
	k.seed("x", "base")
	k.seed("z", "base-z")
	k.seed(".claude/herdr-verify.sh", `[ ! -e "$HOME/verify.fail" ]`)
	k.run("--name", "l-rw", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/l-rw"
	k.sh(wt, "echo lane >x && echo lane-z >z && git commit -qm l-rw -- x z")
	l0 = k.git(wt, "rev-parse", "HEAD")
	k.seed("x", "other")
	k.lnd(wt, "main")
	s15resolve(k, wt)
	k.git(wt, "reset", "-q", "--hard", l0)
	vd := k.T + "/.local/share/hc-fill/verdicts/l-rw"
	write(k.t, vd+"/arw.md", "aid\tarw\nlane\tl-rw\nrun\trw-1\nsha\t"+l0+"\nverdict\tpass\nreport\t-\nround\t1\nbase\tmain\n")
	write(k.t, k.T+"/verify.fail", "")
	k.lndE([]string{"HC_LANE=l-rw", "HC_LANE_JOB=rw-1"}, wt, "main", "--verdict", "arw")
	os.Remove(k.T + "/verify.fail")
	k.check("승인 착지(--verdict) + rerere: rerere가 x를 풀고 검증 실패 → push 없음(종료 1)", k.rc == 1 && strings.Contains(k.errf(), "rerere") && k.otop() == "x")
	k.sh(wt, "echo lane-z-fixed >z && git commit -qm l-rw-fix -- z")
	k.lndE([]string{"HC_LANE=l-rw", "HC_LANE_JOB=rw-1"}, wt, "main", "--verdict", "arw")
	var outside []string
	for _, l := range strings.Split(k.errf(), "\n") {
		if strings.Contains(l, "rerere가 푼 곳 밖") {
			outside = append(outside, l)
		}
	}
	o := strings.Join(outside, "\n")
	k.check("승인 착지(--verdict): rerere 경로(x) 밖의 차이(z 고침)는 종료 2 — z만 이름, push 없음", k.rc == 2 &&
		strings.Contains(o, " z)") && !strings.Contains(o, "x,") && k.otop() == "x")
	os.RemoveAll(k.T + "/.local/share/hc-fill")
	os.Remove(evf) // 위 land 실패가 레인 이름(HC_LANE)으로 남긴 사건 — 뒤 report 시험은 빈 파일에서

	k.mk("t15l") // .claude/herdr-land.sh가 있으면 그것에 맡긴다
	k.seed(".claude/herdr-verify.sh", "echo should-not-run; exit 9")
	k.seed(".claude/herdr-land.sh", `echo "land-script $1 HERDR_LAND=${HERDR_LAND-} $(basename "$PWD")"; exit 6`)
	k.mklane("l-ls")
	wt = k.wt
	k.lnd(wt + "/.claude")
	k.check("land: herdr-land.sh에 기본 브랜치·HERDR_LAND=1을 주고 맡김(하위 폴더에서 불러도 작업 폴더 최상위, 종료 코드 그대로)", k.rc == 6 &&
		s15hasLine(k.out, "land-script main HERDR_LAND=1 l-ls") && !strings.Contains(k.out, "should-not-run"))
	lg := s15llog(k, "l-ls")
	k.check("land-log: herdr-land.sh 출력도 로그로, 실패면 경로", lg != "" && s15hasLine(read(lg), "land-script main HERDR_LAND=1 l-ls") && strings.Contains(k.errf(), "전체 로그 "+lg))
	// 원격 조회(git remote get-url)가 착지 앞에서 실패하면 착지하지 않는다(종료 75, 스크립트도 안 돌림)
	s15fakegit(k, "gl-gu", `[ "$1 $2" = "remote get-url" ] && exit 128`)
	k.lndE([]string{"PATH=" + k.T + "/gl-gu:" + k.env["PATH"]}, wt)
	k.check("land: git remote get-url이 착지 앞에서 실패하면 착지하지 않음(종료 75) — herdr-land.sh도 안 돌림", k.rc == 75 && !strings.Contains(k.out, "land-script"))
	// 원격 조회는 스크립트 착지 성공 직후에 새로 — 그때 조회가 실패하면 착지는 성공(0)이고 앞당기지 않고 경고만
	k.mk("t15lg")
	k.seed(".claude/herdr-land.sh", `: >"$HOME/gu.ok"; exit 0`)
	s15fakegit(k, "gl-gu2", `[ "$1 $2" = "remote get-url" ] && [ -e "$HOME/gu.ok" ] && exit 128`)
	k.mklane("l-lg")
	wt = k.wt
	os.Remove(k.T + "/gu.ok")
	k.lndE([]string{"PATH=" + k.T + "/gl-gu2:" + k.env["PATH"]}, wt)
	k.check("land: 스크립트 착지 뒤 원격 조회는 그때 새로 — 그 조회가 실패하면 종료 0·앞당김 안 함·경고", k.rc == 0 &&
		!grep(k.out, "^hc-lane: 메인 폴더 ") && strings.Contains(k.errf(), "원격 origin을 읽지 못했다"))
	k.mk("t15q") // herdr-land.sh가 자기 검증 로그(.qa/<날짜>/herdr-verify-*)를 남기는 저장소
	k.seed(".claude/herdr-land.sh", `d=.qa/$(date +%F)/herdr-verify-000000-1; mkdir -p "$d"; printf "x\n--- FAIL: TestQaDir (0.1s)\n" >"$d/go-test.log"; echo "──── go 로그 끝"; exit 1`)
	k.mklane("l-qa")
	wt = k.wt
	k.lnd(wt)
	k.check("land-log: herdr-land.sh가 남긴 이번 검증 로그 폴더와 그 안 실패 줄도 찍음", k.rc == 1 &&
		strings.Contains(k.errf(), "검증 로그 폴더 "+wt+"/.qa/") && s15hasLine(k.errf(), "--- FAIL: TestQaDir (0.1s)"))

	k.mk("t15m", "master")
	k.seed(".claude/herdr-verify.sh", "exit 0")
	k.mklane("l-m")
	k.lnd(k.wt)
	k.check("land: 기본 브랜치를 안 주면 origin/HEAD(master)", k.rc == 0 && k.otop("master") == "l-m")
}
