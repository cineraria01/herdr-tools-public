package main

// 쌓아 띄움(docs/hc-fill/stack.md, 2026-11-27 사용자 「쌓은 PR 그 방식으로 하자」): 손잡이 stack에 든 프로젝트에서, 대기 줄의 아직 안 든
// 선행이 같은 프로젝트 대기열 줄 하나이고 그 선행이 감사 통과로 병합 줄에 제출됐으면(원격 큐 기록에 verdict=) 착지를 기다리지 않고
// 그 제출 커밋 P 위에서 띄운다(hc-lane에 HC_LANE_STACK=<선행>@<P>, 14칸 stacked, 기록 stack.tsv). 끄면(기본 -) 오늘 길 그대로.

import (
	"fmt"
	"strings"
	"time"
)

var (
	STACK string // 이번 띄움의 쌓임 「<선행>@<P>」(빈 값 = 쌓지 않음)
	STREF string // 선행 큐 이름(<초>-<선행>)
)

// 쌓아 띄움 기록 stack.tsv — 레인마다 한 줄(띄울 때 덮고, 쌓지 않고 다시 띄우면 지운다)
const (
	skLane  = 1 // 레인
	skRID   = 2 // 줄 ID
	skPred  = 3 // 선행
	skP     = 4 // 선행 제출 커밋 P(40자)
	skQueue = 5 // 선행 큐 이름
	skState = 6 // up·parked·restacked·relaunch·rebased-main
	skAt    = 7 // 바뀐 때(UTC)
)

func stackFile() string { return DATA + "/stack.tsv" }

// stackOn <프로젝트>: 손잡이·설정 stack 목록에 그 프로젝트가 있나(기본 「-」 = 끔)
func stackOn(p string) bool { return p != "" && inlist(p, strings.ReplaceAll(conf("stack"), " ", "")) }

func stackDepth() int {
	if v := conf("stack_depth"); isDigits(v) && atoi10(v) > 0 {
		return atoi10(v)
	}
	return 2
}

// stackRec <레인>: 그 레인의 쌓임 기록(없으면 nil)
func stackRec(lane string) Row {
	for _, r := range readRows(stackFile()) {
		if r.NF() >= skAt && r.F(skLane) == lane {
			return r
		}
	}
	return nil
}

// stackPut: 그 레인 줄을 바꾼다(r이 nil이면 지움)
func stackPut(lane string, r Row) {
	var o []Row
	for _, x := range readRows(stackFile()) {
		if x.NF() >= skAt && x.F(skLane) != lane {
			o = append(o, x)
		}
	}
	if r != nil {
		o = append(o, r)
	}
	if err := writeAtomic(stackFile(), rowsText(o)); err != nil {
		say("경고: 쌓아 띄움 기록을 못 씀: %v", err)
	}
}

// stackSave <레인> <줄 ID>: 떠 있음으로 표시한 뒤 — 쌓아 띄웠으면 기록, 아니면 옛 기록을 지운다
func stackSave(lane, rid string) {
	if dry {
		return
	}
	if STACK == "" {
		if stackRec(lane) != nil {
			stackPut(lane, nil)
		}
		return
	}
	pred, p, _ := strings.Cut(STACK, "@")
	stackPut(lane, Row{lane, rid, pred, p, STREF, "up", utcnow()})
}

// stackEnvs: hc-lane 띄우기에 줄 환경(쌓을 때만)
func stackEnvs() []string {
	if STACK == "" {
		return nil
	}
	return []string{"HC_LANE_STACK=" + STACK}
}

// stackRef <레인> <기준 참조>: 감사 범위의 시작 — 쌓아 띄운 레인이면 P, 아니면 그대로
func stackRef(lane, ref string) string {
	if r := stackRec(lane); r != nil && r.F(skState) != "rebased-main" {
		if q := qrow(lane); q != nil && q.F(qEarly) == "stacked" {
			return r.F(skP)
		}
	}
	return ref
}

