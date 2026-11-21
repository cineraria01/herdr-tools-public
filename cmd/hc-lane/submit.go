package main

// 제출(셸 lib/hc-lane/merger.sh submit_cmd, merger-result.sh saccepted·srecord·mmeta): HEAD를 refs/heads/lane/<레인>과 큐 표식
// refs/merge-queue/<UTC초>-<레인>에 한 번의 원자 push로(같은 레인의 옛 표식은 같은 push에서 지움). 기본 브랜치는 건드리지 않는다.
// 결과를 모르면(전송 오류) 원격을 다시 읽어 판정한다. 종료 0 제출(또는 이미 들어감), 1 실패, 2 판정과 다름

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 제출 기록 값(셸 mmeta가 채우는 mbs_ mid_ mjb_ mvd_ mpid_)
var mbs, mid, mjb, mvd, mpidv string

// mmeta: 제출 기록 커밋 rec(부모 = 원본 sub)의 메시지를 읽는다. rec가 비었거나 「-」면 빈 값으로 참
func mmeta(rec, sub string) bool {
	mbs, mid, mjb, mvd, mpidv = "", "", "", "", ""
	if rec == "" || rec == "-" {
		return true
	}
	if mp, rc := gitOutQ("rev-parse", "-q", "--verify", rec+"^"); rc != 0 || mp != sub {
		return false
	}
	body, rc := gitOutQ("log", "-1", "--format=%B", rec)
	if rc != 0 {
		return false
	}
	first := strings.SplitN(body, "\n", 2)[0]
	var ids, jobs []string
	for _, kv := range strings.Fields(first) {
		k, v, _ := strings.Cut(kv, "=")
		switch k {
		case "base":
			mbs = v
		case "verdict":
			mvd = v
		case "patch":
			mpidv = v
		}
		if strings.HasPrefix(kv, "id=") {
			ids = append(ids, v)
		}
		if strings.HasPrefix(kv, "job=") {
			jobs = append(jobs, v)
		}
	}
	if !idChars(mvd) {
		return false
	}
	if mvd+mpidv != "" {
		if mvd == "" || !(mpidv == "-" || sha40.MatchString(mpidv)) {
			return false
		}
	}
	mid, mjb = strings.Join(ids, "\n"), strings.Join(jobs, "\n")
	// mmeta_refs: 제출 ID 표식은 필수다. 작업 ID는 이름이 유효하지 않을 때만 건너뛴다
	if mid != "" && gitRunO("check-ref-format", "refs/hc-landed/"+mid) != 0 {
		return false
	}
	if mjb != "" && mjb != "-" {
		if rc := gitRunO("check-ref-format", "refs/hc-landed/"+mjb); rc != 0 && rc != 1 {
			return false
		}
	}
	return true
}

var sha40 = regexp.MustCompile(`^[0-9a-f]{40}$`)

const precheck = ".claude/herdr-precheck.sh"

// 제출 상태(셸 ln_ sd_ pending_)
var sln, ssd, spending string

// saccepted: 제출 기록 커밋 mc가 큐 기록 자리·넣음(merge-done)·돌려보냄(merge-failed-meta) 가운데 하나에 있으면 접수
func saccepted(qn, mc string) bool {
	q, d, f := "refs/merge-queue-meta/"+qn, "refs/merge-done/"+sln, "refs/merge-failed-meta/"+qn
	refs, rc := gitOut("ls-remote", "origin", q, d, f)
	if rc != 0 {
		die("land(제출): 원격 접수를 확인하지 못했다(git ls-remote) — 같은 줄을 다시")
	}
	for _, l := range lines(refs) {
		x := strings.Fields(l)
		if len(x) >= 2 && (x[1] == q || x[1] == d || x[1] == f) && x[0] == mc {
			return true
		}
	}
	return false
}

