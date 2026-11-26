package main

import (
	"strings"
	"testing"
	"time"
)

// fill-load-steer: 기계 부하를 재서 한가한 기계부터 고른다. 순수 함수만 — 원격 호출·손잡이 저장은 다른 시험에서

func lsNames(ms []Machine) string {
	var o []string
	for _, m := range ms {
		o = append(o, m.Name)
	}
	return strings.Join(o, ",")
}

func lsMs(names ...string) []Machine {
	var o []Machine
	for _, n := range names {
		o = append(o, Machine{Name: n, Cap: "3", LCap: "-", All: "-"})
	}
	return o
}

// 원격 조각 출력 「load <1분 부하> <코어>」 → 읽음, 꼴이 다르면 모름
func TestLoadParse(t *testing.T) {
	for _, c := range []struct {
		in  string
		ok  bool
		per float64
	}{
		{"load 12.00 12\n", true, 1},
		{"load 1.2 8", true, 0.15},
		{"load  2.10\t12\n", true, 0.175},
		{"", false, 0},
		{"load x 8", false, 0},
		{"load 1.0 0", false, 0}, // 코어 0은 모름
		{"load 1.0", false, 0},
		{"something else", false, 0},
	} {
		l := loadParse(c.in)
		if l.OK != c.ok || c.ok && (l.Per()-c.per > 1e-9 || c.per-l.Per() > 1e-9) {
			t.Errorf("%q: %+v per=%v, want ok=%v per=%v", c.in, l, l.Per(), c.ok, c.per)
		}
	}
}

// ht-skew-verify-cause: 부하 조각에 실린 hc-lane merger --spec-idle 한 줄의 verifies= → 도는 검증 수. 그 줄이 없거나 꼴이 틀리면 모름
func TestLoadParseVerifies(t *testing.T) {
	for _, c := range []struct {
		in    string
		ok    bool
		ver   int
		verOK bool
	}{
		{"load 2.0 8\nload=2.00 cpus=8 verifies=1\n", true, 1, true},
		{"load=2.00 cpus=8 verifies=0\nload 2.0 8", true, 0, true},
		{"load 2.0 8\n", true, 0, false},
		{"load 2.0 8\nload=2.00 cpus=8 verifies=x\n", true, 0, false},
		{"load 2.0 8\nload=2.00 cpus=8\n", true, 0, false},
	} {
		l := loadParse(c.in)
		if l.OK != c.ok || l.Ver != c.ver || l.VerOK != c.verOK {
			t.Errorf("%q: %+v", c.in, l)
		}
	}
	if !strings.Contains(loadLine("a", mLoad{L1: 8, Cores: 8, OK: true, Ver: 1, VerOK: true}, 0, 3, -1, "", "", ""), "부하 8.0/8(코어당 1.00) · 검증 1") {
		t.Error("기계 줄에 검증 1 없음")
	}
	for _, l := range []mLoad{{L1: 8, Cores: 8, OK: true}, {L1: 8, Cores: 8, OK: true, VerOK: true}} {
		if s := loadLine("a", l, 0, 3, -1, "", "", ""); strings.Contains(s, "검증") {
			t.Errorf("모름·0인데 검증 칸: %s", s)
		}
	}
}

// ① 자리가 남는 두 기계 — 코어당 부하가 낮은 쪽이 앞(고치기 전: 설정 차례 그대로)
func TestLoadOrderLowFirst(t *testing.T) {
	ms := lsMs("a", "b")
	ld := map[string]mLoad{"a": {L1: 40, Cores: 12, OK: true}, "b": {L1: 1.2, Cores: 8, OK: true}}
	if got := lsNames(loadOrder(ms, ld, 0.1)); got != "b,a" {
		t.Fatalf("got %s, want b,a", got)
	}
}

// 부하가 비슷하면(띠 안) 설정 차례
func TestLoadOrderSimilarKeepsConf(t *testing.T) {
	ms := lsMs("a", "b")
	ld := map[string]mLoad{"a": {L1: 2.1, Cores: 12, OK: true}, "b": {L1: 1.2, Cores: 8, OK: true}} // 0.175 vs 0.15
	if got := lsNames(loadOrder(ms, ld, 0.1)); got != "a,b" {
		t.Fatalf("got %s, want a,b", got)
	}
}

