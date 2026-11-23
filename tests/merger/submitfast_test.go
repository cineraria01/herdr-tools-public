package merger_test

// submit-fast(2026-11-24): 제출 길의 남은 중복 둘.
//   - 레인 작업 트리에서 전체 검증이 통과한 트리를 제출하면 사전 검사(.claude/herdr-precheck.sh)를 다시 돌리지 않는다 — 검증 본문
//     (tests/repo/verify)이 통과 때 남긴 표식(작업 트리의 git 폴더 herdr-verified-tree: 「<트리> <초>」)과 제출 커밋의 트리가 같을 때만.
//     표식은 병합기·착지가 읽지 않는다(병합기 검증 횟수 그대로).
//   - 병합기 빈 바퀴 쉼의 기본값이 짧다(제출 → 집어 듦이 쉼 대부분이었다).

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

const pcScript = `echo pc >>"$HOME/pc.log"`

// sfFix: 사전 검사가 불린 횟수를 $HOME/pc.log에, 병합기 검증 횟수를 $HOME/v.log에 남기는 fixture(켬 표식 있음)
func sfFix(k *K, name string) *F {
	f := k.fixture(name)
	f.enable()
	f.commitFile(".claude/herdr-precheck.sh", pcScript)
	f.commitFile(".claude/herdr-verify.sh", `echo v >>"$HOME/v.log"`)
	f.commitFile("payload", "yes")
	return f
}

// mark: writer 작업 트리의 통과 표식에 트리 값을 적는다(검증 본문이 남기는 꼴)
func (f *F) mark(tree string) {
	f.t.Helper()
	f.w(`printf '%s %s\n' "$1" "$(date +%s)" >"$(git rev-parse --git-path herdr-verified-tree)"`, tree)
}

func (f *F) pcRuns() int { return countNonEmpty(read(f.home + "/pc.log")) }

func TestSubmitFastPrecheckSkip(t *testing.T) {
	k := newRK(t)

	t.Run("① 통과한 같은 트리 — 사전 검사 0번, ⑦ 병합기 검증은 그대로 1번", func(t *testing.T) {
		f := sfFix(k, "sf-same")
		f.mark(f.g("rev-parse", "HEAD^{tree}"))
		r := f.submit("lane")
		k.check("제출 종료 0·큐에 들어감", r.RC == 0 && f.refs("refs/merge-queue") != "", clog(r))
		k.check("사전 검사 0번", f.pcRuns() == 0, clog(r))
		k.check("건너뜀 한 줄", strings.Contains(r.Out+r.Err, "사전 검사 건너뜀"), clog(r))
		r = f.tick()
		k.check("병합기 한 바퀴 종료 0·main에 들어감", r.RC == 0 && f.og("show", "main:payload") == "yes", clog(r))
		k.check("⑦ 표식이 있어도 병합기 검증 1번", countNonEmpty(read(f.home+"/v.log")) == 1, read(f.home+"/v.log"), clog(r))
	})

	t.Run("② 통과 뒤 커밋을 더함(트리 다름) — 사전 검사 1번", func(t *testing.T) {
		f := sfFix(k, "sf-moved")
		f.mark(f.g("rev-parse", "HEAD^{tree}"))
		f.commitFile("payload2", "more")
		r := f.submit("lane")
		k.check("제출 종료 0", r.RC == 0, clog(r))
		k.check("사전 검사 1번", f.pcRuns() == 1, clog(r))
	})

	t.Run("⑥ 깨진 표식·표식 읽기 실패 — 사전 검사 1번", func(t *testing.T) {
		f := sfFix(k, "sf-garbage")
		f.w(`printf 'not-a-tree\n' >"$(git rev-parse --git-path herdr-verified-tree)"`)
		r := f.submit("lane")
		k.check("깨진 표식: 제출 종료 0·사전 검사 1번", r.RC == 0 && f.pcRuns() == 1, clog(r))

		g := sfFix(k, "sf-unreadable")
		g.w(`mkdir "$(git rev-parse --git-path herdr-verified-tree)"`) // 파일이 아니라 폴더 — 읽기 실패
		r = g.submit("lane")
		k.check("읽기 실패: 제출 종료 0·사전 검사 1번", r.RC == 0 && g.pcRuns() == 1, clog(r))
	})

	t.Run("끄는 손잡이 — 같은 트리여도 사전 검사 1번", func(t *testing.T) {
		f := sfFix(k, "sf-off")
		f.mark(f.g("rev-parse", "HEAD^{tree}"))
		r := f.submit("lane", "HC_LANE_PRECHECK_ALWAYS=1")
		k.check("제출 종료 0·사전 검사 1번", r.RC == 0 && f.pcRuns() == 1, clog(r))
	})
}

// ⑧ 빈 바퀴 쉼의 기본값(--tick을 안 줌)은 옛 30초보다 짧다 — 쉼마다 PATH의 가짜 sleep이 값을 적고, 첫 쉼에서 병합기를 끈다
func TestSubmitFastIdleTick(t *testing.T) {
	w := newWR(t, "sf-tick", false)
	w.enable()
	w.sleepStub(term)
	r := w.exe(w.writer, nil, 2*time.Minute, w.inst+"/hc-lane", "merger", "--repo", w.mclone, "main")
	d := "rc=" + strconv.Itoa(r.RC) + "\n" + r.Err + "\nsleep:\n" + read(w.slog)
	s := w.sleeps()
	w.check("TERM으로만 끝났다(143)", r.RC == 143, d)
	n := -1
	if len(s) == 1 {
		n, _ = strconv.Atoi(strings.TrimPrefix(s[0], "1 "))
	}
	w.check("빈 바퀴 쉼 한 번, 1~29초", n >= 1 && n < 30, d)
}
