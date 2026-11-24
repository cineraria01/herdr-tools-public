package main

// 20. 스스로 배포가 실제로 워커까지 깔리게(2026-11-24 sync-auto-fix — 첫 실행에서 맥만 깔리고 워커 8대가 실패):
// ① 릴리스 없는 커밋 + 스스로 배포(기본) → 맥에도 안 깔고 「스스로 배포 못 함」·종료 ≠ 0 (스스로 배포의 자식 배포도 같은 문턱)
// ② 그 커밋 릴리스가 이미 있으면 → 모든 기계에 받아 깖 ③ --auto-publish on → 공개 단계까지(릴리스 올림) 하고 모든 기계에 깖
// ④ 원격 Go 소스 폴더에 저장소에서 지운 파일이 남은 기계 → 그 기계에서 짓기 성공(남은 파일은 짓기에 끼지 않음)
// ⑤ 그 기계에서 따로 고친 소스 파일 → 지금처럼 충돌, 그 기계 바이너리는 못 깖 — 끝 요약과 --status에 「맥만 새 판」.
// 가짜 gh·file:// 릴리스·가짜 기계(실제 배포·공개 없음).

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	ct "herdr-tools/internal/clitest"
)

// fakeRelease: 가짜 gh 저장소에 bin-<c12> 릴리스(hello 하나 — 부르면 「rel <c12>」)를 둔다(공개 단계를 거치지 않고 「이미 올라 있음」)
func (s *sx) fakeRelease(c12 string) {
	d := s.T + "/rel/bin-" + c12
	body := "#!/bin/sh\necho rel " + c12 + "\n"
	s.put(d+"/hello", body)
	h := sha256.Sum256([]byte(body))
	s.put(d+"/SHA256SUMS", hex.EncodeToString(h[:])+"  hello\n")
}

