package merger_test

// 복구 시험 사례(옛 hc-lane-merger-recovery.test.sh) 셋째 묶음 — 사례 함수 하나가 Test 하나.
// installed_test_paths는 옮기지 않았다: 셸 시험 파일을 깔린 곳에 복사해 머리만 돌려 보는 셸 시험 틀 검사라, 셸 시험이 없어지면 볼 것이
// 없다(Go 시험은 clitest.Repo가 깔린 곳에서 건너뛴다).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/clitest"
	"herdr-tools/internal/proc"
)

// cmust: 셸 set -e 아래 맨 부름 — 0이 아니면 그 사례가 멈춘다
func cmust(k *K, what string, r R) R {
	k.t.Helper()
	if r.RC != 0 {
		k.t.Fatalf("%s 실패(rc %d)\n%s\n%s", what, r.RC, r.Out, r.Err)
	}
	return r
}

// clog: 셸 >"$HOME/log" 2>&1 꼴(표준 출력과 오류를 함께)
func clog(r R) string { return r.Out + "\n" + r.Err }

// cfile: test -f. cnonempty: test -s
func cfile(p string) bool { st, err := os.Stat(p); return err == nil && st.Mode().IsRegular() }

func cnonempty(p string) bool { st, err := os.Stat(p); return err == nil && st.Size() > 0 }

// ccommitIn: commit_file을 dir에서(셸 (cd "$wt" && commit_file …))
func (f *F) ccommitIn(dir, file, content string) {
	f.t.Helper()
	f.sh(dir, `printf '%s\n' "$2" >"$1" && git add -- "$1" && git commit -qm "$1"`, file, content)
}

// cog: 실패하면 멈추는 og(셸 set -e 아래 og update-ref 등)
func (f *F) cog(args ...string) {
	f.t.Helper()
	if !f.ogOK(args...) {
		f.t.Fatalf("og %s 실패", strings.Join(args, " "))
	}
}

// cid: 제출 기록 커밋 제목의 id= 값(셸 og show -s --format=%s … | tr ' ' '\n' | sed -n 's/^id=//p')
func (f *F) cid(rev string) string {
	var ids []string
	for _, w := range strings.Split(f.og("show", "-s", "--format=%s", rev), " ") {
		if v, ok := strings.CutPrefix(w, "id="); ok {
			ids = append(ids, v)
		}
	}
	return strings.Join(ids, "\n")
}

func (f *F) evPath() string { return f.home + "/.local/state/hc-lane/events.tsv" }

// report_queries: 착지 보고가 원격 조상 확인에 실패하면 착지로 적지 않는다
func TestRecoveryReportQueries(t *testing.T) {
	k := newRK(t)
	f := k.fixture("report")
	f.enable()
	f.commitFile("payload", "yes")
	cmust(k, "submit lane", f.submit("lane"))
	sha := f.g("rev-parse", "HEAD")
	k.stub(f.home+"/stub/git", `#!/bin/sh
case "$1 $2 ${4-}" in "merge-base --is-ancestor $fault") touch "$HOME/hit"; exit 128 ;; esac
exec "$real_git" "$@"
`)
	for _, fault := range []string{"refs/remotes/origin/main", "refs/remotes/origin/lane/lane"} {
		k.set("fault=" + fault)
		if fault != "refs/remotes/origin/main" {
			f.w(`rm .git/hc-submitted`)
		}
		r := f.lane(f.writer, []string{f.stubPath()}, "report", "착지", sha, "--name", "lane")
		k.check(fault+": 고장 git 부름", exists(f.home+"/hit"), clog(r))
		k.check(fault+": report 실패", r.RC != 0, clog(r))
		k.check(fault+": 착지 사건 없음", exists(f.evPath()) && f.nev("lane", "착지") == 0)
		f.w(`rm "$HOME/hit"`)
	}
	r := cmust(k, "report 착지", f.lane(f.writer, nil, "report", "착지", sha, "--name", "lane"))
	k.check("제출만 알림", strings.Contains(clog(r), "제출만"), clog(r))
	f.w(`git push -q origin HEAD:main`)
	cmust(k, "report 착지(push 뒤)", f.lane(f.writer, nil, "report", "착지", sha, "--name", "lane"))
	k.check("착지 사건 하나", f.nev("lane", "착지") == 1)
}

// rebase_retry: rebase가 git 오류로 끊기면 돌려보내지 않고 75, 진짜 충돌은 돌려보낸다
func TestRecoveryRebaseRetry(t *testing.T) {
	k := newRK(t)
	f := k.fixture("index-lock")
	f.enable()
	f.commitFile("shared", "lane")
	cmust(k, "submit lane", f.submit("lane"))
	k.stub(f.home+"/stub/git", `#!/bin/sh
if [ "$1" = rebase ] && [ "$2" = -q ]; then touch "$HOME/hit"; exit 128; fi
exec "$real_git" "$@"
`)
	r := f.tick(f.stubPath())
	k.check("고장: rebase 부름", exists(f.home+"/hit"), clog(r))
	k.check("고장: rc 75", r.RC == 75, clog(r))
	k.check("고장: 돌려보냄 없음", f.refs("refs/merge-failed") == "")
	k.check("고장: 큐 남음", f.refs("refs/merge-queue") != "")
	f.w(`git checkout -q main`)
	f.commitFile("shared", "remote")
	f.w(`git push -q origin main`)
	r = cmust(k, "tick", f.tick())
	k.check("충돌: 돌려보냄", f.refs("refs/merge-failed") != "", clog(r))
	k.check("충돌: main 그대로", f.og("show", "main:shared") == "remote")
}

