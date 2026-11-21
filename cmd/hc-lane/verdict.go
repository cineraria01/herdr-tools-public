package main

// 승인 착지 land --verdict <aid>(셸 lib/hc-lane/verdict.sh, 판정 파일 읽기는 watch.sh vload·vmark·vstate·vmyrun).
// 시작 표시와 기존 제출 기록은 승인 원본의 patch-id를 쓴다. 후보의 내용은 lverdictCheck에서 Git 트리로 대조한다.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/proc"
)

// 판정 파일 값(셸 vf va vr vs vv vp vb vo, 표시 vx·끝 vdn, 이 실행 vme)
var lvf, lva, lvr, lvs, lvv, lvb, lvx, lvdn, lvd0, lvme string

// lpatch: 두 커밋 사이 변경의 patch-id(--stable). 변경이 없으면 「-」
func lpatch(a, b string) (string, bool) {
	d, rc := gitOut("--literal-pathspecs", "diff", "-U0", "--binary", a, b, "--")
	if rc != 0 {
		return "", false
	}
	if d == "" {
		return "-", true
	}
	var o bytes.Buffer
	c := exec.Command("git", "patch-id", "--stable")
	c.Stdin, c.Stdout, c.Stderr = strings.NewReader(d+"\n"), &o, os.Stderr
	if code(proc.Run(c)) != 0 {
		return "", false
	}
	p := trim(o.String())
	if i := strings.IndexByte(p, ' '); i >= 0 { // ${p%% *}
		p = p[:i]
	}
	if p == "" {
		return "", false
	}
	return p, true
}

// lvload: 판정 지시를 읽는다. 0 읽음, 1 없음, 2 꼴 틀림
func lvload(lane, aid string) int {
	if lane+aid == "" || !idChars(lane+aid) {
		return 2
	}
	d := os.Getenv("HC_FILL_VERDICTS")
	if d == "" {
		d = os.Getenv("HOME") + "/.local/share/hc-fill/verdicts"
	}
	lvd0 = d + "/" + lane
	lvf, lvx, lvdn = lvd0+"/"+aid+".md", lvd0+"/"+aid+".ack", lvd0+"/"+aid+".done"
	st, err := os.Stat(lvf)
	if err != nil || !st.Mode().IsRegular() {
		return 1
	}
	b, err := os.ReadFile(lvf)
	if err != nil {
		return 2
	}
	get := func(k string) string {
		for _, l := range strings.Split(string(b), "\n") {
			f := strings.Split(l, "\t")
			if f[0] == k {
				if len(f) > 1 {
					return f[1]
				}
				return ""
			}
		}
		return ""
	}
	lva, lvr, lvs, lvv, lvb = get("aid"), get("run"), get("sha"), get("verdict"), get("base")
	if lva != aid || lvr == "" || len(lvs) != 40 || strings.Trim(lvs, "0123456789abcdef") != "" {
		return 2
	}
	if lvv != "pass" && lvv != "fix" {
		return 2
	}
	vb := lvb
	if vb == "" {
		vb = "main"
	}
	if strings.Trim(vb, alnum+"_./-") != "" {
		return 2
	}
	return 0
}

func idChars(s string) bool { return strings.Trim(s, alnum+"_-") == "" }

// lvmyrun: 이 터미널의 실행 ID(HC_LANE_JOB, 없으면 <터미널>@<hc get created_ms>). 모르면 false
func lvmyrun() bool {
	if lvme = os.Getenv("HC_LANE_JOB"); lvme != "" {
		return true
	}
	id := os.Getenv("HC_TERMINAL_ID")
	if id == "" {
		return false
	}
	c, ok := createdMs(id, 10*time.Second)
	if !ok {
		return false
	}
	lvme = id[strings.LastIndex(id, "/")+1:] + "@" + c
	return true
}

