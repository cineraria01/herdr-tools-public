// report.go: hc-lane report <상태> [<sha>] [<한 줄>] [--name <레인>] [--to <지휘>] [--key <키>] — 레인 사건 한 줄
// (셸 판 lib/hc-lane/report.sh report_cmd·report_pending과 같은 꼴·같은 종료 코드)
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"herdr-tools/internal/events"
	"herdr-tools/internal/gitx"
	"herdr-tools/internal/lockx"
	"herdr-tools/internal/proc"
)

// reportCmd(common.go)는 이 명령을 따로 부른다(포장 → 이 Go 판) — 이 파일의 본체는 reportMain
func init() { register("report", reportMain, noHelp) }

// reportPending: 0 병합기 큐에 제출만 됨(착지는 병합기가 적는다), 1 일반 착지 보고, 2 확인 실패. 저장소 밖이면 1
func reportPending(lane, sha string) int {
	c := exec.Command("git", "rev-parse", "--git-common-dir")
	c.Env = append(os.Environ(), "LC_ALL=C")
	ob, err := proc.Combined(c)
	if err != nil {
		if strings.HasPrefix(string(ob), "fatal: not a git repository (or any ") {
			return 1
		}
		return 2
	}
	gd := strings.TrimRight(string(ob), "\n")
	target := ""
	if lines, err := readLines(gd + "/hc-submitted"); err == nil {
		for _, l := range lines {
			f := strings.Fields(l)
			if len(f) >= 3 && f[0] == lane && strings.HasPrefix(f[1], sha) {
				target = f[2]
			}
		}
	} else if !os.IsNotExist(err) {
		return 2
	}
	if target == "" {
		if b, err := os.ReadFile(gd + "/hc-submitting/" + lane); err == nil {
			// read -r _ _ pending: 줄바꿈 없는 줄은 read 실패(모름)
			l, _, nl := strings.Cut(string(b), "\n")
			if !nl {
				return 2
			}
			if f := strings.Fields(l); len(f) >= 3 && strings.HasPrefix(f[2], sha) {
				return 2
			}
		} else if !os.IsNotExist(err) {
			return 2
		}
		ref := "refs/remotes/origin/lane/" + lane
		switch gitx.Run("", nil, nil, "show-ref", "--exists", ref) {
		case 0:
		case 2:
			return 1
		default:
			return 2
		}
		switch gitx.Run("", nil, nil, "merge-base", "--is-ancestor", sha, ref) {
		case 0:
		case 1:
			return 1
		default:
			return 2
		}
		t, rc := gitx.Out("", nil, "for-each-ref", "--format=%(symref:short)", "refs/remotes/origin/HEAD")
		if rc != 0 {
			return 2
		}
		target = strings.TrimPrefix(t, "origin/")
	}
	if target == "" {
		target = "main"
	}
	switch gitx.Run("", nil, nil, "merge-base", "--is-ancestor", sha, "refs/remotes/origin/"+target) {
	case 0:
		return 1
	case 1:
		return 0
	}
	return 2
}

