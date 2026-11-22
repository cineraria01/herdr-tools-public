package hcfill_test

// go-followups-2(2026-11-21): 바깥 명령 시간 상한 · 「완료 뒤 멈춤」 헛알림 빼기 · 큰 파일·바이너리 붙이기(gofix-fill F14).

import (
	"strconv"
	"strings"
	"syscall"
	"time"
)

func init() { reg(95, "j", secJ) }

func secJ(k *K) {
	secJTimeout(k)
	secJStall(k)
	secJPaste(k)
}

// 1. 가짜 git이 app 저장소의 remote 조회에서 멈춘다(2026-11-21 맥 바퀴 37분 멈춤 — 병합기 켬 표식 조회의 get-url): 상한(1초) 안에
// 무리째 거두고 「모름」 — 그 바퀴는 이어서 대기열 줄을 띄우고, 다음 바퀴(멈춤 없음)에 켬 표식을 고친다
func secJTimeout(k *K) {
	k.fresh()
	rm(k.T + "/hang.pid")
	k.cpConf()
	k.confAppend("merger   app   box-2")
	k.fill(nil, "add", "--name", "hg-app", "--project", "app", "--brief", k.brief("hg-app"), "--only", "here")
	k.fill(nil, "add", "--name", "hg-tools", "--project", "tools", "--brief", k.brief("hg-tools"), "--only", "here")
	t0 := time.Now()
	k.runE([]string{"HC_FAKE_GIT_HANG=/Users/me/src/app", "HERDR_TIMEOUT_GIT=1"}, "run")
	el := time.Since(t0)
	k.check("바깥 명령 상한: 멈춘 git을 상한 안에 거두고 「시간 넘김 <명령 앞부분> <초>」 한 줄", el < 25*time.Second &&
		strings.Contains(k.out, "시간 넘김 git -C /Users/me/src/app remote get-url 1"), el.String(), k.out)
	k.check("바깥 명령 상한: 그 조회는 모름(다음 바퀴에)이고 같은 바퀴의 대기열 줄은 띄움",
		strings.Contains(k.out, "병합기 app: 원격을 못 읽음 — 켬 표식은 다음 바퀴에") && k.up("hg-app") && k.up("hg-tools"),
		k.where("hg-app"), k.where("hg-tools"), k.out)
	left := []string{}
	for _, l := range lines(read(k.T + "/hang.pid")) {
		if p, err := strconv.Atoi(strings.TrimSpace(l)); err == nil && p > 1 && syscall.Kill(p, 0) == nil {
			left = append(left, l)
			syscall.Kill(p, syscall.SIGKILL)
		}
	}
	k.check("바깥 명령 상한: 멈춘 git의 무리(손자 sleep)까지 거둠", read(k.T+"/hang.pid") != "" && len(left) == 0, left...)
	k.run("run")
	k.check("바깥 명령 상한: 다음 바퀴(멈춤 없음)에 이어서 켬 표식을 고침", strings.Contains(k.out, "병합기 app: 켬 → box-2"), k.out)
	k.confDel("^merger")
	k.run("run")
}

// 2. 완료 뒤 멈춤 헛알림: 끝남(터미널 없음)·커밋 0·완료 sha가 기본 브랜치에 있음·모름이면 알리지 않고, 없을 때만 알린다
func secJStall(k *K) {
	rep := k.T + "/report.log"
	k.fresh()
	k.cpConf()
	k.confAppend("done_stall 30")
	k.add("sj1", "--only", "box-1")
	k.run("run")
	ev := func(msg string) {
		write(k.t, k.T+"/events", "2026-11-19T01:00:00Z\tm\tbox-1/sj1\tsj1\t완료\tbeef456\t"+msg+"\t1\n")
		rm(rep)
	}
	stalled := func() bool { return hasF(rep, "완료 뒤 멈춤") }
	ev("끝")
	k.runE([]string{"HC_FAKE_MB_RC=1"}, "run")
	k.check("헛알림: 터미널이 닫힌 레인(ls 「끝남」)은 완료 뒤 멈춤을 알리지 않음", !stalled(), read(rep))
	k.lj("box-1/sj1:claude-code:sj1:idle:/")
	ev("고칠 코드 없음(커밋 0)")
	k.runE([]string{"HC_FAKE_MB_RC=1"}, "run")
	k.check("헛알림: 커밋 없는 완료(「커밋 0」)는 알리지 않음", !stalled(), read(rep))
	ev("끝")
	k.runE([]string{"HC_FAKE_MB_RC=0"}, "run")
	k.check("헛알림: 완료 sha가 이미 기본 브랜치에 있으면 알리지 않음", !stalled(), read(rep))
	k.runE([]string{"HC_FAKE_MB_RC=128"}, "run")
	k.check("헛알림: 기본 브랜치에 있는지 모르면(merge-base 실패) 알리지 않음", !stalled(), read(rep))
	k.runE([]string{"HC_FAKE_MB_RC=1", "HC_FAKE_REVFAIL=beef456"}, "run")
	k.check("헛알림: 완료 sha 조회가 실패(I/O 오류 128 — 모름)면 「이 클론에 없음」으로 보지 않고 알리지 않음(아스트라 F12)", !stalled(), read(rep))
	k.runE([]string{"HC_FAKE_MB_RC=1"}, "run")
	k.check("헛알림: 터미널이 살아 있고 커밋이 기본 브랜치에 없을 때만 알림", stalled(), read(rep))
}

// 3. 지시문이 가리키는 큰 파일·바이너리는 붙이지 않고 경로만 — 레인은 띄운다
func secJPaste(k *K) {
	runs := k.DATA + "/runs"
	k.fresh()
	k.cpConf()
	write(k.t, k.HOME+"/.claude-tasks/big.txt", strings.Repeat("가나다라마바사\n", 4000)) // 64KiB 넘음
	write(k.t, k.HOME+"/.claude-tasks/bin.dat", "ELF\x00\x01\x02 바이너리\n")
	write(k.t, k.T+"/pbg.md", "# 지시문 pbg\n설명: ~/.claude-tasks/big.txt 와 ~/.claude-tasks/bin.dat 는 읽지 않아도 된다\n")
	k.fill(nil, "add", "--name", "pbg", "--project", "app", "--brief", k.T+"/pbg.md", "--only", "box-1")
	k.run("run")
	rf := runs + "/pbg-run.md"
	k.check("큰 파일·바이너리: 붙이지 않고 경로만 적고 레인은 띄움", k.where("pbg") == "up box-1 box-1/pbg" &&
		hasF(rf, "## (붙임) 지휘 기계 파일 — ~/.claude-tasks/big.txt\n(붙이지 않음: 큼/바이너리 — 경로만)") &&
		hasF(rf, "## (붙임) 지휘 기계 파일 — ~/.claude-tasks/bin.dat\n(붙이지 않음: 큼/바이너리 — 경로만)") &&
		!hasF(rf, "가나다라마바사") && !hasF(rf, "ELF"), k.where("pbg"), k.out)
	rm(k.HOME + "/.claude-tasks")
}
