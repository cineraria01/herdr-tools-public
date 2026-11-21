package hclane_test

// gofix-lane(2026-11-21 Go 전환 전체 감사 지적): 감사 ID마다 고치기 전 코드에서 실패하던 시험. 판마다 새 K라 절 차례에 기대지 않는다.
// goOnly: 셸 판도 같은 결함이라 셸 판은 고치지 않는 것(「알려진 차이」 — 셸 판은 곧 걷는다). 셸 판에서는 단언하지 않고 기록만 남긴다.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestGofix(t *testing.T) {
	eachImpl(t, func(k *K) {
		for _, c := range []struct {
			name string
			f    func(*K)
		}{
			{"L02", gfKeepGlob}, {"L03", gfMinAge}, {"L04", gfCoordPartial}, {"L05", gfSendTrunc},
			{"L06", gfCoordPerm}, {"L09", gfLandSignal}, {"L13", gfLongKey}, {"E01", gfGcTruncLs}, {"E03", gfReflogFail},
			{"L07L08", gfLegacyPark}, {"L10", gfWrapForcedGo}, {"L12", gfPrecheckCommitted}, {"E07E08", gfStartScreen}, {"E05", gfWatchLockPs},
		} {
			k.t.Run(c.name, func(t *testing.T) {
				pt := k.t
				k.t = t
				defer func() { k.t = pt }()
				c.f(k)
			})
		}
	})
}

// goOnly: 셸 판이 같은 결함인 단언 — go 판에서만 막는다
func (k *K) goOnly(name string, ok bool) {
	k.t.Helper()
	if k.env["HC_LANE_IMPL"] == "go" {
		k.check(name, ok)
		return
	}
	k.t.Logf("알려진 차이(셸 판 안 고침) %s: %v", name, ok)
}

// gfBin: 이 명령에만 먹는 가짜 프로그램 폴더(PATH 맨 앞). 돌려준 값은 extra에 넣을 PATH=…
func (k *K) gfBin(progs map[string]string) string {
	d, err := os.MkdirTemp(k.T, "fk")
	if err != nil {
		k.t.Fatal(err)
	}
	for n, s := range progs {
		if err := os.WriteFile(filepath.Join(d, n), []byte("#!/bin/bash\n"+s+"\n"), 0o755); err != nil {
			k.t.Fatal(err)
		}
	}
	return "PATH=" + d + ":" + k.env["PATH"]
}

// gfRealGit: 가짜 git이 넘길 진짜 git
func (k *K) gfRealGit() string {
	p, err := exec.LookPath("git")
	if err != nil {
		k.t.Fatal(err)
	}
	return p
}

// L02: 보호 무늬의 POSIX 문자 묶음·한글 — 셸 case와 같게 남긴다(로컬 가지)
func gfKeepGlob(k *K) {
	k.mk("gf02")
	k.sh(k.R, `git branch stable-1 && git branch 보관함 && git branch other`)
	k.conf("gc.keep_branches stable-[[:digit:]] 보관*")
	k.run("gc", "--repo", k.R, "--dry-run", "--min-age", "0")
	k.check("L02 gc: [[:digit:]]·한글 보호 무늬는 셸 case처럼 남긴다", k.rc == 0 && strings.Contains(k.out, "지움 1 · 남김 2") &&
		s19lineHas(k, "stable-1", "남길 목록") && s19lineHas(k, "보관함", "남길 목록"))
	k.conf("gc.keep_branches st[a-")
	k.run("gc", "--repo", k.R, "--dry-run", "--min-age", "0")
	k.goOnly("L02 gc: 해석 못 하는 보호 무늬면 아무것도 안 지우고 멈춘다", k.rc != 0 && !strings.Contains(k.out, "지움 "))
	k.noconf()
}

// L03: --min-age 08h 가 0시간이 되지 않는다(셸 판은 8진 오류로 종료 1)
func gfMinAge(k *K) {
	k.mk("gf03")
	k.sh(k.R, `git branch n1 && git branch n2`)
	k.run("gc", "--repo", k.R, "--dry-run", "--min-age", "08h")
	k.check("L03 gc --min-age 08h: 셸 판처럼 숫자 오류로 종료 1, 아무것도 안 고름", k.rc == 1 && !strings.Contains(k.out, "지움 "))
	for _, v := range []string{"99999999999999999999d", "9999999999999999d"} { // 셸 판은 산술이 넘쳐 감싸 돈다
		k.run("gc", "--repo", k.R, "--dry-run", "--min-age", v)
		k.goOnly("L03 gc --min-age "+v+": 넘침 오류로 종료 1, 아무것도 안 고름", k.rc == 1 && !strings.Contains(k.out, "지움 "))
	}
	k.run("gc", "--repo", k.R, "--dry-run", "--min-age", "8h")
	k.check("L03 gc --min-age 8h: 새 가지 둘을 남긴다", k.rc == 0 && strings.Contains(k.out, "지움 0 · 남김 2"))
}

