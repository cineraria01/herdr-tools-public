// herdr-tools-publish: 범용 저장소(비공개)의 origin/main 트리를 공개 저장소에 기록 없이 내보낸다.
// (2026-11-21 사용자 「모두 고로 옮기고 싶어」 — ops-go, 셸 판은 2026-11-22 걷음). 인자·출력·종료 코드·검사(fail-closed)는 옛 셸 판 그대로다.
// 다른 점: 개인 값 검사를 곁의 herdr-tools-config 대신 같은 꾸러미(internal/toolcfg)로 한다. 경로 겹침(NFC·대소문자 접기)은
// 표준 라이브러리에 정규화가 없어 python3로 본다(없으면 검사 못 함 — 내보내지 않는다).
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"herdr-tools/internal/gitx"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/toolcfg"
	"herdr-tools/internal/version"
)

const (
	me    = "herdr-tools-publish"
	usage = `herdr-tools-publish: 범용 저장소(비공개)의 origin/main 트리를 공개 저장소에 기록 없이 내보낸다.
  herdr-tools-publish            검사 → 공개 main 위에 커밋 하나(「herdr-tools <sha> 내보냄」) → push
  herdr-tools-publish --dry-run  바뀔 파일 목록과 검사 결과만 (push 없음, publish.dir을 만들지 않음)
HERDR_TOOLS_PUBLISH_RESULT=<파일>: 게시가 성공(내보냄·같음, --dry-run 아님)했을 때만 검사한 비공개 커밋 sha 한 줄(그 밖은 파일 없음)
비공개 커밋 메시지·작성자는 싣지 않는다. 공개 트리는 비공개 트리와 같은 객체다(attributes를 거치지 않음). 같으면 「같음」.
막기(fail-closed): 겹치는 경로(대소문자·정규화)나 글이 아닌 블롭(NUL·UTF-8 아님)이 있거나, 블롭 내용·경로·링크 대상·공개 커밋 작성자에 개인 값(herdr-tools-config lint)이나 비밀 꼴이 있거나,
검사를 못 하면 아무것도 push하지 않고 종료 1. 설정 키: publish.repo(없으면 종료 2)·publish.dir·publish.author·publish.secrets.
`
)

var tmp string // 실행마다 만든 임시 폴더 — 끝날 때 지운다

func exit(code int) {
	if tmp != "" {
		os.RemoveAll(tmp)
	}
	os.Exit(code)
}

// sweepStaleTmp: 앞 실행이 SIGKILL로 죽으며 남긴 임시 폴더(<TMPDIR>/herdr-tools-publish.*)를 지운다 — owner의 실행이 끝났다고 확정될 때만
// (pid가 없거나 같은 pid의 시작 시각이 다름). owner가 없거나 모르면(ps 실패) 두고 넘어간다
func sweepStaleTmp() {
	base := os.Getenv("TMPDIR")
	if base == "" {
		base = os.TempDir()
	}
	ds, err := filepath.Glob(filepath.Join(base, me+".*"))
	if err != nil {
		return
	}
	for _, d := range ds {
		b, err := os.ReadFile(d + "/owner")
		if err != nil {
			continue
		}
		f := strings.SplitN(strings.TrimRight(string(b), "\n"), "\t", 2)
		pid, err := strconv.Atoi(f[0])
		if err != nil || len(f) != 2 || pid <= 1 || pid == os.Getpid() {
			continue
		}
		if proc.Alive(pid) {
			st, err := proc.StartTime(pid)
			if err != nil || st == f[1] {
				continue // 도는 중이거나 모름
			}
		}
		os.RemoveAll(d)
	}
}

// startTime: 자기 시작 시각(시험이 바꾼다)
var startTime = proc.StartTime

// mkOwnedTmp: 자기 임시 폴더(<TMPDIR>/herdr-tools-publish.*)를 만들고 소유 기록 owner(pid·시작 시각)를 쓴다. 기록을 못 만들면 그 빈
// 폴더만 지우고 오류 — 블롭은 기록이 있는 폴더에만 꺼낸다(재감사 ops/R-213: 시작 시각 조회가 실패해도 기록 없이 블롭을 꺼내, SIGKILL
// 뒤 다음 실행이 소유를 몰라 영영 남겼다)
func mkOwnedTmp() (string, error) {
	d, err := os.MkdirTemp(os.Getenv("TMPDIR"), me+".")
	if err != nil {
		return "", err
	}
	st, err := startTime(os.Getpid())
	if err == nil && (st == "" || strings.ContainsAny(st, "\t\n")) {
		err = fmt.Errorf("시작 시각 꼴이 틀렸다: %q", st)
	}
	if err == nil {
		if err = os.WriteFile(d+"/owner", []byte(fmt.Sprintf("%d\t%s\n", os.Getpid(), st)), 0o600); err != nil {
			os.Remove(d + "/owner")
		}
	}
	if err != nil {
		if rerr := os.Remove(d); rerr != nil {
			err = fmt.Errorf("%w (빈 임시 폴더도 못 지움: %v)", err, rerr)
		}
		return "", fmt.Errorf("소유 기록을 만들지 못했다: %w", err)
	}
	return d, nil
}

