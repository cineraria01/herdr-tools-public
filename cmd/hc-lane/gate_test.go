package main

// ht-alert-gate(2026-11-28): 지휘에게 가는 알림은 한 문(gate.go judge)을 지난다 — 받는 쪽 한 명·갈래·까닭을 사건과 그 창만으로.
// 출발점: 15시간 묵은 남의 저장소 정리가 「뜻밖의 종료 · 최근 11분」으로 엉뚱한 지휘에 갔다(지시문 여섯 장면 ①)

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/events"
	"herdr-tools/internal/knob"
	"herdr-tools/internal/tri"
)

const repoA, repoB = "example.com/team/a", "example.com/team/b"

// gl: 13칸 사건 줄(sha -, turn 0, 실행 <터미널>@1)
func gl(at time.Time, term, lane, state, msg, to, owner, key, repo string) string {
	return strings.Join([]string{at.UTC().Format(time.RFC3339), "m", term, lane, state, "-", msg, "0", to, term + "@1", owner, key, repo}, tab)
}

// clean8: hc-sweep이 쓰는 8칸 정리됨
func clean8(at time.Time, term, lane string) string {
	return strings.Join([]string{at.UTC().Format(time.RFC3339), "m", term, lane, "정리됨", "-", "hc-sweep R2", "0"}, tab)
}

// stubWorld: 순수 판정 재료 — live에 없는 지휘는 받을 수 없음, reps는 저장소 → 대표 후보 차례
func stubWorld(now time.Time, hist []string, live map[string]tri.State, reps map[string][]string) *gworld {
	w := &gworld{now: now, repo: laneRepos(hist), laneOwner: func(string) string { return "-" },
		knob: func(n string) string { return map[string]string{"advise_wait": "15", "alert_escalate_min": "30"}[n] },
		recv: func(a, _ string) tri.State {
			if s, ok := live[a]; ok {
				return s
			}
			return tri.No
		},
		reps: func(r string) []string {
			var o []string
			for _, a := range reps[r] {
				if live[a] != tri.No {
					o = append(o, a)
				}
			}
			return o
		}}
	for _, l := range hist {
		w.hist = append(w.hist, events.Cols(l))
	}
	return w
}

// senders: 그 판정을 지금 보내는 지휘(차례대로)
func senders(v gverdict, now time.Time, cs ...string) string {
	var o []string
	for _, c := range cs {
		if ok, _ := gateSend(v, c, now); ok {
			o = append(o, c)
		}
	}
	return strings.Join(o, ",")
}

var three = map[string]tri.State{"ca": tri.Yes, "cb": tri.Yes, "cc": tri.Yes}

// ① 주인 없는 사건 → 대표 한 명 ② --to(주인 후보)가 죽었으면 같은 규칙으로 대표, 살아 있으면 그 지휘 ⑫ 모르는 종류 → 판단 필요로 간다
func TestGateOneRecipient(t *testing.T) {
	now := time.Now()
	reps := map[string][]string{repoA: {"cb", "ca"}, "": {"cc", "cb", "ca"}}
	l := gl(now, "t8", "inbox-x", "막힘", "요약", "-", "*", "-", repoA)
	w := stubWorld(now, []string{l}, three, reps)
	v := w.judge(events.Cols(l))
	if got := senders(v, now, "ca", "cb", "cc"); got != "cb" || v.br != brNow || !strings.Contains(v.tag, "대표 지휘 cb") {
		t.Errorf("① 저장소 대표 cb 한 명: %q %v %q", got, v.br, v.tag)
	}
	lt := gl(now, "t8", "inbox-x", "완료", "끝", "cz", "-", "-", repoA)
	if got := senders(stubWorld(now, []string{lt}, three, reps).judge(events.Cols(lt)), now, "ca", "cb", "cc"); got != "cb" {
		t.Errorf("② --to 지휘 cz가 없음 → 대표 cb 한 명: %q", got)
	}
	lt = gl(now, "t8", "inbox-x", "완료", "끝", "cc", "-", "-", repoA)
	if v := stubWorld(now, []string{lt}, three, reps).judge(events.Cols(lt)); senders(v, now, "ca", "cb", "cc") != "cc" || v.tag != "" {
		t.Errorf("② --to 지휘 cc가 살아 있음 → cc 한 명(꼬리 없음): %+v", v)
	}
	lg := gl(now, "-", "merger-x", "이상함", "?", "-", "*", "-", "")
	if v := stubWorld(now, []string{lg}, three, reps).judge(events.Cols(lg)); v.br != brNow || v.kind != "모르는 종류" || senders(v, now, "ca", "cb", "cc") != "cc" {
		t.Errorf("⑫ 모르는 종류는 판단 필요로 전역 대표에게: %+v", v)
	}
	// 받는 쪽 확인 못 함 → 판단 필요는 모두(안 가는 쪽이 위험), 소식은 그 한 명
	unk := map[string]tri.State{"ca": tri.Yes, "cb": tri.Unknown, "cc": tri.Yes}
	if got := senders(stubWorld(now, []string{l}, unk, reps).judge(events.Cols(l)), now, "ca", "cb", "cc"); got != "ca,cb,cc" {
		t.Errorf("외부 가정: 대표 확인 못 함 → 판단 필요는 모두: %q", got)
	}
	ln := gl(now, "t8", "inbox-x", "완료", "끝", "-", "*", "-", repoA)
	if got := senders(stubWorld(now, []string{ln}, unk, reps).judge(events.Cols(ln)), now, "ca", "cb", "cc"); got != "cb" {
		t.Errorf("외부 가정: 소식은 모름이어도 한 명: %q", got)
	}
}

