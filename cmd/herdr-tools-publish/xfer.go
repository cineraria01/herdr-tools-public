package main

// 공개 쪽으로 옮기기·공개 저장소에 닿는 git·결과 파일 (2026-11-21 Go 전환 감사 A02·A12·B04)

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"herdr-tools/internal/atomic"
	"herdr-tools/internal/gitx"
	"herdr-tools/internal/proc"
)

// remote: 공개 저장소에 닿는 git(ls-remote·fetch·push) — git 오류 글의 publish.repo 주소 원문을 키 이름으로 바꿔 찍는다
func remote(dir, url string, args ...string) (string, bool) {
	var e bytes.Buffer
	out, rc := gitx.Out("", &e, append([]string{"-c", "user.useConfigOnly=true", "--git-dir=" + dir}, args...)...)
	os.Stderr.WriteString(strings.ReplaceAll(e.String(), url, "publish.repo"))
	return out, rc == 0
}

// done: 게시 성공(내보냄·같음) — 결과 파일이 있으면 검사한 비공개 커밋 sha 한 줄을 반쪽 없이 쓴다(herdr-sync가 그 커밋으로 릴리스를 짓는다)
func done(res, src string) {
	if res == "" {
		return
	}
	if err := atomic.WriteFile(res, []byte(src+"\n"), 0o644); err != nil {
		die("결과 파일을 쓰지 못했다: %v", err)
	}
}

// transfer: 비공개 저장소의 tree와 목록 객체를 pack-objects | unpack-objects로 공개 쪽에 옮기고 rev-list --objects로 확인.
// 두 git을 한 전송으로 묶어 로컬 git 상한(HERDR_TIMEOUT_GIT, proc.Limit) 안에서만 기다린다(B04). 넘기면 두 무리를 다 거두고
// 게시를 멈춘다 — 결과는 모름이라 die(공개 쪽 ref는 그대로, 다음 실행에서 다시)
func transfer(repo, dir, tree string, shas []string) bool {
	pack := exec.Command("git", "-C", repo, "pack-objects", "-q", "--stdout")
	pack.Stdin = strings.NewReader(tree + "\n" + strings.Join(shas, "\n") + "\n")
	pack.Stderr = os.Stderr
	unpack := exec.Command("git", "-c", "user.useConfigOnly=true", "--git-dir="+dir, "unpack-objects", "-q")
	unpack.Stderr = os.Stderr
	proc.Group(pack)
	proc.Group(unpack)
	pipe, err := pack.StdoutPipe()
	if err != nil {
		return false
	}
	unpack.Stdin = pipe
	if pack.Start() != nil {
		return false
	}
	if unpack.Start() != nil {
		pipe.Close()
		proc.StopGroup(pack.Process.Pid, proc.Grace)
		pack.Wait()
		return false
	}
	pipe.Close() // 부모 쪽 끝 — unpack이 먼저 죽으면 pack의 쓰기가 막히지 않고 실패하게
	fin := make(chan bool, 1)
	go func() {
		uerr := unpack.Wait()
		perr := pack.Wait()
		fin <- uerr == nil && perr == nil
	}()
	var limit <-chan time.Time
	d := proc.Limit([]string{"git", "pack-objects"})
	if d > 0 {
		t := time.NewTimer(d)
		defer t.Stop()
		limit = t.C
	}
	select {
	case ok := <-fin:
		if !ok {
			return false
		}
	case <-limit:
		fmt.Fprintf(os.Stderr, "%s: 시간 넘김 git pack-objects | unpack-objects %d\n", me, int(d/time.Second))
		var wg sync.WaitGroup
		for _, c := range []*exec.Cmd{pack, unpack} {
			wg.Add(1)
			go func(pid int) { defer wg.Done(); proc.StopGroup(pid, proc.Grace) }(c.Process.Pid)
		}
		wg.Wait()
		// ponytail: 신호도 안 먹는 커널 대기면 거두지 못한다 — 조금만 더 기다리고 놓아 둔다(proc.Run과 같음)
		select {
		case <-fin:
		case <-time.After(2*proc.Grace + 3*time.Second):
		}
		die("트리 객체를 옮기다 시간 넘김 — 결과 모름, push 안 함 (다음 실행에서 다시)")
	}
	c := exec.Command("git", "-c", "user.useConfigOnly=true", "--git-dir="+dir, "rev-list", "--objects", tree)
	c.Stderr = os.Stderr
	return proc.Run(c) == nil
}
