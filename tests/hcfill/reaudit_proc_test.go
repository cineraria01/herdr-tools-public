package hcfill_test

// gofix2-core(Go 전환 재감사 아스트라 1바퀴) — fill R-06: 앞 바퀴(SIGKILL로 죽음)가 남긴 자식 무리의 소유를 모른 채 그 우두머리가
// 아직 돌면 이번 바퀴를 미룬다(75 — 그 자식이 쥔 잠금과 겹쳐 돌지 않게). 옛 판: 모르는 기록을 두고 바퀴를 그대로 돌았다(종료 0). Go 판만

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func init() { reg(97, "k2", secK2Reaudit) }

func secK2Reaudit(k *K) {
	k.fresh()
	c := exec.Command("sleep", "60")
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := c.Start(); err != nil {
		k.t.Fatal(err)
	}
	defer func() { syscall.Kill(-c.Process.Pid, syscall.SIGKILL); c.Wait() }()
	os.MkdirAll(k.T+"/proc", 0o700)
	// 부모(죽은 앞 바퀴)·시작 시각 없음 — 그 무리가 기록한 그것인지 입증할 수 없다(거두지도 지우지도 않는다)
	rec := fmt.Sprintf("%s/proc/%d", k.T, c.Process.Pid)
	if err := os.WriteFile(rec, []byte(fmt.Sprintf("%d\t\t1999999\tx\n", c.Process.Pid)), 0o600); err != nil {
		k.t.Fatal(err)
	}
	k.run("run")
	k.check("R-06 앞 바퀴 자식의 소유를 모르고 우두머리가 돔: 이번 바퀴 75, 그 무리·기록은 그대로",
		k.rc == 75 && strings.Contains(k.out, "아직 도는지 모른다") && syscall.Kill(c.Process.Pid, 0) == nil && exists(rec), k.out)
	os.Remove(rec)
	k.run("run")
	k.check("R-06 기록을 치운 뒤: 바퀴가 돈다", k.rc == 0, k.out)
}
