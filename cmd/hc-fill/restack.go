package main

// 다시 쌓기(docs/hc-fill/stack.md 「다시 쌓기 판정」): 병합기가 쌓인 제출을 선행 반송·거둠으로 refs/merge-parked/<이름>에 주차하면,
// 바퀴마다 선행을 보고 — 선행이 새 승인 커밋 P'로 줄에 다시 섰으면 P' 위로, 선행이 들었거나 대기열에서 빠졌으면 기본 가지 위로 —
// 자기 커밋만(P..제출) 옮긴다. 깨끗하고 patch-id가 승인과 같으면 승인 그대로 새 큐 이름으로 다시 넣고(병합기가 그 patch-id를 다시
// 대조한다 — 관문은 그대로), 충돌·patch-id 다름이면 주차 참조를 refs/hc-restack/<이름>으로 옮기고 그 레인을 반송 다시 띄움 길로.

import (
	"fmt"
	"io"
	"os"
	"strings"

	"herdr-tools/internal/gitx"
)

func stackTick() {
	for _, r := range qRows() {
		if r.F(qState) != "up" || rowMode(r) != mStacked {
			continue
		}
		name, prj := r.F(qName), r.F(qProj)
		rec := stackRec(name)
		if rec == nil || lcFetch(prj, true) != 0 {
			continue
		}
		m, ok := stackQueue(FR)
		if !ok {
			say("다시 쌓기 보류 %s: 원격 큐를 읽지 못함", name)
			continue
		}
		pn := stackQName(m, "refs/merge-parked/", name)
		if pn == "" {
			continue
		}
		if rec.F(skState) != "parked" {
			say("쌓아 띄움 주차 %s: 선행 %s 반송·거둠으로 병합기가 주차(refs/merge-parked/%s) — 선행이 다시 서면 다시 쌓음", name, rec.F(skPred), pn)
			stackPut(name, rec.Set(skState, "parked").Set(skAt, utcnow()))
		}
		restackOne(r, stackRec(name), m, pn)
	}
}

// restackOne: 주차된 제출 하나의 다음 기준을 정해 옮긴다
func restackOne(r, rec Row, m map[string]string, pn string) {
	name, pred, p := r.F(qName), rec.F(skPred), rec.F(skP)
	onto, stack, why := "", "", ""
	if qn := stackQName(m, "refs/merge-queue/", pred); qn != "" {
		kv := stackMetaKV(FR, "refs/merge-queue-meta/"+qn, pred)
		if kv == nil || kv["verdict"] == "" || kv["orig"] != m["refs/merge-queue/"+qn] || m["refs/heads/lane/"+pred] != kv["orig"] {
			return // 선행이 줄에 섰지만 감사 통과 제출이 아직 아님 — 기다림
		}
		onto, stack, why = kv["orig"], pred+"@"+kv["orig"], "선행 "+pred+" 새 제출 "+head(kv["orig"], 8)
	} else if landingHit(pred) == 0 {
		onto, why = FB, "선행 "+pred+" 들어감"
	} else if qrow(pred) == nil {
		onto, why = FB, "선행 "+pred+" 이 대기열에서 빠짐"
	} else {
		return // 선행이 고치는 중 — 다음 바퀴에
	}
	if dry {
		say("%s다시 쌓을 %s: %s 위로(%s)", P, name, head(onto, 12), why)
		return
	}
	st := restackMove(name, pn, p, onto, stack, m)
	switch st {
	case "ok":
		ns := "restacked"
		if stack == "" {
			ns = "rebased-main"
		}
		np := p
		if stack != "" {
			np = onto
		}
		stackPut(name, rec.Set(skP, np).Set(skState, ns).Set(skAt, utcnow()))
		say("다시 쌓음 %s: %s — 자기 커밋만 옮겨 승인 그대로 줄에", name, why)
		jot("stack", name, why, "다시 쌓음 → "+head(onto, 12), "-", "-")
	case "relaunch":
		stackPut(name, rec.Set(skState, "relaunch").Set(skAt, utcnow()))
		jot("stack", name, why, "다시 쌓기 충돌·승인 다름 → 다시 띄움", "-", "-")
	default:
		return
	}
	if strings.HasSuffix(why, "대기열에서 빠짐") {
		hcLaneReport("중간", "쌓아 띄움 "+name+": "+why+" — 기본 가지 위로 옮김("+st+")", name, dash(r.F(qOwner)), "hf:stack:"+pn)
	}
}

