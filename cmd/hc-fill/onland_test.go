package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 착지 뒤 명령(--on-land, 레인 go-land-build): 착지 → 한 번, 도는 동안 둘 착지 → 겹치지 않고 마지막 판으로 한 번 더, 실패 사건

// onlandEnv: 설정·가짜 hc-lane(받은 인자를 줄마다 적음). 돌려주는 값은 그 기록 파일
func onlandEnv(t *testing.T, repo, cmdline string) string {
	t.Helper()
	setData(t)
	LOCAL, FETCHED, FETCHBAD, dry = "here", "", "", false
	confLines = []string{"local here", "repo app here " + repo, "on-land dev app " + cmdline}
	bin := t.TempDir()
	rec := bin + "/reports"
	sh := "#!/bin/sh\nprintf '%s|' \"$@\" >> " + rec + "\necho >> " + rec + "\n"
	if err := os.WriteFile(bin+"/hc-lane", []byte(sh), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	return rec
}

func gitT(t *testing.T, dir string, env []string, a ...string) string {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid"}, a...)...)
	c.Env = append(os.Environ(), env...)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", a, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestOnlandTickFiresOnceOnLanding(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 없음")
	}
	d := t.TempDir()
	origin, up, local := d+"/o.git", d+"/up", d+"/local"
	gitT(t, d, nil, "init", "-q", "--bare", "-b", "main", origin)
	gitT(t, d, nil, "clone", "-q", origin, up)
	// 같은 이름의 옛 착지(접수 전 커밋 시각)는 이번 착지가 아니다
	old := []string{"GIT_COMMITTER_DATE=@1700000000 +0000", "GIT_AUTHOR_DATE=@1700000000 +0000"}
	gitT(t, up, old, "commit", "-q", "--allow-empty", "-m", "fix-a: 옛 착지")
	gitT(t, up, nil, "push", "-q", "origin", "main")
	gitT(t, d, nil, "clone", "-q", origin, local)
	onlandEnv(t, local, "true")
	SELF = "true" // 실행기는 띄운 척만
	ts := nowUnix() - 5
	if err := os.WriteFile(DATA+"/onland.tsv", []byte(fmt.Sprintf("fix-a\tdev\tapp\tt9\t%d\n", ts)), 0o644); err != nil {
		t.Fatal(err)
	}
	onlandTick()
	if nonEmpty(onlandDir() + "/dev.pending") {
		t.Fatal("옛 착지로 명령을 돌리려 함")
	}
	gitT(t, up, nil, "commit", "-q", "--allow-empty", "-m", "fix-a: 새 착지")
	sha := gitT(t, up, nil, "rev-parse", "HEAD")
	gitT(t, up, nil, "push", "-q", "origin", "main")
	FETCHED = ""
	onlandTick()
	FETCHED = ""
	onlandTick() // 두 번째 바퀴는 더 넣지 않는다
	rs := readRows(onlandDir() + "/dev.pending")
	if len(rs) != 1 || rs[0].F(1) != "fix-a" || rs[0].F(2) != "t9" || rs[0].F(3) != sha {
		t.Fatalf("pending = %v, 바람 fix-a t9 %s 한 줄", rs, sha)
	}
	if nonEmpty(DATA + "/onland.tsv") {
		t.Fatal("착지한 줄이 onland.tsv에 남음")
	}
}

func TestOnlandRunCoalescesWhileRunning(t *testing.T) {
	d := t.TempDir()
	out, started, goF := d+"/out", d+"/started", d+"/go"
	rec := onlandEnv(t, d, fmt.Sprintf(`echo "$HC_ONLAND_LANES $HC_ONLAND_SHA" >> %s; touch %s; while [ ! -e %s ]; do sleep 0.05; done`, out, started, goF))
	if err := onlandPend("dev", "a", "t1", "s1"); err != nil {
		t.Fatal(err)
	}
	done := make(chan int)
	go func() { done <- onlandRun("dev") }()
	for i := 0; !isFile(started); i++ {
		if i > 200 {
			t.Fatal("첫 명령이 안 돎")
		}
		time.Sleep(25 * time.Millisecond)
	}
	for _, x := range [][3]string{{"b", "t1", "s2"}, {"c", "t2", "s3"}} {
		if err := onlandPend("dev", x[0], x[1], x[2]); err != nil {
			t.Fatal(err)
		}
	}
	if rc := onlandRun("dev"); rc != 0 { // 도는 실행기가 있으면 바로 돌아온다(겹쳐 돌지 않음)
		t.Fatalf("두 번째 실행기 rc %d", rc)
	}
	if err := os.WriteFile(goF, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case rc := <-done:
		if rc != 0 {
			t.Fatalf("rc %d", rc)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("실행기가 안 끝남")
	}
	got, _ := readFile(out)
	if got != "a s1\nb,c s3\n" {
		t.Fatalf("명령 실행 = %q, 바람 한 번 + 합친 한 번(마지막 판 s3)", got)
	}
	reps, _ := readFile(rec)
	ls := lines(strings.TrimSpace(reps))
	if len(ls) != 3 { // 첫 번(t1) + 합친 번(t1·t2)
		t.Fatalf("사건 %d줄:\n%s", len(ls), reps)
	}
	for _, l := range ls {
		if !strings.HasPrefix(l, "report|중간|-|착지 뒤 dev 성공") || !strings.Contains(l, "|--name|on-land-dev|--to|t") {
			t.Fatalf("사건 꼴: %s", l)
		}
	}
	if !strings.Contains(ls[2], "|--to|t2|") || !strings.Contains(ls[1], "b,c 착지(s3)") {
		t.Fatalf("합친 번 사건이 두 주인에게 가지 않음:\n%s", reps)
	}
	if nonEmpty(onlandDir() + "/dev.pending") {
		t.Fatal("pending이 남음")
	}
}

// Windows 교차 빌드(gofix-fill이 맞춤) — 실행기 띄우기가 syscall.Setpgid를 직접 쓰면 깨졌다(proc.Group으로)
func TestCrossBuildWindows(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go 없음")
	}
	c := exec.Command(goBin, "build", "-o", os.DevNull, ".")
	c.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0", "GOTOOLCHAIN=local")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("windows/amd64 빌드 실패: %v\n%s", err, out)
	}
}

func TestOnlandRunFailureReportsBlocked(t *testing.T) {
	d := t.TempDir()
	rec := onlandEnv(t, d, "echo 빌드 깨짐; exit 3")
	if err := onlandPend("dev", "a", "-", "s1"); err != nil {
		t.Fatal(err)
	}
	if rc := onlandRun("dev"); rc != 0 {
		t.Fatalf("rc %d", rc)
	}
	reps, _ := readFile(rec)
	if !strings.HasPrefix(reps, "report|막힘|-|착지 뒤 dev 실패(종료 3)") || strings.Contains(reps, "--to") {
		t.Fatalf("실패 사건(주인 없음 → 모든 지휘): %s", reps)
	}
	logs, _ := filepath.Glob(onlandDir() + "/dev-*.log")
	if len(logs) != 1 {
		t.Fatalf("로그 %v", logs)
	}
	if b, _ := readFile(logs[0]); !strings.Contains(b, "빌드 깨짐") || !strings.Contains(reps, logs[0]) {
		t.Fatalf("로그 내용·경로: %q / %s", b, reps)
	}
}
