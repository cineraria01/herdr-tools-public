package hclane_test

// tests/hc-lane.test.sh 20절 가운데(1437–1568줄): events(이 기계 + 연결된 기계), wait(새 사건·상한·/dev/null·멈춤·turn·모름 기계·한글 상태 비교).
// s20a(s20_report_test.go)가 만든 t20 저장소·events.tsv 위치를 이어 쓴다.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func init() { reg(81, "s20b", s20Wait) }

// s20tabs: grep "$tab"(탭 든 줄).
func s20tabs(out string) []string {
	var l []string
	for _, s := range strings.Split(out, "\n") {
		if strings.Contains(s, "\t") {
			l = append(l, s)
		}
	}
	return l
}

// s20col: printf "%s\n" "$x" | cut -f<n> | tr "\n" " ".
func s20col(x string, n int) string {
	var b strings.Builder
	for _, l := range strings.Split(x, "\n") {
		b.WriteString(s20fld(l, n) + " ")
	}
	return b.String()
}

// s20after: grep -Fx -A1 -- <opt> <파일> | grep -cFx <val>.
func s20after(p, opt, val string) int {
	l, n := strings.Split(strings.TrimRight(read(p), "\n"), "\n"), 0
	for i := 1; i < len(l); i++ {
		if l[i] == val && l[i-1] == opt {
			n++
		}
	}
	return n
}

// s20fx: grep -Fxq <s> <파일>.
func s20fx(p, s string) bool { return contains(strings.Split(read(p), "\n"), s) }

