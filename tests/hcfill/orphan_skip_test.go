package hcfill_test

// 재감사 3바퀴 fill/R-3M02: 고아 기록 폴더는 도구가 함께 쓴다. 다른 도구가 남긴 무리는 hc-fill 바퀴를 막지 않고(도구 이름 칸),
// 누구 것인지 모르는(옛 꼴)·제 무리가 남아 바퀴를 건너뜀이 이어지면 10바퀴째 주인 없는 막힘 사건 한 줄(같은 까닭이면 한 번).
// 옛 판: 남은 무리가 사라질 때까지 매 바퀴 75, 사건 0

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func init() { reg(102, "orphanskip", secOrphanSkip) }

func secOrphanSkip(k *K) {
	k.fresh()
	rec := k.T + "/proc" // kit의 HERDR_PROC_DIR
	// 우두머리는 끝나고 배경 sleep만 남은 무리 — 소유를 입증할 수 없다(신호 없이 센다)
	c := exec.Command("sh", "-c", "sleep 60 & echo $! >\"$0/os-bg.tmp\"; mv \"$0/os-bg.tmp\" \"$0/os-bg\"; exit 0", k.T)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := c.Start(); err != nil {
		k.t.Fatal(err)
	}
	pg := c.Process.Pid
	st, err := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pg)).Output()
	if err != nil {
		k.t.Fatal(err)
	}
	for i := 0; i < 300 && !nonEmpty(k.T+"/os-bg"); i++ {
		time.Sleep(20 * time.Millisecond)
	}
	c.Wait()
	defer syscall.Kill(-pg, syscall.SIGKILL)
	put := func(tail string) string {
		os.MkdirAll(rec, 0o700)
		p := fmt.Sprintf("%s/%d", rec, pg)
		write(k.t, p, fmt.Sprintf("%d\t%s\t1999999\tx%s\n", pg, strings.TrimSpace(string(st)), tail))
		return p
	}
	blocked := func() []string {
		var o []string
		for _, l := range lines(read(k.T + "/report.log")) {
			if strings.HasPrefix(l, "report 막힘 ") {
				o = append(o, l)
			}
		}
		return o
	}

	p := put("\t-\therdr-sync")
	k.run("run")
	k.check("다른 도구(herdr-sync)가 남긴 무리 → hc-fill 바퀴는 돈다", k.rc != 75 && !strings.Contains(k.out, "건너뜀"), fmt.Sprint(k.rc), k.out)
	k.check("다른 도구의 기록은 그대로(신호 없음)", nonEmpty(p) && syscall.Kill(pg, 0) != nil && aliveFile(k.T+"/os-bg"), p)

	put("") // 옛 꼴 — 누구 것인지 모른다
	var rcs []int
	for i := 0; i < 9; i++ {
		k.run("run")
		rcs = append(rcs, k.rc)
	}
	k.check("옛 꼴 기록의 남은 무리 → 9바퀴 75·사건 없음", fmt.Sprint(rcs) == "[75 75 75 75 75 75 75 75 75]" && len(blocked()) == 0, fmt.Sprint(rcs), k.out)
	k.run("run")
	b := blocked()
	k.check("10바퀴째 막힘 사건 한 줄(무리 번호·기록 경로)", k.rc == 75 && len(b) == 1 && strings.Contains(b[0], strconv.Itoa(pg)) && strings.Contains(b[0], p), fmt.Sprint(k.rc), read(k.T+"/report.log"))
	k.run("run")
	k.check("11바퀴째는 다시 내지 않음", k.rc == 75 && len(blocked()) == 1, read(k.T+"/report.log"))
	syscall.Kill(-pg, syscall.SIGKILL)
	for i := 0; i < 300 && syscall.Kill(-pg, 0) == nil; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	k.run("run")
	k.check("무리가 사라지면 바퀴가 돈다", k.rc != 75, fmt.Sprint(k.rc), k.out)
	rm(rec)
}

// aliveFile: 파일에 적힌 pid가 산다
func aliveFile(f string) bool {
	p, _ := strconv.Atoi(strings.TrimSpace(read(f)))
	return p > 1 && syscall.Kill(p, 0) == nil
}
