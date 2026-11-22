package util_test

// 옛 tests/hc-sweep.test.sh: 규칙 자체 시험(--self-test) + 고아 쓸기를 진짜 짧은 고아(sleep·스크립트)와 가짜 hc로.
// --only-under로 이 시험 폴더 아래 작업 트리만 보게 해 실제 기계의 다른 레인은 건드리지 않는다. 끝에 고아 0을 확인한다.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// bash: 준비용 셸 조각(실패면 시험 중단). 인자는 $1…로.
func bash(t *testing.T, dir string, e Env, src string, args ...string) string {
	t.Helper()
	out, rc := run(dir, e, "/bin/bash", append([]string{"-c", src, "bash"}, args...)...)
	if rc != 0 {
		t.Fatalf("준비 실패(rc=%d): %s\n%s", rc, src, out)
	}
	return out
}

// mkdbSQL: 색인 DB(codebase-memory 꼴 projects 표) 준비 SQL — sqlite3 명령에 넘긴다(hc-sweep도 sqlite3로 읽는다). wal이면 WAL 모드
func mkdbSQL(wal bool, at string, roots ...string) string {
	var b strings.Builder
	if wal {
		b.WriteString("PRAGMA journal_mode=WAL;\n")
	}
	b.WriteString("CREATE TABLE projects(name TEXT PRIMARY KEY, indexed_at TEXT, root_path TEXT);\n")
	for i, r := range roots {
		fmt.Fprintf(&b, "INSERT INTO projects VALUES ('p%d', '%s', '%s');\n", i, at, strings.ReplaceAll(r, "'", "''"))
	}
	return b.String()
}

func TestHcSweep(t *testing.T) { run1(t, testHcSweep) }

