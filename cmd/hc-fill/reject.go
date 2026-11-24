package main

// 띄우기 거절 멈춤·오래 이어진 일시 실패 알림(2026-11-24 fill-reject-hold — 한 레인이 「원격 브랜치 origin/… 가 이미 있다」로 2시간 18분
// 동안 278번(네 기계에 바퀴마다) 띄우기에 실패했는데 막힘 사건은 0건이었다).
// 기록 reject.tsv 한 줄 = 줄ID 레인 종류(reject|fail) 처음(초) 마지막(초) 알림(사건 키, 아직이면 -) 기계 까닭.
// reject: hc-lane 띄우기가 입력 거절(종료 11 — 다시 해도 같음)이면 그 줄을 멈춰 두고(다시 띄우지 않음) 주인 지휘에 한 번 알린다.
// 푸는 길은 지휘가 정한다: retry(기록을 지워 다음 바퀴에 다시 띄움)·edit(기록도 지움)·rm(줄 ID가 바뀌어 기록이 안 맞음).
// fail: 일시 실패(연결·hc·시간 넘김·미확정)는 지금처럼 다음 바퀴에 다시 띄우되, 같은 줄이 설정 launch_fail_warn분(기본 30, 0이면 끔)
// 넘게 이어 실패하면 한 번 알린다. 성공하면 지운다. 마지막 실패 뒤 그만큼 쉬었다 다시 실패하면 새로 센다

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const rejHC = 11 // hc-lane 띄우기의 입력 거절(cmd/hc-lane rcRefuse)

// REJWHY: launch가 입력 거절로 돌려준(4) 까닭 · LFWHY: 이번 launch 실패의 까닭(hc-lane 출력 끝, 모르면 빈 값)
var REJWHY, LFWHY string

func init() { register("retry", retryCmd) }

// rejRow <레인> <줄ID> <종류>: 그 기록(없으면 nil)
func rejRow(lane, rid, kind string) Row {
	for _, r := range readRows(DATA + "/reject.tsv") {
		if r.F(rjRID) == rid && r.F(rjLane) == lane && r.F(rjKind) == kind {
			return r
		}
	}
	return nil
}

// rejEdit: 대기열 잠금 안에서 기록을 고쳐 쓴다 — keep이 거짓인 줄은 지우고, add가 있으면 덧붙인다. 대기열에 없는 줄 ID의 기록은 함께 거둔다.
// 0 됨 · 그 밖은 못 씀
func rejEdit(keep func(Row) bool, add Row) int {
	rc := 1
	if w := withQ(func() {
		live := map[string]bool{}
		for _, q := range qRows() {
			live[q.F(qRID)] = true
		}
		var out []Row
		for _, r := range readRows(DATA + "/reject.tsv") {
			if live[r.F(rjRID)] && keep(r) {
				out = append(out, r)
			}
		}
		if add != nil {
			out = append(out, add)
		}
		if err := writeAtomic(DATA+"/reject.tsv", rowsText(out)); err != nil {
			say("경고: 띄우기 거절 기록을 쓰지 못함: %v", err)
			return
		}
		rc = 0
	}); w != 0 {
		return w
	}
	return rc
}

// rejPut: 그 줄ID·종류 기록을 r로 바꾼다
func rejPut(r Row) int {
	return rejEdit(func(x Row) bool { return x.F(rjRID) != r.F(rjRID) || x.F(rjKind) != r.F(rjKind) }, r)
}

// rejOwner: 알릴 지휘 — 받은편지 줄은 주인 없음(-, 모든 지휘. stallTick과 같음)
func rejOwner(owner, reply string) string {
	if _, _, inbox := replyOf(dash(reply)); inbox {
		return "-"
	}
	return dash(owner)
}

// rejTell: 아직 안 알린 기록이면 알리고 키를 적는다(넘기기 실패면 다음 바퀴에 다시 — 같은 키라 hc-lane report가 한 번만 낸다)
func rejTell(r Row, owner, msg string) {
	if r.F(rjTold) != "-" {
		return
	}
	key := "hf:" + r.F(rjLane) + ":" + r.F(rjKind) + ":" + r.F(rjRID) + ":" + r.F(rjFirst)
	if aesc(r.F(rjLane), owner, msg, key) == 0 && !dry {
		rejPut(r.Set(rjTold, key))
	}
}

// rejHeld <레인> <주인> <되돌릴곳>: 이 줄(RID)이 띄우기 거절로 멈춰 있으면 그 까닭(아직 못 알렸으면 알린다), 아니면 빈 값
func rejHeld(lane, owner, reply string) string {
	r := rejRow(lane, RID, "reject")
	if r == nil {
		return ""
	}
	rejTell(r, rejOwner(owner, reply), rejMsg(r))
	return r.F(rjWhy)
}

