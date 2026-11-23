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
// 확인은 worker-mac-check <기계>의 「코어 문맥」·「키체인(hc 문맥)」·「brew PATH(hc 문맥)」 줄. 2026-11-24 core-update-cmd부터 그 가운데
// 코어 문맥·키체인이 「통과」가 아니면(실패·모름·줄 없음) 그 기계는 실패(종료 1) — SSH 문맥 코어를 성공으로 넘기면 레인이 키체인에서 멈춘다.
// 전체 상한 --timeout(기본 1800초). 판 맞추기(--update·--all)는 update.go.
package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/version"
	wm "herdr-tools/internal/workermac"
)

//go:embed remote.sh
var remote string

const usage = `사용: hc-core-relaunch [--timeout <초>] <기계>...
      hc-core-relaunch --update [--dry-run] [--busy skip|wait|go] [--timeout <초>] (<기계>... | --all)`

type opts struct {
	update, all, dry bool
	busy             string // skip(기본)·wait·go — 병합기가 검증 중인 기계
	timeout          int    // 전체 상한 초
	ms               []string
}

// parse: 인자. 모르는 옵션·-로 시작하는 기계 이름(ssh가 옵션으로 읽는다)·빈 기계 목록은 ok 거짓(64, 아무 기계에도 손대지 않음)
func parse(a []string) (opts, bool) {
	o := opts{busy: "skip", timeout: 1800}
	for i := 0; i < len(a); i++ {
		switch x := a[i]; {
		case x == "--update":
			o.update = true
		case x == "--all":
			o.all, o.update = true, true
		case x == "--dry-run":
			o.dry = true
		case x == "--busy" || x == "--timeout":
			if i+1 >= len(a) {
				return o, false
			}
			i++
			if x == "--busy" {
				o.busy = a[i]
			} else if n, err := strconv.Atoi(a[i]); err == nil && n > 0 {
				o.timeout = n
			} else {
				return o, false
			}
		case strings.HasPrefix(x, "-"):
			return o, false
		default:
			o.ms = append(o.ms, x)
		}
	}
	if o.busy != "skip" && o.busy != "wait" && o.busy != "go" {
		return o, false
	}
	if (o.dry || o.busy != "skip") && !o.update { // 판 맞추기에만 있는 손잡이
		return o, false
	}
	return o, o.all != (len(o.ms) > 0)
}

func main() {
	wm.Who = "hc-core-relaunch"
	a := os.Args[1:]
	if len(a) == 1 && a[0] == "--version" {
		fmt.Println(version.String("hc-core-relaunch"))
		return
	}
	if len(a) == 1 && (a[0] == "--help" || a[0] == "-h") {
		fmt.Println(usage)
		return
	}
	os.Exit(run(wm.Signals(), a, os.Stdout))
}

// run: 0 모두 성공(또는 할 것 없음) · 1 실패 있음 · 3 실패는 없고 건너뛴 기계 있음(--update) · 64 사용법
func run(ctx context.Context, a []string, w io.Writer) int {
	o, ok := parse(a)
	if !ok {
		fmt.Fprintln(os.Stderr, usage)
		return 64
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(o.timeout)*time.Second)
	defer cancel()
	if o.update {
		return update(ctx, o, w)
	}
	rc := 0
	for _, m := range o.ms {
		fmt.Fprintln(w, "== "+m)
		if late(ctx) {
			fmt.Fprintf(w, "✗ %s: 전체 상한(%d초) 넘김 — 손대지 않음\n", m, o.timeout)
			rc = 1
			continue
		}
		if r := relaunch(ctx, w, m); r != 0 {
			fmt.Fprintf(w, "✗ %s: 다시 띄우기 %s\n", m, failWhy(ctx, r, o.timeout))
			rc = 1
		}
		if ok, why := check(ctx, w, m); !ok {
			fmt.Fprintf(w, "✗ %s: %s\n", m, why)
			rc = 1
		}
	}
	return rc
}

func late(ctx context.Context) bool { return ctx.Err() != nil }

// failWhy: 실행 결과 r을 사람 말로 — 상한에 걸렸으면 그렇게(성공으로 치지 않는다)
func failWhy(ctx context.Context, r, sec int) string {
	if r == wm.TimeoutCode && late(ctx) {
		return fmt.Sprintf("전체 상한(%d초) 넘김", sec)
	}
	return fmt.Sprintf("실패(종료 %d)", r)
}

// relaunch: remote.sh를 ssh로 보내 그 기계 코어를 gui 문맥으로 다시 띄운다. 출력은 w로 흘린다
func relaunch(ctx context.Context, w io.Writer, m string) int {
	c := wm.Command(ctx, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=8", m, "bash -s")
	c.Stdin, c.Stdout, c.Stderr = strings.NewReader(remote), w, os.Stderr
	return wm.Run(ctx, c)
}

var checkLine = regexp.MustCompile(`코어 문맥|키체인\(hc|brew PATH`)

// mustPass: 이 항목(앞부분)이 모두 「통과」여야 그 기계 성공
var mustPass = []string{"코어 문맥", "키체인(hc 문맥)"}

// check: worker-mac-check <기계>의 점검 줄을 찍고 판독한다. 줄 꼴은 「상태\t항목\t세부」
func check(ctx context.Context, w io.Writer, m string) (bool, string) {
	if late(ctx) {
		return false, "점검 못 함 — 전체 상한 넘김"
	}
	var b bytes.Buffer
	c := wm.Command(ctx, "worker-mac-check", m)
	c.Stdout, c.Stderr = &b, &b
	rc := wm.Run(ctx, c) // 종료 코드는 다른 항목(gh·프록시 등)도 담으니 판정은 아래 두 줄로만
	got := map[string]string{}
	for _, l := range strings.Split(b.String(), "\n") {
		if !checkLine.MatchString(l) {
			continue
		}
		fmt.Fprintln(w, l)
		if f := strings.SplitN(l, "\t", 3); len(f) >= 2 {
			for _, k := range mustPass {
				if strings.HasPrefix(f[1], k) {
					got[k] = f[0]
				}
			}
		}
	}
	var bad []string
	for _, k := range mustPass {
		switch st := got[k]; st {
		case "통과":
		case "":
			bad = append(bad, k+" 줄 없음")
		default:
			bad = append(bad, k+" "+st)
		}
	}
	if len(bad) == 0 {
		return true, ""
	}
	why := "점검 " + strings.Join(bad, " · ")
	if rc == wm.TimeoutCode && late(ctx) {
		why += " (점검 중 전체 상한 넘김)"
	} else if len(got) == 0 {
		why += fmt.Sprintf(" (worker-mac-check 종료 %d)", rc)
	}
	return false, why
}