// default_query: 기본 가지·origin 주소를 못 읽으면 멈추고, 읽으면 기본 가지(release)에 착지
func TestRecoveryDefaultQuery(t *testing.T) {
	k := newRK(t)
	f := k.fixture("default")
	f.w(`git push -q origin main:refs/heads/release && git symbolic-ref refs/remotes/origin/HEAD refs/remotes/origin/release`)
	f.commitFile("payload", "yes")
	k.stub(f.home+"/stub/git", `#!/bin/sh
case "$fault:$*" in
  default:*symbolic-ref*"refs/remotes/origin/HEAD"|default:*for-each-ref*"refs/remotes/origin/HEAD"|origin:*"remote get-url origin")
    touch "$HOME/hit"; exit 128 ;;
esac
exec "$real_git" "$@"
`)
	r := f.lane(f.writer, []string{"fault=default", f.stubPath()}, "land")
	k.check("default: 고장 부름·land 실패", exists(f.home+"/hit") && r.RC != 0, clog(r))
	k.check("default: main 그대로", f.og("rev-parse", "main") == f.g("rev-parse", "main"))
	f.w(`rm "$HOME/hit"`)
	r = f.lane(f.writer, []string{"fault=origin", f.stubPath()}, "clean", "--repo", f.writer, "--name", "absent")
	k.check("origin: 고장 부름·clean 실패", exists(f.home+"/hit") && r.RC != 0, clog(r))
	cmust(k, "land", f.lane(f.writer, nil, "land"))
	k.check("release에 착지", f.og("rev-parse", "release") == f.g("rev-parse", "HEAD"))
	k.check("main 그대로", f.og("rev-parse", "main") == f.g("rev-parse", "main"))
}

// landed_lease: 착지한 레인의 임대 참조는 지우고, 다시 밀린 임대는 그대로 둔다
func TestRecoveryLandedLease(t *testing.T) {
	k := newRK(t)
	for _, mode := range []string{"direct", "merger", "approved"} {
		f := k.fixture("landed-" + mode)
		old := f.g("rev-parse", "main")
		f.commitFile("payload", "yes")
		landed := f.g("rev-parse", "HEAD")
		f.w(`git push -q origin "$1:refs/hc-lanes/lane" HEAD:main`, old)
		if mode == "merger" {
			f.enable()
		}
		args := []string{"land", "main"}
		if mode == "approved" {
			k.set("HC_FILL_VERDICTS="+f.home+"/verdicts", "HC_LANE_JOB=run-1")
			write(t, f.home+"/verdicts/lane/a1.md", "aid\ta1\nlane\tlane\nrun\trun-1\nsha\t"+landed+"\nverdict\tpass\nbase\tmain\n")
			args = append(args, "--verdict", "a1")
		}
		ex := []string{"HC_LANE=lane", "HC_LANE_LEASE=" + old}
		cmust(k, mode+": land", f.lane(f.writer, ex, args...))
		k.check(mode+": 임대 지움", f.refs("refs/hc-lanes/lane") == "")
		k.check(mode+": 착지 증거 = 착지 커밋", f.og("for-each-ref", "--format=%(objectname)", "refs/hc-landed/lane/*") == landed)
		f.w(`git push -q origin "$1:refs/hc-lanes/lane"`, landed)
		cmust(k, mode+": land 다시", f.lane(f.writer, ex, args...))
		k.check(mode+": 다시 밀린 임대 그대로", f.og("rev-parse", "refs/hc-lanes/lane") == landed)
	}
}

// receipt_race: 착지 push 직전·직후에 제출이 새 기록으로 바뀌면 새 기록으로 영수증을 남긴다
func TestRecoveryReceiptRace(t *testing.T) {
	k := newRK(t)
	for _, phase := range []string{"before", "after"} {
		f := k.fixture("receipt-" + phase)
		f.enable()
		f.commitFile("payload", "yes")
		cmust(k, phase+": submit lane", f.submit("lane"))
		qref := f.og("for-each-ref", "--format=%(refname)", "refs/merge-queue")
		meta := "refs/merge-queue-meta/" + qref[strings.LastIndex(qref, "/")+1:]
		old := f.og("rev-parse", meta)
		original := f.cid(old)
		sha := f.g("rev-parse", "HEAD")
		next := f.sh(f.home, `git --git-dir="$1" show -s --format=%s "$2" | sed "s/id=$3/id=new-receipt/" | git --git-dir="$1" commit-tree "$2^{tree}" -p "$4"`,
			f.origin, old, original, sha)
		k.set("qref="+qref, "meta="+meta, "next="+next, "sha="+sha, "phase="+phase)
		k.stub(f.home+"/stub/git", `#!/bin/sh
case "$1 $*" in "push "*":refs/heads/main "*":refs/merge-queue/"*)
  if mkdir "$HOME/once" 2>/dev/null; then
    if [ "$phase" = after ]; then "$real_git" "$@" || exit 1; fi
    og() { "$real_git" --git-dir="$HOME/origin.git" "$@"; }
    og update-ref "$qref" "$sha" && og update-ref "$meta" "$next" && og update-ref refs/heads/lane/lane "$sha" || exit 1
    if [ "$phase" = after ]; then exit 0; fi
  fi ;;
esac
exec "$real_git" "$@"
`)
		r := cmust(k, phase+": tick", f.tick(f.stubPath()))
		k.check(phase+": 끼어듦 일어남", clitest.IsDir(f.home+"/once"), clog(r))
		k.check(phase+": merge-done = 새 기록", f.og("rev-parse", "refs/merge-done/lane") == next, clog(r))
		k.check(phase+": 새 기록 증거 = sha", f.og("rev-parse", "refs/hc-landed/new-receipt") == sha)
		want := 0
		if phase != "before" {
			want = 1
		}
		n := 0
		for _, row := range f.events() {
			if col(row, 5) == "착지" && strings.Contains(col(row, 7), original) {
				n++
			}
		}
		k.check(phase+": 옛 id 착지 사건 수", exists(f.evPath()) && n == want)
		k.check(phase+": 큐·가지 빔", f.refs("refs/merge-queue", "refs/merge-queue-meta", "refs/heads/lane") == "")
	}
}

