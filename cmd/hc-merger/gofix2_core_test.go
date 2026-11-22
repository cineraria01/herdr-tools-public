//go:build !windows

package main

// gofix2(Go 전환 재감사 아스트라 1바퀴) — 고치기 전 코드에서 실패하던 시험(두 번째 묶음: R-H01·R-H02·R-H08·R-C06·R-C08·R-T03)

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// revertLane: 승인 제출 — shared.txt를 main-change로 바꿨다 되돌리고(순변경 없음) 레인 파일 하나를 더한다. 순변경(승인 patch-id)은
// 레인 파일뿐이다. 그 뒤 main에도 같은 중간 변경(shared.txt=main-change)이 따로 들어간다(moveMain이 참이면 지금, 아니면 부르는 쪽이)
func (f *fx) revertLane(sec int, lane string) (q, sha string) {
	f.t.Helper()
	if !f.onMain("shared.txt") {
		os.WriteFile(f.seed+"/shared.txt", []byte("base\n"), 0o666)
		f.git(f.seed, "add", "-A")
		f.git(f.seed, "commit", "-qm", "shared base")
		f.git(f.seed, "push", "-q", "origin", "main")
	}
	f.git(f.seed, "checkout", "-q", "-B", lane, "main")
	for _, body := range []string{"main-change\n", "base\n"} {
		os.WriteFile(f.seed+"/shared.txt", []byte(body), 0o666)
		f.git(f.seed, "commit", "-qam", lane+" shared "+strings.TrimSpace(body))
	}
	os.WriteFile(f.seed+"/"+lane+".txt", []byte(lane+"\n"), 0o666)
	f.git(f.seed, "add", "-A")
	f.git(f.seed, "commit", "-qm", lane)
	sha = f.git(f.seed, "rev-parse", "HEAD")
	fork := f.git(f.seed, "merge-base", "HEAD", "main")
	patch := f.patchOf(fork, sha)
	et := f.git(f.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
	meta := f.git(f.seed, "commit-tree", et, "-p", sha, "-m",
		fmt.Sprintf("merge-queue lane=%s id=box-%s orig=%s fork=%s base=main host=box term=- owner=- job=- created_ms=- priority=보통 verdict=v-%s patch=%s", lane, lane, sha, fork, lane, patch))
	name := fmt.Sprintf("%d-%s", sec, lane)
	q = "refs/merge-queue/" + name
	f.git(f.seed, "push", "-q", "origin", sha+":"+q, meta+":refs/merge-queue-meta/"+name, sha+":refs/heads/lane/"+lane)
	f.git(f.seed, "checkout", "-q", "main")
	return
}

// mainChange: seed의 가지 br(main 위)에 shared.txt=main-change 커밋. push하면 main에 「같은 중간 변경」이 따로 든다
func (f *fx) mainChange(br string) {
	f.git(f.seed, "checkout", "-q", "-B", br, "main")
	os.WriteFile(f.seed+"/shared.txt", []byte("main-change\n"), 0o666)
	f.git(f.seed, "commit", "-qam", "main-change")
	f.git(f.seed, "checkout", "-q", "main")
}

func (f *fx) mainFile(file string) string {
	b, _ := exec.Command("git", "--git-dir="+f.origin, "show", "main:"+file).Output()
	return string(b)
}

// R-H01: 승인 구성원이 묶음 뒤쪽이면 — 묶음에 얹은 결과가 승인 순변경과 다르다(rebase가 빈 커밋을 버려 shared.txt를 옛 값으로 되돌림).
// 그 구성원은 빼고(큐에 그대로) 나머지만 착지, main의 shared.txt=main-change는 보존
func TestGofix2BatchTreeChecked(t *testing.T) {
	f, _ := batchFx(t)
	f.sub2(1000000001, "p1", "", "", "p1.txt", "p1\n", false)
	qr, _ := f.revertLane(1000000002, "r1")
	f.mainChange("mc")
	f.git(f.seed, "push", "-q", "origin", "mc:refs/heads/main")
	m := f.merger()
	m.round()
	if got := f.mainFile("shared.txt"); got != "main-change\n" {
		t.Fatalf("승인 밖 파일 shared.txt를 되돌렸다: %q", got)
	}
	if !f.onMain("p1.txt") || f.onMain("r1.txt") || !f.has(qr) || f.has("refs/merge-failed/1000000002-r1") {
		t.Fatal("p1만 들어가고 r1은 큐에 그대로여야")
	}
}

// R-H01: 승인 구성원이 맨 앞이고 얹은 결과가 순변경과 다르면 묶음이 아니라 원본 그대로 혼자 승인 착지(착지가 판정·트리 대조를 본다)
func TestGofix2BatchFrontSolo(t *testing.T) {
	f, landlog := batchFx(t)
	f.revertLane(1000000001, "r1")
	f.sub2(1000000002, "p1", "", "", "p1.txt", "p1\n", false)
	f.mainChange("mc")
	f.git(f.seed, "push", "-q", "origin", "mc:refs/heads/main")
	m := f.merger()
	m.round()
	if got := landLines(t, landlog); len(got) != 1 || !strings.HasPrefix(got[0], "land|v-r1|") || !strings.HasSuffix(got[0], "|r1") {
		t.Fatalf("r1은 혼자 자기 판정으로 착지를 불러야: %v", got)
	}
}

// R-H01: 승인 묶음은 확인한 기준 위에서만 올라간다 — 착지 도중 main이 움직이면(같은 중간 변경이 먼저 듦) 착지의 rebase가 다시
// shared.txt를 되돌린 후보를 만들지만 기준 lease에 막혀 main에 들지 않는다
func TestGofix2BatchLeasesBase(t *testing.T) {
	f, _ := batchFx(t)
	f.revertLane(1000000001, "r1")
	f.sub2(1000000002, "p1", "", "", "p1.txt", "p1\n", false)
	f.mainChange("mover")
	moved := f.dir + "/moved"
	t.Setenv("MOVED", moved)
	t.Setenv("MOVESEED", f.seed)
	old, err := os.ReadFile(gSelf)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Replace(string(old), "  git fetch -q origin || exit 1\n",
		"  [ -e \"$MOVED\" ] || { git -C \"$MOVESEED\" push -q origin mover:refs/heads/main && : >\"$MOVED\"; }\n  git fetch -q origin || exit 1\n", 1)
	if body == string(old) {
		t.Fatal("가짜 착지를 고치지 못했다")
	}
	if err := os.WriteFile(gSelf, []byte(body), 0o777); err != nil {
		t.Fatal(err)
	}
	m := f.merger()
	m.round()
	if _, err := os.Stat(moved); err != nil {
		t.Fatal("착지 도중 main을 움직이지 않았다")
	}
	if got := f.mainFile("shared.txt"); got != "main-change\n" {
		t.Fatalf("기준이 움직인 뒤 승인 묶음이 shared.txt를 되돌렸다: %q", got)
	}
}

// R-H02: 승인 patch-id를 git이 못 구하면(128) 모름 — 묶음에서 빼 혼자 착지로 넘기지 않고(그 대조도 실패하면 반송됐다) 큐를 둔 채 쉰다
func TestGofix2PatchIDUnknownRests(t *testing.T) {
	f, landlog := batchFx(t)
	qa, _ := f.sub2(1000000001, "a1", "v-a1", "", "a1.txt", "a1\n", false)
	qb, _ := f.sub2(1000000002, "a2", "v-a2", "", "a2.txt", "a2\n", false)
	fakeGit(t, " patch-id ")
	m := f.merger()
	m.round()
	if !m.mer || m.mq != "" {
		t.Fatalf("모름이면 쉬어야: mer=%v mq=%q", m.mer, m.mq)
	}
	if got := landLines(t, landlog); len(got) != 0 {
		t.Fatalf("착지를 부르면 안 된다: %v", got)
	}
	if !f.has(qa) || !f.has(qb) || f.has("refs/merge-failed/1000000001-a1") {
		t.Fatal("큐를 그대로 둬야(반송 없음)")
	}
}

// R-C06: push 뒤 복구도 정상 완료처럼 구성원별 판정·patch-id를 병합기 일지에 남긴다(옛 일지 cur 꼴 그대로)
func TestGofix2RecoverJournalVerdict(t *testing.T) {
	f, _ := batchFx(t)
	qa, sa := f.sub2(1000000001, "a1", "v-a1", "", "a1.txt", "a1\n", false)
	qb, sb := f.sub2(1000000002, "a2", "v-a2", "", "a2.txt", "a2\n", false)
	ma := f.git(f.seed, "ls-remote", "origin", "refs/merge-queue-meta/1000000001-a1")
	mb := f.git(f.seed, "ls-remote", "origin", "refs/merge-queue-meta/1000000002-a2")
	ma, mb = strings.Fields(ma)[0], strings.Fields(mb)[0]
	m := f.merger()
	B := f.git(f.clone, "rev-parse", "origin/main")
	f.git(f.clone, "fetch", "-q", "origin", "+refs/merge-queue-meta/*:refs/hc-merger/meta/*", "+refs/merge-queue/*:refs/hc-merger/q/*")
	// 앞 실행이 묶음을 main에 넣고(표식·큐 삭제가 원자로 함께) 정리 전에 죽었다
	f.git(f.seed, "merge", "-q", "--no-ff", "-m", "batch", sa, sb)
	top := f.git(f.seed, "rev-parse", "HEAD")
	f.git(f.seed, "push", "-q", "--atomic", "origin", top+":refs/heads/main", top+":refs/hc-landed/box-a1", top+":refs/hc-landed/box-a2",
		":"+qa, ":"+qb)
	journal := fmt.Sprintf("a1 %s %s\n%s %s a1 %s\n%s %s a2 %s\n", B, sa, qa, sa, ma, qb, sb, mb)
	os.WriteFile(m.mj+"/cur", []byte(journal), 0o666)
	if !m.recover() {
		t.Fatal("정리해야 한다")
	}
	jr, _ := os.ReadFile(gEvdir + "/merger-journal.tsv")
	for _, l := range []string{"a1", "a2"} {
		ok := false
		for _, line := range strings.Split(string(jr), "\n") {
			c := strings.Split(line, "\t")
			if len(c) == 12 && c[2] == l && c[9] == "착지" && strings.Contains(c[11], "앞 병합기 실행이 넣었음") &&
				strings.Contains(c[11], "승인 확인 판정 v-"+l+" patch-id ") {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("%s 복구 일지 줄에 판정·patch-id가 있어야:\n%s", l, jr)
		}
	}
}

// R-C08: 같은 까닭으로 deferAlarm바퀴째 쉰 막힘 사건 쓰기가 실패하면 다음 바퀴에 다시 낸다(한 번 전달되면 더는 내지 않는다)
func TestGofix2DeferAlarmRetries(t *testing.T) {
	f := newFx(t)
	m := f.merger()
	failf := f.dir + "/failreport"
	log := f.dir + "/reports"
	os.WriteFile(gSelf, []byte("#!/bin/sh\n[ -e "+failf+" ] && exit 1\nprintf '%s|' \"$@\" >>"+log+"\necho >>"+log+"\n"), 0o777)
	os.WriteFile(m.mj+"/defer", []byte(fmt.Sprintf("%d\t까닭\n", deferAlarm-1)), 0o666) // 옛 꼴(두 칸)
	os.WriteFile(failf, nil, 0o666)
	m.deferred = "까닭"
	m.deferNote() // deferAlarm번째 — 사건 쓰기 실패
	os.Remove(failf)
	m.deferNote() // 다음 바퀴 — 다시 낸다
	m.deferNote() // 전달된 뒤 — 더 내지 않는다
	b, _ := os.ReadFile(log)
	if n := strings.Count(string(b), "report|막힘|"); n != 1 {
		t.Fatalf("실패한 막힘 사건을 한 번 다시 내야(전달 뒤엔 그만): %d번\n%s", n, b)
	}
	if !strings.Contains(string(b), "|--key|merger:defer:") {
		t.Fatalf("고정 열쇠로 내야: %s", b)
	}
}

// R-T03: 착지 rebase 충돌(종료 3)의 반송 사건에 충돌 파일이 정확히 든다 — hc-fill 다시 맞춤이 읽는 꼴 「착지 실패(종료 3, 충돌 <파일>)」
func TestGofix2ConflictEventFile(t *testing.T) {
	f, _ := batchFx(t)
	f.sub2(1000000001, "c1", "", "", "base.txt", "lane\n", false)
	os.WriteFile(f.seed+"/base.txt", []byte("main\n"), 0o666)
	f.git(f.seed, "commit", "-qam", "main moves")
	f.git(f.seed, "push", "-q", "origin", "main")
	m := f.merger()
	m.round()
	if !f.has("refs/merge-failed/1000000001-c1") {
		t.Fatal("충돌한 제출은 반송돼야")
	}
	ev, _ := os.ReadFile(f.events)
	var hit string
	for _, line := range strings.Split(string(ev), "\n") {
		if strings.HasPrefix(line, "report|막힘|") {
			hit = line
		}
	}
	if !strings.Contains(hit, "병합기: 착지 실패(종료 3, 충돌 base.txt) — ") {
		t.Fatalf("반송 사건에 충돌 파일이 정확히 있어야: %q", hit)
	}
}

// slowGit: 인자에 pat가 든 git 호출만 30초 멈춘다(once 파일이 주어지면 첫 번만) — HERDR_TIMEOUT_GIT=1과 함께 시간 넘김(124)
func slowGit(t *testing.T, pat, once string) string {
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	cond := ""
	if once != "" {
		cond = "[ -e " + once + " ] || { : >" + once + "; sleep 30; }"
	} else {
		cond = "sleep 30"
	}
	os.WriteFile(bin+"/git", []byte(fmt.Sprintf("#!/bin/sh\ncase \" $* \" in *%q*) %s ;; esac\nexec %q \"$@\"\n", pat, cond, real)), 0o777)
	return bin
}

// R-H08: 시작 단계 git의 시간 넘김(모름)은 「저장소 아님·origin 없음」이 아니다 — 단발(--once)은 바꾼 것 없이 75
func TestGofix2StartupGitTimeoutOnce(t *testing.T) {
	for _, pat := range []string{" rev-parse --show-toplevel ", " remote get-url origin ", " status --porcelain "} {
		f := newFx(t)
		self, log := recorder(t)
		bin := slowGit(t, pat, "")
		rc, out := sub(t, "HC_MERGER_TEST_HELPER=resident", "HC_MERGER_TEST_ARGS=--once --repo "+f.clone, "HC_LANE_SELF="+self,
			"PATH="+bin+":"+os.Getenv("PATH"), "HERDR_TIMEOUT_GIT=1", "HERDR_PROC_DIR="+t.TempDir())
		if rc != 75 {
			t.Errorf("%s 시간 넘김이면 75: rc %d\n%s", pat, rc, out)
		}
		if b, _ := os.ReadFile(log); strings.Contains(string(b), "막힘") {
			t.Errorf("%s: 멈춤 사건을 내면 안 된다: %s", pat, b)
		}
	}
}

// R-H08: 상주는 시작 단계 git이 모름이면 멈추지 않고 다시 해 큐 보기에 들어간다
func TestGofix2StartupRetries(t *testing.T) {
	f := newFx(t)
	self, _ := recorder(t)
	bin := slowGit(t, " remote get-url origin ", f.dir+"/slow-once")
	c := exec.Command(os.Args[0], "-test.run=^$")
	c.Env = append(os.Environ(), "HC_MERGER_TEST_HELPER=resident", "HC_MERGER_TEST_ARGS=--tick 1 --repo "+f.clone, "HC_LANE_SELF="+self,
		"PATH="+bin+":"+os.Getenv("PATH"), "HERDR_TIMEOUT_GIT=1", "HERDR_PROC_DIR="+t.TempDir())
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	pr, err := c.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	c.Stderr = c.Stdout
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { syscall.Kill(-c.Process.Pid, syscall.SIGKILL); c.Wait() }()
	seen := make(chan string, 1)
	go func() {
		var all strings.Builder
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			all.WriteString(sc.Text() + "\n")
			if strings.Contains(sc.Text(), "큐를 본다") {
				seen <- all.String()
				return
			}
		}
		seen <- "끝남(큐 보기 전):\n" + all.String()
	}()
	select {
	case out := <-seen:
		if !strings.Contains(out, "큐를 본다") || !strings.Contains(out, "다시(1번째)") {
			t.Fatalf("시작 단계 모름 뒤 다시 해 큐 보기에 들어가야:\n%s", out)
		}
	case <-time.After(90 * time.Second):
		t.Fatal("90초 안에 큐 보기에 들어가지 않았다")
	}
}
