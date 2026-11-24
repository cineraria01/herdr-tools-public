package gitx

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/proc"
)

// LockContention: 같은 클론의 다른 fetch·착지와 참조 잠금이 겹친 git 오류 글
var LockContention = regexp.MustCompile(`cannot lock ref|unable to update local ref|is at [0-9a-f]+ but expected|로컬 레퍼런스를 업데이트할 수 없습니다|\.lock'을 만들 수 없습니다`)

// FetchWaits: ${HC_LANE_FETCH_WAITS:-1 2 4} 낱말들 + 끝 표지 「-」(빈 값도 기본)
func FetchWaits() []string {
	v := os.Getenv("HC_LANE_FETCH_WAITS")
	if v == "" {
		v = "1 2 4"
	}
	return append(strings.Fields(v), "-")
}

// Fetchr: run(git fetch를 그대로 또는 원격으로 부르는 것 — 오류 출력은 받은 writer에)을 돌리고, 참조 잠금 경합 실패면 FetchWaits초 쉬고
// 다시. 오류 출력은 시도마다 모았다가 stderr에(nil이면 버림). 상한에 걸리면(proc.TimeoutCode — 모름) 반쪽 오류 글로 경합을 판정하지
// 않고 그대로 멈춘다(재감사 R-C01 — 「시간 넘김」 줄은 proc이 적었다)
func Fetchr(stderr io.Writer, run func(stderr io.Writer) int) int {
	if stderr == nil {
		stderr = io.Discard
	}
	rc := 0
	for _, w := range FetchWaits() {
		var e bytes.Buffer
		rc = run(&e)
		if rc == proc.TimeoutCode {
			break
		}
		stderr.Write(e.Bytes())
		if rc == 0 || w == "-" || !LockContention.Match(e.Bytes()) {
			break
		}
		fmt.Fprintf(stderr, "hc-lane: git fetch 참조 잠금 경합(같은 클론의 다른 fetch·착지) — %s초 뒤 다시\n", w)
		if s, err := strconv.ParseFloat(w, 64); err == nil && s > 0 {
			time.Sleep(time.Duration(s * float64(time.Second)))
		}
	}
	return rc
}
