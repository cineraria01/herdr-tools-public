// herdr-hooks: Claude Code PreToolUse 훅의 Go 판 (2026-11-21 hooks-go — 사용자 「모두 고로 옮기고 싶어」).
//
// claude/hooks/<훅>.sh가 lib/go-run.sh로 이 바이너리를 exec한다: herdr-hooks <훅> (표준 입력 = 훅 JSON).
// 판정·문구·종료 코드·임대 파일 꼴은 셸 판과 같다 — 두 판이 같은 임대 폴더를 번갈아 읽고 써도 된다.
// 셸 판이 부르던 jq·perl·awk·grep·plutil의 의미(그 기계의 bash 3.2·BWK awk·perl 바이트 의미·macOS grep 문자 부류)를
// 그대로 흉내 낸다. 다른 점: jq·perl·awk가 없는 기계에서도 판정한다(셸 판은 그때 통과·막음으로 떨어졌다).
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
	os.Exit(hooks[os.Args[1]](in, os.Stdout, os.Stderr))
}