// srecord: 접수된 제출을 이 기계 기록(hc-submitted)·제출 사건에 남기고 접수 전 기록을 지운다
func srecord(qn, mc, sha string) {
	if !mmeta(mc, sha) || mid == "" {
		die("land(제출): 접수된 제출 기록을 읽지 못했다")
	}
	line, f := sln+" "+sha+" "+mbs, ssd+"/hc-submitted"
	have := false
	if b, err := os.ReadFile(f); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if l == line {
				have = true
				break
			}
		}
	}
	if !have {
		w, err := os.OpenFile(f, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o666)
		if err == nil {
			_, err = w.WriteString(line + "\n")
			if cerr := w.Close(); err == nil {
				err = cerr
			}
		}
		if err != nil {
			die("land(제출): hc-submitted 쓰기 실패 — 같은 줄을 다시")
		}
	}
	if reportCmd(nil, os.Stderr, "제출", sha, mid+" → refs/merge-queue/"+qn+" (병합기 "+mg+" 가 "+mbs+" 에 넣는다)", "--name", sln, "--key", "submit:"+mid) != 0 {
		die("land(제출): 제출 사건 쓰기 실패 — 같은 줄을 다시")
	}
	if err := os.Remove(spending); err != nil && !os.IsNotExist(err) {
		die("land(제출): 접수 전 기록을 지우지 못했다")
	}
}

// lprio: 제출 우선(HC_LANE_PRIORITY). 없으면 보통, 모르는 값은 보통과 경고
func lprio() string {
	switch p := os.Getenv("HC_LANE_PRIORITY"); p {
	case "", "보통", "normal":
		return "보통"
	case "긴급", "urgent":
		return "긴급"
	case "높음", "high":
		return "높음"
	case "낮음", "low":
		return "낮음"
	default:
		eprintf("hc-lane land: 경고 — HC_LANE_PRIORITY는 긴급·높음·보통·낮음 가운데 하나: %s (보통으로 낸다)\n", p)
		return "보통"
	}
}

func mreserved(l string) bool {
	switch l {
	case base, "merge-batch", "batch", "main", "master":
		return true
	}
	return false
}

// readLine3: read -r a b c <파일 — 끝 줄바꿈이 없으면 실패
func readLine3(p string) (a, b, c string, ok bool) {
	d, err := os.ReadFile(p)
	if err != nil || !bytes.Contains(d, []byte("\n")) {
		return "", "", "", false
	}
	f := strings.Fields(strings.SplitN(string(d), "\n", 2)[0])
	for i := range f {
		switch i {
		case 0:
			a = f[0]
		case 1:
			b = f[1]
		default:
			c = strings.Join(f[2:], " ")
		}
	}
	return a, b, c, true
}

func hostShort() string {
	o, err := exec.Command("hostname", "-s").Output()
	if err != nil {
		return ""
	}
	return trim(string(o))
}

