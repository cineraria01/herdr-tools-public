// hc-merger: hc-lane 병합기 본체(Go 판, 2026-11-21 사용자 결정 「Go로, 마무리 뒤」). `hc-lane merger …`가 부른다(lib/hc-lane/merger-go.sh).
// 계약은 bash 판(lib/hc-lane/merger.sh·merger-result.sh)과 같다 — 같은 원격 참조·일지 파일·사건·git 인자 꼴. 바뀐 것: 잠금이 자식에
// 물리지 않음(O_CLOEXEC), 착지·준비 훅을 새 프로세스 무리로 띄우고 멈출 때 무리째 끔, git 결과를 있음·없음·모름으로 가름,
// 큐를 우선(긴급>높음>보통>낮음) → 제출 시각 순으로 고름, 쉼 값·묶음 크기를 정수로 읽음(아니면 기본값과 경고).
//
// 짜임(bash 판의 서브셸과 같은 꼴): 상주 부모가 병합기 잠금을 쥐고 바퀴마다 일꾼(같은 바이너리 --round)을 띄운다. 일꾼은 후보 잠금을
// 쥐고 한 바퀴를 마친다. 부모가 죽어도(SIGKILL) 일꾼은 그 바퀴를 마치고, 착지를 새로 시작하지 않으며, 착지 뒤 판정은 다음 실행이
// 일지로 한다 — 그동안 다음 병합기는 후보 잠금에 막혀 75로 쉰다(후보 작업 트리를 지우지 않음).
package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"herdr-tools/internal/lockx"
	"herdr-tools/internal/proc"
)

const (
	landGrace  = 5 * time.Second // 착지 무리를 TERM한 뒤 KILL까지
	roundGrace = 8 * time.Second // 부모가 일꾼 무리를 TERM한 뒤 KILL까지(일꾼이 착지 무리를 끌 틈)
)

var (
	gTop   string     // die 사건에 적을 클론
	gRun   bool       // 여기부터 die는 막힘 사건도 남긴다
	gSelf  string     // hc-lane 경로(착지·사건을 부른다)
	gEvdir string     // 상태 폴더 …/hc-lane
	gLocks []*os.File // 쥔 잠금 파일 — 프로세스가 끝날 때까지 닫히지 않게(GC가 닫으면 잠금이 풀린다)
)

func main() {
	// 물려받은 fd(바깥 병합기의 잠금 fd 19 등)를 자식에게 넘기지 않는다(요구 2). Go가 여는 fd는 기본 O_CLOEXEC
	for fd := 3; fd < 1024; fd++ {
		syscall.CloseOnExec(fd)
	}
	gSelf, gEvdir = selfPath(), evdir()
	proc.Who = "hc-lane merger"
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("hc-merger 1")
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--round" {
		roundMain(os.Args[2:])
		return
	}
	residentMain(os.Args[1:])
}

func selfPath() string {
	if s := os.Getenv("HC_LANE_SELF"); s != "" {
		return s
	}
	return "hc-lane"
}

func evdir() string {
	if s := os.Getenv("XDG_STATE_HOME"); s != "" {
		return s + "/hc-lane"
	}
	return os.Getenv("HOME") + "/.local/state/hc-lane"
}

func die(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(os.Stderr, "hc-lane: %s\n", msg)
	if gRun {
		top := gTop
		if top == "" {
			top = "?"
		}
		// 병합기가 멈추면 막힘 사건(main에 쓰는 곳이 하나라 조용히 서면 전체 착지가 선다)
		c := exec.Command(gSelf, "report", "막힘", "-", fmt.Sprintf("병합기 멈춤: %s (클론 %s)", msg, top), "--name", "merger")
		if err := c.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "hc-lane merger: 멈춤 사건을 적지 못했다: %v\n", err)
		}
	}
	os.Exit(1)
}

// onSignals: TERM·INT·HUP이면 지금 기다리는 자식 무리를 끄고(유예 뒤 KILL) 128+신호로 끝낸다
func onSignals(grace time.Duration) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		s := <-ch
		proc.StopCurrent(grace) // 끌 동안 새 자식을 띄우지 않는다(풀지 않고 끝낸다)
		os.Exit(128 + int(s.(syscall.Signal)))
	}()
}

// flockNB: 파일을 열어 비켜 잠근다(lockx). busy = 남이 쥠. 쥔 잠금은 gLocks에 둔다
func flockNB(path string) (busy bool, err error) {
	f, busy, err := lockx.TryLock(path)
	if f != nil {
		gLocks = append(gLocks, f)
	}
	return busy, err
}

var digits = regexp.MustCompile(`^[0-9]+$`)

// intSetting: 정수 설정(요구 5). 비었으면 기본값, 정수가 아니거나 min보다 작으면 기본값과 경고
func intSetting(name, v string, def, min int) int {
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if !digits.MatchString(v) || err != nil || n < min {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 경고 — %s 는 %d 이상 정수여야 한다: %q — 기본값 %d 로\n", name, min, v, def)
		return def
	}
	return n
}

