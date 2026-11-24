package main

// advise-merge-stall-age(2026-11-24): 병합 줄 칸의 재료 — 가장 오래된 제출의 레인(병합기 살아 있음 기록 11칸)과 이 기계 병합기가 지금
// 착지 중인지(클론의 child 기록 + 착지 로그 이름). 고치기 전 코드에는 maQueue가 레인을 돌려주지 않고 stateLanding이 없어 컴파일부터 실패한다.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/proc"
)

func TestMaQueueOldestLane(t *testing.T) {
	old := maSnap
	defer func() { maSnap = old }()
	a := "abc" + strings.Repeat("0", 37) // 가짜 SHA는 실행 중에(추적 파일 비밀 꼴 검사)
	maSnap = mqSnap{p: ",app", s: []Row{{"app", "refs/merge-queue/1795500200-ln-b", a}, {"app", "refs/merge-queue/1795500100-ln-a", a},
		{"app", "refs/herdr/merger/box", a}}}
	lanes, oldest, ln, ok := maQueue("app")
	if !ok || len(lanes) != 2 || oldest != 1795500100 || ln != "ln-a" {
		t.Fatalf("maQueue: %v %d %q %v", lanes, oldest, ln, ok)
	}
	// 기록 한 줄에 11칸으로 남고 다시 읽힌다(옛 10칸 기록은 빈 값)
	x := maRow{P: "app", M: "box", St: "alive", Addr: "t7", K: "2", Oldest: oldest, OldLane: ln}
	r := Row(lsSplit(x.line(), maOldLane))
	if r.F(maOldLane) != "ln-a" || dash0(Row(lsSplit("app\tbox\talive\t1\tt7\t1\t5\t6\t-\t-", maOldLane)).F(maOldLane)) != "" {
		t.Errorf("11칸: %q", x.line())
	}
}

func TestStateLanding(t *testing.T) {
	d := t.TempDir()
	clone, logs := filepath.Join(d, "clone"), filepath.Join(d, "logs")
	if out, err := exec.Command("git", "init", "-q", clone).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(clone, ".git/hc-merger"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	// child 기록 없음 — 착지 중 아님
	if _, _, ok := stateLanding(clone, logs); ok {
		t.Fatal("기록 없는데 착지 중")
	}
	st, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Skip("시작 시각을 못 읽음:", err)
	}
	rec := filepath.Join(clone, ".git/hc-merger/child")
	write := func(p, s string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(rec, fmt.Sprintf("%d\t%s\n", os.Getpid(), st))
	stamp := func(t0 time.Time) string { return t0.In(time.Local).Format("20060102-150405") }
	now := time.Now().Truncate(time.Second)
	// 착지 자식(여기서는 이 시험 프로세스)의 부모 = 병합기 pid. 로그 이름 끝 pid가 그것인 것만 이 병합기의 착지 로그
	mp, other := fmt.Sprint(os.Getppid()), fmt.Sprint(os.Getppid()+1)
	// 감사 atp07wf 중간 1의 꼴: 지금 착지는 대기열 줄 없는 레인(inbox-x, 2분 전), 같은 병합기의 지난 착지(ln-a, 3시간 전),
	// 같은 기계 다른 병합기의 더 늦은 착지(other, 1분 전) — 지난 착지·남의 착지를 고르지 않는다
	for _, n := range []string{"ln-a-" + stamp(now.Add(-3*time.Hour)) + "-" + mp + ".log", "inbox-x-" + stamp(now.Add(-2*time.Minute)) + "-" + mp + ".log",
		"ln-b-prescan-" + stamp(now.Add(-time.Minute)) + "-" + mp + ".log", "ln-c-resolve-" + stamp(now.Add(-time.Minute)) + "-" + mp + ".log",
		"pre-" + now.Format("20060102") + ".log", "other-" + stamp(now.Add(-time.Minute)) + "-" + other + ".log"} {
		write(filepath.Join(logs, n), "")
	}
	ln, at, ok := stateLanding(clone, logs)
	if !ok || ln != "inbox-x" || !at.Equal(now.Add(-2*time.Minute)) {
		t.Fatalf("착지 중: %q %v %v", ln, at, ok)
	}
	// 착지 로그가 없으면 레인 모름·기록 시각
	if ln, at, ok := stateLanding(clone, filepath.Join(d, "없음")); !ok || ln != "모름" || at.IsZero() {
		t.Errorf("로그 없음: %q %v %v", ln, at, ok)
	}
	// 이 병합기 pid의 로그가 없으면(남의 로그만) 레인 모름 — 남의 로그를 고르지 않는다
	logs2 := filepath.Join(d, "logs2")
	if err := os.MkdirAll(logs2, 0o755); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(logs2, "other-"+stamp(now.Add(-time.Minute))+"-"+other+".log"), "")
	if ln, _, ok := stateLanding(clone, logs2); !ok || ln != "모름" {
		t.Errorf("남의 로그만: %q %v", ln, ok)
	}
	// 다른 시작 시각(pid 재사용) — 착지 중 아님
	write(rec, fmt.Sprintf("%d\t%s\n", os.Getpid(), "Thu Jan  1 00:00:00 1970"))
	if _, _, ok := stateLanding(clone, logs); ok {
		t.Error("pid 재사용인데 착지 중")
	}
	// 꼴이 틀린 기록 — 착지 중이라고 하지 않는다(advise는 그때 안 움직임을 본다)
	write(rec, "x\n")
	if _, _, ok := stateLanding(clone, logs); ok {
		t.Error("꼴 틀린 기록인데 착지 중")
	}
}
