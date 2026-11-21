package main

// 2026-11-21 Go 전환 전체 감사(go-final-audit base) 지적의 단위 시험. 이름의 ID는 보고서 지적 ID.

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

// [E10] 기계 이름이 -로 시작하면 ssh·hc 옵션으로 읽힌다 — 사용 오류 64
func TestParseDashName(t *testing.T) {
	for _, a := range [][]string{{"-oProxyCommand=x"}, {"mesh", "-x"}} {
		if _, code := parse(a); code != 64 {
			t.Errorf("%q → %d, 바람 64", a, code)
		}
	}
}

// fakeSetup: ssh(true·id·echo $HOME는 성공, 원격 본문은 SSH_BODY_RC로 끝남)와 hc(machine ls는 HC_LS·HC_LS_RC, 그 밖은 hc.log)
func fakeSetup(t *testing.T) string {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "ssh"), []byte(`#!/bin/sh
case "$*" in
  *" true") exit 0 ;;
  *"id -un") echo u; exit 0 ;;
  *'echo $HOME') echo /h; exit 0 ;;
  *"bash -c"*) echo "▸ 원격 한 줄"; exit "${SSH_BODY_RC:-0}" ;;
esac
exit 0
`), 0o755)
	os.WriteFile(filepath.Join(d, "hc"), []byte(`#!/bin/sh
if [ "$1 $2" = "machine ls" ]; then printf '%s\n' "$HC_LS"; exit "${HC_LS_RC:-0}"; fi
echo "$*" >>"`+d+`/hc.log"
exit 0
`), 0o755)
	os.WriteFile(filepath.Join(d, "herdr-tools-config"), []byte("#!/bin/sh\nexit 3\n"), 0o755)
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	t.Setenv("HERDR_TOOLS_CONFIG", d)
	return d
}

// [E02] hc machine ls가 실패하면 「등록 안 됨」으로 읽지 않는다 — machine add 없이 실패. 정상 JSON이면 공백이 있어도 정확한 이름으로 본다
func TestSetupMachineLsUnknown(t *testing.T) {
	d := fakeSetup(t)
	ctx := context.Background()
	t.Setenv("HC_LS_RC", "75")
	t.Setenv("HC_LS", "")
	if rc := setup(ctx, &wm.Config{Ctx: ctx}, opts{m: "w1", slots: "1"}, "", "20261121"); rc == 0 {
		t.Errorf("machine ls 실패인데 rc 0")
	}
	if strings.Contains(readF(d+"/hc.log"), "machine add") {
		t.Error("machine ls 실패인데 machine add를 불렀다")
	}
	os.Remove(d + "/hc.log")
	t.Setenv("HC_LS_RC", "0")
	t.Setenv("HC_LS", `{"machines": [{"name": "w1", "state": "connected"}]}`)
	setup(ctx, &wm.Config{Ctx: ctx}, opts{m: "w1", slots: "1"}, "", "20261121")
	if strings.Contains(readF(d+"/hc.log"), "machine add") {
		t.Error("공백 든 JSON에 w1이 있는데 machine add")
	}
	os.Remove(d + "/hc.log")
	t.Setenv("HC_LS", `{"machines": [{"name": "w10", "state": "connected"}]}`)
	setup(ctx, &wm.Config{Ctx: ctx}, opts{m: "w1", slots: "1"}, "", "20261121")
	if !strings.Contains(readF(d+"/hc.log"), "machine add w1") {
		t.Error("w10만 있으면 w1은 등록해야 함")
	}
}

// [E03] 원격 설정 본문의 ssh가 255(끊김)로 끝나면 체크리스트로 성공 처리하지 않는다
func TestSetupRemoteCut(t *testing.T) {
	fakeSetup(t)
	ctx := context.Background()
	t.Setenv("HC_LS", `{"machines":[{"name":"w1"}]}`)
	t.Setenv("SSH_BODY_RC", "255")
	if rc := setup(ctx, &wm.Config{Ctx: ctx}, opts{m: "w1", slots: "1", dry: true}, "", "20261121"); rc == 0 {
		t.Error("원격 본문 255인데 rc 0")
	}
	t.Setenv("SSH_BODY_RC", "0")
	if rc := setup(ctx, &wm.Config{Ctx: ctx}, opts{m: "w1", slots: "1", dry: true}, "", "20261121"); rc != 0 {
		t.Errorf("정상 원격 → %d", rc)
	}
}

