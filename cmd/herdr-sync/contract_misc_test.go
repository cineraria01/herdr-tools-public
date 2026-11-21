package main

// herdr-sync 계약 시험 13·14·15·16: 잠금, lib 줄 없음, config.example/sync.list 꼴, 공개 내보내기, 릴리스(Go 판만)

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	ct "herdr-tools/internal/clitest"
)

func (s *sx) lockAndRest() {
	T, M, W1 := s.T, s.MAC, s.W1
	// 13. 잠금: 한 번에 하나
	lock := M + "/.local/state/herdr-sync/lock"
	hold := exec.Command("lockf", "-s", "-t", "0", lock, "sh", "-c", `touch "$1"; while [ ! -e "$2" ]; do sleep 0.1; done`, "_", T+"/held", T+"/release")
	if err := hold.Start(); err != nil {
		s.t.Fatal(err)
	}
	for i := 0; i < 50 && !ct.Exists(T+"/held"); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	s.run("--check")
	s.put(T+"/release", "")
	hold.Wait()
	s.ok("잠금: 다른 herdr-sync가 돌면 바로 끝", s.r.RC == 75 && s.contains("다른 herdr-sync"))

	// 13-1. 범용 저장소에 lib/<도구>/가 생겼는데 개인 sync.list에 bin 줄만 있으면(2026-11-20 herdr-tools-split) 아무 데도 쓰지 않고 멈춘다.
	// bin만 깔리면 모든 기계의 그 도구가 「함수 파일을 못 읽었다」로 멈추기 때문. --check도 같은 판정
	s.put(T+"/repo/lib/hc-lane/a.sh", "a=1\n")
	s.git(T+"/repo", "add", "lib")
	s.git(T+"/repo", "commit", "-qm", "lib")
	s.gitLand(T+"/repo", "push", "-q", "origin", "main")
	s.put(T+"/repo/bin/hc-lane", "#!/bin/sh\necho lib-lane\n")
	s.git(T+"/repo", "commit", "-qam", "lib-lane")
	s.gitLand(T+"/repo", "push", "-q", "origin", "main")
	before := s.snap()
	s.run("--check")
	s.ok("lib 줄 없음 --check: 멈추고 고칠 줄을 찍는다", s.r.RC == 2 && s.has("도구  .local/lib/hc-lane/**  tools:lib/hc-lane/  macos"))
	s.run()
	s.ok("lib 줄 없음: 아무 기계에도 쓰지 않고 멈춘다 (새 bin도 안 깖)", s.r.RC == 2 && s.snap() == before && !strings.Contains(ct.Read(W1+"/.local/bin/hc-lane"), "lib-lane") && s.has("도구  .local/lib/hc-lane/**  tools:lib/hc-lane/  macos"))
	ct.Append(s.t, T+"/conf/sync.list", "도구     .local/lib/hc-lane/**       tools:lib/hc-lane/             macos\n")
	s.git(T+"/conf", "commit", "-qam", "lib")
	s.gitLand(T+"/conf", "push", "-q", "origin", "main")
	s.run()
	s.ok("lib 줄을 더하면: bin과 lib를 함께 깐다", s.r.RC == 0 && s.is(W1+"/.local/lib/hc-lane/a.sh", "a=1\n") && s.is(W1+"/.local/bin/hc-lane", "#!/bin/sh\necho lib-lane\n"))

	s.exampleList()

	// 15. 공개 내보내기: 설정 publish.repo가 있을 때만, --check는 안 부름, 실패해도 크게 경고만 하고 종료 코드는 그대로
	s.ok("publish.repo 없음: 내보내기를 부르지 않는다", !ct.Exists(T+"/published"))
	ct.Append(s.t, T+"/conf/config", "publish.repo  /x/public.git\n")
	s.git(T+"/conf", "commit", "-qam", "pub")
	s.gitLand(T+"/conf", "push", "-q", "origin", "main")
	s.run("--check")
	s.ok("--check: 내보내기를 부르지 않는다", !ct.Exists(T+"/published"))
	s.run()
	s.ok("publish.repo 있음: 맞춘 뒤 내보내기를 부른다", s.r.RC == 0 && strings.TrimRight(ct.Read(T+"/published"), "\n") == T+"/repo "+T+"/conf ")
	s.runEnv([]string{"PUB_FAKE_RC=1"})
	s.ok("내보내기 실패: 크게 경고, 맞추기 종료 코드는 그대로", s.r.RC == 0 && s.has("!!! herdr-sync: 공개 저장소 내보내기 실패 (맞추기는 끝났다) — herdr-tools-publish 출력을 보고 고친 뒤 herdr-tools-publish를 다시 !!!"))

	if s.impl == "go" {
		s.release()
	}
}

