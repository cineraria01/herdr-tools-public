package merger_test

// 복구 시험 사례 둘째 묶음(옛 hc-lane-merger-recovery.test.sh) — 사례 함수 하나가 Test 하나, 판정 줄 하나가 k.check 하나.
// 셸 사례의 맨 명령(set -e라 실패하면 사례가 멈추던 준비)은 bok로 멈춘다.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/clitest"
)

// bok: 셸 set -e의 맨 명령 — 실패면 시험을 멈춘다
func (f *F) bok(what string, r R) {
	f.t.Helper()
	if r.RC != 0 {
		f.t.Fatalf("준비 실패(rc %d): %s\n%s\n%s", r.RC, what, r.Out, r.Err)
	}
}

// bland [VAR=값…]: writer에서 hc-lane land main --verdict a1
func (f *F) bland(extra ...string) R {
	return f.lane(f.writer, extra, "land", "main", "--verdict", "a1")
}

// bverdict <sha> <판정>: $HOME/verdicts/lane/a1.md(셸 printf 'aid\ta1\n…') — 그 폴더를 돌려준다
func (f *F) bverdict(sha, verdict string) string {
	d := f.home + "/verdicts/lane"
	write(f.t, d+"/a1.md", fmt.Sprintf("aid\ta1\nlane\tlane\nrun\trun-1\nsha\t%s\nverdict\t%s\nbase\tmain\n", sha, verdict))
	return d
}

// bstub <이름> <본문>: $HOME/stub/<이름>
func (f *F) bstub(name, body string) { f.t.Helper(); f.stub(f.home+"/stub/"+name, body) }

// bid <커밋>: 제목의 id= 값(셸 og show -s --format=%s … | tr ' ' '\n' | sed -n 's/^id=//p')
func (f *F) bid(c string) string {
	var r []string
	for _, w := range strings.Fields(f.og("show", "-s", "--format=%s", c)) {
		if v, ok := strings.CutPrefix(w, "id="); ok {
			r = append(r, v)
		}
	}
	return strings.Join(r, "\n")
}

// bcount: 줄 가운데 조건이 맞는 수(awk … {n++})
func bcount(rs [][]string, ok func([]string) bool) int {
	n := 0
	for _, r := range rs {
		if ok(r) {
			n++
		}
	}
	return n
}

// bcut2: cut -f2
func bcut2(p string) string {
	var r []string
	for _, row := range rows(p) {
		r = append(r, col(row, 2))
	}
	return strings.Join(r, "\n")
}

func bisFile(p string) bool { st, err := os.Stat(p); return err == nil && st.Mode().IsRegular() }

// approved_submit: 고침 판정은 제출하지 않고, 통과 판정 제출은 다시 불러도 같은 접수이며 묶음에 담겨 착지한다
func TestRecoveryApprovedSubmit(t *testing.T) {
	k := newRK(t)
	f := k.fixture("approved")
	f.enable()
	f.commitFile("approved", "yes")
	k.set("HC_FILL_VERDICTS="+f.home+"/verdicts", "HC_LANE_JOB=run-1")
	approved := f.g("rev-parse", "HEAD")
	for _, verdict := range []string{"fix", "pass"} {
		f.bverdict(approved, verdict)
		if verdict == "fix" {
			r := f.bland("HC_LANE=lane")
			k.check("fix: land 실패", r.RC != 0, r.Out, r.Err)
			k.check("fix: 큐 빔", f.refs("refs/merge-queue") == "")
		}
	}
	f.w(`git checkout -qb another main`)
	f.commitFile("other", "yes")
	f.bok("submit alpha", f.submit("alpha"))
	f.w(`git checkout -q lane`)
	vd := f.home + "/verdicts/lane"
	f.bok("승인 land", f.bland("HC_LANE=lane"))
	k.check("ack가 landing", grepE(read(vd+"/a1.ack"), "^landing"), read(vd+"/a1.ack"))
	receipt := f.og("for-each-ref", "--format=%(objectname)", "refs/merge-queue-meta/*-lane")
	f.bok("승인 land 다시", f.bland("HC_LANE=lane"))
	k.check("접수 그대로", f.og("for-each-ref", "--format=%(objectname)", "refs/merge-queue-meta/*-lane") == receipt)
	r := f.tick()
	f.bok("tick", r)
	k.check("approved 착지", f.og("show", "main:approved") == "yes", r.Out, r.Err)
	k.check("other 착지", f.og("show", "main:other") == "yes")
	logs, _ := filepath.Glob(f.home + "/.local/state/hc-lane/merger-logs/*.log")
	all := ""
	for _, l := range logs {
		all += read(l)
	}
	k.check("병합기 로그에 구성원 승인 확인", grepE(all, `묶음 구성원 승인 확인됨 — lane 제출 ID .* 판정 a1 patch-id `), all)
	k.check("일지에 묶음 2 승인 착지 하나", bcount(f.journal(), func(r []string) bool {
		return col(r, 3) == "lane" && col(r, 7) == "2" && col(r, 10) == "착지" && grepE(col(r, 12), `승인 확인 판정 a1 patch-id `)
	}) == 1)
	k.check("착지 사건 hf:a1:landed 하나", bcount(f.events(), func(r []string) bool {
		return col(r, 4) == "lane" && col(r, 5) == "착지" && col(r, 12) == "hf:a1:landed"
	}) == 1)
}

