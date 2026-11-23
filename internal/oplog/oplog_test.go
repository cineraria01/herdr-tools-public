package oplog

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

// tmp: 시험 기록 경로(HERDR_TOOLS_OPLOG) — 실제 기록을 건드리지 않는다
func tmp(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "st", "ops.tsv")
	t.Setenv(Env, p)
	return p
}

func lines(t *testing.T, p string) []string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// stderr: f 동안 표준 오류를 모은다
func stderr(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	done := make(chan string)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	f()
	os.Stderr = old
	w.Close()
	return <-done
}

func TestLineFormat(t *testing.T) {
	p := tmp(t)
	Log("hc-lane", "land", Die, 1, "한\t줄\n끝")
	Log("hc-lane", "", Warn, -1, "경고")
	ls := lines(t, p)
	if len(ls) != 2 {
		t.Fatalf("두 줄: %q", ls)
	}
	c := strings.Split(ls[0], "\t")
	wd, _ := os.Getwd()
	if len(c) != 8 || !regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$`).MatchString(c[0]) || c[1] != "hc-lane" || c[2] != "land" ||
		c[3] != strconv.Itoa(os.Getpid()) || c[4] != "die" || c[5] != "1" || c[6] != tail64(wd) || c[7] != "한 줄 끝" {
		t.Fatalf("칸: %q", c)
	}
	if c := strings.Split(ls[1], "\t"); len(c) != 8 || c[4] != "warn" || c[5] != "-" || c[2] == "" {
		t.Fatalf("코드 없으면 -·하위 명령 칸은 비지 않음: %q", c)
	}
}

func TestTail64(t *testing.T) {
	if got := tail64("/a/" + strings.Repeat("가", 30) + "/lane"); len(got) > 64 || !strings.HasSuffix(got, "/lane") || !utf8.ValidString(got) {
		t.Fatalf("뒤 64바이트·온 글자: %q", got)
	}
	if tail64("/short") != "/short" {
		t.Fatal("짧으면 그대로")
	}
}

func TestSubFromArgs(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{{[]string{"x"}, "-"}, {[]string{"x", "land", "main"}, "land"}, {[]string{"x", "--check"}, "--check"},
		{[]string{"x", "--token=비밀"}, "-"}, {[]string{"x", strings.Repeat("a", 100)}, strings.Repeat("a", 32)}} {
		if got := sub(c.args); got != c.want {
			t.Errorf("sub(%q) = %q, 기대 %q", c.args, got, c.want)
		}
	}
}

func TestLineCut1000(t *testing.T) {
	p := tmp(t)
	Log(strings.Repeat("t", 200), "s", Warn, 3, strings.Repeat("가", 1000))
	l := lines(t, p)[0]
	if len(l) > MaxLine || !utf8.ValidString(l) || strings.Count(l, "\t") != 7 {
		t.Fatalf("1000바이트 안·온 글자·8칸: %d %v %d", len(l), utf8.ValidString(l), strings.Count(l, "\t")+1)
	}
}

func TestRotate4MB(t *testing.T) {
	p := tmp(t)
	Log("t", "s", Warn, -1, "처음")
	if err := os.Truncate(p, MaxSize+1); err != nil {
		t.Fatal(err)
	}
	Log("t", "s", Warn, -1, "돌린 뒤")
	if st, err := os.Stat(p + ".1"); err != nil || st.Size() != MaxSize+1 {
		t.Fatalf("넘은 파일은 .1로: %v", err)
	}
	if ls := lines(t, p); len(ls) != 1 || !strings.HasSuffix(ls[0], "\t돌린 뒤") {
		t.Fatalf("새 파일에 한 줄: %q", ls)
	}
	if err := os.Truncate(p, MaxSize+2); err != nil {
		t.Fatal(err)
	}
	Log("t", "s", Warn, -1, "또")
	if st, _ := os.Stat(p + ".1"); st.Size() != MaxSize+2 {
		t.Fatal("한 세대만 — .1은 바로 앞 것으로 바뀐다")
	}
	if _, err := os.Stat(p + ".2"); !os.IsNotExist(err) {
		t.Fatal(".2는 없다")
	}
	if err := os.Truncate(p, MaxSize); err != nil {
		t.Fatal(err)
	}
	Log("t", "s", Warn, -1, "딱 4MB")
	if st, _ := os.Stat(p); st.Size() <= MaxSize {
		t.Fatal("4MB 딱이면 안 돌린다")
	}
}

func TestWriteFailNoEffect(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(d+"/f", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(Env, d+"/f/ops.tsv") // 부모가 파일 — 못 쓴다
	failOnce = sync.Once{}
	ended.Store(false)
	got := stderr(t, func() {
		Exited("hc-fill", 3)
		Log("hc-fill", "add", Die, 1, "가")
		Log("hc-fill", "add", Warn, -1, "나")
	})
	ended.Store(false)
	if strings.Count(got, "\n") != 1 || !strings.HasPrefix(got, "hc-fill: 운영 기록을 남기지 못했다") {
		t.Fatalf("실패는 표준 오류에 한 번만: %q", got)
	}
}

func TestConcurrentLinesDontMix(t *testing.T) {
	p := tmp(t)
	var wg sync.WaitGroup
	for g := 0; g < 40; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				Log("t", "s", Warn, g, fmt.Sprintf("%d-%d ", g, i)+strings.Repeat("가", 300))
			}
		}(g)
	}
	wg.Wait()
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		n++
		if l := sc.Text(); strings.Count(l, "\t") != 7 || len(l) > MaxLine || !utf8.ValidString(l) {
			t.Fatalf("섞인 줄: %d칸 %d바이트", strings.Count(l, "\t")+1, len(l))
		}
	}
	if n != 1000 {
		t.Fatalf("1000줄: %d", n)
	}
}

func TestExitedOnlyNonZeroAndNotAfterDie(t *testing.T) {
	p := tmp(t)
	ended.Store(false)
	Exited("hc-lane", 0)
	Exited("hc-lane", 4)
	Fatal("hc-lane", "s", Die, 1, "멈춤")
	Exited("hc-lane", 1)
	ended.Store(false)
	ls := lines(t, p)
	if len(ls) != 2 || strings.Split(ls[0], "\t")[4] != "exit" || strings.Split(ls[0], "\t")[5] != "4" || strings.Split(ls[1], "\t")[4] != "die" {
		t.Fatalf("0은 안 남고, die 뒤 exit는 겹쳐 남지 않는다: %q", ls)
	}
}

// 감사 L2: 계속 도는 경고(proc의 「모름 — 띄우지 않음」 refuse)는 뒤따르는 exit 줄을 끄지 않는다 — 끄는 것은 도구가 끝내는 Fatal·Usage뿐
func TestWarnRefuseDoesNotHideExit(t *testing.T) {
	p := tmp(t)
	ended.Store(false)
	Log("hc-fill", "s", Refuse, -1, "모름 — 띄우지 않음")
	Exited("hc-fill", 3)
	Usage()
	Exited("hc-fill", 2)
	ended.Store(false)
	if ls := lines(t, p); len(ls) != 2 || strings.Split(ls[1], "\t")[4] != "exit" {
		t.Fatalf("refuse 뒤에도 exit 줄, 사용법 종료는 안 남는다: %q", ls)
	}
}

// 감사 M1: 시험 바이너리는 지정이 없으면 기록을 /dev/null로 돌린다 — 그 환경 값을 시험이 띄운 자식 바이너리도 물려받는다
func TestTestBinaryChildrenInherit(t *testing.T) {
	out, err := exec.Command("sh", "-c", `printf %s "$`+Env+`"`).Output()
	if err != nil || string(out) == "" {
		t.Fatalf("자식이 %s를 물려받아야(지정이 없었으면 %s): %q %v", Env, os.DevNull, out, err)
	}
	for name, want := range map[string]bool{"/tmp/go-build1/b001/oplog.test": true, "x.test.exe": true, "/bin/hc-lane": false, "test": false} {
		if testBinary(name) != want {
			t.Errorf("testBinary(%q) != %v", name, want)
		}
	}
}

// 경로 자리: HERDR_TOOLS_OPLOG가 비면 XDG_STATE_HOME, 그것도 없으면 ~/.local/state
func TestPathDefault(t *testing.T) {
	x := t.TempDir()
	t.Setenv("XDG_STATE_HOME", x)
	t.Setenv(Env, "")
	if p, err := Path(); err != nil || p != x+"/herdr-tools/ops.tsv" {
		t.Fatalf("XDG_STATE_HOME 아래: %q %v", p, err)
	}
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/h")
	if p, _ := Path(); p != "/h/.local/state/herdr-tools/ops.tsv" {
		t.Fatalf("없으면 ~/.local/state: %q", p)
	}
}
