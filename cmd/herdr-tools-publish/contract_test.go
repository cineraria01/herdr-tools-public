package main

// herdr-tools-publish 계약 시험(옛 tests/herdr-tools-publish.test.sh, 2026-11-21 go-tests-ops): 임시 비공개 저장소(와 bare origin),
// 임시 공개 bare 저장소, 가짜 설정 폴더를 두고 bin/herdr-tools-publish를 밖에서 부른다. 네트워크·실제 저장소는 쓰지 않는다.
// 판: HERDR_TOOLS_PUBLISH_IMPL, 없으면 HERDR_IMPL, 없으면 go. 다른 사본은 HERDR_TOOLS_PUBLISH=<경로>(곁에 herdr-tools-config가 있어야 한다)

import (
	"bytes"
	"compress/gzip"
	"os"
	"regexp"
	"strings"
	"testing"

	ct "herdr-tools/internal/clitest"
)

// utf16le: iconv -f UTF-8 -t UTF-16LE (ASCII 글만)
func utf16le(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		b.WriteByte(s[i])
		b.WriteByte(0)
	}
	return b.String()
}

func gz(s string) string {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write([]byte(s))
	w.Close()
	return b.String()
}

// pubFix: 시험 하나의 자리와 도우미
type pubFix struct {
	t        *testing.T
	T, pub   string
	c        *ct.C
	r        ct.Res
	unAuthor []string
}

func (f *pubFix) run(args ...string) { f.runEnv(f.unAuthor, args...) }

func (f *pubFix) runEnv(env []string, args ...string) {
	f.r = ct.Cmd{Env: env, Combined: true}.Run("bash", append([]string{f.pub}, args...)...)
}

func (f *pubFix) has(s string) bool { return strings.Contains(f.r.Out, s) }

// pubGit: git --git-dir=public.git …
func (f *pubFix) pubGit(args ...string) ct.Res {
	return ct.Cmd{}.Run("git", append([]string{"--git-dir=" + f.T + "/public.git"}, args...)...)
}

// pmain: 공개 main 커밋(없으면 빈 글)
func (f *pubFix) pmain() string {
	return f.pubGit("rev-parse", "-q", "--verify", "refs/heads/main").Out
}

func (f *pubFix) priv(args ...string) string { return ct.MustGit(f.t, f.T+"/priv", args...) }

func (f *pubFix) commit(msg string) {
	f.priv("add", "-A")
	f.priv("commit", "-qm", msg)
	f.priv("push", "-q", "origin", "main")
}

func (f *pubFix) privTree() string { return ct.Git(f.T+"/priv", "rev-parse", "origin/main^{tree}") }
func (f *pubFix) pubTree() string  { return f.pubGit("rev-parse", "main^{tree}").Out }

// setConf: sed -i "s|^<키> .*|<줄>|" conf/config — 줄이 없으면 그대로(sed와 같다), 줄이 빈 글이면 지움(/^키/d)
func (f *pubFix) setConf(key, line string) {
	p := f.T + "/conf/config"
	var out []string
	for _, l := range strings.SplitAfter(ct.Read(p), "\n") {
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, key) && (line == "" || strings.HasPrefix(l, key+" ")) {
			if line != "" {
				out = append(out, line+"\n")
			}
			continue
		}
		out = append(out, l)
	}
	ct.Put(f.t, p, strings.Join(out, ""))
}

