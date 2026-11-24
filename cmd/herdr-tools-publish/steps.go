package main

// main(main.go)의 단계들(2026-11-24 long-func-sync — 414줄 한 함수를 단계별로): 설정 → 원본 트리 → 블롭 꺼내기 → 검사(겹침·글·개인 값·비밀 꼴)
// → 공개 git 폴더 → 작성자 검사 → 내보내기. 읽기·쓰기 없는 판정(splitAuthor·snap.label)은 꾸러미 안 단위 시험(steps_test.go)으로 잰다

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"herdr-tools/internal/gitx"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/toolcfg"
)

// pubCfg: 설정과 환경에서 정한 값
type pubCfg struct{ repo, conf, url, dir, extra string }

// splitAuthor: publish.author 「이름 <메일>」 → 이름·메일. 꼴이 틀리면 ok 거짓
func splitAuthor(a string) (name, mail string, ok bool) {
	i := strings.LastIndex(a, " <")
	if i < 0 || !strings.HasSuffix(a, ">") {
		return "", "", false
	}
	return a[:i], strings.TrimSuffix(a[i+2:], ">"), true
}

// loadConfig: 저장소·설정 폴더·publish.* 설정을 읽고 git 환경을 맞춘다. publish.repo가 없으면 종료 2
func loadConfig() pubCfg {
	home := os.Getenv("HOME")
	repo := os.Getenv("HERDR_TOOLS_REPO")
	if repo == "" {
		repo = home + "/Documents/app/herdr-tools"
	}
	conf := toolcfg.Dir()
	// 공개 커밋 작성자는 명시값만: 부른 쪽 환경의 작성자 변수는 지우고, git이 호스트 이름으로 지어내지 못하게 한다(useConfigOnly).
	// push·fetch가 인증 질문에서 멈추지 않게, replace ref를 따르지 않게(검사한 블롭 = 옮기는 블롭)
	for _, v := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_AUTHOR_DATE", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "GIT_COMMITTER_DATE", "EMAIL"} {
		os.Unsetenv(v)
	}
	os.Setenv("GIT_TERMINAL_PROMPT", "0")
	if os.Getenv("GIT_SSH_COMMAND") == "" {
		os.Setenv("GIT_SSH_COMMAND", "ssh -o BatchMode=yes")
	}
	os.Setenv("GIT_NO_REPLACE_OBJECTS", "1")
	cfg := func(k, def string) string {
		v, err := toolcfg.Lookup(conf, k, def, os.Stderr)
		if err != nil {
			die("설정을 읽지 못했다: %s/config", conf)
		}
		return v
	}
	url := cfg("publish.repo", "")
	if url == "" {
		fmt.Fprintf(os.Stderr, "%s: 설정 없음 publish.repo (%s/config — 예: config.example/config)\n", me, conf)
		os.Exit(2)
	}
	dir := cfg("publish.dir", "~/.local/share/herdr-tools/public")
	author := cfg("publish.author", "")
	extra := cfg("publish.secrets", "")
	if dir == "~" {
		dir = home
	} else if strings.HasPrefix(dir, "~/") {
		dir = home + "/" + dir[2:]
	}
	if author != "" {
		name, mail, ok := splitAuthor(author)
		if !ok {
			die("publish.author 꼴: 이름 <메일>")
		}
		os.Setenv("GIT_AUTHOR_NAME", name)
		os.Setenv("GIT_AUTHOR_EMAIL", mail)
		os.Setenv("GIT_COMMITTER_NAME", name)
		os.Setenv("GIT_COMMITTER_EMAIL", mail)
	}
	return pubCfg{repo, conf, url, dir, extra}
}

// sourceTree: origin/main을 받아 그 커밋·트리·짧은 이름
func sourceTree(repo string) (src, tree, short string) {
	if _, ok := git("-C", repo, "fetch", "-q", "origin", "main"); !ok {
		die("fetch 실패: %s", repo)
	}
	src, _ = git("-C", repo, "rev-parse", "-q", "--verify", "refs/remotes/origin/main^{commit}") // 전체 이름: 같은 이름의 로컬 가지·태그에 속지 않게
	if src == "" {
		die("origin/main 없음: %s", repo)
	}
	tree, ok := git("-C", repo, "rev-parse", src+"^{tree}")
	if !ok {
		die("origin/main 트리를 읽지 못했다")
	}
	short, _ = git("-C", repo, "rev-parse", "--short", src)
	return src, tree, short
}

