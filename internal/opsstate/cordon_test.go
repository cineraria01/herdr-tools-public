package opsstate

import (
	"strings"
	"testing"
)

// advise 격리 항목: 까닭 머리마다 보일 것·사람이 볼 것 한 줄(ht-probe-cordon), 머리 없는 조각은 저장소 읽기 또는 앞 까닭의 오류 글 속 「; 」
func TestCordonItemKinds(t *testing.T) {
	for why, want := range map[string][]string{
		"p /r: open: x":                                     {"저장소 폴더를 못 읽음"},
		CordonAuthPre + "p h: a; b":                         {"원격 로그인 거절"},
		"p /r: x; " + CordonWrite.Pre + "p h: denied; tail": {"저장소 폴더를 못 읽음·원격 쓰기 거절"},
		CordonPool.Pre + "쓸 계정 0; " + CordonKeychain.Pre + "rc 36; " + CordonTmp.Pre + "x": {
			"에이전트 풀에 쓸 계정 없음·키체인 막힘·작업 임시 폴더를 못 씀"},
		"": {"저장소 폴더를 못 읽음"},
	} {
		it := cordonItem(Machine{Name: "m", Cordon: &Cordon{SinceS: 600, Why: why}})
		if !strings.Contains(it.What, "m 격리 10분째 — "+want[0]) || strings.Count(it.Evidence, " · ") < strings.Count(want[0], "·") {
			t.Errorf("%q → %q / %q", why, it.What, it.Evidence)
		}
	}
	if it := cordonItem(Machine{Name: "m", Cordon: &Cordon{Why: CordonPool.Pre + "x"}}); !strings.Contains(it.Evidence, "worker.proxy") {
		t.Errorf("사람이 볼 것: %q", it.Evidence)
	}
}
