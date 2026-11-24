package main

// 공통 준비(환경·설정 읽기)와 명령 나누기. 하위 명령은 그 명령 파일이 init()에서 register로 넣고(도움말 조각은 help/<명령>.txt),
// 이 파일은 등록표를 돌기만 한다 — 새 하위 명령을 더하는 레인은 이 파일을 고치지 않는다(2026-11-22 go-cmd-registry)

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"herdr-tools/internal/proc"
	"herdr-tools/internal/version"
)

var heldLocks []*os.File // 프로세스가 끝날 때까지 쥐는 잠금(GC가 닫지 않게)

// cmds: 하위 명령 이름 → 본문(끝나면 exit(0)). 「_」로 시작하는 것은 hc-fill이 스스로 부르는 속 명령
var cmds = map[string]func(args []string){}

func register(name string, run func([]string)) { cmds[name] = run }

func main() {
	closeInherited()
	proc.Who = "hc-fill"
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(version.String("hc-fill")) // 릴리스 바이너리는 커밋 12자(gofix-fill F19)
		return
	}
	// 문자열 비교는 바이트로(셸 판과 같은 환경을 자식에게도)
	if v, ok := os.LookupEnv("LC_ALL"); ok && v != "" {
		os.Setenv("LC_CTYPE", v)
	}
	os.Unsetenv("LC_ALL")
	os.Setenv("LC_COLLATE", "C")

	HOME = os.Getenv("HOME")
	cdir := os.Getenv("HERDR_TOOLS_CONFIG")
	if cdir == "" {
		cdir = HOME + "/.config/herdr-tools"
	}
	CONF = cdir + "/hc-fill.tsv"
	DATA = os.Getenv("HC_FILL_DATA")
	if DATA == "" {
		DATA = HOME + "/.local/share/hc-fill"
	}
	Q, LED, AT = DATA+"/queue.tsv", DATA+"/inbox.tsv", DATA+"/audit.tsv"
	if v := os.Getenv("HC_FILL_WAIT"); v != "" {
		WAIT, _ = strconv.Atoi(v)
	}
	if v := os.Getenv("HC_FILL_POLL"); v != "" {
		POLL, _ = strconv.Atoi(v)
	}
	SELF = os.Getenv("HC_FILL_SELF")
	if SELF == "" {
		if e, err := os.Executable(); err == nil {
			SELF = e
		} else {
			SELF = os.Args[0]
		}
	}

	text, ok := readFile(CONF)
	if !isFile(CONF) || !ok {
		die("설정 파일이 없다: %s (저장소 config.example/hc-fill.tsv를 복사해 값을 고친다)", CONF)
	}
	confLines = lines(text)
	single := map[string]bool{}
	for _, k := range strings.Fields("local agent coord task launch gc early early_wait idle hold audit audit_max audit_seat_rounds audit_final wake_every ack_max overlap done_stall inbox_audit dup_warn lows lows_hour lows_min") {
		single[k] = true
	}
	cntk := map[string]int{}
	var dup []string
	for _, l := range confLines {
		if f := fields(l); len(f) > 0 && single[f[0]] {
			if cntk[f[0]]++; cntk[f[0]] == 2 {
				dup = append(dup, f[0])
			}
		}
	}
	if len(dup) > 0 {
		die("설정에 %s 줄이 두 번 있다: %s", strings.Join(dup, " "), CONF)
	}
	LOCAL, AGENT = conf("local"), conf("agent")
	if AGENT == "" {
		AGENT = "claude-code"
	}
	if LOCAL == "" {
		die("설정에 local 줄이 없다: %s", CONF)
	}

	cmd, args := "", []string(nil)
	if len(os.Args) > 1 {
		cmd, args = os.Args[1], os.Args[2:]
	}
	run, ok := cmds[cmd]
	if !ok {
		usage()
	}
	run(args)
	exit(0)
}