// ③ 8칸 정리됨(지어낸 멈춤)은 같은 터미널·레인의 앞 사건에서 주인·저장소를 물려받는다 ⑤ 끝 보고 없이 사라짐 → 판단 필요 한 번·한 명
func TestGateInheritClose(t *testing.T) {
	now := time.Now()
	mid := gl(now.Add(-40*time.Minute), "t9", "lane-q", "중간", "작업 중", "-", "ca@1", "-", repoB)
	name, ok := closeName("", "hc-sweep R2", false)
	if !ok || name != "보고 없이 쉬다 닫힘(hc-sweep R2)" {
		t.Fatalf("⑤ 보고 없이 닫힘 이름: %q %v", name, ok)
	}
	x := events.Cols(clean8(now, "t9", "lane-q"))
	x.State, x.SHA, x.Msg = "멈춤", "-", name
	w := stubWorld(now, []string{mid, x.Raw()}, three, map[string][]string{"": {"cc"}})
	v := w.judge(x)
	if v.kind != "뜻밖의 종료" || v.br != brNow || senders(v, now, "ca", "cb", "cc") != "ca" {
		t.Errorf("③⑤ 주인 ca를 물려받아 한 명: %+v", v)
	}
}

// ④ 끝 보고·넘김·알린 막힘 뒤 정리됨 → 알림 없음 — 「막힘 풀림」 중간이 끼어도
func TestGateCloseQuiet(t *testing.T) {
	for _, end := range []string{"완료", "판정", "제출", "착지", "주차", "막힘"} {
		if why, ok := closeName(end, "hc-sweep R2", false); ok || !strings.Contains(why, end) {
			t.Errorf("④ %s 뒤 정리됨은 조용히: %q %v", end, why, ok)
		}
	}
	if _, ok := closeName("", "hc-sweep R2", true); ok {
		t.Error("④ 쉼을 이미 알렸으면 조용히")
	}
	// newEvents: 막힘 → 중간(막힘 풀림) → 정리됨 은 끝 상태가 막힘 그대로(중간이 지우지 않음)
	k := newCwdKit(t)
	k.write("st/hc-lane/lanes.tsv", "t5"+tab+"lane-c"+tab+"-\n")
	now := time.Now()
	e := &ev{ed: k.d + "/st/hc-lane", cur: k.d + "/cur"}
	ls := []string{gl(now.Add(-35*time.Minute), "t5", "lane-c", "막힘", "막힘 넘김: clean 멈춤", "-", "*", "-", repoA),
		gl(now.Add(-33*time.Minute), "t5", "lane-c", "중간", "중간 막힘 풀림: clean 성공", "-", "*", "-", repoA),
		clean8(now, "t5", "lane-c")}
	for _, o := range e.newEvents(nil, ls, "") {
		if x := events.Cols(o); x.State == "멈춤" {
			t.Errorf("④ 막힘 → 막힘 풀림 → 정리됨 이 멈춤으로: %q", o)
		}
	}
	ls[0] = gl(now.Add(-35*time.Minute), "t5", "lane-c", "중간", "작업 중", "-", "*", "-", repoA)
	n := 0
	for _, o := range e.newEvents(nil, ls, "") {
		if strings.Contains(o, "보고 없이 쉬다 닫힘(hc-sweep R2)") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("⑤ 끝 보고 없이 정리됨 → 한 번: %d", n)
	}
}

