package main

// edit·rm 주차 거절·다른 프로젝트 선행(2026-11-22 fill-edit-after): rm → add는 줄 ID와 주차 표식(14칸 parked — 선행이 들어가면
// refs/park/<레인>에서 --resume으로 이어 띄움)을 버려 먼저 해 둔 준비 커밋 없이 새로 뜨게 했다. edit은 대기열 잠금 안에서 그 줄의
// 선행(4)·기계(5)·지시문(3)·우선(13) 칸만 바꾼다(나머지 칸·줄 ID 그대로).

import (
	"os"
	"strings"
)

func init() {
	register("edit", editCmd)
	register("rm", rmCmd)
}

func editCmd(args []string) {
	DRC = 2 // 검사에 걸리면 거절(종료 2), 아무것도 바꾸지 않음
	if len(args) < 1 || strings.HasPrefix(args[0], "-") {
		usage()
	}
	name, args := args[0], args[1:]
	after, only, brief, prio, anyp := "", "", "", "", false
	for len(args) > 0 {
		if args[0] == "--after-any-project" {
			anyp, args = true, args[1:]
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
	if after == "" && only == "" && brief == "" && prio == "" {
		usage()
	}
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
	st, prj, note := "", "", ""
	rc := withQ(func() {
		r := qrow(name)
		if r == nil {
			die("대기열에 없다: %s", name)
		}
		st, prj = r.F(6), r.F(2)
		if after != "" {
			after = afterFix(name, prj, after, anyp)
		}
		bf := ""
		if brief != "" {
			if st != "wait" {
				die("%s: 이미 떴다(%s) — 떠 있는 레인의 지시문은 바꾸지 않는다(그 레인에 hc send)", name, st)
			}
			if len(thenNames(r.F(3))) > 0 {
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
			if x.F(1) == name {
				for i := x.NF() + 1; i < 13; i++ {
					x = x.Set(i, "-")
				}
				if after != "" {
					x = x.Set(4, after)
					note += ", 선행 " + after
				}
				if only != "" {
					x = x.Set(5, only)
					note += ", 기계 " + only
				}
				if bf != "" {
					x = x.Set(3, bf)
					note += ", 지시문 " + bf
				}
				if prio != "" {
					x = x.Set(13, prio)
					note += ", 우선 " + plabel(prio)
				}
			}
			out = append(out, x)
		}
		if writeAtomic(Q, rowsText(out)) != nil {
			DRC = 1
			die("대기열을 쓰지 못했다: %s", Q)
		}
	})
	if rc != 0 {
		exit(rc)
	}
	say("바꿈 %s (%s)", name, strings.TrimPrefix(note, ", "))
	// 떠 있는 레인: 이미 병합기 큐에 냈으면 그 큐 메타도(prio와 같은 길)
	if prio != "" && st != "wait" && prioMeta(name, prj, plabel(prio)) == 1 {
		say("알림: %s 는 아직 병합기 큐에 안 냈다 — 떠 있는 레인 환경의 HC_LANE_PRIORITY는 띄울 때 값이라, 제출한 뒤 다시 edit --priority 하면 큐 메타를 고친다", name)
	}
}

// rmCmd: rm <이름> [--force]. 주차 중인 줄은 --force 없이 지우지 않는다(종료 2)
func rmCmd(args []string) {
	name, force := "", false
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
	rc := withQ(func() {
		if r := qrow(name); r != nil && r.F(14) == "parked" && !force {
			refuse("%s: 주차 중(refs/park/%s) — 먼저 해 둔 준비 커밋이 있어 지우고 다시 넣으면 새로 뜬다. 선행만 바꿀 거면 hc-fill edit %s --after <선행>, 그래도 지우려면 --force", name, name, name)
		}
		qRm(name)
	})
	if rc != 0 {
		exit(rc)
	}
	say("뺌 %s (떠 있는 터미널은 그대로)", name)
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
		if r.F(1) == n {
			if r.F(2) == prj {
				return prj
			}
			other = r.F(2)
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
