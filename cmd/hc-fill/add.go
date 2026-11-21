package main

// add(--then 포함) — 셸 판 bin/hc-fill add·_then·_add

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func addCmd(args []string) {
	name, prj, brief, after, only, coord := "", "", "", "-", "-", ""
	cx, prio, early, aud, ovk, thn, xo := false, "normal", "-", "-", "-", "", ""
	for len(args) > 0 {
		a := args[0]
		switch a {
		case "--no-early", "--no-audit", "--overlap-ok", "--solo", "--after", "--only", "--priority", "--coord":
			xo += " " + a // --then과 함께 못 씀
		}
		switch a {
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
	if after != "-" {
		for _, a := range words(after) {
			if !okname(a) {
				die("--after 레인 이름: %s", a)
			}
			if w, k := kindwhy(a); k {
				die("--after: %s", w)
			}
		}
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
	if qAdd(tmp, name, prj, bf, after, only, "wait", "-", "-", "-", nowMin(), "-", dash(coord), prio, early, aud, ovk) != 0 {
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
	switch ovk {
	case "overlap-ok":
		note += ", 겹침 봄 끔"
	case "solo":
		note += ", 단독"
	}
	say("더함 %s (%s%s)", name, prj, note)
	if after != "-" {
		rows := qRows()
		for _, a := range words(after) {
			for _, r := range rows {
				if r.F(1) == a && r.F(6) == "wait" && r.F(2) == prj && (r.F(11) == "" || r.F(11) == "-") && !strings.HasPrefix(r.F(15), "audit=") {
					say("알림: %s가 아직 대기 중 — 같은 일의 다음 단계면 --after 대신 --then %s로 한 레인에 이어 붙인다", a, a)
					break
				}
			}
		}
	}
	exit(0)
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
	pp, pb, ps, pst, pr, pk := row.F(2), row.F(3), row.F(6), dash(row.F(9)), dash(row.F(11)), dash(row.F(15))
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
		die("선행 지시문에 붙이지 못했다: %s", pb)
	}
	say("이어 붙임 %s → %s (새 레인 없음, 선행 지시문 %s)", tl, pre, pb)
}

func usage() {
	fmt.Fprint(os.Stderr, usageText)
	exit(2)
}
