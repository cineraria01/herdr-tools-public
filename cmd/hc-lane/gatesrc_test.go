package main

// ht-alert-sources(2026-11-28): 알림을 내는 쪽을 한 문에 맞춘다 — 운영 막힘의 풀림은 같은 이름·같은 열쇠(+ :up), 병합기 풀이 진행은
// 소식, 묵은 안 풀린 판단 필요는 origin 규칙 그대로 시각 붙여 보내되 나이 상한 하나(staleMax 24시간 — 지휘 결정: 커서·기계별 한정은 거둠)

import (
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/events"
)

// 표 1·2: 병합기 터미널 없음 → 다시 돎(같은 열쇠 + :up)이면 「풀림」, 다시 돎이 소식. 격리·닿지 않음·병합기 쉼·시작 단계·운영 에이전트 못 띄움도
func TestGateUpPairs(t *testing.T) {
	now := time.Now()
	reps := map[string][]string{"": {"ca"}}
	for _, c := range []struct{ lane, key string }{
		{"merger-proj", "hf:merger:proj:dead:1700"},
		{"merger-proj", "hf:merger:proj:unknown:1700"},
		{events.OpsCordon, "hf:cordon:box-1:1700"},
		{events.OpsUnreach, "hf:unreach:box-1:1700"},
		{events.OpsMerger, "merger:defer:1700:0000beef"},
		{events.OpsMerger, "hc-merger-start:클론:4242"},
		{events.OpsMerger, "hc-merger-stop:1700"},
		{events.OpsAgent, "hf:agent:launch-fail:1700"},
	} {
		blk := gl(now.Add(-3*time.Hour), "-", c.lane, "막힘", "막힘 "+c.key, "-", "*", c.key, "")
		up := gl(now.Add(-2*time.Hour), "-", c.lane, "중간", "다시 돎", "-", "*", c.key+upSuffix, "")
		w := stubWorld(now, []string{blk, up}, three, reps)
		if v := w.judge(events.Cols(blk)); v.br != brMute || v.open || !strings.HasPrefix(v.why, "풀림(중간") {
			t.Errorf("%s: 같은 열쇠 + :up 뒤 → 풀림: %+v", c.key, v)
		}
		if v := w.judge(events.Cols(up)); v.kind != "운영 풀림" || v.br != brNews {
			t.Errorf("%s: 풀림 사건은 소식: %+v", c.key, v)
		}
		// 반대 장면: 풀림이 없으면 그대로 안 풀림, 다른 열쇠의 :up은 풀지 않음
		other := gl(now.Add(-2*time.Hour), "-", c.lane, "중간", "다른 것 다시 돎", "-", "*", c.key+"9"+upSuffix, "")
		if v := stubWorld(now, []string{blk, other}, three, reps).judge(events.Cols(blk)); !v.open {
			t.Errorf("%s: 다른 열쇠의 :up은 풀지 않음: %+v", c.key, v)
		}
	}
}

// ① 풀림이 막힘보다 먼저 읽힘(기계가 달라 같은 초의 줄 차례가 뒤바뀜) → 잘못 풀지 않음 ② 풀림 뒤 재발(새 열쇠) → 새로 한 번
func TestGateUpOrder(t *testing.T) {
	now := time.Now()
	reps := map[string][]string{"": {"ca"}}
	at := now.Add(-5 * time.Minute)
	blk := gl(at, "-", "merger-proj", "막힘", "병합기 proj 돌지 않음", "-", "*", "hf:merger:proj:dead:1700", "")
	up := strings.Replace(gl(at, "-", "merger-proj", "중간", "다시 돎", "-", "*", "hf:merger:proj:dead:1700:up", ""), "\tm\t", "\tm2\t", 1)
	if v := stubWorld(now, []string{up, blk}, three, reps).judge(events.Cols(blk)); !v.open || v.br != brNow {
		t.Errorf("① 다른 기계의 같은 초 풀림이 앞줄 → 풀지 않음(판단 필요로 감): %+v", v)
	}
	again := gl(now, "-", "merger-proj", "막힘", "병합기 proj 돌지 않음", "-", "*", "hf:merger:proj:dead:1800", "")
	upOld := gl(now.Add(-time.Minute), "-", "merger-proj", "중간", "다시 돎", "-", "*", "hf:merger:proj:dead:1700:up", "")
	if v := stubWorld(now, []string{blk, upOld, again}, three, reps).judge(events.Cols(again)); !v.open || v.br != brNow {
		t.Errorf("② 풀림 뒤 재발(새 열쇠) → 새로 판단 필요: %+v", v)
	}
}

