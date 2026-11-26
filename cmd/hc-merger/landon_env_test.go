//go:build !windows

package main

// ht-merger-landon-env(2026-11-26 18:44 — 워커 착지의 준비 훅이 `npm: command not found` 종료 127 → 일시 실패 75로 한 바퀴 버림):
// 워커 착지(--land-run)의 준비 훅·착지는 그 기계에서 레인 준비 단계·추측 일꾼과 같은 셸 환경(shx.ZX — zsh·~/.zshrc가 있으면 그걸 읽은
// 셸)으로 돈다. 그래도 준비 훅이 명령 없음(127)이면 되풀이하지 않고 운영 사건 한 번 + 그 기계를 이 맨 앞 착지 후보에서 뺀다.

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// landHook: 시드에 준비 훅을 올린다
func landHook(s *sfx, body string) {
	os.WriteFile(s.seed+"/.claude/herdr-setup.sh", []byte("#!/bin/bash\n"+body+"\n"), 0o777)
	s.git(s.seed, "add", "-A")
	s.git(s.seed, "commit", "-qm", "setup hook")
	s.git(s.seed, "push", "-q", "origin", "main")
}

// ① 맨 PATH에 없고 ~/.zshrc가 더하는 도구를 준비 훅이 부름 → 통과해 워커에서 착지. 착지(hc-lane land)도 그 셸 환경으로
func TestLandOnWorkerShellEnv(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh 없음 — ~/.zshrc 셸 환경 길은 zsh가 있는 기계 몫")
	}
	s, runs := landFx(t) // HOME = s.dir
	tools := s.dir + "/shell-only"
	os.MkdirAll(tools, 0o777)
	os.WriteFile(tools+"/only-in-shell", []byte("#!/bin/sh\necho \"$PATH\" >\"$HOME/setup-path\"\n"), 0o777)
	cbm := s.dir + "/cbm" // 코드 색인 도구는 셸 환경에 있어도 준비 훅 PATH에서 뺀다(병합기 기계·추측 일꾼과 같은 규칙)
	os.MkdirAll(cbm, 0o777)
	os.WriteFile(cbm+"/codebase-memory-mcp", []byte("#!/bin/sh\n"), 0o777)
	os.WriteFile(s.dir+"/.zshrc", []byte("export PATH=\""+tools+":"+cbm+":$PATH\"\nexport LANDON_SHELL=zshrc\n"), 0o666)
	landHook(s, "only-in-shell")
	t.Setenv("LAND_ENV_OUT", s.dir+"/land-env")
	s.sub(1000000001, "a", false)
	s.round(1)
	if r := runs(); len(r) != 1 || r[0] != "v1" || !s.onMain("a.txt") {
		t.Fatalf("① 셸 환경의 도구로 준비 훅 통과 → v1에서 착지: 실행 %v 사건 %v\n%s", r, s.evs(), jdump())
	}
	if p, _ := os.ReadFile(s.dir + "/setup-path"); strings.Contains(string(p), cbm) {
		t.Fatalf("① 준비 훅 PATH에서 코드 색인 도구 폴더를 빼야: %s", p)
	}
	if e, _ := os.ReadFile(s.dir + "/land-env"); strings.TrimSpace(string(e)) != "zshrc" {
		t.Fatalf("① 착지도 ~/.zshrc를 읽은 셸에서: %q", e)
	}
}

// ② ~/.zshrc가 없는 기계(shx.ZX의 zsh 없음과 같은 갈래) → 지금 환경 그대로 — 지금 PATH의 도구로 준비 훅 통과
func TestLandOnWorkerNoShellEnv(t *testing.T) {
	s, runs := landFx(t)
	os.WriteFile(s.dir+"/bin/in-path-tool", []byte("#!/bin/sh\nexit 0\n"), 0o777)
	landHook(s, "in-path-tool")
	s.sub(1000000001, "a", false)
	s.round(1)
	if r := runs(); len(r) != 1 || r[0] != "v1" || !s.onMain("a.txt") {
		t.Fatalf("② 지금 PATH로 통과: 실행 %v\n%s", r, jdump())
	}
}

// ③ 워커의 준비 훅이 명령 없음(127) → 반송·일시 실패 되풀이 없이 운영 사건 한 번(어느 명령·어느 기계), 그 기계를 빼고 다음 바퀴 다른 워커로
func TestLandOnWorkerSetupNoCmd(t *testing.T) {
	s, runs := landFx(t)
	landHook(s, `if [ "$(basename "$LANE_REPO")" = v1 ]; then nocmd-xyz; fi`)
	s.sub(1000000001, "a", false)
	s.round(1)
	if r := runs(); len(r) != 1 || r[0] != "v1" || s.onMain("a.txt") || !s.has("refs/merge-queue/1000000001-a") ||
		s.has("refs/merge-failed/1000000001-a") || len(s.refs(landClaims)) != 0 {
		t.Fatalf("③ v1 명령 없음 — 큐 그대로·반송 아님·차례 치움: 실행 %v 차례 %v\n%s", r, s.refs(landClaims), jdump())
	}
	ev := func() (n int, line string) {
		for _, e := range s.evs() {
			if strings.Contains(e, "hc-merger-land-nocmd:v1:") {
				n, line = n+1, e
			}
		}
		return
	}
	if n, e := ev(); n != 1 || !strings.Contains(e, "nocmd-xyz") || !strings.Contains(e, "v1") {
		t.Fatalf("③ 사건 한 번(명령·기계): %d %q 사건들 %v", n, e, s.evs())
	}
	if l := landOnRows(t); len(l) < 2 || !strings.HasPrefix(l[1], "v1 착지 실행 끊김(준비 훅 명령 없음(nocmd-xyz))") {
		t.Fatalf("③ 일지: %q", l)
	}
	s.round(1)
	if r := runs(); len(r) != 2 || r[1] != "v2" || !s.onMain("a.txt") {
		t.Fatalf("③ 다음 바퀴 v2: %v\n%s", r, jdump())
	}
	if n, _ := ev(); n != 1 {
		t.Fatalf("③ 사건은 한 번: %d", n)
	}
}

func TestLandParseNoCmd(t *testing.T) {
	if rc, lost := landParse("nocmd npm"); rc != 0 || lost != "준비 훅 명령 없음(npm)" {
		t.Fatalf("%d %q", rc, lost)
	}
}

func TestMissingCmd(t *testing.T) {
	for in, want := range map[string]string{
		"x\n.claude/herdr-setup.sh: line 5: npm: command not found\n": "npm",
		"zsh: command not found: bun\n":                               "bun",
		"nothing\n":                                                   "?",
	} {
		if got := missingCmd(in); got != want {
			t.Errorf("%q → %q, 기대 %q", in, got, want)
		}
	}
}
