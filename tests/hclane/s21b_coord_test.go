package hclane_test

// tests/hc-lane.test.sh 21절 뒷부분(1): 지휘마다 coord·watch가 보내는 곳(지휘 둘·주인 없는 레인·옛 주인 seen·hc-fill 주인·11칸 수신자),
// coord --set·--check, watch 인자 오류. 앞부분(s21a — mk t21·$WD 아래 watch 시험)의 저장소·상태 폴더를 그대로 쓴다.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

func init() { reg(110, "s21b1", s21b1) }

// s21bRepo: 앞부분이 `mk t21`로 만든 저장소(경로는 결정적). 이 절만 돌릴 때는 없으니 만든다.
func s21bRepo(k *K) {
	k.D, k.B = filepath.Join(k.W, "t21"), "main"
	k.R = filepath.Join(k.D, "저장소 한글")
	if !isDir(k.R) {
		k.mk("t21")
	}
}

// s21bRep: 셸 `VAR=값 "$LANE" report … >/dev/null`.
func (k *K) s21bRep(extra []string, args ...string) {
	k.exe(k.T, "", extra, k.LANE, append([]string{"report"}, args...)...)
}

// s21bLines: 파일 줄들(없으면 nil).
func s21bLines(p string) []string {
	s := read(p)
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

// s21bWho: grep -F sub 파일 | cut -f1 [| sort] | tr "\n" " ".
func s21bWho(p, sub string, sorted bool) string {
	var l []string
	for _, s := range s21bLines(p) {
		if strings.Contains(s, sub) {
			f, _, _ := strings.Cut(s, "\t")
			l = append(l, f)
		}
	}
	if sorted {
		sort.Strings(l)
	}
	o := ""
	for _, s := range l {
		o += s + " "
	}
	return o
}

// s21bCount: grep -c sub 파일.
func s21bCount(p, sub string) int {
	n := 0
	for _, s := range s21bLines(p) {
		if strings.Contains(s, sub) {
			n++
		}
	}
	return n
}

// s21bCut: cut -f1-n 파일(줄마다, 줄바꿈으로 이음).
func s21bCut(p string, n int) string {
	var o []string
	for _, s := range s21bLines(p) {
		f := strings.Split(s, "\t")
		if len(f) > n {
			f = f[:n]
		}
		o = append(o, strings.Join(f, "\t"))
	}
	return strings.Join(o, "\n")
}

// s21bOwner: awk -F"\t" '$1 == addr { o = $3 } END { print o }'.
func s21bOwner(p, addr string) string {
	o := ""
	for _, s := range s21bLines(p) {
		f := strings.Split(s, "\t")
		if f[0] == addr {
			o = ""
			if len(f) > 2 {
				o = f[2]
			}
		}
	}
	return o
}

// s21bRow: awk -F"\t" '$1 == addr' 파일 | grep -q .
func s21bRow(p, addr string) bool {
	for _, s := range s21bLines(p) {
		if f, _, _ := strings.Cut(s, "\t"); f == addr {
			return true
		}
	}
	return false
}

// s21bS: [ -s 파일 ].
func s21bS(p string) bool { fi, err := os.Stat(p); return err == nil && fi.Size() > 0 }

func s21b1(k *K) {
	s21bRepo(k)
	k.set("XDG_STATE_HOME="+k.T+"/wst", `HC_FAKE_LS={"terminals":[]}`, "HC_LANE_WAIT_TICK=1")
	k.unset("HC_FAKE_MACHINES", "HC_FAKE_MACH_HOME", "HC_FAKE_SCREEN")
	k.noconf()
	WD := k.T + "/wst/hc-lane"
	L, lanes := k.HCLOG, WD+"/lanes.tsv"
	send, live := L+".send", L+".live"
	sub := func(s string) func() bool { return func() bool { return strings.Contains(read(send), s) } }
	pids := func(a, b string) func() bool {
		return func() bool { return s21bS(WD+"/watch."+a+"/pid") && s21bS(WD+"/watch."+b+"/pid") }
	}
	watch := func(to, sfx string) *Bg {
		return k.bg(k.T, k.T+"/wo"+sfx, k.T+"/we"+sfx, nil, k.LANE, "watch", "--to", to)
	}
	who := func(s string) string { return s21bWho(send, s, false) }
	whoS := func(s string) string { return s21bWho(send, s, true) }
	rep := func(term, lane string, args ...string) {
		k.s21bRep([]string{"HC_TERMINAL_ID=" + term, "HC_LANE=" + lane}, args...)
	}
	pause := func(ms int) { time.Sleep(time.Duration(ms) * time.Millisecond) }

	// 3차 감사 [중간] 1: 지휘 정보는 지휘마다(coord.<주소>), 사건은 그 레인을 띄운 지휘의 watch만 보낸다
	cs, _ := filepath.Glob(WD + "/coord.*")
	for _, c := range cs {
		os.Remove(c)
	}
	for _, p := range []string{lanes, send, L} {
		os.Remove(p)
	}
	write(k.t, live, "ta1 1111\ntb2 2222\n")
	k.runE([]string{"HC_TERMINAL_ID=ta1", "HC_FAKE_RUN_ADDR=t9aa"}, "--direct", "--name", "lane-ma", "--repo", k.R, "--", "claude")
	pause(1100) // 띄운 시각(초)이 달라야 「가장 최근」이 갈린다
	k.runE([]string{"HC_TERMINAL_ID=tb2", "HC_FAKE_RUN_ADDR=t9bb"}, "--direct", "--name", "lane-mb", "--repo", k.R, "--", "claude")
	k.check("지휘 둘: 지휘마다 지휘 정보 파일, 레인 기록에 띄운 지휘", s21bCut(WD+"/coord.ta1", 3) == "ta1\t1111\tta1" &&
		s21bCut(WD+"/coord.tb2", 3) == "tb2\t2222\ttb2" && contains(s21bLines(lanes), "t9aa\tlane-ma\tta1\tdirect") &&
		contains(s21bLines(lanes), "t9bb\tlane-mb\ttb2\tdirect"))
	k.check("지휘 둘: 각자 자기 watch(lane-watch-<주소>)를 띄움", k.pair("--to", "ta1") && k.pair("--to", "tb2") &&
		k.pair("--name", "lane-watch-ta1") && k.pair("--name", "lane-watch-tb2"))
	wa, wb := watch("ta1", "a"), watch("tb2", "b")
	upto(pids("ta1", "tb2"))
	rep("t9aa", "lane-ma", "완료", "aaaa1111", "에이것")
	rep("t9bb", "lane-mb", "완료", "bbbb2222", "비것")
	rep("t9cc", "lane-fill", "완료", "cccc3333", "필것") // 띄운 지휘가 없는 레인(hc-fill 꼴)
	upto(func() bool { return sub("에이것")() && sub("비것")() && sub("필것")() })
	pause(2500)
	k.check("지휘 둘: 각자 자기 레인 사건만 받음(남의 사건 send 0)", s21bUniq(send, "에이것") == "ta1" && s21bUniq(send, "비것") == "tb2" &&
		s21bCount(send, "에이것") == 1)
	k.check("지휘 둘: 주인을 모르는 레인 사건은 두 지휘 모두 한 번씩(넘겨받지 않음, sent에 안 적음)", whoS("필것") == "ta1 tb2 " && !s21bRow(lanes, "t9cc"))
	k.runE([]string{"HC_TERMINAL_ID=tb2"}, "wait", "--timeout", "0") // 다른 지휘가 hc-lane을 써도
	rep("t9aa", "lane-ma", "착지", "aaaa1111", "에이착지")
	upto(sub("에이착지"))
	k.check("지휘 둘: 다른 지휘가 wait를 걸어도 앞 지휘 watch는 계속 돌고 자기 사건을 받음", !wa.Done() && s21bWho(send, "에이착지", false) == "ta1 ")
	// 4차 감사 [중간] 1: 「보냄」은 사건 하나에 한 번 — 못 받은 것만 넘어감
	write(k.t, L+".sendfail", "")
	rep("t9bb", "lane-mb", "막힘", "-", "비못받음") // tb2가 보내려다 실패(커서·보냄 표시 없음)
	upto(func() bool {
		for _, s := range s21bLines(L + ".sendtry") {
			if strings.Contains(s, "비못받음") && strings.HasPrefix(s, "tb2") {
				return true
			}
		}
		return false
	})
	write(k.t, live, "ta1 1111\n") // tb2가 사라짐
	os.Remove(L + ".sendfail")
	rep("t9cc", "lane-fill", "착지", "cccc3333", "필착지")
	upto(func() bool { return sub("필착지")() && sub("비못받음")() })
	pause(2500)
	k.check("지휘 둘: 한 지휘가 사라지면 주인 모르는 레인 사건은 살아 있는 지휘(ta1)만", who("필착지") == "ta1 ")
	ta1Has := func(s string) int {
		n := 0
		for _, l := range s21bLines(send) {
			if strings.HasPrefix(l, "ta1") && strings.Contains(l, s) {
				n++
			}
		}
		return n
	}
	k.check("지휘 둘: 끝난 지휘(tb2)가 이미 받은 사건(비것)은 ta1에게 다시 안 감, 주인 모르는 필것은 ta1도 한 번만", ta1Has("비것") == 0 && ta1Has("필것") == 1)
	k.check("지휘 둘: tb2가 못 받은 사건(보내기 실패)은 ta1이 한 번 받음", who("비못받음") == "ta1 ")
	wa.stop()
	wb.stop()
	write(k.t, live, "") // 지휘가 아무도 없음
	os.Remove(send)
	rep("t9cc", "lane-fill", "중간", "-", "아무도없음")
	k.run("coord")
	k.check("coord: 살아 있는 지휘가 없으면 종료 1(hc-fill은 watch를 안 띄움)", k.rc == 1 && strings.Contains(k.errf(), "지휘 없음"))
	write(k.t, live, "ta1 1111\n")
	k.runE([]string{"HC_TERMINAL_ID=ta1"}, "wait", "--timeout", "0")
	k.check("보내지 못한 사건은 events에 남아 지휘가 wait를 걸면 받음", k.rc == 0 && strings.Contains(k.out, "아무도없음"))
	// wait로 받은 사건도 「보냄」: 그 지휘가 끝난 뒤 다른 지휘의 watch가 다시 보내지 않음
	write(k.t, live, "ta1 1111\ntb2 2222\n")
	k.runE([]string{"HC_TERMINAL_ID=tb2", "HC_FAKE_RUN_ADDR=t9ee"}, "--direct", "--name", "lane-me", "--repo", k.R, "--", "claude")
	rep("t9ee", "lane-me", "중간", "-", "비웨이트")
	k.runE([]string{"HC_TERMINAL_ID=tb2"}, "wait", "--timeout", "0")
	k.check("wait로 받음(tb2)", k.rc == 0 && strings.Contains(k.out, "비웨이트"))
	write(k.t, live, "ta1 1111\n")
	os.Remove(send)
	wa = watch("ta1", "a")
	rep("t9ee", "lane-me", "중간", "-", "비새것")
	upto(sub("비새것"))
	k.check("지휘가 wait로 받은 사건은 그 지휘가 끝난 뒤 남은 지휘 watch가 다시 안 보냄(새 것만)", sub("비새것")() && !sub("비웨이트")())
	wa.stop()
	// 4차 감사 [낮음] 1: 레인 기록이 -여도 hc parent가 있으면 그 지휘로
	write(k.t, live, "ta1 1111\ntb2 2222\n")
	os.Remove(send)
	appendf(k.t, lanes, "t9dd\tlane-md\t-\n")
	write(k.t, L+".lsfile", `{"terminals":[{"address":"t9dd","name":"lane-md","parent":"mac/ta1","state":"live","status":"working","turn":1}]}`+"\n")
	wa, wb = watch("ta1", "a"), watch("tb2", "b")
	rep("t9dd", "lane-md", "완료", "dddd4444", "디것")
	upto(sub("디것"))
	pause(2500)
	k.check("레인 기록이 -여도 hc parent(ta1)가 있으면 그 지휘에게만", who("디것") == "ta1 ")
	wa.stop()
	wb.stop()
	os.Remove(L + ".lsfile")
	// 5차 감사 [중간] 1: sent는 watch만 쓴다
	write(k.t, live, "ta1 1111\ntb2 2222\n")
	os.Remove(send)
	rep("t9aa", "lane-ma", "중간", "-", "에이늦음")
	k.runE([]string{"HC_TERMINAL_ID=tb2"}, "wait", "--timeout", "0")
	k.check("남의 지휘(tb2)의 wait는 ta1 레인 사건을 보기만 함", k.rc == 0 && strings.Contains(k.out, "에이늦음") && !strings.Contains(read(WD+"/sent"), "에이늦음"))
	wa = watch("ta1", "a")
	upto(sub("에이늦음"))
	k.check("그 뒤에도 주인 ta1의 watch가 자기 레인 사건을 보냄", who("에이늦음") == "ta1 ")
	write(k.t, L+".sendfail", "")
	rep("t9aa", "lane-ma", "막힘", "-", "에이남음") // ta1이 보내려다 실패
	pause(2500)
	write(k.t, live, "tb2 2222\n") // ta1이 끝남
	if !upto(wa.Done) {
		wa.signal(false, syscall.SIGTERM)
	}
	wa.Wait()
	os.Remove(L + ".sendfail")
	wb = watch("tb2", "b")
	upto(sub("에이남음"))
	pause(2500)
	k.check("ta1이 끝난 뒤 tb2 watch는 ta1이 안 보낸 것(에이남음)만 받음 — 보낸 것(에이늦음)은 다시 안 감", who("에이남음") == "tb2 " && s21bCount(send, "에이늦음") == 1)
	k.check("주인이 사라진 레인 사건을 보낸 watch도 주인이 되지 않음(레인 기록 그대로 ta1 — 감사 [중간] 1)", s21bOwner(lanes, "t9aa") == "ta1")
	wb.stop()
	// 6차 감사 [중간] 1(R6): 「보낸 것」 = sent ∪ 그 레인의 옛 주인 seen
	write(k.t, live, "ta1 1111\ntb2 2222\n")
	os.Remove(send)
	k.runE([]string{"HC_TERMINAL_ID=ta1", "HC_FAKE_RUN_ADDR=t9ff"}, "--direct", "--name", "lane-mf", "--repo", k.R, "--", "claude")
	rep("t9ff", "lane-mf", "완료", "ffff6666", "에프하나")
	k.runE([]string{"HC_TERMINAL_ID=ta1"}, "wait", "--timeout", "0")
	k.check("R6: ta1이 wait로 에프하나를 받음", k.rc == 0 && strings.Contains(k.out, "에프하나"))
	rep("t9ff", "lane-mf", "중간", "-", "에프둘")
	write(k.t, live, "tb2 2222\n") // ta1이 끝남
	wb = watch("tb2", "b")
	upto(sub("에프둘"))
	pause(4000)
	k.check("R6: 주인 ta1이 사라진 뒤 tb2는 에프둘만 받고, ta1이 wait로 받은 에프하나는 다시 안 받음", sub("에프둘")() && !sub("에프하나")())
	wb.stop()
	// hcfill-owner(2026-11-17): hc-fill add는 coord --set, 띄울 때 HC_LANE_OWNER로 그 지휘를 레인 기록 주인으로
	write(k.t, live, "ta1 1111\ntb2 2222\n")
	os.Remove(send)
	os.Remove(L)
	os.Remove(WD + "/coord.ta1")
	k.runE([]string{"HC_TERMINAL_ID=ta1"}, "coord", "--set")
	k.check("coord --set: 부른 지휘의 지휘 정보를 남기고 주소를 찍음", k.rc == 0 && k.out == "ta1" && s21bCut(WD+"/coord.ta1", 3) == "ta1\t1111\tta1")
	k.runE([]string{"HC_FAKE_LANETERM=t5ln"}, "coord", "--set", "t5ln")
	k.check("coord --set: 레인 터미널은 지휘 정보로 안 적고 종료 1", k.rc == 1 && !exists(WD+"/coord.t5ln"))
	pause(1100)
	k.runE([]string{"HC_TERMINAL_ID=tb2", "HC_FAKE_RUN_ADDR=t9gg"}, "--start-wait", "0", "--direct", "--name", "lane-mg", "--repo", k.R, "--", "claude")                           // tb2가 가장 최근에 레인을 띄움
	k.runE([]string{"HC_LANE_OWNER=ta1", "HC_LANE_COORD=ta1", "HC_LANE_FILL=1", "HC_FAKE_RUN_ADDR=t9hh"}, "--start-wait", "0", "--name", "lane-mh", "--repo", k.R, "--", "claude") // hc-fill(터미널 밖)
	k.check("HC_LANE_OWNER: 레인 기록 주인이 그 지휘(fill), 레인 환경에도 HC_LANE_OWNER", k.rc == 0 && contains(s21bLines(lanes), "t9hh\tlane-mh\tta1\tfill") && k.has("HC_LANE_OWNER=ta1"))
	k.runE([]string{"HC_LANE_OWNER=ta1", "HC_LANE_COORD=ta1", "HC_LANE_FILL=1", "HC_LANE_PRIORITY=긴급", "HC_FAKE_RUN_ADDR=t9hp"}, "--start-wait", "0", "--name", "lane-mp", "--repo", k.R, "--", "claude")
	k.check("HC_LANE_PRIORITY: hc-fill이 준 제출 우선을 레인 환경에", k.rc == 0 && k.has("HC_LANE_PRIORITY=긴급"))
	appendf(k.t, lanes, "t9ii\tlane-mi\t-\tfill\n") // 주인 없는 옛 hc-fill 레인
	wa, wb = watch("ta1", "a"), watch("tb2", "b")
	upto(pids("ta1", "tb2"))
	rep("t9hh", "lane-mh", "완료", "hhhh8888", "넣은지휘것")
	rep("t9ii", "lane-mi", "완료", "iiii9999", "주인없는것")
	upto(func() bool { return sub("넣은지휘것")() && s21bCount(send, "주인없는것") == 2 })
	pause(2500)
	k.check("대기열 레인: 넣은 지휘(ta1)만 받음 — 가장 최근 지휘(tb2)가 아님", who("넣은지휘것") == "ta1 ")
	k.check("주인 없는 옛 레인: 두 지휘 모두 한 번씩 받고 주인은 그대로 -", whoS("주인없는것") == "ta1 tb2 " && s21bOwner(lanes, "t9ii") == "-")
	// hcfill-early(2026-11-18): report --to <지휘>는 받을 지휘를 정한다
	k.exe(k.T, "", nil, "env", "-u", "HC_TERMINAL_ID", "-u", "HC_LANE", k.LANE, "report", "중간", "-", "받는이정함", "--name", "hc-fill-idle", "--to", "tb2")
	upto(sub("받는이정함"))
	pause(2500)
	k.check("report --to tb2: tb2 watch만 받음(주인 모르는 사건이어도 ta1은 안 받음)", who("받는이정함") == "tb2 ")
	k.run("report", "중간", "-", "x", "--to", "t1;rm")
	k.check("report --to: 주소 글자가 아니면 종료 2", k.rc == 2)
	wa.stop()
	wb.stop()
	// 감사 [중간] 1(b): 주인 지휘(ta1)가 레인이 뜬 뒤 사라지면 살아 있는 모든 지휘가 한 번씩, 누구도 넘겨받지 않는다
	os.Remove(send)
	write(k.t, live, "ta1 1111\ntb2 2222\ntc3 3333\n")
	k.runE([]string{"HC_TERMINAL_ID=tc3"}, "coord", "--set")
	k.runE([]string{"HC_LANE_OWNER=ta1", "HC_LANE_COORD=ta1", "HC_LANE_FILL=1", "HC_FAKE_RUN_ADDR=t9jj"}, "--start-wait", "0", "--name", "lane-mj", "--repo", k.R, "--", "claude")
	write(k.t, live, "tb2 2222\ntc3 3333\n") // ta1이 끝남
	wb, wc := watch("tb2", "b"), watch("tc3", "c")
	upto(pids("tb2", "tc3"))
	rep("t9jj", "lane-mj", "중간", "-", "주인사라짐1")
	upto(func() bool { return s21bCount(send, "주인사라짐1") == 2 })
	rep("t9jj", "lane-mj", "완료", "jjjj0000", "주인사라짐2")
	upto(func() bool { return s21bCount(send, "주인사라짐2") == 2 })
	pause(2500)
	k.check("주인이 사라진 레인: 살아 있는 두 지휘 모두 한 번씩, 다음 사건도 둘 다(넘겨받지 않음)", whoS("주인사라짐1") == "tb2 tc3 " && whoS("주인사라짐2") == "tb2 tc3 ")
	k.check("주인이 사라진 레인: 레인 기록 주인은 그대로 ta1(한 지휘가 영구히 넘겨받지 않음)", s21bOwner(lanes, "t9jj") == "ta1")
	wb.stop()
	wc.stop()
	// fill-handoff-integrity 할 것 4: 사건 수신자는 사건에 실린 수신 지휘(11칸)
	os.Remove(send)
	write(k.t, live, "ta1 1111\ntb2 2222\n")
	appendf(k.t, lanes, "t9kk\tlane-mk\tta1\nt9kk\tlane-mk\ttb2\n")
	wa, wb = watch("ta1", "a"), watch("tb2", "b")
	upto(pids("ta1", "tb2"))
	mk := func(owner string, args ...string) {
		k.s21bRep([]string{"HC_TERMINAL_ID=t9kk", "HC_LANE=lane-mk", "HC_LANE_OWNER=" + owner}, args...)
	}
	mk("ta1", "완료", "kkkk1111", "옛실행늦은것")
	appendf(k.t, WD+"/events.tsv", ev(time.Now().UTC().Format("2006-01-02T15:04:05Z"), "mac", "t9kk", "lane-mk", "중간", "-", "옛꼴모호", "3", "-"))
	upto(func() bool { return sub("옛실행늦은것")() && s21bCount(send, "옛꼴모호") == 2 })
	pause(2500)
	k.check("수신자: 11칸 주인(ta1) 사건은 주소를 새로 쥔 지휘(tb2)가 아니라 ta1만", who("옛실행늦은것") == "ta1 ")
	k.check("수신자: 11칸 없는 옛 사건 + 그 주소 주인 둘 → 귀속 안 함(두 지휘 모두)", whoS("옛꼴모호") == "ta1 tb2 ")
	mk("*", "중간", "-", "주인없음별")
	upto(func() bool { return s21bCount(send, "주인없음별") == 2 })
	k.check("수신자: 11칸 *(주인 없음)은 모든 지휘", whoS("주인없음별") == "ta1 tb2 ")
	wa.stop()
	wb.stop()
	mk("tb2", "중간", "-", "비주인만")
	mk("ta1", "중간", "-", "에이주인만")
	k.runE([]string{"HC_TERMINAL_ID=ta1"}, "wait", "--mine", "--timeout", "0", "--state", "중간")
	k.check("wait --mine: 11칸 주인이 tb2인 사건은 ta1에 안 보이고, ta1인 사건은 보임", !strings.Contains(k.out, "비주인만") && strings.Contains(k.out, "에이주인만"))
	write(k.t, live, "ta1 1111\n")
	k.run("coord", "--check", "ta1")
	k.check("coord --check: 살아 있고 지휘 정보와 맞으면 0, 아무것도 안 씀", k.rc == 0)
	k.run("coord", "--check", "tb2")
	k.check("coord --check: 사라진 지휘는 1", k.rc == 1 && strings.Contains(k.errf(), "coord --check"))
	k.runE([]string{"HC_TERMINAL_ID=ta1"}, "coord", "--set") // 아래 coord 시험: 가장 최근은 ta1
	k.run("coord")
	k.check("coord: 살아 있는 지휘 가운데 가장 최근에 레인을 띄운 곳(지금은 ta1)", k.rc == 0 && k.out == "ta1")
	os.Remove(live)
	k.unset("XDG_STATE_HOME", "HC_LANE_WAIT_TICK")
	k.run("watch")
	k.check("watch: --to 없으면 오류", k.rc != 0 && k.rc != 75 && strings.Contains(k.errf(), "--to"))
	k.run("watch", "--to", "t7cc", "--timeout", "5")
	k.check("watch: --timeout은 wait만", k.rc != 0 && k.rc != 75)
}

// s21bUniq: grep -F sub 파일 | cut -f1 | sort -u (줄바꿈으로 이음).
func s21bUniq(p, sub string) string {
	m := map[string]bool{}
	var l []string
	for _, s := range s21bLines(p) {
		if strings.Contains(s, sub) {
			f, _, _ := strings.Cut(s, "\t")
			if !m[f] {
				m[f] = true
				l = append(l, f)
			}
		}
	}
	sort.Strings(l)
	return strings.Join(l, "\n")
}
