package main

// 외부 명령 부르기: git·hc·hc-lane·mv·sleep·ps. bash 판과 같은 명령·같은 인자 꼴로 부른다 — 계약 시험이 PATH의 가짜 git·mv·sleep로
// 끊김을 흉내 내고, 인자 꼴로 어느 호출인지 가린다(tests/hc-lane-merger*.test.sh).
// git 결과·자식 무리·잠금은 공통 꾸러미(internal/gitx·proc·lockx)가 한다.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"herdr-tools/internal/gitx"
	"herdr-tools/internal/oplog"
	"herdr-tools/internal/proc"
)

// run: 출력은 화면 그대로 · runQ: 출력 모두 버림(>/dev/null 2>&1) · runO: 표준 출력만 버림
func run(args ...string) int  { return gitx.Run("", os.Stdout, os.Stderr, args...) }
func runQ(args ...string) int { return gitx.Run("", nil, nil, args...) }
func runO(args ...string) int { return gitx.Run("", nil, os.Stderr, args...) }

// out: $(git …) — 표준 출력(끝 줄바꿈 뺌)과 종료 코드, 오류 출력은 화면 · outQ: 오류 출력도 버림(2>/dev/null).
// 종료 코드가 0이 아니면 출력은 빈 값(gitx.Out — 반쪽 출력을 믿지 않는다)
func out(args ...string) (string, int)  { return gitx.Out("", os.Stderr, args...) }
func outQ(args ...string) (string, int) { return gitx.Out("", nil, args...) }

// inLog: git을 dir에서, 출력은 로그 파일 뒤에(>>"$lgf_" 2>&1)
func inLog(dir, log string, args ...string) int {
	f, err := os.OpenFile(log, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 로그를 열지 못했다: %v\n", err)
		return 1
	}
	defer f.Close()
	return gitx.Run(dir, f, f, args...)
}

// fetchr: git fetch를 gitx.Fetchr로 — 참조 잠금 경합이면 HC_LANE_FETCH_WAITS(기본 1 2 4초) 쉬고 다시, 시간 넘김(모름)이면 그대로 멈춤.
// quiet = 2>/dev/null
func fetchr(quiet bool, args ...string) int {
	var errw io.Writer = os.Stderr
	if quiet {
		errw = nil
	}
	return gitx.Fetchr(errw, func(e io.Writer) int {
		return gitx.Run("", os.Stdout, e, append([]string{"fetch"}, args...)...)
	})
}

// place: tmp를 dst로 바꿔 넣는다 — bash 판처럼 mv -f 명령으로(계약 시험이 PATH의 mv로 이 자리의 끊김을 흉내 낸다.
// 그래서 internal/atomic이 아니라 mv다)
func place(tmp, dst string) error {
	c := exec.Command("mv", "-f", tmp, dst)
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("mv %s: %w", dst, err)
	}
	return nil
}

// writeVia: dst.tmp에 다 쓰고 fsync한 뒤 바꿔 넣고 폴더도 fsync한다(덮어쓰다 끊겨 반쪽 파일이 남지 않게, 원격을 고치기 전에
// 일지가 디스크에 닿게 — E08·G3: 전원이 끊겨도 빈 일지가 남지 않게)
func writeVia(dst, content string) error {
	f, err := os.OpenFile(dst+".tmp", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		return fmt.Errorf("쓰기 %s.tmp: %w", dst, err)
	}
	_, err = f.WriteString(content)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("쓰기 %s.tmp: %w", dst, err)
	}
	if err := place(dst+".tmp", dst); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(dst))
	if err != nil {
		return fmt.Errorf("폴더 열기 %s: %w", filepath.Dir(dst), err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("폴더 fsync %s: %w", filepath.Dir(dst), err)
	}
	return nil
}

// dieGit: 로컬 git이 실패해 멈출 자리 — 시간 넘김(proc.TimeoutCode, 모름)이면 멈추지 않고 이 바퀴만 쉰다(75 — 다음 바퀴가 일지로 다시 본다,
// F6). 그 밖의 실패는 die
func dieGit(rc int, format string, a ...any) {
	if rc == proc.TimeoutCode {
		msg := fmt.Sprintf(format, a...)
		fmt.Fprintf(os.Stderr, "hc-lane merger: %s — git 시간 넘김(모름), 바꾼 것 없이 다음 바퀴에\n", msg)
		oplog.Log("hc-merger", "", oplog.Timeout, 75, msg)
		os.Exit(75)
	}
	die(format, a...)
}

// 자식 프로세스 무리(요구 1: 자기가 띄운 프로세스만 소유)는 internal/proc: 착지·준비 훅·바퀴 일꾼·빈 바퀴 sleep은 새 무리로 띄우고
// (proc.Wait), 멈출 때(TERM·INT·HUP) 그 무리째 TERM → 유예 → KILL(proc.StopCurrent). 착지가 정상으로 끝난 뒤 무리에 남은 배경
// 프로세스(검증이 남긴 고아)는 건드리지 않는다(계약 시험 30a — 그 정리는 저장소 검증 몫).