// 14. 범용 저장소의 config.example/sync.list: 꼴이 맞고, tools: 경로가 모두 있고, bin/*을 모두 깐다
func (s *sx) exampleList() {
	list := s.here + "/config.example/sync.list"
	if !ct.Exists(list) {
		return
	}
	var rows [][]string // 주석 아닌 줄의 칸
	for _, l := range ct.Lines(ct.Read(list)) {
		if !strings.HasPrefix(l, "#") {
			rows = append(rows, strings.Fields(l))
		}
	}
	tgt := regexp.MustCompile(`^(all|macos|local|[a-z0-9,-]+)$`)
	old := regexp.MustCompile(`(^|,)(all9|mac7|mac-only)(,|$)`)
	bad := false
	for _, f := range rows {
		bad = bad || (len(f) > 0 && (len(f) != 4 || !tgt.MatchString(f[3]) || old.MatchString(f[3])))
	}
	s.ok("config.example/sync.list: 줄마다 네 칸, 대상이 새 이름", !bad)
	for _, f := range rows {
		if len(f) >= 3 && strings.HasPrefix(f[2], "tools:") {
			p := f[2][len("tools:"):]
			s.ok("config.example/sync.list: tools:"+p+" 있음", ct.Exists(s.here+"/"+p))
		}
	}
	text := ct.Read(list)
	bins, _ := filepath.Glob(s.here + "/bin/*")
	for _, f := range bins {
		b := filepath.Base(f)
		s.ok("config.example/sync.list: bin/"+b, strings.Contains(text, " tools:bin/"+b+" "))
	}
	// col: 셋째 칸이 <값>인 줄의 넷째 칸들(awk가 찍는 꼴 — 줄바꿈으로 이음)
	col := func(match func([]string) bool) string {
		var out []string
		for _, f := range rows {
			if len(f) >= 4 && match(f) {
				out = append(out, f[3])
			}
		}
		return strings.Join(out, "\n")
	}
	// Go 판 도구(go.mod·cmd/<이름>)는 소스로 깐다: Go 줄 넷이 한 번에, 대상이 같게(2026-11-21 go-foundation)
	if ct.Exists(s.here + "/go.mod") {
		var g []string
		for _, f := range rows {
			if len(f) >= 4 && (f[2] == "tools:go.mod" || f[2] == "tools:cmd/" || f[2] == "tools:internal/" || f[2] == "tools:lib/go-run.sh") {
				g = append(g, f[2]+" "+f[3])
			}
		}
		sort.Strings(g)
		s.ok("config.example/sync.list: Go 소스 나무 줄 넷, 대상이 같음", strings.Join(g, ",") == "tools:cmd/ all,tools:go.mod all,tools:internal/ all,tools:lib/go-run.sh all")
	}
	// lib/<도구>/(본체가 source하는 함수 파일)는 나무 줄로, 대상은 그 도구 줄과 같게(빠지면 그 기계의 도구가 종료 1)
	// 도구가 함께 쓰는 lib(bin 없음, lib/<이름>/OWNER에 쓰는 도구 이름)는 그 도구 줄과 견준다. 점 파일만 남은 폴더는 lib가 아니다
	libs, _ := filepath.Glob(s.here + "/lib/*")
	for _, d := range libs {
		if !ct.IsDir(d) || !hasVisible(d) {
			continue
		}
		b := filepath.Base(d)
		o := b
		if ct.Exists(d + "/OWNER") {
			o = ct.Lines(ct.Read(d + "/OWNER"))[0]
		}
		binT := col(func(f []string) bool { return f[2] == "tools:bin/"+o })
		libT := col(func(f []string) bool { return f[2] == "tools:lib/"+b+"/" && strings.HasSuffix(f[1], "/**") })
		s.ok("config.example/sync.list: lib/"+b+"/ 나무 줄, 대상은 bin/"+o+"와 같음", binT != "" && libT == binT)
	}
}

