package main

import (
	"strings"
	"testing"
)

// 되돌리기(--at, deploy-log-rollback): 소스 판이 at:<커밋>이면 받지 못해도 그 기계에서 짓지 않는다(올린 판만) — 옛 본체 그대로.
// 받으면 기록(bin.tsv)에 at:<커밋>을 적어, 같은 소스 판의 보통 배포가 「이미 이 판」으로 건너뛰지 않는다
func TestInstallShAtNoBuild(t *testing.T) {
	home, art, sum, src := instEnv(t)
	arm := map[string]string{"uname": uname("arm64")}
	out := runSh(t, home, arm, "set", "c0", "at:c0", "arm64", "demo", "file://"+art+".없음", sum)
	if out != "E\tdemo\t받지 못했다 · 되돌리기는 올린 판만 깐다(짓지 않음)\n" || readOr(home+"/.local/bin/demo") != oldBody || len(leftovers(home)) != 0 {
		t.Fatalf("못 받은 되돌리기: %q 본체 %q 남은 것 %v", out, readOr(home+"/.local/bin/demo"), leftovers(home))
	}
	if out := runSh(t, home, arm, "set", "c0", "at:c0", "arm64", "demo", "file://"+art, sum); out != "W\tdemo\n" {
		t.Fatalf("받은 되돌리기: %q", out)
	}
	if rec := readOr(home + "/.local/state/herdr-sync/bin.tsv"); rec != "demo\t"+sum+"\tat:c0\n" {
		t.Fatalf("기록 %q", rec)
	}
	// 보통 배포(같은 기계 소스 판)는 기록의 sha가 본체와 같으니 충돌이 아니고, 소스 판이 달라 다시 깐다(여기선 그 판 없음 → 지음)
	if out := runSh(t, home, arm, "check", "c1", src, "arm64", "demo", "-", "-"); !strings.HasPrefix(out, "D\tdemo\t") {
		t.Fatalf("되돌린 뒤 보통 --check: %q", out)
	}
}

func TestMachineCommit(t *testing.T) {
	log := []logRow{{c12: "aaaaaaaaaaaa", src: "1.2"}, {c12: "bbbbbbbbbbbb", src: "3.4"}}
	for _, c := range []struct {
		rows      [][3]string
		c12, desc string
	}{
		{[][3]string{{"x", "3.4", "같음"}, {"y", "3.4", "같음"}}, "bbbbbbbbbbbb", "bbbbbbbbbbbb"},
		{[][3]string{{"x", "at:cccccccccccc", "같음"}}, "cccccccccccc", "cccccccccccc"},
		{[][3]string{{"x", "1.2", "같음"}, {"y", "3.4", "따로바뀜"}}, "", "섞임 (aaaaaaaaaaaa 도구 1개 · bbbbbbbbbbbb 도구 1개) · 따로 바뀜 y(따로바뀜)"},
		{[][3]string{{"x", "9.9", "같음"}}, "", "모름 (배포 기록 전 판)"},
		{nil, "", "모름 (깐 기록 없음)"},
	} {
		if c12, d := machineCommit(c.rows, log); c12 != c.c12 || d != c.desc {
			t.Errorf("machineCommit(%v) = %q %q, 바람 %q %q", c.rows, c12, d, c.c12, c.desc)
		}
	}
}
