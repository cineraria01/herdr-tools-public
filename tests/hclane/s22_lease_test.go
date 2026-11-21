package hclane_test

// tests/hc-lane.test.sh 22·23절: 레인 임대·착지 표식(lease·land·clean·herdr-land.sh), early-wait, 착지 줄 우선,
// 주차·이어 띄우기(park·--resume).
// 앞 절(21)에서 물려받는 것: export HC_FAKE_LS='{"terminals":[]}'(가짜 hc ls의 기본 답과 같아 결과가 같다)뿐이고,
// 셸 변수 tab·EVF·AF는 여기서 다시 정한다.

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func init() { reg(120, "s22-23", leaseParkSections) }

const s22tab = "\t"

func s22lref(k *K, name string) string {
	return k.git(k.D+"/origin.git", "rev-parse", "-q", "--verify", "refs/hc-lanes/"+name)
}
func s22lsub(k *K, name string) string {
	return k.git(k.D+"/origin.git", "log", "-1", "--format=%s", "refs/hc-lanes/"+name)
}
func s22mrefs(k *K, name string) string {
	return k.git(k.D+"/origin.git", "for-each-ref", "--format=%(refname) %(objectname)", "refs/hc-landed/"+name+"/")
}
func s22pref(k *K, name string) string {
	return k.git(k.D+"/origin.git", "rev-parse", "-q", "--verify", "refs/park/"+name)
}

// s22nth: 파일의 n번째 줄(1부터 — head -n 1·sed -n 2p). 없으면 빈 값.
func s22nth(p string, n int) string {
	l := strings.Split(read(p), "\n")
	if n-1 < len(l) {
		return l[n-1]
	}
	return ""
}

// s22commit: 작업 트리에서 파일 하나를 커밋(셸 (cd "$wt" && echo … && git add && git commit)).
func s22commit(k *K, dir, file, content, msg string) {
	k.t.Helper()
	k.sh(dir, `echo "$2" >"$1" && git add -- "$1" && git commit -qm "$3" -- "$1"`, file, content, msg)
}

func s22exec(k *K, p, s string) {
	k.t.Helper()
	write(k.t, p, s)
	if err := os.Chmod(p, 0o755); err != nil {
		k.t.Fatal(err)
	}
}

func leaseParkSections(k *K) {
	s22lease(k)
	s22landHerdr(k)
	s22earlyWait(k)
	s22landPrio(k)
	s23park(k)
}

