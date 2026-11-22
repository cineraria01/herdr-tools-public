package main

// 재감사 ops R-27: 실제 게시기(bin/herdr-tools-publish)에 무해한 낱말 경계 패턴(\< \>)을 주고 차단·통과와 로컬 미러(publish.dir) 불변을 본다.
// ERE 단위 시험(internal/toolcfg)만으로는 게시기 매처 호출부가 옛 꼴로 돌아가도 잡지 못했다(인계 직전 판은 낱말 경계 사례를 통과시켰다)

import (
	"os"
	"strings"
	"testing"

	ct "herdr-tools/internal/clitest"
)

func TestPublishWordBoundary(t *testing.T) {
	pub := ct.Tool(t, "herdr-tools-publish", "HERDR_TOOLS_PUBLISH")
	T := ct.TempDir(t)
	ct.Isolate(t, T)
	for k, v := range map[string]string{"HOME": T + "/home", "HERDR_TOOLS_REPO": T + "/priv", "HERDR_TOOLS_CONFIG": T + "/conf",
		"GIT_CONFIG_NOSYSTEM": "1", "GIT_AUTHOR_NAME": "priv", "GIT_AUTHOR_EMAIL": "priv-author@private.example",
		"GIT_COMMITTER_NAME": "priv", "GIT_COMMITTER_EMAIL": "priv-author@private.example", "TMPDIR": T + "/tmp"} {
		t.Setenv(k, v)
	}
	for _, d := range []string{T + "/home", T + "/conf", T + "/tmp"} {
		os.MkdirAll(d, 0o755)
	}
	f := &pubFix{t: t, T: T, pub: pub, unAuthor: []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL"}}
	f.c = &ct.C{T: t, Detail: func() string { return "    | " + strings.Join(ct.Lines(f.r.Out), "\n    | ") }}
	c, put := f.c, func(p, s string) { ct.Put(t, p, s) }

	ct.MustGit(t, T, "init", "-q", "--bare", "-b", "main", T+"/origin.git")
	ct.MustGit(t, T, "init", "-q", "--bare", "-b", "main", T+"/public.git")
	ct.MustGit(t, T, "clone", "-q", T+"/origin.git", T+"/priv")
	put(T+"/priv/docs/d.md", "doc\n")
	f.commit("first")
	// 개인 값(lint, 대소문자 무시)과 publish.secrets(대소문자 가림) 둘 다 낱말 경계 꼴
	put(T+"/conf/private-patterns.txt", `\<pvtok\>`+"\n")
	put(T+"/conf/config", "publish.repo    "+T+"/public.git\npublish.dir     ~/pubclone\npublish.author  Pub Bot <bot@public.example>\n"+
		`publish.secrets \<wbtok[0-9]{3}\>`+"\n")
	f.run()
	c.Ok("첫 내보냄: 종료 0", f.r.RC == 0 && f.has("내보냄"))

	// 낱말 안에 묻힌 글자는 경계가 없어 걸리지 않는다(경계를 빼고 옮기면 여기서 막힌다)
	put(T+"/priv/docs/wb.md", "awbtok123b xpvtokx\n")
	f.commit("inside")
	f.run()
	c.Ok("낱말 안의 wbtok123·pvtok: 경계 없음 → 통과(종료 0)", f.r.RC == 0 && f.has("내보냄"))

	mirror := func() string { return ct.Snap(T+"/home", "", "pubclone") }
	h, m := f.pmain(), mirror()
	for _, s := range []string{"id wbtok123 here", "a PVTOK b"} {
		put(T+"/priv/docs/wb.md", s+"\n")
		f.commit("bound")
		f.run()
		c.Ok("낱말 경계 꼴 ("+s+"): 종료 1, 파일:줄, 값은 안 찍음(비밀 꼴), 공개 main·로컬 미러 그대로",
			f.r.RC == 1 && f.has("docs/wb.md:1:") && !f.has("wbtok123") && f.pmain() == h && mirror() == m)
	}
	c.Ok("임시 파일이 남지 않는다", ct.Empty(T+"/tmp"))
	c.Done()
}
