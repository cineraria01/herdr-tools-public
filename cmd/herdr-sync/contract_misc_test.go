package main

// herdr-sync 계약 시험 13·14·15·16: 잠금, 걷힌 셸 판 줄, config.example/sync.list 꼴, 공개 내보내기, 바이너리 줄·릴리스

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
	"herdr-tools/internal/toolcfg"
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

	// 13-1. 걷힌 셸 판 줄(2026-11-22 go-shell-remove): 범용 저장소에 없는 tools:lib/…·tools:bin/…·.sh 줄이 개인 sync.list에 남으면
	// 아무 데도 쓰지 않고 멈춘다 — 두면 맥에 남은 옛 파일을 저장소로 다시 모은다(맥이 정본). --check도 같은 판정
	s.put(M+"/.local/lib/gone/a.sh", "a=1\n")
	ct.Append(s.t, T+"/conf/sync.list", "도구     .local/lib/gone/**          tools:lib/gone/                macos\n")
	s.git(T+"/conf", "commit", "-qam", "gone")
	s.gitLand(T+"/conf", "push", "-q", "origin", "main")
	before, h0 := s.snap(), s.heads()
	s.run("--check")
	s.ok("걷힌 셸 판 줄 --check: 멈추고 그 줄을 찍는다", s.r.RC == 2 && s.contains("셸 판이 걷혔다") && s.contains("tools:lib/gone/"))
	s.run()
	s.ok("걷힌 셸 판 줄: 아무 데도 쓰지 않고(저장소로 모으지도 않고) 멈춘다", s.r.RC == 2 && s.snap() == before && s.heads() == h0 && !ct.Exists(T+"/repo/lib/gone"))
	s.editLines(T+"/conf/sync.list", func(l string) (string, bool) { return l, !strings.Contains(l, "tools:lib/gone/") })
	s.git(T+"/conf", "commit", "-qam", "gone 줄 지움")
	s.gitLand(T+"/conf", "push", "-q", "origin", "main")
	os.RemoveAll(M + "/.local/lib/gone")
	s.run()
	s.ok("걷힌 줄을 지우면 다시 맞춘다", s.r.RC == 0)
	_ = W1

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
	s.ok("내보내기 실패: 크게 경고, 맞추기 종료 코드는 그대로", s.r.RC == 0 && s.has("!!! herdr-sync: 공개 저장소 내보내기 실패 — herdr-tools-publish 출력을 보고 고친 뒤 herdr-tools-publish를 다시. 바이너리는 기계마다 짓는다 !!!"))
	s.pubTerm()

	s.release()
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
	// 도구는 바이너리 줄로 모두 깐다(2026-11-22 go-shell-remove): cmd/<도구>마다 「.local/bin/<도구> tools:cmd/<도구>」 줄
	text := ct.Read(list)
	cmds, _ := filepath.Glob(s.here + "/cmd/*")
	for _, f := range cmds {
		// 점 폴더(세션 훅이 남긴 무시 폴더 .omc 등 — Go Glob의 *는 점 이름도 맞춘다)는 도구가 아니다(go-cmd-registry에서 발견해 함께 고침)
		if b := filepath.Base(f); ct.IsDir(f) && !strings.HasPrefix(b, ".") {
			s.ok("config.example/sync.list: cmd/"+b+" 바이너리 줄", regexp.MustCompile(`(?m)\s\.local/bin/`+regexp.QuoteMeta(b)+`\s+tools:cmd/`+regexp.QuoteMeta(b)+`\s`).MatchString(text))
		}
	}
	s.ok("config.example/sync.list: 셸 판 줄(tools:bin/·tools:lib/)이 없다", !regexp.MustCompile(`\stools:(bin|lib)/`).MatchString(text))
	// Go 소스 나무 줄 셋(그 기계에서 지을 때), 대상 all
	var g []string
	for _, f := range rows {
		if len(f) >= 4 && (f[2] == "tools:go.mod" || f[2] == "tools:cmd/" || f[2] == "tools:internal/") {
			g = append(g, f[2]+" "+f[3])
		}
	}
	sort.Strings(g)
	s.ok("config.example/sync.list: Go 소스 나무 줄 셋, 대상 all", strings.Join(g, ",") == "tools:cmd/ all,tools:go.mod all,tools:internal/ all")
	var o, e strings.Builder
	s.ok("config.example/sync.list: sync-lint 통과(곁 도구·소스 줄)", toolcfg.SyncLint(s.here, list, &o, &e) == 0)
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