// restackMove: 주차된 제출(P..S)을 onto 위로 옮긴다 — "ok" 다시 넣음 · "relaunch" 다시 띄움으로 넘김 · "" 모름(다음 바퀴)
func restackMove(name, pn, p, onto, stack string, m map[string]string) string {
	repo := FR
	sq, sm := "refs/merge-parked/"+pn, "refs/merge-parked-meta/"+pn
	s, ms := m[sq], m[sm]
	g := func(a ...string) (string, int) {
		o, rc := Cmd{Name: "git", Args: append([]string{"-C", repo}, a...), Err: errDrop}.Run()
		return sub(o), rc
	}
	if ms == "" {
		say("다시 쌓기 보류 %s: 주차 기록 %s 가 없다", name, sm)
		return ""
	}
	if _, rc := g("fetch", "-q", "origin", "+"+sq+":refs/hc-fill/park/"+pn, "+"+sm+":refs/hc-fill/parkm/"+pn); rc != 0 {
		return ""
	}
	if stack != "" {
		if _, rc := g("fetch", "-q", "origin", "+refs/heads/lane/"+strings.SplitN(stack, "@", 2)[0]+":refs/hc-fill/onto/"+name); rc != 0 {
			return ""
		}
	}
	msg, rc := g("log", "-1", "--format=%s", ms)
	if rc != 0 {
		return ""
	}
	if onto, rc = g("rev-parse", "--verify", onto+"^{commit}"); rc != 0 { // 기록 fork= 은 40자로
		return ""
	}
	want := ""
	for _, f := range strings.Fields(msg) {
		if v, ok := strings.CutPrefix(f, "patch="); ok {
			want = v
		}
	}
	tmp, err := os.MkdirTemp("", "hc-fill-restack-")
	if err != nil {
		return ""
	}
	defer os.RemoveAll(tmp)
	wt := tmp + "/wt"
	if _, rc := g("worktree", "add", "-q", "--detach", wt, s); rc != 0 {
		return ""
	}
	defer g("worktree", "remove", "--force", wt)
	if _, rc := (Cmd{Name: "git", Args: []string{"-C", wt, "rebase", "-q", "--onto", onto, p}, Err: errDrop}).Run(); rc != 0 {
		Cmd{Name: "git", Args: []string{"-C", wt, "rebase", "--abort"}, Err: errDrop}.Run()
		return restackRelaunch(name, pn, p, onto, s, ms, "선행 새 기준 위로 옮기면 충돌")
	}
	s2, rc := g("-C", wt, "rev-parse", "HEAD")
	if rc != 0 {
		return ""
	}
	got, rc := gitx.PatchIDIn(repo, io.Discard, onto, s2)
	if rc != 0 {
		return ""
	}
	if want != "" && got != want {
		return restackRelaunch(name, pn, p, onto, s, ms, "옮긴 변경의 patch-id가 승인과 다름")
	}
	var kv []string
	for _, f := range strings.Fields(msg) {
		switch k, _, _ := strings.Cut(f, "="); k {
		case "orig":
			f = "orig=" + s2
		case "fork":
			f = "fork=" + onto
		case "stack":
			continue
		}
		kv = append(kv, f)
	}
	if stack != "" {
		kv = append(kv, "stack="+stack)
	}
	et, rc := g("hash-object", "-t", "tree", "-w", "/dev/null")
	if rc != 0 {
		return ""
	}
	m2, rc := g("commit-tree", et, "-p", s2, "-m", strings.Join(kv, " "))
	if rc != 0 || !gitx.ObjID(m2) {
		return ""
	}
	n2 := rqName(name, m)
	lane := "refs/heads/lane/" + name
	a := []string{s2 + ":refs/merge-queue/" + n2, m2 + ":refs/merge-queue-meta/" + n2, s2 + ":" + lane, ":" + sq, ":" + sm,
		"--force-with-lease=refs/merge-queue/" + n2 + ":", "--force-with-lease=refs/merge-queue-meta/" + n2 + ":",
		"--force-with-lease=" + lane + ":" + m[lane], "--force-with-lease=" + sq + ":" + s, "--force-with-lease=" + sm + ":" + ms}
	if _, rc := g(append([]string{"push", "-q", "--atomic", "origin"}, a...)...); rc != 0 {
		say("다시 쌓기 보류 %s: push 거절·실패 — 다음 바퀴에 원격을 다시 본다", name)
		return ""
	}
	return "ok"
}

// restackRelaunch: 주차 참조를 refs/hc-restack/<이름>으로 옮기고(다음 바퀴가 다시 보지 않게) 반송 다시 띄움 길로 — 절을 쓰고 줄을 bounced 대기로
func restackRelaunch(name, pn, p, onto, s, ms, why string) string {
	sq, sm := "refs/merge-parked/"+pn, "refs/merge-parked-meta/"+pn
	rq := "refs/hc-restack/" + pn
	a := []string{s + ":" + rq, ":" + sq, ":" + sm, "--force-with-lease=" + sq + ":" + s, "--force-with-lease=" + sm + ":" + ms}
	if _, rc := (Cmd{Name: "git", Args: append([]string{"-C", FR, "push", "-q", "--atomic", "origin"}, a...), Err: errDrop}).Run(); rc != 0 {
		return ""
	}
	sec := fmt.Sprintf(`## 다시 쌓기 — 선행이 바뀌어 이 레인의 제출을 새 기준 위로 옮겨야 한다 (hc-fill)
이 레인은 선행 위에 쌓여 감사를 통과해 제출됐는데 선행이 반송·거둠으로 바뀌어 병합기가 주차했다. hc-fill이 자기 커밋을 새 기준 %[2]s 위로 옮기려니 %[3]s — 같은 이름으로 다시 띄웠다.
1. 이 작업 트리를 주차된 제출로 옮긴다: git fetch origin %[4]s 뒤 git reset --keep FETCH_HEAD
2. 자기 커밋만 새 기준 위로: git rebase --onto origin/%[5]s %[1]s —충돌은 양쪽 내용을 살려 풀고 고친 파일을 경로로 git add 한 뒤 git rebase --continue. stash·rebase --skip·--abort 금지.
3. 저장소 검증 뒤 위 감사·착지 절차대로 다시 낸다(hc-lane report 완료 → 감사 판정 → land). 다시 주차하지 않는다.
`, p, head(onto, 12), why, rq, strings.TrimPrefix(strings.TrimPrefix(FB, "origin/"), "refs/heads/"))
	if os.MkdirAll(DATA+"/bounce", 0o755) != nil || writeAtomic(bounceFile(name), sec) != nil {
		return ""
	}
	q := qrow(name)
	if q == nil || qset(true, q.F(qRID), name, "wait", "-", "-", q.F(qStuck), mBounced.val(), "") != 0 {
		return ""
	}
	say("다시 쌓기 %s: %s — 반송 다시 띄움 대기로(%s)", name, why, rq)
	return "relaunch"
}
