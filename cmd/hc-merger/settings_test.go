package main

// 쉼 값·묶음 크기는 정수로(요구 5). 프로세스 무리·잠금 시험은 internal/proc·internal/lockx에 있다.

import "testing"

func TestIntSetting(t *testing.T) {
	for _, c := range []struct {
		v    string
		want int
	}{{"", 30}, {"17", 17}, {"0", 0}, {"abc", 30}, {"-5", 30}, {"1.5", 30}, {"99999999999999999999", 30}} {
		if got := intSetting("--tick", c.v, 30, 0); got != c.want {
			t.Errorf("intSetting(%q) = %d, 바람 %d", c.v, got, c.want)
		}
	}
	if got := intSetting("HC_LANE_MERGER_BATCH", "0", 3, 1); got != 3 {
		t.Errorf("묶음 0은 기본 3이어야: %d", got)
	}
}