func s22lease(k *K) {
	// 22. 레인 임대·착지 표식
	k.mk("t22")
	k.seed(".claude/herdr-verify.sh", "exit 0")
	R2 := k.D + "/둘째 기계"
	k.sh(k.T, `git clone -q "$1/origin.git" "$2" 2>/dev/null`, k.D, R2)
	k.run("--name", "ls-a", "--repo", k.R, "--", "claude")
	la := s22lref(k, "ls-a")
	k.check("임대: 띄우면 원격에 refs/hc-lanes/<레인>(빈 트리, 제목 hc-lane lease <레인> <기계> <UTC>), 레인 환경 HC_LANE_LEASE = 그 값",
		k.rc == 0 && la != "" && grep(s22lsub(k, "ls-a"), `^hc-lane lease ls-a [^ ]+ 20[0-9-]+T[0-9:]+Z$`) && k.has("HC_LANE_LEASE="+la))
	r0 := k.runs()
	k.run("--name", "ls-a", "--repo", R2, "--", "claude")
	k.check("임대: 다른 기계(둘째 클론)에서 같은 이름이면 「이미 돎」 종료 10, hc run 안 부름, 워크트리 되돌림, 임대 그대로",
		k.rc == 10 && k.runs() == r0 && !exists(R2+".wt/ls-a") && !k.gitOK(R2, "show-ref", "-q", "--verify", "refs/heads/ls-a") &&
			s22lref(k, "ls-a") == la && strings.Contains(k.errf(), "이미 돎") && strings.Contains(k.errf(), "hc-lane lease drop --repo"))
	k.run("--read", "--name", "ls-rd", "--repo", R2, "--", "claude")
	k.check("임대: --read(조사·감사)는 임대를 잡지 않음", k.rc == 0 && s22lref(k, "ls-rd") == "")
	// push 응답 유실: 원격에는 들어갔는데 push가 실패로 끝남 → 원격 값이 자기 것이면 잡은 것(띄움), 다른 기계의 다시 시도는 10
	RG, _ := k.exe(k.T, "", nil, "bash", "-c", "command -v git")
	s22exec(k, k.T+"/lostgit/git", "#!/bin/bash\n\""+RG+"\" \"$@\"; r=$?\ncase \" $* \" in *\" push \"*refs/hc-lanes/*) exit 128 ;; esac\nexit $r\n")
	s22exec(k, k.T+"/downgit/git", "#!/bin/bash\ncase \" $* \" in *\" push \"*refs/hc-lanes/*|*\" ls-remote \"*) echo \"fatal: 원격 닿지 않음(가짜)\" >&2; exit 128 ;; esac\nexec \""+RG+"\" \"$@\"\n")
	lost := []string{"PATH=" + k.T + "/lostgit:" + k.env["PATH"]}
	down := []string{"PATH=" + k.T + "/downgit:" + k.env["PATH"]}
	k.runE(lost, "--name", "ls-l", "--repo", k.R, "--", "claude")
	ll := s22lref(k, "ls-l")
	k.check("임대 응답 유실: push가 실패로 끝나도 원격 값이 이 실행의 것이면 띄움(같은 값을 레인 환경에)", k.rc == 0 && ll != "" && k.has("HC_LANE_LEASE="+ll))
	r0 = k.runs()
	k.runE(lost, "--name", "ls-l", "--repo", R2, "--", "claude")
	k.check("임대 응답 유실 뒤 다른 기계에서 다시 시도: 종료 10, 안 띄움(둘이 뜨지 않음)", k.rc == 10 && k.runs() == r0 && s22lref(k, "ls-l") == ll)
	r0 = k.runs()
	k.runE(down, "--name", "ls-d", "--repo", k.R, "--", "claude")
	k.check("임대 조회·push 실패(원격 닿지 않음): 종료 1, hc run 안 부름(모르면 안 띄움), 방금 만든 워크트리 지움",
		k.rc == 1 && k.runs() == r0 && !exists(k.R+".wt/ls-d") && s22lref(k, "ls-d") == "" && strings.Contains(k.errf(), "임대를 잡지 못했다"))
	r0 = k.runs()
	k.runE([]string{"HC_FAKE_LEASE_EXEC4=1"}, "--on", "box", "--name", "ls-u", "--repo", k.R, "--", "claude")
	k.check("임대 --on: 코어가 종료 4(기계 조회 실패)면 원격 없음으로 읽지 않고 종료 1, 안 띄움(아스트라 4차 높음 3)",
		k.rc == 1 && k.runs() == r0 && !exists(k.R+".wt/ls-u") && s22lref(k, "ls-u") == "" && strings.Contains(k.errf(), "원격 없음 확인 못 함"))
	k.run("--on", "box", "--name", "ls-v", "--repo", R2, "--", "claude")
	k.check("임대 --on: 그 기계의 로그인 셸(hc exec)로 잡음", k.rc == 0 && s22lref(k, "ls-v") != "" && strings.Contains(read(k.HCLOG+".exec"), "hc-lane-lease"))
	k.run("lease", "drop", "--repo", k.R, "--name", " ")
	k.check("lease: 공백뿐인 --name은 거절(push 안 함)", k.rc != 0 && s22lref(k, "ls-v") != "")
	// 손으로: lease show·drop
	k.run("lease", "show", "--repo", k.R, "--name", "ls-l")
	k.check("lease show: 「값 제목」", k.rc == 0 && k.out == ll+" "+s22lsub(k, "ls-l"))
	k.run("lease", "drop", "--repo", R2, "--name", "ls-l")
	k.check("lease drop: 놓음(다른 클론에서도), 그 뒤 다른 기계가 띄울 수 있음", k.rc == 0 && s22lref(k, "ls-l") == "" && strings.Contains(k.out, "놓음 refs/hc-lanes/ls-l"))
	k.run("lease", "show", "--repo", k.R, "--name", "ls-l")
	k.check("lease show: 없으면 「-」", k.rc == 0 && k.out == "-")
	k.run("lease", "show", "--repo", k.T+"/없는 저장소", "--name", "ls-l")
	k.check("lease show: 저장소가 아니면 조회 실패(종료 1 — 임대 없음으로 읽지 않음)", k.rc == 1)
	k.run("lease", "drop", "--repo", k.R, "--name", "ls-l", "--branch", "x")
	k.check("lease: --on·--repo·--name 밖의 인자는 거절", k.rc != 0 && k.out == "")
	// land: main·표식·임대 지우기가 한 --atomic push. pre-receive는 한 push에 든 ref를 적기만, update 훅은 ref마다 「ref old」를 적고
	// $T/rejlease가 있으면 임대 ref 하나만 거절한다
	recv, post, rej := k.T+"/recv.log", k.T+"/post.log", k.T+"/rejlease"
	hooks := k.D + "/origin.git/hooks/"
	s22exec(k, hooks+"pre-receive", "#!/bin/sh\nr=; while read -r o n ref; do r=\"$r $ref\"; done; echo \"push$r\" >>\""+recv+"\"\nexit 0\n")
	s22exec(k, hooks+"update", "#!/bin/sh\ncase $1 in refs/hc-lanes/*) [ ! -f \""+rej+"\" ] || { echo \"임대 거절(가짜)\" >&2; exit 1; } ;; esac\nexit 0\n")
	// post-receive는 실제로 바뀐 ref만 받는다
	s22exec(k, hooks+"post-receive", "#!/bin/sh\nr=; while read -r o n ref; do r=\"$r $ref\"; done; echo \"done$r\" >>\""+post+"\"\n")
	wt := k.R + ".wt/ls-a"
	s22commit(k, wt, "ls-a.txt", "a", "feat(x): ls-a-1 다른 제목")
	k.seedp("c", "c")
	os.Remove(recv)
	k.runIn(wt, []string{"HC_LANE_LEASE=" + la}, "land", "main")
	m := k.git(k.D+"/origin.git", "rev-parse", "main")
	rl := read(recv)
	k.check("land: 표식 refs/hc-landed/<레인>/<UTC 초> = 착지 커밋, 임대 지움 — main과 한 push(--atomic)",
		k.rc == 0 && k.otop() == "feat(x): ls-a-1 다른 제목" && grep(s22mrefs(k, "ls-a"), "^refs/hc-landed/ls-a/[0-9]{10} "+m+"$") && s22lref(k, "ls-a") == "" &&
			s22nonEmpty(rl) == 1 && grep(rl, `^push( [^ ]+)*`) && strings.Contains(rl, "refs/heads/main") && strings.Contains(rl, "refs/hc-landed/ls-a/") && strings.Contains(rl, "refs/hc-lanes/ls-a"))
	// 원자: 임대 쪽이 거절되면 그 push는 main도 안 바뀐다 → main만 다시(검증 다시 안 돎), 표식은 따로, 임대는 못 놓았다고 알림
	k.run("--name", "ls-b", "--repo", k.R, "--", "claude")
	lb := s22lref(k, "ls-b")
	wt = k.R + ".wt/ls-b"
	s22commit(k, wt, "ls-b.txt", "b", "ls-b")
	k.seedp("d", "d")
	os.Remove(recv)
	os.Remove(post)
	write(k.t, rej, "")
	k.runIn(wt, []string{"HC_LANE_LEASE=" + lb}, "land", "main")
	os.Remove(rej)
	k.check("land 원자: 임대 ref 하나만 거절돼도 그 push의 main·표식은 안 바뀜(첫 push 통째 취소) → main만 다시 착지, 표식 따로, 임대는 남고 알림",
		k.rc == 0 && k.otop() == "ls-b" && strings.Contains(s22nth(recv, 1), "refs/hc-lanes/ls-b") && s22nth(post, 1) == "done refs/heads/main" &&
			s22nth(recv, 2) == "push refs/heads/main" && s22mrefs(k, "ls-b") != "" && s22lref(k, "ls-b") == lb && strings.Contains(k.errf(), "임대를 놓지 못했다"))
	k.exe(k.T, "", nil, k.LANE, "lease", "drop", "--repo", k.R, "--name", "ls-b")
	// 임대가 이 실행 값이 아니면(손으로 놓고 다른 실행이 잡음) 지우지 않고 착지·표식만
	k.run("--name", "ls-c", "--repo", k.R, "--", "claude")
	wt = k.R + ".wt/ls-c"
	s22commit(k, wt, "ls-c.txt", "c", "ls-c")
	k.exe(k.T, "", nil, k.LANE, "lease", "drop", "--repo", k.R, "--name", "ls-c")
	k.out, _ = k.exe(k.T, "", nil, k.LANE, "--name", "ls-c", "--repo", R2, "--", "claude")
	lc := s22lref(k, "ls-c")
	k.seedp("e", "e")
	k.runIn(wt, []string{"HC_LANE_LEASE=" + fmt.Sprintf("%040d", 0)}, "land", "main") // 40자 0(공개 검사의 16진 꼴에 안 걸리게)
	k.check("land: 임대가 다른 실행의 것이면 그대로 두고 착지·표식만", k.rc == 0 && k.otop() == "ls-c" && s22mrefs(k, "ls-c") != "" && lc != "" && s22lref(k, "ls-c") == lc)
	// clean이 임대를 놓는다(띄운 뒤 아무것도 안 한 레인)
	k.run("--name", "ls-e", "--repo", k.R, "--", "claude")
	k.check("clean 준비: 임대 있음", s22lref(k, "ls-e") != "")
	k.runIn(k.R, nil, "clean", "--repo", k.R, "--branch", "ls-e")
	k.check("clean: 정리하면서 임대 refs/hc-lanes/<레인>도 놓음", k.rc == 0 && s22lref(k, "ls-e") == "" && strings.Contains(k.out, "임대 확인: refs/hc-lanes/ls-e 없음(놓음)"))
}

