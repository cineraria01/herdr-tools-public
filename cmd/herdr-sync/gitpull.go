package main

// 첫 단계(ready)의 main 앞당김: git fetch origin main → git merge --ff-only origin/main(hc-lane ffmain과 같은 꼴).
// git pull은 쓰지 않는다 — pull은 제가 받은 것을 같은 체크아웃이 함께 쓰는 FETCH_HEAD에서 다시 읽어, 그 사이 대기열 바퀴·hc-lane의
// 다른 fetch가 FETCH_HEAD를 고쳐 쓰면 「Cannot fast-forward to multiple branches」 등으로 실패했다(2026-11-24 잠금 글자, 2026-11-26
// 이 글자 — 같은 경합). 이름 붙은 가지만 읽으니 그 경합이 없다. fetch의 참조 잠금 경합은 gitx.Fetchr가 다시 하고(1·2·4초),
// merge의 색인 잠금(같은 폴더의 git status 등)은 여기서 몇 번 다시 한다.

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/gitx"
	"herdr-tools/internal/proc"
)

var lockRe = regexp.MustCompile(`(?i)\.lock\b|cannot lock ref|unable to update local ref|another git process`)

// pullWait: merge 잠금 경합 뒤 다시 하기 전 쉼(번마다 늘린다 — 시험이 줄인다)
var pullWait = time.Second

// ponytail: 다시 하기 5번(쉼 1·2·3·4초) 고정 — 다른 git 한 번은 몇 초 안에 끝난다. 길어지면 손잡이로
const pullTries = 5

// ffMain: r의 main을 origin/main으로 앞당긴다. 되면 ("", false), 안 되면 git이 낸 까닭 한 줄과, 진짜 갈라짐(로컬에 origin/main에
// 없는 커밋)인지
func ffMain(r string) (why string, diverged bool) {
	var eb bytes.Buffer
	rc := gitx.Fetchr(&eb, func(e io.Writer) int {
		c := exec.Command("git", "-C", r, "fetch", "-q", "--no-write-fetch-head", "origin", "main")
		c.Stderr = e
		return proc.Code(proc.Run(c))
	})
	if rc != 0 {
		return "git fetch: " + whyOf(eb.String(), rc), false
	}
	for i := 1; i <= pullTries; i++ {
		eb.Reset()
		c := exec.Command("git", "-C", r, "merge", "-q", "--ff-only", "refs/remotes/origin/main")
		c.Stderr = &eb
		rc = proc.Code(proc.Run(c))
		if rc == 0 {
			return "", false
		}
		why = whyOf(eb.String(), rc)
		if rc == proc.TimeoutCode || !lockRe.MatchString(eb.String()) || i == pullTries {
			break
		}
		os.Stderr.WriteString("herdr-sync: git merge 잠금 경합(" + why + ") — 다시 한다\n")
		time.Sleep(time.Duration(i) * pullWait)
	}
	_, a := gitq("-C", r, "merge-base", "--is-ancestor", "HEAD", "refs/remotes/origin/main")
	return why, a == 1
}

// whyOf: git 표준 오류 마지막 줄, 없으면 종료 코드
func whyOf(stderr string, rc int) string {
	if w := lastLine(stderr); w != "" {
		return w
	}
	return "종료 " + strconv.Itoa(rc)
}

func lastLine(s string) string {
	ls := splitLines(strings.TrimSpace(s))
	if len(ls) == 0 {
		return ""
	}
	return strings.TrimSpace(ls[len(ls)-1])
}
