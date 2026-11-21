package hclane_test

// tests/hc-lane.test.sh 21절 watch 뒷부분(2116–2288줄): 쉼 종료(lane.watch_idle·hc-fill 대기열), HC_LANE_COORD, hc ls 실패·회복,
// 지휘 hc get 연결 실패, 제어 문자, coord pid 대조, watch-hold(질문·권한 창 동안 보류), coord 하위 명령, 레인 터미널에서 띄우기.
// s21a3이 export한 XDG_STATE_HOME=$T/wst 등과 t21 저장소·coord.t7cc를 물려받는다.

import (
	"os"
	"strings"
	"syscall"
	"time"
)

func init() { reg(103, "s21a4", s21aHold) }

// 셸 hr·ask1·ask2·askm·perm·idle·toolask·mdperm: watch-hold 시험의 지휘 화면(Claude Code 2.1.328 실제 화면, 가로줄만 줄임) — printf 결과 바이트 그대로.
var s21aHr = strings.Repeat("─", 60)

func s21aLines(l ...string) string { return strings.Join(l, "\n") + "\n" }

func s21aAsk1() string {
	return s21aLines("❯ 질문 하나를 띄워라", s21aHr, " ☐ 색", "", "색을 고르세요?", "", "❯ 1. 빨강", "     빨간색을 고릅니다", "  2. 파랑", "     파란색을 고릅니다",
		"  3. Type something.", s21aHr, "  4. Chat about this", "", "Enter to select · ↑/↓ to navigate · Esc to cancel")
}

func s21aAsk2() string {
	return s21aLines(s21aHr, "←  ☐ 색  ☐ 맛  ✔ Submit  →", "", "색?", "", "❯ 1. 빨강", "     빨간색", "  2. 파랑", "     파란색", "  3. Type something.", s21aHr, "  4. Chat about this", "",
		"Enter to select · Tab/Arrow keys to navigate · Esc to cancel")
}

func s21aAskm() string {
	return s21aLines(s21aHr, "←  ☐ 과일  ✔ Submit  →", "", "과일?", "", "❯ 1. [ ] 사과", "         사과", "  2. [ ] 배", "         배", "  4. [ ] Type something", "     Submit", s21aHr,
		"  5. Chat about this", "", "Enter to select · ↑/↓ to navigate · Esc to cancel")
}

func s21aPerm() string {
	return s21aLines(s21aHr, "⏺ Bash(rm -rf x)", " "+strings.Repeat("╌", 60), " This shell -c script runs rm and could not be checked",
		" ⚠ Claude Code will automatically block this request in 1:41, to avoid holding up an unattended session", " Do you want to proceed?", " ❯ 1. Yes", "   2. No", " Esc to cancel · Tab to amend")
}

// s21aIdle: 평소 화면(지휘 꼴: 입력 칸 위 가로줄에 제목, 아래 하단 줄 실행기 줄들).
func s21aIdle(msg string) string {
	return s21aLines("⏺ "+msg, "", "✻ Baked for 34s · done 오전 11:49", strings.Repeat("─", 40)+" 레인 착지 확인 ─", "❯ ", s21aHr, "  branch:main | ⇣7", "  [OMC] | Model: Opus 5.5",
		"  ... (+2 lines)", "  ⏵⏵ auto mode on (shift+tab to cycle)")
}

// s21aToolask·s21aMdperm: 화면 위쪽에 남의 입력 칸이나 답 끝 가로줄 + 사용자 말 ❯ 줄이 있어도 맨 아래 창은 창(재감사 R-M1).
func s21aToolask() string {
	return s21aLines("⏺ Bash(hc read t34x | tail -n 4)", "     "+s21aHr, "     ❯ ", "     "+s21aHr, "     branch:x", "", "⏺ 묻겠습니다.", "") + s21aAsk1()
}

func s21aMdperm() string {
	return s21aLines("⏺ 끝났습니다.", s21aHr, "❯ 다음 진행해", "") + s21aPerm()
}