// s22nonEmpty: 빈 줄이 아닌 줄 수(grep -c .).
func s22nonEmpty(s string) int {
	n := 0
	for _, l := range strings.Split(s, "\n") {
		if l != "" {
			n++
		}
	}
	return n
}

func s22landHerdr(k *K) {
	// herdr-land.sh: HERDR_LAND_PUSH_EXTRA로 표식(과 임대 지우기)을 넘김. 그 스크립트가 실으면 한 push, 안 실으면 land_after가 따로
	k.mk("t22h")
	k.seed(".claude/herdr-verify.sh", "exit 0")
	k.seed(".claude/herdr-land.sh", `echo "extra=$HERDR_LAND_PUSH_EXTRA"; git fetch -q origin && git rebase -q "origin/$1" || exit 3; if [ -f "$HOME/useextra" ]; then git push -q --atomic origin "HEAD:refs/heads/$1" $HERDR_LAND_PUSH_EXTRA; else git push -q origin "HEAD:refs/heads/$1"; fi`)
	k.run("--name", "lh-a", "--repo", k.R, "--", "claude")
	lh := s22lref(k, "lh-a")
	wt := k.R + ".wt/lh-a"
	s22commit(k, wt, "lh-a.txt", "h", "lh-a")
	write(k.t, k.T+"/useextra", "")
	k.runIn(wt, []string{"HC_LANE_LEASE=" + lh}, "land", "main")
	os.Remove(k.T + "/useextra")
	m := k.git(k.D+"/origin.git", "rev-parse", "main")
	k.check("herdr-land.sh: 표식·임대 삭제·소유 SHA를 main과 함께 보냄", k.rc == 0 &&
		grep(k.out, "^extra=HEAD:refs/hc-landed/lh-a/[0-9]{10} :refs/hc-lanes/lh-a --force-with-lease=refs/hc-lanes/lh-a:"+lh+"$") &&
		strings.Contains(s22mrefs(k, "lh-a"), " "+m) && s22lref(k, "lh-a") == "")
	k.run("--name", "lh-b", "--repo", k.R, "--", "claude")
	lh = s22lref(k, "lh-b")
	wt = k.R + ".wt/lh-b"
	s22commit(k, wt, "lh-b.txt", "h", "lh-b")
	k.runIn(wt, []string{"HC_LANE_LEASE=" + lh}, "land", "main")
	m = k.git(k.D+"/origin.git", "rev-parse", "main")
	k.check("herdr-land.sh가 EXTRA를 안 실어도: 착지 뒤 표식을 따로 두고(그 커밋이 origin/main에 들어갔을 때) 임대를 놓음",
		k.rc == 0 && k.otop() == "lh-b" && strings.Contains(s22mrefs(k, "lh-b"), " "+m) && s22lref(k, "lh-b") == "")
}

