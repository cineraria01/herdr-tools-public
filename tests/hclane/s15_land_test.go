package hclane_test

// tests/hc-lane.test.sh 15·15a·15r절: hc-lane land(검증·push·거절 재시도·rebase 충돌), land-log, rerere·승인 착지, herdr-land.sh.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func init() { reg(40, "s15-15r", landRerere) }

// s15hasLine: 그 줄이 그대로 있다(printf "%s\n" … | grep -Fx).
func s15hasLine(s, x string) bool { return contains(strings.Split(s, "\n"), x) }

// s15llog <레인>: land-logs/<레인>-*.log 가운데 가장 새것(ls -t | head -n 1), 없으면 "".
func s15llog(k *K, name string) string {
	l, _ := filepath.Glob(filepath.Join(k.T, ".local/state/hc-lane/land-logs", name+"-*.log"))
	sort.SliceStable(l, func(i, j int) bool {
		a, _ := os.Stat(l[i])
		b, _ := os.Stat(l[j])
		return a.ModTime().After(b.ModTime())
	})
	if len(l) == 0 {
		return ""
	}
	return l[0]
}

// s15rrc: 공용 git 폴더에 rr-cache(셸 rrc).
func s15rrc(k *K) bool {
	return isDir(k.git(k.R, "rev-parse", "--path-format=absolute", "--git-common-dir") + "/rr-cache")
}

// s15resolve: 충돌한 x를 lane·other로 풀고 rebase를 잇는다(셸 (cd … && … rebase --continue) — 실패는 버림).
func s15resolve(k *K, wt string) {
	k.exe(wt, "", nil, "bash", "-c", `printf 'lane\nother\n' >x && git add -- x && GIT_EDITOR=true git rebase --continue >/dev/null 2>&1`)
}

// s15fakegit <폴더> <조건 줄>: PATH 앞에 둘 가짜 git(셸 printf 바이트 그대로 — 조건이 맞으면 128, 아니면 진짜 git).
func s15fakegit(k *K, dir, cond string) {
	rg, err := exec.LookPath("git")
	if err != nil {
		k.t.Fatal(err)
	}
	p := filepath.Join(k.T, dir, "git")
	write(k.t, p, fmt.Sprintf("#!/bin/sh\n%s\nexec \"%s\" \"$@\"\n", cond, rg))
	if err := os.Chmod(p, 0o755); err != nil {
		k.t.Fatal(err)
	}
}

// s15pidof <폴더> <커밋>: 그 커밋의 origin/main 대비 patch-id(셸 pidof).
func s15pidof(k *K, dir, c string) string {
	o, _ := k.exe(k.T, "", nil, "bash", "-c",
		`git -C "$1" diff -U0 --binary "$(git -C "$1" merge-base "$2" origin/main)" "$2" | git patch-id --stable | cut -d' ' -f1`, "sh", dir, c)
	return o
}