// interrupted_cleanup: 보낼 편지를 옮기다 끊겨도 다음 바퀴가 착지 증명과 사건을 한 번만 남긴다
func TestRecoveryInterruptedCleanup(t *testing.T) {
	k := newRK(t)
	f := k.fixture("already")
	f.enable()
	f.commitFile("done", "yes")
	f.bok("submit", f.submit("lane"))
	f.w(`git push -q origin HEAD:main`)
	f.bstub("mv", `#!/bin/sh
case $* in *hc-merger/out/*) touch "$HOME/interrupted"; exit 1;; esac
exec /bin/mv "$@"
`)
	r := f.tick(f.stubPath())
	k.check("mv 끊김", exists(f.home+"/interrupted"), r.Out, r.Err)
	k.check("rc 75", r.RC == 75, r.Out, r.Err)
	k.check("큐 빔", f.refs("refs/merge-queue") == "")
	f.bok("tick", f.tick())
	k.check("착지 증명", f.refs("refs/hc-landed/lane/") != "")
	k.check("착지 사건 하나", f.nev("lane", "착지") == 1)
}

// withdrawn_candidate: 원자 push 거절 뒤 제출이 물려 있으면 착지 영수증·사건을 남기지 않는다
func TestRecoveryWithdrawnCandidate(t *testing.T) {
	k := newRK(t)
	f := k.fixture("withdrawn")
	f.enable()
	f.commitFile("work", "first")
	f.bok("submit", f.submit("lane"))
	f.bstub("git", `#!/bin/sh
case "$1 $*" in
  "push "*":refs/heads/main "*":refs/merge-queue/"*)
    "$real_git" push -q origin HEAD:main || exit 1
    for ref in $("$real_git" --git-dir="$HOME/origin.git" for-each-ref --format='%(refname)' refs/merge-queue refs/merge-queue-meta); do
      "$real_git" --git-dir="$HOME/origin.git" update-ref -d "$ref" || exit 1
    done
    touch "$HOME/withdrawn"
    exit 1 ;;
esac
exec "$real_git" "$@"
`)
	r := f.tick(f.stubPath())
	f.bok("tick", r)
	k.check("물림 끼움", exists(f.home+"/withdrawn"), r.Out, r.Err)
	k.check("영수증·증명 없음", f.refs("refs/merge-done", "refs/hc-landed/lane") == "")
	k.check("착지 사건 없음", f.nev("lane", "착지") == 0)
}

// batch_queries: 묶음 정책·이력·병합 트리 조회가 실패하면 75로 미루고 큐를 남긴다
func TestRecoveryBatchQueries(t *testing.T) {
	k := newRK(t)
	f := k.fixture("policy")
	f.w(`git checkout -q main`)
	f.commitFile(".claude/herdr-merge-solo", "fragments")
	f.w(`git push -q origin main`)
	f.enable()
	for _, name := range []string{"alpha", "zulu"} {
		f.w(`git checkout -q -B "$1" main && mkdir -p fragments`, name)
		f.commitFile("fragments/"+name, "yes")
		f.bok("submit "+name, f.submit(name))
	}
	f.bstub("git", `#!/bin/sh
case "$fault $1 $*" in
  "policy show "*:.claude/herdr-merge-solo|"history rev-list "*" -- fragments"|"tree merge-tree "*)
    touch "$HOME/injected"; exit 128 ;;
esac
exec "$real_git" "$@"
`)
	for _, fault := range []string{"policy", "history", "tree"} {
		k.set("fault=" + fault)
		r := f.tick(f.stubPath())
		k.check(fault+": 실패 끼움", exists(f.home+"/injected"), r.Out, r.Err)
		k.check(fault+": rc 75", r.RC == 75, r.Out, r.Err)
		k.check(fault+": main 그대로", f.og("rev-parse", "main") == f.g("rev-parse", "main"))
		k.check(fault+": 큐 둘", countNonEmpty(f.og("for-each-ref", "--format=x", "refs/merge-queue")) == 2)
		os.Remove(f.home + "/injected")
	}
	r := f.tick()
	f.bok("tick", r)
	k.check("zulu 착지", f.og("show", "main:fragments/zulu") == "yes", r.Out, r.Err)
}

