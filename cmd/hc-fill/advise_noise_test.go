package main

// ht-advise-noise(2026-11-28): 끊긴 기계의 끝난 옛 줄 175건이 레인마다 lane-long으로, --then 이어서 단계를 선행으로 가진 줄이 「풀 수 있음」으로
// 나왔다. 가짜 hc·가짜 대기열(advFx)로 모으기부터 본다 — 이미 쌓인 대기열(옛 16칸 줄·끝난 옛 줄)에 처음 돌 때·두 번째 실행·연결 모름·
// Windows 꼴 cwd가 섞인 hc ls·주인 없는 줄. 고치기 전 코드로는 lane-long이 옛 줄마다 나오고 이어서 단계에 free-wait가 나와 실패한다

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/opsstate"
)

// noiseFx: advFx + 기계 x에 끝난 옛 줄 n개(착지·정리됨 — 반은 줄 ID 없는 옛 16칸 줄, 지난 값은 끊긴 기계라 old) + 살아 있는 줄 live-x(주인 없음)
func noiseFx(t *testing.T, n int, mls string) {
	t.Helper()
	advFx(t)
	if err := os.WriteFile(DATA+"/mls", []byte(mls), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	add := now.Add(-7 * 24 * time.Hour).Format("2006-01-02T15:04")
	q, _ := os.ReadFile(DATA + "/queue.tsv")
	l, _ := os.ReadFile(DATA + "/last.tsv")
	var qb, lb strings.Builder
	qb.Write(q)
	lb.Write(l)
	for i := range n {
		name, rid, ev := fmt.Sprintf("old-%d", i), fmt.Sprintf("ro%d", i), "정리됨"
		if i%2 == 0 {
			ev = "착지"
		}
		c := []string{name, "app", "b", "-", "-", "up", "x", fmt.Sprintf("x/t%d", 100+i), "-", add, "-", "m/t1", "normal", "-", "-", "-", rid}
		if i%3 == 0 { // 옛 16칸 줄(줄 ID 없음)
			c, rid = c[:16], ""
		}
		qb.WriteString(strings.Join(c, "\t") + "\n")
		lb.WriteString(strings.Join([]string{name, ev, "abcd1234", now.Add(-3 * time.Hour).Format("15:04"), "끝", "-", "old", rid}, "\t") + "\n")
	}
	qb.WriteString(strings.Join([]string{"live-x", "app", "b", "-", "-", "up", "x", "x/t9", "-", add, "-", "-", "normal", "-", "-", "-", "rlx"}, "\t") + "\n")
	lb.WriteString(strings.Join([]string{"live-x", "중간", "-", now.Add(-4 * time.Hour).Format("15:04"), "반쯤", "-", "old", "rlx"}, "\t") + "\n")
	os.WriteFile(DATA+"/queue.tsv", []byte(qb.String()), 0o644)
	os.WriteFile(DATA+"/last.tsv", []byte(lb.String()), 0o644)
	// Windows 꼴 cwd가 섞인 hc ls(다른 기계 터미널)
	os.WriteFile(DATA+"/lsjson", []byte(`{"terminals":[{"address":"w/t5","name":"lane-a","status":"working"},`+
		`{"address":"win/t1","name":"win-job","status":"idle","cwd":"C:\\Users\\u\\repo"},`+
		`{"address":"t2","name":"`+watchName("m/t1")+`","status":"idle"}]}`), 0o644)
}

const downMLS = `{"machines":[{"name":"w","state":"connected"},{"name":"x","state":"disconnected"}]}`

func noiseKeys(a opsstate.Advice) []string {
	var k []string
	for _, it := range a.Items {
		k = append(k, it.Key)
	}
	sort.Strings(k)
	return k
}

// ① 끊긴 기계 + 끝난 옛 줄 여럿 + 살아 있는 줄 하나 → 기계 항목 1개에 그 레인 이름, lane-long 0 · 두 번째 실행도 같은 항목·같은 지문
// ⑧ state와 advise의 지문이 같은 재료로 함께 — 기계가 돌아오면 둘 다 같이 바뀐다 ④ 돌아오면 레인별 판정
func TestAdviseNoiseDownMachine(t *testing.T) {
	noiseFx(t, 40, downMLS)
	now := time.Now()
	a := adviseNow(now)
	var mach *opsstate.Item
	long := 0
	for i, it := range a.Items {
		if it.Kind == "lane-long" && it.Key != "lane-long:lane-a" { // lane-a: 닿는 기계 w에서 2시간째 일하는 진짜 멈춤 — 지금처럼 나온다(②)
			t.Errorf("lane-long이 나옴: %s", it.Summary())
		}
		if it.Kind == "lane-long" {
			long++
		}
		if it.Key == "machine:x" {
			mach = &a.Items[i]
		}
	}
	if long != 1 {
		t.Errorf("닿는 기계의 진짜 멈춘 lane-a가 lane-long으로 나와야: %v", noiseKeys(a))
	}
	if mach == nil || mach.What != "기계 x 모름: 연결 안 됨 — 살아 있는 레인 1개(live-x)" {
		t.Fatalf("기계 항목: %+v\n%v", mach, noiseKeys(a))
	}
	b := adviseNow(now)
	if fmt.Sprint(noiseKeys(a)) != fmt.Sprint(noiseKeys(b)) || a.Fingerprint != b.Fingerprint {
		t.Errorf("두 번째 실행이 다름: %v %s / %v %s", noiseKeys(a), a.Fingerprint, noiseKeys(b), b.Fingerprint)
	}
	stateFP := func() string {
		s, f := stateCollectF(now)
		tr := opsstate.Trim(s, knobInt("state_done"))
		tr.Notable = stateNotable(tr, f, stateThresholds())
		return opsstate.Fingerprint(tr)
	}
	if fp := stateFP(); fp != a.Fingerprint {
		t.Errorf("state 지문 %s ≠ advise 지문 %s", fp, a.Fingerprint)
	}
	// 기계가 돌아옴: 옛 줄의 터미널은 hc ls에 없어 끝남, live-x도 목록에 없어 끝남 → 레인별 판정(lane-long 0, 기계 항목 없음)
	os.WriteFile(DATA+"/mls", []byte(`{"machines":[{"name":"w","state":"connected"},{"name":"x","state":"connected"}]}`), 0o644)
	c := adviseNow(now)
	for _, k := range noiseKeys(c) {
		if strings.HasPrefix(k, "machine:") || strings.HasPrefix(k, "lane-long:") && k != "lane-long:lane-a" {
			t.Errorf("돌아온 뒤 남은 항목 %s", k)
		}
	}
	if c.Fingerprint == a.Fingerprint || stateFP() != c.Fingerprint {
		t.Errorf("돌아온 뒤 지문: advise %s(전 %s) state %s", c.Fingerprint, a.Fingerprint, stateFP())
	}
	// 돌아왔고 live-x 터미널이 살아 있으면 그 레인은 다시 lane-long(같은 단계 4시간)
	os.WriteFile(DATA+"/lsjson", []byte(`{"terminals":[{"address":"w/t5","name":"lane-a","status":"working"},{"address":"x/t9","name":"live-x","status":"working"}]}`), 0o644)
	if !strings.Contains(fmt.Sprint(noiseKeys(adviseNow(now))), "lane-long:live-x") {
		t.Errorf("돌아온 기계의 살아 있는 줄이 lane-long으로 안 나옴: %v", noiseKeys(adviseNow(now)))
	}
}

// 바퀴 순서 반대: 기계 연결을 모름(hc machine ls 못 읽음) → 끊김으로 단정하지 않고 판정 못 함, 옛 줄 lane-long 0
func TestAdviseNoiseConnUnknown(t *testing.T) {
	noiseFx(t, 10, "")
	os.WriteFile(DATA+"/mls", []byte("not json"), 0o644)
	for _, it := range adviseNow(time.Now()).Items {
		if it.Kind == "lane-long" || it.Kind == "machine" {
			t.Errorf("연결 모름인데 %s", it.Summary())
		}
	}
}

// ⑤ --then 이어 붙인 단계를 --after로 가진 줄 → 풀 수 있음 아님 · ⑥ 진짜로 대기열에 없는 선행은 지금처럼 권고 · ⑦ json 칸·키 꼴 그대로
func TestAdviseNoiseThenAfter(t *testing.T) {
	advFx(t)
	brief := DATA + "/lane-a.md"
	os.WriteFile(brief, []byte("# lane-a\n\n## 이어서: lane-a-2 (hc-fill --then)\n다음 단계\n"), 0o644)
	add := time.Now().Add(-3 * time.Hour).Format("2006-01-02T15:04")
	q, _ := os.ReadFile(DATA + "/queue.tsv")
	q = bytes.Replace(q, []byte("lane-a\tapp\tb\t"), []byte("lane-a\tapp\t"+brief+"\t"), 1)
	row := func(c ...string) string { return strings.Join(c, "\t") + "\n" }
	os.WriteFile(DATA+"/queue.tsv", append(q, []byte(
		row("then-w", "app", "b", "lane-a-2", "-", "wait", "-", "-", "-", add, "-", "m/t1", "normal", "-", "-", "-", "r21")+
			row("gone-w", "app", "b", "lane-zz", "-", "wait", "-", "-", "-", add, "-", "m/t1", "normal", "-", "-", "-", "r22"))...), 0o644)
	_, f := stateCollectF(time.Now())
	if f.Then["lane-a-2"] != "lane-a" {
		t.Fatalf("이어 붙인 이름 대응: %v", f.Then)
	}
	a := adviseNow(time.Now())
	ks := fmt.Sprint(noiseKeys(a))
	if strings.Contains(ks, "free-wait:then-w") || !strings.Contains(ks, "free-wait:gone-w") {
		t.Errorf("이어서 단계는 권하지 않고 대기열에 없는 선행은 권고: %s", ks)
	}
	var buf bytes.Buffer
	if err := opsstate.AdviseJSON(&buf, a); err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	var items []map[string]json.RawMessage
	json.Unmarshal(buf.Bytes(), &top)
	json.Unmarshal(top["items"], &items)
	tk := []string{}
	for k := range top {
		tk = append(tk, k)
	}
	sort.Strings(tk)
	if fmt.Sprint(tk) != "[at fingerprint items schema]" || len(items) == 0 {
		t.Fatalf("json 맨 위 칸: %v", tk)
	}
	ik := []string{}
	for k := range items[0] {
		ik = append(ik, k)
	}
	sort.Strings(ik)
	if fmt.Sprint(ik) != "[claim cost evidence key kind next sure what]" {
		t.Errorf("json 항목 칸: %v", ik)
	}
}
