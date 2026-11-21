package main

// git을 쓰는 단위 시험: 임시 bare 원격 + 병합기 클론, 가짜 hc-lane(사건만 적음)·가짜 git(끊김). 실제 hc·망은 쓰지 않는다.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type fx struct {
	t                        *testing.T
	dir, origin, seed, clone string
	events                   string
}

func (f *fx) git(dir string, args ...string) string {
	f.t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	o, err := c.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, o)
	}
	return strings.TrimSpace(string(o))
}

// newFx: 원격(main에 base 커밋)과 병합기 클론. 가짜 hc-lane은 report 인자를 events에 한 줄로 적는다
func newFx(t *testing.T) *fx {
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := &fx{t: t, dir: d, origin: d + "/o.git", seed: d + "/seed", clone: d + "/m", events: d + "/events"}
	for k, v := range map[string]string{"HOME": d, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null",
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t",
		"HC_LANE_FETCH_WAITS": "", "XDG_STATE_HOME": d + "/state"} {
		t.Setenv(k, v)
	}
	f.git(d, "init", "-q", "--bare", "-b", "main", f.origin)
	f.git(d, "init", "-q", "-b", "main", f.seed)
	os.WriteFile(f.seed+"/base.txt", []byte("base\n"), 0o666)
	f.git(f.seed, "add", "-A")
	f.git(f.seed, "commit", "-qm", "base")
	f.git(f.seed, "remote", "add", "origin", f.origin)
	f.git(f.seed, "push", "-q", "origin", "main")
	f.git(d, "clone", "-q", f.origin, f.clone)
	self := d + "/hc-lane"
	gSelf, gEvdir, gRun = self, d+"/state/hc-lane", false
	// 진짜 report처럼 사건 파일(…/events.tsv — 4칸 레인·5칸 상태·7칸 글)에도 적는다: 같은 사건을 두 번 내지 않는지 보려면 필요하다
	os.WriteFile(self, []byte("#!/bin/sh\nprintf '%s|' \"$@\" >>"+f.events+"\necho >>"+f.events+
		"\nprintf 't\\tbox\\t-\\t%s\\t%s\\t%s\\t%s\\n' \"$6\" \"$2\" \"$3\" \"$4\" >>"+gEvdir+"/events.tsv\n"), 0o777)
	os.MkdirAll(gEvdir, 0o777)
	t.Chdir(f.clone)
	return f
}

func (f *fx) merger() *M {
	mj := f.clone + "/.git/hc-merger"
	os.MkdirAll(mj+"/out", 0o777)
	os.MkdirAll(mj+"/fail", 0o777)
	os.MkdirAll(f.clone+".merger-wt", 0o777)
	return &M{parent: os.Getppid(), top: f.clone, base: "main", mj: mj, mwr: f.clone + ".merger-wt", ml: f.dir, mme: "box", batch: 3}
}

