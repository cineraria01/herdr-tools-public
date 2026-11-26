package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func setData(t *testing.T) {
	t.Helper()
	DATA = t.TempDir()
	Q, LED, AT = DATA+"/queue.tsv", DATA+"/inbox.tsv", DATA+"/audit.tsv"
	qHeld = false
}

// cksum 값이 POSIX cksum(1)과 같다(셸 판과 같은 결정적 ID — rl…·al…)
func TestCksumMatchesSystem(t *testing.T) {
	if _, err := exec.LookPath("cksum"); err != nil {
		t.Skip("cksum 없음")
	}
	for _, s := range []string{"", "a", "lane-x ~/inbox/done/x.md", strings.Repeat("가나다", 200)} {
		c := exec.Command("cksum")
		c.Stdin = strings.NewReader(s)
		out, err := c.Output()
		if err != nil {
			t.Fatal(err)
		}
		want := strings.Fields(string(out))[0]
		if got := strconv.FormatUint(uint64(cksum([]byte(s))), 10); got != want {
			t.Errorf("cksum(%q) = %s, 바람 %s", s, got, want)
		}
	}
}

// qset: 줄 ID가 다르면(rm·다시 add된 새 줄) 옛 실행으로 덮지 않고 1. 옛 줄(12칸)에 먼저 칸을 주면 13칸 normal·나머지 -로 채운다
func TestQsetRowID(t *testing.T) {
	setData(t)
	old := "a\tapp\tb\t-\t-\twait\t-\t-\t-\t2026-11-21T10:00\t-\t-"
	cur := pad16("n", "app", "b", "-", "-", "wait", "-", "-", "-", "t", "-", "-", "normal", "-", "-", "-").Set(qRID, "r1-1-1")
	if err := os.WriteFile(Q, []byte(old+"\n"+cur.Line()+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rc := qset(true, "r9-9-9", "n", "up", "m", "m/n", "-", "", ""); rc != 1 {
		t.Fatalf("다른 줄 ID: rc %d, 바람 1", rc)
	}
	if rc := qset(true, "r1-1-1", "n", "up", "m", "m/n", "-", "", ""); rc != 0 || qrow("n").F(qState) != "up" {
		t.Fatalf("같은 줄 ID: rc %d, 줄 %v", rc, qrow("n"))
	}
	if rc := qset(false, "", "a", "up", "m", "m/a", "-", "early", ""); rc != 0 {
		t.Fatal(rc)
	}
	if r := qrow("a"); r.F(qPrio) != "normal" || r.F(qEarly) != "early" || r.NF() != qEarly {
		t.Fatalf("옛 줄 채움: %v", r)
	}
}

// withQ 안의 crash·die는 그 잠금 일만 끝내고(셸 판 qlock 자식) 잠금을 놓는다 — 다음 잠금이 바로 잡힌다
func TestWithQCrashReleases(t *testing.T) {
	setData(t)
	os.Setenv("HC_FILL_CRASH_AT", "x-point")
	defer os.Unsetenv("HC_FILL_CRASH_AT")
	if rc := withQ(func() { crash("x-point") }); rc != 137 {
		t.Fatalf("crash: rc %d, 바람 137", rc)
	}
	if qHeld {
		t.Fatal("잠금 표시가 남음")
	}
	DRC = 1
	if rc := withQ(func() { die("시험 die") }); rc != 1 {
		t.Fatalf("die: rc %d", rc)
	}
	done := make(chan int, 1)
	go func() { done <- withQ(func() {}) }()
	select {
	case rc := <-done:
		if rc != 0 {
			t.Fatalf("다음 잠금 rc %d", rc)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("잠금이 풀리지 않았다")
	}
	if b, _ := os.ReadFile(DATA + "/crash.log"); string(b) != "x-point\n" {
		t.Fatalf("crash.log %q", b)
	}
}

// 원자 쓰기: 같은 폴더 임시 파일에 써서 바꿔 넣는다. 강제 종료로 남은 10분 넘은 임시 파일은 다음 바퀴(sweepTmp)가 거두고, 새것은 둔다
func TestAtomicAndSweep(t *testing.T) {
	setData(t)
	if err := writeAtomic(Q, "x\n"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(Q); string(b) != "x\n" {
		t.Fatalf("내용 %q", b)
	}
	old := []string{DATA + "/.queue.tsv.tmp123", DATA + "/held.tsv.tmp", DATA + "/x.md.tmp.42"}
	fresh := DATA + "/.own.tsv.tmp999"
	for _, p := range append(old, fresh) {
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-time.Hour)
	for _, p := range old {
		os.Chtimes(p, past, past)
	}
	sweepTmp()
	for _, p := range old {
		if exists(p) {
			t.Errorf("남음: %s", filepath.Base(p))
		}
	}
	if !exists(fresh) || !exists(Q) {
		t.Error("새 임시 파일·대기열을 지웠다")
	}
}

// git 모름: log가 실패하면(반쪽 출력 뒤 종료 128) 착지 판정을 「없음」이 아니라 모름으로
func TestLandedUnknownOnGitFailure(t *testing.T) {
	dir := t.TempDir()
	git := filepath.Join(dir, "git")
	if err := os.WriteFile(git, []byte("#!/bin/sh\necho 'abc1234 lane-x: 끝'\nexit 128\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	FR, FB = dir, "origin/main"
	if hit, ok := landed("lane-x"); ok || hit != "" {
		t.Fatalf("모름이어야: %q %v", hit, ok)
	}
	if err := os.WriteFile(git, []byte("#!/bin/sh\necho 'abc1234 feat(x): lane-x 끝'\necho 'def5678 lane-xy: 다른 것'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	tickReset() // 다음 바퀴 — 이력은 바퀴에 한 번 읽는다(fill-after-cost)
	if hit, ok := landed("lane-x"); !ok || hit != "abc1234 feat(x): lane-x 끝" {
		t.Fatalf("있음: %q %v", hit, ok)
	}
	if hit, ok := landed("lane"); !ok || hit != "" {
		t.Fatalf("단어 경계: %q %v", hit, ok)
	}
}

// 자리 세기(ht-seat-working-only): 대기열이 띄운 것만 — 상태 모름(뜨는 중)·working은 셈, 대기열 밖(사람·지휘 세션)·다른 종류·done·idle은 안 셈
func TestCountSeatsSimple(t *testing.T) {
	setData(t)
	HOME = t.TempDir()
	defer func(o bool) { dry = o }(dry) // 뒤 시험에 dry를 남기지 않는다(ops-knobs-reland: 뒤의 TestKnobRemoteTick이 push 대신 dry-run으로 돌았다)
	dry, AGENT, LOCAL = true, "claude-code", "here"
	LANEADDR = []string{"b/l1"}
	ROWS = []Row{pad16("l1", "p", "/b.md", "-", "-", "up", "b", "b/l1"), pad16("q", "p", "/b.md", "-", "-", "up", "b", "b/q"),
		pad16("d", "p", "/b.md", "-", "-", "up", "s", "s/d")}
	defer func() { ROWS = nil }()
	TERMS = []Term{
		{"b", "b/l1", "env", "l1", "-", "/"},
		{"b", "b/p", "env", "proxy", "-", "/"},
		{"b", "b/q", "claude-code", "q", "working", "/r.wt/q"},
		{"b", "b/w", "claude-code", "w", "working", "/"},
		{"s", "s/d", "claude-code", "d", "done", "/r.wt/d"},
		{"b", "b/r", "claude-code", "r", "idle", "/srv/app"},
	}
	countSeats()
	if got := strings.Join(SEATS, ","); got != "b,b" {
		t.Fatalf("SEATS %q, 바람 b,b(l1 뜨는 중·q 일함)", got)
	}
	if !HELDW.Load() || len(HNEW) != 2 {
		t.Fatalf("HELDW %v HNEW %v", HELDW.Load(), HNEW)
	}
}

// 대기열을 못 읽으면 고쳐 쓰는 쪽은 빈 목록으로 읽지 않고 끝난다(Fable L1)
func TestQRowsWUnreadable(t *testing.T) {
	setData(t)
	if err := os.Mkdir(Q, 0o755); err != nil { // 읽히지 않는 대기열(폴더)
		t.Fatal(err)
	}
	qHeld = true // 잠금 안: exit가 lockExit로 올라온다
	defer func() {
		qHeld = false
		if e, ok := recover().(lockExit); !ok || e.code != 1 {
			t.Fatalf("종료 1이어야: %v", e)
		}
	}()
	qRowsW()
	t.Fatal("못 읽었는데 돌아왔다")
}

// Windows·Linux 교차 빌드가 깨지지 않는다(gofix-fill F15 — syscall.Kill·Setpgid·CloseOnExec(int)로 Windows 빌드가 깨졌다)
func TestCrossBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	for _, osys := range []string{"windows", "linux"} {
		c := exec.Command("go", "build", "-o", filepath.Join(t.TempDir(), "hf"), ".")
		c.Env = append(os.Environ(), "GOOS="+osys, "GOARCH=amd64", "CGO_ENABLED=0")
		if out, err := c.CombinedOutput(); err != nil {
			t.Errorf("%s 빌드 실패: %v\n%s", osys, err, out)
		}
	}
}

// 릴리스 바이너리의 --version은 커밋을 보인다(gofix-fill F19)
func TestVersionShowsCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	bin := filepath.Join(t.TempDir(), "hf")
	if out, err := exec.Command("go", "build", "-ldflags", "-X herdr-tools/internal/version.Commit=abcdef012345", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("빌드: %v\n%s", err, out)
	}
	out, err := exec.Command(bin, "--version").Output()
	if err != nil || strings.TrimSpace(string(out)) != "hc-fill (Go abcdef012345)" {
		t.Fatalf("--version = %q (%v)", out, err)
	}
}

// newid 꼴: <접두><36진 초>-<6글자> — Go 판이 실제로 쓰는 ID(받은편지·감사)를 본다(gofix-fill F17 — 계약 시험은 셸 함수만 봤다)
func TestNewidShape(t *testing.T) {
	re := regexp.MustCompile(`^r[0-9a-z]+-[0-9a-z]{6}$`)
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		id := newid("r")
		if !re.MatchString(id) || !reqIDOK(id) {
			t.Fatalf("newid 꼴이 틀림: %q", id)
		}
		seen[id] = true
	}
	if len(seen) < 45 {
		t.Fatalf("newid가 겹친다: %d/50", len(seen))
	}
	// 접수 표시에서 읽은 요청 ID는 꼴이 맞을 때만 경로에 쓴다(Fable E1)
	for _, bad := range []string{"x/../../../../escaped", "r1-abc", "", "rl0123", "r1abc-abcdef/.."} {
		if reqIDOK(bad) {
			t.Errorf("reqIDOK(%q) = 참", bad)
		}
	}
	if !reqIDOK("rl0a1b2c3d") {
		t.Error("옛 줄 rl<8자리>를 거절")
	}
}
