package main

// advise 명령(2026-11-24 ops-advise, docs/ops/advise.md — 사용자 「워커들 놀지 말고 다 쉬는것 없이 다 돌려서 그 시간이야 ?」):
// 지금 손댈 것의 목록. 읽기만 한다 — 재료는 상태 한 장(stateCollectF)과 같고, 판정은 internal/opsstate.Advise.
// adviseTick: run 바퀴가 advise_every분마다 목록을 다시 보고 새 항목이 생기면 운영 사건 hc-fill-advise 한 번(항목 열쇠로 되풀이 없음)

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/events"
	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/journal"
	"herdr-tools/internal/opsstate"
)

// adviseAgentTakes: 운영 에이전트가 조치 방식이고 띄울 준비(명령·틀)가 되었고 새 항목이 모두 그 범위 안인가 — 그러면 그 항목은
// 운영 에이전트가 맡는다(opsagent.go). 판단만·끔·범위 밖이 하나라도 있으면 거짓(지휘에 지금처럼 바로)
func adviseAgentTakes(fresh []opsstate.Item) bool {
	if oaMode() != "조치" || confRaw(oaLaunchK) == "" {
		return false
	}
	if _, ok := readFile(oaTmplPath()); !ok {
		return false
	}
	return len(oaScoped(fresh)) == len(fresh)
}

func init() { register("advise", adviseCmd) }

// adviseCmd: advise [--json]
func adviseCmd(args []string) {
	js := false
	for _, a := range args {
		if a != "--json" || js {
			usage()
		}
		js = true
	}
	a := adviseNow(time.Now())
	var err error
	if js {
		err = opsstate.AdviseJSON(os.Stdout, a)
	} else {
		err = opsstate.AdviseHuman(os.Stdout, a)
	}
	if err != nil {
		die("%v", err)
	}
	exit(0)
}

// adviseNow: 상태 한 장 기본(추린 것)과 같은 상태로 손댈 것 — 지문도 state 기본과 같다
func adviseNow(now time.Time) opsstate.Advice {
	s, f := stateCollectF(now)
	tr := opsstate.Trim(s, knobInt("state_done"))
	it := opsstate.Advise(tr, f, stateThresholds())
	tr.Notable = opsstate.Summaries(it)
	opsstate.Claims(it, s, f.Acts, journal.Who(), now, time.Duration(knobInt("advise_claim"))*time.Minute) // 지문·요약 뒤 — 조치함은 거기 안 든다
	return opsstate.Advice{Schema: opsstate.AdviseSchema, At: now, Fingerprint: opsstate.Fingerprint(tr), Items: it}
}

// adviseWait <대기 줄>: 그 줄이 무엇을 기다리나 — 거절·겹침(overlap.tsv, run 바퀴가 적음)·선행, 띄울 수 있는 기계
func adviseWait(r Row, now time.Time) opsstate.Wait {
	name, p := r.F(qName), r.F(qProj)
	w := opsstate.Wait{Lane: name, Project: p, Reject: rejWhy(name, r.F(qRID)), Can: []string{}}
	if a, ok := stateAddedAge(r.F(qAdded), now); ok {
		w.AgeS = ptrI(a)
	}
	if a := dash(r.F(qAfter)); a != "-" {
		w.After = words(a)
	}
	if x := lsFirst(DATA+"/overlap.tsv", name); x != nil {
		w.Front = x.F(wtFront)
		switch k := x.F(wtKey); {
		case k == "단독":
			w.Solo = true
		case k != "" && k != "-":
			w.Keys = strings.Split(k, ",")
		}
	}
	only, nd := dash(r.F(qOnly)), needs(r.F(qBrief)) // 지시문 「필요:」 능력 없는 기계엔 안 뜬다(placeCands와 같게 — 감사 atp22es 낮음 1)
	light, _ := laneWeight(r)
	for _, mc := range machines() {
		switch {
		case repoOf(p, mc.Name) == "" || !mhas(mc.Name, nd):
		case !light && machLight(mc.Name): // 보통 레인은 가벼운 일만 받는 기계에 안 뜬다(placeCands와 같게, weight.go)
			if only == "-" || inlist(mc.Name, only) {
				w.LightOnly = append(w.LightOnly, mc.Name)
			}
		case only == "-" || inlist(mc.Name, only):
			w.Can = append(w.Can, mc.Name)
		default:
			w.Wider = append(w.Wider, mc.Name)
		}
	}
	if why, since := pinOf(name, r.F(qRID)); why != "" && since > 0 {
		w.Pin, w.PinS = why, ptrI(int(max(0, now.Unix()-since)))
	}
	return w
}

