// Package hcfill_test: hc-fill 계약 시험(옛 tests/hc-fill.test.sh를 그대로 옮김, 2026-11-21 go-tests-fill).
// 임시 HOME·가짜 hc·hc-lane·git·herdr-balance(testdata/, PATH 앞)와 저장소 예시 설정(config.example/hc-fill.tsv)을 두고
// hc-fill Go 바이너리(clitest.Bin)를 밖에서 exec해 출력·종료 코드·상태 파일을 본다. 실제 레인·기계·네트워크는 쓰지 않는다.
// 단언 이름·차례는 셸 시험의 것 그대로다(절마다 파일 하나 — reg로 차례를 적는다). 판은 Go 하나다(2026-11-22 go-shell-remove).
//
// 설정은 HC_FILL_EXAMPLE·HC_FILL_AUDIT_EXAMPLE(없으면 저장소 config.example).
// 절마다 하위 시험이고 저마다 새 상태에서 돈다 — 일부 절만: go test -run '^TestHcFill$/^(a|k2)$' ./tests/hcfill/
// 실행: go test ./tests/hcfill/
package hcfill_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"herdr-tools/internal/clitest"
)

var repo string // 저장소 루트

func TestMain(m *testing.M) {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	repo = filepath.Clean(filepath.Join(wd, "..", ".."))
	clitest.Main(m) // 끝나면 Bin 폴더를 지운다(재감사 base R-225)
}

type section struct {
	order int
	name  string
	f     func(k *K)
}

var sections []section

// reg: 절을 차례 order로 등록한다(파일마다 init에서).
func reg(order int, name string, f func(k *K)) { sections = append(sections, section{order, name, f}) }

// TestHcFill: 절마다 하위 시험 하나 — 절은 저마다 새 K(임시 폴더·가짜 HOME·설정)에서 돌아 혼자·어떤 차례로도 맞는다
// (2026-11-24 contract-subtests: 옛 판은 한 K를 차례로 이어 써 -run으로 고를 수 없었다). 고르기는 -run '^TestHcFill$/^(a|b)$'.
func TestHcFill(t *testing.T) {
	sort.SliceStable(sections, func(i, j int) bool { return sections[i].order < sections[j].order })
	for _, s := range sections {
		t.Run(s.name, func(t *testing.T) {
			k := newK(t)
			s.f(k)
			t.Logf("# %d/%d 통과", k.n-k.fails, k.n)
		})
	}
}

// K: 시험 하나의 상태(셸 시험의 전역 변수들).
type K struct {
	t             *testing.T
	T, HOME       string // 임시 폴더 · 가짜 HOME
	FILL, EX, EXA string // 시험할 hc-fill · 예시 설정 · 감사 예시 틀
	HL, REALGIT   string // 진짜 hc-lane(Go 바이너리) · 진짜 git
	DATA, Q, GCL  string // $HOME/.local/share/hc-fill · 대기열 · gc.log
	HELD, LT, W   string // held.tsv · hc-lane 레인 기록 · 작업 트리 시험 폴더
	env           map[string]string
	out           string
	rc            int
	n, fails      int
	bg            *exec.Cmd         // 배경 watch(11절)
	ek            int               // ev가 매기는 사건 시각(분)
	V             map[string]string // 절을 넘어 이어 쓰는 값(셸 시험의 전역 변수 RA·rf·later 등)
}

