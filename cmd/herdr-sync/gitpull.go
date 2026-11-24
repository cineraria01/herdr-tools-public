package main

// 첫 단계 git pull --ff-only(ready)가 같은 체크아웃에서 도는 다른 프로세스의 fetch(대기열 바퀴 등)와 겹치면 잠금(.lock·cannot lock ref)으로
// 실패한다(2026-11-24 지휘 알림 — 배포 한 번·--check 한 번, 다시 돌리면 됐다). 잠금 경합으로 보이는 실패만 짧게 몇 번 다시 하고, 그래도
// 안 되거나 진짜 갈라짐(fast-forward 아님 등)이면 git이 낸 까닭 한 줄을 붙여 끝낸다.

import (
	"bytes"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/proc"
)

var lockRe = regexp.MustCompile(`(?i)\.lock\b|cannot lock ref|unable to update local ref|another git process`)

// pullWait: 잠금 경합 뒤 다시 하기 전 쉼(번마다 늘린다 — 시험이 줄인다)
var pullWait = time.Second

// ponytail: 다시 하기 5번(쉼 1·2·3·4초) 고정 — 대기열 fetch 한 번은 몇 초 안에 끝난다. 길어지면 손잡이로
const pullTries = 5

// pullFF: git -C r pull -q --ff-only. 되면 "", 안 되면 git이 낸 까닭 한 줄(표준 오류 마지막 줄, 없으면 종료 코드)
func pullFF(r string) string {
	why := ""
	for i := 1; i <= pullTries; i++ {
		var eb bytes.Buffer
		c := exec.Command("git", "-C", r, "pull", "-q", "--ff-only")
		c.Stderr = &eb
		rc := proc.Code(proc.Run(c))
		if rc == 0 {
			return ""
		}
		why = lastLine(eb.String())
		if why == "" {
			why = "종료 " + strconv.Itoa(rc)
		}
		if rc == proc.TimeoutCode || !lockRe.MatchString(eb.String()) || i == pullTries {
			break
		}
		os.Stderr.WriteString("herdr-sync: git pull 잠금 경합(" + why + ") — 다시 한다\n")
		time.Sleep(time.Duration(i) * pullWait)
	}
	return why
}

func lastLine(s string) string {
	ls := splitLines(strings.TrimSpace(s))
	if len(ls) == 0 {
		return ""
	}
	return strings.TrimSpace(ls[len(ls)-1])
}