// resident_sleep: 상주 병합기는 할 일을 마친 뒤 --tick 초만큼 쉰다
func TestRecoveryResidentSleep(t *testing.T) {
	k := newRK(t)
	f := k.fixture("idle")
	f.enable()
	f.commitFile("payload", "yes")
	cmust(k, "submit lane", f.submit("lane"))
	k.stub(f.home+"/stub/sleep", `#!/bin/sh
# 실제 착지와 큐 삭제가 끝난 뒤의 첫 sleep 인자를 전달한다.
bare="$HOME/origin.git"
if [ "$("$real_git" --git-dir="$bare" show main:payload 2>/dev/null)" = yes ] &&
   [ -z "$("$real_git" --git-dir="$bare" for-each-ref refs/merge-queue)" ]; then
  printf '%s\n' "$*" >"$HOME/delay.tmp" && mv "$HOME/delay.tmp" "$HOME/delay"
  exec perl -e 'for (1..1200) { last if -e "$ENV{HOME}/release"; select undef,undef,undef,0.05 }'
fi
exec /bin/sleep "$@"
`)
	cmd := k.mergerCmd(f.writer, []string{f.stubPath()}, f.home+"/log", "--repo", f.mclone, "--tick", "17")
	proc.Group(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }() // 끝남은 done으로만 본다
	t.Cleanup(func() {
		touch(t, f.home+"/release")
		proc.StopGroup(cmd.Process.Pid, 2*time.Second)
		<-done
	})
wait:
	for i := 0; i < 1200 && !cnonempty(f.home+"/delay"); i++ {
		select {
		case <-done:
			break wait
		case <-time.After(50 * time.Millisecond):
		}
	}
	k.check("쉼 인자 받음", cnonempty(f.home+"/delay"), read(f.home+"/log"))
	k.check("쉼 = 17", read(f.home+"/delay") == "17\n", read(f.home+"/delay"))
}

// reverted_upstream: 같은 변경이 main에 들어갔다 되돌려졌어도 레인의 변경은 다시 얹힌다
func TestRecoveryRevertedUpstream(t *testing.T) {
	k := newRK(t)
	for _, mode := range []string{"single", "batch"} {
		f := k.fixture("reapplied-" + mode)
		f.enable()
		f.commitFile("shared", "wanted")
		cmust(k, mode+": submit alpha", f.submit("alpha"))
		f.w(`git checkout -q main && echo wanted >shared && git commit -qam '다른 레인의 같은 변경' && git revert --no-edit HEAD >/dev/null && git push -q origin main`)
		if mode == "batch" {
			f.w(`git checkout -qb zulu`)
			f.commitFile("companion", "yes")
			cmust(k, mode+": submit zulu", f.submit("zulu"))
		}
		r := cmust(k, mode+": tick", f.tick())
		k.check(mode+": shared = wanted", f.og("show", "main:shared") == "wanted", clog(r))
		k.check(mode+": 큐·돌려보냄 빔", f.refs("refs/merge-queue", "refs/merge-failed") == "")
		if mode == "batch" {
			k.check(mode+": companion 착지", f.og("show", "main:companion") == "yes")
		}
	}
}

// merge_resolution: 손으로 푼 병합 커밋은 다시 얹지 않고 돌려보내고, 자동 병합은 착지한다
func TestRecoveryMergeResolution(t *testing.T) {
	k := newRK(t)
	for _, mode := range []string{"ahead", "diverged", "batch", "automatic"} {
		f := k.fixture("merge-" + mode)
		f.commitFile("left", "yes")
		f.w(`git checkout -qb side main`)
		f.commitFile("right", "yes")
		f.w(`git checkout -q lane && git merge -q --no-commit --no-ff side`)
		if mode != "automatic" {
			f.w(`echo resolved >resolution && git add -- resolution`)
		}
		f.w(`git commit -qm '두 갈래 병합'`)
		original := f.g("rev-parse", "HEAD")
		if mode == "ahead" {
			cmust(k, mode+": land", f.lane(f.writer, nil, "land", "main"))
			k.check(mode+": main = 병합 커밋", f.og("rev-parse", "main") == original)
			continue
		}
		f.w(`git checkout -q main`)
		f.commitFile("independent", "yes")
		f.w(`git push -q origin main`)
		f.enable()
		if mode == "batch" {
			f.w(`git checkout -qb alpha`)
			f.commitFile("companion", "yes")
			cmust(k, mode+": submit alpha", f.submit("alpha"))
		}
		f.w(`git checkout -q lane`)
		cmust(k, mode+": submit zulu", f.submit("zulu"))
		r := cmust(k, mode+": tick", f.tick())
		if mode == "automatic" {
			k.check(mode+": left·right 착지", f.og("show", "main:left") == "yes" && f.og("show", "main:right") == "yes", clog(r))
			k.check(mode+": merge-done 있음", f.refs("refs/merge-done/zulu") != "")
			continue
		}
		k.check(mode+": 레인 가지 = 병합 커밋", f.og("rev-parse", "refs/heads/lane/zulu") == original, clog(r))
		k.check(mode+": 착지 기록 없음", f.refs("refs/merge-done/zulu", "refs/hc-landed/zulu/") == "")
		k.check(mode+": 돌려보냄 있음", f.refs("refs/merge-failed/*-zulu") != "")
		if mode == "batch" {
			k.check(mode+": companion 착지", f.og("show", "main:companion") == "yes")
		}
	}
}

