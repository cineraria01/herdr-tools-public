package repo_test

// 자체 실행기 금지(gofix3-runner, 재감사 2바퀴 계약 P6): 도구는 자식을 internal/proc 함수로만 띄운다. cmd/·internal/의 시험 아닌 Go
// 파일에 exec.CommandContext·.WaitDelay·직접 c.Start()·ps로 읽은 PID 나무에 신호(ppid= 읽기 + syscall.Kill)가 있으면 실패 —
// 아래 허용 목록(파일 · 이유)만 빼고. 새 자체 실행기가 생기면 여기서 걸린다(다른 레인 몫 자리는 그 레인이 고치면 목록에서 지운다).

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"herdr-tools/internal/clitest"
)

// runnerAllow: 경로(앞부분이 같으면 그 아래 모두) → 이유
var runnerAllow = map[string]string{
	"internal/proc/":                    "proc 자신 — 실행·소유·마감 부품",
	"internal/clitest/":                 "시험 도우미(시험만 부른다)",
	"cmd/herdr-hooks/hookexec.go":       "훅 감독 — 1바퀴 R-M01 고침, 2바퀴 지적 없음",
	"cmd/herdr-hooks/hookexec_unix.go":  "훅 감독 — 1바퀴 R-M01 고침, 2바퀴 지적 없음",
	"cmd/herdr-hooks/cotenant_scan.go":  "휴지통 rm을 뒤에서 놓아 보냄(기다리지 않음, 결과를 읽지 않음) — 다른 레인 몫",
	"cmd/hc-fill/gc.go":                 "바퀴와 따로 도는 정리 실행기(무리·잠금) — 다른 레인 몫",
	"cmd/hc-fill/onland.go":             "착지 뒤 실행기 — gofix3-proc 몫(proc 기록으로)",
	"cmd/hc-fill/audit.go":              "hc send 20초 — 다른 레인 몫",
	"cmd/hc-lane/hcx.go":                "capt(CommandContext·WaitDelay·killTree) — lane/R-215·R-216은 재감사 2바퀴에서 뺀 지적(확인·재현된 것만)",
	"cmd/hc-lane/sys_unix.go":           "killTree(ps PID 나무 신호) — lane/R-216은 재감사 2바퀴에서 뺀 지적(확인·재현된 것만)",
	"cmd/hc-lane/start.go":              "감시 실행기를 놓아 보냄(Release) — 다른 레인 몫",
	"cmd/hc-lane/verdict.go":            "hc get 시한 — 다른 레인 몫",
	"cmd/hc-launchd-guard/main.go":      "proc.WaitMax 위 WaitDelay(관 기다림) — 다른 레인 몫",
	"cmd/herdr-settings-merge/write.go": "cp -p 60초 — 다른 레인 몫",
	"cmd/herdr-sync/agent.go":           "proc.Start로 띄운 파이프 묶음의 WaitDelay — 다른 레인 몫(gofix3-sync)",
	"cmd/hc-sweep/main.go":              "run(proc.Start + WaitDelay) — base/R-204·R-215는 재감사 2바퀴에서 뺀 지적(Mythos 판정)",
	"cmd/herdr-balance/main.go":         "runLim(CommandContext·WaitDelay) — base/R-216은 재감사 2바퀴에서 뺀 지적(Mythos 판정)",
	"internal/workermac/workermac.go":   "Command·Run(CommandContext·WaitDelay) — base/R-216은 재감사 2바퀴에서 뺀 지적(Mythos 판정)",
	"cmd/hc-sweep/orphans.go":           "남의 고아를 끄는 sweep 고유 일 — gofix3-sweep 몫(base/R-221)",
	"cmd/hc-sweep/judge.go":             "고아 판정용 PID 나무 읽기 — gofix3-sweep 몫",
}

var (
	reCtxCmd  = regexp.MustCompile(`exec\.CommandContext\(`)
	reWaitDel = regexp.MustCompile(`\.WaitDelay\b`)
	reStart   = regexp.MustCompile(`\.Start\(\)`)
	rePpid    = regexp.MustCompile(`ppid=`)
	reKill    = regexp.MustCompile(`syscall\.Kill\([^-]`)
)

func runnerAllowed(p string) bool {
	for k := range runnerAllow {
		if p == k || (strings.HasSuffix(k, "/") && strings.HasPrefix(p, k)) {
			return true
		}
	}
	return false
}

func TestNoOwnRunners(t *testing.T) {
	R := clitest.Root(t)
	files, err := tracked(R, "cmd/*.go", "internal/*.go")
	if err != nil {
		t.Fatal(err)
	}
	var bad []string
	for _, p := range files {
		if strings.HasSuffix(p, "_test.go") || runnerAllowed(p) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(R, p))
		if err != nil {
			t.Fatalf("%s 를 못 읽음: %v", p, err)
		}
		s := string(b)
		var why []string
		if reCtxCmd.MatchString(s) {
			why = append(why, "exec.CommandContext")
		}
		if reWaitDel.MatchString(s) {
			why = append(why, "WaitDelay")
		}
		if reStart.MatchString(s) {
			why = append(why, "직접 Start()")
		}
		if rePpid.MatchString(s) && reKill.MatchString(s) {
			why = append(why, "PID 나무 신호")
		}
		if len(why) > 0 {
			bad = append(bad, p+": "+strings.Join(why, "·"))
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Errorf("자체 실행기(proc 함수로 바꾸거나 이유를 적어 runnerAllow에):\n  %s", strings.Join(bad, "\n  "))
	}
}
