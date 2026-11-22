//go:build !windows

package main

// 유닉스 전용 호출(gofix L11 — Windows 교차 빌드가 깨지지 않게 sys_windows.go와 짝)

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// killTree: 그 pid의 자손(ps 한 번으로 본 부모 관계)을 모두 KILL — 상한에 걸린 조회의 배경 손자를 남기지 않게(R-23).
// ps를 못 읽으면 아무것도 안 한다(부른 쪽이 우두머리는 끈다).
// ponytail: ps를 본 뒤 신호까지의 짧은 틈에 끝나 번호가 다시 쓰인 프로세스는 가리지 못한다 — 상한에 걸린 드문 길이라 받아들인다
func killTree(root int) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-A", "-o", "pid=", "-o", "ppid=").Output()
	if err != nil {
		return
	}
	kids := map[int][]int{}
	for _, ln := range strings.Split(string(out), "\n") {
		f := strings.Fields(ln)
		if len(f) != 2 {
			continue
		}
		p, e1 := strconv.Atoi(f[0])
		pp, e2 := strconv.Atoi(f[1])
		if e1 == nil && e2 == nil {
			kids[pp] = append(kids[pp], p)
		}
	}
	seen := map[int]bool{root: true}
	for q := []int{root}; len(q) > 0; q = q[1:] {
		for _, k := range kids[q[0]] {
			if !seen[k] {
				seen[k] = true
				q = append(q, k)
				_ = syscall.Kill(k, syscall.SIGKILL)
			}
		}
	}
}

// umask: 지금 생성 마스크(바꾸지 않고 읽는다)
func umask() int {
	um := syscall.Umask(0)
	syscall.Umask(um)
	return um
}

// closeInherited: 물려받은 fd(바깥 병합기 잠금 fd 18·19 등)를 자식에게 넘기지 않는다
func closeInherited() {
	for fd := 3; fd < 1024; fd++ {
		syscall.CloseOnExec(fd)
	}
}

// execve: 이 프로세스를 그 바이너리로 바꾼다(돌아오면 실패)
func execve(bin string, argv, env []string) error { return syscall.Exec(bin, argv, env) }
