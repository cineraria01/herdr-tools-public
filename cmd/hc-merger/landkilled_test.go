//go:build !windows

package main

// 끊긴 워커 착지(2026-11-26 ht-land-run-killed): mini-m4 착지 터미널이 hc kill꼴(무리에 HUP 다음 TERM)로 끊겨 결과 파일도 없이
// 사라졌고(병합기는 15분 넘게 기다림), 그 실행의 defer가 못 돌아 남은 착지 작업 트리가 같은 레인 가지를 쥐어 다음 실행이
// 「cannot force update the branch … used by worktree」 → nostart 70으로 시작도 못 했다.

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// ⑪ 끊긴 앞 실행이 같은 레인 가지를 쥔 착지 작업 트리를 남겨도 다음 실행은 착지한다(그 기계를 죽음으로 세지 않는다)
func TestLandOnStaleWorktreeOfKilledRun(t *testing.T) {
	s, runs := landFx(t)
	s.sub(1000000001, "a", false)
	s.git(s.dir+"/v1", "worktree", "add", "-q", "-f", "-B", "hc-merger-land/a", s.dir+"/v1.land-wt/ee7942f3068f", "HEAD")
	s.round(1)
	if r := runs(); len(r) != 1 || r[0] != "v1" || !s.onMain("a.txt") {
		t.Fatalf("⑪ 남은 작업 트리가 있어도 v1에서 착지: 실행 %v\n%s", r, jdump())
	}
}

// ⑫ 감싸개가 hc kill(무리에 HUP·TERM, 약 1초 뒤 KILL — hc 0.1.22 실측, 감사 atp2sd0 중간 1)을 받아도 결과 파일에 끊김을
// 남긴다 — 착지(자식)가 신호 뒤 거두느라 1초를 넘겨도. 병합기는 터미널이 사라졌는지 읽기에 기대지 않고 다음 결과 읽기에 끊김으로
// 본다. 착지가 결과를 이미 적었으면 덮지 않는다
func TestLandWrapKilled(t *testing.T) {
	for _, pre := range []string{"", "0"} {
		d := t.TempDir()
		res := d + "/w/r.rc"
		body := `trap 'sleep 2; exit 1' HUP TERM; sleep 30 & wait $!` // 신호 뒤 2초 걸려 끝나는 착지
		if pre != "" {
			body = `echo ` + pre + ` >"$HC_MERGER_LAND_RESULT"; ` + body
		}
		c := exec.Command("sh", "-c", landWrap, "hc-merger-land", "sh", "-c", body)
		c.Env = append(os.Environ(), "HC_MERGER_LAND_DIR="+d+"/w", "HC_MERGER_LAND_RESULT="+res)
		c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(300 * time.Millisecond)
		syscall.Kill(-c.Process.Pid, syscall.SIGHUP)
		syscall.Kill(-c.Process.Pid, syscall.SIGTERM)
		time.Sleep(time.Second)
		syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
		done := make(chan error, 1)
		go func() { done <- c.Wait() }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
			t.Fatal("⑫ 감싸개가 끝나지 않음")
		}
		b, _ := os.ReadFile(res)
		rc, lost := landParse(strings.TrimSpace(string(b)))
		if pre == "" && !strings.Contains(lost, "끊김") || pre != "" && (lost != "" || rc != 0) {
			t.Fatalf("⑫ 앞 결과 %q → 파일 %q · %d %q", pre, b, rc, lost)
		}
	}
}
