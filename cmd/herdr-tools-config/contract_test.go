package main

// herdr-tools-config 계약 시험(옛 tests/herdr-tools-config.test.sh, 2026-11-21 go-tests-ops): 임시 HOME·가짜 설정 폴더로
// bin/herdr-tools-config를 밖에서 불러 get·file·lint·sync-lint를 본다. 실제 ~/.config·네트워크는 쓰지 않는다.
// 판: HERDR_TOOLS_CONFIG_IMPL, 없으면 HERDR_IMPL, 없으면 go. 다른 사본은 HERDR_TOOLS_CONFIG_BIN=<경로>

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ct "herdr-tools/internal/clitest"
)

func TestContract(t *testing.T) {
	tool := ct.Tool(t, "herdr-tools-config", "HERDR_TOOLS_CONFIG_BIN")
	ex := filepath.Join(ct.Repo(t), "config.example")
	T := ct.TempDir(t)
	ct.Isolate(t, T)
	t.Setenv("HERDR_TOOLS_CONFIG_IMPL", ct.Impl("HERDR_TOOLS_CONFIG_IMPL"))
	for k, v := range map[string]string{"HOME": T + "/home", "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t", "GIT_CONFIG_NOSYSTEM": "1"} {
		t.Setenv(k, v)
	}
	ct.Unsetenv(t, "HERDR_TOOLS_CONFIG")
	for _, d := range []string{T + "/home", T + "/c1", T + "/c2", T + "/공백 폴더"} {
		os.MkdirAll(d, 0o755)
	}

	var r ct.Res
	c := &ct.C{T: t, Detail: func() string { return fmt.Sprintf("  rc=%d out=[%s] err=[%s]", r.RC, r.Out, r.Err) }}
	// run <설정 폴더> <인자…>: 설정 폴더가 -이면 HERDR_TOOLS_CONFIG를 두지 않는다
	run := func(cfg string, args ...string) {
		var env []string
		if cfg != "-" {
			env = []string{"HERDR_TOOLS_CONFIG=" + cfg}
		}
		r = ct.Cmd{Env: env}.Run("bash", append([]string{tool}, args...)...)
	}
	// runIn <폴더> <설정 폴더> <인자…>: (cd 폴더 && HERDR_TOOLS_CONFIG=… bash TOOL …)
	runIn := func(dir, cfg string, args ...string) {
		r = ct.Cmd{Dir: dir, Env: []string{"HERDR_TOOLS_CONFIG=" + cfg}}.Run("bash", append([]string{tool}, args...)...)
	}
	err1 := func() bool { return ct.NewlineCount(r.Err) == 1 }
	errHas := func(s string) bool { return strings.Contains(r.Err, s) }
	put := func(p, s string) { ct.Put(t, p, s) }

	put(T+"/c1/config", "# 주석 줄\n  # 들여쓴 주석\nsync.prebuild   make -C ~/src  prebuild\nbalance.remote\tw1/ w2/\nworker.proxy\nworker.ssh_key ~/.ssh/id_test\n\nBad.Key 무시된다\n")
	put(T+"/c2/config", "balance.remote x/\n")

	// get
	run(T+"/c1", "get", "sync.prebuild")
	c.Ok("get: 값은 줄 끝까지, 앞뒤 빈칸만 뺌", r.RC == 0 && r.Out == "make -C ~/src  prebuild")
	run(T+"/c1", "get", "balance.remote")
	c.Ok("get: 탭으로 나눠도 됨", r.RC == 0 && r.Out == "w1/ w2/")
	run(T+"/c1", "get", "worker.proxy")
	c.Ok("get: 키 혼자 있으면 빈 값, 종료 0", r.RC == 0 && r.Out == "")
	run(T+"/c1", "get", "worker.proxy", "fallback")
	c.Ok("get: 빈 값도 값이라 기본값을 쓰지 않음", r.RC == 0 && r.Out == "")
	run(T+"/c1", "get", "worker.tools", "git node")
	c.Ok("get: 없는 키 + 기본값 → 기본값", r.RC == 0 && r.Out == "git node")
	run(T+"/c1", "get", "worker.tools", "")
	c.Ok("get: 없는 키 + 빈 기본값 → 빈 값, 종료 0", r.RC == 0 && r.Out == "")
	run(T+"/c1", "get", "worker.tools")
	c.Ok("get: 없는 키, 기본값 없음 → 종료 3 + 한 줄 안내", r.RC == 3 && r.Out == "" && err1() && errHas("설정 없음 worker.tools ("+T+"/c1/config — 예: config.example/config)"))
	run(T+"/c2", "get", "balance.remote")
	c.Ok("get: HERDR_TOOLS_CONFIG를 바꾸면 그 폴더를 읽음", r.RC == 0 && r.Out == "x/")
	put(T+"/home/.config/herdr-tools/config", "worker.repo src/p\n")
	run("-", "get", "worker.repo")
	c.Ok("get: 기본 폴더는 ~/.config/herdr-tools", r.RC == 0 && r.Out == "src/p")
	run(T+"/없음", "get", "worker.repo")
	c.Ok("get: 설정 파일 없음 → 종료 3", r.RC == 3 && err1() && errHas("설정 없음 worker.repo ("+T+"/없음/config"))
	run(T+"/없음", "get", "worker.repo", "dflt")
	c.Ok("get: 설정 파일 없음 + 기본값 → 기본값", r.RC == 0 && r.Out == "dflt")
	put(T+"/c2/config", "worker.repo a\nworker.tools git\nworker.repo b\n")
	run(T+"/c2", "get", "worker.tools")
	c.Ok("get: 같은 키 두 번은 다른 키를 물어도 종료 2", r.RC == 2 && err1() && errHas("worker.repo"))
	put(T+"/c2/config", "worker.proxy\r\nworker.repo a b\r\n\r\n# c\r\n")
	run(T+"/c2", "get", "worker.proxy", "fallback")
	c.Ok("get: CRLF config의 키 혼자 줄도 빈 값", r.RC == 0 && r.Out == "")
	run(T+"/c2", "get", "worker.repo")
	c.Ok("get: CRLF config의 값 끝 CR은 뺌", r.RC == 0 && r.Out == "a b")
	run(T+"/c1", "get", "Bad.Key")
	c.Ok("get: 키 이름 꼴이 틀리면 종료 2", r.RC == 2 && err1())
	run(T+"/c1", "get")
	c.Ok("get: 키 없이 부르면 종료 2", r.RC == 2)
	run(T+"/c1", "get", "a.b", "c", "d")
	c.Ok("get: 인자가 넘치면 종료 2", r.RC == 2)

	// file
	sp := T + "/공백 폴더"
	put(sp+"/worker-checklist.txt", "x\n")
	put(sp+"/settings/keys.json", "{}\n")
	run(sp, "file", "worker-checklist.txt")
	c.Ok("file: 있으면 절대 경로, 종료 0", r.RC == 0 && r.Out == sp+"/worker-checklist.txt")
	run(sp, "file", "settings/keys.json")
	c.Ok("file: 하위 폴더 이름도 됨", r.RC == 0 && r.Out == sp+"/settings/keys.json")
	runIn(T, "c1", "file", "config")
	c.Ok("file: 상대 경로 설정 폴더도 절대 경로로", r.RC == 0 && r.Out == T+"/c1/config")
	run(sp, "file", "sync.list")
	c.Ok("file: 없으면 종료 3 + 한 줄 안내", r.RC == 3 && r.Out == "" && err1() && errHas("sync.list"))
	run(sp, "file", "../c1/config")
	c.Ok("file: ..로 폴더 밖은 종료 2", r.RC == 2 && r.Out == "")
	run(sp, "file", "/etc/hosts")
	c.Ok("file: 절대 경로는 종료 2", r.RC == 2 && r.Out == "")
	run(sp, "file", "settings")
	c.Ok("file: 폴더는 파일이 아니라 종료 3", r.RC == 3)

	lintContract(t, c, &r, T, tool, runIn)
	syncLintContract(t, c, &r, T, run, ex)

	// 부름
	run(T + "/c1")
	c.Ok("인자 없으면 종료 2", r.RC == 2)
	run(T+"/c1", "nope")
	c.Ok("모르는 명령은 종료 2", r.RC == 2)

	// config.example: 계약 §3의 키가 모두 있고, 가짜 패턴에 걸리지 않음
	for _, k := range []string{"sync.prebuild", "balance.remote", "worker.repo", "worker.trust", "worker.slots_file", "worker.tools", "worker.ssh_key", "worker.proxy"} {
		run(ex, "get", k)
		c.Ok("config.example: "+k+" 있음", r.RC == 0 && r.Out != "")
	}
	runIn(filepath.Dir(ex), ex, "lint", "config.example/config")
	c.Ok("config.example: 예시 config는 예시 패턴에 안 걸림", r.RC == 0 && r.Out == "")
	c.Done()
}

