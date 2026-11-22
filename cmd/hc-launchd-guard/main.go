// hc-launchd-guard: launchd 작업을 Herdr 코어 문맥으로 옮기는 얇은 지킴이(2026-11-21 go-launchd-tcc, Go 판 2026-11-22 go-shell-remove)
//
//	hc-launchd-guard [--log <파일>] <이름> -- <도구> [인자…]
//
// 이 기계에 <이름> 터미널이 살아 있으면 아무것도 안 하고 0. 없으면
// hc run --bg --no-parent --outlive-parent --rm --name <이름> --cwd $HOME -- /usr/bin/env -u HC_TERMINAL_ID -u HC_SCOPE -u HC_TERMINAL_SOCKET <도구 절대 경로> [인자…]
// 로 띄우고 0. 살아 있는지 모르면(hc ls 실패·시간 넘김·출력 관을 붙든 자손, 목록 아님, 같은 이름 이 기계 항목의 주소·상태를 모름)
// 띄우지 않고 75 — launchd가 다시 부른다. hc run이 실패했는데 터미널도 없으면 75(까닭 한 줄). 그 자리에서 도구를 돌리는 길은 없다.
// hc ls는 출력 수집까지 HC_LAUNCHD_GUARD_LS_SECS(20)초, hc run은 HC_LAUNCHD_GUARD_RUN_SECS(60)초 상한 — 넘으면 그 무리째 거둔다
// (멈춘 hc에 지킴이가 안 끝나면 launchd가 다시 부르지 않는다). --log: 이 지킴이의 줄과 도구 출력을 그 파일 끝에 잇는다.
// <도구>가 python·python3·python3.*이고 다음 인자가 파이썬 파일이 아니면(첫 줄 #!…python 아님) 해석기를 빼고 그 파일을 직접 돌린다 —
// 이미 깔린 옛 plist가 `python3 ~/.local/bin/hc-sweep`으로 넘겨도 그 자리의 Go 바이너리가 돈다.
//
// 왜: launchd가 띄운 새 Go 바이너리는 ~/Documents 첫 접근에서 macOS 개인 정보 보호 창에 사람 응답까지 잠든다(빌드마다 서명이 바뀌어
// 판마다 다시 묻는다). Herdr 터미널에서는 창이 안 뜬다. 지킴이 자신은 cwd를 $HOME으로 옮기고 hc 클라이언트만 부르며 ~/Documents를 열지
// 않으니 launchd에서 Go로 돌아도 창이 안 뜬다(시험 TestTCC가 고정). 터미널 표식을 지우는 것은 도구가 띄우는 hc run이 그 터미널의
// 자식이 되어 같이 끝나지 않게다.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/proc"
	"herdr-tools/internal/version"
)

const me = "hc-launchd-guard"

// exitUnknown: 살아 있는지 모름 — launchd가 다시 부른다(EX_TEMPFAIL)
const exitUnknown = 75

var nameRe = regexp.MustCompile(`^[a-z0-9_-]+$`)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	proc.Who = me
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println(version.String(me))
		return 0
	}
	log := ""
	if len(args) > 0 && args[0] == "--log" {
		if len(args) < 2 {
			return usage()
		}
		log, args = args[1], args[2:]
	}
	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	if !nameRe.MatchString(name) {
		fmt.Fprintf(os.Stderr, "%s: 이름은 a-z 0-9 - _ (1~32자): %s\n", me, name)
		return 2
	}
	if len(name) > 32 || len(args) < 3 || args[1] != "--" || args[2] == "" {
		return usage()
	}
	tool, err := exec.LookPath(args[2])
	if err == nil {
		tool, err = filepath.Abs(tool)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: 도구를 못 찾았다: %s\n", me, args[2])
		return 127
	}
	rest := args[3:]
	if isPython(filepath.Base(tool)) && len(rest) > 0 && regular(rest[0]) && !pythonFile(rest[0]) {
		if tool, err = filepath.Abs(rest[0]); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", me, err)
			return 2
		}
		rest = rest[1:]
	}
	home := os.Getenv("HOME")
	if home == "" {
		fmt.Fprintf(os.Stderr, "%s: HOME이 없다\n", me)
		return 2
	}
	if log != "" {
		if log, err = filepath.Abs(log); err == nil {
			var f *os.File
			if f, err = os.OpenFile(log, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644); err == nil {
				os.Stdout, os.Stderr = f, f
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: 기록 파일을 못 연다: %v\n", me, err)
			return 1
		}
	}
	// 자식(hc)은 모두 $HOME에서 — launchd가 준 cwd(/)나 부른 곳(~/Documents 아래일 수 있다)을 물려주지 않는다
	if err := os.Chdir(home); err != nil {
		fmt.Fprintf(os.Stderr, "%s: HOME으로 못 옮긴다: %v\n", me, err)
		return 2
	}
	g := guard{name: name, lsMax: secs("HC_LAUNCHD_GUARD_LS_SECS", 20), runMax: secs("HC_LAUNCHD_GUARD_RUN_SECS", 60)}

	switch st, why := g.alive(); st {
	case alive:
		return 0
	case unknown:
		return g.unknown("hc ls를 못 읽음(" + why + ")")
	}
	cmd := append([]string{"/usr/bin/env", "-u", "HC_TERMINAL_ID", "-u", "HC_SCOPE", "-u", "HC_TERMINAL_SOCKET", tool}, rest...)
	if log != "" {
		cmd = append([]string{"/bin/sh", "-c", `exec "$@" >>"$0" 2>&1`, log}, cmd...)
	}
	c := exec.Command("hc", append([]string{"run", "--bg", "--no-parent", "--outlive-parent", "--rm", "--name", name, "--cwd", home, "--"}, cmd...)...)
	rc := proc.WaitMax(c, "", g.runMax)
	if rc == 0 {
		g.say("Herdr 터미널로 띄움")
		return 0
	}
	switch st, why := g.alive(); st {
	case alive:
		return 0 // 이름 겹침: 그사이 다른 지킴이가 띄웠다(또는 상한 넘긴 hc run이 띄웠다)
	case unknown:
		return g.unknown(fmt.Sprintf("hc run 실패(종료 %d) 뒤 hc ls를 못 읽음(%s)", rc, why))
	}
	g.say(fmt.Sprintf("hc run 실패(종료 %d)이고 터미널도 없음 — 띄우지 못해 75, launchd가 다시 부른다", rc))
	return exitUnknown
}