// L04: hc get이 실패하며 잘린 응답 — 끝났다(1)가 아니라 모름(2)
func gfCoordPartial(k *K) {
	full := k.gfBin(map[string]string{"hc": `[ "$1" = get ] && { echo '{"address":"t1","name":"boss","pid":4242,"state":"live"}'; exit 0; }; exit 3`})
	k.runE([]string{full}, "coord", "--set", "t1")
	k.check("L04 준비: coord --set t1", k.rc == 0)
	cut := k.gfBin(map[string]string{"hc": `[ "$1" = get ] && { printf '{"address":"t1","name":"boss","pid":4242,"state":"live"'; exit 1; }; exit 3`})
	k.runE([]string{cut}, "coord", "--check", "t1")
	k.goOnly("L04 coord --check: 실패한 hc get의 잘린 응답은 모름(2)", k.rc == 2)
	gone := k.gfBin(map[string]string{"hc": `[ "$1" = get ] && { echo '{"error":{"kind":"not_found"}}' >&2; exit 3; }; exit 3`})
	k.runE([]string{gone}, "coord", "--check", "t1")
	k.check("L04 coord --check: 확인된 not_found는 1", k.rc == 1)
}

// L05: hc ls가 0으로 끝났는데 목록이 잘림 — 보낸 척(0) 하지 않는다
func gfSendTrunc(k *K) {
	p := k.gfBin(map[string]string{"hc": `case $1 in
ls) printf '{"terminals":[{"address":"t2","state":"live","name":"lane-a","parent":"boss"},'; exit 0 ;;
send) echo "$*" >>"$HCLOG.send"; exit 0 ;;
esac; exit 3`})
	os.Remove(k.HCLOG + ".send")
	k.runE([]string{p}, "send", "--all", "--from", "boss", "확인")
	k.goOnly("L05 send --all: 잘린 hc ls 목록이면 실패 종료, 아무에게도 안 보냄", k.rc != 0 && !exists(k.HCLOG+".send"))
	ok := k.gfBin(map[string]string{"hc": `case $1 in ls) echo '{"terminals":[]}'; exit 0 ;; esac; exit 3`})
	k.runE([]string{ok}, "send", "--all", "--from", "boss", "확인")
	k.check("L05 send --all: 올바른 빈 목록은 대상 0건으로 성공", k.rc == 0 && strings.Contains(k.out, "보냄 0 · 실패 0"))
}

// L06: umask 077이면 지휘 기록은 0600(셸 판과 같게 — 마스크를 무시하지 않는다)
func gfCoordPerm(k *K) {
	p := k.gfBin(map[string]string{"hc": `[ "$1" = get ] && { echo '{"address":"t6","name":"boss","pid":4242,"state":"live"}'; exit 0; }; exit 3`})
	k.out, k.rc = k.exe(k.T, filepath.Join(k.T, "err"), []string{p}, "bash", "-c", `umask 077; exec "$0" "$@"`, k.LANE, "coord", "--set", "t6")
	fi, err := os.Stat(filepath.Join(k.T, ".local/state/hc-lane/coord.t6"))
	k.check("L06 coord --set: umask 077이면 coord 파일 0600", k.rc == 0 && err == nil && fi.Mode().Perm() == 0o600)
}

// L09: 착지 스크립트가 SIGKILL로 끝나면 137(128+9) — 아무 신호나 143으로 바꾸지 않는다
func gfLandSignal(k *K) {
	k.mk("gf09")
	k.seed(".claude/herdr-land.sh", `kill -KILL $$`)
	k.git(k.R, "pull", "-q", "--ff-only")
	k.mklane("gf09l")
	k.lnd(k.wt, "main")
	k.check("L09 land: 착지 스크립트 SIGKILL은 종료 137", k.rc == 137)
}

