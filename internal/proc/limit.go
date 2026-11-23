package proc

// 바깥 명령 시간 상한(2026-11-21 go-followups-2): 맥 hc-fill 바퀴 하나가 자식 `git remote get-url origin`이 커널 안에서 잠들어
// 37분 멈췄다 — 상한 없이 기다려 그동안 대기열이 하나도 뜨지 않았다. git·hc·hc-lane·ssh·gh·curl은 여기 Run·Output을 거쳐
// 상한 안에서만 기다린다. 넘으면 그 무리를 거두고 「시간 넘김 <명령·하위 명령> <초>」 한 줄 — 결과는 모름(ErrTimeout, TimeoutCode)이다.
// 실행 자체는 run.go, 출력 관은 output.go.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/oplog"
)

// TimeoutCode: 상한에 걸린 명령의 종료 코드(coreutils timeout과 같은 124) — 부른 쪽 셋 결과에서는 모름이다
const TimeoutCode = 124

// ErrTimeout: 상한에 걸렸거나(자식이 끝나지 않음) 자식이 끝났는데 출력 관이 마감 안에 닫히지 않았다 — 결과는 모름.
// errors.Is(err, proc.ErrTimeout)로 가린다. 「실패」·「없음」·「성공」으로 읽지 않는다(상태를 바꾸지 않고 다음 바퀴에)
var ErrTimeout = errors.New("시간 넘김")

// Grace: 상한에 걸린 무리를 TERM한 뒤 KILL까지. 마감 뒤 관·무리를 거두는 데만 쓴다(그 사이 온 출력·종료 코드는 채택하지 않는다)
var Grace = 2 * time.Second

// TimeoutError: 상한에 걸림(Pipe면 자식은 끝났는데 출력 관을 쥔 프로세스가 남음). Why가 있으면 띄우기 전에 멈춤 — 고아 기록·시작 시각을
// 확정하지 못해 명령을 시작시키지 않았다(재감사 R-H05, 결과는 같은 모름)
type TimeoutError struct {
	Head string
	D    time.Duration
	Pipe bool
	Why  string
}

func (e *TimeoutError) Error() string {
	if e.Why != "" {
		return fmt.Sprintf("모름 %s 띄우지 않음: %s", e.Head, e.Why)
	}
	if e.Pipe {
		return fmt.Sprintf("시간 넘김 %s 출력 관이 닫히지 않음", e.Head)
	}
	return fmt.Sprintf("시간 넘김 %s %d", e.Head, int(e.D/time.Second))
}

// Is: errors.Is(err, ErrTimeout)
func (e *TimeoutError) Is(t error) bool { return t == ErrTimeout }

// 기본 상한(초) — 환경 변수로 바꾼다(0이면 상한 없음)
func envSec(name string, def int) time.Duration {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return time.Duration(n) * time.Second
		}
		note(oplog.Warn, "경고 — %s=%q 를 읽지 못해 기본 %d초", name, v, def)
	}
	return time.Duration(def) * time.Second
}

// Limit: 명령(이름·인자)의 상한. 0 = 상한 없음(정해 둔 명령이 아니거나, 기다리는 것이 일인 명령 — hc wait·hc-lane land 등).
// 로컬 git HERDR_TIMEOUT_GIT(60), 원격 git·hc·ssh·gh·curl HERDR_TIMEOUT_REMOTE(120), hc-lane HERDR_TIMEOUT_LANE(600).
// 하위 명령은 맨 앞 낱말만, 옵션은 하위 명령의 옵션 자리(`--`·실행할 명령 앞)만 본다 — 옵션 값·넘기는 명령의 인자는 판정에 쓰지 않는다
func Limit(args []string) time.Duration {
	if len(args) == 0 {
		return 0
	}
	rest := args[1:]
	switch filepath.Base(args[0]) {
	case "git":
		switch s, next := gitSub(rest); s {
		case "fetch", "push", "pull", "ls-remote", "clone", "submodule", "worktree": // worktree: 큰 저장소 checkout
			return envSec("HERDR_TIMEOUT_REMOTE", 120)
		case "remote":
			if next == "update" || next == "prune" || next == "show" {
				return envSec("HERDR_TIMEOUT_REMOTE", 120)
			}
		}
		return envSec("HERDR_TIMEOUT_GIT", 60)
	case "hc":
		s := sub(rest)
		opts := hcOpts(s, rest)
		if _, bg := opts["--bg"]; s == "wait" || (s == "run" && !bg) {
			return 0 // 기다림이 일
		}
		r := envSec("HERDR_TIMEOUT_REMOTE", 120)
		if v, ok := opts["--timeout"]; ok && r > 0 {
			// 원격 작업이 받은 상한과 로컬 hc 프로세스의 마감은 따로 — 원격 상한 + 전송 여유(재감사 R-19: 원격 상한이 있다고 로컬 hc를
			// 상한 없이 기다렸다). 0·못 읽는 값은 원격도 상한 없음이라 로컬도 없음
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				return time.Duration(n)*time.Second + r
			}
			return 0
		}
		return r
	case "ssh", "gh", "curl":
		return envSec("HERDR_TIMEOUT_REMOTE", 120)
	case "hc-lane":
		switch sub(rest) {
		case "land", "wait", "watch":
			return 0 // 검증·착지·기다림이 일이다
		}
		return envSec("HERDR_TIMEOUT_LANE", 600) // 레인 띄우기(옵션으로 시작하는 꼴)는 그 저장소 herdr-setup.sh(의존성 설치·빌드)를 기다린다
	}
	return 0
}