// 감사 atp7hcw M1: 넘김 막힘 10초 뒤 clean 메모(「막힘 풀림: hc-lane clean 성공」)가 한 바퀴에 들어도 막힘은 풀리지 않고 한 명에게 간다 —
// 메모는 정리가 끝났다는 뜻이지 일이 다시 움직였다는 뜻이 아니다. 뒤 정리됨은 끝 상태 막힘이라 조용(closeName)
func TestGateCleanMemo(t *testing.T) {
	now := time.Now()
	blk := gl(now.Add(-20*time.Second), "t5", "lane-c", "막힘", "넘김: 범위 밖이라 고치지 않음", "-", "*", "-", repoA)
	memo := gl(now.Add(-10*time.Second), "t5", "lane-c", "중간", cleanMemo+" — lane-c 작업 트리·가지 정리 끝", "-", "*", "-", repoA)
	w := stubWorld(now, []string{blk, memo}, three, map[string][]string{"": {"ca", "cb"}})
	v := w.judge(events.Cols(blk))
	if v.br != brNow || !v.open || senders(v, now, "ca", "cb", "cc") != "ca" {
		t.Errorf("넘김 막힘 + clean 메모 → 막힘이 한 명에게: %+v", v)
	}
	if _, ok := closeName("막힘", "hc-sweep R2", false); ok {
		t.Error("막힘 뒤 정리됨은 조용히")
	}
}

// ⑥ 묵은 사건 → 안 보냄(받는 쪽 지휘는 기록 한 줄), 안 풀린 내 판단 필요 → 보냄(시각 붙음). 풀린 묵은 막힘·소식은 안 보냄
func TestGateStale(t *testing.T) {
	now := time.Now()
	old := now.Add(-15 * time.Hour)
	reps := map[string][]string{"": {"ca", "cb"}}
	news := gl(old, "t8", "lane-a", "완료", "끝", "-", "*", "-", "")
	blk := gl(old, "t8", "lane-b", "막힘", "사람 손", "-", "*", "-", "")
	w := stubWorld(now, []string{news, blk}, three, reps)
	if v := w.judge(events.Cols(news)); !v.stale || senders(v, now, "ca", "cb", "cc") != "" {
		t.Errorf("⑥ 묵은 소식은 아무도 안 보냄: %+v", v)
	}
	v := w.judge(events.Cols(blk))
	if !v.stale || !v.open || senders(v, now, "ca", "cb", "cc") != "ca" {
		t.Errorf("⑥ 안 풀린 묵은 판단 필요 → 받는 쪽 ca만: %+v", v)
	}
	res := gl(old.Add(time.Minute), "t8", "lane-b", "중간", "막힘 풀림: 다시 돎", "-", "*", "-", "")
	if v := stubWorld(now, []string{blk, res}, three, reps).judge(events.Cols(blk)); v.br != brMute || senders(v, now, "ca", "cb") != "" {
		t.Errorf("⑥ 풀린 막힘은 안 보냄: %+v", v)
	}
	// 지휘 ca의 watch: 묵은 소식은 커서에 적고 watch.log에 한 줄, 막힘은 [시각]을 앞에 붙여 보냄
	k := newCwdKit(t)
	e := &ev{ed: k.d + "/st/hc-lane", cur: k.d + "/cur.ca", wto: "ca", win: []string{news, blk}}
	k.acoordAt("ca", 100, "")
	k.acoordAt("cb", 200, "")
	out := e.gate([]string{news, blk})
	if len(out) != 1 || out[0] != blk || !strings.HasPrefix(e.rpre[lineKey(blk)], "[") {
		t.Fatalf("⑥ ca가 보낼 것은 막힘 하나(시각 머리): %q %q", out, e.rpre)
	}
	cur, _ := os.ReadFile(e.cur)
	logb, _ := os.ReadFile(e.ed + "/watch.log")
	if !strings.Contains(string(cur), lineKey(news)) || !strings.Contains(string(logb), "안 보냄(묵음") {
		t.Errorf("⑥ 묵은 소식은 커서·기록 한 줄: 커서 %q 기록 %q", cur, logb)
	}
}

