// hc-core-relaunch <기계>... — 원격 맥(워커·서버)의 herdr-core를 launchd(gui) 문맥으로 다시 띄운다. 도는 프로그램은 살린다.
// 2026-11-16 사용자 요청으로 만듦(셸), 2026-11-22 Go로(go-shell-remove — 인자·출력·종료 코드는 셸 판 그대로). 실측: 원격 맥 네 대에서 죽은 터미널 0개.
// 왜: SSH(hc 재접속·hc machine add/update)로 뜬 코어는 SSH 문맥이라 hc 터미널에서 키체인이 막힌다(User interaction is not allowed).
// 어떻게: 그 기계 gui 도메인에 한 번짜리 launchd 작업을 올려 로그인 셸에서 `herdr-core server restart --fresh`(프로그램은 hold가 붙듦)를 돌린다
// (원격 조각 remote.sh를 ssh … bash -s로 — 원격에는 이 바이너리가 없어도 된다).
//
//	--fresh라 새 터미널은 그 계정의 로그인 환경(~/.zprofile의 brew 등)을 받는다. 도는 프로그램은 제 환경 그대로다.
//	재시작하는 몇 초 동안 SSH로 붙은 `herdr-core connect`를 SIGSTOP해 둔다 — 안 그러면 다른 기계의 hc 재접속이
//	SSH 문맥 코어를 먼저 띄운다(한 워커 맥에서 세 번 연속 그렇게 졌다). 끝나면 SIGCONT.
//
// 확인은 worker-mac-check <기계>의 「코어 문맥」·「키체인(hc 문맥)」·「brew PATH(hc 문맥)」 줄.
package main

import (
	"bufio"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"

	"herdr-tools/internal/version"
)

//go:embed remote.sh
var remote string

var checkLine = regexp.MustCompile(`코어 문맥|키체인\(hc|brew PATH`)

func main() {
	a := os.Args[1:]
	if len(a) == 1 && a[0] == "--version" {
		fmt.Println(version.String("hc-core-relaunch"))
		return
	}
	const usage = "사용: hc-core-relaunch <기계>..."
	if len(a) == 1 && (a[0] == "--help" || a[0] == "-h") {
		fmt.Println(usage)
		return
	}
	// -로 시작하는 이름은 ssh가 옵션으로 읽는다(-oProxyCommand=… 등) — 하나라도 있으면 아무 기계에도 손대지 않는다
	if len(a) < 1 || slices.ContainsFunc(a, func(m string) bool { return strings.HasPrefix(m, "-") }) {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(64)
	}
	rc := 0
	for _, m := range a {
		fmt.Println("== " + m)
		c := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=8", m, "bash -s")
		c.Stdin, c.Stdout, c.Stderr = strings.NewReader(remote), os.Stdout, os.Stderr
		if c.Run() != nil {
			rc = 1
		}
		check(m)
	}
	os.Exit(rc)
}

// check: worker-mac-check <기계> 2>&1 | grep -E "코어 문맥|키체인\(hc|brew PATH" (그 종료 코드는 셸 판처럼 보지 않는다)
func check(m string) {
	c := exec.Command("worker-mac-check", m)
	out, err := c.StdoutPipe()
	if err != nil {
		return
	}
	c.Stderr = c.Stdout
	if c.Start() != nil {
		return
	}
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		if checkLine.MatchString(sc.Text()) {
			fmt.Println(sc.Text())
		}
	}
	c.Wait()
}