// proof_ref: 착지 증거 참조 이름을 못 확인하면 착지하지 않고 75, 다음 바퀴에 증거를 둘 다 남긴다
func TestRecoveryProofRef(t *testing.T) {
	k := newRK(t)
	f := k.fixture("proof-ref")
	f.enable()
	f.commitFile("payload", "yes")
	cmust(k, "submit lane", f.submit("lane", "HC_LANE_JOB=run-7"))
	sid := f.cid(f.og("for-each-ref", "--format=%(objectname)", "refs/merge-queue-meta"))
	k.stub(f.home+"/stub/git", `#!/bin/sh
[ "$1 $2" != "check-ref-format refs/hc-landed/$fault" ] || { touch "$HOME/hit"; exit 128; }
exec "$real_git" "$@"
`)
	for _, fault := range []string{sid, "run-7"} {
		k.set("fault=" + fault)
		r := f.tick(f.stubPath())
		k.check(fault+": 고장 부름", exists(f.home+"/hit"), clog(r))
		k.check(fault+": rc 75", r.RC == 75, clog(r))
		k.check(fault+": main 그대로", f.og("rev-parse", "main") == f.g("rev-parse", "main"))
		k.check(fault+": 큐 남음", f.refs("refs/merge-queue") != "")
		f.w(`rm "$HOME/hit"`)
	}
	r := cmust(k, "tick", f.tick())
	for _, id := range []string{sid, "run-7"} {
		k.check(id+": 증거 = main", f.og("rev-parse", "--verify", "refs/hc-landed/"+id) == f.og("rev-parse", "main"), clog(r))
	}
	k.check("착지 사건 하나", f.nev("lane", "착지") == 1)
}

// postpush_cleanup: main push 뒤 증거 push가 끊겨도 다음 바퀴에 마무리하고 사건·일지는 한 번씩
func TestRecoveryPostpushCleanup(t *testing.T) {
	k := newRK(t)
	f := k.fixture("cleanup-push")
	f.enable()
	f.commitFile("payload", "yes")
	cmust(k, "submit lane", f.submit("lane"))
	journal := f.mclone + "/.git/hc-merger/cur"
	k.stub(f.home+"/stub/git", `#!/bin/sh
case "$1 $*" in "push "*:refs/hc-landed/lane/*) touch "$HOME/hit"; exit 128 ;; esac
exec "$real_git" "$@"
`)
	for _, attempt := range []string{"1", "2"} {
		r := f.tick(f.stubPath())
		k.check(attempt+": 증거 push 부름", exists(f.home+"/hit"), clog(r))
		k.check(attempt+": rc 75", r.RC == 75, clog(r))
		k.check(attempt+": merge-done 있음", f.refs("refs/merge-done/lane") != "")
		k.check(attempt+": 증거 없음", f.refs("refs/hc-landed/lane/") == "")
		k.check(attempt+": payload 착지", f.og("show", "main:payload") == "yes")
		k.check(attempt+": 일지 남음", cnonempty(journal))
		k.check(attempt+": 착지 사건 아직 없음", exists(f.evPath()) && f.nev("lane", "착지") == 0)
		f.w(`rm "$HOME/hit"`)
	}
	f.w(`cp "$1" "$HOME/before-cleanup"`, journal)
	cmust(k, "tick", f.tick())
	f.w(`cp "$HOME/before-cleanup" "$1"`, journal)
	r := cmust(k, "tick 다시", f.tick())
	k.check("일지 지움", !cfile(journal), clog(r))
	k.check("가지·큐 빔", f.refs("refs/heads/lane", "refs/merge-queue", "refs/merge-queue-meta") == "")
	k.check("착지 사건 하나", f.nev("lane", "착지") == 1)
	n := 0
	for _, row := range f.journal() {
		if col(row, 3) == "lane" && col(row, 10) == "착지" {
			n++
		}
	}
	k.check("일지 착지 줄 하나", n == 1)
}

// verdict_identity: 감사 판정의 실행(run)이 지금 터미널 실행과 다르면 착지하지 않는다
func TestRecoveryVerdictIdentity(t *testing.T) {
	k := newRK(t)
	f := k.fixture("identity")
	f.commitFile("payload", "yes")
	sha := f.g("rev-parse", "HEAD")
	base := f.og("rev-parse", "main")
	vd := f.home + "/verdicts"
	k.set("HC_LANE=lane", "HC_TERMINAL_ID=t11", "HC_FILL_VERDICTS="+vd)
	mkdirs(t, vd+"/lane")
	k.stub(f.home+"/stub/hc", `#!/bin/sh
case $1 in
  get) echo '{"state":"live","created_ms":11,"foreground":{"created_ms":22},"turn":1}'
       test "$fault" = ok ;;
  ls) echo '{"terminals":[]}' ;;
  *) exit 3 ;;
esac
`)
	for _, fault := range []string{"failed", "ok"} {
		k.set("fault=" + fault)
		write(t, vd+"/lane/a1.md", "aid\ta1\nlane\tlane\nrun\tt11@22\nsha\t"+sha+"\nverdict\tpass\nbase\tmain\n")
		r := f.lane(f.writer, []string{f.stubPath()}, "land", "main", "--verdict", "a1")
		k.check(fault+": land 실패", r.RC != 0, clog(r))
		k.check(fault+": ack 없음", !exists(vd+"/lane/a1.ack"))
		k.check(fault+": main 그대로", f.og("rev-parse", "main") == base)
		cmust(k, fault+": report 완료", f.lane(f.writer, []string{f.stubPath()}, "report", "완료", sha))
		want := "-"
		if fault != "failed" {
			want = "t11@11"
		}
		ev := f.events()
		got := ""
		if len(ev) > 0 {
			got = col(ev[len(ev)-1], 10)
		}
		k.check(fault+": 마지막 사건 10칸", got == want, got)
	}
	write(t, vd+"/lane/a1.md", strings.Replace(read(vd+"/lane/a1.md"), "t11@22", "t11@11", 1))
	r := cmust(k, "land 바른 실행", f.lane(f.writer, []string{f.stubPath()}, "land", "main", "--verdict", "a1"))
	k.check("바른 실행: 착지", f.og("rev-parse", "main") == sha, clog(r))
}

