package main

// 띄우기 거절 멈춤·오래 이어진 일시 실패 알림(2026-11-24 fill-reject-hold — 한 레인이 「원격 브랜치 origin/… 가 이미 있다」로 2시간 18분
// 동안 278번(네 기계에 바퀴마다) 띄우기에 실패했는데 막힘 사건은 0건이었다).
// 기록 reject.tsv 한 줄 = 줄ID 레인 종류(reject|fail) 처음(초) 마지막(초) 알림(사건 키, 아직이면 -) 기계 까닭 [임대 값].
// reject: hc-lane 띄우기가 「다시 해도 같음」(종료 11 — 그 구분은 hc-lane이 한 곳에서 정한다, cmd/hc-lane launchaux.go)이면 그 줄을
// 멈춰 두고(다시 띄우지 않음) 주인 지휘에 한 번 알린다. 이것만 본다 — 종료 코드를 하나씩 덧대지 않는다(2026-11-28 ht-retry-class:
// 임대 「이미 돎」이 따로 종료 10이라 일시로 읽혀 51번 다시 띄웠다).
// 푸는 길: 지휘의 retry(기록을 지워 다음 바퀴에 다시 띄움)·edit(기록도 지움)·rm(줄 ID가 바뀌어 기록이 안 맞음), 그리고 hc-lane이
// 「풀리면 다시: 임대 <값>」을 준 거절은 바퀴마다 그 임대를 보아(hc-lane lease show 한 번) 값이 바뀌거나 없어지면 스스로 푼다(못 읽으면 그대로).
// 풀면 알린 막힘 열쇠 + :up 중간을 한 번(docs/hc-lane/watch.md 종류별 풀림).
// fail: 일시 실패(연결·hc·시간 넘김·미확정)는 지금처럼 다음 바퀴에 다시 띄우되, 같은 줄이 설정 launch_fail_warn분(기본 30, 0이면 끔)
// 넘게 이어 실패하면 한 번 알린다. 성공하면 지운다. 마지막 실패 뒤 그만큼 쉬었다 다시 실패하면 새로 센다

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const rejHC = 11 // hc-lane 띄우기의 입력 거절(cmd/hc-lane rcRefuse)

// REJWHY: launch가 입력 거절로 돌려준(4) 까닭 · REJLEASE: 그 거절이 풀릴 임대 값(없으면 빈 값) · LFWHY: 이번 launch 실패의 까닭(hc-lane 출력 끝, 모르면 빈 값)
var REJWHY, REJLEASE, LFWHY string

// rejWatchPre: hc-lane 「풀리면 다시: 임대 <sha>」 줄(cmd/hc-lane leaseWatchPre와 같은 글)
const rejWatchPre = "hc-lane: 풀리면 다시: 임대 "

// rejParse <hc-lane 출력>: 거절(종료 11)의 까닭(끝 줄)과 풀릴 임대 값
func rejParse(out string) {
	REJWHY, REJLEASE = runeCut(strings.TrimPrefix(lastLine(out), "hc-lane: "), 200), ""
	for _, l := range lines(out) {
		if v, ok := strings.CutPrefix(l, rejWatchPre); ok && v != "" && !strings.ContainsAny(v, " \t") {
			REJLEASE = v
		}
	}
}

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

// rejHeld <레인> <프로젝트> <주인> <되돌릴곳>: 이 줄(RID)이 띄우기 거절로 멈춰 있으면 그 까닭(아직 못 알렸으면 알린다), 아니면 빈 값.
// 임대 값이 있는 거절은 지금 임대가 그 값이 아니면(없어짐·다른 값) 풀고 빈 값 — 이 바퀴에 다시 띄운다
func rejHeld(lane, p, owner, reply string) string {
	r := rejRow(lane, RID, "reject")
	if r == nil {
		return ""
	}
	ow, repo := rejOwner(owner, reply), repoOf(p, r.F(rjMach))
	if v := dash(r.F(rjLease)); v != "-" && !dry && repo != "" {
		now, _, ok := rejLeaseNow(lane, r.F(rjMach), repo)
		if lr := repoOf(p, LOCAL); !ok && r.F(rjMach) != LOCAL && lr != "" { // 임대는 원격 참조 — 그 기계가 못 읽으면 이 기계 클론으로
			now, _, ok = rejLeaseNow(lane, LOCAL, lr)
		}
		if ok && now != v {
			how := "임대가 바뀌었다(" + now + ")"
			if now == "-" {
				how = "임대가 놓였다"
			}
			if rejEdit(func(x Row) bool { return x.F(rjRID) != RID || x.F(rjKind) != "reject" }, nil) == 0 {
				say("풀림 %s: 띄우기 거절의 %s — 다시 띄운다", lane, how)
				rejUp(r, ow, how+" — hc-fill이 다시 띄운다")
				return ""
			}
		}
	}
	rejTell(r, ow, rejMsg(r, repo))
	return r.F(rjWhy)
}

