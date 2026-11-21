package hclane_test

// tests/hc-lane.test.sh 21절 첫 턴 확인(1937–2006줄, launch-submit-check 2026-11-20): status가 잡힌 뒤 lane.first_turn_wait초 안에
// turn > 0 또는 status working을 못 보면 사건 막힘 「첫 턴 없음 <주소>」. 사건은 따로 둔 상태 폴더($T/submit-state)에.
// s21a1이 만든 t18 저장소와 지시문($BD/BRIEF.md)을 쓴다.

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

func init() { reg(101, "s21a2", s21aTurn) }

// 셸 시험의 가짜 시계(date·sleep)와 perl(alarm 초를 기록) — 바이트 그대로.
const s21aDate = `#!/bin/sh
if [ "$1" = +%s ]; then cat "$HCLOG.time"; else exec /bin/date "$@"; fi
`

const s21aSleep = `#!/bin/bash
if [ "$(cat "$HCLOG.time")" = 0 ]; then
  read -r -t 5 <>"$HCLOG.release" || touch "$HCLOG.late"
fi
echo $(( $(cat "$HCLOG.time") + $1 )) >"$HCLOG.time"
`

const s21aPerl = `#!/bin/sh
[ "${4-}" != hc ] || echo "$3" >>"$HCLOG.alarm"
exec /usr/bin/perl "$@"
`

// s21aFev: 셸 fev <이름> — 따로 둔 상태 폴더의 사건 가운데 그 레인의 막힘 줄들.
func s21aFev(sev, name string) []string {
	var o []string
	for _, l := range strings.Split(read(sev), "\n") {
		if strings.Contains(l, "\t"+name+"\t막힘\t") {
			o = append(o, l)
		}
	}
	return o
}

