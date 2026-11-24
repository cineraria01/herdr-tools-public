package main

// 운영 조치 일지(2026-11-24 ops-journal, docs/ops/journal.md): 조치 명령(knob set·unset, prio, edit, rm, retry, merger restart,
// 병합기 스스로 다시 띄우기)이 jot 한 줄로 internal/journal에 남기고, hc-filljournal이 읽어 보인다. 일지를 못 써도 조치는 그대로다.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"herdr-tools/internal/journal"
)

func init() { register("journal", journalCmd) }

// jot: 조치 한 줄(누가 = 부른 터미널 주소 또는 사람). 같은 대상·같은 조치가 최근에 되풀이됐으면 표준 오류에 한 줄(막지 않는다)
func jot(action, target, why, result, undo, state string) {
	jotBy("", action, target, why, result, undo, state)
}

// jotBy: 누가를 정해(바퀴가 스스로 한 조치는 「hc-fill 스스로」)
func jotBy(by, action, target, why, result, undo, state string) {
	journal.Record(os.Stderr, journal.Entry{By: by, Tool: "hc-fill", Action: action, Target: target, Why: why, Result: result, Undo: undo, State: state})
}

// takeWhy: 인자 어디에 있든 --why <글>을 떼어 낸다(값이 없으면 잘못 부름). 손잡이 set 말고는 권함(빈 값 허용)
func takeWhy(args []string) ([]string, string) {
	var out []string
	why := ""
	for i := 0; i < len(args); i++ {
		if args[i] != "--why" {
			out = append(out, args[i])
			continue
		}
		if i+1 >= len(args) {
			usage()
		}
		i++
		why = args[i]
	}
	return out, why
}

// parseSince: 「2h」「90m」(지금부터 거꾸로) 또는 시각(RFC3339·「2006-01-02 15:04」)
func parseSince(s string, now time.Time) (time.Time, error) {
	if d, err := time.ParseDuration(s); err == nil && d >= 0 {
		return now.Add(-d), nil
	}
	if t, err := parseUntil(s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("--since 꼴은 기간(2h·90m) 또는 시각(RFC3339·「2006-01-02 15:04」)이다: %q", s)
}

// journal [--json] [--since <기간|시각>] [--by <주소|사람>]
func journalCmd(args []string) {
	js := false
	var f journal.Filter
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--json":
			js = true
		case a == "--by" && i+1 < len(args):
			i++
			f.By = args[i]
		case a == "--since" && i+1 < len(args):
			i++
			t, err := parseSince(args[i], time.Now())
			if err != nil {
				DRC = 2
				die("%v", err)
			}
			f.Since = t
		default:
			usage()
		}
	}
	es, bad, err := journal.Read(f)
	if err != nil {
		die("일지를 읽지 못했다(모름 — 「없음」 아님): %v", err)
	}
	if bad > 0 {
		fmt.Fprintf(os.Stderr, "hc-fill: 알림 — 일지에 읽지 못한 줄 %d개(건너뜀)\n", bad)
	}
	if js {
		if es == nil {
			es = []journal.Entry{}
		}
		b, err := json.MarshalIndent(es, "", "  ")
		if err != nil {
			die("%v", err)
		}
		fmt.Println(string(b))
		return
	}
	if len(es) == 0 {
		say("일지 없음")
		return
	}
	for _, e := range es {
		say("%s  %s  %s %s %s — %s", e.Time.Local().Format("2006-01-02 15:04:05"), e.By, e.Tool, e.Action, e.Target, e.Result)
		var tail []string
		if e.Why != "" {
			tail = append(tail, "왜 "+e.Why)
		}
		tail = append(tail, "되돌리기 "+e.Undo)
		if e.State != "" {
			tail = append(tail, "상태 "+e.State)
		}
		say("    %s", strings.Join(tail, " · "))
	}
}