// rejLeaseNow <레인> <기계> <저장소>: 지금 임대 값(없으면 -)과 그 제목. ok=false면 모름(못 읽음 — 풂·놓음의 근거가 아니다)
func rejLeaseNow(lane, m, repo string) (sha, subj string, ok bool) {
	a := []string{"lease", "show"}
	if m != LOCAL {
		a = append(a, "--on", m)
	}
	o, rc := Cmd{Name: "hc-lane", Args: append(a, "--repo", repo, "--name", lane), Err: errPass}.Run() // 표준 출력만 — 섞인 오류 글을 값으로 읽어 헛되이 풀지 않게
	sha, subj, _ = strings.Cut(lastLine(sub(o)), " ")
	if rc != 0 || (sha != "-" && (len(sha) < 7 || strings.Trim(sha, "0123456789abcdef") != "")) {
		return "", "", false
	}
	return sha, subj, true
}

// rejUp <기록> <주인> <어떻게>: 알린 거절 막힘이 풀렸다 — 같은 열쇠 + :up 중간 한 번(못 적으면 그 레인의 뒤 보고가 풀림 노릇)
func rejUp(r Row, owner, how string) {
	if t := r.F(rjTold); t != "-" && t != "" && !dry {
		ao := owner
		if ao == "-" {
			ao = ""
		}
		if hcLaneReport("중간", r.F(rjLane)+" 띄우기 거절 풀림 — "+how, r.F(rjLane), ao, t+":up") != 0 {
			say("경고 %s: 거절 풀림 사건을 못 적음(그 레인의 뒤 보고가 풀림 노릇)", r.F(rjLane))
		}
	}
}

func rejMsg(r Row, repo string) string {
	fix := "까닭을 고쳤으면 hc-fill retry " + r.F(rjLane)
	if dash(r.F(rjLease)) != "-" {
		on := ""
		if r.F(rjMach) != LOCAL {
			on = " --on " + r.F(rjMach)
		}
		fix = "임대가 놓이거나 바뀌면 hc-fill이 스스로 다시 띄운다 — 끝난 실행이 남긴 임대면 hc-lane lease drop" + on + " --repo " + repo + " --name " + r.F(rjLane) +
			"(살아 있는 실행이면 끝나기를 기다린다), 곧바로 다시 보려면 hc-fill retry " + r.F(rjLane)
	}
	return fmt.Sprintf("%s 띄우기 거절 — %s. 다시 띄우지 않고 멈춰 둠(이 거절은 @%s 한 곳의 것 — 그 기계만의 까닭이면 edit --only로 다른 기계를 고를 수 있음): "+
		"%s, 줄을 바꾸려면 hc-fill edit, 빼려면 hc-fill rm", r.F(rjLane), r.F(rjWhy), r.F(rjMach), fix)
}

// rejHold <레인> <기계> <저장소> <까닭> <임대 값|""> <주인> <되돌릴곳>: 다시 해도 같음 — 멈춰 두고 한 번 알린다
func rejHold(lane, m, repo, why, lease, owner, reply string) {
	if dry {
		return
	}
	now := strconv.FormatInt(time.Now().Unix(), 10)
	r := Row{RID, lane, "reject", now, now, "-", m, flat1(why) + " (@" + m + ")", dash(lease)}
	if rejPut(r) != 0 {
		say("경고 %s: 띄우기 거절 기록을 못 남김 — 다음 바퀴에 다시 띄운다", lane)
		return
	}
	say("띄우기 거절 %s @%s: %s — 이 줄을 멈춰 둠(지휘가 retry·edit·rm)", lane, m, why)
	rejTell(r, rejOwner(owner, reply), rejMsg(r, repo))
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

// rejDrop <레인>: 그 레인의 기록을 모두 지운다(retry·edit) — 알린 거절이면 풀림(:up) 한 번. 지운 수(-1 못 씀)
func rejDrop(lane string) int {
	var old []Row
	for _, r := range readRows(DATA + "/reject.tsv") {
		if r.F(rjLane) == lane {
			old = append(old, r)
		}
	}
	if len(old) > 0 && rejEdit(func(x Row) bool { return x.F(rjLane) != lane }, nil) != 0 {
		return -1
	}
	if q := qrow(lane); q != nil {
		for _, r := range old {
			if r.F(rjKind) == "reject" && r.F(rjRID) == q.F(qRID) {
				rejUp(r, rejOwner(q.F(qOwner), q.F(qReply)), "지휘가 풀었다(retry·edit) — 다음 바퀴에 다시 띄운다")
			}
		}
	}
	return len(old)
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