// ② 부하 모름 → 설정 차례 자리 그대로(한가함으로 앞당기지도, 뒤로 밀지도 않는다). 아는 기계끼리만 자리를 바꾼다
func TestLoadOrderUnknownKeepsSlot(t *testing.T) {
	ms := lsMs("a", "u", "b")
	ld := map[string]mLoad{"a": {L1: 40, Cores: 12, OK: true}, "b": {L1: 1, Cores: 8, OK: true}} // u 모름
	if got := lsNames(loadOrder(ms, ld, 0.1)); got != "b,u,a" {
		t.Fatalf("got %s, want b,u,a", got)
	}
	if got := lsNames(loadOrder(ms, nil, 0.1)); got != "a,u,b" {
		t.Fatalf("모두 모름: got %s, want a,u,b", got)
	}
}

// ③ 후보가 모두 몹시 바쁨 → 이번 바퀴 건너뜀(까닭 있음), 오래 기다린 레인은 가장 덜 바쁜 곳 하나
func TestLoadGateAllBusy(t *testing.T) {
	ms := lsMs("a", "b")
	ld := map[string]mLoad{"a": {L1: 40, Cores: 12, OK: true}, "b": {L1: 30, Cores: 10, OK: true}} // 3.33, 3.0
	got, why := loadGate(ms, ld, 2.0, 5*time.Minute, 30*time.Minute)
	if len(got) != 0 || why == "" {
		t.Fatalf("짧게 기다림: got %s why %q, want 없음+까닭", lsNames(got), why)
	}
	got, why = loadGate(ms, ld, 2.0, 31*time.Minute, 30*time.Minute)
	if lsNames(got) != "b" || why == "" {
		t.Fatalf("오래 기다림: got %s why %q, want b+까닭", lsNames(got), why)
	}
	// 하나라도 바쁘지 않으면 그대로
	ld["b"] = mLoad{L1: 1, Cores: 10, OK: true}
	if got, why := loadGate(ms, ld, 2.0, 0, 30*time.Minute); lsNames(got) != "a,b" || why != "" {
		t.Fatalf("한 대 한가: got %s why %q", lsNames(got), why)
	}
	// 모름이 섞이면 「모두 바쁨」이 아니다
	delete(ld, "b")
	ld["a"] = mLoad{L1: 40, Cores: 12, OK: true}
	if got, why := loadGate(ms, ld, 2.0, 0, 30*time.Minute); lsNames(got) != "a,b" || why != "" {
		t.Fatalf("모름 섞임: got %s why %q", lsNames(got), why)
	}
	// 손잡이 0이면 문턱 끔
	ld["b"] = mLoad{L1: 30, Cores: 10, OK: true}
	if got, _ := loadGate(ms, ld, 0, 0, 30*time.Minute); lsNames(got) != "a,b" {
		t.Fatalf("문턱 끔: got %s", lsNames(got))
	}
}

// ⑥ 치우침 사건은 되풀이 간격 안에 다시 안 간다
func TestLoadSkewDue(t *testing.T) {
	now := time.Unix(10000, 0)
	if !skewDue(0, now, 60*time.Minute) {
		t.Fatal("처음은 보낸다")
	}
	if skewDue(now.Add(-59*time.Minute).Unix(), now, 60*time.Minute) {
		t.Fatal("간격 안은 안 보낸다")
	}
	if !skewDue(now.Add(-60*time.Minute).Unix(), now, 60*time.Minute) {
		t.Fatal("간격 지나면 보낸다")
	}
}

// ⑦ status 기계 줄 꼴: 이름 · 부하/코어(코어당) · 레인/자리 · 조정 · 안 띄운 까닭
func TestLoadStatusLine(t *testing.T) {
	got := loadLine("a", mLoad{L1: 40, Cores: 12, OK: true}, 2, 3, 0, " · 쉼 1", "자리 2(지휘, 30분)", "모두 바쁨")
	for _, w := range []string{"a", "40.0/12", "3.33", "2/3 · 빈 0 · 쉼 1", "자리 2(지휘, 30분)", "모두 바쁨"} {
		if !strings.Contains(got, w) {
			t.Errorf("%q 에 %q 없음", got, w)
		}
	}
	got = loadLine("u", mLoad{}, 0, 3, -1, "", "", "")
	if !strings.Contains(got, "모름") || strings.Contains(got, "0.00") || strings.Contains(got, "빈") {
		t.Errorf("모름 줄: %q", got)
	}
}