// guard_queries: guard install이 origin·hooksPath를 못 읽으면 훅을 깔지 않는다
func TestRecoveryGuardQueries(t *testing.T) {
	k := newRK(t)
	f := k.fixture("guard")
	k.stub(f.home+"/stub/git", `#!/bin/sh
case "$fault:$*" in origin:*" remote"|hooks:*" config --get core.hooksPath") touch "$HOME/hit"; exit 128 ;; esac
exec "$real_git" "$@"
`)
	hook := f.writer + "/.git/hooks/pre-push"
	for _, fault := range []string{"origin", "hooks"} {
		k.set("fault=" + fault)
		r := f.lane(f.writer, []string{f.stubPath()}, "guard", "install", "--repo", f.writer)
		k.check(fault+": install 실패", r.RC != 0, clog(r))
		k.check(fault+": 고장 부름", cfile(f.home+"/hit"))
		k.check(fault+": 훅 없음", !exists(hook))
		f.w(`rm "$HOME/hit"`)
	}
	cmust(k, "guard install", f.lane(f.writer, nil, "guard", "install", "--repo", f.writer))
	cmust(k, "guard status", f.lane(f.writer, nil, "guard", "status", "--repo", f.writer))
	k.check("훅 실행 가능", clitest.IsExec(hook))
}

// journal_retry: 일지를 못 쓰면 75, 다음 바퀴에 일지를 한 번만 적는다
func TestRecoveryJournalRetry(t *testing.T) {
	k := newRK(t)
	for _, outcome := range []string{"accept", "reject"} {
		f := k.fixture("journal-" + outcome)
		f.enable()
		if outcome == "reject" {
			f.commitFile(".claude/herdr-verify.sh", "exit 1")
		} else {
			f.commitFile("payload", "yes")
		}
		cmust(k, outcome+": submit lane", f.submit("lane"))
		journal := f.home + "/.local/state/hc-lane/merger-journal.tsv"
		f.w(`mkdir "$1"`, journal)
		r := f.tick()
		k.check(outcome+": rc 75", r.RC == 75, clog(r))
		result := "착지"
		if outcome == "accept" {
			k.check(outcome+": payload 착지", f.og("show", "main:payload") == "yes")
			k.check(outcome+": cur 남음", cnonempty(f.mclone+"/.git/hc-merger/cur"))
		} else {
			result = "돌려보냄"
			k.check(outcome+": 돌려보냄 있음", f.refs("refs/merge-failed") != "")
			m, _ := filepath.Glob(f.mclone + "/.git/hc-merger/fail/*-lane")
			k.check(outcome+": fail 기록 남음", len(m) == 1 && cnonempty(m[0]), strings.Join(m, " "))
		}
		f.w(`rmdir "$1"`, journal)
		cmust(k, outcome+": tick", f.tick())
		cmust(k, outcome+": tick 다시", f.tick())
		n := 0
		for _, row := range f.journal() {
			if col(row, 3) == "lane" && col(row, 10) == result {
				n++
			}
		}
		k.check(outcome+": 일지 "+result+" 한 줄", n == 1, read(journal))
	}
}

// launch_response: hc run 응답이 끊기면 작업 폴더·임대를 남기고, 거절이면 아무것도 남기지 않는다
func TestRecoveryLaunchResponse(t *testing.T) {
	k := newRK(t)
	f := k.fixture("launch-response")
	k.stub(f.home+"/stub/hc", `#!/bin/sh
case $1 in
  ls) echo '{"terminals":[]}' ;;
  get) exit 3 ;;
  run)
    if [ "$mode" = refused ]; then echo '{"error":{"kind":"name_taken"}}' >&2; exit 1; fi
    while [ "$1" != --cwd ]; do shift; done
    echo 작성중 >"$2/draft"
    echo '응답이 끊겼다' >&2
    exit 1 ;;
  *) exit 1 ;;
esac
`)
	for _, mode := range []string{"lost", "refused"} {
		k.set("mode=" + mode)
		r := f.lane(f.writer, []string{f.stubPath()}, "--name", mode, "--repo", f.writer, "--start-wait", "0", "--", "true")
		k.check(mode+": 띄우기 실패", r.RC != 0, clog(r))
		wt := f.writer + ".wt/" + mode
		if mode == "lost" {
			k.check(mode+": 작성 파일 남음", cfile(wt+"/draft"))
			k.check(mode+": 작성 내용 그대로", read(wt+"/draft") == "작성중\n")
			k.check(mode+": 임대 참조 남음", f.refs("refs/hc-lanes/"+mode) != "")
		} else {
			k.check(mode+": 작업 폴더 없음", !exists(wt))
			k.check(mode+": 가지 없음", strings.TrimSpace(f.g("for-each-ref", "refs/heads/"+mode)) == "")
		}
	}
}

