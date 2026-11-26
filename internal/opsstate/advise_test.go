package opsstate

// ops-advise(2026-11-24): 지금 손댈 것 — 가짜 상태·가짜 대기 줄로 항목(무엇·근거·놓치면·다음 명령·확신)을 본다.
// 고치기 전 코드로는 Advise·Facts·Item이 없어 컴파일부터 실패한다(①~⑦·⑨ 모두).

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var ath = Thresholds{StageMin: 90, SkewHi: 2, SkewLo: 0.5, IdleMin: 10, WaitMin: 15}

// aBase: 기계 a 빈 자리 3(자리 5·에이전트 2, 레인 0/5), 앞 레인 하나가 일하는 중
func aBase() State {
	return State{Schema: 1, At: t0,
		Machines: []Machine{{Name: "a", Reach: "ok", Cores: ip(12), LoadPerCore: fp(0.3), Live: ip(2), Seats: ip(5), Lanes: ip(0), LaneCap: ip(5), Free: ip(3)}},
		Queue:    Queue{Counts: map[string]int{}},
		Lanes:    []Lane{{Name: "lane-front", Machine: "a", Stage: "작업 중(working)", StageAgeS: ip(40 * 60), Open: true}},
	}
}

func kinds(it []Item, k string) []Item {
	var o []Item
	for _, x := range it {
		if x.Kind == k {
			o = append(o, x)
		}
	}
	return o
}

// ① 빈 자리 3 + 대기 줄 둘이 문서 한 파일 겹침으로 대기 → 풀 수 있는 대기 둘, 확실, 다음 명령 줄이 그대로 칠 수 있는 꼴
func TestAdviseFreeWaitDoc(t *testing.T) {
	f := Facts{Waits: []Wait{
		{Lane: "doc-a", Project: "p", AgeS: ip(30 * 60), Front: "lane-front", Keys: []string{"docs/x.md"}},
		{Lane: "doc-b", Project: "p", AgeS: ip(30 * 60), Front: "lane-front", Keys: []string{"docs/x.md"}},
	}}
	it := kinds(Advise(aBase(), f, ath), "free-wait")
	if len(it) != 2 {
		t.Fatalf("풀 수 있는 대기 %d개: %+v", len(it), Advise(aBase(), f, ath))
	}
	x := it[0]
	if x.Sure != SureYes {
		t.Errorf("문서 한 파일 겹침인데 확신 %q", x.Sure)
	}
	want := "hc-fill edit doc-a --overlap-ok --why '겹침 풀기(hc-fill advise)'"
	if len(x.Next) != 1 || x.Next[0] != want {
		t.Errorf("다음 명령:\n got %q\nwant %q", x.Next, want)
	}
	for _, w := range []string{"빈 자리 3", "docs/x.md", "lane-front", "작업 중(working)", "40분"} {
		if !strings.Contains(x.Evidence, w) {
			t.Errorf("근거에 %q 없음: %s", w, x.Evidence)
		}
	}
	if x.Cost == "" || x.Key != "free-wait:doc-a" {
		t.Errorf("놓치면·열쇠: %+v", x)
	}
	// 기다린 지 idle분 안이면 아직 아니다
	f.Waits[0].AgeS, f.Waits[1].AgeS = ip(60), ip(60)
	if n := len(kinds(Advise(aBase(), f, ath), "free-wait")); n != 0 {
		t.Errorf("막 들어온 대기 줄인데 항목 %d", n)
	}
	// 빈 자리가 없으면 아니다
	s := aBase()
	s.Machines[0].Live, s.Machines[0].Free = ip(5), ip(0)
	f.Waits[0].AgeS = ip(30 * 60)
	if n := len(kinds(Advise(s, f, ath), "free-wait")); n != 0 {
		t.Errorf("빈 자리 없는데 항목 %d", n)
	}
}

// ② 꾸러미 전체 겹침 → 판단 필요, 놓치면에 기다림과 충돌 풀이 비용 둘 다
func TestAdviseFreeWaitPackage(t *testing.T) {
	f := Facts{Waits: []Wait{{Lane: "pkg-a", Project: "p", AgeS: ip(30 * 60), Front: "lane-front", Keys: []string{"cmd/x/"}}}}
	it := kinds(Advise(aBase(), f, ath), "free-wait")
	if len(it) != 1 || it[0].Sure != SureJudge {
		t.Fatalf("꾸러미 겹침: %+v", it)
	}
	if c := it[0].Cost; !strings.Contains(c, "기다림") || !strings.Contains(c, "충돌") {
		t.Errorf("놓치면에 기다림·충돌 비용: %s", c)
	}
	// 코드 한 파일도 판단 필요, 단독은 겹침을 꺼도 안 풀려 사람이 정한다(명령 없음)
	f.Waits[0].Keys = []string{"cmd/x/a.go"}
	if it := kinds(Advise(aBase(), f, ath), "free-wait"); len(it) != 1 || it[0].Sure != SureJudge {
		t.Errorf("코드 한 파일: %+v", it)
	}
	f.Waits[0].Keys, f.Waits[0].Solo = nil, true
	if it := kinds(Advise(aBase(), f, ath), "free-wait"); len(it) != 1 || it[0].Sure != SureHuman || len(it[0].Next) != 0 {
		t.Errorf("단독: %+v", it)
	}
}