// stackQueue <저장소>: 원격 큐 표식·기록 「참조 → sha」. ok=false면 모름
func stackQueue(repo string) (map[string]string, bool) {
	o, rc := Cmd{Name: "git", Args: []string{"-C", repo, "ls-remote", "origin", "refs/merge-queue/*", "refs/merge-queue-meta/*", "refs/heads/lane/*", "refs/merge-parked/*", "refs/merge-parked-meta/*"}, Err: errDrop}.Run()
	if rc != 0 {
		return nil, false
	}
	m := map[string]string{}
	for _, l := range lines(o) {
		if s, r, ok := strings.Cut(l, "\t"); ok && len(s) == 40 {
			m[r] = s
		}
	}
	return m, true
}

// stackQName <원격 참조들> <접두> <레인>: 그 레인의 큐 이름(<10자리 초>-<레인>) 하나. 둘 이상·없음은 빈 값
func stackQName(m map[string]string, pre, lane string) string {
	got := ""
	for r := range m {
		n, ok := strings.CutPrefix(r, pre)
		if ok && len(n) == 11+len(lane) && n[10] == '-' && isDigits(n[:10]) && n[11:] == lane {
			if got != "" {
				return ""
			}
			got = n
		}
	}
	return got
}

// stackMetaKV <저장소> <기록 참조> <참조 이름>: 제출 기록 첫 줄의 key=value. 못 받으면 nil
func stackMetaKV(repo, ref, as string) map[string]string {
	if _, rc := (Cmd{Name: "git", Args: []string{"-C", repo, "fetch", "-q", "origin", "+" + ref + ":refs/hc-fill/stack/" + as}, Err: errDrop}).Run(); rc != 0 {
		return nil
	}
	o, rc := Cmd{Name: "git", Args: []string{"-C", repo, "log", "-1", "--format=%s", "refs/hc-fill/stack/" + as}, Err: errDrop}.Run()
	if rc != 0 {
		return nil
	}
	kv := map[string]string{}
	for _, f := range strings.Fields(o) {
		if k, v, ok := strings.Cut(f, "="); ok {
			kv[k] = v
		}
	}
	return kv
}

// stackPick <대기 줄> <14칸>: 이 줄을 쌓아 띄울 수 있으면 STACK·STREF를 채우고 true. 판정은 바로 앞 afterOK가 fetch한 저장소(FR)로
func stackPick(r Row, ef string) bool {
	STACK, STREF = "", ""
	name, prj, after, ak := r.F(qName), r.F(qProj), r.F(qAfter), dash(r.F(qAudit))
	if !stackOn(prj) || ef == "no-early" || strings.HasPrefix(ak, "audit=") {
		return false
	}
	pred := ""
	for _, a := range words(after) {
		n, op := afterAt(a, prj)
		if landingHit(n) == 0 {
			continue
		}
		if op != prj || pred != "" {
			say("쌓아 띄움 안 함 %s: 아직 안 든 선행이 하나가 아니거나 다른 프로젝트(%s)", name, a)
			return false
		}
		pred = tmap(n)
	}
	if q := qrow(pred); pred == "" || q == nil || q.F(qProj) != prj {
		say("쌓아 띄움 안 함 %s: 선행 %s 가 같은 프로젝트 대기열 줄이 아니다", name, pred)
		return false
	}
	m, ok := stackQueue(FR)
	if !ok {
		say("쌓아 띄움 보류 %s: 원격 큐를 읽지 못함", name)
		return false
	}
	p, qn, depth := "", "", 0
	for at := pred; ; {
		n := stackQName(m, "refs/merge-queue/", at)
		if n == "" {
			if at == pred {
				say("쌓아 띄움 안 함 %s: 선행 %s 가 병합 줄에 없다(아직 제출 전·반송·지휘가 거둠)", name, pred)
				return false
			}
			break // 그 위 조상은 줄에 없다(들었다) — 사슬 끝
		}
		depth++
		if depth > stackDepth() {
			say("쌓아 띄움 안 함 %s: 사슬 깊이 %d 넘음(손잡이 stack_depth)", name, stackDepth())
			return false
		}
		kv := stackMetaKV(FR, "refs/merge-queue-meta/"+n, at)
		if kv == nil || kv["orig"] != m["refs/merge-queue/"+n] {
			say("쌓아 띄움 보류 %s: 선행 %s 의 제출 기록을 읽지 못함", name, at)
			return false
		}
		if at == pred {
			if kv["verdict"] == "" || m["refs/heads/lane/"+pred] != kv["orig"] {
				say("쌓아 띄움 안 함 %s: 선행 %s 제출에 감사 통과 판정이 없다(감사 전·손 제출)", name, pred)
				return false
			}
			p, qn = kv["orig"], n
		}
		up, _, ok := strings.Cut(kv["stack"], "@")
		if !ok {
			break
		}
		at = up
	}
	STACK, STREF = pred+"@"+p, qn
	say("쌓아 띄움 %s: 선행 %s 제출 %s(감사 통과) 위에서 — 착지를 기다리지 않음", name, pred, head(p, 8))
	return true
}