// createdMs: hc get <주소>의 맨 위 터미널이 살아 있으면 created_ms(0 이상 정수)
func createdMs(addr string, limit time.Duration) (string, bool) {
	if addr == "" {
		return "", false
	}
	var o bytes.Buffer
	c := exec.Command("hc", "get", addr, "--json")
	c.Stdout = &o
	if err := c.Start(); err != nil {
		return "", false
	}
	t := time.AfterFunc(limit, func() { c.Process.Kill() })
	err := c.Wait()
	t.Stop()
	if err != nil {
		return "", false
	}
	var v struct {
		State     string       `json:"state"`
		CreatedMs *json.Number `json:"created_ms"`
	}
	dec := json.NewDecoder(&o)
	dec.UseNumber()
	if dec.Decode(&v) != nil || v.State != "live" || v.CreatedMs == nil {
		return "", false
	}
	f, err := v.CreatedMs.Float64()
	if err != nil || f < 0 || f != math.Floor(f) {
		return "", false
	}
	if i, err := v.CreatedMs.Int64(); err == nil {
		return strconv.FormatInt(i, 10), true
	}
	return strconv.FormatFloat(f, 'f', -1, 64), true
}

// lvmark: 표시를 배타적으로 만든다. 0 만듦, 1 이미 있음, 2 실패
func lvmark(s string) int {
	f, err := os.CreateTemp(lvd0, ".ack.")
	if err != nil {
		return 2
	}
	t := f.Name()
	_, err = f.WriteString(s + "\n")
	f.Close()
	if err != nil {
		os.Remove(t)
		return 2
	}
	err = os.Link(t, lvx)
	os.Remove(t)
	if err == nil {
		return 0
	}
	if _, e := os.Stat(lvx); e == nil {
		return 1
	}
	return 2
}

// lvstate: none|taken|landing|void (표시 첫 줄 첫 칸)
func lvstate() string {
	b, err := os.ReadFile(lvx)
	if err != nil {
		if _, e := os.Lstat(lvx); e != nil {
			return "none"
		}
		return ""
	}
	return cutF(strings.SplitN(string(b), "\n", 2)[0], 1)
}

// cutF: cut -f<n>(탭이 없는 줄은 줄 그대로)
func cutF(l string, n int) string {
	if !strings.Contains(l, "\t") {
		return l
	}
	f := strings.Split(l, "\t")
	if n <= len(f) {
		return f[n-1]
	}
	return ""
}

// cutFile: cut -f<n> <파일> — 줄마다
func cutFile(p string, n int) string {
	b, _ := os.ReadFile(p)
	var o []string
	for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		o = append(o, cutF(l, n))
	}
	return strings.Join(o, "\n")
}

