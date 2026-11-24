package main

import "testing"

// repoAct: 맥 ↔ 저장소 한 줄 판정(읽기·쓰기 없음) — 계약 시험(가짜 맥·저장소)보다 훨씬 빠른 본보기 단위 시험
func TestRepoAct(t *testing.T) {
	const old = "저장소에 없음 — 옛 base라 지운 것인지 모른다"
	for _, c := range []struct {
		name                      string
		m, r, b                   string
		oldBase, take, pull, sect bool
		act, why                  string
	}{
		{"저장소가 base 그대로 — 맥이 새로움", "m2", "b", "b", false, false, false, false, "모음", ""},
		{"맥이 base 그대로 — 저장소가 새로움", "b", "r2", "b", false, false, false, false, "깖", ""},
		{"둘 다 바뀜 — 충돌", "m2", "r2", "b", false, false, false, false, "", ""},
		{"--take repo:가 둘 다 바뀜을 모음으로", "m2", "r2", "b", false, true, false, false, "모음", ""},
		{"--pull repo:가 둘 다 바뀜을 깖으로", "m2", "r2", "b", false, false, true, false, "깖", ""},
		{"--take가 --pull보다 먼저", "m2", "r2", "b", false, true, true, false, "모음", ""},
		{"base 없음·맥만 있음 — 모음", "m", "-", "-", false, false, false, false, "모음", ""},
		{"저장소에서 지움(경로 아는 base) — 깖(맥 지움)", "b", "-", "b", false, false, false, false, "깖", ""},
		{"구역 파일이 저장소에서 지워짐 — 충돌", "b", "-", "b", false, false, false, true, "", ""},
		{"구역 파일은 --pull이어도 지우지 않음", "b", "-", "b", false, false, true, true, "", ""},
		{"옛 두 칸 base에서 저장소에 없음 — 멈춤", "b", "-", "b", true, false, false, false, "", old},
		{"옛 base라도 --pull repo:면 지움", "b", "-", "b", true, false, true, false, "깖", ""},
		{"옛 base라도 저장소에 있으면 깖", "b", "r2", "b", true, false, false, false, "깖", ""},
	} {
		act, why := repoAct(c.m, c.r, c.b, c.oldBase, c.take, c.pull, c.sect)
		if act != c.act || why != c.why {
			t.Errorf("%s: repoAct(%q,%q,%q,옛 %v,take %v,pull %v,구역 %v) = (%q,%q), 기대 (%q,%q)",
				c.name, c.m, c.r, c.b, c.oldBase, c.take, c.pull, c.sect, act, why, c.act, c.why)
		}
	}
}