// adviseKnown: 이미 사건으로 알린 열쇠(<DATA>/advise.tsv 둘째 줄부터) — opsstate.Facts.Known
func adviseKnown() map[string]bool {
	k := map[string]bool{}
	if ls := readLines(DATA + "/advise.tsv"); len(ls) > 1 {
		for _, x := range ls[1:] {
			k[x] = true
		}
	}
	return k
}

// adviseHeld <손댈 것>: 지난 열쇠 가운데 이번에 판정 못 한 재료로 찾는 것(읽었고 항목이 없을 때만 지운다). 병합기 모름은 그 프로젝트의
// merger·merge-stall·merge-slow, 배포 모름은 그 기계(재료 deploy 모름이면 모든 deploy), 감시 모름은 그 지휘, 상주 판 모름은 resident.
// hc·바퀴 기록·기계 연결 모름은 자리·레인 판정 모두가 기대므로 전부, 모르는 꼴도 전부(모름은 모름). 일지(journal)는 손댈 것의 재료가 아니다
func adviseHeld(items []opsstate.Item) func(string) bool {
	all := false
	var exact, pre []string
	for _, it := range items {
		if it.Kind != "unknown" {
			continue
		}
		k := strings.TrimPrefix(it.Key, "unknown:")
		kind, name, _ := strings.Cut(k, ":")
		switch {
		case k == "source:journal":
		case k == "source:deploy":
			pre = append(pre, "deploy:")
		case k == "resident":
			pre = append(pre, "resident:")
		case kind == "deploy" || kind == "watch":
			exact = append(exact, k)
		case kind == "merger":
			exact = append(exact, k, "merge-stall:"+name, "merge-slow:"+name)
		default:
			all = true
		}
	}
	return func(key string) bool {
		if all || has(exact, key) {
			return true
		}
		for _, p := range pre {
			if strings.HasPrefix(key, p) {
				return true
			}
		}
		return false
	}
}

// adviseMsg <새 항목>: 사건 글 「손댈 것: <첫 요약> · 조치함: <…> 외 N」 — 첫 항목 말고 조치함이 있으면 그 수. 사건 줄은 탭 구분이라 탭·줄바꿈을 없앤다
func adviseMsg(its []opsstate.Item) string {
	msg := "손댈 것: " + its[0].Summary()
	if c := its[0].Claim; c != "" {
		msg += " · 조치함: " + c
	}
	if len(its) > 1 {
		msg += fmt.Sprintf(" 외 %d", len(its)-1)
		k := 0
		for _, it := range its[1:] {
			if it.Claim != "" {
				k++
			}
		}
		if k > 0 {
			msg += fmt.Sprintf("(그 가운데 조치함 %d — hc-fill advise)", k)
		}
	}
	return strings.Join(strings.Fields(msg), " ")
}

// adviseRoute <새 항목>: 레인 항목(굶는 감사·끝났는데 보고 없음·묶인 기계 등)은 그 줄의 주인 지휘에게만 손댈 것 — 주인마다 한 사건
// (ht-advise-only-stuck·ht-advise-claim: 같은 알림을 지휘 둘이 각자 받아 따로 손댔다). 주인이 없거나, 주인 터미널이 확인된 없음·끝남·셸만
// 남음이면(사라진 주인의 watch로만 가면 아무 지휘도 못 받는다 — 감사 atp2vpf 중간 1) rest(모두에게). 주인 조회 모름은 주인에게
func adviseRoute(fresh []opsstate.Item) (owners []string, byOwner map[string][]opsstate.Item, rest []opsstate.Item) {
	byOwner = map[string][]opsstate.Item{}
	gone := map[string]bool{}
	for _, it := range fresh {
		owner := "-"
		if l := it.Lane(); l != "" {
			owner = qown(l)
		}
		if g, seen := gone[owner]; owner != "-" && !seen {
			t, found, err := hcGet(owner)
			g = err == nil && (!found || t.State != "live" || hcjson.SendBlocked(t, found, err) != "") // hc-lane getState와 같은 기준 + 셸만 남음
			gone[owner] = g
		}
		if owner == "-" || gone[owner] {
			rest = append(rest, it)
			continue
		}
		if byOwner[owner] == nil {
			owners = append(owners, owner)
		}
		byOwner[owner] = append(byOwner[owner], it)
	}
	return owners, byOwner, rest
}