// stale_hook_query: 갈래점 조회가 실패하면 옛 착지 훅을 돌리지 않고, 다시 하면 새 훅을 돌린다
func TestRecoveryStaleHookQuery(t *testing.T) {
	k := newRK(t)
	f := k.fixture("stale")
	f.w(`git checkout -q main`)
	f.commitFile(".claude/herdr-land.sh", `touch "$HOME/old"`)
	f.w(`git push -q origin main && git checkout -q -B lane main`)
	f.commitFile("work", "yes")
	f.w(`git checkout -q main`)
	f.commitFile(".claude/herdr-land.sh", `touch "$HOME/new"`)
	f.w(`git push -q origin main && git checkout -q lane`)
	f.bstub("git", `#!/bin/sh
case "$*" in "merge-base HEAD "*) touch "$HOME/injected"; exit 128;; esac
exec "$real_git" "$@"
`)
	r := f.lane(f.writer, []string{f.stubPath()}, "land", "main")
	k.check("land 실패", r.RC != 0, r.Out, r.Err)
	k.check("실패 끼움", exists(f.home+"/injected"))
	k.check("옛 훅 안 돎", !exists(f.home+"/old"))
	f.bok("land", f.lane(f.writer, nil, "land", "main"))
	k.check("새 훅 돎", exists(f.home+"/new"))
}

// submit_retry: 제출 응답을 잃어도 다시 제출하면 같은 제출 ID 하나로 남는다(큐에 있든 이미 착지했든)
func TestRecoverySubmitRetry(t *testing.T) {
	k := newRK(t)
	for _, state := range []string{"queued", "landed"} {
		f := k.fixture("submit-" + state)
		f.enable()
		f.commitFile("payload", "yes")
		f.bstub("git", `#!/bin/sh
case "$1 $*" in "ls-remote "*refs/merge-failed-meta/*)
  touch "$HOME/interrupted"; exit 128 ;;
esac
exec "$real_git" "$@"
`)
		r := f.submit("lane", f.stubPath())
		k.check(state+": 제출 실패", r.RC != 0, r.Out, r.Err)
		k.check(state+": 끊김 끼움", exists(f.home+"/interrupted"))
		meta := f.og("for-each-ref", "--format=%(objectname)", "refs/merge-queue-meta")
		id := f.bid(meta)
		k.check(state+": 제출 ID 있음", id != "")
		if state != "queued" {
			f.bok("tick", f.tick())
		}
		f.bok("submit", f.submit("lane"))
		f.bok("submit 다시", f.submit("lane"))
		sub := read(f.writer + "/.git/hc-submitted")
		k.check(state+": 제출 기록", hasLine(sub, "lane "+f.g("rev-parse", "HEAD")+" main"), sub)
		k.check(state+": 제출 사건 하나", bcount(f.events(), func(r []string) bool {
			return col(r, 4) == "lane" && col(r, 5) == "제출" && strings.HasPrefix(col(r, 7), id+" ")
		}) == 1)
		k.check(state+": 접수 그대로", f.og("for-each-ref", "--format=%(objectname)", "refs/merge-queue-meta", "refs/merge-done") == meta)
	}
}

