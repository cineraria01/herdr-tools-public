package main

// 거짓 반송 다시 넣기(2026-11-24 knob-test-leak — 운영 손잡이가 시험으로 새어 병합기가 무관한 제출을 검증 실패로 돌려보냈는데, 같은 커밋·
// 같은 감사 판정으로 줄에 다시 세울 손이 없어 작업 트리를 다시 만들어 rebase·제출하는 레인을 따로 띄웠다).
// hc-fill requeue <레인> --why <까닭> [--dry-run]: 병합기가 **검증 실패**(막힘 사건 「병합기: 착지 실패(종료 1…」)로 돌려보낸 그 레인의 마지막
// 제출을 그 커밋·그 제출 기록(감사 판정·승인 patch-id가 든 커밋) 그대로 줄 끝(새 큐 이름 <지금 초>-<레인>)에 다시 넣는다.
// 옮기기는 병합기 반송(cmd/hc-merger fail)의 거울 — 반송 참조 둘을 지우고 큐 참조 둘을 만드는 한 번의 원자 push, 모든 참조에 lease.
// 줄에 선 뒤는 보통 길(검증 맡김 → 착지)이고 관문은 그대로다. 사람·에이전트가 명시로 부를 때만 — 병합기·바퀴는 스스로 부르지 않는다.
// 거절(종료 1): 반송 없음·이미 줄에 있음·검증 실패 반송이 아님(충돌 등)·감사 판정이 취소됨·기본 가지에 이미 듦·기록이 없음.
// 모름(종료 75): 원격·사건을 못 읽음 — 같은 줄을 다시.

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/events"
	"herdr-tools/internal/gitx"
)

func init() { register("requeue", requeueCmd) }

var rqVerifyFail = regexp.MustCompile(`^병합기: 착지 실패\(종료 1[,)]`)

// rqFound: 그 레인의 마지막 반송
type rqFound struct {
	proj, repo, n, s, rec string
	refs                  map[string]string
}

