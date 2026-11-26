package jobtmp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// runWrapped: Wrap한 argv를 hc 터미널 안처럼(HC_TERMINAL_ID) 돌린다. 명령은 자기 TMPDIR·MAC_CHROMIUM_TMPDIR을 out에 적는다
func runWrapped(t *testing.T, root, home string, run bool, term string) (out string, argv []string) {
	t.Helper()
	f := filepath.Join(t.TempDir(), "out")
	argv = Wrap(root, "lane-a", "/w", run, []string{"sh", "-c", `printf '%s\n%s\n' "${TMPDIR-}" "${MAC_CHROMIUM_TMPDIR-}" >"$0"`, f})
	c := exec.Command(argv[0], argv[1:]...)
	c.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "TMPDIR=/sys-tmp"}
	if term != "" {
		c.Env = append(c.Env, "HC_TERMINAL_ID="+term)
	}
	if b, err := c.CombinedOutput(); err != nil {
		t.Fatalf("감싼 명령 실패: %v %s", err, b)
	}
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b), argv
}

// ① exec 꼴: TMPDIR이 <뿌리>/<6글자>이고 표식(주소·이름·시각)이 있다. 폴더는 남는다(hc-sweep이 지운다)
func TestWrapExecGivesJobDir(t *testing.T) {
	home := shortHome(t)
	out, _ := runWrapped(t, Default, home, false, "t9zz")
	lines := strings.Split(out, "\n")
	d := lines[0]
	root := Expand(Default, home)
	if filepath.Dir(d) != root || len(filepath.Base(d)) != 6 {
		t.Fatalf("TMPDIR %q 가 %s/<6글자> 아님", d, root)
	}
	a, n, at, ok := Owner(d)
	if !ok || a != "t9zz" || n != "lane-a" || time.Since(time.Unix(at, 0)) > time.Minute {
		t.Fatalf("표식 %q %q %d %v", a, n, at, ok)
	}
	if mac := lines[1]; (mac == d) != (runtimeDarwin()) {
		t.Fatalf("MAC_CHROMIUM_TMPDIR %q (Darwin %v)", mac, runtimeDarwin())
	}
	if st, err := os.Stat(root); err != nil || st.Mode().Perm() != 0o700 {
		t.Fatalf("뿌리 권한: %v %v", st, err)
	}
}

// run 꼴: 끝나면 폴더를 지운다
func TestWrapRunRemoves(t *testing.T) {
	home := shortHome(t)
	out, _ := runWrapped(t, "~/r", home, true, "t9zz")
	d := strings.Split(out, "\n")[0]
	if filepath.Dir(d) != filepath.Join(home, "r") {
		t.Fatalf("TMPDIR %q", d)
	}
	if _, err := os.Lstat(d); !os.IsNotExist(err) {
		t.Fatalf("run 꼴이 끝났는데 %s 가 남음", d)
	}
}

// run 꼴: TERM(hc kill)을 받으면 폴더를 지우고 끝난다
func TestWrapRunTermRemoves(t *testing.T) {
	home := shortHome(t)
	ready := filepath.Join(home, "ready")
	argv := Wrap("~/r", "spec-x", "/w", true, []string{"sh", "-c", `printf %s "$TMPDIR" >"$0"; sleep 30`, ready})
	c := exec.Command(argv[0], argv[1:]...)
	c.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "HC_TERMINAL_ID=t1"}
	var errb strings.Builder
	c.Stderr = &errb
	defer func() {
		if t.Failed() {
			t.Log("stderr:", errb.String())
		}
	}()
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	var d string
	for i := 0; i < 100 && d == ""; i++ {
		b, _ := os.ReadFile(ready)
		d = string(b)
		time.Sleep(50 * time.Millisecond)
	}
	if d == "" {
		t.Fatal("감싼 명령이 시작하지 않음")
	}
	syscall.Kill(-c.Process.Pid, syscall.SIGTERM) // hc kill처럼 무리 전체에
	c.Wait()
	if _, err := os.Lstat(d); !os.IsNotExist(err) {
		t.Fatalf("TERM 뒤 %s 가 남음", d)
	}
}

