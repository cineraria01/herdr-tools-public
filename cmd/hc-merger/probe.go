package main

// 기계 점검 항목(ht-probe-cordon, 2026-11-28, docs/hc-fill/cordon.md 「점검 항목」 — 업계의 합성 점검: 레인이 하는 일을 작게 끝까지 해 본다):
// hc-fill이 바퀴마다 이미 부르는 --spec-idle --repo-read 조각에 키체인·임시 폴더를, env HC_LANE_PROBE_POOL=<명령>이면 에이전트 풀(<명령>
// status의 쓸 계정 수)을 함께 재어 「probe <항목> ok|bad|na|unknown <글>」 한 줄씩 낸다(--repo-read 줄들 앞). 셋 다 동시에, 각자 probeMax 안.
// 판정이 그 기계 안에서 분명한 것만 여기서 가른다(키체인 종료 36 · 임시 파일 쓰기 오류·안 끝남 · 쓸 계정 0) — 풀 출력(계정 이름)은 수 말고 싣지 않는다.
// 풀 명령을 인자가 아닌 env로 받는 까닭: 옛 판은 env를 모른 채 그대로 돌아(인자 거절 → 다시 부름 없이) 그 줄만 없다 = 모름

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"herdr-tools/internal/jobtmp"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/workermac"
)

// probeMax: 항목 하나의 상한(시험이 줄인다) — 저장소 읽기 상한과 같아 조각 시간을 늘리지 않는다
var probeMax = 5 * time.Second

// keychainArgv: 키체인 점검 명령(hc-sweep 건강 점검·worker-mac-check와 같은 명령 — 시험이 바꾼다)
var keychainArgv = func() []string { return workermac.KeychainArgs(os.Getenv("HOME")) }

// probeLines: 항목마다 한 줄(차례 고정 — 키체인·임시 폴더·풀)
func probeLines() []string {
	ps := []func() string{probeKeychain, probeTmp}
	if c := os.Getenv("HC_LANE_PROBE_POOL"); c != "" {
		ps = append(ps, func() string { return probePool(c) })
	}
	o := make([]string, len(ps))
	var wg sync.WaitGroup
	for i, p := range ps {
		wg.Add(1)
		go func() { defer wg.Done(); o[i] = p() }()
	}
	wg.Wait()
	return o
}

// probeKeychain: 0 열림 · 36 막힘(User interaction is not allowed) · 그 밖·안 끝남은 모름 · macOS 아니면 해당 없음
func probeKeychain() string {
	if runtime.GOOS != "darwin" {
		return "probe keychain na macOS 아님"
	}
	a := keychainArgv()
	err := proc.RunMax(exec.Command(a[0], a[1:]...), probeMax)
	var ee *exec.ExitError
	switch {
	case err == nil:
		return "probe keychain ok 열림"
	case errors.As(err, &ee) && ee.ExitCode() == workermac.KeychainLocked:
		return "probe keychain bad rc 36 User interaction is not allowed"
	case errors.Is(err, proc.ErrTimeout):
		return "probe keychain unknown " + probeMax.String() + " 안에 안 끝남"
	}
	return "probe keychain unknown " + flat(err.Error())
}

// tmpRoot·tmpTryFn: 임시 뿌리 설정과 쓰기 한 번(시험이 바꾼다)
var (
	tmpRoot  = func() (string, bool) { return confGet(jobtmp.Key, jobtmp.Default) }
	tmpTryFn = tmpTry
)

// probeTmp: 작업별 임시 뿌리(설정 jobtmp.root — 레인이 TMPDIR을 만드는 곳)에 파일 하나 만들고 지우기. 쓰기 오류·안 끝남은 나쁨(저장소 읽기와
// 같은 까닭 — 막힌 쓰기는 레인도 막는다), 뿌리를 끈 기계는 해당 없음, 설정을 못 읽으면 모름
func probeTmp() string {
	root, ok := tmpRoot()
	switch {
	case !ok:
		return "probe tmp unknown 설정 " + jobtmp.Key + "을 못 읽음"
	case root == "" || root == "off":
		return "probe tmp na 작업 임시 폴더 끔"
	}
	r := jobtmp.Expand(root, os.Getenv("HOME"))
	ch := make(chan string, 1)
	try := tmpTryFn
	go func() { ch <- try(r) }()
	t := time.NewTimer(probeMax)
	defer t.Stop()
	select {
	case s := <-ch:
		return "probe tmp " + s
	case <-t.C: // 막힌 쓰기는 그대로 둔다(끝낼 수 없다) — 프로세스가 끝나며 거둔다
		return fmt.Sprintf("probe tmp bad %s 쓰기가 %s 안에 안 끝남", r, probeMax)
	}
}

func tmpTry(r string) string {
	if err := os.MkdirAll(r, 0o700); err != nil {
		return "bad " + flat(err.Error())
	}
	f, err := os.CreateTemp(r, ".probe-*")
	if err != nil {
		return "bad " + flat(err.Error())
	}
	_, werr := f.WriteString("probe\n")
	if err := errors.Join(werr, f.Close(), os.Remove(f.Name())); err != nil {
		return "bad " + flat(err.Error())
	}
	return "ok " + r
}

// probePool: <명령> status의 쓸 계정 수(worker-mac-check와 같은 판정 — workermac.PoolUsable). 0이라고 분명히 말할 때만 나쁨,
// 명령 없음·안 끝남·수를 못 읽음은 모름. 출력은 싣지 않는다(계정 이름)
func probePool(cmd string) string {
	var ob bytes.Buffer
	c := exec.Command(cmd, "status")
	c.Stdout, c.Stderr = &ob, &ob
	err := proc.RunMax(c, probeMax)
	n, ok := workermac.PoolUsable(ob.String())
	switch {
	case errors.Is(err, proc.ErrTimeout):
		return "probe pool unknown " + probeMax.String() + " 안에 안 끝남"
	case errors.Is(err, exec.ErrNotFound):
		return "probe pool unknown 명령 없음"
	case !ok:
		rc := "0"
		if err != nil {
			rc = flat(err.Error())
		}
		return "probe pool unknown 쓸 계정 수를 못 읽음(" + rc + ")"
	case n == 0:
		return "probe pool bad 쓸 계정 0"
	}
	return fmt.Sprintf("probe pool ok 쓸 계정 %d", n)
}
