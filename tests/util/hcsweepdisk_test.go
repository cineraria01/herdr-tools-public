package util_test

// 옛 tests/hc-sweep-disk.test.sh: hc-sweep 디스크 쓸기. 가짜 $TMPDIR에 오래된·새·열린·다른 이름·링크·쓰기 막힌 항목을 두고
// bin/hc-sweep 한 바퀴(가짜 hc — 터미널 없음)가 정리 대상만 지우는지, 디스크 부족 알림(hc-lane report → 시험 HOME의
// events.tsv)이 같은 상태에서 한 번만 나가고 회복 뒤 다시 나가는지 본다.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestHcSweepDisk(t *testing.T) { eachImpl(t, "hc-sweep", testHcSweepDisk) }

func testHcSweepDisk(t *testing.T, T string, e Env) {
	SWEEP := filepath.Join(repo, "bin", "hc-sweep")
	var out string
	c := checker{t: t, out: &out}
	mkdirs(t, T+"/bin", T+"/badbin", T+"/home", T+"/cfg", T+"/tmp", T+"/outside/keep")
	script(t, T+"/bin/hc", T, "#!/bin/sh\n[ \"$1\" = ls ] && { echo '{\"terminals\": []}'; exit 0; }\nexit 1\n")
	script(t, T+"/badbin/lsof", T, "#!/bin/sh\nexit 1\n")
	// 「go가 도는가」(ps -x -o comm=)는 가짜 목록 @T@/ps-comm으로 — 실제 ps면 검증이 함께 돌리는 다른 꾸러미의 go test가 보여 결과가 바뀐다.
	// 다른 ps 부름(세션 찾기 등)은 진짜로
	script(t, T+"/bin/ps", T, "#!/bin/sh\n[ \"$*\" = \"-x -o comm=\" ] && exec cat @T@/ps-comm\nexec /bin/ps \"$@\"\n")
	psComm := func(goRunning bool) {
		l := "/bin/sh\nhc-sweep\nps\n"
		if goRunning {
			l += "/opt/homebrew/bin/go\n"
		}
		write(t, T+"/ps-comm", l)
	}
	psComm(false)
	G, OLD := T+"/gocache", T+"/home/Library/Caches/go-build"
	if e[implVar("hc-sweep")] == "go" { // Go 판 바이너리는 바깥 GOCACHE로 미리 짓는다(아래 GOCACHE를 시험 폴더로 바꾸면 빈 캐시로 다시 지을 수 있다)
		if o, rc := run(T, e, SWEEP, "--version"); rc != 0 {
			t.Fatalf("hc-sweep --version: %d %s", rc, o)
		}
	}
	// GOCACHE도 시험 폴더로: 병합기는 GOCACHE=~/.cache/go-build를 물려준다 — 그대로면 문턱을 크게 둔 바퀴가 진짜 Go 캐시를 훑고 줄인다
	e = e.With("PATH="+T+"/bin:"+e["PATH"], "HOME="+T+"/home", "HERDR_TOOLS_CONFIG="+T+"/cfg", "TMPDIR="+T+"/tmp", "GOCACHE="+G, "HC_SWEEP_GO_COMM=")
	cfg := func(ls ...string) { write(t, T+"/cfg/config", strings.Join(ls, "\n")+"\n") }
	sweepEnv := func(env Env, args ...string) {
		out, _ = run(T, env, SWEEP, append([]string{"--only-under", T}, args...)...)
	}
	sweep := func(args ...string) { sweepEnv(e, args...) }
	EV := T + "/home/.local/state/hc-lane/events.tsv"
	LOG := T + "/home/.claude-tasks/closed.log"
	old := time.Now().Add(-5 * time.Hour).Format("200601021504.05")
	age := func(ps ...string) { // find … -exec touch -h: 링크는 링크 자체, 폴더는 안쪽까지
		bash(t, T, e, `o=$1; shift; find "$@" -exec touch -h -t "$o" {} +`, append([]string{old}, ps...)...)
	}

	X := T + "/tmp"
	for _, d := range []string{"go-build111/b001", "TestFoo222/001", "TestRO333/001/mod", "go-buildnew/b001",
		"TestOpen444/001", "other-old", "TestMixed555/001", "node-compile-cache"} {
		write(t, X+"/"+d+"/f", strings.Repeat("\x00", 4096))
	}
	write(t, X+"/TestFile.txt", "x")
	os.Symlink(T+"/outside/keep", X+"/go-buildlink")
	age(X+"/go-build111", X+"/TestFoo222", X+"/TestRO333", X+"/TestOpen444", X+"/other-old", X+"/TestMixed555",
		X+"/node-compile-cache", X+"/TestFile.txt", X+"/go-buildlink", T+"/outside")
	for _, d := range []string{"TestRO333/001/mod", "TestRO333/001", "TestOpen444/001"} { // go 모듈 캐시 꼴(쓰기 막힘)
		os.Chmod(X+"/"+d, 0o555)
	}
	now := time.Now()
	os.Chtimes(X+"/TestMixed555/001/f", now, now) // 안쪽 하나만 새로움
	sl := exec.Command("sleep", "120")
	sl.Dir = X + "/TestOpen444/001"
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sl.Process.Kill(); sl.Wait() })
	time.Sleep(300 * time.Millisecond)
	age(X+"/TestOpen444", X+"/TestRO333") // 위 chmod·cd가 바꾼 시각을 다시 오래되게

	// lsof 실패면 아무것도 안 지움
	cfg("sweep.disk_free_gb 0")
	sweepEnv(e.With("PATH=" + T + "/badbin:" + e["PATH"]))
	c.ok("lsof 실패면 건너뜀", exists(X+"/go-build111") && exists(X+"/TestFoo222") && strings.Contains(out, "lsof 실패"))

	// 끔(sweep.tmp_hours 0), 숫자 아닌 값은 그 쓸기만 끔(알림은 돈다)
	cfg("sweep.tmp_hours 0", "sweep.disk_free_gb 0")
	sweep()
	c.ok("tmp_hours 0이면 끔", exists(X+"/go-build111"))
	cfg("sweep.tmp_hours 3h", "sweep.disk_free_gb 1000000000")
	sweep("--dry-run")
	c.ok("숫자 아닌 tmp_hours는 그 쓸기만 끄고 알림은 돈다",
		exists(X+"/go-build111") && ordered(out, "sweep.tmp_hours 값이 숫자가 아님", "디스크 부족 알림 (보낼 것)"))

	// dry-run은 찍기만
	cfg("sweep.disk_free_gb 0")
	sweep("--dry-run")
	c.ok("dry-run은 찍기만", exists(X+"/go-build111") && strings.Contains(out, "지울 임시: "+X+"/go-build111"))

	sweep()
	allGone := true
	for _, d := range []string{"go-build111", "TestFoo222", "TestRO333"} {
		if _, err := os.Lstat(X + "/" + d); err == nil {
			allGone = false
			t.Errorf("FAIL - 안 지움 %s", d)
		}
	}
	c.ok("오래되고 안 열린 go-build*·Test*(쓰기 막힌 것 포함) 지움", allGone)
	kept := true
	for _, d := range []string{"go-buildnew", "TestOpen444", "other-old", "TestMixed555", "node-compile-cache", "TestFile.txt", "go-buildlink", "outside/keep"} {
		p := X + "/" + d
		if d == "outside/keep" {
			p = T + "/outside/keep"
		}
		if !exists(p) {
			kept = false
			t.Errorf("FAIL - 지우면 안 되는 것을 지움 %s", d)
		}
	}
	c.ok("새·열린·안쪽이 새·다른 이름·파일·링크(와 그 대상)는 남김", kept)
	fi, err := os.Stat(X + "/TestOpen444/001")
	c.ok("남긴 열린 폴더의 권한은 그대로", err == nil && fi.Mode().Perm() == 0o555)
	c.ok("closed.log에 한 줄(3개)", grep(read(LOG), `임시 찌꺼기 지움\(3.0시간 넘음\).*3개`))
	c.ok("문턱 0이면 알림 없음", read(EV) == "")

	// 디스크 부족 알림: 문턱을 아주 크게 → 부족. 두 바퀴에도 한 번만, dry-run은 안 보냄
	cfg("sweep.disk_free_gb 1000000000")
	sweep("--dry-run")
	c.ok("dry-run은 알림을 찍기만", read(EV) == "" && strings.Contains(out, "보낼 것"))
	sweep()
	sweep()
	ev := read(EV)
	c.ok("같은 상태에서 한 번만 알림", grepc(ev, "hc-disk-low") == 1)
	line := ""
	for _, l := range strings.Split(ev, "\n") {
		if strings.Contains(l, "hc-disk-low") {
			line = l
		}
	}
	c.ok("알림 줄: 중간·여유·사용률·세 곳 크기", ordered(line, "\t중간\t", "디스크 여유", "GB(사용 ", "%) < 문턱 1000000000",
		"~/Library/Caches/go-build 없음", "~/.cache/go-build 없음"))
	// Go 캐시를 「안 쓰는 옛 캐시」로 짐작하지 않는다(disk-low-machine — mini-m4에서 지운 뒤 다시 찼다)
	c.ok("Go 캐시 문구는 사실대로", !strings.Contains(line, "옛 캐시") && strings.Contains(line, "Go 캐시는 쓰는 곳이 없을 때만 손으로"))
	// 기계 칸(둘째)은 이 기계 이름(HC_LANE_MACHINE, 없으면 hostname) — watch가 터미널 없는 사건 끝 괄호에 이 칸을 보인다
	mach := e["HC_LANE_MACHINE"]
	if mach == "" {
		h, _ := exec.Command("hostname", "-s").Output()
		mach = strings.TrimSpace(string(h))
	}
	f := strings.Split(line, "\t")
	c.ok("알림 사건의 기계 칸 = 이 기계", len(f) > 1 && f[1] == mach)
	// 회복(여유 ≥ 문턱의 1.5배) 뒤 다시 부족이면 또 한 번
	cfg("sweep.disk_free_gb 0.000001")
	sweep()
	// 알림 키는 초 단위 시각(hc-disk-low:<시각>) — 첫 알림과 같은 초면 hc-lane이 같은 키로 거른다. 바퀴는 몇 분 간격이라 실제로는 안 겹친다
	time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second)))
	cfg("sweep.disk_free_gb 1000000000")
	sweep()
	c.ok("회복 뒤 다시 부족이면 다시 알림", grepc(read(EV), "hc-disk-low") == 2)

	// launchd 작업과 같은 PATH(/usr/sbin 없음)에서도 lsof를 찾는다(감사 중간 1): 오래된 폴더는 지우고 열린 폴더는 남긴다
	cfg("sweep.disk_free_gb 0")
	mkdirs(t, X+"/go-buildlaunchd/b001")
	age(X + "/go-buildlaunchd")
	sweepEnv(e.With("PATH=" + T + "/bin:/usr/bin:/bin"))
	c.ok("launchd PATH에서도 lsof로 판정해 지움", !exists(X+"/go-buildlaunchd") && exists(X+"/TestOpen444"))
	sl.Process.Kill()
	sl.Wait()

	// Go 빌드 캐시 상한(2026-11-21 mini-m4 ~/.cache/go-build 하루 13.4GB): 여유가 문턱 1.5배 아래면 GOCACHE를 sweep.gocache_gb 아래로
	// 오래된 항목부터 줄이고, GOCACHE와 다른 옛 자리 ~/Library/Caches/go-build는 비운다. go가 돌면(가짜 ps 목록에 go) 건너뜀
	z := strings.Repeat("\x00", 4096)
	for i := 1; i <= 4; i++ {
		f := fmt.Sprintf("%s/0%d/h%d-a", G, i%2, i)
		write(t, f, z)
		m := time.Date(2026, 1, 1, 0, i, 0, 0, time.Local)
		os.Chtimes(f, m, m)
	}
	write(t, G+"/ab/new-d", z)
	write(t, OLD+"/00/x-a", z)
	write(t, G+"/README", "r")
	cfg("sweep.disk_free_gb 1000000000", "sweep.gocache_gb 0.000009", "sweep.tmp_hours 0") // 항목 다섯 20KB → 9KB 아래: h1·h2·h3을 지움
	psComm(true)
	sweep()
	c.ok("Go 캐시: go가 돌면 건너뜀", exists(G+"/00/h2-a") && exists(OLD+"/00/x-a") && strings.Contains(out, "go가 도는 중"))
	psComm(false)
	sweep("--dry-run")
	c.ok("Go 캐시: dry-run은 찍기만", exists(G+"/01/h1-a") && strings.Contains(out, "Go 캐시 줄이기 (할 것)"))
	sweep()
	var left []string
	filepath.Walk(G, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			left = append(left, strings.TrimPrefix(p, G+"/"))
		}
		return nil
	})
	sort.Strings(left)
	c.ok("Go 캐시: 오래된 것부터 상한 아래로, README 남김, 옛 자리 비움, closed.log 한 줄",
		strings.Join(left, " ") == "00/h4-a README ab/new-d" && !exists(OLD) && strings.Contains(read(LOG), "Go 캐시 줄임"))
	write(t, OLD+"/00/y-a", "x")
	cfg("sweep.disk_free_gb 0.000001", "sweep.gocache_gb 0.000004", "sweep.tmp_hours 0")
	sweep()
	c.ok("Go 캐시: 디스크 여유가 넉넉하면 안 건드림", exists(G+"/00/h4-a") && exists(OLD+"/00/y-a"))
	cfg("sweep.disk_free_gb 1000000000", "sweep.gocache_gb 1", "sweep.tmp_hours 0")
	sweepEnv(e.With("GOCACHE=" + OLD))
	c.ok("Go 캐시: GOCACHE가 옛 자리면 비우지 않음", exists(OLD+"/00/y-a"))
}
