package hclane_test

// gofix-lane T1(감사 Fable T1): hc-lane 길의 시간 상한·신호 계약 — 상한이 걸린 결과는 「모름」(상태를 안 바꾸고 다시), 신호로 끄면
// 상한 때문에 따로 무리가 된 자식(git push 등)도 함께 끝난다. 두 판이 같아야 하는 것은 check, 셸 판도 같은 결함이면 goOnly

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestGofixLimits(t *testing.T) {
	eachImpl(t, func(k *K) {
		for _, c := range []struct {
			name string
			f    func(*K)
		}{{"M1", gfSignalPush}, {"M2", gfSlowRebase}, {"M3", gfGcStuckGit}, {"M4", gfWaitStuckHc}, {"M01", gfLandTimeout75}} {
			k.t.Run(c.name, func(t *testing.T) {
				pt := k.t
				k.t = t
				defer func() { k.t = pt }()
				c.f(k)
			})
		}
	})
}

// M1·L01: 착지 push 도중(느린 pre-push 훅) hc-lane 무리에 TERM — push·훅이 함께 끝나고 origin/main은 그대로
func gfSignalPush(k *K) {
	k.mk("gfm1")
	k.seed(".claude/herdr-verify.sh", "exit 0")
	k.git(k.R, "pull", "-q", "--ff-only")
	k.mklane("gfm1l")
	mark := filepath.Join(k.T, "m1-hook")
	os.Remove(mark)
	hooks := k.git(k.R, "rev-parse", "--path-format=absolute", "--git-common-dir") + "/hooks"
	write(k.t, hooks+"/pre-push", "#!/bin/sh\necho $$ >"+sq(mark)+"\nsleep 6\nexit 0\n")
	os.Chmod(hooks+"/pre-push", 0o755)
	before := k.git(k.D+"/origin.git", "rev-parse", "main")
	b := k.bg(k.wt, filepath.Join(k.T, "m1o"), filepath.Join(k.T, "m1e"), []string{"HC_LANE=gfm1l"}, k.LANE, "land", "main")
	got := upto(func() bool { return strings.TrimSpace(read(mark)) != "" })
	b.signal(true, syscall.SIGTERM)
	b.Wait()
	time.Sleep(7 * time.Second) // 훅이 살아 있었다면 끝나 push가 들어갈 시간
	hp := strings.TrimSpace(read(mark))
	hookGone := hp != "" && !alive(atoiT(hp))
	k.check("M1 land: push 도중 TERM이면 push·훅이 함께 끝나고 origin/main 그대로",
		got && hookGone && k.git(k.D+"/origin.git", "rev-parse", "main") == before)
	os.Remove(hooks + "/pre-push")
}

// M2: rebase 중 느린 병합 드라이버 — 작업 트리를 바꾸는 rebase는 로컬 git 상한(HERDR_TIMEOUT_GIT)으로 끊지 않는다(셸 판과 같게 착지)
func gfSlowRebase(k *K) {
	k.mk("gfm2")
	k.seed(".claude/herdr-verify.sh", "exit 0")
	k.seed("m.txt", "1\n2\n3\n4\n5\n6\n7")
	k.git(k.R, "pull", "-q", "--ff-only")
	k.run("--name", "gfm2l", "--repo", k.R, "--", "true")
	k.wt = k.R + ".wt/gfm2l"
	k.sh(k.wt, `printf 'L\n2\n3\n4\n5\n6\n7\n' >m.txt && git commit -qam lane`)
	k.seedp("m.txt", "1\n2\n3\n4\n5\n6\nM")
	cd := k.git(k.R, "rev-parse", "--path-format=absolute", "--git-common-dir")
	write(k.t, cd+"/info/attributes", "m.txt merge=slow\n")
	k.git(k.R, "config", "merge.slow.driver", "sleep 4; git merge-file %A %O %B")
	k.lndE([]string{"HC_LANE=gfm2l", "HERDR_TIMEOUT_GIT=2"}, k.wt, "main")
	k.check("M2 land: 느린 병합 드라이버 rebase도 끊지 않고 착지(종료 0)", k.rc == 0 && !exists(k.git(k.wt, "rev-parse", "--git-dir")+"/rebase-merge"))
	os.Remove(cd + "/info/attributes")
}

// M3: gc의 로컬 git이 멈추면 상한(HERDR_TIMEOUT_GIT) 안에서 끝나고 모름 — 아무것도 지우지 않고 종료 1
func gfGcStuckGit(k *K) {
	k.mk("gfm3")
	k.sh(k.R, `git branch old1`)
	p := k.gfBin(map[string]string{"git": `for a; do [ "$a" = symbolic-ref ] && exec sleep 40; done; exec ` + sq(k.gfRealGit()) + ` "$@"`})
	t0 := time.Now()
	k.runE([]string{p, "HERDR_TIMEOUT_GIT=1"}, "gc", "--repo", k.R, "--min-age", "0")
	k.goOnly("M3 gc: 멈춘 git은 상한 안에서 끊고 종료 1, 안 지움", time.Since(t0) < 20*time.Second && k.rc == 1 && k.gitOK(k.R, "rev-parse", "-q", "--verify", "refs/heads/old1"))
}

// M4: wait 바퀴의 hc ls가 멈추면 상한(HERDR_TIMEOUT_REMOTE) 안에서 끝난다(--timeout 0은 한 바퀴)
func gfWaitStuckHc(k *K) {
	if k.env["HC_LANE_IMPL"] != "go" { // 셸 판은 hc ls에 상한이 없다 — 30초를 다 기다리니 돌리지 않는다(알려진 차이)
		k.t.Logf("알려진 차이(셸 판 안 고침) M4: 셸 판 wait의 hc ls에는 상한이 없다")
		return
	}
	p := k.gfBin(map[string]string{"hc": `case $1 in ls) exec sleep 30 ;; get) echo '{"address":"t4m","name":"boss","pid":4242,"state":"live"}'; exit 0 ;; esac; exit 3`})
	t0 := time.Now()
	k.runE([]string{p, "HERDR_TIMEOUT_REMOTE=1", "HC_TERMINAL_ID=t4m", "XDG_STATE_HOME=" + filepath.Join(k.T, "wst-m4")}, "wait", "--timeout", "0")
	k.check("M4 wait --timeout 0: 멈춘 hc ls는 상한 안에서 끊고 돌아온다", time.Since(t0) < 15*time.Second && k.rc != 0)
}

// M01: 착지의 바깥 명령이 시간 상한에 걸리면 모름 — 종료 1(실패)이 아니라 75(다시), 아무것도 보내지 않음
func gfLandTimeout75(k *K) {
	k.mk("gfm0")
	k.seed(".claude/herdr-verify.sh", "exit 0")
	k.git(k.R, "pull", "-q", "--ff-only")
	k.mklane("gfm0l")
	before := k.git(k.D+"/origin.git", "rev-parse", "main")
	p := k.gfBin(map[string]string{"git": `for a; do [ "$a" = fetch ] && exec sleep 40; done; exec ` + sq(k.gfRealGit()) + ` "$@"`})
	k.lndE([]string{p, "HC_LANE=gfm0l", "HERDR_TIMEOUT_REMOTE=1", "HC_LANE_FETCH_WAITS=-"}, k.wt, "main")
	k.goOnly("M01 land: fetch 시간 넘김은 종료 75, origin/main 그대로", k.rc == 75 && k.git(k.D+"/origin.git", "rev-parse", "main") == before)
}

func atoiT(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
