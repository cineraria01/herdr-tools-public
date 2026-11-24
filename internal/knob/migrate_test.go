//go:build !windows

package knob

import "testing"

// knobs-migrate: 고정으로 박혔던 값을 등록 표로 옮긴다. 기본은 지금 코드의 값 그대로(옮기는 것만으로 동작이 바뀌지 않는다),
// 도구를 망가뜨리는 값(0초 주기·0개 묶음 등)은 범위 밖으로 거절. 다른 기계의 도구가 읽는 값은 Remote.
func TestMigratedDefs(t *testing.T) {
	want := []struct {
		name, tool, def string
		remote          bool
		bad             []string // 범위 밖이어야 하는 값
	}{
		// 병합기(병합기 기계 — 원격 참조로 읽음, 단위 초·개)
		{"merger_batch", "hc-merger", "3", true, []string{"0", "-1"}},
		{"merger_tick", "hc-merger", "10", true, []string{"0"}},
		{"merger_spec_start", "hc-merger", "300", true, []string{"-1"}},
		{"merger_spec_timeout", "hc-merger", "3600", true, []string{"0", "59"}},
		{"merger_front_wait", "hc-merger", "600", true, []string{"0", "59"}},
		{"merger_pre_max", "hc-merger", "2700", true, []string{"0", "59"}},
		{"merger_pre_rebuild", "hc-merger", "1", true, []string{"-1"}},
		// 정리 도구(모든 기계 — 원격 참조로 읽음, 단위 분)
		{"sweep_r1", "hc-sweep", "30", true, []string{"0"}},
		{"sweep_r2", "hc-sweep", "30", true, []string{"-1"}},
		{"sweep_r3", "hc-sweep", "360", true, []string{"0"}},
	}
	for _, w := range want {
		d, ok := Find(w.name)
		if !ok {
			t.Errorf("%s가 등록 표에 없다", w.name)
			continue
		}
		if d.Tool != w.tool || d.Default != w.def || d.Remote != w.remote {
			t.Errorf("%s: 도구 %q 기본 %q 원격 %v — 원하는 것 %q %q %v", w.name, d.Tool, d.Default, d.Remote, w.tool, w.def, w.remote)
		}
		if d.Desc == "" || d.Unit == "" {
			t.Errorf("%s: 설명·단위가 비었다", w.name)
		}
		for _, b := range w.bad {
			if d.Check(b) == nil {
				t.Errorf("%s: 도구를 망가뜨리는 값 %q를 받았다", w.name, b)
			}
		}
	}
}