func TestContract(t *testing.T) {
	pub := ct.Tool(t, "herdr-tools-publish", "HERDR_TOOLS_PUBLISH")
	T := ct.TempDir(t)
	ct.Isolate(t, T)
	t.Setenv("HERDR_TOOLS_PUBLISH_IMPL", ct.Impl("HERDR_TOOLS_PUBLISH_IMPL"))
	for k, v := range map[string]string{"HOME": T + "/home", "HERDR_TOOLS_REPO": T + "/priv", "HERDR_TOOLS_CONFIG": T + "/conf",
		"GIT_CONFIG_NOSYSTEM": "1", "GIT_AUTHOR_NAME": "priv", "GIT_AUTHOR_EMAIL": "priv-author@private.example",
		"GIT_COMMITTER_NAME": "priv", "GIT_COMMITTER_EMAIL": "priv-author@private.example",
		"TMPDIR": T + "/tmp"} { // 이 시험의 임시 폴더만 본다(같은 기계의 다른 내보내기와 겹치지 않게)
		t.Setenv(k, v)
	}
	for _, d := range []string{T + "/home", T + "/conf", T + "/tmp"} {
		os.MkdirAll(d, 0o755)
	}
	f := &pubFix{t: t, T: T, pub: pub, unAuthor: []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL"}}
	f.c = &ct.C{T: t, Detail: func() string {
		ls := ct.Lines(f.r.Out)
		if len(ls) > 20 {
			ls = ls[len(ls)-20:]
		}
		return "    | " + strings.Join(ls, "\n    | ")
	}}
	c, put := f.c, func(p, s string) { ct.Put(t, p, s) }

	// 비공개 저장소: 커밋 둘(메시지에 비밀 낱말), 실행 파일·링크·하위 폴더
	ct.MustGit(t, T, "init", "-q", "--bare", "-b", "main", T+"/origin.git")
	ct.MustGit(t, T, "init", "-q", "--bare", "-b", "main", T+"/public.git")
	ct.MustGit(t, T, "clone", "-q", T+"/origin.git", T+"/priv")
	put(T+"/priv/bin/a", "#!/bin/sh\necho a\n")
	ct.Chmod(t, T+"/priv/bin/a", 0o755)
	put(T+"/priv/docs/d.md", "doc\n")
	put(T+"/priv/docs/old.md", "gone\n")
	os.Symlink("../docs/d.md", T+"/priv/bin/link")
	f.commit("private-msg-one")
	put(T+"/priv/docs/d.md", "doc2\n")
	f.commit("private-msg-two")
	put(T+"/conf/private-patterns.txt", "secret-word\n")
	put(T+"/conf/config", "publish.repo    "+T+"/public.git\npublish.dir     ~/pubclone\npublish.author  Pub Bot <bot@public.example>\n")

	// 1. 설정 없음·검사 도구 없음
	os.Rename(T+"/conf/config", T+"/conf/config.keep")
	f.run()
	c.Ok("publish.repo 없음: 종료 2, 공개 쪽 그대로", f.r.RC == 2 && f.has("publish.repo") && f.pmain() == "")
	os.Rename(T+"/conf/config.keep", T+"/conf/config")
	b, _ := os.ReadFile(pub)
	put(T+"/lone/herdr-tools-publish", string(b))
	f.r = ct.Cmd{Combined: true}.Run("bash", T+"/lone/herdr-tools-publish")
	// 셸 판은 곁에 검사 도구가 있어야 한다. Go 판은 검사가 바이너리 안에 있고, 홀로 둔 사본은 공통 실행기가 없어 Go 판을 못 쓴다(못박았으면 종료 1)
	c.Ok("곁에 검사 도구(herdr-tools-config)·공통 실행기 없음: 종료 1, push 없음", f.r.RC == 1 && (f.has("herdr-tools-config") || f.has("공통 실행기 없음")) && f.pmain() == "")
	os.Rename(T+"/conf/private-patterns.txt", T+"/pp.keep")
	f.run()
	c.Ok("개인 패턴 파일 없음: 검사 못 함 → 종료 1, push 없음", f.r.RC == 1 && f.has("검사 못 함") && f.pmain() == "")
	put(T+"/conf/private-patterns.txt", "# 주석만\n")
	f.run()
	c.Ok("패턴 줄 없음: 종료 1, push 없음", f.r.RC == 1 && f.has("검사 못 함") && f.pmain() == "")
	os.Rename(T+"/pp.keep", T+"/conf/private-patterns.txt")

	// 2. --dry-run: 바뀔 파일 목록·검사 결과만, push 없음
	f.run("--dry-run")
	c.Ok("--dry-run: 파일 목록과 검사 통과, push 없음", f.r.RC == 0 && f.has("A\tbin/a") && f.has("A\tdocs/old.md") && f.has("검사 통과") && f.pmain() == "")

	// 3. 첫 내보냄: 커밋 하나, 기록·비공개 메시지·이메일 없음, 트리는 비공개 main과 같음
	f.run()
	psha := ct.Git(T+"/priv", "rev-parse", "--short", "origin/main")
	c.Ok("첫 내보냄: 종료 0", f.r.RC == 0 && f.has("내보냄"))
	c.Ok("공개 쪽 커밋 하나", f.pubGit("rev-list", "--count", "main").Out == "1")
	c.Ok("메시지는 「herdr-tools <sha> 내보냄」 한 줄", f.pubGit("log", "-1", "--format=%B", "main").Out == "herdr-tools "+psha+" 내보냄")
	c.Ok("작성자는 publish.author", f.pubGit("log", "-1", "--format=%an <%ae>|%cn <%ce>", "main").Out == "Pub Bot <bot@public.example>|Pub Bot <bot@public.example>")
	privRx := regexp.MustCompile("private-msg|priv-author")
	c.Ok("비공개 메시지·이메일 없음 (git log -p)", !privRx.MatchString(f.pubGit("log", "-p", "--format=fuller", "main").Out))
	c.Ok("트리가 비공개 origin/main과 같음 (실행 비트·링크 포함)", f.pubTree() == f.privTree())
	c.Ok("publish.dir의 ~는 집", ct.IsDir(T+"/home/pubclone"))

	// 4. 바뀐 것 없음 → 커밋 없음
	h := f.pmain()
	f.run()
	c.Ok("바뀐 것 없음: 「같음」, 커밋 없음", f.r.RC == 0 && f.has("같음") && f.pmain() == h)

	// 5. 파일 지움 반영, 부모는 앞 공개 커밋 (기록 없이 이어짐)
	f.priv("rm", "-q", "docs/old.md")
	f.priv("commit", "-qm", "private-msg-three")
	f.priv("push", "-q", "origin", "main")
	f.run()
	c.Ok("지움 반영: 공개 트리에 old.md 없음, 커밋 둘", f.r.RC == 0 && f.pubGit("cat-file", "-e", "main:docs/old.md").RC != 0 && f.pubGit("rev-list", "--count", "main").Out == "2" && f.pubGit("rev-parse", "main^").Out == h)

	// 6. 개인 패턴 걸림 → push 없음, 파일:줄, 종료 1
	h = f.pmain()
	put(T+"/priv/docs/leak.md", "x\nhas secret-word here\n")
	f.commit("leak")
	f.run()
	c.Ok("개인 패턴: 종료 1, 파일:줄, push 없음", f.r.RC == 1 && f.has("docs/leak.md:2:") && f.pmain() == h)
	f.run("--dry-run")
	c.Ok("개인 패턴 --dry-run: 종료 1과 걸린 줄", f.r.RC == 1 && f.has("docs/leak.md:2:"))
	f.priv("rm", "-q", "docs/leak.md")
	f.commit("unleak")

	secretsContract(f, h)
	overlapContract(f)
	blobContract(f)

	// 8. publish.dir에 다른 저장소가 있으면 건드리지 않는다
	ct.MustGit(t, T, "init", "-q", "--bare", T+"/other.git")
	f.setConf("publish.dir", "publish.dir "+T+"/other.git")
	f.run()
	c.Ok("publish.dir의 origin이 다름: 종료 1", f.r.RC == 1 && f.has("origin"))

	c.Ok("임시 파일이 남지 않는다", ct.Empty(T+"/tmp"))
	c.Done()
}

// 시험 파일 자체에 비밀 꼴이 없게 조각을 이어 만든다
const (
	ghp = "ghp_"
	key = "-----BEGIN"
	a16 = "Zk3mQ9vT2pLx8RwN"
	b16 = "4bYc7HdJ1sFgU6eA"
	h20 = "9f86d081884c7d659a2f"
)

// 7. 비밀 꼴 걸림 → 종료 1 (값은 찍지 않는다), 7-2. 감사 반려(2026-11-16) 재현
func secretsContract(f *pubFix, h string) {
	t, T, c := f.t, f.T, f.c
	put := func(p, s string) { ct.Put(t, p, s) }
	for _, s := range []string{"token: " + ghp + a16 + b16 + a16, key + " OPENSSH PRIVATE KEY-----", "AK" + "IAABCDEFGHIJKLMNOP",
		"x = " + a16 + b16, "hex " + h20 + h20, "slack xo" + "xb-1234567890-abcdefghij", "sk = s" + "k-" + a16 + b16} {
		put(T+"/priv/docs/sec.md", "ok\n"+s+"\n")
		f.commit("sec")
		f.run()
		c.Ok("비밀 꼴 ("+s[:6]+"…): 종료 1, 파일:줄, 값은 안 찍음, push 없음", f.r.RC == 1 && f.has("docs/sec.md:2:") && !f.has(s) && f.pmain() == h)
	}
	put(T+"/priv/docs/sec.md", "customtok-ZZZ\n")
	f.commit("sec")
	f.run()
	c.Ok("설정에 없는 꼴은 통과", f.r.RC == 0)
	h = f.pmain()
	ct.Append(t, T+"/conf/config", "publish.secrets customtok-[A-Z]{3}\n")
	put(T+"/priv/docs/sec.md", "customtok-YYY\n")
	f.commit("sec")
	f.run()
	c.Ok("설정 키 publish.secrets로 더한 꼴: 종료 1", f.r.RC == 1 && f.has("docs/sec.md:1:") && f.pmain() == h)
	f.priv("rm", "-q", "docs/sec.md")
	f.commit("unsec")
	put(T+"/priv/docs/ok.md", "HOME/Library/Logs/herdr-core-restart\nABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789/_\n")
	f.commit("ok")
	f.run()
	c.Ok("흔한 글자(경로·알파벳 목록)는 비밀로 안 봄", f.r.RC == 0)

	// 7-2. UTF-8 아닌 바이트·NUL 파일, 파일 이름, 링크 대상, export-subst, 작성자
	h = f.pmain()
	put(T+"/priv/docs/l1.txt", "caf\xe9 secret-word\n")
	f.commit("l1")
	f.run()
	c.Ok("Latin-1 파일의 개인 값: 종료 1, push 없음", f.r.RC == 1 && f.has("docs/l1.txt:1:") && f.pmain() == h)
	put(T+"/priv/docs/l1.txt", "caf\xe9 "+"AK"+"IAABCDEFGHIJKLMNOP\n")
	f.commit("l1")
	f.run()
	c.Ok("Latin-1 파일의 비밀 꼴: 종료 1", f.r.RC == 1 && f.has("docs/l1.txt:1:") && f.pmain() == h)
	f.priv("rm", "-q", "docs/l1.txt")
	put(T+"/priv/docs/b.bin", "a\x00b\nsecret-word\n")
	f.commit("nul")
	f.run()
	c.Ok("NUL 파일의 개인 값: 종료 1", f.r.RC == 1 && f.has("docs/b.bin:2:") && f.pmain() == h)
	f.priv("rm", "-q", "docs/b.bin")
	put(T+"/priv/docs/notes-secret-word.md", "x\n")
	f.commit("name")
	f.run()
	c.Ok("파일 이름의 개인 값: 종료 1", f.r.RC == 1 && f.has("docs/notes-secret-word.md:0:") && f.pmain() == h)
	f.priv("rm", "-q", "docs/notes-secret-word.md")
	put(T+"/priv/docs/"+a16+b16+".md", "x\n")
	f.commit("name")
	f.run()
	c.Ok("파일 이름의 비밀 꼴: 종료 1, 이름은 안 찍음", f.r.RC == 1 && !f.has(a16+b16) && f.pmain() == h)
	f.priv("rm", "-q", "docs/"+a16+b16+".md")
	os.Symlink("../"+a16+b16, T+"/priv/docs/lnk")
	f.commit("lnk")
	f.run()
	c.Ok("링크 대상 글자의 비밀 꼴: 종료 1", f.r.RC == 1 && f.pmain() == h)
	f.priv("rm", "-q", "docs/lnk")
	f.commit("unlnk")
	put(T+"/priv/.gitattributes", "ver.txt export-subst\n")
	put(T+"/priv/ver.txt", "v $Format:%ae %s$\n")
	f.commit("private-subject")
	f.run()
	c.Ok("export-subst: 비공개 메일·제목이 들어가지 않고 블롭 그대로", f.r.RC == 0 && f.pubGit("show", "main:ver.txt").Out == "v $Format:%ae %s$" && !regexp.MustCompile("private-subject|priv-author").MatchString(f.pubGit("log", "-p", "main").Out) && f.pubTree() == f.privTree())
	put(T+"/priv/docs/more.md", "more\n")
	f.commit("more")
	f.r = ct.Cmd{Combined: true, Env: []string{"GIT_AUTHOR_EMAIL=secret-word@x", "GIT_COMMITTER_EMAIL=secret-word@x"}}.Run("bash", f.pub)
	c.Ok("환경의 GIT_AUTHOR_*·GIT_COMMITTER_*는 안 씀(publish.author만)", f.r.RC == 0 && f.pubGit("log", "-1", "--format=%ae %ce", "main").Out == "bot@public.example bot@public.example")
	put(T+"/priv/docs/more.md", "more2\n")
	f.commit("more")
	h = f.pmain()
	f.setConf("publish.author", "")
	put(T+"/home/.gitconfig", "[user]\n\tname = me\n\temail = me@secret-word.example\n")
	f.run()
	c.Ok("publish.author 없음 + 전역 메일이 개인 패턴에 걸림: 종료 1, push 없음", f.r.RC == 1 && f.has("작성자") && f.pmain() == h)
	put(T+"/home/.gitconfig", "[user]\n\tname = me\n\temail = me@ok.example\n")
	f.run()
	c.Ok("publish.author 없음 + 깨끗한 전역 메일: 내보냄", f.r.RC == 0 && f.pubGit("log", "-1", "--format=%ae", "main").Out == "me@ok.example")
	ct.Append(t, T+"/conf/config", "publish.author  Pub Bot <bot@public.example>\n")
	f.setConf("publish.dir", "publish.dir "+T+"/fresh-dir")
	f.run("--dry-run")
	c.Ok("--dry-run: publish.dir을 만들지 않는다", f.r.RC == 0 && !ct.Lexists(T+"/fresh-dir"))
}