func s20Wait(k *K) {
	EV := k.s20ev()
	seen := filepath.Join(k.T, ".local/state/hc-lane/seen")
	outx := func(s string) bool { return contains(strings.Split(k.out, "\n"), s) }
	has := func(s string) bool { return strings.Contains(k.out, s) }

	// events: 이 기계 + 연결된 기계를 시각 순으로, 연결 안 된 기계는 「모름」
	write(k.t, EV, "")
	MH := filepath.Join(k.T, "mh")
	k.set("HC_FAKE_MACH_HOME=" + MH)
	mkdirs(k.t, MH+"/m1/.local/state/hc-lane", MH+"/m2/.local/state/hc-lane")
	appendf(k.t, EV, ev("2026-11-16T02:00:00Z", "here", "t1", "lane-l", "중간", "-", "로컬"))
	write(k.t, MH+"/m1/.local/state/hc-lane/events.tsv", ev("2026-11-16T02:00:03Z", "host1", "t7", "lane-a", "완료", "aaaa1111", "일")+
		ev("2026-11-16T02:00:01Z", "host1", "t8", "lane-b", "중간", "-", "둘"))
	write(k.t, MH+"/m2/.local/state/hc-lane/events.tsv", ev("2026-11-16T02:00:02Z", "host2", "t9", "lane-c", "착지", "cccc2222", "셋"))
	k.set(`HC_FAKE_MACHINES={"machines":[{"host":"h1","name":"m1","state":"connected"},{"name":"m2","host":"h2","state":"connected"},{"name":"m3","state":"connected"},{"name":"m4","state":"disconnected"}]}`)
	k.run("events", "--all")
	evs := strings.Join(s20tabs(k.out), "\n")
	k.check("events --all: 종료 0, 네 줄", k.rc == 0 && len(s20tabs(evs)) == 4)
	k.check("events --all: 시각 순", s20col(evs, 4) == "lane-l lane-b lane-c lane-a ")
	k.check("events --all: 원격 줄은 hc 기계 이름과 <기계>/<터미널> 주소", strings.Contains(evs, "\tm1\tm1/t7\tlane-a\t") && strings.Contains(evs, "\tm2\tm2/t9\t"))
	k.check("events --all: 연결 안 된 기계·exec 실패는 「모름 <기계>」 한 줄", outx("모름 m3") && outx("모름 m4"))
	// lane-nosh: sh를 띄우지 못하는 기계(Windows — hc exec가 spawn_failed·「cannot start sh」, 종료 1)는 레인 기록이 없는 기계로 보고
	// 조용히 건너뛴다. 그 밖의 오류(machine_unknown 등)는 그대로 「모름」
	write(k.t, MH+"/m5/.nosh", "")
	k.runE([]string{`HC_FAKE_MACHINES={"machines":[{"name":"m1","state":"connected"},{"name":"m3","state":"connected"},{"name":"m5","state":"connected"}]}`}, "events", "--all")
	k.check("events --all: sh 없는 기계는 「모름」 없이 0줄, 다른 오류는 그대로 「모름」(lane-nosh)", k.rc == 0 && !has("m5") && outx("모름 m3") && has("\tm1\tm1/t7\t"))
	os.RemoveAll(MH + "/m5")
	k.run("events", "--on", "m2")
	k.check("events --on: 이 기계와 그 기계만", k.rc == 0 && s20col(k.out, 4) == "lane-l lane-c ")
	k.run("events", "--all", "--since", "2026-11-16T02:00:02Z")
	k.check("events --since: 그 시각부터", s20col(strings.Join(s20tabs(k.out), "\n"), 4) == "lane-c lane-a ")
	k.check("events: 아무것도 안 바꿈(seen 없음)", !exists(seen))

	// wait: 본 적 없는 사건을 찍고 0, 같은 사건은 다시 안 알림, 상한 75, 원격은 바퀴마다 기계당 한 번
	k.set(`HC_FAKE_LS={"terminals":[]}`)
	os.Remove(k.HCLOG + ".exec")
	os.Remove(k.HCLOG + ".ls")
	write(k.t, seen, "") // 이미 있는 커서(바닥 없음): 고정 시각 사건을 그대로 본다
	k.run("wait", "--all", "--timeout", "0")
	k.check("wait: 쌓인 새 사건을 찍고 종료 0", k.rc == 0 && len(s20tabs(k.out)) == 4)
	k.check("wait: 원격은 기계당 hc exec 한 번, 연결 안 된 기계는 안 부름", s20after(k.HCLOG+".exec", "--on", "m1") == 1 && s20after(k.HCLOG+".exec", "--on", "m4") == 0)
	lsl := read(k.HCLOG + ".ls")
	k.check("wait: 레인 터미널은 hc ls 한 번(--all·라벨)", len(s20lines(k.HCLOG+".ls")) == 1 && strings.Contains(lsl, "--label hc-lane=1") && strings.Contains(lsl, "--all"))
	k.run("wait", "--all", "--timeout", "0")
	k.check("wait: 같은 사건은 두 번 알리지 않음, 상한이면 75", k.rc == 75 && len(s20tabs(k.out)) == 0)
	k.runE([]string{"HC_LANE=lane-q"}, "report", "중간", "-", "반쯤")
	k.run("wait", "--state", "완료,착지", "--timeout", "0")
	k.check("wait --state: 고른 상태만, 아니면 75", k.rc == 75)
	k.run("wait", "--timeout", "0")
	k.check("wait: --state로 거른 사건은 다른 wait에 남음", k.rc == 0 && has("lane-q\t중간\t-\t반쯤"))
	wout, werr := filepath.Join(k.T, "wout"), filepath.Join(k.T, "werr")
	w := k.bg(k.T, wout, werr, []string{"HC_LANE_WAIT_TICK=1"}, k.LANE, "wait", "--timeout", "1")
	time.Sleep(2 * time.Second)
	k.exe(k.T, "", []string{"HC_LANE=lane-q"}, k.LANE, "report", "착지", "dddd4444", "끝")
	wrc := w.Wait()
	k.check("wait: 기다리다 새 사건이 생기면 그 줄을 찍고 0", wrc == 0 && strings.Contains(read(wout), "lane-q\t착지\tdddd4444"))
	kids, _ := k.exe(k.T, "", nil, "pgrep", "-P", strconv.Itoa(w.Pid))
	k.check("wait: 끝난 뒤 남은 프로세스 없음", !alive(w.Pid) && kids == "" && w.Done())
	k.run("wait", "--timeout", "0")
	k.check("wait: 그 사건도 다시 안 알림", k.rc == 75)

	// wait: 출력이 /dev/null이면 읽을 사람이 없으니 seen을 건드리지 않고 종료 2(2026-11-17 지휘의 >/dev/null 배경 wait가 사건을 삼킴)
	bak := read(EV)
	k.exe(k.T, "", []string{"HC_LANE=lane-q"}, k.LANE, "report", "착지", "ffff6666", "끝")
	seen0 := read(seen)
	nrc := k.bg(k.T, os.DevNull, filepath.Join(k.T, "err"), nil, k.LANE, "wait", "--timeout", "0").Wait()
	k.check("wait >/dev/null: 종료 2, 한국어 오류, seen 안 바뀜", nrc == 2 && strings.Contains(k.errf(), "/dev/null") && read(seen) == seen0)
	frc := k.bg(k.T, wout, filepath.Join(k.T, "err"), nil, k.LANE, "wait", "--timeout", "0").Wait()
	k.check("wait >파일: 그 사건은 아직 남아 있어 찍는다", frc == 0 && strings.Contains(read(wout), "ffff6666"))
	write(k.t, EV, bak) // 뒤 시험이 events.tsv 줄 수를 센다

	// 멈춤: report 없이 끝났거나 막힌 레인 터미널(hc-lane이 띄운 것 = 라벨 hc-lane=1)
	appendf(k.t, EV, ev("2026-11-16T02:10:00Z", "here", "t43", "lane-landed", "착지", "eeee5555", "끝", "3"))
	k.run("wait", "--timeout", "0") // 위 착지 줄을 먼저 본다
	k.set(`HC_FAKE_LS={"terminals":[{"address":"t40","name":"lane-dead","state":"finished"},{"address":"m1/t41","name":"lane-blk","state":"live","status":"blocked"},{"address":"t42","name":"lane-ok","state":"live","status":"working"},{"address":"t43","name":"lane-landed","state":"finished","turn":4}]}`)
	k.run("wait", "--timeout", "0")
	k.check("wait 멈춤: --all 없으면 이 기계 레인만", k.rc == 0 && has("\tt40\tlane-dead\t멈춤\t-\t뜻밖의 종료(finished)") && !has("lane-blk"))
	k.check("wait 멈춤: 일하는 레인·착지를 알린 레인은 아님", !has("lane-ok") && !has("lane-landed\t멈춤"))
	k.run("wait", "--all", "--timeout", "0")
	k.check("wait 멈춤 --all: 다른 기계의 막힌 레인", k.rc == 0 && has("m1/t41\tlane-blk\t멈춤\t-\tblocked") && !has("lane-dead"))
	k.run("wait", "--all", "--timeout", "0")
	k.check("wait 멈춤: 두 번 알리지 않음", k.rc == 75)
	k.check("wait: seen 말고는 events.tsv도 안 바꿈", len(s20lines(EV)) == 4)

	// 감사 반영 2·4·5: report 없이 done·idle로 쉬는 레인, turn이 바뀌면 다시, 같은 주소의 다른 레인
	appendf(k.t, EV, ev("2026-11-16T02:20:00Z", "here", "t51", "lane-fin", "완료", "ffff6666", "끝"))
	appendf(k.t, EV, ev("2026-11-16T02:20:01Z", "here", "t52", "lane-mid", "중간", "-", "반"))
	appendf(k.t, EV, ev("2026-11-16T02:20:02Z", "here", "t60", "old-lane", "착지", "abcd7777", "옛"))
	lsI := func(a, b int) string {
		return fmt.Sprintf(`HC_FAKE_LS={"terminals":[{"address":"t50","name":"lane-idle","state":"live","status":"done","turn":%d},{"address":"t51","name":"lane-fin","state":"live","status":"done","turn":2},{"address":"t52","name":"lane-mid","state":"live","status":"idle","turn":1},{"address":"t60","name":"new-lane","state":"finished"},{"address":"t70","name":"lane-b2","state":"live","status":"blocked","turn":%d}]}`, a, b)
	}
	k.runE([]string{lsI(3, 1)}, "wait", "--timeout", "0")
	k.check("멈춤: report 없이 done으로 쉬는 레인(turn 포함)", k.rc == 0 && has("\tt50\tlane-idle\t멈춤\t-\tdone turn 3"))
	k.check("멈춤: 마지막 사건이 중간뿐이면 idle도", has("\tt52\tlane-mid\t멈춤\t-\tidle"))
	k.check("멈춤: 완료를 알리고 쉬는 레인은 아님", !has("lane-fin\t멈춤"))
	k.check("멈춤: 같은 주소의 옛 레인 착지는 새 레인을 면제하지 않음", has("\tt60\tnew-lane\t멈춤\t-\t뜻밖의 종료(finished)"))
	k.runE([]string{lsI(3, 1)}, "wait", "--timeout", "0")
	k.check("멈춤: turn이 그대로면 다시 안 알림", k.rc == 75)
	k.runE([]string{lsI(4, 5)}, "wait", "--timeout", "0")
	k.check("멈춤: 다시 쉬거나(turn 4) 다시 막히면(turn 5) 또 알림", k.rc == 0 && has("lane-idle\t멈춤\t-\tdone turn 4") && has("lane-b2\t멈춤\t-\tblocked turn 5"))

	// 재감사 A: report가 그때의 hc turn을 적고, 그 turn+1보다 뒤에 쉬면(이어 받은 일을 report 없이 끝냄) 멈춤
	k.runE([]string{"HC_FAKE_GET_TURN=5", "HC_TERMINAL_ID=t80", "HC_LANE=lane-t"}, "report", "완료", "abc1234", "첫", "일")
	k.check("report: 여덟째 칸에 그때의 hc turn", k.rc == 0 && s20fld(k.s20tail(), 8) == "5")
	k.exe(k.T, "", []string{"HC_LANE=x"}, k.LANE, "report", "중간")
	k.check("report: 터미널 밖이면 turn -", s20fld(k.s20tail(), 8) == "-")
	lsT := func(s string, n int) string {
		return fmt.Sprintf(`HC_FAKE_LS={"terminals":[{"address":"t80","name":"lane-t","state":"live","status":"%s","turn":%d}]}`, s, n)
	}
	k.runE([]string{lsT("done", 6)}, "wait", "--timeout", "0")
	k.check("멈춤 turn: 완료 report한 그 턴이 끝나 쉬는 것(turn+1)은 아님", !has("lane-t\t멈춤"))
	k.runE([]string{lsT("done", 8)}, "wait", "--timeout", "0")
	k.check("멈춤 turn: 완료 뒤 turn이 오른 done은 멈춤", k.rc == 0 && has("\tt80\tlane-t\t멈춤\t-\tdone turn 8"))
	k.runE([]string{lsT("idle", 8)}, "wait", "--timeout", "0")
	k.check("멈춤 turn: 같은 turn에서 done→idle은 다시 안 알림(쉼으로 묶음)", k.rc == 75)
	k.runE([]string{`HC_FAKE_LS={"terminals":[{"address":"t81","name":"lane-nr","state":"live","status":"done","turn":3}]}`}, "wait", "--timeout", "0")
	k.check("멈춤 쉼: report 없는 레인 done turn 3은 알림", k.rc == 0 && has("lane-nr\t멈춤\t-\tdone turn 3"))
	k.runE([]string{`HC_FAKE_LS={"terminals":[{"address":"t81","name":"lane-nr","state":"live","status":"idle","turn":3}]}`}, "wait", "--timeout", "0")
	k.check("멈춤 쉼: 같은 turn에서 idle로 바뀌어도 다시 안 알림", k.rc == 75)
	k.runE([]string{"HC_FAKE_GET_TURN=7", "HC_FAKE_GET_SLEEP=10.4258", "HC_TERMINAL_ID=t82", "HC_LANE=lane-slow"}, "report", "중간", "-", "느린", "코어")
	k.check("report: hc get이 응답하지 않으면 3초 안에 turn -로 적음", k.rc == 0 && s20fld(k.s20tail(), 4)+"\t"+s20fld(k.s20tail(), 8) == "lane-slow\t-")
	slow, _ := k.exe(k.T, "", nil, "pgrep", "-fx", "sleep 10.4258")
	k.check("report: 느린 hc get을 끊은 뒤 남은 프로세스 없음(이 시험만의 표지 — 다른 검증의 sleep 10을 안 잡게)", slow == "")
	// 재감사 낮음 2: 이번 바퀴에 못 읽은 기계의 레인은 멈춤으로 보지 않는다(그 기계 사건을 모르니)
	mkdirs(k.t, k.T+"/mh2/m1")
	k.runE([]string{"HC_FAKE_MACH_HOME=" + k.T + "/mh2", `HC_FAKE_MACHINES={"machines":[{"name":"m1","state":"connected"},{"name":"m3","state":"connected"},{"name":"m4","state":"disconnected"}]}`,
		`HC_FAKE_LS={"terminals":[{"address":"m1/t91","name":"lane-r1","state":"live","status":"done","turn":2},{"address":"m3/t92","name":"lane-r3","state":"live","status":"done","turn":2},{"address":"m4/t93","name":"lane-r4","state":"finished"},{"address":"m3/t94","name":"lane-r5","state":"live","status":"blocked","turn":3}]}`},
		"wait", "--all", "--timeout", "0")
	k.check("멈춤: 읽은 기계 레인은 알리고, 모름 기계 레인은 판정 대신 「연결 안 됨」(하루 한 번)", k.rc == 0 && has("m1/t91\tlane-r1\t멈춤") && has("m3/t92\tlane-r3\t멈춤\t-\t연결 안 됨 m3") &&
		has("m4/t93\tlane-r4\t멈춤\t-\t연결 안 됨 m4") && !grep(k.out, "lane-r[34]\t멈춤\t-\t(done|뜻밖)"))
	k.check("멈춤: 모름 기계 레인이라도 blocked는 그대로 알림(감사 4차 [낮음] 1)", has("m3/t94\tlane-r5\t멈춤\t-\tblocked turn 3"))
	// lane-nosh: sh·ps를 못 띄우는 기계는 「모름」이 아니라 사건 없는 기계 — 그 기계 레인은 「연결 안 됨」 대신 정상 판정
	write(k.t, k.T+"/mh3/m5/.nosh", "")
	k.runE([]string{"HC_FAKE_MACH_HOME=" + k.T + "/mh3", `HC_FAKE_MACHINES={"machines":[{"name":"m5","state":"connected"}]}`,
		`HC_FAKE_LS={"terminals":[{"address":"m5/t95","name":"lane-r6","state":"live","status":"done","turn":2}]}`},
		"wait", "--all", "--timeout", "0")
	k.check("멈춤: sh 없는 기계 레인은 「연결 안 됨」 없이 정상 판정(lane-nosh)", k.rc == 0 && has("m5/t95\tlane-r6\t멈춤\t-\tdone turn 2") && !has("연결 안 됨") && !strings.Contains(k.errf(), "모름 m5"))

	// 작업 중 발견해 함께 고침: macOS awk는 UTF-8 로캘에서 문자열 비교가 strcoll이라 "착지" == "멈춤"이 참이었다
	// — 같은 한 줄로 완료→착지를 알리면 착지가 삼켜지고, --state 한 상태 거름이 꺼졌다. 비교는 LC_ALL=C로 한다
	utf := []string{"LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8"}
	k.runE([]string{"HC_LANE=lane-s", "HC_TERMINAL_ID=t99"}, "report", "완료", "abc1234", "같은", "줄")
	k.run("wait", "--timeout", "0")
	k.runE([]string{"HC_LANE=lane-s", "HC_TERMINAL_ID=t99"}, "report", "착지", "abc1234", "같은", "줄")
	k.runE(utf, "wait", "--timeout", "0")
	k.check("wait: 같은 레인·같은 한 줄이라도 완료 뒤 착지를 알림(한글 상태 비교)", k.rc == 0 && has("lane-s\t착지\tabc1234"))
	k.runE([]string{"HC_LANE=lane-s"}, "report", "중간", "-", "반쯤", "둘")
	k.runE(utf, "wait", "--state", "멈춤", "--timeout", "0")
	k.check("wait --state 멈춤: 중간 사건은 거름(한 상태만 골라도)", k.rc == 75 && !has("반쯤 둘"))
	k.run("wait", "--timeout", "0")
}
