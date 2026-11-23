//go:build !windows

package proc

// gofix5-fill(Go 전환 재감사 바퀴 4): fill/R-4M02 · base/R-4M04 · hooks/R-4M02 · ops/R-4M01 — 다른 도구가 남긴 keep 기록(우두머리 삶)과
// 「부모 생존 모름」 기록은 그 도구만 센다. 옛 시험은 keep 갈래를 안 봐 orphan.go의 o.mine()을 지워도 통과했다.

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
	"testing"
)

func TestGofix5OtherToolKeepRecord(t *testing.T) {
	d := t.TempDir() + "/rec"
	t.Setenv("HERDR_PROC_DIR", d)
	old := Who
	defer func() { Who = old }()
	c, bg := startGroup(t, "wait")
	pg := c.Process.Pid
	defer func() { syscall.Kill(-pg, syscall.SIGKILL); c.Wait() }()
	st, err := StartTime(pg)
	if err != nil {
		t.Fatal(err)
	}
	p := putRecLine(t, d, pg, st, keepMark, "herdr-sync")
	Who = "hc-fill"
	var got []int
	for i := 0; i < 3; i++ {
		got = append(got, Orphans())
	}
	if fmt.Sprint(got) != "[0 0 0]" || !exists(p) || !Alive(bg) {
		t.Fatalf("다른 도구의 살아 있는 keep 기록: Orphans 세 번=%v 기록=%v, 기대 [0 0 0]·그대로", got, exists(p))
	}
	Who = "herdr-sync"
	if n := Orphans(); n != 1 || !exists(p) {
		t.Fatalf("제 도구의 keep 기록은 세야: Orphans=%d 기록=%v", n, exists(p))
	}
}

// 부모가 살아 있는데 시작 시각(ps)을 모름: 그 기록을 남긴 도구만 센다
func TestGofix5ParentUnsure(t *testing.T) {
	d := t.TempDir()
	rd := d + "/rec"
	t.Setenv("HERDR_PROC_DIR", rd)
	old, oldPs := Who, psPath
	defer func() { Who, psPath = old, oldPs }()
	par, _ := startGroup(t, "wait")
	ppid := par.Process.Pid
	defer func() { syscall.Kill(-ppid, syscall.SIGKILL); par.Wait() }()
	pg, st, _ := leaderGone(t)
	psPath = d + "/ps"
	if err := os.WriteFile(psPath, []byte("#!/bin/sh\ncase \" $* \" in *\" "+strconv.Itoa(ppid)+" \"*) exit 1 ;; esac\nexec /bin/ps \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rd, 0o700); err != nil {
		t.Fatal(err)
	}
	p := fmt.Sprintf("%s/%d", rd, pg)
	if err := os.WriteFile(p, []byte(fmt.Sprintf("%d\t%s\t%d\tx\t-\therdr-sync\n", pg, st, ppid)), 0o600); err != nil {
		t.Fatal(err)
	}
	Who = "hc-fill"
	if n := Orphans(); n != 0 || !exists(p) {
		t.Fatalf("다른 도구의 부모 생존 모름 기록: Orphans=%d 기록=%v, 기대 0·그대로", n, exists(p))
	}
	Who = "herdr-sync"
	if n := Orphans(); n != 1 || !exists(p) {
		t.Fatalf("제 도구의 부모 생존 모름 기록은 세야(모름): Orphans=%d 기록=%v", n, exists(p))
	}
}
