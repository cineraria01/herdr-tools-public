package rest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"herdr-tools/internal/upstream"
)

// 판 확인(ht-upstream-contract ③④·외부 가정): 기록이 없으면 지금 그대로(처음 깔릴 때 보류 없음), 확인 안 된 판·틀린 판·판을 못 읽는
// 세션은 모름(닫지 않음 — R3는 hc-sweep settle이 지금 규칙대로), 통과한 판은 평소대로. 한 기계에 판이 둘이면 그 세션의 판으로. Look은 판을 안 본다
func TestOfUpstreamGate(t *testing.T) {
	dirs(t)
	now := float64(time.Now().UnixMilli())
	os.MkdirAll(SessDir, 0o755)
	sess := func(pid, ver string) {
		b := `{"sessionId":"s` + pid + `"`
		if ver != "" {
			b += `,"version":"` + ver + `"`
		}
		os.WriteFile(filepath.Join(SessDir, pid+".json"), []byte(b+"}"), 0o644)
	}
	sess("1", "2.0.1")
	sess("2", "2.0.2")
	sess("3", "")
	of := func(pid float64) Verdict { return Of(map[string]any{"pid": pid}, now, 3.6e7, noPS, nil) }
	for _, p := range []float64{1, 2, 3} {
		if v := of(p); v.State != Quiet {
			t.Fatalf("기록 없음 = 지금 그대로(pid %v): %+v", p, v)
		}
	}
	put := func(r *upstream.Rec) {
		if err := upstream.Update(UpFile, func(*upstream.Rec) *upstream.Rec { return r }); err != nil {
			t.Fatal(err)
		}
	}
	put(&upstream.Rec{HC: "0.1.22", Bundles: map[string]*upstream.Bundle{
		upstream.ID("2.0.1", "0.1.22"): {State: upstream.Trusted},
		upstream.ID("2.0.2", "0.1.22"): {State: upstream.Pending},
	}})
	if v := of(1); v.State != Quiet {
		t.Errorf("믿는 판: 평소대로 %+v", v)
	}
	if v := of(2); v.State != Unknown || v.Sig != "이 판은 아직 확인 안 됨(claude-2.0.2_hc-0.1.22)" || v.Sid != "s2" {
		t.Errorf("확인 안 된 판(옛 세션 옆 새 세션): 모름 %+v", v)
	}
	if v := of(3); v.State != Unknown || v.Sig != "판 모름(세션 판 - · hc 0.1.22)" {
		t.Errorf("세션 판 없음(꼴이 다름): 모름 %+v", v)
	}
	if v := Look(map[string]any{"pid": 2.0}, now, 3.6e7, noPS, nil); v.State != Quiet {
		t.Errorf("Look은 판을 안 본다: %+v", v)
	}
	put(&upstream.Rec{HC: "0.1.23", Bundles: map[string]*upstream.Bundle{upstream.ID("2.0.1", "0.1.22"): {State: upstream.Trusted}}})
	if v := of(1); v.State != Unknown {
		t.Errorf("hc 판이 바뀜: 모름 %+v", v)
	}
	put(&upstream.Rec{HC: "0.1.23", Bundles: map[string]*upstream.Bundle{upstream.ID("2.0.1", "0.1.23"): {State: upstream.Failed}}})
	if v := of(1); v.State != Unknown || v.Sig != "이 판은 실물 점검에서 틀림(claude-2.0.1_hc-0.1.23)" {
		t.Errorf("틀린 판: 모름 %+v", v)
	}
	os.WriteFile(UpFile, []byte("{"), 0o644)
	if v := of(1); v.State != Unknown || v.Sig != "판 기록 못 읽음" {
		t.Errorf("깨진 기록: 모름 %+v", v)
	}
	if v := Of(map[string]any{"pid": 9.0}, now, 3.6e7, noPS, nil); v.State != Quiet {
		t.Errorf("세션 없는 셸은 판과 상관없다: %+v", v)
	}
}