// ⑦ 옛 커서가 남은 주소에 watch가 새로 뜸(열흘 묵은 #since) → 묵은 남의 사건을 한꺼번에 보내지 않는다(①의 15시간 묵은 정리)
func TestGateOldCursorNewWatch(t *testing.T) {
	k := newCwdKit(t)
	k.acoordAt("ca", 100, repoA)
	k.write("st/hc-lane/lanes.tsv", "t5"+tab+"inbox-r"+tab+"-\n")
	now := time.Now()
	k.write("cur.ca", "#since"+tab+now.Add(-240*time.Hour).UTC().Format(time.RFC3339)+"\n")
	old := now.Add(-15 * time.Hour)
	ls := []string{gl(old, "t5", "inbox-r", "막힘", "막힘 넘김: clean 멈춤", "-", "*", "-", repoB),
		gl(old.Add(time.Minute), "t5", "inbox-r", "중간", "중간 막힘 풀림: clean 성공", "-", "*", "-", repoB),
		gl(old.Add(2*time.Minute), "t5", "inbox-r", "완료", "끝", "-", "*", "-", repoB),
		clean8(old.Add(30*time.Minute), "t5", "inbox-r")}
	e := &ev{ed: k.d + "/st/hc-lane", cur: k.d + "/cur.ca", wto: "ca", win: ls}
	nw := e.newEvents(nil, ls, "")
	if out := e.gate(nw); len(out) != 0 {
		t.Errorf("⑦ 15시간 묵은 남의 저장소 사건 → 보냄 0: %q", out)
	}
	if again := e.gate(e.newEvents(nil, ls, "")); len(again) != 0 {
		t.Errorf("⑦ 두 번째 바퀴도 0: %q", again)
	}
}

// ⑧ 기다렸다 판단: 문턱 전 풀림 → 0건, 넘김 → 한 번, 보류 중 watch 다시 뜸(새 판 exec — 메모리 없음) → 같은 판정 ⑨ 도구 이름 사건이 같은 이름의 다른 사건으로 풀리지 않음
func TestGateHold(t *testing.T) {
	now := time.Now()
	front := func(at time.Time, lane, sha string) string {
		return gl(at, "t4pn", "merger", "막힘", "병합기 맨 앞 "+lane+" 검증을 검증 기계에 맡기지 못해 기다린다", "-", "*", "hc-merger-front:"+sha, "")
	}
	f := front(now.Add(-3*time.Minute), "lane-f", "aaa")
	reps := map[string][]string{"": {"ca"}}
	if v := stubWorld(now, []string{f}, three, reps).judge(events.Cols(f)); v.br != brHold || senders(v, now, "ca", "cb") != "" {
		t.Fatalf("⑧ 문턱 전 → 보류: %+v", v)
	}
	landed := gl(now.Add(-time.Minute), "t4pn", "lane-f", "착지", "병합기 착지", "-", "*", "-", "")
	if v := stubWorld(now, []string{f, landed}, three, reps).judge(events.Cols(f)); v.br != brMute {
		t.Errorf("⑧ 3분 뒤 착지로 풀림 → 안 보냄: %+v", v)
	}
	f20 := front(now.Add(-20*time.Minute), "lane-f", "aaa")
	v1 := stubWorld(now, []string{f20}, three, reps).judge(events.Cols(f20))
	v2 := stubWorld(now, []string{f20}, three, reps).judge(events.Cols(f20)) // 새 판 exec — 보류 상태는 사건 시각에서 나오니 잃지 않음
	if v1.br != brNow || senders(v1, now, "ca", "cb") != "ca" || v1 != v2 {
		t.Errorf("⑧ 문턱 넘김 → 한 번·같은 판정: %+v %+v", v1, v2)
	}
	blk := gl(now.Add(-time.Hour), "t4pn", "merger", "막힘", "병합기 멈춤: 켬 표식이 다른 기계", "-", "*", "hf:x", "")
	other := gl(now.Add(-50*time.Minute), "t4pn", "merger", "중간", "직접 검증으로 맡김", "-", "*", "hc-merger-land-local:bbb", "")
	if v := stubWorld(now, []string{blk, other}, three, reps).judge(events.Cols(blk)); !v.open || v.br != brNow {
		t.Errorf("⑨ 병합기의 다른 사건이 병합기 막힘을 풀지 않는다: %+v", v)
	}
	same := gl(now.Add(-50*time.Minute), "t4pn", "merger", "중간", "다시 돎", "-", "*", "hf:x", "")
	if v := stubWorld(now, []string{blk, same}, three, reps).judge(events.Cols(blk)); v.open {
		t.Errorf("⑨ 같은 키의 막힘 아닌 뒤 사건은 풂: %+v", v)
	}
	bounce := gl(now.Add(-5*time.Minute), "t4pn", "lane-g", "막힘", "병합기: 착지 실패(종료 1, 충돌 a.go) — 다시 맞춤", "-", "*", "-", "")
	if v := stubWorld(now, []string{bounce}, three, reps).judge(events.Cols(bounce)); v.br != brHold {
		t.Errorf("⑧ 충돌 반송은 다시 제출을 기다림: %+v", v)
	}
	// 같은 초에 적힌 앞 줄(중간)은 뒤 막힘을 풀지 않는다 — 같은 기계 파일의 줄 차례로 가른다(tests/hclane s21b1 #16에서 찾음)
	mid := gl(now.Add(-time.Minute), "t6", "lane-k", "중간", "진행", "-", "*", "-", "")
	blk2 := gl(now.Add(-time.Minute), "t6", "lane-k", "막힘", "같은 초 막힘", "-", "*", "-", "")
	if v := stubWorld(now, []string{mid, blk2}, three, reps).judge(events.Cols(blk2)); !v.open || v.br != brNow {
		t.Errorf("같은 초 앞 줄 중간이 막힘을 풀었다: %+v", v)
	}
	if v := stubWorld(now, []string{blk2, mid}, three, reps).judge(events.Cols(blk2)); v.open {
		t.Errorf("같은 초 뒤 줄 중간은 막힘을 푼다: %+v", v)
	}
	resub := gl(now.Add(-3*time.Minute), "t6", "lane-g", "제출", "다시 제출", "-", "*", "submit:x", "")
	if v := stubWorld(now, []string{bounce, resub}, three, reps).judge(events.Cols(bounce)); v.br != brMute {
		t.Errorf("⑧ 다시 제출로 풀린 반송은 안 보냄: %+v", v)
	}
}

