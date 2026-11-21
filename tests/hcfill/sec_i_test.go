package hcfill_test

// 셸 시험 10z(2026-11-21 hc-fill-go 추가: 받은편지 레인 감사·완료 뒤 멈춤·지휘 기계 파일 붙이기)와 11(잠금·사용법·newid).

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func init() { reg(90, "i", secI) }

func secI(k *K) {
	runs := k.DATA + "/runs"
	// 받은편지 레인 감사: 지시문이 「hc-fill 감사」를 요구하거나 설정 inbox_audit on이면 쓰기 레인처럼 감사·착지 절·감사 칸 auto
	k.ifresh()
	k.confAppend("audit    " + k.EXA)
	rm(k.T + "/events")
	k.mj("box-1", "shared", "slow")
	k.req(k.IS(), "inq-a", "app", "-", "slow")
	appendFile(k.t, k.IS()+"/inq-a.brief.md", "감사: hc-fill 감사(이 실행과 다른 Claude)\n")
	k.run("run")
	k.check("받은편지 줄이라도 지시문이 「hc-fill 감사」를 요구하면 감사·착지 절·감사 칸 auto",
		k.state("inq-a") == "up" && hasF(runs+"/inq-a-run.md", "감사·착지 (hc-fill 자동)") && k.col15("inq-a") == "auto")
	k.ifresh()
	k.confAppend("audit    "+k.EXA, "inbox_audit on")
	rm(k.T + "/events")
	k.mj("box-1", "shared", "slow")
	k.req(k.IS(), "inq-b", "app", "-", "slow")
	k.run("run")
	k.check("설정 inbox_audit on: 받은편지 줄에도 감사 칸 auto", k.col15("inq-b") == "auto")

	// 완료 뒤 멈춤(Go 판 — 셸 판에는 없다): 완료 뒤 감사도 착지·제출도 없이 설정 done_stall분이 지나면 주인 지휘에 막힘 한 번
	if k.impl != "bash" {
		rep := k.T + "/report.log"
		k.fresh()
		rm(k.T + "/events")
		k.cpConf()
		k.confAppend("done_stall 30")
		k.add("st1", "--only", "box-1")
		k.run("run")
		k.lj("box-1/st1:claude-code:st1:idle:/")
		write(k.t, k.T+"/events", "2026-11-19T01:00:00Z\tm\tbox-1/st1\tst1\t완료\tfeed123\t끝\t1\n")
		k.run("run")
		k.check("완료 뒤 멈춤: 감사·착지 없이 done_stall분 지나면 막힘 한 번(키 hf:<레인>:stall:<sha>)",
			len(fgrepLines(strings.Join(fgrepLines(read(rep), "완료 뒤 멈춤"), "\n"), "--key hf:st1:stall:feed123")) > 0, read(rep))
		appendFile(k.t, k.T+"/events", "2026-11-19T01:05:00Z\tm\tbox-1/st1\tst1\t착지\tfeed123\t착지\t1\n")
		rm(rep)
		k.run("run")
		k.check("완료 뒤 착지를 적었으면 멈춤 알림 없음", !hasF(rep, "완료 뒤 멈춤"), read(rep))
	}

	// 지휘 기계 파일 붙이기: 다른 기계에 띄울 때 지시문이 가리키는 이 기계 HOME 아래 파일 원문을 run 지시문 끝에
	k.fresh()
	write(k.t, k.HOME+"/note.md", "메모 원문\n")
	write(k.t, k.T+"/pst.md", "# 지시문 pst\n본문 `~/note.md` 를 읽는다\n")
	k.fill(nil, "add", "--name", "pst", "--project", "app", "--brief", k.T+"/pst.md", "--only", "box-1")
	k.run("run")
	k.check("다른 기계에 띄우면 지휘 기계 파일 원문을 붙임", k.where("pst") == "up box-1 box-1/pst" &&
		hasF(runs+"/pst-run.md", "## (붙임) 지휘 기계 파일 원문 — ~/note.md") && hasFx(runs+"/pst-run.md", "메모 원문"))
	write(k.t, k.T+"/psl.md", "# 지시문 psl\n본문 ~/note.md 참고\n")
	k.fill(nil, "add", "--name", "psl", "--project", "app", "--brief", k.T+"/psl.md", "--only", "here")
	k.run("run")
	k.check("이 기계에 띄우면 붙이지 않음", k.state("psl") == "up" && !hasF(runs+"/psl-run.md", "(붙임)"))
	write(k.t, k.HOME+"/sec.md", "token = abcdef0123456789abcdef\n")
	write(k.t, k.T+"/pss.md", "# 지시문 pss\n본문 ~/sec.md 참고\n")
	k.fill(nil, "add", "--name", "pss", "--project", "app", "--brief", k.T+"/pss.md", "--only", "box-1")
	k.run("run")
	k.check("비밀 값 꼴 파일이면 띄우지 않고 지휘에 한 번", k.where("pss") == "wait - -" &&
		strings.Contains(k.out, "원문을 붙이지 못해 띄우지 않음") && hasF(k.T+"/report.log", "--key hf:pss:paste:"))
	rm(k.HOME+"/note.md", k.HOME+"/sec.md")

	secILock(k)
}

