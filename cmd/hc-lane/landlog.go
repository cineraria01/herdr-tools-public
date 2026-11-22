package main

// landlog.go: 착지 로그·실패 알림·rebase 도우미(land.go에서 나눔 — 줄 예산 600)

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ---- 착지 로그: 검증(또는 herdr-land.sh) 출력을 화면 그대로 + 상태 폴더 land-logs/<레인>-<시각>-<pid>.log ----

func llogInit() {
	ld := evdir() + "/land-logs"
	if os.MkdirAll(ld, 0o777) != nil {
		llog = ""
		return
	}
	// ponytail: 14일 지난 것만 지움(find -mtime +14 = 만 15일 이상), 더 오래 둘 일이 생기면 설정으로
	cutoff := time.Now().Add(-15 * 24 * time.Hour)
	filepath.WalkDir(ld, func(p string, d fs.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(d.Name(), ".log") {
			if st, e := os.Lstat(p); e == nil && !st.ModTime().After(cutoff) {
				os.Remove(p)
			}
		}
		return nil
	})
	ln := os.Getenv("HC_LANE")
	if ln == "" {
		ln, _ = gitOutQ("symbolic-ref", "-q", "--short", "HEAD")
	}
	if ln == "" {
		ln = "land"
	}
	llog = fmt.Sprintf("%s/%s-%s-%d.log", ld, trTo(ln, alnum+"._-"), time.Now().Format("20060102-150405"), os.Getpid())
	if f, err := os.Create(llog); err != nil {
		llog = ""
	} else {
		f.Close()
	}
}

// qadirs: .qa/*/herdr-verify-* 폴더(정렬)
func qadirs() []string {
	m, _ := filepath.Glob(".qa/*/herdr-verify-*")
	var o []string
	for _, p := range m {
		if st, err := os.Lstat(p); err == nil && st.IsDir() {
			o = append(o, p)
		}
	}
	sort.Strings(o)
	return o
}

func appendLog(s string) {
	if f, err := os.OpenFile(llog, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o666); err == nil {
		f.WriteString(s)
		f.Close()
	}
}

// lrun: 명령 출력(오류 출력 포함)을 화면(표준 출력)과 로그에. 종료 코드는 명령 것
func lrun(argv ...string) int {
	return lrunF(strings.Join(argv, " "), func(out, errw io.Writer) int {
		return runCmd(cmdOpt{stdout: out, stderr: errw, stdin: os.Stdin}, argv[0], argv[1:]...)
	})
}

// lrunF: lrun의 속 — 함수(fetchr 등)도 받는다. desc는 로그 머리 줄의 명령
func lrunF(desc string, fn func(out, errw io.Writer) int) int {
	if llog == "" {
		return fn(os.Stdout, os.Stderr)
	}
	h, _ := gitOutQ("rev-parse", "--short", "HEAD")
	appendLog(fmt.Sprintf("=== %s HEAD %s: %s\n", time.Now().Format("2006-01-02 15:04:05"), h, desc))
	f, err := os.OpenFile(llog, os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		return fn(os.Stdout, os.Stderr)
	}
	defer f.Close()
	w := &syncWriter{w: io.MultiWriter(os.Stdout, f)}
	return fn(w, w)
}

// syncWriter: 표준 출력과 오류 출력을 한 쓰개로(exec가 같은 쓰개면 한 파이프로 묶는다 — 차례가 섞이지 않게)
type syncWriter struct{ w io.Writer }

func (s *syncWriter) Write(p []byte) (int, error) { return s.w.Write(p) }

var failLine = regexp.MustCompile(`--- FAIL:|^FAIL[[:space:]]+[^[:space:]]|panic:|\(fail\)|✗|✖|not ok`)