func usage() int {
	fmt.Fprintf(os.Stderr, "쓰는 법: %s [--log <파일>] <이름> -- <도구> [인자…]\n", me)
	return 2
}

func isPython(b string) bool {
	return b == "python" || b == "python3" || strings.HasPrefix(b, "python3.")
}

func regular(p string) bool { st, err := os.Stat(p); return err == nil && st.Mode().IsRegular() }

// pythonFile: 첫 줄이 #!이고 python이 든다(못 읽으면 아님 — 셸 판 read 실패와 같게)
func pythonFile(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	b, _ := bufio.NewReader(io.LimitReader(f, 4096)).ReadBytes('\n')
	return bytes.HasPrefix(b, []byte("#!")) && bytes.Contains(b, []byte("python"))
}

// secs: 상한 초(0 = 상한 없음). 못 읽으면 경고하고 기본값
func secs(k string, def int) time.Duration {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return time.Duration(n) * time.Second
		}
		fmt.Fprintf(os.Stderr, "%s: 경고 — %s=%q 를 읽지 못해 기본 %d초\n", me, k, v, def)
	}
	return time.Duration(def) * time.Second
}

type guard struct {
	name          string
	lsMax, runMax time.Duration
}

type state int

const (
	none state = iota
	alive
	unknown
)

func (g guard) say(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s %s: %s\n", time.Now().Format("2006-01-02 15:04:05"), me, g.name, msg)
}

func (g guard) unknown(why string) int {
	g.say(why + " — 살아 있는지 몰라 띄우지 않음(75, launchd가 다시 부른다)")
	return exitUnknown
}

// alive: 이 기계에 같은 이름 터미널이 살아 있는가. 출력 수집까지 lsMax 안 — hc가 끝났어도 자손이 출력 관을 쥐면 그 무리를 거두고 모름
func (g guard) alive() (state, string) {
	var out bytes.Buffer
	c := exec.Command("hc", "ls", "--json")
	c.Stdout = &out
	// 타이머(lsMax, 시작부터)가 먼저 무리째 거두게 — 무리를 벗어난 자손이 관을 쥐면 그때 Go가 관을 닫는다
	c.WaitDelay = g.lsMax + proc.Grace
	if g.lsMax == 0 {
		c.WaitDelay = 0
	}
	if rc := proc.WaitMax(c, "", g.lsMax); rc != 0 {
		return unknown, fmt.Sprintf("종료 %d", rc)
	}
	return judge(out.Bytes(), g.name)
}

// judge: hc ls --json 목록에서 판정. 같은 이름 항목은 주소가 비지 않은 문자열이고 이 기계(주소에 / 없음)·상태가 알려진 값
// (live·finished — hc의 터미널 상태)일 때만 센다. 같은 이름인데 주소·상태를 모르거나, 목록 꼴이 아니면 모름
func judge(b []byte, name string) (state, string) {
	var v struct {
		Terminals *[]json.RawMessage `json:"terminals"`
	}
	if err := json.Unmarshal(b, &v); err != nil || v.Terminals == nil {
		return unknown, "목록 아님"
	}
	why := ""
	for _, raw := range *v.Terminals {
		var e map[string]any
		if json.Unmarshal(raw, &e) != nil || e == nil {
			why = "항목이 객체가 아님"
			continue
		}
		if n, _ := e["name"].(string); n != name {
			continue
		}
		addr, _ := e["address"].(string)
		if addr == "" {
			why = "같은 이름 항목에 주소 없음"
			continue
		}
		if strings.Contains(addr, "/") {
			continue // 다른 기계
		}
		switch s, _ := e["state"].(string); s {
		case "live":
			return alive, ""
		case "finished":
		default:
			why = fmt.Sprintf("같은 이름 항목의 상태를 모름(%q)", s)
		}
	}
	if why != "" {
		return unknown, why
	}
	return none, ""
}