func ckey(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 128 && (r == '_' || r == '-' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') {
			return r
		}
		return '_'
	}, s)
}

// plain: 출력하는 명령에 따옴표 없이 넣는 가지 이름 글자
func plain(s string) bool {
	return s != "" && strings.Trim(s, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789/_.-") == ""
}

func machine() string {
	if m := os.Getenv("HC_LANE_MACHINE"); m != "" {
		return m
	}
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return strings.SplitN(h, ".", 2)[0]
}

// originBase: 원격 기본 가지(bash repo_defaults와 같은 조회). 실패면 빈 값
func originBase() string {
	switch runQ("-C", ".", "remote", "get-url", "origin") {
	case 0:
		refs, rc := out("-C", ".", "for-each-ref", "--format=%(symref)", "refs/remotes/origin/HEAD")
		if rc != 0 {
			return ""
		}
		if b := strings.TrimPrefix(refs, "refs/remotes/origin/"); b != "" {
			return b
		}
		return "main"
	case 2:
		refs, rc := out("-C", ".", "for-each-ref", "--format=%(refname)", "refs/heads/main", "refs/heads/master")
		if rc != 0 {
			return ""
		}
		l := "\n" + refs + "\n"
		if !strings.Contains(l, "\nrefs/heads/main\n") && strings.Contains(l, "\nrefs/heads/master\n") {
			return "master"
		}
		return "main"
	}
	return ""
}

func physical(p string) (string, error) {
	a, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(a)
}

// residentMain: merger --repo <전용 클론> [--once] [--tick <초>] [<기본 브랜치>] — 한 클론에 하나(잠금)
func residentMain(args []string) {
	once, repo, base, tickArg := false, "", "", ""
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--repo" || a == "--tick":
			if i+1 >= len(args) {
				die("%s 값이 없다", a)
			}
			i++
			if a == "--repo" {
				repo = args[i]
			} else {
				tickArg = args[i]
			}
		case a == "--once":
			once = true
		case strings.HasPrefix(a, "-"):
			die("merger: 알 수 없는 인자: %s", a)
		default:
			if base != "" {
				die("merger: 남는 인자: %s", a)
			}
			base = a
		}
	}
	// 「쉬지 않고 ls-remote」를 막는 값 — 정수가 아니면 기본 30초(요구 5)
	tick := intSetting("--tick", tickArg, 30, 0)
	batch := intSetting("HC_LANE_MERGER_BATCH", os.Getenv("HC_LANE_MERGER_BATCH"), 3, 1)
	if repo == "" {
		die("merger: --repo <병합기 전용 클론> 이 필요하다")
	}
	// Go 빌드 캐시를 macOS가 비우는 ~/Library/Caches 밖에(착지가 도는 저장소 검증용 — bash 판과 같음)
	if os.Getenv("GOCACHE") == "" {
		os.Setenv("GOCACHE", os.Getenv("HOME")+"/.cache/go-build")
	}
	if err := os.Chdir(repo); err != nil {
		die("merger: git 저장소가 아니다: %s", repo)
	}
	top, rc := outQ("rev-parse", "--show-toplevel")
	if rc != 0 || top == "" || os.Chdir(top) != nil {
		die("merger: git 저장소가 아니다: %s", repo)
	}
	if runQ("remote", "get-url", "origin") != 0 {
		die("merger: origin이 없다: %s", top)
	}
	if base == "" {
		if base = originBase(); base == "" {
			die("merger: 기본 브랜치 조회 실패")
		}
	}
	if !plain(base) {
		die("merger: 기본 브랜치 이름에 쓸 수 없는 글자가 있다: %s", base)
	}
	if err := os.MkdirAll(gEvdir, 0o777); err != nil {
		die("merger: 상태 폴더를 만들지 못했다: %s", gEvdir)
	}
	// 잠금 = 이 프로세스가 연 파일의 flock(bash 판과 같은 파일·같은 뜻 — 두 판이 함께 돌지 못한다). 자식에 물리지 않으니 병합기가 죽으면
	// 바로 놓인다(착지 자식·검증이 남긴 고아가 살아 있어도)
	lk := gEvdir + "/merger." + ckey(top) + ".lock"
	busy, err := flockNB(lk)
	if busy {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 이 클론의 병합기가 아직 돈다 — 잠금 %s\n", lk)
		os.Exit(75)
	}
	if err != nil {
		die("merger: 잠금을 걸지 못했다(%v): %s", err, lk)
	}
	gRun, gTop = true, top
	onSignals(roundGrace)
	mj, rc := out("rev-parse", "--git-path", "hc-merger")
	if rc != 0 || mj == "" || os.MkdirAll(mj+"/out", 0o777) != nil || os.MkdirAll(mj+"/fail", 0o777) != nil {
		die("merger: 일지 폴더를 만들지 못했다")
	}
	if mj, err = physical(mj); err != nil {
		die("merger: 일지 폴더를 만들지 못했다")
	}
	// 후보 작업 트리 자리: 클론 옆 <클론>.merger-wt/ — .git 밖·점으로 시작하는 마디 없이(Vite fs.deny 등)
	mwr := top + ".merger-wt"
	if err := os.MkdirAll(mwr, 0o777); err != nil {
		die("merger: 후보 작업 트리 폴더를 만들지 못했다: %s", mwr)
	}
	if mwr, err = physical(mwr); err != nil {
		die("merger: 후보 작업 트리 폴더를 만들지 못했다: %s.merger-wt", top)
	}
	// 클론 작업 트리는 분리 HEAD로(가지를 체크아웃해 두면 착지 뒤 메인 폴더 맞추기가 클론 작업 트리를 바꾼다). 깨끗할 때만, 한 번
	switch runO("symbolic-ref", "-q", "HEAD") {
	case 0:
		st, rc := out("status", "--porcelain", "--untracked-files=all")
		if rc != 0 || st != "" || run("checkout", "-q", "--detach") != 0 {
			die("merger: 병합기 클론을 분리 HEAD로 두지 못했다(커밋 안 된 변경?): %s", top)
		}
	case 1:
	default:
		fmt.Fprintln(os.Stderr, "hc-lane merger: HEAD의 가지 조회 실패")
		os.Exit(75)
	}
	ml := gEvdir + "/merger-logs"
	if err := os.MkdirAll(ml, 0o777); err != nil {
		die("merger: 로그 폴더를 만들지 못했다: %s", ml)
	}
	mme := machine()
	if once {
		fmt.Printf("hc-lane merger: %s (%s) 큐가 빌 때까지 한 번\n", top, base)
	} else {
		fmt.Printf("hc-lane merger: %s (%s) 큐를 본다 — 빈 바퀴 %d초\n", top, base, tick)
	}
	exe, err := os.Executable()
	if err != nil {
		die("merger: 자기 실행 파일을 찾지 못했다: %v", err)
	}
	for {
		c := exec.Command(exe, "--round", strconv.Itoa(os.Getpid()), top, base, mj, mwr, ml, mme, strconv.Itoa(batch))
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		switch rc := proc.Wait(c, ""); rc {
		case 76:
			continue
		case 0, 75:
			if once {
				os.Exit(rc)
			}
		default:
			os.Exit(rc)
		}
		// 빈 바퀴 쉼은 sleep 명령으로(bash 판과 같은 자리 — 시험이 PATH의 sleep으로 쉼 값을 본다). 멈추면 무리째 끈다
		s := exec.Command("sleep", strconv.Itoa(tick))
		s.Stdout, s.Stderr = os.Stdout, os.Stderr
		proc.Wait(s, "")
	}
}

