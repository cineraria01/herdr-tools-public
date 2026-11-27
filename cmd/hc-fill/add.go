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
	wt := "" // 무게 18칸(weight.go) — 빈 값이면 17칸 줄(지시문으로 추정)
	for len(args) > 0 {
		a := args[0]
		switch a {
		case "--no-early", "--no-audit", "--overlap-ok", "--solo", "--after", "--only", "--priority", "--coord", "--on-land", "--after-any-project", "--pr-of", "--light", "--weight":
			xo += " " + a // --then과 함께 못 씀
		}
		switch a {
		case "--light":
			wt, args = weightArg(wt, "light", "light"), args[1:]
			continue
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
		case "--weight":
			wt = weightArg(wt, v, "light", "normal")
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
	b, ext := carryPrep(name, prj, b, map[string]bool{}, true) // 지휘 기계 경로를 레인 작업 트리 .qa/… 로(carry.go)
	carryCheck(name, prj, b, ext, true)                        // 실을 자료가 상한을 넘으면 아무것도 쓰기 전에 거절
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
		f := []string{name, prj, bf, after, only, "wait", "-", "-", "-", nowMin(), "-", dash(coord), prio, early, aud, ovk}
		if wt != "" {
			f = append(f, wt)
		}
		qAdd(tmp, f...)
	}) != 0 {
		os.Remove(tmp)
		exit(1)
	}
	carryKeep(name, ext, true)
	say("더함 %s (%s%s)", name, prj, addNote(after, only, coord, prio, early, aud, onl, prof, wt, ovk))
	dupWarn(name, prj, bf)
	siblingNote(name, prj, coord, time.Now())
	thenHint(after, prj)
	if ovk == "-" {
		claimNote(bf)
	}
	exit(0)
}

// claimNote <지시문>: 쓰기 줄 지시문에 값 있는 「파일:」·「쓰기:」 예약 줄(겹침 lkeys와 같은 ovExRe)이 없으면 알림 한 줄 — 막지 않는다
// (2026-11-28 ht-fill-smalls-1128 — 같은 폴더를 고치는 두 레인이 나란히 떠 뒤 것이 합친 뒤 컴파일 실패로 반송됐다, 한쪽에 그 줄이 없었다)
func claimNote(brief string) {
	for _, l := range readLines(brief) {
		if m := ovExRe.FindStringSubmatch(l); m != nil && strings.TrimSpace(m[3]) != "" {
			return
		}
	}
	say("알림: 지시문에 「파일:」 줄이 없다 — 다른 레인과 겹쳐도 기다리지 않는다")
}

// claimNoteEdit <이름> <지시문 바꿈>: edit가 지시문을 바꾼 줄이 겹침을 보는 쓰기 줄(겹침 봄 끔·단독·받은편지·감사 줄 아님)이면 claimNote
func claimNoteEdit(name string, changed bool) {
	r := qrow(name)
	if ov := dash(r.F(qOverlap)); changed && r != nil && ov != "overlap-ok" && ov != "solo" && dash(r.F(qReply)) == "-" && !strings.HasPrefix(r.F(qAudit), "audit=") {
		claimNote(r.F(qBrief))
	}
}

// addNote: 「더함」 줄의 괄호 안(프로젝트 뒤) — 기본값이 아닌 칸만
func addNote(after, only, coord, prio, early, aud, onl, prof, wt, ovk string) string {
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
	if wt != "" {
		note += ", 무게 " + weightLabel[wt]
	}
	switch ovk {
	case "overlap-ok":
		note += ", 겹침 봄 끔"
	case "solo":
		note += ", 단독"
	}
	return note
}

// siblingNote <레인> <프로젝트> <넣는 지휘> <지금>: 같은 프로젝트에 다른 지휘가 손잡이 sibling_min분 안에 넣은 안 끝난 쓰기 줄(대기·떠 있음·감사 중 —
// 마지막 사건이 착지·정리됨이면 끝남 — 제출은 착지 전이라 보인다, dupDone과 다름)마다 한 줄. 넣기는 막지 않는다 — 같은 알림을 지휘 둘이 각자 받아 같은 일을 따로 넣은 일 뒤(2026-11-25)
func siblingNote(name, prj, coord string, now time.Time) {
	lim := knobInt("sibling_min")
	if lim <= 0 {
		return
	}
	ended := map[string]bool{}
	for _, x := range readRows(DATA + "/last.tsv") {
		if s := x.F(lsState); s == "착지" || s == "정리됨" {
			ended[x.F(lsName)+"\t"+x.F(lsRID)] = true
		}
	}
	for _, r := range qRows() {
		n, own, st := r.F(qName), dash(r.F(qOwner)), r.F(qState)
		if n == name || r.F(qProj) != prj || own == "-" || own == coord || strings.HasPrefix(r.F(qAudit), "audit=") || st != "wait" && st != "up" {
			continue
		}
		if ended[n+"\t"] || ended[n+"\t"+r.F(qRID)] {
			continue
		}
		age, ok := stateAddedAge(r.F(qAdded), now)
		if !ok || age > lim*60 {
			continue
		}
		say("알림: 같은 프로젝트에 다른 지휘가 막 넣은 레인 — 같은 일이면 합쳐라: %s (지휘 %s, %d분 전) — %s", n, own, max(age, 0)/60, briefTitle(r.F(qBrief)))
	}
}

// briefTitle: 지시문의 빈 줄 아닌 첫 줄(머리 # 뗌)
func briefTitle(brief string) string {
	for _, l := range readLines(brief) {
		if t := strings.TrimSpace(strings.TrimLeft(l, "#")); t != "" {
			return t
		}
	}
	return "(제목 없음)"
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
	add, ext := carryPrep(pre, prj, add, carryTaken(pre), true) // 사본 폴더는 선행 레인 것(carry.go)
	mid := fmt.Sprintf("\n## 이어서: %s (hc-fill add --then, %s)\n위 일을 끝내 커밋한 뒤 같은 작업 트리에서 이어서 한다. 이 절의 커밋 제목 머리는 `%s:`으로 쓴다(이 이름을 --after로 기다리는 레인이 풀리게).\n위 일의 완료 보고·착지는 하지 않는다 — 이 절까지 끝낸 뒤 한 번만 report 완료.\n",
		tl, nowMin(), tl)
	carryCheck(pre, prj, []byte(string(old)+mid+string(add)), ext, false) // 띄울 때 실릴 범위 그대로(앞 단계 + 이어 붙일 것)
	if err1 != nil || err2 != nil || writeAtomic(pb, string(old)+mid+string(add)) != nil {
		DRC = 1 // 검사 거절(2)이 아니라 쓰기 실패 — 셸 판과 같은 1(Fable L3)
		die("선행 지시문에 붙이지 못했다: %s", pb)
	}
	carryKeep(pre, ext, false)
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
