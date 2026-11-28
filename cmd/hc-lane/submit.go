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

	"herdr-tools/internal/atomic"
	"herdr-tools/internal/events"
	"herdr-tools/internal/gitx"
	"herdr-tools/internal/landing"
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

// lsq: 원격 참조 조회(gitx.LsRemote — 줄 꼴·요청 이름·끝 줄바꿈까지 본다). 모름(실패·시간 넘김·잘린 줄·요청 밖 이름)이면 큐·메타·
// 접수 전 기록을 그대로 두고 75(같은 줄을 다시)로 끝낸다 — 「없음」은 종료 2(맞는 참조 없음)뿐(재감사 lane/R-212)
func lsq(why string, patterns ...string) map[string]string {
	m, st := gitx.LsRemote("", "origin", os.Stderr, patterns...)
	if st == gitx.Unknown {
		eprintf("hc-lane land(제출): %s — 원격을 읽지 못했다(git ls-remote 실패·잘림·요청 밖 이름). 큐는 그대로 — 같은 줄을 다시\n", why)
		exit(75)
	}
	return m
}

// saccepted: 제출 기록 커밋 mc가 큐 기록 자리·넣음(merge-done)·돌려보냄(merge-failed-meta) 가운데 하나에 있으면 접수
func saccepted(qn, mc string) bool {
	q, d, f := "refs/merge-queue-meta/"+qn, "refs/merge-done/"+sln, "refs/merge-failed-meta/"+qn
	refs := lsq("원격 접수를 확인하지 못했다", q, d, f)
	return refs[q] == mc || refs[d] == mc || refs[f] == mc
}

