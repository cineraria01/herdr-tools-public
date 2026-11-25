package main

// 사람 리뷰의 착지 막기·풀기(prhuman.go에서 나눔, docs/hc-fill/pr.md 「사람 리뷰 — 착지 막기」): 막는 글이 처리되기 전에는 병합 줄에 선 그 PR
// 제출을 refs/merge-failed/<이름>으로 빼고(병합기 반송과 같은 꼴 — 병합 엔진은 그대로), 처리되면(넘긴 뒤 고친 커밋의 자동 감사 통과) done,
// 재승인 기다림·지휘가 푼 막음은 뺀 제출을 줄 끝에 다시 세운다. 장부·git만 쓴다 — gh와 상관없이 바퀴마다(감사 1차 중간 1)

import (
	"fmt"
	"io"
	"os"
	"strings"

	"herdr-tools/internal/events"
	"herdr-tools/internal/gitx"
)

// humanHold: 막기·풀기 — prTick이 gh 확인·무늬 읽기 앞에서 부른다: gh가 느리거나 없거나 바퀴 예산이 차도 고치지 않은 제출이 그 바퀴에 착지하지 않게
func (r *prRun) humanHold() {
	if prHumanOn() {
		r.phPending(r.phByPR())
	}
}

// phPending: 막는 글이 처리 전인 PR — 처리됐으면 done(재승인 기다림이 켜져 있으면 승인까지), 아니면 병합 줄에 선 제출을 뺀다
func (r *prRun) phPending(byPR map[string]*prEnt) {
	rows := readRows(phFile())
	now := map[string]string{}
	for _, P := range phOrder(rows) {
		T, notes := phOpen(rows, P)
		e := byPR[P]
		if e != nil {
			r.phBack(e, P, phUnbacked(rows, P))
		}
		if T == "" || e == nil || e.State != "open" {
			continue
		}
		fam := r.phFamily(e)
		sha := phFixed(fam, T)
		if sha != "" && (knobInt("pr_human_approve") == 0 || phApproved(rows, P, T)) {
			phAdd("done", P, sha, utcnow(), "-")
			say("사람 리뷰 PR #%s 반영됨 %s", e.Num, head(sha, 8))
			continue
		}
		kind, label := "fix", "사람 리뷰 반영 중"
		if sha != "" {
			kind, label = "approve", "사람 리뷰 기다림(재승인)"
		}
		now[P] = label
		pulled, from, ok := r.phPullAll(e, P, fam, kind)
		if !ok || pulled == "" || kind == "approve" {
			continue
		}
		cur := e.Lane
		if len(fam) > 0 {
			cur = fam[len(fam)-1]
		}
		if how := r.phRoute(e, cur, notes, pulled, from, true); how != "" {
			phAdd("hand", P, "-", cur, how, utcnow(), "1")
		}
	}
	var b strings.Builder
	for _, P := range phOrder(rows) {
		if now[P] != "" {
			b.WriteString(P + "\t" + now[P] + "\n")
		}
	}
	if err := writeAtomic(prFile("pr-human-now.tsv"), b.String()); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: 사람 리뷰: %v\n", err)
	}
}

func phOrder(rows []Row) []string {
	var o []string
	for _, x := range rows {
		if x.F(phWhat) != "on" && x.F(phWhat) != "note" && !has(o, x.F(phPR)) {
			o = append(o, x.F(phPR))
		}
	}
	return o
}

// phOpen: 마지막 done 뒤 첫 막는 넘김 시각(없으면 빈 값)과 그 뒤 넘긴 글
func phOpen(rows []Row, P string) (string, []Row) {
	T, ids := "", map[string]bool{}
	for _, x := range rows {
		if x.F(phPR) != P {
			continue
		}
		switch x.F(phWhat) {
		case "done":
			T, ids = "", map[string]bool{}
		case "hand":
			if x.F(phBlock) == "1" && T == "" {
				T = x.F(phTime)
			}
			for _, id := range strings.Split(x.F(phIDs), ",") {
				ids[id] = true
			}
		}
	}
	var notes []Row
	for _, x := range rows {
		if x.F(phWhat) == "note" && x.F(phPR) == P && ids[x.F(phID)] {
			notes = append(notes, x)
		}
	}
	return T, notes
}

