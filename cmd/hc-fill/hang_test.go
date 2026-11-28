package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// 멈춘 레인(2026-11-25 ht-lane-hang-detect): 한 레인이 「Ionizing… (20m 39s)」로 도구 호출 없이 20분 넘게 멈췄고, 그 앞에는
// 「1 shell still running」을 기다리는데 검증 프로세스가 없어 1시간 넘게 아무도 몰랐다
func TestHangSpinParse(t *testing.T) {
	for _, c := range []struct {
		scr  string
		mins int
		body string
		ok   bool
	}{
		{"● 검증을 돌린다\n\n✻ Ionizing… (20m 39s · ↓ 1.2k tokens · esc to interrupt)\n  ⎿  Tip: x\n> \n", 20, "● 검증을 돌린다", true},
		{"a\n✢ Thinking… (1h 2m 3s)\n", 62, "a", true},
		{"a\n· Pondering… (39s · esc to interrupt)\n", 0, "a", true},
		{"a\n✻ Brewed for 3m 2s\n", 0, "", false}, // 턴 끝 줄은 진행 표시가 아니다
		{"a\n✻ Ionizing… (esc to interrupt)\n", 0, "", false},
	} {
		m, b, ok := hangSpin(c.scr)
		if ok != c.ok || (ok && (m != c.mins || b != c.body)) {
			t.Errorf("%q: %d %q %v — 바람 %d %q %v", c.scr, m, b, ok, c.mins, c.body, c.ok)
		}
	}
	// 감사 atp1rzf 중간: 마지막 턴 끝 줄만 본다 — 옛 턴 줄·본문 인용은 기다림이 아니다(실제 화면 꼴)
	for _, c := range []struct {
		scr  string
		want bool
	}{
		{"● 배경으로\n✻ Crunched for 21m 24s · done 오후 4:35 · 1 shell still running\n> \n", true},
		{"✻ Brewed for 1m · 2 shells still running\n", true},
		{"✻ Crunched for 21m 24s · 1 shell still running\n● 완료 보고\n✻ Baked for 2m 3s · done 오후 4:40\n> \n", false},
		{"● 「1 shell still running」은 죽은 셸이다\n✻ Baked for 2m 3s\n", false},
		{"  ⎿  1 shell still running\n", false},
		// 감사 atp1sl0 낮음: 사람이 Escape로 끊은 턴은 끝 줄을 안 찍는다 — 옛 턴 줄 아래 Interrupted면 기다림이 아니다
		{"✻ Crunched for 21m 24s · 1 shell still running\n● 배경 셸이 끝나 이어 본다\n  ⎿  Interrupted · What should Claude do instead?\n> \n", false},
	} {
		if got := hangShellWait(c.scr); got != c.want {
			t.Errorf("%q: %v — 바람 %v", c.scr, got, c.want)
		}
	}
}

