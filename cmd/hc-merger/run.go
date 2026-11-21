package main

// 외부 명령 부르기: git·hc·hc-lane·mv·sleep·ps. bash 판과 같은 명령·같은 인자 꼴로 부른다 — 계약 시험이 PATH의 가짜 git·mv·sleep로
// 끊김을 흉내 내고, 인자 꼴로 어느 호출인지 가린다(tests/hc-lane-merger*.test.sh).
// git 결과·자식 무리·잠금은 공통 꾸러미(internal/gitx·proc·lockx)가 한다.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/gitx"
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

var lockContention = regexp.MustCompile(`cannot lock ref|unable to update local ref|is at [0-9a-f]+ but expected|로컬 레퍼런스를 업데이트할 수 없습니다|\.lock'을 만들 수 없습니다`)

// fetchr: git fetch를 돌리고, 같은 클론의 다른 fetch·착지와 참조 잠금이 겹쳐 난 실패면 HC_LANE_FETCH_WAITS(기본 1 2 4초) 쉬고 다시
// (bash fetchr와 같음). quiet = 2>/dev/null
func fetchr(quiet bool, args ...string) int {
	var errw io.Writer = os.Stderr
	if quiet {
		errw = io.Discard
	}
	waits := strings.Fields(os.Getenv("HC_LANE_FETCH_WAITS"))
	if _, set := os.LookupEnv("HC_LANE_FETCH_WAITS"); !set {
		waits = []string{"1", "2", "4"}
	}
	waits = append(waits, "-")
	rc := 0
	for _, w := range waits {
		var e bytes.Buffer
		rc = gitx.Run("", os.Stdout, &e, append([]string{"fetch"}, args...)...)
		errw.Write(e.Bytes())
		if rc == 0 || w == "-" || !lockContention.Match(e.Bytes()) {
			break
		}
		fmt.Fprintf(errw, "hc-lane: git fetch 참조 잠금 경합(같은 클론의 다른 fetch·착지) — %s초 뒤 다시\n", w)
		if s, err := strconv.ParseFloat(w, 64); err == nil {
			time.Sleep(time.Duration(s * float64(time.Second)))
		}
	}
	return rc
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

// writeVia: dst.tmp에 다 쓴 뒤 바꿔 넣는다(덮어쓰다 끊겨 반쪽 파일이 남지 않게)
func writeVia(dst, content string) error {
	if err := os.WriteFile(dst+".tmp", []byte(content), 0o666); err != nil {
		return fmt.Errorf("쓰기 %s.tmp: %w", dst, err)
	}
	return place(dst+".tmp", dst)
}

// 자식 프로세스 무리(요구 1: 자기가 띄운 프로세스만 소유)는 internal/proc: 착지·준비 훅·바퀴 일꾼·빈 바퀴 sleep은 새 무리로 띄우고
// (proc.Wait), 멈출 때(TERM·INT·HUP) 그 무리째 TERM → 유예 → KILL(proc.StopCurrent). 착지가 정상으로 끝난 뒤 무리에 남은 배경
// 프로세스(검증이 남긴 고아)는 건드리지 않는다(계약 시험 30a — 그 정리는 저장소 검증 몫).