// phUnbacked: 풀린 막음마다(done 줄) 그 앞에 마지막으로 뺀 제출 하나 — 재승인 기다림으로 뺀 것(approve)이거나 지휘가 푼 것(done SHA 「-」 —
// human-ok, 그 제출이 고치지 않은 채 착지할 길을 잃지 않게, 감사 1차 중간 2). 고쳐서 풀린 막음의 옛 fix 뺌은 세우지 않는다. 이미 다시 세운 것은 뺀다
func phUnbacked(rows []Row, P string) []Row {
	var pend, o, keep []Row
	back := map[string]bool{}
	for _, x := range rows {
		if x.F(phPR) != P {
			continue
		}
		switch x.F(phWhat) {
		case "hold":
			pend = append(pend, x)
		case "done":
			if l := len(pend); l > 0 && (pend[l-1].F(phQKind) == "approve" || x.F(phSHA) == "-") {
				o = append(o, pend[l-1])
			}
			pend = nil
		case "back":
			back[x.F(phQ)] = true
		}
	}
	for _, x := range o {
		if !back[x.F(phQ)] {
			keep = append(keep, x)
		}
	}
	return keep
}

// phFixed <레인들> <넘긴 때>: 넘긴 뒤 완료한 커밋이 자동 감사를 통과했으면 그 SHA. 자동 감사가 아닌 줄은 넘긴 뒤 제출한 SHA.
// 넘기기 전에 이미 그 레인들이 보고(완료·제출)한 SHA는 고친 커밋이 아니다 — 같은 HEAD로 다시 부른 완료는 처리로 안 셈(감사 1차 낮음 1)
func phFixed(fam []string, since string) string {
	old := map[string]bool{}
	for _, l := range EV {
		if ev := events.Cols(l); has(fam, ev.Lane) && (ev.State == "완료" || ev.State == "제출") && ev.Time <= since && ev.SHA != "" {
			old[ev.SHA] = true
		}
	}
	for _, a := range readRows(AT) {
		if a.F(atVerdict) != "pass" || !has(fam, a.F(atLane)) {
			continue
		}
		for _, l := range EV {
			if ev := events.Cols(l); ev.Lane == a.F(atLane) && ev.State == "완료" && auSameSha(ev.SHA, a.F(atSHA)) && ev.Time > since && !old[ev.SHA] {
				return ev.SHA
			}
		}
	}
	for _, n := range fam {
		if q := qrow(n); q == nil || q.F(qAudit) == "auto" {
			continue
		}
		// ponytail: 자동 감사가 없는 줄(받은편지·--no-audit)은 넘긴 뒤 제출을 처리로 본다 — 그 레인의 자기 감사를 믿는다
		for _, l := range EV {
			if ev := events.Cols(l); ev.Lane == n && ev.State == "제출" && ev.Time > since && len(ev.SHA) == 40 && !old[ev.SHA] {
				return ev.SHA
			}
		}
	}
	return ""
}

// phApproved: 넘긴 뒤 허용 작성자의 승인 리뷰가 있나
func phApproved(rows []Row, P, since string) bool {
	for _, x := range rows {
		if x.F(phWhat) == "note" && x.F(phPR) == P && x.F(phRes) == "approve" && x.F(phAt) > since {
			return true
		}
	}
	return false
}

// phPullAll: 병합 줄에 선 그 PR 레인들의 제출을 뺀다(hold 줄). 마지막으로 뺀 큐 이름과 레인. 착지 중·못 읽음·못 옮김이면 ok=false(다음 바퀴에)
func (r *prRun) phPullAll(e *prEnt, P string, fam []string, kind string) (string, string, bool) {
	n, from := "", ""
	for _, l := range fam {
		lr, qr := mqlanding(e.Proj, l), mqueued(e.Proj, l)
		if lr != 1 || qr == 2 { // 착지 중(곧 착지 뒤 길)·모름
			return "", "", false
		}
		if qr == 1 {
			continue
		}
		q, ok := phPull(e.Proj, l)
		if !ok {
			return "", "", false
		}
		n, from = q, l
		phAdd("hold", P, q, l, utcnow(), kind)
		say("사람 리뷰 PR #%s 처리 전 — %s 제출을 병합 줄에서 뺌(refs/merge-failed/%s)", e.Num, l, q)
	}
	return n, from, true
}