func s21aTurn(k *K) {
	R, T := k.R, k.T
	BD := k.s21aBD()
	SD := T + "/submit-state"
	SEV := SD + "/hc-lane/events.tsv"
	sk := func() { os.Remove(k.HCLOG + ".keys"); os.Remove(k.HCLOG + ".get") }
	gets9 := func() int { return s21aCountRe(read(k.HCLOG+".get"), `t9zz$`) }
	bl := func(extra []string, name string) {
		k.runE(extra, "--brief", BD+"/BRIEF.md", "--name", name, "--repo", R, "--", "claude", "--model", "m", "{brief} 지시문을 읽고 진행하라. 너는 worker다.")
	}
	fevn := func(name string) int { return len(s21aFev(SEV, name)) }
	start := func(s string) bool { return strings.Contains(lastLine(k.out), `"start":"`+s+`"`) }
	clockbin := T + "/turn-clock"
	mkdirs(k.t, clockbin)
	for name, s := range map[string]string{"date": s21aDate, "sleep": s21aSleep} {
		write(k.t, clockbin+"/"+name, s)
		os.Chmod(clockbin+"/"+name, 0o755)
	}
	cpath := "PATH=" + clockbin + ":" + k.env["PATH"]
	// hc-fill처럼 두 출력이 모두 닫혀 주소를 받은 뒤에만 다음 10초로 간다. 첫 idle 뒤의 조회는 끊긴 기계를 흉내 낸다.
	if err := syscall.Mkfifo(k.HCLOG+".release", 0o644); err != nil {
		k.t.Fatal(err)
	}
	fifo, err := os.OpenFile(k.HCLOG+".release", os.O_RDWR, 0) // 셸 exec 8<>
	if err != nil {
		k.t.Fatal(err)
	}
	k.conf("lane.start_wait 1", "lane.first_turn_wait 60")
	sk()
	write(k.t, k.HCLOG+".time", "0\n")
	cmd := exec.Command(k.LANE, "--name", "lane-ftq", "--repo", R, "--", "claude", "첫 지시") // out=$(… 2>&1)
	cmd.Dir = T
	cmd.Env = k.environ([]string{cpath, "HC_LANE_POLL=10", "HC_FAKE_NOSTATUS=idle", "HC_FAKE_GET9_FAIL=2-99", "HC_LANE_FILL=1", "XDG_STATE_HOME=" + SD})
	cmd.WaitDelay = 30 * time.Second
	b, err := cmd.CombinedOutput()
	k.out, k.rc = strings.TrimRight(string(b), "\n"), 0
	if err != nil {
		k.rc = -1
		if ee, ok := err.(*exec.ExitError); ok {
			k.rc = ee.ExitCode()
		}
	}
	// 사건이 보일 때까지 FIFO를 열어 둔다: 감시가 첫 sleep에 닿기 전에 닫으면 「go」가 버려져 5초 뒤 late가 된다
	fifo.WriteString("go\n")
	for i := 0; i < 100 && fevn("lane-ftq") == 0; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	fifo.Close()
	var js struct{ Lane, Start string }
	jok := json.Unmarshal([]byte(lastLine(k.out)), &js) == nil && js.Lane == "t9zz" && js.Start == "ok"
	k.check("대기열은 첫 idle에 주소를 받고, 연결이 끊겨도 감시는 따로 막힘을 적는다", k.rc == 0 && !exists(k.HCLOG+".late") && jok && fevn("lane-ftq") == 1)
	k.conf("lane.start_wait 1", "lane.first_turn_wait 1")
	sk()
	write(k.t, k.HCLOG+".time", "100\n")
	bl([]string{cpath, "HC_LANE_POLL=2", "XDG_STATE_HOME=" + SD, "HC_FAKE_NOSTATUS=idleuntil", "HC_FAKE_IDLE_GETS=2"}, "lane-ft-deadline")
	k.check("유예 1초·간격 2초면 1초만 기다리고 마감에 새 조회를 시작하지 않는다", k.rc == 0 && strings.TrimRight(read(k.HCLOG+".time"), "\n") == "101" &&
		gets9() == 1 && fevn("lane-ft-deadline") == 1)
	// 조회는 실제 남은 시간으로 제한한다. capt가 perl alarm에 넘기는 초를 기록한다.
	write(k.t, clockbin+"/perl", s21aPerl)
	os.Chmod(clockbin+"/perl", 0o755)
	k.conf("lane.start_wait 1", "lane.first_turn_wait 3")
	sk()
	write(k.t, k.HCLOG+".time", "100\n")
	bl([]string{cpath, "HC_LANE_POLL=2", "XDG_STATE_HOME=" + SD, "HC_FAKE_NOSTATUS=idleuntil", "HC_FAKE_IDLE_GETS=2"}, "lane-ft-alarm")
	k.check("3초 유예에서 2초 기다린 뒤에는 남은 1초만 조회에 준다", k.rc == 0 && strings.TrimRight(read(k.HCLOG+".alarm"), "\n") == "1" && gets9() == 2)
	k.conf("lane.start_wait 1", "lane.first_turn_wait 2")
	sk()
	bl([]string{"XDG_STATE_HOME=" + SD, "HC_FAKE_GET9_FAIL=1-1", "HC_FAKE_NOSTATUS=idle"}, "lane-ft-first-fail")
	k.check("첫 조회가 working을 출력하고 실패하면 다시 조회하고, 이후 idle만 보이면 막힘", k.rc == 0 && gets9() >= 2 && start("noturn") && fevn("lane-ft-first-fail") == 1)
	sk()
	bl([]string{"XDG_STATE_HOME=" + SD, "HC_FAKE_NOSTATUS=idleturn", "HC_FAKE_IDLE_GETS=3"}, "lane-ft1")
	k.check("turn > 0이면(working을 못 봤어도) 조용히 start ok — 사건·키 없음", k.rc == 0 && !exists(k.HCLOG+".keys") && gets9() >= 3 && start("ok") && fevn("lane-ft1") == 0)
	sk()
	bl([]string{"XDG_STATE_HOME=" + SD, "HC_FAKE_NOSTATUS=idleuntil", "HC_FAKE_IDLE_GETS=3"}, "lane-ft2")
	k.check("첫 턴 진행 중(working·turn 0)도 조용히 start ok", k.rc == 0 && gets9() >= 3 && start("ok") && fevn("lane-ft2") == 0)
	sk()
	bl([]string{"XDG_STATE_HOME=" + SD, "HC_LANE_OWNER=t1own", "HC_FAKE_NOSTATUS=idle"}, "lane-ft3")
	f3 := strings.Join(s21aFev(SEV, "lane-ft3"), "\n")
	k.check("마감까지 turn 0·idle이면 사건 막힘 하나 「첫 턴 없음 <주소>」(주인 지휘 --to), 키 없음, 종료 0 + 주소", k.rc == 0 && !exists(k.HCLOG+".keys") &&
		fevn("lane-ft3") == 1 && s21aCut(f3, 9, 9) == "t1own" && strings.Contains(f3, "첫 턴 없음 t9zz") &&
		strings.Contains(lastLine(k.out), `"lane":"t9zz"`) && start("noturn"))
	sk()
	bl([]string{"XDG_STATE_HOME=" + SD, "HC_FAKE_GET9_FAIL=2-4", "HC_FAKE_NOSTATUS=idleturn", "HC_FAKE_IDLE_GETS=6"}, "lane-ft4")
	k.check("조회 실패가 섞여도(실패한 조회의 출력은 안 믿음) 마감까지 계속 — 그 뒤 turn > 0이면 ok", k.rc == 0 && gets9() >= 6 && start("ok") && fevn("lane-ft4") == 0)
	mkdirs(k.t, T+"/event-fail/hc-lane/events.tsv") // 레인 기록은 성공하고 사건 추가만 실패한다
	sk()
	bl([]string{"XDG_STATE_HOME=" + T + "/event-fail", "HC_FAKE_NOSTATUS=idle"}, "lane-ftw")
	k.check("막힘 사건을 못 적어도 띄우기 결과는 그대로(종료 0 + 주소) — 오류 출력 한 줄만", k.rc == 0 && strings.Contains(lastLine(k.out), `"lane":"t9zz"`) &&
		strings.Count(k.errf(), "\n") == 1 && strings.Contains(k.errf(), "사건을 적지 못했다"))
	k.conf("lane.start_wait 1", "lane.first_turn_wait 08")
	sk()
	bl([]string{"XDG_STATE_HOME=" + SD, "HC_FAKE_NOSTATUS=idleturn", "HC_FAKE_IDLE_GETS=3"}, "lane-ft7")
	k.check("lane.first_turn_wait 08(앞 0)도 십진수로(셈 오류 없이) 종료 0", k.rc == 0 && start("ok") && !strings.Contains(k.errf(), "value too great"))
	k.conf("lane.start_wait 1", "lane.first_turn_wait 0")
	sk()
	bl([]string{"XDG_STATE_HOME=" + SD, "HC_FAKE_NOSTATUS=idle"}, "lane-ft8")
	k.check("lane.first_turn_wait 0: 첫 턴 확인 끔(status만 보고 ok)", k.rc == 0 && start("ok") && fevn("lane-ft8") == 0)
	k.conf("lane.first_turn_wait 하나")
	k.run("--name", "lane-ft9", "--repo", R, "--", "claude")
	k.check("lane.first_turn_wait가 숫자가 아니면 띄우기 전에 오류", k.rc != 0 && strings.Contains(k.errf(), "lane.first_turn_wait") && !exists(R+".wt/lane-ft9"))
	k.noconf()
}