func die(f string, a ...any) {
	fmt.Fprintf(os.Stderr, me+": "+f+"\n", a...)
	exit(1)
}

// git: 표준 출력(끝 줄바꿈 뺌)과 성공. 표준 오류는 그대로 흘린다
func git(args ...string) (string, bool) {
	out, rc := gitx.Out("", os.Stderr, args...)
	return out, rc == 0
}

func masked(p string) string { return filepath.Dir(p) + "/(이름 가림)" } // 이름이 곧 값일 수 있어 막을 때는 폴더만

func main() {
	dry := false
	switch {
	case len(os.Args) == 1:
	case len(os.Args) == 2 && os.Args[1] == "--dry-run":
		dry = true
	case len(os.Args) == 2 && os.Args[1] == "--version":
		fmt.Println(version.String(me))
		return
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	// 결과 파일(A02): 낡은 값을 남기지 않게 먼저 지운다 — 게시가 성공했을 때만 다시 쓴다(done)
	res := os.Getenv("HERDR_TOOLS_PUBLISH_RESULT")
	if res != "" {
		if err := os.Remove(res); err != nil && !os.IsNotExist(err) {
			die("결과 파일을 지우지 못했다: %v", err)
		}
	}
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
		i := strings.LastIndex(author, " <")
		if i < 0 || !strings.HasSuffix(author, ">") {
			die("publish.author 꼴: 이름 <메일>")
		}
		name, mail := author[:i], strings.TrimSuffix(author[i+2:], ">")
		os.Setenv("GIT_AUTHOR_NAME", name)
		os.Setenv("GIT_AUTHOR_EMAIL", mail)
		os.Setenv("GIT_COMMITTER_NAME", name)
		os.Setenv("GIT_COMMITTER_EMAIL", mail)
	}

	if _, ok := git("-C", repo, "fetch", "-q", "origin", "main"); !ok {
		die("fetch 실패: %s", repo)
	}
	src, _ := git("-C", repo, "rev-parse", "-q", "--verify", "refs/remotes/origin/main^{commit}") // 전체 이름: 같은 이름의 로컬 가지·태그에 속지 않게
	if src == "" {
		die("origin/main 없음: %s", repo)
	}
	tree, ok := git("-C", repo, "rev-parse", src+"^{tree}")
	if !ok {
		die("origin/main 트리를 읽지 못했다")
	}
	short, _ := git("-C", repo, "rev-parse", "--short", src)
	sweepStaleTmp()
	t0, err := mkOwnedTmp()
	if err != nil {
		die("임시 폴더를 만들지 못했다 — 게시하지 않는다: %v", err)
	}
	tmp = t0
	// 신호로 끝날 때도 자식 무리를 거둔 뒤 이 임시 폴더를 지운다(재감사 ops R-10 — 옛 판은 TERM이면 남겼다). SIGKILL로 남은 것은
	// 다음 실행이 owner(pid·시작 시각)로 소유를 확인해 지운다(sweepStaleTmp)
	proc.OnStop(func() { os.RemoveAll(t0) })

	// 트리의 블롭마다 검사한다. 경로대로 디스크에 꺼내지 않는다(macOS 디스크는 대소문자·정규화를 구분하지 않아 경로가 겹치고 링크를 따라
	// 바깥에 쓴다). 블롭은 <임시>/b/<sha>로만 꺼낸다: 검사한 바이트 = 공개로 가는 객체. -t: 하위 트리 이름도(빈 하위 트리는 -r만으로 안 나옴)
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
	bdir := tmp + "/b"
	if err := os.Mkdir(bdir, 0o700); err != nil {
		exit(1)
	}
	var paths, tpaths, shas []string // shas: 옮길 객체(블롭·하위 트리)
	bpaths := map[string][]string{}  // 블롭 sha → 경로들
	bad := false
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
		shas = append(shas, sha)
		switch mode + ":" + typ {
		case "100644:blob", "100755:blob", "120000:blob":
		case "040000:tree":
			tpaths = append(tpaths, p)
			if sha == empty {
				fmt.Printf("빈 하위 트리는 내보내지 않는다: %s\n", masked(p))
				bad = true
			}
			continue
		default:
			die("내보내지 않는 항목(mode %s %s — 하위 모듈 등): %s", mode, typ, masked(p))
		}
		if _, err := os.Stat(bdir + "/" + sha); err != nil {
			c := exec.Command("git", "-C", repo, "cat-file", "blob", sha)
			c.Stderr = os.Stderr
			data, err := proc.Output(c)
			if err != nil || os.WriteFile(bdir+"/"+sha, data, 0o600) != nil {
				die("블롭을 꺼내지 못했다: %s", masked(p))
			}
		}
		bpaths[sha] = append(bpaths[sha], p)
		paths = append(paths, p)
	}
	if len(paths) == 0 {
		die("트리가 비었다")
	}
	names := append(append([]string(nil), paths...), tpaths...) // 줄 i = 파일 경로, 그 뒤는 하위 트리 이름
	namesFile := tmp + "/names"
	if err := os.WriteFile(namesFile, []byte(strings.Join(names, "\n")+"\n"), 0o600); err != nil {
		exit(1)
	}
	var blobFiles []string
	for sha := range bpaths {
		blobFiles = append(blobFiles, bdir+"/"+sha)
	}
	sort.Strings(blobFiles)
	// show: 검사 줄 「<임시>/b/<sha>:줄: 까닭」을 그 블롭의 경로들로, 「<임시>/names:줄: 까닭」을 경로(가림이면 폴더만)로 바꿔 찍는다
	show := func(lines []string, mask bool) {
		set := map[string]bool{}
		for _, l := range lines {
			switch {
			case strings.HasPrefix(l, bdir+"/"):
				r := l[len(bdir)+1:]
				i := strings.IndexByte(r, ':')
				for _, p := range bpaths[r[:i]] {
					set[p+r[i:]] = true
				}
			case strings.HasPrefix(l, namesFile+":"):
				r := l[len(namesFile)+1:]
				i := strings.IndexByte(r, ':')
				n, _ := strconv.Atoi(r[:i])
				f := ""
				if n >= 1 && n <= len(names) {
					f = names[n-1]
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
		for _, l := range out {
			fmt.Println(l)
		}
	}

	// 검사 0: 겹치는 경로 — NFC·대소문자 접기로 같아지는 서로 다른 경로(폴더 포함)는 macOS 디스크에서 한 자리다
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
	if o := strings.TrimRight(string(ov), "\n"); o != "" {
		for _, l := range strings.Split(o, "\n") {
			fmt.Printf("경로가 겹친다(대소문자·유니코드 정규화만 다름 — macOS에서 한 파일): %s\n", l)
		}
		bad = true
	}

	// 검사 0-2: 글이 아닌 블롭 — NUL이 들었거나 UTF-8로 안 풀리면 글자 검사가 속을 못 보니 내보내지 않는다
	var nontext []string
	for _, f := range blobFiles {
		d, err := os.ReadFile(f)
		if err != nil {
			die("글 검사 못 함")
		}
		if bytes.IndexByte(d, 0) >= 0 || !utf8.Valid(d) {
			nontext = append(nontext, f+":0: 글이 아닌 블롭(NUL 또는 UTF-8 아님)은 내보내지 않는다")
		}
	}
	if len(nontext) > 0 {
		show(nontext, false)
		bad = true
	}

	// 검사 1: 개인 값 — 블롭 내용(바이트, 링크 대상 글자 포함)과 경로를 lint로
	if !toolcfg.HasPatterns(conf + "/private-patterns.txt") {
		die("개인 값 검사 못 함 — %s/private-patterns.txt에 패턴이 없다", conf)
	}
	lint := func(files ...string) (int, []string) {
		var b bytes.Buffer
		rc := toolcfg.Lint(conf, false, files, &b, os.Stderr)
		return rc, lines(b.String())
	}
	switch rc, hits := lint(append(blobFiles, namesFile)...); rc {
	case 0:
	case 1:
		show(hits, false)
		bad = true
	default:
		die("개인 값 검사 못 함 (herdr-tools-config lint 실패)")
	}

	// 검사 2: 비밀 꼴 — 범용 꼴(ERE) + 설정 publish.secrets(ERE 하나). 블롭 내용·경로를 바이트로. 경로가 걸리면 이름이 곧 값이라 가린다
	if hits, ok := scan(append(blobFiles, namesFile), extra); !ok {
		die("비밀 꼴 검사 못 함 (grep 실패)")
	} else if len(hits) > 0 {
		show(hits, true)
		bad = true
	}
	// 막히면 --dry-run도 여기서 끝낸다(B13): 뒤의 변경 목록(diff-tree)이 가린 이름을 원래 경로로 찍지 않게
	blocked := func() {
		fmt.Fprintf(os.Stderr, "%s: 막음 — 위 줄을 고치기 전에는 내보내지 않는다 (비공개 %s)\n", me, short)
		exit(1)
	}
	if bad {
		blocked()
	}

	// 공개 쪽 git 폴더(bare): 있으면 origin이 publish.repo인지 보고, 없으면 만든다.
	// --dry-run은 아무것도 쓰지 않는다: 객체는 임시 bare에 받고, 작성자는 실제 폴더가 있으면 거기서 읽기만 한다
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
	g := func(args ...string) (string, bool) {
		return git(append([]string{"-c", "user.useConfigOnly=true", "--git-dir=" + dir}, args...)...)
	}

	// 검사 3: 공개 커밋 작성자·커미터 — 커밋을 만들 그 git 폴더에서 본다(publish.author, 없으면 그 폴더의 git 설정, 지어낸 값은 안 받음)
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
	switch rc, _ := lint(identFile); rc {
	case 0:
	case 1:
		fmt.Println("공개 커밋 작성자·커미터가 개인 값에 걸린다 — 설정 publish.author(이름 <noreply 메일>)를 둔다")
		blocked()
	default:
		die("개인 값 검사 못 함 (작성자)")
	}
	// 작성자·커미터에도 비밀 꼴(B12) — 값은 찍지 않는다
	if hits, ok := scan([]string{identFile}, extra); !ok {
		die("비밀 꼴 검사 못 함 (작성자)")
	} else if len(hits) > 0 {
		fmt.Println("공개 커밋 작성자·커미터가 비밀 꼴에 걸린다 — 설정 publish.author(이름 <noreply 메일>)를 고친다")
		blocked()
	}
	fmt.Printf("검사 통과 (비공개 %s, 파일 %d개)\n", short, len(paths))
	// 공개 저장소에 닿는 git은 remote로: 출력·오류 글에 publish.repo 주소 원문을 싣지 않는다(A12 — 인증값이 든 주소일 수 있다)
	// 성공 응답도 요청한 이름 한 줄·완전한 객체 ID여야 받는다(gitx.LsRemote — 재감사 ops/R-220: SHA 앞 12자만 온 응답을 짧은 이름으로
	// 풀어 「같음」·결과 파일을 썼다). 「공개 main 없음」은 --exit-code 종료 2뿐
	var lse bytes.Buffer
	pm, st := gitx.LsRemote(dir, url, &lse, "refs/heads/main")
	os.Stderr.WriteString(strings.ReplaceAll(lse.String(), url, "publish.repo"))
	if st == gitx.Unknown {
		die("공개 저장소를 읽지 못했다 (publish.repo — git ls-remote 실패·잘린 응답)")
	}
	parent := pm["refs/heads/main"]
	var ptree string
	if parent != "" {
		if _, ok := remote(dir, url, "fetch", "-q", url, "refs/heads/main"); !ok {
			die("공개 저장소 fetch 실패 (publish.repo)")
		}
		if ptree, ok = g("rev-parse", parent+"^{tree}"); !ok {
			die("공개 main을 읽지 못했다")
		}
		// 앞 실행이 push 뒤 로컬 ref를 못 바꿨으면(B16) 확인한 원격 값으로 맞춘다
		if !dry {
			if l, _ := gitx.Out("", nil, "--git-dir="+dir, "rev-parse", "-q", "--verify", "refs/heads/main"); l != parent {
				if _, ok := g("update-ref", "refs/heads/main", parent); !ok {
					fmt.Fprintf(os.Stderr, "%s: 경고 — 로컬 refs/heads/main을 원격 값으로 맞추지 못했다 (%s)\n", me, dir)
				}
			}
		}
	} else {
		ptree, _ = g("hash-object", "-t", "tree", "/dev/null")
	}
	if tree == ptree {
		fmt.Printf("같음 (비공개 %s)\n", short)
		if !dry {
			done(res, src)
		}
		exit(0)
	}
	// 트리·블롭 객체만 옮긴다(비공개 커밋 객체는 옮기지 않는다). 공개 트리 = 비공개 트리 sha. 검사한 목록 그대로, 블롭까지 있는지 확인
	if !transfer(repo, dir, tree, shas) {
		die("트리 객체를 공개 쪽으로 옮기지 못했다")
	}
	if c := exec.Command("git", "-c", "user.useConfigOnly=true", "--git-dir="+dir, "diff-tree", "-r", "--name-status", ptree, tree); true {
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		if err := proc.Run(c); err != nil { // B14: 못 읽었으면(시간 넘김 포함) 미리보기·내보내기를 끝내지 않는다
			die("변경 목록을 읽지 못했다 (git diff-tree)")
		}
	}
	if dry {
		fmt.Println("(--dry-run: push 안 함)")
		exit(0)
	}
	args := []string{"commit-tree", tree}
	if parent != "" {
		args = append(args, "-p", parent)
	}
	commit, ok := g(append(args, "-m", "herdr-tools "+short+" 내보냄")...)
	if !ok {
		die("커밋을 만들지 못했다")
	}
	if _, ok := remote(dir, url, "push", "-q", url, commit+":refs/heads/main"); !ok {
		die("push 실패 (publish.repo, fast-forward만 한다)")
	}
	// 원격은 이미 바뀌었다 — 로컬 ref 실패는 따로 알리고 종료 코드는 원격 결과(다음 실행이 원격 값으로 맞춘다)
	if _, ok := g("update-ref", "refs/heads/main", commit); !ok {
		fmt.Fprintf(os.Stderr, "%s: 경고 — 공개 push는 됐지만 로컬 refs/heads/main을 바꾸지 못했다 (%s, 다음 실행에서 원격 값으로 맞춘다)\n", me, dir)
	}
	cs, _ := g("rev-parse", "--short", commit)
	fmt.Printf("내보냄 %s (비공개 %s → publish.repo)\n", cs, short)
	done(res, src)
	exit(0)
}

func lines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

var secretPats = []string{`-----BEGIN[A-Z0-9 ]*PRIVATE KEY`, `gh[pousr]_[A-Za-z0-9]{30,}`, `github_pat_[A-Za-z0-9_]{30,}`,
	`(^|[^A-Za-z0-9])sk-[A-Za-z0-9_-]{30,}`, `xox[abprs]-[A-Za-z0-9-]{10,}`, `AKIA[0-9A-Z]{16}`, `AIza[0-9A-Za-z_-]{35}`,
	`(^|[^0-9A-Fa-f])[0-9A-Fa-f]{40,}`}

var (
	tokRe   = regexp.MustCompile(`[A-Za-z0-9+/=_-]{32,}`)
	upRe    = regexp.MustCompile(`[A-Z]`)
	lowRe   = regexp.MustCompile(`[a-z]`)
	digitRe = regexp.MustCompile(`[0-9]`)
	alphaRe = regexp.MustCompile(`ABCDEFGH|abcdefgh|0123456789`)
)

// scan: 걸린 곳마다 「파일:줄: 까닭」(값은 안 찍음). 못 읽거나 설정 꼴이 틀리면 거짓(검사 못 함)
func scan(files []string, extra string) ([]string, bool) {
	pats := append([]string(nil), secretPats...)
	if extra != "" {
		pats = append(pats, extra)
	}
	res := make([]*toolcfg.ERE, len(pats))
	for i, p := range pats {
		re, err := toolcfg.CompileERE(p, false) // 셸 판 LC_ALL=C grep -E와 같은 ERE·바이트 판정
		if err != nil {
			return nil, false
		}
		res[i] = re
	}
	var hits []string
	for _, f := range files {
		d, err := os.ReadFile(f)
		if err != nil {
			return nil, false
		}
		ls := bytes.Split(d, []byte("\n"))
		if len(ls) > 0 && len(ls[len(ls)-1]) == 0 {
			ls = ls[:len(ls)-1]
		}
		for i, l := range ls {
			for j, re := range res {
				if re.Match(l) {
					hits = append(hits, fmt.Sprintf("%s:%d: 비밀 꼴 %s", f, i+1, pats[j]))
				}
			}
			// 긴 base64 꼴 토큰: 32자 넘게 이어지고 대·소문자·숫자가 다 있음. 알파벳 목록(ABCDEFGH…·0123456789)은 뺀다
			for _, tok := range tokRe.FindAll(l, -1) {
				if upRe.Match(tok) && lowRe.Match(tok) && digitRe.Match(tok) && !alphaRe.Match(tok) {
					hits = append(hits, fmt.Sprintf("%s:%d: 비밀 꼴 긴 토큰", f, i+1))
				}
			}
		}
	}
	return hits, true
}