func landRerere(k *K) {
	evf := filepath.Join(k.T, ".local/state/hc-lane/events.tsv")
	origin := func() string { return k.D + "/origin.git" }
	tail1 := func() string { return lastLine(strings.TrimRight(read(evf), "\n")) }
	cat := func(p string) string { return strings.TrimRight(read(p), "\n") }

	// 15. hc-lane land
	k.mk("t15")
	k.mklane("l-nov")
	wt := k.wt
	k.seed("c", "c")
	k.lnd(wt)
	k.check("land: herdr-verify.sh가 없으면 실패, push 안 함", k.rc == 1 && k.otop() == "c" && strings.Contains(k.errf(), "herdr-verify.sh"))
	k.seed(".claude/wt-verify.sh", "exit 0")
	k.lnd(wt)
	k.check("land: 옛 wt-verify.sh만 있어도 실패(옛 이름 갈래 없음)", k.rc == 1 && k.otop() == ".claude/wt-verify.sh")
	k.seed(".claude/herdr-verify.sh", `echo "verify $(git log -1 --format=%s) HERDR_LAND=${HERDR_LAND-}" >>"$HOME/verify.log"; echo verify-said-hi`)
	os.Remove(k.T + "/verify.log")
	k.lnd(wt)
	k.check("land-log: 통과면 화면은 그대로(실패 줄·로그 경로 없음), 로그만 더 생김", k.rc == 0 && s15hasLine(k.out, "verify-said-hi") &&
		!strings.Contains(k.errf(), "전체 로그") && s15hasLine(read(s15llog(k, "l-nov")), "verify-said-hi"))
	k.check("land: 검증이 있으면 rebase 뒤(그 HEAD로) 검증하고 push", k.rc == 0 && k.otop() == "l-nov" &&
		lastLine(k.git(origin(), "log", "-2", "--format=%s", "main")) == ".claude/herdr-verify.sh" && s15hasLine(read(k.T+"/verify.log"), "verify l-nov HERDR_LAND="))
	k.check("land: 착지 줄 출력", strings.Contains(k.out, "hc-lane land: 착지"))
	k.run("--name", "l-vf", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/l-vf"
	k.sh(wt, `printf 'echo vfail; exit 9\n' >.claude/herdr-verify.sh && git commit -qm vf -- .claude/herdr-verify.sh`)
	k.lnd(wt)
	k.check("land: 검증이 실패하면 멈추고 push 안 함", k.rc == 1 && k.otop() == "l-nov" && strings.Contains(k.out, "vfail") && strings.Contains(k.errf(), "검증 실패"))

	// 15a. land-log
	tab := "\t"
	k.run("--name", "l-vlog", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/l-vlog"
	write(k.t, wt+"/.claude/herdr-verify.sh", `i=0; while [ $i -lt 3000 ]; do echo "noise-a $i"; i=$((i + 1)); done
echo '--- FAIL: TestFlaky (0.01s)'; printf 'FAIL\texample.com/pkg/x\t0.2s\n'
i=0; while [ $i -lt 3000 ]; do echo "noise-b $i"; i=$((i + 1)); done
echo '(fail) bun thing [3ms]'; echo 'not ok 4 - tap'; echo '✗ mocha it' >&2; echo 'panic: boom'; echo FAIL; exit 1
`)
	k.sh(wt, "git commit -qm vlog -- .claude/herdr-verify.sh")
	k.lndE([]string{"HC_LANE=l-vlog"}, wt)
	lg := s15llog(k, "l-vlog")
	k.check("land-log: 검증 실패면 상태 폴더 land-logs/<레인>-*.log에 전체 출력(오류 출력 포함)", k.rc == 1 && lg != "" &&
		s15hasLine(read(lg), "noise-a 0") && s15hasLine(read(lg), "noise-b 2999") && s15hasLine(read(lg), "✗ mocha it"))
	k.check("land-log: 화면에도 그대로(로그 tee)", s15hasLine(k.out, "noise-b 2999"))
	e := k.errf()
	k.check("land-log: 긴 출력에서도 실패 줄을 뽑아 찍음(--- FAIL·FAIL<탭>패키지·(fail)·not ok·✗·panic)", s15hasLine(e, "--- FAIL: TestFlaky (0.01s)") &&
		s15hasLine(e, "FAIL"+tab+"example.com/pkg/x"+tab+"0.2s") && s15hasLine(e, "(fail) bun thing [3ms]") && s15hasLine(e, "not ok 4 - tap") &&
		s15hasLine(e, "✗ mocha it") && s15hasLine(e, "panic: boom") && !strings.Contains(e, "noise"))
	k.check("land-log: 실패면 로그 경로 한 줄, push 안 함", k.otop() == "l-nov" && strings.Contains(e, "전체 로그 "+lg))
	k.check("land-log: 레인 안(HC_LANE)이면 중간 사건에 로그 경로", strings.Contains(tail1(), tab+"l-vlog"+tab+"중간"+tab) && strings.Contains(tail1(), lg))
	k.run("--name", "l-vcap", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/l-vcap"
	write(k.t, wt+"/.claude/herdr-verify.sh", "i=0; while [ $i -lt 100 ]; do echo \"--- FAIL: TCap$i\"; i=$((i + 1)); done; exit 1\n")
	k.sh(wt, "git commit -qm vcap -- .claude/herdr-verify.sh")
	k.lnd(wt)
	ncap := 0
	for _, l := range strings.Split(k.errf(), "\n") {
		if strings.HasPrefix(l, "--- FAIL: TCap") {
			ncap++
		}
	}
	k.check("land-log: 실패 줄은 최대 40줄, 사건은 레인 밖이면 안 적음", k.rc == 1 && ncap == 40 && !strings.Contains(tail1(), "l-vcap"))
	os.Remove(evf) // 뒤 report 시험은 빈 events.tsv에서 시작한다
	k.mklane("l-dirty")
	wt = k.wt
	appendf(k.t, wt+"/l-dirty.txt", "more\n")
	k.lnd(wt)
	k.check("land: 커밋 안 된 변경이 있으면 멈춤", k.rc == 1 && strings.Contains(k.errf(), "커밋 안 된") && k.otop() == "l-nov")
	k.lnd(wt, "main", "extra")
	k.check("land: 남는 인자는 거절", k.rc != 0 && strings.Contains(k.errf(), "알 수 없는 인자: extra"))
	k.lnd(wt, "--on", "box")
	k.check("land: 옵션(--on 등)은 거절", k.rc != 0 && strings.Contains(k.errf(), "hc-lane land [<기본 브랜치>]"))
	k.lnd(k.T)
	k.check("land: git 폴더 밖이면 거절", k.rc != 0 && strings.Contains(k.errf(), "git 작업 폴더"))

	k.mk("t15r") // 거절 재시도: 첫 검증 동안 다른 쪽이 main에 먼저 넣는다
	k.seed(".claude/herdr-verify.sh", `n=$(cat "$HOME/race" 2>/dev/null || echo 0); echo $((n + 1)) >"$HOME/race"; [ "$n" -lt "$RACE_N" ] || exit 0; cd "$RACE_SEED" && git pull -q --rebase origin main && echo "$n" >"race$n" && git add "race$n" && git commit -qm "race$n" && HERDR_LAND=1 git push -q origin HEAD:main`)
	k.mklane("l-race")
	k.lndE([]string{"RACE_SEED=" + k.D + "/seed", "RACE_N=1"}, k.wt)
	k.check("land: 거절되면 fetch·rebase·검증부터 다시 해 착지", k.rc == 0 && k.otop() == "l-race" &&
		lastLine(k.git(origin(), "log", "-2", "--format=%s", "main")) == "race0" && cat(k.T+"/race") == "2" && strings.Contains(k.errf(), "거절됨"))
	k.mklane("l-race2")
	k.lndE([]string{"HERDR_LAND_TRIES=2", "RACE_SEED=" + k.D + "/seed", "RACE_N=99"}, k.wt)
	k.check("land: 거절이 HERDR_LAND_TRIES번이면 종료 4, 레인 커밋은 안 나감", k.rc == 4 && cat(k.T+"/race") == "4" &&
		!s15hasLine(k.git(origin(), "log", "--format=%s", "main"), "l-race2"))
	k.mklane("l-pf")
	k.git(k.R, "config", "remote.origin.pushurl", k.T+"/없는 원격.git")
	k.lndE([]string{"RACE_SEED=" + k.D + "/seed", "RACE_N=0"}, k.wt)
	k.git(k.R, "config", "--unset", "remote.origin.pushurl")
	k.check("land: 거절이 아닌 push 실패는 다시 하지 않고 멈춤", k.rc == 1 && strings.Contains(k.errf(), "다른 쪽 push가 앞선 것이 아니다") && !strings.Contains(k.errf(), "거절됨"))

	k.mk("t15c") // rebase 충돌
	k.seed("x", "base")
	k.seed(".claude/herdr-verify.sh", "exit 0")
	k.run("--name", "l-c", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/l-c"
	k.sh(wt, "echo lane >x && git commit -qm l-c -- x")
	k.seed("x", "other")
	k.lnd(wt, "main")
	k.check("land: rebase 충돌이면 종료 3, push 안 함", k.rc == 3 && k.otop() == "x" && strings.Contains(k.errf(), "rebase --continue"))
	k.git(wt, "rebase", "--abort")

	landRerere2(k, evf)
}
