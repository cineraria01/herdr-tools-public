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
	"herdr-tools/internal/oplog"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/version"
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
	closeInherited()
	gSelf, gEvdir = selfPath(), evdir()
	proc.Who = "hc-lane merger"
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(version.String("hc-merger")) // 릴리스 커밋이 보이게(B25)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--spec-verify" { // 추측 검증 일꾼(specverify.go)
		os.Exit(specVerifyMain(os.Args[2:]))
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
	oplog.Log("hc-merger", "", oplog.Die, 1, msg)
	if gRun {
		top := gTop
		if top == "" {
			top = "?"
		}
		// 병합기가 멈추면 막힘 사건(main에 쓰는 곳이 하나라 조용히 서면 전체 착지가 선다)
		reportBlocked(fmt.Sprintf("병합기 멈춤: %s (클론 %s)", msg, top))
	}
	os.Exit(1)
}

// reportBlocked: 막힘 사건(--name merger)을 상한 안에서 적는다(E05 — 사건 쓰기가 멈춰도 병합기가 서지 않게)
func reportBlocked(msg string) {
	if _, rc, e := boundedRun(reportMax, gSelf, nil, "report", "막힘", "-", msg, "--name", "merger"); rc != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 막힘 사건을 적지 못했다(종료 %d): %s\n", rc, strings.TrimSpace(e))
	}
}