// hasVisible: [ -n "$(ls d)" ] — 점으로 시작하지 않는 것이 있는지
func hasVisible(d string) bool {
	es, _ := os.ReadDir(d)
	for _, e := range es {
		if !strings.HasPrefix(e.Name(), ".") {
			return true
		}
	}
	return false
}

// fakeGH: 가짜 gh — 릴리스를 $HCT/rel/<태그>/에 둔다
const fakeGH = `#!/bin/bash
echo "$*" >> "$HCT/gh.calls"
[ "$1" = release ] || exit 2
op=$2 tag=$3; shift 3
d=$HCT/rel/$tag
case $op in
  view) [ -d "$d" ] || { echo "release not found" >&2; exit 1; }
    ls "$d" ;;
  download) while [ $# -gt 0 ]; do case $1 in --pattern) p=$2; shift ;; --dir) o=$2; shift ;; esac; shift; done
    [ -f "$d/$p" ] && cp "$d/$p" "$o/" ;;
  create | upload) [ -z "${GH_FAKE_FAIL-}" ] || exit 1
    mkdir -p "$d"; while [ $# -gt 0 ]; do case $1 in --repo | --title | --notes) shift ;; --clobber) ;; *) cp "$1" "$d/" ;; esac; shift; done ;;
  *) exit 2 ;;
esac
`