// ⑩ 받는 쪽 칸만 다른 두 줄 → 한 번 ⑪ 밀어 넣지 않음 갈래 → 화면 0·커서에 적힘
func TestGateDedupMute(t *testing.T) {
	k := newCwdKit(t)
	k.acoordAt("ca", 100, "")
	now := time.Now()
	a := strings.Join([]string{utcNow(), "m", "-", "on-land-deploy", "중간", "-", "착지 뒤 deploy 성공: 같음", "-", "ca", "-", "ca@1", "-", "-", "ops"}, tab)
	b := strings.Replace(a, "\tca\t-\tca@1", "\tcb\t-\tcb@1", 1)
	e := &ev{ed: k.d + "/st/hc-lane", cur: k.d + "/cur.ca", wto: "ca"}
	if nw := e.newEvents(nil, []string{a, b}, ""); len(nw) != 1 {
		t.Errorf("⑩ 받는 쪽 칸만 다른 두 줄 → 한 번: %q", nw)
	}
	e.win = []string{a}
	if out := e.gate([]string{a}); len(out) != 0 || e.vd[lineKey(a)].br != brMute {
		t.Errorf("⑪ 착지 뒤 명령 성공은 밀어 넣지 않음: %q %+v", out, e.vd[lineKey(a)])
	}
	if c, _ := os.ReadFile(e.cur); !strings.Contains(string(c), lineKey(a)) {
		t.Error("⑪ 밀어 넣지 않은 사건은 커서에(다시 판정 안 함)")
	}
	ref := gl(now, "-", events.OpsAdvise, "중간", "손댈 것 참고 — 주인 지휘에 보냄: chain:x", "-", "*", events.AdviseRefKey+"1", "")
	if v := classify(events.Cols(ref)); v.br != brMute {
		t.Errorf("⑪ 손댈 것 참고 사본은 밀어 넣지 않음: %v", v.name)
	}
}

