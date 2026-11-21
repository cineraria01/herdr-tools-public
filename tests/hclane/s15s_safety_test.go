package hclane_test

// tests/hc-lane.test.sh 15s·16절: 착지 안전(보내는 것은 검증한 커밋 그 자체 — git 실패를 성공·없음으로 읽지 않음, 그룹 Ctrl-C, push 주소)과
// hc-lane guard(공용 git 폴더 pre-push 관문). 두 절 모두 mk로 새 저장소에서 시작해 앞 절 상태에 기대지 않는다.

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func init() { reg(50, "s15s-16", s15sSec) }

func s15sSec(k *K) {
	s15sSafety(k)
	s15sGuard(k)
}

// s15sFake: $T/<폴더>/git 가짜(셸 printf가 만든 바이트 그대로)를 쓰고, 그 폴더를 PATH 앞에 둔 VAR=값을 돌려준다.
func (k *K) s15sFake(dir, script string) string {
	k.t.Helper()
	p := filepath.Join(k.T, dir, "git")
	write(k.t, p, script)
	if err := os.Chmod(p, 0o755); err != nil {
		k.t.Fatal(err)
	}
	return "PATH=" + filepath.Join(k.T, dir) + ":" + k.env["PATH"]
}

// s15sWc: 셸 $(wc -l <파일 2>/dev/null | tr -d ' ') — 파일이 없으면 빈 값.
func s15sWc(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strconv.Itoa(strings.Count(string(b), "\n"))
}

// s15sCount: 셸 grep -c "^<앞>" 파일.
func s15sCount(p, prefix string) int {
	n := 0
	for _, l := range strings.Split(read(p), "\n") {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}
	return n
}

// s15sHasLine: 그 줄이 그대로 있다(grep -Fx / grep -x).
func s15sHasLine(s, x string) bool { return contains(strings.Split(s, "\n"), x) }

func (k *K) s15sOrigin() string { return k.git(k.D+"/origin.git", "rev-parse", "main") }

