//go:build !windows

package selfexec

// 시험(reexec-longrun): ① 실행 파일이 바뀌면 다음 바퀴에 새 판으로(가짜 바이너리 둘 — 실제 exec) ② 안 바뀌면 그대로
// ③ 이 실행이 띄워 기다리는 자식이 있는 동안은 하지 않는다 ④ exec 실패 → 한 줄 남기고 계속 ⑤ 인자·환경·작업 폴더 유지

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/proc"
)

// TestMain: SELFEXEC_TEST_WATCH=<경로>면 이 시험 바이너리가 오래 도는 고리 흉내 — 표준 입력 한 줄마다 한 바퀴(Check) 뒤 "still"
func TestMain(m *testing.M) {
	if p := os.Getenv("SELFEXEC_TEST_WATCH"); p != "" {
		s := at("흉내", p, nil)
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			s.Check()
			os.Stdout.WriteString("still\n")
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func put(t *testing.T, p, s string) {
	t.Helper()
	// 새 판은 rename으로 바꿔 넣는다(herdr-sync와 같게 — 도는 판의 파일은 그대로)
	tmp := p + ".new"
	if err := os.WriteFile(tmp, []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, p); err != nil {
		t.Fatal(err)
	}
}

// ①②⑤: 실제 exec — 흉내 고리가 지켜보는 가짜 바이너리를 v1 → v2로 바꾸면 두 바퀴 뒤 v2가 같은 인자·환경·작업 폴더로 뜬다
func TestReexecReal(t *testing.T) {
	d := t.TempDir()
	bin := filepath.Join(d, "tool")
	put(t, bin, "#!/bin/sh\necho v1\n")
	wd := t.TempDir()
	c := exec.Command(os.Args[0], "-test.run=^$", "둘째 인자")
	c.Dir = wd
	c.Env = append(os.Environ(), "SELFEXEC_TEST_WATCH="+bin, "SELFEXEC_MARK=표식 값")
	in, err := c.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	c.Stderr = &stderr
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	rd := bufio.NewReader(out)
	tick := func() string {
		t.Helper()
		if _, err := in.Write([]byte("\n")); err != nil {
			t.Fatalf("바퀴를 보내지 못했다: %v (stderr %s)", err, stderr.String())
		}
		l, err := rd.ReadString('\n')
		if err != nil {
			t.Fatalf("바퀴 결과를 읽지 못했다: %v (stderr %s)", err, stderr.String())
		}
		return strings.TrimSpace(l)
	}
	if got := tick(); got != "still" { // ② 안 바뀜
		t.Fatalf("안 바뀐 판에서 %q", got)
	}
	put(t, bin, "#!/bin/sh\n[ \"$1\" != --version ] || { echo 'tool (Go v2)'; exit 0; }\nprintf 'v2|%s|%s|%s\\n' \"$*\" \"$SELFEXEC_MARK\" \"$(pwd -P)\"\nexit 0\n")
	if got := tick(); got != "still" { // 처음 본 바뀜 — 다 쓰였는지 한 바퀴 더 본다
		t.Fatalf("처음 본 바뀜에 바로 exec했다: %q", got)
	}
	in.Write([]byte("\n"))
	l, _ := rd.ReadString('\n')
	in.Close()
	c.Wait()
	wdp, _ := filepath.EvalSymlinks(wd)
	want := "v2|-test.run=^$ 둘째 인자|표식 값|" + wdp
	if strings.TrimSpace(l) != want {
		t.Fatalf("새 판 출력 %q, 바람 %q (stderr %s)", strings.TrimSpace(l), want, stderr.String())
	}
	if !strings.Contains(stderr.String(), "흉내: 넘겨줌 "+bin+" 새 판이 깔렸다(옛 판 ") || !strings.Contains(stderr.String(), " → 새 판 ") {
		t.Fatalf("옛 판 → 새 판 줄이 없다: %s", stderr.String())
	}
}

// fake: exec를 세는 가짜(err를 돌려준다)
func fake(t *testing.T, err error) (*Self, string, *int, *strings.Builder) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "tool")
	put(t, p, "v1")
	var log strings.Builder
	s := at("시험", p, func(l string) { log.WriteString(l + "\n") })
	n := 0
	s.exec = func(bin string, argv, env []string) error { n++; return err }
	s.probe = func(string) (string, error) { return "tool (Go 새 판)", nil } // 새 판이 뜬다(내용이 가짜라 실제로는 못 띄움)
	return s, p, &n, &log
}

