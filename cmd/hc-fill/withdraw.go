package main

// 병합 줄에서 거두기(2026-11-26 ht-merger-withdraw — 한 프로젝트 병합 줄에서 먼저 착지한 변경과 뜻이 부딪힌 제출이 어느 묶음에 들어가도
// 실패했다. 주인 지휘는 16:55에 진짜 충돌임을 알았지만 큐에서 뺄 명령이 없어, 병합기가 스스로 돌려보낼 때까지 묶음·직접 검증 네 바퀴(약
// 1시간 반)를 태웠고 다른 제출 넷이 그 뒤에 섰다).
// hc-fill withdraw <레인> --why <까닭> [--force] [--dry-run] [--json]: 그 레인의 줄에 선 제출을 병합기 반송과 같은 꼴로 뺀다 — 큐 표식·기록을
// refs/merge-failed/<같은 이름>·refs/merge-failed-meta/<같은 이름>으로 한 번의 원자 push(모든 참조에 본 값 lease — requeue의 거울).
//   - 병합기가 그새 착지시켰거나 반송했으면 lease가 져 「이미 착지·반송됨」(아무것도 안 바꿈). 착지 실행이 돌고 있어도 거둔다 — 그 착지의
//     기본 가지 push가 같은 큐 lease로 지고 병합기는 결과를 버린다(cmd/hc-merger judge gone). 착지 push가 먼저면 거두기가 진다.
//   - 흔적은 반송과 같다: 주인에게 막힘 사건 「지휘가 거둠: <까닭> — 원본 …, refs/merge-failed/<이름> 로 옮김」(반송 다시 띄움·PR 거울이 병합기
//     반송처럼 받는다 — bounceFrom), 운영 일지 한 줄. 레인은 지금 길대로 고쳐 다시 낸다.
//   - 누가: 그 제출의 주인 지휘(제출 기록 owner=, 없으면 대기열 줄 주인)만. 다른 지휘·주인 없는 제출은 --force(일지에 남김). 레인 안에서는 못 부른다.
//   - 병합기가 스스로 거두지 않는다 — 사람·지휘가 부를 때만(운영 에이전트도 사람에게 넘긴다, opsagent_gate).
// 종료: 0 거둠(dry-run이면 거둘 수 있음) · 1 거절(줄에 없음·이미 착지·반송·주인 아님) · 2 잘못 부름·레인 안 · 75 원격 모름(같은 줄을 다시)

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"herdr-tools/internal/gitx"
)

func init() { register("withdraw", withdrawCmd) }

// wdEvRe: 거두기 사건 글(까닭은 「 — 원본 」 앞까지)
var wdEvRe = regexp.MustCompile(`^(지휘가 거둠: .*?) — 원본 `)

// bounceFrom: 반송 사건으로 받을 기계·글인가 — 병합기 기계의 사건, 또는 이 기계에서 지휘가 거둔 사건(withdraw)
func bounceFrom(mg map[string]bool, mach, msg string) bool {
	return mg[mach] || mach == LOCAL && wdEvRe.MatchString(msg)
}

// wdBeforePush: 원격을 읽은 뒤·push 전(시험이 병합기 착지를 끼운다)
var wdBeforePush = func() {}

// wdOut: 결과(--json 한 줄 — 칸은 docs/hc-fill/withdraw.md)
type wdOut struct {
	Lane    string `json:"lane"`
	Project string `json:"project"`
	Queue   string `json:"queue"`
	SHA     string `json:"sha"`
	Owner   string `json:"owner"`
	By      string `json:"by"`
	Force   bool   `json:"force"`
	DryRun  bool   `json:"dry_run"`
	Result  string `json:"result"` // 거둠 · 거둘 수 있음 · 거절 · 모름
	Detail  string `json:"detail"`
}

