package util_test

// 옛 tests/worker-mac.test.sh: worker-mac-check·worker-mac-setup 시험. 가짜 ssh·hc(PATH 앞)·임시 HOME 둘(맥·원격)·config.example 사본으로.
// 원격 쪽 시스템 명령(launchctl·pmset·sudo 등)은 가짜 원격 HOME의 ~/.local/bin에 둔 가짜가 받는다. 실제 기계·네트워크는 쓰지 않는다.
// 예시 설정은 WORKER_MAC_EXAMPLE=<config.example 폴더>.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// workerMacBin: worker-mac-check·worker-mac-setup·hc-core-relaunch·herdr-tools-config Go 바이너리를 지은 폴더
func workerMacBin(t *testing.T) string {
	t.Helper()
	tool(t, "worker-mac-setup")
	tool(t, "hc-core-relaunch")
	return filepath.Dir(tool(t, "worker-mac-check"))
}

// hasLine: 줄 하나가 s와 같은가(grep -Fxq).
func hasLine(out, s string) bool {
	for _, l := range strings.Split(out, "\n") {
		if l == s {
			return true
		}
	}
	return false
}

// cfgDel·cfgSet: 설정 파일의 키 줄 지우기·바꾸기(없으면 더함).
func cfgDel(t *testing.T, f, k string) {
	t.Helper()
	re := regexp.MustCompile("^" + regexp.QuoteMeta(k) + `([[:space:]]|$)`)
	var keep []string
	for _, l := range strings.SplitAfter(read(f), "\n") {
		if l != "" && !re.MatchString(strings.TrimSuffix(l, "\n")) {
			keep = append(keep, l)
		}
	}
	write(t, f, strings.Join(keep, ""))
}

func cfgSet(t *testing.T, f, k, v string) {
	t.Helper()
	cfgDel(t, f, k)
	write(t, f, read(f)+k+" "+v+"\n")
}

// trustJSON: ~/.claude.json — 안내 끝 + 주어진 경로마다 신뢰.
func trustJSON(t *testing.T, f string, paths ...string) {
	t.Helper()
	pr := map[string]any{}
	for _, p := range paths {
		pr[p] = map[string]bool{"hasTrustDialogAccepted": true}
	}
	b, err := json.Marshal(map[string]any{"hasCompletedOnboarding": true, "projects": pr})
	if err != nil {
		t.Fatal(err)
	}
	write(t, f, string(b)+"\n")
}