func newK(t *testing.T) *K {
	T, err := os.MkdirTemp("", "hc-fill-test.")
	if err != nil {
		t.Fatal(err)
	}
	if T, err = filepath.EvalSymlinks(T); err != nil {
		t.Fatal(err)
	}
	k := &K{t: t, T: T, HOME: T + "/home", env: clitest.Env(T), V: map[string]string{}}
	bd := clitest.BinDir(t, "hc-fill", "hc-lane", "hc-merger", "herdr-tools-config")
	k.FILL, k.HL = bd+"/hc-fill", bd+"/hc-lane"
	k.EX = envOr("HC_FILL_EXAMPLE", repo+"/config.example/hc-fill.tsv")
	k.EXA = envOr("HC_FILL_AUDIT_EXAMPLE", repo+"/config.example/hc-fill-audit.md")
	if k.REALGIT, err = exec.LookPath("git"); err != nil {
		t.Fatal(err)
	}
	k.DATA = k.HOME + "/.local/share/hc-fill"
	k.Q, k.GCL, k.HELD = k.DATA+"/queue.tsv", k.DATA+"/gc.log", k.DATA+"/held.tsv"
	k.LT = k.HOME + "/.local/state/hc-lane/lanes.tsv"
	k.W = T + "/w"
	t.Cleanup(k.cleanup)
	k.set("GIT_CEILING_DIRECTORIES="+T+":"+filepath.Dir(T), "HC_REAL_GIT="+k.REALGIT,
		"PATH="+T+"/bin:"+os.Getenv("PATH"), "HOME="+k.HOME, "T_LOG="+T, "HERDR_TOOLS_CONFIG="+T+"/conf",
		"HC_FILL_WAIT=0", "HC_FILL_POLL=0", "HC_FAKE_COORD=t8kk",
		"HERDR_PROC_DIR="+T+"/proc", // 고아 무리 기록(internal/proc)도 시험 폴더에 — 사용자 캐시를 쓰거나 남의 기록을 거두지 않게
		// macOS awk의 ==는 strcoll — en_US.UTF-8에서 "판정"=="완료"가 참이었다(2026-11-19 fill-then). 그 로캘로 돌려 늘 잡히게
		"LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
	mkdirs(t, T+"/bin", T+"/conf", k.HOME)
	for _, f := range []string{"hc", "hc-lane", "git", "herdr-balance", "sysctl", "hc-sweep"} {
		b, err := os.ReadFile(filepath.Join(repo, "tests", "hcfill", "testdata", f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(T+"/bin/"+f, b, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if !exists(k.EX) {
		t.Fatalf("FAIL - 예시 설정이 없다: %s", k.EX)
	}
	k.cpConf()
	write(t, T+"/commits", "")
	return k
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// cleanup: 남은 배경 watch·정리 프로세스를 끄고 임시 폴더를 지운다(셸 시험의 EXIT trap).
func (k *K) cleanup() {
	if k.bg != nil && k.bg.Process != nil {
		killtree(k.bg.Process.Pid)
		k.bg.Wait()
	}
	for _, p := range k.leftw() {
		killtree(p)
	}
	for _, p := range pgrepF(k.DATA + "/gc.lock") {
		killtree(p)
	}
	os.RemoveAll(k.T)
}

// --- 환경 ---

func (k *K) set(kv ...string) {
	for _, s := range kv {
		key, v, _ := strings.Cut(s, "=")
		k.env[key] = v
	}
}

func (k *K) unset(keys ...string) {
	for _, key := range keys {
		delete(k.env, key)
	}
}

func (k *K) environ(extra []string) []string {
	m := map[string]string{}
	for key, v := range k.env {
		m[key] = v
	}
	for _, s := range extra {
		key, v, _ := strings.Cut(s, "=")
		m[key] = v
	}
	l := make([]string, 0, len(m))
	for key, v := range m {
		l = append(l, key+"="+v)
	}
	return l
}

// --- 단언 ---

// check: 셸 시험의 t "<이름>" '<조건>'. 실패면 출력(k.out)과 덧붙인 진단을 찍는다.
func (k *K) check(name string, ok bool, diag ...string) {
	k.t.Helper()
	k.n++
	if ok {
		k.t.Logf("ok %d - %s", k.n, name)
		return
	}
	k.fails++
	var b strings.Builder
	for _, l := range strings.Split(k.out, "\n") {
		b.WriteString("    | " + l + "\n")
	}
	for _, d := range diag {
		b.WriteString("    + " + d + "\n")
	}
	k.t.Errorf("FAIL %d - %s\n%s", k.n, name, b.String())
}

// --- 실행 ---

// exe: dir에서 extra를 더한 환경으로 name을 돌려 합친 출력과 종료 코드. 띄우지 못하면 rc -1과 그 오류.
func (k *K) exe(dir string, extra []string, name string, args ...string) (string, int) {
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env = dir, k.environ(extra)
	cmd.WaitDelay = 30 * time.Second // 출력을 물고 남은 손자가 있어도 끝나게
	b, err := cmd.CombinedOutput()
	return strings.TrimRight(string(b), "\n"), exitCode(err)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// run: 셸 시험의 run — k.out·k.rc를 채우고 배경 정리가 끝날 때까지 기다린다.
func (k *K) run(args ...string) { k.runE(nil, args...) }

// runE: VAR=값 을 그 실행에만 더해 run(셸의 `VAR=값 run …`).
func (k *K) runE(extra []string, args ...string) {
	k.out, k.rc = k.exe(k.T, extra, k.FILL, args...)
	k.gcwait()
}

// fill: 출력을 버리고 hc-fill을 부른다(셸의 `"$FILL" … >/dev/null 2>&1`). k.out은 그대로.
func (k *K) fill(extra []string, args ...string) int {
	_, rc := k.exe(k.T, extra, k.FILL, args...)
	return rc
}

var bgPid = regexp.MustCompile(`배경으로 띄움\(pid ([0-9]+)\)`)

// gcwait: 바퀴가 배경으로 띄운 정리(「배경으로 띄움(pid N)」)가 끝날 때까지 — 다음 시험의 gc.log·DATA에 늦게 쓰지 않게.
func (k *K) gcwait() {
	m := bgPid.FindAllStringSubmatch(k.out, -1)
	if m == nil {
		return
	}
	pid, _ := strconv.Atoi(m[len(m)-1][1])
	for syscall.Kill(pid, 0) == nil {
		time.Sleep(50 * time.Millisecond)
	}
}

// bash: 셸 글을 bash -c로(T에서, k.env + extra). 지시문이 내는 셸 조각을 그대로 돌려 볼 때.
func (k *K) bash(script string, extra ...string) (string, int) {
	return k.exe(k.T, extra, "bash", "-c", script)
}

// --- 가짜 상태 ---

// fresh: 셸 시험의 fresh.
func (k *K) fresh() {
	rm(k.DATA)
	for _, f := range []string{"report.log", "lane.log", "landprio.log", "events", "hc.log", "balance.log", "git.log", "gc.log",
		"coord.log", "owner.log", "coordset.log", "coordcheck.log"} {
		rm(k.T + "/" + f)
	}
	write(k.t, k.T+"/commits", "")
	k.mj("box-1", "box-2", "shared", "slow")
	k.lj()
	k.unset("HC_FAKE_LS_LANE", "HC_FAKE_NOSTATUS", "HC_FAKE_LANE_FAIL", "HC_FAKE_FETCH_FAIL", "HC_FAKE_GONE", "HC_FAKE_EXEC_FAIL",
		"HC_FAKE_LANE_START", "HC_FAKE_FINISHED", "HC_FAKE_COORD_GONE", "HC_FAKE_REPORT_RC", "HC_FAKE_SEND_FAIL")
}

// mj: 기계 상태 — 적은 기계만 connected(box-1 box-2 shared slow 가운데).
func (k *K) mj(connected ...string) {
	var l []string
	for _, m := range []string{"box-1", "box-2", "shared", "slow"} {
		st := "disconnected"
		if has(connected, m) {
			st = "connected"
		}
		l = append(l, fmt.Sprintf(`{"name":"%s","state":"%s"}`, m, st))
	}
	k.set(`HC_FAKE_MACHINES={"machines":[` + strings.Join(l, ",") + `]}`)
}

// lj: 터미널 — "<주소>:<agent>:<이름>[:<상태, 기본 working>[:<cwd, 기본 />]]". 지휘 t0이 늘 맨 앞.
func (k *K) lj(specs ...string) {
	l := []string{`{"address":"t0","agent":"claude-code","name":"coord","state":"live","status":"working","cwd":"/"}`}
	for _, x := range specs {
		f := strings.SplitN(x, ":", 5)
		for len(f) < 5 {
			f = append(f, "")
		}
		st, cw := f[3], f[4]
		if st == "" {
			st = "working"
		}
		if cw == "" {
			cw = "/"
		}
		l = append(l, fmt.Sprintf(`{"address":"%s","agent":"%s","name":"%s","state":"live","status":"%s","cwd":"%s"}`, f[0], f[1], f[2], st, cw))
	}
	k.set(`HC_FAKE_LS={"terminals":[` + strings.Join(l, ",") + `]}`)
}

// ljq: lj와 같고, 그 터미널을 대기열이 띄운 것으로도 적는다(이름의 줄을 넣고 up·기계·주소로) — 자리는 대기열이 띄운 것만
// 센다(ht-seat-working-only, docs/hc-fill/slots.md). 기계를 「일하는 에이전트」로 채우는 차림에 쓴다
func (k *K) ljq(specs ...string) {
	k.lj(specs...)
	for _, x := range specs {
		f := strings.SplitN(x, ":", 4)
		m, _, _ := strings.Cut(f[0], "/")
		if k.where(f[2]) == "" {
			k.add(f[2])
		}
		k.hSetQ(f[2], map[int]string{6: "up", 7: m, 8: f[0]})
	}
}

// brief: $T/<이름>.md 지시문을 쓰고 그 경로.
func (k *K) brief(name string) string {
	p := k.T + "/" + name + ".md"
	write(k.t, p, "# 지시문 "+name+"\n본문\n")
	return p
}

// add: 셸 시험의 add <이름> [인자…] — 프로젝트 app, 출력 버림.
func (k *K) add(name string, extra ...string) int {
	return k.fill(nil, append([]string{"add", "--name", name, "--project", "app", "--brief", k.brief(name)}, extra...)...)
}

// where: 대기열에서 그 이름 줄의 「상태 기계 주소」(6·7·8칸, 줄마다 한 줄).
func (k *K) where(name string) string {
	var l []string
	for _, r := range rows(k.Q) {
		if col(r, 1) == name {
			l = append(l, col(r, 6)+" "+col(r, 7)+" "+col(r, 8))
		}
	}
	return strings.Join(l, "\n")
}

// state: where의 첫 낱말(셸의 where … | cut -d" " -f1).
func (k *K) state(name string) string { s, _, _ := strings.Cut(k.where(name), " "); return s }

// up: 셸의 up — 그 레인이 떠 있음.
func (k *K) up(name string) bool { return k.state(name) == "up" }

// qcol: 대기열에서 그 이름 첫 줄의 c칸(1부터, 없으면 "").
func (k *K) qcol(name string, c int) string {
	for _, r := range rows(k.Q) {
		if col(r, 1) == name {
			return col(r, c)
		}
	}
	return ""
}

// qnf: 그 이름 첫 줄의 칸 수(awk NF).
func (k *K) qnf(name string) int {
	for _, r := range rows(k.Q) {
		if col(r, 1) == name {
			return len(r)
		}
	}
	return 0
}

// launched: hc-lane 띄우기 인자 줄 가운데 그 줄과 같은 것의 수(grep -Fxc).
func (k *K) launched(line string) int { return countFx(k.T+"/lane.log", line) }

// --- 설정 ---

func (k *K) conf() string { return k.T + "/conf/hc-fill.tsv" }

// cpConf: 예시 설정으로 되돌린다(cp "$EX" …).
func (k *K) cpConf() { copyFile(k.t, k.EX, k.conf()) }

// confAppend: 설정 끝에 줄을 더한다(echo … >> …).
func (k *K) confAppend(lines ...string) { appendFile(k.t, k.conf(), strings.Join(lines, "\n")+"\n") }

// confSed: 설정의 줄마다 정규식 re를 repl로(sed s/re/repl/ — Go regexp, repl의 $1 꼴).
func (k *K) confSed(re, repl string) { sedFile(k.t, k.conf(), re, repl) }

// confDel: 정규식에 맞는 줄을 지운다(sed /re/d).
func (k *K) confDel(re string) { delLines(k.t, k.conf(), re) }

// --- 프로세스(11절 watch) ---

func tree(pid int) []int {
	var l []int
	for _, c := range pgrep("-P", strconv.Itoa(pid)) {
		l = append(l, tree(c)...)
	}
	return append(l, pid)
}

// killtree: 먼저 STOP으로 멈춰 새 자식을 못 띄우게 한 뒤 다시 세어 끈다(세고 끄는 사이에 뜬 sleep이 고아로 남지 않게).
func killtree(pid int) {
	for _, p := range tree(pid) {
		syscall.Kill(p, syscall.SIGSTOP)
	}
	l := tree(pid)
	for _, p := range l {
		syscall.Kill(p, syscall.SIGTERM)
	}
	for _, p := range l {
		syscall.Kill(p, syscall.SIGCONT)
	}
}

// pgrepOK: ok=false면 프로세스 목록을 못 읽음(종료 2 이상 — 「없음」(종료 1)과 다르다, 아스트라 E06)
func pgrepOK(args ...string) ([]int, bool) {
	b, err := exec.Command("pgrep", args...).Output()
	if c := exitCode(err); c != 0 && c != 1 {
		return nil, false
	}
	var l []int
	for _, f := range strings.Fields(string(b)) {
		if p, err := strconv.Atoi(f); err == nil {
			l = append(l, p)
		}
	}
	return l, true
}

// pgrep: 정리용(못 읽으면 빈 목록 — 정리할 것을 못 찾을 뿐). 단언에는 pgrepOK·leftwOK
func pgrep(args ...string) []int { l, _ := pgrepOK(args...); return l }

func pgrepF(pat string) []int { return pgrep("-f", pat) }

// leftw: 정리용 leftwOK(못 읽으면 빈 목록)
func (k *K) leftw() []int { l, _ := k.leftwOK(); return l }

// leftwOK: 이 시험의 hc-fill watch — 명령 줄에 $T가 든 것(셸 판), 또는 이 시험의 watch.lock을 연 것(Go 판).
// ok=false면 프로세스 목록이나 살아 있는 후보의 연 파일(lsof)을 못 읽었다 — 「남은 것 0」으로 세지 않는다(R-30). 그사이 끝난 후보는 뺀다
func (k *K) leftwOK() ([]int, bool) {
	seen := map[int]bool{}
	mine, ok1 := pgrepOK("-f", k.T+"/hc-fill watch")
	for _, p := range mine {
		seen[p] = true
	}
	all, ok2 := pgrepOK("-f", "/hc-fill watch")
	for _, p := range all {
		b, err := exec.Command("lsof", "-p", strconv.Itoa(p)).Output()
		if err != nil {
			if !errors.Is(syscall.Kill(p, 0), syscall.ESRCH) {
				ok2 = false // 살아 있는데 소유를 못 읽음 — 모름
			}
			continue
		}
		if strings.Contains(string(b), k.DATA+"/watch.lock") {
			seen[p] = true
		}
	}
	var l []int
	for p := range seen {
		l = append(l, p)
	}
	sort.Ints(l)
	return l, ok1 && ok2
}

// --- 파일·글 도우미 ---

func mkdirs(t *testing.T, ds ...string) {
	t.Helper()
	for _, d := range ds {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// write: 부모 폴더를 만들고 쓴다.
func write(t *testing.T, p, s string) {
	t.Helper()
	mkdirs(t, filepath.Dir(p))
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeX(t *testing.T, p, s string) {
	t.Helper()
	write(t, p, s)
	if err := os.Chmod(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func appendFile(t *testing.T, p, s string) {
	t.Helper()
	mkdirs(t, filepath.Dir(p))
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	write(t, to, string(b))
}

func sedFile(t *testing.T, p, re, repl string) {
	t.Helper()
	r := regexp.MustCompile(re)
	l := strings.Split(read(p), "\n")
	for i := range l {
		l[i] = r.ReplaceAllString(l[i], repl)
	}
	write(t, p, strings.Join(l, "\n"))
}

func delLines(t *testing.T, p, re string) {
	t.Helper()
	r := regexp.MustCompile(re)
	var keep []string
	for _, l := range strings.Split(read(p), "\n") {
		if !r.MatchString(l) {
			keep = append(keep, l)
		}
	}
	write(t, p, strings.Join(keep, "\n"))
}

func rm(ps ...string) {
	for _, p := range ps {
		os.RemoveAll(p)
	}
}

func read(p string) string { b, _ := os.ReadFile(p); return string(b) }

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func isDir(p string) bool { fi, err := os.Stat(p); return err == nil && fi.IsDir() }

// nonEmpty: [ -s 파일 ].
func nonEmpty(p string) bool { fi, err := os.Stat(p); return err == nil && fi.Size() > 0 }
