package main

// sync-pin-commit(2026-11-24 — 지휘가 herdr-sync를 돌리는 1~2분 사이에 병합기가 새 착지를 기본 가지에 넣자, 맥은 앞 커밋으로 지은 판·
// 릴리스 없음·나머지 기계는 「다름 (바이너리)」로 끝난 반쪽 배포): 배포 한 번은 커밋 하나로 끝까지 간다.
// 가짜 게시기가 진짜처럼 origin main을 fetch해 검사한 커밋을 알리고, 그 앞(before)이나 뒤(after)에 다른 클론에서 Go 소스를 고친 착지를
// origin에 넣어 「실행 도중 기본 가지가 움직임」을 만든다. 실제 배포·실제 기계는 쓰지 않는다(contract_test.go의 가짜 hc·gh).

import (
	"os"
	"regexp"
	"strings"
	"testing"

	ct "herdr-tools/internal/clitest"
)

// pinPublisher: 진짜 게시기처럼 fetch 뒤 refs/remotes/origin/main을 검사한 커밋으로 알린다. PIN_LAND=before|after면 그 앞·뒤에 착지 하나
const pinPublisher = `#!/bin/sh
echo "$HERDR_TOOLS_REPO $HERDR_TOOLS_CONFIG $*" >> "$HCT/published"
land() {
  d=$HCT/rlane
  [ -d "$d" ] || git clone -q "$HCT/origin.git" "$d" || exit 1
  git -C "$d" pull -q --ff-only || exit 1
  echo "// land $1" >> "$d/cmd/hello/main.go"
  git -C "$d" commit -qam "land $1" && git -C "$d" push -q origin HEAD:main || exit 1
}
[ "${PIN_LAND-}" != before ] || land before
git -C "$HERDR_TOOLS_REPO" fetch -q origin main || exit 1
[ -z "${HERDR_TOOLS_PUBLISH_RESULT-}" ] || git -C "$HERDR_TOOLS_REPO" rev-parse refs/remotes/origin/main > "$HERDR_TOOLS_PUBLISH_RESULT"
[ "${PIN_LAND-}" != after ] || land after
`

// pinSetup: 처음 맞춘 뒤 바이너리 줄 하나(hello, macos)·Go 소스 나무 줄 셋·publish.repo를 더하고 한 번 맞춰 릴리스·깔기까지 같게
func (s *sx) pinSetup() {
	T := s.T
	s.synced()
	s.putx(T+"/bin/gh", fakeGH)
	s.putx(T+"/bin/herdr-tools-publish", pinPublisher)
	s.t.Setenv("HERDR_SYNC_RELEASE_URL", "file://"+T+"/rel")
	if os.Getenv("GOCACHE") == "" {
		s.t.Setenv("GOCACHE", s.here+"/.bin/go-build")
	}
	s.put(T+"/repo/go.mod", "module herdr-tools\n\ngo 1.24\n")
	s.put(T+"/repo/cmd/hello/main.go", "package main\n\nimport (\n\t\"fmt\"\n\n\t\"herdr-tools/internal/version\"\n)\n\nfunc main() { fmt.Println(\"hello\", version.Commit) }\n")
	s.put(T+"/repo/internal/version/version.go", "package version\n\nvar Commit = \"dev\"\n")
	s.git(T+"/repo", "add", "go.mod", "cmd", "internal")
	s.git(T+"/repo", "commit", "-qm", "go")
	s.gitLand(T+"/repo", "push", "-q", "origin", "main")
	s.editLines(T+"/conf/config", func(l string) (string, bool) { return l, !strings.HasPrefix(l, "publish.repo ") })
	ct.Append(s.t, T+"/conf/config", "publish.repo  https://github.com/test/pub.git\n")
	ct.Append(s.t, T+"/conf/sync.list", "도구 .local/bin/hello tools:cmd/hello macos\nGo .local/lib/herdr-go/go.mod tools:go.mod all\n"+
		"Go .local/lib/herdr-go/cmd/** tools:cmd/ all\nGo .local/lib/herdr-go/internal/** tools:internal/ all\n")
	s.git(T+"/conf", "commit", "-qam", "rel")
	s.gitLand(T+"/conf", "push", "-q", "origin", "main")
	s.run()
	if s.r.RC != 0 || !s.contains("릴리스 올림 bin-") {
		s.t.Fatalf("준비: 바이너리 줄을 더해 맞추기 종료 %d\n%s", s.r.RC, s.r.Out)
	}
}

// ver: 그 HOME의 ~/.local/bin/hello가 찍는 새긴 커밋(12자)
func ver(home string) string {
	return strings.TrimPrefix(ct.Cmd{}.Run(home+"/.local/bin/hello").Out, "hello ")
}

