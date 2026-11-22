package main

// herdr-sync 계약 시험 17: 실패 주입(Go 판 — 2026-11-22 gofix-ops, 감사 O-06). git·조각이 실패하거나 시간을 넘기거나 파일을 못 읽을 때
// 그것을 깨끗함·없음·성공으로 읽어 맥·저장소·기계를 바꾸지 않는다. 셸 판도 같은 결함(B01·B02)이 있지만 셸 판은 곧 걷어 알려진 차이로 둔다.
// 가짜 git(PATH 앞): GIT_FAIL_SUB(빈칸으로 여럿)의 하위 명령이면 출력 없이 128, GIT_SLEEP_SUB면 3초 잔 뒤 진짜 git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	ct "herdr-tools/internal/clitest"
	"herdr-tools/internal/proc"
)

const fakeGit = `#!/bin/sh
sub= skip=
for a; do
  if [ -n "$skip" ]; then skip=; continue; fi
  case $a in -C | -c) skip=1 ;; -*) ;; *) sub=$a; break ;; esac
done
case " ${GIT_FAIL_SUB-} " in *" $sub "*) exit 128 ;; esac
if [ "$sub" = "${GIT_SLEEP_SUB-}" ]; then
  [ -z "${GIT_PID_FILE-}" ] || echo $$ > "$GIT_PID_FILE"
  sleep "${GIT_SLEEP_SECS:-3}"
fi
exec REALGIT "$@"
`

// killMid: herdr-sync를 띄워 가짜 git이 sub에서 자는 동안 SIGKILL한다(부모만 — 자식 무리는 남는다). 자던 가짜 git의 pid
func (s *sx) killMid(path, sub string) int { return s.signalMid(path, sub, os.Kill) }

// signalMid: killMid와 같되 신호를 고른다
func (s *sx) signalMid(path, sub string, sig os.Signal) int {
	pidf := s.T + "/gitpid"
	os.Remove(pidf)
	c := exec.Command(s.sync)
	c.Env = append(os.Environ(), path, "GIT_SLEEP_SUB="+sub, "GIT_SLEEP_SECS=30", "GIT_PID_FILE="+pidf)
	if err := c.Start(); err != nil {
		s.t.Fatal(err)
	}
	for i := 0; i < 600 && !ct.Exists(pidf); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	// 부모가 그 무리를 고아 기록(proc.RecordDir()/<무리>)에 적은 뒤에 죽인다 — 띄운 뒤 적기 전에 죽이면 다음 실행이 거둘 무리를 몰라
	// 부하에서 「push 중에 죽은 뒤 … 거둔다」가 흔들렸다(2026-11-22 verify-parallel — mini-m4 부하 30·병렬 검증에서 두 번, 작업 중 발견해 함께 고침)
	var gp int
	fmt.Sscanf(ct.Read(pidf), "%d", &gp)
	if d := proc.RecordDir(); d != "" && gp > 0 {
		for i := 0; i < 100 && !ct.Exists(filepath.Join(d, fmt.Sprint(gp))); i++ {
			time.Sleep(100 * time.Millisecond)
		}
	}
	c.Process.Signal(sig)
	s.sigRC = -1
	if err := c.Wait(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			s.sigRC = ee.ExitCode()
		}
	} else {
		s.sigRC = 0
	}
	var pid int
	fmt.Sscanf(ct.Read(pidf), "%d", &pid)
	if pid == 0 {
		s.t.Fatalf("가짜 git %s가 돌지 않았다", sub)
	}
	return pid
}

// relist: 개인 저장소 sync.list를 바꿔 커밋·push(시험 준비)
func (s *sx) relist(content string) {
	s.put(s.T+"/conf/sync.list", content)
	s.git(s.T+"/conf", "commit", "-qam", "list")
	s.gitLand(s.T+"/conf", "push", "-q", "origin", "main")
}