// ⑬ 놓침 막기: 대표가 죽음 → 다음 한 명 바로, 대표가 30분 안 받음 → 다음 한 명 한 번(받았으면 보냄 기록에 있어 newEvents가 거른다)
func TestGateEscalate(t *testing.T) {
	now := time.Now()
	reps := map[string][]string{"": {"ca", "cb", "cc"}}
	l := gl(now.Add(-10*time.Minute), "t8", "lane-a", "막힘", "사람 손", "-", "*", "-", "")
	dead := map[string]tri.State{"ca": tri.No, "cb": tri.Yes, "cc": tri.Yes}
	if got := senders(stubWorld(now, []string{l}, dead, reps).judge(events.Cols(l)), now, "ca", "cb", "cc"); got != "cb" {
		t.Errorf("⑬ 대표 죽음 → 다음 cb: %q", got)
	}
	v := stubWorld(now, []string{l}, three, reps).judge(events.Cols(l))
	if got := senders(v, now, "ca", "cb", "cc"); got != "ca" {
		t.Errorf("⑬ 10분은 대표 ca만: %q", got)
	}
	later := now.Add(25 * time.Minute)
	if got := senders(v, later, "ca", "cb", "cc"); got != "ca,cb" {
		t.Errorf("⑬ 35분 안 받음 → 다음 cb도 한 번(ca는 받았으면 보냄 기록으로 둘 다 멈춤): %q", got)
	}
	if ok, tag := gateSend(v, "cb", later); !ok || !strings.Contains(tag, "놓침 막기(ca") {
		t.Errorf("⑬ 꼬리: %q", tag)
	}
	news := gl(now.Add(-40*time.Minute), "t8", "lane-a", "완료", "끝", "-", "*", "-", "")
	if got := senders(stubWorld(now, []string{news}, three, reps).judge(events.Cols(news)), now, "cb", "cc"); got != "" {
		t.Errorf("⑬ 소식은 놓침 막기 없음: %q", got)
	}
}

// 어느 지휘의 watch로 돌려도 같은 사건에 같은 판정 — 지휘 셋의 서로 다른 커서(하나는 그 사건을 이미 봄, 하나는 열흘 묵은 #since, 하나는 빈)
func TestGateSameAnswerAnyWatch(t *testing.T) {
	k := newCwdKit(t)
	k.acoordAt("ca", 300, repoA)
	k.acoordAt("cb", 100, repoA)
	k.acoordAt("cc", 200, repoB)
	now := time.Now()
	ls := []string{gl(now.Add(-time.Minute), "t8", "inbox-x", "막힘", "사람 손", "-", "*", "-", repoA),
		gl(now.Add(-time.Minute), "t9", "inbox-y", "완료", "끝", "-", "*", "-", repoB),
		gl(now.Add(-time.Minute), "-", "merger", "막힘", "병합기 멈춤", "-", "*", "-", "")}
	k.write("cur.ca", "#since"+tab+now.Add(-240*time.Hour).UTC().Format(time.RFC3339)+"\n")
	k.write("cur.cb", "#since"+tab+now.Add(-time.Hour).UTC().Format(time.RFC3339)+"\n"+lineKey(ls[1])+"\n")
	got := map[string][]string{}
	for _, c := range []string{"ca", "cb", "cc"} {
		e := &ev{ed: k.d + "/st/hc-lane", cur: k.d + "/cur." + c, wto: c, win: ls, lrepo: laneRepos(ls)}
		for _, l := range ls {
			if out := e.gate([]string{l}); len(out) == 1 {
				got[events.Cols(l).Lane] = append(got[events.Cols(l).Lane], c)
			}
		}
	}
	want := map[string]string{"inbox-x": "cb", "inbox-y": "cc", "merger": "cb"}
	for lane, w := range want {
		if strings.Join(got[lane], ",") != w {
			t.Errorf("%s: 보낸 지휘 %v, 기대 %s 한 명", lane, got[lane], w)
		}
	}
}