func s22earlyWait(k *K) {
	// fill-dep-needs: hc-fill이 써 준 선행 상태(~/.local/share/hc-fill/after.tsv)를 early-wait가 본다
	AF := k.T + "/.local/share/hc-fill/after.tsv"
	now := strconv.FormatInt(time.Now().Unix(), 10)
	write(k.t, AF, ev("pa", "go", "진행 중(working)", now)+ev("pb", "go", "대기열에서 기다림", now)+ev("pz", "stop", "터미널 없음", now))
	k.run("early-wait", "pa", "pb")
	k.check("early-wait: 선행이 모두 go·새 기록이면 0, 선행마다 까닭 한 줄", k.rc == 0 && strings.Contains(k.out, "pa: 진행 중(working)") && strings.Contains(k.out, "pb: 대기열에서 기다림"))
	write(k.t, AF, ev("pa", "go", "x", now)+ev("pb", "stop", "막힘: 시험 실패", now))
	k.run("early-wait", "pa", "pb")
	k.check("early-wait: stop 선행이 하나라도 있으면 1, 그 선행과 까닭", k.rc == 1 && strings.Contains(k.out, "pb: 막힘: 시험 실패"))
	k.run("early-wait", "pa", "zz")
	k.check("early-wait: 기록에 없는 선행은 1(상태 모름)", k.rc == 1 && strings.Contains(k.out, "zz: "))
	n, _ := strconv.ParseInt(now, 10, 64)
	write(k.t, AF, ev("pa", "go", "x", strconv.FormatInt(n-1200, 10)))
	k.run("early-wait", "pa")
	k.check("early-wait: 기록이 15분 넘게 묵으면 1(hc-fill이 안 돎 — 예전처럼 멈춤)", k.rc == 1 && strings.Contains(k.out, "묵"))
	os.Remove(AF)
	k.run("early-wait", "pa")
	k.check("early-wait: 기록 파일이 없으면 1", k.rc == 1)
	k.run("early-wait")
	k.check("early-wait: 선행 없이 부르면 종료 2", k.rc == 2)
}

