//go:build !windows

package main

// 2026-11-22 Go 전환 재감사(아스트라 1바퀴) base 지적 가운데 프로세스를 띄워 보는 hc-sweep 실패 시험(gofix2-base). 이름 앞 [ID]는 지적 ID.
// 신호는 이 시험이 띄운 자식·자손에만(pid와 시작 시각을 처음 본 그대로 견준 뒤) 보낸다

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"herdr-tools/internal/proc"
)

// 시험 바이너리를 대역으로 다시 부를 때(O2_HELPER) — 시험보다 먼저 init에서 돌고 끝낸다(TestMain을 쓰지 않아 다른 시험 파일과 겹치지 않는다)
func init() {
	mode := os.Getenv("O2_HELPER")
	if mode == "" {
		return
	}
	switch mode {
	case "idle": // 고아 대역 — 스스로 2분 뒤 끝난다
		time.Sleep(2 * time.Minute)
	case "trap": // 받은 TERM·USR1을 O2_SIGF에 적고 끝나지 않는다(60초 뒤 스스로 끝)
		ch := make(chan os.Signal, 4)
		signal.Notify(ch, syscall.SIGTERM, syscall.SIGUSR1)
		os.WriteFile(os.Getenv("O2_PIDF"), []byte(strconv.Itoa(os.Getpid())), 0o644)
		end := time.After(60 * time.Second)
		for {
			select {
			case s := <-ch:
				name := "TERM"
				if s == syscall.SIGUSR1 {
					name = "USR1"
				}
				f, _ := os.OpenFile(os.Getenv("O2_SIGF"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
				f.WriteString(name + "\n")
				f.Close()
			case <-end:
				os.Exit(0)
			}
		}
	case "sweeper": // 쓸기 대역 — run으로 자식(pid를 O2_PIDF에 적고 오래 자는)을 띄워 기다린다. 시험이 이 프로세스만 KILL한다
		run(60*time.Second, nil, "sh", "-c", `echo $$ > "$O2_PIDF"; exec sleep 60`)
	case "next": // 다음 회차 대역 — 명령 하나를 run으로 부른다
		run(10*time.Second, nil, "true")
	}
	os.Exit(0)
}

// o2Watch: 이 시험의 자손 pid를 처음 본 시작 시각과 함께 적어 두고, 끝날 때 살아 있으면 그것일 때만 KILL
func o2Watch(t *testing.T, pid int) {
	t.Helper()
	st, err := proc.StartTime(pid)
	if err != nil {
		if alive(pid) {
			t.Fatalf("자손 %d 시작 시각을 못 읽음: %v", pid, err)
		}
		return // 이미 끝남 — 거둘 것 없음
	}
	t.Cleanup(func() {
		if now, err := proc.StartTime(pid); err == nil && now == st {
			syscall.Kill(pid, syscall.SIGKILL)
		}
	})
}

// o2ReadPid: 준비 파일에 pid가 적힐 때까지(넉넉한 기한)
func o2ReadPid(t *testing.T, f string) int {
	t.Helper()
	for end := time.Now().Add(20 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if b, err := os.ReadFile(f); err == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && n > 0 {
				return n
			}
		}
	}
	t.Fatalf("준비 파일 %s에 pid가 안 적힘", f)
	return 0
}

// o2GoneWithin: 기한 안에 끝나면 true
func o2GoneWithin(pid int, d time.Duration) bool {
	for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		if !alive(pid) {
			return true
		}
	}
	return !alive(pid)
}

func o2Helper(t *testing.T, mode string, env ...string) *exec.Cmd {
	t.Helper()
	c := exec.Command(os.Args[0])
	c.Env = append(append(os.Environ(), "O2_HELPER="+mode), env...)
	return c
}

// [R-11·R-32] 명령 상한: 포장처럼 한 번 더 exec한 셸이 띄운 손자도 상한 뒤에 남지 않는다(지금은 직접 자식만 KILL — 손자가 남아 뒤에 파일을 쓴다)
func TestO2RunTimeoutReapsGrandchild(t *testing.T) {
	gpid := filepath.Join(t.TempDir(), "gpid")
	env := append(os.Environ(), "O2_GPID="+gpid)
	r := run(3*time.Second, env, "sh", "-c", `exec sh -c 'sleep 60 & echo $! > "$O2_GPID"; wait'`)
	if r.err == nil {
		t.Fatalf("시간 초과가 모름(err)이 아님: %+v", r)
	}
	g := o2ReadPid(t, gpid)
	o2Watch(t, g)
	if !o2GoneWithin(g, 10*time.Second) {
		t.Fatalf("상한에 걸린 명령의 손자 %d가 남음", g)
	}
}

// [R-11·R-32] 직접 자식은 0으로 끝났는데 손자가 출력 관을 쥔 채 남으면: 결과는 모름이고, 그 손자도 거둔다
func TestO2RunPipeHolderReaped(t *testing.T) {
	gpid := filepath.Join(t.TempDir(), "gpid")
	env := append(os.Environ(), "O2_GPID="+gpid)
	r := run(30*time.Second, env, "sh", "-c", `exec sh -c 'sleep 60 & echo $! > "$O2_GPID"'`)
	g := o2ReadPid(t, gpid)
	o2Watch(t, g)
	if r.err == nil {
		t.Fatalf("출력 관이 안 닫혔는데 성공으로 읽음: %+v", r)
	}
	if !o2GoneWithin(g, 10*time.Second) {
		t.Fatalf("출력 관을 쥔 손자 %d가 남음", g)
	}
}

