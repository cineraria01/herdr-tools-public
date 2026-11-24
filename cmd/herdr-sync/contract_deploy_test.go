package main

// 19. 배포 기록·상태·되돌리기·착지 뒤(2026-11-24 deploy-log-rollback): ① 배포 → 기록 한 줄(기계별 결과) ② 부분 실패 → 기록에 실패 기계
// ③ --status가 기계별 판·밀린 커밋을 보임(닿지 못한 기계는 모름) ④ --at <옛 커밋> → 그 판이 깔리고 다음 보통 배포가 충돌 없이 새 판으로
// ⑤ 없는 커밋 → 거절·목록 ⑦ 스스로 배포 켬 → 줄이 빈 뒤 한 번(공개 단계 없음), 끔 → 안 함 ⑧ 스스로 배포 실패 → 0 아닌 종료(hc-fill 막힘).
// ⑥(착지 알림 합침)은 hc-fill 쪽 단위 시험(onland_every_test.go). 가짜 gh·file:// 릴리스, 실제 기계·네트워크 없음.

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	ct "herdr-tools/internal/clitest"
)

// fakeGHList: fakeGH에 release list(올린 태그)를 더한 것
const fakeGHList = `#!/bin/bash
echo "$*" >> "$HCT/gh.calls"
[ "$1" = release ] || exit 2
if [ "$2" = list ]; then ls "$HCT/rel" 2>/dev/null; exit 0; fi
op=$2 tag=$3; shift 3
d=$HCT/rel/$tag
case $op in
  view) [ -d "$d" ] || { echo "release not found" >&2; exit 1; }
    ls "$d" ;;
  download) while [ $# -gt 0 ]; do case $1 in --pattern) p=$2; shift ;; --dir) o=$2; shift ;; esac; shift; done
    [ -f "$d/$p" ] && cp "$d/$p" "$o/" ;;
  create | upload) mkdir -p "$d"; while [ $# -gt 0 ]; do case $1 in --repo | --title | --notes) shift ;; --clobber) ;; *) cp "$1" "$d/" ;; esac; shift; done ;;
  *) exit 2 ;;
esac
`

// relPrep: 바이너리 줄 hello(macos)·Go 소스 나무 줄·publish.repo·가짜 gh — 16(release)과 같은 준비
func (s *sx) relPrep() {
	T := s.T
	s.putx(T+"/bin/gh", fakeGHList)
	s.t.Setenv("HERDR_SYNC_RELEASE_URL", "file://"+T+"/rel")
	if os.Getenv("GOCACHE") == "" {
		s.t.Setenv("GOCACHE", s.here+"/.bin/go-build")
	}
	s.put(T+"/repo/go.mod", "module herdr-tools\n\ngo 1.24\n")
	s.put(T+"/repo/cmd/hello/main.go", "package main\n\nimport (\n\t\"fmt\"\n\n\t\"herdr-tools/internal/version\"\n)\n\nfunc main() { fmt.Println(\"hello\", version.Commit) }\n")
	s.put(T+"/repo/internal/version/version.go", "package version\n\nvar Commit = \"dev\"\n")
	s.git(T+"/repo", "add", "go.mod", "cmd", "internal")
	s.git(T+"/repo", "commit", "-qm", "go: 처음")
	s.gitLand(T+"/repo", "push", "-q", "origin", "main")
	s.editLines(T+"/conf/config", func(l string) (string, bool) { return l, !strings.HasPrefix(l, "publish.repo ") })
	ct.Append(s.t, T+"/conf/config", "publish.repo  https://github.com/test/pub.git\n")
	ct.Append(s.t, T+"/conf/sync.list", "도구 .local/bin/hello tools:cmd/hello macos\nGo .local/lib/herdr-go/go.mod tools:go.mod all\n"+
		"Go .local/lib/herdr-go/cmd/** tools:cmd/ all\nGo .local/lib/herdr-go/internal/** tools:internal/ all\n")
	s.git(T+"/conf", "commit", "-qam", "rel")
	s.gitLand(T+"/conf", "push", "-q", "origin", "main")
}

// landCommit: 범용 저장소에 레인 착지 흉내(제목 머리 = 레인) — hello 소스에 한 줄 더해 판을 바꾼다. 커밋 12자
func (s *sx) landCommit(subj string) string {
	ct.Append(s.t, s.T+"/repo/cmd/hello/main.go", "// "+subj+"\n")
	s.git(s.T+"/repo", "commit", "-qam", subj)
	s.gitLand(s.T+"/repo", "push", "-q", "origin", "main")
	return ct.Git(s.T+"/repo", "rev-parse", "HEAD")[:12]
}

// lastLog: --log 1의 줄
func (s *sx) lastLog() string {
	s.run("--log", "1")
	return strings.TrimSpace(s.r.Out)
}

func logRows(T string) int {
	return len(ct.Lines(ct.Read(T + "/mac/.local/state/herdr-sync/deploy.tsv")))
}