func submitCmd() {
	llogInit()
	sln = os.Getenv("HC_LANE")
	if sln == "" {
		sln, _ = gitOutQ("symbolic-ref", "-q", "--short", "HEAD")
	}
	if !lnameOK(sln) {
		n := sln
		if n == "" {
			n = "(분리된 HEAD)"
		}
		die("land(제출): 레인 이름은 a-z 0-9 - _ 32자까지여야 큐에 넣는다: %s", n)
	}
	if mreserved(sln) {
		die("land(제출): 레인 이름 %s 은 병합기가 쓰는 가지 이름이라 큐에 넣지 않는다 — 레인 작업 트리에서 HC_LANE=<레인>으로", sln)
	}
	if gitRunQ("remote", "get-url", "origin") != 0 {
		die("land(제출): origin이 없는 저장소는 병합기로 넣지 않는다")
	}
	for _, f := range []string{"rebase-merge", "rebase-apply", "MERGE_HEAD", "CHERRY_PICK_HEAD"} {
		gp, rc := gitOut("rev-parse", "--git-path", f)
		if rc != 0 || gp == "" {
			die("land(제출): git rev-parse --git-path 실패 — 진행 중인 rebase·merge가 있는지 모른다")
		}
		if _, err := os.Stat(gp); err == nil {
			die("land(제출): 진행 중인 rebase·merge가 있다(%s). 마친 뒤 같은 줄을 다시", f)
		}
	}
	st, rc := gitOut("status", "--porcelain", "--untracked-files=normal")
	if rc != 0 {
		die("land(제출): git status 실패 — 작업 트리를 모른다")
	}
	if st != "" {
		die("land(제출): 커밋 안 된 변경·새 파일이 있다(git status). 커밋하거나 치운 뒤 같은 줄을 다시")
	}
	if os.Getenv("HERDR_LAND_AFTER") != "" {
		die("land(제출): 선행(HERDR_LAND_AFTER)은 병합기 최소판에 없다 — 선행이 main에 들어간 뒤 HERDR_LAND_AFTER 없이 같은 줄을")
	}
	sh, rc := gitOut("rev-parse", "-q", "--verify", "HEAD")
	if rc != 0 {
		die("land(제출): HEAD를 읽지 못했다")
	}
	if lrunF("fetchr git fetch -q origin", func(o, e io.Writer) int { return fetchr(o, e, "fetch", "-q", "origin") }) != 0 {
		die("land(제출): git fetch 실패")
	}
	ob, rc := gitOut("rev-parse", "-q", "--verify", "refs/remotes/origin/"+base+"^{commit}")
	if rc != 0 {
		die("land(제출): origin/%s 가 없다", base)
	}
	lpid = ""
	if lvd != "" {
		bref, blab = ob, "origin/"+base
		lverdictStart()
		if sh != lvs {
			eprintf("hc-lane land: 판정 %s 의 승인 커밋과 제출 HEAD가 다르다\n", lvd)
			exit(2)
		}
	}
	if ssd, rc = gitOut("rev-parse", "--path-format=absolute", "--git-common-dir"); rc != 0 || ssd == "" {
		die("land(제출): 제출 기록 폴더 조회 실패")
	}
	spending = ssd + "/hc-submitting/" + sln
	if st, err := os.Stat(spending); err == nil && st.Mode().IsRegular() {
		pn, pc, ps, ok := readLine3(spending)
		if !ok {
			die("land(제출): 접수 전 기록을 읽지 못했다")
		}
		if saccepted(pn, pc) {
			srecord(pn, pc, ps)
			if ps == sh && mbs == base && (lvd == "" || lvd == mvd) {
				fmt.Printf("hc-lane land: 제출 %s 의 원격 접수와 사건을 복구했다\n", mid)
				exit(0)
			}
		} else if err := os.Remove(spending); err != nil && !os.IsNotExist(err) {
			die("land(제출): 이전 접수 기록을 지우지 못했다")
		}
	}
	switch ia := gitRun("merge-base", "--is-ancestor", sh, ob); ia {
	case 0:
		if rc := landKnown(sh); rc != 0 {
			exit(rc)
		}
		lverdictDone(sh)
		fmt.Printf("hc-lane land: 이 레인의 커밋은 이미 origin/%s 에 있다 — 제출할 것 없음\n", base)
		exit(0)
	case 1:
	default:
		die("land(제출): git merge-base --is-ancestor 실패(종료 %d)", ia)
	}
	laneRef := "refs/heads/lane/" + sln
	rs, rc := gitOut("ls-remote", "origin", laneRef, "refs/merge-queue/*", "refs/merge-queue-meta/*", "refs/merge-failed/*", "refs/merge-done/"+sln)
	if rc != 0 {
		die("land(제출): 원격을 읽지 못했다(git ls-remote) — 같은 줄을 다시")
	}
	cur := field2(rs, laneRef)
	// 같은 커밋을 이미 냈으면 새 제출을 만들지 않는다: 큐에 그 SHA로 남아 있으면 접수 그대로, refs/merge-done/<레인>의 원본이 이 커밋이면 이미 넣음
	receiptMatches := func(ref, oid string) bool {
		if oid == "" {
			return false
		}
		if gitRunQ("cat-file", "-e", oid+"^{commit}") != 0 && fetchr(os.Stdout, os.Stderr, "fetch", "-q", "origin", ref) != 0 {
			die("land(제출): 제출 기록을 받지 못했다")
		}
		parents, rc := gitOut("show", "-s", "--format=%P", oid)
		if rc != 0 {
			die("land(제출): 제출 기록의 부모를 읽지 못했다")
		}
		if parents != sh {
			return false
		}
		if !mmeta(oid, sh) {
			die("land(제출): 제출 기록을 읽지 못했다")
		}
		return mbs == base && (lvd == "" || (mvd == lvd && mpidv == lpid))
	}
	queueRe := regexp.MustCompile(`^refs/merge-queue/[0-9]+-`)
	oldq := ""
	for _, l := range lines(rs) {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		if n := queueRe.ReplaceAllString(f[1], ""); n != f[1] && n == sln && f[0] == sh {
			oldq = f[1]
			break
		}
	}
	oldm := "refs/merge-queue-meta/" + oldq[strings.LastIndex(oldq, "/")+1:]
	if cur == sh && oldq != "" && receiptMatches(oldm, field2(rs, oldm)) {
		s, _ := gitOut("rev-parse", "--short", sh)
		fmt.Printf("hc-lane land: 이 커밋(%s)은 이미 병합기 큐에 있다(lane/%s) — 새로 내지 않음. 결과는 병합기가 사건으로 알린다\n", s, sln)
		exit(0)
	}
	if receiptMatches("refs/merge-done/"+sln, field2(rs, "refs/merge-done/"+sln)) {
		fmt.Printf("hc-lane land: 이 커밋은 병합기가 이미 %s 에 넣었다(refs/merge-done/%s) — 제출할 것 없음\n", base, sln)
		exit(0)
	}
	// 제출 전 싼 사전 검사(go-followups): 병합기의 긴 검증 뒤에야 반송될 것을 몇 초 안에 — 실패면 큐에 넣지 않는다
	if isFile(precheck) {
		if rc := lrun("bash", precheck); rc != 0 {
			lfail(rc)
			die("land(제출): 사전 검사(%s)가 실패해 큐에 넣지 않았다(종료 %d) — 고쳐 커밋한 뒤 같은 줄을 다시", precheck, rc)
		}
	}
	// 같은 레인의 옛 표식·기록(다시 제출하면 새 것만 남는다)
	oldRe := regexp.MustCompile(`^refs/merge-queue(-meta)?/`)
	tenDigits := regexp.MustCompile(`^[0-9]{10}-`)
	var olds [][2]string
	for _, l := range lines(rs) {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		n := oldRe.ReplaceAllString(f[1], "")
		if n != f[1] && tenDigits.MatchString(n) && len(n) >= 11 && n[11:] == sln {
			olds = append(olds, [2]string{f[1], f[0]})
		}
	}
	// 큐 이름은 지금 큐와 반송된 것(refs/merge-failed/)에 없는 초로
	e := time.Now().Unix()
	for {
		taken := false
		for _, l := range lines(rs) {
			for _, k := range []string{"queue", "queue-meta", "failed"} {
				if strings.HasSuffix(l, "refs/merge-"+k+"/"+strconv.FormatInt(e, 10)+"-"+sln) {
					t := strings.TrimSuffix(l, "refs/merge-"+k+"/"+strconv.FormatInt(e, 10)+"-"+sln)
					if t != "" && strings.ContainsAny(t[len(t)-1:], " \t\n\r\v\f") {
						taken = true
					}
				}
			}
		}
		if !taken {
			break
		}
		e++
	}
	qn := strconv.FormatInt(e, 10) + "-" + sln
	q, qm := "refs/merge-queue/"+qn, "refs/merge-queue-meta/"+qn
	fk, rc := gitOut("merge-base", sh, ob)
	if rc != 0 {
		die("land(제출): 갈래점을 구하지 못했다(git merge-base)")
	}
	ow := os.Getenv("HC_LANE_OWNER")
	if ow == "" {
		ow = "-"
	}
	ow = trTo(ow, alnum+"_.,/-")
	orf := ""
	if r := os.Getenv("HC_LANE_OWNER_RUN"); r != "" && strings.Trim(r, "0123456789") == "" {
		orf = " owner_run=" + r
	}
	mt := os.Getenv("HC_LANE_MACHINE")
	if mt == "" {
		mt = hostShort()
	}
	if mt = trTo(mt, alnum+"_.-"); mt == "" {
		mt = "-"
	}
	sid := fmt.Sprintf("%s-%s-%d", mt, time.Now().UTC().Format("20060102T150405"), os.Getpid())
	born, ok := createdMs(os.Getenv("HC_TERMINAL_ID"), 3*time.Second)
	if !ok {
		born = "-"
	}
	job := os.Getenv("HC_LANE_JOB")
	if job == "" {
		job = "-"
	}
	term := os.Getenv("HC_TERMINAL_ID")
	if term == "" {
		term = "-"
	}
	msg := fmt.Sprintf("merge-queue lane=%s id=%s orig=%s fork=%s base=%s host=%s term=%s owner=%s%s job=%s created_ms=%s priority=%s",
		sln, sid, sh, fk, base, mt, term, ow, orf, trTo(job, alnum+"_.~-"), born, lprio())
	if lvd != "" {
		msg += " verdict=" + lvd + " patch=" + lpid
	}
	et, rc := gitOut("hash-object", "-t", "tree", "-w", "/dev/null")
	mc := ""
	if rc == 0 {
		mc, rc = gitOut("commit-tree", et, "-p", sh, "-m", msg)
	}
	if rc != 0 {
		die("land(제출): 제출 기록 커밋을 만들지 못했다")
	}
	args := []string{sh + ":" + laneRef, sh + ":" + q, mc + ":" + qm, "--force-with-lease=" + laneRef + ":" + cur, "--force-with-lease=" + q + ":", "--force-with-lease=" + qm + ":"}
	for _, o := range olds {
		args = append(args, ":"+o[0], "--force-with-lease="+o[0]+":"+o[1])
	}
	if os.MkdirAll(ssd+"/hc-submitting", 0o777) != nil || os.WriteFile(spending+".tmp", []byte(qn+" "+mc+" "+sh+"\n"), 0o666) != nil ||
		os.Rename(spending+".tmp", spending) != nil {
		die("land(제출): 접수 전 기록을 쓰지 못했다")
	}
	po, pr := gitOut2(nil, append([]string{"push", "--atomic", "--porcelain", "origin"}, args...)...)
	if llog != "" {
		appendLog(po + "\n")
	}
	// 원격 접수 확인: push가 0이어도 다시 읽어 본다(응답이 끊긴 실패도 같은 판정)
	if !saccepted(qn, mc) {
		eprintf("%s\n", po)
		die("land(제출): push 실패(위 git 출력, 종료 %d) — 같은 줄을 다시", pr)
	}
	srecord(qn, mc, sh)
	s, _ := gitOut("rev-parse", "--short", sh)
	fmt.Printf("hc-lane land: 제출 %s → %s (lane/%s) — 병합기(%s)가 %s 에 넣고 결과를 사건(착지·막힘 --name %s)으로 알린다. 이 레인은 여기서 끝(report 착지는 부르지 않아도 된다 — 불러도 적히지 않음). 다음은 지시문의 hc-lane clean 줄(작업 트리 정리)을 돌리고 턴을 끝낸다\n",
		s, q, sln, mg, base, sln)
	exit(0)
}
