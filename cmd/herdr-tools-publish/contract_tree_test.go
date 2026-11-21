package main

// herdr-tools-publish 계약 시험 뒤쪽: 경로 겹침(7-3)·글이 아닌 블롭·replace ref(7-4)·빈 하위 트리·원격 이름(7-5)

import (
	"regexp"
	"strings"

	ct "herdr-tools/internal/clitest"
)

// g: git -C priv -c core.ignorecase=false -c core.precomposeunicode=false … (입력 in)
func (f *pubFix) g(in string, args ...string) string {
	r := ct.Cmd{Stdin: in}.Run("git", append([]string{"-C", f.T + "/priv", "-c", "core.ignorecase=false", "-c", "core.precomposeunicode=false"}, args...)...)
	if r.RC != 0 {
		f.t.Fatalf("git %s: %d %s", strings.Join(args, " "), r.RC, r.Err)
	}
	return r.Out
}

// ix <모드> <경로> <내용>: 작업 트리로는 못 만드는 트리라 index에 직접 넣는다
func (f *pubFix) ix(mode, path, content string) {
	f.g("", "update-index", "--add", "--cacheinfo", mode+","+f.g(content, "hash-object", "-w", "--stdin")+","+path)
}

func (f *pubFix) icommit(msg string) {
	f.g("", "commit", "-qm", msg)
	f.g("", "push", "-q", "origin", "main")
}

func (f *pubFix) iclean(paths ...string) {
	f.g("", append([]string{"rm", "-q", "--cached", "--"}, paths...)...)
	f.icommit("clean")
}

// 7-3. 재감사(2026-11-16) 재현: 대소문자·정규화만 다른 경로가 macOS 디스크에서 겹쳐 한쪽이 검사 없이 나가던 것
func overlapContract(f *pubFix) {
	t, T, c := f.t, f.T, f.c
	h := f.pmain()
	f.ix("100644", "Notes.txt", "secret-word\n")
	f.ix("100644", "notes.txt", "ok\n")
	f.icommit("case")
	f.run()
	pair := regexp.MustCompile(`^경로가 겹친다.*: [0-9]+번째·[0-9]+번째 파일$`)
	bare := regexp.MustCompile(`^경로가 겹친다.*: 파일$`)
	matchLine := func(rx *regexp.Regexp) bool {
		for _, l := range ct.Lines(f.r.Out) {
			if rx.MatchString(l) {
				return true
			}
		}
		return false
	}
	c.Ok("대소문자만 다른 두 파일: 종료 1, push 없음, 안내는 한 줄", f.r.RC == 1 && matchLine(pair) && !matchLine(bare) && f.pmain() == h)
	f.iclean("Notes.txt", "notes.txt")
	f.g("x\n", "hash-object", "-w", "--stdin")
	f.ix("120000", "A", "x.txt")
	f.ix("100644", "a", "secret-word\n")
	f.icommit("linkcase")
	f.run()
	c.Ok("링크 A + 파일 a: 종료 1, push 없음", f.r.RC == 1 && f.pmain() == h)
	f.iclean("A", "a")
	ct.Put(t, T+"/outside", "keep\n")
	f.ix("120000", "D", T)
	f.ix("100644", "d/outside", "secret-word\n")
	f.icommit("linkdir")
	f.run()
	c.Ok("링크 폴더 D + d/아래 파일: 종료 1, push 없음, 바깥 파일 그대로", f.r.RC == 1 && f.pmain() == h && ct.Is(T+"/outside", "keep"))
	f.iclean("D", "d/outside")
	nfd, nfc := "cafe\xcc\x81.md", "caf\xc3\xa9.md"
	f.ix("100644", nfd, "secret-word\n")
	f.ix("100644", nfc, "ok\n")
	f.icommit("nfd")
	f.run()
	c.Ok("NFC·NFD만 다른 두 파일: 종료 1, push 없음", f.r.RC == 1 && f.has("겹") && f.pmain() == h)
	f.iclean(nfd, nfc)
	f.ix("100644", "Docs/z.md", "ok\n")
	f.icommit("dircase")
	f.run()
	c.Ok("폴더 이름 대소문자만 다름(Docs·docs): 종료 1", f.r.RC == 1 && f.has("겹") && f.pmain() == h)
	f.iclean("Docs/z.md")
	f.run()
	c.Ok("겹침을 뺀 뒤: 내보냄", f.r.RC == 0)
	h = f.pmain()
	ct.Git(T+"/priv", "checkout", "-q", "--", ".")
	ct.Put(t, T+"/priv/docs/s1.md", "secret-word\n")
	ct.Put(t, T+"/priv/docs/s2.md", "secret-word\n")
	f.commit("same")
	f.run()
	c.Ok("같은 블롭 두 경로: 둘 다 찍음", f.r.RC == 1 && f.has("docs/s1.md:1:") && f.has("docs/s2.md:1:") && f.pmain() == h)
	f.priv("rm", "-q", "docs/s1.md", "docs/s2.md")
	ct.Put(t, T+"/priv/docs/p.md", "p\n")
	f.commit("pushurl")
	ct.MustGit(t, T, "init", "-q", "--bare", T+"/other-push.git")
	ct.MustGit(t, T, "--git-dir="+T+"/fresh-dir", "config", "remote.origin.pushurl", T+"/other-push.git")
	f.run()
	c.Ok("공개 clone의 pushurl을 따르지 않고 publish.repo로 push", f.r.RC == 0 && f.pmain() != h && !ct.GitOK(T, "--git-dir="+T+"/other-push.git", "rev-parse", "-q", "--verify", "refs/heads/main"))
}

