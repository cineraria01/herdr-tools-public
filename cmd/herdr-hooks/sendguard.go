package main

// hc send·keys 받는 쪽 확인(ht-send-shell-guard, 2026-11-25): 지휘가 `hc send t3yx "…"`를 보냈는데 t3yx는 에이전트가 끝나 zsh만 남은
// 터미널이라 글이 셸 프롬프트에 쳐졌다. hc(herdr-core)는 이 저장소 밖이라 hc-lane-wait-guard(Bash 명령마다 도는 편의 막)에 갈래를 둔다 —
// settings.json 훅 항목을 늘리지 않아 herdr-sync가 바이너리만 바꿔 깐다. 명령의 `hc send|keys <대상>`마다 hc get으로 받는 쪽을 보고
// 셸만 남았으면(hcjson.SendTarget 없음 — Go 도구가 직접 부르는 hc send·keys와 같은 판정) 막는다. 판정 못 함(hc 없음·실패·시간 넘김·변수 대상)은 막지 않고
// additionalContext로 경고만 — 지금 동작을 깨지 않는다. 서비스 터미널(hcjson.ServiceTerm — 프록시 서버·병합기 검증·감시)도 원래 보내는 곳이
// 아니라 막는다: 글은 버려지고 ctrl+c는 서버를 끊는다(감사 atp1om3 중간 1). 막 뜬 에이전트(상태 보고 전)는 서비스로 읽지 않는다.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/tri"
)

// ponytail: 명령 자리(줄 머리·; & | ( ) { } ` 뒤·빈칸 뒤)의 hc send|keys를 글자로 찾는다 — 따옴표 안의 「… hc send t1」(echo 인자 등)도
// 걸려 한 번 더 확인할 수 있다(셸만 남은 대상이면 막힘). 셸 문법 전체를 가르지 않는다
var hcSendRe = regexp.MustCompile("(?:^|[\\s;&|(){}`])(?:[^\\s;&|(){}'\"`]*/)?hc[ \\t]+(send|keys)(?:\\s|$)")

// hcSendGuard: 막으면 2(까닭은 stderr), 아니면 0(판정 못 한 대상은 stdout에 additionalContext 경고)
func hcSendGuard(cmd string, stdout, stderr io.Writer) int {
	ms := hcSendRe.FindAllStringSubmatchIndex(cmd, -1)
	if ms == nil {
		return 0
	}
	hookBudget(4 * time.Second) // 훅 제한 5초(settings) 안
	var warns []string
	for _, m := range ms {
		verb := cmd[m[2]:m[3]]
		target, ok := sendTarget(verb, cmd[m[3]:])
		if !ok {
			warns = append(warns, fmt.Sprintf("hc %s 의 대상을 글자로 정하지 못했다(변수·명령 치환·없음)", verb))
			continue
		}
		switch st, why := sendTargetState(target); st {
		case tri.No:
			fmt.Fprintf(stderr, "%s는 %s — 보내지 않는다(hc %s). 붙여 넣은 글·키는 셸이 명령으로 실행하거나 서비스를 끊는다 — "+
				"주소를 다시 보거나(hc ls) 그 자리에 에이전트를 새로 띄워라.\n", target, why, verb)
			return 2
		case tri.Unknown:
			warns = append(warns, fmt.Sprintf("hc %s %s: %s", verb, target, why))
		case tri.Yes:
		}
	}
	if len(warns) > 0 {
		msg := "hc send·keys 받는 쪽에 에이전트가 있는지 판정하지 못해 막지 않았다 — " + strings.Join(warns, " · ") +
			". 셸만 남은 터미널이면 글이 셸 명령으로 실행되니 hc get <대상>으로 확인하라."
		b, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": "PreToolUse", "additionalContext": msg}})
		fmt.Fprintln(stdout, string(b))
	}
	return 0
}

// sendTarget: hc send|keys 뒤 글 s에서 대상(첫 피연산자). 옵션 --if·--timeout(send)은 값을 하나 먹는다.
// 대상에 따옴표 밖·큰따옴표 안의 $·`가 있거나 대상이 없으면 ok=false
func sendTarget(verb, s string) (string, bool) {
	words, dyn := shellWords(s)
	for i := 0; i < len(words); i++ {
		w := words[i]
		switch {
		case w == "--":
			if i+1 < len(words) && !dyn[i+1] && words[i+1] != "" {
				return words[i+1], true
			}
			return "", false
		case verb == "send" && (w == "--if" || w == "--timeout"):
			i++
		case strings.HasPrefix(w, "-") && !dyn[i]:
		default:
			return w, !dyn[i] && w != ""
		}
	}
	return "", false
}

// shellWords: s 앞에서부터 셸 낱말 — 따옴표·\를 벗기고, 따옴표 밖 ; & | < > ( ) ` 줄바꿈에서 멈춘다.
// dyn[i]: 그 낱말에 작은따옴표 밖의 $·`가 있다(셸이 펼친다 — 글자로는 대상을 모른다)
func shellWords(s string) (words []string, dyn []bool) {
	var cur strings.Builder
	in, d, q := false, false, byte(0)
	flush := func() {
		if in {
			words, dyn = append(words, cur.String()), append(dyn, d)
		}
		cur.Reset()
		in, d = false, false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case q == '\'':
			if c == '\'' {
				q = 0
			} else {
				cur.WriteByte(c)
			}
		case q == '"':
			switch {
			case c == '"':
				q = 0
			case c == '\\' && i+1 < len(s):
				i++
				cur.WriteByte(s[i])
			default:
				d = d || c == '$' || c == '`'
				cur.WriteByte(c)
			}
		case c == '\'' || c == '"':
			q, in = c, true
		case c == '\\' && i+1 < len(s):
			i++
			cur.WriteByte(s[i])
			in = true
		case c == ' ' || c == '\t':
			flush()
		case strings.IndexByte(";&|<>()\n\r`", c) >= 0:
			flush()
			return words, dyn
		default:
			d = d || c == '$'
			cur.WriteByte(c)
			in = true
		}
	}
	flush()
	return words, dyn
}

// sendTargetState: 받는 쪽에 에이전트가 있나 — hc get 한 번. 없는 터미널(not_found)은 있음으로(hc send가 스스로 실패한다)
func sendTargetState(target string) (tri.State, string) {
	hc := hcBin()
	if hc == "" {
		return tri.Unknown, "hc를 찾지 못함"
	}
	out, errOut, rc := hookRun([]string{"hc", "get"}, 0, nil, "", hc, "get", target, "--json")
	if rc == timeoutRC {
		return tri.Unknown, "hc get이 시간 안에 답하지 않음"
	}
	t, found, err := hcjson.ParseGet([]byte(hcjson.GetOut(out, errOut, rc)), rc, "")
	if err != nil {
		return tri.Unknown, err.Error()
	}
	if !found {
		return tri.Yes, ""
	}
	return hcjson.SendTarget(t)
}

// hcBin: 실행할 수 있는 hc — PATH, 없으면 ~/.local/bin/hc. 둘 다 아니면 ""
func hcBin() string {
	hc, err := exec.LookPath("hc")
	if err != nil {
		hc = os.Getenv("HOME") + "/.local/bin/hc"
	}
	if fi, err := os.Stat(hc); err != nil || fi.IsDir() || fi.Mode()&0o111 == 0 {
		return ""
	}
	return hc
}
