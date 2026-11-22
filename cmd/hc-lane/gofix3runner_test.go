//go:build !windows

package main

// gofix3-runner(Go 전환 재감사 2바퀴) — 자식 실행·소유·마감 지적의 고치기 전 실패 시험. 이름 앞 R-ID는 lane 영역 보고 지적 ID.
// 시간 기대는 넉넉한 기한으로(부하 3~20에서 흔들리지 않게), 자기 자식만 본다(호스트 ps 무관).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/proc"
)

// fakeBin: d에 실행 파일 name(본문 sh)을 두고 PATH 앞에
func fakeBin(t *testing.T, d, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(d, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// absentFor: 표식 m이 until까지 생기지 않나(생기면 거짓)
func absentFor(m string, until time.Time) bool {
	for time.Now().Before(until) {
		if _, err := os.Stat(m); err == nil {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
	_, err := os.Stat(m)
	return err != nil
}

func shortGrace(t *testing.T) {
	old := proc.Grace
	proc.Grace = 500 * time.Millisecond
	t.Cleanup(func() { proc.Grace = old })
}

// R-217: gc의 `sh -c` 안 git도 상한 안에서 — 안쪽 git 4초, HERDR_TIMEOUT_GIT=1이면 ≤ 1초+유예에 「확인 못 함」(2), 안쪽 git도 함께 거둔다
func TestR217GcNestedGitBounded(t *testing.T) {
	shortGrace(t)
	d := t.TempDir()
	m := filepath.Join(d, "inner-done")
	fakeBin(t, d, "git", `for a in "$@"; do
  case $a in merge-base) exit 1;; --merges) exit 0;; --no-merges) sleep 4; : >`+m+`; exit 0;; esac
done
exit 0
`)
	t.Setenv("PATH", d+":/usr/bin:/bin")
	t.Setenv("HERDR_TIMEOUT_GIT", "1")
	oldOn, oldRepo, oldBref := on, repo, bref
	on, repo, bref = "", d, "refs/heads/main"
	t.Cleanup(func() { on, repo, bref = oldOn, oldRepo, oldBref })
	start := time.Now()
	_, rc := gin("abc" + strings.Repeat("0", 37)) // 비밀 꼴 검사에 안 걸리게 실행 중에 만든 가짜 커밋
	el := time.Since(start)
	if rc != 2 {
		t.Errorf("상한에 걸린 조회는 확인 못 함(2)이어야 — 후보 보존: rc %d", rc)
	}
	if el > 3*time.Second {
		t.Errorf("sh -c 안 git의 4초를 기다렸다(상한 1초): %v (lane/R-217)", el)
	}
	if !absentFor(m, start.Add(5500*time.Millisecond)) {
		t.Errorf("상한 뒤 안쪽 git이 살아 표식을 썼다 — 무리째 거두지 않음")
	}
}

// R-218: 원격 시작 뒤 창 이름 붙이기(herdr-app do space.rename)가 3초 걸려도 HERDR_TIMEOUT_REMOTE=1이면 짧은 상한 안에 돌아온다
func TestR218RenameBounded(t *testing.T) {
	shortGrace(t)
	d := t.TempDir()
	m := filepath.Join(d, "renamed")
	fakeBin(t, d, "herdr-app", `case $1 in
  ls) printf '{"spaces":[{"id":"box/sp","name":null}]}\n';;
  do) sleep 3; : >`+m+`;;
esac
`)
	t.Setenv("PATH", d+":/usr/bin:/bin")
	t.Setenv("HERDR_TIMEOUT_REMOTE", "1")
	oldOn, oldHsp, oldAddr := on, hsp, addr
	on, hsp, addr = "box", "sp", "box/t1"
	t.Cleanup(func() { on, hsp, addr = oldOn, oldHsp, oldAddr })
	start := time.Now()
	showRemote()
	if el := time.Since(start); el > 2500*time.Millisecond {
		t.Errorf("이름 붙이기를 상한 없이 기다렸다: %v (lane/R-218)", el)
	}
	if addr != "box/t1" {
		t.Errorf("받은 터미널 주소가 바뀌었다: %q", addr)
	}
	if !absentFor(m, start.Add(4*time.Second)) {
		t.Errorf("상한 뒤 이름 붙이기 자식이 살아 표식을 썼다")
	}
}
