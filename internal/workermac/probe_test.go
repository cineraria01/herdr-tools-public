package workermac

import "testing"

// 풀 프록시 status 출력에서 쓸 계정 수만(2026-11-28 실제 꼴 「Usable now:     2/6 accounts」 — 계정 줄은 시험 대역 값)
func TestPoolUsable(t *testing.T) {
	for out, want := range map[string]int{
		"Server:         running (pid 1, port 1)\nUsable now:     2/6 accounts\n    Status:   limit (not usable now)\n": 2,
		"Usable now:\t0/6 accounts\n": 0,
		"usable now: 13":              13,
	} {
		if n, ok := PoolUsable(out); !ok || n != want {
			t.Errorf("%q → %d %v, 바라는 것 %d", out, n, ok, want)
		}
	}
	for _, out := range []string{"", "Server: stopped", "    Status:   limit (not usable now)", "Usable now: -"} {
		if n, ok := PoolUsable(out); ok {
			t.Errorf("%q → %d, 모름이어야", out, n)
		}
	}
}