// stackSec <레인> <14칸> <기본 가지 참조>: run 지시문 끝 쌓아 띄움 절(먼저 띄움 절 자리)
func stackSec(lane, ef, base string) string {
	pred, p, _ := strings.Cut(STACK, "@")
	b := strings.TrimPrefix(base, "origin/")
	first := ""
	switch ef {
	case "parked":
		first = "0. 이 작업 트리는 주차한 가지(준비 커밋)에서 만들었다. 먼저 `git fetch origin && git rebase " + p + "` — 선행 커밋 위로(origin/" + b + " 로 rebase하지 않는다). 충돌은 양쪽 내용을 살려 푼다.\n"
	case "bounced":
		first = "0. 위 반송·다시 쌓기 절 2의 rebase 기준 `origin/" + b + "` 대신 선행 커밋 `" + p + "`(다시 쌓기 절이면 `git rebase --onto " + p + " <옛 선행 커밋>`). 그 밖은 그 절 그대로.\n"
	}
	return fmt.Sprintf("\n## 쌓아 띄움 — 기준은 선행 %[1]s %[2]s(아직 %[3]s에 없음) (hc-fill, %[4]s)\n"+
		"선행 %[1]s가 감사를 통과해 병합 줄에 제출됐다(커밋 %[5]s). hc-fill이 그 착지를 기다리지 않고 그 커밋 위에서 이 레인을 띄웠다(레인 환경 HC_LANE_STACK=%[1]s@%[5]s).\n"+
		"%[6]s"+
		"1. 위 지시문의 본 작업을 끝까지 하고 감사·착지 절차대로 완료 보고 → 판정 → land(제출)까지 한다. 이 레인의 범위는 %[2]s..HEAD다 — 감사·승인도 그 범위만 본다.\n"+
		"2. 선행 %[1]s의 파일·커밋은 고치지 않는다. 선행 쪽을 고쳐야 하면 hc-lane report 막힘 - \"선행 %[1]s 쪽 수정 필요: <까닭>\" 을 부르고 멈춘다.\n"+
		"3. `git rebase origin/%[3]s` 하지 않는다(선행 커밋이 범위에 섞인다). 병합기가 선행 다음 차례로 넣고, 선행이 반송되면 이 제출은 주차돼 hc-fill이 다시 쌓는다.\n",
		pred, head(p, 8), b, time.Now().Format("2006-01-02 15:04"), p, first)
}

// stackNote <레인>: ls·status 한 마디
func stackNote(lane string) string {
	r := stackRec(lane)
	if r == nil {
		return "쌓아 띄움"
	}
	switch r.F(skState) {
	case "parked":
		return "쌓아 띄움 — 주차(선행 " + r.F(skPred) + " 반송)"
	case "restacked":
		return "다시 쌓음(선행 " + r.F(skPred) + " " + head(r.F(skP), 8) + ")"
	case "rebased-main":
		return "다시 쌓음(선행 빠짐 — 기본 가지 위로)"
	}
	return "쌓아 띄움(선행 " + r.F(skPred) + " " + head(r.F(skP), 8) + " 제출됨)"
}