// ③ 한 줄 사슬 넷 → 사슬 항목(길이·노는 자리·코어), 사슬 가운데 줄은 따로 항목을 내지 않는다
func TestAdviseChain(t *testing.T) {
	f := Facts{Waits: []Wait{
		{Lane: "c1", Project: "p", AgeS: ip(3600), After: []string{"lane-front"}},
		{Lane: "c2", Project: "p", AgeS: ip(3600), After: []string{"c1"}},
		{Lane: "c3", Project: "p", AgeS: ip(3600), After: []string{"c2"}},
		{Lane: "c4", Project: "p", AgeS: ip(3600), After: []string{"c3"}},
	}}
	it := Advise(aBase(), f, ath)
	ch := kinds(it, "chain")
	if len(ch) != 1 {
		t.Fatalf("사슬 항목 %d: %+v", len(ch), it)
	}
	for _, w := range []string{"사슬 4", "c1 → c2 → c3 → c4", "빈 자리 3", "코어 12"} {
		if !strings.Contains(ch[0].Evidence, w) {
			t.Errorf("사슬 근거에 %q 없음: %s", w, ch[0].Evidence)
		}
	}
	if ch[0].Sure != SureJudge || len(ch[0].Next) != 3 || ch[0].Next[0] != "hc-fill edit c2 --after - --why '사슬 풀기(hc-fill advise)'" {
		t.Errorf("사슬 다음 명령: %q", ch[0].Next)
	}
	for _, x := range kinds(it, "free-wait") {
		if x.Key != "free-wait:c1" {
			t.Errorf("사슬 가운데 줄에 따로 항목: %+v", x)
		}
	}
}

// ④ 터미널 error → 멈춘 레인, 들여다볼 줄과 이어 가게 할 줄
func TestAdviseLaneError(t *testing.T) {
	s := aBase()
	s.Lanes[0].Stage = "작업 중(error)"
	it := kinds(Advise(s, Facts{Addr: map[string]string{"lane-front": "w/t5"}}, ath), "lane-error")
	if len(it) != 1 {
		t.Fatalf("멈춘 레인: %+v", Advise(s, Facts{}, ath))
	}
	if strings.Join(it[0].Next, "\n") != "hc read w/t5\nhc-fill nudge lane-front --why '멈춘 레인(hc-fill advise)'" {
		t.Errorf("다음 명령: %q", it[0].Next)
	}
	// 주소를 모르면 그대로 칠 수 없으니 명령을 내지 않고 사람이 정한다
	it = kinds(Advise(s, Facts{}, ath), "lane-error")
	if len(it) != 1 || len(it[0].Next) != 0 || it[0].Sure != SureHuman {
		t.Errorf("주소 모름: %+v", it)
	}
}

// ⑤ 착지 뒤 미배포 → 밀린 배포(병합 줄이 비면 확실, 차 있으면 판단 필요)
func TestAdviseDeploy(t *testing.T) {
	s := aBase()
	s.Deploy = []Deploy{{Machine: "a", Version: "abc1234", Behind: ip(3)}}
	s.Sources = []Source{{Name: "deploy", AgeS: ip(25 * 60)}}
	it := kinds(Advise(s, Facts{}, ath), "deploy")
	if len(it) != 1 || it[0].Sure != SureYes || strings.Join(it[0].Next, "\n") != "herdr-sync --status a\nherdr-sync a" {
		t.Fatalf("밀린 배포: %+v", it)
	}
	if !strings.Contains(it[0].Evidence, "3커밋") || !strings.Contains(it[0].Evidence, "25분") {
		t.Errorf("배포 근거: %s", it[0].Evidence)
	}
	s.Merge = []MergeLine{{Project: "p", Length: ip(2), Merger: "alive"}}
	if it := kinds(Advise(s, Facts{}, ath), "deploy"); len(it) != 1 || it[0].Sure != SureJudge {
		t.Errorf("병합 줄이 찼을 때: %+v", it)
	}
}

// ⑥ 아무 일 없음 → 항목 없음, 사람용은 「손댈 것 없음」 한 줄
func TestAdviseNothing(t *testing.T) {
	if it := Advise(aBase(), Facts{}, ath); len(it) != 0 {
		t.Fatalf("조용한데 항목: %+v", it)
	}
	var b bytes.Buffer
	if err := AdviseHuman(&b, Advice{At: t0, Fingerprint: "f", Items: nil}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "손댈 것 없음") {
		t.Errorf("사람용: %s", b.String())
	}
	var j bytes.Buffer
	if err := AdviseJSON(&j, Advice{Schema: AdviseSchema, At: t0, Items: nil}); err != nil || !strings.Contains(j.String(), `"items":[]`) {
		t.Errorf("json 빈 목록: %v %s", err, j.String())
	}
}