// 진행 메모는 소식(표 4) — 「충돌 — 병합 쪽에서 푸는 중」. 풀이 감사 통과는 그 열쇠 + :up(소식이자 사건 막힘의 풀림), 실패 낱말은 판단 필요
func TestGateResolveProgress(t *testing.T) {
	now := time.Now()
	reps := map[string][]string{"": {"ca"}}
	run := gl(now, "-", events.OpsMerger, "중간", "충돌 — 병합 쪽에서 푸는 중: 레인 l-a, 충돌 a.go", "-", "*", "hc-merger-resolve:abc", "")
	pass := gl(now, "-", events.OpsMerger, "중간", "충돌 — 병합 쪽 풀이 감사 통과(판정 x): 레인 l-a", "-", "*", "hc-merger-resolve:abc:up", "")
	bad := gl(now, "-", events.OpsMerger, "중간", "충돌 — 병합 쪽 풀이 못 함", "-", "*", "hc-merger-resolve:abc", "")
	w := stubWorld(now, []string{run, pass, bad}, three, reps)
	if v := w.judge(events.Cols(run)); v.br != brNews || v.kind != "병합기 풀이 진행" {
		t.Errorf("푸는 중 → 소식: %+v", v)
	}
	if v := w.judge(events.Cols(pass)); v.br != brNews || v.kind != "운영 풀림" {
		t.Errorf("풀이 감사 통과 → 소식: %+v", v)
	}
	if v := w.judge(events.Cols(bad)); v.br != brNow {
		t.Errorf("풀이 실패 낱말 → 판단 필요: %+v", v)
	}
	ask := gl(now.Add(-time.Minute), "-", events.OpsMerger, "막힘", "충돌 — 풀 수 있음: …", "-", "*", "hc-merger-resolve:abc", "")
	if v := stubWorld(now, []string{ask, pass}, three, reps).judge(events.Cols(ask)); v.br != brMute {
		t.Errorf("사건 방식 풀이 막힘은 통과(:up)로 풀림: %+v", v)
	}
	// 옛 판이 낸 열쇠(hc-merger-resolved:<R>)는 운영 알림 그대로(판정이 지금과 같다 — 운영 장면 ③)
	oldPass := gl(now, "-", events.OpsMerger, "중간", "충돌 — 병합 쪽 풀이 감사 통과", "-", "*", "hc-merger-resolved:def", "")
	if v := w.judge(events.Cols(oldPass)); v.br != brNow || v.kind != "운영 알림" {
		t.Errorf("옛 열쇠는 지금 판정 그대로: %+v", v)
	}
}