// phPull <프로젝트> <레인>: 병합 줄의 그 레인 제출을 refs/merge-failed/<이름>으로(병합기 반송과 같은 꼴). 큐 이름·됨
func phPull(p, lane string) (string, bool) {
	repo, n, s := repoOf(p, LOCAL), "", ""
	for _, x := range MQS {
		if loc := mgQueueRe.FindStringIndex(x.F(mqRef)); x.F(mqProj) == p && loc != nil && x.F(mqRef)[loc[1]:] == lane {
			n, s = strings.TrimPrefix(x.F(mqRef), "refs/merge-queue/"), x.F(mqSHA)
		}
	}
	if n == "" || repo == "" {
		return "", false
	}
	q, qm := "refs/merge-queue/"+n, "refs/merge-queue-meta/"+n
	m, st := gitx.LsRemote(repo, "origin", io.Discard, q, qm)
	if st != gitx.Yes || m[q] != s {
		return "", false
	}
	if !phMove(repo, q, qm, "refs/merge-failed/"+n, "refs/merge-failed-meta/"+n, s, m[qm]) {
		return "", false
	}
	var keep []Row
	for _, x := range MQS {
		if x.F(mqRef) != q {
			keep = append(keep, x)
		}
	}
	MQS = keep
	return n, true
}

// phMove: 제출 참조 fq(기록 fm)를 tq(tm)로 — 한 번의 원자 push, 모든 참조에 lease(requeue와 같은 꼴). 응답만 끊겼으면 원격을 보고 됨
func phMove(repo, fq, fm, tq, tm, s, rec string) bool {
	a := []string{"fetch", "-q", "--no-tags", "--no-write-fetch-head", "origin", fq}
	if rec != "" {
		a = append(a, fm)
	}
	if run2(repo, a...) != 0 {
		return false
	}
	p := []string{s + ":" + tq, ":" + fq, "--force-with-lease=" + tq + ":", "--force-with-lease=" + fq + ":" + s}
	if rec != "" {
		p = append(p, rec+":"+tm, ":"+fm, "--force-with-lease="+tm+":", "--force-with-lease="+fm+":"+rec)
	}
	if run2(repo, append([]string{"push", "-q", "--atomic", "--porcelain", "origin"}, p...)...) == 0 {
		return true
	}
	m, st := gitx.LsRemote(repo, "origin", io.Discard, tq, fq)
	return st != gitx.Unknown && m[tq] == s && m[fq] == ""
}

// phBack: 풀린 막음의 뺀 제출(phUnbacked)을 줄 끝에 다시 세운다(back 줄). 반송 참조가 이미 없거나 그 레인의 더 새 제출이 줄에 있으면 「-」로 끝낸다
func (r *prRun) phBack(e *prEnt, P string, holds []Row) {
	repo := repoOf(e.Proj, LOCAL)
	for _, h := range holds {
		if repo == "" {
			return
		}
		n, lane := h.F(phQ), h.F(phQLane)
		fr, fm := "refs/merge-failed/"+n, "refs/merge-failed-meta/"+n
		m, st := gitx.LsRemote(repo, "origin", io.Discard, "refs/merge-queue/*", "refs/merge-queue-meta/*", "refs/merge-failed/*", "refs/merge-failed-meta/*")
		switch {
		case st == gitx.Unknown:
			say("사람 리뷰 PR #%s: 병합 줄을 못 읽어 %s 를 다시 못 세움 — 다음 바퀴에(refs/merge-failed/%s)", e.Num, lane, n)
		case m[fr] == "" || phLaneQueued(m, lane): // 이미 치웠거나 그 레인의 더 새 제출이 줄에 있다 — 옛 것은 세우지 않는다
			phAdd("back", P, n, lane, utcnow(), "-")
		default:
			n2 := rqName(lane, m)
			if phMove(repo, fr, fm, "refs/merge-queue/"+n2, "refs/merge-queue-meta/"+n2, m[fr], m[fm]) {
				phAdd("back", P, n, lane, utcnow(), n2)
				say("사람 리뷰 PR #%s 막음 풂 — %s 제출을 병합 줄에 다시 세움(%s)", e.Num, lane, n2)
			}
		}
	}
}

func phLaneQueued(m map[string]string, lane string) bool {
	for ref := range m {
		if q, ok := strings.CutPrefix(ref, "refs/merge-queue/"); ok && rqLaneOf(q) == lane {
			return true
		}
	}
	return false
}
