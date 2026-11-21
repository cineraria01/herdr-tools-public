package hclane_test

// tests/hc-lane.test.sh 8~11절: --on(hc exec·hc run --on)·기계 공간 라벨·인자 오류·출력된 착지 절차·반쯤 만든 것.

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func init() { reg(20, "s08-11", s08Run) }

func s08Run(k *K) {
	s08On(k)
	s08Space(k)
	s08Args(k)
	s08Steps(k)
	s08Half(k)
}

// s08pairExec: hc exec 기록($HCLOG.exec)에서 옵션 다음 값(grep -Fx -A1 opt | grep -Fx val).
func (k *K) s08pairExec(opt, val string) bool {
	l := k.hclog(".exec")
	for i, s := range l {
		if s == opt && i+1 < len(l) && l[i+1] == val {
			return true
		}
	}
	return false
}

// s08match: 정규식이 걸린 줄들(grep -E).
func s08match(s, re string) []string {
	r := regexp.MustCompile(re)
	var l []string
	for _, x := range strings.Split(s, "\n") {
		if r.MatchString(x) {
			l = append(l, x)
		}
	}
	return l
}

// s08steps: 출력의 번호 줄에서 번호를 뗀 명령들(sed -n 's/^[0-9]\. //p')을 파일로.
func (k *K) s08steps(name string) {
	var b strings.Builder
	for _, x := range s08match(k.out, `^[0-9]\. `) {
		b.WriteString(x[3:] + "\n")
	}
	write(k.t, filepath.Join(k.T, name), b.String())
}

// s08bash: T에서 bash <인자…>를 돌려 stdout+stderr(셸 >파일 2>&1)·종료 코드.
func (k *K) s08bash(args ...string) (string, int) {
	ep := filepath.Join(k.T, "steps.err")
	o, rc := k.exe(k.T, ep, nil, "bash", args...)
	return o + "\n" + read(ep), rc
}

