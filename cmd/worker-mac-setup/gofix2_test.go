package main

// 2026-11-22 Go 전환 재감사(go-reaudit base) 지적의 실패 시험. 이름의 ID는 보고서 지적 ID.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	wm "herdr-tools/internal/workermac"
)

// w2BadLs: 기계 식별자가 비었거나 행이 깨진 hc machine ls --json — 부재를 확정할 수 없다(모름)
var w2BadLs = []string{
	`{"machines":[null]}`,
	`{"machines":[{"state":"connected"}]}`,
	`{"machines":[{"name":null,"state":"connected"}]}`,
	`{"machines":[{"name":""}]}`,
	`{"machines":[{"name":"w2"},null]}`,
}

// [R-17] 이름 없는·null 행이 든 목록은 「w1 없음」이 아니다 — machineKnown은 오류, setup은 machine add 없이 실패
func TestR17MachineLsIncompleteRow(t *testing.T) {
	d := fakeSetup(t)
	ctx := context.Background()
	for _, ls := range w2BadLs {
		t.Setenv("HC_LS", ls)
		if k, err := machineKnown(ctx, "w1"); err == nil {
			t.Errorf("%s → known=%v err=nil, 바람 오류(모름)", ls, k)
		}
		if ls != w2BadLs[0] && ls != w2BadLs[1] {
			continue // setup은 원격 단계 뒤 3초를 쉬니 대표 둘만
		}
		os.Remove(d + "/hc.log")
		if rc := setup(ctx, &wm.Config{Ctx: ctx}, opts{m: "w1", slots: "1"}, "", "20261122"); rc == 0 {
			t.Errorf("%s → setup rc 0, 바람 실패", ls)
		}
		if strings.Contains(readF(d+"/hc.log"), "machine add") {
			t.Errorf("%s → machine add를 불렀다", ls)
		}
	}
}

// [R-17] 원격 메시 조각(cli)도 같다: 깨진 행이 든 목록이면 hc=실패, machine add 안 함(--check가 아닐 때도)
func TestR17MeshCliIncompleteRow(t *testing.T) {
	d := t.TempDir()
	home, bin := d+"/home", d+"/home/.local/bin"
	os.MkdirAll(home+"/.ssh", 0o700)
	os.MkdirAll(bin, 0o755)
	if b, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", d+"/hk").CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v %s", err, b)
	}
	pub := strings.Fields(readF(d + "/hk.pub"))
	os.WriteFile(home+"/.ssh/known_hosts", []byte("192.0.2.2 "+pub[0]+" "+pub[1]+"\n"), 0o600)
	lf, _ := exec.Command("ssh-keygen", "-lf", d+"/hk.pub").Output()
	fp := strings.Fields(string(lf))[1]
	os.WriteFile(bin+"/ssh", []byte("#!/bin/sh\n[ \"$1\" = -G ] && { echo 'hostname 192.0.2.2'; echo 'user bu'; }\nexit 0\n"), 0o755)
	os.WriteFile(bin+"/hc", []byte(`#!/bin/sh
if [ "$1 $2" = "machine ls" ]; then printf '%s\n' "$HC_LS"; exit 0; fi
echo "$*" >>"`+d+`/add.log"
`), 0o755)
	script := "B=wb H4=192.0.2.2 BU=bu FP=" + wm.Quote(fp) + ";" + piece("head") + piece("cli")
	for _, ls := range []string{`{"machines":[null]}`, `{"machines":[{"state":"connected"}]}`, `{"machines":[{"name":null}]}`, `{"machines":[{"name":""}]}`} {
		os.Remove(d + "/add.log")
		c := exec.Command("bash", "-c", script, "_", "0", "20261122")
		c.Env = append(os.Environ(), "HOME="+home, "HC_LS="+ls)
		out, err := c.Output()
		if err != nil {
			t.Fatalf("cli: %v %s", err, out)
		}
		if !strings.Contains(string(out), "hc=실패") || readF(d+"/add.log") != "" {
			t.Errorf("%s → %q add %q, 바람 hc=실패·add 없음", ls, out, readF(d+"/add.log"))
		}
	}
}