func requeueCmd(args []string) {
	args, why := takeWhy(args)
	dryRun := false
	var pos []string
	for _, a := range args {
		switch {
		case a == "--dry-run":
			dryRun = true
		case strings.HasPrefix(a, "-"):
			usage()
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) != 1 || why == "" || !okname(pos[0]) {
		usage()
	}
	exit(requeue(pos[0], why, dryRun))
}

// requeue: 0 넣음(dry-run이면 넣을 수 있음) · 1 거절 · 75 모름
func requeue(lane, why string, dryRun bool) int {
	no := func(f string, a ...any) int {
		msg := fmt.Sprintf(f, a...)
		say("다시 넣지 않음 %s: %s", lane, msg)
		if !dryRun {
			jot("requeue", lane, why, "거절: "+msg, "-", "-")
		}
		return 1
	}
	unk := func(f string, a ...any) int {
		say("다시 넣기 모름 %s: %s — 같은 줄을 다시", lane, fmt.Sprintf(f, a...))
		return 75
	}
	f, st := rqFind(lane)
	switch st {
	case gitx.Unknown:
		return unk("병합기 프로젝트의 원격 반송 참조를 다 읽지 못했다")
	case gitx.No:
		return no("병합기가 돌려보낸 제출(refs/merge-failed/<초>-%s)이 원격에 없다", lane)
	case gitx.Yes:
	}
	fr, fm := "refs/merge-failed/"+f.n, "refs/merge-failed-meta/"+f.n
	for r := range f.refs {
		if q, ok := strings.CutPrefix(r, "refs/merge-queue/"); ok && rqLaneOf(q) == lane {
			return no("그 레인의 제출이 이미 줄에 있다(%s)", r)
		}
	}
	if f.rec == "" {
		return no("반송된 제출의 기록(%s)이 없다 — 감사 판정을 이어 줄 수 없다", fm)
	}
	ev, ok := rqEvent(f.proj, f.n)
	if !ok {
		return unk("병합기 기계의 사건을 읽지 못했다(hc-lane events)")
	}
	if ev == "" {
		return no("반송 사건(%s)을 병합기 사건에서 못 찾았다 — 까닭을 모르니 다시 넣지 않는다", fr)
	}
	if !rqVerifyFail.MatchString(ev) {
		return no("검증 실패 반송이 아니다(%s) — 충돌 반송은 다시 맞춤(reland)으로", bcutS(ev, 120))
	}
	if !rqFetch(f) {
		return unk("반송된 제출·기록을 받지 못했다(git fetch)")
	}
	vd, id := rqMeta(f.repo, f.rec, f.s)
	if vd == "?" {
		return no("반송된 제출 기록(%s)을 읽지 못했다", first8s(f.rec))
	}
	if r := arow(vd); vd != "" && r != nil && r.F(atState) == "superseded" {
		return no("감사 판정 %s 가 취소됐다(새 차수로 넘어감) — 새 판정으로 착지해야 한다", vd)
	}
	if ref, ok := projectRef(f.repo); !ok {
		return unk("기본 가지를 모른다")
	} else if base := strings.TrimPrefix(ref, "origin/"); run2(f.repo, "fetch", "-q", "origin", base) != 0 {
		return unk("기본 가지를 받지 못했다(git fetch)")
	} else if run2(f.repo, "merge-base", "--is-ancestor", f.s, "FETCH_HEAD") == 0 {
		return no("그 제출(%s)은 이미 기본 가지에 있다", first8s(f.s))
	}
	if id != "" {
		if _, ok := f.refs["refs/hc-landed/"+id]; ok {
			return no("그 제출(ID %s)은 이미 착지했다(refs/hc-landed/%s)", id, id)
		}
	}
	n2 := rqName(lane, f.refs)
	q2, qm2 := "refs/merge-queue/"+n2, "refs/merge-queue-meta/"+n2
	plan := fmt.Sprintf("%s → %s (제출 %s, 기록 %s, 감사 판정 %s)", fr, q2, first8s(f.s), first8s(f.rec), dashS(vd))
	if dryRun {
		say("다시 넣을 수 있음 %s: %s — 반송 까닭 「%s」", lane, plan, bcutS(ev, 120))
		return 0
	}
	p := []string{f.s + ":" + q2, f.rec + ":" + qm2, ":" + fr, ":" + fm,
		"--force-with-lease=" + q2 + ":", "--force-with-lease=" + qm2 + ":",
		"--force-with-lease=" + fr + ":" + f.s, "--force-with-lease=" + fm + ":" + f.rec}
	if run2(f.repo, append([]string{"push", "-q", "--atomic", "--porcelain", "origin"}, p...)...) != 0 {
		m, st := gitx.LsRemote(f.repo, "origin", io.Discard, q2, qm2, fr, fm)
		switch {
		case st != gitx.Unknown && m[q2] == f.s && m[qm2] == f.rec && m[fr] == "" && m[fm] == "": // 응답만 끊김
		case st == gitx.Unknown:
			return unk("다시 넣었는지 확인하지 못했다(push 실패 뒤 원격을 못 읽음)")
		default:
			return no("옮기지 못했다(push 거절 — 그새 병합기·레인이 반송 참조를 바꿈). 원격을 보고 다시")
		}
	}
	say("다시 넣음 %s: %s — 줄 끝에 섰다(검증 맡김 → 착지는 보통 길)", lane, plan)
	jot("requeue", lane, why, "됨: "+plan, "-", "반송 까닭: "+bcutS(ev, 160))
	return 0
}

// rqLaneOf: 큐 이름 <초>-<레인>의 레인(꼴이 아니면 빈 값)
func rqLaneOf(n string) string {
	sec, l, ok := strings.Cut(n, "-")
	if !ok || !isDigits(sec) {
		return ""
	}
	return l
}

// rqFind: 병합기를 켠 프로젝트 가운데 그 레인의 마지막 반송(초가 가장 큰 것). 한 프로젝트라도 못 읽었는데 못 찾으면 모름
func rqFind(lane string) (rqFound, gitx.State) {
	var ps []string
	for _, f := range confRows("merger") {
		if len(f) >= 2 {
			ps = append(ps, f[1])
		}
	}
	var best rqFound
	bestSec, unk := int64(-1), false
	for _, p := range uniq(ps) {
		repo := repoOf(p, LOCAL)
		if repo == "" {
			unk = true
			continue
		}
		m, st := gitx.LsRemote(repo, "origin", io.Discard, "refs/merge-failed/*", "refs/merge-failed-meta/*", "refs/merge-queue/*",
			"refs/merge-queue-meta/*", "refs/hc-landed/*")
		if st == gitx.Unknown {
			unk = true
			continue
		}
		for r, s := range m {
			n, ok := strings.CutPrefix(r, "refs/merge-failed/")
			if !ok || rqLaneOf(n) != lane {
				continue
			}
			sec, _ := strconv.ParseInt(n[:strings.IndexByte(n, '-')], 10, 64)
			if sec > bestSec {
				bestSec = sec
				best = rqFound{proj: p, repo: repo, n: n, s: s, rec: m["refs/merge-failed-meta/"+n], refs: m}
			}
		}
	}
	switch {
	case bestSec >= 0:
		return best, gitx.Yes
	case unk:
		return best, gitx.Unknown
	}
	return best, gitx.No
}

// rqEvent: 병합기 기계가 적은 그 반송의 막힘 사건 글(마지막 것). ok=false 못 읽음, 빈 값 = 못 찾음(못 읽은 기계가 있으면 못 읽음)
func rqEvent(p, n string) (string, bool) {
	mg := map[string]bool{}
	args := []string{"events"}
	for _, f := range confRows("merger") {
		if len(f) >= 3 && f[1] == p {
			for _, m := range f[2:] {
				if !mg[m] {
					mg[m] = true
					if m != LOCAL {
						args = append(args, "--on", m)
					}
				}
			}
		}
	}
	out, rc := Cmd{Name: "hc-lane", Args: args}.Run()
	if rc != 0 {
		return "", false
	}
	msg, unk := "", false
	for _, e := range lines(sub(out)) {
		if m, ok := strings.CutPrefix(e, "모름 "); ok && mg[m] {
			unk = true
			continue
		}
		r := splitRow(e)
		if r.F(evState) == "막힘" && mg[r.F(evMach)] && strings.Contains(r.F(evMsg), "refs/merge-failed/"+n+" ") {
			msg = r.F(evMsg)
		}
	}
	if msg == "" && unk {
		return "", false
	}
	return msg, true
}

// rqFetch: 반송된 제출·기록 객체를 이 클론에 받는다(push가 보낼 수 있게 — 참조는 만들지 않는다, FETCH_HEAD만)
func rqFetch(f rqFound) bool {
	return run2(f.repo, "fetch", "-q", "origin", "refs/merge-failed/"+f.n, "refs/merge-failed-meta/"+f.n) == 0
}

// rqMeta: 제출 기록 커밋(부모 = 제출)의 첫 줄에서 감사 판정(verdict=)·제출 ID(id=). 못 읽거나 부모가 다르면 「?」
func rqMeta(repo, rec, s string) (vd, id string) {
	if p, rc := run("git", "-C", repo, "rev-parse", "-q", "--verify", rec+"^"); rc != 0 || sub(p) != s {
		return "?", ""
	}
	body, rc := run("git", "-C", repo, "log", "-1", "--format=%B", rec)
	if rc != 0 {
		return "?", ""
	}
	first, _, _ := strings.Cut(body, "\n")
	for _, kv := range strings.Fields(first) {
		k, v, _ := strings.Cut(kv, "=")
		switch k {
		case "verdict":
			vd = v
		case "id":
			if id == "" {
				id = v
			}
		}
	}
	if vd == "-" {
		vd = ""
	}
	return vd, id
}

// rqName: 줄 끝 새 큐 이름 <지금 초>-<레인> — 원격의 큐·반송 이름과 겹치면 초를 늘린다
func rqName(lane string, refs map[string]string) string {
	used := map[string]bool{}
	for r := range refs {
		for _, pre := range []string{"refs/merge-queue/", "refs/merge-queue-meta/", "refs/merge-failed/", "refs/merge-failed-meta/"} {
			if n, ok := strings.CutPrefix(r, pre); ok {
				used[n] = true
			}
		}
	}
	sec := time.Now().Unix()
	for used[fmt.Sprintf("%010d-%s", sec, lane)] {
		sec++
	}
	return fmt.Sprintf("%010d-%s", sec, lane)
}

func run2(repo string, a ...string) int {
	_, rc := run("git", append([]string{"-C", repo}, a...)...)
	return rc
}

func first8s(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func dashS(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func bcutS(s string, n int) string { return events.Bcut(n, events.Flat(s)) }