func s22landPrio(k *K) {
	// 착지 줄 우선: HERDR_LAND_PRIORITY를 herdr-land.sh에 넘긴다(숫자 아니면 0)
	k.mk("t15p")
	k.seed(".claude/herdr-land.sh", `echo "land-prio ${HERDR_LAND_PRIORITY-없음}"`)
	k.mklane("l-lp")
	k.lndE([]string{"HERDR_LAND_PRIORITY=3"}, k.wt, "main")
	k.check("land: HERDR_LAND_PRIORITY를 herdr-land.sh에 넘김", k.rc == 0 && contains(strings.Split(k.out, "\n"), "land-prio 3"))
	k.lndE([]string{"HERDR_LAND_PRIORITY=1;x"}, k.wt, "main")
	k.check("land: 숫자가 아니면 0", contains(strings.Split(k.out, "\n"), "land-prio 0"))
	k.lnd(k.wt, "main")
	k.check("land: 없으면 0", contains(strings.Split(k.out, "\n"), "land-prio 0"))
	os.Remove(k.HCLOG)
	k.runE([]string{"HERDR_LAND_PRIORITY=2", "HC_LANE_FILL=1", "HC_FAKE_RUN_ADDR=t9lp"}, "--start-wait", "0", "--name", "lane-lp", "--repo", k.R, "--", "claude")
	k.check("띄우기: HERDR_LAND_PRIORITY를 레인 환경에", k.rc == 0 && k.has("HERDR_LAND_PRIORITY=2"))
}