// lfail: 실패면 실패 줄(최대 40)·이번에 생긴 .qa/*/herdr-verify-* 폴더·로그 경로, 레인 안이면 「중간」 사건에도
func lfail(rc int) {
	if llog == "" {
		return
	}
	old := map[string]bool{}
	for _, l := range lines(qa0) {
		old[l] = true
	}
	var qn []string
	for _, d := range qadirs() {
		if !old[d] {
			qn = append(qn, d)
		}
	}
	eprintf("hc-lane land: 실패 줄(로그에서, 최대 40줄):\n")
	// 줄 단위로 읽고 서로 다른 실패 40줄을 찾으면 멈춘다(로그 전체를 한 버퍼에 올리지 않는다, gofix L14)
	seen, n := map[string]bool{}, 0
	pick := func(l string) bool {
		loc := failLine.FindStringIndex(l)
		if loc == nil {
			return true
		}
		if len(l) > 400 { // 긴 줄(조각)은 맞은 자리 앞뒤 200바이트만 보인다(R-28)
			l = strings.ToValidUTF8(l[max(0, loc[0]-200):min(len(l), loc[1]+200)], "")
		}
		if !seen[l] {
			seen[l] = true
			eprintf("%s\n", l)
			n++
		}
		return n < 40
	}
	eachLine(llog, pick)
	for _, d := range qn {
		filepath.WalkDir(d, func(p string, e fs.DirEntry, err error) error {
			if n >= 40 {
				return filepath.SkipAll
			}
			if err == nil && e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".log") {
				eachLine(p, pick)
			}
			return nil
		})
	}
	wd := os.Getenv("PWD")
	for _, d := range qn {
		eprintf("hc-lane land: 검증 로그 폴더 %s/%s\n", wd, strings.TrimPrefix(d, "./"))
	}
	eprintf("hc-lane land: 전체 로그 %s\n", llog)
	if os.Getenv("HC_LANE") != "" {
		reportCmd(nil, nil, "중간", "-", fmt.Sprintf("land 실패(종료 %d) — 전체 로그 %s", rc, llog))
	}
}

// lerr: 오류 출력과 착지 로그에
func lerr(s string) {
	if s == "" {
		return
	}
	eprintf("%s\n", s)
	if llog != "" {
		appendLog(s + "\n")
	}
}

// mainwt: 메인 폴더(첫 작업 트리)
func mainwt() string {
	wl, _ := gitOut("worktree", "list", "--porcelain")
	if l := strings.SplitN(wl, "\n", 2)[0]; strings.HasPrefix(l, "worktree ") {
		return l[len("worktree "):]
	}
	return ""
}

// ---- rebase(rerere 이어 가기 포함) ----

var (
	conflictRe = regexp.MustCompile(`^CONFLICT \([^)]*\): Merge conflict in (.*)$`)
	resolvedRe = regexp.MustCompile(`^Resolved '(.*)' using previous resolution\.$`)
	stagedRe   = regexp.MustCompile(`^Staged '(.*)' using previous resolution\.$`)
)

// rrPaths: LC_ALL=C rebase 출력의 내용 충돌 경로
func rrPaths(out string) []string {
	var o []string
	for _, l := range strings.Split(out, "\n") {
		if m := conflictRe.FindStringSubmatch(l); m != nil {
			o = append(o, m[1])
		}
	}
	return o
}

// rrOK: 멈춘 rebase를 rerere가 다 풀었나 — 커밋 적용 중 멈췄고, 충돌 줄이 모두 내용 충돌이며, 그 경로가 모두 rerere로 풀렸고,
// 미병합·rerere remaining이 비었을 때만
func rrOK(out string) bool {
	if runCmd(cmdOpt{stderr: os.Stderr}, "git", "rev-parse", "-q", "--verify", "REBASE_HEAD") != 0 {
		return false
	}
	ls := strings.Split(out, "\n")
	for _, l := range ls {
		if m := resolvedRe.FindStringSubmatch(l); m != nil {
			if gitRun("add", "--", m[1]) != 0 {
				return false
			}
		}
	}
	rp := rrPaths(out)
	nc := 0
	for _, l := range ls {
		if strings.HasPrefix(l, "CONFLICT ") {
			nc++
		}
	}
	if len(rp) == 0 || nc != countNonEmpty(strings.Join(rp, "\n")) {
		return false
	}
	ok := map[string]bool{}
	for _, l := range ls {
		if m := stagedRe.FindStringSubmatch(l); m != nil {
			ok[m[1]] = true
		} else if m := resolvedRe.FindStringSubmatch(l); m != nil {
			ok[m[1]] = true
		}
	}
	for _, p := range rp {
		if !ok[p] {
			return false
		}
	}
	u, rc := gitOut("diff", "--name-only", "--diff-filter=U")
	if rc != 0 || u != "" {
		return false
	}
	u, rc = gitOut("rerere", "remaining")
	return rc == 0 && u == ""
}