// supersede: 레인이 큐 qn을 갈아 끼운다는 표식 — 착지 중 표식 자리(landing.Prefix+qn)에 새 기록 커밋 val을 「본 값 그대로일 때만」
// (없으면 없음 lease — 서버가 원자 비교) 쓰는 push 인자. 병합기가 조회 뒤 push 전에 그 자리에 착지 중 표식을 올렸으면 같은 원자
// push 전체가 거절된다(재감사 lane/R-213·R-214). val은 큐 SHA가 아니라 landing.Of가 착지 중으로 보지 않고, 병합기가 다음 바퀴에 지운다
func supersede(refs map[string]string, qn, val string) []string {
	mk := landing.Prefix + qn
	return []string{val + ":" + mk, "--force-with-lease=" + mk + ":" + refs[mk]}
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
	// 제출은 남의 이름(--name)으로 적지만 이 저장소 안에서 부르니 13칸 저장소를 넘긴다(watch-project-route)
	if runCmd(cmdOpt{stderr: os.Stderr, env: []string{"HC_LANE_REPO_ID=" + repoHere()}}, selfExe(), "report", "제출", sha, mid+" → refs/merge-queue/"+qn+" (병합기 "+mg+" 가 "+mbs+" 에 넣는다)", "--name", sln, "--key", "submit:"+mid) != 0 {
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

// metaPrio <메타 ref> <옛 SHA> <우선 라벨> <원격 목록>: 메타 커밋 제목의 priority=가 다르면 커밋 객체를 글 그대로(priority만) 다시 써
// --force-with-lease로 갈아 끼우고 참. 같으면 거짓. 못 바꾸면 경고하고 거짓(큐 그대로 — 제출은 이미 있다).
// 같은 원자 push에 갈아 끼움 표식(supersede)을 실어, 조회 뒤 병합기가 그 항목에 착지 중 표식을 올렸으면 메타를 바꾸지 않는다(lane/R-214)
func metaPrio(ref, old, pl string, refs map[string]string) bool {
	var b bytes.Buffer
	if runCmd(cmdOpt{stdout: &b}, "git", "cat-file", "commit", old) != 0 {
		eprintf("hc-lane land: 경고 — 큐 메타 %s 를 못 읽어 우선을 그대로 둔다\n", ref)
		return false
	}
	head, msg, ok := strings.Cut(b.String(), "\n\n")
	if !ok {
		eprintf("hc-lane land: 경고 — 큐 메타 %s 꼴을 몰라 우선을 그대로 둔다\n", ref)
		return false
	}
	subj, rest, nl := strings.Cut(msg, "\n")
	ws, found := strings.Fields(subj), false
	for i, w := range ws {
		if strings.HasPrefix(w, "priority=") {
			ws[i], found = "priority="+pl, true
		}
	}
	if !found {
		ws = append(ws, "priority="+pl)
	}
	ns := strings.Join(ws, " ")
	if ns == subj {
		return false
	}
	if nl {
		ns += "\n" + rest
	}
	var nb bytes.Buffer
	if runCmd(cmdOpt{stdout: &nb, stderr: os.Stderr, stdin: strings.NewReader(head + "\n\n" + ns)}, "git", "hash-object", "-t", "commit", "-w", "--stdin") != 0 {
		eprintf("hc-lane land: 경고 — 새 큐 메타를 못 만들어 우선을 그대로 둔다\n")
		return false
	}
	nw := strings.TrimSpace(nb.String())
	if !gitx.ObjID(nw) {
		eprintf("hc-lane land: 경고 — 새 큐 메타 ID를 읽지 못해 우선을 그대로 둔다(%q)\n", cut(nw, 80))
		return false
	}
	args := append([]string{"push", "-q", "--atomic", "--porcelain", "origin", nw + ":" + ref, "--force-with-lease=" + ref + ":" + old},
		supersede(refs, strings.TrimPrefix(ref, "refs/merge-queue-meta/"), nw)...)
	if po, rc := gitOut2(nil, args...); rc != 0 {
		eprintf("%s\nhc-lane land: 경고 — 큐 메타 %s 를 못 바꿨다(push 거절 — 그 사이 병합기가 착지를 시작했거나 메타를 바꿨다) — 우선 그대로\n", po, ref)
		return false
	}
	fmt.Printf("hc-lane land: 이 커밋은 이미 병합기 큐에 있다(lane/%s) — 새로 내지 않고 큐 메타 %s 만 priority=%s 로 바꿨다\n", sln, ref, pl)
	return true
}

// submitConflict <origin/<기본> 커밋> <제출 커밋>(conflict-fast A, 2026-11-23 — 병합기 충돌 반송 28건 가운데 8건은 제출할 때 이미
// 기본 브랜치와 충돌했는데 줄에서 중앙값 29분을 기다린 뒤 반송됐다): 3자 합침으로 충돌하면 큐에 넣지 않고 종료 3(직접 착지의 rebase
// 충돌과 같은 「양쪽 살려 풀고 같은 줄을 다시」 — 다만 rebase는 시작하지 않았다). 판정을 못 하면(git 실패·시간 넘김·옛 git) 지금처럼 낸다.
// rebase를 마치면 origin/<기본>이 제출 커밋의 조상이라 3자 합침은 늘 깨끗하다 — 같은 줄을 다시 부르면 이 검사는 통과한다
func submitConflict(ob, sh string) {
	if stackSkipConflict(ob, sh) { // 쌓인 제출이고 선행이 아직 기본 가지에 없다 — 병합기가 후보를 지을 때 본다(stack.go)
		return
	}
	fs, rc := gitx.MergeConflicts("", ob, sh)
	switch rc {
	case 0:
		return
	case 1:
	default:
		eprintf("hc-lane land(제출): origin/%s 와의 충돌을 판정하지 못했다(git merge-tree 종료 %d) — 판정 없이 제출한다(병합기가 차례에 본다)\n", base, rc)
		return
	}
	// 기준에 같은 patch-id 커밋이 든 제출은 rebase가 그 커밋을 건너뛰어 깨끗할 수 있다 — 거절하지 않고 차례에 맡긴다(감사 atoxvrv-yecgh5 낮음 3)
	eq, rc := gitx.CherryEquivalent("", ob, sh)
	if rc != 0 {
		eprintf("hc-lane land(제출): origin/%s 와의 충돌을 판정하지 못했다(같은 patch-id 조회 git 종료 %d) — 판정 없이 제출한다(병합기가 차례에 본다)\n", base, rc)
		return
	}
	if eq {
		return
	}
	j := events.Flat(strings.Join(fs, " "))
	if c := events.Bcut(200, j); c != j { // 글자 중간을 자르지 않는다
		j = c + " …"
	}
	if j == "" {
		j = "파일 모름"
	}
	eprintf("hc-lane land(제출): 제출 커밋(%s)이 origin/%s 와 충돌한다(충돌 %s) — 큐에 넣지 않았다. git rebase origin/%s 로 양쪽 내용을 살려 풀고(고친 파일을 경로로 git add → git rebase --continue) 푼 파일 둘레의 빌드·시험만 돌린 뒤 같은 줄(hc-lane land %s)을 다시 — 전체 검증은 병합기가 돈다\n",
		cut(sh, 9), base, j, base, base)
	if lvd != "" {
		// rebase하면 승인 커밋이 바뀐다 — 병합기 반송 뒤 다시 맞춤과 같은 길(--verdict 없이 다시 냄, 새 감사 없음).
		// 이 줄이 마지막 줄이라 대기열 제출(hc-fill autosubmit)이 넘김 까닭으로 쓴다 — 충돌 파일을 여기에도(ht-autosubmit-rebase)
		eprintf("hc-lane land(제출): 판정 %s 가 붙은 제출이 origin/%s 와 충돌한다(충돌 %s) — rebase하면 승인 커밋이 바뀌니 다시 낼 때는 --verdict 없이 hc-lane land %s (병합기 충돌 반송 뒤 다시 맞춤과 같다)\n", lvd, base, j, base)
	}
	exit(3)
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
	parkGate(sh) // 주차된 커밋은 내지 않는다 — 판정 시작 표시를 두기 전(stands.go)
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
	rs := lsq("원격 큐를 읽지 못했다", laneRef, "refs/merge-queue/*", "refs/merge-queue-meta/*", "refs/merge-failed/*", "refs/merge-done/"+sln, landing.Prefix+"*")
	rl := landing.Refs(rs) // 이름순 — 같은 원격 상태면 늘 같은 판정
	cur := rs[laneRef]
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
	for _, r := range rl {
		if n := queueRe.ReplaceAllString(r.Name, ""); n != r.Name && n == sln && r.SHA == sh {
			oldq = r.Name
			break
		}
	}
	oldm := "refs/merge-queue-meta/" + oldq[strings.LastIndex(oldq, "/")+1:]
	lq := landing.Of(rl, sln) // 병합기가 지금 착지 중인 이 레인 제출 — 메타·큐를 바꾸는 어떤 일보다 먼저 본다(lane/R-214)
	if cur == sh && oldq != "" && receiptMatches(oldm, rs[oldm]) {
		s, _ := gitOut("rev-parse", "--short", sh)
		if lq != "" {
			fmt.Printf("hc-lane land: 이 커밋(%s)은 병합기가 지금 착지 중이다(refs/merge-queue/%s) — 큐·메타·우선 그대로. 결과는 병합기가 사건으로 알린다\n", s, lq)
			exit(0)
		}
		// 우선만 바꿔 다시 냄(fill-edit-after): 적은 HC_LANE_PRIORITY가 지금 메타와 다르면 메타만 갈아 끼운다. 적지 않았으면 그대로
		// (띄울 때 값이 남은 레인이 지휘가 올린 우선을 되돌리지 않게)
		if os.Getenv("HC_LANE_PRIORITY") != "" && metaPrio(oldm, rs[oldm], lprio(), rs) {
			exit(0)
		}
		fmt.Printf("hc-lane land: 이 커밋(%s)은 이미 병합기 큐에 있다(lane/%s) — 새로 내지 않음. 결과는 병합기가 사건으로 알린다\n", s, sln)
		exit(0)
	}
	if receiptMatches("refs/merge-done/"+sln, rs["refs/merge-done/"+sln]) {
		fmt.Printf("hc-lane land: 이 커밋은 병합기가 이미 %s 에 넣었다(refs/merge-done/%s) — 제출할 것 없음\n", base, sln)
		exit(0)
	}
	submitGuard(lq, cur, sh)
	submitConflict(ob, sh)
	// 제출 전 싼 사전 검사(go-followups): 병합기의 긴 검증 뒤에야 반송될 것을 몇 초 안에 — 실패면 큐에 넣지 않는다
	// 돌리는 것은 제출 커밋 속 판만(무시·추적 안 된 로컬 파일을 돌리지 않는다 — committedHook, gofix L12)
	// 후보 트리에서 훅의 있음·없음·조회 실패를 가른다 — 조회 실패를 「선택 훅 없음」으로 읽고 제출하지 않는다(R-16)
	lt, lr := gitOutQ("ls-tree", "--name-only", sh, "--", precheck)
	inCommit := 1
	switch {
	case lr != 0:
		eprintf("hc-lane land(제출): 제출 커밋(%s)에서 사전 검사 %s 를 찾지 못했다(git ls-tree 종료 %d) — 큐에 넣지 않았다. 같은 줄을 다시\n", cut(sh, 9), precheck, lr)
		exit(75)
	case lt == precheck:
		inCommit = 0
	case lt != "":
		eprintf("hc-lane land(제출): git ls-tree 응답을 읽지 못했다(%q) — 큐에 넣지 않았다. 같은 줄을 다시\n", cut(lt, 80))
		exit(75)
	}
	if inCommit == 0 {
		committedHook(sh, precheck)
	} else if isFile(precheck) {
		die("land(제출): %s 가 제출 커밋(%s)에 없다(추적 안 됨·무시됨) — 커밋하거나 지운 뒤 같은 줄을 다시", precheck, cut(sh, 9))
	}
	if inCommit == 0 {
		if at, ok := verifiedTree(sh); ok {
			fmt.Printf("hc-lane land: 사전 검사 건너뜀 — 이 트리는 전체 검증 통과(%s)\n", at)
		} else if rc := lrun("bash", precheck); rc != 0 {
			lfail(rc)
			die("land(제출): 사전 검사(%s)가 실패해 큐에 넣지 않았다(종료 %d) — 고쳐 커밋한 뒤 같은 줄을 다시", precheck, rc)
		}
	}
	// 같은 레인의 옛 표식·기록(다시 제출하면 새 것만 남는다)
	oldRe := regexp.MustCompile(`^refs/merge-queue(-meta)?/`)
	tenDigits := regexp.MustCompile(`^[0-9]{10}-`)
	var olds [][2]string
	for _, r := range rl {
		n := oldRe.ReplaceAllString(r.Name, "")
		if n != r.Name && tenDigits.MatchString(n) && len(n) >= 11 && n[11:] == sln {
			olds = append(olds, [2]string{r.Name, r.SHA})
		}
	}
	// 큐 이름은 지금 큐와 반송된 것(refs/merge-failed/)에 없는 초로
	e := time.Now().Unix()
	for {
		n := strconv.FormatInt(e, 10) + "-" + sln
		if rs["refs/merge-queue/"+n] == "" && rs["refs/merge-queue-meta/"+n] == "" && rs["refs/merge-failed/"+n] == "" {
			break
		}
		e++
	}
	qn := strconv.FormatInt(e, 10) + "-" + sln
	q, qm := "refs/merge-queue/"+qn, "refs/merge-queue-meta/"+qn
	fk, rc := forkOf(sh, ob) // 쌓인 레인은 선행 커밋 P(stack.go)
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
		sln, sid, sh, fk, base, mt, term, ow, orf, trTo(job, alnum+"_.~-"), born, lprio()) + stackMeta(sh, ob)
	if lvd != "" {
		msg += " verdict=" + lvd + " patch=" + lpid
	}
	et, rc := gitOut("hash-object", "-t", "tree", "-w", "/dev/null")
	mc := ""
	if rc == 0 {
		mc, rc = gitOut("commit-tree", et, "-p", sh, "-m", msg)
	}
	if rc != 0 || !gitx.ObjID(mc) {
		die("land(제출): 제출 기록 커밋을 만들지 못했다")
	}
	args := []string{sh + ":" + laneRef, sh + ":" + q, mc + ":" + qm, "--force-with-lease=" + laneRef + ":" + cur, "--force-with-lease=" + q + ":", "--force-with-lease=" + qm + ":"}
	for _, o := range olds {
		args = append(args, ":"+o[0], "--force-with-lease="+o[0]+":"+o[1])
		if on, ok := strings.CutPrefix(o[0], "refs/merge-queue/"); ok {
			args = append(args, supersede(rs, on, mc)...) // 조회 뒤 병합기가 이 옛 제출의 착지를 시작했으면 push 전체 거절(lane/R-213)
		}
	}
	markLaneName(sln)
	// fsync·rename·폴더 fsync — push 뒤 기계가 갑자기 꺼져도 접수 전 기록이 남게(gofix E06)
	if os.MkdirAll(ssd+"/hc-submitting", 0o777) != nil || atomic.WriteFile(spending, []byte(qn+" "+mc+" "+sh+"\n"), 0o666) != nil {
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