// [E03] 전체 설정에서 부른 워커 연결이 문제(1)면 최종 결과도 1 — 체크리스트는 그대로 찍는다
func TestSetupMeshFailCounts(t *testing.T) {
	fakeSetup(t)
	ctx := context.Background()
	t.Setenv("HC_LS", `{"machines":[{"name":"w1"}]}`)
	// 가짜 ssh의 원격 조각은 ts4= 줄이 없어 mesh가 「Tailscale 주소를 못 읽음」으로 센다
	if rc := setup(ctx, &wm.Config{Ctx: ctx}, opts{m: "w1", slots: "1", dry: true}, "w1 w2", "20261121"); rc != 1 {
		t.Errorf("워커 연결 문제인데 rc %d", rc)
	}
}

// [E11] hc machine add --install에 상한 — 넘기면 등록 결과를 모르니 실패로 끝내고 원격 단계로 가지 않는다
func TestSetupAddTimeout(t *testing.T) {
	d := fakeSetup(t)
	os.WriteFile(filepath.Join(d, "hc"), []byte(`#!/bin/sh
if [ "$1 $2" = "machine ls" ]; then echo '{"machines":[]}'; exit 0; fi
echo "$*" >>"`+d+`/hc.log"
exec sleep 30
`), 0o755)
	old := addSec
	addSec = 1
	defer func() { addSec = old }()
	ctx := context.Background()
	start := time.Now()
	if rc := setup(ctx, &wm.Config{Ctx: ctx}, opts{m: "w1", slots: "1"}, "", "20261121"); rc != 1 {
		t.Errorf("add 시간 넘김인데 rc %d", rc)
	}
	if time.Since(start) > 15*time.Second {
		t.Error("상한이 안 먹음")
	}
	if !strings.Contains(readF(d+"/hc.log"), "machine add w1 --install --name w1") {
		t.Error("machine add를 부르지 않았다")
	}
}

// [E02] 원격 메시 조각(cli): 워커의 hc machine ls 실패·JSON 아님은 「없음」이 아니라 실패(등록 안 함), 정상 JSON은 정확한 이름으로
func TestMeshCliMachineLs(t *testing.T) {
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
if [ "$1 $2" = "machine ls" ]; then printf '%s\n' "$HC_LS"; exit "${HC_LS_RC:-0}"; fi
echo "$*" >>"`+d+`/add.log"
`), 0o755)
	script := "B=wb H4=192.0.2.2 BU=bu FP=" + wm.Quote(fp) + ";" + piece("head") + piece("cli")
	run := func(check, ls, lsrc string) string {
		os.Remove(d + "/add.log")
		c := exec.Command("bash", "-c", script, "_", check, "20261121")
		c.Env = append(os.Environ(), "HOME="+home, "HC_LS="+ls, "HC_LS_RC="+lsrc)
		out, err := c.Output()
		if err != nil {
			t.Fatalf("cli: %v %s", err, out)
		}
		return string(out)
	}
	for _, x := range []struct{ check, ls, rc, want string }{
		{"0", "", "75", "hc=실패(machine ls를 못 읽음"},
		{"1", "", "75", "hc=실패(machine ls를 못 읽음"},
		{"0", "아님", "0", "hc=실패(machine ls를 못 읽음"},
		{"0", `{"machines": [{"name": "wb", "state": "connected"}]}`, "0", "hc=있음"},
	} {
		if out := run(x.check, x.ls, x.rc); !strings.Contains(out, x.want) || readF(d+"/add.log") != "" {
			t.Errorf("%+v → %q add %q", x, out, readF(d+"/add.log"))
		}
	}
	if out := run("0", `{"machines":[{"name":"wb0"}]}`, "0"); !strings.Contains(out, "hc=넣음") || !strings.Contains(readF(d+"/add.log"), "machine add wb --name wb") {
		t.Errorf("wb0만 있으면 wb 등록: %q", out)
	}
}

// [B25] --version에 릴리스 커밋(-ldflags -X …version.Commit)이 보인다
func TestVersionShowsCommit(t *testing.T) {
	bin := t.TempDir() + "/worker-mac-setup"
	if b, err := exec.Command("go", "build", "-ldflags", "-X herdr-tools/internal/version.Commit=abc123def456", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("빌드: %v %s", err, b)
	}
	if out, err := exec.Command(bin, "--version").Output(); err != nil || string(out) != "worker-mac-setup (Go abc123def456)\n" {
		t.Fatalf("--version: %q %v", out, err)
	}
}

func readF(p string) string { b, _ := os.ReadFile(p); return string(b) }