// clean_submission_owner: 다른 레인 이름으로 제출된 가지는 주차 전에는 지우지 않는다
func TestRecoveryCleanSubmissionOwner(t *testing.T) {
	k := newRK(t)
	f := k.fixture("clean-submission")
	k.stub(f.home+"/stub/hc", `#!/bin/sh
case $1 in run) echo '{"address":"t11"}' ;; ls) echo '{"terminals":[]}' ;; *) exit 3 ;; esac
`)
	cmust(k, "띄우기", f.lane(f.writer, []string{f.stubPath()}, "--name", "submitted", "--branch", "alias", "--repo", f.writer, "--start-wait", "0", "--", "true"))
	wt := f.writer + ".wt/alias"
	f.ccommitIn(wt, "작업", "보존")
	sha := f.git(wt, "rev-parse", "HEAD")
	f.w(`git push -q origin "$1:refs/heads/lane/other"`, sha)
	r := f.lane(f.writer, nil, "clean", "--repo", f.writer, "--branch", "alias")
	k.check("주차 전 clean 실패", r.RC != 0, clog(r))
	k.check("작업 커밋 그대로", f.git(wt, "rev-parse", "HEAD") == sha)
	k.check("작업 파일 그대로", read(wt+"/작업") == "보존\n")
	lease := f.g("rev-parse", "refs/hc-lease-owners/alias")
	cmust(k, "park", f.lane(wt, []string{"HC_LANE=submitted", "HC_LANE_LEASE=" + lease}, "park"))
	r = cmust(k, "clean", f.lane(f.writer, nil, "clean", "--repo", f.writer, "--branch", "alias"))
	k.check("작업 폴더 지움", !exists(wt), clog(r))
	k.check("주차 = 작업 커밋", f.og("rev-parse", "refs/park/submitted") == sha)
}

// approval_location: 감사 뒤 승인 범위 밖 줄로 옮긴 수정은 거절(2), 승인된 커밋은 main이 앞서도 착지
func TestRecoveryApprovalLocation(t *testing.T) {
	k := newRK(t)
	for _, state := range []string{"same", "advanced"} {
		f := k.fixture("location-" + state)
		f.w(`git checkout -q main`)
		f.commitFile(".claude/herdr-verify.sh", `test ! -e "$HOME/failverify"`)
		file := "작업 규칙"
		f.w(`printf '%s\n' 첫째 이전 사이1 사이2 사이3 사이4 둘째 이전 끝1 끝2 끝3 끝4 >"$1" && git add -- "$1" && git commit -qm 규칙 && git push -q origin main &&
git checkout -q -B lane main && sed '2s/이전/이후/' "$1" >edit && mv edit "$1" && git commit -qam '첫째 수정 승인'`, file)
		approved := f.g("rev-parse", "HEAD")
		vd := f.home + "/verdicts"
		k.set("HC_FILL_VERDICTS="+vd, "HC_LANE=lane", "HC_LANE_JOB=run-1")
		write(t, vd+"/lane/a1.md", "aid\ta1\nlane\tlane\nrun\trun-1\nsha\t"+approved+"\nverdict\tpass\nbase\tmain\n")
		if state == "advanced" {
			f.w(`git checkout -q main && echo 새끝 >>"$1" && git commit -qam '같은 파일의 독립 변경' && git push -q origin main && git checkout -q lane`, file)
		}
		base := f.og("rev-parse", "main")
		touch(t, f.home+"/failverify")
		r := f.lane(f.writer, nil, "land", "main", "--verdict", "a1")
		k.check(state+": 검증 실패로 land 실패", r.RC != 0, clog(r))
		k.check(state+": 검증 실패 알림", strings.Contains(clog(r), "검증 실패"), clog(r))
		good := f.g("rev-parse", "HEAD")
		f.w(`sed -e '2s/이후/이전/' -e '8s/이전/이후/' "$1" >edit && mv edit "$1" && git commit -qam '둘째 수정으로 이동'`, file)
		f.w(`rm "$HOME/failverify"`)
		r = f.lane(f.writer, nil, "land", "main", "--verdict", "a1")
		k.check(state+": 옮긴 수정 rc 2", r.RC == 2, clog(r))
		k.check(state+": main 그대로", f.og("rev-parse", "main") == base)
		f.w(`git checkout -q -B lane "$1"`, good)
		r = cmust(k, state+": land 승인 커밋", f.lane(f.writer, nil, "land", "main", "--verdict", "a1"))
		k.check(state+": main = 승인 커밋", f.og("rev-parse", "main") == good, clog(r))
	}
}

// hook_lease_query: 착지 훅 전에 임대 조회가 끊기면 75로 훅을 돌리지 않는다
func TestRecoveryHookLeaseQuery(t *testing.T) {
	k := newRK(t)
	f := k.fixture("hook-lease-query")
	base := f.g("rev-parse", "main")
	write(t, f.writer+"/.claude/herdr-land.sh", "touch \"$HOME/hook-ran\"\ngit push -q --atomic origin HEAD:main $HERDR_LAND_PUSH_EXTRA\n")
	f.w(`git add -- .claude/herdr-land.sh && git commit -qm '원자 착지 훅' && git push -q origin "$1:refs/hc-lanes/lane"`, base)
	k.stub(f.home+"/stub/git", `#!/bin/sh
if [ "$*" = 'ls-remote origin refs/hc-lanes/lane' ]; then touch "$HOME/hit"; exit 128; fi
exec "$real_git" "$@"
`)
	k.set("HC_LANE=lane", "HC_LANE_LEASE="+base)
	r := f.lane(f.writer, []string{f.stubPath()}, "land", "main")
	k.check("고장: 임대 조회 부름", exists(f.home+"/hit"), clog(r))
	k.check("고장: rc 75", r.RC == 75, clog(r))
	k.check("고장: 훅 안 돎", !exists(f.home+"/hook-ran"))
	k.check("고장: main 그대로", f.og("rev-parse", "main") == base)
	r = cmust(k, "land", f.lane(f.writer, nil, "land", "main"))
	k.check("착지", f.og("rev-parse", "main") == f.g("rev-parse", "HEAD"), clog(r))
	k.check("임대 지움", f.refs("refs/hc-lanes/lane") == "")
}