func lverdictStart() {
	lane := os.Getenv("HC_LANE")
	if lane == "" {
		die("land --verdict: 레인 터미널 안에서 부른다($HC_LANE 없음)")
	}
	switch lvload(lane, lvd) {
	case 1:
		die("land --verdict: 판정 %s 가 없다(%s)", lvd, lvf)
	case 2:
		die("land --verdict: 판정 파일 꼴이 틀렸다: %s", lvf)
	}
	if lvv != "pass" {
		eprintf("hc-lane land: 판정 %s 는 통과가 아니다(%s) — 착지하지 않았다\n", lvd, lvv)
		exit(2)
	}
	vb := lvb
	if vb == "" {
		vb = "main"
	}
	if vb != base {
		eprintf("hc-lane land: 판정 %s 의 기본 브랜치는 %s 이다\n", lvd, vb)
		exit(2)
	}
	if !lvmyrun() {
		die("land --verdict: 이 터미널의 실행을 못 읽었다($HC_LANE_JOB·hc get) — 착지하지 않았다, 같은 줄을 다시")
	}
	if lvr != lvme {
		eprintf("hc-lane land: 판정 %s 는 다른 실행(%s)의 것 — 착지하지 않았다\n", lvd, lvr)
		exit(2)
	}
	if _, err := os.Stat(lvdn); err == nil {
		fmt.Printf("hc-lane land: 판정 %s 는 이미 착지했다(%s)\n", lvd, cutLines(cutFile(lvdn, 2), 8))
		exit(0)
	}
	st := lvstate()
	if st == "none" {
		h, rc := gitOut("rev-parse", "--verify", "-q", "HEAD")
		if rc != 0 {
			die("land --verdict: HEAD를 읽지 못했다(git 실패) — 착지하지 않았다, 같은 줄을 다시")
		}
		if h != lvs {
			eprintf("hc-lane land: 승인 커밋이 아니다 — 판정 %s %s, HEAD %s. 착지하지 않았다. 지금 커밋으로 hc-lane report 완료 %s 를 부르고 기다린다\n", lvd, cut(lvs, 8), cut(h, 8), cut(h, 8))
			exit(2)
		}
		mb, rc := gitOut("merge-base", lvs, bref)
		if rc != 0 {
			die("land --verdict: %s 와의 갈림점을 못 찾았다 — 착지하지 않았다", blab)
		}
		var ok bool
		if lpid, ok = lpatch(mb, lvs); !ok {
			die("land --verdict: 승인 변경의 patch-id를 못 구했다(git 실패) — 착지하지 않았다, 같은 줄을 다시")
		}
		if lvmark("landing\t"+utc()+"\t"+lvme+"\t"+lpid) == 2 {
			die("land --verdict: 시작 표시를 두지 못했다: %s", lvx)
		}
		st = lvstate()
	}
	switch st {
	case "landing":
		if cutFile(lvx, 3) != lvme {
			eprintf("hc-lane land: 판정 %s 착지를 다른 실행이 시작했다 — 착지하지 않았다\n", lvd)
			exit(2)
		}
		if lpid = cutFile(lvx, 4); lpid == "" {
			die("land --verdict: 시작 표시에 patch-id가 없다: %s", lvx)
		}
		fmt.Printf("hc-lane land: 판정 %s(%s) 승인 착지 — 변경 patch-id %s\n", lvd, cut(lvs, 8), cut(lpid, 12))
	case "void":
		eprintf("hc-lane land: 판정 %s 는 hc-fill이 취소했다 — 착지하지 않았다. 다음 지시를 기다린다\n", lvd)
		exit(2)
	default:
		eprintf("hc-lane land: 판정 %s 표시가 착지 시작이 아니다(%s) — 착지하지 않았다\n", lvd, st)
		exit(2)
	}
}

// cutLines: cut -c 1-n(줄마다)
func cutLines(s string, n int) string {
	ls := strings.Split(s, "\n")
	for i, l := range ls {
		ls[i] = cut(l, n)
	}
	return strings.Join(ls, "\n")
}

// lverdictBeforeRebase: 바뀐 경로의 예외는 이 승인 원본에서 rerere를 실제로 거친 결과(모드와 blob)에만 준다
func lverdictBeforeRebase(onto string) {
	h, rc := gitOut("rev-parse", "--verify", "HEAD")
	if rc != 0 {
		die("land: rebase 전 HEAD 조회 실패")
	}
	if h == lorig {
		return
	}
	b, rc := gitOut("merge-base", "HEAD", onto)
	if rc != 0 {
		die("land: rebase 전 갈래점 조회 실패")
	}
	lverdictCheck(b)
}

