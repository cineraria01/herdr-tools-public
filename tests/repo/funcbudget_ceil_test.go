package repo_test

import (
	"strings"
	"testing"
)

// 예산 값은 천장(2026-11-24 func-budget-headroom): 나란히 도는 레인이 같은 긴 함수에 몇 줄씩 더해도 얹은 뒤 통과하고,
// 줄인 함수는 천장을 내리게 하며, 새 긴 함수는 그대로 실패한다
func TestFuncBudgetCeil(t *testing.T) {
	has := func(v []string, s string) bool {
		for _, l := range v {
			if strings.Contains(l, s) {
				return true
			}
		}
		return false
	}
	for _, c := range [][2]int{{201, 225}, {207, 225}, {214, 225}, {215, 225}, {216, 250}, {241, 275}, {250, 275}, {255, 275}, {265, 275}, {292, 325}, {328, 350}} {
		if got := funcCeil(c[0]); got != c[1] {
			t.Errorf("funcCeil(%d) = %d, 바라는 값 %d", c[0], got, c[1])
		}
	}
	for n := funcLimit + 1; n < 1000; n++ { // 천장의 여유는 늘 funcCeilMin 이상 funcSlackMax 미만(적은 값이 곧바로 「내려라」에 걸리지 않게)
		if s := funcCeil(n) - n; s < funcCeilMin || s >= funcSlackMax {
			t.Fatalf("funcCeil(%d) 여유 %d", n, s)
		}
	}
	k := "a.go big"
	b := map[string]int{k: funcCeil(207)} // 레인 둘이 갈라질 때의 목록: 207줄 → 천장 225
	// ① 두 레인이 각각 5줄씩 더해 얹은 뒤 217줄 — 천장 안이라 통과(옛 판: 목록이 207이라 실패)
	if v := checkFuncBudget(map[string]int{k: 217}, b, nil, false); len(v) != 0 {
		t.Errorf("① 천장 안의 늘어남은 통과여야: %v", v)
	}
	// ② 천장을 넘김 — 실패, 글에 함수·지금 줄 수·천장·고치는 법
	v := checkFuncBudget(map[string]int{k: 226}, b, nil, false)
	if !has(v, "함수가 천장을 넘었다 a.go big 226줄 (천장 225)") || !has(v, "함수를 쪼개라") {
		t.Errorf("② 천장을 넘으면 실패여야: %v", v)
	}
	// ③ 50줄 넘게 줄었는데 천장 그대로 — 실패 「내려라」(값은 funcCeil)
	if v := checkFuncBudget(map[string]int{k: 260}, map[string]int{k: 325}, nil, false); !has(v, "a.go big 260줄 (천장 325) — 천장을 내려라: 275") {
		t.Errorf("③ 많이 줄었는데 천장 그대로면 실패여야: %v", v)
	}
	if v := checkFuncBudget(map[string]int{k: 276}, map[string]int{k: 325}, nil, false); len(v) != 0 {
		t.Errorf("③ 여유 49줄은 통과여야: %v", v)
	}
	// 한도 아래로 내려간 함수의 줄은 지운다
	if v := checkFuncBudget(map[string]int{k: funcLimit}, b, nil, false); !has(v, "함수가 한도 아래로 줄었다 a.go big 200줄") {
		t.Errorf("한도 아래로 줄면 예산 줄을 지우게 해야: %v", v)
	}
	// ④ 새 함수가 200줄 넘김 — 예산 없이 실패
	if v := checkFuncBudget(map[string]int{"a.go new": funcLimit + 1}, nil, nil, false); !has(v, "새 긴 함수 a.go new 201줄 (한도 200)") || !has(v, "함수를 쪼개라") {
		t.Errorf("④ 새 긴 함수는 실패여야: %v", v)
	}
	// 꼴 바꿈(갈래점 옛 꼴 → 이 판 천장 꼴)에서만 레인의 올림을 견주지 않는다
	old, cur := "# 「<줄 수> <경로> <함수>」\n207 a.go big\n", "# 「<천장> <경로> <함수>」\n225 a.go big\n"
	if !ceilMigration(old, cur) || ceilMigration(cur, cur) || ceilMigration(old, old) || ceilMigration(cur, old) {
		t.Error("ceilMigration은 갈래점 옛 꼴·이 판 천장 꼴일 때만 참이어야")
	}
	// 레인은 천장을 올리지 못한다(긴 함수가 한없이 자라지 않게)
	if v := checkFuncBudget(map[string]int{k: 230}, map[string]int{k: 250}, b, true); !has(v, "레인이 함수 예산을 늘렸다 a.go big 250 (갈래점 225)") {
		t.Errorf("레인의 천장 올림은 실패여야: %v", v)
	}
}