// snap: 꺼낸 트리 — 검사할 파일과 옮길 객체
type snap struct {
	bdir, namesFile string              // <임시>/b(블롭 <sha>) · <임시>/names(줄 i = names[i-1])
	paths, tpaths   []string            // 파일 경로 · 하위 트리 이름
	names           []string            // paths 뒤에 tpaths
	shas            []string            // 옮길 객체(블롭·하위 트리)
	bpaths          map[string][]string // 블롭 sha → 경로들
	blobFiles       []string            // <임시>/b/<sha>, 정렬
	bad             bool                // 빈 하위 트리가 있다
}

// readTree: 트리의 블롭마다 검사하려고 꺼낸다. 경로대로 디스크에 꺼내지 않는다(macOS 디스크는 대소문자·정규화를 구분하지 않아 경로가 겹치고
// 링크를 따라 바깥에 쓴다). 블롭은 <임시>/b/<sha>로만 꺼낸다: 검사한 바이트 = 공개로 가는 객체. -t: 하위 트리 이름도(빈 하위 트리는 -r만으로 안 나옴)
func readTree(repo, tree string) *snap {
	var lsz bytes.Buffer
	if gitx.Run("", &lsz, os.Stderr, "-C", repo, "ls-tree", "-r", "-t", "-z", "--full-tree", tree) != 0 {
		die("트리 목록을 읽지 못했다")
	}
	ents, ok := gitx.SplitZ(lsz.String()) // 끝 NUL까지 온 목록만(잘린 목록을 덜 검사하고 내보내지 않게 — 작업 중 발견)
	if !ok {
		die("트리 목록이 온전하지 않다(git ls-tree -z 잘림)")
	}
	empty, ok := git("-C", repo, "hash-object", "-t", "tree", "/dev/null")
	if !ok {
		exit(1)
	}
	s := &snap{bdir: tmp + "/b", namesFile: tmp + "/names", bpaths: map[string][]string{}}
	if err := os.Mkdir(s.bdir, 0o700); err != nil {
		exit(1)
	}
	for _, ent := range ents {
		meta, p, ok := strings.Cut(ent, "\t")
		f := strings.Fields(meta)
		if !ok || len(f) != 3 || p == "" || !gitx.ObjID(f[2]) {
			die("트리 목록 줄 꼴이 틀렸다(git ls-tree -z)")
		}
		mode, typ, sha := f[0], f[1], f[2]
		if strings.Contains(p, "\n") {
			die("줄바꿈이 든 경로는 내보내지 않는다")
		}
		s.shas = append(s.shas, sha)
		switch mode + ":" + typ {
		case "100644:blob", "100755:blob", "120000:blob":
		case "040000:tree":
			s.tpaths = append(s.tpaths, p)
			if sha == empty {
				fmt.Printf("빈 하위 트리는 내보내지 않는다: %s\n", masked(p))
				s.bad = true
			}
			continue
		default:
			die("내보내지 않는 항목(mode %s %s — 하위 모듈 등): %s", mode, typ, masked(p))
		}
		if _, err := os.Stat(s.bdir + "/" + sha); err != nil {
			c := exec.Command("git", "-C", repo, "cat-file", "blob", sha)
			c.Stderr = os.Stderr
			data, err := proc.Output(c)
			if err != nil || os.WriteFile(s.bdir+"/"+sha, data, 0o600) != nil {
				die("블롭을 꺼내지 못했다: %s", masked(p))
			}
		}
		s.bpaths[sha] = append(s.bpaths[sha], p)
		s.paths = append(s.paths, p)
	}
	if len(s.paths) == 0 {
		die("트리가 비었다")
	}
	s.names = append(append([]string(nil), s.paths...), s.tpaths...)
	if err := os.WriteFile(s.namesFile, []byte(strings.Join(s.names, "\n")+"\n"), 0o600); err != nil {
		exit(1)
	}
	for sha := range s.bpaths {
		s.blobFiles = append(s.blobFiles, s.bdir+"/"+sha)
	}
	sort.Strings(s.blobFiles)
	return s
}