// clean_rebased_receipt: 다시 얹혀 착지한 레인은 영수증이 맞을 때만 clean이 지운다
func TestRecoveryCleanRebasedReceipt(t *testing.T) {
	k := newRK(t)
	f := k.fixture("clean-rebased")
	f.enable()
	wt := f.writer + ".wt/feature"
	f.w(`git worktree add -qb feature "$1" main`, wt)
	f.ccommitIn(wt, "기능", "보존")
	cmust(k, "submit feature", f.lane(wt, []string{"HC_LANE=feature"}, "land", "main"))
	original := f.git(wt, "rev-parse", "HEAD")
	f.w(`git checkout -q main`)
	f.commitFile("independent", "yes")
	f.w(`git push -q origin main`)
	r := cmust(k, "tick", f.tick())
	k.check("기능 착지", f.og("show", "main:기능") == "보존", clog(r))
	k.check("영수증 부모 = 원래 커밋", f.og("rev-parse", "refs/merge-done/feature^") == original)
	k.check("레인 가지 지움", f.refs("refs/heads/lane/feature") == "")
	k.check("원래 커밋은 main 조상 아님", !f.ogOK("merge-base", "--is-ancestor", original, "main"))
	receipt := f.og("rev-parse", "refs/merge-done/feature")
	f.cog("update-ref", "refs/merge-done/feature", "main")
	r = f.lane(f.writer, nil, "clean", "--repo", f.writer, "--branch", "feature")
	k.check("틀린 영수증: clean 실패", r.RC != 0, clog(r))
	k.check("틀린 영수증: 작업 파일 남음", cfile(wt+"/기능"))
	f.cog("update-ref", "refs/merge-done/feature", receipt)
	r = cmust(k, "clean", f.lane(f.writer, nil, "clean", "--repo", f.writer, "--branch", "feature"))
	k.check("작업 폴더 지움", !exists(wt), clog(r))
	k.check("가지 지움", strings.TrimSpace(f.g("for-each-ref", "refs/heads/feature")) == "")
}

// batch_default: 기본 묶음은 셋 — 넷 제출이면 셋 한 묶음과 하나
func TestRecoveryBatchDefault(t *testing.T) {
	k := newRK(t)
	f := k.fixture("batch-default")
	f.enable()
	for _, name := range []string{"alpha", "beta", "gamma", "omega"} {
		f.w(`git checkout -q -b "$1" main`, name)
		f.commitFile(name, "yes")
		cmust(k, "submit "+name, f.submit(name))
	}
	r := cmust(k, "tick", f.tick())
	n, sizes := 0, map[string]int{}
	for _, row := range f.journal() {
		if col(row, 10) == "착지" {
			n++
			sizes[col(row, 7)]++
		}
	}
	k.check("착지 넷: 셋 묶음 셋·하나 묶음 하나", n == 4 && sizes["3"] == 3 && sizes["1"] == 1, clog(r), read(f.home+"/.local/state/hc-lane/merger-journal.tsv"))
}

// notify_run: 제출한 터미널의 실행이 그대로일 때만 착지를 알린다
func TestRecoveryNotifyRun(t *testing.T) {
	k := newRK(t)
	for _, mode := range []string{"reused", "same", "unknown"} {
		f := k.fixture("notify-" + mode)
		f.enable()
		f.commitFile("payload", "yes")
		k.stub(f.home+"/stub/hc", `#!/bin/sh
case $1 in
  get) born=$(cat "$HOME/created") || exit 1
       echo "{\"name\":\"lane\",\"state\":\"live\",\"created_ms\":$born}"
       test -z "$born" && exit 1; exit 0 ;;
  ls) echo '{"terminals":[]}' ;;
  send) printf '%s\n' "$2" >>"$HOME/sent" ;;
  *) exit 3 ;;
esac
`)
		created := "11\n"
		if mode == "unknown" {
			created = ""
		}
		write(t, f.home+"/created", created)
		cmust(k, mode+": submit lane", f.submit("lane", f.stubPath(), "HC_TERMINAL_ID=t11"))
		created = "22\n"
		if mode == "same" {
			created = "11\n"
		}
		write(t, f.home+"/created", created)
		r := cmust(k, mode+": tick", f.tick(f.stubPath()))
		if mode == "same" {
			k.check(mode+": t11에 알림", read(f.home+"/sent") == "t11\n", clog(r))
		} else {
			k.check(mode+": 알림 없음", !exists(f.home+"/sent"), clog(r))
		}
		k.check(mode+": 착지 사건 하나", f.nev("lane", "착지") == 1)
	}
}

// notify_lock_fd: 알림 확인(hc get)에 병합기 잠금 fd 19를 물려주지 않는다
func TestRecoveryNotifyLockFd(t *testing.T) {
	k := newRK(t)
	f := k.fixture("notify-fd")
	f.enable()
	f.commitFile("payload", "yes")
	k.stub(f.home+"/stub/hc", `#!/bin/bash
case $1 in
  get) if ( : >&19 ) 2>/dev/null; then touch "$HOME/inherited"; fi
       touch "$HOME/checked"; echo '{"name":"lane","state":"live","created_ms":123}' ;;
  ls) echo '{"terminals":[]}' ;;
  send) : ;;
  *) exit 3 ;;
esac
`)
	cmust(k, "submit lane", f.submit("lane", f.stubPath(), "HC_TERMINAL_ID=t11"))
	f.w(`rm "$HOME/checked"`)
	r := cmust(k, "tick", f.tick(f.stubPath()))
	k.check("알림 확인함", cfile(f.home+"/checked"), clog(r))
	k.check("fd 19 안 물려줌", !exists(f.home+"/inherited"))
}

