package main

// herdr-sync 계약 시험 18: 재감사 바퀴 3(gofix4-sync) — 원격 sh 띄우기 실패의 까닭(ops/R-3A13), 나무 모음 도중 SIGKILL(ops/R-3A22),
// --pull 원격 전송 255(ops/R-3A23). 실제 herdr-sync 진입점으로 본다

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	ct "herdr-tools/internal/clitest"
	"herdr-tools/internal/proc"
)

// fakeTarStall: 진짜 tar를 감싼다. FAKE_TAR_STALL=<파일>이고 목록(-T)이 맥 모음 나무 목록(mc<n>)이면 묶음 40960바이트만 내고
// 시작 표식(<파일>에 자기 pid)을 적은 뒤 멈춘다(exec sleep — 같은 pid). 그 밖은 진짜 tar 그대로
const fakeTarStall = `#!/bin/sh
list= prev=
for a; do [ "$prev" = -T ] && list=$a; prev=$a; done
if [ -n "${FAKE_TAR_STALL-}" ]; then case ${list##*/} in mc[0-9]*)
  REALTAR "$@" | head -c 40960; echo $$ > "$FAKE_TAR_STALL"; exec sleep 30 ;; esac; fi
exec REALTAR "$@"
`

func (s *sx) gofix4() {
	T, M, W1 := s.T, s.MAC, s.W1
	s.run()
	s.ok("gofix4 앞: 깨끗하게 끝남", s.r.RC == 0)

	// ops/R-3A13: hc는 sh 부재(os error 2)와 그 밖의 띄우기 실패(자원 35·파일 한도 24·권한 13)에 같은 kind·머리말을 낸다.
	// 부재만 「sh 없음」 건너뜀이고, 잠깐의 띄우기 실패는 연결 실패(종료 1)다
	for _, e := range []string{"Resource temporarily unavailable (os error 35)", "Too many open files (os error 24)", "Permission denied (os error 13)"} {
		s.put(W1+"/SPAWN", e)
		s.run("--check")
		s.ok("sh 띄우기 실패 「"+e+"」: --check 종료 1, 다름 (연결)", s.r.RC == 1 && s.has("w1: 다름 (연결)") && !s.contains("w1: 건너뜀"))
		s.run("--pull", "w1:.claude/docs/a.md")
		s.ok("sh 띄우기 실패 「"+e+"」: --pull은 연결 실패로 멈춘다", s.r.RC != 0 && s.contains("w1 연결 실패") && !s.contains("건너뜀 (sh 없음"))
	}
	s.put(W1+"/SPAWN", "No such file or directory (os error 2)")
	s.run("--check")
	s.ok("sh 부재(os error 2): 건너뜀, 종료 0", s.r.RC == 0 && s.has("w1: 건너뜀 (sh 없음 — Windows 등)"))
	os.Remove(W1 + "/SPAWN")
	s.run("--check")
	s.ok("SPAWN 걷은 뒤: 같음", s.r.RC == 0 && s.has("w1: 같음"))

	// ops/R-3A23: --pull의 원격 전송이 255로 끝나면 가져오지 못했다고 알리고 종료 1, 맥 파일·개인 저장소 그대로
	k := ".agents/skills/alpha/SKILL.md"
	mac0, c0 := ct.Read(M+"/"+k), s.corigin("rev-parse", "main")
	s.put(W1+"/"+k, "alpha t255\n")
	s.runEnv([]string{"HC_FAKE_T255=w1"}, "--pull", "w1:"+k)
	s.ok("--pull 원격 전송 255: 종료 1, 「가져오지 못했다」, 맥·개인 저장소 그대로", s.r.RC == 1 && s.contains("가져오지 못했다") &&
		s.contains("보냄 255") && s.is(M+"/"+k, mac0) && s.corigin("rev-parse", "main") == c0 && !s.contains("맥 가져옴 "+k))
	s.run("--take", "w1:"+k)
	s.ok("--pull 255 뒤 --take: 되돌림", s.r.RC == 0 && s.is(W1+"/"+k, mac0))

	// ops/R-202·R-3A22: 나무 모음의 tar가 40960바이트를 낸 사이 herdr-sync를 SIGKILL — 저장소 최종 파일은 옛 값 그대로,
	// 다음 실행 두 번 종료 0·일지 없음·원격에 새 값
	big, rf := s.C+"/beta/big.md", T+"/conf/skills/claude/beta/big.md"
	s.put(big, "old big\n")
	s.run()
	s.ok("나무 모음 SIGKILL 앞: 옛 값을 모았다", s.r.RC == 0 && s.is(rf, "old big\n"))
	nb := string(bytes.Repeat([]byte("0123456789abcde\n"), 8192)) // 131072바이트
	s.put(big, nb)
	real, err := exec.LookPath("tar")
	if err != nil {
		s.t.Fatal(err)
	}
	ft, pidf := T+"/faketar", T+"/tarpid"
	s.putx(ft+"/tar", strings.Replace(fakeTarStall, "REALTAR", real, 2))
	os.Remove(pidf)
	c := exec.Command(s.sync)
	c.Env = append(os.Environ(), "PATH="+ft+":"+os.Getenv("PATH"), "FAKE_TAR_STALL="+pidf)
	if err := c.Start(); err != nil {
		s.t.Fatal(err)
	}
	for i := 0; i < 600 && !ct.Exists(pidf); i++ { // 시작 표식: 가짜 tar가 묶음을 내기 시작함
		time.Sleep(100 * time.Millisecond)
	}
	var tp int
	fmt.Sscanf(ct.Read(pidf), "%d", &tp)
	if tp == 0 {
		c.Process.Kill()
		c.Wait()
		s.t.Fatal("가짜 tar가 나무 모음에서 돌지 않았다")
	}
	// 받는 쪽 tar가 그 40960바이트를 어딘가(준비 폴더 또는 최종 경로)에 쓸 때까지
	wrote := func() bool {
		if !s.is(rf, "old big\n") {
			return true
		}
		fi, err := os.Stat(T + "/conf/.git/herdr-sync-stage/big.md")
		return err == nil && fi.Size() > 0
	}
	for i := 0; i < 100 && !wrote(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	c.Process.Kill()
	c.Wait()
	if p, err := os.FindProcess(tp); err == nil && proc.Alive(tp) { // 자기가 띄운 가짜 tar(sleep)만 끈다
		p.Kill()
	}
	s.ok("나무 모음 도중 SIGKILL: 저장소 최종 파일은 옛 값 그대로", s.is(rf, "old big\n"))
	jp := M + "/.local/state/herdr-sync/repo-journal.tsv"
	s.run()
	r1 := s.r.RC
	s.run()
	s.ok("SIGKILL 뒤 다음 실행 두 번 종료 0, 일지 없음, 원격에 새 값", r1 == 0 && s.r.RC == 0 && !ct.Exists(jp) &&
		s.corigin("show", "main:skills/claude/beta/big.md") == strings.TrimSuffix(nb, "\n") && s.is(rf, nb))
}
