//go:build !windows

package main

// gc-patchid-once: gc의 「들어감」 판정(gin)이 기준 가지 쪽 patch-id를 참조마다·실행마다 다시 만들지 않는다.
// ① 꼴마다 판정이 지금과 같다 ② 다시 돌면 기준 쪽 계산 0 ③ 기준이 움직이면 새 커밋만 ④ 깨진 캐시는 무시·위조는 확인에서 걸림
// ⑤ 기준에서 빠진 커밋의 캐시 줄 정리

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type pidFx struct {
	t    *testing.T
	d    string // 저장소
	glog string // 가짜 git이 patch-id에 넘긴 입력을 쌓는 곳
}

func (f *pidFx) git(args ...string) string {
	f.t.Helper()
	c := exec.Command("git", append([]string{"-C", f.d}, args...)...)
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	b, err := c.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, b)
	}
	return strings.TrimSpace(string(b))
}

// commit: 파일 하나를 새로 더하는 커밋(패치가 문맥과 상관없게)
func (f *pidFx) commit(file string) string {
	f.t.Helper()
	os.WriteFile(filepath.Join(f.d, file), []byte(file+"\n"), 0o644)
	f.git("add", "--", file)
	f.git("commit", "-q", "-m", file)
	return f.git("rev-parse", "HEAD")
}

func (f *pidFx) cache() string {
	b, _ := os.ReadFile(filepath.Join(f.d, ".git", "hc-lane-patch-id"))
	return string(b)
}

var commitLine = regexp.MustCompile(`(?m)^commit ([0-9a-f]{40})`)

// based: 지난 비움 뒤 patch-id에 들어간 커밋 가운데 지금 main(또는 then)에 든 것
func (f *pidFx) based(then map[string]bool) []string {
	f.t.Helper()
	b, _ := os.ReadFile(f.glog)
	var out []string
	for _, m := range commitLine.FindAllStringSubmatch(string(b), -1) {
		if then[m[1]] {
			out = append(out, m[1])
		}
	}
	return out
}

func (f *pidFx) mainSet() map[string]bool {
	s := map[string]bool{}
	for _, x := range strings.Fields(f.git("rev-list", "main")) {
		s[x] = true
	}
	return s
}

func (f *pidFx) resetLog() { os.WriteFile(f.glog, nil, 0o644) }

// newPidFx: main f0..f9, 가지 anc(조상)·cp(cherry-pick으로 들어감)·part(둘 중 하나만)·out(안 들어감)·rew(들어갔다가 main이 되감김)
func newPidFx(t *testing.T) (*pidFx, map[string]string) {
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git 없음")
	}
	d := t.TempDir()
	bin := t.TempDir()
	f := &pidFx{t: t, d: d, glog: filepath.Join(bin, "patch-id.in")}
	os.WriteFile(filepath.Join(bin, "git"), []byte(`#!/bin/sh
for a; do if [ "$a" = patch-id ]; then tee -a "$GLOG" | "$REALGIT" "$@"; exit $?; fi; done
exec "$REALGIT" "$@"
`), 0o755)
	t.Setenv("REALGIT", real)
	t.Setenv("GLOG", f.glog)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	f.git("init", "-q", "-b", "main")
	f.git("config", "commit.gpgsign", "false")
	tip := map[string]string{}
	var fs []string
	for i := 0; i < 10; i++ {
		fs = append(fs, f.commit("f"+string(rune('0'+i))))
	}
	tip["anc"] = fs[5]
	side := func(name string, files ...string) []string {
		f.git("checkout", "-q", "-b", name, fs[2])
		var cs []string
		for _, x := range files {
			cs = append(cs, f.commit(x))
		}
		f.git("checkout", "-q", "main")
		tip[name] = cs[len(cs)-1]
		return cs
	}
	cp := side("cp", "x1")
	part := side("part", "y1", "y2")
	side("out", "z1")
	rew := side("rew", "w1")
	f.git("cherry-pick", cp[0])
	f.git("cherry-pick", part[0])
	tip["preRew"] = f.git("rev-parse", "HEAD")
	f.git("cherry-pick", rew[0])
	oldOn, oldCore, oldRepo, oldBref := on, core, repo, bref
	on, core, repo, bref = "", "", d, "refs/heads/main"
	t.Cleanup(func() { on, core, repo, bref = oldOn, oldCore, oldRepo, oldBref })
	f.resetLog()
	return f, tip
}

func ginWant(t *testing.T, tip map[string]string, want map[string]string) {
	t.Helper()
	for k, w := range want {
		n, r := gin(tip[k])
		got := n
		if r != 1 {
			got = map[int]string{0: "들어감", 2: "모름"}[r]
		}
		if got != w {
			t.Errorf("%s: 판정 %q, 기대 %q", k, got, w)
		}
	}
}

var pidWant = map[string]string{"anc": "들어감", "cp": "들어감", "part": "1", "out": "1", "rew": "들어감"}