// 표 3(지휘 결정 — 묵은 예외는 나이 상한 하나): 끊겼다 붙은 기계의 2.5시간 전 막힘 → 시각 붙여 한 번(넘겨줌 전후 모두 — 커서가 방금
// 움직였어도, 새 판으로 넘어간 watch여도). 25시간 전 → 안 감·기록 한 줄. 새 커서 + 7일 전 안 풀린 막힘 → 안 감
func TestGateStaleCap(t *testing.T) {
	k := newCwdKit(t)
	k.acoordAt("ca", 100, "")
	now := time.Now()
	late := strings.Replace(gl(now.Add(-150*time.Minute), "m2/t8", "lane-r", "막힘", "끊긴 동안 막힘", "-", "*", "-", ""), "\tm\t", "\tm2\t", 1)
	day := gl(now.Add(-25*time.Hour), "t8", "lane-d", "막힘", "하루 넘은 막힘", "-", "*", "-", "")
	other := gl(now.Add(-time.Minute), "t9", "lane-o", "완료", "다른 기계 소식", "-", "*", "-", "")
	for name, curLines := range map[string]string{
		"넘겨줌 전(커서가 1분 전에 움직임)": lineKey(other) + "\n",
		"넘겨줌 뒤(새 판, 커서 그대로)":   "#since" + tab + now.Add(-time.Hour).UTC().Format(time.RFC3339) + "\n" + lineKey(other) + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			k.write("cur.ca", curLines)
			e := &ev{ec: "watch", ed: k.d + "/st/hc-lane", cur: k.d + "/cur.ca", wto: "ca", win: []string{late, day, other}}
			out := e.gate([]string{late, day})
			if len(out) != 1 || out[0] != late || !strings.HasPrefix(e.rpre[lineKey(late)], "[") {
				t.Fatalf("늦게 들어온 2.5시간 전 막힘만 시각 붙여 한 번: %q", out)
			}
			cur, _ := os.ReadFile(e.cur)
			if !strings.Contains(string(cur), lineKey(day)) {
				t.Error("상한 넘은 막힘은 커서에(다음 바퀴에 다시 판정 안 함)")
			}
			e.appendCur(lineKey(late)) // 보낸 줄은 emit이 커서에 적는다
			if again := e.newEvents(nil, []string{late, day}, ""); len(again) != 0 {
				t.Errorf("두 번째 바퀴는 문에 안 들어옴: %q", again)
			}
		})
	}
	logb, _ := os.ReadFile(k.d + "/st/hc-lane/watch.log")
	if !strings.Contains(string(logb), "상한 1440분 넘음") || !strings.Contains(string(logb), "하루 넘은 막힘") {
		t.Errorf("상한 넘은 것은 기록 한 줄: %q", logb)
	}
	// 새 커서(옛 커서가 없는 주소에 처음 뜬 watch) + 7일 전 안 풀린 막힘 → 안 감
	week := gl(now.Add(-7*24*time.Hour), "t8", "lane-w", "막힘", "일주일 묵은 막힘", "-", "*", "-", "")
	e := &ev{ec: "watch", ed: k.d + "/st/hc-lane", cur: k.d + "/new.ca", wto: "ca", win: []string{week}}
	if out := e.gate([]string{week}); len(out) != 0 {
		t.Errorf("새 커서 + 7일 전 막힘 → 안 감: %q", out)
	}
}

// 감사 atp7ncn 낮음 1: 표 2의 레인 쪽 종류 — 멈춘 레인·완료 뒤 멈춤·첫 턴 없음은 그 레인의 뒤 보고로 풀림, 없으면 열림
func TestGateLaneKindPairs(t *testing.T) {
	now := time.Now()
	reps := map[string][]string{"": {"ca"}}
	for _, msg := range []string{"l-h 멈춘 레인 — 「생각 중」 표시 25분째", "l-h 완료 뒤 멈춤 — 70분째 감사·착지 없음", "첫 턴 없음 t8"} {
		blk := gl(now.Add(-time.Hour), "-", "l-h", "막힘", msg, "-", "*", "hf:l-h:x", "")
		for _, y := range []struct{ st, m string }{{"중간", "다시 이어 감"}, {"완료", "끝"}, {"착지", "넣음"}} {
			nx := gl(now.Add(-time.Minute), "t8", "l-h", y.st, y.m, "-", "*", "-", "")
			if v := stubWorld(now, []string{blk, nx}, three, reps).judge(events.Cols(blk)); v.open || v.br != brMute {
				t.Errorf("%q 뒤 %s → 풀림: %+v", msg, y.st, v)
			}
		}
		if v := stubWorld(now, []string{blk}, three, reps).judge(events.Cols(blk)); !v.open || v.br != brNow {
			t.Errorf("%q 뒤 보고 없음 → 열림: %+v", msg, v)
		}
	}
}

