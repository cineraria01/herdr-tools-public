package hclane_test

// gofix-lane T1(감사 Fable T1): hc-lane 길의 시간 상한·신호 계약 — 상한이 걸린 결과는 「모름」(상태를 안 바꾸고 다시), 신호로 끄면
// 상한 때문에 따로 무리가 된 자식(git push 등)도 함께 끝난다.

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestGofixLimits(t *testing.T) {
	withK(t, func(k *K) {
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
	// 훅은 시작 표식을 쓰고, 살아남으면 6초 뒤 늦은 표식을 쓴다 — 훅의 PID를 파일로 읽어 생존을 보지 않는다(②-F 시험 규칙, R-213 꼴)
	mark, late := filepath.Join(k.T, "m1-hook"), filepath.Join(k.T, "m1-hook-late")
	os.Remove(mark)
	os.Remove(late)
	hooks := k.git(k.R, "rev-parse", "--path-format=absolute", "--git-common-dir") + "/hooks"
	write(k.t, hooks+"/pre-push", "#!/bin/sh\n: >"+sq(mark)+"\nsleep 6\n: >"+sq(late)+"\nexit 0\n")
	os.Chmod(hooks+"/pre-push", 0o755)
	before := k.git(k.D+"/origin.git", "rev-parse", "main")
	b := k.bg(k.wt, filepath.Join(k.T, "m1o"), filepath.Join(k.T, "m1e"), []string{"HC_LANE=gfm1l"}, k.LANE, "land", "main")
	got := upto(func() bool { return exists(mark) })
	b.signal(true, syscall.SIGTERM)
	b.Wait()
	time.Sleep(7 * time.Second) // 훅이 살아 있었다면 끝나 push가 들어갈 시간
	hookGone := exists(mark) && !exists(late)
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
	// 상한은 착지의 다른 git에도 걸리고 proc의 띄우기 준비(ps·고아 기록)도 그 안에서 세니 부하(조각 여럿 -race)에 넉넉히 3초, 드라이버는 그보다 긴 5초
	// (2026-11-23 shard-load-flaky)
	k.git(k.R, "config", "merge.slow.driver", "sleep 5; git merge-file %A %O %B")
	k.lndE([]string{"HC_LANE=gfm2l", "HERDR_TIMEOUT_GIT=3"}, k.wt, "main")
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
	k.check("M3 gc: 멈춘 git은 상한 안에서 끊고 종료 1, 안 지움", time.Since(t0) < 20*time.Second && k.rc == 1 && k.gitOK(k.R, "rev-parse", "-q", "--verify", "refs/heads/old1"))
}

// M4: wait 바퀴의 hc ls가 멈추면 상한(HERDR_TIMEOUT_REMOTE) 안에서 끝난다(--timeout 0은 한 바퀴)
func gfWaitStuckHc(k *K) {
	// R-33: 조회가 정말 시작됐는지(시작 표식)·기대 종료 75·미확정 사유를 본다. R-23: 상한에 걸린 조회의 손자(배경 subshell)도 함께 거둔다
	// R-222: 손자 찾기(ps -A -o pid= -o ppid=)는 가짜 ps 등록부로 — 가짜 hc와 그 손자가 스스로 「pid ppid」를 올린다(호스트 ps 가시성과 무관).
	// 경로는 따로 된 인자로 넘긴다
	st, gs, gc, reg := filepath.Join(k.T, "m4-start"), filepath.Join(k.T, "m4-gc-start"), filepath.Join(k.T, "m4-grandchild"), filepath.Join(k.T, "m4-ps.reg")
	p := k.gfBin(map[string]string{"hc": `case $1 in ls) printf '%s\t%s\talive\thc ls\n' $$ $PPID >>` + sq(reg) + `; : >` + sq(st) +
		`; sh -c 'printf "%s\t%s\talive\tsh\n" $$ $PPID >>"$1"; : >"$2"; sleep 3; : >"$3"' sh ` + sq(reg) + " " + sq(gs) + " " + sq(gc) +
		` & exec sleep 30 ;; get) echo '{"address":"t4m","name":"boss","pid":4242,"state":"live"}'; exit 0 ;; esac; exit 3`})
	// 가짜 hc ls와 손자가 표식을 쓴 뒤 상한(1초)에 걸려야 이 꼴이다 — proc의 띄우기 준비(ps·고아 기록)도 그 1초 안에서 세니 부하(조각 여럿
	// -race)에서는 표식 전에 거둬질 수 있다. 그때는 준비가 안 된 것이라 다시 준비한다(5번까지 — 2026-11-23 shard-load-flaky)
	var el time.Duration
	for try := 0; try < 5 && (try == 0 || !exists(st) || !exists(gs)); try++ {
		for _, f := range []string{st, gs, gc} {
			os.Remove(f)
		}
		write(k.t, reg, "")
		t0 := time.Now()
		k.runE([]string{p, "HC_FAKE_PS_REG=" + reg, "HERDR_TIMEOUT_REMOTE=1", "HC_TERMINAL_ID=t4m", "XDG_STATE_HOME=" + filepath.Join(k.T, "wst-m4")}, "wait", "--timeout", "0")
		el = time.Since(t0)
	}
	k.check("M4 wait --timeout 0: 멈춘 hc ls는 상한 안에서 끊고 75(모름)로 돌아온다", exists(st) && el < 15*time.Second && k.rc == 75 && strings.Contains(k.errf(), "hc ls"))
	time.Sleep(5 * time.Second) // 손자가 살아 있었다면 표식을 쓸 시간
	k.check("R-23 wait: 상한에 걸린 hc ls의 손자도 거둬 표식이 안 생긴다(손자 시작·등록부 조회를 먼저 확인)",
		exists(gs) && strings.Contains(read(k.HCLOG+".ps"), "-A -o pid= -o ppid=") && !exists(gc))
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
	k.check("M01 land: fetch 시간 넘김은 종료 75, origin/main 그대로", k.rc == 75 && k.git(k.D+"/origin.git", "rev-parse", "main") == before)
}
