package main

// 추측 병렬 검증 — 검증 기계 일꾼: hc-lane merger --spec-verify <아이디> <기본 브랜치>(병합기가 hc run --on <기계> --cwd <그 기계 클론>으로 띄움).
// spec 참조를 받아 검증 자리 기다림(busy_wait초 안에 한가해지지 않으면 state=busy를 남기고 끝 — verifierpick.go) →
// <클론>.spec-wt/<아이디 8>에 분리 작업 트리 → 준비 훅 → 저장소 검증 → 통과면 verified 기록,
// 실패면 spec-fail 참조, 준비 실패·검증이 트리를 바꿈이면 state=lost(모름) → 작업 트리 정리. 결과를 쓰기 전에 spec 참조가 그대로인지 본다
// (병합기가 버린 추측에 결과를 남기지 않게). 종료: 0 결과를 씀(또는 버려진 추측) · 1 못 함 · 2 인자 · 75 원격 받기 실패·죽은 앞 일꾼의
// 자식 무리가 남음(proc.Orphans — 준비 훅·검증 명령은 고아 기록에 올라 다음 일꾼이 거두거나 미룬다)

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/gitx"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/shx"
	"herdr-tools/internal/verified"
)

func specVerifyMain(a []string) int {
	shard := "" // 나눔 몫 「i/K」(specshards.go)
	if len(a) == 3 {
		shard = a[2]
		a = a[:2]
		if _, ok := shardArg(shard); !ok {
			fmt.Fprintln(os.Stderr, "hc-lane merger: --spec-verify — 몫은 <i>/<K>(1 ≤ i ≤ K ≤ 8, K ≥ 2)")
			return 2
		}
	}
	if len(a) != 2 || len(a[0]) != 40 || strings.Trim(a[0], "0123456789abcdef") != "" || !plain(a[1]) {
		fmt.Fprintln(os.Stderr, "hc-lane merger: --spec-verify <아이디 40자> <기본 브랜치> [<i>/<K>]")
		return 2
	}
	id, base := a[0], a[1]
	own := shardOwn(id, shard)
	if orphansDefer("추측 작업 트리") { // 죽은 앞 일꾼의 준비 훅·검증이 남았으면 그 작업 트리를 지우거나 쓰기 전에(merger R-209)
		return 75
	}
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
	if rc1 != 0 || rc2 != 0 || rc3 != 0 || !ok || r.conflict || r.dispatchX || r.busy {
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
	if own != id {
		wt = wtr + "/" + own[:8] + own[40:] // 몫마다 따로(같은 기계에 두 몫이 와도 겹치지 않게)
	}
	if err := os.MkdirAll(wtr, 0o777); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: --spec-verify — %v\n", err)
		return 1
	}
	specRmWT(wt)
	specSweep(wtr) // 끝낸 앞 일꾼(hc kill — defer가 안 돎)이 남긴 작업 트리(frontsteady.go)
	lg := wt + ".log"
	// 검증 자리를 busy_wait초 안에 못 얻으면 검증하지 않고 「바쁨」(verifierpick.go) — 병합기가 곧바로 다른 한가한 기계로 다시 고른다
	if lf, err := os.OpenFile(lg, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666); err == nil {
		waited := specBusyWait(lf)
		lf.Close()
		if waited != "" && shard != "" {
			return specShardEnd(r, shard, "busy") // 몫의 바쁨은 그 몫의 끝일 뿐 — spec 참조는 다른 몫이 쓰고 있다
		}
		if waited != "" {
			return specWriteBusy(r, waited)
		}
	} else {
		fmt.Fprintf(os.Stderr, "hc-lane merger: --spec-verify — 로그를 열지 못했다: %v\n", err)
		return 1
	}
	if inLog("", lg, "worktree", "add", "-q", "-f", "--detach", wt, cm) != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: --spec-verify — 작업 트리를 만들지 못했다(로그 %s)\n", lg)
		return 1
	}
	defer specRmWT(wt)
	var ev specEvidence
	res := specRunVerify(wt, lg, top, base, id, shard, tree, &ev)
	if shard != "" && (res != resFail || ev.test == "") { // 몫은 시험 실패만 추측 실패 — 그 밖의 끝은 「그 몫은 끝남」(통과 기록은 마무리가)
		return specShardEnd(r, shard, [...]string{"pass", "env", "lost"}[res])
	}
	return specWrite(top, r, res, ev, lg)
}