func TestPinCommit(t *testing.T) {
	if testing.Short() || goBin() == "" {
		t.Skip("go 없음 또는 -short")
	}
	s := newSX(t)
	s.pinSetup()
	T, M, W1 := s.T, s.MAC, s.W1
	src := "/.local/lib/herdr-go/cmd/hello/main.go"

	// ③ 기본 가지가 안 움직임 → 지금과 같은 출력(알림 없음)
	s.run()
	s.ok("③ 안 움직임: 종료 0·알림 없음·모두 같음", s.r.RC == 0 && !s.contains("배포한 판") && s.has("맥: 같음") && s.has("w1: 같음") && s.has("lx: 같음"))

	// ① 게시 앞에 착지(오늘 09:41 꼴: 저장소 받기 뒤·게시기 fetch 앞) → 반쪽 배포가 없다: 맥·기계의 바이너리와 소스가 한 커밋,
	// 그 커밋의 릴리스를 받고(기계마다 짓지 않음), 모두 같음
	s.runEnv([]string{"PIN_LAND=before"})
	tip := s.origin("rev-parse", "main")
	s.ok("① 게시 앞 착지: 릴리스를 건너뛰지 않는다", s.r.RC == 0 && !s.contains("릴리스 건너뜀") && s.has("릴리스 올림 bin-"+tip[:12]))
	s.ok("① 게시 앞 착지: 맥·w1 바이너리가 같은 커밋(릴리스에서 받음)", ver(M) == tip[:12] && ver(W1) == tip[:12] && s.has("맥 받음 hello") && s.has("w1 받음 hello") && !s.contains("지음 hello"))
	s.ok("① 게시 앞 착지: 맥·w1 Go 소스도 그 커밋", strings.Contains(ct.Read(M+src), "// land before") && ct.Read(M+src) == ct.Read(W1+src))
	s.ok("① 게시 앞 착지: 모두 같음", s.has("맥: 같음") && s.has("w1: 같음") && s.has("lx: 같음"))
	s.run("--check")
	s.ok("① 게시 앞 착지: 뒤 --check도 같음", s.r.RC == 0)

	// ① 게시 뒤에 착지(기계를 맞추는 동안) → 모두 정한 커밋으로 같음, 끝에 알림 한 줄(실패 아님)
	pin := ct.Git(T+"/repo", "rev-parse", "HEAD")
	s.runEnv([]string{"PIN_LAND=after"})
	now := s.origin("rev-parse", "main")
	s.ok("① 게시 뒤 착지: 정한 커밋으로 모두 같음, 종료 0", s.r.RC == 0 && pin != now && ver(M) == pin[:12] && ver(W1) == pin[:12] &&
		s.has("맥: 같음") && s.has("w1: 같음") && s.has("lx: 같음"))
	s.ok("① 게시 뒤 착지: 끝에 알림 한 줄", strings.Count(s.r.Out, "배포한 판 ") == 1 &&
		s.rx(`^배포한 판 `+pin[:12]+` — 기본 가지는 그새 `+now[:12]+`로 움직였다\(다음 배포에\)$`))
	s.ok("① 게시 뒤 착지: 알림은 끝 요약 뒤", regexp.MustCompile(`(?s)lx: 같음\n.*배포한 판 `).MatchString(s.r.Out))
	// ④ 배포 기록에 정한 커밋과 기계별 결과(뒤에 움직인 끝이 아니라)
	ls := ct.Lines(ct.Read(M + "/.local/state/herdr-sync/deploy.tsv"))
	lg := strings.Split(ls[len(ls)-1], "\t")
	s.ok("④ 배포 기록: 정한 커밋·기계별 결과", len(lg) == 8 && lg[2] == "0" && lg[3] == pin[:12] && strings.Contains(lg[7], "맥=") && strings.Contains(lg[7], "w1="))

	// ② 한 기계가 그 커밋의 판을 못 받음(릴리스 올리기 실패 + 그 기계 소스가 따로 바뀌어 짓지도 못함) → 종료 ≠ 0, 끝 요약에 그 기계와 까닭
	s.run()
	os.RemoveAll(T + "/rel")
	s.put(W1+src, ct.Read(W1+src)+"// w1에서 고침\n")
	ct.Append(s.t, T+"/repo/cmd/hello/main.go", "// 새 판\n")
	s.git(T+"/repo", "commit", "-qam", "새 판")
	s.gitLand(T+"/repo", "push", "-q", "origin", "main")
	s.runEnv([]string{"GH_FAKE_FAIL=1"})
	s.ok("② 못 받은 기계: 종료 ≠ 0", s.r.RC != 0)
	s.ok("② 못 받은 기계: 끝 요약에 그 기계와 까닭", s.rx(`^w1: .*바이너리.*hello.*릴리스 없음`))
}
