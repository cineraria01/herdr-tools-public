package util_test

// gofix2-base(R-02·R-06·R-36·R-39): hc-sweep 터미널 쓸기·인자·잠금 계약. 임시 HOME·가짜 hc(부른 차례를 hc.log에)·가짜 ps로 Go 판만 본다.
// 차례는 표식 파일(사건)로 맞춘다 — sleep 없음

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// j2Sweep: Go 판 hc-sweep용 임시 환경(T/bin이 PATH 앞, 디스크 쓸기 끔, Tailscale 끔)과 원본이 사라진 오래된 색인 gone.db
func j2Sweep(t *testing.T) (T string, e Env, gone string) {
	T = tmpdir(t)
	e = isolated(T)
	mkdirs(t, T+"/bin", T+"/home", T+"/cfg", T+"/tmp", T+"/cache/codebase-memory-mcp")
	e = e.With("PATH="+T+"/bin:"+e["PATH"], "HOME="+T+"/home", "HERDR_TOOLS_CONFIG="+T+"/cfg", "XDG_CACHE_HOME="+T+"/cache",
		"TMPDIR="+T+"/tmp", "HC_LANE_MACHINE=m", "HC_FILL_DATA="+T+"/fill")
	write(t, T+"/cfg/config", "sweep.disk_free_gb 0\n")
	claudeSess(t, T+"/home")
	gone = T + "/cache/codebase-memory-mcp/gone.db"
	bash(t, T, e, `sqlite3 "$1" "$2" >/dev/null && touch -t 202601010000 "$1"`, gone, mkdbSQL(false, "2026-11-12T07:51:04Z", T+"/repo.wt/audit-gone"))
	return T, e, gone
}

// claudeSess: 가짜 Claude 터미널(get 답의 "pid":4343)의 세션 파일 — 실측 꼴 ~/.claude/sessions/<pid>.json. 세션 자료 없는 Claude는 쉼
// 모름이라 닫지 않으니(ht-rest-one-gate) 닫기 규칙을 보는 시험은 세션이 있는 꼴로 둔다(대화 기록 없음 = 쉼)
func claudeSess(t *testing.T, home string) {
	write(t, home+"/.claude/sessions/4343.json", `{"pid":4343,"sessionId":"sid-fix"}`)
}

// j2Idle: 7시간 쉰 worker 터미널(get 답) — cwd는 없는 폴더라 작업 트리·착지 전 커밋 아님, R2 대상
func j2Idle(addr, name string, created int64, lane bool) string {
	lab := ""
	if lane {
		lab = `"labels":{"hc-lane":"1"},`
	}
	return fmt.Sprintf(`{"address":"%s","name":"%s","state":"live","pid":4343,"status":{"app":"claude-code","state":"idle"},%s"parent":"p/t0","created_ms":%d,"status_ms":%d,"turn":4,"cwd":"/nonexistent-j2/%s"}`+"\n",
		addr, name, lab, created, created, name)
}

const j2Ls = `{"terminals":[{"address":"t1","state":"live","status":"idle"}]}` + "\n"

// j2Calls: hc.log의 줄(부른 차례)
func j2Calls(T string) []string {
	var l []string
	for _, s := range strings.Split(read(T+"/hc.log"), "\n") {
		if s != "" {
			l = append(l, s)
		}
	}
	return l
}

// [R-39] 끝표시 「--」만 받으면 쓸기 전에 사용 오류 2 — hc를 부르지 않고 색인을 지우지 않는다. --self-test --도 2
func TestJ2SweepDashDash(t *testing.T) {
	T, e, gone := j2Sweep(t)
	script(t, T+"/bin/hc", T, `#!/bin/bash
echo "$*" >>"@T@/hc.log"
case $1 in ls) echo '{"terminals":[]}' ;; *) exit 1 ;; esac
`)
	SWEEP := tool(t, "hc-sweep")
	out, rc := run(T, e, SWEEP, "--")
	c := checker{t: t, out: &out, rc: &rc}
	c.ok("--만: 종료 2", rc == 2)
	c.ok("--만: hc를 부르지 않음", len(j2Calls(T)) == 0)
	c.ok("--만: 색인 gone.db 보존", exists(gone))
	out, rc = run(T, e, SWEEP, "--self-test", "--")
	c.ok("--self-test --: 종료 2", rc == 2)
}

