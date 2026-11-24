package main

// 2026-11-24 지휘: 「배포 밀림」 거짓 양성 — 착지 직후·착지 뒤 배포가 도는 중에는 밀림 항목·사건이 없고, deploy_grace분 넘게 밀려 있으면
// 항목 하나·사건 한 번. 가짜 범용 저장소(origin/main이 기록상 판보다 5커밋 앞, 커밋 시각은 두 시간 전)와 배포 기록.
// 고치기 전 코드로는 착지 직후·도는 중에도 deploy 항목이 나와 실패한다(deployHold가 없어 컴파일부터)

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/opsstate"
)

func dhFx(t *testing.T) {
	t.Helper()
	advFx(t)
	repo := t.TempDir()
	old := strconv.FormatInt(time.Now().Add(-2*time.Hour).Unix(), 10) + " +0000"
	git := func(a ...string) string {
		c := exec.Command("git", append([]string{"-C", repo}, a...)...)
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
			"GIT_AUTHOR_DATE="+old, "GIT_COMMITTER_DATE="+old)
		b, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", a, err, b)
		}
		return strings.TrimSpace(string(b))
	}
	git("init", "-q")
	var first string
	for i := 0; i < 6; i++ {
		git("commit", "-q", "--allow-empty", "-m", "c"+strconv.Itoa(i))
		if i == 0 {
			first = git("rev-parse", "HEAD")
		}
	}
	git("update-ref", "refs/remotes/origin/main", "HEAD")
	t.Setenv("HERDR_TOOLS_REPO", repo)
	d := HOME + "/.local/state/herdr-sync"
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	line := strings.Join([]string{time.Now().Add(-time.Hour).UTC().Format(time.RFC3339), "landed", "0", first[:12], "-", "-", "-", "m=씀"}, "\t")
	if err := os.WriteFile(d+"/deploy.tsv", []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(onlandDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	confLines = append(confLines, "on-land deploy tools herdr-sync --landed", "on-land build app make app")
}

func deployItems(now time.Time) []opsstate.Item {
	var o []opsstate.Item
	for _, it := range adviseNow(now).Items {
		if it.Kind == "deploy" {
			o = append(o, it)
		}
	}
	return o
}

func TestAdviseDeployHoldAfterLand(t *testing.T) {
	dhFx(t)
	now := time.Now()
	// 착지 직후: 착지마다 기준(head)이 방금 바뀜
	head := onlandDir() + "/deploy.head"
	if err := os.WriteFile(head, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if it := deployItems(now); len(it) != 0 {
		t.Fatalf("착지 직후 밀림: %+v", it)
	}
	if err := os.Chtimes(head, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	// 배포가 도는 중(표식의 pid가 살아 있음)
	mark := HOME + "/.local/state/herdr-sync/deploying"
	if err := os.WriteFile(mark, []byte(strconv.Itoa(os.Getpid())+"\t"+strconv.FormatInt(now.Unix(), 10)+"\tlanded\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if it := deployItems(now); len(it) != 0 {
		t.Fatalf("배포 도는 중 밀림: %+v", it)
	}
	// 착지 뒤 명령 차례가 남음
	if err := os.Remove(mark); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(onlandDir()+"/deploy.pending", []byte("lane-a\t-\tabc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if it := deployItems(now); len(it) != 0 {
		t.Fatalf("착지 뒤 명령 차례 남음 밀림: %+v", it)
	}
	if err := os.Remove(onlandDir() + "/deploy.pending"); err != nil {
		t.Fatal(err)
	}
	// deploy_grace(5분) 넘게 밀려 있음 → 항목 하나, 사건 한 번
	if it := deployItems(now); len(it) != 1 || it[0].Key != "deploy:m" {
		t.Fatalf("밀림 항목 하나여야: %+v", it)
	}
	adviseTick(now)
	adviseTick(now.Add(6 * time.Minute))
	if n := grepN(reports(t), "배포 m: 5커밋 밀림"); n > 1 {
		t.Fatalf("밀림 사건은 한 번: %q", reports(t))
	}
	var seen bool
	for _, l := range readLines(DATA + "/advise.tsv") {
		seen = seen || l == "deploy:m"
	}
	if !seen {
		t.Fatalf("밀림 항목이 사건으로 나가지 않음: %q", reports(t))
	}
}