func withdrawCmd(args []string) {
	args, dryRun := takeDry(args)
	args, why := takeWhy(args)
	force, js := false, false
	var pos []string
	for _, a := range args {
		switch {
		case a == "--force":
			force = true
		case a == "--json":
			js = true
		case strings.HasPrefix(a, "-"):
			usage()
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) != 1 || why == "" || !okname(pos[0]) {
		usage()
	}
	exit(withdraw(pos[0], why, force, dryRun, js))
}

// withdraw: 종료 코드(위). js면 사람 글 대신 한 줄 객체
func withdraw(lane, why string, force, dryRun, js bool) int {
	o := wdOut{Lane: lane, By: os.Getenv("HC_TERMINAL_ID"), Force: force, DryRun: dryRun}
	end := func(rc int, res, f string, a ...any) int {
		o.Result, o.Detail = res, fmt.Sprintf(f, a...)
		if js {
			b, _ := json.Marshal(o)
			fmt.Println(string(b))
		} else {
			head := map[string]string{"거둠": "거둠", "거둘 수 있음": "거둘 수 있음", "거절": "거두지 않음", "모름": "거두기 모름"}[res]
			say("%s %s: %s", head, lane, o.Detail)
		}
		if res == "거절" && !dryRun {
			jot("withdraw", lane, why, "거절: "+o.Detail, "-", "-")
		}
		return rc
	}
	if os.Getenv("HC_LANE") != "" {
		return end(2, "거절", "레인 안에서는 거두지 않는다 — 주인 지휘가 부른다")
	}
	f, st := wdFind(lane)
	switch st {
	case gitx.Unknown:
		return end(75, "모름", "병합기 프로젝트의 원격 큐를 다 읽지 못했다 — 같은 줄을 다시")
	case gitx.No:
		return end(1, "거절", "그 레인의 제출이 병합 줄에 없다 — %s", wdGone(lane, f.refs))
	case gitx.Yes:
	}
	o.Project, o.Queue, o.SHA = f.proj, f.n, f.s
	q, qm, fr, fm := "refs/merge-queue/"+f.n, "refs/merge-queue-meta/"+f.n, "refs/merge-failed/"+f.n, "refs/merge-failed-meta/"+f.n
	if run2(f.repo, "fetch", "-q", "--no-tags", "--no-write-fetch-head", "origin", q, qm) != 0 && (f.rec != "" || run2(f.repo, "fetch", "-q", "--no-tags", "--no-write-fetch-head", "origin", q) != 0) {
		return end(75, "모름", "줄에 선 제출·기록을 받지 못했다(git fetch) — 같은 줄을 다시")
	}
	owner, id := wdMeta(f.repo, f.rec, f.s)
	if owner == "" {
		owner = strings.TrimPrefix(qown(lane), "-")
	}
	o.Owner = owner
	if mine := o.By != "" && owner != "" && ownerAddrIs(o.By, owner); !mine && !force {
		if owner == "" {
			return end(1, "거절", "주인 지휘가 없는 제출이다 — 거두려면 --force(일지에 남는다)")
		}
		return end(1, "거절", "주인 지휘(%s)가 아니다(부른 쪽 %s) — 다른 지휘가 거두려면 --force(일지에 남는다)", owner, dashS(o.By))
	}
	plan := fmt.Sprintf("%s → %s (제출 %s, 기록 %s)", q, fr, first8s(f.s), dashS(first8s(f.rec)))
	if dryRun {
		return end(0, "거둘 수 있음", "%s — 미리 보기(--dry-run, 아무것도 안 바꿈)", plan)
	}
	wdBeforePush()
	p := []string{f.s + ":" + fr, ":" + q, "--force-with-lease=" + fr + ":", "--force-with-lease=" + q + ":" + f.s}
	if f.rec != "" {
		p = append(p, f.rec+":"+fm, ":"+qm, "--force-with-lease="+fm+":", "--force-with-lease="+qm+":"+f.rec)
	}
	if po, rc := run("git", append([]string{"-C", f.repo, "push", "-q", "--atomic", "--porcelain", "origin"}, p...)...); rc != 0 {
		if rc, res, msg := wdLost(f, id, po); rc != 0 {
			return end(rc, res, "%s", msg)
		}
	}
	by := dashS(o.By)
	ev := fmt.Sprintf("지휘가 거둠: %s — 원본 %s, 제출 ID %s, %s 로 옮김(lane/%s 는 그대로, 거둔 쪽 %s) — 고쳐 hc-lane land main 다시",
		bcutS(why, 200), first8s(f.s), dashS(id), fr, lane, by)
	evNote := ""
	if hcLaneReport("막힘", ev, lane, owner, "hf:withdraw:"+f.n) != 0 {
		evNote = " — 주인 알림 사건을 못 냄(hc-lane report 실패, 반송 참조는 옮겨짐)"
	}
	res := "됨: " + plan
	if !(o.By != "" && owner != "" && ownerAddrIs(o.By, owner)) {
		res = fmt.Sprintf("됨(--force, 주인 %s): %s", dashS(owner), plan)
	}
	jot("withdraw", lane, why, res+evNote, "레인이 고쳐 hc-lane land main으로 다시 제출(반송 다시 띄움이 같은 이름으로 띄운다)", "-")
	return end(0, "거둠", "%s — 반송과 같은 흔적(주인 %s에게 막힘 사건)%s", plan, dashS(owner), evNote)
}

// wdFound: 줄에 선 그 레인의 제출
type wdFound struct {
	proj, repo, n, s, rec string
	refs                  map[string]string // 그 프로젝트 원격 목록(없으면 모든 프로젝트를 합친 것 — 「줄에 없음」 까닭용)
}

// wdFind: 병합기를 켠 프로젝트 가운데 큐에 선 그 레인의 제출(초가 가장 큰 것). 못 찾았는데 못 읽은 프로젝트가 있으면 모름
func wdFind(lane string) (wdFound, gitx.State) {
	var ps []string
	for _, f := range confRows("merger") {
		if len(f) >= 2 {
			ps = append(ps, f[1])
		}
	}
	var best wdFound
	all, bestSec, unk := map[string]string{}, int64(-1), false
	for _, p := range uniq(ps) {
		repo := repoOf(p, LOCAL)
		if repo == "" {
			unk = true
			continue
		}
		m, st := gitx.LsRemote(repo, "origin", io.Discard, "refs/merge-queue/*", "refs/merge-queue-meta/*", "refs/merge-failed/*", "refs/hc-landed/*")
		if st == gitx.Unknown {
			unk = true
			continue
		}
		for r, s := range m {
			all[r] = s
			n, ok := strings.CutPrefix(r, "refs/merge-queue/")
			if !ok || rqLaneOf(n) != lane {
				continue
			}
			if sec, _ := strconv.ParseInt(n[:strings.IndexByte(n, '-')], 10, 64); sec > bestSec {
				bestSec, best = sec, wdFound{proj: p, repo: repo, n: n, s: s, rec: m["refs/merge-queue-meta/"+n], refs: m}
			}
		}
	}
	switch {
	case bestSec >= 0:
		return best, gitx.Yes
	case unk:
		return best, gitx.Unknown
	}
	best.refs = all
	return best, gitx.No
}

// wdGone: 줄에 없는 레인의 마지막 흔적 — 반송(refs/merge-failed/<초>-<레인>, 초 = 제출 시각)과 착지 표식(refs/hc-landed/<레인>/<초>,
// 초 = 착지 시각) 가운데 늦은 것
func wdGone(lane string, refs map[string]string) string {
	fSec, fn, lSec, ln := int64(-1), "", int64(-1), ""
	for r := range refs {
		if n, ok := strings.CutPrefix(r, "refs/merge-failed/"); ok && rqLaneOf(n) == lane {
			if sec, _ := strconv.ParseInt(n[:strings.IndexByte(n, '-')], 10, 64); sec > fSec {
				fSec, fn = sec, r
			}
		}
		if x, ok := strings.CutPrefix(r, "refs/hc-landed/"+lane+"/"); ok {
			if sec, err := strconv.ParseInt(x, 10, 64); err == nil && sec > lSec {
				lSec, ln = sec, r
			}
		}
	}
	switch {
	case lSec >= 0 && lSec >= fSec:
		return "이미 착지(" + ln + ")"
	case fSec >= 0:
		return "이미 반송됨(" + fn + ")"
	}
	return "내지 않았거나 흔적이 없다"
}

// wdMeta: 제출 기록 첫 줄의 주인 지휘(owner=, 「-」은 빈 값)·제출 ID(id=). 기록이 없거나 못 읽으면 빈 값
func wdMeta(repo, rec, s string) (owner, id string) {
	if rec == "" {
		return "", ""
	}
	if p, rc := run("git", "-C", repo, "rev-parse", "-q", "--verify", rec+"^"); rc != 0 || sub(p) != s {
		return "", ""
	}
	body, rc := run("git", "-C", repo, "log", "-1", "--format=%B", rec)
	if rc != 0 {
		return "", ""
	}
	first, _, _ := strings.Cut(body, "\n")
	for _, kv := range strings.Fields(first) {
		switch k, v, _ := strings.Cut(kv, "="); k {
		case "owner":
			owner = strings.TrimPrefix(v, "-")
		case "id":
			if id == "" {
				id = v
			}
		}
	}
	return owner, id
}

// wdLost: push가 실패한 뒤 — 0이면 응답만 끊겼고 옮겨졌다(서버가 거절한 줄 「!」이 없고 원격이 옮긴 모양). 아니면 종료 코드·결과·까닭
func wdLost(f wdFound, id, po string) (int, string, string) {
	q, fr, fm := "refs/merge-queue/"+f.n, "refs/merge-failed/"+f.n, "refs/merge-failed-meta/"+f.n
	pats := []string{q, fr, fm}
	if id != "" {
		pats = append(pats, "refs/hc-landed/"+id)
	}
	m, st := gitx.LsRemote(f.repo, "origin", io.Discard, pats...)
	rejected := false // 서버가 거절한 참조(--porcelain 「!」 줄) — 없으면 응답만 끊겼을 수 있다
	for _, l := range lines(po) {
		rejected = rejected || strings.HasPrefix(l, "!")
	}
	switch {
	case st == gitx.Unknown:
		return 75, "모름", "거뒀는지 확인하지 못했다(push 실패 뒤 원격을 못 읽음) — 원격을 보고 다시"
	case m[q] == f.s:
		return 75, "모름", "push가 실패했는데 제출은 줄에 그대로다 — 같은 줄을 다시"
	case m[q] != "":
		return 1, "거절", "그새 그 큐 표식이 바뀌었다(다시 제출 등) — 원격을 보고 다시"
	case !rejected && m[fr] == f.s && m[fm] == f.rec:
		return 0, "", ""
	case m[fr] == f.s:
		return 1, "거절", "그새 이미 반송됨(병합기가 돌려보냄, " + fr + ") — 아무것도 안 바꿈"
	case id != "" && m["refs/hc-landed/"+id] != "":
		return 1, "거절", "그새 이미 착지(refs/hc-landed/" + id + ") — 거두기가 졌다(아무것도 안 바꿈)"
	}
	return 1, "거절", "그새 큐에서 빠졌다(착지·치움 — 착지 표식은 아직 못 봄) — 원격을 보고 다시"
}
