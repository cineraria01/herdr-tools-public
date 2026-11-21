package main

// 리눅스 받기 시험 — 2026-11-21 go-linux-dist(사용자 「난 다 고로 변경 하고 싶어」).
// 릴리스에 linux/amd64를 더하는 도구 고르기(목록에서 — 이름을 박지 않는다)와, 리눅스 받기 조각의 해시 확인·구조 가드.

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// 리눅스 대상 기계가 있는 도구만: 범용 저장소 bin/<도구> 줄의 대상이 all이거나 리눅스 기계 이름을 담을 때.
// macos·local·macOS 기계 이름만인 줄, 개인 저장소 줄, cmd/ 에 없는 이름은 빼낸다
func TestLinuxTools(t *testing.T) {
	repo := "/r"
	units := []unit{
		{k: ".local/bin/merge", r: "bin/merge", d: repo, t: "all"},
		{k: ".local/bin/lane", r: "bin/lane", d: repo, t: "macos"},
		{k: ".local/bin/fill", r: "bin/fill", d: repo, t: "local"},
		{k: ".local/bin/foo", r: "bin/foo", d: repo, t: "box-m,box-l"},
		{k: ".local/bin/bar", r: "bin/bar", d: repo, t: "box-m"},
		{k: ".local/bin/priv", r: "bin/priv", d: "/conf", t: "all"},
		{k: ".local/bin/noc", r: "bin/noc", d: repo, t: "all"},
		{k: ".local/lib/herdr-go/cmd/lane/main.go", r: "cmd/lane/main.go", d: repo, t: "all", tree: true}, // 소스 줄은 도구 줄이 아니다
	}
	tools := []string{"bar", "fill", "foo", "lane", "merge", "priv"}
	osOf := map[string]string{"box-m": "Darwin", "box-l": "Linux"}
	if got, want := linuxTools(tools, units, repo, osOf), []string{"foo", "merge"}; !reflect.DeepEqual(got, want) {
		t.Errorf("linuxTools = %v, 바람 %v", got, want)
	}
	if got := linuxTools(tools, units[1:3], repo, osOf); len(got) != 0 {
		t.Errorf("리눅스 대상이 없는데 %v", got)
	}
	// all 줄은 리눅스 기계가 이번에 안 닿아도 짓는다(SHA256SUMS가 닿은 기계에 따라 바뀌지 않게)
	if got, want := linuxTools(tools, units, repo, map[string]string{"box-m": "Darwin"}), []string{"merge"}; !reflect.DeepEqual(got, want) {
		t.Errorf("리눅스 기계를 모를 때 %v, 바람 %v", got, want)
	}
}

// lxRun: 리눅스 받기 조각(installFor("Linux"))을 임시 HOME에서 sh로. uname -m은 가짜(arch)
func lxRun(t *testing.T, home, arch string, args ...string) string {
	t.Helper()
	fb := t.TempDir()
	if err := os.WriteFile(fb+"/uname", []byte("#!/bin/sh\ncase $1 in -m) echo "+arch+" ;; *) echo Linux ;; esac\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := exec.Command("sh", append([]string{"-c", installFor("Linux"), "_"}, args...)...)
	c.Env = append(os.Environ(), "HOME="+home, "PATH="+fb+":"+os.Getenv("PATH"))
	out, _ := c.Output()
	return string(out)
}

// lxEnv: 임시 HOME에 깐 소스(go.mod)와 받을 바이너리. src = 그 소스 판(install은 판이 다르면 K로 건너뛴다)
func lxEnv(t *testing.T) (home, bin, art, sum, src string) {
	t.Helper()
	home = t.TempDir()
	bin = home + "/.local/lib/herdr-go/.bin"
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(home+"/.local/lib/herdr-go/go.mod", []byte("module herdr-tools\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	art = t.TempDir() + "/demo-linux-amd64"
	b := []byte("#!/bin/sh\necho NEW\n")
	if err := os.WriteFile(art, b, 0o644); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(b)
	return home, bin, art, hex.EncodeToString(h[:]), srcOf(t, home)
}

func TestInstallLinux(t *testing.T) {
	t.Run("해시 다름은 쓰지 않음", func(t *testing.T) {
		home, bin, art, _, src := lxEnv(t)
		out := lxRun(t, home, "x86_64", "file://"+art, strings.Repeat("0", 64), "demo", src)
		if !strings.HasPrefix(out, "E\tdemo\t") {
			t.Errorf("E 줄이 아니다: %q", out)
		}
		if _, err := os.Stat(bin + "/demo"); err == nil {
			t.Errorf("해시가 다른데 바이너리를 썼다")
		}
		if _, err := os.Stat(bin + "/demo.src"); err == nil {
			t.Errorf("해시가 다른데 소스 판을 썼다")
		}
	})
	t.Run("해시 같으면 받음", func(t *testing.T) {
		home, bin, art, sum, src := lxEnv(t)
		if out := lxRun(t, home, "amd64", "file://"+art, sum, "demo", src); out != "W\tdemo\n" {
			t.Errorf("출력 %q", out)
		}
		if b, _ := os.ReadFile(bin + "/demo.src"); string(b) != src+"\n" {
			t.Errorf(".src %q", b)
		}
		if out := lxRun(t, home, "x86_64", "file://"+art, sum, "demo", src); out != "S\tdemo\n" {
			t.Errorf("두 번째는 같음이어야: %q", out)
		}
	})
	t.Run("amd64가 아니면 받지 않음", func(t *testing.T) {
		home, bin, art, sum, src := lxEnv(t)
		if out := lxRun(t, home, "aarch64", "file://"+art, sum, "demo", src); out != "K\tdemo\t리눅스 amd64가 아니다\n" {
			t.Errorf("출력 %q", out)
		}
		if _, err := os.Stat(bin + "/demo"); err == nil {
			t.Errorf("다른 구조인데 받았다")
		}
	})
}
