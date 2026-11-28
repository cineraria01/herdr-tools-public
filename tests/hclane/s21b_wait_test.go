package hclane_test

// tests/hc-lane.test.sh 21절 뒷부분(2): 기계 이름 바꿈(sweep-rename 2), killed·read·fill 레인의 끝남(sweep-rename 5·M1·N1·L1·watch-parent),
// 작업 트리 아래 배경 프로세스(sweep-rename 6). 저장소는 앞부분의 t21.

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func init() { reg(111, "s21b2", s21b2) }

func s21bNow() string { return time.Now().UTC().Format("2006-01-02T15:04:05Z") }

func s21b2(k *K) {
	s21bRepo(k)
	k.unset("XDG_STATE_HOME", "HC_LANE_WAIT_TICK", "HC_FAKE_MACHINES", "HC_FAKE_MACH_HOME", "HC_FAKE_SCREEN")
	k.set(`HC_FAKE_LS={"terminals":[]}`)
	L := k.HCLOG
	live := L + ".live"
	EV := k.T + "/.local/state/hc-lane/events.tsv"
	has := func(s string) bool { return strings.Contains(k.out, s) }

	// sweep-rename 2(2026-11-17): 열쇠에서 기계 이름을 빼고, 옛 꼴 커서 줄도 같은 사건으로 읽는다
	MH := k.T + "/mh"
	mkdirs(k.t, MH+"/oldm/.local/state/hc-lane")
	nw := s21bNow()
	write(k.t, MH+"/oldm/.local/state/hc-lane/events.tsv", ev(nw, "x", "t5", "lane-rn", "완료", "abcd1234", "이름바꿈", "2"))
	k.conf("lane.wait_grace 0", "lane.wait_busy", "lane.wait_procs")
	lsR := func(m string) string {
		return fmt.Sprintf(`{"terminals":[{"address":"%s/t6","name":"lane-rn2","cwd":"/","state":"live","status":"blocked","turn":2},{"address":"%s/t7","name":"lane-rn3","cwd":"/","state":"live","status":"done","turn":2}]}`, m, m)
	}
	mach := func(m string) string {
		return `HC_FAKE_MACHINES={"machines":[{"name":"` + m + `","state":"connected"}]}`
	}
	k.runE([]string{"HC_TERMINAL_ID=tR", "HC_FAKE_MACH_HOME=" + MH, mach("oldm"), "HC_FAKE_LS=" + lsR("oldm")}, "wait", "--all", "--since", nw, "--timeout", "0")
	n := 0
	for _, l := range strings.Split(k.out, "\n") {
		if strings.Contains(l, "\tlane-rn") {
			n++
		}
	}
	k.check("이름 바꿈(준비): 옛 이름으로 사건·멈춤 셋을 받음", k.rc == 0 && n == 3)
	if err := os.Rename(MH+"/oldm", MH+"/newm"); err != nil {
		k.t.Fatal(err)
	}
	k.runE([]string{"HC_TERMINAL_ID=tR", "HC_FAKE_MACH_HOME=" + MH, mach("newm"), "HC_FAKE_LS=" + lsR("newm")}, "wait", "--all", "--timeout", "0")
	k.check("이름 바꿈: 기계 이름만 다른 같은 사건·멈춤은 다시 안 나옴", !has("\tlane-rn"))
	// 옮김: 옛 판이 적은 커서(기계 이름이 든 줄 그대로·멈춤 열쇠)도 같은 사건으로 읽는다
	SD := k.T + "/.local/state/hc-lane"
	write(k.t, SD+"/seen.tM", "#since\t"+nw+"\n"+ev(nw, "oldm", "oldm/t5", "lane-rn", "완료", "abcd1234", "이름바꿈", "2")+
		"멈춤\toldm/t6\tlane-rn2\tblocked turn 2\n멈춤\toldm/t7\tlane-rn3\t쉼 turn 2\n")
	k.runE([]string{"HC_TERMINAL_ID=tM", "HC_FAKE_MACH_HOME=" + MH, mach("newm"), "HC_FAKE_LS=" + lsR("newm")}, "wait", "--all", "--timeout", "0")
	k.check("이름 바꿈 옮김: 옛 꼴 커서 줄(옛 기계 이름)과 같은 사건은 안 나옴", !has("\tlane-rn"))
	k.noconf()

	// sweep-rename 5(2026-11-17 지휘): 지휘가 닫은 레인(killed)과 --read·hc-fill 레인의 「완료」 뒤 끝남은 「뜻밖의 종료」가 아니다
	lanes := SD + "/lanes.tsv"
	k.runE([]string{"HC_FAKE_RUN_ADDR=tk1"}, "--read", "--start-wait", "0", "--name", "lane-k1", "--repo", k.R, "--", "claude")
	k.check("read 레인 기록(lanes.tsv 넷째 칸 read)", k.rc == 0 && contains(s21bLines(lanes), "tk1\tlane-k1\t-\tread"))
	k.runE([]string{"HC_FAKE_LIVE=t7cc", "HC_LANE_COORD=t7cc", "HC_FAKE_RUN_ADDR=tk2x"}, "--start-wait", "0", "--name", "lane-k2x", "--repo", k.R, "--", "claude")
	k.check("HC_LANE_COORD만 물려받은 구현 레인은 fill이 아님(재감사 [낮음] 4 — fill은 hc-fill이 HC_LANE_FILL=1로)", k.rc == 0 && contains(s21bLines(lanes), "tk2x\tlane-k2x\t-"))
	k.runE([]string{"HC_FAKE_LIVE=t7cc", "HC_LANE_COORD=t7cc", "HC_LANE_FILL=1", "HC_FAKE_RUN_ADDR=tk2"}, "--start-wait", "0", "--name", "lane-k2", "--repo", k.R, "--", "claude")
	k.check("hc-fill 레인 기록(넷째 칸 fill)", k.rc == 0 && contains(s21bLines(lanes), "tk2\tlane-k2\t-\tfill"))
	k.runE([]string{"HC_FAKE_RUN_ADDR=tk5"}, "--start-wait", "0", "--name", "lane-k5", "--repo", k.R, "--", "claude")
	nw = s21bNow()
	appendf(k.t, EV, ev(nw, "here", "tk1", "lane-k1", "완료", "-", "조사끝", "2")+ev(nw, "here", "tk2", "lane-k2", "완료", "abcd1234", "고침", "2")+
		ev(nw, "here", "tk3", "lane-k3", "완료", "abcd1234", "감사기다림", "2")+ev(nw, "here", "tk4", "lane-k4", "중간", "-", "반", "2")+
		ev(nw, "here", "tk5", "lane-k5", "완료", "abcd1234", "구현", "2")+
		ev(nw, "here", "tk6", "lane-k6", "완료", "abcd1234", "손", "2")+ev(nw, "here", "tk6", "lane-k6", "정리됨", "-", "hc-sweep R2", "3"))
	appendf(k.t, lanes, "tk6\tlane-k6\t-\tfill\n")
	k.runE([]string{"HC_TERMINAL_ID=tK"}, "wait", "--state", "완료,중간", "--timeout", "0") // report 줄은 먼저 소비
	k.runE([]string{"HC_TERMINAL_ID=tK", `HC_FAKE_LS={"terminals":[{"address":"tk1","name":"lane-k1","state":"finished","reason":"exited","turn":3},{"address":"tk2","name":"lane-k2","state":"finished","reason":"exited","turn":3},{"address":"tk3","name":"lane-k3","state":"finished","reason":"killed","signal":1,"turn":3},{"address":"tk4","name":"lane-k4","state":"finished","reason":"killed","signal":1,"turn":9},{"address":"tk5","name":"lane-k5","state":"finished","reason":"exited","turn":3}]}`},
		"wait", "--state", "멈춤", "--timeout", "0")
	k.check("killed: 지휘가 닫은 레인은 완료·중간 뒤라도 안 알림", !grep(k.out, "lane-k[34]\t멈춤"))
	k.check("read·hc-fill 레인: 완료 그 턴의 끝남·정리됨은 조용", !grep(k.out, "lane-k[126]\t멈춤"))
	k.check("구현 레인: 완료(감사 기다림) 뒤 혼자 끝나도 조용(한 문 ht-alert-gate — 끝 보고 뒤)", k.rc == 75 && !has("lane-k5\t멈춤"))
	// 재감사 [중간] M1: 부모(지휘)가 죽어 함께 killed된 레인은 그대로 알림 — 한 문 뒤로는 끝 보고 뒤 끝남이 조용하니 아래 killed 장면은
	// 끝 보고 없이(중간 뒤) 끝난 레인으로 본다
	write(k.t, live, "tPlive 3333\n")
	nw = s21bNow()
	appendf(k.t, EV, ev(nw, "here", "tk8", "lane-k8", "중간", "abcd1234", "부모죽음", "2")+ev(nw, "here", "tk9", "lane-k9", "중간", "abcd1234", "부모살아", "2"))
	k.runE([]string{"HC_TERMINAL_ID=tK"}, "wait", "--state", "중간", "--timeout", "0")
	k.runE([]string{"HC_TERMINAL_ID=tK", `HC_FAKE_LS={"terminals":[{"address":"tk8","name":"lane-k8","parent":"mac/tPdead","state":"finished","reason":"killed","signal":1,"turn":3},{"address":"tk9","name":"lane-k9","parent":"tPlive","state":"finished","reason":"killed","signal":1,"turn":3}]}`},
		"wait", "--state", "멈춤", "--timeout", "0")
	k.check("killed M1: 부모(지휘)가 죽어 함께 끝난 레인은 그대로 뜻밖의 종료", k.rc == 0 && has("\ttk8\tlane-k8\t멈춤\t-\t뜻밖의 종료(finished)"))
	k.check("killed M1: 부모가 살아 있으면(지휘가 직접 닫음) 조용", !has("lane-k9\t멈춤"))
	write(k.t, live, "tPlive 3333\n")
	// 3차 감사 [중간] N1: 원격 레인의 parent 「<지휘 기계>/t…」. 낮음 1: 조용히 넘긴 killed는 커서에 적어 다시 안 옴
	MH3 := k.T + "/mh3"
	mkdirs(k.t, MH3+"/m1/.local/state/hc-lane")
	nw = s21bNow()
	write(k.t, MH3+"/m1/.local/state/hc-lane/events.tsv", ev(nw, "x", "tkR", "lane-kR", "중간", "abcd1234", "원격살아", "2")+ev(nw, "x", "tkS", "lane-kS", "중간", "abcd1234", "원격죽음", "2"))
	lsN := `{"terminals":[{"address":"m1/tkR","name":"lane-kR","parent":"mac/tPlive","state":"finished","reason":"killed","signal":1,"turn":3},{"address":"m1/tkS","name":"lane-kS","parent":"mac/tPdead","state":"finished","reason":"killed","signal":1,"turn":3}]}`
	m1 := []string{"HC_TERMINAL_ID=tN", "HC_FAKE_MACH_HOME=" + MH3, mach("m1")}
	k.runE(m1, "wait", "--all", "--since", nw, "--state", "중간", "--timeout", "0")
	k.runE(append(m1, "HC_FAKE_LS="+lsN), "wait", "--all", "--state", "멈춤", "--timeout", "0")
	k.check("killed N1: 원격 레인 parent가 「<이 기계>/t…」여도 살아 있으면 조용", !has("lane-kR\t멈춤"))
	k.check("killed N1: 원격 레인 parent가 죽었으면 그대로 뜻밖의 종료", k.rc == 0 && has("m1/tkS\tlane-kS\t멈춤\t-\t뜻밖의 종료(finished)"))
	// 3차 감사 [낮음] L1: 들어온 연결의 상대는 machine ls에 없다 — 그대로 풀리는 parent는 떼지 않는다
	appendf(k.t, live, "peer/tPeer 4444\n")
	nw = s21bNow()
	appendf(k.t, EV, ev(nw, "here", "tkL", "lane-kL", "중간", "abcd1234", "들어온연결", "2"))
	k.runE([]string{"HC_TERMINAL_ID=tN"}, "wait", "--since", nw, "--state", "중간", "--timeout", "0")
	k.runE([]string{"HC_TERMINAL_ID=tN", `HC_FAKE_LS={"terminals":[{"address":"tkL","name":"lane-kL","parent":"peer/tPeer","state":"finished","reason":"killed","signal":1,"turn":3}]}`}, "wait", "--state", "멈춤", "--timeout", "0")
	k.check("killed L1: machine ls에 없지만 그대로 풀리는 parent(들어온 연결의 지휘)가 살아 있으면 조용", !has("lane-kL\t멈춤"))
	os.Remove(live)
	k.runE(append(m1, "HC_FAKE_LS="+lsN), "wait", "--all", "--state", "멈춤", "--timeout", "0")
	k.check("killed 낮음 1: 조용히 넘긴 killed는 부모(지휘)가 끝난 뒤에도 다시 안 옴", !has("lane-kR\t멈춤"))
	os.Remove(live)
	// watch-parent(2026-11-17 지휘 실측): 지휘 기계에선 parent가 앞부분 없이 「t7」
	write(k.t, live, "tPlive 3333\n")
	nw = s21bNow()
	appendf(k.t, MH3+"/m1/.local/state/hc-lane/events.tsv", ev(nw, "x", "tkT", "lane-kT", "중간", "abcd1234", "접두없음살아", "2")+ev(nw, "x", "tkU", "lane-kU", "중간", "abcd1234", "접두없음죽음", "2"))
	lsP2 := `{"terminals":[{"address":"m1/tkT","name":"lane-kT","parent":"tPlive","die_with_parent":true,"state":"finished","reason":"killed","signal":1,"turn":3},{"address":"m1/tkU","name":"lane-kU","parent":"tPdead","die_with_parent":true,"state":"finished","reason":"killed","signal":1,"turn":3}]}`
	k.runE(m1, "wait", "--all", "--since", nw, "--state", "중간", "--timeout", "0")
	k.runE(append(m1, "HC_FAKE_LS="+lsP2), "wait", "--all", "--state", "멈춤", "--timeout", "0")
	k.check("killed watch-parent: 원격 레인 parent가 앞부분 없는 「t…」(지휘 기계 기준)이고 살아 있으면 조용", !has("lane-kT\t멈춤"))
	k.check("killed watch-parent: 앞부분 없는 parent가 죽었으면 그대로 뜻밖의 종료", k.rc == 0 && has("m1/tkU\tlane-kU\t멈춤\t-\t뜻밖의 종료(finished)"))
	os.Remove(live)
	// sweep-rename 6(2026-11-17 지휘): 그 레인 작업 트리 경로가 명령 줄에 든 프로세스가 살아 있으면 쉬는 중이 아니라 바쁨
	GS, PSF := k.T+"/gscreen", k.T+"/fakeps"
	write(k.t, GS, "✻ Churned for 3m · done 12:33 PM\n❯ \n")
	lsB := `{"terminals":[{"address":"tb9","name":"lane-b9","cwd":"/w/app.wt/lane-b9","state":"live","status":"done","turn":2}]}`
	write(k.t, PSF, " 4000     1 node agent-runner.js\n 4001  4000 claude --model x\n 6001     1 bash /w/app.wt/lane-b9/.claude/herdr-verify.sh\n 6002     1 bash /w/app.wt/lane-b99/x\n")
	bgEnv := []string{"HC_TERMINAL_ID=tK", "HC_FAKE_PID=4000", "HC_FAKE_PS=" + PSF, "HC_FAKE_SCREEN=" + GS, "HC_FAKE_STATUS_AGE=660", "HC_FAKE_LS=" + lsB}
	k.runE(bgEnv, "wait", "--state", "멈춤", "--timeout", "0")
	k.check("배경: 작업 트리 아래 프로세스가 살아 있으면 쉬는 레인을 멈춤으로 안 알림", k.rc == 75)
	write(k.t, PSF, " 4000     1 node agent-runner.js\n 4001  4000 claude --model x\n 6002     1 bash /w/app.wt/lane-b99/x\n")
	k.runE(bgEnv, "wait", "--state", "멈춤", "--timeout", "0")
	k.check("배경: 그 프로세스가 끝났으면(이름만 비슷한 옆 작업 트리는 안 셈) 멈춤 done", k.rc == 0 && has("\ttb9\tlane-b9\t멈춤\t-\tdone turn 2"))
}