func s23park(k *K) {
	// 23. 주차·이어 띄우기
	k.mk("t23")
	k.seed(".claude/herdr-verify.sh", "exit 0")
	EVF := k.T + "/.local/state/hc-lane/events.tsv"
	k.run("--name", "pk-a", "--repo", k.R, "--", "claude")
	pa := s22lref(k, "pk-a")
	wt := k.R + ".wt/pk-a"
	write(k.t, wt+"/pk.txt", "prep\n")
	k.runIn(wt, []string{"HC_LANE=pk-a", "HC_LANE_LEASE=" + pa}, "park")
	k.check("park: 커밋 안 된 새 파일이 있으면 종료 1, 원격 refs/park/<레인> 안 만듦·임대 그대로", k.rc == 1 && s22pref(k, "pk-a") == "" && s22lref(k, "pk-a") == pa)
	k.sh(wt, `git add -- pk.txt && git commit -qm "pk-a: 준비" -- pk.txt`)
	ps := k.git(wt, "rev-parse", "HEAD")
	k.runIn(wt, []string{"HC_LANE=pk-a", "HC_LANE_LEASE=" + pa}, "park", "실패 시험만")
	evl := lastLine(strings.TrimRight(read(EVF), "\n"))
	k.check("park: HEAD를 원격 refs/park/<레인>에(lane 가지는 안 만듦), 임대는 같은 push로 놓음, 「주차」 사건(sha·한 줄)",
		k.rc == 0 && s22pref(k, "pk-a") == ps && !k.gitOK(k.D+"/origin.git", "rev-parse", "-q", "--verify", "refs/heads/lane/pk-a") && s22lref(k, "pk-a") == "" &&
			strings.Contains(evl, s22tab+"pk-a"+s22tab+"주차"+s22tab+ps+s22tab) && strings.Contains(evl, "실패 시험만") && strings.Contains(k.out, "임대 놓음"))
	k.runIn(wt, []string{"HC_LANE=pk-a"}, "park")
	k.check("park: 같은 커밋으로 다시 불러도 종료 0(같은 값으로 모임)", k.rc == 0 && s22pref(k, "pk-a") == ps)
	k.runIn(k.R, nil, "clean", "--repo", k.R, "--branch", "pk-a")
	k.check("park 뒤 clean: 원격 refs/park/<레인>에 그대로 있는 가지라 「주차됨」으로 지움(자리 비움)",
		k.rc == 0 && !exists(wt) && !k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/pk-a") && strings.Contains(k.out, "주차됨: 브랜치 pk-a 은 원격 refs/park/pk-a"))
	k.seedp("c", "c") // 그사이 선행이 main에 들어감
	k.run("--resume", "--name", "pk-a", "--repo", k.R, "--", "claude")
	k.check("--resume: 주차한 가지에서 새 작업 트리(준비 커밋 그대로), upstream은 origin/main, 임대 새로 잡음",
		k.rc == 0 && k.git(wt, "rev-parse", "HEAD") == ps && k.git(wt, "rev-parse", "--abbrev-ref", "@{u}") == "origin/main" && s22lref(k, "pk-a") != "")
	k.run("--resume", "--name", "pk-z", "--repo", k.R, "--", "claude")
	k.check("--resume: 주차한 가지가 없으면 안 띄움(종료 1, 작업 트리 없음)", k.rc != 0 && !exists(k.R+".wt/pk-z") && strings.Contains(k.errf(), "refs/park/pk-z"))
	// 옛 판이 주차한 lane/<레인>(refs/park/<레인> 없음)도 받는다
	k.git(k.D+"/origin.git", "update-ref", "refs/heads/lane/pk-o", ps)
	k.run("--resume", "--name", "pk-o", "--repo", k.R, "--", "claude")
	k.check("--resume: refs/park 없으면 옛 주차 가지 lane/<레인>에서", k.rc == 0 && k.git(k.R+".wt/pk-o", "rev-parse", "HEAD") == ps)
	k.run("--read", "--resume", "--name", "pk-r", "--repo", k.R, "--", "claude")
	k.check("--resume은 --read와 함께 쓰지 않음", k.rc != 0)
	k.mk("t23n")
	k.git(k.R, "remote", "remove", "origin")
	mkdirs(k.t, k.R+".wt")
	k.git(k.R, "worktree", "add", "-q", "-b", "pk-n", k.R+".wt/pk-n")
	k.runIn(k.R+".wt/pk-n", []string{"HC_LANE=pk-n"}, "park")
	k.check("park: 원격 없는 저장소는 종료 3(기다림으로)", k.rc == 3)
}