func TestHangTick(t *testing.T) {
	now := time.Now()
	spin := "● 검증\n✻ Ionizing… (20m 39s · esc to interrupt)\n> \n"
	fp := hangFP("● 검증")
	shellScr := "● 배경으로 돌린다\n✻ Crunched for 21m 24s · done 오후 4:35 · 1 shell still running\n> \n"
	tool := "61775 61774 node agent-wrap\n61776 61775 claude\n61777 61776 codebase-memory-mcp\n61790 61776 /bin/zsh -c source /h/.claude/shell-snapshots/s.sh && eval 'x'\n"
	// MCP 서버는 도구 셸이 아니고, 남의 터미널 아래 도구 셸은 안 센다
	idle := "61775 61774 node agent-wrap\n61776 61775 claude\n61777 61776 codebase-memory-mcp\n500 1 launchd-x\n501 500 /bin/zsh -c source /h/.claude/shell-snapshots/other.sh\n"
	for _, c := range []struct {
		name, status, scr, ps, aud, tname string
		prev                              string // hang.tsv(빈 값 = 없음)
		esc                               string // hang-esc.tsv
		knobs                             string
		want                              string // 알림 글 조각("" = 알리지 않음)
		keys                              int    // hc keys 부른 수
		row                               string // hang.tsv에 남을 종류("" = 줄 없음)
		escHas, escNot                    string // 끝난 뒤 hang-esc.tsv에 있어야·없어야 할 조각
		get                               string // hc get 응답 JSON(빈 값 = 판정 칸 없는 기본)
		up                                string // upstream.tsv(빈 값 = 없음)
	}{
		// ht-upstream-contract ⑨: 그 기계에 확인 안 된 판이 있으면 Escape도 글도 안 보내고 알리기만(보류 아닌 기계 줄은 상관없음)
		{name: "확인 안 된 판 — 끊지 않고 알리기만", status: "working", scr: spin, ps: idle,
			prev: fmt.Sprintf("w\tm/w\ta\t%d\t%s\n", now.Add(-16*time.Minute).Unix(), fp), up: "m\thold\tclaude-2_hc-1\t0\t0\t-\t-\n",
			want: "끊지 않음(이 기계 에이전트 판이 아직 실물 점검으로 확인 안 됨", row: "a"},
		{name: "판 기록 ok — 평소대로 끊음", status: "working", scr: spin, ps: idle,
			prev: fmt.Sprintf("w\tm/w\ta\t%d\t%s\n", now.Add(-16*time.Minute).Unix(), fp), up: "m\tok\t-\t0\t0\t-\t-\nx\thold\tc\t0\t0\t-\t-\n",
			want: "w 멈춘 레인", keys: 1, row: "a"},
		{name: "끊음 기록 정리 — 같은 레인 옛 줄은 버리고 다른 레인 최근 줄은 남김", status: "working", scr: spin, ps: idle,
			prev: fmt.Sprintf("w\tm/w\ta\t%d\t%s\n", now.Add(-16*time.Minute).Unix(), fp),
			esc: fmt.Sprintf("w\tm/w\t%d\nx\tm/x\t%d\ny\tm/y\t%d\n", now.Add(-2*time.Hour).Unix(), now.Add(-10*time.Minute).Unix(),
				now.Add(-2*time.Hour).Unix()),
			want: "w 멈춘 레인", keys: 1, row: "a",
			escHas: fmt.Sprintf("x\tm/x\t%d\nw\tm/w\t%d\n", now.Add(-10*time.Minute).Unix(), now.Unix()), escNot: "y\t"},
		// ht-send-target-live: 에이전트가 끝나 셸만 남았으면 Escape도 글도 보내지 않는다(셸이 글을 명령으로 실행한다)
		{name: "셸만 남음 — 끊지 않고 알리기만", status: "working", scr: spin, ps: idle,
			prev: fmt.Sprintf("w\tm/w\ta\t%d\t%s\n", now.Add(-16*time.Minute).Unix(), fp),
			get:  `{"address":"m/w","state":"live","name":"w","pid":61774,"created_ms":1,"cwd":"/wt","argv":["zsh","-l"],"foreground":{"argv":["zsh","-l"],"pid":61774},"status":null}`,
			want: "끊지 않음(받는 쪽 에이전트가 끝나 셸만 남음", row: "a"},
		{name: "생각 중 멈춤 — 끊고 이어 보냄", status: "working", scr: spin, ps: idle,
			prev: fmt.Sprintf("w\tm/w\ta\t%d\t%s\n", now.Add(-16*time.Minute).Unix(), fp),
			want: "w 멈춘 레인 — 「생각 중」 표시 20분째", keys: 1, row: "a"},
		{name: "한 시간 안에 이미 끊음 — 알리기만", status: "working", scr: spin, ps: idle,
			prev: fmt.Sprintf("w\tm/w\ta\t%d\t%s\n", now.Add(-16*time.Minute).Unix(), fp),
			esc:  fmt.Sprintf("w\tm/w\t%d\n", now.Add(-30*time.Minute).Unix()),
			want: "한 시간 안에 이미 끊음", row: "a"},
		{name: "hang_esc 0 — 알리기만", status: "working", scr: spin, ps: idle, knobs: "hang_esc 0",
			prev: fmt.Sprintf("w\tm/w\ta\t%d\t%s\n", now.Add(-16*time.Minute).Unix(), fp),
			want: "끊지 않음(손잡이 hang_esc 0)", row: "a"},
		{name: "처음 봄 — 적기만", status: "working", scr: spin, ps: idle, row: "a"},
		{name: "화면이 바뀜 — 다시 셈", status: "working", scr: spin, ps: idle,
			prev: fmt.Sprintf("w\tm/w\ta\t%d\tother\n", now.Add(-16*time.Minute).Unix()), row: "a"},
		{name: "도구 셸이 도는 중(긴 검증) — 멈춤 아님", status: "working", scr: spin, ps: tool,
			prev: fmt.Sprintf("w\tm/w\ta\t%d\t%s\n", now.Add(-16*time.Minute).Unix(), fp), row: "a"},
		{name: "배경 셸 기다리는데 프로세스 없음", status: "idle", scr: shellScr, ps: idle,
			prev: fmt.Sprintf("w\tm/w\tb\t%d\t-\n", now.Add(-4*time.Minute).Unix()),
			want: "w 멈춘 레인 — 「1 shell still running」", row: "b"},
		{name: "배경 셸 처음 봄", status: "idle", scr: shellScr, ps: idle, row: "b"},
		{name: "옛 턴 줄에만 still running — 완료 뒤 감사 기다림", status: "done", ps: idle,
			scr:  "✻ Crunched for 21m 24s · 1 shell still running\n● 완료 보고\n✻ Baked for 2m 3s · done 오후 4:40\n> \n",
			prev: fmt.Sprintf("w\tm/w\tb\t%d\t-\n", now.Add(-40*time.Minute).Unix())},
		{name: "입력 기다림(blocked) — 화면을 읽지 않음", status: "blocked", scr: shellScr, ps: idle,
			prev: fmt.Sprintf("w\tm/w\tb\t%d\t-\n", now.Add(-40*time.Minute).Unix())},
		{name: "배경 셸이 살아 있음 — 알리지 않고 b-live로 적음(상태 한 장의 배경 일 기다림)", status: "idle", scr: shellScr, ps: tool,
			prev: fmt.Sprintf("w\tm/w\tb\t%d\t-\n", now.Add(-40*time.Minute).Unix()), row: hangBgLive},
		{name: "감사 터미널은 대상 아님", status: "working", scr: spin, ps: idle, aud: "audit=x",
			prev: fmt.Sprintf("w\tm/w\ta\t%d\t%s\n", now.Add(-16*time.Minute).Unix(), fp)},
		{name: "주소의 이름이 다름(사람·병합기 터미널) — 대상 아님", status: "working", scr: spin, ps: idle, tname: "human",
			prev: fmt.Sprintf("w\tm/w\ta\t%d\t%s\n", now.Add(-16*time.Minute).Unix(), fp)},
	} {
		t.Run(c.name, func(t *testing.T) {
			DATA = t.TempDir()
			Q, LOCAL, dry, CONN = DATA+"/queue.tsv", "m", false, nil
			confLines = strings.Split(strings.TrimSpace("hang_min 15\n"+c.knobs), "\n")
			tn := c.tname
			if tn == "" {
				tn = "w"
			}
			TERMS = []Term{{M: "m", Addr: "m/w", Name: tn, Status: c.status}}
			wr := func(f, s string) {
				if err := os.WriteFile(DATA+"/"+f, []byte(s), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			log := DATA + "/calls.log"
			wr("hc-lane", "#!/bin/sh\nprintf 'hc-lane %s\\n' \"$*\" >>"+log+"\n")
			wr("scr.txt", c.scr)
			wr("ps.txt", c.ps)
			wr("ps", "#!/bin/sh\ncat "+DATA+"/ps.txt\n")
			get := c.get
			if get == "" {
				get = `{"address":"m/w","state":"live","name":"w","pid":61774,"created_ms":1,"cwd":"/wt","status":null}`
			}
			wr("get.json", get+"\n")
			wr("hc", "#!/bin/sh\nprintf 'hc %s\\n' \"$*\" >>"+log+"\ncase $1 in\n"+
				"read) cat "+DATA+"/scr.txt;;\n"+
				"get) cat "+DATA+"/get.json;;\n"+
				"esac\nexit 0\n")
			t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
			aud := c.aud
			if aud == "" {
				aud = "auto"
			}
			wr("queue.tsv", strings.Join([]string{"w", "app", "b", "-", "m", "up", "m", "m/w", "-", "t", "-", "own", "normal", "-", aud, "-", "r1"}, "\t")+"\n")
			if c.prev != "" {
				wr("hang.tsv", c.prev)
			}
			if c.esc != "" {
				wr("hang-esc.tsv", c.esc)
			}
			if c.up != "" {
				wr("upstream.tsv", c.up)
			}
			hangTick(now)
			b, _ := os.ReadFile(log)
			calls := string(b)
			if c.want == "" && strings.Contains(calls, "hc-lane report") {
				t.Fatalf("알리지 않아야: %q", calls)
			}
			if c.want != "" && (!strings.Contains(calls, "hc-lane report 막힘 - ") || !strings.Contains(calls, c.want)) {
				t.Fatalf("알림 %q 없음: %q", c.want, calls)
			}
			if n := strings.Count(calls, "hc keys m/w esc"); n != c.keys {
				t.Fatalf("hc keys esc %d번 — 바람 %d: %q", n, c.keys, calls)
			}
			if c.keys > 0 && !strings.Contains(calls, "hc send m/w 앞 요청이 20분 멈춰 끊었다") {
				t.Fatalf("이어 가라는 글 없음: %q", calls)
			}
			if aud != "auto" || c.tname != "" || c.status == "blocked" {
				if strings.Contains(calls, "hc read") {
					t.Fatalf("대상 아닌 터미널 화면을 읽었다: %q", calls)
				}
			}
			h, _ := os.ReadFile(DATA + "/hang.tsv")
			got := ""
			if r := splitRow(strings.TrimSpace(string(h))); len(h) > 0 {
				got = r.F(hgKind)
			}
			if got != c.row {
				t.Fatalf("hang.tsv 종류 %q — 바람 %q (%q)", got, c.row, h)
			}
			e, _ := os.ReadFile(DATA + "/hang-esc.tsv")
			if c.escHas != "" && (string(e) != c.escHas || strings.Contains(string(e), c.escNot)) {
				t.Fatalf("hang-esc.tsv %q — 바람 %q(없을 것 %q)", e, c.escHas, c.escNot)
			}
		})
	}
}
