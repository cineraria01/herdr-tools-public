package main

import (
	"os"
	"testing"

	"herdr-tools/internal/upstream"
)

// ht-upstream-contract: 판 기록 줄 — 기록 없음 na · 깨짐 unknown · 보류 없고 점검할 기준 owed · 보류(확인 안 된 판) hold · 다 끝남 ok -
func TestProbeUpstreamLine(t *testing.T) {
	f := t.TempDir() + "/up.json"
	old := upFile
	upFile = func() string { return f }
	t.Cleanup(func() { upFile = old })
	if got := probeUpstream(); got != "probe upstream na 판 기록 없음" {
		t.Fatalf("없음: %q", got)
	}
	os.WriteFile(f, []byte("{"), 0o644)
	if got := probeUpstream(); got[:len("probe upstream unknown ")] != "probe upstream unknown " {
		t.Fatalf("깨짐: %q", got)
	}
	put := func(bs map[string]*upstream.Bundle) {
		os.Remove(f)
		var live []string // 모두 살아 있는 세션의 묶음(죽은 것은 보류·점검 대상이 아니다 — internal/upstream Owed)
		for id := range bs {
			live = append(live, id)
		}
		upstream.Update(f, func(*upstream.Rec) *upstream.Rec { return &upstream.Rec{HC: "1", Bundles: bs, Live: live} })
	}
	put(map[string]*upstream.Bundle{"a": {State: upstream.Trusted, Base: true}})
	if got := probeUpstream(); got != "probe upstream ok a" {
		t.Fatalf("기준 점검 차례: %q", got)
	}
	put(map[string]*upstream.Bundle{"a": {State: upstream.Trusted, Base: true}, "b": {State: upstream.Pending}})
	if got := probeUpstream(); got != "probe upstream hold b" {
		t.Fatalf("보류: %q", got)
	}
	put(map[string]*upstream.Bundle{"a": {State: upstream.Trusted, Base: true, Probed: "x"}, "c": {State: upstream.Failed}})
	if got := probeUpstream(); got != "probe upstream hold -" {
		t.Fatalf("틀린 판은 다시 점검하지 않음: %q", got)
	}
	put(map[string]*upstream.Bundle{"a": {State: upstream.Trusted, Probed: "x"}})
	if got := probeUpstream(); got != "probe upstream ok -" {
		t.Fatalf("다 끝남: %q", got)
	}
}