// L13: 긴 --key·--to가 사건 줄을 깨거나 Go 판을 panic시키지 않는다 — 쓰기 전에 인자 오류(2)
func gfLongKey(k *K) {
	evf := filepath.Join(k.T, ".local/state/hc-lane/events.tsv")
	before := read(evf)
	long := strings.Repeat("k", 1100)
	k.runE([]string{"HC_LANE=gf13"}, "report", "중간", "-", "확인", "--key", long)
	k.goOnly("L13 report --key 1100자: 종료 2, 사건 안 씀", k.rc == 2 && read(evf) == before)
	k.runE([]string{"HC_LANE=gf13"}, "report", "중간", "-", "확인", "--to", long)
	k.goOnly("L13 report --to 1100자: 종료 2, 사건 안 씀", k.rc == 2 && read(evf) == before)
	k.check("L13 report: panic 없음", !strings.Contains(k.errf(), "panic"))
	k.runE([]string{"HC_LANE=gf13"}, "report", "중간", "-", strings.Repeat("가", 600), "--key", "k1")
	ln := lastLine(strings.TrimRight(read(evf), "\n"))
	k.check("L13 report: 긴 한 줄 칸은 잘라도 12칸·키 그대로", k.rc == 0 && len(strings.Split(ln, "\t")) == 12 && strings.HasSuffix(ln, "\tk1"))
}

// E01: hc ls가 배열을 열자마자 끝난 채 0 — 작업 폴더를 쓰는 터미널을 모르니 아무것도 안 지운다
func gfGcTruncLs(k *K) {
	k.mk("gfe1")
	k.land("gfe1l")
	p := k.gfBin(map[string]string{"hc": `[ "$1" = ls ] && { printf '{"terminals":['; exit 0; }; exit 3`})
	k.runE([]string{p}, "gc", "--repo", k.R, "--dry-run", "--min-age", "0")
	k.goOnly("E01 gc: 잘린 hc ls면 종료 1, 아무것도 안 고름", k.rc == 1 && !strings.Contains(k.out, "지움 "))
	k.runE([]string{p}, "clean", "--repo", k.R, "--branch", "gfe1l", "--dry-run")
	k.goOnly("E01 clean: 잘린 hc ls면 지우지 않고 멈춘다", k.rc != 0 && k.kept("gfe1l"))
}

// L10·L3: 포장 — HC_LANE_IMPL=go인데 실행기가 Go 판을 못 띄우면(두 번째 --version 실패) 셸 판이 사건을 쓰지 않고 실패.
// __has가 신호로 끝나면 강제 go는 실패(1), auto는 셸 판으로
func gfWrapForcedGo(k *K) {
	evf := filepath.Join(k.T, ".local/state/hc-lane/events.tsv")
	d, err := os.MkdirTemp(k.T, "gb")
	if err != nil {
		k.t.Fatal(err)
	}
	cnt := filepath.Join(d, "n")
	gb := filepath.Join(d, "hc-lane-go")
	os.WriteFile(gb, []byte(`#!/bin/bash
case $1 in
--version) [ -e `+sq(cnt)+` ] && exit 1; : >`+sq(cnt)+`; echo "hc-lane 1"; exit 0 ;;
__has) [ -n "${GB_KILL-}" ] && kill -KILL $$; exit 0 ;;
esac
exit 0
`), 0o755)
	before := read(evf)
	k.runE([]string{"HC_LANE_IMPL=go", "HC_LANE_GO_BIN=" + gb, "HC_LANE=gf10"}, "report", "중간", "-", "확인")
	k.check("L10 강제 go: 실행기가 Go 판을 못 띄우면 실패, 셸 판이 사건을 안 씀", k.rc != 0 && read(evf) == before)
	os.Remove(cnt)
	k.runE([]string{"HC_LANE_IMPL=go", "HC_LANE_GO_BIN=" + gb, "GB_KILL=1", "HC_LANE=gf10"}, "report", "중간", "-", "확인")
	k.check("L3 강제 go: __has가 신호로 끝나면 종료 1, 사건 안 씀", k.rc == 1 && read(evf) == before && !strings.Contains(k.errf(), "가운데 하나"))
	os.Remove(cnt)
	k.runE([]string{"HC_LANE_IMPL=auto", "HC_LANE_GO_BIN=" + gb, "GB_KILL=1", "HC_LANE=gf10", "HC_TERMINAL_ID=t9"}, "report", "중간", "-", "확인")
	k.check("L3 auto: __has가 신호로 끝나면 셸 판으로 사건을 쓴다", k.rc == 0 && read(evf) != before)
}

