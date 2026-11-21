package hclane_test

// 절 등록과 여러 절이 함께 쓰는 셸 함수(conf·land·gone·lnd·mklane·ev·upto …)를 옮긴 것.

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
)

type sec struct {
	order int
	name  string
	f     func(*K)
}

var secs []sec

// reg: 절 하나를 셸 시험 차례(order)로 등록한다. 절끼리 export·설정을 물려받으니 한 판 안에서는 같은 K로 차례로 돈다.
func reg(order int, name string, f func(*K)) { secs = append(secs, sec{order, name, f}) }

// TestHcLane: 판마다 절을 차례로 하위 시험으로(go test -run 'TestHcLane/go/s20' 꼴로 골라 돌 수 있다 — 앞 절 상태가 필요한 절은 함께).
func TestHcLane(t *testing.T) {
	sort.SliceStable(secs, func(i, j int) bool { return secs[i].order < secs[j].order })
	eachImpl(t, func(k *K) {
		for _, s := range secs {
			k.t.Run(s.name, func(t *testing.T) {
				pt := k.t
				k.t = t
				defer func() { k.t = pt }()
				s.f(k)
			})
		}
	})
}

// conf <줄…>: $T/.config/herdr-tools/config를 그 줄들로. noconf: 지운다.
func (k *K) conf(lines ...string) {
	write(k.t, filepath.Join(k.T, ".config/herdr-tools/config"), strings.Join(lines, "\n")+"\n")
}

func (k *K) noconf() { os.Remove(filepath.Join(k.T, ".config/herdr-tools/config")) }

// land <레인>: 띄우고 커밋 하나를 origin/main에 넣은 뒤 무시된 파일(.omc/state) 하나를 남긴다. k.wt를 정한다.
func (k *K) land(name string) {
	k.t.Helper()
	k.run("--name", name, "--repo", k.R, "--", "true")
	k.wt = k.R + ".wt/" + name
	k.sh(k.wt, `echo "$1" >"$1.txt" && git add -- "$1.txt" && git commit -qm "$1" -- "$1.txt" && git push -q origin HEAD:main`, name)
	write(k.t, k.wt+"/.omc/state/s.json", "s\n")
}

// mklane <레인>: 띄우고 커밋 하나(push 없음). k.wt를 정한다.
func (k *K) mklane(name string) {
	k.t.Helper()
	k.run("--name", name, "--repo", k.R, "--", "true")
	k.wt = k.R + ".wt/" + name
	k.sh(k.wt, `echo "$1" >"$1.txt" && git add -- "$1.txt" && git commit -qm "$1" -- "$1.txt"`, name)
}

// gone: 폴더·가지·워크트리 등록이 모두 없다. kept: 폴더와 가지가 있다. regd: 워크트리로 등록돼 있다(셸 reg).
func (k *K) gone(name string) bool {
	return !exists(k.R+".wt/"+name) && !k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/"+name) &&
		!strings.Contains(k.git(k.R, "worktree", "list", "--porcelain"), k.R+".wt/"+name)
}

func (k *K) kept(name string) bool {
	return isDir(k.R+".wt/"+name) && k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/"+name)
}

func (k *K) regd(name string) bool {
	return contains(strings.Split(k.git(k.R, "worktree", "list", "--porcelain"), "\n"), "worktree "+k.R+".wt/"+name)
}

// last: 출력 끝 줄에 그 글이 있다.
func (k *K) last(s string) bool { return strings.Contains(lastLine(k.out), s) }

// lnd <폴더> [인자…]: 그 폴더에서 hc-lane land. lndE는 `VAR=값 lnd …`.
func (k *K) lnd(dir string, args ...string) { k.lndE(nil, dir, args...) }

func (k *K) lndE(extra []string, dir string, args ...string) {
	k.runIn(dir, extra, append([]string{"land"}, args...)...)
}

// otop [가지]: origin 그 가지 끝 커밋 제목.
func (k *K) otop(br ...string) string {
	b := "main"
	if len(br) > 0 {
		b = br[0]
	}
	return k.git(k.D+"/origin.git", "log", "-1", "--format=%s", b)
}

// seedp: seed 클론을 당겨 온 뒤 seed.
func (k *K) seedp(file, content string) {
	k.t.Helper()
	k.sh(k.D+"/seed", `git pull -q --rebase origin "$1"`, k.B)
	k.seed(file, content)
}

// ev: 사건 줄(셸 ev·ev8 — 칸을 탭으로 잇고 끝 줄바꿈).
func ev(f ...string) string { return strings.Join(f, "\t") + "\n" }

// appendf: 파일 끝에 덧붙인다(셸 >>).
func appendf(t *testing.T, p, s string) {
	t.Helper()
	mkdirs(t, filepath.Dir(p))
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

// upto: 조건이 맞을 때까지 0.1초마다, 8초까지.
func upto(f func() bool) bool {
	for i := 0; i < 80; i++ {
		if f() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// alive: 그 pid가 살아 있다(kill -0).
func alive(pid int) bool { return pid > 0 && syscall.Kill(pid, 0) == nil }

// Bg: 배경으로 띄운 명령(셸 `… &`). 자기 프로세스 무리로 띄우고, 절이 끝나면(Cleanup) 무리째 끈다.
type Bg struct {
	Pid  int
	done chan struct{}
	rc   int
}

// bg: dir에서 이름을 배경으로. stdout·stderr는 파일 경로(빈 값이면 버림). `VAR=값`은 extra.
func (k *K) bg(dir, outPath, errPath string, extra []string, name string, args ...string) *Bg {
	k.t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env = dir, k.environ(extra)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if outPath != "" {
		f, err := os.Create(outPath)
		if err != nil {
			k.t.Fatal(err)
		}
		defer f.Close()
		cmd.Stdout = f
	}
	if errPath != "" {
		f, err := os.Create(errPath)
		if err != nil {
			k.t.Fatal(err)
		}
		defer f.Close()
		cmd.Stderr = f
	}
	if err := cmd.Start(); err != nil {
		k.t.Fatal(err)
	}
	b := &Bg{Pid: cmd.Process.Pid, done: make(chan struct{}), rc: -1}
	go func() {
		cmd.Wait()
		b.rc = cmd.ProcessState.ExitCode()
		if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			b.rc = 128 + int(ws.Signal())
		}
		close(b.done)
	}()
	k.t.Cleanup(func() { syscall.Kill(-b.Pid, syscall.SIGKILL); b.Wait() })
	return b
}

// Wait: 끝날 때까지 기다려 종료 코드(신호로 끝났으면 128+신호 — 셸 wait 꼴).
func (b *Bg) Wait() int { <-b.done; return b.rc }

// Done: 끝났는지(기다리지 않음).
func (b *Bg) Done() bool {
	select {
	case <-b.done:
		return true
	default:
		return false
	}
}

// stop: TERM을 보내고 기다린다(셸 stopw).
func (b *Bg) stop() { syscall.Kill(b.Pid, syscall.SIGTERM); b.Wait() }

// runs: 가짜 hc run을 부른 횟수(hc.log의 「run」 줄 수).
func (k *K) runs() int {
	n := 0
	for _, l := range k.hclog("") {
		if l == "run" {
			n++
		}
	}
	return n
}