// binterrupt: 셸 interrupt_candidate — 병합기를 후보 검증 중에 SIGKILL하고, 살아 있는 후보가 끝날 때까지 75로 미루다 착지
func (f *F) binterrupt(mode string, extra ...string) {
	t := f.t
	t.Helper()
	cmd := f.mergerCmd(f.writer, extra, f.home+"/first.log", "--repo", f.mclone, "--once", "main")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	release := f.home + "/release"
	t.Cleanup(func() {
		os.WriteFile(release, nil, 0o644)
		if !waited {
			cmd.Wait()
		}
	})
	cand := f.home + "/candidate"
	waitFor(300, 50*time.Millisecond, func() bool { return read(cand) != "" })
	f.check(mode+": 후보 경로 남김", read(cand) != "", read(f.home+"/first.log"))
	cmd.Process.Kill()
	cmd.Wait()
	waited = true
	lock := f.home + "/.local/state/hc-lane/merger.*.lock"
	if m, _ := filepath.Glob(lock); len(m) > 0 {
		lock = m[0]
	}
	free := tryLock(lock)
	r := f.tick(extra...)
	present := bisFile(strings.TrimSpace(read(cand)) + "/.git")
	os.WriteFile(release, nil, 0o644)
	var retry R
	for i := 0; i < 300; i++ {
		retry = f.tick(extra...)
		if retry.RC != 75 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	f.check(mode+": 살아 있는 후보에 rc 75", r.RC == 75, r.Out, r.Err)
	f.check(mode+": 병합기 잠금 풀림", free)
	f.check(mode+": 후보 작업 트리 남음", present)
	f.check(mode+": 놓은 뒤 착지 rc 0", retry.RC == 0, retry.Out, retry.Err)
	f.check(mode+": 큐 빔", f.refs("refs/merge-queue") == "")
}

// live_candidate: 죽은 병합기의 후보 검증이 아직 돌면 그 후보를 지우지 않고 기다렸다가 「앞 병합기 실행」으로 착지한다
func TestRecoveryLiveCandidate(t *testing.T) {
	k := newRK(t)
	f := k.fixture("live")
	f.enable()
	write(t, f.writer+"/.claude/herdr-verify.sh", `if mkdir "$HOME/first" 2>/dev/null; then
  pwd -P >"$HOME/candidate"
  perl -e 'for (1..600) { last if -e "$ENV{HOME}/release"; select undef,undef,undef,0.05 }'
  test -f .git
fi
`)
	f.w(`git commit -qam '검증 대기'`)
	f.bok("submit", f.submit("lane"))
	f.binterrupt("검증")
	k.check("앞 병합기 실행 착지 하나", bcount(f.events(), func(r []string) bool {
		return col(r, 5) == "착지" && strings.Contains(col(r, 7), "앞 병합기 실행")
	}) == 1)
}

// live_worktree: 후보 작업 트리를 만든 직후 병합기가 죽어도 같다
func TestRecoveryLiveWorktree(t *testing.T) {
	k := newRK(t)
	f := k.fixture("live-worktree")
	f.enable()
	f.commitFile("payload", "yes")
	f.bok("submit", f.submit("lane"))
	f.bstub("git", `#!/bin/bash
if [ "$1 $2" = 'worktree add' ] && mkdir "$HOME/first" 2>/dev/null; then
  "$real_git" "$@" || exit 1
  printf '%s\n' "${@: -2:1}" >"$HOME/candidate"
  perl -e 'for (1..600) { last if -e "$ENV{HOME}/release"; select undef,undef,undef,0.05 }'
  exit 0
fi
exec "$real_git" "$@"
`)
	f.binterrupt("작업 트리", f.stubPath())
}

// git_lock_fd: 병합기가 부르는 git에 병합기 잠금(fd 19)이 물리지 않는다
func TestRecoveryGitLockFD(t *testing.T) {
	k := newRK(t)
	f := k.fixture("fd")
	f.enable()
	f.bstub("git", `#!/bin/bash
if [ "$1" = ls-remote ]; then
  touch "$HOME/called"
  if ( : >&19 ) 2>/dev/null; then touch "$HOME/inherited"; fi
fi
exec "$real_git" "$@"
`)
	r := f.tick(f.stubPath())
	f.bok("tick", r)
	k.check("ls-remote 불림", exists(f.home+"/called"), r.Out, r.Err)
	k.check("fd 19 안 물림", !exists(f.home+"/inherited"))
}

// candidate_checkout: 후보 가지 checkout이 실패하면 75로 미루고 큐 둘을 남긴다
func TestRecoveryCandidateCheckout(t *testing.T) {
	k := newRK(t)
	f := k.fixture("checkout")
	f.enable()
	for _, name := range []string{"alpha", "zulu"} {
		f.w(`git checkout -q -B "$1" main`, name)
		f.commitFile(name, "yes")
		f.bok("submit "+name, f.submit(name))
	}
	f.bstub("git", `#!/bin/sh
case "$*" in 'checkout -q -B alpha'|'checkout -q -f alpha') touch "$HOME/injected"; exit 128;; esac
exec "$real_git" "$@"
`)
	r := f.tick(f.stubPath())
	k.check("실패 끼움", exists(f.home+"/injected"), r.Out, r.Err)
	k.check("rc 75", r.RC == 75, r.Out, r.Err)
	k.check("main 그대로", f.og("rev-parse", "main") == f.g("rev-parse", "main"))
	k.check("큐 둘", countNonEmpty(f.og("for-each-ref", "--format=x", "refs/merge-queue")) == 2)
	r = f.tick()
	f.bok("tick", r)
	k.check("zulu 착지", f.og("show", "main:zulu") == "yes", r.Out, r.Err)
}

// lease_owner: 옛 임대로 하는 clean·park는 남의 새 임대를 놓지 않고, 가지 이름으로 잡은 임대는 그 가지의 clean이 놓는다
func TestRecoveryLeaseOwner(t *testing.T) {
	k := newRK(t)
	f := k.fixture("leases")
	old := f.g("rev-parse", "main")
	f.commitFile("next", "new")
	other := f.g("rev-parse", "HEAD")
	f.w(`git push -q origin "$1:refs/hc-lanes/retired"`, other)
	// 옛 클론의 clean 재시도. 같은 이름의 새 실행은 다른 기계에서 이미 임대를 잡았다.
	f.bok("옛 임대 clean", f.lane(f.writer, []string{"HC_LANE=retired", "HC_LANE_LEASE=" + old}, "clean", "--repo", f.writer, "--name", "retired"))
	k.check("clean: 새 임대 그대로", f.og("rev-parse", "--verify", "refs/hc-lanes/retired") == other)
	r := f.lane(f.writer, []string{"HC_LANE=retired", "HC_LANE_LEASE=" + old}, "park")
	k.check("park: 실패", r.RC != 0, r.Out, r.Err)
	k.check("park: 주차 가지 없음", f.refs("refs/heads/lane/retired") == "")
	k.check("park: 새 임대 그대로", f.og("rev-parse", "--verify", "refs/hc-lanes/retired") == other)
	// 셸 판은 lease_core take를 source해 불렀다 — Go hc-lane에는 lease take 하위 명령이 없어(show|drop만) 같은 임대를 잡는
	// 레인 띄우기(--name owned --branch branch-alias)로 잡고, 그 임대 주인 표식 refs/hc-lease-owners/branch-alias를 본다
	f.bstub("hc", `#!/bin/sh
case $1 in run) echo '{"address":"t11"}' ;; ls) echo '{"terminals":[]}' ;; *) exit 3 ;; esac
`)
	f.bok("임대 잡기(레인 띄우기)", f.lane(f.writer, []string{f.stubPath()}, "--name", "owned", "--branch", "branch-alias", "--repo", f.writer, "--start-wait", "0", "--", "true"))
	own := f.g("rev-parse", "refs/hc-lease-owners/branch-alias")
	k.check("take: 임대 = 주인 표식", own != "" && f.og("rev-parse", "refs/hc-lanes/owned") == own, own)
	f.bok("가지 clean", f.lane(f.writer, nil, "clean", "--repo", f.writer, "--branch", "branch-alias"))
	k.check("clean: 가지 임대 놓음", f.refs("refs/hc-lanes/owned") == "")
}

// event_id: 보낼 편지를 되풀이해도 사건은 한 번만, 제출 ID가 앞부분만 같은 다른 편지와 섞지 않는다
func TestRecoveryEventID(t *testing.T) {
	k := newRK(t)
	f := k.fixture("ids")
	f.enable()
	f.bok("tick", f.tick())
	events := f.home + "/.local/state/hc-lane/events.tsv"
	box := f.mclone + "/.git/hc-merger/out"
	write(t, box+"/first", "착지\n-\n완료 — 제출 ID box-1234\nlane\n\n\n\n제출 ID box-1234\n")
	f.bok("tick first", f.tick())
	second := "착지\n-\n완료 — 제출 ID box-123\nlane\n\n\n\n제출 ID box-123\n"
	write(t, box+"/second", second)
	f.bok("tick second", f.tick())
	write(t, box+"/second", second)
	f.bok("tick replay", f.tick())
	k.check("box-123 사건 하나", bcount(rows(events), func(r []string) bool { return col(r, 7) == "완료 — 제출 ID box-123" }) == 1, read(events))
}

// parked_branch: 주차 가지가 없으면 --resume이 띄우지 않고, 있으면(refs/park/<레인>) 다른 가지 이름으로 그 커밋에서 띄운다
func TestRecoveryParkedBranch(t *testing.T) {
	k := newRK(t)
	f := k.fixture("parked")
	f.commitFile("prepared", "yes")
	f.w(`git push -q origin HEAD:refs/heads/lane/parked && git fetch -q origin`)
	f.og("update-ref", "-d", "refs/heads/lane/parked")
	f.bstub("hc", `#!/bin/sh
case $1 in run) touch "$HOME/launched"; echo '{"address":"t11"}';; ls) echo '{"terminals":[]}';; *) exit 3;; esac
`)
	r := f.lane(f.writer, []string{f.stubPath()}, "--resume", "--name", "parked", "--repo", f.writer, "--start-wait", "0", "--", "true")
	k.check("없음: 띄우기 실패", r.RC != 0, r.Out, r.Err)
	k.check("없음: hc run 안 함", !exists(f.home+"/launched"))
	k.check("없음: 작업 트리 없음", !exists(f.writer+".wt/parked"))
	// 지금 park 꼴(refs/park/<레인>)로 둔다 — 주차 사건 없는 맨 lane/<레인>은 남의 제출 가지라 재개하지 않는다(재감사 R-15)
	f.w(`git push -q origin HEAD:refs/park/parked`)
	r = f.lane(f.writer, []string{f.stubPath()}, "--resume", "--name", "parked", "--branch", "alt", "--repo", f.writer, "--start-wait", "0", "--", "true")
	f.bok("resume --branch alt", r)
	k.check("있음: hc run", exists(f.home+"/launched"), r.Out, r.Err)
	k.check("있음: 주차 커밋에서", f.git(f.writer+".wt/alt", "rev-parse", "HEAD") == f.g("rev-parse", "HEAD"))
}

// isolate_root: 시험 틀의 격리(clitest.Isolate — 셸 tests/isolate.sh)는 T 위 저장소를 git이 찾지 못하게 하고, T 아래 새 저장소는 그대로 쓴다
func TestRecoveryIsolateRoot(t *testing.T) {
	k := newRK(t)
	f := k.fixture("ceiling")
	sub := f.writer + "/build/test"
	mkdirs(t, sub+"/sub")
	clitest.Isolate(t, sub)
	ceil := []string{"GIT_CEILING_DIRECTORIES=" + os.Getenv("GIT_CEILING_DIRECTORIES")}
	r := f.exe(sub, ceil, 0, "git", "rev-parse", "--git-dir")
	k.check("T에서 바깥 저장소 못 찾음", r.RC != 0, r.Out)
	r = f.exe(sub, ceil, 0, "git", "-C", "sub", "rev-parse", "--git-dir")
	k.check("T/sub에서 바깥 저장소 못 찾음", r.RC != 0, r.Out)
	f.bok("git init sub", f.exe(sub, ceil, 0, "git", "init", "-q", "-b", "main", "sub"))
	r = f.exe(sub, ceil, 0, "git", "-C", "sub", "rev-parse", "--show-toplevel")
	k.check("새 저장소 꼭대기 = T/sub", r.Out == sub+"/sub", r.Out, r.Err)
}

// brerere: 셸 rerere_content — rerere가 푼 충돌 풀이를 승인 범위 안으로 보지 않는다(advanced: 기본 가지가 같은 파일을 다시 고친 뒤)
func brerere(t *testing.T, mode string) {
	k := newRK(t)
	f := k.fixture("resolution-" + mode)
	k.set("GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=rerere.enabled", "GIT_CONFIG_VALUE_0=true", "GIT_CONFIG_KEY_1=rerere.autoUpdate", "GIT_CONFIG_VALUE_1=true")
	f.w(`git checkout -q main`)
	f.commitFile(".claude/herdr-verify.sh", `test ! -e "$HOME/failverify"`)
	f.w(`git checkout -q -B lane main`)
	f.commitFile("shared", "lane")
	approved := f.g("rev-parse", "HEAD")
	f.w(`git checkout -q main`)
	f.commitFile("shared", "other")
	f.w(`git push -q origin main && git checkout -q lane`)
	r := f.shr(f.writer, nil, `git rebase main`)
	k.check(mode+": 준비 rebase 충돌", r.RC != 0, r.Out, r.Err)
	f.w(`printf 'lane\nother\n' >shared && git add -- shared && GIT_EDITOR=true git rebase --continue >/dev/null 2>&1 && git checkout -q -B lane "$1"`, approved)
	k.set("HC_FILL_VERDICTS="+f.home+"/verdicts", "HC_LANE=lane", "HC_LANE_JOB=run-1")
	vd := f.bverdict(approved, "pass")
	touch(t, f.home+"/failverify")
	r = f.bland()
	k.check(mode+": 검증 실패 rc 1", r.RC == 1, r.Out, r.Err)
	k.check(mode+": 검증 실패 찍음", strings.Contains(r.Out+r.Err, "검증 실패"), r.Out, r.Err)
	resolved := f.g("rev-parse", "HEAD")
	os.Remove(f.home + "/failverify")
	if mode == "advanced" {
		f.w(`git checkout -q main && echo '새 기준 내용' >>shared && git commit -qam '기본 가지에서 같은 파일을 다시 고침' && git push -q origin main`)
		base := f.og("rev-parse", "main")
		f.w(`git checkout -q -B lane main && git show "$1:shared" >shared && git commit -qam '옛 충돌 풀이를 적용'`, resolved)
		r = f.bland()
		k.check(mode+": 옛 풀이 rc 2", r.RC == 2, r.Out, r.Err)
		k.check(mode+": main 그대로", f.og("rev-parse", "main") == base)
		k.check(mode+": 승인 완료 없음", !exists(vd+"/a1.done"))
		return
	}
	f.commitFile("shared", "later-edit")
	r = f.bland()
	k.check(mode+": 뒤 수정 rc 2", r.RC == 2, r.Out, r.Err)
	k.check(mode+": main:shared = other", f.og("show", "main:shared") == "other")
	f.w(`git checkout -q -B lane "$1"`, resolved)
	r = f.bland()
	f.bok("풀이 그대로 land", r)
	k.check(mode+": 풀이 착지", f.og("rev-parse", "main") == resolved, r.Out, r.Err)
}

// rerere_content: 같은 기준에서
func TestRecoveryRerereContent(t *testing.T) { brerere(t, "same") }

// rerere_base: 기준이 나아간 뒤
func TestRecoveryRerereBase(t *testing.T) { brerere(t, "advanced") }

// approval_retry: 완료 기록을 잃은 승인 착지는 다시 해도 같은 sha로 끝내고, 승인과 무관한 커밋은 완료로 적지 않는다
func TestRecoveryApprovalRetry(t *testing.T) {
	k := newRK(t)
	for _, state := range []string{"fast_forward", "unrelated"} {
		f := k.fixture(state)
		f.commitFile(".claude/herdr-verify.sh", `test ! -e "$HOME/failverify"`)
		f.commitFile("payload", "yes")
		approved := f.g("rev-parse", "HEAD")
		k.set("HC_FILL_VERDICTS="+f.home+"/verdicts", "HC_LANE=lane", "HC_LANE_JOB=run-1")
		donefile := f.bverdict(approved, "pass") + "/a1.done"
		if state == "fast_forward" {
			f.bok("land", f.bland())
			os.Remove(donefile)
			f.bok("land 다시", f.bland())
			k.check(state+": 완료 sha = 승인", bcut2(donefile) == approved, read(donefile))
			continue
		}
		touch(t, f.home+"/failverify")
		r := f.bland()
		k.check(state+": 검증 실패로 land 실패", r.RC != 0, r.Out, r.Err)
		f.w(`git checkout -q -B lane main`)
		r = f.bland()
		k.check(state+": 무관한 커밋 land 실패", r.RC != 0, r.Out, r.Err)
		k.check(state+": 완료 없음", !exists(donefile))
	}
}

// hook_lease: 착지 훅이 임대를 바꿔 치면 main을 올리지 않는다
func TestRecoveryHookLease(t *testing.T) {
	k := newRK(t)
	f := k.fixture("hook-lease")
	write(t, f.writer+"/.claude/herdr-land.sh", `git --git-dir="$HOME/origin.git" update-ref refs/hc-lanes/lane "$replacement" "$HC_LANE_LEASE" || exit 1
git push --atomic origin HEAD:main $HERDR_LAND_PUSH_EXTRA
`)
	f.w(`git add -- .claude/herdr-land.sh && git commit -qm '전용 착지'`)
	old := f.g("rev-parse", "main")
	replacement := f.g("rev-parse", "HEAD")
	k.set("replacement=" + replacement)
	f.w(`git push -q origin "$1:refs/hc-lanes/lane" "$2:refs/heads/other"`, old, replacement)
	r := f.lane(f.writer, []string{"HC_LANE=lane", "HC_LANE_LEASE=" + old}, "land", "main")
	k.check("임대 바꿔 침", f.og("rev-parse", "--verify", "refs/hc-lanes/lane") == replacement, r.Out, r.Err)
	k.check("land 실패", r.RC != 0, r.Out, r.Err)
	k.check("main 그대로", f.og("rev-parse", "main") == old)
}

// approval_space: 승인 뒤 공백 한 칸을 바꾼 것도 승인 범위 밖(rc 2)이다
func TestRecoveryApprovalSpace(t *testing.T) {
	k := newRK(t)
	f := k.fixture("approval-space")
	f.commitFile(".claude/herdr-verify.sh", `test ! -e "$HOME/failverify"`)
	f.commitFile("안내 문구", "두 단어")
	approved := f.g("rev-parse", "HEAD")
	k.set("HC_FILL_VERDICTS="+f.home+"/verdicts", "HC_LANE=lane", "HC_LANE_JOB=run-1")
	vd := f.bverdict(approved, "pass")
	touch(t, f.home+"/failverify")
	r := f.bland()
	k.check("검증 실패로 land 실패", r.RC != 0, r.Out, r.Err)
	k.check("검증 실패 찍음", strings.Contains(r.Out+r.Err, "검증 실패"), r.Out, r.Err)
	os.Remove(f.home + "/failverify")
	f.commitFile("안내 문구", "두단어")
	r = f.bland()
	k.check("공백 바꿈 rc 2", r.RC == 2, r.Out, r.Err)
	k.check("main 그대로", f.og("rev-parse", "main") == f.g("rev-parse", "main"))
	k.check("승인 완료 없음", !exists(vd+"/a1.done"))
}

// return_proof: 돌려보냄 push 응답을 잃으면 원격으로 판정 — 물렸으면 막힘 없이, 들어갔으면 막힘 하나
func TestRecoveryReturnProof(t *testing.T) {
	k := newRK(t)
	for _, state := range []string{"withdrawn", "response_lost"} {
		f := k.fixture("return-" + state)
		f.enable()
		f.commitFile(".claude/herdr-verify.sh", "exit 1")
		f.bok("submit", f.submit("lane"))
		f.bstub("git", `#!/bin/sh
case "$1 $*" in
  "push "*":refs/merge-failed/"*)
    touch "$HOME/attempt"
    if [ "$state" = response_lost ]; then "$real_git" "$@" || exit 128
    else
      for ref in $("$real_git" --git-dir="$HOME/origin.git" for-each-ref --format='%(refname)' refs/merge-queue refs/merge-queue-meta); do
        "$real_git" --git-dir="$HOME/origin.git" update-ref -d "$ref" || exit 128
      done
    fi
    exit 128 ;;
esac
exec "$real_git" "$@"
`)
		k.set("state=" + state)
		r := f.tick(f.stubPath())
		f.bok("tick", r)
		expect := 1
		if state == "withdrawn" {
			expect = 0
		}
		k.check(state+": 돌려보냄 push 시도", exists(f.home+"/attempt"), r.Out, r.Err)
		k.check(state+": 막힘 사건 수", f.nev("lane", "막힘") == expect)
		k.check(state+": 돌려보냄 기록 수", countNonEmpty(f.og("for-each-ref", "--format=x", "refs/merge-failed-meta")) == expect)
	}
}

// cleanup_retry: 남은 후보 작업 트리 정리(remove·prune)가 실패하면 75로 미루고, 다시 하면 착지·정리한다
func TestRecoveryCleanupRetry(t *testing.T) {
	k := newRK(t)
	f := k.fixture("cleanup")
	f.enable()
	f.commitFile("payload", "yes")
	f.bok("submit", f.submit("lane"))
	root := f.mclone + ".merger-wt"
	mkdirs(t, root+"/c-123456")
	f.sh(f.mclone, `git worktree add -q --detach "$1" HEAD`, root+"/c-abcdef")
	f.bstub("git", `#!/bin/sh
case "$fault $1 $2" in "remove worktree remove"|"prune worktree prune") touch "$HOME/hit"; exit 128;; esac
exec "$real_git" "$@"
`)
	for _, fault := range []string{"remove", "prune"} {
		k.set("fault=" + fault)
		r := f.tick(f.stubPath())
		k.check(fault+": 실패 끼움", exists(f.home+"/hit"), r.Out, r.Err)
		k.check(fault+": rc 75", r.RC == 75, r.Out, r.Err)
		k.check(fault+": main 그대로", f.og("rev-parse", "main") == f.g("rev-parse", "main"))
		os.Remove(f.home + "/hit")
	}
	r := f.tick()
	f.bok("tick", r)
	k.check("착지", f.og("show", "main:payload") == "yes", r.Out, r.Err)
	left, _ := os.ReadDir(root)
	k.check("후보 폴더 빔", len(left) == 0, fmt.Sprint(left))
}

// clone_state: 병합기 클론 상태 조회(HEAD·rebase-merge 경로·rebase --abort)가 실패하면 75로 미룬다
func TestRecoveryCloneState(t *testing.T) {
	k := newRK(t)
	f := k.fixture("clone-state")
	f.enable()
	f.bok("빈 tick", f.tick())
	f.commitFile("payload", "yes")
	f.bok("submit", f.submit("lane"))
	f.bstub("git", `#!/bin/sh
case "$fault:$*" in
  "head:symbolic-ref -q HEAD"|"path:rev-parse --git-path rebase-merge"|"abort:rebase --abort") touch "$HOME/hit"; exit 128 ;;
esac
exec "$real_git" "$@"
`)
	for _, fault := range []string{"head", "path", "abort"} {
		k.set("fault=" + fault)
		if fault == "abort" {
			mkdirs(t, f.mclone+"/.git/rebase-merge")
		}
		r := f.tick(f.stubPath())
		k.check(fault+": 실패 끼움", exists(f.home+"/hit"), r.Out, r.Err)
		k.check(fault+": rc 75", r.RC == 75, r.Out, r.Err)
		k.check(fault+": main 그대로", f.og("rev-parse", "main") == f.g("rev-parse", "main"))
		os.Remove(f.home + "/hit")
	}
	if err := os.Remove(f.mclone + "/.git/rebase-merge"); err != nil {
		t.Fatal(err)
	}
	r := f.tick()
	f.bok("tick", r)
	k.check("착지", f.og("show", "main:payload") == "yes", r.Out, r.Err)
}

// approved_in_main: 승인한 sha가 이미 main에 있으면 제출 없이 완료로 적는다
func TestRecoveryApprovedInMain(t *testing.T) {
	k := newRK(t)
	f := k.fixture("already-approved")
	f.enable()
	f.commitFile("payload", "yes")
	approved := f.g("rev-parse", "HEAD")
	f.w(`git push -q origin HEAD:main`)
	k.set("HC_FILL_VERDICTS="+f.home+"/verdicts", "HC_LANE=lane", "HC_LANE_JOB=run-1")
	vd := f.bverdict(approved, "pass")
	r := f.bland()
	f.bok("land", r)
	k.check("완료 sha = 승인", bcut2(vd+"/a1.done") == approved, read(vd+"/a1.done"), r.Out, r.Err)
	k.check("큐 빔", f.refs("refs/merge-queue") == "")
}

// queue_name: 큐 이름의 가지 꼴 검사(check-ref-format --branch)가 실패해도 돌려보내지 않고 착지한다
func TestRecoveryQueueName(t *testing.T) {
	k := newRK(t)
	f := k.fixture("queue-name")
	f.enable()
	f.commitFile("payload", "yes")
	f.bok("submit", f.submit("lane"))
	f.bstub("git", `#!/bin/sh
case "$*" in 'check-ref-format --branch lane') exit 128 ;; esac
exec "$real_git" "$@"
`)
	r := f.tick(f.stubPath())
	f.bok("tick", r)
	k.check("돌려보냄 없음", f.refs("refs/merge-failed") == "", r.Out, r.Err)
	k.check("착지", f.og("show", "main:payload") == "yes")
}

// reserved_queue: 제출 기록 없이 ref로 넣은 옛 큐(이름이 기본 가지와 같은 레인)도 입력이고, 병합기 클론의 main을 건드리지 않고 돌려보낸다
func TestRecoveryReservedQueue(t *testing.T) {
	k := newRK(t)
	f := k.fixture("reserved")
	f.enable()
	old := f.g("rev-parse", "main")
	f.commitFile("payload", "yes")
	f.w(`git push -q origin HEAD:refs/merge-queue/1000000000-main`)
	r := f.tick()
	f.bok("tick", r)
	k.check("병합기 클론 main 그대로", f.git(f.mclone, "rev-parse", "--verify", "refs/heads/main") == old, r.Out, r.Err)
	k.check("main 그대로", f.og("rev-parse", "main") == old)
	k.check("돌려보냄 = 큐 커밋", f.og("rev-parse", "refs/merge-failed/1000000000-main") == f.g("rev-parse", "HEAD"))
}