// rebaseMergeFlag: 병합 커밋은 기본 rebase에서 사라진다. fast-forward면 원본을 쓰고(keep), 새 기준에는 자동으로 재현할 수 있는
// 병합만 다시 만든다(--rebase-merges). 종료 75 다시, 2 풀이를 재현 못 함
func rebaseMergeFlag(onto string) (string, int) {
	merges, rc := gitOut("rev-list", "--merges", onto+"..HEAD")
	if rc != 0 {
		return "", 75
	}
	if merges == "" {
		return "", 0
	}
	switch gitRun("merge-base", "--is-ancestor", onto, "HEAD") {
	case 0:
		return "keep", 0
	case 1:
	default:
		return "", 75
	}
	for _, m := range strings.Fields(merges) {
		d, rc := gitOut("show", "--remerge-diff", "--no-ext-diff", "--format=%P", "--name-only", m)
		if rc != 0 {
			return "", 75
		}
		// 부모가 셋 이상이거나, 두 부모를 다시 합친 트리와 다른 경로가 있으면 원래 풀이를 잃을 수 있다
		if strings.Count(d, " ") >= 2 || strings.Contains(d, "\n") {
			lerr(fmt.Sprintf("hc-lane land: 병합 커밋 %s의 풀이를 자동으로 재현할 수 없다. 원본을 보존했다. 새 기준에서 풀이를 확인한 뒤 다시 제출한다", cut(m, 9)))
			return "", 2
		}
	}
	return "--rebase-merges", 0
}

// rb: 승인 대조와 rerere는 같은 기준 커밋에서 수행한다. 종료 코드는 rebase 것
func rb(onto string) int {
	var resolved []string
	if lpid != "" {
		o, rc := gitOut("rev-parse", "--verify", onto+"^{commit}")
		if rc != 0 {
			die("land: 승인 rebase 기준 조회 실패")
		}
		onto = o
		lverdictBeforeRebase(onto)
	}
	flag, rc := rebaseMergeFlag(onto)
	if rc != 0 {
		exit(rc)
	}
	if flag == "keep" {
		return 0
	}
	args := []string{"rebase", "-q", "--reapply-cherry-picks"}
	if flag != "" {
		args = append(args, flag)
	}
	ro, rr := gitWrite2([]string{"LC_ALL=C"}, append(args, onto)...)
	lerr(ro)
	for rr != 0 && rrOK(ro) {
		lerr(fmt.Sprintf("hc-lane land: rerere가 기록한 풀이로 충돌을 다 풀었다(%s) — rebase를 이어 간다", strings.Join(rrPaths(ro), ",")))
		if lpid != "" {
			resolved = append(resolved, rrPaths(ro)...)
		}
		ro, rr = gitWrite2([]string{"LC_ALL=C", "GIT_EDITOR=true"}, "rebase", "--continue")
		lerr(ro)
	}
	// 모든 착지 길에서: 색인 잠금(index.lock)으로 rebase가 시작도 못 했으면(미해결 없음·멈춘 rebase 없음) 다시 할 수 있는 75,
	// 미해결 항목·멈춘 rebase 조회 불능도 75(R-26). 훅 거절 같은 다른 중단은 다시 해도 같으니 3 그대로. 병합기 안은 미해결이 없으면 늘 75
	if rr != 0 {
		un, rc := gitOut("ls-files", "--unmerged")
		if rc != 0 {
			lerr("hc-lane land: rebase 실패. 미해결 항목을 확인하지 못했다(git ls-files 실패) — 같은 줄을 다시")
			exit(75)
		}
		if un == "" {
			if mgx == "1" {
				lerr("hc-lane land: rebase 실패. 미해결 충돌이 확인되지 않아 큐에서 다시 시도한다")
				exit(75)
			}
			switch rebaseStopped() {
			case 1:
				if strings.Contains(ro, "index.lock") {
					lerr("hc-lane land: rebase가 시작되지 못했다(내용 충돌 아님 — 색인 잠금). 같은 줄을 다시")
					exit(75)
				}
			case 2:
				lerr("hc-lane land: rebase 실패. 멈춘 rebase가 있는지 확인하지 못했다(git rev-parse 실패) — 같은 줄을 다시")
				exit(75)
			}
		}
	}
	if rr == 0 && len(resolved) > 0 {
		lverdictResolved(resolved, onto)
	}
	return rr
}

// rebaseStopped: 0 멈춘 rebase가 있음(rebase-merge·rebase-apply), 1 없음, 2 모름(git·파일 조회 실패)
func rebaseStopped() int {
	for _, f := range []string{"rebase-merge", "rebase-apply"} {
		gp, r := gitOut("rev-parse", "--git-path", f)
		if r != 0 || gp == "" {
			return 2
		}
		if _, err := os.Lstat(gp); err == nil {
			return 0
		} else if !os.IsNotExist(err) {
			return 2
		}
	}
	return 1
}