// submit: seed에서 main 위 커밋 하나를 큐 이름 <초>-<레인>으로 낸다(큐 표식·제출 기록·lane 가지 — 제출 쪽과 같은 꼴)
func (f *fx) submit(sec int, lane, prio string) (q, sha, meta, id string) {
	f.git(f.seed, "checkout", "-q", "-B", lane, "main")
	os.WriteFile(f.seed+"/"+lane+".txt", []byte(lane+"\n"), 0o666)
	f.git(f.seed, "add", "-A")
	f.git(f.seed, "commit", "-qm", lane)
	sha = f.git(f.seed, "rev-parse", "HEAD")
	fork := f.git(f.seed, "merge-base", "HEAD", "main")
	et := f.git(f.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
	id = "box-" + lane
	meta = f.git(f.seed, "commit-tree", et, "-p", sha, "-m",
		fmt.Sprintf("merge-queue lane=%s id=%s orig=%s fork=%s base=main host=box term=- owner=- job=- created_ms=- priority=%s", lane, id, sha, fork, prio))
	name := fmt.Sprintf("%d-%s", sec, lane)
	q = "refs/merge-queue/" + name
	f.git(f.seed, "push", "-q", "origin", sha+":"+q, meta+":refs/merge-queue-meta/"+name, sha+":refs/heads/lane/"+lane)
	f.git(f.seed, "checkout", "-q", "main")
	return
}

func (f *fx) has(ref string) bool {
	c := exec.Command("git", "--git-dir="+f.origin, "rev-parse", "-q", "--verify", ref)
	return c.Run() == nil
}

func TestQueuePriorityThenTime(t *testing.T) {
	f := newFx(t)
	f.submit(1000000001, "low", "낮음")
	f.submit(1000000002, "plain", "보통")
	f.submit(1000000003, "urgent", "긴급")
	f.submit(1000000004, "old", "") // 우선 없는 옛 기록 = 보통
	m := f.merger()
	m.qs = f.git(f.clone, "ls-remote", "origin", "refs/merge-queue/*", "refs/merge-queue-meta/*")
	f.git(f.clone, "fetch", "-q", "origin", "+refs/merge-queue-meta/*:refs/hc-merger/meta/*")
	q, ok := m.queue()
	if !ok {
		t.Fatal("queue 실패")
	}
	var got []string
	for _, e := range q {
		got = append(got, e[0][strings.LastIndex(e[0], "-")+1:])
	}
	if strings.Join(got, ",") != "urgent,plain,old,low" {
		t.Fatalf("우선 → 제출 시각 순이어야: %v", got)
	}
}

// fakeGit: 인자에 pat가 들면 종료 128(「모름」), 아니면 진짜 git
func fakeGit(t *testing.T, pat string) {
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	os.WriteFile(bin+"/git", []byte(fmt.Sprintf("#!/bin/sh\ncase \" $* \" in *%q*) echo 'fatal: unable to access' >&2; exit 128 ;; esac\nexec %q \"$@\"\n", pat, real)), 0o777)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
}

func TestGitUnknownStopsRound(t *testing.T) {
	f := newFx(t)
	f.git(f.seed, "push", "-q", "origin", "main:refs/herdr/merger/box")
	q, _, _, _ := f.submit(1000000001, "lane", "보통")
	m := f.merger()
	fakeGit(t, " ls-remote ")
	m.round()
	if !m.mer || m.mq != "" {
		t.Fatalf("원격을 못 읽으면 쉬어야(mer) 한다: mer=%v mq=%q", m.mer, m.mq)
	}
	if !f.has(q) || f.has("refs/merge-failed/1000000001-lane") {
		t.Fatal("모름이면 큐를 건드리지 않아야 한다")
	}
	// 제출 기록을 못 읽으면(git log 실패) 「없음」이 아니라 실패
	f.git(f.clone, "fetch", "-q", "origin", "+refs/merge-queue-meta/*:refs/hc-merger/meta/*")
	meta := f.git(f.clone, "rev-parse", "refs/hc-merger/meta/1000000001-lane")
	sha := f.git(f.clone, "rev-parse", meta+"^")
	fakeGit(t, " log -1 ")
	if _, ok := readMeta(meta, sha); ok {
		t.Fatal("git log 실패를 빈 기록으로 읽으면 안 된다")
	}
}

// 복구 일지 각 단계에서 죽기: 착지 전 · main push 뒤(정리 전) · 정리 뒤(사건 전)
func TestRecoverStages(t *testing.T) {
	f := newFx(t)
	q, sha, meta, id := f.submit(1000000001, "lane", "보통")
	m := f.merger()
	B := f.git(f.clone, "rev-parse", "origin/main")
	journal := fmt.Sprintf("lane %s %s\n%s %s lane %s\n", B, sha, q, sha, meta)
	f.git(f.clone, "fetch", "-q", "origin", "+refs/merge-queue-meta/*:refs/hc-merger/meta/*", "+refs/merge-queue/*:refs/hc-merger/q/*")

	// 1) 착지 전에 죽음: 표식이 없다 — 후보를 버리고 큐는 그대로(다시 고른다), 사건 없음
	os.WriteFile(m.mj+"/cur", []byte(journal), 0o666)
	if !m.recover() || m.mq != "x" {
		t.Fatal("일지를 정리해야 한다")
	}
	if _, err := os.Stat(m.mj + "/cur"); err == nil || !f.has(q) {
		t.Fatal("일지는 지우고 큐는 남겨야 한다")
	}
	if _, err := os.Stat(f.events); err == nil {
		t.Fatal("넣지 않은 제출의 사건을 쓰면 안 된다")
	}

	// 2) main push(제출 ID 표식·큐 삭제가 원자로 함께) 뒤, 정리 전에 죽음 — 완료로 정리하고 착지 사건 하나
	f.git(f.seed, "push", "-q", "--atomic", "origin", sha+":refs/heads/main", sha+":refs/hc-landed/"+id, ":"+q,
		":refs/merge-queue-meta/1000000001-lane", meta+":refs/merge-done/lane", ":refs/heads/lane/lane")
	os.WriteFile(m.mj+"/cur", []byte(journal), 0o666)
	m = f.merger()
	if !m.recover() {
		t.Fatal("정리해야 한다")
	}
	ev, _ := os.ReadFile(f.events)
	if strings.Count(string(ev), "report|착지|") != 1 || !strings.Contains(string(ev), "앞 병합기 실행이 넣었음") {
		t.Fatalf("착지 사건 하나: %s", ev)
	}
	if out := f.git(f.clone, "ls-remote", "origin", "refs/hc-landed/lane/*"); !strings.Contains(out, sha) {
		t.Fatal("레인 표식을 둬야 한다")
	}

	// 같은 일지를 다시 정리해도(사건 전 죽음 뒤 재시도) 사건·일지 줄은 하나
	os.WriteFile(m.mj+"/cur", []byte(journal), 0o666)
	if !f.merger().recover() {
		t.Fatal("다시 정리해야 한다")
	}
	ev, _ = os.ReadFile(f.events)
	if strings.Count(string(ev), "report|착지|") != 1 {
		t.Fatalf("다시 정리해도 사건은 하나: %s", ev)
	}
	jr, _ := os.ReadFile(gEvdir + "/merger-journal.tsv")
	if strings.Count(string(jr), "\t착지\t") != 1 {
		t.Fatalf("병합기 일지 줄은 하나: %s", jr)
	}

	// 3) 정리 뒤 사건 전에 죽음: 보낼 함에 남은 사건을 다음 바퀴가 보내고 지운다
	os.WriteFile(m.mj+"/out/1000000002-x", []byte("착지\n-\n보낼 함 시험\nx\n\n\n\n\n\n\n\n"), 0o666)
	f.merger().flush()
	ev, _ = os.ReadFile(f.events)
	if !strings.Contains(string(ev), "보낼 함 시험") {
		t.Fatal("보낼 함의 사건을 보내야 한다")
	}
	if _, err := os.Stat(m.mj + "/out/1000000002-x"); err == nil {
		t.Fatal("보낸 뒤 지워야 한다")
	}
}

func TestBcutKeepsWholeRunes(t *testing.T) {
	s := strings.Repeat("가", 120) // 360바이트
	if got := bcut(300, s); len(got) != 300 || got != strings.Repeat("가", 100) {
		t.Fatalf("300바이트에서 온 글자로: %d", len(got))
	}
	if got := bcut(301, s); got != strings.Repeat("가", 100) {
		t.Fatal("반쪽 글자는 버려야 한다")
	}
}