// claudeJSON: ~/.claude.json의 [안내 끝, 신뢰 경로들(차례대로)].
func claudeJSON(f string) (bool, []string, bool) {
	var v struct {
		HasCompletedOnboarding bool                       `json:"hasCompletedOnboarding"`
		Projects               map[string]json.RawMessage `json:"projects"`
	}
	if json.Unmarshal([]byte(read(f)), &v) != nil {
		return false, nil, false
	}
	keys := []string{}
	for k := range v.Projects {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return v.HasCompletedOnboarding, keys, true
}

func TestWorkerMac(t *testing.T) {
	// 두 도구(worker-mac-check·worker-mac-setup)를 한 흐름으로 본다
	run1(t, testWorkerMac)
}

func testWorkerMac(t *testing.T, T string, e Env) {
	BIN := workerMacBin(t)
	EX := os.Getenv("WORKER_MAC_EXAMPLE")
	if EX == "" {
		EX = filepath.Join(repo, "config.example")
	}
	if !exists(EX + "/config") {
		t.Fatal("FAIL - 예시 설정이 없다: " + EX + "/config")
	}
	TLOG, RHOME, HOME := T+"/log", T+"/rh", T+"/mac"
	e = e.With("T_LOG="+TLOG, "RHOME="+RHOME, "HOME="+HOME, "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_NOSYSTEM=1")
	mkdirs(t, T+"/bin", TLOG, RHOME+"/.local/bin", HOME+"/.local/bin", RHOME+"/Library/Application Support/Google/Chrome")
	os.Symlink(BIN+"/herdr-tools-config", T+"/bin/herdr-tools-config")
	os.Symlink(BIN+"/worker-mac-check", T+"/bin/worker-mac-check")
	// brew 폴더 자리: 기본 PATH에 넣어 둔다(hc 터미널 PATH 점검이 통과하게)
	mkdirs(t, T+"/brew", T+"/brew2")
	e = e.With("WORKER_MAC_BREW_DIR="+T+"/brew", "PATH="+T+"/bin:"+T+"/brew:"+e["PATH"])

	// 가짜 ssh: 옵션(과 --)을 건너뛰고 기계 이름을 ssh.log에, 나머지 명령은 원격 HOME에서(원격 가짜 명령 폴더를 PATH 앞에) sh -c로 돈다.
	// SSH_FAKE_FAIL이면 연결 안 됨, SSH_FAKE_CUT이면 원격 본문(base64)은 앞 두 줄만 내고 255(끊김).
	script(t, T+"/bin/ssh", T, `#!/bin/bash
while [ $# -gt 0 ]; do case $1 in -o) shift 2 ;; -n|-t) shift ;; --) shift; break ;; *) break ;; esac; done
echo "$1" >>"$T_LOG/ssh.log"; shift
[ -z "${SSH_FAKE_FAIL-}" ] || exit 255
cd "$RHOME" || exit 255
if [ -n "${SSH_FAKE_CUT-}" ] && [[ "$*" == *base64* ]]; then HOME=$RHOME PATH=$RHOME/.local/bin:$PATH /bin/sh -c "$*" | head -n 2; exit 255; fi
HOME=$RHOME PATH=$RHOME/.local/bin:$PATH exec /bin/sh -c "$*"
`)
	// 가짜 hc(맥 쪽): machine ls(진짜 hc --json 꼴)는 w1 연결됨, exec는 -- 뒤 명령을 원격 HOME에서. 그 밖은 hc.log에.
	script(t, T+"/bin/hc", T, `#!/bin/bash
case $1 in
  machine) [ "$2" = ls ] && { echo '{"machines":[{"name":"w1","state":"connected"}]}'; exit 0; }; echo "$*" >>"$T_LOG/hc.log" ;;
  exec) while [ "$1" != -- ]; do shift; done; shift; cd "$RHOME" && HOME=$RHOME exec "$@" ;;
  *) echo "$*" >>"$T_LOG/hc.log" ;;
esac
`)
	// 원격 가짜 시스템 명령. Dock pid 123, 코어 pid 77, 둘 다 asid 5(코어 GUI 문맥).
	w := func(p, body string) { script(t, p, T, "#!/bin/bash\n"+body+"\n") }
	R := RHOME + "/.local/bin"
	w(R+"/pgrep", `case "$1 ${2-}" in "-x "*) echo 123 ;; *connect*) ;; *) echo 77 ;; esac`)
	w(R+"/launchctl", `case "$1 $2" in "print pid/"*) echo "asid = 5"; echo "name = dev.herdr.core.t" ;; "print gui/"*) echo "0 - com.t.herdr-core" ;; *) echo "$*" >>"$T_LOG/launchctl.log" ;; esac`)
	w(R+"/defaults", `id -un`)
	w(R+"/pmset", `echo " SleepDisabled 1"`)
	w(R+"/fdesetup", `echo "FileVault is Off."`)
	w(R+"/sudo", `echo "sudo: a password is required"; exit 1`)
	w(R+"/sqlite3", `exit 1`)
	w(R+"/xcode-select", `echo /nonexistent`)
	w(R+"/pkgutil", `exit 1`)
	w(R+"/security", `exit 0`)
	w(R+"/gh", `echo "Logged in to github.com account tester (keyring)"`)
	w(R+"/herdr-core", `echo "{\"live\":0}"`)
	w(R+"/hc", `echo "$*" >>"$T_LOG/remote-hc.log"`)
	for _, d := range []string{R, HOME + "/.local/bin"} {
		w(d+"/git", `[ "${1-}" = --version ] && { echo "git version 9.9.9"; exit 0; }; exec /usr/bin/git "$@"`)
		w(d+"/node", `echo v99.0.0`)
		w(d+"/fooz", `echo "fooz 1.2.3"; echo "둘째 줄"`)
	}
	w(R+"/example-proxy", `[ "$1" = status ] && { echo "proxy running"; echo "usable now: 2"; }`)
	w(R+"/other-proxy", `[ "$1" = status ] && echo "usable now: 0"`)

	var out string
	var rc int
	c := checker{t: t, out: &out, rc: &rc}
	has := func(s string) bool { return strings.Contains(out, s) }
	conf := T + "/conf/config"
	newcfg := func() {
		os.RemoveAll(T + "/conf")
		if b, err := exec.Command("cp", "-R", EX, T+"/conf").CombinedOutput(); err != nil {
			t.Fatalf("cp: %v %s", err, b)
		}
		os.Remove(T + "/conf/worker-checklist.txt")
		e["HERDR_TOOLS_CONFIG"] = T + "/conf"
	}
	del := func(k string) { cfgDel(t, conf, k) }
	set := func(k, v string) { cfgSet(t, conf, k, v) }
	newhome := func() {
		for _, p := range []string{"src", "work", "Documents", ".config", ".local/state"} {
			os.RemoveAll(RHOME + "/" + p)
		}
		ms, _ := filepath.Glob(RHOME + "/.claude.json*")
		for _, m := range ms {
			os.RemoveAll(m)
		}
		write(t, TLOG+"/ssh.log", "")
	}
	tool := func(env Env, name string, args ...string) { out, rc = run(T, env, BIN+"/"+name, args...) }
	check := func(args ...string) { tool(e, "worker-mac-check", args...) }
	setup := func(args ...string) { tool(e, "worker-mac-setup", args...) }
	gitRepo := func(d string) {
		bash(t, T, e, `d=$1; mkdir -p "$d" && /usr/bin/git -C "$d" init -q -b main && /usr/bin/git -C "$d" commit -q --allow-empty -m t \
  && /usr/bin/git -C "$d" remote add origin https://example.invalid/p.git`, RHOME+"/"+d)
	}
	trust := func(ps ...string) { trustJSON(t, RHOME+"/.claude.json", ps...) }
	cat := func(p string) string { return strings.TrimRight(read(p), "\n") }

	// ── worker-mac-check ──
	newcfg()
	newhome()
	gitRepo("src/example-app")
	trust(RHOME + "/src/example-app")
	mkdirs(t, RHOME+"/.config/example-app", RHOME+"/Documents")
	write(t, RHOME+"/.config/example-app/verify-slots", "2\n")
	check("w1")
	c.ok("check: worker.repo 첫 경로의 저장소", rc == 0 && has("통과\t저장소\t"+RHOME+"/src/example-app (main, https://example.invalid/p.git)"))
	c.ok("check: 신뢰는 찾은 저장소와 그 부모로", has("통과\tClaude 첫 실행·신뢰\t안내 끝, 신뢰 "+RHOME+"/src=false example-app=true"))
	c.ok("check: worker.slots_file 파일", has("통과\tverify-slots\t2"))
	c.ok("check: worker.tools 도구만 판 비교", has("통과\t판 git\t9.9.9 (맥과 같음)") && has("통과\t판 node\tv99.0.0 (맥과 같음)") && !has("판 bun") && !has("판 go"))
	c.ok("check: worker.proxy 상태 줄", has("통과\texample-proxy status\tproxy running usable now: 2"))

	newhome()
	gitRepo("work/example-app")
	write(t, RHOME+"/.claude.json", "{}\n")
	check("w1")
	c.ok("check: worker.repo는 처음 있는 것", has("통과\t저장소\t"+RHOME+"/work/example-app"))
	c.ok("check: 신뢰 없으면 실패", rc == 1 && has("실패\tClaude 첫 실행·신뢰\t안내 끝=false, 신뢰 "+RHOME+"/work=false example-app=false"))
	c.ok("check: slots 파일이 없으면 실패", has("실패\tverify-slots\t파일 없음"))
	trust(RHOME + "/work")
	check("w1")
	c.ok("check: 둘째 후보(work/…) 저장소는 그 부모 신뢰로 통과", has("통과\tClaude 첫 실행·신뢰\t안내 끝, 신뢰 "+RHOME+"/work=true example-app=false"))
	trust(RHOME+"/src", RHOME+"/src/example-app")
	check("w1")
	c.ok("check: 찾은 저장소와 상관없는 worker.trust 경로 신뢰로는 통과 안 함", has("실패\tClaude 첫 실행·신뢰\t안내 끝=true, 신뢰 "+RHOME+"/work=false example-app=false"))
	newhome()
	gitRepo("src/example-app")
	trust(RHOME + "/src/example-web")
	check("w1")
	c.ok("check: 형제 폴더 신뢰로는 통과 안 함", has("실패\tClaude 첫 실행·신뢰\t안내 끝=true, 신뢰 "+RHOME+"/src=false example-app=false"))

	newhome()
	trust(RHOME + "/src")
	check("w1")
	c.ok("check: worker.repo 경로가 다 없으면 첫 경로로 실패", has("실패\t저장소\t~/src/example-app 없음"))
	c.ok("check: 저장소가 없으면 첫 후보와 그 부모의 신뢰", has("통과\tClaude 첫 실행·신뢰\t안내 끝, 신뢰 "+RHOME+"/src=true example-app=false"))
	newcfg()
	set("worker.repo", "x*")
	newhome()
	gitRepo("xa")
	check("w1")
	c.ok("check: 목록의 *는 원격에서 펼치지 않음", has("실패\t저장소\t~/x* 없음"))

	newcfg()
	set("worker.tools", "fooz")
	set("worker.proxy", "other-proxy")
	newhome()
	check("w1")
	c.ok("check: worker.tools를 바꾸면 그 도구(--version 첫 줄)", has("통과\t판 fooz\tfooz 1.2.3 (맥과 같음)") && !has("판 git"))
	c.ok("check: worker.proxy를 바꾸면 그 명령의 status", has("실패\tother-proxy status\tusable now: 0"))

	newcfg()
	for _, k := range []string{"worker.repo", "worker.trust", "worker.slots_file", "worker.tools", "worker.proxy"} {
		del(k)
	}
	newhome()
	mkdirs(t, RHOME+"/Documents/app")
	trust(RHOME + "/Documents/app")
	check("w1")
	c.ok("check: worker.repo 없음 → 모름 설정 없음", has("모름\t저장소\t설정 없음(worker.repo)"))
	c.ok("check: worker.trust 없음 → Documents/app", has("통과\tClaude 첫 실행·신뢰\t안내 끝, 신뢰 Documents/app=true"))
	c.ok("check: worker.slots_file 없음 → 모름 설정 없음", has("모름\t검증 자리 파일\t설정 없음(worker.slots_file)"))
	c.ok("check: worker.tools 없음 → git node", has("판 git\t") && has("판 node\t") && grepc(out, "\t판 ") == 2)
	c.ok("check: worker.proxy 없음 → 모름 설정 없음", has("모름\t프록시 status\t설정 없음(worker.proxy)"))
	set("worker.trust", "src src/example-app")
	trust(RHOME + "/src")
	check("w1")
	c.ok("check: worker.repo 없음 → worker.trust 경로가 모두 신뢰여야 통과", has("실패\tClaude 첫 실행·신뢰\t안내 끝=true, 신뢰 src=true src/example-app=false"))
	os.Remove(conf)
	check("w1")
	c.ok("check: 설정 파일이 없으면 건너뛰는 항목마다 모름", has("설정 없음(worker.repo)") && has("설정 없음(worker.slots_file)") && has("설정 없음(worker.proxy)") && grep(out, "^-- w1: .*모름 [1-9]"))
	newcfg()
	del("worker.proxy")
	write(t, conf, read(conf)+"worker.proxy\nworker.repo\nworker.slots_file\n")
	del("worker.repo")
	del("worker.slots_file")
	write(t, conf, read(conf)+"worker.repo\nworker.slots_file\n")
	check("w1")
	c.ok("check: 빈 값(키 혼자)은 일부러 끈 것 → 줄 없음", !has("설정 없음") && !has("저장소") && !has("verify-slots") && !has(" status\t"))

	newcfg()
	write(t, conf, read(conf)+"worker.tools git\n")
	newhome()
	check("w1")
	c.ok("check: 같은 키 두 번이면 종료 2, 접속 안 함", rc == 2 && read(TLOG+"/ssh.log") == "")

	// ── hc 터미널 PATH에 brew ──
	newcfg()
	newhome()
	check("w1")
	c.ok("check: hc 터미널 PATH에 brew 폴더가 있으면 통과", has("통과\tbrew PATH(hc 문맥)\t"+T+"/brew 있음"))
	tool(e.With("WORKER_MAC_BREW_DIR="+T+"/brew2"), "worker-mac-check", "w1")
	c.ok("check: brew 폴더가 있는데 hc 터미널 PATH에 없으면 실패", rc == 1 && has("실패\tbrew PATH(hc 문맥)\t"+T+"/brew2 없음") && has("hc-core-relaunch w1"))
	tool(e.With("WORKER_MAC_BREW_DIR="+T+"/nobrew"), "worker-mac-check", "w1")
	c.ok("check: brew 폴더가 없는 기계는 해당 없음", has("통과\tbrew PATH(hc 문맥)\t해당 없음: "+T+"/nobrew 없음"))

	// [E01] 원격 점검 본문이 끊기면(ssh 255) 받은 줄은 두고 「실패 원격 점검」, 종료 1 — 끝까지 온 점검에는 그 줄이 없다
	c.ok("[E01] check: 끝까지 온 원격 점검에는 「원격 점검」 줄 없음", !has("원격 점검") && has("TCC 화면 기록"))
	tool(e.With("SSH_FAKE_CUT=1"), "worker-mac-check", "w1")
	c.ok("[E01] check: 끊긴 원격 점검은 받은 줄 + 실패·종료 1", rc == 1 && has("통과\t자동 로그인\t") && !has("TCC 화면 기록") &&
		has("실패\t원격 점검\t끊김(종료 255) — 받은 줄까지만 봤다"))

	// [E10] -로 시작하는 기계 이름은 ssh·hc 옵션으로 읽힌다 — 사용 오류 64, 접속 안 함
	newhome()
	check("-oProxyCommand=x")
	c.ok("[E10] check: -로 시작하는 이름 → 64, 접속 안 함", rc == 64 && read(TLOG+"/ssh.log") == "")
	setup("w1", "-x")
	c.ok("[E10] setup: 모르는 옵션 → 64, 접속 안 함", rc == 64 && read(TLOG+"/ssh.log") == "")
	setup("mesh", "-oProxyCommand=x")
	c.ok("[E10] setup mesh: -로 시작하는 이름 → 64", rc == 64 && read(TLOG+"/ssh.log") == "")

	// ── hc-core-relaunch ──
	// 원격에 남긴 core-restart.sh를 가짜 로그인 셸로 직접 돌려, 코어 재시작이 로그인 환경에서 --fresh로 가는지 본다.
	newcfg()
	newhome()
	w(R+"/herdr-core", `echo "herdr-core $* PATH=$PATH" >>"$T_LOG/core.log"; echo "{\"live\":0}"`)
	w(T+"/loginsh", `[ "$1" = -lc ] || exit 9; export PATH="/login/brew:$PATH"; exec /bin/sh -c "$2"`)
	out, rc = run(T, e, BIN+"/hc-core-relaunch", "w1")
	c.ok("relaunch: 원격에 한 번짜리 launchd 작업을 올림", rc == 0 && grep(read(TLOG+"/launchctl.log"), "^bootstrap gui/"))
	write(t, TLOG+"/core.log", "")
	out, rc = run(RHOME, e.With("HOME="+RHOME, "SHELL="+T+"/loginsh", "PATH="+R+":/usr/bin:/bin"), "bash", RHOME+"/.local/state/core-restart.sh")
	c.ok("relaunch: 로그인 셸 환경으로 server restart --fresh", rc == 0 && grep(read(TLOG+"/core.log"), "^herdr-core server restart --fresh PATH=/login/brew:"))
	w(R+"/herdr-core", `echo "{\"live\":0}"`)

	// ── worker-mac-setup ──
	newcfg()
	newhome()
	setup("w1", "--slots", "3")
	c.ok("setup: 성공", rc == 0)
	c.ok("setup: --slots는 worker.slots_file에", cat(RHOME+"/.config/example-app/verify-slots") == "3")
	c.ok("setup: worker.trust 경로를 신뢰로", func() bool {
		ob, keys, ok := claudeJSON(RHOME + "/.claude.json")
		return ok && ob && strings.Join(keys, "\n") == RHOME+"/src\n"+RHOME+"/src/example-app"
	}())
	c.ok("setup: worker.trust 첫 경로 폴더를 만듦", exists(RHOME+"/src"))
	c.ok("setup: 체크리스트 파일 없음 → 8번이 Xcode", grep(out, `^ 8\. \[그 맥\] \(Xcode`) && grep(out, `^ 9\. \[이 맥\] 점검: worker-mac-check w1`))
	c.ok("setup: 체크리스트 파일 없음을 알림", has("설정 없음(worker-checklist.txt)"))

	newcfg()
	newhome()
	write(t, T+"/conf/worker-checklist.txt", "# 주석\n[이 맥] 예시 로그인 {M}:\n    ! ssh -t {M} 예시\n\n[그 맥] 둘째 단계\n")
	setup("w1")
	c.ok("setup: worker-checklist.txt를 8번부터 붙임, {M}은 기계 이름", hasLine(out, " 8. [이 맥] 예시 로그인 w1:") && hasLine(out, "    ! ssh -t w1 예시") && hasLine(out, " 9. [그 맥] 둘째 단계"))
	c.ok("setup: 설정이 다 있으면 설정 없음 줄 없음", !has("설정 없음"))
	c.ok("setup: worker.mesh에 없는 기계는 워커 연결을 건너뜀", has("워커 연결: w1 은(는) worker.mesh에 없어 건너뜀") && !grep(read(TLOG+"/ssh.log"), "^worker-[abc]$"))
	c.ok("setup: 붙인 뒤 번호가 이어짐", grep(out, `^10\. \[그 맥\] \(Xcode`) && grep(out, `^11\. \[이 맥\] 점검`) && !has("# 주석"))
	c.ok("setup: --slots 없으면 1", cat(RHOME+"/.config/example-app/verify-slots") == "1")

	newcfg()
	del("worker.slots_file")
	del("worker.trust")
	del("worker.mesh")
	newhome()
	setup("w1", "--slots", "3")
	c.ok("setup: worker.slots_file 없음 + --slots → 종료 64, 접속 안 함", rc == 64 && read(TLOG+"/ssh.log") == "")
	setup("w1")
	c.ok("setup: worker.slots_file 없음 → slots 단계 없음", rc == 0 && !has("verify-slots") && !exists(RHOME+"/.config"))
	c.ok("setup: worker.slots_file 없음을 알림", has("설정 없음(worker.slots_file)"))
	c.ok("setup: worker.mesh 없음 → 워커 연결 건너뜀을 알림", has("설정 없음(worker.mesh): 워커 연결 단계를 건너뜀"))
	c.ok("setup: worker.trust 없음 → Documents/app", func() bool {
		_, keys, ok := claudeJSON(RHOME + "/.claude.json")
		return ok && strings.Join(keys, "\n") == RHOME+"/Documents/app"
	}())

	newcfg()
	set("worker.trust", "it's src")
	set("worker.slots_file", ".config/it's/slots")
	newhome()
	setup("w1")
	c.ok("setup: 경로에 따옴표가 있어도 폴더·자리 파일·뒤 단계", func() bool {
		fi, err := os.Stat(RHOME + "/.local/state/worker-mac-setup/root-setup.sh")
		return rc == 0 && exists(RHOME+"/it's") && cat(RHOME+"/.config/it's/slots") == "1" && err == nil && fi.Mode()&0o111 != 0
	}())

	newcfg()
	newhome()
	tool(e.With("SSH_FAKE_FAIL=1"), "worker-mac-setup", "w1")
	c.ok("setup: SSH 안내는 worker.ssh_key", rc == 2 && has("IdentityFile ~/.ssh/id_ed25519_workers") && has("ssh-copy-id -i ~/.ssh/id_ed25519_workers.pub w1"))
	del("worker.ssh_key")
	tool(e.With("SSH_FAKE_FAIL=1"), "worker-mac-setup", "w1")
	c.ok("setup: worker.ssh_key 없음 → ~/.ssh/id_ed25519", has("IdentityFile ~/.ssh/id_ed25519)") && has("ssh-copy-id -i ~/.ssh/id_ed25519.pub w1"))

	newcfg()
	newhome()
	setup("w1", "--dry-run", "--slots", "2")
	c.ok("setup --dry-run: 원격에 쓰지 않음", rc == 0 && !exists(RHOME+"/.claude.json") && !exists(RHOME+"/src") && has("(dry-run) ~/.claude.json: hasCompletedOnboarding=true, 신뢰 src src/example-app"))
}
