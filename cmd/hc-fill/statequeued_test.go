package main

// ht-fill-submitted-line(2026-11-26 — 다른 프로젝트 받은편지 레인이 제출 뒤 94분째 「제출됨(병합기 줄 모름)」·「같은 단계에 94분」으로
// 올랐는데 실제로는 큐에 서서 4개 묶음 추측 검증 중이었다): 상태 한 장이 병합 줄 차례를 늘 「모름」(Pos −1)으로 두었다. 바퀴가 이미 읽은
// 큐(병합기 살아 있음 기록 12칸 — 줄 차례대로 레인)와 병합기 specwait 구성원을 읽어 차례·「묶음 검증 중」을 보이고, 병합기가 살아 있고
// 줄에 있는 제출에는 lane-long을 올리지 않는다. 고치기 전 코드에는 maRow.Lanes가 없어 컴파일부터 실패한다

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// sqFx: stFx + 병합기 클론(이 기계 m, 터미널 t9) + lane-a의 마지막 사건 「제출」 2시간 전. ma = merger-alive.tsv 한 줄
func sqFx(t *testing.T) (clone string) {
	t.Helper()
	stFx(t, `{"machines":[{"name":"w","state":"connected"},{"name":"x","state":"connected"}]}`)
	confLines = append(confLines, "repo app m /r", "repo app w /r", "merger app m")
	clone = filepath.Join(t.TempDir(), "clone")
	if out, err := exec.Command("git", "init", "-q", clone).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	if err := os.MkdirAll(clone+"/.git/hc-merger", 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	sqWr(t, DATA+"/lsjson", `{"terminals":[{"address":"w/t5","name":"lane-a","state":"live","status":"working","cwd":"/r"},`+
		`{"address":"t9","name":"merger-app","state":"live","status":"working","cwd":"`+clone+`"}]}`)
	sqWr(t, DATA+"/last.tsv", "lane-a\t제출\tabcd1234\t"+now.Add(-2*time.Hour).Format("15:04")+"\tx → refs/merge-queue/1-lane-a\t-\tfresh\tr1\n")
	return clone
}

func sqWr(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// sqMA: 병합기 살아 있음 기록(상태 큐수 줄 [병합기 기록 JSON])
func sqMA(t *testing.T, st, k, lanes string, rec ...string) {
	now := itoa(time.Now().Unix())
	sqWr(t, DATA+"/merger-alive.tsv", strings.Join(append([]string{"app", "m", st, now, "t9", k, "-", now, "-", "-", "-", lanes}, rec...), "\t")+"\n")
}

// sqLane: 상태 한 장의 lane-a와 손댈 것에 lane-long:lane-a가 있나
func sqLane(t *testing.T) (stage string, queued, long bool) {
	t.Helper()
	now := time.Now()
	for _, l := range stateCollect(now).Lanes {
		if l.Name == "lane-a" {
			stage, queued = l.Stage, l.Queued
		}
	}
	for _, it := range adviseNow(now).Items {
		long = long || it.Key == "lane-long:lane-a"
	}
	return
}

func TestStateSubmittedQueued(t *testing.T) {
	sqFx(t)
	now := time.Now()
	// 병합기 기록(ht-merger-state-record — 옛 재료는 클론의 specwait): 맨 앞 추측을 w에 맡겨 12분째 기다림, 구성원 ln-x·lane-a
	rec := `{"v":1,"at":` + itoa(now.Unix()) + `,"progress_at":` + itoa(now.Add(-30*time.Second).Unix()) + `,"machine":"m","phase":"spec-wait","phase_since":` +
		itoa(now.Add(-12*time.Minute).Unix()) + `,"members":["ln-x","lane-a"],"waits":[{"machine":"w","addr":"w/t3","role":"추측","at":0}]}`

	// ① 큐에 있고 도는 추측 묶음의 구성원 → 차례·묶음 검증 중, lane-long 없음
	sqMA(t, "alive", "2", "ln-x,lane-a", rec, "-")
	if st, q, long := sqLane(t); st != "제출됨(병합기 줄 2번째 · 묶음 검증 중 w 12분째)" || !q || long {
		t.Errorf("① 묶음 구성원: %q queued=%v long=%v", st, q, long)
	}
	// 큐에만 있음(묶음 아님 — 병합기가 쉼) → 차례, lane-long 없음
	sqMA(t, "alive", "2", "ln-x,lane-a", `{"v":1,"phase":"idle"}`, "-")
	if st, q, long := sqLane(t); st != "제출됨(병합기 줄 2번째)" || !q || long {
		t.Errorf("큐에만 있음: %q queued=%v long=%v", st, q, long)
	}
	// ③ 큐에도 없고 결과도 없음(잃어버림) → lane-long
	sqMA(t, "alive", "1", "ln-x")
	if st, q, long := sqLane(t); st != "제출됨(줄에서 빠짐 — 결과 기다림)" || q || !long {
		t.Errorf("③ 잃어버림: %q queued=%v long=%v", st, q, long)
	}
	// ④ 큐 조회 실패 → 줄 모름(지금 그대로), lane-long
	sqMA(t, "unknown", "?", "?")
	if st, q, long := sqLane(t); st != "제출됨(병합기 줄 모름)" || q || !long {
		t.Errorf("④ 큐 못 읽음: %q queued=%v long=%v", st, q, long)
	}
	// 옛 기록(11칸 — 레인 칸 없음)도 모름
	n := itoa(now.Unix())
	sqWr(t, DATA+"/merger-alive.tsv", strings.Join([]string{"app", "m", "alive", n, "t9", "2", "-", n, "-", "-", "ln-x"}, "\t")+"\n")
	if st, _, _ := sqLane(t); st != "제출됨(병합기 줄 모름)" {
		t.Errorf("옛 기록: %q", st)
	}
	// ⑤ 병합기 죽음 → 차례는 보이되 정상 기다림이 아님(lane-long), 병합기 항목 그대로
	sqMA(t, "gone", "2", "ln-x,lane-a")
	st, q, long := sqLane(t)
	merger := false
	for _, it := range adviseNow(now).Items {
		merger = merger || it.Key == "merger:app"
	}
	if st != "제출됨(병합기 줄 2번째)" || q || !long || !merger {
		t.Errorf("⑤ 병합기 죽음: %q queued=%v long=%v merger=%v", st, q, long, merger)
	}
}

// ② 빠졌다가 다시 선 제출: 새 초로 줄 끝에 선 표식(requeue rqName 꼴)이 차례대로 기록되고 다시 읽혀 차례가 보인다
func TestMaQueueRequeuedOrder(t *testing.T) {
	old := maSnap
	defer func() { maSnap = old }()
	a := "abc" + strings.Repeat("0", 37)
	maSnap = mqSnap{p: ",app", s: []Row{{"app", "refs/merge-queue/1795509999-lane-a", a}, {"app", "refs/merge-queue/1795500100-ln-x", a},
		{"app", "refs/merge-queue/1795500200-ln-y", a}, {"app", "refs/herdr/merger/box", a}}}
	lanes, _, _, ok := maQueue("app")
	if !ok || strings.Join(lanes, ",") != "ln-x,ln-y,lane-a" {
		t.Fatalf("줄 차례: %v %v", lanes, ok)
	}
	x := maRow{P: "app", M: "box", St: "alive", Addr: "t7", K: "3", Lanes: lanes, LanesOK: true}
	DATA = t.TempDir()
	sqWr(t, maFile(), x.line()+"\n")
	r, ok := maLoad()["app"]
	pos := func(x maRow, ok bool, l string) string { p, k := maPos(x, ok, l); return fmt.Sprint(p, k) }
	if !ok || pos(r, true, "lane-a") != "3 true" || pos(r, true, "ln-x") != "1 true" || pos(r, true, "nope") != "0 true" || pos(r, false, "lane-a") != "0 false" {
		t.Errorf("다시 읽은 차례: %+v", r)
	}
	// 빈 줄은 「-」로 남고 줄에 없음, 못 읽음은 「?」로 남고 모름
	for _, c := range []struct {
		x    maRow
		want string
	}{{maRow{P: "app", K: "0", LanesOK: true}, "0 true"}, {maRow{P: "app", K: "?"}, "0 false"}} {
		sqWr(t, maFile(), c.x.line()+"\n")
		if got := pos(maLoad()["app"], true, "lane-a"); got != c.want {
			t.Errorf("%q: %s, want %s", c.x.line(), got, c.want)
		}
	}
}
