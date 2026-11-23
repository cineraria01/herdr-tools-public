package util_test

// gofix5-sweep(2026-11-23 Go 전환 재감사 바퀴 4): hc-sweep 분리 작업 트리 닫기·지우기를 CLI 실제 경로(hc ls → hc get → detachedClean →
// dirInUse → judge → closeTerm → detachedAfterClose)로 본다. 가짜 hc의 exec은 실제 명령(git·sh)을 그 폴더에서 돌린다.
// base/R-4M03(연결 시험 없음) · R-4M01(무시된 .qa 보관물째 지움) · R-4M02(hc exec 실패를 분리로 읽음) · merger/R-4A03(다른 철자 cwd) ·
// 다시 켠 기본과 HC_SWEEP_DETACHED_REMOVE=0(닫기만)

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/clitest"
)

// dtCase: 끝난 감사 터미널 t5(40분 쉼)의 작업 트리 W 꼴과 그 바퀴의 hc 답
type dtCase struct {
	worker, branch, qa, getChange bool
	others                        func(W string) []string // hc ls에 더할 live 터미널 JSON
	env                           []string
}

// dtSweep: 메인 R과 분리 작업 트리 W를 만들고 쓸기를 한 번 돌린다
func dtSweep(t *testing.T, c dtCase) (T, W, out string, rc int) {
	T, e, _ := j2Sweep(t)
	R, W := T+"/repo", T+"/repo.wt/aud"
	bash(t, T, e, `set -e; g() { git -c user.name=t -c user.email=t@t "$@"; }
g init -q -b main "$1"; cd "$1"; printf '.qa/\n' >.gitignore; g add .gitignore; g commit -qm base
g worktree add -q --detach "$2"`, R, W)
	if c.branch {
		bash(t, W, e, `git -c user.name=t -c user.email=t@t switch -q -c lane && git -c user.name=t -c user.email=t@t commit -q --allow-empty -m lane`)
	}
	if c.qa {
		write(t, W+"/.qa/x/REPORT.md", "r\n")
	}
	now := time.Now().UnixMilli()
	lane := ""
	if c.worker {
		lane = `"parent":"p/t0","labels":{"hc-lane":"1"},`
	}
	write(t, T+"/get-t5.json", fmt.Sprintf(`{"address":"t5","name":"aud","state":"live","status":{"app":"claude-code","state":"done"},%s`+
		`"created_ms":%d,"status_ms":%d,"turn":3,"cwd":"%s"}`+"\n", lane, now-90*60000, now-40*60000, W))
	var others []string
	if c.others != nil {
		others = c.others(W)
	}
	lsCwd := W
	if c.getChange { // 닫기가 「남김」인데 hc ls로는 아무도 그 폴더를 안 쓰는 꼴 — 지우기는 「닫힘」에만 묶여야 한다(변이 S9)
		lsCwd = "/"
	}
	self := fmt.Sprintf(`{"address":"t5","state":"live","status":"done","cwd":"%s"}`, lsCwd)
	write(t, T+"/ls.json", `{"terminals":[`+strings.Join(append([]string{self}, others...), ",")+"]}\n")
	write(t, T+"/ls-after.json", `{"terminals":[`+strings.Join(others, ",")+"]}\n") // 닫힌 t5는 목록에서 빠진다
	script(t, T+"/bin/hc", T, `#!/bin/bash
echo "$*" >>"@T@/hc.log"
case $1 in
  ls) [ -e "@T@/killed-t5" ] && exec cat "@T@/ls-after.json"; exec cat "@T@/ls.json" ;;
  get) n=$(( $(cat "@T@/gets-$2" 2>/dev/null || echo 0) + 1 )); echo $n >"@T@/gets-$2"
       [ -e "@T@/killed-$2" ] && exec sed 's/"state":"live"/"state":"finished"/' "@T@/get-$2.json"
       [ -n "${HC_FAKE_GET_CHANGE-}" ] && [ $n -ge 2 ] && exec sed 's/"status_ms":/"status_ms":1/' "@T@/get-$2.json"
       [ -f "@T@/get-$2.json" ] && exec cat "@T@/get-$2.json"; exit 1 ;;
  read) echo "% " ;;
  kill) touch "@T@/killed-$2" ;;
  rm) ;;
  exec) case "$*" in *"${HC_FAKE_EXEC_FAIL-@none@}"*) echo "hc exec 실패" >&2; exit 1 ;; esac   # 그 글이 든 exec만 hc 자신이 실패(종료 1)
        shift; c=; while [ "$1" != -- ]; do [ "$1" != --cwd ] || c=$2; shift; done; shift
        cd "$c" && exec "$@" ;;
  *) exit 1 ;;
esac
`)
	if c.getChange {
		c.env = append(c.env, "HC_FAKE_GET_CHANGE=1")
	}
	out, rc = run(T, e.With(c.env...), tool(t, "hc-sweep"), "--only-under", T)
	return T, W, out, rc
}

