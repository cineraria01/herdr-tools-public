package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"herdr-tools/internal/clitest"
)

// TestLogRotate (guard-log-rotate, 2026-11-23): 띄우기 전 --log 파일이 상한(HC_LAUNCHD_GUARD_LOG_MAX 바이트, 기본 8MB, 0 = 안 돌림)을
// 넘으면 <파일>.1로 옮기고(있던 .1은 덮음) 새 파일에 잇는다. 이름 바꾸기 실패면 그대로 잇고 도구는 뜬다
func TestLogRotate(t *testing.T) {
	e := setup(t)
	c := &clitest.C{T: t, Detail: func() string { return "hc 기록:\n" + e.log() }}
	old := strings.Repeat("old line\n", 20) // 180바이트
	run := func(jl string, kv ...string) string {
		return e.gArgs([]string{"--log", jl, "job1", "--", "wrap", "z"}, append([]string{"HC_FAKE_RUN_EXEC=1"}, kv...)...)
	}

	jl := filepath.Join(e.T, "rot.log")
	clitest.Put(t, jl, old)
	clitest.Put(t, jl+".1", "older\n")
	c.Ok("상한 넘음: 띄움 0", run(jl, "HC_LAUNCHD_GUARD_LOG_MAX=100") == "rc 0" && e.runs() == 1)
	c.Ok("상한 넘음: 옛 내용이 .1로(있던 .1은 덮음)", clitest.Read(jl+".1") == old)
	nl := clitest.Lines(clitest.Read(jl))
	c.Ok("상한 넘음: 새 파일 머리에 돌렸다는 한 줄", len(nl) > 0 && strings.Contains(nl[0], "job1: 기록이 180바이트(상한 100)라 "+jl+".1로 옮기고 새로 연다"))
	c.Ok("상한 넘음: 도구 출력이 새 파일에", clitest.HasLine(clitest.Read(jl), "wrap z") && !strings.Contains(clitest.Read(jl), "old line"))

	jl = filepath.Join(e.T, "under.log")
	clitest.Put(t, jl, old)
	c.Ok("상한 아래: 띄움 0", run(jl, "HC_LAUNCHD_GUARD_LOG_MAX=1000") == "rc 0")
	c.Ok("상한 아래: 그대로 이음(.1 없음)", strings.HasPrefix(clitest.Read(jl), old) && clitest.HasLine(clitest.Read(jl), "wrap z") && !clitest.Lexists(jl+".1"))

	jl = filepath.Join(e.T, "zero.log")
	clitest.Put(t, jl, old)
	c.Ok("상한 0: 돌리지 않음", run(jl, "HC_LAUNCHD_GUARD_LOG_MAX=0") == "rc 0" && strings.HasPrefix(clitest.Read(jl), old) && !clitest.Lexists(jl+".1"))

	ro := filepath.Join(e.T, "ro")
	jl = filepath.Join(ro, "job.log")
	clitest.Put(t, jl, old)
	clitest.Chmod(t, ro, 0o555)
	t.Cleanup(func() { os.Chmod(ro, 0o755) })
	c.Ok("이름 바꾸기 실패(읽기 전용 폴더): 도구는 뜸 0", run(jl, "HC_LAUNCHD_GUARD_LOG_MAX=100") == "rc 0" && e.runs() == 1)
	c.Ok("이름 바꾸기 실패: 그대로 이음(.1 없음)", strings.HasPrefix(clitest.Read(jl), old) && clitest.HasLine(clitest.Read(jl), "wrap z") && !clitest.Lexists(jl+".1"))

	jl = filepath.Join(e.T, "alive.log")
	clitest.Put(t, jl, old)
	c.Ok("터미널이 살아 있으면 돌리지 않음(상주 도구가 쥔 파일을 옮기지 않는다)",
		e.gArgs([]string{"--log", jl, "job1", "--", "wrap"}, "HC_LAUNCHD_GUARD_LOG_MAX=100", "HC_FAKE_LS="+live("t5", "job1")) == "rc 0" && !clitest.Lexists(jl+".1"))
	c.Done()
}