// TestContractF: 19 — 배포 기록·--status·--at·--landed·--auto
func TestContractF(t *testing.T) {
	s := newSX(t)
	s.synced()
	s.relPrep()
	T, W1, LX := s.T, s.W1, s.LX
	wb := W1 + "/.local/bin/hello"
	hello := func() string { return strings.TrimSpace(ct.Cmd{}.Run(wb).Out) }

	// ① 배포 → 기록 한 줄(준비 synced의 배포도 이미 적혀 있다)
	n0 := logRows(T)
	s.ok("① 준비의 배포·--take도 기록됨(--check 빼고)", n0 == 4)
	s.run()
	c1 := ct.Git(T+"/repo", "rev-parse", "HEAD")[:12]
	s.ok("① 배포: 맥·w1이 hello를 받음", s.r.RC == 0 && s.has("w1 받음 hello") && hello() == "hello "+c1)
	l := s.lastLog()
	s.ok("① 기록 한 줄: 종류·종료·범용 커밋·부른 곳·기계별 결과(씀)", s.r.RC == 0 && strings.Contains(l, " 배포 종료 0 · 범용 "+c1+" · 개인 ") &&
		strings.Contains(l, " · 부른 곳 ") && strings.Contains(l, "맥=씀") && strings.Contains(l, "w1=씀") && strings.Contains(l, "lx=씀"))
	s.ok("① 도는 배포 표식은 끝나면 지운다", !ct.Exists(s.MAC+"/.local/state/herdr-sync/deploying"))
	s.run()
	s.ok("① 다시 배포: 기록에 같음", strings.Contains(s.lastLog(), "맥=같음 w1=같음 lx=같음") && logRows(T) == n0+2)
	s.run("--check")
	s.ok("① --check는 기록하지 않는다", logRows(T) == n0+2)

	// ③ --status: 기계별 판·밀린 커밋·병합 줄, 닿지 못한 기계는 모름
	c2 := s.landCommit("lane-two: 둘째")
	s.gitLand(T+"/repo", "push", "-q", "origin", "HEAD:refs/merge-queue/100-lane-x")
	s.put(LX+"/OFFLINE", "")
	before := ct.Read(T + "/mac/.local/state/herdr-sync/deploy.tsv")
	s.run("--status")
	s.ok("③ --status: origin/main·밀린 커밋(제목 머리)·병합 줄", s.r.RC == 0 && s.contains("origin/main "+c2) &&
		s.contains("배포할 것 1커밋: lane-two · 병합 줄 1건 남음 (맥 기준)") && s.contains("지금 도는 배포 없음"))
	s.ok("③ --status: 기계별 깔린 판·뒤 커밋 수", s.has("맥: "+c1+" · 뒤 1커밋") && s.has("w1: "+c1+" · 뒤 1커밋"))
	s.ok("③ --status: 닿지 못한 기계는 모름, 아무것도 쓰지 않음", s.has("lx: 모름 (닿지 않음·읽지 못함)") &&
		ct.Read(T+"/mac/.local/state/herdr-sync/deploy.tsv") == before && hello() == "hello "+c1)
	s.gitLand(T+"/repo", "push", "-q", "origin", ":refs/merge-queue/100-lane-x")

	// ② 부분 실패 → 기록에 실패 기계(lx 연결 안 됨)
	s.run()
	s.ok("② 부분 실패: 종료 1·w1은 새 판", s.r.RC == 1 && hello() == "hello "+c2)
	l = s.lastLog()
	s.ok("② 기록: 종료 1, lx=실패, w1=씀", strings.Contains(l, " 배포 종료 1 · 범용 "+c2) && strings.Contains(l, "lx=실패") && strings.Contains(l, "w1=씀"))
	os.Remove(LX + "/OFFLINE")
	s.run()

	// ④ --at <옛 커밋>: --check는 보이기만, 그다음 그 판을 깔고 기록을 맞춤 → 다음 보통 배포는 충돌 없이 새 판으로
	s.run("--at", c1, "--check", "w1")
	s.ok("④ --at --check: 무엇이 바뀔지만", s.r.RC == 1 && s.has("w1 다름:바이너리 hello (판이 다르거나 모름)") && hello() == "hello "+c2)
	s.run("--at", c1)
	s.ok("④ --at: 그 판 릴리스를 받아 깖(짓지 않음)", s.r.RC == 0 && s.has("되돌리기 bin-"+c1+" (바이너리만)") && s.has("w1 받음 hello") && hello() == "hello "+c1 &&
		!s.contains("지음"))
	s.ok("④ --at 기록: 되돌림·그 커밋", strings.Contains(s.lastLog(), " 되돌림 종료 0 · 범용 "+c1))
	s.run("--status", "w1")
	s.ok("④ --status: 되돌린 판으로 보임", s.has("w1: "+c1+" · 뒤 1커밋"))
	s.run()
	s.ok("④ 다음 보통 배포: 충돌 없이 새 판으로", s.r.RC == 0 && !s.contains("충돌") && s.has("w1 받음 hello") && hello() == "hello "+c2)

	// ⑤ 없는 커밋 → 거절·올린 판 목록, 아무것도 깔지 않음
	s.run("--at", "deadbeefdead")
	s.ok("⑤ 없는 판: 거절과 목록", s.r.RC == 1 && s.contains("그 판의 릴리스가 없다 (bin-deadbeefdead)") && s.contains(c1) && s.contains(c2) && hello() == "hello "+c2)
	s.run("--at", "nope")
	s.ok("⑤ 모르는 커밋: 거절", s.r.RC == 1 && s.contains("커밋을 모른다: nope"))

	// ⑦ 스스로 배포 끔(기본): 알림 줄만, 배포 안 함
	n := logRows(T)
	s.run("--landed")
	s.ok("⑦ 착지 뒤(밀린 것 없음): 「배포할 것 없음」", s.r.RC == 0 && s.contains("배포할 것 없음 (깔린 판 "+c2+" = origin/main)"))
	s.landCommit("lane-three: 셋째")
	s.run("--landed")
	s.ok("⑦ 끔: 알림 줄만, 배포 안 함", s.r.RC == 0 && s.contains("배포할 것 1커밋: lane-three · 병합 줄 0건 남음") && logRows(T) == n && hello() == "hello "+c2)

	// ⑦ 켬: 병합 줄이 빌 때까지 미룬 뒤 한 번, 공개 단계 없음
	s.run("--auto", "on")
	s.ok("⑦ --auto on", s.r.RC == 0 && s.has("스스로 배포 켬 ("+s.MAC+"/.local/state/herdr-sync/auto-deploy)"))
	s.gitLand(T+"/repo", "push", "-q", "origin", "HEAD:refs/merge-queue/200-lane-y")
	pub, calls := ct.Read(T+"/published"), ct.Read(T+"/gh.calls")
	c := exec.Command(s.sync, "--landed")
	c.Env = append(os.Environ(), "HERDR_SYNC_AUTO_POLL=0.1", "HERDR_SYNC_AUTO_WAIT=600")
	out, err := os.Create(T + "/landed.out")
	if err != nil {
		t.Fatal(err)
	}
	c.Stdout, c.Stderr = out, out
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	s.ok("⑦ 켬: 병합 줄이 남은 동안은 배포하지 않는다", logRows(T) == n)
	s.gitLand(T+"/repo", "push", "-q", "origin", ":refs/merge-queue/200-lane-y")
	werr := c.Wait()
	out.Close()
	s.r = ct.Res{Out: ct.Read(T + "/landed.out")}
	c3 := ct.Git(T+"/repo", "rev-parse", "HEAD")[:12]
	s.ok("⑦ 켬: 줄이 빈 뒤 한 번 배포(기록 부른 곳 스스로)", werr == nil && logRows(T) == n+1 && strings.Contains(s.lastLog(), " 부른 곳 스스로 ") && !s.contains("기다림 상한"))
	s.r = ct.Res{Out: ct.Read(T + "/landed.out")}
	s.ok("⑦ 켬: 공개 단계(내보내기·릴리스 올림) 없음, 기계는 각자 지음", s.contains("공개 건너뜀") && ct.Read(T+"/published") == pub &&
		!strings.Contains(strings.TrimPrefix(ct.Read(T+"/gh.calls"), calls), "release create") && hello() == "hello "+c3)
	s.ok("⑦ 켬: 끝 줄이 결과", s.contains("스스로 배포 끝 — 배포할 것 1커밋: lane-three"))

	// ⑧ 스스로 배포 실패 → 0 아닌 종료(hc-fill이 막힘 사건으로)
	s.putx(wb, "#!/bin/sh\necho hand\n")
	s.landCommit("lane-four: 넷째")
	s.runEnv([]string{"HERDR_SYNC_AUTO_WAIT=0"}, "--landed")
	s.ok("⑧ 실패: 종료 1·끝 줄 「스스로 배포 실패」·기록에 충돌", s.r.RC == 1 && s.contains("스스로 배포 실패 (종료 1) — 배포할 것 1커밋: lane-four") &&
		strings.Contains(s.lastLog(), "w1=충돌"))
	s.run("--auto", "off")
	s.ok("--auto off", s.r.RC == 0 && s.has("스스로 배포 끔") && !ct.Exists(s.MAC+"/.local/state/herdr-sync/auto-deploy"))
	s.run("--log", "x")
	s.ok("--log 틀린 수: 종료 2", s.r.RC == 2)
	ct.Unsetenv(s.t, "HERDR_SYNC_RELEASE_URL")
	s.ok("임시 파일이 남지 않는다", ct.Empty(T+"/tmp"))
	s.c.Done()
}