// w2Mesh: 가짜 ssh — 원격 조각(base64)을 풀어 info·auth·cli를 가린다. info는 wa 192.0.2.1·wb 192.0.2.2,
// auth는 W2_AUTH, cli는 W2_CLI 그대로. HOME known_hosts에 두 주소의 ed25519
func w2Mesh(t *testing.T) {
	d := t.TempDir()
	os.MkdirAll(d+"/.ssh", 0o700)
	if b, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", d+"/hk").CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v %s", err, b)
	}
	pub := strings.Fields(readF(d + "/hk.pub"))
	os.WriteFile(d+"/.ssh/known_hosts", []byte("192.0.2.1 "+pub[0]+" "+pub[1]+"\n192.0.2.2 "+pub[0]+" "+pub[1]+"\n"), 0o600)
	os.WriteFile(filepath.Join(d, "ssh"), []byte(`#!/bin/bash
# 인자: -n -o BatchMode=yes -o ConnectTimeout=8 -- <기계> <원격 명령>
m=$7; cmd=$8
s=$(printf '%s' "$cmd" | sed 's/.*echo \([A-Za-z0-9+/=]*\) | base64.*/\1/' | base64 -D)
if printf '%s' "$s" | grep -q 'echo "ts4='; then
  case $m in wa) n=1 ;; *) n=2 ;; esac
  printf 'user=u\nts4=192.0.2.%s\npub=ssh-ed25519 AAAAw2 %s\n' "$n" "$m"
elif printf '%s' "$s" | grep -q 'authorized_keys'; then
  printf '%s\n' "$W2_AUTH"
elif printf '%s' "$s" | grep -q 'known_hosts'; then
  printf '%s\n' "$W2_CLI"
fi
exit 0
`), 0o755)
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	t.Setenv("HOME", d)
}

// [R-38] mesh --check: auth·cli 응답의 필수 칸이 비거나·겹치거나·모르는 값이면 그 방향은 실패(1) — 접속 성공만으로 성공 처리하지 않는다
func TestR38MeshCheckIncompleteReply(t *testing.T) {
	w2Mesh(t)
	ctx := context.Background()
	t.Setenv("W2_AUTH", "있음")
	t.Setenv("W2_CLI", "kh=있음\ncfg=있음\nhc=있음\nconn=됨")
	if rc := meshRun(ctx, "wa wb", "", true, "20261122"); rc != 0 {
		t.Fatalf("정상 응답 → rc %d, 바람 0 (가짜가 틀림)", rc)
	}
	for _, c := range []struct{ name, auth, cli string }{
		{"auth 빈 출력", "", "kh=있음\ncfg=있음\nhc=있음\nconn=됨"},
		{"auth 모르는 값", "뭔가", "kh=있음\ncfg=있음\nhc=있음\nconn=됨"},
		{"auth 겹침", "있음\n있음", "kh=있음\ncfg=있음\nhc=있음\nconn=됨"},
		{"cli 접속만", "있음", "conn=됨"},
		{"cli kh 없음", "있음", "cfg=있음\nhc=있음\nconn=됨"},
		{"cli hc 빈 값", "있음", "kh=있음\ncfg=있음\nhc=\nconn=됨"},
		{"cli 겹침", "있음", "kh=있음\nkh=있음\ncfg=있음\nhc=있음\nconn=됨"},
		{"cli 모르는 값", "있음", "kh=있음\ncfg=뭔가\nhc=있음\nconn=됨"},
		{"cli conn 겹침", "있음", "kh=있음\ncfg=있음\nhc=있음\nconn=됨\nconn=안 됨"},
	} {
		t.Setenv("W2_AUTH", c.auth)
		t.Setenv("W2_CLI", c.cli)
		if rc := meshRun(ctx, "wa wb", "", true, "20261122"); rc != 1 {
			t.Errorf("%s → rc %d, 바람 1(점검 실패)", c.name, rc)
		}
	}
}
