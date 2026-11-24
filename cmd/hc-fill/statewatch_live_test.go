package main

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"herdr-tools/internal/journal"
)

// watch-need-live(2026-11-24): 「지휘 감시 안 돎」 거짓 항목 — 에이전트가 며칠 전 끝나 셸만 남은 지휘 터미널과 끝난 줄을 「감시가 필요하다」로
// 보고 바퀴마다 hc-lane coord --watch를 헛불렀다. 입구는 손댈 것(adviseNow — 상태 한 장 기본과 같은 상태)과 run 바퀴의 watchOwners.
// 가짜 hc(ls·machine ls)와 가짜 hc-lane(부른 것 적음, dead 목록의 지휘는 종료 1 「지휘 없음」, 그 밖은 「띄움」)

type wnlFx struct {
	t   *testing.T
	dir string
}

func wnlSetup(t *testing.T) *wnlFx {
	t.Helper()
	oh, ol, ot, oc, os2 := HOME, LOCAL, TERMS, CONN, LSSNAP
	t.Cleanup(func() {
		HOME, LOCAL, TERMS, CONN, LSSNAP, knobOnce, knobES = oh, ol, ot, oc, os2, sync.Once{}, nil
	})
	setData(t)
	HOME = t.TempDir()
	LOCAL, dry = "m", false
	confLines = []string{"local m", "machine m 3 2 -", "machine w 3 - -"}
	knobOnce, knobES = sync.Once{}, nil
	t.Setenv(journal.Env, DATA+"/journal.jsonl")
	f := &wnlFx{t, t.TempDir()}
	f.wr(f.dir+"/hc", "#!/bin/sh\ncase \"$1\" in ls) cat '"+f.dir+"/lsjson' || exit 1;; machine) cat '"+f.dir+"/mls';; esac\n", 0o755)
	f.wr(f.dir+"/hc-lane", "#!/bin/sh\necho \"$*\" >>'"+f.dir+"/calls'\n"+
		"if grep -qx \"$3\" '"+f.dir+"/dead' 2>/dev/null; then echo \"hc-lane: 지휘 없음 — $3 의 에이전트가 끝나 셸만 남음\" >&2; exit 1; fi\n"+
		"echo \"hc-lane: 감시 lane-watch-x 띄움\"\n", 0o755)
	f.wr(f.dir+"/mls", `{"machines":[{"name":"w","state":"connected"}]}`, 0o644)
	t.Setenv("XDG_STATE_HOME", f.dir+"/state")
	if err := os.MkdirAll(f.dir+"/state/hc-lane", 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", f.dir+":"+os.Getenv("PATH"))
	return f
}

func (f *wnlFx) wr(p, s string, mode os.FileMode) {
	f.t.Helper()
	if err := os.WriteFile(p, []byte(s), mode); err != nil {
		f.t.Fatal(err)
	}
}

// ls: 터미널 「주소 이름 상태」(이름·상태 「-」는 null — 셸만 남은 지휘는 이름 없음·상태 없음)
func (f *wnlFx) ls(terms ...[3]string) {
	var es []string
	for _, x := range terms {
		e := `{"address":"` + x[0] + `","state":"live","cwd":"/"`
		if x[1] != "-" {
			e += `,"name":"` + x[1] + `"`
		}
		if x[2] != "-" {
			e += `,"status":"` + x[2] + `"`
		}
		es = append(es, e+"}")
	}
	f.wr(f.dir+"/lsjson", `{"terminals":[`+strings.Join(es, ",")+`]}`, 0o644)
}

// queue: 줄 「이름 주소 주인 마지막보고(없으면 빈 글)」 — 모두 떠 있음(up), 기계는 주소의 앞(없으면 m)
func (f *wnlFx) queue(rows ...[4]string) {
	add := time.Now().Add(-72 * time.Hour).Format("2006-01-02T15:04")
	var q, l strings.Builder
	for i, r := range rows {
		m := "m"
		if a, _, ok := strings.Cut(r[1], "/"); ok {
			m = a
		}
		rid := "r" + itoa(int64(i))
		q.WriteString(strings.Join([]string{r[0], "app", "b", "-", "-", "up", m, r[1], "-", add, "-", r[2], "normal", "-", "-", "-", rid}, "\t") + "\n")
		if r[3] != "" {
			l.WriteString(strings.Join([]string{r[0], r[3], "-", "10:00", "-", "10:01", "fresh", rid}, "\t") + "\n")
		}
	}
	f.wr(Q, q.String(), 0o644)
	f.wr(DATA+"/last.tsv", l.String(), 0o644)
}

func (f *wnlFx) dead(owners ...string) { f.wr(f.dir+"/dead", strings.Join(owners, "\n")+"\n", 0o644) }

// tick: run 바퀴가 이미 읽은 표(TERMS·CONN, 띄우기 전이라 표를 믿음)로 watchOwners 한 번 — 이번에 부른 hc-lane 줄
func (f *wnlFx) tick() []string {
	f.t.Helper()
	b, _ := os.ReadFile(f.dir + "/lsjson")
	var ok bool
	if TERMS, ok = parseTerms(string(b)); !ok {
		f.t.Fatal("lsjson 못 읽음")
	}
	CONN, LSSNAP = []string{"w"}, true
	before, _ := os.ReadFile(f.dir + "/calls")
	watchOwners()
	after, _ := os.ReadFile(f.dir + "/calls")
	return strings.Fields(strings.ReplaceAll(strings.TrimSpace(string(after[len(before):])), "coord --watch ", ""))
}

// watchItems: 손댈 것 가운데 감시 항목(감시 안 돎·감시 모름) 열쇠와 무엇
func watchItems() []string {
	var o []string
	for _, it := range adviseNow(time.Now()).Items {
		if strings.HasPrefix(it.Key, "watch:") || strings.HasPrefix(it.Key, "unknown:watch:") {
			o = append(o, it.Kind+" "+it.What)
		}
	}
	return o
}

// ① 실제 장면 그대로: 에이전트가 끝나 셸만 남은 지휘(이름·상태 없음) + 그 주인의 옛 줄(마지막 보고 정리됨·제출·중간, 레인 터미널 없음)
// → 항목 0, hc-lane coord --watch 0 — 바퀴를 두 번 돌아도
func TestWatchNeedDeadCoordOldRows(t *testing.T) {
	f := wnlSetup(t)
	f.ls([3]string{"t1", "-", "-"})
	f.dead("t1")
	f.queue([4]string{"x1", "w/t5", "t1", "정리됨"}, [4]string{"x2", "w/t6", "t1", "제출"}, [4]string{"x3", "t7", "t1", "중간"})
	if it := watchItems(); len(it) != 0 {
		t.Errorf("감시 항목이 나옴: %v", it)
	}
	for i := 0; i < 2; i++ {
		if c := f.tick(); len(c) != 0 {
			t.Errorf("바퀴 %d: hc-lane coord --watch 부름 %v", i+1, c)
		}
	}
}

// ①-2 셸만 남은 지휘인데 그 레인 하나는 아직 돈다(열린 줄): 첫 바퀴는 hc-lane에 한 번 묻고(거절 — 종료 1) 그 지문을 적는다.
// 두 번째 바퀴는 부르지 않고 항목도 없다. 그 터미널에 에이전트를 다시 띄우면(상태가 생겨 지문이 바뀜) 다시 묻고 띄운다.
// 순서 반대(③ — 손댈 것은 바퀴의 stall 단계에서 watchOwners보다 먼저 돈다): 첫 바퀴 전 상태 한 장은 아직 기록이 없어 항목 1(놓치지 않기)
func TestWatchNeedDeadCoordLiveLane(t *testing.T) {
	f := wnlSetup(t)
	f.ls([3]string{"t1", "-", "-"}, [3]string{"w/t8", "x4", "working"})
	f.dead("t1")
	f.queue([4]string{"x4", "w/t8", "t1", ""}, [4]string{"x5", "w/t5", "t1", "중간"})
	if it := watchItems(); len(it) != 1 {
		t.Errorf("기록 전 항목 %v — 1이어야(순서 반대)", it)
	}
	if c := f.tick(); strings.Join(c, ",") != "t1" {
		t.Fatalf("첫 바퀴 부름 %v — t1 한 번이어야", c)
	}
	if it := watchItems(); len(it) != 0 {
		t.Errorf("거절 기록 뒤 항목: %v", it)
	}
	if c := f.tick(); len(c) != 0 {
		t.Errorf("두 번째 바퀴 부름 %v — 0이어야", c)
	}
	// 그 터미널에 에이전트가 다시 뜸: 지문이 바뀌어 다시 묻는다(이번엔 지휘 — 띄움), 기록은 지워진다
	f.ls([3]string{"t1", "coord", "working"}, [3]string{"w/t8", "x4", "working"})
	f.dead()
	if it := watchItems(); len(it) != 1 {
		t.Errorf("에이전트가 다시 뜬 지휘의 감시 항목 %v — 1이어야", it)
	}
	if c := f.tick(); strings.Join(c, ",") != "t1" {
		t.Errorf("다시 뜬 지휘 부름 %v", c)
	}
	if b, _ := os.ReadFile(DATA + "/watchdead.tsv"); strings.TrimSpace(string(b)) != "" {
		t.Errorf("기록이 남음: %q", b)
	}
}

// 감사 atp0flj 중간 1: 에이전트는 살아 있는데 hc-lane이 거절하는 지휘(재개해 pid가 지휘 정보와 다름 — 아직 coord --set 전). status가
// idle → working → idle로 바뀌어도 항목 0·부름 0(같은 열쇠가 상태가 바뀔 때마다 새 사건으로 나가지 않게). 지휘 정보(coord.<주소>)가
// 바뀌면(재개한 지휘가 레인을 띄우거나 wait를 걸면) 다시 묻고, 이번엔 받혀 띄운다
func TestWatchNeedLiveCoordRefusedStatusFlip(t *testing.T) {
	f := wnlSetup(t)
	coord := f.dir + "/state/hc-lane/coord.t1"
	f.wr(coord, "t1\t111\tcoord\t1700000000\n", 0o600)
	f.dead("t1")
	f.queue([4]string{"x4", "w/t8", "t1", ""})
	for i, st := range []string{"idle", "working", "idle", "done"} {
		f.ls([3]string{"t1", "coord", st}, [3]string{"w/t8", "x4", "working"})
		if i == 0 {
			if c := f.tick(); strings.Join(c, ",") != "t1" {
				t.Fatalf("첫 바퀴 부름 %v", c)
			}
			continue
		}
		if it := watchItems(); len(it) != 0 {
			t.Errorf("상태 %s: 항목 %v", st, it)
		}
		if c := f.tick(); len(c) != 0 {
			t.Errorf("상태 %s: 부름 %v", st, c)
		}
	}
	f.wr(coord, "t1\t222\tcoord\t1700000600\n", 0o600)
	f.dead()
	if it := watchItems(); len(it) != 1 {
		t.Errorf("지휘 정보가 바뀐 뒤 항목 %v — 1이어야(다시 물을 때까지 놓치지 않기)", it)
	}
	if c := f.tick(); strings.Join(c, ",") != "t1" {
		t.Errorf("지휘 정보가 바뀐 뒤 부름 %v", c)
	}
}

// ② 대조: 살아 있는 지휘 + 떠 있는 레인(보고 없음) + 감시 없음 → 항목 1·watch 띄움 1(권고 명령은 hc-lane이 받는 명령)
func TestWatchNeedLiveCoord(t *testing.T) {
	f := wnlSetup(t)
	f.ls([3]string{"t2", "coord", "idle"}, [3]string{"w/t9", "y1", "working"})
	f.queue([4]string{"y1", "w/t9", "t2", ""})
	it := adviseNow(time.Now()).Items
	n := 0
	for _, x := range it {
		if x.Key == "watch:t2" {
			n++
			if x.Kind != "watch" || len(x.Next) != 1 || x.Next[0] != "hc-lane coord --watch t2" {
				t.Errorf("항목: %+v", x)
			}
		}
	}
	if n != 1 {
		t.Errorf("감시 항목 %d개: %+v", n, it)
	}
	if c := f.tick(); strings.Join(c, ",") != "t2" {
		t.Errorf("부름 %v — t2 한 번이어야", c)
	}
}

// ③ 살아 있는 지휘 + 줄이 모두 끝남(터미널 없음·보고 중간 / 터미널 있음·보고 착지·정리됨) → 항목 0·부름 0
func TestWatchNeedLiveCoordAllDone(t *testing.T) {
	f := wnlSetup(t)
	f.ls([3]string{"t2", "coord", "idle"}, [3]string{"w/t9", "z2", "done"}, [3]string{"w/t4", "z3", "idle"})
	f.queue([4]string{"z1", "w/t5", "t2", "중간"}, [4]string{"z2", "w/t9", "t2", "착지"}, [4]string{"z3", "w/t4", "t2", "정리됨"},
		[4]string{"z4", "w/t3", "t2", ""})
	if it := watchItems(); len(it) != 0 {
		t.Errorf("감시 항목: %v", it)
	}
	if c := f.tick(); len(c) != 0 {
		t.Errorf("부름 %v", c)
	}
}

// ④ 지휘 터미널이 아예 없음(사라진 흔적) → 항목 0, 바퀴도 부르지 않음(표를 믿을 수 있을 때)
func TestWatchNeedCoordGone(t *testing.T) {
	f := wnlSetup(t)
	f.ls([3]string{"w/t9", "y1", "working"})
	f.queue([4]string{"y1", "w/t9", "t3", ""})
	if it := watchItems(); len(it) != 0 {
		t.Errorf("감시 항목: %v", it)
	}
	if c := f.tick(); len(c) != 0 {
		t.Errorf("부름 %v", c)
	}
}

// ⑤ hc ls를 못 읽음 → 지금처럼 「모름」(감시 칸 판정 못 함)
func TestWatchNeedLsUnknown(t *testing.T) {
	wnlSetup(t)
	q := strings.Join([]string{"y1", "app", "b", "-", "-", "up", "w", "w/t9", "-", "2026-11-20T10:00", "-", "t2", "normal", "-", "-", "-", "r0"}, "\t") + "\n"
	if err := os.WriteFile(Q, []byte(q), 0o644); err != nil {
		t.Fatal(err)
	}
	it := watchItems() // lsjson이 없어 가짜 hc ls는 종료 1
	if len(it) != 1 || !strings.HasPrefix(it[0], "unknown ") {
		t.Errorf("감시 항목: %v — 모름 하나여야", it)
	}
}

// ⑥ 운영 장면: 끝난 지 며칠 된 옛 줄 마흔 개(주인 다섯 — 셸만 남은 지휘 둘·사라진 지휘 셋, 보고 정리됨·제출·중간·없음·착지, 레인 터미널
// 모두 없음)가 쌓인 대기열로 깔린 첫 바퀴 → 새 항목 0·헛호출 0. 거절 기록이 아직 없는 첫 바퀴다
func TestWatchNeedOldQueueFirstTick(t *testing.T) {
	f := wnlSetup(t)
	f.ls([3]string{"o1", "-", "-"}, [3]string{"o2", "-", "-"}, [3]string{"w/t1", "unrelated", "working"})
	f.dead("o1", "o2")
	owners := []string{"o1", "o2", "o3", "o4", "o5"}
	reps := []string{"정리됨", "제출", "중간", "", "착지"}
	var rows [][4]string
	for i := 0; i < 40; i++ {
		addr := "w/t" + itoa(int64(100+i))
		if i%3 == 0 {
			addr = "t" + itoa(int64(100+i))
		}
		rows = append(rows, [4]string{"old" + itoa(int64(i)), addr, owners[i%5], reps[i%5]})
	}
	f.queue(rows...)
	if it := watchItems(); len(it) != 0 {
		t.Errorf("감시 항목: %v", it)
	}
	if c := f.tick(); len(c) != 0 {
		t.Errorf("첫 바퀴 부름 %v", c)
	}
}