// 7-4. 3차 감사(2026-11-16) 재현: UTF-16·압축 블롭 속 값은 바이트 grep을 통과했다 → 글이 아닌 블롭(NUL·UTF-8 아님)은 막는다.
// 7-5. 4차 감사(2026-11-16) 재현: 빈 하위 트리의 이름은 ls-tree -r에 안 나와 검사를 비켜 공개됐다 → 트리 이름도 검사, 빈 트리는 막음
func blobContract(f *pubFix) {
	t, T, c := f.t, f.T, f.c
	put := func(p, s string) { ct.Put(t, p, s) }
	h := f.pmain()
	put(T+"/priv/docs/w.txt", utf16le("note secret-word\n"))
	f.commit("u16")
	f.run()
	c.Ok("UTF-16 개인 값: 종료 1, push 없음, 경로 찍음", f.r.RC == 1 && f.has("docs/w.txt:0: 글이 아닌 블롭") && f.pmain() == h)
	f.priv("rm", "-q", "docs/w.txt")
	put(T+"/priv/docs/w.gz", gz("note secret-word\n"))
	f.commit("gz")
	f.run()
	c.Ok("gzip 개인 값: 종료 1, push 없음", f.r.RC == 1 && f.has("docs/w.gz:0: 글이 아닌 블롭") && f.pmain() == h)
	f.priv("rm", "-q", "docs/w.gz")
	put(T+"/priv/docs/t.txt", utf16le("tok "+ghp+a16+b16+a16+"\n"))
	f.commit("u16tok")
	f.run()
	c.Ok("UTF-16 비밀 꼴: 종료 1, push 없음", f.r.RC == 1 && f.has("docs/t.txt:0: 글이 아닌 블롭") && f.pmain() == h)
	f.priv("rm", "-q", "docs/t.txt")
	f.commit("unbin")
	f.run()
	c.Ok("글이 아닌 블롭을 뺀 뒤: 내보냄", f.r.RC == 0)
	h = f.pmain()
	// replace ref: 검사(cat-file)와 옮기기(pack-objects)가 다른 블롭을 보면 안 된다
	put(T+"/priv/docs/rep.md", "secret-word\n")
	f.commit("rep")
	badBlob := f.priv("rev-parse", "HEAD:docs/rep.md")
	okBlob := ct.Cmd{Stdin: "ok\n"}.Run("git", "-C", T+"/priv", "hash-object", "-w", "--stdin").Out
	f.priv("replace", badBlob, okBlob)
	f.run()
	c.Ok("replace ref가 있어도 진짜 블롭을 검사: 종료 1, push 없음", f.r.RC == 1 && f.pmain() == h)
	f.priv("replace", "-d", badBlob)
	// --dry-run은 막힌 블롭을 실제 publish.dir에 쓰지 않는다
	f.run("--dry-run")
	c.Ok("--dry-run: 실제 publish.dir에 아무것도 안 씀", f.r.RC == 1 && !ct.GitOK(T, "--git-dir="+T+"/fresh-dir", "cat-file", "-e", badBlob))
	f.priv("rm", "-q", "docs/rep.md")
	f.commit("unrep")

	// 7-5.
	h = f.pmain()
	withtree := func(name string) { // 지금 HEAD 트리에 빈 하위 트리 <이름>을 더한 커밋을 push
		empty := ct.Cmd{}.Run("git", "-C", T+"/priv", "hash-object", "-t", "tree", "/dev/null").Out
		in := f.priv("ls-tree", "HEAD") + "\n040000 tree " + empty + "\t" + name + "\n"
		tr := ct.Cmd{Stdin: in}.Run("git", "-C", T+"/priv", "mktree").Out
		f.priv("update-ref", "HEAD", f.priv("commit-tree", tr, "-p", "HEAD", "-m", "emptytree"))
		f.priv("push", "-q", "origin", "main")
	}
	untree := func() {
		f.priv("update-ref", "HEAD", f.priv("commit-tree", f.priv("rev-parse", "HEAD~1^{tree}"), "-p", "HEAD", "-m", "untree"))
		f.priv("push", "-q", "origin", "main")
	}
	withtree("secret-word-dir")
	f.run()
	c.Ok("이름이 개인 값인 빈 하위 트리: 종료 1, push 없음", f.r.RC == 1 && f.pmain() == h)
	untree()
	withtree("x" + a16 + b16)
	f.run()
	c.Ok("이름이 비밀 꼴인 빈 하위 트리: 종료 1, 이름 안 찍음", f.r.RC == 1 && !f.has(a16+b16) && f.pmain() == h)
	untree()
	withtree("plain-empty")
	f.run()
	c.Ok("깨끗한 이름이라도 빈 하위 트리는 막음", f.r.RC == 1 && f.has("빈 하위 트리") && f.pmain() == h)
	untree()
	f.run()
	c.Ok("빈 하위 트리를 뺀 뒤: 같음 또는 내보냄", f.r.RC == 0)
	// origin/main은 refs/remotes/origin/main 전체 이름으로 (같은 이름의 로컬 가지에 속지 않음)
	ct.Git(T+"/priv", "branch", "origin/main", "HEAD~3")
	put(T+"/priv/docs/r.md", "r\n")
	f.commit("remote")
	f.run()
	c.Ok("로컬 가지 origin/main이 있어도 원격 origin/main 트리를 내보냄", f.r.RC == 0 && f.pubTree() == ct.Git(T+"/priv", "rev-parse", "refs/remotes/origin/main^{tree}"))
	f.priv("branch", "-q", "-D", "origin/main")
	h = f.pmain()
	// 개인 패턴은 대소문자를 무시한다
	put(T+"/priv/docs/up.md", "Secret-Word here\n")
	f.commit("up")
	f.run()
	c.Ok("개인 값 대소문자 달라도 막음", f.r.RC == 1 && f.has("docs/up.md:1:") && f.pmain() == h)
	f.priv("rm", "-q", "docs/up.md")
	f.commit("unup")
}