// 16. 바이너리 줄·릴리스(2026-11-21 사용자 「github 에다가 빌드 한것도 처리해서 … 각 컴퓨터에서는 받아서」, 2026-11-22 go-shell-remove):
// 내보내기가 되면 맥에서 바이너리 줄의 도구를 지어 publish.repo 릴리스 bin-<커밋 12자>에 올리고(가짜 gh — $T/rel/<태그>/), 기계는
// 맥이 넘긴 sha256이 맞을 때만 받아(file:// 주소) ~/.local/bin/<도구>에 깐다. 못 받으면 그 기계 소스로 짓고, 그것도 안 되면 크게 다름
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
	// 바이너리 줄 하나(macos)와 Go 소스 나무 줄 셋(all — 그 기계에서 지을 때)
	ct.Append(s.t, T+"/conf/sync.list", "도구 .local/bin/hello tools:cmd/hello macos\nGo .local/lib/herdr-go/go.mod tools:go.mod all\n"+
		"Go .local/lib/herdr-go/cmd/** tools:cmd/ all\nGo .local/lib/herdr-go/internal/** tools:internal/ all\n")
	s.git(T+"/conf", "commit", "-qam", "rel")
	s.gitLand(T+"/conf", "push", "-q", "origin", "main")
	s.run()
	tag := "bin-" + ct.Git(T+"/repo", "rev-parse", "HEAD")[:12]
	rel := T + "/rel/" + tag
	s.ok("릴리스: 맥에서 지어 올림(바이너리 줄의 도구 + SHA256SUMS)", s.r.RC == 0 && s.has("릴리스 올림 "+tag) && ct.IsExec(rel+"/hello") && regexp.MustCompile(`(?m)  hello$`).MatchString(ct.Read(rel+"/SHA256SUMS")))
	hb, _ := os.ReadFile(rel + "/hello")
	sum := sha256.Sum256(hb)
	var first []string
	for _, l := range ct.Lines(ct.Read(rel + "/SHA256SUMS")) {
		first = append(first, strings.SplitN(l, " ", 2)[0])
	}
	s.ok("릴리스: 커밋을 바이너리에 새김(빌드 입력은 범용 저장소)", ct.Cmd{}.Run(rel+"/hello").Out == "hello "+tag[4:] && hex.EncodeToString(sum[:]) == strings.Join(first, "\n"))
	wb := W1 + "/.local/bin/hello"
	s.ok("깔기: macOS 기계·맥은 해시가 맞으면 ~/.local/bin/<도구>에 받고 기록", s.has("w1 받음 hello") && s.has("맥 받음 hello") && ct.Cmd{}.Run(wb).Out == "hello "+tag[4:] && ct.IsExec(M+"/.local/bin/hello") &&
		strings.HasPrefix(ct.Read(W1+"/.local/state/herdr-sync/bin.tsv"), "hello\t"+hex.EncodeToString(sum[:])+"\t"))
	s.ok("깔기: 옛 자리(.local/lib/herdr-go/.bin)는 쓰지 않는다", !ct.Exists(W1+"/.local/lib/herdr-go/.bin/hello"))
	s.ok("릴리스: 로그에 publish.repo 주소 원문이 없다", !s.contains("github.com/test/pub"))
	s.ok("깔기: 리눅스로 가는 바이너리 줄이 없는 도구는 linux/amd64로 짓지도 깔지도 않는다(go-linux-dist)", !ct.Exists(rel+"/hello-linux-amd64") && !ct.Exists(LX+"/.local/bin/hello"))
	s.put(T+"/gh.calls", "")
	s.run()
	calls := ct.Read(T + "/gh.calls")
	s.ok("릴리스: 같은 커밋·같은 해시는 다시 안 올림, 이미 깐 기계는 다시 안 받음", s.r.RC == 0 && s.has("릴리스 같음 "+tag) && !regexp.MustCompile(`(?m)^release (create|upload)`).MatchString(calls) && !s.contains(" 받음 hello"))
	s.run("--check")
	s.ok("깔기 --check: 깐 판은 같음", !s.contains("다름:바이너리"))
	// sync-bin-conflict: 그 기계에서 손으로 바꾼 바이너리(bin.tsv의 마지막 깐 sha와 다름)는 덮지 않고 충돌, 설정·지움은 계속, --take면 덮고 bak에
	hand := "#!/bin/sh\necho hand\n"
	s.putx(wb, hand)
	s.run("w1")
	s.ok("깔기: 손 교체한 바이너리는 충돌·안 덮음", s.r.RC == 1 && s.has("충돌 w1:.local/bin/hello (따로 바뀜)") && s.rx(`^w1: 충돌 1건`) && ct.Read(wb) == hand &&
		!s.contains("w1 바이너리를 다 깔지 못했다"))
	s.run("--check", "w1")
	s.ok("깔기 --check: 손 교체는 같은 충돌", s.r.RC == 1 && s.has("충돌 w1:.local/bin/hello (따로 바뀜)") && ct.Read(wb) == hand)
	s.run("--take", "w1:.local/bin/hello", "w1")
	s.ok("깔기 --take: 덮고 옛 판을 bak에", s.r.RC == 0 && s.contains("w1 받음 hello (따로 바뀐 판을 덮음 — 옛 판은 bak/") && ct.Cmd{}.Run(wb).Out == "hello "+tag[4:] &&
		ct.FindName(W1+"/.local/state/herdr-sync/bak", "hello"))
	s.run("--pull", "w1:.local/bin/hello", "w1")
	s.ok("깔기: 바이너리는 --pull을 받지 않는다", s.r.RC != 0 && s.contains("바이너리는 --pull을 받지 않는다"))
	// 받은 것의 해시가 다르면 쓰지 않고, 그 기계 소스 판이 같으니 그 기계에서 짓는다(같은 커밋을 새김)
	ct.Append(s.t, rel+"/hello", "x")
	os.Remove(wb)
	os.Remove(W1 + "/.local/state/herdr-sync/bin.tsv")
	s.run("w1")
	s.ok("깔기: 받은 것의 해시가 다르면 그 기계에서 짓는다", s.r.RC == 0 && s.has("w1 지음 hello (릴리스를 못 받아 그 기계에서 — 받은 것의 해시가 다르다)") && ct.Cmd{}.Run(wb).Out == "hello "+tag[4:])
	s.ok("깔기: 실패한 받기·짓기는 임시 파일을 남기지 않음", !ct.FindName(W1+"/.local/bin", ".hello.*"))
	// 그 기계 소스가 따로 바뀌어(충돌) 짓지도 못하면: 크게 다름(바이너리), 그 기계의 설정·지움을 미룬다 — 옛 포장으로 떨어지지 않는다
	s.put(W1+"/.local/lib/herdr-go/cmd/hello/main.go", hello+"// w1에서 고침\n")
	os.Remove(wb)
	s.put(W1+"/.local/bin/claude-md-sync", "old\n") // 지움 줄(local)은 w1에 안 가지만 설정·지움 미룸 표시만 본다
	s.run("w1")
	s.ok("깔기: 받기·짓기 모두 안 되면 크게 다름(바이너리)·종료 1·설정·지움 미룸", s.r.RC == 1 && s.contains("!!! herdr-sync: w1 hello 바이너리를 깔지 못했다: 받은 것의 해시가 다르다 · 그 기계 Go 소스 판이 범용 저장소와 다르다 !!!") &&
		s.contains("w1 바이너리를 다 깔지 못했다 — 그 기계의 설정·지움은 미룬다") && s.rx(`^w1: 다름 \(바이너리\)`) && !ct.Exists(wb))
	s.run("--take", "w1:.local/lib/herdr-go/cmd/hello/main.go", "w1")
	s.ok("깔기: 소스 충돌을 풀면 다음 실행이 짓는다", s.r.RC == 0 && s.has("w1 지음 hello (릴리스를 못 받아 그 기계에서 — 받은 것의 해시가 다르다)") && ct.Cmd{}.Run(wb).Out == "hello "+tag[4:])
	os.Remove(wb)
	s.run("--check", "w1")
	s.ok("깔기 --check: 없으면 다름(바이너리)·종료 1, 쓰지 않음", s.r.RC == 1 && s.has("w1 다름:바이너리 hello (없음)") && !ct.Exists(wb))
	os.RemoveAll(rel)
	s.runEnv([]string{"GH_FAKE_FAIL=1"})
	s.ok("릴리스: 올리기 실패는 크게 경고, 기계는 각자 짓는다, 종료 코드 그대로", s.r.RC == 0 && s.has("!!! herdr-sync: 릴리스 만들기 실패 "+tag+" (gh release create — 맥 gh 인증·공개 미러 쓰기 권한) — 기계는 각자 짓는다 !!!") &&
		s.has("w1 지음 hello (릴리스를 못 받아 그 기계에서 — 릴리스 없음)") && ct.Cmd{}.Run(wb).Out == "hello "+tag[4:])
	// 게시기가 검사한 커밋이 HEAD와 트리가 다르면(그사이 origin/main이 앞섬) 짓지도 올리지도 않는다(감사 A02·O-02)
	s.put(T+"/gh.calls", "")
	s.runEnv([]string{"PUB_FAKE_CHECKED=" + ct.Git(T+"/repo", "rev-parse", "HEAD~1")})
	s.ok("릴리스: 검사한 커밋이 HEAD와 다르면 건너뜀(크게 경고), gh를 부르지 않음", s.r.RC == 0 && s.contains("릴리스 건너뜀 — 공개 검사한 커밋이 범용 저장소 HEAD") && ct.Read(T+"/gh.calls") == "")
	s.runEnv([]string{"PUB_FAKE_CHECKED=-"})
	s.ok("릴리스: 게시기가 검사한 커밋을 안 알리면 건너뜀", s.r.RC == 0 && s.contains("검사한 커밋을 알려 주지 않았다") && ct.Read(T+"/gh.calls") == "")
	ct.Unsetenv(s.t, "HERDR_SYNC_RELEASE_URL")
}
