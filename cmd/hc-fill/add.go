package main

// add(--then 포함) — 셸 판 bin/hc-fill add·_then·_add

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/oplog"
)

func init() { register("add", addCmd) }

func addCmd(args []string) {
	name, prj, brief, after, only, coord, prof := "", "", "", "-", "-", "", ""
	cx, prio, early, aud, ovk, thn, xo, onl, anyp := false, "normal", "-", "-", "-", "", "", "", false
	for len(args) > 0 {
		a := args[0]
		switch a {
		case "--no-early", "--no-audit", "--overlap-ok", "--solo", "--after", "--only", "--priority", "--coord", "--on-land", "--after-any-project", "--pr-of":
			xo += " " + a // --then과 함께 못 씀
		}
		switch a {
		case "--after-any-project":
			anyp, args = true, args[1:]
			continue
		case "--no-early":
			early, args = "no-early", args[1:]
			continue
		case "--no-audit":
			aud, args = "no-audit", args[1:]
			continue
		case "--overlap-ok":
			if ovk == "solo" {
				die("--solo는 --overlap-ok와 함께 쓰지 않는다")
			}
			ovk, args = "overlap-ok", args[1:]
			continue
		case "--solo":
			if ovk == "overlap-ok" {
				die("--solo는 --overlap-ok와 함께 쓰지 않는다")
			}
			ovk, args = "solo", args[1:]
			continue
		}
		if len(args) < 2 {
			usage()
		}
		v := args[1]
		switch a {
		case "--name":
			name = v
		case "--project":
			prj = v
		case "--brief":
			brief = v
		case "--after":
			after = v
		case "--only":
			only = v
		case "--coord":
			coord, cx = v, true
		case "--then":
			thn = v
		case "--on-land":
			onl = v
		case "--pr-of":
			prof = v
		case "--priority":
			p, ok := pnorm(v)
			if !ok {
				die("--priority는 긴급|높음|보통|낮음(urgent|high|normal|low): %s", v)
			}
			prio = p
		default:
			usage()
		}
		args = args[2:]
	}
	if thn != "" {
		DRC = 2 // 아래 검사에 걸리면 거절(종료 2), 아무것도 바꾸지 않음
		if xo != "" {
			die("--then은%s와 함께 쓰지 않는다 — 이어 붙인 일은 선행 줄의 설정·주인을 따른다", xo)
		}
		if !okname(thn) {
			die("--then 레인 이름: %s", thn)
		}
	}
	if !okname(name) {
		die("--name은 a-z 0-9 - _ 32자까지: %s", name)
	}
	if strings.HasPrefix(name, "audit-") {
		die("--name은 audit- 로 시작하지 않는다(자동 감사 줄 이름 자리): %s", name)
	}
	if w, k := kindwhy(name); k {
		die("%s", w)
	}
	prOfCheck(name, prof)
	if !projectKnown(prj) {
		die("설정에 repo 줄이 없는 프로젝트: %s", prj)
	}
	if !strings.HasPrefix(brief, "/") {
		wd, _ := os.Getwd()
		brief = wd + "/" + brief
	}
	if !isFile(brief) {
		die("지시문이 없다: %s", brief)
	}
	if strings.ContainsAny(brief, " \t\n\v\f\r") {
		die("지시문 경로에 공백을 쓰지 않는다: %s", brief)
	}
	if why, b := briefBad(brief); b {
		die("지시문을 읽지 못함(%s): %s", why, brief)
	}
	if thn != "" {
		exit(thenAdd(name, thn, prj, brief))
	}
	if x := thenof(name); x != "" {
		die("%s은 이미 %s에 이어 붙였다(--then) — 다른 이름으로", name, x)
	}
	if anyp && after == "-" {
		die("--after-any-project는 --after와 함께 쓴다")
	}
	if after != "-" {
		after = afterFix(name, prj, after, anyp) // 잘못된 이름 종료 1, 다른 프로젝트 선행 거절 종료 2
	}
	if onl != "" {
		onlandCheck(onl, prj)
	}
	if only != "-" {
		for _, m := range words(only) {
			if !machineKnown(m) {
				die("설정에 machine 줄이 없는 기계: %s", m)
			}
		}
	}
	// 주인 지휘: 지휘 정보를 남겨(hc-lane coord --set) 그 지휘 watch가 뜨고 보내게. 지휘가 아니면 --coord는 거절, 저절로 잡은 것은 -
	if !cx {
		coord = os.Getenv("HC_TERMINAL_ID")
	}
	if coord != "" {
		if strings.Trim(coord, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_./-") != "" {
			die("--coord 주소 글자: %s", coord)
		}
		ce, rc := Cmd{Name: "hc-lane", Args: []string{"coord", "--set", coord}, Err: errMerge}.Run()
		if rc != 0 {
			if cx {
				die("--coord %s 를 지휘로 적지 못했다: %s", coord, lastLine(ce))
			}
			// coord --set 실패는 「지휘 아님」과 「확인 못 함」이 같은 꼴 — 지휘가 아님이 확인될 때만 주인 없이(재감사 R-09)
			if !notCoord(coord) {
				fmt.Fprintf(os.Stderr, "hc-fill: 미룸 — %s 를 주인 지휘로 적지 못했고 지휘가 아닌지도 확인하지 못했다(%s). 주인 없이 넣지 않았다 — 다시 add 한다\n", coord, lastLine(ce))
				exit(75)
			}
			say("알림: %s 는 지휘 터미널이 아니라 주인 없이 넣음 — 이 레인 사건은 모든 지휘에게 간다 (%s)", coord, lastLine(ce))
			coord = ""
		}
	}
	// 지시문은 내용을 데이터 폴더로 복사해 둔다(launchd의 run이 ~/Documents 같은 곳을 못 읽어도 띄울 수 있게)
	bf := DATA + "/briefs/added/" + name + ".md"
	tmp := bf + ".tmp." + strconv.Itoa(os.Getpid())
	b, err := os.ReadFile(brief)
	if err != nil || os.MkdirAll(filepath.Dir(bf), 0o755) != nil || os.WriteFile(tmp, b, 0o644) != nil || len(b) == 0 {
		os.Remove(tmp)
		die("지시문을 복사하지 못했다: %s → %s", brief, bf)
	}
	if withQ(func() {
		// 착지 뒤 명령 줄을 대기열 줄보다 먼저 — 같은 이름이 이미 있으면 qAdd가 거절하니 그 전에 남의 줄을 바꾸지 않게 여기서도 본다
		if onl != "" && !qHasNameW(name) { // 못 읽으면 남의 착지 뒤 명령을 바꾸기 전에 끝(R-21)
			onlandPut(name, onl, prj, coord)
		}
		prOfAdd(name, prof) // 원래 PR 잇기(prof.go)
		qAdd(tmp, name, prj, bf, after, only, "wait", "-", "-", "-", nowMin(), "-", dash(coord), prio, early, aud, ovk)
	}) != 0 {
		os.Remove(tmp)
		exit(1)
	}
	note := ""
	if after != "-" {
		note = ", 선행 " + after
	}
	if only != "-" {
		note += ", 기계 " + only
	}
	own := coord
	if own == "" {
		own = "없음(모든 지휘)"
	}
	note += ", 주인 " + own + ", 우선 " + plabel(prio)
	if early != "-" {
		note += ", 먼저 띄움 끔"
	}
	if aud != "-" {
		note += ", 자동 감사 끔"
	}
	if onl != "" {
		note += ", 착지 뒤 " + onl
	}
	note += prOfNote(prof)
	switch ovk {
	case "overlap-ok":
		note += ", 겹침 봄 끔"
	case "solo":
		note += ", 단독"
	}
	say("더함 %s (%s%s)", name, prj, note)
	dupWarn(name, prj, bf)
	thenHint(after, prj)
	exit(0)
}