// ①: 꼴마다 판정이 지금(캐시 없는 계산)과 같고, 두 번째(캐시)에도 같고, main이 되감기면 「들어감」이던 것이 「안 들어감」으로
func TestGcPidSameVerdicts(t *testing.T) {
	f, tip := newPidFx(t)
	ginWant(t, tip, pidWant)
	ginWant(t, tip, pidWant)
	f.git("reset", "-q", "--hard", tip["preRew"])
	ginWant(t, tip, map[string]string{"anc": "들어감", "cp": "들어감", "part": "1", "out": "1", "rew": "1"})
}

// ②: 두 번째 실행에서 「안 들어감」 참조의 판정은 기준 쪽 커밋 patch-id를 하나도 다시 만들지 않는다.
// 「들어감」은 캐시로 맞춘 기준 커밋만(가지 쪽 커밋 수 이하) 새로 계산해 확인한다
func TestGcPidSecondRunNoBase(t *testing.T) {
	f, tip := newPidFx(t)
	ginWant(t, tip, pidWant)
	if f.cache() == "" {
		t.Fatalf("첫 실행 뒤 캐시가 없다")
	}
	ms := f.mainSet()
	f.resetLog()
	ginWant(t, tip, map[string]string{"part": "1", "out": "1"})
	if b := f.based(ms); len(b) != 0 {
		t.Errorf("두 번째 실행의 안 들어감 판정이 기준 쪽 patch-id를 %d개 다시 만들었다", len(b))
	}
	f.resetLog()
	ginWant(t, tip, map[string]string{"cp": "들어감"})
	if b := f.based(ms); len(b) != 1 {
		t.Errorf("들어감 확인은 맞춘 기준 커밋 1개만 새로 계산해야: %d개", len(b))
	}
}

// ③: main이 움직이면 새로 든 커밋만 계산한다
func TestGcPidBaseMovedOnlyNew(t *testing.T) {
	f, tip := newPidFx(t)
	ginWant(t, tip, pidWant)
	old := f.mainSet()
	n1, n2 := f.commit("g1"), f.commit("g2")
	ms := f.mainSet()
	f.resetLog()
	ginWant(t, tip, map[string]string{"out": "1", "part": "1"})
	got := f.based(ms)
	for _, x := range got {
		if old[x] {
			t.Errorf("이미 계산한 기준 커밋 %s를 다시 계산했다", x[:8])
		}
	}
	if len(got) != 2 || !(contains(got, n1) && contains(got, n2)) {
		t.Errorf("새로 든 두 커밋만 계산해야: %v", got)
	}
}

// ④: 꼴이 깨진 캐시 줄은 무시하고 계산한다. 꼴은 맞는데 값이 틀린 줄(위조)로 「들어감」이 나오지 않는다 — 들어감은 새로 계산해 확인
func TestGcPidBrokenCache(t *testing.T) {
	f, tip := newPidFx(t)
	k := filepath.Join(f.d, ".git", "hc-lane-patch-id")
	os.WriteFile(k, []byte("zzz\nabc def ghi\n0123\n\x00\x01 broken\n"), 0o644)
	ginWant(t, tip, pidWant)
	// 위조: main의 한 커밋(main~3)의 patch-id를 out 가지 커밋의 patch-id로 적는다
	c := exec.Command("sh", "-c", `git log -p --no-color --no-ext-diff -1 "$1" | git patch-id --verbatim`, "x", tip["out"])
	c.Dir = f.d
	b, err := c.Output()
	if err != nil || len(strings.Fields(string(b))) != 2 {
		t.Fatalf("patch-id: %v %q", err, b)
	}
	mc := f.git("rev-parse", "main~3")
	os.WriteFile(k, []byte(f.cache()+strings.Fields(string(b))[0]+" "+mc+"\n"), 0o644)
	ginWant(t, tip, map[string]string{"out": "1"})
	ginWant(t, tip, pidWant)
}

// ⑤: 기준에서 빠진 커밋(되감김)의 캐시 줄은 정리하고, 가지 쪽 커밋은 애초에 캐시에 두지 않는다
func TestGcPidPrune(t *testing.T) {
	f, tip := newPidFx(t)
	ginWant(t, tip, pidWant)
	rewOnMain := f.git("rev-parse", "main")
	if !strings.Contains(f.cache(), rewOnMain) {
		t.Fatalf("main 끝 커밋이 캐시에 없다:\n%s", f.cache())
	}
	for _, k := range []string{"out", "part", "cp", "rew"} {
		if strings.Contains(f.cache(), tip[k]) {
			t.Errorf("가지 쪽 커밋 %s(%s)이 캐시에 있다", k, tip[k][:8])
		}
	}
	f.git("reset", "-q", "--hard", tip["preRew"])
	gcPrune()
	if strings.Contains(f.cache(), rewOnMain) {
		t.Errorf("되감겨 빠진 커밋 줄이 정리되지 않았다")
	}
	if !strings.Contains(f.cache(), f.git("rev-parse", "main~1")) {
		t.Errorf("main에 남은 커밋 줄까지 지웠다:\n%s", f.cache())
	}
}