func lintContract(t *testing.T, c *ct.C, r *ct.Res, T, tool string, runIn func(string, string, ...string)) {
	put := func(p, s string) { ct.Put(t, p, s) }
	has := func(l string) bool { return ct.HasLine(r.Out, l) }
	put(T+"/c1/private-patterns.txt", "# 주석\n\nsecret-host-[0-9]+\n/Users/bob/\n")
	R := T + "/repo"
	put(R+"/a.txt", "ok\nssh secret-host-7 here\n")
	put(R+"/sub/b c.sh", "cd /Users/bob/x\nsecret-host-1\n")
	put(R+"/clean.txt", "clean\n")
	put(R+"/sub/private-patterns.txt", "secret-host-[0-9]+\nsecret-host-9\n")
	put(R+"/untracked.txt", "secret-host-3\n")
	ct.MustGit(t, R, "init", "-q")
	ct.MustGit(t, R, "add", "a.txt", "sub/b c.sh", "clean.txt", "sub/private-patterns.txt")
	ct.MustGit(t, R, "commit", "-qm", "i")
	runIn(R, T+"/c1", "lint")
	c.Ok("lint: 걸리면 종료 1, 파일:줄: 패턴", r.RC == 1 && has("a.txt:2: secret-host-[0-9]+") && has("sub/b c.sh:1: /Users/bob/") && has("sub/b c.sh:2: secret-host-[0-9]+"))
	c.Ok("lint: 인자 없으면 git ls-files만(추적 안 하는 파일은 뺌)", !strings.Contains(r.Out, "untracked") && len(ct.Lines(r.Out)) == 4)
	c.Ok("lint: 쓰는 패턴 파일이 아닌 다른 private-patterns.txt는 검사함", has("sub/private-patterns.txt:2: secret-host-[0-9]+"))
	runIn(R, T+"/c1", "lint", "clean.txt", "sub/private-patterns.txt")
	c.Ok("lint: 파일을 주면 그 파일만(패턴 파일도 준 대로 검사)", r.RC == 1 && r.Out == "sub/private-patterns.txt:2: secret-host-[0-9]+")
	runIn(R, T+"/c1", "lint", "clean.txt")
	c.Ok("lint: 안 걸리면 종료 0, 출력 없음", r.RC == 0 && r.Out == "")
	runIn(R, T+"/c2", "lint")
	c.Ok("lint: 패턴 파일이 둘 다 없으면 종료 0 + 표준 오류 한 줄", r.RC == 0 && r.Out == "" && ct.NewlineCount(r.Err) == 1)
	runIn(R, T+"/c2", "lint", "--strict")
	c.Ok("lint --strict: 패턴 파일이 둘 다 없으면 종료 3 + 표준 오류 한 줄", r.RC == 3 && r.Out == "" && ct.NewlineCount(r.Err) == 1 && strings.Contains(r.Err, "패턴 파일 없음"))
	put(T+"/c2/private-patterns.txt", "# 주석만\n\n")
	runIn(R, T+"/c2", "lint", "--strict")
	c.Ok("lint --strict: 패턴 줄이 없으면 종료 3 + 표준 오류 한 줄", r.RC == 3 && r.Out == "" && ct.NewlineCount(r.Err) == 1)
	runIn(R, T+"/c2", "lint")
	c.Ok("lint: strict 아니면 패턴 줄이 없어도 종료 0", r.RC == 0 && r.Out == "")
	os.Remove(T + "/c2/private-patterns.txt")
	runIn(R, T+"/c1", "lint", "--strict", "clean.txt", "sub/private-patterns.txt")
	c.Ok("lint --strict: 패턴이 있으면 strict 아닌 것과 같음(파일 인자도)", r.RC == 1 && r.Out == "sub/private-patterns.txt:2: secret-host-[0-9]+")
	put(T+"/home/.local/share/herdr-tools/private-patterns.txt", "clean\n")
	runIn(R, T+"/c2", "lint")
	c.Ok("lint: 설정 폴더에 없으면 ~/.local/share/herdr-tools 패턴", r.RC == 1 && r.Out == "clean.txt:0: 경로 clean\nclean.txt:1: clean")
	put(T+"/c2/private-patterns.txt", "ok(\n")
	runIn(R, T+"/c2", "lint", "a.txt")
	c.Ok("lint: 틀린 패턴은 종료 2", r.RC == 2 && strings.Contains(r.Err, "ok("))
	runIn(T, T+"/c1", "lint")
	c.Ok("lint: 인자 없고 git 저장소 밖이면 종료 2", r.RC == 2)
	// 패턴 폴더가 저장소 안(상대 경로로 줌): 쓰는 그 패턴 파일만 뺀다
	R2 := T + "/repo2"
	put(R2+"/conf/private-patterns.txt", "# 예: secret-host-5\nsecret-host-[0-9]+\n")
	put(R2+"/ex/private-patterns.txt", "# 예: secret-host-6\n")
	put(R2+"/x:y.txt", "secret-host-2\n")
	put(R2+"/target.txt", "secret-host-4\n")
	os.Symlink("/Users/bob/private", R2+"/lnk")
	os.Symlink("target.txt", R2+"/lnk2")
	ct.MustGit(t, R2, "init", "-q")
	ct.MustGit(t, R2, "add", "conf", "ex", "x:y.txt", "target.txt", "lnk", "lnk2")
	ct.MustGit(t, R2, "commit", "-qm", "i")
	runIn(R2, "conf", "lint")
	c.Ok("lint: 저장소 안 패턴 파일은 그 하나만 빠지고 다른 패턴 파일은 걸림", r.RC == 1 && !ct.HasPrefixLine(r.Out, "conf/") && has("ex/private-patterns.txt:1: secret-host-[0-9]+"))
	c.Ok("lint: 파일 이름에 :가 있어도 파일:줄: 패턴", has("x:y.txt:1: secret-host-[0-9]+"))
	c.Ok("lint: 심볼릭 링크는 대상 내용을 훑지 않음", !ct.HasPrefixLine(r.Out, "lnk2:") && has("target.txt:1: secret-host-[0-9]+"))
	put(T+"/c2/private-patterns.txt", "/Users/bob/\n")
	runIn(R2, T+"/c2", "lint")
	c.Ok("lint: 심볼릭 링크는 대상 경로 글자를 검사", r.RC == 1 && r.Out == "lnk:1: /Users/bob/")
	os.Symlink("/Users/bob/leak", R2+"/-lnk3")
	ct.MustGit(t, R2, "add", "--", "-lnk3")
	ct.MustGit(t, R2, "commit", "-qm", "l")
	runIn(R2, T+"/c2", "lint")
	c.Ok("lint: 이름이 -로 시작하는 링크도 검사", r.RC == 1 && has("-lnk3:1: /Users/bob/") && r.Err == "")
	os.MkdirAll(T+"/c3", 0o755)
	os.Symlink(R2+"/conf/private-patterns.txt", T+"/c3/private-patterns.txt")
	runIn(R2, T+"/c3", "lint")
	c.Ok("lint: 설정 폴더의 패턴 파일이 저장소 파일로 가는 링크여도 그 파일을 뺌", r.RC == 1 && !ct.HasPrefixLine(r.Out, "conf/") && has("ex/private-patterns.txt:1: secret-host-[0-9]+"))
	put(T+"/c2/private-patterns.txt", "clean\r\n")
	runIn(R, T+"/c2", "lint")
	c.Ok("lint: CRLF 패턴 파일도 줄 끝 CR을 빼고 잡음", r.RC == 1 && r.Out == "clean.txt:0: 경로 clean\nclean.txt:1: clean")

	// UTF-8 아닌 바이트·NUL이 든 파일도 검사한다(macOS grep -I는 통째로 건너뛰었다 — 2026-11-16 publish-public 감사)
	put(R+"/latin1.txt", "caf\xe9 secret-host-1\n")
	put(R+"/blob.bin", "a\x00b\nsecret-host-2\n")
	runIn(R, T+"/c1", "lint", "latin1.txt", "blob.bin")
	c.Ok("lint: UTF-8 아닌 바이트·NUL 파일도 잡음", r.RC == 1 && has("latin1.txt:1: secret-host-[0-9]+") && has("blob.bin:2: secret-host-[0-9]+"))
	put(R+"/secret-host-8.md", "clean\n")
	runIn(R, T+"/c1", "lint", "secret-host-8.md", R+"/clean.txt")
	c.Ok("lint: 상대 경로로 준 파일은 경로 글자도 검사(절대 경로는 내용만)", r.RC == 1 && r.Out == "secret-host-8.md:0: 경로 secret-host-[0-9]+")
	runIn(R, T+"/c1", "lint", "clean.txt", "no-such.txt")
	c.Ok("lint: 못 읽는 파일이 있으면 검사 못 함, 종료 2", r.RC == 2 && strings.Contains(r.Err, "검사 못 함"))
	put(R+"/upper.txt", "SECRET-HOST-3 up\n")
	runIn(R, T+"/c1", "lint", "upper.txt")
	c.Ok("lint: 대소문자를 무시한다", r.RC == 1 && r.Out == "upper.txt:1: secret-host-[0-9]+")
	// 비ASCII(감사 C05/O-05): 바이트 단위·대소문자 무시는 ASCII만(LC_ALL=C grep -iE) — école은 École에 안 걸린다
	put(T+"/c2/private-patterns.txt", "école\n")
	put(R+"/up8.txt", "École here\n")
	put(R+"/lo8.txt", "école here\n")
	put(R+"/École.md", "x\n")
	runIn(R, T+"/c2", "lint", "up8.txt", "École.md")
	c.Ok("lint: 비ASCII 대소문자는 접지 않음(내용·경로)", r.RC == 0 && r.Out == "")
	runIn(R, T+"/c2", "lint", "lo8.txt", "up8.txt")
	c.Ok("lint: 비ASCII 같은 바이트는 잡음", r.RC == 1 && r.Out == "lo8.txt:1: école")
	for _, f := range []string{"latin1.txt", "blob.bin", "secret-host-8.md", "upper.txt", "up8.txt", "lo8.txt", "École.md"} {
		os.Remove(R + "/" + f)
	}

	// 추적 목록 조회가 실패하면 공개용 개인 값 검사는 완료되지 않았다.
	put(T+"/list-failure/git", "#!/bin/sh\n[ \"$1\" != ls-files ] || exit 128\nexec /usr/bin/git \"$@\"\n")
	ct.Chmod(t, T+"/list-failure/git", 0o755)
	*r = ct.Cmd{Dir: R, Env: []string{"PATH=" + T + "/list-failure:" + os.Getenv("PATH"), "HERDR_TOOLS_CONFIG=" + T + "/c1"}}.Run("bash", tool, "lint", "--strict")
	c.Ok("lint: git ls-files가 실패하면 검사 오류", r.RC == 2)
}

