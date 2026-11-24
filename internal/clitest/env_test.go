//go:build !windows

package clitest

import (
	"os"
	"strings"
	"testing"
)

// test-env-allowlist ①: Env가 만든 환경에는 허용 목록·keep·임시 HOME·GIT_CEILING_DIRECTORIES 말고는 없고, 레인 값은 하나도 안 샌다
func TestEnvAllowOnly(t *testing.T) {
	Poison(t)
	t.Setenv("HC_FILL_EXAMPLE", "/x/hc-fill.tsv")
	T := TempDir(t)
	e := Env(T, "HC_FILL_EXAMPLE")
	ok := map[string]bool{"HOME": true, "GIT_CEILING_DIRECTORIES": true, "HC_FILL_EXAMPLE": true}
	for _, k := range EnvAllow {
		ok[k] = true
	}
	for k := range e {
		if !ok[k] {
			t.Errorf("허용 목록 밖 변수: %s", k)
		}
	}
	if l := Leaked(e); len(l) > 0 {
		t.Errorf("샌 레인 값: %v", l)
	}
	if e["HOME"] != T || !strings.HasPrefix(e["GIT_CEILING_DIRECTORIES"], T+":") || e["HC_FILL_EXAMPLE"] != "/x/hc-fill.tsv" {
		t.Errorf("임시 HOME·천장·keep: %v", e)
	}
}

// test-env-allowlist ③: 검증 실행기가 조각에 넘기는 메모리 디스크 TMPDIR과 부팅 볼륨 임시 폴더는 Env·Isolate 뒤에도 그대로다
func TestEnvKeepsVerifyTmp(t *testing.T) {
	T := TempDir(t) // TMPDIR을 바꾸기 앞에서 — 아래 TMPDIR은 있는 폴더가 아니다
	t.Setenv("TMPDIR", "/Volumes/herdr-verify-ram-1/")
	t.Setenv("HERDR_VERIFY_BOOT_TMPDIR", "/var/folders/x/T/")
	e := Env(T)
	if e["TMPDIR"] != "/Volumes/herdr-verify-ram-1/" || e["HERDR_VERIFY_BOOT_TMPDIR"] != "/var/folders/x/T/" {
		t.Fatalf("Env가 검증 임시 폴더를 잃었다: %q %q", e["TMPDIR"], e["HERDR_VERIFY_BOOT_TMPDIR"])
	}
	Isolate(t, T)
	if os.Getenv("TMPDIR") != "/Volumes/herdr-verify-ram-1/" || os.Getenv("HERDR_VERIFY_BOOT_TMPDIR") != "/var/folders/x/T/" {
		t.Fatalf("Isolate가 검증 임시 폴더를 잃었다: %q %q", os.Getenv("TMPDIR"), os.Getenv("HERDR_VERIFY_BOOT_TMPDIR"))
	}
}

// 시험 바이너리가 init에서 넣는 일지·운영 기록 /dev/null(internal/journal·oplog)과 go test의 PWD는 남는다 — 처음 판의 Isolate가 지워
// herdr-sync 계약 시험(TestContractA)이 띄운 도구가 시험 HOME에 일지를 써 「그대로」 단언 셋이 깨졌다
func TestEnvKeepsTestBinaryValues(t *testing.T) {
	t.Setenv("HERDR_TOOLS_JOURNAL", os.DevNull)
	t.Setenv("HERDR_TOOLS_OPLOG", os.DevNull)
	t.Setenv("PWD", "/x/pkg")
	T := TempDir(t)
	e := Env(T)
	if e["HERDR_TOOLS_JOURNAL"] != os.DevNull || e["HERDR_TOOLS_OPLOG"] != os.DevNull {
		t.Fatalf("Env가 일지 /dev/null을 잃었다: %v", e)
	}
	Isolate(t, T)
	if os.Getenv("HERDR_TOOLS_JOURNAL") != os.DevNull || os.Getenv("HERDR_TOOLS_OPLOG") != os.DevNull || os.Getenv("PWD") != "/x/pkg" {
		t.Fatalf("Isolate가 시험 바이너리 값을 잃었다: %q %q %q", os.Getenv("HERDR_TOOLS_JOURNAL"), os.Getenv("HERDR_TOOLS_OPLOG"), os.Getenv("PWD"))
	}
}

// Isolate: 시험 프로세스 환경에서 레인 값을 지우고(이 프로세스의 go 도구 값 GO*와 HOME은 남김) keep은 남긴다
func TestIsolateAllowOnly(t *testing.T) {
	home := os.Getenv("HOME")
	Poison(t)
	t.Setenv("HC_FILL_EXAMPLE", "/x/hc-fill.tsv")
	Isolate(t, TempDir(t), "HC_FILL_EXAMPLE")
	for _, k := range LaneEnv {
		if v, ok := os.LookupEnv(k); ok && !strings.HasPrefix(k, "GO") {
			t.Errorf("Isolate 뒤에도 남은 레인 값: %s=%s", k, v)
		}
	}
	if os.Getenv("GOFLAGS") != "poison-GOFLAGS" || os.Getenv("HOME") != home || os.Getenv("HC_FILL_EXAMPLE") != "/x/hc-fill.tsv" {
		t.Errorf("go 도구 값·HOME·keep은 남아야: GOFLAGS=%q HOME=%q keep=%q", os.Getenv("GOFLAGS"), os.Getenv("HOME"), os.Getenv("HC_FILL_EXAMPLE"))
	}
}