// 16. 릴리스(Go 판만, 2026-11-21 사용자 「github 에다가 빌드 한것도 처리해서 … 각 컴퓨터에서는 받아서」): 내보내기가 되면 맥에서 cmd/*를 지어
// publish.repo 릴리스 bin-<커밋 12자>에 올리고(가짜 gh — $T/rel/<태그>/), macOS 기계는 맥이 넘긴 sha256이 맞을 때만 받는다(file:// 주소)
func (s *sx) release() {
	T, M, W1, LX := s.T, s.MAC, s.W1, s.LX
	s.putx(T+"/bin/gh", fakeGH)
	s.t.Setenv("HERDR_SYNC_RELEASE_URL", "file://"+T+"/rel")
	if os.Getenv("GOCACHE") == "" {
		s.t.Setenv("GOCACHE", s.here+"/.bin/go-build")
	}
	// hello는 새긴 커밋을 찍는다(시험 바이너리가 -X …version.Commit을 실제로 받았는지 — 감사 T01·O-14)
	gomod := "module herdr-tools\n\ngo 1.24\n"
	hello := "package main\n\nimport (\n\t\"fmt\"\n\n\t\"herdr-tools/internal/version\"\n)\n\nfunc main() { fmt.Println(\"hello\", version.Commit) }\n"
	ver := "package version\n\nvar Commit = \"dev\"\n"
	s.put(T+"/repo/go.mod", gomod)
	s.put(T+"/repo/cmd/hello/main.go", hello)
	s.put(T+"/repo/internal/version/version.go", ver)
	s.git(T+"/repo", "add", "go.mod", "cmd", "internal")
	s.git(T+"/repo", "commit", "-qm", "go")
	s.gitLand(T+"/repo", "push", "-q", "origin", "main")
	s.editLines(T+"/conf/config", func(l string) (string, bool) {
		if strings.HasPrefix(l, "publish.repo ") {
			return "publish.repo  https://github.com/test/pub.git", true
		}
		return l, true
	})
	s.git(T+"/conf", "commit", "-qam", "rel")
	s.gitLand(T+"/conf", "push", "-q", "origin", "main")
	for _, h := range []string{M, W1, LX} { // Go 소스를 깐 기계(받는 자리 — 소스 판이 범용 저장소와 같아야 받는다)
		s.put(h+"/.local/lib/herdr-go/go.mod", gomod)
		s.put(h+"/.local/lib/herdr-go/cmd/hello/main.go", hello)
		s.put(h+"/.local/lib/herdr-go/internal/version/version.go", ver)
	}
	s.run()
	tag := "bin-" + ct.Git(T+"/repo", "rev-parse", "HEAD")[:12]
	src := ct.Cmd{}.Run("bash", "-c", `. "$1"; go_run_root=$2; go_run_src`, "_", s.here+"/lib/go-run.sh", T+"/repo").Out
	rel := T + "/rel/" + tag
	s.ok("릴리스: 맥에서 지어 올림(바이너리 + SHA256SUMS)", s.r.RC == 0 && s.has("릴리스 올림 "+tag) && ct.IsExec(rel+"/hello") && regexp.MustCompile(`(?m)  hello$`).MatchString(ct.Read(rel+"/SHA256SUMS")))
	hb, _ := os.ReadFile(rel + "/hello")
	sum := sha256.Sum256(hb)
	var first []string
	for _, l := range ct.Lines(ct.Read(rel + "/SHA256SUMS")) {
		first = append(first, strings.SplitN(l, " ", 2)[0])
	}
	s.ok("릴리스: 커밋을 바이너리에 새김(빌드 입력은 범용 저장소)", ct.Cmd{}.Run(rel+"/hello").Out == "hello "+tag[4:] && hex.EncodeToString(sum[:]) == strings.Join(first, "\n"))
	wb := W1 + "/.local/lib/herdr-go/.bin"
	s.ok("받기: macOS 기계·맥은 해시가 맞으면 받고 소스 판을 곁에", s.has("w1 받음 hello ("+tag+")") && ct.Cmd{}.Run(wb+"/hello").Out == "hello "+tag[4:] && strings.TrimRight(ct.Read(wb+"/hello.src"), "\n") == src && ct.IsExec(M+"/.local/lib/herdr-go/.bin/hello"))
	s.ok("릴리스: 로그에 publish.repo 주소 원문이 없다", !s.contains("github.com/test/pub"))
	s.ok("받기: 리눅스로 가는 bin/ 줄이 없는 도구는 linux/amd64로 짓지도 받지도 않는다(go-linux-dist)", !ct.Exists(rel+"/hello-linux-amd64") && !ct.Exists(LX+"/.local/lib/herdr-go/.bin/hello"))
	s.put(T+"/gh.calls", "")
	s.run()
	calls := ct.Read(T + "/gh.calls")
	s.ok("릴리스: 같은 커밋·같은 해시는 다시 안 올림, 이미 받은 기계는 다시 안 받음", s.r.RC == 0 && s.has("릴리스 같음 "+tag) && !regexp.MustCompile(`(?m)^release (create|upload)`).MatchString(calls) && !s.contains(" 받음 hello"))
	ct.Append(s.t, rel+"/hello", "x")
	os.Remove(wb + "/hello")
	os.Remove(wb + "/hello.src")
	s.run()
	s.ok("받기: 받은 것의 해시가 다르면 쓰지 않고 크게 경고, 맞추기 종료 코드는 그대로", s.r.RC == 0 && !ct.Exists(wb+"/hello") && s.contains("!!! herdr-sync: w1 hello 바이너리를 받지 않음: 해시가 다르다"))
	s.ok("받기: 실패한 받기는 임시 파일을 남기지 않음", !ct.FindName(wb, ".hello.*"))
	os.RemoveAll(rel)
	s.runEnv([]string{"GH_FAKE_FAIL=1"})
	s.ok("릴리스: 올리기 실패는 크게 경고, 받지 않음, 종료 코드 그대로", s.r.RC == 0 && s.has("!!! herdr-sync: 릴리스 만들기 실패 "+tag+" (gh release create — 맥 gh 인증·공개 미러 쓰기 권한) — 기계는 각자 빌드한다 !!!") && !ct.Exists(wb+"/hello"))
	// 게시기가 검사한 커밋이 HEAD와 트리가 다르면(그사이 origin/main이 앞섬) 짓지도 올리지도 않는다(감사 A02·O-02)
	s.put(T+"/gh.calls", "")
	s.runEnv([]string{"PUB_FAKE_CHECKED=" + ct.Git(T+"/repo", "rev-parse", "HEAD~1")})
	s.ok("릴리스: 검사한 커밋이 HEAD와 다르면 건너뜀(크게 경고), gh를 부르지 않음", s.r.RC == 0 && s.contains("릴리스 건너뜀 — 공개 검사한 커밋이 범용 저장소 HEAD") && ct.Read(T+"/gh.calls") == "")
	s.runEnv([]string{"PUB_FAKE_CHECKED=-"})
	s.ok("릴리스: 게시기가 검사한 커밋을 안 알리면 건너뜀", s.r.RC == 0 && s.contains("검사한 커밋을 알려 주지 않았다") && ct.Read(T+"/gh.calls") == "")
	ct.Unsetenv(s.t, "HERDR_SYNC_RELEASE_URL")
}