// secILock: 11. 잠금 — watch는 한 번에 하나, run도 겹치지 않음. 사용법·newid.
func secILock(k *K) {
	k.fresh()
	self := k.T + "/hc-fill" // $T 안 링크로 띄워 명령 줄에 $T가 들어간다(셸 판 leftw가 그것으로 고른다)
	if err := os.Symlink(k.FILL, self); err != nil {
		k.t.Fatal(err)
	}
	wout, err := os.Create(k.T + "/watch.out")
	if err != nil {
		k.t.Fatal(err)
	}
	defer wout.Close()
	bg := exec.Command(self, "watch", "--every", "1")
	bg.Dir, bg.Env, bg.Stdout, bg.Stderr = k.T, k.environ(nil), wout, wout
	if err := bg.Start(); err != nil {
		k.t.Fatal(err)
	}
	k.bg = bg
	for i := 0; i < 50 && !exists(k.DATA+"/watch.lock"); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	k.out, k.rc = k.exe(k.T, nil, self, "watch", "--every", "1")
	k.check("watch: 이미 돌고 있으면 종료 75", k.rc == 75 && grep(k.out, `이미`))
	want := 1
	if k.impl == "bash" {
		want = 2
	}
	k.check("watch: 띄운 뒤 lockf 안쪽까지 둘 이상 돈다(남은 watch 검사가 제 구실을 하는지 — Go 판은 잠금을 쥔 한 프로세스)", len(k.leftw()) >= want)
	killtree(bg.Process.Pid)
	bg.Wait()
	k.bg = nil
	for i := 0; i < 30 && len(k.leftw()) > 0; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	var ps []string
	for _, p := range k.leftw() {
		ps = append(ps, strconv.Itoa(p))
	}
	k.out = ""
	if len(ps) > 0 {
		b, _ := exec.Command("ps", "-o", "pid,ppid,command", "-p", strings.Join(ps, ",")).CombinedOutput()
		k.out = string(b)
	}
	k.check("watch를 멈춘 뒤 남은 watch 0(lockf가 띄운 안쪽 루프가 고아로 남지 않음)", len(k.leftw()) == 0)
	k.run("ls")
	k.check("watch를 멈춘 뒤 ls 정상", k.rc == 0)
	k.run("nope")
	k.check("모르는 명령은 종료 2, 사용법은 README와 같은 말(그 레인에서 빼고)", k.rc == 2 && grep(k.out, `그 기계를 그 레인에서 빼고`))

	// SIGPIPE를 무시한 채 물려받은 셸에서도 newid가 끝난다 — 끝없는 /dev/urandom을 tr로 읽으면 head가 끝나도 tr이 안 죽어 run이 멈췄다
	// (제 프로세스 그룹에서 돌리고 10초가 넘으면 그룹째 멈춘다 — 남은 tr이 시험을 붙잡거나 고아로 남지 않게)
	common := k.FILL[:strings.LastIndex(k.FILL, "/")] + "/../lib/hc-fill/common.sh"
	k.out, k.rc = k.exe(k.T, nil, "perl", "-e", `$SIG{PIPE}="IGNORE"; setpgrp(0, 0); my $p = fork; if (!$p) { exec "bash", "-c", q{eval "$(grep -E "^(b36|newid)\(\)" "$1")"; newid a}, "-", $ARGV[0] }
  $SIG{ALRM} = sub { $SIG{TERM} = "IGNORE"; kill "TERM", -$$; exit 124 }; alarm 10; waitpid($p, 0); exit($? >> 8)`, common)
	k.check("newid: SIGPIPE를 무시해도 10초 안에 끝나고 a<시각>-<6글자>", k.rc == 0 && grep(k.out, `^a[0-9a-z]+-[0-9a-z]{6}$`))
}