// thenHint <선행 목록> <프로젝트>: 선행이 아직 대기 중인 같은 프로젝트 줄이면 --then을 알리는 한 줄(add의 마지막 단계)
func thenHint(after, prj string) {
	if after == "-" {
		return
	}
	rows := qRows()
	for _, a := range words(after) {
		for _, r := range rows {
			if r.F(qName) == a && r.F(qState) == "wait" && r.F(qProj) == prj && (r.F(qReply) == "" || r.F(qReply) == "-") && !strings.HasPrefix(r.F(qAudit), "audit=") {
				say("알림: %s가 아직 대기 중 — 같은 일의 다음 단계면 --after 대신 --then %s로 한 레인에 이어 붙인다", a, a)
				break
			}
		}
	}
}

// thenAdd: run.lock(run 한 바퀴 내내 — 지시문을 잠금 밖에서 읽고 띄운 뒤에야 up으로 바꾼다)과 대기열 잠금을 쥐고 선행 지시문에 이어 붙인다
func thenAdd(tl, pre, prj, brief string) int {
	if err := os.MkdirAll(DATA, 0o755); err != nil {
		die("%v", err)
	}
	wait := 600
	if v := os.Getenv("HC_FILL_THEN_WAIT"); v != "" {
		wait, _ = strconv.Atoi(v)
	}
	rf := lockWait(DATA+"/run.lock", time.Duration(wait)*time.Second)
	if rf == nil {
		fmt.Fprintln(os.Stderr, "hc-fill: run 바퀴나 대기열 잠금이 끝나지 않아 그만뒀다, 다시 부른다")
		return 75
	}
	defer rf.Close()
	rc := withQ(func() { thenLocked(tl, pre, prj, brief) })
	if rc == 75 {
		fmt.Fprintln(os.Stderr, "hc-fill: run 바퀴나 대기열 잠금이 끝나지 않아 그만뒀다, 다시 부른다")
	}
	return rc
}