func s15sSafety(k *K) {
	// 15s. 착지 안전(2026-11-20 land-metrics 레인 — 아스트라 감사·운영 모델): 보내는 것은 검증한 커밋 그 자체
	k.mk("t15s")
	k.seed(".claude/herdr-verify.sh", "exit 0")
	rg, _ := k.exe(k.T, "", nil, "sh", "-c", "command -v git")
	T := k.T
	// 검증하는 동안 HEAD가 바뀌면(검증이 커밋을 더함) 검증하지 않은 커밋을 보내지 않는다
	k.run("--name", "s-mv", "--repo", k.R, "--", "true")
	wt := k.R + ".wt/s-mv"
	k.sh(wt, `printf 'git commit -q --allow-empty -m sneaky; exit 0\n' >.claude/herdr-verify.sh && git commit -qm s-mv -- .claude/herdr-verify.sh`)
	o0 := k.s15sOrigin()
	k.lnd(wt)
	k.check("land: 검증하는 동안 HEAD가 바뀌면 보내지 않음 — 검증 안 한 커밋이 main에 안 들어감",
		k.rc != 0 && k.s15sOrigin() == o0 && strings.Contains(k.errf(), "HEAD가 바뀌었다"))
	// 정상 경로가 고정한 기준 SHA로 rebase하고 후보 SHA를 명시해 push하는지 — 가짜 git이 받은 인자를 그대로 대조
	ar := k.s15sFake("gs-ar", fmt.Sprintf(`#!/bin/sh
case $1 in rebase|push) echo "$*" >>"$HOME/ar.log" ;; esac
exec "%s" "$@"
`, rg))
	k.run("--name", "s-ar", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/s-ar"
	k.sh(wt, `printf 'echo x >>"$HOME/vc.log"; exit 0\n' >.claude/herdr-verify.sh && git commit -qm s-ar -- .claude/herdr-verify.sh`)
	ar0 := k.s15sOrigin()
	os.Remove(T + "/ar.log")
	os.Remove(T + "/vc.log")
	k.lndE([]string{ar}, wt)
	vc1 := s15sWc(T + "/vc.log")
	m := k.s15sOrigin()
	k.check("land: rebase는 고정한 기준 SHA(그때 origin/main)로, push는 원격 이름 origin에 후보 SHA:refs/heads/main으로(HEAD·참조 이름·주소가 아니라)",
		k.rc == 0 && k.otop() == "s-ar" && vc1 == "1" && s15sHasLine(read(T+"/ar.log"), "rebase -q --reapply-cherry-picks "+ar0) &&
			grep(read(T+"/ar.log"), "^push --atomic origin "+m+":refs/heads/main "+m+":refs/hc-landed/s-ar/[0-9]{10}$"))
	// 이미 반영된 깨끗한 레인을 다시 부르면 검증·push 없이 「이미 들어가 있다」·종료 0
	os.Remove(T + "/ar.log")
	k.lndE([]string{ar}, wt)
	vc2 := s15sWc(T + "/vc.log")
	k.check("land: 이미 반영된 깨끗한 레인 재호출은 종료 0·검증 0번·push 0번",
		k.rc == 0 && strings.Contains(k.out, "이미 들어가 있다") && !grep(read(T+"/ar.log"), "^push ") && vc1 == "1" && vc2 == vc1)
	o0 = k.s15sOrigin()
	// rebase 뒤 HEAD 조회가 0인데 값이 이 저장소의 커밋이 아니면 검증·push 안 함(빈 후보로 push하면 main 지우기)
	gh := k.s15sFake("gs-gh", fmt.Sprintf(`#!/bin/sh
m='%s'
[ "$1" = rebase ] && : >"$m"
if [ -e "$m" ] && [ "$1" = rev-parse ]; then for a in "$@"; do [ "$a" = HEAD ] && { [ -z "$GHOST" ] || echo "$GHOST"; exit 0; }; done; fi
exec "%s" "$@"
`, T+"/gs-gh.mark", rg))
	k.mklane("s-gh")
	wt = k.wt
	os.Remove(T + "/gs-gh.mark")
	k.lndE([]string{"GHOST=" + fmt.Sprintf("%040d", 1), gh}, wt)
	k.check("land: rebase 뒤 HEAD 값이 커밋이 아니면(없는 SHA) 아무것도 안 보냄",
		k.rc != 0 && k.s15sOrigin() == o0 && strings.Contains(k.errf(), "커밋이 아니다"))
	os.Remove(T + "/gs-gh.mark")
	k.lndE([]string{"GHOST=", gh}, wt)
	k.check("land: rebase 뒤 HEAD 조회가 빈 값이면 아무것도 안 보냄(빈 후보 = main 지우기)",
		k.rc != 0 && k.git(k.D+"/origin.git", "rev-parse", "-q", "--verify", "main") == o0 && strings.Contains(k.errf(), "HEAD를 읽지 못했다"))
	// 검증이 추적 파일·색인을 바꾸면 검증한 파일과 보낼 커밋이 다르다 — 보내지 않음
	k.run("--name", "s-vt", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/s-vt"
	k.sh(wt, `printf 'echo "# 검증이 고침" >>.claude/herdr-verify.sh; exit 0\n' >.claude/herdr-verify.sh && git commit -qm s-vt -- .claude/herdr-verify.sh`)
	k.lnd(wt)
	k.check("land: 검증이 추적 파일을 바꾸면 push 안 함", k.rc != 0 && k.s15sOrigin() == o0 && strings.Contains(k.errf(), "추적 파일"))
	k.run("--name", "s-vi", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/s-vi"
	k.sh(wt, `echo a >s-vi.txt && git add -- s-vi.txt && printf 'echo b >s-vi.txt; git add -- s-vi.txt; exit 0\n' >.claude/herdr-verify.sh && git commit -qm s-vi -- s-vi.txt .claude/herdr-verify.sh`)
	k.lnd(wt)
	k.check("land: 검증이 색인을 바꿔도(git add) push 안 함", k.rc != 0 && k.s15sOrigin() == o0 && strings.Contains(k.errf(), "추적 파일"))
	// 보낼 커밋이 없어도(후보 = 기준) rebase 뒤 작업 트리 확인은 한다
	tc := k.s15sFake("gs-tc", fmt.Sprintf(`#!/bin/sh
if [ "$1" = rebase ]; then "%s" "$@"; r=$?; echo "# rebase가 고침" >>.claude/herdr-verify.sh; exit $r; fi
exec "%s" "$@"
`, rg, rg))
	k.run("--name", "s-tc", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/s-tc"
	k.lndE([]string{tc}, wt)
	k.check("land: 후보 = 기준이어도 rebase 뒤 추적 파일이 바뀌었으면 「이미 들어가 있다」로 끝내지 않고 멈춤",
		k.rc != 0 && strings.Contains(k.errf(), "rebase 뒤") && !strings.Contains(k.out, "이미 들어가 있다"))
	// git 실패를 성공·없음·「아님」으로 읽지 않는다(운영 모델)
	st := k.s15sFake("gs-st", fmt.Sprintf(`#!/bin/sh
[ "$1" = status ] && exit 128
exec "%s" "$@"
`, rg))
	lost := k.s15sFake("gs-lost", fmt.Sprintf(`#!/bin/sh
if [ "$1" = push ]; then "%s" "$@" >/dev/null 2>&1; exit 1; fi
exec "%s" "$@"
`, rg, rg)) // 들어갔는데 응답이 사라짐
	pf := k.s15sFake("gs-pf", fmt.Sprintf(`#!/bin/sh
m='%s'
if [ "$1" = push ]; then "%s" "$@" >/dev/null 2>&1; : >"$m"; exit 1; fi
case $1 in fetch|ls-remote) [ -e "$m" ] && exit 128 ;; esac
exec "%s" "$@"
`, T+"/gs-pf.mark", rg, rg)) // 응답 유실 + 다시 볼 ls-remote·fetch도 실패
	mb := k.s15sFake("gs-mb", fmt.Sprintf(`#!/bin/sh
m='%s'
if [ "$1" = push ]; then : >"$m"; exit 1; fi
[ "$1" = merge-base ] && [ -e "$m" ] && exit 128
exec "%s" "$@"
`, T+"/gs-mb.mark", rg)) // push 실패 뒤 견주기 실패
	k.mklane("s-st")
	k.lndE([]string{st}, k.wt)
	k.check("land: git status가 실패하면 「깨끗함」으로 읽지 않고 멈춤 — push 안 함",
		k.rc != 0 && k.s15sOrigin() == o0 && strings.Contains(k.errf(), "git status"))
	k.mklane("s-lost")
	k.lndE([]string{lost}, k.wt)
	k.check("land: push 응답이 사라져도(원격에 들어감) fetch해 후보가 원격 기본에 있으면 착지로 끝냄 — 종료 0",
		k.rc == 0 && k.otop() == "s-lost" && strings.Contains(k.out, "들어가 있다"))
	// 착지 뒤 메인 폴더 앞당김 — 저장소 설정 mergeOptions=--squash
	k.mklane("s-sq")
	k.git(k.R, "config", "branch.main.mergeOptions", "--squash")
	k.lndE([]string{lost}, k.wt)
	k.git(k.R, "config", "--unset", "branch.main.mergeOptions")
	k.check("land: 응답 유실 복구 뒤 메인 폴더 앞당김도 squash 설정에 안 바뀜 — 메인 폴더 main = origin/main, 깨끗",
		k.rc == 0 && k.git(k.R, "rev-parse", "main") == k.s15sOrigin() && k.git(k.R, "status", "--porcelain", "--untracked-files=no") == "")
	// 메인 폴더 앞당김의 git 조회 실패. 가짜 git은 메인 폴더 안에서만 실패
	mainf, err := filepath.EvalSymlinks(k.R)
	if err != nil {
		k.t.Fatal(err)
	}
	mf := k.s15sFake("gs-mf", fmt.Sprintf(`#!/bin/sh
if [ -n "$MF_FAIL" ] && [ "$(pwd -P)" = "$MAINF" ]; then
  case "$MF_FAIL:$1" in status:status|lsfiles:ls-files) exit 128 ;; esac
  if [ "$MF_FAIL:$1" = revparse:merge ]; then "%s" "$@"; r=$?; : >"$HOME/mf.merged"; exit $r; fi
  if [ "$MF_FAIL:$1" = revparse:rev-parse ] && [ -e "$HOME/mf.merged" ]; then for a in "$@"; do [ "$a" = HEAD ] && exit 128; done; fi
fi
exec "%s" "$@"
`, rg, rg))
	for _, f := range []string{"status", "lsfiles"} {
		k.mklane("s-mf-" + f)
		lm0 := k.git(k.R, "rev-parse", "main")
		k.lndE([]string{"MAINF=" + mainf, "MF_FAIL=" + f, mf}, k.wt)
		k.check("land: 메인 폴더 앞당김에서 "+f+" 조회가 실패하면 깨끗함으로 읽지 않고 안 앞당김(착지는 성공)",
			k.rc == 0 && k.otop() == "s-mf-"+f && k.git(k.R, "rev-parse", "main") == lm0 && strings.Contains(k.errf(), "앞당김 건너뜀"))
	}
	k.mklane("s-mf-rp")
	os.Remove(T + "/mf.merged")
	k.lndE([]string{"MAINF=" + mainf, "MF_FAIL=revparse", mf}, k.wt)
	k.check("land: 메인 폴더 merge는 성공했는데 HEAD만 못 읽으면 「그대로 둠」이 아니라 merge 성공·최종 HEAD 확인 불가",
		k.rc == 0 && exists(T+"/mf.merged") && k.git(k.R, "rev-parse", "main") == k.s15sOrigin() &&
			strings.Contains(k.errf(), "최종 HEAD") && !strings.Contains(k.errf(), "그대로 둠"))
	k.mklane("s-pf")
	os.Remove(T + "/gs-pf.mark")
	k.lndE([]string{pf}, k.wt)
	k.check("land: push 응답이 사라지고 다시 볼 fetch도 실패하면 착지·거절로 정하지 않고 「모름」으로 멈춤",
		k.rc != 0 && strings.Contains(k.errf(), "모름") && !strings.Contains(k.errf(), "거절됨"))
	k.mklane("s-mb")
	o0 = k.s15sOrigin()
	os.Remove(T + "/gs-mb.mark")
	k.lndE([]string{mb}, k.wt)
	k.check("land: push 실패 뒤 원격과 견주기(merge-base)가 실패하면 「거절」로 읽어 다시 하지 않고 「모름」으로 멈춤",
		exists(T+"/gs-mb.mark") && k.rc != 0 && k.rc != 4 && strings.Contains(k.errf(), "모름") && !strings.Contains(k.errf(), "거절됨") && k.s15sOrigin() == o0)
	// push 전 착지 훅 갈래점 조회(merge-base)부터 실패하면 push를 시도하지 않고 멈춘다
	k.mklane("s-mb0")
	o0 = k.s15sOrigin()
	os.Remove(T + "/gs-mb0.mark")
	mb0 := k.s15sFake("gs-mb0", fmt.Sprintf(`#!/bin/sh
if [ "$1" = push ]; then : >"%s"; exit 1; fi
[ "$1" = merge-base ] && exit 128
exec "%s" "$@"
`, T+"/gs-mb0.mark", rg))
	k.lndE([]string{mb0}, k.wt)
	k.check("land: push 전 착지 훅 갈래점 조회(merge-base)가 실패하면 push를 시도하지 않고 멈춤",
		k.rc != 0 && !exists(T+"/gs-mb0.mark") && strings.Contains(k.errf(), "갈래점") && k.s15sOrigin() == o0)
	// push만 실패(원격은 닿고 그대로): 「push 실패」로 멈추고 다시 하지 않음
	pn := k.s15sFake("gs-pn", fmt.Sprintf(`#!/bin/sh
[ "$1" = push ] && exit 1
exec "%s" "$@"
`, rg))
	k.mklane("s-pn")
	o0 = k.s15sOrigin()
	k.lndE([]string{pn}, k.wt)
	k.check("land: push만 실패하고 원격은 그대로면 「push 실패」로 멈춤 — 거절 아님, 다시 안 함",
		k.rc == 1 && strings.Contains(k.errf(), "다른 쪽 push가 앞선 것이 아니다") && !strings.Contains(k.errf(), "거절됨") && k.s15sOrigin() == o0)
	// 두 번째 견주기만 실패 — 첫 merge-base는 1(없음), 두 번째는 128
	mb2 := k.s15sFake("gs-mb2", fmt.Sprintf(`#!/bin/sh
if [ "$1" = push ]; then echo push >>"$HOME/mb2.log"; exit 1; fi
if [ "$1" = merge-base ] && [ -s "$HOME/mb2.log" ]; then echo mb >>"$HOME/mb2.log"; [ "$(grep -c "^mb" "$HOME/mb2.log")" = 1 ] && exit 1; exit 128; fi
exec "%s" "$@"
`, rg))
	k.mklane("s-mb2")
	o0 = k.s15sOrigin()
	os.Remove(T + "/mb2.log")
	k.lndE([]string{mb2}, k.wt)
	k.check("land: push 실패 뒤 첫 견주기 1·두 번째 견주기 128이면 「모름」으로 멈춤 — 거절 아님, push 다시 0번",
		k.rc != 0 && k.rc != 4 && strings.Contains(k.errf(), "모름") && !strings.Contains(k.errf(), "거절됨") &&
			s15sCount(T+"/mb2.log", "push") == 2 && s15sCount(T+"/mb2.log", "mb") == 2 && k.s15sOrigin() == o0)
	// git remote get-url의 일시 오류(종료 2 아님)를 원격 없음으로 읽으면 원격 저장소인데 로컬 main을 올린다
	gu := k.s15sFake("gs-gu", fmt.Sprintf(`#!/bin/sh
[ "$1 $2" = "remote get-url" ] && exit 128
exec "%s" "$@"
`, rg))
	k.mklane("s-gu")
	lm0 := k.git(k.R, "rev-parse", "main")
	k.lndE([]string{gu}, k.wt)
	k.check("land: git remote get-url이 실패하면 원격 없음으로 읽지 않고 멈춤 — 로컬 main 그대로",
		k.rc != 0 && strings.Contains(k.errf(), "git remote get-url") && k.git(k.R, "rev-parse", "main") == lm0)
	// 메인 폴더를 앞당긴 직후 다른 착지가 더 앞당기면 실제 HEAD를 알린다. 메인 폴더 main이 origin보다 앞서니 이 절 맨 끝에 둔다
	ah := k.s15sFake("gs-ah", fmt.Sprintf(`#!/bin/sh
if [ "$1" = merge ] && [ "$(pwd -P)" = "$MAINF" ]; then "%s" "$@" || exit; exec "%s" commit -q --allow-empty -m further; fi
exec "%s" "$@"
`, rg, rg, rg))
	k.mklane("s-ah")
	k.lndE([]string{"MAINF=" + mainf, ah}, k.wt)
	k.check("land: 메인 폴더가 목표보다 더 앞으로 갔으면(목표를 품음) 「그대로 둠」이 아니라 실제 HEAD로 앞당김",
		k.rc == 0 && k.git(k.R, "log", "-1", "--format=%s", "main") == "further" &&
			k.gitOK(k.R, "merge-base", "--is-ancestor", k.s15sOrigin(), "main") && strings.Contains(k.out, "메인 폴더 앞당김") &&
			strings.Contains(k.out, k.git(k.R, "rev-parse", "--short", "main")) && !strings.Contains(k.errf(), "그대로 둠"))

	s15sSigint(k)
	s15sPushURL(k)
}

// s15sSigint: 착지 스크립트 도중 터미널 Ctrl-C(그룹 SIGINT)면 스크립트도 같이 멈춰 push가 없다.
func s15sSigint(k *K) {
	T := k.T
	k.mk("t15sg")
	// 스크립트는 스스로 시간을 재지 않는다 — 신호를 받거나 부모의 제어 파이프가 닫힐 때까지 기다리고, 「go」 줄을 받았을 때만 push
	k.seed(".claude/herdr-land.sh", `git fetch -q origin && git rebase -q origin/main && echo $$ >"$HOME/sg.pid" && read -r l <"$HOME/sg.fifo" && [ "$l" = go ] || { : >"$HOME/sg.closed"; exit 9; }; HERDR_LAND=1 git push -q origin HEAD:main`)
	k.mklane("s-gi")
	fifo := T + "/sg.fifo"
	for _, f := range []string{"/sg.pid", "/sg.closed", "/sg.fifo"} {
		os.Remove(T + f)
	}
	o0 := k.s15sOrigin()
	if err := syscall.Mkfifo(fifo, 0o666); err != nil {
		k.t.Fatal(err)
	}
	// 셸 exec 7<>: 부모만 쥔 쓰기 끝(Go는 fd를 close-on-exec로 열어 자식에 안 넘긴다 — 셸 7>&-) — 닫으면 스크립트의 read가 EOF로 풀린다
	ctl, err := os.OpenFile(fifo, os.O_RDWR, 0)
	if err != nil {
		k.t.Fatal(err)
	}
	defer ctl.Close()
	// 셸 python 감싸기의 signal(SIGINT, SIG_DFL): 이 시험 프로세스가 SIGINT를 무시한 채 떴어도 자식은 기본값으로 —
	// Go는 fork 뒤 무시가 아닌 처리기만 기본값으로 돌리니, 띄우는 동안만 처리기를 둔다. 제 그룹(setpgid)은 k.bg가
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT)
	b := k.bg(k.wt, "", T+"/err", nil, k.LANE, "land")
	signal.Stop(sigc)
	i := 0
	for ; !(exists(T+"/sg.pid") && read(T+"/sg.pid") != "") && i < 600; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	rdyf := read(T+"/sg.pid") == ""
	ksig := b.signal(true, syscall.SIGINT) == nil // 신호 전송 성공과 hc-lane이 SIGINT로 끝남(130)을 함께 본다
	// 부모의 시간 상한: 10초 안에 안 끝나면 제어 파이프를 닫아 풀어 준다
	for i = 0; !b.Done() && i < 100; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	ctl.Close()
	k.rc = b.Wait()
	sgp, _ := strconv.Atoi(strings.TrimSpace(read(T + "/sg.pid"))) // 스크립트가 끝났는지 그 pid로만 본다(0이면 보지 않음)
	for i = 0; sgp > 0 && alive(sgp) && i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	sgl := sgp > 0 && alive(sgp)
	k.check("land: 착지 스크립트 도중 그룹 Ctrl-C면 스크립트도 끝나 push 없음 — 신호로 끝남(130), 파이프 닫힘으로 풀린 것 아님",
		!rdyf && ksig && !sgl && k.rc == 130 && !exists(T+"/sg.closed") && k.s15sOrigin() == o0)
	if k.t.Failed() {
		k.t.Logf("rc=%d rdyf=%v ksig=%v sgl=%v", k.rc, rdyf, ksig, sgl)
	}
}

// s15sPushURL: origin의 push 주소가 여럿이거나 조회가 실패하거나 검증 도중 바뀌면, 또 includeIf onbranch가 있으면 착지하지 않는다.
func s15sPushURL(k *K) {
	T := k.T
	rg, _ := k.exe(T, "", nil, "sh", "-c", "command -v git")
	k.mk("t15pu")
	k.seed(".claude/herdr-verify.sh", "exit 0")
	k.mklane("p-2")
	wt := k.wt
	o0 := k.s15sOrigin()
	k.git(k.R, "config", "--add", "remote.origin.pushurl", k.D+"/origin.git")
	k.git(k.R, "config", "--add", "remote.origin.pushurl", k.D+"/없는 곳.git")
	k.lnd(wt)
	k.check("land: origin의 push 주소가 둘이면 착지를 시작하지 않음 — 첫 곳에도 안 보냄",
		k.rc != 0 && strings.Contains(k.errf(), "push 주소가 하나가 아니다") && k.s15sOrigin() == o0)
	k.git(k.R, "config", "--unset-all", "remote.origin.pushurl")
	pu := k.s15sFake("gs-pu", fmt.Sprintf(`#!/bin/sh
[ "$1 $2 $3" = "remote get-url --push" ] && exit 128
exec "%s" "$@"
`, rg))
	k.lndE([]string{pu}, wt)
	k.check("land: push 주소 조회가 실패하면 착지하지 않음(모르면 안 함)",
		k.rc != 0 && strings.Contains(k.errf(), "push 주소를 읽지 못했다") && k.s15sOrigin() == o0)
	// 검증 스크립트가 push 주소 둘(origin과 없는 곳)을 더한다 — 보내기 전에 멈추고 origin에도 안 보낸다
	k.run("--name", "p-cv", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/p-cv"
	k.sh(wt, `printf 'u=$(git remote get-url origin); git config --add remote.origin.pushurl "$u"; git config --add remote.origin.pushurl "$u.없음"; exit 0\n' >.claude/herdr-verify.sh && git commit -qm p-cv -- .claude/herdr-verify.sh`)
	o0 = k.s15sOrigin()
	k.lnd(wt)
	pu0 := k.git(k.R, "remote", "get-url", "origin")
	k.check("land: 검증하는 동안 push 주소가 바뀌면 보내지 않음 — origin에도 안 들어감, 진단에 주소를 찍지 않음(아스트라 8차 높음 3)",
		k.rc != 0 && strings.Contains(k.errf(), "push 주소가 바뀌었다") && !strings.Contains(k.errf(), pu0) && k.s15sOrigin() == o0)
	k.git(k.R, "config", "--unset-all", "remote.origin.pushurl")
	// 브랜치별 조건 설정(includeIf onbranch)이 있으면 착지를 시작하지 않는다. 설정 조회가 실패해도 시작하지 않는다
	k.mklane("p-ii")
	wt = k.wt
	o0 = k.s15sOrigin()
	k.git(k.R, "config", "includeIf.onbranch:p-ii.path", T+"/없는.gitconfig")
	k.lnd(wt)
	k.git(k.R, "config", "--unset", "includeIf.onbranch:p-ii.path")
	k.check("land: includeIf onbranch 설정이 있으면 착지를 시작하지 않음 — origin에 안 들어감",
		k.rc != 0 && strings.Contains(k.errf(), "includeIf onbranch") && k.s15sOrigin() == o0)
	ic := k.s15sFake("gs-ic", fmt.Sprintf(`#!/bin/sh
[ "$1 $2" = "config --get-regexp" ] && exit 128
exec "%s" "$@"
`, rg))
	k.lndE([]string{ic}, wt)
	k.check("land: 조건 설정 조회가 실패하면 착지하지 않음(모르면 안 함)",
		k.rc != 0 && strings.Contains(k.errf(), "git 설정을 읽지 못했다") && k.s15sOrigin() == o0)
	// ls-remote는 이름 끝도 맞춘다 — 가짜 git: push는 보내지 않고 실패, ls-remote는 그 앞에 후보를 가리키는 refs/archive/refs/heads/main 줄을 더함
	lr := k.s15sFake("gs-lr", fmt.Sprintf(`#!/bin/sh
[ "$1" = push ] && exit 1
if [ "$1" = ls-remote ]; then printf "%%s\trefs/archive/refs/heads/main\n" "$(git rev-parse HEAD)"; fi
exec "%s" "$@"
`, rg))
	k.mklane("p-lr")
	o0 = k.s15sOrigin()
	k.lndE([]string{lr}, k.wt)
	k.check("land: ls-remote에서 둘째 칸이 정확히 refs/heads/main인 줄만 본다 — 다른 ref의 후보로 착지라 하지 않음",
		k.rc != 0 && !strings.Contains(k.out, "들어가 있다") && k.s15sOrigin() == o0)
}

// s15sPush: 셸 `git -C <폴더> push … 2>"$T/<err>"; prc=$?` 의 종료 코드.
func (k *K) s15sPush(extra []string, errName string, args ...string) int {
	_, rc := k.exe(k.T, filepath.Join(k.T, errName), extra, "git", append([]string{"-C", k.R, "push", "-q", "origin"}, args...)...)
	return rc
}

// s15sGuard: 16. hc-lane guard — 공용 git 폴더 pre-push 관문.
func s15sGuard(k *K) {
	T := k.T
	g := func(args ...string) { k.run(append([]string{"guard"}, args...)...) }
	k.mk("t16")
	hookf := k.R + "/.git/hooks/pre-push"
	g("status", "--repo", k.R)
	k.check("guard status: 없으면 종료 1", k.rc == 1 && strings.Contains(k.out, "없음"))
	g("install", "--repo", k.R)
	fi, err := os.Stat(hookf)
	k.check("guard install: 공용 git 폴더 hooks/pre-push, 실행 가능, 기본 브랜치 main",
		k.rc == 0 && err == nil && fi.Mode()&0o111 != 0 && s15sHasLine(read(hookf), "base=main"))
	g("status", "--repo", k.R)
	k.check("guard status: 깔리면 종료 0", k.rc == 0 && strings.Contains(k.out, "기본 브랜치 main"))
	g("install", "--repo", k.R)
	k.check("guard install: 다시 해도 됨(자기 것은 새로 씀)", k.rc == 0 && strings.Contains(k.out, "새로"))
	k.sh(k.R, `git pull -q --ff-only && echo m >m && git add -- m && git commit -qm m -- m`)
	before := k.otop()
	prc := k.s15sPush(nil, "push.err", "HEAD:main")
	k.check("guard: main으로 가는 손 push는 거절, 새 착지 줄을 알림",
		prc != 0 && strings.Contains(read(T+"/push.err"), "새 착지 줄: hc-lane land main") && k.otop() == before)
	k.run("--name", "g-wt", "--repo", k.R, "--", "true")
	_, prc = k.exe(k.R+".wt/g-wt", T+"/push.err", nil, "bash", "-c", `echo w >w && git add -- w && git commit -qm w -- w && git push -q origin HEAD:main`)
	k.check("guard: 레인 워크트리에서도 거절(공용 훅)", prc != 0 && strings.Contains(read(T+"/push.err"), "hc-lane land main") && k.otop() == before)
	g("status", "--repo", k.R+".wt/g-wt")
	k.check("guard status: 워크트리 경로로 물어도 같은 훅", k.rc == 0)
	prc = k.s15sPush(nil, "push.err", "HEAD:refs/heads/feat/x", "HEAD:refs/integ/x", "HEAD:refs/claims/x")
	k.check("guard: 다른 브랜치·refs/integ·refs/claims는 통과", prc == 0 &&
		k.gitOK(k.D+"/origin.git", "show-ref", "-q", "--verify", "refs/heads/feat/x") &&
		k.gitOK(k.D+"/origin.git", "show-ref", "-q", "--verify", "refs/integ/x") &&
		k.gitOK(k.D+"/origin.git", "show-ref", "-q", "--verify", "refs/claims/x"))
	prc = k.s15sPush(nil, "push.err", "HEAD:refs/heads/feat/y", "HEAD:main")
	k.check("guard: 여러 참조 가운데 main이 섞이면 전부 거절",
		prc != 0 && k.otop() == before && !k.gitOK(k.D+"/origin.git", "show-ref", "-q", "--verify", "refs/heads/feat/y"))
	prc = k.s15sPush(nil, "push.err", ":main")
	k.check("guard: main 지우기 push도 거절", prc != 0 && k.otop() == before)
	prc = k.s15sPush([]string{"HERDR_LAND=1"}, "push.err", "HEAD:main")
	k.check("guard: HERDR_LAND=1이면 main push 통과", prc == 0 && k.otop() == "m")
	k.seedp(".claude/herdr-verify.sh", "exit 0")
	k.lnd(k.R + ".wt/g-wt")
	k.check("guard + land: hc-lane land는 관문을 지나 착지", k.rc == 0 && k.otop() == "w")
	g("remove", "--repo", k.R)
	k.check("guard remove: 자기 것을 지움", k.rc == 0 && !exists(hookf))
	_, prc = k.exe(k.R, T+"/push.err", nil, "bash", "-c", `git pull -q --ff-only && echo m2 >m2 && git add -- m2 && git commit -qm m2 -- m2 && git push -q origin HEAD:main`)
	k.check("guard remove: 그 뒤 손 push 통과", prc == 0 && k.otop() == "m2")
	g("remove", "--repo", k.R)
	k.check("guard remove: 없으면 지울 것 없음, 종료 0", k.rc == 0 && strings.Contains(k.out, "지울 것 없음"))
	g("install", "--repo", k.R, "--branch", "release")
	prc = k.s15sPush(nil, "push.err", "HEAD:main", "HEAD:refs/heads/next")
	prc2 := k.s15sPush(nil, "push2.err", "HEAD:refs/heads/release")
	k.check("guard install --branch: 그 브랜치만 막음", prc == 0 && prc2 != 0 && strings.Contains(read(T+"/push2.err"), "hc-lane land release"))
	g("remove", "--repo", k.R)

	k.mk("t16o") // 남의 pre-push 훅·core.hooksPath는 건드리지 않는다
	hookf = k.R + "/.git/hooks/pre-push"
	write(k.t, hookf, "#!/bin/sh\necho mine\n")
	if err := os.Chmod(hookf, 0o755); err != nil {
		k.t.Fatal(err)
	}
	g("install", "--repo", k.R)
	hl := strings.Split(read(hookf), "\n")
	k.check("guard install: 다른 pre-push 훅이 있으면 덮지 않고 멈춤", k.rc != 0 && strings.Contains(k.errf(), "다른 pre-push") && len(hl) > 1 && hl[1] == "echo mine")
	g("status", "--repo", k.R)
	k.check("guard status: 다른 훅이면 종료 3", k.rc == 3)
	g("remove", "--repo", k.R)
	k.check("guard remove: 남의 훅은 지우지 않음", k.rc != 0 && exists(hookf))
	os.Remove(hookf)
	k.git(k.R, "config", "core.hooksPath", ".githooks")
	g("install", "--repo", k.R)
	k.check("guard install: core.hooksPath가 있으면 멈추고 아무것도 안 씀",
		k.rc != 0 && strings.Contains(k.errf(), "core.hooksPath") && !exists(hookf) && !exists(k.R+"/.githooks/pre-push"))
	k.git(k.R, "config", "--unset", "core.hooksPath")
	g("bogus", "--repo", k.R)
	k.check("guard: 모르는 동작은 사용법(종료 2)", k.rc == 2)
	g("install")
	k.check("guard: --repo 없으면 사용법(종료 2)", k.rc == 2)
}