func reportMain(a []string) int {
	var rn, st, sha, msg, to, key, kind string
	k := 0
	for i := 0; i < len(a); i++ {
		switch a[i] {
		case "--name":
			rn = val(a, i)
			i++
			continue
		case "--to":
			to = val(a, i)
			i++
			continue
		case "--key":
			key = val(a, i)
			i++
			continue
		case "--kind":
			kind = val(a, i)
			i++
			continue
		}
		k++
		switch k {
		case 1:
			st = a[i]
		case 2:
			sha = a[i]
		case 3:
			msg = a[i]
		default:
			msg += " " + a[i]
		}
	}
	if st == "" || !inWords(evstates, st) {
		s := st
		if s == "" {
			s = "(없음)"
		}
		fmt.Fprintf(os.Stderr, "hc-lane report: 상태는 %s 가운데 하나: %s\n", evstates, s)
		return 2
	}
	if !onlyChars(to, alnum+"_.,/-") {
		fmt.Fprintf(os.Stderr, "hc-lane report: --to 지휘 주소 글자: %s\n", to)
		return 2
	}
	if kind != "" && kind != events.KindOps && kind != events.KindLane {
		fmt.Fprintf(os.Stderr, "hc-lane report: --kind 는 %s 또는 %s: %s\n", events.KindLane, events.KindOps, kind)
		return 2
	}
	if !onlyChars(key, alnum+"_.:/-") {
		fmt.Fprintf(os.Stderr, "hc-lane report: --key 글자(A-Za-z0-9_.:/-): %s\n", key)
		return 2
	}
	// 「제출」은 hc-lane land만 적는다(--key submit:<제출 ID>) — 손으로 적은 제출의 첫 낱말이 제출 ID로 읽혀 회신이 멈췄다(2026-11-23)
	if id, ok := strings.CutPrefix(key, "submit:"); st == "제출" && (!ok || !events.IsSubmitID(id)) {
		fmt.Fprintln(os.Stderr, "hc-lane report: 제출은 hc-lane land가 적는다(제출 ID 키 --key submit:<ID> 없음) — 감사·진행 글은 중간으로")
		return 2
	}
	// 대문자 sha도 sha로(소문자로 적음). 7~40자가 아닌 16진 낱말(DB·CAFE)은 한 줄 칸으로 그대로
	join := func(s string) string {
		if msg == "" {
			return s
		}
		return s + " " + msg
	}
	switch {
	case sha == "" || sha == "-":
		sha = "-"
	case sha == "HEAD": // reportSHA가 푼다
	case !onlyChars(sha, "0123456789ABCDEFabcdef"):
		msg, sha = join(sha), "-"
	case len(sha) >= 7 && len(sha) <= 40:
		sha = strings.ToLower(sha)
	default:
		msg, sha = join(sha), "-"
	}
	// 완료·중간 sha와 HEAD는 이 작업 트리에서 전체 해시로(reportsha.go) — 못 풀면 사건을 쓰지 않는다
	sha, ok := reportSHA(st, sha)
	if !ok {
		return 2
	}
	m := machineName()
	tid := os.Getenv("HC_TERMINAL_ID")
	g := ""
	if tid != "" {
		if out, rc := hcRun(3, errDrop, "get", tid, "--json"); rc == 0 {
			g = out
		}
	}
	gm := jparse(g)
	tn := topNum(gm, "turn", "")
	lane := rn
	if lane == "" {
		lane = os.Getenv("HC_LANE")
	}
	if lane == "" {
		lane, _ = jget(gm, "name")
	}
	if lane == "" {
		lane, _ = gitx.Out("", nil, "symbolic-ref", "-q", "--short", "HEAD")
		ob, _ := gitx.Out("", nil, "symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD")
		ob = strings.TrimPrefix(ob, "origin/")
		if ob == "" {
			ob = "main"
		}
		if lane == "main" || lane == "master" || lane == ob {
			lane = ""
		}
	}
	// 병합기에 제출만 한 커밋의 「착지」는 적지 않는다 — 착지는 병합기가 적는다
	if st == "착지" && sha != "-" && lane != "" {
		switch reportPending(lane, sha) {
		case 0:
			fmt.Printf("hc-lane report: %s 는 병합기 큐에 제출만 됐다(lane/%s) — 착지는 병합기가 사건(착지·막힘 --name %s)으로 적는다. 아무것도 적지 않음\n", sha, lane, lane)
			return 0
		case 1:
		default:
			die("report: %s 제출·착지 조회 실패 — 사건을 적지 않았다. 같은 줄을 다시", lane)
		}
	}
	// 10칸 발행 실행 = <터미널>@<created_ms>(HC_LANE_JOB이 있으면 그것), 11칸 수신 지휘 = HC_LANE_OWNER[@HC_LANE_OWNER_RUN], 12칸 키
	rk := os.Getenv("HC_LANE_JOB")
	if !onlyChars(rk, "abcdefghijklmnopqrstuvwxyz0123456789~-") {
		rk = ""
	}
	if rk == "" {
		rk = "-"
		if cm, ok := liveCreatedMs(g); ok && g != "" && tid != "" {
			rk = bcut(64, flat(lastSeg(tid))) + "@" + cm
		}
		if rk == "-" && tid != "" {
			fmt.Fprintln(os.Stderr, "hc-lane report: 경고 — 발행 실행(10칸)을 못 적었다(hc get created_ms 없음). 완료라면 잠시 뒤 같은 줄을 다시 부른다(실행을 모르는 완료는 자동 감사가 열지 않는다)")
		}
	}
	ow := os.Getenv("HC_LANE_OWNER")
	if ow == "" || !onlyChars(ow, alnum+"_./*-") {
		ow = "-"
	}
	if ow != "-" && ow != "*" {
		if r := os.Getenv("HC_LANE_OWNER_RUN"); digits(r) {
			ow += "@" + r
		}
	}
	e := events.Event{Time: utcNow(), Machine: m, Term: tid, Lane: lane, State: st, SHA: sha, Msg: msg,
		Turn: tn, To: to, Run: rk, Owner: ow, Key: key}
	// 식별자 칸은 자르지 않으니 쓰기 전에 사건 전체를 공통 검사로 — 줄 예산(1000바이트)을 넘기면 입력 오류, 사건 안 씀(재감사 R-27·R-C03)
	if err := e.Check(); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane report: %v\n", err)
		return 2
	}
	line := withRepoKind(e, reportRepo(rn != ""), kind)
	ed := evdir()
	if err := os.MkdirAll(ed, 0o777); err != nil {
		die("report: 폴더를 만들지 못했다: %s", ed)
	}
	f := strings.Split(line, tab)
	evf := ed + "/events.tsv"
	if key != "" {
		// 같은 12칸 키 줄이 있으면 더하지 않는다. 확인과 더하기를 한 잠금 안에서. 읽기 실패는 「키 없음」이 아니다
		have, err := keyedAppend(ed+"/events.lock", evf, key, line)
		if err != nil {
			die("report: 키 확인·쓰기 실패(--key %s): %s", key, evf)
		}
		if have {
			fmt.Printf("%s %s: 이미 있음(키 %s)\n", f[3], st, key)
			return 0
		}
	} else if err := appendLines(evf, line); err != nil {
		die("report: 쓰지 못했다: %s", evf)
	}
	fmt.Printf("%s %s: %s %s\n", f[3], st, sha, f[6])
	if st == "완료" && rn == "" && key == "" { // 자기 작업 트리의 완료만(도구가 --name으로 적는 사건은 아님) — reportpre.go
		prePost(lane, sha)
	}
	return 0
}