func syncLintContract(t *testing.T, c *ct.C, r *ct.Res, T string, run func(string, ...string), ex string) {
	put := func(p, s string) { ct.Put(t, p, s) }
	app := func(p, s string) { ct.Append(t, p, s) }
	// sync-lint: lib/<도구>/가 있는 도구에 bin 줄만 있으면 더할 줄을 찍고 종료 1. 대상이 같거나 all인 lib 나무 줄이면 통과
	os.MkdirAll(T+"/sr/lib/tool-a", 0o755)
	os.MkdirAll(T+"/sr/lib/tool-b", 0o755)
	put(T+"/sl", "# 시험\n도구 .local/bin/tool-a tools:bin/tool-a macos\n도구 .local/bin/tool-b tools:bin/tool-b local\n도구 .local/bin/tool-c tools:bin/tool-c all\n")
	run("-", "sync-lint", T+"/sr", T+"/sl")
	c.Ok("sync-lint: lib 줄 없음 — 도구마다 더할 줄, 종료 1 (lib 폴더 없는 도구는 안 봄)", r.RC == 1 && r.Out == "도구  .local/lib/tool-a/**  tools:lib/tool-a/  macos\n도구  .local/lib/tool-b/**  tools:lib/tool-b/  local" && strings.Contains(r.Err, "lib 나무 줄이 없다"))
	app(T+"/sl", "도구 .local/lib/tool-a/** tools:lib/tool-a/ macos\n도구 .local/lib/tool-b/** tools:lib/tool-b all\n")
	run("-", "sync-lint", T+"/sr", T+"/sl")
	c.Ok("sync-lint: 같은 대상·all lib 줄이 있으면 통과", r.RC == 0 && r.Out == "")
	put(T+"/wrong-dir", strings.ReplaceAll(ct.Read(T+"/sl"), ".local/lib/tool-a/**", "elsewhere/lib/tool-a/**"))
	run("-", "sync-lint", T+"/sr", T+"/wrong-dir")
	c.Ok("sync-lint: 대상 기계가 같아도 로더가 못 찾는 설치 경로는 거절", r.RC == 1 && r.Out == "도구  .local/lib/tool-a/**  tools:lib/tool-a/  macos")
	app(T+"/sl", "도구 .local/bin/tool-a tools:bin/tool-a box-1\n")
	run("-", "sync-lint", T+"/sr", T+"/sl")
	c.Ok("sync-lint: 다른 대상의 bin 줄은 그 대상 lib 줄이 따로 있어야", r.RC == 1 && r.Out == "도구  .local/lib/tool-a/**  tools:lib/tool-a/  box-1")
	run("-", "sync-lint", T+"/sr", T+"/none")
	c.Ok("sync-lint: sync.list 없음은 종료 2", r.RC == 2)
	// 빈 <sync.list> 인자는 기본 경로(셸 판 ${2:-…}, 감사 L01)
	os.MkdirAll(T+"/c4", 0o755)
	put(T+"/c4/sync.list", ct.Read(T+"/sl"))
	run(T+"/c4", "sync-lint", T+"/sr", "")
	c.Ok("sync-lint: 빈 sync.list 인자는 설정 폴더의 sync.list", r.RC == 1 && r.Out == "도구  .local/lib/tool-a/**  tools:lib/tool-a/  box-1")
	// 줄 끝 CR·끝 줄바꿈 없음은 오류(herdr-sync 셸 판이 그 줄을 못 맞추거나 버린다 — 감사 O-10), 두 판 같은 출력
	nl := T + "/nl.list"
	put(nl, "# 시험\n도구 .local/bin/tool-c tools:bin/tool-c all\r\n도구 .local/bin/tool-d tools:bin/tool-d all")
	run("-", "sync-lint", T+"/sr", nl)
	c.Ok("sync-lint: 줄 끝 CR·끝 줄바꿈 없음은 줄 번호와 종료 1", r.RC == 1 && r.Out == "" &&
		r.Err == "herdr-tools-config: "+nl+":2: 줄 끝 CR — 줄바꿈은 LF만\nherdr-tools-config: "+nl+":3: 끝 줄바꿈 없음 — 마지막 줄 뒤에 줄바꿈을 붙인다\n")
	// 함께 쓰는 lib(lib/<이름>/OWNER = 쓰는 도구): hc-lane bin 줄은 hc-merger 나무 줄도 같은 대상으로 요구한다(2026-11-21 merger-go)
	os.MkdirAll(T+"/sd/lib/hc-lane", 0o755)
	put(T+"/sd/lib/hc-merger/OWNER", "hc-lane\n")
	put(T+"/sd.list", "도구 .local/bin/hc-lane tools:bin/hc-lane macos\n도구 .local/lib/hc-lane/** tools:lib/hc-lane/ macos\n")
	run("-", "sync-lint", T+"/sd", T+"/sd.list")
	c.Ok("sync-lint: hc-lane에 hc-merger lib 줄이 없으면 더할 줄", r.RC == 1 && r.Out == "도구  .local/lib/hc-merger/**  tools:lib/hc-merger/  macos")
	app(T+"/sd.list", "도구 .local/lib/hc-merger/** tools:lib/hc-merger/ macos\n")
	run("-", "sync-lint", T+"/sd", T+"/sd.list")
	c.Ok("sync-lint: hc-merger lib 줄이 있으면 통과", r.RC == 0 && r.Out == "")
	// Go 판 도구(cmd/<이름>, OWNER = 그 bin 도구): 그 bin 줄에 Go 소스 나무 줄 넷을 덮는 대상으로 요구한다(2026-11-21 go-foundation)
	os.MkdirAll(T+"/sg/cmd/tool-g", 0o755)
	put(T+"/sg/go.mod", "")
	put(T+"/sg/cmd/hc-merger/OWNER", "hc-lane\n")
	put(T+"/sg.list", "도구 .local/bin/hc-lane tools:bin/hc-lane macos\n도구 .local/bin/tool-g tools:bin/tool-g local\n")
	run("-", "sync-lint", T+"/sg", T+"/sg.list")
	c.Ok("sync-lint: Go 줄 없음 — bin 줄마다 넷, 종료 1", r.RC == 1 && ct.CountNonEmpty(r.Out) == 8 && ct.HasLine(r.Out, "도구  .local/lib/herdr-go/cmd/**  tools:cmd/  macos") && ct.HasLine(r.Out, "도구  .local/lib/go-run.sh  tools:lib/go-run.sh  local") && strings.Contains(r.Err, "Go 소스 나무 줄이 없다"))
	app(T+"/sg.list", "Go .local/lib/herdr-go/go.mod tools:go.mod macos\nGo .local/lib/herdr-go/cmd/** tools:cmd/ macos\nGo .local/lib/herdr-go/internal/** tools:internal/ macos\nGo .local/lib/go-run.sh tools:lib/go-run.sh macos\n")
	run("-", "sync-lint", T+"/sg", T+"/sg.list")
	c.Ok("sync-lint: macos Go 줄은 macos·local bin을 덮는다", r.RC == 0 && r.Out == "")
	app(T+"/sg.list", "도구 .local/bin/tool-g tools:bin/tool-g box-1\n")
	run("-", "sync-lint", T+"/sg", T+"/sg.list")
	c.Ok("sync-lint: 덮지 않는 대상의 bin 줄은 Go 줄이 따로", r.RC == 1 && ct.CountNonEmpty(r.Out) == 4 && ct.HasLine(r.Out, "도구  .local/lib/herdr-go/go.mod  tools:go.mod  box-1"))
	os.Remove(T + "/sg/go.mod")
	run("-", "sync-lint", T+"/sg", T+"/sg.list")
	c.Ok("sync-lint: go.mod 없는 저장소는 Go 줄을 안 봄", r.RC == 0)
	run("-", "sync-lint", filepath.Dir(ex), ex+"/sync.list")
	c.Ok("sync-lint: config.example/sync.list 통과", r.RC == 0 && r.Out == "")
}