// wait_submit_owner: wait --mine은 제출한 지휘 실행이 그대로일 때만 제출 사건을 보인다
func TestRecoveryWaitSubmitOwner(t *testing.T) {
	k := newRK(t)
	f := k.fixture("wait-owner")
	f.enable()
	f.commitFile("payload", "yes")
	cmust(k, "submit lane", f.submit("lane", "HC_LANE_OWNER=box/t1", "HC_LANE_OWNER_RUN=11"))
	k.stub(f.home+"/stub/hc", `#!/bin/sh
case $1 in
  get) echo "{\"address\":\"t1\",\"state\":\"live\",\"pid\":123,\"name\":\"owner\",\"created_ms\":$created}"; test "$fault" = ok ;;
  ls) echo '{"terminals":[]}' ;;
  *) exit 3 ;;
esac
`)
	for _, mode := range []string{"failed", "reused", "same"} {
		fault, created := "ok", "11"
		switch mode {
		case "failed":
			fault = "error"
		case "reused":
			created = "22"
		}
		r := f.lane(f.writer, []string{"fault=" + fault, "created=" + created, f.stubPath(), "HC_TERMINAL_ID=t1"},
			"wait", "--mine", "--state", "제출", "--timeout", "0")
		seen := strings.Contains(clog(r), "\tlane\t제출\t")
		if mode == "same" {
			k.check(mode+": rc 0", r.RC == 0, clog(r))
			k.check(mode+": 제출 사건 보임", seen, clog(r))
		} else {
			k.check(mode+": rc 75", r.RC == 75, clog(r))
			k.check(mode+": 제출 사건 안 보임", !seen, clog(r))
		}
	}
}

// result_owner: 착지·막힘 사건은 제출한 지휘(9칸)와 그 실행(11칸)을 싣고, watch는 그 실행에만 보낸다
func TestRecoveryResultOwner(t *testing.T) {
	k := newRK(t)
	// 셸 판은 lib/hc-lane/watch.sh의 wfilter를 source해 줄 하나를 걸렀다 — Go 판은 같은 거르기(push.go wfilter)를 hc-lane watch
	// 한 바퀴(HC_LANE_WATCH_IDLE=0)로 밖에서 부른다. 가짜 hc는 지휘 t1(pid·이름은 coord 파일과 같게)과 hc send 받기
	k.stub(k.T+"/watch-bin/hc", `#!/bin/sh
case $1 in
  get) echo "{\"address\":\"t1\",\"state\":\"live\",\"pid\":123,\"name\":\"owner\",\"created_ms\":$watch_created}" ;;
  ls) echo '{"terminals":[]}' ;;
  send) printf '%s\n' "$3" >>"$HOME/watch-sent" ;;
  *) exit 3 ;;
esac
`)
	for _, mode := range []string{"landed", "returned", "legacy", "unowned"} {
		f := k.fixture("owner-" + mode)
		f.enable()
		if mode == "returned" {
			f.commitFile(".claude/herdr-verify.sh", "exit 1")
		} else {
			f.commitFile("payload", "yes")
		}
		ex := []string{"HC_LANE_OWNER=box/t1", "HC_LANE_OWNER_RUN=11"}
		switch mode {
		case "legacy":
			ex = []string{"HC_LANE_OWNER=box/t1", "HC_LANE_OWNER_RUN="}
		case "unowned":
			ex = []string{"HC_LANE_OWNER=*", "HC_LANE_OWNER_RUN="}
		}
		cmust(k, mode+": submit lane", f.submit("lane", ex...))
		r := cmust(k, mode+": tick", f.tick("HC_LANE_OWNER=t9", "HC_LANE_OWNER_RUN=99"))
		event, owner, to := "착지", "box/t1@11", "box/t1"
		switch mode {
		case "returned":
			event = "막힘"
		case "legacy":
			owner = "box/t1"
		case "unowned":
			owner, to = "*", "-"
		}
		rs := evs(f.evPath(), "lane", event)
		k.check(mode+": 사건 9·11칸 = 받는 지휘·주인", len(rs) == 1 && col(strings.Split(rs[0], "\t"), 9) == to &&
			col(strings.Split(rs[0], "\t"), 11) == owner, clog(r), strings.Join(rs, "\n"))
		if owner != "box/t1@11" || len(rs) != 1 {
			continue
		}
		write(t, f.home+"/.local/state/hc-lane/coord.t1", "t1\t123\towner\t0\n")
		watch := func(created string) R {
			return f.lane(f.writer, []string{"PATH=" + k.T + "/watch-bin:" + k.env["PATH"], "watch_created=" + created,
				"HC_LANE_WATCH_IDLE=0", "HC_LANE_WAIT_TICK=0", "HC_LANE_WATCH_HOLD_MAX=0"}, "watch", "--to", "t1")
		}
		// 다른 실행(22)을 먼저: 거른 줄은 커서에 안 적히니 뒤의 같은 실행(11) 바퀴가 그 줄을 다시 본다
		r = watch("22")
		k.check(mode+": 다른 실행이면 안 보냄", r.RC == 0 && !hasLine(r.Out, rs[0]), clog(r))
		r = watch("11")
		k.check(mode+": 같은 실행이면 보냄", r.RC == 0 && hasLine(r.Out, rs[0]), clog(r))
	}
}
