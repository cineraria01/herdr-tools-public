package hclane_test

// tests/hc-lane.test.sh 20절 뒤(1570–1802줄): 멈춤 유예(화면·설정), 배경 프로세스 판정, 끝남·정리됨, --mine, hc ls 실패,
// lane.notify, 지휘별 커서, print-steps·띄우기의 report 줄. s20a·s20b가 남긴 events.tsv·seen·HC_FAKE_*를 이어 쓴다.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func init() { reg(82, "s20c", s20Stop) }

// s20lineN: printf "%s\n" "$out" | grep -F <a> | grep -F <b> … 에 걸리는 줄 수.
func s20lineN(out string, subs ...string) int {
	n := 0
	for _, l := range strings.Split(out, "\n") {
		ok := true
		for _, s := range subs {
			ok = ok && strings.Contains(l, s)
		}
		if ok {
			n++
		}
	}
	return n
}

// s20now: date -u +%Y-%m-%dT%H:%M:%SZ.
func s20now() string { return time.Now().UTC().Format("2006-01-02T15:04:05Z") }

func s20Stop(k *K) {
	EV := k.s20ev()
	has := func(s string) bool { return strings.Contains(k.out, s) }
	wt0 := []string{"wait", "--timeout", "0"}

	// wait-grace(2026-11-16 실측: 배경 셸 둘이 도는 동안 턴을 끝낸 Claude 레인을 멈춤으로 알렸다):
	// done·idle은 화면 끝에 배경 작업 표시가 없고 그 status가 된 지 lane.wait_grace분(기본 10)이 지나야 멈춤. blocked는 바로
	GS := filepath.Join(k.T, "gscreen")
	scr := func(l ...string) { write(k.t, GS, strings.Join(l, "\n")+"\n") }
	lsG := func(a, s string, n int) string {
		return fmt.Sprintf(`HC_FAKE_LS={"terminals":[{"address":"%s","name":"lane-g","cwd":"/","state":"live","status":"%s","turn":%d}]}`, a, s, n)
	}
	// g: HC_FAKE_SCREEN=$GS [HC_FAKE_STATUS_AGE=<초>] HC_FAKE_LS=$(lsG …) [더…] run wait --timeout 0
	g := func(age, a, s string, n int, more ...string) {
		e := []string{"HC_FAKE_SCREEN=" + GS, lsG(a, s, n)}
		if age != "" {
			e = append(e, "HC_FAKE_STATUS_AGE="+age)
		}
		k.runE(append(e, more...), wt0...)
	}
	scr("● 검증을 배경으로 돌린다", "", "✻ Churned for 3m · done 12:33 PM · 2 shells still running", "────", "❯ ", "────", "  footer")
	k.s20rm(".get", ".read")
	g("1200", "tg1", "done", 2)
	k.check("유예: 화면 끝에 배경 셸 표시가 있으면 20분 쉬어도 멈춤 아님", k.rc == 75 && !has("lane-g\t멈춤") && s20fx(k.HCLOG+".read", "tg1"))
	scr("✻ Worked · done · 1 monitor still running", "❯ ")
	g("1200", "tg1", "done", 2)
	k.check("유예: monitor still running도 배경 작업", k.rc == 75)
	scr("✻ Churned for 3m · done 12:33 PM", "❯ ")
	k.s20rm(".read")
	g("300", "tg2", "idle", 2)
	k.check("유예: 표시가 없어도 쉰 지 10분 안이면 멈춤 아님(화면도 안 읽음)", k.rc == 75 && !has("lane-g\t멈춤") && !s20fx(k.HCLOG+".read", "tg2"))
	g("660", "tg2", "idle", 2)
	k.check("유예: 표시 없고 10분 넘게 쉬면 멈춤", k.rc == 0 && has("\ttg2\tlane-g\t멈춤\t-\tidle turn 2"))
	k.s20rm(".get")
	g("660", "tg2", "done", 2)
	k.check("유예: 이미 알린 쉼은 다시 안 알리고 hc get도 안 부름", k.rc == 75 && !s20fx(k.HCLOG+".get", "tg2"))
	scr("✻ done · 2 shells still running")
	k.s20rm(".get")
	g("5", "tg3", "blocked", 2)
	k.check("유예 없음: blocked는 배경 표시·10분 안이어도 바로 멈춤(hc get 안 부름)", k.rc == 0 && has("\ttg3\tlane-g\t멈춤\t-\tblocked turn 2") && !s20fx(k.HCLOG+".get", "tg3"))
	// 감사 반려 [중간] 1: 배경 표시는 마지막 턴 끝 줄(✻ 줄)에서만 보고, 쉼이 lane.wait_busy_max분(기본 30)을 넘으면 표시와 상관없이 멈춤
	var tail14 []string
	for i := 1; i <= 14; i++ {
		tail14 = append(tail14, fmt.Sprintf("  꼬리 %d", i))
	}
	scr(append([]string{"✻ Crunched for 13m · done 12:33 PM · 2 shells still running", "⏺ 배경 검증 통과. 끝.", "✻ Cooked for 9s · done 12:48 PM", "────", "❯ "}, tail14...)...)
	g("660", "th1", "done", 3)
	k.check("유예: 옛 턴 끝 줄의 배경 표시는 무시 — 마지막 ✻ 줄에 없으면 10분 뒤 멈춤", k.rc == 0 && has("\tth1\tlane-g\t멈춤\t-\tdone turn 3"))
	scr(append([]string{"⏺ 검증을 배경으로", "✻ Churned for 3m · done 12:33 PM · 2 shells still running", "────", "❯ "}, tail14...)...)
	g("1500", "th2", "done", 3)
	k.check("유예: 마지막 ✻ 줄에 배경 표시면 30분 안은 멈춤 아님", k.rc == 75)
	g("1860", "th2", "done", 3)
	k.check("유예 상한: 배경 표시가 있어도 30분 넘게 쉬면 멈춤", k.rc == 0 && has("\tth2\tlane-g\t멈춤\t-\tdone turn 3"))
	// 감사 [낮음] 3: hc read 실패는 배경 작업 없음으로(멈춤), hc get 실패·status_ms 없음은 유예 없이 멈춤
	g("700", "th3", "done", 3, "HC_FAKE_READ_FAIL=1")
	k.check("실패: hc read가 실패하면 배경 작업으로 숨기지 않고 멈춤", k.rc == 0 && has("\tth3\tlane-g\t멈춤"))
	g("", "th4", "done", 3)
	k.check("실패: hc get이 실패하면(status_ms 모름) 배경 표시가 있어도 바로 멈춤", k.rc == 0 && has("\tth4\tlane-g\t멈춤"))
	// 감사 [낮음] 2: 같은 turn에서 done→idle로 바뀌어 status_ms가 새로 찍혀도 처음 본 쉼 시각부터 센다
	scr("✻ Churned · done · 1 shell still running", "❯ ")
	g("650", "th5", "done", 3)
	k.check("쉼 시각: 배경 셸 표시로 붙든 done(11분)은 멈춤 아님", k.rc == 75)
	scr("✻ Churned · done", "❯ ")
	g("5", "th5", "idle", 3)
	k.check("쉼 시각: 같은 turn에서 idle로 바뀌어 status_ms가 새로 찍혀도 처음 쉼부터 세어 멈춤", k.rc == 0 && has("\tth5\tlane-g\t멈춤\t-\tidle turn 3"))
	k.conf("lane.wait_grace 0", "lane.wait_busy")
	g("5", "tg4", "done", 2)
	k.check("설정: lane.wait_grace 0·lane.wait_busy 빈 값이면 유예 끔(바로 멈춤)", k.rc == 0 && has("\ttg4\tlane-g\t멈춤"))
	k.conf("lane.wait_grace 1", "lane.wait_busy compiling")
	scr("✻ done · compiling")
	g("120", "tg5", "done", 2)
	k.check("설정: 표시 글자 목록과 분을 바꿀 수 있음", k.rc == 75)
	k.conf("lane.wait_grace 열")
	k.runE([]string{lsG("tg6", "done", 2)}, wt0...)
	k.check("설정: lane.wait_grace가 숫자가 아니면 오류", k.rc != 0 && k.rc != 75 && strings.Contains(k.errf(), "lane.wait_grace"))
	k.noconf()

	// quiet-alerts 1: 배경 작업은 화면이 아니라 프로세스로 — 터미널 pid 아래 Claude Bash 도구 셸(명령 줄에 shell-snapshots)이 살아 있으면
	// 배경 작업 중. MCP 서버처럼 늘 사는 자손·남의 셸은 안 셈. ps가 실패하면 화면 판정, 30분 상한은 그대로
	PSF := filepath.Join(k.T, "fakeps")
	snap := `/bin/zsh -c source /Users/u/.claude/shell-snapshots/snapshot-zsh-1.sh >/dev/null 2>&1 && eval 'bash verify.sh'`
	ps := func(l ...string) { write(k.t, PSF, strings.Join(l, "\n")+"\n") }
	pe := []string{"HC_FAKE_PID=4000", "HC_FAKE_PS=" + PSF}
	ps(" 4000     1 node agent-runner.js", " 4001  4000 claude --model x", " 4002  4001 "+snap, " 4003  4001 node /x/mcp-server.js", " 5001     1 "+snap)
	scr("✻ Churned for 3m · done 12:33 PM", "❯ ") // 화면에는 배경 표시가 없다(화면 판정이면 멈춤)
	k.s20rm(".ps")
	g("660", "tp1", "done", 2, pe...)
	k.check("프로세스: 터미널 pid 자손에 shell-snapshots 셸이 살아 있으면 화면 표시가 없어도 멈춤 아님", k.rc == 75 && strings.Contains(read(k.HCLOG+".ps"), "-A -o pid=,ppid=,command="))
	ps(" 4000     1 node agent-runner.js", " 4001  4000 claude --model x", " 4003  4001 node /x/mcp-server.js", " 5001     1 "+snap, " 5002  4100 "+snap)
	scr("✻ Churned for 3m · done 12:33 PM · 2 shells still running", "❯ ") // 화면은 배경 표시(옛 글자)
	g("660", "tp2", "done", 2, pe...)
	k.check("프로세스 없음이어도 화면에 배경 표시가 있으면 멈춤 아님(셸 없는 배경 작업 — 감사 [낮음] 4)", k.rc == 75)
	scr("✻ Churned for 3m · done 12:33 PM", "❯ ")
	g("660", "tp2b", "done", 2, pe...)
	k.check("프로세스: 자손이 MCP 서버 꼴뿐이고(남의 셸은 안 셈) 화면 표시도 없으면 유예 뒤 멈춤", k.rc == 0 && has("\ttp2b\tlane-g\t멈춤\t-\tdone turn 2"))
	scr("✻ Churned for 3m · done 12:33 PM · 2 shells still running", "❯ ")
	g("300", "tp3", "done", 2, pe...)
	k.check("프로세스: 유예(10분) 안이면 자손이 없어도 멈춤 아님", k.rc == 75)
	g("660", "tp4", "done", 2, "HC_FAKE_PS_FAIL=1", "HC_FAKE_PID=4000")
	k.check("프로세스: ps가 실패하면 화면 판정(배경 표시 → 멈춤 아님)", k.rc == 75)
	ps(" 4000     1 node agent-runner.js", " 4002  4000 "+snap)
	g("1860", "tp5", "done", 2, pe...)
	k.check("프로세스 상한: 배경 셸이 살아 있어도 30분 넘게 쉬면 멈춤", k.rc == 0 && has("\ttp5\tlane-g\t멈춤"))
	k.s20rm(".exec", ".ps")
	k.runE(append([]string{`HC_FAKE_MACHINES={"machines":[{"name":"m1","state":"connected"}]}`, "HC_FAKE_SCREEN=" + GS, "HC_FAKE_STATUS_AGE=660",
		`HC_FAKE_LS={"terminals":[{"address":"m1/tq1","name":"lane-q1","cwd":"/","state":"live","status":"done","turn":2},{"address":"m1/tq2","name":"lane-q2","cwd":"/","state":"live","status":"idle","turn":2}]}`}, pe...),
		"wait", "--all", "--timeout", "0")
	nps := 0
	for _, l := range strings.Split(read(k.HCLOG+".exec"), "\n") {
		if l == "ps" {
			nps++
		}
	}
	k.check("프로세스 원격: 기계당 hc exec --on 한 번(ps), 두 레인 다 배경 작업 중", k.rc == 75 && len(s20lines(k.HCLOG+".ps")) == 1 && nps == 1)
	k.conf("lane.wait_procs")
	scr("✻ Churned for 3m · done 12:33 PM", "❯ ")
	g("660", "tp6", "done", 2, pe...)
	k.check("설정: lane.wait_procs 빈 값이면 프로세스 판정 끔(화면만)", k.rc == 0 && has("\ttp6\tlane-g\t멈춤"))
	k.noconf()

	// quiet-alerts 2·3(감사 반려 [높음] 1·2): 조용히 넘기는 것은 마지막 report가 착지·판정일 때의 끝남·정리됨뿐.
	// 그 밖(완료·막힘·중간·report 없음)의 끝남은 「뜻밖의 종료(<상태>)」, 정리됨(hc-sweep이 닫고 hc rm — 목록에서 사라짐)은 그 사건 자체를
	// 「뜻밖의 종료(정리됨)」 멈춤으로 알린다(wait가 안 돌던 틈에 닫혀도 다음 wait에서 나오게)
	appendf(k.t, EV, ev("2026-11-16T03:00:00Z", "here", "tf1", "lane-f1", "완료", "abcd1234", "끝")+
		ev("2026-11-16T03:00:01Z", "here", "tf3", "lane-f3", "중간", "-", "반")+
		ev("2026-11-16T03:00:02Z", "here", "tf3", "lane-f3", "정리됨", "-", "hc-sweep R2")+
		ev("2026-11-16T03:00:03Z", "here", "tf4", "lane-f4", "판정", "-", "감사", "3")+
		ev("2026-11-16T03:00:04Z", "here", "tf4", "lane-f4", "정리됨", "-", "hc-sweep R2", "4")+
		ev("2026-11-16T03:00:05Z", "here", "tf7", "lane-f7", "막힘", "-", "질문")+
		ev("2026-11-16T03:00:06Z", "here", "tf8", "lane-f8", "착지", "abcd1234", "끝", "2")+
		ev("2026-11-16T03:00:07Z", "here", "tf10", "lane-f10", "판정", "-", "turn", "모름")+
		ev("2026-11-16T03:00:08Z", "here", "tf11", "lane-f11", "착지", "abcd1234", "turn", "모름")+
		ev("2026-11-16T03:00:09Z", "here", "tf11", "lane-f11", "정리됨", "-", "hc-sweep R2")+
		ev("2026-11-16T03:00:10Z", "here", "tf12", "lane-f12", "제출", "abcd1234", "sub → 큐", "2")+
		ev("2026-11-16T03:00:11Z", "here", "tf12", "lane-f12", "정리됨", "-", "hc-sweep R2", "6"))
	k.runE([]string{`HC_FAKE_LS={"terminals":[{"address":"tf1","name":"lane-f1","state":"finished"},{"address":"tf2","name":"lane-f2","state":"finished"},{"address":"tf3","name":"lane-f3","state":"finished"},{"address":"tf4","name":"lane-f4","state":"finished"},{"address":"tf7","name":"lane-f7","state":"finished"},{"address":"tf8","name":"lane-f8","state":"finished","turn":3},{"address":"tf10","name":"lane-f10","state":"finished","turn":3}]}`}, wt0...)
	k.check("끝남·정리됨: report turn을 모르면 착지·판정 뒤라도 알림(감사 3차 [낮음] 3)", has("\ttf10\tlane-f10\t멈춤\t-\t뜻밖의 종료(finished)") && has("\ttf11\tlane-f11\t멈춤\t-\t뜻밖의 종료(정리됨)"))
	k.check("끝남: report 없이 끝나면 「뜻밖의 종료」", k.rc == 0 && has("\ttf2\tlane-f2\t멈춤\t-\t뜻밖의 종료(finished)"))
	k.check("끝남: 완료(착지 전) 뒤 끝남은 알림(옛 판 회귀 — 감사 [높음] 1)", has("\ttf1\tlane-f1\t멈춤\t-\t뜻밖의 종료(finished)"))
	k.check("끝남: 막힘 뒤 끝남도 알림", has("\ttf7\tlane-f7\t멈춤\t-\t뜻밖의 종료(finished)"))
	k.check("끝남: 착지·판정 뒤 끝남은 안 알림", !grep(k.out, `lane-f[48]`+"\t멈춤\t-\t"+`뜻밖의 종료\(finished`))
	k.check("정리됨: 중간 뒤 정리됨은 「뜻밖의 종료(정리됨)」, 그 끝남은 두 번 안 알림", has("\ttf3\tlane-f3\t멈춤\t-\t뜻밖의 종료(정리됨) hc-sweep R2") && !has("lane-f3\t멈춤\t-\t뜻밖의 종료(finished)"))
	// go-alert-quiet(2026-11-22): 제출 뒤 정리됨은 turn과 상관없이 안 알림
	k.check("정리됨: 제출 뒤(그 턴 지나) 정리됨은 안 알림(go-alert-quiet)", !has("lane-f12\t멈춤"))
	k.check("정리됨: 판정 뒤 정리됨은 안 알림, 정리됨 줄 그대로는 안 찍음", !has("lane-f4\t멈춤") && !has("\t정리됨\t"))
	k.run("events")
	k.check("정리됨: hc-lane events에는 보임", has("lane-f3\t정리됨"))
	k.runE([]string{`HC_FAKE_LS={"terminals":[{"address":"tf5","name":"lane-f5","cwd":"/","state":"live","status":"done","turn":3}]}`}, wt0...)
	// 감사 [높음] 2 재현 꼴: wait가 안 도는 사이에 report 없이 쉬던 레인을 hc-sweep이 닫고 지움(hc ls에 없음) → 다음 wait --state 멈춤에서 나옴
	empty := `HC_FAKE_LS={"terminals":[]}`
	appendf(k.t, EV, ev(s20now(), "here", "tf9", "lane-f9", "중간", "-", "반"))
	appendf(k.t, EV, ev(s20now(), "here", "tf9", "lane-f9", "정리됨", "-", "hc-sweep R2"))
	k.runE([]string{empty}, "wait", "--state", "멈춤", "--timeout", "0")
	k.check("정리됨: 틈에 닫혀 목록에서 사라진 보고 없는 레인도 다음 wait에서 멈춤", k.rc == 0 && has("\ttf9\tlane-f9\t멈춤\t-\t뜻밖의 종료(정리됨)"))
	k.runE([]string{empty}, "wait", "--state", "멈춤", "--timeout", "0")
	k.check("정리됨: 같은 정리됨은 두 번 안 알림", k.rc == 75)
	appendf(k.t, EV, ev(s20now(), "here", "tf6", "lane-f6", "완료", "abcd1234", "끝"))
	appendf(k.t, EV, ev(s20now(), "here", "tf6", "lane-f6", "정리됨", "-", "hc-sweep R2"))
	k.run(wt0...)
	k.runE([]string{`HC_FAKE_LS={"terminals":[{"address":"tf6","name":"lane-f6","cwd":"/","state":"live","status":"done","turn":1}]}`}, wt0...)
	k.check("정리됨: 쉬는 레인은 정리됨 앞의 마지막 report(완료)로 판정(닫기 실패해도 거짓 멈춤 없음)", k.rc == 75)
	// 재감사 [높음] 1: 착지·판정 뒤 이어 받은 일(turn이 오름)에서 끝나거나 정리되면 알린다 — 쉼 판정과 같은 turn 규칙(report turn+1 이하만 조용)
	nw := s20now()
	appendf(k.t, EV, ev(nw, "here", "ta1", "lane-ta1", "판정", "-", "감사", "3")+ev(nw, "here", "ta2", "lane-ta2", "판정", "-", "감사", "3")+
		ev(nw, "here", "tb1", "lane-tb1", "착지", "abcd1234", "끝", "2")+ev(nw, "here", "tb1", "lane-tb1", "정리됨", "-", "hc-sweep R2", "7")+
		ev(nw, "here", "tb2", "lane-tb2", "판정", "-", "감사", "3")+ev(nw, "here", "tb2", "lane-tb2", "정리됨", "-", "hc-sweep R2", "4"))
	k.runE([]string{empty}, "wait", "--state", "판정,착지", "--timeout", "0") // report 줄은 먼저 소비
	k.runE([]string{`HC_FAKE_LS={"terminals":[{"address":"ta1","name":"lane-ta1","state":"finished","turn":6},{"address":"ta2","name":"lane-ta2","state":"finished","turn":4}]}`},
		"wait", "--state", "멈춤", "--timeout", "0")
	k.check("turn: 판정 뒤 turn이 오른(이어 받은 일) 끝남은 알림", k.rc == 0 && has("\tta1\tlane-ta1\t멈춤\t-\t뜻밖의 종료(finished)"))
	k.check("turn: 착지 뒤 turn이 오른 정리됨은 알림", has("\ttb1\tlane-tb1\t멈춤\t-\t뜻밖의 종료(정리됨)"))
	k.check("turn: 판정 그 턴(turn+1)의 끝남·정리됨은 조용", !grep(k.out, `lane-t[ab]2`+"\t멈춤"))
	// 재감사 [낮음] 2: 쉼(done turn n)으로 이미 알린 레인을 hc-sweep이 같은 turn에 닫으면 다시 알리지 않음
	k.runE([]string{`HC_FAKE_LS={"terminals":[{"address":"tc1","name":"lane-tc1","cwd":"/","state":"live","status":"done","turn":5}]}`}, "wait", "--state", "멈춤", "--timeout", "0")
	k.check("쉼 알림 먼저(준비)", k.rc == 0 && has("lane-tc1\t멈춤\t-\tdone turn 5"))
	appendf(k.t, EV, ev(s20now(), "here", "tc1", "lane-tc1", "정리됨", "-", "hc-sweep R2", "5"))
	k.runE([]string{empty}, "wait", "--state", "멈춤", "--timeout", "0")
	k.check("정리됨: 같은 turn에 쉼으로 이미 알린 레인은 다시 안 알림", k.rc == 75)
	// wait-landed(2026-11-16 실측): 착지 뒤 같은 sha의 「완료」를 그 턴에 한 번 더 report하고 끝나면 착지 뒤 종료다.
	// 다른 sha의 완료·중간이 뒤에 오거나, 착지와 완료 사이·그 뒤에 turn이 오르면(이어 받은 일), sha가 -면(판정 포함) 여전히 알린다
	nw = s20now()
	e8 := func(f ...string) string { return ev(append([]string{nw, "here"}, f...)...) }
	appendf(k.t, EV, e8("td1", "lane-td1", "착지", "6d295b6f", "착지", "2")+e8("td1", "lane-td1", "완료", "6d295b6f", "완료", "2")+
		e8("td2", "lane-td2", "착지", "6d295b6f", "착지", "2")+e8("td2", "lane-td2", "완료", "abcd1234", "새일", "2")+
		e8("td3", "lane-td3", "착지", "6d295b6f", "착지", "2")+e8("td3", "lane-td3", "완료", "6d295b6f", "완료", "4")+
		e8("td4", "lane-td4", "착지", "6d295b6f", "착지", "2")+e8("td4", "lane-td4", "완료", "6d295b6f", "완료", "2")+e8("td4", "lane-td4", "중간", "-", "반", "2")+
		e8("td5", "lane-td5", "판정", "-", "감사", "2")+e8("td5", "lane-td5", "완료", "-", "끝", "2")+
		e8("td6", "lane-td6", "착지", "-", "착지", "2")+e8("td6", "lane-td6", "완료", "-", "새일", "2")+
		e8("td7", "lane-td7", "착지", "6d295b6f", "착지", "2")+e8("td7", "lane-td7", "완료", "6d295b6f", "완료", "5")+
		e8("td8", "lane-td8", "착지", "6d295b6", "착지", "2")+e8("td8", "lane-td8", "완료", "6d295b6f00112233445566778899"+"aabbccddeeff", "완료", "2")+
		e8("td9", "lane-td9", "착지", "6d295b6f", "착지", "2")+e8("td9", "lane-td9", "완료", "6d295b6f", "완료", "3")+
		e8("te1", "lane-te1", "착지", "6d295b6f", "착지", "2")+e8("te1", "lane-te1", "완료", "6d295b6f", "완료", "2")+e8("te1", "lane-te1", "정리됨", "-", "hc-sweep R2", "3")+
		e8("te2", "lane-te2", "착지", "6d295b6f", "착지", "2")+e8("te2", "lane-te2", "완료", "abcd1234", "새일", "2")+e8("te2", "lane-te2", "정리됨", "-", "hc-sweep R2", "3"))
	k.runE([]string{empty}, "wait", "--state", "착지,판정,완료,중간", "--timeout", "0") // report 줄은 먼저 소비
	k.runE([]string{`HC_FAKE_LS={"terminals":[{"address":"td1","name":"lane-td1","state":"finished","turn":3},{"address":"td2","name":"lane-td2","state":"finished","turn":3},{"address":"td3","name":"lane-td3","state":"finished","turn":7},{"address":"td4","name":"lane-td4","state":"finished","turn":3},{"address":"td5","name":"lane-td5","state":"finished","turn":3},{"address":"td6","name":"lane-td6","state":"finished","turn":3},{"address":"td7","name":"lane-td7","state":"finished","turn":6},{"address":"td8","name":"lane-td8","state":"finished","turn":3},{"address":"td9","name":"lane-td9","state":"finished","turn":4}]}`},
		"wait", "--state", "멈춤", "--timeout", "0")
	k.check("착지 뒤 같은 sha 완료: 그 턴에 끝남·정리됨은 조용(실측 꼴, 짧은 쪽 길이 앞자리 비교)", k.rc == 0 && !grep(k.out, `lane-t(d1|d8|e1)`+"\t멈춤"))
	k.check("sha가 -면 같은 sha로 치지 않음: 판정·착지 - 뒤 완료 - 의 끝남은 알림(감사 wait-landed [높음] 1)", has("\ttd5\tlane-td5\t멈춤\t-\t뜻밖의 종료(finished)") && has("\ttd6\tlane-td6\t멈춤\t-\t뜻밖의 종료(finished)"))
	k.check("착지와 같은 sha 완료 사이에 turn이 오르면(이어 받은 일) 끝남은 알림(감사 wait-landed [높음] 1)", has("\ttd7\tlane-td7\t멈춤\t-\t뜻밖의 종료(finished)"))
	k.check("착지 다음 턴(turn+1)의 같은 sha 완료도 새 일: 끝남은 알림(재감사 [낮음] 2)", has("\ttd9\tlane-td9\t멈춤\t-\t뜻밖의 종료(finished)"))
	k.check("착지 뒤 다른 sha 완료 뒤 끝남·정리됨은 알림", has("\ttd2\tlane-td2\t멈춤\t-\t뜻밖의 종료(finished)") && has("\tte2\tlane-te2\t멈춤\t-\t뜻밖의 종료(정리됨)"))
	k.check("착지 뒤 같은 sha 완료여도 turn이 오른 뒤 끝남은 알림", has("\ttd3\tlane-td3\t멈춤\t-\t뜻밖의 종료(finished)"))
	k.check("착지·완료 뒤 중간이 오면 끝남은 알림", has("\ttd4\tlane-td4\t멈춤\t-\t뜻밖의 종료(finished)"))
	// 지휘 요청(감사 3차 [높음] 1로 기본 꺼짐): wait는 기본으로 모든 레인 사건, --mine이면 레인의 hc parent가 이 지휘($HC_TERMINAL_ID —
	// 원격 레인은 <기계>/<t…>로 본다)인 사건만. parent를 모르면(hc ls에 없음) 보여 준다(fail-open).
	// 기본이 거르면 지휘가 바뀌거나(옛 parent는 죽은 터미널) wait를 다른 터미널에서 부를 때 사건이 다 가려진다
	nw = s20now()
	appendf(k.t, EV, ev(nw, "here", "tw1", "lane-own", "완료", "abcd1234", "내것", "1")+ev(nw, "m1", "m1/tw2", "lane-own2", "완료", "abcd1234", "내것원격", "1")+
		ev(nw, "here", "tw3", "lane-other", "완료", "abcd1234", "남의것", "1")+ev(nw, "here", "tw4", "lane-gone", "완료", "abcd1234", "모름", "1"))
	lsP := `HC_FAKE_LS={"terminals":[{"address":"tw1","name":"lane-own","parent":"tW","cwd":"/","state":"live","status":"working","turn":1},{"address":"m1/tw2","name":"lane-own2","parent":"mac/tW","cwd":"/","state":"live","status":"working","turn":1},{"address":"tw3","name":"lane-other","parent":"mac/tX","cwd":"/","state":"live","status":"working","turn":1}]}`
	k.runE([]string{"HC_TERMINAL_ID=tNEW", lsP}, "wait", "--state", "완료", "--timeout", "0")
	k.check("parent 기본: 거르지 않음 — 다른(옛) 지휘가 parent인 레인 사건도 보임", k.rc == 0 && has("lane-own\t완료") && has("lane-other\t완료"))
	k.runE([]string{"HC_TERMINAL_ID=tW", lsP}, "wait", "--mine", "--state", "완료", "--timeout", "0")
	k.check("parent --mine: 내 레인(로컬·원격 꼴)과 parent 모르는 레인만, 남의 지휘 레인은 거름", k.rc == 0 && has("lane-own\t완료") && has("lane-own2\t완료") && has("lane-gone\t완료") && !has("lane-other"))
	k.run("wait", "--everyone", "--timeout", "0")
	k.check("--everyone은 없음(기본이 전부)", k.rc == 1 && strings.Contains(k.errf(), "--everyone"))
	// 감사 [낮음] 5: hc ls가 바퀴마다 실패하면 3바퀴째에 멈춤꼴 사건 한 줄로 끝낸다(배경 wait의 stderr는 끝나기 전엔 안 보인다)
	k.runE([]string{"HC_LANE_WAIT_TICK=0", "HC_FAKE_LS_FAIL=1"}, "wait", "--state", "멈춤", "--timeout", "1")
	k.check("hc ls 실패 3바퀴: 사건 한 줄로 알리고 종료 0", k.rc == 0 && has("멈춤\t-\thc ls 실패 3바퀴"))

	s20Notify(k)
}