// [R-11·R-32] 쓸기 부모만 KILL됐을 때(launchd·사람이 pid 하나만 끔): 다음 회차가 pid·시작 시각을 견줘 남은 자기 자식을 거둔다
func TestO2ParentKillNextRoundReaps(t *testing.T) {
	d := t.TempDir()
	pidf := filepath.Join(d, "kid")
	recs := filepath.Join(d, "proc") // 고아 기록 자리(이 시험 것만)
	sw := o2Helper(t, "sweeper", "O2_PIDF="+pidf, "HERDR_PROC_DIR="+recs)
	if err := sw.Start(); err != nil {
		t.Fatal(err)
	}
	kid := o2ReadPid(t, pidf)
	o2Watch(t, kid)
	sw.Process.Kill() // 부모 pid 하나만 KILL(무리째가 아님)
	sw.Wait()
	next := o2Helper(t, "next", "HERDR_PROC_DIR="+recs)
	if out, err := next.CombinedOutput(); err != nil {
		t.Fatalf("다음 회차 대역: %v %s", err, out)
	}
	if !o2GoneWithin(kid, 10*time.Second) {
		t.Fatalf("부모만 죽은 쓸기의 자식 %d가 다음 회차 뒤에도 남음", kid)
	}
}

// [R-19] 루트가 처음 본 그 실행이 아니면(소유가 바뀜) 그 트리 전체를 보류한다 — 지금은 루트만 거르고 자손에는 TERM을 보낸다
func TestO2KillTreeRootChanged(t *testing.T) {
	d := t.TempDir()
	pidf, sigf := filepath.Join(d, "kid"), filepath.Join(d, "sigs")
	root := exec.Command("sh", "-c", `"$0" & wait`, os.Args[0])
	root.Env = append(os.Environ(), "O2_HELPER=trap", "O2_PIDF="+pidf, "O2_SIGF="+sigf)
	if err := root.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Process.Kill(); root.Wait() })
	kid := o2ReadPid(t, pidf)
	o2Watch(t, kid)
	procs, err := processes(run)
	if err != nil {
		t.Fatal("ps:", err)
	}
	rp := root.Process.Pid
	if procs[kid].ppid != rp {
		t.Fatalf("자손 %d의 부모가 루트 %d가 아님: %+v", kid, rp, procs[kid])
	}
	rootInfo := procs[rp]
	rootInfo.age += 3600 // 한 시간 전에 본 「루트」 — 지금 그 pid는 다른 실행이다
	_, did, err := killTree([]int{rp}, map[int]procInfo{rp: rootInfo, kid: procs[kid]}, time.Now(), 0)
	if err != nil || did {
		t.Fatalf("루트가 다른 실행인데 신호: did %v err %v", did, err)
	}
	// 우리 USR1이 적힌 뒤 보면, 그보다 먼저 보낸 TERM은 이미 적혀 있다(신호 번호가 낮고 먼저 왔다)
	syscall.Kill(kid, syscall.SIGUSR1)
	var got string
	for end := time.Now().Add(20 * time.Second); time.Now().Before(end) && !strings.Contains(got, "USR1") && alive(kid); time.Sleep(20 * time.Millisecond) {
		b, _ := os.ReadFile(sigf)
		got = string(b)
	}
	if !alive(kid) {
		t.Fatalf("루트 소유가 바뀌었는데 자손 %d를 끔(TERM 뒤 KILL): 받은 신호 %q", kid, got)
	}
	if !strings.Contains(got, "USR1") {
		t.Fatalf("자손이 신호를 못 적음: %q", got)
	}
	if strings.Contains(got, "TERM") {
		t.Fatalf("루트 소유가 바뀌었는데 자손 %d에 TERM을 보냄: %q", kid, got)
	}
}

// [R-16] 이 기계 터미널의 상태(state)를 모르면 그 작업 트리 아래 고아를 「쓰는 터미널 없음」으로 끄려 하지 않는다(보류).
// 대조: 같은 터미널이 live면 후보 0
func TestO2OrphansUnknownState(t *testing.T) {
	d := t.TempDir()
	wt := filepath.Join(d, "repo.wt", "lane")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	oldBin := binDir
	binDir = filepath.Join(d, "nobin") // 설정 도구 없음 = 기본값(끄기 off)
	t.Cleanup(func() { binDir = oldBin })
	// 고아 대역: 명령 줄·cwd가 그 작업 트리 아래이고 부모가 1
	pidf := filepath.Join(d, "orphan")
	sp := exec.Command("sh", "-c", `"$0" "$1" </dev/null >/dev/null 2>&1 & echo $! > "$2"`, os.Args[0], filepath.Join(wt, "o2-marker"), pidf)
	sp.Dir = wt
	sp.Env = append(os.Environ(), "O2_HELPER=idle")
	if err := sp.Run(); err != nil {
		t.Fatal(err)
	}
	op := o2ReadPid(t, pidf)
	o2Watch(t, op)
	for end := time.Now().Add(20 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if ps, err := processes(run); err == nil && ps[op].ppid == 1 {
			break
		}
		if time.Now().After(end) {
			t.Fatalf("고아 대역 %d의 부모가 1이 안 됨", op)
		}
	}
	term := func(extra map[string]any) []any {
		m := map[string]any{"address": "o2t1", "cwd": wt, "name": "lane"}
		for k, v := range extra {
			m[k] = v
		}
		return []any{m}
	}
	if n, err := sweepOrphans(term(map[string]any{"state": "live"}), true, 0, d); err != nil || n != 0 {
		t.Fatalf("대조(live 터미널): 후보 %d err %v", n, err)
	}
	for name, extra := range map[string]map[string]any{"state 없음": {}, "state null": {"state": nil}, "state 숫자": {"state": 1.0}} {
		if n, _ := sweepOrphans(term(extra), true, 0, d); n != 0 {
			t.Errorf("%s: 터미널 상태를 모르는데 고아 %d개를 끌 것으로 고름", name, n)
		}
	}
}