// onSignals: TERM·INT·HUP이면 지금 기다리는 자식 무리를 끄고(유예 뒤 KILL) 128+신호로 끝낸다
// 이 처리기 하나가 맡는다(proc.TakeSignals) — proc 처리기(유예 2초)가 함께 받아 먼저 끝내면 상주가 일꾼이 거두는 중에 끝났다(재감사 R-H04)
func onSignals(grace time.Duration) {
	proc.TakeSignals()
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

// ckeyBytes: bash 판을 C 로캘(launchd 등)에서 돌린 꼴 — 바이트마다 _ 하나
func ckeyBytes(s string) string {
	b := []byte(s)
	for i, c := range b {
		if !(c == '_' || c == '-' || c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z') {
			b[i] = '_'
		}
	}
	return string(b)
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

// originBase: 원격 기본 가지(bash repo_defaults와 같은 조회)와 git 종료 코드(0이 아니면 빈 값 — gitUnknown이면 모름)
func originBase() (string, int) {
	switch rc := runQ("-C", ".", "remote", "get-url", "origin"); rc {
	case 0:
		refs, rc := out("-C", ".", "for-each-ref", "--format=%(symref)", "refs/remotes/origin/HEAD")
		if rc != 0 {
			return "", rc
		}
		if b := strings.TrimPrefix(refs, "refs/remotes/origin/"); b != "" {
			return b, 0
		}
		return "main", 0
	case 2:
		refs, rc := out("-C", ".", "for-each-ref", "--format=%(refname)", "refs/heads/main", "refs/heads/master")
		if rc != 0 {
			return "", rc
		}
		l := "\n" + refs + "\n"
		if !strings.Contains(l, "\nrefs/heads/main\n") && strings.Contains(l, "\nrefs/heads/master\n") {
			return "master", 0
		}
		return "main", 0
	default:
		return "", rc
	}
}

// gitUnknown: 시작 단계 git 종료 코드가 「모름」인가 — 시간 넘김(proc.TimeoutCode)·신호로 끝남(128 넘음). 확인된 부재·오류가 아니다(R-H08)
func gitUnknown(rc int) bool { return rc == proc.TimeoutCode || rc > 128 }

// startRepo: 시작 단계 — 클론·origin·기본 브랜치 확인. 0 됨 · 75 모름(git 시간 넘김 등 — 바꾼 것 없음). 확인된 부재는 die
func startRepo(repo, base string) (top, b string, rc int) {
	if err := os.Chdir(repo); err != nil {
		die("merger: git 저장소가 아니다: %s", repo)
	}
	top, rc = outQ("rev-parse", "--show-toplevel")
	if gitUnknown(rc) {
		return "", "", 75
	}
	if rc != 0 || top == "" || os.Chdir(top) != nil {
		die("merger: git 저장소가 아니다: %s", repo)
	}
	switch rc := runQ("remote", "get-url", "origin"); {
	case rc == 0:
	case gitUnknown(rc):
		return "", "", 75
	default:
		die("merger: origin이 없다: %s", top)
	}
	if base == "" {
		var rc int
		if base, rc = originBase(); gitUnknown(rc) {
			return "", "", 75
		}
		if base == "" {
			die("merger: 기본 브랜치 조회 실패")
		}
	}
	return top, base, 0
}

// detachClone: 클론 작업 트리를 분리 HEAD로(가지를 체크아웃해 두면 착지 뒤 메인 폴더 맞추기가 클론 작업 트리를 바꾼다). 깨끗할 때만.
// 0 됨 · 75 모름(바꾼 것 없음 — 다시) · 커밋 안 된 변경이 확인되면 die
func detachClone(top string) int {
	switch rc := runO("symbolic-ref", "-q", "HEAD"); rc {
	case 0:
		st, rc := out("status", "--porcelain", "--untracked-files=all")
		if gitUnknown(rc) {
			return 75
		}
		if rc != 0 || st != "" {
			die("merger: 병합기 클론을 분리 HEAD로 두지 못했다(커밋 안 된 변경?): %s", top)
		}
		if rc := run("checkout", "-q", "--detach"); rc != 0 {
			if gitUnknown(rc) {
				return 75
			}
			die("merger: 병합기 클론을 분리 HEAD로 두지 못했다(git checkout 종료 %d): %s", rc, top)
		}
	case 1:
	default:
		fmt.Fprintf(os.Stderr, "hc-lane merger: HEAD의 가지 조회 실패(종료 %d)\n", rc)
		return 75
	}
	return 0
}

// startWait: 시작 단계 git이 모름이면 — 단발(--once)은 바꾼 것 없이 75, 상주는 tick초 뒤 다시(R-H08 — 잠시 느린 로컬 git으로 상주가 서지
// 않게). 같은 자리에서 deferAlarm번 이어지면 막힘 사건을 한 번(지휘가 받을 진단)
func startWait(once bool, tick int, what string, n *int) {
	if once {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 시작 단계 %s 결과 모름(git 시간 넘김 등) — 바꾼 것 없이 쉰다\n", what)
		os.Exit(75)
	}
	*n++
	fmt.Fprintf(os.Stderr, "hc-lane merger: 시작 단계 %s 결과 모름(git 시간 넘김 등) — %d초 뒤 다시(%d번째)\n", what, max(tick, 1), *n)
	if *n == deferAlarm {
		reportBlocked(fmt.Sprintf("병합기 시작 단계 %s 가 %d번 이어 모름(git 시간 넘김 등) — 다시 하는 중", what, *n))
	}
	time.Sleep(time.Duration(max(tick, 1)) * time.Second)
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
	once, repo, base, tickArg, tickSet := false, "", "", "", false
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
				tickArg, tickSet = args[i], true
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
	// 「쉬지 않고 ls-remote」를 막는 값 — 명시한 값이 정수가 아니면 bash 판처럼 멈춘다(E10), 없으면 30초
	if tickSet && !digits.MatchString(tickArg) {
		die("merger: --tick은 초(정수): %s", tickArg)
	}
	tick := intSetting("--tick", tickArg, 30, 0)
	batch := intSetting("HC_LANE_MERGER_BATCH", os.Getenv("HC_LANE_MERGER_BATCH"), 3, 1)
	if repo == "" {
		die("merger: --repo <병합기 전용 클론> 이 필요하다")
	}
	// Go 빌드 캐시를 macOS가 비우는 ~/Library/Caches 밖에(착지가 도는 저장소 검증용 — bash 판과 같음)
	if os.Getenv("GOCACHE") == "" {
		os.Setenv("GOCACHE", os.Getenv("HOME")+"/.cache/go-build")
	}
	var top string
	for tries := 0; ; {
		t, b, rc := startRepo(repo, base)
		if rc == 0 {
			top, base = t, b
			break
		}
		startWait(once, tick, "클론·origin 확인", &tries)
	}
	if !plain(base) {
		die("merger: 기본 브랜치 이름에 쓸 수 없는 글자가 있다: %s", base)
	}
	if err := os.MkdirAll(gEvdir, 0o777); err != nil {
		die("merger: 상태 폴더를 만들지 못했다: %s", gEvdir)
	}
	// 잠금 = 이 프로세스가 연 파일의 flock(bash 판과 같은 파일·같은 뜻 — 두 판이 함께 돌지 못한다). 자식에 물리지 않으니 병합기가 죽으면
	// 바로 놓인다(착지 자식·검증이 남긴 고아가 살아 있어도)
	// 잠금 파일 이름은 bash 판 tr이 로캘에 따라 글자(UTF-8)마다 또는 바이트(C)마다 _ 하나라 두 꼴을 다 쥔다(E04·F5 — 클론 경로에 한글이 있어도
	// 어느 로캘의 bash 판과도 함께 돌지 못하게). ASCII 경로면 한 파일
	for _, k := range []string{ckey(top), ckeyBytes(top)} {
		lk := gEvdir + "/merger." + k + ".lock"
		busy, err := flockNB(lk)
		if busy {
			fmt.Fprintf(os.Stderr, "hc-lane merger: 이 클론의 병합기가 아직 돈다 — 잠금 %s\n", lk)
			os.Exit(75)
		}
		if err != nil {
			die("merger: 잠금을 걸지 못했다(%v): %s", err, lk)
		}
		if ckey(top) == ckeyBytes(top) {
			break
		}
	}
	gRun, gTop = true, top
	onSignals(roundGrace)
	var err error
	var mj string
	for tries := 0; ; {
		var rc int
		if mj, rc = out("rev-parse", "--git-path", "hc-merger"); !gitUnknown(rc) {
			break
		}
		startWait(once, tick, "일지 폴더 조회", &tries)
	}
	if mj == "" || os.MkdirAll(mj+"/out", 0o777) != nil || os.MkdirAll(mj+"/fail", 0o777) != nil {
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
	for tries := 0; ; {
		rc := detachClone(top)
		if rc == 0 {
			break
		}
		startWait(once, tick, "클론 HEAD 분리", &tries)
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
	for fails := 0; ; {
		pause, rc := tick, 127
		if exe := workerExe(); exe != "" {
			c := exec.Command(exe, "--round", strconv.Itoa(os.Getpid()), top, base, mj, mwr, ml, mme, strconv.Itoa(batch))
			c.Stdout, c.Stderr = os.Stdout, os.Stderr
			rc = proc.Wait(c, "")
		} else {
			fmt.Fprintln(os.Stderr, "hc-lane merger: 바퀴 일꾼 hc-merger를 찾지 못했다(자기 자리에도 PATH에도 없음)")
		}
		switch rc {
		case 76:
			fails = 0
			continue
		case 0, 75:
			fails = 0
			if once {
				os.Exit(rc)
			}
		case 1: // 일꾼이 멈추며 막힘 사건을 이미 냈다
			os.Exit(rc)
		default: // 못 떴거나(127 — 바이너리를 치움) 신호·패닉으로 사건 없이 끝났다(G2). 바퀴 도중이었으면 다음 바퀴가 일지로 잇는다
			if fails++; fails >= workerMaxFails {
				die("merger: 바퀴 일꾼이 뜻밖에 끝났다(종료 %d, 연속 %d번)", rc, fails)
			}
			msg := fmt.Sprintf("병합기 일꾼 다시: 바퀴 일꾼이 뜻밖에 끝났다(종료 %d, 연속 %d/%d번) — %d초 뒤 다시 (클론 %s)",
				rc, fails, workerMaxFails, workerRetryWait, top)
			fmt.Fprintf(os.Stderr, "hc-lane merger: 경고 — %s\n", msg)
			reportMid(msg)
			pause = workerRetryWait
		}
		// 빈 바퀴 쉼은 sleep 명령으로(bash 판과 같은 자리 — 시험이 PATH의 sleep으로 쉼 값을 본다). 멈추면 무리째 끈다
		s := exec.Command("sleep", strconv.Itoa(pause))
		s.Stdout, s.Stderr = os.Stdout, os.Stderr
		proc.Wait(s, "")
	}
}

const (
	workerMaxFails  = 5  // 바퀴 일꾼이 이만큼 이어서 뜻밖에 끝나면 막힘·멈춤
	workerRetryWait = 30 // 그 전까지는 이만큼 쉬고 다시(초)
)

// workerExe: 바퀴마다 일꾼 경로를 새로 찾는다 — 자기 실행 파일 자리(herdr-sync가 rename으로 바꿔 넣으면 다음 바퀴가 새 판), 없으면
// PATH의 hc-merger. 시작 때 잡은 경로만 쓰면 그 자리를 치운 뒤 매 바퀴 127로 섰다(2026-11-22). 둘 다 없으면 빈 값
func workerExe() string {
	if p, err := os.Executable(); err == nil {
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0 {
			return p
		}
	}
	p, _ := exec.LookPath("hc-merger")
	return p
}

// orphansDefer: 죽은 앞 실행이 남긴 자식 무리(준비 훅·검증·착지 — proc 고아 기록)를 먼저 거두고, 거두지 못했거나 소유를 모르는 것이
// 남았으면 그 작업 트리를 쓰지 않고 75로 미룬다(재감사 2바퀴 merger R-209 — 옛 판은 기록 없이 띄워 SIGKILL된 일꾼의 훅이 남아 같은
// 작업 트리를 함께 썼다). 참이면 부른 쪽은 75로 끝낸다
func orphansDefer(what string) bool {
	n := proc.Orphans()
	if n > 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 앞 실행이 남긴 자식 무리 %d개가 아직 도는지 모른다(%s) — %s를 쓰지 않고 미룸\n", n, proc.RecordDir(), what)
	}
	return n > 0
}

// reportMid: 중간 사건(--name merger)을 상한 안에서 — 못 적어도 경고만
func reportMid(msg string) {
	if _, rc, e := boundedRun(reportMax, gSelf, nil, "report", "중간", "-", msg, "--name", "merger"); rc != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 중간 사건을 적지 못했다(종료 %d): %s\n", rc, strings.TrimSpace(e))
	}
}

// roundMain: --round <부모 pid> <클론> <기본> <일지 폴더> <후보 폴더> <로그 폴더> <기계> <묶음> — 한 바퀴. 종료 76 일을 함(바로 다음 바퀴),
// 75 원격 오류·남은 일로 쉼, 0 할 일 없음, 1 멈춤
func roundMain(a []string) {
	gRun = true // 인자부터 틀려도(상주와 일꾼의 판이 갈림 — F14) 막힘 사건을 남긴다
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
	if busy {
		fmt.Fprintln(os.Stderr, "hc-lane merger: 앞 실행이 후보 작업 트리를 쓰고 있다")
		os.Exit(75)
	}
	// 둘 다 죽은 앞 실행의 착지가 아직 돌거나(자식 기록의 pid + 시작 시각) 그 무리·고아 무리가 남았으면 그 후보도 쓰는 중이다 — 쉰다.
	// 남은 무리·모름은 쉰 바퀴로 세어 이어지면 막힘 사건(재감사 3바퀴 merger/R-3M01 — 옛 판은 round에 못 들어가 사건 없이 끝없이 75)
	if why, count := m.childWait(); why != "" {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 앞 실행이 후보 작업 트리를 쓰고 있다 — %s\n", why)
		if count {
			m.deferred = why
			m.deferNote()
		}
		os.Exit(75)
	}
	m.round()
	m.deferNote()
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