func s08On(k *K) {
	// 8. --on: git·설정은 hc exec --on, 실행은 hc run --on (가짜 hc exec가 그 자리에서 돌림)
	k.mk("t8")
	k.seed(".claude/herdr-setup.sh", `echo "${HL_ZSHRC-none}|${HL_ZPROFILE-none}" > setup-ran`)
	k.run("--on", "box", "--name", "lane-o", "--repo", k.R, "--", "echo", "hi")
	k.check("--on: 종료 0", k.rc == 0)
	k.check("--on: git·설정을 hc exec --on box로", k.s08pairExec("--on", "box") && contains(k.hclog(".exec"), "worktree") && exists(k.R+".wt/lane-o/setup-ran"))
	want := "none|none"
	if _, err := exec.LookPath("zsh"); err == nil {
		want = "read|read"
	}
	k.check("--on: 설정 단계는 로그인 셸(~/.zprofile)과 ~/.zshrc를 읽고 돈다(zsh 있으면)", strings.TrimRight(read(k.R+".wt/lane-o/setup-ran"), "\n") == want)
	k.check("--on: hc run --on box --cwd 워크트리", k.pair("--on", "box") && k.pair("--cwd", k.R+".wt/lane-o"))
	k.check("--on: JSON에 기계", strings.Contains(lastLine(k.out), `"on":"box"`))
	k.seed(".claude/herdr-setup.sh", "exit 5")
	k.run("--on", "box", "--name", "lane-o2", "--repo", k.R, "--", "echo", "hi")
	k.check("--on setup 실패: 방금 만든 워크트리·브랜치를 hc exec --on box로 지움", k.rc != 0 && !exists(k.R+".wt/lane-o2") &&
		!k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/lane-o2") && contains(k.hclog(".exec"), "remove"))
	k.seed(".claude/herdr-setup.sh", "git commit -q --allow-empty -m setup-commit; exit 3")
	k.run("--on", "box", "--name", "lane-o3", "--repo", k.R, "--", "echo", "hi")
	var ul []string
	for _, x := range strings.Split(k.errf(), "\n") {
		if i := strings.LastIndex(x, "지우려면: "); i >= 0 {
			ul = append(ul, x[i+len("지우려면: "):])
		}
	}
	undo := strings.Join(ul, "\n")
	ok := k.rc != 0 && isDir(k.R+".wt/lane-o3") && strings.Contains(undo, "hc exec --on box -- git -C") && strings.Contains(undo, "&& hc exec --on box -- git -C")
	if ok {
		_, urc := k.exe(k.T, "", nil, "bash", "-c", strings.Replace(undo, "branch -d", "branch -D", 1))
		ok = urc == 0 && !exists(k.R+".wt/lane-o3") && !k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/lane-o3")
	}
	k.check("--on setup 실패(고유 커밋): 남기고 안내하는 명령이 hc exec --on box 꼴, 그 명령으로 정리됨(감사 [낮음] 4)", ok)
}

func s08Space(k *K) {
	// 8b. 다른 기계 레인은 맥 Herdr 창의 그 기계 공간(이름 = 기계, 기계마다 하나)에 탭으로(hc run --label herdr.space=<id>)
	k.mk("t8b")
	rm := func(sfx ...string) {
		for _, s := range sfx {
			os.Remove(k.HCLOG + s)
		}
	}
	app := []string{"HC_FAKE_APP=1"}
	rm(".app", ".appls")
	k.runE(app, "--on", "box", "--name", "lane-a", "--repo", k.R, "--", "echo", "hi")
	k.check("원격 표시: 창에 그 기계 공간이 없으면 고정 id lanes로 붙인다", k.rc == 0 && k.pair("--label", "herdr.space=lanes"))
	k.check("원격 표시: 창이 만든 공간에 기계 이름을 붙인다(space.rename box/lanes → box)", contains(k.hclog(".app"), "do space.rename --id box/lanes --name box"))
	rm("", ".app")
	k.runE(app, "--read", "--on", "box", "--name", "lane-b", "--repo", k.R, "--", "echo", "hi")
	k.check("원격 표시: 같은 기계 두 번째 레인(--read 감사도)은 같은 공간(lanes)에, 이름은 다시 안 붙임", k.rc == 0 && k.pair("--label", "herdr.space=lanes") && !strings.Contains(read(k.HCLOG+".app"), "rename"))
	rm("", ".app")
	write(k.t, k.HCLOG+".appls", `{"spaces":[{"id":"box/sp9","name":"box","machine":"box"},{"id":"other/sp9","name":"box","machine":"other"}]}`+"\n")
	k.runE(app, "--on", "box", "--name", "lane-c", "--repo", k.R, "--", "echo", "hi")
	k.check("원격 표시: 그 기계에 「<기계>」 공간이 있으면 그 id(창이 보이는 「box/」 접두는 뗀다 — 붙이면 새 공간)", k.rc == 0 && k.pair("--label", "herdr.space=sp9") && !strings.Contains(read(k.HCLOG+".app"), "rename"))
	rm("", ".app", ".appls")
	k.run("--on", "box", "--name", "lane-d", "--repo", k.R, "--", "echo", "hi")
	k.check("원격 표시: 창이 없으면 띄우기는 성공, 라벨은 lanes, 알림 한 줄", k.rc == 0 && k.pair("--label", "herdr.space=lanes") && strings.Contains(k.errf(), "Herdr 창이 없어"))
	rm("")
	k.runE(app, "--name", "lane-e", "--repo", k.R, "--", "echo", "hi")
	k.check("원격 표시: 로컬 레인은 herdr.space 라벨 없음(지휘 옆 분할은 hc-fill의 herdr-balance)", k.rc == 0 && !grep(read(k.HCLOG), "^herdr.space="))
	k.run("--dry-run", "--on", "box", "--name", "lane-g", "--repo", k.R, "--", "echo", "hi")
	k.check("원격 표시: dry-run 줄에 라벨", strings.Contains(k.out, "--label herdr.space=lanes"))
}

func s08Args(k *K) {
	// 9. 인자 오류
	os.Remove(k.HCLOG)
	k.run("--name", "Bad_Name", "--repo", k.R, "--", "true")
	k.check("인자: 대문자 이름 거절", k.rc != 0)
	k.run("--name", "lane-q", "--repo", k.R)
	k.check("인자: 실행 명령 없으면 거절", k.rc != 0)
	k.run("--name", "lane-q", "--repo", k.T+"/없는 폴더", "--", "true")
	k.check("인자: git 저장소 아니면 거절", k.rc != 0 && !exists(k.HCLOG))
	k.run("--name", "lane-q", "--branch", "a;id", "--repo", k.R, "--", "true")
	k.check("인자: 브랜치 이름의 특수 문자 거절", k.rc != 0 && !exists(k.HCLOG) && !exists(k.R+".wt/a;id"))
}

func s08Steps(k *K) {
	origin := func() string { return k.D + "/origin.git" }
	// 10. 출력된 착지·정리 절차를 그대로 돌린다(branch.autoSetupMerge=false여도 branch -d까지 통해야 한다)
	k.mk("t10")
	k.seed(".gitignore", ".omc/")
	k.seed(".claude/herdr-verify.sh", "echo herdr-verify-ran")
	k.git(k.R, "config", "branch.autoSetupMerge", "false")
	k.run("--name", "lane-l", "--repo", k.R, "--", "true")
	wt := k.R + ".wt/lane-l"
	k.sh(wt, `echo land >land && git add -- land && git commit -qm land -- land`)
	write(k.t, wt+"/.omc/state/s.json", "s\n") // 레인 세션이 남기는 무시 폴더
	k.seed("c", "c")                           // 그 사이 origin이 앞서 나가 rebase가 필요하게
	k.s08steps("steps.sh")
	so, src := k.s08bash("-e", "steps.sh")
	k.check("착지 절차: 그대로 실행해 종료 0", src == 0)
	k.check("착지 절차: origin에 레인 커밋이 앞 커밋 위로", k.git(origin(), "log", "-1", "--format=%s", "main") == "land" && lastLine(k.git(origin(), "log", "-2", "--format=%s", "main")) == "c")
	k.check("착지 절차: 워크트리·브랜치 정리됨(무시 폴더가 남아도)", !exists(wt) && !k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/lane-l"))
	k.check("착지 절차: herdr-verify.sh를 돌리고 착지", strings.Contains(so, "herdr-verify-ran") && strings.Contains(so, "hc-lane land: 착지"))
	if src != 0 {
		k.t.Logf("steps.out:\n%s", so)
	}

	// 10b. 검증이 실패하면 push가 나가지 않는다: 출력된 줄을 멈추지 않고(bash -e 없이) 차례로 다 돌려도
	k.mk("t10b")
	k.seed(".claude/herdr-verify.sh", "echo verify-failed; exit 7")
	k.run("--name", "lane-vf", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/lane-vf"
	k.sh(wt, `echo vf >vf && git add -- vf && git commit -qm vf -- vf`)
	k.s08steps("steps-vf.sh")
	so, _ = k.s08bash("steps-vf.sh")
	k.check("착지 절차: 검증이 실패하면 줄을 다 돌려도 push가 안 나감", k.git(origin(), "log", "-1", "--format=%s", "main") != "vf" && strings.Contains(so, "verify-failed"))
	k.check("착지 절차: 검증이 실패하면 정리도 멈춰 워크트리·브랜치가 남음", exists(wt+"/vf") && k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/lane-vf"))
	num := s08match(k.out, `^[0-9]\. `)
	k.check("착지 절차: 착지는 hc-lane land 한 줄, 이어 쓰지 말라는 안내", strings.Contains(strings.Join(num, "\n"), "&& hc-lane land main") && strings.Contains(k.out, "세미콜론으로 이어 쓰지 말 것"))
	k.check("착지 절차: 번호 줄은 명령만, 안내 # 줄에는 ; > | & 없음(대화형 zsh에 붙여 넣어도 명령이 안 됨)",
		!strings.Contains(strings.Join(num, "\n"), "#") && !grep(strings.Join(s08match(k.out, `^   #`), "\n"), "[;>|&]"))

	// 10c. rebase가 충돌로 멈추면 반쯤 된 HEAD가 push되지 않는다(줄을 멈추지 않고 다 돌려도)
	k.mk("t10c")
	k.seed("x", "base")
	k.seed(".claude/herdr-verify.sh", "exit 0")
	k.run("--name", "lane-rc", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/lane-rc"
	k.sh(wt, `echo l1 >l1 && git add -- l1 && git commit -qm l1 -- l1 && echo lane >x && git commit -qm l2 -- x`)
	k.seed("x", "other") // origin의 x가 바뀌어 l2가 충돌한다
	k.s08steps("steps-rc.sh")
	k.s08bash("steps-rc.sh")
	k.check("착지 절차: rebase 충돌이면 줄을 다 돌려도 origin은 그대로(앞 커밋 l1만 나가지 않음)", k.git(origin(), "log", "-1", "--format=%s", "main") == "x" &&
		!contains(strings.Split(k.git(origin(), "log", "--format=%s", "main"), "\n"), "l1"))
	k.git(wt, "rebase", "--abort")
}

func s08Half(k *K) {
	// 11. 아무것도 만들기 전에 막거나, 반쯤 만든 것은 지우는 명령을 알린다
	k.mk("t11")
	k.runE([]string{"HC_FAKE_LIVE=lane-u"}, "--name", "lane-u", "--repo", k.R, "--", "true")
	k.check("이름 사용 중: 오류, 워크트리·브랜치·hc run 없음", k.rc != 0 && strings.Contains(k.errf(), "lane-u") && !exists(k.R+".wt/lane-u") &&
		!k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/lane-u") && !exists(k.HCLOG))
	k.runE([]string{"HC_FAKE_ID=t12"}, "--name", "t12", "--repo", k.R, "--", "true")
	k.check("ID가 같은 다른 이름의 터미널은 막지 않음", k.rc == 0)
	k.runE([]string{"HC_FAKE_RUN_FAIL=1"}, "--name", "lane-v", "--repo", k.R, "--", "true")
	k.check("hc run 실패: 방금 만든 워크트리·브랜치를 지움(다음 시도가 막히지 않게)", k.rc != 0 && strings.Contains(k.errf(), "지웠다") && !exists(k.R+".wt/lane-v") &&
		!k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/lane-v"))
	k.git(k.R, "remote", "set-head", "origin", "-d")
	k.run("--name", "lane-w", "--repo", k.R, "--", "true")
	k.check("origin/HEAD 없음: main으로", k.rc == 0 && k.has("LANE_BASE=main"))
	k.mk("t12", "master")
	k.git(k.R, "remote", "set-head", "origin", "-d")
	k.run("--name", "lane-k", "--repo", k.R, "--", "true")
	k.check("origin/HEAD 없고 main도 없음: 오류, 아무것도 안 만듦", k.rc != 0 && strings.Contains(k.errf(), "origin/main") && !exists(k.R+".wt") && !exists(k.HCLOG))
}