// sub: 하위 명령 — 맨 앞 낱말이 옵션이 아닐 때만(`hc-lane --name x …`처럼 옵션으로 시작하면 하위 명령 없음)
func sub(a []string) string {
	if len(a) > 0 && !strings.HasPrefix(a[0], "-") {
		return a[0]
	}
	return ""
}

// hcValue: 값을 받는 hc 옵션(그다음 낱말은 값)
var hcValue = map[string]bool{"--on": true, "--cwd": true, "--env": true, "--timeout": true, "--name": true, "--label": true,
	"--parent": true, "--if": true, "--output": true, "--version": true, "--from": true, "--via": true}

// hcOpts: hc 하위 명령 뒤 옵션 자리의 옵션들과 값(값 없는 옵션은 빈 값) — `--`에서, run·exec는 실행할 명령(첫 위치 인자)에서 멈춘다
func hcOpts(s string, rest []string) map[string]string {
	m := map[string]string{}
	for i := 1; i < len(rest); i++ {
		a := rest[i]
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "-") {
			k, v, hasEq := strings.Cut(a, "=")
			if hcValue[k] && !hasEq && i+1 < len(rest) {
				i++
				v = rest[i]
			}
			m[k] = v
			continue
		}
		if s == "run" || s == "exec" {
			break
		}
	}
	return m
}

// gitSub: git의 하위 명령과 그다음 낱말(-C·-c 등 값 받는 전역 옵션은 건너뜀)
func gitSub(a []string) (string, string) {
	for i := 0; i < len(a); i++ {
		switch s := a[i]; {
		case s == "-C" || s == "-c" || s == "--git-dir" || s == "--work-tree" || s == "--namespace":
			i++
		case strings.HasPrefix(s, "-"):
		default:
			if i+1 < len(a) {
				return s, a[i+1]
			}
			return s, ""
		}
	}
	return "", ""
}

var (
	plainWord = regexp.MustCompile(`^[a-z][a-z-]*$`)
	gitNested = map[string]bool{"remote": true, "worktree": true, "stash": true, "submodule": true, "notes": true, "config": true}
)

// head: 사건·로그 줄에 적는 명령 앞부분 — 이름과 하위 명령만(인자 값·사건 본문·주소·URL은 적지 않는다).
// git은 -C 경로까지(어느 저장소인지), -c 값은 「…」로, 하위 명령 뒤 낱말은 소문자 낱말(remote get-url 등)일 때만
func head(args []string) string {
	if len(args) == 0 {
		return ""
	}
	name := filepath.Base(args[0])
	rest := args[1:]
	switch name {
	case "git":
		out := []string{name}
		for i := 0; i < len(rest); i++ {
			s := rest[i]
			switch {
			case (s == "-C" || s == "--git-dir" || s == "--work-tree") && i+1 < len(rest):
				out = append(out, s, rest[i+1])
				i++
			case (s == "-c" || s == "--namespace") && i+1 < len(rest):
				out = append(out, s, "…")
				i++
			case strings.HasPrefix(s, "-"):
				out = append(out, s)
			default:
				out = append(out, s)
				if i+1 < len(rest) && gitNested[s] && plainWord.MatchString(rest[i+1]) {
					out = append(out, rest[i+1])
				}
				return strings.Join(out, " ")
			}
		}
		return strings.Join(out, " ")
	case "hc", "hc-lane", "gh":
		if s := sub(rest); s != "" && plainWord.MatchString(s) {
			return name + " " + s
		}
	}
	return name
}