func s21aHold(k *K) {
	R, T := k.R, k.T
	WD := k.s21aWD()
	live := []string{"HC_FAKE_LIVE=t7cc"}
	send := func() string { return read(k.HCLOG + ".send") }
	we := func() string { return read(T + "/we") }
	stopOr := func(b *Bg) { // upto '! alive $w' || kill $w
		if !upto(b.Done) {
			b.signal(false, syscall.SIGTERM)
		}
	}
	// 레인이 다 끝나고 lane.watch_idle분(시험은 초) 동안 사건도 없으면 watch가 스스로 끝남
	write(k.t, k.HCLOG+".lsfile", `{"terminals":[{"address":"t60","name":"lane-z","parent":"mac/t7cc","state":"live","status":"working","turn":1}]}`+"\n")
	w6 := k.s21aW(append([]string{"HC_LANE_WATCH_IDLE=2"}, live...), "--to", "t7cc")
	time.Sleep(3500 * time.Millisecond)
	k.check("watch: 이 지휘의 살아 있는 레인이 있으면 쉬어도 안 끝남", !w6.Done())
	write(k.t, k.HCLOG+".lsfile", `{"terminals":[{"address":"t60","name":"lane-z","parent":"mac/t7cc","state":"finished","turn":1},{"address":"t61","name":"lane-o","parent":"mac/t7","state":"live","status":"working","turn":1}]}`+"\n")
	upto(w6.Done)
	wrc := w6.Wait()
	k.check("watch: 이 지휘의 레인이 다 끝나고 그 시간이 지나면 종료 0(남의 지휘 레인은 안 셈), 잠금 지움", wrc == 0 && strings.Contains(we(), "끝낸다") && !exists(WD+"/watch.t7cc"))
	os.Remove(k.HCLOG + ".lsfile")
	// 감사 [중간] 1: hc-fill(launchd, $HC_TERMINAL_ID 없음)이 띄운 레인도 지휘(HC_LANE_COORD — hc-fill 설정 coord)의 watch를 띄움
	os.Remove(k.HCLOG)
	k.runE([]string{"HC_FAKE_LIVE=t7cc", "HC_LANE_COORD=t7cc"}, "--name", "lane-a5", "--repo", R, "--", "claude")
	k.check("HC_LANE_COORD(터미널 밖)면 그 지휘의 watch를 띄움", k.rc == 0 && k.pair("--name", "lane-watch-t7cc") && k.pair("--to", "t7cc") && k.has("--rm"))
	os.Remove(k.HCLOG)
	k.runE([]string{"HC_FAKE_LIVE=t7cc", "HC_LANE_COORD=t7cc", "HC_TERMINAL_ID=t3zz"}, "--name", "lane-a6", "--repo", R, "--", "claude")
	k.check("HC_LANE_COORD가 $HC_TERMINAL_ID보다 앞", k.rc == 0 && k.pair("--to", "t7cc") && !k.has("t3zz"))
	// 쉼 종료는 hc-fill 대기열에 걸린 요청(wait·up)이 없을 때만
	write(k.t, T+"/fq/queue.tsv", "lane-q1\tp\tb\t-\t-\twait\t-\t-\t-\t-\t-\n")
	fq := append([]string{"HC_FILL_DATA=" + T + "/fq", "HC_LANE_WATCH_IDLE=2"}, live...)
	w7 := k.s21aW(fq, "--to", "t7cc")
	time.Sleep(3500 * time.Millisecond)
	k.check("watch: hc-fill 대기열에 기다리는 요청이 있으면 레인이 없어도 안 끝남", !w7.Done())
	write(k.t, T+"/fq/queue.tsv", "lane-q1\tp\tb\t-\t-\tsent\tm\tm/t1\t-\t-\t-\n")
	upto(w7.Done)
	wrc = w7.Wait()
	k.check("watch: 대기열이 비면(sent만) 그 시간 뒤 끝남", wrc == 0 && strings.Contains(we(), "끝낸다"))
	// 2차 감사 [중간] A: 끝난 up 줄은 세지 않는다 — 살아 있는 up 레인은 hc ls가 센다
	write(k.t, T+"/fq/queue.tsv", "lane-q1\tp\tb\t-\t-\tup\tm\tm/t1\t-\t-\t-\n")
	w7 = k.s21aW(fq, "--to", "t7cc")
	stopOr(w7)
	wrc = w7.Wait()
	k.check("watch: 대기열의 끝난 up 줄은 세지 않고 그 시간 뒤 끝남", wrc == 0 && strings.Contains(we(), "끝낸다"))
	// 감사 [중간] 2: hc ls 실패는 실패 시작·회복 때 한 번씩만 보냄
	os.Remove(k.HCLOG + ".send")
	write(k.t, k.HCLOG+".lsfail", "")
	w8 := k.s21aW(live, "--to", "t7cc")
	upto(func() bool { return strings.Contains(send(), "hc ls 실패") })
	time.Sleep(7 * time.Second)
	k.check("watch: hc ls가 계속 실패해도 알림은 한 번", s21aCountF(send(), "hc ls 실패") == 1)
	os.Remove(k.HCLOG + ".lsfail")
	upto(func() bool { return strings.Contains(send(), "hc ls 회복") })
	time.Sleep(2500 * time.Millisecond)
	k.check("watch: 되살아나면 「회복」 한 번", s21aCountF(send(), "hc ls 회복") == 1)
	write(k.t, k.HCLOG+".lsfail", "")
	upto(func() bool { return s21aCountF(send(), "hc ls 실패") == 2 })
	k.check("watch: 다시 실패하면 한 번 더", s21aCountF(send(), "hc ls 실패") == 2)
	os.Remove(k.HCLOG + ".lsfail")
	w8.stop()
	// 감사 5: 지휘 hc get이 연결 실패면 끝내지 않고 계속 돈다(「없음」일 때만 끝냄)
	write(k.t, k.HCLOG+".getdown", "")
	w9 := k.s21aW(live, "--to", "t7cc")
	time.Sleep(5 * time.Second)
	k.check("watch: 지휘 hc get 연결 실패는 사라짐이 아님(계속 돈다)", !w9.Done())
	os.Remove(k.HCLOG + ".getdown")
	// 감사 [낮음] 1: 보내는 글에서 제어 문자를 지움
	os.Remove(k.HCLOG + ".send")
	k.s21aRep("lane-w", "중간", "-", "esc\033[2Jx\001y")
	upto(func() bool { return send() != "" })
	k.check("watch: hc send 글에 제어 문자가 없음", strings.Contains(send(), "esc[2Jxy") && !strings.ContainsAny(send(), "\001\002\003\004\005\006\007\010\033"))
	w9.stop()
	// 2차 감사 [중간] B: coord 파일의 pid·이름이 지금 그 주소의 터미널과 다르면 보내지 않고 끝내며, 사건은 커서에 안 적혀 events에 남는다
	os.Remove(k.HCLOG + ".send")
	k.s21aRep("lane-w", "완료", "cafe1234", "남의터미널")
	w10 := k.s21aW([]string{"HC_FAKE_LIVE=t7cc", "HC_FAKE_LIVE_PID=9999"}, "--to", "t7cc")
	stopOr(w10)
	wrc = w10.Wait()
	k.check("watch: 그 주소의 pid가 coord와 다르면 hc send 0번, 「지휘 없음」 남기고 종료 0", wrc == 0 && send() == "" && strings.Contains(we(), "지휘 없음") &&
		strings.Contains(read(WD+"/watch.log"), "지휘 없음"))
	k.runE([]string{"HC_TERMINAL_ID=t7cc", "HC_FAKE_LIVE=t7cc"}, "wait", "--timeout", "0")
	k.check("watch가 못 보낸 사건은 커서에 안 적혀 지휘의 wait가 받음", k.rc == 0 && strings.Contains(k.out, "남의터미널"))

	// watch-hold: 지휘가 질문·권한 창을 띄운 동안(status blocked, 또는 status를 안 알리는 하단 줄 지휘의 화면에 그 창 글자) 보내지 않고
	// 커서에도 안 적는다. 창이 닫히면 쌓인 사건을 한 번에. 상태를 못 읽으면 보낸다, lane.watch_hold_max 넘게 막혀 있으면 보낸다
	k.runE([]string{"HC_TERMINAL_ID=t7cc", "HC_FAKE_LIVE=t7cc"}, "wait", "--timeout", "0") // 앞 시험의 남은 사건은 커서로
	os.Remove(k.HCLOG + ".send")
	dlg := T + "/dlg.txt"
	write(k.t, dlg, "")
	k.set("HC_FAKE_SCREEN=" + dlg) // 지휘 화면(배경 watch가 도중에 바뀐 화면을 보게 파일로)
	cstat := k.HCLOG + ".cstat"
	write(k.t, cstat, `{"app":"claude-code","msg":"a } b","state":"blocked"}`+"\n") // msg가 state보다 앞(실제 hc 꼴), 그 안의 「}」(감사 L4)
	pidUp := func() { upto(func() bool { return strings.TrimSpace(read(WD+"/watch.t7cc/pid")) != "" }) }
	seen := func() string { return read(WD + "/seen.t7cc") }
	wh := k.s21aW(live, "--to", "t7cc")
	pidUp()
	k.s21aRep("lane-h", "완료", "abcd1111", "창중하나")
	k.s21aRep("lane-h2", "중간", "-", "창중둘")
	time.Sleep(3 * time.Second)
	k.check("watch-hold: 지휘 status가 blocked면 hc send 0번, 커서에 안 적음", send() == "" && !strings.Contains(seen(), "창중") && strings.Contains(we(), "질문·권한 창"))
	os.Remove(cstat)
	upto(func() bool { return send() != "" })
	time.Sleep(1500 * time.Millisecond)
	one := false // grep -F 창중하나 send | grep -Fq 창중둘
	for _, l := range strings.Split(send(), "\n") {
		if strings.Contains(l, "창중하나") && strings.Contains(l, "창중둘") {
			one = true
		}
	}
	k.check("watch-hold: 창이 닫히면(idle) 쌓인 사건을 한 번에 보냄", k.s21aSends() == 1 && one && strings.Contains(seen(), "창중하나"))
	// status를 안 알리는 지휘(하단 줄 실행기 — status null): 화면 맨 아래 덩어리가 질문·권한 창이면 막힘(감사 M2)
	os.Remove(k.HCLOG + ".send")
	write(k.t, cstat, "null\n")
	k.s21aRep("lane-h", "중간", "-", "화면창")
	for _, s := range []struct {
		name string
		f    func() string
	}{{"ask1", s21aAsk1}, {"ask2", s21aAsk2}, {"askm", s21aAskm}, {"perm", s21aPerm}, {"toolask", s21aToolask}, {"mdperm", s21aMdperm}} {
		write(k.t, dlg, s.f())
		time.Sleep(2500 * time.Millisecond)
		k.check("watch-hold: status 없는 지휘 화면이 실제 창("+s.name+")이면 보내지 않음", send() == "" && !strings.Contains(seen(), "화면창"))
	}
	write(k.t, dlg, s21aIdle("끝났습니다."))
	upto(func() bool { return send() != "" })
	k.check("watch-hold: 화면에서 창이 사라지면 보냄", strings.Contains(send(), "화면창"))
	// 거짓 양성(감사 M2 재현 둘): 지휘 대화 글에 창 글자를 인용하고 평소 입력 칸·하단 줄이 보이면 창이 아니다
	os.Remove(k.HCLOG + ".send")
	write(k.t, dlg, s21aIdle("기본 글자는 「Type something」·「Chat about this」·「Tab/Arrow keys to navigate」입니다."))
	k.s21aRep("lane-h", "중간", "-", "인용하나")
	upto(func() bool { return send() != "" })
	k.check("watch-hold: 대화 글에 질문 창 글자 인용 + 입력 칸이면 창 아님(보냄)", strings.Contains(send(), "인용하나"))
	os.Remove(k.HCLOG + ".send")
	write(k.t, dlg, s21aIdle("t34x가 「Do you want to proceed?」 창에 걸림 — tell Claude what to do differently"))
	k.s21aRep("lane-h", "중간", "-", "인용둘")
	upto(func() bool { return send() != "" })
	k.check("watch-hold: 대화 글에 권한 창 글자 인용 + 입력 칸이면 창 아님(보냄)", strings.Contains(send(), "인용둘"))
	wh.stop()
	// status 없음 + hc read 실패: 판정 못 함 → 보냄
	os.Remove(k.HCLOG + ".send")
	write(k.t, dlg, s21aPerm()) // 읽혔다면 창
	wh = k.s21aW([]string{"HC_FAKE_READ_FAIL=1", "HC_FAKE_LIVE=t7cc"}, "--to", "t7cc")
	pidUp()
	k.s21aRep("lane-h", "중간", "-", "못읽음")
	upto(func() bool { return send() != "" })
	k.check("watch-hold: 화면을 못 읽으면(판정 실패) 보냄", strings.Contains(send(), "못읽음"))
	wh.stop()
	// 오래 막힘(감사 M1): status null + 화면으로 잡은 창은 상한이 지나도 보내지 않고 「보류 중」 한 줄만(바퀴마다 되풀이 안 함)
	os.Remove(k.HCLOG + ".send")
	os.Remove(WD + "/watch.log")
	write(k.t, cstat, "null\n")
	write(k.t, dlg, s21aAsk1())
	wh = k.s21aW([]string{"HC_LANE_WATCH_HOLD_MAX=2", "HC_FAKE_LIVE=t7cc"}, "--to", "t7cc")
	pidUp()
	k.s21aRep("lane-h", "중간", "-", "화면오래")
	upto(func() bool { return strings.Contains(read(WD+"/watch.log"), "보류 중") })
	time.Sleep(3 * time.Second)
	k.check("watch-hold: status null + 창 + 상한 넘음이면 send 0회, 「보류 중」 한 줄(watch.log·stderr)", send() == "" &&
		s21aCountRe(read(WD+"/watch.log"), "t7cc 질문 창 .*보류 중") == 1 && s21aCountF(we(), "보류 중") == 1)
	wh.stop()
	k.runE([]string{"HC_TERMINAL_ID=t7cc", "HC_FAKE_LIVE=t7cc"}, "wait", "--timeout", "0")
	k.check("watch-hold: 보류한 사건은 커서에 없어 지휘의 wait가 받음", k.rc == 0 && strings.Contains(k.out, "화면오래"))
	// status blocked는 상한(lane.watch_hold_max)이 넘으면 보냄
	os.Remove(k.HCLOG + ".send")
	write(k.t, cstat, `{"state":"blocked"}`+"\n")
	wh = k.s21aW([]string{"HC_LANE_WATCH_HOLD_MAX=3", "HC_FAKE_LIVE=t7cc"}, "--to", "t7cc")
	pidUp()
	k.s21aRep("lane-h", "중간", "-", "오래막힘")
	time.Sleep(1500 * time.Millisecond)
	k.check("watch-hold: 상한 전에는 안 보냄", send() == "")
	upto(func() bool { return send() != "" })
	time.Sleep(2500 * time.Millisecond)
	k.check("watch-hold: status blocked가 lane.watch_hold_max 넘으면 보냄, 「넘게」 한 줄만", strings.Contains(send(), "오래막힘") && s21aCountF(we(), "넘게") == 1)
	wh.stop()
	k.unset("HC_FAKE_SCREEN")
	os.Remove(cstat)
	os.Remove(k.HCLOG + ".send")
	os.Remove(WD + "/coord.t7cc")
	w11 := k.s21aW(live, "--to", "t7cc")
	k.s21aRep("lane-w", "중간", "-", "파일없음")
	stopOr(w11)
	wrc = w11.Wait()
	k.check("watch: coord 파일이 없어도 보내지 않고 「지휘 없음」으로 끝남", wrc == 0 && send() == "" && strings.Contains(we(), "지휘 없음"))
	k.runE([]string{"HC_TERMINAL_ID=t7cc", "HC_FAKE_LIVE=t7cc"}, "wait", "--timeout", "0")
	k.check("지휘가 wait를 걸면 지휘 정보를 coord.<주소>에 남김(그 사건도 받음)", s21aCut(read(WD+"/coord.t7cc"), 1, 3) == "t7cc\t4242\tt7cc" && strings.Contains(k.out, "파일없음"))
	k.runE(live, "coord")
	k.check("coord: 파일과 hc get(pid·이름)이 같으면 주소, 종료 0", k.rc == 0 && k.out == "t7cc")
	k.runE([]string{"HC_FAKE_LIVE=t7cc", "HC_FAKE_LIVE_PID=9999"}, "coord")
	k.check("coord: pid가 다르면 종료 1, 까닭 한 줄", k.rc == 1 && k.out == "" && strings.Contains(k.errf(), "지휘 없음"))
	os.Remove(k.HCLOG)
	k.runE([]string{"HC_FAKE_LIVE=t7cc", "HC_FAKE_LIVE_PID=9999", "HC_LANE_COORD=t7cc"}, "--name", "lane-a7", "--repo", R, "--", "claude")
	k.check("레인 띄우기: HC_LANE_COORD가 coord와 안 맞으면 watch를 안 띄우고 한 줄(레인은 뜸)", k.rc == 0 && !k.has("watch") && strings.Contains(k.errf(), "지휘 없음"))
	bak := read(WD + "/coord.t7cc")
	write(k.t, T+"/coord.bak", bak)
	k.runE([]string{"HC_FAKE_LANETERM=t5ln", "HC_TERMINAL_ID=t5ln"}, "--name", "lane-a8", "--repo", R, "--", "claude")
	k.check("레인 터미널(라벨 hc-lane=1)에서 띄우면 지휘 정보로 안 적고 watch도 안 띄움", strings.TrimRight(read(WD+"/coord.t7cc"), "\n") == strings.TrimRight(read(T+"/coord.bak"), "\n") &&
		!exists(WD+"/coord.t5ln") && !k.pair("--to", "t5ln"))
}