type specResult int

const (
	resPass specResult = iota
	resFail
	resLost
)

// specRunVerify: 준비 훅 → 저장소 검증(시간 상한 HC_LANE_SPEC_TIMEOUT) → 트리·작업 트리가 그대로인지. 실패면 ev에 근거
func specRunVerify(wt, lg, top, base, id, shard, tree string, ev *specEvidence) (res specResult) {
	lf, err := os.OpenFile(lg, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		return resLost
	}
	defer lf.Close()
	var pp *partPub // 통과 조각(specparts.go) — 통과가 아닌 끝에(몫이면 늘) 마지막으로 올린다(다시 맡은·마무리 일꾼이 남은 조각만 돈다)
	defer func() {
		if pp != nil && (res != resPass || shard != "") {
			pp.push()
		}
	}()
	env := append(os.Environ(), "LANE_REPO="+top, "LANE_BASE="+base)
	if _, err := os.Stat(wt + "/.claude/herdr-setup.sh"); err == nil {
		c := shellCmd(wt, "sh", "-c", noIndexSh, "hc-merger", "bash", ".claude/herdr-setup.sh")
		c.Stdout, c.Stderr = lf, lf
		c.Env = env
		if rc := proc.WaitMax(c, "", setupMax()); rc != 0 {
			fmt.Fprintf(lf, "hc-lane merger: 추측 준비 훅 실패(종료 %d) — 모름\n", rc)
			return resLost
		}
	}
	prepared, ok := specPrepare(wt, lf, base, env)
	if !ok {
		return resLost
	}
	if _, err := os.Stat(wt + "/" + verified.Script); err != nil {
		fmt.Fprintf(lf, "hc-lane merger: 추측 트리에 %s 가 없다 — 모름\n", verified.Script)
		return resLost
	}
	var off int64 // 이 검증의 출력만 본다(같은 아이디의 로그에 앞 실행이 남았을 수 있다)
	if fi, err := lf.Stat(); err == nil {
		off = fi.Size()
	}
	pp = specPartsSeed(id, shardOwn(id, shard), tree, wt+".parts", lf)
	c := shellCmd(wt, "bash", verified.Script)
	c.Stdout, c.Stderr = lf, lf
	c.Env = append(env, "HC_LANE=spec-"+id[:8], "HERDR_SPEC_VERIFY=1", "HERDR_VERIFIED_TREE=", "HERDR_VERIFY_PARTS="+pp.file,
		"HERDR_VERIFY_SHARD="+shard) // 나눔 몫(없으면 빈 값 — 전체)
	max := time.Duration(specTimeout()) * time.Second
	stopParts := pp.watch()
	rc, ran := proc.WaitFor(c, "", max, max <= 0)
	stopParts()
	fmt.Fprintf(lf, "hc-lane merger: 추측 검증 종료 %d\n", rc)
	if rc == proc.TimeoutCode || !ran { // 못 띄움(검증은 안 돎)·시간 넘김은 모름 — spec-fail이 아니다(재감사 3바퀴 merger/R-3M02)
		return resLost
	}
	t, trc := gitx.Out(wt, lf, "rev-parse", "HEAD^{tree}")
	st, sok := wtState(wt, lf) // 준비 훅이 고친 추적 파일은 그대로여야 한다(훅 없으면 깨끗해야 — 지금 그대로)
	if trc != 0 || !sok || t != tree || st != prepared {
		fmt.Fprintln(lf, "hc-lane merger: 검증이 트리·추적 파일을 바꿨다(또는 못 읽음) — 모름")
		return resLost
	}
	if rc != 0 {
		ev.rc = rc
		// 시험 실패로 가르는 기준(merger-batch-culprit): 보통 종료(1~125 — 75 기다림 상한 뺌, 126·127 실행 못 함과 신호는 범위 밖)이고
		// 이 검증의 출력에 실패한 시험 줄(failLine)이 있다. 아니면 환경 문제와 가를 수 없다 — 병합기는 그 실패를 반송 근거로 쓰지 않는다
		if rc >= 1 && rc <= 125 && rc != 75 {
			ev.test = firstFail(lg, off)
		}
		return resFail
	}
	return resPass
}