func TestContractAutoFix(t *testing.T) {
	s := newSX(t)
	s.synced()
	s.relPrep()
	T, W1 := s.T, s.W1
	run := func(p string) string { return strings.TrimSpace(ct.Cmd{}.Run(p).Out) }
	wb, mb := W1+"/.local/bin/hello", s.MAC+"/.local/bin/hello"
	s.run()
	c1 := ct.Git(T+"/repo", "rev-parse", "HEAD")[:12]
	if s.r.RC != 0 || run(wb) != "hello "+c1 {
		t.Fatalf("준비: 첫 배포 종료 %d\n%s", s.r.RC, s.r.Out)
	}
	s.run("--auto", "on")

	// ① 릴리스 없는 커밋 + 스스로 배포(기본): 맥에도 안 깔고 못 함·종료 ≠ 0, 배포 기록도 없음
	n := logRows(T)
	c2 := s.landCommit("lane-a: 하나")
	s.runEnv([]string{"HERDR_SYNC_AUTO_WAIT=0"}, "--landed")
	s.ok("① 릴리스 없음: 종료 ≠ 0·「스스로 배포 못 함」 한 줄", s.r.RC != 0 &&
		s.contains("스스로 배포 못 함: 이 커밋의 릴리스가 없다 (bin-"+c2+") — herdr-sync로 배포하거나 herdr-sync --auto-publish on"))
	s.ok("① 맥·w1 모두 옛 판 그대로, 배포 기록 없음", run(mb) == "hello "+c1 && run(wb) == "hello "+c1 && logRows(T) == n)
	// 착지 뒤 확인과 자식 배포 사이에 새 착지가 들어와도(릴리스 없는 커밋) 자식 배포가 같은 문턱에서 멈춘다
	s.runEnv([]string{"HERDR_SYNC_NO_PUBLISH=1", "HERDR_SYNC_BY=스스로"})
	s.ok("① 자식 배포도 릴리스가 없으면 아무 데도 안 깖", s.r.RC != 0 && s.contains("스스로 배포 못 함: 이 커밋의 릴리스가 없다 (bin-"+c2+")") &&
		run(mb) == "hello "+c1 && run(wb) == "hello "+c1 && !s.contains("지음"))

	// ② 그 커밋 릴리스가 이미 있으면 → 모든 기계에 받아 깖(공개 단계 없음)
	c3 := s.landCommit("lane-b: 둘")
	s.fakeRelease(c3)
	pub := ct.Read(T + "/published")
	s.runEnv([]string{"HERDR_SYNC_AUTO_WAIT=0"}, "--landed")
	s.ok("② 릴리스 있음: 맥·w1 받음", s.r.RC == 0 && s.contains("스스로 배포 끝 — 배포할 것 2커밋") && run(mb) == "rel "+c3 && run(wb) == "rel "+c3 &&
		ct.Read(T+"/published") == pub)

	// ③ --auto-publish on → 공개 단계(내보내기·릴리스 올림)까지 하고 모든 기계에 깖
	s.run("--auto-publish", "on")
	s.ok("③ --auto-publish on", s.r.RC == 0 && s.has("스스로 배포가 공개 단계까지 한다 ("+s.MAC+"/.local/state/herdr-sync/auto-publish)"))
	s.run("--status")
	s.ok("③ --status에 공개 손잡이", s.has("스스로 배포가 공개 단계까지 한다 ("+s.MAC+"/.local/state/herdr-sync/auto-publish)"))
	c4 := s.landCommit("lane-c: 셋")
	s.runEnv([]string{"HERDR_SYNC_AUTO_WAIT=0"}, "--landed")
	s.ok("③ 공개·릴리스 올림 뒤 맥·w1 받음", s.r.RC == 0 && ct.Read(T+"/published") != pub && ct.Exists(T+"/rel/bin-"+c4+"/SHA256SUMS") &&
		s.contains("w1 받음 hello") && run(mb) == "hello "+c4 && run(wb) == "hello "+c4)
	s.run("--auto-publish", "off")
	s.ok("③ --auto-publish off", s.r.RC == 0 && s.has("스스로 배포 공개 단계 끔") && !ct.Exists(s.MAC+"/.local/state/herdr-sync/auto-publish"))
	s.run("--auto", "off")

	// ④ 원격 소스 폴더에 저장소에서 지운 파일이 남음(그중 하나는 함께 지으면 깨지는 .go) → 그 기계에서 짓기 성공
	g := W1 + "/.local/lib/herdr-go"
	s.put(g+"/cmd/hello/usage.txt", "옛 쓰임\n")
	s.put(g+"/cmd/hello/gone.go", "package main\n\nfunc main() {}\n")
	c5 := s.landCommit("lane-d: 넷")
	s.runEnv([]string{"PUB_FAKE_RC=1"}) // 공개 실패 → 릴리스 없음 → 기계마다 짓기
	s.ok("④ 지운 파일이 남은 기계: 지음", s.contains("w1 지음 hello") && run(wb) == "hello "+c5 && ct.Exists(g+"/cmd/hello/usage.txt"))

	// ⑤ 그 기계에서 따로 고친 소스 파일 → 충돌(덮지 않음), 그 기계 바이너리는 못 깖 — 「맥만 새 판」
	s.put(g+"/internal/version/version.go", "package version\n\nvar Commit = \"hand\"\n")
	c6 := s.landCommit("lane-e: 다섯")
	s.runEnv([]string{"PUB_FAKE_RC=1"})
	s.ok("⑤ 따로 고친 파일: 충돌·덮지 않음", s.r.RC != 0 && s.contains("충돌 w1 .local/lib/herdr-go/internal/version/version.go") &&
		ct.Read(g+"/internal/version/version.go") == "package version\n\nvar Commit = \"hand\"\n" && run(wb) == "hello "+c5)
	s.ok("⑤ 끝 요약: 맥만 새 판", run(mb) == "hello "+c6 && s.contains("맥만 새 판 "+c6+" — 워커 1대 옛 판 (w1)"))
	s.run("--status")
	s.ok("⑤ --status: 맥만 새 판", s.contains("맥만 새 판 "+c6+" — 워커 1대 옛 판 (w1)"))
	ct.Unsetenv(s.t, "HERDR_SYNC_RELEASE_URL")
	s.c.Done()
}