func dtKills(T string) int {
	n := 0
	for _, l := range j2Calls(T) {
		if strings.HasPrefix(l, "kill t5") {
			n++
		}
	}
	return n
}

func dtLive(addr, cwd string) string {
	return fmt.Sprintf(`{"address":"%s","state":"live","status":"working","cwd":"%s"}`, addr, cwd)
}

// [R-4M03·다시 켜기] 분리·깨끗 → 닫고(hc kill 1) 기본으로 git worktree remove. HC_SWEEP_DETACHED_REMOVE=0이면 닫기만
func TestSweepDetachedCloseRemove(t *testing.T) {
	T, W, out, rc := dtSweep(t, dtCase{})
	c := checker{t: t, out: &out, rc: &rc}
	c.ok("기본: 닫힘 R2 t5", strings.Contains(out, "닫힘: R2 t5 aud") && dtKills(T) == 1)
	c.ok("기본: 작업 트리 지움", !exists(W) && strings.Contains(out, "작업 트리 정리: "))
	c.ok("기본: 종료 0", rc == 0)

	T, W, out, rc = dtSweep(t, dtCase{env: []string{"HC_SWEEP_DETACHED_REMOVE=0"}})
	c = checker{t: t, out: &out, rc: &rc}
	c.ok("=0: 닫기만", dtKills(T) == 1 && exists(W) && strings.Contains(out, "작업 트리 남김(분리 지우기 꺼짐)"))
}

// [R-4M03·R-4A03] 다른 live 터미널이 그 폴더(아래·다른 철자)를 쓰면 닫지도 지우지도 않는다. 닫기가 「남김」이면 지우지 않는다
func TestSweepDetachedInUseKept(t *testing.T) {
	for name, others := range map[string]func(W string) []string{
		"그 아래 cwd": func(W string) []string { os.MkdirAll(W+"/sub", 0o755); return []string{dtLive("t6", W+"/sub")} },
		"다른 철자":    func(W string) []string { return []string{dtLive("t6", clitest.AltSpelling(t, W, false))} },
	} {
		T, W, out, rc := dtSweep(t, dtCase{others: others})
		c := checker{t: t, out: &out, rc: &rc}
		c.ok(name+": hc kill 없음·작업 트리 남음", dtKills(T) == 0 && exists(W))
	}
	T, W, out, rc := dtSweep(t, dtCase{getChange: true})
	c := checker{t: t, out: &out, rc: &rc}
	c.ok("닫기 직전 바뀜(남김): 지우지 않음", dtKills(T) == 0 && exists(W) && strings.Contains(out, "남김(닫기 직전"))
}

// [R-4M01] 무시된 .qa 보관물이 든 분리·깨끗 작업 트리는 닫지도 지우지도 않는다(유일한 사본)
func TestSweepDetachedIgnoredArchiveKept(t *testing.T) {
	T, W, out, rc := dtSweep(t, dtCase{qa: true, env: []string{"HC_SWEEP_DETACHED_REMOVE=1"}})
	c := checker{t: t, out: &out, rc: &rc}
	c.ok("보관물 남음", exists(W+"/.qa/x/REPORT.md"))
	c.ok("hc kill 없음", dtKills(T) == 0)
}

// [R-4M02] 가지 위 레인 작업 트리(착지 전 커밋)에서 가지·분리를 묻는 hc exec만 실패하면 모름 — 닫지 않는다
func TestSweepDetachedExecFailKept(t *testing.T) {
	T, W, out, rc := dtSweep(t, dtCase{worker: true, branch: true, env: []string{"HC_FAKE_EXEC_FAIL=symbolic"}})
	c := checker{t: t, out: &out, rc: &rc}
	c.ok("hc kill 없음·작업 트리 남음", dtKills(T) == 0 && exists(W))
}