// roundMain: --round <부모 pid> <클론> <기본> <일지 폴더> <후보 폴더> <로그 폴더> <기계> <묶음> — 한 바퀴. 종료 76 일을 함(바로 다음 바퀴),
// 75 원격 오류·남은 일로 쉼, 0 할 일 없음, 1 멈춤
func roundMain(a []string) {
	if len(a) != 8 {
		die("merger: --round 인자 수가 틀렸다")
	}
	parent, err := strconv.Atoi(a[0])
	batch, err2 := strconv.Atoi(a[7])
	if err != nil || err2 != nil {
		die("merger: --round 인자가 틀렸다")
	}
	m := &M{parent: parent, top: a[1], base: a[2], mj: a[3], mwr: a[4], ml: a[5], mme: a[6], batch: batch}
	if err := os.Chdir(m.top); err != nil {
		die("merger: git 저장소가 아니다: %s", m.top)
	}
	gRun, gTop = true, m.top
	onSignals(landGrace)
	// 생성부터 정리까지 이 일꾼이 후보 잠금을 소유한다. 부모가 죽어도 이 바퀴는 마칠 수 있다
	busy, err := flockNB(m.mj + "/candidate.lock")
	if err != nil {
		die("merger: 후보 잠금을 잡지 못했다: %v", err)
	}
	// 둘 다 죽은 앞 실행의 착지가 아직 돌면(자식 기록의 pid + 시작 시각) 그 후보도 쓰는 중이다
	if busy || proc.ChildBusy(m.mj+"/child", landGrace) {
		fmt.Fprintln(os.Stderr, "hc-lane merger: 앞 실행이 후보 작업 트리를 쓰고 있다")
		os.Exit(75)
	}
	m.round()
	switch {
	case m.mq != "":
		os.Exit(76)
	case m.mer:
		os.Exit(75)
	}
	os.Exit(0)
}

// parentAlive: 상주 부모가 아직 이 일꾼의 부모인가(죽으면 init·launchd 아래로 옮겨진다)
func (m *M) parentAlive() bool { return os.Getppid() == m.parent }
