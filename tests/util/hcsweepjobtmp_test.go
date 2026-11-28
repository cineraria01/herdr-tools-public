package util_test

// 작업별 임시 폴더 지우기(ht-job-tmpdir, docs/hc-sweep/job-tmp.md): hc-sweep 한 바퀴(가짜 hc)가
// ③ 닫은 터미널의 폴더를 통째로(안에 이름 꼴 열 가지가 섞여도) ④ 주인이 hc ls에 없고 유예가 지난 폴더를 지우고, 주인이 살아 있거나·
// 유예 안이거나·hc ls를 못 읽으면 남긴다. ⑤ 배경 실행이 남아 안 닫은 터미널의 폴더는 그대로. 안전띠 ① 목록이 이 기계를 다 담지 않았으면
// 아무것도 안 지움 ② 고아라도 맨 위가 유예 안에 바뀌었으면 미룸. ⑧ jobtmp.root off면 지금 그대로

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestHcSweepJobTmp(t *testing.T) { run1(t, testHcSweepJobTmp) }

func testHcSweepJobTmp(t *testing.T, T string, e Env) {
	SWEEP := tool(t, "hc-sweep")
	var out string
	c := checker{t: t, out: &out}
	R := T + "/jroot"
	mkdirs(t, T+"/bin", T+"/home", T+"/cfg", T+"/tmp", R)
	claudeSess(t, T+"/home")
	e = e.With("PATH="+T+"/bin:"+e["PATH"], "HOME="+T+"/home", "HERDR_TOOLS_CONFIG="+T+"/cfg", "TMPDIR="+T+"/tmp").Without("XDG_STATE_HOME", "HC_LANE_MACHINE")
	cfg := func(ls ...string) {
		write(t, T+"/cfg/config", strings.Join(append([]string{"sweep.tmp_hours 0", "sweep.disk_free_gb 0"}, ls...), "\n")+"\n")
	}
	cfg("jobtmp.root " + R)
	script(t, T+"/bin/hc", T, `#!/bin/bash
case $1 in
  ls) [ -f "@T@/ls.json" ] || exit 3; exec cat "@T@/ls.json" ;;
  get) [ -e "@T@/killed-$2" ] && { exec sed 's/"state":"live"/"state":"finished"/' "@T@/get-$2.json"; }
       [ -f "@T@/get-$2.json" ] && exec cat "@T@/get-$2.json" ;;
  read) [ -f "@T@/screen-$2" ] && exec cat "@T@/screen-$2"; echo "✻ Done"; exit 0 ;;
  kill) touch "@T@/killed-$2"; exit 0 ;;
  rm) exit 0 ;;
esac
exit 1
`)
	old := time.Now().Add(-2 * time.Hour)
	// job: 작업 폴더 하나(표식 주인·이름, 표식 시각 at). 안에 hc 작업이 남기는 이름 꼴 열 가지, 모두 at으로
	job := func(dir, addr, name string, at time.Time) string {
		p := R + "/" + dir
		for _, n := range []string{"go-build1/b", "TestFoo1/001", "tmp.Ab12Cd", "audit-x", "com.google.Chrome.Ab12Cd/SingletonCookie",
			".com.google.Chrome.Xy", "proj-agent-tools-1", "release-macos-test-Ab12Cd", "pva-git-Ab12Cd", "herdr-verify.Ab12Cd"} {
			write(t, p+"/"+n+"/f", "x")
		}
		write(t, p+"/.hc-job", fmt.Sprintf("%s\n%s\n%d\n", addr, name, at.Unix()))
		filepath.Walk(p, func(q string, _ os.FileInfo, _ error) error { return os.Chtimes(q, at, at) })
		return p
	}
	sweep := func(args ...string) { out, _ = run(T, e, SWEEP, append([]string{"--only-under", T}, args...)...) }
	oldMs := old.Unix() * 1000
	term := func(a, name, extra string) { // 쉰 지 2시간 된 worker 레인 터미널(cwd는 없는 폴더 — 작업 트리 아님)
		write(t, T+"/get-"+a+".json", fmt.Sprintf(`{"address":"%s","name":"%s","state":"live","pid":4343,"status":{"app":"claude-code","state":"idle"},"labels":{"hc-lane":"1"},"created_ms":%d,"status_ms":%d,"turn":4,"cwd":"%s"%s}`+"\n",
			a, name, oldMs, oldMs, T+"/gone-"+a, extra))
	}

	// ③ 닫은 터미널 t61: 그 폴더는 통째로. ⑤ t63은 화면에 배경 실행이 남아 안 닫음 → 폴더 그대로. ④ 주인 살아 있음(t64 working)·
	// 주인 없음(t99)·주인 없어도 유예 안(t98 표식 방금)·같은 주소 다른 이름(t64/old-run). ② 주인 없어도 맨 위 항목이 방금 바뀜(t97)
	term("t61", "lane-a", "")
	term("t63", "lane-bg", "")
	write(t, T+"/screen-t63", "작업 중\n· 1 shell still running\n")
	write(t, T+"/get-t64.json", `{"address":"t64","name":"lane-w","state":"live","pid":4343,"status":{"app":"claude-code","state":"working"},"cwd":"/"}`+"\n")
	ls := `{"terminals":[{"address":"t61","name":"lane-a","state":"live","status":"idle"},{"address":"t63","name":"lane-bg","state":"live","status":"idle"},` +
		`{"address":"t64","name":"lane-w","state":"live","status":"working"},{"address":"win/t1","name":"w","state":"live","cwd":"C:\\Users\\w"}]}`
	write(t, T+"/ls.json", ls+"\n")
	J61, J63, J64 := job("aaaaa1", "t61", "lane-a", old), job("aaaaa3", "t63", "lane-bg", old), job("aaaaa4", "t64", "lane-w", old)
	J64o, J99, J98 := job("aaaaa5", "t64", "old-run", old), job("aaaaa9", "t99", "lane-gone", old), job("aaaaa8", "t98", "lane-new", time.Now())
	J97 := job("aaaaa7", "t97", "lane-touch", old)
	now := time.Now()
	os.Chtimes(J97+"/audit-x", now, now) // 맨 위 항목 하나가 방금 바뀜

	// hc ls 못 읽음 → 아무것도 안 지움(닫지도 않음)
	os.Rename(T+"/ls.json", T+"/ls.bak")
	sweep()
	c.ok("hc ls 못 읽음: 아무 작업 폴더도 안 지움", exists(J61) && exists(J99) && exists(J64o))
	os.Rename(T+"/ls.bak", T+"/ls.json")

	// ① 목록을 다 담았다고 볼 수 없으면 안 지움: 이 기계 줄이 하나도 없음 · 오류 칸 · state 없는 줄
	for _, bad := range []string{
		`{"terminals":[{"address":"win/t1","name":"w","state":"live"}]}`,
		`{"error":{"kind":"partial"},"terminals":[{"address":"t64","name":"lane-w","state":"live"}]}`,
		`{"terminals":[{"address":"t64","name":"lane-w","state":"live"},{"address":"t65","name":"x"}]}`,
	} {
		write(t, T+"/ls.json", bad+"\n")
		sweep()
		c.ok("안전띠 ①: 목록을 못 믿으면 안 지움 — "+bad, exists(J99) && exists(J64o) && strings.Contains(out, "hc ls 목록을 믿을 수 없음"))
	}
	write(t, T+"/ls.json", ls+"\n")

	// ⑧ 끄기: jobtmp.root off면 지금 그대로
	cfg("jobtmp.root off")
	sweep("--dry-run")
	c.ok("⑧ off: 작업 폴더를 보지 않음", !strings.Contains(out, "작업 임시") && exists(J99))
	cfg("jobtmp.root " + R)

	sweep("--dry-run")
	c.ok("dry-run: 지울 것만 찍고 안 지움", strings.Contains(out, "지울 작업 임시(주인 터미널 없음): "+J99) && exists(J99))

	sweep()
	c.ok("③ 닫은 터미널의 폴더는 통째로(이름 꼴 열 가지 섞임)", strings.Contains(out, "닫힘: R") && !exists(J61) && strings.Contains(out, "지운 작업 임시(터미널 닫음): "+J61))
	c.ok("⑤ 배경 실행이 남아 안 닫은 터미널의 폴더는 그대로", !exists(T+"/killed-t63") && exists(J63))
	c.ok("④ 주인 없고 유예 지남: 지움", !exists(J99))
	c.ok("④ 주인 살아 있음(주소+이름): 남김", exists(J64))
	c.ok("④ 같은 주소 다른 이름(앞 실행): 주인 없음으로 지움", !exists(J64o))
	c.ok("④ 유예 안(표식 방금): 남김", exists(J98))
	c.ok("안전띠 ②: 고아라도 맨 위가 유예 안에 바뀌면 미룸", exists(J97) && strings.Contains(out, "작업 임시 미룸") && strings.Contains(out, J97))
	c.ok("Windows 줄이 섞여도 안 깨짐", !strings.Contains(out, "hc ls 목록을 믿을 수 없음"))
	c.ok("closed.log에 한 줄씩", grepc(read(T+"/home/.claude-tasks/closed.log"), "작업 임시 폴더 지움") == 3)
}