// prepareHook: 추측 준비 훅(spec-script-land) — setup 뒤·검증 앞에 「얹은 뒤 다시 만들어야 하는 파생 파일」을 만든다
const prepareHook = ".claude/herdr-spec-prepare.sh"

// specPrepare: 준비 훅을 돌리고 그 뒤 작업 트리 상태(wtState)를 돌려준다 — 훅이 없거나 돌리지 않으면 ""(깨끗해야 한다, 지금 그대로).
// 훅은 착지를 .claude/herdr-land.sh에 맡기는 저장소(기본 브랜치에 그 스크립트)에서만 돈다: 그 스크립트가 착지 중에 스스로 검증하니
// 「검증한 트리 ≠ 넣을 트리」가 정확성을 깨지 않는다. 내장 착지는 추측 통과 기록으로 검증을 건너뛰므로 돌리지 않고 한 줄 알린다.
// 훅 실패·시간 넘김·환경을 못 읽음은 모름(ok=false) — 시험 실패·반송 근거가 아니다
func specPrepare(wt string, lf *os.File, base string, env []string) (string, bool) {
	if _, err := os.Stat(wt + "/" + prepareHook); err != nil {
		return "", true
	}
	hl, rc := gitx.Out(wt, lf, "ls-tree", "--name-only", "refs/remotes/origin/"+base, "--", ".claude/herdr-land.sh")
	if rc != 0 {
		fmt.Fprintf(lf, "hc-lane merger: 착지 스크립트가 있는지 못 읽었다 — 모름\n")
		return "", false
	}
	if hl == "" {
		fmt.Fprintf(lf, "hc-lane merger: %s 는 내장 착지 저장소에서 돌리지 않는다(추측 통과로 검증을 건너뛰니 검증한 트리와 넣을 트리가 같아야 한다)\n", prepareHook)
		return "", true
	}
	bs, rc1 := gitx.Out(wt, lf, "merge-base", "HEAD", "refs/remotes/origin/"+base)
	subs, rc2 := gitx.Out(wt, lf, "log", "--first-parent", "--reverse", "--format=%s", bs+"..HEAD")
	if rc1 != 0 || rc2 != 0 {
		fmt.Fprintf(lf, "hc-lane merger: 준비 훅의 기준·구성원을 못 읽었다 — 모름\n")
		return "", false
	}
	var mem []string
	for _, s := range strings.Split(subs, "\n") {
		if r, ok := parseSpec(s); ok {
			mem = append(mem, strings.Split(r.group, "+")...)
		}
	}
	c := shellCmd(wt, "sh", "-c", noIndexSh, "hc-merger", "bash", prepareHook)
	c.Stdout, c.Stderr = lf, lf
	c.Env = append(env[:len(env):len(env)], "HERDR_SPEC_BASE="+base, "HERDR_SPEC_BASE_SHA="+bs, "HERDR_SPEC_MEMBERS="+strings.Join(mem, " "))
	if rc := proc.WaitMax(c, "", setupMax()); rc != 0 {
		fmt.Fprintf(lf, "hc-lane merger: 추측 준비 훅(%s) 실패(종료 %d) — 모름\n", prepareHook, rc)
		return "", false
	}
	return wtState(wt, lf)
}

// wtState: 추적 파일의 작업 트리 상태 — 깨끗하면 "", 아니면 그 상태의 트리(git stash create — 아무것도 바꾸지 않고 객체만 쓴다)
func wtState(wt string, lf *os.File) (string, bool) {
	sc, rc := gitx.Out(wt, lf, "stash", "create")
	if rc != 0 {
		return "", false
	}
	if sc == "" {
		return "", true
	}
	t, rc := gitx.Out(wt, lf, "rev-parse", sc+"^{tree}")
	return t, rc == 0
}

