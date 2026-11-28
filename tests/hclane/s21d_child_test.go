package hclane_test

// ht-watch-child-job(2026-11-27): 레인이 띄운 하위 스크립트 터미널(지휘 기계 레인 기록에 없음, 부모 = 그 레인)이 끝나도 어느 지휘에게도
// 멈춤이 가지 않고, 기록에 든 진짜 레인이 뜻밖에 끝나면 그 주인 지휘에게만 간다. 지휘 둘(tc1·tc2)의 watch를 함께 돌려 본다.

import (
	"os"
	"strings"
)

func init() { reg(114, "s21d-child", s21dChild) }

func s21dChild(k *K) {
	k.set("XDG_STATE_HOME="+k.T+"/wch", "HC_LANE_WAIT_TICK=1")
	k.unset("HC_FAKE_MACHINES", "HC_FAKE_MACH_HOME", "HC_FAKE_SCREEN", "HC_LANE_OWNER", "HC_LANE_REPO_ID")
	k.noconf()
	WD := k.T + "/wch/hc-lane"
	send, live := k.HCLOG+".send", k.HCLOG+".live"
	os.Remove(send)
	write(k.t, live, "tc1 1111\ntc2 2222\n")
	for _, c := range []string{"tc1", "tc2"} {
		k.runE([]string{"HC_TERMINAL_ID=" + c}, "coord", "--set")
		k.check("s21d: 지휘 "+c+" coord --set", k.rc == 0)
	}
	write(k.t, WD+"/lanes.tsv", "t9aa\tht-par\ttc1\tread\n") // tc1이 이 기계에서 띄운 레인(launchsteps.go recordLane 꼴)
	who := func(s string) string { return s21bWho(send, s, true) }
	mark := func(tag string) {
		k.exe(k.T, "", nil, "env", "-u", "HC_TERMINAL_ID", "-u", "HC_LANE", k.LANE, "report", "중간", "-", tag, "--name", "hc-fill-idle", "--to", "tc1,tc2")
		k.check("s21d 표지 "+tag+" 받음(--to 목록은 주인 후보 — 받을 수 있는 첫 tc1 한 명, ht-alert-gate)", upto(func() bool { return who(tag) == "tc1 " }))
	}
	// 부모 레인 t9aa와 그 레인이 띄운 스크립트 터미널 t9cc(jt-real — 기록 없음, 부모 t9aa, exit 0)
	ls := func(par string) string {
		return `HC_FAKE_LS={"terminals":[{"address":"t9aa","name":"ht-par","cwd":"/","state":"` + par + `","reason":"exited","turn":3},` +
			`{"address":"t9cc","name":"jt-real","norec":true,"parent":"t9aa","state":"finished","reason":"exited","exit_code":0,"turn":0}]}`
	}
	k.set(ls("live"))
	wa, wb := k.bg(k.T, k.T+"/rca", k.T+"/rcea", nil, k.LANE, "watch", "--to", "tc1"), k.bg(k.T, k.T+"/rcb", k.T+"/rceb", nil, k.LANE, "watch", "--to", "tc2")
	upto(func() bool { return s21bS(WD+"/watch.tc1/pid") && s21bS(WD+"/watch.tc2/pid") })
	now := s20now()
	appendf(k.t, WD+"/events.tsv", ev(now, "m", "t9cc", "jt-real", "정리됨", "-", "hc-sweep R2", "0"))
	mark("표지가")
	k.check("① 레인이 띄운 스크립트 터미널의 끝남·정리됨은 어느 지휘에게도 멈춤이 아님", who("jt-real") == "" &&
		!strings.Contains(read(WD+"/seen.tc1")+read(WD+"/seen.tc2"), "jt-real"))
	wa.stop()
	wb.stop()
	// ③ 기록에 든 레인이 착지·판정 없이 끝남 → 주인 tc1에게만
	k.set(ls("finished"))
	wa, wb = k.bg(k.T, k.T+"/rca", k.T+"/rcea", nil, k.LANE, "watch", "--to", "tc1"), k.bg(k.T, k.T+"/rcb", k.T+"/rceb", nil, k.LANE, "watch", "--to", "tc2")
	k.check("③ 기록에 든 레인의 뜻밖의 종료는 주인 tc1이 받음", upto(func() bool { return strings.Contains(who("ht-par 멈춤"), "tc1") }))
	mark("표지나")
	k.check("③ 다른 지휘 tc2에는 안 감(seen.tc2에도 없음)", who("ht-par 멈춤") == "tc1 " && !strings.Contains(read(WD+"/seen.tc2"), "멈춤\tt9aa\tht-par") &&
		strings.Contains(read(WD+"/seen.tc1"), "멈춤\tt9aa\tht-par"))
	k.check("① 그 바퀴에도 jt-real은 어디에도 없음", who("jt-real") == "")
	k.t.Logf("보낸 글(hc send):\n%s\nseen.tc1:\n%s\nseen.tc2:\n%s", read(send), read(WD+"/seen.tc1"), read(WD+"/seen.tc2"))
	wa.stop()
	wb.stop()
}
