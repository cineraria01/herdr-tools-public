package hclane_test

// watch-project-route(2026-11-24): 주인 없는(또는 주인이 사라진) 레인의 사건은 그 레인 저장소를 다루는 살아 있는 지휘에게만, 없으면 모두에게.
// 저장소는 origin 주소를 정규화한 식별자(사건 13칸·지휘 저장소 기록 coordrepo.<주소>). 「안 받음」은 고정 pause 대신 뒤에 적은 표지 사건
// (report --to 두 지휘)을 둘 다 받은 뒤에 본다 — 표지보다 앞에 적힌 사건의 판정은 그때 이미 끝났다.

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

func init() { reg(111, "s21c", s21c) }

func s21c(k *K) {
	// 유예는 길게(600초) — 유예가 지나야 하는 장면만 그 watch를 유예 2초로 띄우고 받을 때까지 기다린다(유예는 watch가 처음 미룬 때부터라
	// 사건 시각을 옛 값으로 지어도 안 넘는다)
	k.set("XDG_STATE_HOME="+k.T+"/wsr", `HC_FAKE_LS={"terminals":[]}`, "HC_LANE_WAIT_TICK=1", "HC_LANE_WATCH_REPO_GRACE=600")
	defer k.unset("HC_LANE_WATCH_REPO_GRACE")
	k.unset("HC_FAKE_MACHINES", "HC_FAKE_MACH_HOME", "HC_FAKE_SCREEN", "HC_LANE_OWNER", "HC_LANE_REPO_ID")
	k.noconf()
	WD := k.T + "/wsr/hc-lane"
	L := k.HCLOG
	send, live := L+".send", L+".live"
	for _, p := range []string{send, L, L + ".sendfail"} {
		os.Remove(p)
	}
	// 저장소 A·B: 원격 원문은 기계와 무관한 주소(가짜), insteadOf로 이 시험의 bare 저장소를 받는다. A1·A2는 클론 경로·주소 꼴이 다른 같은 저장소
	P := filepath.Join(k.W, "s21c")
	k.sh(k.T, `set -e; mkdir -p "$1"; cd "$1"
for r in A B; do git init -q --bare -b main "$r.git"; git clone -q "$r.git" "seed$r" 2>/dev/null
  (cd "seed$r" && echo x >x && git add x && git commit -qm x && git push -q origin HEAD:main); done
mk() { git clone -q "$2.git" "$1"; git -C "$1" remote set-url origin "$3"; git -C "$1" config "url.$PWD/$2.git.insteadOf" "$3"; }
mk "클론 A1" A git@Example.com:Team/A.git
mk "다른/곳/A2" A https://example.com/team/a
mk B1 B ssh://git@example.com:22/team/b.git`, P)
	A1, A2, B1 := P+"/클론 A1", P+"/다른/곳/A2", P+"/B1"
	const rA, rB = "example.com/team/a", "example.com/team/b"
	write(k.t, live, "ta1 1111\ntb2 2222\n")
	launch := func(coord, addr, name, repo string) {
		k.runE([]string{"HC_TERMINAL_ID=" + coord, "HC_FAKE_RUN_ADDR=" + addr}, "--start-wait", "0", "--read", "--name", name, "--repo", repo, "--", "claude")
		k.check("s21c: "+coord+"가 "+name+" 띄움", k.rc == 0)
	}
	launch("ta1", "t8aa", "rx-a", A1)
	launch("tb2", "t8bb", "rx-b", B1)
	k.check("⑩ 지휘 저장소 기록: 띄운 저장소의 origin 정규화(scp 꼴·대문자·.git·ssh 포트를 같은 꼴로), pid별",
		read(WD+"/coordrepo.ta1") == "1111\t"+rA+"\n" && read(WD+"/coordrepo.tb2") == "2222\t"+rB+"\n")

	who := func(s string) string { return s21bWho(send, s, true) }
	n := 0
	// mark: 표지 사건을 받을 지휘 모두에게 보내고 그들이 받을 때까지 — 그 앞 사건의 판정이 끝났다
	mark := func(to ...string) {
		n++
		tag := "표지" + string(rune('A'+n))
		k.exe(k.T, "", nil, "env", "-u", "HC_TERMINAL_ID", "-u", "HC_LANE", k.LANE, "report", "중간", "-", tag, "--name", "hc-fill-idle", "--to", strings.Join(to, ","))
		k.check("s21c 표지 "+tag+" 받음", upto(func() bool { return who(tag) == strings.Join(to, " ")+" " }))
	}
	// rep: dir(저장소 안·밖)에서 report — 주인은 owner(*는 주인 없음)
	rep := func(dir, owner, term, lane string, args ...string) {
		k.exe(dir, "", []string{"HC_TERMINAL_ID=" + term, "HC_LANE=" + lane, "HC_LANE_OWNER=" + owner}, k.LANE, append([]string{"report"}, args...)...)
	}
	now := func() string { return time.Now().UTC().Format("2006-01-02T15:04:05Z") }

	wa, wb := k.bg(k.T, k.T+"/rwa", k.T+"/rwea", nil, k.LANE, "watch", "--to", "ta1"), k.bg(k.T, k.T+"/rwb", k.T+"/rweb", nil, k.LANE, "watch", "--to", "tb2")
	upto(func() bool { return s21bS(WD+"/watch.ta1/pid") && s21bS(WD+"/watch.tb2/pid") })

	// ① 주인 없는 A 레인 사건 → A 지휘(ta1)만. ⑩ 사건은 다른 클론(A2 — 경로·주소 꼴이 다름)에서 적었다
	rep(A2, "*", "t8cc", "inbox-a", "중간", "-", "알원에이")
	evs := s21bLines(WD + "/events.tsv")
	k.check("⑩ 사건 13칸: 다른 클론에서 적어도 같은 저장소 식별자", len(evs) > 0 && strings.HasSuffix(evs[len(evs)-1], "\t"+rA))
	mark("ta1", "tb2")
	k.check("① 주인 없는 A 사건은 A 지휘만(B 지휘 send 0)", who("알원에이") == "ta1 ")
	// ⑧ 병합기 착지(--name으로 적어 13칸 없음, 주인 *) → 같은 레인 이름의 13칸으로 A — A 지휘만
	appendf(k.t, WD+"/events.tsv", ev(now(), "mac", "-", "inbox-a", "착지", "abcdef12", "병합기착지팔", "-", "-", "-", "*", "-"))
	mark("ta1", "tb2")
	k.check("⑧ 병합기 착지(주인 없음)는 그 저장소 지휘만", who("병합기착지팔") == "ta1 ")
	// ③ 레인 저장소 모름(옛 꼴 12칸·모르는 레인) → 둘 다
	appendf(k.t, WD+"/events.tsv", ev(now(), "mac", "t8dd", "old-x", "중간", "-", "옛꼴삼", "-", "-", "-", "*", "-"))
	mark("ta1", "tb2")
	k.check("③ 레인 저장소를 모르면 두 지휘 모두", who("옛꼴삼") == "ta1 tb2 ")
	// ⑦ 주인 있는 사건은 저장소와 상관없이 주인만(B 지휘가 주인인 레인이 A 저장소에서 적어도 tb2만)
	rep(A2, "tb2", "t8bb", "rx-b", "중간", "-", "주인칠")
	mark("ta1", "tb2")
	k.check("⑦ 주인 있는 사건은 지금 그대로 주인만", who("주인칠") == "tb2 ")
	// ④ 지휘의 저장소를 모름(coord --set만, 저장소 기록 없음 — 옛 판 꼴) → 그 지휘는 다 받는다
	write(k.t, live, "ta1 1111\ntb2 2222\ntc3 3333\n")
	k.runE([]string{"HC_TERMINAL_ID=tc3"}, "coord", "--set")
	wc := k.bg(k.T, k.T+"/rwc", k.T+"/rwec", nil, k.LANE, "watch", "--to", "tc3")
	upto(func() bool { return s21bS(WD + "/watch.tc3/pid") })
	rep(B1, "*", "t8ee", "inbox-b", "중간", "-", "비사")
	mark("ta1", "tb2", "tc3")
	k.check("④ 저장소를 모르는 지휘(tc3)도 받고, A만 다루는 지휘(ta1)는 안 받음", who("비사") == "tb2 tc3 ")
	wc.stop()
	// 유예(지휘 지시): 그 저장소 지휘(ta1)·저장소 모르는 지휘(tc3)가 살아 있고 watch는 없다(유휴로 끝남 — 감사 1차 높음 1 장면).
	// 그들의 커서에 없는 동안 유예 안이면 미루고, 유예 뒤면 보내고, 커서에 있으면 받은 것으로 친다
	wa.stop()
	old := time.Now().Add(-10 * time.Minute).UTC().Format("2006-01-02T15:04:05Z")
	oldEv := func(term, lane, msg string) []string {
		return []string{old, "mac", term, lane, "막힘", "-", msg, "-", "-", "-", "*", "-", rA}
	}
	rep(A2, "*", "t8kk", "inbox-a4", "막힘", "-", "에이유예")
	mark("tb2")
	k.check("유예 안·미확인: 그 저장소 지휘가 받았는지 모르는 동안은 미룸(tb2 안 받음, 커서에도 안 적음)", who("에이유예") == "" && !strings.Contains(read(WD+"/seen.tb2"), "에이유예"))
	// 유예가 지나야 하는 장면: 그 watch만 유예 2초로 다시 띄우고 받을 때까지 기다린다(사건 시각은 유예와 상관없다 — 일부러 10분 전)
	wb.stop()
	wb = k.bg(k.T, k.T+"/rwb", k.T+"/rweb", []string{"HC_LANE_WATCH_REPO_GRACE=2"}, k.LANE, "watch", "--to", "tb2")
	appendf(k.t, WD+"/events.tsv", ev(oldEv("t8ll", "inbox-a5", "에이십일")...))
	k.check("⑪ 유예 뒤 미확인: A 지휘가 살아 있어도(watch 없음) 그 커서에 없으면 tb2가 받음", upto(func() bool { return who("에이십일") == "tb2 " }))
	cf := oldEv("t8mm", "inbox-a6", "에이확인")
	appendf(k.t, WD+"/seen.ta1", ev(append([]string{cf[0], "*"}, cf[2:]...)...)) // ta1이 wait로 받은 꼴(열쇠는 기계 칸 *)
	appendf(k.t, WD+"/events.tsv", ev(cf...))
	mark("tb2")
	k.check("유예 뒤 확인됨: 그 저장소 지휘 커서에 있으면 tb2는 안 보내고 받은 것으로 커서에 적음", who("에이확인") == "" && strings.Contains(read(WD+"/seen.tb2"), "에이확인"))
	// ② A 지휘가 끝남(터미널 없음) → A 사건은 남은 지휘가 받는다. ⑨ 끝난 A 지휘가 이미 받은 사건(알원에이·병합기착지팔)은 다시 안 감
	write(k.t, live, "tb2 2222\n")
	k.check("② ta1 watch가 지휘 사라짐으로 끝남", upto(wa.Done))
	rep(A1, "*", "t8ff", "inbox-a2", "중간", "-", "에이이")
	mark("tb2")
	k.check("② A 지휘가 사라지면 A 사건은 살아 있는 지휘(tb2)가 받음", who("에이이") == "tb2 ")
	k.check("⑨ 사라진 A 지휘가 받은 사건은 tb2에 다시 안 감", who("알원에이") == "ta1 " && who("병합기착지팔") == "ta1 ")
	// ⑤ 한 지휘가 두 저장소(tb2가 A에도 레인을 띄움) ⑥ 같은 저장소 지휘 둘(새 지휘 td4도 A) → 둘 다 한 번씩
	write(k.t, live, "tb2 2222\ntd4 4444\n")
	k.runE([]string{"HC_TERMINAL_ID=td4"}, "coord", "--set")
	launch("td4", "t8gg", "rx-d", A2)
	launch("tb2", "t8hh", "rx-b2", A1)
	wd := k.bg(k.T, k.T+"/rwd", k.T+"/rwed", nil, k.LANE, "watch", "--to", "td4")
	upto(func() bool { return s21bS(WD + "/watch.td4/pid") })
	rep(A2, "*", "t8ii", "inbox-a3", "중간", "-", "에이육")
	rep(B1, "*", "t8jj", "inbox-b2", "중간", "-", "비오")
	mark("tb2", "td4")
	k.check("⑤ 두 저장소를 다루는 지휘(tb2)는 A·B 모두 받음", strings.Contains(who("에이육"), "tb2") && who("비오") == "tb2 ")
	k.check("⑥ 같은 저장소 지휘 둘은 둘 다 한 번씩", who("에이육") == "tb2 td4 " && s21bCount(send, "에이육") == 2)
	wb.stop()
	wd.stop()
	// ⑫ 새 지휘(te5 — coord --set이 커서 바닥을 지금으로)보다 앞에 적힌 주인 없는 A 사건: te5는 바닥 앞이라 안 보내고, 유예 뒤 te5 커서에
	// 없으니 B 지휘(tf6)가 받는다(감사 2차 중간 1 장면 — watch가 하나도 없던 때 적힌 사건)
	write(k.t, live, "te5 5555\ntf6 6666\n")
	launch("tf6", "t8nn", "rx-f", B1)
	appendf(k.t, WD+"/events.tsv", ev(oldEv("t8oo", "inbox-a7", "바닥앞십이")...))
	k.runE([]string{"HC_TERMINAL_ID=te5"}, "coord", "--set")
	launch("te5", "t8pp", "rx-e", A1)
	we, wf := k.bg(k.T, k.T+"/rwe", k.T+"/rwee", nil, k.LANE, "watch", "--to", "te5"), k.bg(k.T, k.T+"/rwf", k.T+"/rwef", []string{"HC_LANE_WATCH_REPO_GRACE=2"}, k.LANE, "watch", "--to", "tf6")
	upto(func() bool { return s21bS(WD+"/watch.te5/pid") && s21bS(WD+"/watch.tf6/pid") })
	k.check("⑫ 새 지휘 바닥 앞 주인 없는 A 사건은 유예 뒤 B 지휘(tf6)가 받음", upto(func() bool { return who("바닥앞십이") == "tf6 " }))
	mark("te5", "tf6")
	k.check("⑫ te5는 바닥 앞이라 안 받음(tf6 한 번만)", who("바닥앞십이") == "tf6 ")
	we.stop()
	wf.stop()
	// ⑬ 바퀴마다 짓는 멈춤 줄(1칸이 늘 「지금」 — 감사 3차 높음 1): A 지휘(te5)는 살아 있고 watch가 없다. 주인 없는 A 레인 둘이 blocked.
	// 유예는 이 watch가 처음 미룬 때부터라 inbox-a8 멈춤은 유예(2초) 뒤 tf6이 받고, te5가 wait로 이미 받은 inbox-a9 멈춤은 안 받는다
	rep(A2, "*", "t8qq", "inbox-a8", "중간", "-", "멈춤앞팔")
	rep(A2, "*", "t8rr", "inbox-a9", "중간", "-", "멈춤앞구")
	lsT := func(as ...string) string {
		var t []string
		for _, a := range as {
			n := map[string]string{"t8qq": "inbox-a8", "t8rr": "inbox-a9"}[a]
			t = append(t, `{"address":"`+a+`","name":"`+n+`","cwd":"/","state":"live","status":"blocked"}`)
		}
		return `HC_FAKE_LS={"terminals":[` + strings.Join(t, ",") + `]}`
	}
	k.runE([]string{"HC_TERMINAL_ID=te5", lsT("t8rr")}, "wait", "--timeout", "0")
	k.check("⑬ 준비: te5가 wait로 inbox-a9 멈춤을 받음", k.rc == 0 && strings.Contains(k.out, "inbox-a9\t멈춤"))
	wf = k.bg(k.T, k.T+"/rwf", k.T+"/rwef", []string{lsT("t8qq", "t8rr"), "HC_LANE_WATCH_REPO_GRACE=2"}, k.LANE, "watch", "--to", "tf6")
	k.check("⑬ 주인 없는 A 레인 멈춤(바퀴마다 짓는 줄)도 유예 뒤 B 지휘(tf6)가 받음", upto(func() bool { return strings.Contains(read(send), "inbox-a8 멈춤") }))
	mark("tf6")
	k.check("⑬ A 지휘 커서에 있는 멈춤(inbox-a9)은 tf6에 안 감", !strings.Contains(read(send), "inbox-a9 멈춤"))
	wf.stop()
}