// ⑦ 한 번 치우기(--tmp-backlog <시간>): dry-run은 종류별 개수·크기(큰 차례, 같은 머리는 한 줄)와 --only에 붙일 한 줄을 보이고 안 지운다.
// 지우는 것은 --only로 고른 종류만(없으면 거절). 운영체제 것·새 것·열린 것은 --only에 들어도 뺀다(ht-tmp-backlog-only)
func TestHcSweepTmpBacklog(t *testing.T) { run1(t, testHcSweepTmpBacklog) }

func testHcSweepTmpBacklog(t *testing.T, T string, e Env) {
	SWEEP := tool(t, "hc-sweep")
	var out string
	c := checker{t: t, out: &out}
	X := T + "/tmp"
	mkdirs(t, T+"/bin", T+"/home", T+"/cfg", X)
	claudeSess(t, T+"/home")
	write(t, T+"/cfg/config", "jobtmp.root off\n")
	// 가짜 lsof: @T@/lsof-held의 경로를 다른 프로세스가 연 것으로
	script(t, T+"/bin/lsof", T, "#!/bin/sh\nprintf 'p%s\\nfcwd\\nn/\\n' \"$PPID\"\n"+
		"[ -f @T@/lsof-held ] && while read -r p; do printf 'p1\\nfcwd\\nn%s\\n' \"$p\"; done < @T@/lsof-held\nexit 0\n")
	e = e.With("PATH="+T+"/bin:"+e["PATH"], "HOME="+T+"/home", "HERDR_TOOLS_CONFIG="+T+"/cfg", "TMPDIR="+X, "XDG_STATE_HOME="+T+"/state")
	big := strings.Repeat("x", 200000) // 0.0MB가 아니게
	oldDirs := []string{"wails3-find-AbCdEf", "wails3-find-XyZw12", "tmp.Ab12Cd", "proj-agent-tools-123", "scoped_dir123_45",
		"com.google.Chrome.qmjab12", "com.google.Chrome.zzkq", "lane-dev-app-foo", "lane-dev-app-bar", "com.apple.foo", "TemporaryItems", "audit-held"}
	for _, d := range oldDirs {
		write(t, X+"/"+d+"/f", big)
	}
	for _, d := range []string{"contentlinkingd", "kw", "mediaanalysisd-access"} { // 데몬 같은 이름·한 개뿐 0MB → 그 밖
		mkdirs(t, X+"/"+d)
	}
	write(t, X+"/uv-1234abcd5678.lock", "x")
	write(t, X+"/uv-9876fedc4321.lock", "x")
	write(t, X+"/tmp.New123/f", "x") // 새 것
	write(t, X+"/wails3-find-Mixed1/old", "x")
	write(t, X+"/wails3-find-Mixed1/new", "x") // 안쪽 하나만 새로움
	write(t, T+"/lsof-held", X+"/audit-held\n")
	old := time.Now().Add(-72 * time.Hour)
	age := func(p string) {
		filepath.Walk(p, func(q string, _ os.FileInfo, _ error) error { return os.Chtimes(q, old, old) })
	}
	for _, p := range append(oldDirs, "uv-1234abcd5678.lock", "uv-9876fedc4321.lock", "wails3-find-Mixed1", "contentlinkingd", "kw", "mediaanalysisd-access") {
		age(X + "/" + p)
	}
	now := time.Now()
	os.Chtimes(X+"/wails3-find-Mixed1/new", now, now)
	pick := regexp.MustCompile(`(?m)^--only '([^']*)'$`)

	out, _ = run(T, e, SWEEP, "--tmp-backlog", "48", "--dry-run")
	c.ok("dry-run: 종류별 개수", grep(out, `(?m)^  wails3-find-\*\t2개\t`) && grep(out, `(?m)^  tmp\.\*\t1개\t`) &&
		grep(out, `(?m)^  proj-agent-tools-\*\t1개\t`) && grep(out, `(?m)^  scoped_\*\t1개\t`) && grep(out, `(?m)^  uv-\*\.lock\t2개\t`))
	c.ok("③ 같은 머리는 한 줄(mktemp 꼬리가 소문자뿐이어도)", grep(out, `(?m)^  com\.google\.Chrome\.\*\t2개\t`) && grep(out, `(?m)^  lane-dev-app-\*\t2개\t`) &&
		!strings.Contains(out, "qmj") && !strings.Contains(out, "zzkq"))
	c.ok("dry-run: 합계·남김·운영체제 것 뺌 한 줄", strings.Contains(out, "지울 것 14개") && strings.Contains(out, "열린 것 1개·새 것 2개 남김") &&
		strings.Contains(out, "운영체제 것 2개 뺌"))
	c.ok("dry-run: 크기 큰 차례", strings.Index(out, "  wails3-find-*") < strings.Index(out, "  tmp.*") && strings.Index(out, "  tmp.*") < strings.Index(out, "  uv-*.lock"))
	m := pick.FindStringSubmatch(out)
	c.ok("dry-run: --only에 붙일 한 줄(데몬 같은 것·한 개 0MB는 그 밖)", m != nil && strings.Contains(m[1], "wails3-find-*") && strings.Contains(m[1], "com.google.Chrome.*") &&
		!strings.Contains(m[1], "contentlinkingd") && !strings.Contains(m[1], "kw") && !strings.Contains(m[1], "mediaanalysisd") &&
		grep(out, `(?m)^그 밖\(고르지 않음\): .*contentlinkingd`) && grep(out, `(?m)^그 밖\(고르지 않음\): .*mediaanalysisd-\*`) && grep(out, `(?m)^그 밖\(고르지 않음\): .*kw`))
	c.ok("dry-run: 아무것도 안 지움", exists(X+"/tmp.Ab12Cd") && exists(X+"/wails3-find-AbCdEf"))
	c.ok("dry-run: 기록 파일 자리 한 줄·파일은 안 만듦", strings.Contains(out, T+"/state/hc-sweep/tmp-backlog/") && !exists(T+"/state/hc-sweep/tmp-backlog"))

	// ① --only 없이 지우려 함 → 거절, 아무것도 안 지움
	out, rc := run(T, e, SWEEP, "--tmp-backlog", "48")
	c.ok("① --only 없이 지우기는 거절(종료 2)", rc == 2 && strings.Contains(out, "--only") && strings.Contains(out, "--dry-run") &&
		exists(X+"/tmp.Ab12Cd") && exists(X+"/wails3-find-AbCdEf") && exists(X+"/contentlinkingd"))

	// ② 고른 종류만 지우고 나머지는 그대로 ④ 열린 것·새 것은 골라도 남김
	out, _ = run(T, e, SWEEP, "--tmp-backlog", "48", "--only", "tmp.*, wails3-find-*,audit-*")
	c.ok("② 고른 종류만 지움", !exists(X+"/tmp.Ab12Cd") && !exists(X+"/wails3-find-AbCdEf") && !exists(X+"/wails3-find-XyZw12") &&
		exists(X+"/proj-agent-tools-123") && exists(X+"/uv-1234abcd5678.lock") && exists(X+"/com.google.Chrome.zzkq") && exists(X+"/kw"))
	c.ok("④ 열린 것·새 것·안쪽이 새로운 것은 골라도 남김", exists(X+"/audit-held") && exists(X+"/tmp.New123") && exists(X+"/wails3-find-Mixed1") &&
		strings.Contains(out, "지움 3개"))
	c.ok("치우기: closed.log 한 줄", grepc(read(T+"/home/.claude-tasks/closed.log"), "임시 찌꺼기 한 번 치우기") == 1)
	// 지운 이름 낱낱이: 상태 폴더 hc-sweep/tmp-backlog/ 아래 실행마다 한 파일 — 한 줄에 시각·경로·바이트, 끝 줄 합계
	logs, _ := filepath.Glob(T + "/state/hc-sweep/tmp-backlog/*.log")
	lg := ""
	if len(logs) == 1 {
		lg = read(logs[0])
	}
	c.ok("지운 이름 기록: 파일 하나, 이름 수 = 지운 수(3), 끝 줄 합계", len(logs) == 1 && grepc(lg, "\t"+X+"/") == 3 &&
		strings.Contains(lg, "\t"+X+"/tmp.Ab12Cd\t") && strings.Contains(lg, X+"/wails3-find-XyZw12") && grep(lg, `합계\t3개\t`) &&
		strings.Contains(out, logs[0]))

	// ⑤ dry-run 끝 줄을 --only에 넣으면 같은 집합
	out, _ = run(T, e, SWEEP, "--tmp-backlog", "48", "--dry-run")
	m = pick.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("--only 줄 없음:\n%s", out)
	}
	kindsOf := func(s string) string { // 종류 줄(큰 차례)을 쉼표로
		var ks []string
		for _, x := range regexp.MustCompile(`(?m)^  (\S+)\t`).FindAllStringSubmatch(s, -1) {
			ks = append(ks, x[1])
		}
		return strings.Join(ks, ",")
	}
	out, _ = run(T, e, SWEEP, "--tmp-backlog", "48", "--dry-run", "--only", m[1])
	same := kindsOf(out) == m[1] && !strings.Contains(out, "그 밖")
	out, _ = run(T, e, SWEEP, "--tmp-backlog", "48", "--only", m[1])
	c.ok("⑤ dry-run 끝 줄로 고르면 같은 집합 — 그 밖만 남고 나머지는 지움", same && kindsOf(out) == m[1] &&
		!exists(X+"/proj-agent-tools-123") && !exists(X+"/uv-1234abcd5678.lock") && !exists(X+"/com.google.Chrome.zzkq") && !exists(X+"/lane-dev-app-foo") &&
		exists(X+"/contentlinkingd") && exists(X+"/kw") && exists(X+"/mediaanalysisd-access") && exists(X+"/com.apple.foo") && exists(X+"/audit-held"))

	// 레인 안(TMPDIR에 작업 표식 .hc-job)에서는 지우기를 거절 — 바깥 운영체제 자리(가짜 getconf)를 지우지 않는다. dry-run은 된다
	J, O := T+"/jobtmp", T+"/ostmp"
	write(t, J+"/.hc-job", "t1\nlane\n1\n")
	write(t, O+"/tmp.Os12Ab/f", "x")
	age(O + "/tmp.Os12Ab")
	script(t, T+"/bin/getconf", T, "#!/bin/sh\necho "+O+"\n") // 실행 권한 — 진짜 getconf로 새면 실제 사용자 임시 폴더다
	ej := e.With("TMPDIR=" + J)
	out, rc = run(T, ej, SWEEP, "--tmp-backlog", "48", "--dry-run")
	if !strings.Contains(out, "한 번 치우기("+O+",") {
		t.Fatalf("가짜 getconf가 안 쓰임 — 지우기 시험을 멈춘다:\n%s", out)
	}
	c.ok("레인 안 dry-run은 바깥 자리를 보임", rc == 0 && strings.Contains(out, "지울 것 1개") && exists(O+"/tmp.Os12Ab"))
	write(t, J+"/tmp.Jb12Cd/f", "x") // 작업 임시 폴더 안 것도 그대로여야
	age(J + "/tmp.Jb12Cd")
	out, rc = run(T, ej, SWEEP, "--tmp-backlog", "48", "--only", "tmp.*")
	c.ok("레인 안에서는 지우기 거절(종료 2, --only 있어도)", rc == 2 && exists(O+"/tmp.Os12Ab") && exists(J+"/tmp.Jb12Cd") && strings.Contains(out, "레인 안"))
	out, rc = run(T, ej, SWEEP, "--tmp-backlog", "48")
	c.ok("레인 안에서 --only도 없이 지우기 → 거절·아무것도 안 지움", rc == 2 && exists(O+"/tmp.Os12Ab") && exists(J+"/tmp.Jb12Cd"))
	out, rc = run(T, e.With("TMPDIR="+J+"/"), SWEEP, "--tmp-backlog", "48", "--only", "tmp.*") // ht-jobtmp-slash: macOS 꼴(끝 「/」) TMPDIR도 레인 안
	c.ok("끝 「/」 있는 작업 TMPDIR도 레인 안 → 거절", rc == 2 && exists(O+"/tmp.Os12Ab") && exists(J+"/tmp.Jb12Cd") && strings.Contains(out, "레인 안"))

	write(t, T+"/bin/lsof", "#!/bin/sh\nexit 1\n")
	write(t, X+"/tmp.Again1/f", "x")
	age(X + "/tmp.Again1")
	out, rc = run(T, e, SWEEP, "--tmp-backlog", "48", "--only", "tmp.*")
	c.ok("lsof 실패면 하나도 안 지우고 종료 1", rc == 1 && exists(X+"/tmp.Again1") && strings.Contains(out, "lsof 실패"))
}