// [R-06] 잠금 파일을 못 열면(sweep.lock 자리가 폴더) 쓸기 전에 0 아닌 종료 — 색인 삭제·hc kill/rm 없음
func TestJ2SweepLockOpenFail(t *testing.T) {
	T, e, gone := j2Sweep(t)
	mkdirs(t, T+"/home/.local/state/hc-sweep/sweep.lock")
	write(t, T+"/ls.json", j2Ls)
	write(t, T+"/get-t1.json", j2Idle("t1", "lane-a", (time.Now().Unix()-7*3600)*1000, false))
	script(t, T+"/bin/hc", T, `#!/bin/bash
echo "$*" >>"@T@/hc.log"
case $1 in
  ls) exec cat "@T@/ls.json" ;;
  get) [ -e "@T@/killed-$2" ] && { echo "{\"address\":\"$2\",\"state\":\"finished\"}"; exit 0; }
       [ -f "@T@/get-$2.json" ] && exec cat "@T@/get-$2.json"; exit 1 ;;
  read) echo "  ⏵⏵ bypass permissions on (shift+tab to cycle)" ;;
  kill) touch "@T@/killed-$2" ;;
  rm) ;;
  *) exit 1 ;;
esac
`)
	out, rc := run(T, e, tool(t, "hc-sweep"), "--only-under", T)
	c := checker{t: t, out: &out, rc: &rc}
	c.ok("잠금 못 엶: 0 아닌 종료", rc != 0)
	calls := strings.Join(j2Calls(T), "\n")
	c.ok("잠금 못 엶: hc kill·rm 없음", !grep(calls, `^(kill|rm) `))
	c.ok("잠금 못 엶: 색인 gone.db 보존", exists(gone))
}

// [R-36] kill이 실패(75)하고 터미널이 그대로 live면 events.tsv에 「정리됨」(완료)을 남기지 않고, 쓸기 종료 코드도 0이 아니다
// (정리됨은 hc-lane wait가 뜻밖의 종료·멈춤으로 소비한다 — kill 전에는 의도만)
func TestJ2SweepKillFailNoCleaned(t *testing.T) {
	T, e, _ := j2Sweep(t)
	write(t, T+"/ls.json", j2Ls)
	write(t, T+"/get-t1.json", j2Idle("t1", "lane-a", (time.Now().Unix()-7*3600)*1000, true))
	script(t, T+"/bin/hc", T, `#!/bin/bash
echo "$*" >>"@T@/hc.log"
case $1 in
  ls) exec cat "@T@/ls.json" ;;
  get) [ -f "@T@/get-$2.json" ] && exec cat "@T@/get-$2.json"; exit 1 ;;
  read) echo "  ⏵⏵ bypass permissions on (shift+tab to cycle)" ;;
  kill) echo '{"error":{"kind":"unavailable","message":"core busy"}}' >&2; exit 75 ;;
  rm) ;;
  *) exit 1 ;;
esac
`)
	out, rc := run(T, e, tool(t, "hc-sweep"), "--only-under", T)
	c := checker{t: t, out: &out, rc: &rc}
	c.ok("준비: kill을 불렀음", grep(strings.Join(j2Calls(T), "\n"), `^kill t1`))
	cleaned := false
	for _, l := range strings.Split(read(T+"/home/.local/state/hc-lane/events.tsv"), "\n") {
		if f := strings.Split(l, "\t"); len(f) > 4 && f[2] == "t1" && f[4] == "정리됨" {
			cleaned = true
		}
	}
	c.ok("kill 실패: events.tsv에 정리됨(완료) 없음", !cleaned)
	c.ok("kill 실패: 쓸기 종료 코드 0 아님", rc != 0)
}