// 표 8: hc-sweep의 새 꼴 정리됨(같은 실행의 주인·저장소·실행 열쇠를 실은 13칸)도 8칸과 같은 판정 — 끝 보고 뒤면 조용, 보고 없이 사라졌으면
// 그 줄의 주인에게(물려받기 없이). 옛 판 워커의 8칸이 섞여도 같다
func TestGateClean13(t *testing.T) {
	k := newCwdKit(t)
	k.write("st/hc-lane/lanes.tsv", "t5"+tab+"lane-c"+tab+"-\n"+"t6"+tab+"lane-d"+tab+"-\n")
	k.acoordAt("ca", 100, "")
	k.acoordAt("cb", 200, "")
	now := time.Now()
	c13 := func(at time.Time, term, lane string) string {
		return strings.Join([]string{at.UTC().Format(time.RFC3339), "m", term, lane, "정리됨", "-", "hc-sweep R2", "0", "-", term + "@1", "cb@200", "-", repoA}, tab)
	}
	done := []string{gl(now.Add(-5*time.Minute), "t5", "lane-c", "완료", "끝", "-", "cb@200", "-", repoA), c13(now, "t5", "lane-c")}
	e := &ev{ed: k.d + "/st/hc-lane", cur: k.d + "/cur.cb", wto: "cb", win: done}
	for _, o := range e.newEvents(nil, done, "") {
		if events.Cols(o).State == "멈춤" {
			t.Errorf("끝 보고 뒤 13칸 정리됨은 조용: %q", o)
		}
	}
	gone := []string{gl(now.Add(-5*time.Minute), "t6", "lane-d", "중간", "작업 중", "-", "cb@200", "-", repoA), c13(now, "t6", "lane-d"),
		done[0], clean8(now, "t5", "lane-c")} // 옛 판 워커의 8칸(끝 보고 뒤) — 조용
	e2 := &ev{ed: k.d + "/st/hc-lane", cur: k.d + "/cur2.cb", wto: "cb", win: gone}
	var stop []string
	for _, o := range e2.newEvents(nil, gone, "") {
		if events.Cols(o).State == "멈춤" {
			stop = append(stop, o)
		}
	}
	if len(stop) != 1 {
		t.Fatalf("보고 없이 사라진 13칸 정리됨 → 멈춤 하나: %q", stop)
	}
	e2.win = gone
	if out := e2.gate(stop); len(out) != 1 || e2.vd[lineKey(stop[0])].owner != "cb" {
		t.Errorf("그 줄의 주인 cb에게: %q %+v", out, e2.vd[lineKey(stop[0])])
	}
}

// 운영 장면 ①: 판이 올라가는 순간 이미 쌓인 상태 — 짝 없는 옛 막힘(옛 꼴 열쇠, 주인 꼬리)과 옛 사본이 섞인 창에 처음 돌 때, 옛 막힘이
// 「풀림」 소식으로 쏟아지지 않고(풀림 사건이 없으니 소식도 없다), 옛 판이 이미 판정한 것(커서에 있음)은 다시 문에 들어오지 않는다
func TestGateUpgradeFirstRun(t *testing.T) {
	k := newCwdKit(t)
	k.acoordAt("ca", 100, "")
	now := time.Now()
	var ls, keys []string
	for i := 0; i < 20; i++ {
		at := now.Add(-time.Duration(3+i) * time.Hour)
		ls = append(ls, gl(at, "-", "merger-proj", "막힘", "병합기 proj 돌지 않음", "-", "*", "hf:merger:proj:dead:17"+string(rune('a'+i))+":t1", ""),
			gl(at.Add(time.Minute), "-", "merger-proj", "중간", "병합기 proj 다시 돎", "-", "*", "hf:merger:proj:up:18"+string(rune('a'+i))+":t1", ""),
			gl(at, "-", events.OpsAdvise, "중간", "손댈 것 참고 — 주인 지휘에 보냄: x", "-", "*", events.AdviseRefKey+"1", ""))
	}
	for _, l := range ls {
		keys = append(keys, lineKey(l))
	}
	k.write("cur.ca", strings.Join(keys, "\n")+"\n") // 옛 판이 그때 판정해 커서에 적음
	e := &ev{ec: "watch", ed: k.d + "/st/hc-lane", cur: k.d + "/cur.ca", wto: "ca", win: ls}
	if nw := e.newEvents(nil, ls, ""); len(e.gate(nw)) != 0 {
		t.Errorf("이미 쌓인 옛 막힘·옛 다시 돎·사본 → 보냄 0: %q", nw)
	}
	if again := e.gate(e.newEvents(nil, ls, "")); len(again) != 0 {
		t.Errorf("두 번째 바퀴도 0: %q", again)
	}
}
