package util_test

// gofix4-sweep(R-3M02): 착지한 keep 셸 레인 닫기(sweep-landed-keep)를 CLI 실제 경로(hc ls → hc get → judge → closeTerm → hc kill)로 본다.
// hc keep이 켜진 레인 터미널은 에이전트가 끝나면 셸로 바뀌어 status 없이 live로 남는다 — 전경(foreground)이 셸이고 마지막 사건이 착지면 닫는다

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// kpTerm: keep 셸 레인 t7의 hc get 답 — 실제 hc 0.1.21 꼴(gofix5 R-4C01): 잃은 프로그램을 셸로 바꾸면 위 argv·pid도 그 셸
// (["/bin/zsh","-l"]·터미널 pid 4242)이 된다. fg는 전경 프로그램 이름, fgPid는 전경 pid(4242면 쉬는 셸 자신, 다르면 그 셸에서 도는 것 —
// R-4A09). argv·pid는 hc-sweep이 버려야 할 칸
func kpTerm(created int64, fg string, fgPid int) string {
	return fmt.Sprintf(`{"address":"t7","name":"land-keep","state":"live","keep":true,"status":null,"labels":{"hc-lane":"1","where":"app / 1"},`+
		`"created_ms":%d,"status_ms":%d,"turn":5,"cwd":"/nonexistent-kp/land-keep","argv":["/bin/zsh","-l"],"pid":4242,`+
		`"foreground":{"name":"%s","pid":%d,"argv":["%s","-l"]}}`+"\n", created, created, fg, fgPid, fg)
}

// kpSweep: 가짜 hc(get은 get-t7.json, kill 뒤엔 finished)와 이 실행 키의 착지 사건을 두고 쓸기를 돌린다(전경은 터미널 자신)
func kpSweep(t *testing.T, fg string, args ...string) (T, out string, rc int) {
	return kpSweepPid(t, fg, 4242, args...)
}

func kpSweepPid(t *testing.T, fg string, fgPid int, args ...string) (T, out string, rc int) {
	T, e, _ := j2Sweep(t)
	created := (time.Now().Unix() - 14*3600) * 1000
	write(t, T+"/ls.json", `{"terminals":[{"address":"t7","state":"live","agent":"zsh","status":null}]}`+"\n")
	write(t, T+"/get-t7.json", kpTerm(created, fg, fgPid))
	write(t, T+"/home/.local/state/hc-lane/events.tsv",
		fmt.Sprintf("2026-11-21T10:00:00Z\tm\tt7\tland-keep\t착지\tabc1234\t-\t5\t-\tt7@%d\n", created))
	script(t, T+"/bin/hc", T, `#!/bin/bash
echo "$*" >>"@T@/hc.log"
case $1 in
  ls) exec cat "@T@/ls.json" ;;
  get) [ -e "@T@/killed-$2" ] && exec sed 's/"state":"live"/"state":"finished"/' "@T@/get-$2.json"
       [ -f "@T@/get-$2.json" ] && exec cat "@T@/get-$2.json"; exit 1 ;;
  read) echo "% " ;;
  kill) touch "@T@/killed-$2" ;;
  rm) ;;
  *) exit 1 ;;
esac
`)
	out, rc = run(T, e, tool(t, "hc-sweep"), append([]string{"--only-under", T}, args...)...)
	return T, out, rc
}

func kpKills(T string) int {
	n := 0
	for _, l := range j2Calls(T) {
		if strings.HasPrefix(l, "kill t7") {
			n++
		}
	}
	return n
}

// [R-3M02] 착지로 끝난 keep 셸 레인(status null · 전경 zsh · 작업 트리 없음)은 dry-run에 「닫을 것: R2」, 실제 쓸기에 hc kill 1회
func TestKeepShellLandedCloses(t *testing.T) {
	_, out, rc := kpSweep(t, "zsh", "--dry-run")
	c := checker{t: t, out: &out, rc: &rc}
	c.ok("dry-run: 닫을 것 R2 t7", strings.Contains(out, "닫을 것: R2 t7 land-keep"))

	T, out2, rc2 := kpSweep(t, "zsh")
	c = checker{t: t, out: &out2, rc: &rc2}
	c.ok("쓸기: hc kill t7 1회", kpKills(T) == 1)
	c.ok("쓸기: 닫힘 줄", strings.Contains(out2, "닫힘: R2 t7"))
	c.ok("쓸기: 종료 0", rc2 == 0)
}

// [R-3M02] 전경이 셸이 아니면(keep 셸에서 사람이 띄운 프로그램이 돎) 닫지 않는다
func TestKeepShellForegroundBusyKept(t *testing.T) {
	T, out, rc := kpSweep(t, "vim")
	c := checker{t: t, out: &out, rc: &rc}
	c.ok("전경 vim: hc kill 없음", kpKills(T) == 0)
}

// [R-4A09] 전경이 셸 이름이어도 터미널 자신이 아니면(그 셸에서 스크립트·bash -c가 돎 — pid 다름) 닫지 않는다
func TestKeepShellForegroundScriptKept(t *testing.T) {
	T, out, rc := kpSweepPid(t, "zsh", 70614)
	c := checker{t: t, out: &out, rc: &rc}
	c.ok("전경 zsh · pid 다름: hc kill 없음", kpKills(T) == 0)
}