func thenLocked(tl, pre, prj, brief string) {
	row := qrow(pre)
	if row == nil {
		if x := thenof(pre); x != "" {
			refuse("%s은 %s에 이어 붙였다 — --then %s", pre, x, x)
		}
		refuse("선행이 대기열에 없다: %s", pre)
	}
	pp, pb, ps, pst, pr, pk := row.F(qProj), row.F(qBrief), row.F(qState), dash(row.F(qStuck)), dash(row.F(qReply)), dash(row.F(qAudit))
	if ps != "wait" {
		refuse("%s: 이미 떴다(%s) — --after %s으로 넣거나 그 레인에 hc send", pre, ps, pre)
	}
	if pst != "-" {
		refuse("%s: 상태 안 잡힘으로 남긴 터미널이 있다(%s) — 정리하고 rm 뒤 다시 add, 또는 --after %s", pre, pst, pre)
	}
	if !strings.HasPrefix(pb, DATA+"/briefs/added/") {
		refuse("%s: 지시문이 사본이 아니다(%s) — rm 뒤 다시 add", pre, pb)
	}
	if pr != "-" {
		refuse("%s: 받은편지 줄에는 이어 붙이지 않는다", pre)
	}
	if strings.HasPrefix(pk, "audit=") {
		refuse("%s: 자동 감사 줄에는 이어 붙이지 않는다", pre)
	}
	if pp != prj {
		refuse("%s: 프로젝트가 다르다(%s)", pre, pp)
	}
	if qHasName(tl) {
		refuse("이미 대기열에 있다: %s", tl)
	}
	if x := thenof(tl); x != "" {
		refuse("%s은 이미 %s에 이어 붙였다", tl, x)
	}
	if why, b := briefBad(brief); b {
		refuse("지시문을 읽지 못함(%s): %s", why, brief)
	}
	old, err1 := os.ReadFile(pb)
	add, err2 := os.ReadFile(brief)
	mid := fmt.Sprintf("\n## 이어서: %s (hc-fill add --then, %s)\n위 일을 끝내 커밋한 뒤 같은 작업 트리에서 이어서 한다. 이 절의 커밋 제목 머리는 `%s:`으로 쓴다(이 이름을 --after로 기다리는 레인이 풀리게).\n위 일의 완료 보고·착지는 하지 않는다 — 이 절까지 끝낸 뒤 한 번만 report 완료.\n",
		tl, nowMin(), tl)
	if err1 != nil || err2 != nil || writeAtomic(pb, string(old)+mid+string(add)) != nil {
		DRC = 1 // 검사 거절(2)이 아니라 쓰기 실패 — 셸 판과 같은 1(Fable L3)
		die("선행 지시문에 붙이지 못했다: %s", pb)
	}
	say("이어 붙임 %s → %s (새 레인 없음, 선행 지시문 %s)", tl, pre, pb)
}

func usage() {
	fmt.Fprint(os.Stderr, usageText)
	next("hc-fill --skill(에이전트 안내 — 흔한 흐름·상태 보기·되돌리기)을 읽고, 위 사용법의 인자만 쓴다") // agent-skill-guides
	oplog.Usage()                                                          // 사용법은 이상이 아니다 — exit 줄을 남기지 않는다
	exit(2)
}

// notCoord <주소>: 그 터미널이 지휘가 아님이 hc get으로 확인됨(레인·감시 라벨, 끝남·not_found). 조회 실패·잘린 응답·라벨 없는 살아 있는
// 터미널은 거짓(모름 — 지휘일 수 있다)
func notCoord(addr string) bool {
	out, rc := Cmd{Name: "hc", Args: []string{"get", addr, "--json"}, Err: errMerge}.Run()
	vs, ok := decodeAll(out)
	if !ok || len(vs) != 1 {
		return false
	}
	if jstr(jget(vs[0], "error", "kind")) == "not_found" {
		return true
	}
	if rc != 0 || jstr(jget(vs[0], "address")) == "" {
		return false
	}
	return jstr(jget(vs[0], "state")) == "finished" || jstr(jget(vs[0], "labels", "hc-lane")) == "1" || jstr(jget(vs[0], "labels", "hc-lane-watch")) == "1"
}
