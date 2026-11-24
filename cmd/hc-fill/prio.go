package main

// prio의 병합기 큐 메타 고치기(2026-11-22 fill-prio-pass): 이미 낸 레인의 refs/merge-queue-meta/<초>-<레인> 제목 priority=만 바꾼다.
// 커밋 객체를 글 그대로 다시 써(머리 칸 — 트리·부모·작성자·날짜 — 와 나머지 글 그대로) lease로 push한다.

import (
	"regexp"
	"strings"
)

func init() { register("prio", prioCmd) }

func prioCmd(args []string) {
	args, dryP := takeDry(args)
	args, why := takeWhy(args)
	if len(args) != 2 {
		usage()
	}
	p, ok := pnorm(args[1])
	if !ok {
		die("우선순위는 긴급|높음|보통|낮음(urgent|high|normal|low): %s", args[1])
	}
	if dryP {
		prioPreview(args[0], p)
		exit(0)
	}
	st, prj, op := "", "", ""
	rc := withQ(func() {
		rows := qRowsW() // 못 읽으면 「없다」가 아니라 종료 1(R-29)
		for _, x := range rows {
			if x.F(1) == args[0] {
				st, prj, op = x.F(6), x.F(2), x.F(13)
				break
			}
		}
		if st == "" {
			noLane(args[0])
		}
		var out []Row
		for _, x := range rows {
			if x.F(1) == args[0] {
				for i := x.NF() + 1; i < 13; i++ {
					x = x.Set(i, "-")
				}
				x = x.Set(13, p)
			}
			out = append(out, x)
		}
		if writeAtomic(Q, rowsText(out)) != nil {
			exit(1)
		}
	})
	if rc != 0 {
		exit(1)
	}
	say("우선 %s → %s", args[0], plabel(p))
	jot("prio", args[0], why, "됨", "hc-fill prio "+args[0]+" "+plabel(op), "상태 "+st+" · 우선 "+plabel(op)+" → "+plabel(p))
	// 떠 있는 레인: 이미 병합기 큐에 냈으면 그 큐 메타도(2026-11-22 fill-prio-pass — 지휘가 메타 제목을 손으로 고쳤다)
	if st != "wait" && prioMeta(args[0], prj, plabel(p)) == 1 {
		say("알림: %s 는 아직 병합기 큐에 안 냈다 — 떠 있는 레인 환경의 HC_LANE_PRIORITY는 띄울 때 값이라, 제출한 뒤 다시 prio 하면 큐 메타를 고친다", args[0])
	}
}

// prioMeta <레인> <프로젝트> <우선 라벨>: 0 바꿈(같은 값이면 그대로) · 1 큐에 낸 것이 없음 · 2 못 바꿈(경고만 — 그대로 둠)
// ponytail: 병합기가 그 항목을 착지하는 바로 그 사이에 바꾸면 병합기 정리가 메타를 못 지운다(다음 바퀴 일지 판정) — 지휘가 손으로 하던 것과 같은 창
func prioMeta(lane, prj, pl string) int {
	repo := repoOf(prj, LOCAL)
	if repo == "" {
		say("경고: %s 큐 메타: 이 기계(%s)에 %s 저장소가 설정에 없다 — 큐 메타를 못 봄", lane, LOCAL, prj)
		return 2
	}
	o, rc := run("git", "-C", repo, "ls-remote", "origin", "refs/merge-queue-meta/*")
	if rc != 0 {
		say("경고: %s 큐 메타: 원격을 못 읽었다(git ls-remote) — 그대로 둠", lane)
		return 2
	}
	re := regexp.MustCompile(`^refs/merge-queue-meta/[0-9]+-` + regexp.QuoteMeta(lane) + `$`)
	res := 1
	for _, l := range lines(o) {
		f := strings.Fields(l)
		if len(f) != 2 || !re.MatchString(f[1]) {
			continue
		}
		if r := prioMetaOne(repo, f[1], f[0], pl); res == 1 || r == 2 {
			res = r
		}
	}
	return res
}

func prioMetaOne(repo, ref, old, pl string) int {
	if _, rc := run("git", "-C", repo, "fetch", "-q", "--no-write-fetch-head", "origin", ref); rc != 0 {
		say("경고: %s 를 못 받았다(git fetch) — 그대로 둠", ref)
		return 2
	}
	obj, rc := run("git", "-C", repo, "cat-file", "commit", old)
	head, msg, ok := strings.Cut(obj, "\n\n")
	if rc != 0 || !ok {
		say("경고: %s 커밋을 못 읽었다 — 그대로 둠", ref)
		return 2
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
		return 0
	}
	if nl {
		ns += "\n" + rest
	}
	nw, rc := Cmd{Name: "git", Args: []string{"-C", repo, "hash-object", "-t", "commit", "-w", "--stdin"},
		In: strings.NewReader(head + "\n\n" + ns)}.Run()
	nw = strings.TrimSpace(nw)
	if rc != 0 || nw == "" {
		say("경고: %s 새 메타 커밋을 못 만들었다 — 그대로 둠", ref)
		return 2
	}
	if _, rc := run("git", "-C", repo, "push", "-q", "--porcelain", "origin", nw+":"+ref, "--force-with-lease="+ref+":"+old); rc != 0 {
		say("경고: %s 를 못 바꿨다(push 거절 — 그 사이 병합기가 바꿨거나 지웠다, lease %.8s) — 그대로 둠", ref, old)
		return 2
	}
	say("큐 메타 %s → priority=%s", ref, pl)
	return 0
}
