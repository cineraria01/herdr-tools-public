package util_test

// 2026-11-21 Go 전환 전체 감사(go-final-audit base — 아스트라 6.2 max·Fable 5.2 xhigh) 가운데 hc-sweep 지적의 계약 시험.
// 이름 앞 [ID]는 보고서 지적 ID.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/lockx"
)

func TestHcSweepAudit(t *testing.T) { run1(t, testHcSweepAudit) }

func testHcSweepAudit(t *testing.T, T string, e Env) {
	SWEEP := tool(t, "hc-sweep")
	var out string
	var rc int
	c := checker{t: t, out: &out, rc: &rc}
	has := func(s string) bool { return strings.Contains(out, s) }
	mkdirs(t, T+"/bin", T+"/home/.claude-tasks", T+"/cfg", T+"/tmp", T+"/cache")
	write(t, T+"/cfg/config", "sweep.disk_free_gb 0\n")
	// 가짜 hc: ls는 $T/ls.json. get <주소>: kill 뒤면 $T/after-kill(종료 코드·stderr — 없으면 finished), 아니면 n번째 부름에
	// $T/get-<주소>.<n>.json, 없으면 $T/get-<주소>.json. read는 $T/read.txt(없으면 프롬프트 줄). exec는 $T/exec-rc·exec-err가 있으면 그것,
	// 없으면 --cwd로 들어가 그대로 실행(폴더가 없으면 hc처럼 실패). kill·rm은 표식만
	script(t, T+"/bin/hc", T, `#!/bin/bash
case $1 in
  ls) exec cat "@T@/ls.json" ;;
  get) if [ -e "@T@/killed-$2" ]; then
         [ -f "@T@/after-kill" ] || exec sed 's/"state":"live"/"state":"finished"/' "@T@/get-$2.json"
         sed -n 2p "@T@/after-kill" >&2; exit "$(sed -n 1p "@T@/after-kill")"
       fi
       n=$(( $(cat "@T@/getn-$2" 2>/dev/null || echo 0) + 1 )); echo $n >"@T@/getn-$2"
       [ -f "@T@/get-$2.$n.json" ] && exec cat "@T@/get-$2.$n.json"
       [ -f "@T@/get-$2.json" ] && exec cat "@T@/get-$2.json"
       echo '{"error":{"kind":"not_found","message":"no terminal"}}' >&2; exit 3 ;;
  read) [ -f "@T@/read.txt" ] && exec cat "@T@/read.txt"; echo "  ⏵⏵ bypass permissions on (shift+tab to cycle)" ;;
  exec) [ -f "@T@/exec-rc" ] && { cat "@T@/exec-err" >&2; exit "$(cat "@T@/exec-rc")"; }
        shift; d=
        while [ "$1" != -- ]; do case $1 in --cwd) d=$2; shift 2 ;; --env) export "$2"; shift 2 ;; --timeout) shift 2 ;; *) shift ;; esac; done
        shift; [ -z "$d" ] || cd "$d" 2>/dev/null || { echo '{"error":{"message":"cannot start folder: No such file or directory"}}' >&2; exit 1; }
        exec "$@" ;;
  kill) touch "@T@/killed-$2" ;;
  rm) ;;
  *) exit 1 ;;
esac
`)
	e = e.With("PATH="+T+"/bin:"+e["PATH"], "HOME="+T+"/home", "HERDR_TOOLS_CONFIG="+T+"/cfg",
		"XDG_CACHE_HOME="+T+"/cache", "TMPDIR="+T+"/tmp",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_NOSYSTEM=1")
	e = e.Without("XDG_STATE_HOME", "HC_LANE_MACHINE", "HC_FILL_DATA")
	claudeSess(t, T+"/home")
	sweep := func(args ...string) { out, rc = run(T, e, SWEEP, append([]string{"--only-under", T}, args...)...) }
	reset := func() {
		for _, g := range []string{"killed-*", "getn-*", "get-*.json", "after-kill", "read.txt", "exec-rc", "exec-err"} {
			ms, _ := filepath.Glob(T + "/" + g)
			for _, m := range ms {
				os.Remove(m)
			}
		}
		os.Remove(T + "/home/.local/state/hc-lane/events.tsv")
	}
	ms := func(ago time.Duration) int64 { return time.Now().Add(-ago).UnixMilli() }
	// term: hc get 객체. worker면 hc-lane 레인(라벨), 아니면 사람 칸 아닌 에이전트(R3 후보)
	term := func(addr, name string, worker bool, state string, created, changed int64, cwd string) string {
		lab := ""
		if worker {
			lab = `"labels":{"hc-lane":"1"},`
		}
		return fmt.Sprintf(`{"address":"%s","name":"%s","state":"live","pid":4343,"status":{"app":"claude-code","state":"%s"},%s"created_ms":%d,"status_ms":%d,"turn":4,"cwd":"%s"}`+"\n",
			addr, name, state, lab, created, changed, cwd)
	}
	ls := func(addrs ...string) {
		var s []string
		for _, a := range addrs {
			s = append(s, fmt.Sprintf(`{"address":"%s","state":"live","status":"idle"}`, a))
		}
		write(t, T+"/ls.json", `{"terminals":[`+strings.Join(s, ",")+"]}\n")
	}

	// [B02] 위치 인자 하나가 뒤의 --dry-run을 무력화했다(Go 판 flag가 첫 위치 인자에서 멈춤) — 쓸기 전에 사용 오류 2
	IX := T + "/cache/codebase-memory-mcp"
	mkdirs(t, IX)
	bash(t, T, e, `exec sqlite3 "$1" "$2" >/dev/null`, IX+"/gone.db", mkdbSQL(false, "2026-11-12T07:51:04Z", T+"/repo.wt/없음"))
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	os.Chtimes(IX+"/gone.db", old, old)
	ls()
	out, rc = run(T, e, SWEEP, "typo", "--dry-run")
	c.ok("[B02] 남은 위치 인자: 종료 2, 색인을 안 지움", rc == 2 && exists(IX+"/gone.db"))
	os.Remove(IX + "/gone.db")

	old7h := ms(7 * time.Hour)
	// [B04] get과 kill 사이에 주소가 다른 실행으로 재사용되면 kill하지 않는다(닫기 직전 이름·생성 시각·상태 다시 대조)
	reset()
	ls("t1")
	write(t, T+"/get-t1.1.json", term("t1", "plain", false, "idle", old7h, old7h, T+"/gone-t1"))
	write(t, T+"/get-t1.json", term("t1", "other", false, "working", ms(time.Minute), ms(time.Minute), T+"/gone-t1"))
	sweep("--min-age", "0")
	c.ok("[B04] 재사용된 주소(이름·생성 시각·상태가 바뀜)는 kill 안 함", !exists(T+"/killed-t1"))
	c.ok("[B04] 재사용이면 정리됨·closed.log도 안 씀", !strings.Contains(read(T+"/home/.claude-tasks/closed.log"), "\tt1\tplain\tR3\t"))

	// [B05] kill 뒤 hc get이 not_found가 아닌 실패(일시 단절 75)면 「닫힘」으로 읽지 않는다(그래야 hc-lane clean으로 안 간다).
	// not_found(rc 3)면 감싸는 실행기가 지운 것이라 닫힘
	reset()
	ls("t2")
	write(t, T+"/get-t2.json", term("t2", "plain2", false, "idle", old7h, old7h, T+"/gone-t2"))
	write(t, T+"/after-kill", "75\n{\"error\":{\"kind\":\"io\",\"message\":\"connection reset\"}}\n")
	sweep("--min-age", "0")
	c.ok("[B05] kill 뒤 get 일시 실패: 닫힘 아님(모름)", exists(T+"/killed-t2") && (!has("닫힘")))
	reset()
	ls("t2")
	write(t, T+"/get-t2.json", term("t2", "plain2", false, "idle", old7h, old7h, T+"/gone-t2"))
	write(t, T+"/after-kill", "3\n{\"error\":{\"kind\":\"not_found\",\"message\":\"no terminal\"}}\n")
	sweep("--min-age", "0")
	c.ok("[B05] kill 뒤 get not_found: 닫힘(이미 지워짐)", has("닫힘(이미 지워짐): R3 t2"))

	// [E12] keep 목록 파일이 있는데 못 읽으면 보호할 이름이 없다고 보지 않는다 — 그 바퀴 터미널을 닫지 않음
	reset()
	ls("t3")
	write(t, T+"/get-t3.json", term("t3", "keepme", false, "idle", old7h, old7h, T+"/gone-t3"))
	KEEP := T + "/home/.claude-tasks/keep"
	write(t, KEEP, "keepme\n")
	os.Chmod(KEEP, 0)
	sweep("--dry-run", "--min-age", "0")
	os.Chmod(KEEP, 0o644)
	c.ok("[E12] keep을 못 읽으면 닫을 것 없음", !has("닫을 것: R3 t3"))
	os.Remove(KEEP)

	idle31 := ms(31 * time.Minute)
	// [E08] hc read가 종료 0에 빈 화면이면 「배경 실행 없음」이 아니라 모름 — R2로 닫지 않는다
	reset()
	ls("t4")
	write(t, T+"/get-t4.json", term("t4", "lane-e08", true, "idle", ms(2*time.Hour), idle31, T+"/gone-t4"))
	write(t, T+"/read.txt", "")
	sweep("--dry-run", "--min-age", "0")
	c.ok("[E08] 빈 화면이면 R2 아님", (!has("닫을 것: R2 t4") && has("남김: t4")))

	// [E07] 작업 폴더가 있는데 git cherry가 다른 까닭(문구에 No such file or directory)으로 실패하면 착지 전 커밋 모름 — 닫지 않음
	reset()
	ls("t5")
	mkdirs(t, T+"/plain-e07")
	write(t, T+"/get-t5.json", term("t5", "lane-e07", true, "idle", ms(2*time.Hour), idle31, T+"/plain-e07"))
	write(t, T+"/exec-rc", "128")
	write(t, T+"/exec-err", "fatal: unable to read .git/objects/info/alternates: No such file or directory\n")
	sweep("--dry-run", "--min-age", "0")
	c.ok("[E07] 폴더가 있고 cherry 실패면 R2 아님", (!has("닫을 것: R2 t5") && has("남김: t5")))

	// [B03] 같은 이름의 옛 레인 사건(다른 실행 키)·이 터미널보다 먼저 생긴 원격 표식은 새 레인의 착지 증거가 아니다
	reset()
	G, R := T+"/lg", T+"/lg/repo"
	mkdirs(t, G)
	bash(t, T, e, `G=$1 R=$2
git init -q --bare -b main "$G/origin.git" && git clone -q "$G/origin.git" "$R" 2>/dev/null &&
  git -C "$R" commit -q --allow-empty -m base && git -C "$R" push -q origin HEAD:main || exit 1
git -C "$R" worktree add -q -b land-x "$R.wt/land-x" origin/main 2>/dev/null || exit 1
(cd "$R.wt/land-x" && echo a >a && git add a && git commit -q -m x && git push -q origin HEAD:main) || exit 1
git -C "$R" push -q origin "HEAD:refs/hc-landed/land-x/1600000000" && git -C "$R" fetch -q origin`, G, R)
	EVT := T + "/home/.local/state/hc-lane/events.tsv"
	write(t, EVT, "2026-11-01T10:00:00Z\tm\tt40\tland-x\t착지\tabc1234\t-\t3\t-\tt40@1600000000000\n"+
		"2026-11-01T10:00:00Z\tm\tt41\tland-x\t완료\tabc1234\t-\t3\n")
	ls("t81")
	write(t, T+"/get-t81.json", term("t81", "land-x", true, "done", 1700000000000, idle31, R+".wt/land-x"))
	sweep("--dry-run", "--min-age", "0")
	c.ok("[B03] 옛 실행의 착지 사건·옛 원격 표식은 증거 아님",
		(has("남김: t81 land-x — 작업 트리 남음 — 착지 증거 없음") && !has("hc-lane clean land-x")))
	// 이 실행 키의 사건이면 증거(교착 풀기는 그대로)
	write(t, EVT, "2026-11-21T10:00:00Z\tm\tt81\tland-x\t제출\tabc1234\t-\t3\t-\tt81@1700000000000\n")
	sweep("--dry-run", "--min-age", "0")
	c.ok("[B03] 이 실행 키의 제출 사건은 증거", has("(닫은 뒤 hc-lane clean land-x)"))
	// 이 터미널이 생긴 뒤 원격 표식(UTC 초 > created의 초)이고 그 커밋이 main에 든 이 가지의 착지면 증거(R-01)
	os.Remove(EVT)
	bash(t, T, e, `git -C "$1" push -q origin "HEAD:refs/hc-landed/land-x/1800000000"`, R+".wt/land-x")
	sweep("--dry-run", "--min-age", "0")
	c.ok("[B03] 이 터미널 뒤의 원격 표식은 증거", has("(닫은 뒤 hc-lane clean land-x)"))

	// [B07·gofix-core 넘김] 두 판 공통 잠금: 다른 쓸기(어느 판이든)가 sweep.lock을 쥐면 아무것도 안 하고 75. --dry-run은 잡지 않는다
	reset()
	ls("t1")
	write(t, T+"/get-t1.json", term("t1", "plain", false, "idle", old7h, old7h, T+"/gone-t1"))
	mkdirs(t, T+"/home/.local/state/hc-sweep")
	lk, busy, err := lockx.TryLock(T + "/home/.local/state/hc-sweep/sweep.lock")
	if err != nil || busy {
		t.Fatalf("잠금 준비: %v %v", busy, err)
	}
	sweep("--min-age", "0")
	c.ok("[B07] 잠금을 남이 쥐면 75·안 닫음", rc == 75 && !exists(T+"/killed-t1"))
	sweep("--dry-run", "--min-age", "0")
	c.ok("[B07] --dry-run은 잠금과 상관없이 돈다", rc == 0 && has("닫을 것: R3 t1"))
	lk.Close()
	sweep("--min-age", "0")
	c.ok("[B07] 잠금이 풀리면 돈다", rc == 0 && exists(T+"/killed-t1"))
}
