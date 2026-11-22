package main

// 추측 병렬 검증 — 검증 기계 일꾼: hc-lane merger --spec-verify <아이디> <기본 브랜치>(병합기가 hc run --on <기계> --cwd <그 기계 클론>으로 띄움).
// spec 참조를 받아 <클론>.spec-wt/<아이디 8>에 분리 작업 트리 → 준비 훅 → 부하가 높으면 기다림 → 저장소 검증 → 통과면 verified 기록,
// 실패면 spec-fail 참조, 준비 실패·검증이 트리를 바꿈이면 state=lost(모름) → 작업 트리 정리. 결과를 쓰기 전에 spec 참조가 그대로인지 본다
// (병합기가 버린 추측에 결과를 남기지 않게). 종료: 0 결과를 씀(또는 버려진 추측) · 1 못 함 · 2 인자 · 75 원격 받기 실패

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/gitx"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/verified"
)

func specVerifyMain(a []string) int {
	if len(a) != 2 || len(a[0]) != 40 || strings.Trim(a[0], "0123456789abcdef") != "" || !plain(a[1]) {
		fmt.Fprintln(os.Stderr, "hc-lane merger: --spec-verify <아이디 40자> <기본 브랜치>")
		return 2
	}
	id, base := a[0], a[1]
	top, rc := outQ("rev-parse", "--show-toplevel")
	if rc != 0 || os.Chdir(top) != nil {
		fmt.Fprintln(os.Stderr, "hc-lane merger: --spec-verify — git 클론 안에서 부른다")
		return 1
	}
	ref := specRefs + id
	if fetchr(false, "-q", "origin", "+"+ref+":"+ref, "+refs/heads/"+base+":refs/remotes/origin/"+base) != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: --spec-verify — 추측 참조를 받지 못했다")
		return 75
	}
	cm, rc1 := out("rev-parse", "-q", "--verify", ref+"^{commit}")
	tree, rc2 := out("rev-parse", "-q", "--verify", ref+"^{tree}")
	subj, rc3 := out("log", "-1", "--format=%s", cm)
	r, ok := parseSpec(subj)
	if rc1 != 0 || rc2 != 0 || rc3 != 0 || !ok || r.conflict || r.dispatchX {
		fmt.Fprintf(os.Stderr, "hc-lane merger: --spec-verify — %s 는 검증할 추측이 아니다\n", ref)
		return 1
	}
	r.id, r.commit, r.tree = id, cm, tree
	defer runQ("update-ref", "-d", ref) // 이 클론(레인 클론일 수 있다)에 추측 참조를 남기지 않는다
	// 떴다는 표식 — 병합기가 못 뜬 일꾼을 시간 넘김까지 기다리지 않게(못 올려도 검증은 한다)
	if et, rc := outQ("hash-object", "-t", "tree", "-w", "/dev/null"); rc == 0 {
		if sc, ok := commitTree("", et, "", "hc-merger-spec-run machine="+machine()); ok {
			runQ("push", "-q", "--force-with-lease="+specStarts+id+":", "origin", sc+":"+specStarts+id)
		}
	}
	wtr := top + ".spec-wt"
	wt := wtr + "/" + id[:8]
	if err := os.MkdirAll(wtr, 0o777); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: --spec-verify — %v\n", err)
		return 1
	}
	specRmWT(wt)
	lg := wt + ".log"
	if inLog("", lg, "worktree", "add", "-q", "-f", "--detach", wt, cm) != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: --spec-verify — 작업 트리를 만들지 못했다(로그 %s)\n", lg)
		return 1
	}
	defer specRmWT(wt)
	res := specRunVerify(wt, lg, top, base, id, tree)
	return specWrite(top, r, res, lg)
}

type specResult int

const (
	resPass specResult = iota
	resFail
	resLost
)

// specRunVerify: 준비 훅 → 부하 기다림 → 저장소 검증(시간 상한 HC_LANE_SPEC_TIMEOUT) → 트리·작업 트리가 그대로인지
func specRunVerify(wt, lg, top, base, id, tree string) specResult {
	lf, err := os.OpenFile(lg, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		return resLost
	}
	defer lf.Close()
	env := append(os.Environ(), "LANE_REPO="+top, "LANE_BASE="+base)
	if _, err := os.Stat(wt + "/.claude/herdr-setup.sh"); err == nil {
		c := exec.Command("bash", ".claude/herdr-setup.sh")
		c.Dir, c.Stdout, c.Stderr = wt, lf, lf
		c.Env = append(env, "PATH="+pathWithout("codebase-memory-mcp"))
		if rc := proc.WaitMax(c, "", setupMax()); rc != 0 {
			fmt.Fprintf(lf, "hc-lane merger: 추측 준비 훅 실패(종료 %d) — 모름\n", rc)
			return resLost
		}
	}
	specLoadWait(lf)
	if _, err := os.Stat(wt + "/" + verified.Script); err != nil {
		fmt.Fprintf(lf, "hc-lane merger: 추측 트리에 %s 가 없다 — 모름\n", verified.Script)
		return resLost
	}
	c := exec.Command("bash", verified.Script)
	c.Dir, c.Stdout, c.Stderr = wt, lf, lf
	c.Env = append(env, "HC_LANE=spec-"+id[:8], "HERDR_SPEC_VERIFY=1", "HERDR_VERIFIED_TREE=")
	rc := proc.WaitMax(c, "", time.Duration(specTimeout())*time.Second)
	fmt.Fprintf(lf, "hc-lane merger: 추측 검증 종료 %d\n", rc)
	if rc == proc.TimeoutCode {
		return resLost
	}
	t, trc := gitx.Out(wt, lf, "rev-parse", "HEAD^{tree}")
	st, src := gitx.Out(wt, lf, "status", "--porcelain", "--untracked-files=no")
	if trc != 0 || src != 0 || t != tree || st != "" {
		fmt.Fprintln(lf, "hc-lane merger: 검증이 트리·추적 파일을 바꿨다(또는 못 읽음) — 모름")
		return resLost
	}
	if rc != 0 {
		return resFail
	}
	return resPass
}