// ② 안 바뀜 → 몇 바퀴를 돌아도 exec하지 않는다. 쓰는 중(바퀴마다 또 바뀜)이어도
func TestUnchangedAndSettling(t *testing.T) {
	s, p, n, _ := fake(t, os.ErrInvalid)
	for range 3 {
		s.Check()
	}
	put(t, p, "v2-반쯤")
	s.Check()
	put(t, p, "v2-다 쓴 판")
	s.Check()
	if *n != 0 {
		t.Fatalf("안 바뀌었거나 쓰는 중인 판으로 exec했다(%d번)", *n)
	}
	s.Check()
	if *n != 1 {
		t.Fatalf("다 쓴 판으로 exec하지 않았다(%d번)", *n)
	}
}

// ③ 이 실행이 띄워 기다리는 자식이 있으면 하지 않는다(exec하면 그 자식이 고아로 남는다) — 자식이 끝난 뒤 바퀴에 한다
func TestBusySkips(t *testing.T) {
	s, p, n, _ := fake(t, os.ErrInvalid)
	put(t, p, "v2")
	s.Check() // 처음 본 바뀜
	c := exec.Command("sleep", "30")
	proc.Group(c)
	release, err := proc.Start(c)
	if err != nil {
		t.Fatal(err)
	}
	s.Check()
	s.Check()
	busyN := *n
	c.Process.Kill()
	c.Wait()
	release()
	if busyN != 0 {
		t.Fatalf("자식이 도는 동안 exec했다(%d번)", busyN)
	}
	s.Check()
	if *n != 1 {
		t.Fatalf("자식이 끝난 뒤 exec하지 않았다(%d번)", *n)
	}
}

// ④ exec 실패 → 한 줄 남기고 이 판으로 계속, 같은 판으로 다시 시도하지 않는다. 그 뒤 또 새 판이면 다시 한다
func TestExecFailKeepsRunning(t *testing.T) {
	s, p, n, log := fake(t, errors.New("가짜 실패"))
	put(t, p, "v2")
	s.Check()
	s.Check()
	if *n != 1 || !strings.Contains(log.String(), "새 판으로 exec 실패(가짜 실패) — 이 판으로 계속") {
		t.Fatalf("exec %d번, 기록 %q", *n, log.String())
	}
	for range 3 {
		s.Check()
	}
	if *n != 1 {
		t.Fatalf("실패한 판으로 다시 시도했다(%d번)", *n)
	}
	time.Sleep(10 * time.Millisecond) // 다른 수정 시각
	put(t, p, "v3-더 긴 판")
	s.Check()
	s.Check()
	if *n != 2 {
		t.Fatalf("그 뒤 새 판으로 다시 하지 않았다(%d번)", *n)
	}
}

// 실행 파일을 못 읽음(까는 중·지움) → 아무것도 하지 않고, 다시 나타난 같은 판은 바뀜이 아니다
func TestMissingIsNoop(t *testing.T) {
	s, p, n, _ := fake(t, os.ErrInvalid)
	os.Remove(p)
	s.Check()
	s.Check()
	if *n != 0 {
		t.Fatalf("없는 실행 파일로 exec했다(%d번)", *n)
	}
	var z *Self
	z.Check() // 경로를 못 잡은 Self(nil)도 그냥 지나간다
}