func (s *sx) failures() {
	T, M, W1 := s.T, s.MAC, s.W1
	real, err := exec.LookPath("git")
	if err != nil {
		s.t.Fatal(err)
	}
	fg := T + "/fakegit"
	s.putx(fg+"/git", strings.Replace(fakeGit, "REALGIT", real, 1))
	path := "PATH=" + fg + ":" + os.Getenv("PATH")
	s.run()
	s.ok("실패 주입 앞: 깨끗하게 끝남", s.r.RC == 0)
	bal := M + "/.local/bin/herdr-balance"
	balWant := ct.Read(bal)

	// B01·O-01·K-01: 저장소가 더러운데 git status가 실패·시간 넘김이면 깨끗함으로 읽지 않는다(커밋 안 된 내용을 맥에 깔지 않음)
	s.put(T+"/repo/bin/herdr-balance", "#!/bin/sh\necho uncommitted\n")
	s.runEnv([]string{path, "GIT_FAIL_SUB=status"})
	s.ok("git status 실패: 멈추고 커밋 안 된 내용을 깔지 않는다", s.r.RC != 0 && s.contains("저장소 상태를 읽지 못했다") && s.is(bal, balWant))
	s.runEnv([]string{path, "GIT_SLEEP_SUB=status", "HERDR_TIMEOUT_GIT=1"})
	s.ok("git status 시간 넘김: 모름 — 멈추고 깔지 않는다", s.r.RC != 0 && s.contains("저장소 상태를 읽지 못했다") && s.is(bal, balWant))
	s.git(T+"/repo", "checkout", "--", "bin/herdr-balance")

	// B02: 보낼 커밋 조회(rev-list)가 실패하면 「보낼 것 없음」으로 읽고 base만 적지 않는다
	s.put(bal, "#!/bin/sh\necho bal 2\n")
	before := s.origin("rev-parse", "main")
	s.runEnv([]string{path, "GIT_FAIL_SUB=rev-list"})
	s.ok("git rev-list 실패: push를 건너뛰고 끝내지 않는다(종료 1, 원격 그대로)", s.r.RC != 0 && s.contains("보낼 커밋을 알지 못했다") && s.origin("rev-parse", "main") == before)
	s.run()
	s.ok("다음 실행: 남은 커밋을 보낸다", s.r.RC == 0 && strings.Contains(s.origin("show", "main:bin/herdr-balance"), "bal 2"))
	balWant = ct.Read(bal)

	// A01·O-08: 저장소 파일을 못 읽으면(prebuild가 권한을 뺌) 「저장소에서 지워짐」으로 맥 파일을 치우지 않는다
	rb := T + "/repo/bin/herdr-balance"
	s.editLines(T+"/conf/config", func(l string) (string, bool) {
		if strings.HasPrefix(l, "sync.prebuild ") {
			return l + "; chmod 000 " + rb, true
		}
		return l, true
	})
	s.git(T+"/conf", "commit", "-qam", "prebuild chmod")
	s.gitLand(T+"/conf", "push", "-q", "origin", "main")
	s.run()
	os.Chmod(rb, 0o755)
	s.ok("저장소 파일 읽기 실패: 멈추고 맥 파일을 지우지 않는다", s.r.RC != 0 && s.contains("저장소 파일을 읽지 못했다") && s.is(bal, balWant) && !s.contains("맥 지움 .local/bin/herdr-balance"))
	s.editLines(T+"/conf/config", func(l string) (string, bool) {
		if strings.HasPrefix(l, "sync.prebuild ") {
			return strings.TrimSuffix(l, "; chmod 000 "+rb), true
		}
		return l, true
	})
	s.git(T+"/conf", "commit", "-qam", "prebuild back")
	s.gitLand(T+"/conf", "push", "-q", "origin", "main")
	s.run()
	s.ok("되돌린 뒤: 깨끗하게 끝남", s.r.RC == 0)

	// B18: 맥 원본을 못 읽으면 「맥에 없음」으로 읽어 저장소에서 지우지 않는다
	os.Chmod(bal, 0o000)
	before = s.origin("rev-parse", "main")
	s.run()
	os.Chmod(bal, 0o755)
	s.ok("맥 원본 읽기 실패: 멈추고 저장소에서 지우지 않는다", s.r.RC != 0 && s.origin("rev-parse", "main") == before && !s.contains("맥 모음 .local/bin/herdr-balance"))

	// B08: 나무 안 맥 파일을 못 읽으면(cksum 실패) 그 파일을 없음으로 읽지 않는다
	var tf string
	filepath.WalkDir(M+"/.agents/skills", func(p string, d os.DirEntry, err error) error {
		rel := strings.TrimPrefix(p, M+"/.agents/skills/")
		if err == nil && tf == "" && d.Type().IsRegular() && !strings.HasPrefix(rel, "herdr-desktop/") && !strings.Contains("/"+rel, "/.") {
			tf = p
		}
		return nil
	})
	if tf == "" {
		s.t.Fatal("나무 파일을 못 찾았다")
	}
	cb := s.corigin("rev-parse", "main")
	os.Chmod(tf, 0o000)
	s.run()
	os.Chmod(tf, 0o644)
	s.ok("나무 파일 읽기 실패: 멈추고 개인 저장소에서 지우지 않는다", s.r.RC != 0 && s.corigin("rev-parse", "main") == cb && s.contains("나무를 읽지 못했다"))
	s.run()
	s.ok("되돌린 뒤: 깨끗하게 끝남(나무)", s.r.RC == 0)

	// B10: 기계 조회가 잘려 OS 줄이 없으면 「OS 알 수 없음」 건너뜀(종료 0)이 아니라 연결 실패
	s.runEnv([]string{"HC_FAKE_NOO=w1"})
	s.ok("조회가 잘림(O 줄 없음): 연결 실패로 종료 1", s.r.RC != 0 && s.contains("w1 연결 실패") && !s.contains("w1: 건너뜀"))

	// B09: 기계에서 base를 못 적으면(상태 폴더 쓰기 금지) 그 기계를 「같음」으로 보이지 않는다
	st := W1 + "/.local/state/herdr-sync"
	s.put(M+"/.claude/docs/a.md", "a fail\n")
	s.put(W1+"/.claude/docs/a.md", "a fail\n") // 내용은 이미 같고 base만 낡음 → B만 돈다
	os.Chmod(st, 0o555)
	s.run()
	os.Chmod(st, 0o755)
	s.ok("기계 base 쓰기 실패: 그 기계 줄이 같음이 아니고 종료 1", s.r.RC != 0 && !s.has("w1: 같음") && s.contains("w1 base를 쓰지 못했다"))
	s.run()
	s.ok("되돌린 뒤: 깨끗하게 끝남(base)", s.r.RC == 0)

	// B05·A06: git add 뒤 commit 중에 부모만 SIGKILL — 다음 실행이 남은 자식 무리를 거두고, 일지로 자기 스테이징만 되돌려 다시 모은다
	s.put(bal, "#!/bin/sh\necho bal 3\n")
	gp := s.killMid(path, "commit")
	s.run()
	s.ok("모으다 죽은 뒤: 남은 commit 무리를 거두고 스테이징을 되돌린 뒤 다시 모아 보낸다", s.r.RC == 0 && s.contains("거둔다") &&
		s.contains("스테이징을 되돌렸다") && strings.Contains(s.origin("show", "main:bin/herdr-balance"), "bal 3") && !proc.Alive(gp))
	s.put(bal, "#!/bin/sh\necho bal 4\n")
	gp = s.killMid(path, "push")
	s.run()
	s.ok("push 중에 죽은 뒤: 남은 push 무리를 거두고 다시 보낸다(겹쳐 돌지 않음)", s.r.RC == 0 && s.contains("거둔다") &&
		strings.Contains(s.origin("show", "main:bin/herdr-balance"), "bal 4") && !proc.Alive(gp))
	// O-04: Ctrl-C(INT)·TERM이면 따로 띄운 push 무리를 거두고 임시 폴더를 지운 뒤 130·143으로 끝난다
	s.put(bal, "#!/bin/sh\necho bal 4b\n")
	tmpBefore, _ := os.ReadDir(T + "/tmp")
	gp = s.signalMid(path, "push", os.Interrupt)
	tmpAfter, _ := os.ReadDir(T + "/tmp")
	for i := 0; i < 30 && proc.Alive(gp); i++ { // 거둔 자식이 init에 거둬질 때까지
		time.Sleep(100 * time.Millisecond)
	}
	s.ok("INT: push 무리를 거두고 임시 폴더를 지우고 130(또는 INT로 끝남)", (s.sigRC == 130 || s.sigRC == -1) && !proc.Alive(gp) && len(tmpAfter) == len(tmpBefore))
	s.run()
	s.ok("INT 뒤 다시: 남은 커밋을 보낸다", s.r.RC == 0 && strings.Contains(s.origin("show", "main:bin/herdr-balance"), "bal 4b"))
	// 일지와 다르게 바뀐 스테이징은 건드리지 않고 멈춘다(남의 손일 수 있다)
	s.put(bal, "#!/bin/sh\necho bal 5\n")
	s.killMid(path, "commit")
	s.put(T+"/repo/bin/herdr-balance", "#!/bin/sh\necho someone\n")
	s.run()
	s.ok("일지와 다른 스테이징: 되돌리지 않고 멈춘다", s.r.RC != 0 && s.contains("바뀌었다") && s.is(T+"/repo/bin/herdr-balance", "#!/bin/sh\necho someone\n"))
	s.git(T+"/repo", "reset", "-q", "--", "bin/herdr-balance")
	s.git(T+"/repo", "checkout", "--", "bin/herdr-balance")
	os.Remove(M + "/.local/state/herdr-sync/repo-journal.tsv")
	s.run()
	s.ok("되돌린 뒤: 깨끗하게 끝남(일지)", s.r.RC == 0 && strings.Contains(s.origin("show", "main:bin/herdr-balance"), "bal 5"))

	// ops/R-215: 실제 진입점에서 commit 실패 뒤 되돌림(reset)까지 실패하면 일지를 남긴다 — 장애를 걷은 다음 실행이 그 일지로 되돌리고 다시 모은다.
	// 원인 먼저: 첫 실행이 실제로 커밋 단계까지 가 스테이징했는지(색인에 새 값) 본 뒤 일지·성공 줄·대상 파일을 함께 단언한다
	jp := M + "/.local/state/herdr-sync/repo-journal.tsv"
	rbal := T + "/repo/bin/herdr-balance"
	before = s.origin("rev-parse", "main")
	s.put(bal, "#!/bin/sh\necho bal 6\n")
	s.runEnv([]string{path, "GIT_FAIL_SUB=commit reset"})
	s.ok("commit·reset 실패: 커밋 단계까지 갔다(색인에 새 값)", strings.Contains(s.git(T+"/repo", "show", ":bin/herdr-balance"), "bal 6"))
	s.ok("commit·reset 실패: 종료 1, 일지 남음, 성공 줄 없음, 원격 그대로", s.r.RC != 0 && ct.Exists(jp) &&
		s.contains("일지") && !s.contains("맥 모음 .local/bin/herdr-balance") && s.origin("rev-parse", "main") == before)
	s.run()
	s.ok("장애를 걷은 뒤: 일지로 되돌리고 다시 모아 보낸다, 일지 없음, 저장소 깨끗", s.r.RC == 0 && s.contains("스테이징을 되돌렸다") &&
		!ct.Exists(jp) && strings.Contains(s.origin("show", "main:bin/herdr-balance"), "bal 6") && s.git(T+"/repo", "status", "--porcelain") == "" &&
		s.is(rbal, "#!/bin/sh\necho bal 6\n"))
	// commit만 실패: 되돌림이 되면 일지를 지우고 저장소를 쓰기 전으로(색인·작업 트리 = HEAD)
	before = s.origin("rev-parse", "main")
	s.put(bal, "#!/bin/sh\necho bal 7\n")
	s.runEnv([]string{path, "GIT_FAIL_SUB=commit"})
	s.ok("commit 실패: 종료 1, 되돌려 일지 없음·저장소 깨끗·원격 그대로", s.r.RC != 0 && !ct.Exists(jp) && s.git(T+"/repo", "status", "--porcelain") == "" &&
		s.is(rbal, "#!/bin/sh\necho bal 6\n") && s.origin("rev-parse", "main") == before && !s.contains("맥 모음 .local/bin/herdr-balance"))
	s.run()
	s.ok("commit 실패 뒤 다시: 모아 보낸다", s.r.RC == 0 && strings.Contains(s.origin("show", "main:bin/herdr-balance"), "bal 7"))

	// C04·O-10: 끝 줄바꿈 없는 마지막 줄(셸 판은 버린다)·CR 줄은 판마다 갈리니 맞추기 전에 멈춘다. B11: 뿌리 밖 경로는 멈춘다
	orig := ct.Read(T + "/conf/sync.list")
	s.relist(orig + "지움     .local/bin/herdr-balance       -        local")
	s.run()
	s.ok("sync.list 끝 줄바꿈 없음: 종료 2, 아무것도 안 지움", s.r.RC == 2 && s.contains("끝 줄바꿈이 없다") && ct.Exists(bal))
	s.relist(orig + "맥도구   .local/bin/x\r\n")
	s.run()
	s.ok("sync.list 줄 끝 CR: 종료 2", s.r.RC == 2 && s.contains("CR"))
	for _, l := range []string{"맥도구   ../out/x   tools:bin/x   local\n", "맥도구   .local/bin/x   tools:../x   local\n", "지움   /etc/x   -   local\n"} {
		s.relist(orig + l)
		s.run()
		s.ok("sync.list 뿌리 밖 경로 멈춤: "+strings.Fields(l)[1]+" "+strings.Fields(l)[2], s.r.RC == 2 && s.contains("뿌리 밖"))
	}
	s.relist(orig)
	s.run()
	s.ok("되돌린 뒤: 깨끗하게 끝남(목록)", s.r.RC == 0)
}