func rejMsg(r Row) string {
	return fmt.Sprintf("%s 띄우기 거절 — %s. 다시 띄우지 않고 멈춰 둠(이 거절은 @%s 한 곳의 것 — 그 기계만의 까닭이면 edit --only로 다른 기계를 고를 수 있음): "+
		"까닭을 고쳤으면 hc-fill retry %s, 줄을 바꾸려면 hc-fill edit, 빼려면 hc-fill rm", r.F(rjLane), r.F(rjWhy), r.F(rjMach), r.F(rjLane))
}

// rejHold <레인> <기계> <까닭> <주인> <되돌릴곳>: 입력 거절 — 멈춰 두고 한 번 알린다
func rejHold(lane, m, why, owner, reply string) {
	if dry {
		return
	}
	now := strconv.FormatInt(time.Now().Unix(), 10)
	r := Row{RID, lane, "reject", now, now, "-", m, flat1(why) + " (@" + m + ")"}
	if rejPut(r) != 0 {
		say("경고 %s: 띄우기 거절 기록을 못 남김 — 다음 바퀴에 다시 띄운다", lane)
		return
	}
	say("띄우기 거절 %s @%s: %s — 이 줄을 멈춰 둠(지휘가 retry·edit·rm)", lane, m, why)
	rejTell(r, rejOwner(owner, reply), rejMsg(r))
}

// rejFail <레인> <까닭> <주인> <되돌릴곳>: 이번 바퀴의 일시 실패를 센다 — 상한을 넘겼으면 한 번 알린다
func rejFail(lane, why, owner, reply string) {
	lim := auMinutes("launch_fail_warn", 30)
	if dry || lim == 0 {
		return
	}
	now := time.Now().Unix()
	r := rejRow(lane, RID, "fail")
	if r == nil || now-int64(num(r.F(rjLast))) > int64(lim)*60 {
		r = Row{RID, lane, "fail", strconv.FormatInt(now, 10), "", "-", "-", ""}
	}
	if why == "" {
		why = "까닭은 hc-fill 바퀴 출력에"
	}
	r = r.Set(rjLast, strconv.FormatInt(now, 10)).Set(rjWhy, flat1(why))
	if rejPut(r) != 0 {
		return
	}
	if mins := (now - int64(num(r.F(rjFirst)))) / 60; mins >= int64(lim) {
		rejTell(r, rejOwner(owner, reply), fmt.Sprintf("%s 띄우기가 %d분째 실패 — 마지막 %s. 바퀴마다 다시 띄우는 중(상한 설정 launch_fail_warn)", lane, mins, r.F(rjWhy)))
	}
}

// rejOK <레인>: 띄웠다 — 일시 실패 기록을 지운다
func rejOK(lane string) {
	if !dry && rejRow(lane, RID, "fail") != nil {
		rejEdit(func(x Row) bool { return x.F(rjRID) != RID || x.F(rjKind) != "fail" }, nil)
	}
}

// rejDrop <레인>: 그 레인의 기록을 모두 지운다(retry·edit). 지운 수(-1 못 씀)
func rejDrop(lane string) int {
	n := 0
	for _, r := range readRows(DATA + "/reject.tsv") {
		if r.F(rjLane) == lane {
			n++
		}
	}
	if n > 0 && rejEdit(func(x Row) bool { return x.F(rjLane) != lane }, nil) != 0 {
		return -1
	}
	return n
}

// rejWhy <레인> <줄ID>: ls·status 표시용 — 멈춰 있으면 까닭, 아니면 빈 값
func rejWhy(lane, rid string) string {
	if r := rejRow(lane, rid, "reject"); r != nil {
		return r.F(rjWhy)
	}
	return ""
}

// retry <레인>: 띄우기 거절로 멈춘 줄을 다음 바퀴에 다시 띄운다
func retryCmd(args []string) {
	args, why := takeWhy(args)
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		usage()
	}
	name := args[0]
	if !qHasName(name) {
		DRC = 2
		noLane(name)
	}
	held := rejWhy(name, curRid(name))
	switch n := rejDrop(name); {
	case n < 0:
		die("띄우기 거절 기록을 고치지 못했다: %s", DATA+"/reject.tsv")
	case held == "":
		say("멈춘 줄이 아니다 %s — 그대로(다음 바퀴에 평소대로)", name)
	default:
		say("다시 시도 %s — 다음 바퀴에 띄운다(지난 거절: %s)", name, held)
		jot("retry", name, why, "됨", "-", "지난 거절: "+held)
	}
}