// specEvidence: 추측 실패의 근거 — 검증 종료 코드와, 시험 실패로 가를 수 있으면 그 첫 실패 시험 줄(가를 수 없으면 test 빈 값)
type specEvidence struct {
	rc   int
	test string
}

// shellCmd: 이 기계의 셸 환경에서 wt 안의 명령 — 일꾼은 hc run --on으로 떠 hc 코어 PATH(bun 등이 없다)를 물려받으니, 같은 기계에서
// hc-lane이 레인의 준비 단계를 돌리는 길(shx.ZX — zsh·~/.zshrc가 있으면 그걸 읽은 셸)을 그대로 쓴다(merger-journal-rounds B, 2026-11-23)
func shellCmd(wt string, args ...string) *exec.Cmd {
	c := exec.Command("sh", append([]string{"-c", shx.ZX, "hc-merger", wt, "1"}, args...)...)
	c.Dir = wt
	return c
}

// noIndexSh: 코드 색인 도구(codebase-memory-mcp)가 든 폴더를 PATH에서 빼고 exec — 준비 훅의 색인을 건너뛰게(곧 지울 폴더를 색인하면
// 검증과 CPU를 다툰다). ~/.zshrc가 PATH를 다시 짠 뒤에 빼야 해서 셸 안에서 한다(병합기 기계 쪽은 pathWithout과 같은 규칙)
const noIndexSh = `p=; IFS=:; set -f
for d in $PATH; do [ -f "$d/codebase-memory-mcp" ] && [ -x "$d/codebase-memory-mcp" ] && continue; p=${p:+$p:}$d; done
PATH=$p; export PATH; exec "$@"`

// specWrite: 결과를 원격에. spec 참조가 그대로일 때만
func specWrite(top string, r specRow, res specResult, ev specEvidence, lg string) int {
	ref := specRefs + r.id
	now, rc := lsRemote("origin", ref) // 잘린 줄은 모름(1) — 「병합기가 버렸다」로 읽지 않는다(재감사 R-H03과 같은 꼴, 작업 중 발견)
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
		// exit·test는 끝에 덧붙인다(옛 판 기록은 test가 없다 = 가를 수 없음). 시험 실패면 본문 = 첫 실패 시험 줄(specCulprit이 읽는다)
		test := 0
		if ev.test != "" {
			test = 1
		}
		msg := fmt.Sprintf("hc-merger-spec-fail id=%s machine=%s at=%d log=%s exit=%d test=%d", r.id, machine(), at, lg, ev.rc, test)
		if ev.test != "" {
			msg += "\n\n" + ev.test
		}
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

// specWriteBusy: 「바쁨」 — spec 참조를 그 기계·state=busy(본문 = 기다린 것 한 줄, 한 번 더 확인 메모가 있으면 그 뒤에)로 고쳐 쓴다.
// 참조가 그새 바뀌었으면(병합기가 버렸거나 다시 띄움) 남기지 않는다. 통과·실패 기록은 쓰지 않는다(반송·범인 판정에 안 쓰인다)
func specWriteBusy(r specRow, waited string) int {
	msg := specMsg(r.parent, r.group, machine(), r.at, "busy") + "\n\n"
	if note, again := specRecheckOf(&r); again {
		msg += note + "\n"
	}
	msg += waited
	bc, ok := commitTree("", r.tree, r.commit+"^", msg)
	if !ok {
		return 1
	}
	ref := specRefs + r.id
	if runO("push", "-q", "--force-with-lease="+ref+":"+r.commit, "origin", bc+":"+ref) != 0 {
		fmt.Println("hc-lane merger: 바쁨을 남기지 못했다(병합기가 이 추측을 그새 바꿨거나 원격 실패) — 병합기는 시간 넘김으로 안다")
		return 1
	}
	fmt.Printf("hc-lane merger: 추측 검증 바쁨 — %s\n", waited)
	return 0
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
