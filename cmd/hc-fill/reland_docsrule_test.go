package main

import (
	"os"
	"strings"
	"testing"
)

// 다시 맞춤에 저장소 문서 규칙(docs-rule-guard): 대상 저장소 기본 가지(origin/main)에 .claude/herdr-docs-rule.md가 있으면
// 다시 맞춤 지시문에 「## 이 저장소의 문서 규칙」 절, 원래 레인에 보내는 글에는 한 줄로 덧붙인다. 없으면 지금 그대로(알림 없음),
// 못 읽으면(기본 가지가 없음) 알림 한 줄 뒤 지금 그대로
func TestRelandDocsRule(t *testing.T) {
	const rule = "- README에는 목차만.\n- 새 문서는 목차 이름순 자리에 한 줄.\n"
	for _, c := range []struct {
		name string
		set  func(w string, git func(string, ...string))
		has  bool
		warn bool
	}{
		{"있음", func(w string, git func(string, ...string)) {
			if err := os.MkdirAll(w+"/.claude", 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(w+"/"+relandDocsRuleFile, []byte(rule), 0o644); err != nil {
				t.Fatal(err)
			}
			git(w, "add", "--", relandDocsRuleFile)
			git(w, "commit", "-q", "-m", "rule")
			git(w, "push", "-q", "origin", "HEAD:main")
			git(w, "fetch", "-q", "origin")
		}, true, false},
		{"없음", func(w string, git func(string, ...string)) {
			git(w, "push", "-q", "origin", "HEAD:main")
			git(w, "fetch", "-q", "origin")
		}, false, false},
		{"못 읽음", func(string, func(string, ...string)) {}, false, true}, // origin/main이 없다
	} {
		t.Run(c.name, func(t *testing.T) {
			w, git := rlRepo(t)
			for _, l := range []string{"100-l-live", "200-l-gone"} {
				git(w, "push", "-q", "origin", "HEAD:refs/merge-failed/"+l)
			}
			c.set(w, git)
			if err := os.MkdirAll(w+".wt/l-live", 0o755); err != nil {
				t.Fatal(err)
			}
			OREC, TERMS = nil, []Term{{"here", "here/t1", "claude", "l-live", "idle", "-"}}
			conflict := "착지 실패(종료 3, 충돌 README.md)"
			EV = []string{rlEv("2026-11-22T00:01:00Z", "l-live", "100-l-live", conflict, "t9"), rlEv("2026-11-22T00:02:00Z", "l-gone", "200-l-gone", conflict, "t9")}
			out := dupCapture(t, relandTick)

			r := qrow("l-gone-reland")
			if r == nil {
				t.Fatalf("다시 맞춤 줄 없음: %v\n%s", qRows(), out)
			}
			b, _ := readFile(r.F(3))
			sends, _ := readFile(DATA + "/sends")
			if !strings.Contains(sends, "send|here/t1|병합기 반송(rebase 충돌 README.md, refs/merge-failed/100-l-live)") {
				t.Fatalf("원래 레인에 보냄: %q", sends)
			}
			if c.has {
				if !strings.Contains(b, "\n## 이 저장소의 문서 규칙\n"+strings.TrimSpace(rule)+"\n") {
					t.Errorf("지시문에 문서 규칙 절 없음:\n%s", b)
				}
				if !strings.Contains(sends, "hc-lane land main 다시(전체 검증은 병합기가 돈다) — 이 저장소의 문서 규칙: README에는 목차만. / 새 문서는 목차 이름순 자리에 한 줄.|") {
					t.Errorf("보낸 글에 문서 규칙 없음: %q", sends)
				}
			} else if strings.Contains(b, "문서 규칙") || strings.Contains(sends, "문서 규칙") {
				t.Errorf("규칙이 없는데 붙음:\n%s\n%q", b, sends)
			}
			if got := strings.Contains(out, "알림: 다시 맞춤 지시문에 문서 규칙을 못 붙임 — 프로젝트 app 기본 가지의 .claude/herdr-docs-rule.md 를 못 읽음"); got != c.warn {
				t.Errorf("알림 %v, 바람 %v:\n%s", got, c.warn, out)
			}
		})
	}
}