// L12: 제출 전 사전 검사는 제출 커밋 속 판만 돌린다 — 무시된(추적 안 된) 로컬 .claude/herdr-precheck.sh는 돌리지 않고 제출도 안 한다
func gfPrecheckCommitted(k *K) {
	k.mk("gf12")
	k.mklane("gf12l")
	mark := filepath.Join(k.T, "pc-ran")
	os.Remove(mark)
	k.sh(k.wt, `mkdir -p .claude && printf ': >%s\n' "$1" >.claude/herdr-precheck.sh && echo .claude/herdr-precheck.sh >>"$(git rev-parse --git-common-dir)/info/exclude"`, sq(mark))
	k.git(k.D+"/origin.git", "update-ref", "refs/herdr/merger/box", "main")
	env := []string{"HC_LANE=gf12l", "HC_LANE_MGON_WAITS=0"}
	k.lndE(env, k.wt, "main")
	q := k.git(k.D+"/origin.git", "for-each-ref", "--format=%(refname)", "refs/merge-queue/")
	k.goOnly("L12 무시된 로컬 사전 검사: 돌리지 않고 제출 안 함(실패)", k.rc != 0 && !exists(mark) && q == "")
	k.sh(k.wt, `git add -f -- .claude/herdr-precheck.sh && git commit -qm pc -- .claude/herdr-precheck.sh`)
	k.lndE(env, k.wt, "main")
	q = k.git(k.D+"/origin.git", "for-each-ref", "--format=%(refname)", "refs/merge-queue/")
	k.check("L12 커밋한 사전 검사: 돌리고 제출", k.rc == 0 && exists(mark) && q != "")
}

// E07: 실패한 hc read의 화면(신뢰 질문)으로 키를 보내지 않는다. E08: 막힘 줄의 비밀 꼴 값은 가린다(줄 종류는 그대로)
func gfStartScreen(k *K) {
	k.mk("gfe7")
	sc := filepath.Join(k.T, "sc-e7")
	k.conf("lane.start_wait 1")
	s21aTrust(sc, "❯", " ")
	real := "hc" // 시험 PATH의 가짜 hc
	for _, d := range filepath.SplitList(k.env["PATH"]) {
		if exists(filepath.Join(d, "hc")) {
			real = filepath.Join(d, "hc")
			break
		}
	}
	p := k.gfBin(map[string]string{"hc": `[ "$1" = read ] && { cat "$HC_FAKE_SCREEN"; exit 1; }; exec ` + sq(real) + ` "$@"`})
	k.s21aRmKeys()
	k.runE([]string{p, "HC_FAKE_SCREEN=" + sc, "HC_FAKE_NOSTATUS=1"}, "--name", "lane-e7", "--repo", k.R, "--", "claude")
	k.goOnly("E07 실패한 hc read 화면으로는 신뢰 키를 안 보냄(종료 5)", k.rc == 5 && !exists(k.HCLOG+".keys"))
	tok := "tk" + strings.Repeat("Q7", 20)
	write(k.t, sc, "Login expired · Please run /login token="+tok+" https://auth.example/cb?code="+tok+"\n")
	k.runE([]string{"HC_FAKE_SCREEN=" + sc, "HC_FAKE_NOSTATUS=1"}, "--name", "lane-e8", "--repo", k.R, "--", "claude")
	k.goOnly("E08 막힘 줄: 종류·글은 남기고 비밀 꼴 값은 가림", k.rc == 6 && strings.Contains(k.errf(), "Login expired") && !strings.Contains(k.errf(), tok))
	// E09: hc run이 0으로 끝났는데 주소가 없으면 실행을 모른다 — 성공(0)이 아니다
	p = k.gfBin(map[string]string{"hc": `[ "$1" = run ] && exit 0; exec ` + sq(real) + ` "$@"`})
	k.runE([]string{p}, "--read", "--name", "lane-e9", "--repo", k.R, "--", "true")
	k.goOnly("E09 hc run 주소 없음: 종료 0이 아니다", k.rc != 0 && strings.Contains(k.errf(), "주소를 읽지 못했다"))
	k.noconf()
}

