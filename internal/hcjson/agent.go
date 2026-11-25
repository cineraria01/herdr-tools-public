package hcjson

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"herdr-tools/internal/events"
	"herdr-tools/internal/tri"
)

// AgentState: 그 터미널에 지금 터미널 입력을 받는 프로그램(에이전트)이 있나 — hc get 한 번(t)으로. 있음,
// 없음(대화형 셸 자신이 전경이고 status 없음 — 붙여 넣은 글을 셸이 명령으로 실행한다), 모름(까닭). 근거는 2026-11-23 실측
// (hc 0.1.21·0.1.22, 견본 cmd/hc-lane/testdata/agent, docs/hc-lane/watch.md 「받는 쪽 에이전트 확인」):
// 에이전트가 돌면 전경 pid가 셸과 다르다(job control). zsh -c 꼴은 자식이 돌아도 전경 pid가 셸과 같지만 그 셸은 터미널을 읽지 않으니 있음.
// 판정이 헷갈리면 모름(다시 봄)이지 없음(끝냄)이 아니다 — 도는 지휘에게 사건이 안 가는 것이 가장 비싼 실수다.
// hc-lane(지휘 확인)과 herdr-hooks(hc send·keys 받는 쪽 확인)가 함께 쓴다
func AgentState(t Terminal) (tri.State, string) {
	var fg struct {
		PID  *int     `json:"pid"`
		Argv []string `json:"argv"`
	}
	var argv []string
	if r, ok := t.Raw["foreground"]; ok && string(r) != "null" {
		if json.Unmarshal(r, &fg) != nil || fg.PID == nil || *fg.PID <= 0 {
			return tri.Unknown, "hc get의 전경 프로세스(foreground pid)를 못 읽음"
		}
		// 전경이 다른 프로그램(에이전트·그 배경 명령·하단 줄 실행기)이면 있음. 인자 없는 중첩 대화형 셸(bash·zsh -i 등)이면 아래 같은
		// 판정. nix-shell의 `bash --rcfile <경로>` 꼴은 스크립트 인자로 보아 있음으로 읽는다(안전한 쪽)
		if *fg.PID != t.PID && (len(fg.Argv) == 0 || !InteractiveShell(fg.Argv)) {
			return tri.Yes, ""
		}
		if len(fg.Argv) == 0 {
			return tri.Unknown, "전경이 맨 위 프로세스인데 hc get에 전경 argv가 없다"
		}
		argv = fg.Argv
	} else {
		r, ok := t.Raw["argv"]
		if !ok {
			// ponytail: 판정 칸(foreground·argv)이 둘 다 없는 응답은 가를 근거가 없어 이 확인을 건너뛴다(옛 동작) — 잰 hc 0.1.21·0.1.22는 둘 다 준다
			return tri.Yes, ""
		}
		// 전경 칸이 없다(옛 hc·막 뜸) — 맨 위 argv가 대화형 셸일 때만 모름, 셸이 아닌 프로그램이면 그 프로그램이 터미널을 받는다
		if json.Unmarshal(r, &argv) != nil || len(argv) == 0 {
			return tri.Unknown, "hc get의 argv를 못 읽음"
		}
		if InteractiveShell(argv) {
			return tri.Unknown, "맨 위가 대화형 셸인데 hc get에 전경 프로세스가 없다"
		}
		return tri.Yes, ""
	}
	if !InteractiveShell(argv) {
		return tri.Yes, ""
	}
	if _, ok := t.Raw["status"]; !ok {
		return tri.Unknown, "전경이 대화형 셸인데 hc get에 status 칸이 없다"
	}
	if t.Status != "" {
		return tri.Unknown, "전경이 대화형 셸인데 status가 " + t.Status
	}
	return tri.No, "에이전트가 끝나 셸만 남음(전경 " + events.Flat(strings.Join(argv, " ")) + ")"
}

// InteractiveShell: 터미널을 읽는 셸인가 — 셸 이름(로그인 셸의 앞 「-」 무시)이고 -c·스크립트 인자가 없다
func InteractiveShell(argv []string) bool {
	switch filepath.Base(strings.TrimPrefix(argv[0], "-")) {
	case "sh", "bash", "zsh", "fish", "dash", "ksh", "mksh", "tcsh", "csh":
	default:
		return false
	}
	for _, a := range argv[1:] {
		if !strings.HasPrefix(a, "-") || !strings.HasPrefix(a, "--") && strings.ContainsRune(a, 'c') {
			return false
		}
	}
	return true
}