// ⑭ 묶음 머리는 사건 시각, 훨씬 이른 항목은 자기 시각, 대표라서 받은 항목은 꼬리, 자유 글은 뒤에서 자름
func TestGateDigestText(t *testing.T) {
	now := time.Now()
	e := &ev{rtag: map[string]string{}}
	a := gl(now.Add(-50*time.Minute), "t8", "lane-a", "완료", "끝", "-", "*", "-", "")
	b := gl(now.Add(-2*time.Minute), "t8", "lane-a", "착지", strings.Repeat("가", 150), "-", "*", "-", "")
	c := gl(now, "t9", "lane-b", "제출", "제출함", "-", "*", "-", "")
	e.rtag[lineKey(c)] = " → 대표 지휘 cb"
	got := e.digestText([]string{a, b, c})
	want := "hc-lane 묶음 — 3건(" + clock(now.Add(-50*time.Minute)) + "~" + clock(now) + ")"
	if !strings.HasPrefix(got, want) {
		t.Errorf("⑭ 머리: %q, 기대 %q로 시작", got, want)
	}
	if !strings.Contains(got, "lane-a: ["+clock(now.Add(-50*time.Minute))+"] 완료 끝 → 착지 ") || !strings.Contains(got, "…") {
		t.Errorf("⑭ 이른 항목 시각·자르기: %q", got)
	}
	if !strings.Contains(got, "lane-b: 제출 제출함 → 대표 지휘 cb") {
		t.Errorf("⑭ 항목 꼬리: %q", got)
	}
}

// 운영 장면 넷: ① 쌓인 상태(큰 커서·열흘 묵은 #since·옛 sent)에 처음 돌 때 — 옛 사건은 안 보내고 2시간 안의 안 풀린 막힘은 잃지 않음
// ② 두 번째 실행(같은 바퀴 두 번) → 한 번 ③ 바퀴 순서 반대(대표 watch가 죽음) → 다음 사람이 받음(놓치지 않음)
// ④ 외부 가정(8·12·13·14칸 섞임·깨진 줄·hc get 모름) → 판단 필요는 보내는 쪽
func TestGateOpsScenes(t *testing.T) {
	k := newCwdKit(t)
	k.acoordAt("ca", 100, "")
	k.acoordAt("cb", 200, "")
	now := time.Now()
	old := gl(now.Add(-9*time.Hour), "t8", "lane-o", "완료", "옛 완료", "-", "*", "-", "")
	blk := gl(now.Add(-20*time.Minute), "t8", "lane-b", "막힘", "진짜 막힘", "-", "*", "-", "")
	k.write("cur.ca", "#since"+tab+now.Add(-240*time.Hour).UTC().Format(time.RFC3339)+"\n"+lineKey(gl(now.Add(-30*time.Hour), "t1", "x", "완료", "a", "-", "*", "-", ""))+"\n")
	k.write("st/hc-lane/sent", lineKey(gl(now.Add(-40*time.Hour), "t1", "y", "완료", "b", "-", "*", "-", ""))+"\n")
	ls := []string{old, blk, strings.Join([]string{utcNow(), "m", "t9"}, tab), // 깨진 줄
		strings.Join([]string{utcNow(), "m", "t7", "lane-8", "중간", "-", "옛 8칸", "0"}, tab)}
	e := &ev{ec: "watch", ed: k.d + "/st/hc-lane", cur: k.d + "/cur.ca", wto: "ca", wsf: k.d + "/st/hc-lane/sent", win: ls}
	if rc := e.emit(e.newEvents(nil, ls, "")); rc != 0 {
		t.Fatalf("① 첫 바퀴 emit %d", rc)
	}
	sent, _ := os.ReadFile(e.wsf)
	if !strings.Contains(string(sent), lineKey(blk)) || strings.Contains(string(sent), lineKey(old)) {
		t.Errorf("① 막힘은 보내 공용 기록에, 옛 완료는 안 보냄: %q", sent)
	}
	if nw := e.newEvents(nil, ls, ""); len(nw) != 0 {
		t.Errorf("② 두 번째 바퀴 → 0: %q", nw)
	}
	eb := &ev{ec: "watch", ed: k.d + "/st/hc-lane", cur: k.d + "/cur.cb", wto: "cb", wsf: k.d + "/st/hc-lane/sent", win: ls}
	if nw := eb.newEvents(nil, ls, ""); len(eb.gate(nw)) != 0 {
		t.Errorf("② 다른 지휘 watch도 공용 기록으로 안 보냄: %q", nw)
	}
	// ③ 대표(ca)의 watch가 죽어 있음 → 다음 cb가 바로 받는다
	up := watchUp
	watchUp = func(_, a string) bool { return a != "ca" }
	defer func() { watchUp = up }()
	l3 := gl(now, "t8", "lane-c", "막힘", "대표 watch 없음", "-", "*", "-", "")
	e3 := &ev{ed: k.d + "/st/hc-lane", cur: k.d + "/cur.cb", wto: "cb", win: []string{l3}}
	if out := e3.gate([]string{l3}); len(out) != 1 {
		t.Errorf("③ 대표 watch가 죽음 → cb가 받음: %q", out)
	}
	// ④ 맨 앞 대표(손잡이 alert_coord ca)를 확인 못 함(hc get 깨짐) → 판단 필요는 모두가 보냄, 소식은 그 한 명(ca 몫 — cb는 안 보냄)
	watchUp = up
	if err := knob.Set(knob.Path(os.Getenv("HOME")), knob.Entry{Name: "alert_coord", Value: "ca", By: "시험", Why: "-", At: now.Unix()}, true, now); err != nil {
		t.Fatal(err)
	}
	k.write("get.ca", "깨진 응답")
	l4 := gl(now, "t8", "lane-d", "막힘", "모름", "-", "*", "-", "")
	n4 := gl(now, "t8", "lane-e", "완료", "모름", "-", "*", "-", "")
	e4 := &ev{ed: k.d + "/st/hc-lane", cur: k.d + "/cur.cb", wto: "cb", win: []string{l4, n4}}
	if out := e4.gate([]string{l4, n4}); len(out) != 1 || out[0] != l4 || !e4.vd[lineKey(l4)].all {
		t.Errorf("④ 확인 못 함 → 판단 필요만 cb도 보냄: %q %+v", out, e4.vd[lineKey(l4)])
	}
}

