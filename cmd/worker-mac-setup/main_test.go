package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	wm "herdr-tools/internal/workermac"
)

func TestParse(t *testing.T) {
	o, code := parse([]string{"w1", "--slots", "3", "--dry-run"})
	if code != -1 || o.m != "w1" || o.slots != "3" || !o.ss || !o.dry || o.meshOnly {
		t.Errorf("%+v %d", o, code)
	}
	if o, code = parse([]string{"mesh", "--check"}); code != -1 || !o.meshOnly || !o.check || o.m != "" {
		t.Errorf("mesh %+v %d", o, code)
	}
	if o, code = parse([]string{"w1", "--slots"}); code != -1 || o.slots != "" || !o.ss {
		t.Errorf("--slots 값 없음 %+v %d", o, code) // 값 없는 --slots는 setup이 64로 끝낸다(셸 판과 같음)
	}
	if _, code = parse([]string{"--dry-run"}); code != 64 {
		t.Errorf("기계 없음 → %d", code)
	}
	for _, c := range []struct {
		s  string
		ok bool
	}{{"1", true}, {"12", true}, {"00", true}, {"0", false}, {"", false}, {"-1", false}, {"2x", false}} {
		if slotsOK(c.s) != c.ok {
			t.Errorf("slotsOK(%q)", c.s)
		}
	}
}

// 원격 조각: 넷 다 있고, 앞붙임 + head + 조각을 base64로 넘긴 명령을 bash가 그대로 읽는다(값에 따옴표·빈칸이 들어도)
func TestPiecesAndRemoteCmd(t *testing.T) {
	for _, p := range []string{"head", "info", "auth", "cli"} {
		if strings.TrimSpace(piece(p)) == "" {
			t.Errorf("조각 %s 없음", p)
		}
	}
	if !strings.HasPrefix(piece("head"), "set -u; CHECK=$1; D=$2\n") || strings.Contains(piece("info"), "#@") {
		t.Error("조각 나누기")
	}
	pre := "PUB=" + wm.Quote("ssh-ed25519 AAA it's c") + " FROM=" + wm.Quote("1.2.3.4,::1") + ";"
	cmd := remoteCmd(pre+piece("head")+`printf '%s|%s|%s|%s' "$PUB" "$FROM" "$CHECK" "$D"`, "1", "20261121")
	out, err := exec.Command("/bin/sh", "-c", cmd).Output()
	if err != nil {
		t.Fatal(err)
	}
	if want := "ssh-ed25519 AAA it's c|1.2.3.4,::1|1|20261121"; string(out) != want {
		t.Errorf("%q, 바람 %q", out, want)
	}
}

// 개인 단계 붙이기: 8번부터, 이어지는 줄·주석·빈 줄·{M}·CRLF
func TestPersonal(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cl.txt")
	os.WriteFile(p, []byte("# 주석\r\n[이 맥] 예시 {M}:\r\n    ! ssh -t {M} 예시\n\n[그 맥] 둘째"), 0o644)
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	n := personal(p, "w1", 8)
	w.Close()
	os.Stdout = old
	b := make([]byte, 4096)
	k, _ := r.Read(b)
	if want := " 8. [이 맥] 예시 w1:\n    ! ssh -t w1 예시\n 9. [그 맥] 둘째\n"; string(b[:k]) != want || n != 10 {
		t.Errorf("%q n=%d", b[:k], n)
	}
}

// 상한 시간: 원격 조각 실행(rrun)이 멈춰도 돌아오고, mesh는 그 기계를 「접속 안 됨」으로 센다
func TestMeshTimeout(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "ssh"), []byte("#!/bin/sh\nexec sleep 30\n"), 0o755)
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	old := rrunSec
	rrunSec = 1
	defer func() { rrunSec = old }()
	start := time.Now()
	if rc := meshRun(context.Background(), "wa wb", "", true, "20261121"); rc != 1 {
		t.Errorf("rc %d", rc)
	}
	if time.Since(start) > 15*time.Second {
		t.Error("상한이 안 먹음")
	}
	if rc := meshRun(context.Background(), "wa wb", "wz", true, "20261121"); rc != 64 {
		t.Errorf("목록 밖 → %d", rc)
	}
}

// 설정 오류(같은 키 두 번 등)는 setup이 접속 전에 종료 2, --slots인데 worker.slots_file이 없으면 64
func TestSetupConfig(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "herdr-tools-config"), []byte(`#!/bin/bash
[ "$1" = file ] && exit 3
case $2 in worker.trust) [ -n "${BAD-}" ] && { echo "같은 키 두 번" >&2; exit 2; }; exit 3 ;; *) exit 3 ;; esac
`), 0o755)
	os.WriteFile(filepath.Join(d, "ssh"), []byte("#!/bin/sh\necho ssh >>"+d+"/ssh.log; exit 255\n"), 0o755)
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	ctx := context.Background()
	if rc := setup(ctx, &wm.Config{Ctx: ctx}, opts{m: "w1", slots: "3", ss: true}, "", "20261121"); rc != 64 {
		t.Errorf("--slots·설정 없음 → %d", rc)
	}
	t.Setenv("BAD", "1")
	if rc := setup(ctx, &wm.Config{Ctx: ctx}, opts{m: "w1", slots: "1"}, "", "20261121"); rc != 2 {
		t.Errorf("설정 오류 → %d", rc)
	}
	if _, err := os.Stat(filepath.Join(d, "ssh.log")); err == nil {
		t.Error("설정 오류인데 접속했다")
	}
}