// withRepo: 사건 줄에 13칸 저장소(watch-project-route)를 — 모르면 지금 꼴 12칸 그대로. 1000바이트 안에 들게 한 줄 칸을 줄이고,
// 그래도 안 들면 13칸을 뺀다(모름 — 모든 지휘에게 가는 쪽).
// 14칸 종류(ops-events): --kind, 없으면 이름 표(events.KindOf)로. 레인 사건은 싣지 않아 줄이 지금 꼴 그대로다(열쇠 같음).
// 종류 칸이 저장소 칸보다 먼저 자리를 받는다(운영을 레인으로 읽으면 글 머리·거름이 틀린다)
func withRepo(e events.Event, rid string) string { return withRepoKind(e, rid, "") }

func withRepoKind(e events.Event, rid, kind string) string {
	line := e.Line()
	if kind == "" {
		kind = events.KindOf(strings.Split(line, tab))
	}
	kc := "" // 덧붙는 13·14칸
	if events.WithKind(line, kind) != line {
		kc = tab + kind
	}
	tail := ""
	switch {
	case rid != "":
		tail = tab + rid + kc
	case kc != "":
		tail = tab + "-" + kc
	}
	if over := len(line) + len(tail) - events.MaxLine; over > 0 {
		shown := fld(strings.Split(line, tab), 7)
		e.Msg = bcut(len(shown)-over, shown)
		line = e.Line()
	}
	if len(line)+len(tail) > events.MaxLine && rid != "" { // 저장소 칸을 빼고 다시(모름)
		return withRepoKind(e, "", kind)
	}
	return line + tail
}

// keyedAppend: 잠금(셸 판 lockf -k -t 30과 같은 flock) 안에서 12칸이 key인 줄이 없으면 더한다. have = 이미 있음
func keyedAppend(lockPath, evf, key, line string) (have bool, err error) {
	deadline := time.Now().Add(30 * time.Second)
	for {
		lf, busy, err := lockx.TryLock(lockPath)
		if err != nil {
			return false, err
		}
		if !busy {
			defer lf.Close()
			break
		}
		if time.Now().After(deadline) {
			return false, fmt.Errorf("잠금 기다림 30초")
		}
		time.Sleep(100 * time.Millisecond)
	}
	lines, err := readLines(evf)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	for _, l := range lines {
		f := strings.Split(l, tab)
		if len(f) >= 12 && f[11] == key {
			return true, nil
		}
	}
	return false, appendLines(evf, line)
}

func inWords(list, w string) bool {
	for _, x := range strings.Fields(list) {
		if x == w {
			return true
		}
	}
	return false
}
