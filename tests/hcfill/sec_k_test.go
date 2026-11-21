package hcfill_test

// gofix-fill(2026-11-21 Go 전환 전체 감사): 바깥 명령 시간 상한에 걸린 결과는 「모름」 — 상태를 바꾸지 않고 다음 바퀴에.
// 셸 판은 상한이 없어(늦어도 끝까지 기다림) 같은 입력에서 늘 맞게 끝난다 — 두 판 모두 돈다.

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func init() { reg(96, "k", secK) }

func secK(k *K) {
	secKLaunchTimeout(k)
	secKFetchTimeout(k)
	secKCoordTimeout(k)
	secKReadFail(k)
	secKOrphan(k)
	secKRoundTimeout(k)
	secKLocalLimits(k)
	secKThenRace(k)
	secKNeedTwo(k)
	secKLsUnknown(k)
	secKUmask(k)
	secKPaste(k)
}

// F04(아스트라): run이 SIGKILL로 죽어도 상한 걸어 따로 무리로 띄운 자식(hc ls)이 다음 run과 겹치지 않는다 —
// 다음 run이 첫 바깥 명령 전에 고아 기록으로 그 무리를 거둔다(gofix-core internal/proc 고아 기록·거둠)
func secKOrphan(k *K) {
	k.fresh()
	pf := k.T + "/ls-slow.pid"
	rm(pf)
	bg := exec.Command(k.FILL, "run")
	bg.Dir, bg.Env = k.T, k.environ([]string{"HC_FAKE_LS_SLOW=30"})
	bg.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := bg.Start(); err != nil {
		k.t.Fatal(err)
	}
	for i := 0; i < 100 && !nonEmpty(pf); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	syscall.Kill(-bg.Process.Pid, syscall.SIGKILL) // run 무리째(사람·hc-sweep·재시작이 끄는 꼴)
	bg.Wait()
	pid, _ := strconv.Atoi(strings.TrimSpace(read(pf)))
	alive := func() bool { return pid > 0 && syscall.Kill(pid, 0) == nil }
	k.run("run")
	gone := false
	for i := 0; i < 50; i++ {
		if gone = !alive(); gone {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	k.check("run이 SIGKILL로 죽은 뒤 다음 run: 남은 hc ls 무리를 거둠(겹치지 않음)", pid > 0 && gone && k.rc == 0, fmt.Sprint(pid), k.out)
	if alive() {
		syscall.Kill(pid, syscall.SIGKILL)
	}
}

// T2 ③④(Fable)·L2: watch가 바퀴 상한(HC_FILL_ROUND_TIMEOUT)에 run을 TERM으로 끊으면 run은 상한 걸린 자식 무리(hc ls)를 거두고
// 끝내기 훅(held.tsv 비움·지시문 사본 지움)을 돈다. 셸 판에는 바퀴 상한이 없다(Go 판만)
func secKRoundTimeout(k *K) {
	if k.impl == "bash" {
		return
	}
	k.fresh()
	pf := k.T + "/ls-slow.pid"
	rm(pf)
	write(k.t, k.DATA+"/held.tsv", "x\t1\n")
	write(k.t, k.DATA+"/.run-brief.tmp", "옛 사본\n")
	wout, err := os.Create(k.T + "/watch2.out")
	if err != nil {
		k.t.Fatal(err)
	}
	defer wout.Close()
	bg := exec.Command(k.FILL, "watch", "--every", "60")
	bg.Dir, bg.Env, bg.Stdout, bg.Stderr = k.T, k.environ([]string{"HC_FAKE_LS_SLOW=30", "HC_FILL_ROUND_TIMEOUT=2"}), wout, wout
	bg.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := bg.Start(); err != nil {
		k.t.Fatal(err)
	}
	for i := 0; i < 100 && !strings.Contains(read(k.T+"/watch2.out"), "바퀴를 끝낸다"); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(read(pf)))
	gone := false
	for i := 0; i < 50; i++ {
		if gone = pid > 0 && syscall.Kill(pid, 0) != nil; gone {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	syscall.Kill(-bg.Process.Pid, syscall.SIGTERM)
	bg.Wait()
	k.check("바퀴 상한: run을 끊고(바퀴를 끝낸다) 상한 걸린 hc ls 무리도 거둠", strings.Contains(read(k.T+"/watch2.out"), "바퀴를 끝낸다") && gone,
		fmt.Sprint(pid), read(k.T+"/watch2.out"))
	k.check("바퀴 상한으로 끝난 run도 끝내기 훅을 돈다(held.tsv 비움·지시문 사본 지움)", !nonEmpty(k.DATA+"/held.tsv") && !exists(k.DATA+"/.run-brief.tmp"),
		read(k.DATA+"/held.tsv"))
	if pid > 0 && syscall.Kill(pid, 0) == nil {
		syscall.Kill(pid, syscall.SIGKILL)
	}
}

// F3(Fable)·E01(아스트라): 이 기계 감사의 완료 sha 풀기(git rev-parse)와 herdr-balance도 상한 안에서만 — 멈추면 run.lock을 쥔 채
// 끝없이 기다렸다. 셸 판은 상한이 없다(Go 판만)
func secKLocalLimits(k *K) {
	if k.impl == "bash" {
		return
	}
	// herdr-balance가 답하지 않음: 상한(1초)마다 끊고 다섯 번 뒤 알림 — run은 끝난다
	k.fresh()
	k.add("hb", "--only", "here")
	t0 := time.Now()
	k.runE([]string{"HC_FAKE_BALANCE_SLOW=30", "HC_FILL_BALANCE_TIMEOUT=1"}, "run")
	k.check("herdr-balance가 멈춰도 상한 안에 끝남(분할 붙이기 실패 알림, 레인은 뜸)", time.Since(t0) < 25*time.Second && k.state("hb") == "up" &&
		strings.Contains(k.out, "분할 붙이기 실패"), time.Since(t0).String(), k.out)
	// 이 기계 레인의 완료 sha 풀기가 늦음: 상한(1초)에 끊고 감사를 미룸(다음 바퀴에)
	k.afresh()
	k.add("lr", "--only", "here")
	k.run("run")
	k.lj("lr:claude-code:lr:done:/")
	k.ev("lr", "lr", "완료", "abc1234", "끝")
	t0 = time.Now()
	k.runE([]string{"HC_FAKE_REVSLOW=abc1234", "HERDR_TIMEOUT_GIT=1"}, "run")
	k.check("이 기계 감사의 rev-parse가 늦으면 상한에 끊고 감사를 미룸", time.Since(t0) < 3*time.Second && strings.Contains(k.out, "자동 감사 미룸 lr") &&
		k.eQ15("audit=lr") == 0, time.Since(t0).String(), k.out)
	k.run("run")
	k.check("다음 바퀴(늦지 않음)에 감사를 엶", k.eQ15("audit=lr") == 1, k.out)
	k.lj()
}

// E03(아스트라): 일반 add가 잠금 밖에서 지휘를 적는 사이 같은 이름이 --then으로 붙으면 대기열 줄로 또 넣지 않는다
func secKThenRace(k *K) {
	if k.impl == "bash" { // 셸 판도 두 곳에 들어간다 — 알려진 차이(셸 판은 곧 걷는다)
		return
	}
	k.fresh()
	k.add("first")
	hook := k.FILL + " add --name next --project app --brief " + k.brief("nextt") + " --then first"
	rc := k.fill([]string{"HC_TERMINAL_ID=tq1", "HC_FAKE_COORDSET_HOOK=" + hook}, "add", "--name", "next", "--project", "app", "--brief", k.brief("next"))
	fb := read(k.DATA + "/briefs/added/first.md")
	k.check("add와 add --then이 겹치면 한 곳에만(--then이 먼저 붙었으면 일반 add는 거절)", strings.Contains(fb, "## 이어서: next ") && rc != 0 && k.where("next") == "",
		fmt.Sprint(rc), k.where("next"), fb)
}

// F1·T1(Fable): 능력 막힘 레인이 둘이어도 need.tsv가 줄마다 — 막힘 알림은 레인마다 한 번, ls 비고는 둘 다
func secKNeedTwo(k *K) {
	rep := k.T + "/report.log"
	k.fresh()
	k.cpConf()
	k.gNb("na", "kube")
	k.gNb("nb", "kube")
	k.run("run")
	k.run("run")
	k.run("run")
	k.check("능력 막힘 둘: need.tsv 두 줄, 막힘 사건 레인마다 한 번", len(lines(read(k.DATA+"/need.tsv"))) == 2 &&
		countE(rep, `막힘 .*--name na`) == 1 && countE(rep, `막힘 .*--name nb`) == 1, read(k.DATA+"/need.tsv"), read(rep))
	k.run("ls")
	k.check("능력 막힘 둘: ls 비고가 둘 다", gLine(k.out, "na ", "필요 kube") && gLine(k.out, "nb ", "필요 kube"), k.out)
	k.cpConf()
}

// E04(아스트라): hc machine ls를 못 읽으면 목록에 없는 원격 레인을 「끝남」으로 단정하지 않는다
func secKLsUnknown(k *K) {
	if k.impl == "bash" { // 셸 판은 「끝남」 — 알려진 차이(셸 판은 곧 걷는다)
		return
	}
	k.fresh()
	k.add("lu", "--only", "box-1")
	k.run("run")
	k.lj() // 목록에서 빠짐
	k.set("HC_FAKE_MACHINES=")
	k.run("ls")
	k.mj("box-1", "box-2", "shared", "slow")
	k.check("기계 조회 실패 + 목록에 없음: 끝남이 아니라 모름", gLine(k.out, "lu ", "모름(연결 확인 못 함)") && !gLine(k.out, "lu ", "끝남"), k.out)
}

// F08(아스트라): 제한적인 umask(077)에서 대기열·지시문 사본이 누구나 읽는 파일이 되지 않는다(gofix-core의 atomic.WriteFile이 umask를 따름)
func secKUmask(k *K) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	k.fresh()
	k.add("um")
	k.run("prio", "um", "high")
	k.add("um2", "--then", "um")
	for _, f := range []string{k.Q, k.DATA + "/briefs/added/um.md"} {
		st, err := os.Stat(f)
		k.check("umask 077: "+f[strings.LastIndex(f, "/")+1:]+" 은 0600", err == nil && st.Mode().Perm() == 0o600, fmt.Sprint(err), fmt.Sprint(st))
	}
}

// F07(아스트라)·F4(Fable): 붙이기는 붙일 폴더(기본 ~/.claude-tasks) 안 파일만, 받은편지 줄에는 붙이지 않는다
func secKPaste(k *K) {
	if k.impl == "bash" { // 셸 판 paste_home은 HOME 아래 아무 파일 — 알려진 차이(셸 판은 곧 걷는다)
		return
	}
	runs := k.DATA + "/runs"
	k.fresh()
	write(k.t, k.HOME+"/.netrc", "machine example.invalid login me password hunter2\n")
	write(k.t, k.HOME+"/.claude-tasks/x/report.md", "보고 원문\n")
	write(k.t, k.T+"/kpa.md", "# 지시문 kpa\n참고: ~/.netrc 와 ~/.claude-tasks/x/report.md 를 본다\n")
	k.fill(nil, "add", "--name", "kpa", "--project", "app", "--brief", k.T+"/kpa.md", "--only", "box-1")
	k.run("run")
	rf := read(runs + "/kpa-run.md")
	k.check("붙이기: ~/.claude-tasks 파일은 붙이고 ~/.netrc는 안 붙임", k.where("kpa") == "up box-1 box-1/kpa" &&
		strings.Contains(rf, "보고 원문") && !strings.Contains(rf, "hunter2"), k.where("kpa"), rf)
	write(k.t, k.T+"/kpb.md", "# 지시문 kpb\n참고: ~/.claude-tasks/x/report.md\n")
	k.fill(nil, "add", "--name", "kpb", "--project", "app", "--brief", k.T+"/kpb.md", "--only", "box-1")
	k.hSetQ("kpb", map[int]string{11: "box-2:/tmp/x/done/kpb.md"}) // 받은편지 줄로(본문을 다른 기계가 씀)
	k.run("run")
	k.check("붙이기: 받은편지 줄에는 지휘 기계 파일을 붙이지 않음", k.state("kpb") == "up" && !hasF(runs+"/kpb-run.md", "(붙임)"), k.where("kpb"), read(runs+"/kpb-run.md"))
	rm(k.HOME+"/.netrc", k.HOME+"/.claude-tasks")
}

// hc 응답을 다 읽지 못했거나 실패로 끝났으면 그 출력으로 자리·시작을 정하지 않는다
func secKReadFail(k *K) {
	in := strings.Contains
	// F02(아스트라): 주소 없는 터미널이 섞인 목록 — 빈 주소를 local 자리로 세고 띄우지 않는다(셸 판 jq 오류처럼 멈춤)
	k.fresh()
	k.add("h2", "--only", "box-1")
	k.set(`HC_FAKE_LS={"terminals":[{"address":"t0","agent":"claude-code","name":"coord","state":"live","status":"working","cwd":"/"},` +
		`{"agent":"claude-code","name":"x","state":"live","status":"working","cwd":"/"}]}`)
	k.run("run")
	k.check("hc ls에 주소 없는 터미널: 종료 1, 띄우지 않음", k.rc == 1 && k.where("h2") == "wait - -", k.where("h2"), k.out)
	k.lj()
	// F03(아스트라): hc get이 멀쩡한 객체 뒤에 잘린 JSON을 붙이고 실패 종료 — 시작 확인으로 보지 않는다(상태 안 잡힘 → 다음 기계)
	k.fresh()
	k.runE([]string{"HC_FAKE_GET_BAD=box-1/h3"}, "add", "--name", "h3", "--project", "app", "--brief", k.brief("h3"))
	k.runE([]string{"HC_FAKE_GET_BAD=box-1/h3"}, "run")
	k.check("hc get 실패 종료·잘린 응답: 시작 확인으로 보지 않음(box-1 남기고 box-2)", k.where("h3") == "up box-2 box-2/h3" && in(k.bQline("h3"), "box-1=box-1/h3"),
		k.where("h3"), k.out)
	if k.impl == "bash" { // E05: 셸 판은 hc exec 실패의 「no」를 그대로 믿는다 — 알려진 차이(셸 판은 곧 걷는다)
		return
	}
	// E05(아스트라): 원격 작업 트리 검사가 「no」를 찍고 실패 종료 — 작업 트리 없음으로 안 세지 않는다(모름 → 셈)
	W := k.W
	mkdirs(k.t, W+"/app", W+"/app.wt/k1", W+"/app.wt/k2")
	k.fresh()
	k.lj("box-1/k1:claude-code:k1:idle:"+W+"/app.wt/k1", "box-1/k2:claude-code:k2:idle:"+W+"/app.wt/k2")
	k.add("h5")
	k.runE([]string{"HC_FAKE_EXEC_NO=app.wt/k"}, "run")
	k.check("작업 트리 검사가 실패 종료: 출력(no)을 믿지 않고 셈(box-1 꽉 참 → box-2)", k.where("h5") == "up box-2 box-2/h5" &&
		in(k.out, "자리 box-1 box-1/k1 k1: idle, 작업 트리 확인 못 함 → 셈"), k.where("h5"), k.out)
	k.lj()
}

// F01(아스트라)·F2(Fable): hc-lane 띄우기가 상한을 넘으면 첫 기계에서 이미 시작했을 수 있다 — 같은 바퀴에 다른 기계로 또 띄우지 않는다.
// 다음 바퀴에 첫 기계의 터미널이 보이면 그것을 채택한다(시도 기록이 첫 기계 그대로라)
func secKLaunchTimeout(k *K) {
	k.fresh()
	k.add("kl")
	starts := func() int { // 실제 착수(--print-steps 빼고)만 적히는 landprio.log
		n := 0
		for _, l := range lines(read(k.T + "/landprio.log")) {
			if strings.HasPrefix(l, "kl ") {
				n++
			}
		}
		return n
	}
	k.runE([]string{"HC_FAKE_LANE_SLOW=box-1/kl=3", "HERDR_TIMEOUT_LANE=1"}, "run")
	k.check("띄우기 시간 넘김: 같은 바퀴에 다른 기계로 또 띄우지 않음(착수 한 번)", starts() == 1 && !strings.HasPrefix(k.where("kl"), "up box-2"),
		k.where("kl"), read(k.T+"/landprio.log"), k.out)
	try := k.DATA + "/try.tsv"
	if k.impl != "bash" { // 셸 판은 상한이 없어 첫 바퀴에 그대로 뜬다 — 미확정 기록(?)은 Go 판만
		k.check("띄우기 시간 넘김: 시도 기록은 첫 기계 그대로·미확정(?)", hasE(try, "^kl\t[^\t]*\tbox-1\t[^\t]*\t[^\t]*\t\\?\t"), read(try))
		k.run("run") // 첫 기계는 연결됐는데 그 이름 터미널이 안 보임 → 안 떴다고 보고 다시 띄울 수 있다(같은 기계 차례부터)
		k.check("띄우기 시간 넘김 뒤 첫 기계에 터미널이 없으면 미확정을 풀고 다시 띄움", starts() == 2 && k.where("kl") == "up box-1 box-1/kl", k.where("kl"), k.out)
	}
	// 떠 있는 kl을 지우고 다시: 첫 기계에 터미널이 보이면 채택
	k.fresh()
	k.add("kl")
	k.runE([]string{"HC_FAKE_LANE_SLOW=box-1/kl=3", "HERDR_TIMEOUT_LANE=1"}, "run")
	k.lj("box-1/kl:claude-code:kl:working:/home/me/src/app.wt/kl")
	k.set(`HC_FAKE_LS_LANE={"terminals":[{"address":"box-1/kl"}]}`)
	k.run("run")
	k.unset("HC_FAKE_LS_LANE")
	k.check("띄우기 시간 넘김: 다음 바퀴에 첫 기계의 터미널을 채택(다시 띄우지 않음)", starts() == 1 && k.where("kl") == "up box-1 box-1/kl",
		k.where("kl"), k.out)
	if k.impl == "bash" { // 셸 판은 E02(띄운 직후 죽고 첫 기계가 끊김 → 다른 기계에 또)가 그대로 — 알려진 차이(셸 판은 곧 걷는다)
		return
	}
	// 첫 기계가 끊기면 다른 기계에 띄우지 않고 기다린다 — 시간 넘김(F01)과 띄운 직후 죽음(E02) 둘 다 미확정
	for _, env := range [][]string{{"HC_FAKE_LANE_SLOW=box-1/kl=3", "HERDR_TIMEOUT_LANE=1"}, {"HC_FILL_CRASH_AT=after-launch"}} {
		k.fresh()
		k.add("kl")
		k.runE(env, "run")
		k.mj("box-2", "shared", "slow")
		k.run("run")
		k.check("미확정 시도의 기계가 끊기면 다른 기계에 띄우지 않음 ("+env[0]+")", starts() == 1 && k.where("kl") == "wait - -" && grep(k.out, "미확정 시도 기다림 kl"),
			k.where("kl"), k.out)
		k.mj("box-1", "box-2", "shared", "slow")
	}
}

// F05(아스트라): 선행 판정의 fetch가 상한을 넘으면 기본 가지를 모른다 — 대기열에서 빠진 선행을 「착지 안 하고 끝남(stop)」으로 정하지 않는다
func secKFetchTimeout(k *K) {
	k.fresh()
	k.add("kp", "--only", "box-1")
	k.add("ke", "--after", "kp", "--only", "slow")
	k.run("run")
	k.check("fetch 시간 넘김 준비: kp는 box-1, ke는 slow에 먼저 띄움", k.where("kp") == "up box-1 box-1/kp" && k.where("ke") == "up slow slow/ke", k.where("kp"), k.where("ke"))
	delLines(k.t, k.Q, "^kp\t") // 선행이 착지해 대기열에서 빠짐
	write(k.t, k.T+"/commits", "aaaa1111 kp: 끝\n")
	k.lj("slow/ke:claude-code:ke")
	k.runE([]string{"HC_FAKE_FETCH_SLOW=3", "HERDR_TIMEOUT_REMOTE=1"}, "run")
	k.check("fetch 시간 넘김: 대기열에 없는 선행을 stop으로 정하지 않음(모름)", !strings.HasPrefix(k.gAfv("kp"), "stop"), k.gAfv("kp"), k.out)
	if k.impl != "bash" { // 셸 판은 fetch 실패(명시)도 stop — 알려진 차이(셸 판은 곧 걷는다)
		k.runE([]string{"HC_FAKE_FETCH_FAIL=1"}, "run")
		k.check("fetch 실패: 대기열에 없는 선행을 stop으로 정하지 않음(모름)", !strings.HasPrefix(k.gAfv("kp"), "stop"), k.gAfv("kp"), k.out)
	}
	k.run("run")
	k.check("fetch 시간 넘김 뒤 다음 바퀴(정상): 선행 들어감", strings.HasPrefix(k.gAfv("kp"), "go 들어감"), k.gAfv("kp"))
	write(k.t, k.T+"/commits", "")
	k.lj()
}

// F06(아스트라)·L8(Fable): 주인 지휘 확인(hc-lane coord --check)이 상한을 넘으면 주인이 사라졌는지 모른다 —
// 주인 없이(사건은 모든 지휘에게) 띄우지 않고 이번 바퀴는 미룬다. 다음 바퀴(정상)에 주인 그대로 띄운다
func secKCoordTimeout(k *K) {
	k.fresh()
	k.runE([]string{"HC_TERMINAL_ID=tq1"}, "add", "--name", "ko", "--project", "app", "--brief", k.brief("ko"))
	k.runE([]string{"HC_FAKE_COORD_SLOW=3", "HERDR_TIMEOUT_LANE=1"}, "run")
	k.check("주인 확인 시간 넘김: 주인 없이 띄우지 않음", !hasFx(k.T+"/owner.log", "없음") && k.qcol("ko", 12) == "tq1",
		read(k.T+"/owner.log"), k.where("ko"), k.out)
	k.run("run")
	k.check("주인 확인 시간 넘김 뒤 다음 바퀴(정상): 주인 tq1로 띄움", k.up("ko") && hasFx(k.T+"/owner.log", "tq1") && !hasFx(k.T+"/owner.log", "없음"),
		read(k.T+"/owner.log"), k.where("ko"))
}