// L07: 옛 판 주차(원격 lane/<레인> + 같은 SHA의 「주차」 사건)는 clean이 「주차됨」으로 정리 — 사건 없는 lane/<레인>(반송된 제출)은 멈춤.
// L08: --resume --dry-run도 원격 주차 가지를 본다(로컬에 받아 둔 것이 없어도)
func gfLegacyPark(k *K) {
	k.mk("gf07")
	k.seed(".claude/herdr-verify.sh", "exit 0")
	og := k.D + "/origin.git"
	for _, n := range []string{"lg-a", "lg-b"} {
		k.run("--name", n, "--repo", k.R, "--", "claude")
		w := k.R + ".wt/" + n
		k.sh(w, `echo x >"$1.txt" && git add -- "$1.txt" && git commit -qm "$1" -- "$1.txt" && git push -q origin "HEAD:refs/heads/lane/$1"`, n)
	}
	sa := k.git(k.R+".wt/lg-a", "rev-parse", "HEAD")
	k.runE([]string{"HC_LANE=lg-a"}, "report", "주차", sa, "옛 판 주차")
	k.run("clean", "--repo", k.R, "--branch", "lg-a", "--dry-run")
	k.goOnly("L07 clean: 옛 판 주차(lane/<레인> + 주차 사건)는 「주차됨」, 종료 0", k.rc == 0 && strings.Contains(k.out, "주차됨"))
	k.run("clean", "--repo", k.R, "--branch", "lg-b", "--dry-run")
	k.check("L07 clean: 주차 사건 없는 lane/<레인>은 주차로 안 읽음(멈춤)", k.rc != 0 && !strings.Contains(k.out, "주차됨"))
	k.git(og, "update-ref", "refs/park/rs-a", k.git(og, "rev-parse", "main"))
	k.run("--resume", "--dry-run", "--name", "rs-a", "--repo", k.R, "--", "claude")
	k.goOnly("L08 --resume --dry-run: 원격 refs/park/<레인>을 보고 시작점을 알림, 종료 0", k.rc == 0 && strings.Contains(k.out, "refs/park/rs-a") &&
		!k.gitOK(k.R, "rev-parse", "-q", "--verify", "refs/hc-park/rs-a"))
}

// E05: 잠금의 pid가 살아 있는데 ps를 못 읽으면 소유자를 모른다 — 잠금을 넘겨받지 않고 75
func gfWatchLockPs(k *K) {
	if k.env["HC_LANE_IMPL"] != "go" { // 셸 판은 잠금을 넘겨받아 끝없이 돈다 — 돌리지 않는다(알려진 차이)
		k.t.Logf("알려진 차이(셸 판 안 고침) E05: 셸 판은 ps 실패에 잠금을 넘겨받는다")
		return
	}
	st := filepath.Join(k.T, "wst-e5")
	wd := st + "/hc-lane"
	write(k.t, wd+"/coord.t7e5", "t7e5\t4242\tt7e5\t0\n")
	sl := exec.Command("sleep", "60")
	if err := sl.Start(); err != nil {
		k.t.Fatal(err)
	}
	defer func() { sl.Process.Kill(); sl.Wait() }()
	write(k.t, wd+"/watch.t7e5/pid", strconv.Itoa(sl.Process.Pid)+"\n")
	p := k.gfBin(map[string]string{"ps": `exit 1`})
	k.runE([]string{p, "XDG_STATE_HOME=" + st, "HC_FAKE_LIVE=t7e5", `HC_FAKE_LS={"terminals":[]}`, "HC_LANE_WAIT_TICK=1"}, "watch", "--to", "t7e5")
	k.goOnly("E05 watch: ps 실패면 살아 있는 pid의 잠금을 넘겨받지 않고 75", k.rc == 75 && strings.TrimSpace(read(wd+"/watch.t7e5/pid")) == strconv.Itoa(sl.Process.Pid))
}

// E03: reflog 조회 실패는 「기록 없음」이 아니다 — 새 가지를 남기고 오류(1)
func gfReflogFail(k *K) {
	k.mk("gfe3")
	k.sh(k.R, `git pull -q --ff-only && GIT_COMMITTER_DATE='2020-01-01T00:00:00Z' GIT_AUTHOR_DATE='2020-01-01T00:00:00Z' git commit -q --allow-empty -m old && git push -q origin HEAD:main && git branch recent`)
	k.run("gc", "--repo", k.R, "--dry-run")
	k.check("E03 준비: 정상 reflog면 새 가지는 「새것」으로 남김", k.rc == 0 && s19lineHas(k, "recent", "새것"))
	p := k.gfBin(map[string]string{"git": `for a; do [ "$a" = reflog ] && exit 128; done; exec ` + sq(k.gfRealGit()) + ` "$@"`})
	k.runE([]string{p}, "gc", "--repo", k.R, "--dry-run")
	k.goOnly("E03 gc: reflog 실패면 그 가지를 남기고 종료 1", k.rc == 1 && !s19lineHas(k, "recent", "지움") && strings.Contains(k.out, "지움 0"))
}