// j2SwapHc: 주소 t1이 swapped 표식 뒤 다른 실행(이름 other·다른 created_ms)으로 바뀌는 가짜 hc. 가짜 ps는 t1을 한 번 본 뒤
// (got-t1) swap-on-ps 표식이 있으면 바꾼다(세션 PID를 찾는 동안의 교체). swap-on-kill이면 본래 레인 kill 직후 바꾼다
func j2SwapHc(t *testing.T, T string, otherState string) {
	old := (time.Now().Unix() - 7*3600) * 1000
	write(t, T+"/ls.json", j2Ls)
	write(t, T+"/orig.json", j2Idle("t1", "lane-a", old, false))
	write(t, T+"/other.json", strings.Replace(j2Idle("t1", "other", old+5000, false), `"state":"live"`, `"state":"`+otherState+`"`, 1))
	write(t, T+"/other-fin.json", strings.Replace(j2Idle("t1", "other", old+5000, false), `"state":"live"`, `"state":"finished"`, 1))
	script(t, T+"/bin/hc", T, `#!/bin/bash
L="@T@/hc.log"
case $1 in
  ls) echo ls >>"$L"; exec cat "@T@/ls.json" ;;
  get) echo "get $2" >>"$L"; [ "$2" = t1 ] || exit 1; touch "@T@/got-t1"
       if [ -e "@T@/swapped" ]; then [ -e "@T@/killed-other" ] && exec cat "@T@/other-fin.json"; exec cat "@T@/other.json"; fi
       [ -e "@T@/killed" ] && { echo '{"address":"t1","name":"lane-a","state":"finished"}'; exit 0; }
       exec cat "@T@/orig.json" ;;
  read) echo "  ⏵⏵ bypass permissions on (shift+tab to cycle)" ;;
  kill) echo "kill $2" >>"$L"
        if [ -e "@T@/swapped" ]; then touch "@T@/killed-other"
        else touch "@T@/killed"; [ -e "@T@/swap-on-kill" ] && { touch "@T@/swapped"; echo swap >>"$L"; }; fi ;;
  rm) echo "rm $2" >>"$L" ;;
  *) exit 1 ;;
esac
`)
	script(t, T+"/bin/ps", T, `#!/bin/bash
if [ -e "@T@/swap-on-ps" ] && [ -e "@T@/got-t1" ] && [ ! -e "@T@/swapped" ]; then touch "@T@/swapped"; echo swap >>"@T@/hc.log"; fi
exec /bin/ps "$@"
`)
}

// j2After: calls에서 swap 줄 뒤에 re에 걸린 줄이 있나
func j2After(calls []string, re string) bool {
	seen := false
	for _, l := range calls {
		if l == "swap" {
			seen = true
		} else if seen && grep(l, re) {
			return true
		}
	}
	return false
}

// [R-02] 마지막 확인 뒤(세션 PID를 찾는 동안) 주소가 다른 실행으로 바뀌면 그 주소를 kill·rm하지 않는다 — 각 변경 직전 다시 확인
func TestJ2SweepSwapBeforeKill(t *testing.T) {
	T, e, _ := j2Sweep(t)
	j2SwapHc(t, T, "live")
	write(t, T+"/swap-on-ps", "")
	out, rc := run(T, e, tool(t, "hc-sweep"), "--only-under", T)
	c := checker{t: t, out: &out, rc: &rc}
	calls := j2Calls(T)
	c.ok("교체 뒤 kill 없음(다른 실행을 끔): "+strings.Join(calls, " | "), !j2After(calls, `^kill t1`))
	c.ok("교체 뒤 rm 없음: "+strings.Join(calls, " | "), !j2After(calls, `^rm t1`))
}

// [R-02] 본래 레인을 끈 직후 주소가 다른 실행(이미 끝난 행)으로 바뀌면 그 행은 우리 것이 아니다 — rm 보류
func TestJ2SweepSwapAfterKill(t *testing.T) {
	T, e, _ := j2Sweep(t)
	j2SwapHc(t, T, "finished")
	write(t, T+"/swap-on-kill", "")
	out, rc := run(T, e, tool(t, "hc-sweep"), "--only-under", T)
	c := checker{t: t, out: &out, rc: &rc}
	calls := j2Calls(T)
	c.ok("준비: 본래 레인 kill 뒤 교체: "+strings.Join(calls, " | "), grep(strings.Join(calls, "\n"), `^swap$`))
	c.ok("끝난 행의 소유가 다르면 rm 없음: "+strings.Join(calls, " | "), !j2After(calls, `^rm t1`))
}
