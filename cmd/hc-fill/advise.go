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

	"herdr-tools/internal/opsstate"
)

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
	return opsstate.Advice{Schema: opsstate.AdviseSchema, At: now, Fingerprint: opsstate.Fingerprint(tr), Items: it}
}

// adviseWait <대기 줄>: 그 줄이 무엇을 기다리나 — 거절·겹침(overlap.tsv, run 바퀴가 적음)·선행, 띄울 수 있는 기계, 다시 넣는 add 한 줄
func adviseWait(r Row, now time.Time) opsstate.Wait {
	name, p := r.F(qName), r.F(qProj)
	w := opsstate.Wait{Lane: name, Project: p, Reject: rejWhy(name, r.F(qRID)), Parked: r.F(qEarly) == "parked", Can: []string{}}
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
	only := dash(r.F(qOnly))
	for _, mc := range machines() {
		if repoOf(p, mc.Name) != "" && (only == "-" || inlist(mc.Name, only)) {
			w.Can = append(w.Can, mc.Name)
		}
	}
	// 받은편지 줄은 rm이 요청을 취소한다 — 다시 넣는 줄을 내지 않는다(사람이 정한다)
	if _, _, inbox := replyOf(r.F(qReply)); !inbox {
		w.Readd = rmUndo(r)
		if r.F(qEarly) == "no-early" {
			w.Readd += " --no-early"
		}
		if r.F(qAudit) == "no-audit" {
			w.Readd += " --no-audit"
		}
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
	for _, it := range adviseNow(now).Items {
		switch {
		case it.Kind == "unknown":
		case prev[it.Key]:
			keep = append(keep, it.Key)
		default:
			fresh = append(fresh, it)
		}
	}
	if len(fresh) > 0 {
		msg := "손댈 것: " + fresh[0].Summary()
		if len(fresh) > 1 {
			msg += fmt.Sprintf(" 외 %d", len(fresh)-1)
		}
		msg = strings.Join(strings.Fields(msg), " ") // 사건 줄은 탭 구분 — 글 안의 탭·줄바꿈을 없앤다
		// 이름은 events.OpsAdvise — 이름 표 시험(internal/events ops_test)이 글자로 찾는다
		if hcLaneReport("중간", msg, "hc-fill-advise", "", "") == 0 {
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