// label: 검사 줄 「<임시>/b/<sha>:줄: 까닭」을 그 블롭의 경로들로, 「<임시>/names:줄: 까닭」을 경로(mask면 폴더만)로 바꿔 겹침 없이 정렬해 돌려준다
func (s *snap) label(lines []string, mask bool) []string {
	set := map[string]bool{}
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, s.bdir+"/"):
			r := l[len(s.bdir)+1:]
			i := strings.IndexByte(r, ':')
			for _, p := range s.bpaths[r[:i]] {
				set[p+r[i:]] = true
			}
		case strings.HasPrefix(l, s.namesFile+":"):
			r := l[len(s.namesFile)+1:]
			i := strings.IndexByte(r, ':')
			n, _ := strconv.Atoi(r[:i])
			f := ""
			if n >= 1 && n <= len(s.names) {
				f = s.names[n-1]
			}
			if mask {
				d := "."
				if j := strings.LastIndexByte(f, '/'); j >= 0 {
					d = f[:j]
				}
				f = fmt.Sprintf("%s/(이름 가림, %s번째 파일)", d, r[:i])
			}
			set[f+":0:"+r[i+1:]+" (경로)"] = true
		default:
			set[l] = true
		}
	}
	var out []string
	for l := range set {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

func (s *snap) show(lines []string, mask bool) {
	for _, l := range s.label(lines, mask) {
		fmt.Println(l)
	}
}

// overlaps: 검사 0 — NFC·대소문자 접기로 같아지는 서로 다른 경로(폴더 포함)는 macOS 디스크에서 한 자리다. 걸리면 찍고 참
func overlaps(names []string) bool {
	py := exec.Command("python3", "-I", "-c", `
import sys, unicodedata
seen, out = {}, []
for i, line in enumerate(sys.stdin.buffer.read().split(b"\n")[:-1], 1):
    parts = line.decode("utf-8", "surrogateescape").split("/")
    for k in range(1, len(parts) + 1):
        o = "/".join(parts[:k])
        j, prev = seen.setdefault(unicodedata.normalize("NFC", o).casefold(), (i, o))
        if prev != o:
            out.append("%d번째·%d번째 파일" % (j, i))
            break
print("\n".join(out))`)
	py.Stdin = strings.NewReader(strings.Join(names, "\n") + "\n")
	py.Stderr = os.Stderr
	ov, err := py.Output()
	if err != nil {
		die("경로 겹침 검사 못 함 (python3)")
	}
	o := strings.TrimRight(string(ov), "\n")
	if o == "" {
		return false
	}
	for _, l := range strings.Split(o, "\n") {
		fmt.Printf("경로가 겹친다(대소문자·유니코드 정규화만 다름 — macOS에서 한 파일): %s\n", l)
	}
	return true
}

// nonText: 검사 0-2 — NUL이 들었거나 UTF-8로 안 풀리면 글자 검사가 속을 못 보니 내보내지 않는다. 걸리면 찍고 참
func (s *snap) nonText() bool {
	var nontext []string
	for _, f := range s.blobFiles {
		d, err := os.ReadFile(f)
		if err != nil {
			die("글 검사 못 함")
		}
		if bytes.IndexByte(d, 0) >= 0 || !utf8.Valid(d) {
			nontext = append(nontext, f+":0: 글이 아닌 블롭(NUL 또는 UTF-8 아님)은 내보내지 않는다")
		}
	}
	if len(nontext) == 0 {
		return false
	}
	s.show(nontext, false)
	return true
}

// lintFiles: 개인 값 검사(herdr-tools-config lint) — 종료 코드와 걸린 줄
func lintFiles(conf string, files ...string) (int, []string) {
	var b bytes.Buffer
	rc := toolcfg.Lint(conf, false, files, &b, os.Stderr)
	return rc, lines(b.String())
}

// leaks: 검사 1(개인 값 — 블롭 내용(바이트, 링크 대상 글자 포함)과 경로를 lint로)과 검사 2(비밀 꼴 — 범용 꼴(ERE) + 설정 publish.secrets(ERE
// 하나), 블롭 내용·경로를 바이트로. 경로가 걸리면 이름이 곧 값이라 가린다). 걸리면 찍고 참
func (s *snap) leaks(c pubCfg) bool {
	bad := false
	if !toolcfg.HasPatterns(c.conf + "/private-patterns.txt") {
		die("개인 값 검사 못 함 — %s/private-patterns.txt에 패턴이 없다", c.conf)
	}
	switch rc, hits := lintFiles(c.conf, append(s.blobFiles, s.namesFile)...); rc {
	case 0:
	case 1:
		s.show(hits, false)
		bad = true
	default:
		die("개인 값 검사 못 함 (herdr-tools-config lint 실패)")
	}
	if hits, ok := scan(append(s.blobFiles, s.namesFile), c.extra); !ok {
		die("비밀 꼴 검사 못 함 (grep 실패)")
	} else if len(hits) > 0 {
		s.show(hits, true)
		bad = true
	}
	return bad
}

// blocked: 막음 — 종료 1
func blocked(short string) {
	fmt.Fprintf(os.Stderr, "%s: 막음 — 위 줄을 고치기 전에는 내보내지 않는다 (비공개 %s)\n", me, short)
	exit(1)
}

// publicDir: 공개 쪽 git 폴더(bare): 있으면 origin이 publish.repo인지 보고, 없으면 만든다. 돌려주는 dir = 객체·커밋을 둘 폴더, idir = 작성자를 읽을 폴더.
// --dry-run은 아무것도 쓰지 않는다: 객체는 임시 bare에 받고, 작성자는 실제 폴더가 있으면 거기서 읽기만 한다
func publicDir(dir, url string, dry bool) (string, string) {
	idir := dir
	if _, err := os.Stat(dir); err == nil {
		if v, _ := gitx.Out("", nil, "--git-dir="+dir, "rev-parse", "--is-bare-repository"); v != "true" {
			die("publish.dir이 bare git 저장소가 아니다: %s", dir)
		}
		if v, _ := gitx.Out("", nil, "--git-dir="+dir, "remote", "get-url", "origin"); v != url {
			die("publish.dir의 origin이 publish.repo와 다르다: %s", dir)
		}
		if dry {
			dir = tmp + "/public.git"
		}
	} else {
		if dry {
			dir = tmp + "/public.git"
		}
		idir = dir
	}
	if _, err := os.Stat(dir); err != nil {
		if os.MkdirAll(filepath.Dir(dir), 0o755) != nil {
			die("공개 clone을 만들지 못했다: %s", dir)
		}
		if _, ok := git("init", "-q", "--bare", dir); !ok {
			die("공개 clone을 만들지 못했다: %s", dir)
		}
		if _, ok := git("--git-dir="+dir, "remote", "add", "origin", url); !ok {
			die("공개 clone을 만들지 못했다: %s", dir)
		}
	}
	return dir, idir
}

// checkIdent: 검사 3 — 공개 커밋 작성자·커미터를 커밋을 만들 그 git 폴더에서 본다(publish.author, 없으면 그 폴더의 git 설정, 지어낸 값은 안 받음).
// 개인 값·비밀 꼴(B12)에 걸리면 막는다 — 값은 찍지 않는다
func checkIdent(c pubCfg, idir, short string) {
	var ident strings.Builder
	for _, v := range []string{"GIT_AUTHOR_IDENT", "GIT_COMMITTER_IDENT"} {
		id, rc := gitx.Out("", nil, "-c", "user.useConfigOnly=true", "--git-dir="+idir, "var", v)
		if rc != 0 {
			die("공개 커밋 작성자를 정하지 못했다 — 설정 publish.author(이름 <메일>)를 둔다")
		}
		// 끝의 「<시각> <시간대>」 두 칸을 뺀다
		for k := 0; k < 2; k++ {
			if i := strings.LastIndexByte(id, ' '); i >= 0 {
				id = id[:i]
			}
		}
		ident.WriteString(id + "\n")
	}
	identFile := tmp + "/ident"
	if os.WriteFile(identFile, []byte(ident.String()), 0o600) != nil {
		exit(1)
	}
	switch rc, _ := lintFiles(c.conf, identFile); rc {
	case 0:
	case 1:
		fmt.Println("공개 커밋 작성자·커미터가 개인 값에 걸린다 — 설정 publish.author(이름 <noreply 메일>)를 둔다")
		blocked(short)
	default:
		die("개인 값 검사 못 함 (작성자)")
	}
	if hits, ok := scan([]string{identFile}, c.extra); !ok {
		die("비밀 꼴 검사 못 함 (작성자)")
	} else if len(hits) > 0 {
		fmt.Println("공개 커밋 작성자·커미터가 비밀 꼴에 걸린다 — 설정 publish.author(이름 <noreply 메일>)를 고친다")
		blocked(short)
	}
}

// pgit: 공개 쪽 git 폴더에서(지어낸 작성자 안 받음)
func pgit(dir string, args ...string) (string, bool) {
	return git(append([]string{"-c", "user.useConfigOnly=true", "--git-dir=" + dir}, args...)...)
}

// export: 공개 main과 견주어 같으면 「같음」, 아니면 트리 객체를 옮기고 변경 목록을 찍은 뒤 커밋 하나를 push한다(--dry-run은 목록까지). 끝나면 종료
func export(c pubCfg, dir, res, src, tree, short string, shas []string, dry bool) {
	// 공개 저장소에 닿는 git은 remote로: 출력·오류 글에 publish.repo 주소 원문을 싣지 않는다(A12 — 인증값이 든 주소일 수 있다)
	// 성공 응답도 요청한 이름 한 줄·완전한 객체 ID여야 받는다(gitx.LsRemote — 재감사 ops/R-220: SHA 앞 12자만 온 응답을 짧은 이름으로
	// 풀어 「같음」·결과 파일을 썼다). 「공개 main 없음」은 --exit-code 종료 2뿐
	url := c.url
	var lse bytes.Buffer
	pm, st := gitx.LsRemote(dir, url, &lse, "refs/heads/main")
	os.Stderr.WriteString(strings.ReplaceAll(lse.String(), url, "publish.repo"))
	if st == gitx.Unknown {
		die("공개 저장소를 읽지 못했다 (publish.repo — git ls-remote 실패·잘린 응답)")
	}
	parent := pm["refs/heads/main"]
	var ptree string
	if parent != "" {
		var ok bool
		if _, ok := remote(dir, url, "fetch", "-q", url, "refs/heads/main"); !ok {
			die("공개 저장소 fetch 실패 (publish.repo)")
		}
		if ptree, ok = pgit(dir, "rev-parse", parent+"^{tree}"); !ok {
			die("공개 main을 읽지 못했다")
		}
		// 앞 실행이 push 뒤 로컬 ref를 못 바꿨으면(B16) 확인한 원격 값으로 맞춘다
		if !dry {
			if l, _ := gitx.Out("", nil, "--git-dir="+dir, "rev-parse", "-q", "--verify", "refs/heads/main"); l != parent {
				if _, ok := pgit(dir, "update-ref", "refs/heads/main", parent); !ok {
					fmt.Fprintf(os.Stderr, "%s: 경고 — 로컬 refs/heads/main을 원격 값으로 맞추지 못했다 (%s)\n", me, dir)
				}
			}
		}
	} else {
		ptree, _ = pgit(dir, "hash-object", "-t", "tree", "/dev/null")
	}
	if tree == ptree {
		fmt.Printf("같음 (비공개 %s)\n", short)
		if !dry {
			done(res, src)
		}
		exit(0)
	}
	// 트리·블롭 객체만 옮긴다(비공개 커밋 객체는 옮기지 않는다). 공개 트리 = 비공개 트리 sha. 검사한 목록 그대로, 블롭까지 있는지 확인
	if !transfer(c.repo, dir, tree, shas) {
		die("트리 객체를 공개 쪽으로 옮기지 못했다")
	}
	dt := exec.Command("git", "-c", "user.useConfigOnly=true", "--git-dir="+dir, "diff-tree", "-r", "--name-status", ptree, tree)
	dt.Stdout, dt.Stderr = os.Stdout, os.Stderr
	if err := proc.Run(dt); err != nil { // B14: 못 읽었으면(시간 넘김 포함) 미리보기·내보내기를 끝내지 않는다
		die("변경 목록을 읽지 못했다 (git diff-tree)")
	}
	if dry {
		fmt.Println("(--dry-run: push 안 함)")
		exit(0)
	}
	args := []string{"commit-tree", tree}
	if parent != "" {
		args = append(args, "-p", parent)
	}
	commit, ok := pgit(dir, append(args, "-m", "herdr-tools "+short+" 내보냄")...)
	if !ok {
		die("커밋을 만들지 못했다")
	}
	if _, ok := remote(dir, url, "push", "-q", url, commit+":refs/heads/main"); !ok {
		die("push 실패 (publish.repo, fast-forward만 한다)")
	}
	// 원격은 이미 바뀌었다 — 로컬 ref 실패는 따로 알리고 종료 코드는 원격 결과(다음 실행이 원격 값으로 맞춘다)
	if _, ok := pgit(dir, "update-ref", "refs/heads/main", commit); !ok {
		fmt.Fprintf(os.Stderr, "%s: 경고 — 공개 push는 됐지만 로컬 refs/heads/main을 바꾸지 못했다 (%s, 다음 실행에서 원격 값으로 맞춘다)\n", me, dir)
	}
	cs, _ := pgit(dir, "rev-parse", "--short", commit)
	fmt.Printf("내보냄 %s (비공개 %s → publish.repo)\n", cs, short)
	done(res, src)
	exit(0)
}