// s20rm: rm -f "$HCLOG<접미사>" …
func (k *K) s20rm(sfx ...string) {
	for _, s := range sfx {
		os.Remove(k.HCLOG + s)
	}
}

func s20Notify(k *K) {
	EV := k.s20ev()
	has := func(s string) bool { return strings.Contains(k.out, s) }
	// quiet-alerts 2: lane.notify no면 hc-lane이 띄우는 레인·감사(--read)에 hc run --no-notify
	k.s20rm("")
	k.run("--name", "lane-n1", "--repo", k.R, "--", "true")
	k.check("notify 기본(yes): --no-notify 없음", k.rc == 0 && k.has("run") && !k.has("--no-notify"))
	k.conf("lane.notify no")
	k.s20rm("")
	k.run("--name", "lane-n2", "--repo", k.R, "--", "true")
	k.check("notify no: 레인 hc run에 --no-notify", k.rc == 0 && k.has("run") && k.has("--no-notify"))
	k.s20rm("")
	k.run("--read", "--name", "lane-n3", "--repo", k.R, "--", "true")
	k.check("notify no: --read(감사)에도 --no-notify", k.rc == 0 && k.has("--no-notify"))
	k.run("--dry-run", "--name", "lane-n4", "--repo", k.R, "--", "true")
	k.check("notify no: --dry-run 줄에도 --no-notify", k.rc == 0 && s20lineN(k.out, "hc run --bg", "--no-notify") > 0)
	k.conf("lane.notify maybe")
	k.run("--name", "lane-n5", "--repo", k.R, "--", "true")
	k.check("notify: yes·no가 아니면 띄우기 전에 오류", k.rc != 0 && strings.Contains(k.errf(), "lane.notify") && !exists(k.R+".wt/lane-n5"))
	k.noconf()

	// 감사 반영 3·6: 커서는 지휘(HC_TERMINAL_ID)마다 따로, 새 커서는 2시간 전부터
	k.set(`HC_FAKE_LS={"terminals":[]}`)
	appendf(k.t, EV, ev("2000-01-01T00:00:00Z", "here", "t1", "old-ev", "완료", "-", "옛날"))
	k.runE([]string{"HC_LANE=lane-k"}, "report", "완료", "abcdef1", "둘이", "받을", "것")
	k.runE([]string{"HC_TERMINAL_ID=tA"}, "wait", "--timeout", "0")
	oA, rA := k.out, k.rc
	k.runE([]string{"HC_TERMINAL_ID=tB"}, "wait", "--timeout", "0")
	oB, rB := k.out, k.rc
	st := filepath.Join(k.T, ".local/state/hc-lane")
	isFile := func(p string) bool { return exists(p) && !isDir(p) }
	k.check("커서: 지휘 둘이 같은 사건을 각자 받음", rA == 0 && rB == 0 && strings.Contains(oA, "lane-k\t완료") && strings.Contains(oB, "lane-k\t완료") &&
		isFile(st+"/seen.tA") && isFile(st+"/seen.tB"))
	k.check("커서: 새 커서는 2시간 전 사건부터(옛 사건을 쏟지 않음)", !strings.Contains(oA, "old-ev"))
	k.runE([]string{"HC_TERMINAL_ID=tA"}, "wait", "--timeout", "0")
	k.check("커서: 같은 지휘에게는 다시 안 알림", k.rc == 75)
	k.unset("HC_FAKE_MACH_HOME", "HC_FAKE_MACHINES", "HC_FAKE_LS")

	// print-steps·띄우기: report 줄, HC_LANE·라벨
	k.run("--print-steps", "--name", "lane-p", "--repo", k.R)
	k.check("print-steps: 쓰기 레인에 hc-lane report 줄(중간·완료·착지·막힘)", s20lineN(k.out, "hc-lane report", "완료") > 0 && s20lineN(k.out, "hc-lane report", "착지") > 0)
	k.run("--print-steps", "--read", "--name", "lane-pr", "--repo", k.R)
	k.check("print-steps --read: 판정 줄은 hc-lane report 판정 <대상 커밋 40자 sha, 아니면 ->(sha는 구조 칸)", k.rc == 0 && has("hc-lane report 판정 <대상 커밋 40자 sha, 커밋이 아니면 ->") && !has("hc-lane land"))
	k.check("print-steps --read: rm 변수·와일드카드 경로 금지 줄(mktemp -d)", s20lineN(k.out, "mktemp -d", "rm -rf", `"${X:?}"`) > 0)
	k.run("--print-steps", "--name", "lane-p", "--repo", k.R)
	k.check("print-steps: 쓰기 레인에도 rm 변수·와일드카드 경로 금지 줄 한 번", s20lineN(k.out, "mktemp -d", "와일드카드") == 1)
	k.s20rm("")
	k.run("--name", "lane-e", "--repo", k.R, "--", "true")
	k.check("띄우기: HC_LANE=<이름>을 넘기고 라벨 hc-lane=1", k.rc == 0 && k.has("HC_LANE=lane-e") && k.pair("--label", "hc-lane=1"))
	k.s20rm("")
	k.run("--read", "--name", "lane-er", "--repo", k.R, "--", "true")
	k.check("띄우기 --read: HC_LANE·라벨, 판정 줄 안내", k.rc == 0 && k.has("HC_LANE=lane-er") && k.pair("--label", "hc-lane=1") && has("hc-lane report 판정 <대상 커밋"))
}
