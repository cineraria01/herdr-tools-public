package main

// edit·rm 주차 거절·다른 프로젝트 선행(2026-11-22 fill-edit-after): rm → add는 줄 ID와 주차 표식(14칸 parked — 선행이 들어가면
// refs/park/<레인>에서 --resume으로 이어 띄움)을 버려 먼저 해 둔 준비 커밋 없이 새로 뜨게 했다. edit은 대기열 잠금 안에서 그 줄의
// 선행(4)·기계(5)·지시문(3)·우선(13) 칸만 바꾼다(나머지 칸·줄 ID 그대로).
// 겹침칸(16)·먼저 띄움(14)(2026-11-24 fill-edit-hands): 지휘가 겹침 검사를 끄려고 rm 뒤 add --overlap-ok로 다시 넣었다(줄 ID가 바뀌고,
// 뜨기 직전이면 run과 경합). 아직 안 뜬 줄(대기·주차)만 — 뜬 줄에는 뜻이 없다(겹침은 띄울 때 한 번 보고, 먼저 띄움은 띄울지의 판단).

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func init() {
	register("edit", editCmd)
	register("rm", rmCmd)
}

func editCmd(args []string) {
	DRC = 2 // 검사에 걸리면 거절(종료 2), 아무것도 바꾸지 않음
	args, why := takeWhy(args)
	if len(args) < 1 || strings.HasPrefix(args[0], "-") {
		usage()
	}
	name, args := args[0], args[1:]
	// prof: 원래 PR(--pr-of, -는 지움 — prof.go)
	after, only, brief, prio, anyp, prof := "", "", "", "", false, ""
	ovk, early := "", "" // 바꿀 겹침칸(overlap-ok·-)·먼저 띄움 칸(-·no-early), 빈 값은 그대로
	wt := ""             // 바꿀 무게 18칸(light·normal·- = 지시문으로 추정, weight.go)
	for len(args) > 0 {
		flag := func(cur *string, v string) {
			if *cur != "" && *cur != v {
				die("%s: 반대 인자와 함께 쓰지 않는다", args[0])
			}
			*cur, args = v, args[1:]
		}
		switch args[0] {
		case "--after-any-project":
			anyp, args = true, args[1:]
			continue
		case "--overlap-ok":
			flag(&ovk, "overlap-ok")
			continue
		case "--overlap-check":
			flag(&ovk, "-")
			continue
		case "--early":
			flag(&early, "-")
			continue
		case "--no-early":
			flag(&early, "no-early")
			continue
		case "--light":
			wt, args = weightArg(wt, "light", "light"), args[1:]
			continue
		}
		if len(args) < 2 {
			usage()
		}
		v := args[1]
		switch args[0] {
		case "--after":
			after = v
		case "--only":
			only = v
		case "--brief":
			brief = v
		case "--pr-of":
			prof = v
		case "--weight": // -는 지시문으로 추정
			wt = weightArg(wt, v, "light", "normal", "-")
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
	if after == "" && only == "" && brief == "" && prio == "" && ovk == "" && early == "" && prof == "" && wt == "" {
		usage()
	}
	prOfCheck(name, prof)
	if anyp && after == "" {
		die("--after-any-project는 --after와 함께 쓴다")
	}
	if only != "" && only != "-" {
		for _, m := range words(only) {
			if !machineKnown(m) {
				die("설정에 machine 줄이 없는 기계: %s", m)
			}
		}
	}
	var bt []byte
	if brief != "" {
		if !strings.HasPrefix(brief, "/") {
			wd, _ := os.Getwd()
			brief = wd + "/" + brief
		}
		if why, b := briefBad(brief); b {
			die("지시문을 읽지 못함(%s): %s", why, brief)
		}
		var err error
		if bt, err = os.ReadFile(brief); err != nil {
			die("지시문을 읽지 못함: %s", brief)
		}
	}
	st, prj, note, undo := "", "", "", ""
	rc := withRun(func() {
		r := qrow(name)
		if r == nil {
			noLane(name)
		}
		undo = editUndo(r, after != "", only != "", brief != "", prio != "", ovk != "", early != "", wt != "")
		if prof != "" {
			old := prOfGet(name)
			if undo == "-" {
				undo = "hc-fill edit " + name
			}
			undo += " --pr-of " + shq(dash(old))
		}
		r = editParked(r, brief != "" || ovk != "" || early != "" || wt != "") // 주차를 보고하고 끝난 줄은 바퀴 전에도 주차로(editparked.go)
		st, prj = r.F(qState), r.F(qProj)
		if ovk != "" || early != "" || wt != "" { // 무게도 띄울 곳을 고를 때 한 번 본다 — 아직 안 뜬 줄만
			editNotUp(r, ovk != "", early != "")
		}
		if after != "" {
			after = afterFix(name, prj, after, anyp)
		}
		bf := ""
		if brief != "" {
			if st != "wait" {
				die("%s: 이미 떴다(%s) — 떠 있는 레인의 지시문은 바꾸지 않는다(그 레인에 hc send)", name, st)
			}
			if len(thenNames(r.F(qBrief))) > 0 {
				die("%s: 지시문에 「## 이어서:」(--then으로 붙인 일)가 있다 — 바꾸면 그 일을 잃는다. 새 지시문에 그 절까지 넣어 rm --force 뒤 다시 add", name)
			}
			bf = DATA + "/briefs/added/" + name + ".md"
			if os.MkdirAll(DATA+"/briefs/added", 0o755) != nil || writeAtomic(bf, string(bt)) != nil {
				DRC = 1
				die("지시문을 복사하지 못했다: %s → %s", brief, bf)
			}
		}
		var out []Row
		for _, x := range qRowsW() {
			if x.F(qName) == name {
				for i := x.NF() + 1; i < 13; i++ {
					x = x.Set(i, "-")
				}
				if after != "" {
					x = x.Set(qAfter, after)
					note += ", 선행 " + after
				}
				if only != "" {
					x = x.Set(qOnly, only)
					note += ", 기계 " + only
				}
				if bf != "" {
					x = x.Set(qBrief, bf)
					note += ", 지시문 " + bf
				}
				if prio != "" {
					x = x.Set(qPrio, prio)
					note += ", 우선 " + plabel(prio)
				}
				if ovk != "" || early != "" {
					for i := x.NF() + 1; i < qRID; i++ { // 16칸까지 채움(우선은 기본 보통) — 17칸 줄 ID는 있는 그대로
						v := "-"
						if i == qPrio {
							v = "normal"
						}
						x = x.Set(i, v)
					}
				}
				if ovk != "" {
					x = x.Set(qOverlap, ovk)
					note += ", 겹침 " + map[string]string{"-": "봄", "overlap-ok": "봄 끔"}[ovk]
				}
				if early != "" {
					x = x.Set(qEarly, early)
					note += ", 먼저 띄움 " + map[string]string{"-": "켬", "no-early": "끔"}[early]
				}
				if wt != "" {
					x = setWeight(x, wt)
					note += ", 무게 " + weightLabel[wt]
				}
			}
			out = append(out, x)
		}
		if writeAtomic(Q, rowsText(out)) != nil {
			DRC = 1
			die("대기열을 쓰지 못했다: %s", Q)
		}
		if prof != "" {
			if err := prOfPut(name, prof); err != nil {
				DRC = 1
				die("%v", err)
			}
			note += ", 원래 PR " + prof
		}
	})
	if rc != 0 {
		exit(rc)
	}
	say("바꿈 %s (%s)", name, strings.TrimPrefix(note, ", "))
	jot("edit", name, why, "됨", undo, "상태 "+st+" · "+strings.TrimPrefix(note, ", "))
	if rejDrop(name) > 0 { // 띄우기 거절로 멈춘 줄은 고친 뒤 다시 시도(reject.go)
		say("다시 시도 %s — 띄우기 거절 멈춤을 풀었다", name)
	}
	// 떠 있는 레인: 이미 병합기 큐에 냈으면 그 큐 메타도(prio와 같은 길)
	if prio != "" && st != "wait" && prioMeta(name, prj, plabel(prio)) == 1 {
		say("알림: %s 는 아직 병합기 큐에 안 냈다 — 떠 있는 레인 환경의 HC_LANE_PRIORITY는 띄울 때 값이라, 제출한 뒤 다시 edit --priority 하면 큐 메타를 고친다", name)
	}
}

// withRun: run.lock(run 한 바퀴 내내 쥔다 — 줄·지시문을 잠금 밖에서 읽어 사본을 뜨고, 띄운 뒤에야 up으로 바꾼다)과 대기열 잠금 안에서 fn.
// edit가 대기열 잠금만 쥐면 발사 중(사본을 뜬 뒤·up 전)에 선행·지시문을 바꿔, 대기열은 새 지시문인데 레인은 옛 사본으로 떴다
// (재감사2 fill/R-208). --then(thenAdd)과 같은 잠금 차례·기다림 상한(HC_FILL_THEN_WAIT 초, 기본 600). 75 기다림 넘김 — 아무것도 안 바꿈
func withRun(fn func()) int {
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
	rc := withQ(fn)
	if rc == 75 {
		fmt.Fprintln(os.Stderr, "hc-fill: run 바퀴나 대기열 잠금이 끝나지 않아 그만뒀다, 다시 부른다")
	}
	return rc
}

// rmCmd: rm <이름> [--force]. 주차 중인 줄은 --force 없이 지우지 않는다(종료 2)
func rmCmd(args []string) {
	args, dryR := takeDry(args)
	args, why := takeWhy(args)
	name, force, st, undo := "", false, "", "-"
	for _, a := range args {
		switch {
		case a == "--force":
			force = true
		case name == "" && !strings.HasPrefix(a, "-"):
			name = a
		default:
			usage()
		}
	}
	if name == "" {
		usage()
	}
	if dryR {
		rmPreview(name, force)
		exit(0)
	}
	rc := withQ(func() {
		r := qrow(name)
		if r != nil && r.F(qEarly) == "parked" && !force {
			refuse("%s: 주차 중(refs/park/%s) — 먼저 해 둔 준비 커밋이 있어 지우고 다시 넣으면 새로 뜬다. 선행만 바꿀 거면 hc-fill edit %s --after <선행>, 그래도 지우려면 --force", name, name, name)
		}
		if r != nil {
			st, undo = r.F(qState), rmUndo(r)
		}
		qRm(name)
	})
	if rc != 0 {
		exit(rc)
	}
	say("뺌 %s (떠 있는 터미널은 그대로)", name)
	prCloseReq(name, "hc-fill rm "+why) // 열린 PR이 있으면 다음 바퀴에 까닭 댓글 뒤 닫는다(pr.go)
	jot("rm", name, why, "됨", undo, "상태 "+st)
}

// editNotUp: 겹침칸·먼저 띄움을 바꿀 수 있는 줄인가(아니면 거절 — DRC). 대기(wait)·주차만, 감사 줄·단독 줄·주차 줄의 먼저 띄움은 아님
func editNotUp(r Row, ov, ea bool) {
	n, st := r.F(qName), r.F(qState)
	switch {
	case st == "sent":
		die("%s: 이미 끝났다(되돌려줌) — 바꿀 것이 없다", n)
	case st != "wait":
		die("%s: 이미 떴다(%s) — 떠 있는 레인의 겹침 검사·먼저 띄움은 띄울 때 한 번 본 것이라 바꾸지 않는다", n, st)
	case strings.HasPrefix(r.F(qAudit), "audit="):
		die("%s: 감사 줄이다 — 겹침 검사·먼저 띄움을 쓰지 않는다", n)
	case ov && r.F(qOverlap) == "solo":
		die("%s: 단독(--solo) 줄이다 — 겹침 검사를 따로 바꾸지 않는다. 바꾸려면 rm 뒤 다시 add", n)
	case ea && r.F(qEarly) == "parked":
		die("%s: 주차 중(refs/park/%s) — 이미 먼저 띄워 준비 커밋이 있다. 먼저 띄움 칸을 바꾸면 이어 띄우기를 잃는다", n, n)
	}
}

// editUndo: 바꾸기 전 줄 r에서, 바꾼 칸만 옛 값으로 되돌리는 edit 한 줄(지시문은 옛 사본 경로 — 같은 경로면 덮였으니 뺀다)
func editUndo(r Row, a, o, b, p, ov, ea, w bool) string {
	s := "hc-fill edit " + r.F(qName)
	if a {
		s += " --after " + shq(dash(r.F(qAfter)))
	}
	if o {
		s += " --only " + shq(dash(r.F(qOnly)))
	}
	if b && r.F(qBrief) != DATA+"/briefs/added/"+r.F(qName)+".md" {
		s += " --brief " + shq(r.F(qBrief))
	}
	if p {
		s += " --priority " + plabel(r.F(qPrio))
	}
	if ov {
		s += map[bool]string{true: " --overlap-ok", false: " --overlap-check"}[r.F(qOverlap) == "overlap-ok"]
	}
	if ea {
		s += map[bool]string{true: " --no-early", false: " --early"}[r.F(qEarly) == "no-early"]
	}
	if w {
		s += " --weight " + dash(r.F(qWeight))
	}
	if s == "hc-fill edit "+r.F(qName) {
		return "-"
	}
	return s
}

// rmUndo: 뺀 줄을 다시 넣는 add 한 줄(새 줄 ID — 주차한 준비 커밋·시도 기록은 돌아오지 않는다)
func rmUndo(r Row) string {
	s := "hc-fill add --name " + r.F(qName) + " --project " + shq(r.F(qProj)) + " --brief " + shq(r.F(qBrief)) + " --priority " + plabel(r.F(qPrio))
	if a := dash(r.F(qAfter)); a != "-" {
		s += " --after " + shq(a)
	}
	if o := dash(r.F(qOnly)); o != "-" {
		s += " --only " + shq(o)
	}
	if c := dash(r.F(qOwner)); c != "-" {
		s += " --coord " + shq(c)
	}
	if o := prOfGet(r.F(qName)); o != "" {
		s += " --pr-of " + shq(o)
	}
	if w := r.F(qWeight); w == "light" || w == "normal" {
		s += " --weight " + w
	}
	return s
}

// afterFix <레인> <프로젝트> <선행 목록> <다른 프로젝트 허용>: 선행 이름을 검사하고(잘못이면 die) 고친 목록. 다른 프로젝트 레인
// (그 프로젝트 대기열에 있거나 그 main에 착지)은 착지를 이 프로젝트 main에서만 찾아 영원히 「아직」이라 거절 —
// 허용이면 <이름>@<프로젝트>로 적어 그 프로젝트 main에서 찾게. 어디에도 없으면 알림만(손으로 띄운 레인일 수 있다)
func afterFix(lane, prj, list string, anyp bool) string {
	if list == "-" {
		return "-"
	}
	var out []string
	for _, a := range words(list) {
		n, op, at := strings.Cut(a, "@")
		if !okname(n) || (at && !projectKnown(op)) {
			die("--after 레인 이름: %s", a)
		}
		if w, k := kindwhy(n); k {
			die("--after: %s", w)
		}
		if !at {
			switch op = afterProj(n, prj); {
			case op == "":
				say("알림: 선행 %s 는 %s 대기열·착지 기록에 없다 — 손으로 띄운 같은 프로젝트 레인이면 그대로 기다린다", n, prj)
			case op == prj:
			case anyp:
				a = n + "@" + op
			default: // 이름 검사(die — add는 종료 1)와 달리 거절은 늘 종료 2
				refuse("--after %s: %s 프로젝트 레인이다 — 착지를 이 프로젝트(%s) main에서만 찾아 %s 는 영원히 「아직」이 된다. 정말 기다리려면 --after-any-project(%s@%s로 적고 그 프로젝트 main에서 찾음)",
					n, op, prj, lane, n, op)
			}
		}
		out = append(out, a)
	}
	return strings.Join(out, ",")
}

// afterProj <선행> <프로젝트>: 그 선행이 속한 프로젝트 — 같은 프로젝트 대기열·다른 프로젝트 대기열·이 프로젝트 main·다른 프로젝트 main 차례(없으면 빈 값)
func afterProj(n, prj string) string {
	other := ""
	for _, r := range qRows() {
		if r.F(qName) == n {
			if r.F(qProj) == prj {
				return prj
			}
			other = r.F(qProj)
		}
	}
	if other != "" {
		return other
	}
	if landedIn(prj, n) {
		return prj
	}
	seen := map[string]bool{prj: true}
	for _, f := range confRows("repo") {
		if p := F1(f, 1); len(f) >= 3 && f[2] == LOCAL && !seen[p] {
			seen[p] = true
			if landedIn(p, n) {
				return p
			}
		}
	}
	return ""
}

// landedIn <프로젝트> <레인>: 이 기계 그 프로젝트 저장소의 기본 가지(fetch 없이 지금 참조)에 제목 머리가 그 레인인 커밋이 있으면 참
func landedIn(p, lane string) bool {
	repo := repoOf(p, LOCAL)
	if repo == "" {
		return false
	}
	ref, ok := projectRef(repo)
	if !ok {
		return false
	}
	ofr, ofb := FR, FB
	FR, FB = repo, ref
	h, ok := landed(lane)
	FR, FB = ofr, ofb
	return ok && h != ""
}