// ⑦ 재료를 못 읽음 → 「판정 못 함」 항목(없음으로 내지 않는다). 병합기 모름·기계 연결 모름도
func TestAdviseUnknown(t *testing.T) {
	s := aBase()
	s.Sources = []Source{{Name: "hc", Why: "hc ls 못 읽음"}}
	s.Merge = []MergeLine{{Project: "p", Merger: "unknown"}}
	s.Machines = append(s.Machines, MachineUnknown("u", "연결 모름(hc machine ls 못 읽음)"))
	it := kinds(Advise(s, Facts{}, ath), "unknown")
	all := ""
	for _, x := range it {
		all += x.Summary() + "\n"
	}
	for _, w := range []string{"판정 못 함: 재료 hc 못 읽음", "판정 못 함: 병합기 p", "판정 못 함: 기계 u"} {
		if !strings.Contains(all, w) {
			t.Errorf("%q 없음:\n%s", w, all)
		}
	}
}

// ⑨ 눈에 띄는 것 = 지금 손댈 것의 요약(같은 판정을 두 군데 두지 않는다)
func TestNotableIsAdvise(t *testing.T) {
	s := fake()
	var sum []string
	for _, x := range Advise(s, Facts{}, th) {
		sum = append(sum, x.Summary())
	}
	if got := Notable(s, th); strings.Join(got, "\n") != strings.Join(sum, "\n") || len(got) == 0 {
		t.Errorf("눈에 띄는 것:\n%q\n손댈 것:\n%q", got, sum)
	}
}

// 다음 명령에 자리표시(<…>)가 없다 — 항목 종류마다
func TestAdviseNoPlaceholders(t *testing.T) {
	s := fake()
	s.Lanes = append(s.Lanes, Lane{Name: "aud", Machine: "a", Stage: "감사 대기(자리 없음 40분)", StageAgeS: ip(60)},
		Lane{Name: "blk", Machine: "a", Stage: "작업 중(blocked)"}, Lane{Name: "dn", Machine: "a", Stage: "작업 중(done)"})
	s.Merge = append(s.Merge, MergeLine{Project: "q", Length: ip(1), Merger: "gone"}, MergeLine{Project: "r", Length: ip(2), Merger: "alive", LastRoundAgeS: ip(3600)})
	s.Watch = []Watch{{Owner: "t2", Alive: bp(false), Need: true}}
	f := Facts{Addr: map[string]string{"aud": "a/t1", "blk": "a/t2", "dn": "a/t3", "lane-stuck": "a/t4"},
		Waits: []Wait{{Lane: "rj", Project: "p", AgeS: ip(60), Reject: "가지가 이미 있음"},
			{Lane: "only-a", Project: "p", AgeS: ip(60), Can: []string{"a"}}}} // 바쁜 a로만 갈 줄 — 치우침
	s.Machines[0].Live, s.Machines[0].Free = ip(2), ip(1)
	it := Advise(s, f, ath)
	seen := map[string]bool{}
	for _, x := range it {
		seen[x.Kind] = true
		for _, n := range x.Next {
			if strings.ContainsAny(n, "<>") {
				t.Errorf("%s 다음 명령에 자리표시: %s", x.Kind, n)
			}
		}
		if x.Sure != SureYes && x.Sure != SureJudge && x.Sure != SureHuman {
			t.Errorf("%s 확신 %q", x.Kind, x.Sure)
		}
	}
	for _, k := range []string{"audit-starve", "lane-blocked", "lane-done", "lane-long", "merger", "merge-stall", "watch", "rejected", "deploy", "skew", "machine"} {
		if !seen[k] {
			t.Errorf("항목 종류 %s 없음", k)
		}
	}
}

// json 칸 이름이 문서(docs/ops/advise.md 「json 칸」 표)와 맞다
func TestAdviseJSONKeysMatchDoc(t *testing.T) {
	var j bytes.Buffer
	it := Advise(fake(), Facts{}, th)
	if err := AdviseJSON(&j, Advice{Schema: AdviseSchema, At: t0, Fingerprint: "f", Items: it}); err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(j.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	var walk func(p string, v any)
	walk = func(p string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, c := range x {
				q := k
				if p != "" {
					q = p + "." + k
				}
				got[q] = true
				walk(q, c)
			}
		case []any:
			for _, c := range x {
				walk(p+"[]", c)
			}
		}
	}
	walk("", v)
	doc, err := os.ReadFile("../../docs/ops/advise.md")
	if err != nil {
		t.Fatal(err)
	}
	sec := string(doc)
	i := strings.Index(sec, "## json 칸")
	if i < 0 {
		t.Fatal("문서에 「## json 칸」 절이 없음")
	}
	want := map[string]bool{}
	for _, m := range regexp.MustCompile("(?m)^\\| `([a-z0-9_.\\[\\]]+)` \\|").FindAllStringSubmatch(sec[i:], -1) {
		want[m[1]] = true
	}
	var miss, extra []string
	for k := range got {
		if !want[k] {
			miss = append(miss, k)
		}
	}
	for k := range want {
		if !got[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(miss)
	sort.Strings(extra)
	if len(miss)+len(extra) > 0 {
		t.Errorf("문서에 없는 json 칸: %v\njson에 없는 문서 칸: %v", miss, extra)
	}
}