func testHcSweep(t *testing.T, T string, e Env) {
	SWEEP := tool(t, "hc-sweep")
	P := T + "/pids"
	mkdirs(t, P, T+"/bin", T+"/home", T+"/cfg", T+"/tmp") // tmp: 디스크 쓸기가 진짜 임시 폴더를 쓸지 않게
	var out string
	var rc int
	c := checker{t: t, out: &out}
	pids := map[string]int{}
	// sig: 이 시험이 띄운 양수 pid에만 신호(pid 0·음수는 무리 전체 — B16: 자식 조회 실패 뒤 0이 들어가 시험 무리가 TERM으로 죽었다)
	sig := func(p int, s syscall.Signal) {
		if p > 0 {
			syscall.Kill(p, s)
		}
	}
	t.Cleanup(func() {
		for _, p := range pids {
			sig(p, syscall.SIGKILL)
		}
	})
	pid := func(n string) int { return pids[n] }

	out, rc = run(T, e, SWEEP, "--self-test")
	c.ok("self-test", strings.TrimSpace(out) == "ok" && rc == 0)

	// 가짜 hc: ls는 $T/ls.json, get <주소>는 $T/get-<주소>.json(없으면 실패)
	script(t, T+"/bin/hc", T, `#!/bin/bash
case $1 in
  ls) exec cat "@T@/ls.json" ;;
  get) [ -f "@T@/get-$2.json" ] && exec cat "@T@/get-$2.json" ;;
esac
exit 1
`)
	e = e.With("PATH="+T+"/bin:"+e["PATH"], "HOME="+T+"/home", "HERDR_TOOLS_CONFIG="+T+"/cfg",
		"XDG_CACHE_HOME="+T+"/cache", "TMPDIR="+T+"/tmp")
	sweepEnv := func(env Env, args ...string) {
		out, rc = run(T, env, SWEEP, append([]string{"--only-under", T}, args...)...)
	}
	sweep := func(args ...string) { sweepEnv(e, args...) }
	// 디스크 쓸기는 끈다(hcsweepdisk_test가 본다): 기본 문턱 10GB면 여유가 15GB 아래인 기계에서 진짜 GOCACHE(병합기가 물려줌)를
	// 줄이고 hc-disk-low 줄이 events.tsv에 더해져, 결과가 그 기계 디스크에 따라 바뀐다
	cfg := func(ls ...string) {
		write(t, T+"/cfg/config", strings.Join(append([]string{"sweep.disk_free_gb 0"}, ls...), "\n")+"\n")
	}
	cfg()
	LOG := T + "/home/.claude-tasks/closed.log"
	has := func(s string) bool { return strings.Contains(out, s) }

	// 색인 쓸기: 원본(root_path)이 모두 사라진 codebase-memory 색인 DB만 지운다. 고아·터미널이 없는 상태에서 먼저 본다
	IX := T + "/cache/codebase-memory-mcp"
	mkdirs(t, IX+"/logs", T+"/repo-live")
	mkdb := func(n string, roots ...string) {
		bash(t, T, e, `exec sqlite3 "$1" "$2" >/dev/null`, IX+"/"+n+".db", mkdbSQL(false, "2026-11-12T07:51:04Z", roots...))
	}
	mkdb("live", T+"/repo-live")
	mkdb("gone", T+"/repo.wt/audit-x")
	write(t, IX+"/gone.db-wal", "w")
	write(t, IX+"/gone.db-shm", "s")
	mkdb("gone2", T+"/repo.wt/audit-y")
	write(t, IX+"/gone2.db-journal", "") // 실제 색인은 롤백 저널 방식
	if err := os.Symlink("/Volumes/hcsweep-test-없음", T+"/usblink"); err != nil {
		t.Fatal(err)
	}
	mkdb("vlink", T+"/usblink/repo") // 링크를 거쳐 /Volumes(빠진 디스크) — 남김
	bash(t, T, e, `exec sqlite3 "$1" "$2" >/dev/null`, IX+"/walgone.db", mkdbSQL(true, "2026", T+"/repo.wt/audit-w"))
	mkdb("bad", T+"/repo.wt/audit-bad")
	mkdirs(t, IX+"/bad.db-shm") // 곁 파일을 못 지움(폴더 — 롤백 방식이라 SQLite는 안 봄) — 이 DB만 실패하고 나머지는 계속
	mkdb("recent", T+"/repo.wt/new")
	mkdb("vol", "/Volumes/hcsweep-test-없음/repo")
	mkdb("empty", "")
	mkdb("mixed", T+"/repo-live", T+"/repo.wt/audit-z")
	mkdb("_config", T+"/repo.wt/cfg")
	write(t, IX+"/broken.db", "not a db")
	write(t, IX+"/logs/a.log", "x")
	old2026 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	ents, _ := os.ReadDir(IX)
	for _, en := range ents {
		if en.Name() != "recent.db" {
			os.Chtimes(IX+"/"+en.Name(), old2026, old2026)
		}
	}
	ix := func(n string) bool { return exists(IX + "/" + n) }
	write(t, T+"/ls.json", `{"terminals":[]}`+"\n")
	sweep("--dry-run")
	c.ok("색인 dry-run: 원본 없는 gone·gone2·walgone·bad만 찍고 안 지움", func() bool {
		for _, n := range []string{"gone", "gone2", "walgone", "bad"} {
			if !has("지울 색인: " + n + " ") {
				return false
			}
		}
		return rc == 0 && grepc(out, "지울 색인:") == 4 && ix("gone.db") && ix("gone.db-wal")
	}())
	os.Rename(T+"/ls.json", T+"/ls.bak")
	sweep()
	c.ok("색인: hc ls가 실패해도 색인 쓸기는 돎(gone·gone2·walgone 지움, -wal·-shm·-journal 포함)",
		rc != 0 && !ix("gone.db") && !ix("gone.db-wal") && !ix("gone.db-shm") && !ix("gone2.db") && !ix("gone2.db-journal") && !ix("walgone.db"))
	c.ok("색인: WAL DB를 열며 새로 생긴 -wal·-shm도 지움", func() bool {
		ents, _ := os.ReadDir(IX)
		for _, en := range ents {
			if strings.HasPrefix(en.Name(), "walgone") {
				return false
			}
		}
		return true
	}())
	c.ok("색인: 한 DB의 오류(bad)는 그 DB만 실패로 찍고 나머지는 지움", has("색인 쓸기 실패: bad") && ix("bad.db"))
	os.Rename(T+"/ls.bak", T+"/ls.json")
	c.ok("색인: 원본 있음·최근·/Volumes·링크로 /Volumes·빈 root·원본 하나라도 있음·깨진 파일·_config.db·logs는 남김",
		ix("live.db") && ix("recent.db") && ix("vol.db") && ix("vlink.db") && ix("empty.db") && ix("mixed.db") && ix("broken.db") && ix("_config.db") && ix("logs/a.log"))
	log := read(LOG)
	c.ok("색인: closed.log에 이름·root·크기 한 줄씩",
		grepc(log, "\t색인:gone\t.*색인 지움\\(원본 없음\\)\t"+q(T+"/repo.wt/audit-x")+"\t-\t크기 ") == 1 &&
			strings.Contains(log, "\t색인:gone2\t") && !strings.Contains(log, "\t색인:bad\t"))
	c.ok("색인: 요약 한 줄", has("색인 쓸기: 지움 3개"))
	os.Remove(IX + "/bad.db-shm")
	os.Remove(IX + "/bad.db")
	sweep()
	c.ok("색인: 다시 돌면 지울 것 없음 — 0개면 요약 줄도 안 찍음, 링크로 /Volumes(vlink)는 bad 없는 바퀴에서도 남음",
		rc == 0 && !has("색인 쓸기") && ix("live.db") && ix("vlink.db"))

	// orphan <이름> <cwd> <명령…>: 부모가 1인 프로세스를 띄워 pid를 pids[<이름>]에
	orphan := func(n, d string, argv ...string) {
		s := bash(t, T, e, `d=$1; shift; cd "$d" || exit 1; "$@" </dev/null >/dev/null 2>&1 & echo $!`, append([]string{d}, argv...)...)
		p, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			t.Fatalf("orphan %s: %q", n, s)
		}
		pids[n] = p
		for i := 0; i < 10; i++ {
			if o, _ := exec.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(p)).Output(); strings.TrimSpace(string(o)) == "1" {
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Errorf("준비 실패: %s 부모가 1이 아님", n)
	}
	// idle <작업 트리>: 그 아래 bin/idle(자식 sleep 하나를 두고 잠듦)
	idle := func(d string) {
		mkdirs(t, d+"/tests")
		script(t, d+"/bin/idle", T, "#!/bin/bash\nsleep 300\n")
	}
	kid := func(n string) {
		for i := 0; i < 10; i++ {
			o, _ := exec.Command("pgrep", "-P", strconv.Itoa(pids[n])).Output()
			if k, err := strconv.Atoi(strings.TrimSpace(strings.SplitN(string(o), "\n", 2)[0])); err == nil && k > 0 {
				pids[n+"-kid"] = k
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Fatalf("준비 실패: %s(pid %d)의 자식을 못 찾음(pgrep)", n, pids[n]) // 0을 남기면 아래 신호가 무리 전체로 간다(B16)
	}

	W := T + "/app.wt"
	A, A2, B, C, E, K, H, O, S := W+"/a", W+"/a2", W+"/b", W+"/c", W+"/e", W+"/k", T+"/main.wt/h", T+"/agents/x", W+"/s"
	mkdirs(t, A, A2, O, S, T+"/main", T+"/elsewhere", T+"/x y")
	os.Symlink("/bin/sleep", T+"/x y/herdr-core") // 공백 든 실행 경로(맥 코어 꼴 …/Herdr Core.app/Contents/MacOS/herdr-core). 복사본은 서명에 죽는다
	os.Symlink(T, T+"/link")
	for _, d := range []string{B, C, E, K, H} {
		idle(d)
	}
	orphan("a", A, "sleep", "300")             // cwd로만 걸림, 작업 트리 있음 → 찍기만(보류)
	orphan("a2", A2, "sleep", "300")           // cwd로만 걸림, 작업 트리 지움 → 끔
	orphan("b", T, B+"/bin/idle")              // 명령 줄, 터미널이 링크 경로($T/link/…)로 b를 씀 → 남김
	orphan("c", T, C+"/tests/../bin/idle")     // 명령 줄, 작업 트리 지움 → 끔(자식 sleep도)
	orphan("e", T, E+"/bin/idle")              // 명령 줄, 작업 트리 있음, 쓰는 터미널 없음 → 끔
	orphan("h", T, H+"/bin/idle")              // 명령 줄, 본 저장소 $T/main을 터미널이 씀 → 보류, 작업 트리 지우면 끔
	orphan("k", T, K+"/bin/idle")              // 명령 줄, 작업 트리 지움, 자손이 live 터미널 프로세스 → 보류
	orphan("o", O, "sleep", "300")             // 작업 트리 밖(launchd 작업 흉내) → 남김
	orphan("s", S, T+"/x y/herdr-core", "300") // 공백 든 경로의 keep(herdr-core), cwd로만, 작업 트리 지움 → 남김(H56)
	kid("b")
	kid("c")
	kid("k")
	for _, d := range []string{A2, C, K, S} {
		os.RemoveAll(d)
	}
	write(t, T+"/ls.json", fmt.Sprintf(`{"terminals":[{"address":"t1","state":"live","cwd":"%[1]s/link/app.wt/b"},{"address":"t2","state":"finished","cwd":"%[2]s"},
 {"address":"t4","state":"live","cwd":"%[1]s/main/sub"},{"address":"t5","state":"live","cwd":"%[1]s/elsewhere"},
 {"address":"box/t3","state":"live","cwd":"%[1]s/app.wt/a-other"}]}
`, T, A))
	write(t, T+"/get-t1.json", `{"address":"t1","pid":1}`+"\n")
	write(t, T+"/get-t4.json", `{"address":"t4","pid":1}`+"\n")
	write(t, T+"/get-t5.json", fmt.Sprintf(`{"address":"t5","pid":%d}`+"\n", pid("k-kid")))
	hasp := func(prefix, n string) bool { return has(fmt.Sprintf("%s pid %d ", prefix, pid(n))) }
	none := func() bool { return !grep(out, fmt.Sprintf("(끌|끈) 고아: pid (%d|%d) ", pid("b"), pid("o"))) }
	allalive := func(ns ...string) bool {
		for _, n := range ns {
			if !alive(pid(n)) {
				return false
			}
		}
		return true
	}
	ALL := []string{"a", "a2", "b", "c", "e", "h", "k", "o", "s"}

	cfg("sweep.orphan_kill on")
	sweep("--dry-run", "--min-age", "0")
	c.ok("dry-run: 끌 것(a2·c·e)만 「끌 고아」, 안 끔", rc == 0 && hasp("끌 고아:", "a2") && hasp("끌 고아:", "c") && hasp("끌 고아:", "e") && allalive(ALL...))
	c.ok("dry-run: cwd로만·작업 트리 있음은 보류", grep(out, fmt.Sprintf("보류 고아: pid %d .*cwd로만", pid("a"))))
	c.ok("dry-run: 본 저장소 터미널이 있으면 보류", grep(out, fmt.Sprintf("보류 고아: pid %d .*본 저장소 터미널 있음", pid("h"))))
	c.ok("dry-run: 자손에 live 터미널 프로세스가 있으면 보류", grep(out, fmt.Sprintf("보류 고아: pid %d .*자손에 터미널", pid("k"))))
	c.ok("dry-run: 링크 경로로 쓰는 터미널·작업 트리 밖은 목록에 없음", none() && !has(fmt.Sprintf("pid %d ", pid("b"))))
	c.ok("dry-run: 공백 든 실행 경로라도 keep(herdr-core)은 목록에 없음", !has(fmt.Sprintf("pid %d ", pid("s"))))

	sweep()
	c.ok("기본 최소 나이(10분): 막 생긴 고아는 안 끔", rc == 0 && allalive(ALL...) && !has("고아: pid"))

	cfg("sweep.wt_suffix .lanes", "sweep.orphan_kill on")
	sweep("--dry-run", "--min-age", "0")
	c.ok("설정 sweep.wt_suffix: 다른 접미사면 .wt 아래는 안 봄", rc == 0 && !has(fmt.Sprintf("pid %d ", pid("a2"))))
	cfg("sweep.orphan_kill on")

	os.Rename(T+"/ls.json", T+"/ls.bak")
	sweep("--min-age", "0")
	c.ok("hc ls 실패: 아무것도 안 끄고 실패", rc != 0 && allalive(ALL...))
	write(t, T+"/ls.json", "{}\n")
	sweep("--min-age", "0")
	c.ok("hc ls에 terminals 목록이 없음: 고아 쓸기 건너뜀, 안 끔", has("고아 쓸기 건너뜀") && allalive(ALL...))
	os.Rename(T+"/ls.bak", T+"/ls.json")

	cfg() // 끄기 키 없음
	sweep("--min-age", "0")
	sweep("--min-age", "0")
	c.ok("끄기 설정 없음(기본 끔): 찍기만, 안 끔", grep(out, fmt.Sprintf("끄기 꺼짐.*pid %d ", pid("e"))) && allalive(ALL...))
	c.ok("끄기 꺼짐: closed.log에 한 번만 기록", grepc(read(LOG), fmt.Sprintf("pid:%d\t.*관찰", pid("e"))) == 1)
	cfg("sweep.orphan_kill yes")
	sweep("--min-age", "0")
	c.ok("모르는 끄기 값: 끄지 않음", allalive(ALL...))

	cfg("sweep.orphan_kill on")
	sweep("--min-age", "0")
	c.ok("작업 트리 지움(cwd로만) a2: 끔", gone(pid("a2")))
	c.ok("지워진 작업 트리 c: 끔, 자식 sleep도", gone(pid("c")) && gone(pid("c-kid")))
	c.ok("작업 트리 있고 쓰는 터미널 없음 e: 끔", gone(pid("e")))
	c.ok("남김: cwd로만 a, 링크 경로 터미널 b, 본 저장소 h, 자손 터미널 k, 밖 o, 공백 경로 keep s", allalive("a", "b", "h", "k", "o", "s"))
	log = read(LOG)
	// [E05] 명령 줄(인자 — 자격증명이 섞일 수 있다)을 남기지 않고 실행 파일만
	c.ok("기록: pid·나이·실행 파일",
		grep(log, fmt.Sprintf("(?m)pid:%d\t.*고아\\(쓰는 터미널 없음\\)\t%s\t-\t나이 0분 /bin/bash$", pid("e"), q(E))) &&
			grep(log, fmt.Sprintf("pid:%d\t.*작업 트리 지워짐", pid("c"))) && !strings.Contains(log, E+"/bin/idle"))

	os.RemoveAll(H)
	sweep("--min-age", "0")
	c.ok("본 저장소 터미널이 있어도 작업 트리를 지우면 끔", gone(pid("h")))

	// quiet-alerts 3: 닫은 레인 터미널(라벨 hc-lane=1)은 닫기 전에 events.tsv에 「정리됨」 한 줄(hc-lane wait는 안 알리고 events에 보임)
	e = e.Without("XDG_STATE_HOME", "HC_LANE_MACHINE")
	old := (time.Now().Unix() - 7*3600) * 1000
	write(t, T+"/ls.json", `{"terminals":[{"address":"t61","state":"live","status":"idle"},{"address":"t62","state":"live","status":"done"}]}`+"\n")
	for _, a := range []string{"t61", "t62"} {
		lab := `"labels":{"hc-lane":"1"},`
		if a == "t62" {
			lab = ""
		}
		write(t, T+"/get-"+a+".json", fmt.Sprintf(`{"address":"%s","name":"lane-%s","state":"live","status":{"app":"claude-code","state":"idle"},%s"parent":"p/t1","created_ms":%d,"status_ms":%d,"turn":4,"cwd":"%s"}`+"\n",
			a, a, lab, old, old, T+"/gone-"+a))
	}
	script(t, T+"/bin/hc", T, `#!/bin/bash
case $1 in
  ls) exec cat "@T@/ls.json" ;;
  get) [ -e "@T@/killed-$2" ] && { echo "{\"address\":\"$2\",\"state\":\"finished\"}"; exit 0; }
       [ -f "@T@/get-$2.json" ] && exec cat "@T@/get-$2.json" ;;
  kill) [ -s "@T@/home/.local/state/hc-lane/events.tsv" ] && cp "@T@/home/.local/state/hc-lane/events.tsv" "@T@/ev-at-kill"; touch "@T@/killed-$2"; exit 0 ;;
  rm) exit 0 ;;
esac
exit 1
`)
	EVT := T + "/home/.local/state/hc-lane/events.tsv"
	sweep("--min-age", "0")
	c.ok("정리됨: 두 터미널 다 닫음(R3)", has("닫힘: R3 t61") && has("닫힘: R3 t62"))
	ev := read(EVT)
	c.ok("정리됨: 레인 터미널만 events.tsv에 한 줄(8칸: 시각·기계·주소·레인·정리됨·-·까닭·turn)", func() bool {
		if lines(ev) != 1 {
			return false
		}
		f := strings.Split(strings.TrimRight(ev, "\n"), "\t")
		return len(f) == 8 && f[2] == "t61" && f[3] == "lane-t61" && f[4] == "정리됨" && f[5] == "-" && strings.Contains(f[6], "hc-sweep R3") && f[7] == "4"
	}())
	c.ok("정리됨: kill 전에 적음(그 사이 wait가 끝남을 봐도 뜻밖의 종료로 안 알리게)", strings.Contains(read(T+"/ev-at-kill"), "정리됨"))

	sig(pid("b-kid"), syscall.SIGTERM)
	sig(pid("k-kid"), syscall.SIGTERM)
	// H110: 정리됨을 못 쓰면(events.tsv 자리 못 만듦) 레인 터미널은 닫지 않는다 — 닫으면 hc-lane wait가 reason killed로 보고
	// 조용히 넘겨 뜻밖의 종료 알림이 사라진다. 레인 아닌 터미널은 그대로 닫는다
	rmKilled := func() {
		ms, _ := filepath.Glob(T + "/killed-*")
		for _, m := range ms {
			os.Remove(m)
		}
		os.Remove(EVT)
	}
	rmKilled()
	write(t, T+"/notdir", "x")
	sweepEnv(e.With("XDG_STATE_HOME="+T+"/notdir"), "--min-age", "0")
	c.ok("정리됨 못 씀: 레인 터미널은 kill 안 함", !exists(T+"/killed-t61") && has("실패(정리됨 못 씀): R3 t61"))
	c.ok("정리됨 못 씀: 레인 아닌 터미널은 닫음", exists(T+"/killed-t62") && has("닫힘: R3 t62"))
	c.ok("정리됨 못 씀: closed.log에 남김", grep(read(LOG), "\tt61\t.*실패\\(정리됨 못 씀\\)"))

	// 2026-11-21 fill-submit-release: 마지막 hc-lane 사건이 「제출」(병합기 큐에 넣고 끝남)인 레인은 쉰 지 30분을 기다리지 않고 R2로 닫는다.
	// 옛 판은 30분(R2)·6시간(R3)을 기다렸다. 같은 주소의 다른 이름·다른 실행 키(옛 실행)의 제출, 제출 뒤 다른 사건이 오면 그대로 기다린다
	rmKilled()
	recent := (time.Now().Unix() - 60) * 1000
	write(t, T+"/ls.json", `{"terminals":[{"address":"t71","state":"live","status":"idle"},{"address":"t72","state":"live","status":"idle"},{"address":"t73","state":"live","status":"idle"},{"address":"t74","state":"live","status":"idle"}]}`+"\n")
	for _, a := range []string{"t71", "t72", "t73", "t74"} {
		write(t, T+"/get-"+a+".json", fmt.Sprintf(`{"address":"%s","name":"lane-%s","state":"live","status":{"app":"claude-code","state":"idle"},"labels":{"hc-lane":"1"},"created_ms":1700000000000,"status_ms":%d,"turn":3,"cwd":"%s"}`+"\n",
			a, a, recent, T+"/gone-"+a))
	}
	write(t, EVT, "2026-11-21T10:00:00Z\tm\tt71\tlane-t71\t제출\tabc1234\tsub-1 → 큐\t3\t-\tt71@1700000000000\n"+
		"2026-11-21T10:00:00Z\tm\tt72\tlane-zz\t제출\tabc1234\tsub-2 → 큐\t3\n"+
		"2026-11-21T10:00:00Z\tm\tt73\tlane-t73\t제출\tabc1234\tsub-3 → 큐\t3\t-\tt73@1600000000000\n"+
		"2026-11-21T10:00:00Z\tm\tt74\tlane-t74\t제출\tabc1234\tsub-4 → 큐\t3\n2026-11-21T10:01:00Z\tm\tt74\tlane-t74\t중간\t-\t다시 일함\t4\n")
	script(t, T+"/bin/hc", T, `#!/bin/bash
case $1 in
  ls) exec cat "@T@/ls.json" ;;
  get) [ -e "@T@/killed-$2" ] && { echo "{\"address\":\"$2\",\"state\":\"finished\"}"; exit 0; }
       [ -f "@T@/get-$2.json" ] && exec cat "@T@/get-$2.json" ;;
  exec) echo '{"error":{"message":"cannot start folder: No such file or directory"}}' >&2; exit 1 ;;
  read) echo "  ⏵⏵ bypass permissions on (shift+tab to cycle)"; exit 0 ;;
  kill) touch "@T@/killed-$2"; exit 0 ;;
  rm) exit 0 ;;
esac
exit 1
`)
	sweep("--min-age", "0")
	c.ok("제출 뒤 쉰 지 1분 레인: R2로 바로 닫음", exists(T+"/killed-t71") && has("닫힘: R2 t71"))
	c.ok("제출 뒤: 다른 이름·옛 실행의 제출, 제출 뒤 중간 사건이면 안 닫음",
		!exists(T+"/killed-t72") && !exists(T+"/killed-t73") && !exists(T+"/killed-t74"))

	// 교착 풀기(2026-11-21 mini-m6-1 land-metrics 16시간): hc-sweep은 남은 작업 트리 때문에 터미널을, hc-lane gc는 그 폴더를 쓰는
	// 터미널 때문에 작업 트리를 남겨 서로 기다렸다. 착지했고(cherry + 없음 + 착지 증거) 깨끗한 작업 트리의 done 레인은 한 바퀴에
	// 터미널을 닫고 hc-lane clean으로 작업 트리·가지를 지운다. 더러움·착지 증거 없음·착지 전 커밋은 남기고 --dry-run에 까닭
	e = e.With("GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_NOSYSTEM=1")
	G, R := T+"/lg", T+"/lg/repo"
	mkdirs(t, G)
	bash(t, T, e, `G=$1 R=$2
git init -q --bare -b main "$G/origin.git" && git clone -q "$G/origin.git" "$R" 2>/dev/null &&
  git -C "$R" commit -q --allow-empty -m base && git -C "$R" push -q origin HEAD:main || exit 1
for b in land-x land-d land-n land-u; do git -C "$R" worktree add -q -b "$b" "$R.wt/$b" origin/main 2>/dev/null || exit 1; done
(cd "$R.wt/land-x" && echo a >a && git add a && git commit -q -m x && git push -q origin HEAD:main) || exit 1
(cd "$R.wt/land-d" && git merge -q --ff-only origin/main && echo d >dirty) || exit 1
(cd "$R.wt/land-u" && echo u >u && git add u && git commit -q -m u) || exit 1
git -C "$R" fetch -q origin`, G, R)
	rmKilled()
	write(t, EVT, "2026-11-21T10:00:00Z\tm\tt81\tland-x\t제출\tabc1234\tsub → 큐\t3\n"+
		"2026-11-21T10:00:00Z\tm\tt82\tland-d\t착지\tabc1234\t-\t3\n2026-11-21T10:00:00Z\tm\tt84\tland-u\t완료\tabc1234\t-\t3\n")
	idle31 := (time.Now().Unix() - 31*60) * 1000
	for _, x := range [][2]string{{"t81", "land-x"}, {"t82", "land-d"}, {"t83", "land-n"}, {"t84", "land-u"}} {
		write(t, T+"/get-"+x[0]+".json", fmt.Sprintf(`{"address":"%s","name":"%s","state":"live","status":{"app":"claude-code","state":"done"},"labels":{"hc-lane":"1"},"created_ms":1700000000000,"status_ms":%d,"turn":5,"cwd":"%s"}`+"\n",
			x[0], x[1], idle31, R+".wt/"+x[1]))
	}
	script(t, T+"/bin/hc", T, `#!/bin/bash
case $1 in
  ls) printf '{"terminals":['; s=
      for a in t81 t82 t83 t84; do [ -e "@T@/killed-$a" ] && continue
        printf '%s{"address":"%s","state":"live","status":"done","cwd":"%s"}' "$s" "$a" "$(sed -n 's/.*"cwd":"\([^"]*\)".*/\1/p' "@T@/get-$a.json")"; s=,; done
      echo ']}' ;;
  get) [ -e "@T@/killed-$2" ] && { echo "{\"address\":\"$2\",\"state\":\"finished\"}"; exit 0; }
       [ -f "@T@/get-$2.json" ] && exec cat "@T@/get-$2.json"; exit 1 ;;
  exec) shift; d=
        while [ "$1" != -- ]; do case $1 in --cwd) d=$2; shift 2 ;; --env) export "$2"; shift 2 ;; --timeout) shift 2 ;; *) shift ;; esac; done
        shift; [ -z "$d" ] || cd "$d" || exit 1; exec "$@" ;;
  read) echo "  ⏵⏵ bypass permissions on (shift+tab to cycle)" ;;
  kill) touch "@T@/killed-$2" ;;
  rm) ;;
  *) exit 1 ;;
esac
`)
	sweep("--dry-run", "--min-age", "0")
	c.ok("교착 dry-run: 착지·깨끗한 레인은 닫을 것 + clean 예고", grep(out, `닫을 것: R2 t81 land-x .*\(닫은 뒤 hc-lane clean land-x\)`))
	c.ok("교착 dry-run: 남김 까닭 — 더러움·증거 없음·착지 전", has("남김: t82 land-d — 작업 트리 남음 — 커밋 안 된 변경") &&
		has("남김: t83 land-n — 작업 트리 남음 — 착지 증거 없음") && has("남김: t84 land-u — 작업 트리 남음 — 착지 전 커밋"))
	sweep("--min-age", "0")
	_, ref := run(T, e, "git", "-C", R, "show-ref", "-q", "--verify", "refs/heads/land-x")
	// B26: 부재는 show-ref 종료 1만(128 = 저장소를 못 엶은 실패)
	c.ok("교착: 착지·깨끗한 레인은 한 바퀴에 터미널 닫고 작업 트리·가지 지움", exists(T+"/killed-t81") && !exists(R+".wt/land-x") && ref == 1)
	c.ok("교착: closed.log에 정리 한 줄", strings.Contains(read(LOG), "\tt81\tland-x\t작업 트리 정리(hc-lane clean land-x)\t"+R+"\t"))
	c.ok("교착: 더러움·증거 없음·착지 전은 터미널·작업 트리 그대로", !exists(T+"/killed-t82") && !exists(T+"/killed-t83") && !exists(T+"/killed-t84") &&
		exists(R+".wt/land-d") && exists(R+".wt/land-n") && exists(R+".wt/land-u"))

	for _, n := range []string{"a", "b", "k", "o", "s"} {
		sig(pid(n), syscall.SIGTERM)
	}
	c.ok("이 시험의 고아 0", func() bool {
		for _, p := range pids {
			if !gone(p) {
				return false
			}
		}
		return true
	}())
}