func lverdictEntry(path, rev string) (string, int) {
	return gitOut("--literal-pathspecs", "ls-tree", "--format=%(objectmode) %(objectname)", rev, "--", path)
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func lverdictResolved(paths []string, onto string) {
	for _, p := range paths {
		e, rc := lverdictEntry(p, "HEAD")
		var in string
		if rc == 0 {
			in, rc = lverdictEntry(p, onto)
		}
		if rc == 0 {
			f, err := os.OpenFile(lrrf, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o666)
			if err == nil {
				_, err = fmt.Fprintf(f, "%s\t%s\t%s\t%s\t%s\n", lorig, lpid, dash(e), p, dash(in))
				if cerr := f.Close(); err == nil {
					err = cerr
				}
			}
			if err == nil {
				continue
			}
		}
		die("land: rerere 결과를 기록하지 못했다: %s", p)
	}
}

// lverdictCheck: 현재 기준과 승인 원본을 합친 트리를 후보와 견준다. 다른 경로가 모두 rerere가 이 승인 착지에서 실제로 풀어
// 색인에 올린 경로면 통과하고 lrrn에 그 수. 그 밖의 차이는 종료 2
func lverdictCheck(bref string) {
	bc, rc := gitOut("rev-parse", "--verify", bref+"^{commit}")
	if rc != 0 {
		die("land: 승인 대조 기준 조회 실패")
	}
	merged, rc := gitOut("-c", "core.quotePath=false", "merge-tree", "--write-tree", "--name-only", "--no-messages", "--merge-base="+lmb, bc, lorig)
	conflicts := ""
	switch rc {
	case 0:
	case 1:
		i := strings.IndexByte(merged, '\n')
		if i < 0 {
			die("land: 승인 원본의 충돌 경로를 읽지 못했다")
		}
		conflicts = merged[i+1:]
	default:
		die("land: 승인 원본을 현재 기준에 병합한 트리를 구하지 못했다")
	}
	tree := strings.SplitN(merged, "\n", 2)[0]
	changed, rc := gitOut("-c", "core.quotePath=false", "diff-tree", "-r", "--no-renames", "--ignore-submodules=none", "--name-only", tree, "HEAD", "--")
	if rc != 0 {
		die("land: 승인 원본의 병합 트리와 후보의 경로를 대조하지 못했다")
	}
	if changed+conflicts == "" {
		return
	}
	set := map[string]bool{}
	for _, p := range strings.Split(changed+"\n"+conflicts, "\n") {
		set[p] = true
	}
	var paths []string
	for p := range set {
		paths = append(paths, p)
	}
	sort.Strings(paths) // LC_ALL=C sort -u
	rec, recErr := os.ReadFile(lrrf)
	var bad []string
	lrrn = 0
	for _, p := range paths {
		if p == "" {
			continue
		}
		e, rc := lverdictEntry(p, "HEAD")
		var in string
		if rc == 0 {
			in, rc = lverdictEntry(p, bc)
		}
		if rc != 0 {
			die("land: rerere 경로 조회 실패: %s", p)
		}
		// 마지막으로 맞는 기록 줄의 결과(awk가 끝까지 읽고 x를 덮어쓴다)
		x := false
		if recErr == nil {
			sc := bufio.NewScanner(bytes.NewReader(rec))
			sc.Buffer(make([]byte, 1<<20), 1<<20)
			for sc.Scan() {
				f := strings.Split(sc.Text(), "\t")
				if len(f) >= 5 && f[0] == lorig && f[1] == lpid && f[3] == p && f[4] == dash(in) {
					x = f[2] == dash(e)
				}
			}
		}
		if x {
			lrrn++
		} else {
			bad = append(bad, p)
		}
	}
	if len(bad) == 0 {
		fmt.Printf("hc-lane land: rerere로 바뀐 경로 %d개 — 승인 변경과 다른 곳이 rerere가 푼 경로뿐이라 이어 간다\n", lrrn)
		return
	}
	s, _ := gitOut("rev-parse", "--short=8", "HEAD")
	eprintf("hc-lane land: rebase 뒤 변경이 승인 내용과 다르다(rerere가 푼 곳 밖 — %s). push하지 않았다. 지금 커밋으로 hc-lane report 완료 %s 를 부르고 기다린다(새 감사)\n", strings.Join(bad, ", "), s)
	exit(2)
}

// lverdictPrecheck: 위임 경로 — 위임 전에 같은 기준으로 rebase해 차이가 rebase뿐인지(충돌 종료 3, 다르면 종료 2)
func lverdictPrecheck() {
	if !tclean("승인 rebase 전") {
		die("land: 커밋 안 된 변경이나 멈춘 rebase가 있다(git status). 커밋하거나 rebase를 마친 뒤 같은 줄을 다시")
	}
	if remote && fetchr(os.Stdout, os.Stderr, "fetch", "-q", "origin") != 0 {
		die("land: git fetch 실패 — 착지하지 않았다")
	}
	if rb(bref) != 0 {
		eprintf("hc-lane land: 승인 착지 중 rebase 충돌 — 양쪽 내용을 살려 풀고 git add → git rebase --continue 뒤 커밋으로 hc-lane report 완료 <sha>(새 감사)\n")
		exit(3)
	}
	lverdictCheck(bref)
	if lvd != "" {
		lverdictSave("HEAD", "hook")
	}
}

func lverdictSave(rev, kind string) {
	sha, rc := gitOut("rev-parse", "--verify", rev+"^{commit}")
	if rc == 0 {
		if os.WriteFile(lvdn+".candidate.tmp", []byte(sha+" "+kind+"\n"), 0o666) == nil && os.Rename(lvdn+".candidate.tmp", lvdn+".candidate") == nil {
			return
		}
	}
	die("land: 판정 %s 의 검증 후보를 기록하지 못했다", lvd)
}

func lverdictRecover() {
	candidate, kind, marker := "", "", ""
	ref := "refs/hc-verdict/" + os.Getenv("HC_LANE") + "/" + lvd
	if st, err := os.Stat(lvdn + ".candidate"); err == nil && st.Mode().IsRegular() {
		b, err := os.ReadFile(lvdn + ".candidate")
		if err != nil || !bytes.Contains(b, []byte("\n")) { // read -r는 끝 줄바꿈이 없으면 실패
			die("land: 판정 %s 의 후보 기록 읽기 실패", lvd)
		}
		f := strings.Fields(strings.SplitN(string(b), "\n", 2)[0])
		if len(f) > 0 {
			candidate = f[0]
		}
		if len(f) > 1 {
			kind = strings.Join(f[1:], " ")
		}
		if kind == "hook" && remote {
			refs, rc := gitOut("ls-remote", "origin", ref)
			if rc != 0 {
				die("land: 전용 훅의 착지 표식 조회 실패")
			}
			marker = field2(refs, ref)
			if marker != "" && fetchr(os.Stdout, os.Stderr, "fetch", "-q", "origin") != 0 {
				die("land: 전용 훅 착지 뒤 git fetch 실패")
			}
		}
	}
	for _, sha := range []string{marker, candidate, lvs} {
		if sha == "" {
			continue
		}
		switch gitRun("merge-base", "--is-ancestor", sha, bref) {
		case 0:
			fmt.Printf("hc-lane land: 판정 %s 의 커밋 %s 이 이미 %s 에 있다\n", lvd, cut(sha, 8), blab)
			if remote {
				if rc := landKnown(sha); rc != 0 {
					exit(rc)
				}
			}
			lverdictDone(sha)
			if remote {
				ffmain(mainwt(), base)
			}
			exit(0)
		case 1:
		default:
			die("land: 판정 %s 의 착지 여부 조회 실패", lvd)
		}
	}
}

// lverdictDone: 착지 끝 표시(hc-fill이 본다). 못 쓰면 알림만 — 착지는 됐고 착지 사건도 남는다
func lverdictDone(sha string) {
	if lvd == "" {
		return
	}
	if sha == "" {
		sha, _ = gitOut("rev-parse", "--verify", "HEAD")
	}
	if sha == "" {
		die("land: 끝 표시에 쓸 커밋 조회 실패")
	}
	t := fmt.Sprintf("%s.tmp.%d", lvdn, os.Getpid())
	if os.WriteFile(t, []byte("landed\t"+sha+"\t"+utc()+"\n"), 0o666) != nil || os.Rename(t, lvdn) != nil {
		eprintf("hc-lane land: 착지는 됐는데 끝 표시를 못 썼다: %s — hc-lane report 착지 를 꼭 부른다\n", lvdn)
	}
}