// ⑪ hc-lane events --explain: 사건마다 판정 한 줄 — 밀어 넣지 않은 것(착지 뒤 명령 성공·제출 뒤 닫힘)도 까닭과 함께 보인다
func TestGateExplain(t *testing.T) {
	k := newCwdKit(t)
	k.acoordAt("ca", 100, "")
	k.write("st/hc-lane/lanes.tsv", "t5"+tab+"lane-s"+tab+"-\n")
	now := time.Now()
	all := []string{gl(now.Add(-9*time.Minute), "t5", "lane-s", "제출", "제출함", "-", "*", "submit:x", ""),
		clean8(now.Add(-5*time.Minute), "t5", "lane-s"),
		gl(now.Add(-3*time.Minute), "-", "on-land-deploy", "중간", "착지 뒤 deploy 성공", "-", "*", "-", ""),
		gl(now, "t8", "lane-b", "막힘", "사람 손", "-", "*", "-", "")}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	so := os.Stdout
	os.Stdout = w
	e := &ev{ec: "events", ed: k.d + "/st/hc-lane"}
	e.explain(all)
	os.Stdout = so
	w.Close()
	b := make([]byte, 1<<16)
	n, _ := r.Read(b)
	got := string(b[:n])
	for _, want := range []string{"lane-s 정리됨 「hc-sweep R2」 — 밀어 넣지 않음 · 정리됨 · 끝 보고(제출) 뒤 도구가 닫음",
		"on-land-deploy 중간 「착지 뒤 deploy 성공」 — 밀어 넣지 않음 · 착지 뒤 명령 성공",
		"lane-b 막힘 「사람 손」 — 판단 필요 → ca(대표) · 레인 막힘"} {
		if !strings.Contains(got, want) {
			t.Errorf("--explain에 %q 없음:\n%s", want, got)
		}
	}
	if strings.Count(got, "\n") != len(all) {
		t.Errorf("사건마다 한 줄: %d줄\n%s", strings.Count(got, "\n"), got)
	}
}

// acoordAt: 살아 있는 지휘 a(뜬 시각 created, watch가 돎)가 저장소 r을 맡는다(빈 값이면 기록 없음 — 모든 저장소)
func (k *cwdKit) acoordAt(a string, created int, r string) {
	pid := strconv.Itoa(1000 + created)
	k.write("get."+a, `{"address":"`+a+`","cwd":"/","created_ms":`+strconv.Itoa(created)+`,"foreground":{"name":"node","pid":7},"name":"`+a+`","pid":`+pid+`,"state":"live"}`)
	k.write("st/hc-lane/coord."+a, a+tab+pid+tab+a+tab+"0\n")
	if r != "" {
		k.write("st/hc-lane/coordrepo."+a, pid+tab+r+"\n")
	}
}
