//go:build !windows

package clitest

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// B29: 상한을 넘긴 명령은 그 무리(손자 포함)째 꺼지고 RC 124·Timeout으로 돌아온다 — 시험 마감까지 매달리지 않는다.
// 손자는 1초 뒤 늦은 표식을 쓴다 — 시작 표식을 먼저 본 뒤 늦은 표식이 끝까지 없어야(무리째 꺼짐). 바깥 마감 안 회수는 contract_test.go
func TestRunTimeoutKillsGroup(t *testing.T) {
	d := TempDir(t)
	S, M := filepath.Join(d, "start"), filepath.Join(d, "late")
	t0 := time.Now()
	r := Cmd{Max: 300 * time.Millisecond}.Run("sh", "-c", `(sleep 1; : >"$2") & : >"$1"; wait`, "sh", S, M)
	if !r.Timeout || r.RC != 124 || !strings.Contains(r.Err, "시간 넘김") {
		t.Fatalf("시간 넘김이어야: %+v", r)
	}
	if d := time.Since(t0); d > 10*time.Second {
		t.Fatalf("상한 뒤 곧 돌아와야: %v", d)
	}
	WaitFile(t, S, time.Second)
	time.Sleep(1500 * time.Millisecond)
	if !Absent(t, M) {
		t.Fatal("손자도 꺼져야 — 늦은 표식이 생겼다")
	}
}

// 끝난 뒤 출력 관을 쥔 손자: 우두머리는 0이어도 상한까지 관이 안 닫히면 124·무리째 끈다
func TestRunPipeHolder(t *testing.T) {
	d := TempDir(t)
	S, M := filepath.Join(d, "start"), filepath.Join(d, "late")
	r := Cmd{Max: 500 * time.Millisecond}.Run("sh", "-c", `(: >"$1"; sleep 1; : >"$2") & exit 0`, "sh", S, M)
	if !r.Timeout || r.RC != 124 {
		t.Fatalf("관을 쥔 손자는 시간 넘김이어야: %+v", r)
	}
	WaitFile(t, S, time.Second)
	time.Sleep(1500 * time.Millisecond)
	if !Absent(t, M) {
		t.Fatal("관을 쥔 손자가 살아남았다")
	}
}

func TestRunNormal(t *testing.T) {
	r := Cmd{}.Run("sh", "-c", `echo out; echo err >&2; exit 3`)
	if r.Out != "out" || r.Err != "err\n" || r.RC != 3 || r.Timeout {
		t.Fatalf("%+v", r)
	}
	if r := (Cmd{Stdin: "a\nb\n", Combined: true}).Run("sh", "-c", `cat; echo e >&2; kill -TERM $$`); r.Out != "a\nb\ne" || r.RC != 128+15 {
		t.Fatalf("입력·합친 출력·신호 종료: %+v", r)
	}
	if r := (Cmd{Exact: true, Env: []string{"A=1"}}).Run("/usr/bin/env"); r.Out != "A=1" {
		t.Fatalf("Exact는 Env만: %+v", r)
	}
	if r := (Cmd{}).Run("/없는/명령"); r.RC != 127 {
		t.Fatalf("못 띄우면 127: %+v", r)
	}
}
