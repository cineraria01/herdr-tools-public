package main

// 2026-11-21 Go 전환 전체 감사(base) hc-sweep 지적 가운데 단위로 보는 것. 이름 앞 [ID]는 보고서 지적 ID

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// [E13] lsof 출력이 온전하지 않으면(이 프로세스 기록 없음·필드 꼴 아님) 열린 경로를 모름(nil) — 0개로 읽고 지우지 않는다
func TestOpenPathsIncomplete(t *testing.T) {
	me := strconv.Itoa(os.Getpid())
	if opq(fakeRun(0, "p123\n", "", nil)) != nil {
		t.Fatal("pid 줄만 있는 출력을 받아들임")
	}
	if opq(fakeRun(0, "p1\nfcwd\nn/a\nlsof: WARNING x\n", "", nil)) != nil {
		t.Fatal("필드 꼴이 아닌 줄을 받아들임")
	}
	m := opq(fakeRun(0, "p1\nfcwd\nn/a/b\np"+me+"\nfcwd\nn/c\n", "", nil))
	if m == nil || !m["/a/b"] || !m["/c"] {
		t.Fatalf("온전한 출력 %v", m)
	}
}

// [B22] ps가 0으로 끝나도 목록이 비면 go가 도는지 모른다 — 캐시를 건드리지 않는다
func TestGoBusyEmpty(t *testing.T) {
	if !goBusy(fakeRun(0, "", "", nil)) || !goBusy(fakeRun(0, "\n\n", "", nil)) {
		t.Fatal("빈 ps를 go 없음으로 읽음")
	}
	if goBusy(fakeRun(0, "/bin/zsh\nps\n", "", nil)) {
		t.Fatal("go 없는 목록")
	}
	t.Setenv("HC_SWEEP_GO_COMM", "hcsweep-no-such-comm")
	if goBusy(run) {
		r := run(30*time.Second, nil, "ps", "-x", "-o", "comm=")
		t.Fatalf("진짜 ps에서 없는 이름을 도는 것으로: rc %d err %v 줄 %d %q", r.rc, r.err, strings.Count(r.out, "\n"), r.errs)
	}
}

// [E06·B07] 알림은 대기 표식 → 보고 → 「보냄」. 보고가 실패하거나 중간에 끊겨 대기 표식이 남으면 다음 바퀴가 같은 키로 다시 보낸다
func TestDiskAlertPending(t *testing.T) {
	d := t.TempDir()
	state := filepath.Join(d, "disk-low")
	var keys []string
	rc := 1
	rep := func(_ time.Duration, _ []string, name string, args ...string) result {
		if name == "du" {
			return result{out: "1\tx\n"}
		}
		keys = append(keys, args[len(args)-1])
		return result{rc: rc}
	}
	lane := filepath.Join(d, "hc-lane")
	os.WriteFile(lane, []byte("#!/bin/sh\n"), 0o755)
	if diskAlert(1, 1, 10, state, d, false, lane, rep) != "" {
		t.Fatal("실패한 보고를 보냄으로")
	}
	if b, _ := os.ReadFile(state); !strings.HasPrefix(string(b), pendingMark+"\t") {
		t.Fatalf("대기 표식이 아님: %q", b)
	}
	rc = 0
	if diskAlert(1, 1, 10, state, d, false, lane, rep) == "" {
		t.Fatal("대기 표식인데 다시 보내지 않음")
	}
	if len(keys) != 2 || keys[0] != keys[1] || !strings.HasPrefix(keys[0], "hc-disk-low:") || strings.Trim(keys[0],
		"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_.:/-") != "" { // hc-lane report --key 글자
		t.Fatalf("같은 키로 다시 보내야 함: %v", keys)
	}
	if b, _ := os.ReadFile(state); strings.HasPrefix(string(b), pendingMark) {
		t.Fatalf("보낸 뒤에도 대기: %q", b)
	}
	if diskAlert(1, 1, 10, state, d, false, lane, rep) != "" || len(keys) != 2 {
		t.Fatal("보낸 뒤 또 보냄")
	}
	// 셸 판이 쓴 표식(시각<탭>글)은 보낸 것으로 본다
	os.WriteFile(state, []byte("2026-01-01T00:00:00+0900\tx\n"), 0o644)
	if diskAlert(1, 1, 10, state, d, false, lane, rep) != "" || len(keys) != 2 {
		t.Fatal("셸 판 표식을 다시 보냄")
	}
	// 회복 뒤 다시 부족이면 새 키 — 같은 초 안이라도(키가 초 단위면 hc-lane이 같은 사건으로 걸러 두 번째 알림이 사라졌다.
	// 2026-11-22 병합기 TestHcSweepDisk 「회복 뒤 다시 부족이면 다시 알림」 흔들림, verify-parallel이 함께 고침)
	diskAlert(100*gb, 1, 10, state, d, false, lane, rep) // 회복: 표식 지움
	if diskAlert(1, 1, 10, state, d, false, lane, rep) == "" || len(keys) != 3 || keys[2] == keys[1] {
		t.Fatalf("회복 뒤 다시 부족: 새 키로 한 번 더 보내야 함: %v", keys)
	}
}

// [B05] hc가 not_found를 답했을 때만 「없음」 — 그 밖의 실패는 모름
func TestNotFoundIn(t *testing.T) {
	if !notFoundIn(`{"error":{"kind":"not_found","message":"x"}}`) || !notFoundIn("warn\n{\"kind\":\"not_found\"}\n") {
		t.Fatal("not_found를 못 알아봄")
	}
	if notFoundIn(`{"error":{"kind":"io","message":"connection reset"}}`) || notFoundIn("") {
		t.Fatal("다른 실패를 not_found로")
	}
}

// [E04] 고아를 끄기 전 다시 본 나이가 처음 본 뒤 흐른 만큼 늘지 않았으면(그 pid를 다른 실행이 받음) 끄지 않는다
func TestKillTreePidReuse(t *testing.T) {
	c := exec.Command("sleep", "30") // 이 시험이 띄운 자식만 대상으로(잘못 끄더라도 이것)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Process.Kill(); c.Wait() })
	kid := c.Process.Pid
	procs, err := processes(run)
	if err != nil {
		t.Fatal("ps를 못 읽음:", err)
	}
	pi, ok := procs[kid]
	if !ok {
		t.Fatal("ps에 자식이 없음")
	}
	pi.age += 3600 // 한 시간 전에 본 「같은 pid」 — 지금 이 자식은 그보다 젊다 = 다른 실행
	left, did, err := killTree([]int{kid}, map[int]procInfo{kid: pi}, time.Now(), 0)
	if err != nil || did || left != 0 || !alive(kid) {
		t.Fatalf("다른 실행을 끄려 함: %d %v %v", left, did, err)
	}
	// 같은 실행(나이가 맞음)이면 끈다
	procs, _ = processes(run)
	_, did, err = killTree([]int{kid}, map[int]procInfo{kid: procs[kid]}, time.Now(), 2*time.Second)
	if err != nil || !did {
		t.Fatalf("같은 실행을 안 끔: %v %v", did, err)
	}
}
