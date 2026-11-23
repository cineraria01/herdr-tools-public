//go:build !windows

package merger_test

// 재감사 4바퀴 merger/R-4M01 · lane/R-4M01: 종료 0으로 끝난 검증·착지 훅·제출 사전 검사가 남긴 배경 프로세스가 출력 관을 유예(10초)
// 넘게 쥐어도 우두머리의 종료 코드(0)를 그대로 쓴다 — 통과. 옛 판(gofix4)은 124(모름) → lfail의 report가 lastTimeout을 지워 land 1 →
// 병합기가 통과한 제출을 반송했다

import (
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// holdScript: 출력 관을 30초 쥐는 배경 sleep을 남기고 0으로 끝나는 훅 본문(그 pid는 $HOME/bg에 — 시험이 거둔다)
const holdScript = `sleep 30 & echo $! >>"$HOME/bg"; echo 검사 통과`

// reapBg: 훅이 남긴 배경 sleep(자기 시험이 띄운 것만, $HOME/bg의 pid)을 거둔다
func reapBg(f *F) {
	for _, l := range strings.Fields(read(f.home + "/bg")) {
		if p, err := strconv.Atoi(l); err == nil && p > 1 {
			syscall.Kill(p, syscall.SIGKILL)
		}
	}
}

func TestR4PipeHeldExitZero(t *testing.T) {
	k := newRK(t)

	t.Run("제출 사전 검사", func(t *testing.T) {
		f := k.fixture("held-precheck")
		t.Cleanup(func() { reapBg(f) })
		f.enable()
		f.commitFile(".claude/herdr-precheck.sh", holdScript)
		f.commitFile("payload", "yes")
		r := f.submit("lane")
		k.check("제출 종료 0(사전 검사가 0이면 큐에)", r.RC == 0, clog(r))
		k.check("큐에 들어감", f.refs("refs/merge-queue") != "", clog(r))
	})

	t.Run("병합기 — 검증", func(t *testing.T) {
		f := k.fixture("held-merger")
		t.Cleanup(func() { reapBg(f) })
		f.enable()
		f.commitFile(".claude/herdr-verify.sh", holdScript)
		f.commitFile("payload", "yes")
		r := f.submit("lane")
		k.check("제출 종료 0", r.RC == 0, clog(r))
		r = f.tick()
		k.check("병합기 한 바퀴 종료 0", r.RC == 0, clog(r))
		k.check("main에 들어감", f.og("show", "main:payload") == "yes", clog(r))
		k.check("반송 없음", f.refs("refs/merge-failed") == "", f.refs("refs/merge-failed"))
		k.check("착지 사건 1", f.nev("lane", "착지") == 1, read(f.home+"/.local/state/hc-lane/events.tsv"))
	})

	t.Run("직접 착지 — 검증", func(t *testing.T) {
		f := k.fixture("held-direct")
		t.Cleanup(func() { reapBg(f) })
		f.commitFile(".claude/herdr-verify.sh", holdScript)
		f.commitFile("payload", "yes")
		r := f.submit("lane")
		k.check("land 종료 0", r.RC == 0, clog(r))
		k.check("main에 들어감", f.og("show", "main:payload") == "yes", clog(r))
		k.check("경고 한 줄", strings.Contains(r.Err, "출력 관을"), r.Err)
	})

	t.Run("직접 착지 — 착지 훅", func(t *testing.T) {
		f := k.fixture("held-hook")
		t.Cleanup(func() { reapBg(f) })
		f.commitFile(".claude/herdr-land.sh", `git push -q origin HEAD:"$1" || exit 1; `+holdScript)
		f.commitFile("payload", "yes")
		r := f.submit("lane")
		k.check("land 종료 0", r.RC == 0, clog(r))
		k.check("main에 들어감", f.og("show", "main:payload") == "yes", clog(r))
	})

	t.Run("0 아닌 종료는 그 코드", func(t *testing.T) {
		f := k.fixture("held-fail")
		t.Cleanup(func() { reapBg(f) })
		f.commitFile(".claude/herdr-verify.sh", holdScript+"; exit 3")
		f.commitFile("payload", "yes")
		r := f.submit("lane")
		k.check("land 실패(0 아님·75 아님)", r.RC != 0 && r.RC != 75, clog(r))
		k.check("main에 안 들어감", f.og("show", "main:payload") != "yes", clog(r))
	})
}