// hc 터미널 밖(HC_TERMINAL_ID 없음)·⑧ 끄기(off)·Windows cwd: 지금 그대로(폴더 안 만듦)
func TestWrapOffAndOutside(t *testing.T) {
	home := shortHome(t)
	if out, _ := runWrapped(t, Default, home, false, ""); !strings.HasPrefix(out, "/sys-tmp\n") {
		t.Fatalf("hc 밖인데 TMPDIR이 바뀜: %q", out)
	}
	if out, argv := runWrapped(t, "off", home, false, "t9zz"); !strings.HasPrefix(out, "/sys-tmp\n") || argv[0] != "sh" || argv[1] != "-c" || argv[2] == script {
		t.Fatalf("off인데 감쌈: %q %q", out, argv[:2])
	}
	if _, err := os.Lstat(Expand(Default, home)); !os.IsNotExist(err) {
		t.Fatal("감싸지 않았는데 뿌리가 생김")
	}
	a := []string{"claude"}
	if got := Wrap(Default, "x", `C:\Users\x\repo`, false, a); len(got) != 1 {
		t.Fatalf("Windows cwd를 감쌈: %q", got)
	}
}

// ⑥ 길이: 뿌리 상한 42자면 작업 폴더 49자(지금 macOS 기본 TMPDIR) 이하, 가장 긴 소켓 꼴을 더해도 104바이트 안. 넘는 뿌리는 감싸지 않고 기본으로
func TestSocketLength(t *testing.T) {
	if !strings.Contains(script, fmt.Sprintf("-gt %d ]", MaxRoot)) {
		t.Fatalf("감싸개의 뿌리 상한이 MaxRoot(%d)와 다름", MaxRoot)
	}
	sysTmp := "/var/folders/xx/" + strings.Repeat("x", 30) + "/T/" // macOS 기본 TMPDIR 꼴(/var/folders/<2>/<30>/T/, 49자 — 워커 실측)
	if MaxDir > len(sysTmp) {
		t.Fatalf("작업 폴더 상한 %d > 기본 TMPDIR %d", MaxDir, len(sysTmp))
	}
	if n := MaxDir + 1 + len(Longest) + 1; n > SockMax {
		t.Fatalf("소켓 경로 %d > %d", n, SockMax)
	}
	for _, h := range []string{"/Users/alice", "/Users/a-twenty-char-name-x", "/home/w"} {
		if r := Expand(Default, h); len(r) > MaxRoot {
			t.Fatalf("기본 뿌리 %s (%d자) > %d — 집 이름이 이만하면 감싸지 않는다", r, len(r), MaxRoot)
		}
	}
	home := shortHome(t)
	long := filepath.Join(home, strings.Repeat("x", MaxRoot))
	if out, _ := runWrapped(t, long, home, false, "t9zz"); !strings.HasPrefix(out, "/sys-tmp\n") {
		t.Fatalf("42자 넘는 뿌리를 썼다: %q", out)
	}
}

// 한 줄: 기록·시험 대역이 hc run 인자를 한 줄로 적는다. dash(리눅스 sh)·bash 모두 문법이 맞다
func TestScriptOneLine(t *testing.T) {
	if strings.ContainsAny(script, "\n\t\\") || !strings.HasPrefix(script, "r=$1 n=$2 m=$3; shift 3; case") {
		t.Fatalf("감싸개가 한 줄이 아님: %q", script[:40])
	}
	for _, sh := range []string{"sh", "dash", "bash"} {
		if _, err := exec.LookPath(sh); err != nil {
			continue
		}
		if b, err := exec.Command(sh, "-n", "-c", script).CombinedOutput(); err != nil {
			t.Fatalf("%s 문법 오류: %v %s", sh, err, b)
		}
	}
}

func TestOwnerMalformed(t *testing.T) {
	d := t.TempDir()
	for _, s := range []string{"", "t1\nx\n", "\nx\n1\n", "t1\nx\n12a\n", "t1\nx\n\n", "t1\tx\t1\n"} {
		os.WriteFile(filepath.Join(d, Marker), []byte(s), 0o644)
		if _, _, _, ok := Owner(d); ok {
			t.Fatalf("잘못된 표식 %q 를 받음", s)
		}
	}
}

// shortHome: 짧은 집(/tmp/jt…) — t.TempDir()는 길어 뿌리가 42자 상한에 걸린다
func shortHome(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("/tmp", "jt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	return d
}

func runtimeDarwin() bool {
	o, _ := exec.Command("uname").Output()
	return strings.TrimSpace(string(o)) == "Darwin"
}
