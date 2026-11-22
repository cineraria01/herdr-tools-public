// herdr-hooks: Claude Code PreToolUse 훅 (2026-11-21 hooks-go — 사용자 「모두 고로 옮기고 싶어」, 셸 판은 2026-11-22 걷음).
//
// settings.json 훅 항목이 ~/.local/bin/herdr-hooks <훅>을 바로 부른다(표준 입력 = 훅 JSON) — 명령 줄은 claude/hooks/tests의 hookCmds.
// 판정·문구·종료 코드·임대 파일 꼴은 옛 셸 판(claude/hooks/*.sh)과 같고, 그 셸 판이 부르던 jq·perl·awk·grep·plutil의 의미
// (bash 3.2·BWK awk·perl 바이트 의미·macOS grep 문자 부류)를 그대로 흉내 낸다(골든 testdata/shell-golden.jsonl).
// 실행 파일·판을 환경 변수로 바꾸지 않는다. 시험 손잡이(HERDR_HOOKS_TEST_*)는 -tags hookstest 빌드에만 있다(knobs*.go).
//
//	herdr-hooks cotenant-guard | kill-by-lookup-guard | hc-lane-wait-guard | lane-no-shell-edit | loop-no-question
//	herdr-hooks --version
package main

import (
	"fmt"
	"io"
	"os"
)

const version = "herdr-hooks 1"

// hook: 표준 입력 전체 → 종료 코드. 표준 출력·오류는 그 훅의 셸 판과 같은 꼴로 쓴다.
type hook func(in []byte, stdout, stderr io.Writer) int

var hooks = map[string]hook{
	"cotenant-guard":       cotenantGuard,
	"kill-by-lookup-guard": killByLookupGuard,
	"hc-lane-wait-guard":   hcLaneWaitGuard,
	"lane-no-shell-edit":   laneNoShellEdit,
	"loop-no-question":     loopNoQuestion,
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(version)
		return
	}
	if len(os.Args) != 2 || hooks[os.Args[1]] == nil {
		fmt.Fprintln(os.Stderr, "쓰임: herdr-hooks <cotenant-guard|kill-by-lookup-guard|hc-lane-wait-guard|lane-no-shell-edit|loop-no-question> (표준 입력 = 훅 JSON) | --version")
		os.Exit(1)
	}
	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		in = nil // 셸 판도 못 읽은 입력은 빈 입력과 같게 다룬다
	}
	os.Exit(call(os.Args[1], in, os.Stdout, os.Stderr))
}

// call: 훅 하나 — 뜻밖의 패닉은 Go 기본 종료 2(= Claude Code의 「막음」)가 아니라, 편의 막 넷은 0(fail-open, 셸 판 헤더와 같게)·
// cotenant-guard는 1(설정 onFailure로 막힘). 스택 대신 한 줄(Fable L2)
func call(name string, in []byte, stdout, stderr io.Writer) (code int) {
	defer func() {
		if r := recover(); r != nil {
			code = 0
			if name == "cotenant-guard" {
				code = 1
			}
			fmt.Fprintf(stderr, "%s: 내부 오류(%v) — 종료 %d\n", name, r, code)
		}
	}()
	return hooks[name](in, stdout, stderr)
}