// specWrite: 결과를 원격에. spec 참조가 그대로일 때만
func specWrite(top string, r specRow, res specResult, lg string) int {
	ref := specRefs + r.id
	now, rc := outQ("ls-remote", "origin", ref)
	if rc != 0 {
		return 1
	}
	if c, _, _ := strings.Cut(now, "\t"); c != r.commit {
		fmt.Println("hc-lane merger: 병합기가 이 추측을 버렸다 — 결과를 남기지 않는다")
		return 0
	}
	at := time.Now().Unix()
	switch res {
	case resPass:
		if verified.Record(top, verified.Rec{Tree: r.tree, Machine: machine(), At: at}, os.Stderr) != gitx.Yes {
			return 1
		}
		fmt.Printf("hc-lane merger: 추측 검증 통과 — %s%s\n", verified.Prefix, r.tree)
	case resFail:
		et, rc := out("hash-object", "-t", "tree", "-w", "/dev/null")
		if rc != 0 {
			return 1
		}
		msg := fmt.Sprintf("hc-merger-spec-fail id=%s machine=%s at=%d log=%s", r.id, machine(), at, lg)
		fc, ok := commitTree("", et, "", msg)
		if !ok || runO("push", "-q", "--force-with-lease="+specFails+r.id+":", "origin", fc+":"+specFails+r.id) != 0 {
			return 1
		}
		fmt.Printf("hc-lane merger: 추측 검증 실패 — 로그 %s\n", lg)
	default:
		lc, ok := commitTree("", r.tree, r.commit+"^", specMsg(r.parent, r.group, "-", r.at, "lost"))
		if !ok || runO("push", "-q", "--force-with-lease="+ref+":"+r.commit, "origin", lc+":"+ref) != 0 {
			return 1
		}
		fmt.Printf("hc-lane merger: 추측 검증 모름 — 로그 %s\n", lg)
	}
	return 0
}

// specRmWT: 일꾼 작업 트리를 지운다(git이 못 지우면 직접 — <클론>.spec-wt/ 아래 자기 폴더만)
func specRmWT(wt string) {
	if _, err := os.Lstat(wt); err != nil {
		runQ("worktree", "prune")
		return
	}
	if runQ("worktree", "remove", "-f", "-f", wt) != 0 && strings.HasSuffix(filepath.Dir(wt), ".spec-wt") {
		os.RemoveAll(wt)
	}
	runQ("worktree", "prune")
}

// specLoadWait: 1분 부하가 코어 수를 넘으면 30초씩 기다린다(HC_LANE_SPEC_LOAD_WAIT 초, 기본 900). 레인·검증 자리와 다투지 않게 —
// 검증 자리 자체는 저장소 검증 스크립트의 기계 잠금(기계별 검증 자리 수 등)이 나눈다.
// ponytail: 부하 한 값만 본다, hc-fill 레인 자리 수까지 보려면 검증 기계 목록에 자리 정보를 싣는다
func specLoadWait(lf *os.File) {
	limit := time.Now().Add(time.Duration(intSetting("HC_LANE_SPEC_LOAD_WAIT", os.Getenv("HC_LANE_SPEC_LOAD_WAIT"), 900, 0)) * time.Second)
	for {
		l, ok := load1()
		if !ok || l <= float64(runtime.NumCPU()) || time.Now().After(limit) {
			return
		}
		fmt.Fprintf(lf, "hc-lane merger: 부하 %.1f > 코어 %d — 30초 기다림\n", l, runtime.NumCPU())
		time.Sleep(30 * time.Second)
	}
}

// load1: 1분 부하(리눅스 /proc/loadavg, macOS sysctl vm.loadavg). 못 읽으면 ok=false
func load1() (float64, bool) {
	s := ""
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		s = string(b)
	} else if o, err := exec.Command("sysctl", "-n", "vm.loadavg").Output(); err == nil {
		s = strings.Trim(strings.TrimSpace(string(o)), "{} ")
	}
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0, false
	}
	v, err := strconv.ParseFloat(f[0], 64)
	return v, err == nil
}
