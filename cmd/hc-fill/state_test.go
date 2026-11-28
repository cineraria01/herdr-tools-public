package main

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"herdr-tools/internal/journal"
	"herdr-tools/internal/opsstate"
)

// ops-state 모으기(2026-11-24): 가짜 hc·바퀴 기록으로 stateCollect — 닿지 못한 기계는 모름, 바퀴 기록을 다시 쓰고 원격 호출은
// hc ls·hc machine ls 두 번뿐, 감시 이름 규칙, 배포 기록 풀기

func stFx(t *testing.T, machineLS string) {
	t.Helper()
	oh := HOME
	t.Cleanup(func() { HOME, knobOnce, knobES = oh, sync.Once{}, nil })
	DATA = t.TempDir()
	HOME = t.TempDir()
	Q, AT, LOCAL, dry = DATA+"/queue.tsv", DATA+"/audit.tsv", "m", false
	confLines = []string{"local m", "machine m 3 2 -", "machine w 3 - -", "machine x 2 - -"}
	knobOnce, knobES = sync.Once{}, nil
	t.Setenv(journal.Env, DATA+"/journal.jsonl")
	wr := func(f, s string) {
		if err := os.WriteFile(DATA+"/"+f, []byte(s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	wr("lsjson", `{"terminals":[{"address":"w/t5","name":"lane-a","status":"working"},{"address":"t2","name":"`+watchName("m/t1")+`","status":"idle"}]}`)
	wr("mls", machineLS)
	wr("hc", "#!/bin/sh\nprintf '%s\\n' \"$*\" >>'"+DATA+"/hc.log'\ncase \"$1\" in ls) cat '"+DATA+"/lsjson';; machine) cat '"+DATA+"/mls' || exit 1;; esac\n")
	restFake(t, DATA)
	t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
	now := time.Now()
	add := now.Add(-3 * time.Hour).Format("2006-01-02T15:04")
	wr("queue.tsv", strings.Join([]string{"lane-a", "app", "b", "-", "-", "up", "w", "w/t5", "-", add, "-", "m/t1", "normal", "-", "-", "-", "r1"}, "\t")+"\n"+
		strings.Join([]string{"lane-b", "app", "b", "lane-a", "-", "wait", "-", "-", "-", add, "-", "m/t1", "normal", "-", "-", "-", "r2"}, "\t")+"\n")
	wr("load.tsv", "m\t1.20\t12\t"+itoa(now.Unix()-30)+"\t1\t3\t-\t0\t0\n"+"w\t30.00\t10\t"+itoa(now.Unix()-30)+"\t1\t3\t-\t0\t0\n"+
		"x\t0.10\t8\t"+itoa(now.Unix()-30)+"\t0\t2\t-\t0\t0\n")
	wr("last.tsv", "lane-a\t중간\t-\t"+now.Add(-2*time.Hour).Format("15:04")+"\t시험 통과\t-\tfresh\tr1\n")
	wr("tick.tsv", itoa(now.Unix()-40)+"\t900\t-\t-\n")
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// ② 닿지 못한 기계(연결 안 됨)는 모름 — 바퀴 기록에 지난 부하가 있어도 그것을 지금 값으로 적지 않는다. 원격 호출은 두 번
func TestStateCollectUnknownMachine(t *testing.T) {
	stFx(t, `{"machines":[{"name":"w","state":"connected"},{"name":"x","state":"disconnected"}]}`)
	s := stateCollect(time.Now())
	by := map[string]opsstate.Machine{}
	for _, m := range s.Machines {
		by[m.Name] = m
	}
	if x := by["x"]; x.Reach != "unknown" || x.Why != "연결 안 됨" || x.LoadPerCore != nil || x.Live != nil {
		t.Errorf("닿지 못한 기계: %+v", x)
	}
	if w := by["w"]; w.Reach != "ok" || w.LoadPerCore == nil || *w.LoadPerCore != 3 || w.Lanes == nil || *w.Lanes != 1 {
		t.Errorf("닿은 기계: %+v", w)
	}
	if m := by["m"]; m.LaneCap == nil || *m.LaneCap != 2 || m.Seats == nil || *m.Seats != 3 {
		t.Errorf("이 기계 상한: %+v", m)
	}
	b, _ := os.ReadFile(DATA + "/hc.log")
	if n := strings.Count(string(b), "\n"); n != 2 {
		t.Errorf("hc 부름 %d번(ls·machine ls 두 번이어야): %s", n, b)
	}
	for _, src := range s.Sources {
		if src.Name == "hc" && src.Calls != 2 {
			t.Errorf("hc 재료 호출 수 %d", src.Calls)
		}
		if src.Name == "tick" && (src.AgeS == nil || *src.AgeS < 40) {
			t.Errorf("바퀴 기록 나이: %+v", src)
		}
	}
	// 레인 단계·머문 시간·마지막 사건, 대기 줄 까닭, 감시
	if len(s.Lanes) != 1 || s.Lanes[0].Stage != "작업 중(working)" || s.Lanes[0].StageAgeS == nil || *s.Lanes[0].StageAgeS < 7000 ||
		s.Lanes[0].LastEvent != "중간 시험 통과" {
		t.Errorf("레인: %+v", s.Lanes)
	}
	if o := s.Queue.Oldest; o == nil || o.Lane != "lane-b" || o.Why != "선행 lane-a" || s.Queue.Counts["wait"] != 1 {
		t.Errorf("대기열: %+v %+v", s.Queue, o)
	}
	if len(s.Watch) != 1 || s.Watch[0].Alive == nil || !*s.Watch[0].Alive {
		t.Errorf("감시: %+v", s.Watch)
	}
	// 눈에 띄는 것: 같은 단계 2시간·배포 기록 없음은 재료 못 읽음으로(치우침 w 3.00·m 0.10은 옮길 대기 줄 없이는 안 낸다 — fill-polish-1124)
	s.Notable = opsstate.Notable(s, opsstate.Thresholds{StageMin: 90, SkewHi: 2, SkewLo: 0.5})
	all := strings.Join(s.Notable, "\n")
	for _, w := range []string{"기계 x 모름", "레인 lane-a", "재료 deploy 못 읽음"} {
		if !strings.Contains(all, w) {
			t.Errorf("눈에 띄는 것에 %q 없음:\n%s", w, all)
		}
	}
	var j bytes.Buffer
	if err := opsstate.JSON(&j, s); err != nil || !strings.Contains(j.String(), `"reach":"unknown"`) {
		t.Errorf("json: %v %s", err, j.String())
	}
}

// hc machine ls를 못 읽으면 원격 기계는 모두 모름(연결 안 됨이 아니라 연결 모름), 이 기계는 닿음
func TestStateCollectConnUnknown(t *testing.T) {
	stFx(t, "")
	os.Remove(DATA + "/mls")
	s := stateCollect(time.Now())
	for _, m := range s.Machines {
		if m.Name == "m" && m.Reach != "ok" || m.Name != "m" && (m.Reach != "unknown" || !strings.Contains(m.Why, "연결 모름")) {
			t.Errorf("%+v", m)
		}
	}
}

// 감사 atozipa 낮음 2: 마지막 바퀴에 못 읽은 기계의 지난 부하는 모름(치우침에도 안 쓴다), 그 바퀴에 읽은 기계는 그대로
func TestStateCollectStaleLoad(t *testing.T) {
	stFx(t, `{"machines":[{"name":"w","state":"connected"},{"name":"x","state":"connected"}]}`)
	now := time.Now()
	// 끝 칸 = 도는 검증 수(ht-skew-verify-cause) — 부하와 같은 때 읽은 값이라 같은 신선함 규칙. 칸 없음·「-」은 모름
	os.WriteFile(DATA+"/load.tsv", []byte("m\t1.20\t12\t"+itoa(now.Unix()-30)+"\t1\t3\t-\t0\t0\t1\n"+
		"w\t30.00\t10\t"+itoa(now.Unix()-3600)+"\t1\t3\t-\t0\t0\t2\n"+"x\t0.10\t8\t"+itoa(now.Unix()-30)+"\t0\t2\t-\t0\t0\t-\n"), 0o644)
	s := stateCollect(now)
	for _, m := range s.Machines {
		switch m.Name {
		case "w":
			if m.Reach != "ok" || m.LoadPerCore != nil || m.Live == nil || m.Verifies != nil {
				t.Errorf("지난 부하: %+v", m)
			}
		case "m":
			if m.LoadPerCore == nil || m.Verifies == nil || *m.Verifies != 1 {
				t.Errorf("그 바퀴 부하: %+v", m)
			}
		case "x":
			if m.LoadPerCore == nil || m.Verifies != nil {
				t.Errorf("검증 모름: %+v", m)
			}
		}
	}
}

// 감시 이름: hc-lane 규칙(글자 밖은 _, 소문자, 21자 자름)
func TestStateWatchName(t *testing.T) {
	if got := watchName("Box-1/T12"); got != "lane-watch-box-1_t12" {
		t.Errorf("%s", got)
	}
	if got := watchName(strings.Repeat("a", 30)); got != "lane-watch-"+strings.Repeat("a", 21) {
		t.Errorf("%s", got)
	}
}

// 배포 기록: 기계마다 같음·씀이 된 마지막 커밋, 실패만 있으면 빈 값(모름), 차례는 처음 본 차례
func TestStateDeployInstalled(t *testing.T) {
	ls := []string{
		"2026-11-24T10:00:00+09:00\t배포\t0\taaaaaaaaaaaa\ts\tc\tby\tp=씀 q=같음",
		"2026-11-24T11:00:00+09:00\t배포\t1\tbbbbbbbbbbbb\ts\tc\tby\tp=씀 q=실패 r=실패",
		"broken line",
	}
	v, order, at := deployInstalled(ls)
	if strings.Join(order, ",") != "p,q,r" || v["p"] != "bbbbbbbbbbbb" || v["q"] != "aaaaaaaaaaaa" || v["r"] != "" || at.Unix() != 1795485600 {
		t.Errorf("%v %v %v", v, order, at)
	}
}
