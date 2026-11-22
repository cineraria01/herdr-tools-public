package hclane_test

// gofix2-core(Go 전환 재감사 아스트라 1바퀴) — 프로세스 지적 R-19 고치기 전 실패 시험(Go 판만 단언, 셸 판은 곧 걷는다)

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// R-19: 배경 시작(hc run --bg)이 멈추면 원격 상한(HERDR_TIMEOUT_REMOTE) 안에서 끊고 「결과를 확인하지 못했다」로 실패(작업 트리 보존)
// (옛 판: hc run을 상한 없이 c.Run()으로 기다려 상한 1초에도 4초 넘게 섰다)
func TestReauditLaunchRunLimit(t *testing.T) {
	withK(t, func(k *K) {
		real := ""
		for _, d := range filepath.SplitList(k.env["PATH"]) {
			if exists(filepath.Join(d, "hc")) {
				real = filepath.Join(d, "hc")
				break
			}
		}
		p := k.gfBin(map[string]string{"hc": `[ "$1" = run ] && { sleep 8; exit 0; }; exec ` + sq(real) + ` "$@"`})
		k.mk("r19")
		t0 := time.Now()
		k.runE([]string{p, "HERDR_TIMEOUT_REMOTE=1"}, "--name", "lane-r19", "--repo", k.R, "--", "claude")
		el := time.Since(t0)
		k.check("R-19 hc run --bg 멈춤: 상한 안에 끊고 실패(결과 모름), 작업 트리 보존",
			k.rc != 0 && el < 7*time.Second && strings.Contains(k.out+k.errf(), "결과를 확인하지 못했다") && isDir(k.R+".wt/lane-r19"))
	})
}