// adviseKey <한 사건의 항목>: 조치 방식 운영 에이전트가 다 맡을 항목이면 키(지휘에는 묶음 — coord-digest, 지휘 watch의 digest.go가
// 이 키 머리로 가른다), 아니면 빈 값. 사건마다 따로 본다(주인 사건·모두에게 사건 — 감사 atp2vpf 낮음 1)
func adviseKey(its []opsstate.Item, now time.Time) string {
	if adviseAgentTakes(its) {
		return events.AdviseAgentKey + strconv.FormatInt(now.Unix(), 10)
	}
	return ""
}

// adviseTick <지금>: advise_every분(손잡이, 0이면 끔)마다 손댈 것을 다시 보고, 지난번 목록(<DATA>/advise.tsv — 첫 줄 본 초, 그다음
// 줄마다 열쇠)에 없던 항목이 생기면 운영 사건 한 번. 판정 못 함은 사건으로 내지 않는다. 보내기를 실패하면 그 열쇠를 안 적어 다음에 다시
func adviseTick(now time.Time) {
	every := knobInt("advise_every")
	if every == 0 || dry {
		return
	}
	p := DATA + "/advise.tsv"
	ls := readLines(p)
	prev := map[string]bool{}
	if len(ls) > 0 {
		if t, err := strconv.ParseInt(ls[0], 10, 64); err == nil && now.Unix()-t < int64(every)*60 && now.Unix() >= t {
			return
		}
		for _, k := range ls[1:] {
			prev[k] = true
		}
	}
	var keep []string
	var fresh []opsstate.Item
	items := adviseNow(now).Items
	// 재료를 못 읽은 바퀴: 그 재료로 찾는 지난 열쇠는 그대로 두고(지우면 회복한 바퀴에 같은 사건이 다시 나갔다), 그 재료에 딸린 새 항목도
	// 보내지 않고 미룬다(hc ls 한 번 못 읽을 때마다 단계 「모름」 레인이 lane-long으로 새로 나갔다) — 다음에 읽힌 바퀴에도 있으면 그때 한 번(advise-fixes)
	held := adviseHeld(items)
	now1 := map[string]bool{}
	for _, it := range items {
		now1[it.Key] = true
		switch {
		case it.Kind == "unknown":
		case prev[it.Key]:
			keep = append(keep, it.Key)
		case held(it.Key):
		default:
			fresh = append(fresh, it)
		}
	}
	for _, k := range ls[min(1, len(ls)):] {
		if !now1[k] && held(k) {
			keep = append(keep, k)
			now1[k] = true
		}
	}
	owners, byOwner, rest := adviseRoute(fresh)
	var refs []string
	for _, o := range owners {
		its := byOwner[o]
		if hcLaneReport("중간", adviseMsg(its), "hc-fill-advise", o, adviseKey(its, now)) != 0 {
			say("손댈 것 알림 실패(hc-lane report --to %s) — 다음에 다시", o)
			continue
		}
		for _, it := range its {
			keep = append(keep, it.Key)
			refs = append(refs, it.Key+" → "+o)
		}
	}
	// 참고는 실패해도 다시 보내지 않는다 — 손댈 것은 주인에게 갔다
	if len(refs) > 0 && hcLaneReport("중간", "손댈 것 참고 — 주인 지휘에 보냄: "+strings.Join(refs, ", "), "hc-fill-advise", "",
		events.AdviseRefKey+strconv.FormatInt(now.Unix(), 10)) != 0 {
		say("손댈 것 참고 알림 실패(hc-lane report) — 다시 보내지 않음")
	}
	fresh = rest
	if len(fresh) > 0 {
		// 이름은 events.OpsAdvise — 이름 표 시험(internal/events ops_test)이 글자로 찾는다
		if hcLaneReport("중간", adviseMsg(fresh), "hc-fill-advise", "", adviseKey(fresh, now)) == 0 {
			for _, it := range fresh {
				keep = append(keep, it.Key)
			}
		} else {
			say("손댈 것 알림 실패(hc-lane report) — 다음에 다시")
		}
	}
	if err := writeAtomic(p, strings.Join(append([]string{strconv.FormatInt(now.Unix(), 10)}, keep...), "\n")+"\n"); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
	}
}
