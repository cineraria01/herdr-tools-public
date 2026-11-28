package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/opsstate"
)

// ht-smalls-1129: 장부가 다른 줄이 이어 받음·이미 main을 적은 반송 줄은 끝난 줄 — 「반송됨 → <이어 받은 줄>」·「반송됨 — 이미 main」. 장부에 줄이
// 없는 반송(옛 줄·방금 반송돼 처리 전·다시 맞춤 꺼짐)은 글만 「반송됨(이어 받은 줄 기록 없음)」이고 끝난 줄로 세지도 숨기지도 않는다(지휘 결정 —
// 감사 atp82fg 중간 1). 살아서 고치는 줄·다시 제출한 줄·원격을 못 읽음은 지금 글 그대로. 고치기 전은 모두 「감사 통과 — 대기열이 제출함」
func TestStatusBounceEnd(t *testing.T) {
	setData(t)
	HOME, LOCAL = t.TempDir(), "here"
	bin := t.TempDir()
	lsr := filepath.Join(bin, "ls-remote.txt")
	for n, body := range map[string]string{
		"git": "#!/bin/sh\n[ -f '" + bin + "/remote-bad' ] && exit 128\nfor a; do [ \"$a\" = ls-remote ] && exec cat '" + lsr + "'; done\nexit 1\n",
		"hc":  "#!/bin/sh\n[ \"$1\" = ls ] && echo '{\"terminals\":[{\"address\":\"t-alive\",\"name\":\"alive\",\"status\":\"idle\"}]}' && exit 0\nexit 1\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, n), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	confLines = []string{"repo app here " + bin}
	defer func() { confLines = nil }()

	now := time.Now().Truncate(time.Minute)
	at := func(m int) time.Time { return now.Add(time.Duration(-m) * time.Minute) }
	row := func(name string) Row {
		r := make(Row, qNF)
		for i := range r {
			r[i] = "-"
		}
		return r.Set(qName, name).Set(qProj, "app").Set(qMach, "here").Set(qAddr, "t-"+name).Set(qState, "up").Set(qOwner, "t9").
			Set(qAdded, at(300).Format("2006-01-02T15:04")).Set(qRID, fmt.Sprintf("r%d-1-1", at(300).Unix())).Set(qAudit, "auto")
	}
	names := []string{"rl", "im", "old", "alive", "resub", "stale", "fresh"}
	var q, au, last strings.Builder
	for _, n := range names {
		q.WriteString(row(n).Line() + "\n")
		au.WriteString(strings.Join([]string{"a-" + n, n, "1", "x", "s", "here", "t-" + n, "taken", "0", "pass", "0", "0", "-", "-", "0", "asub,asubok", "-"}, "\t") + "\n")
		if n != "alive" {
			last.WriteString(strings.Join([]string{n, "정리됨", "-", "10:00", "-", "-", "fresh", "-"}, "\t") + "\n")
		}
	}
	fs := func(m int, lane string) string { return fmt.Sprintf("%d-%s", at(m).Unix(), lane) }
	utc := func(m int) string { return at(m).UTC().Format("2006-01-02T15:04:05Z") }
	// 장부에 줄이 없는 반송 둘 — old(200분 전, 옛 줄)·fresh(30분 전, 바퀴가 아직 처리 전). 둘 다 끝남이 아니다(지휘 결정 — 감사 atp82fg 중간 1)
	ledger := "#since\t" + utc(10000) + "\n" +
		strings.Join([]string{fs(50, "rl"), "rl", "rl", "넣음", "rl-reland", utc(40), "t9", "자동 다시 맞춤 넣음 rl-reland"}, "\t") + "\n" +
		strings.Join([]string{fs(50, "rl"), "rl", "rl", "알림", "넣음", utc(40)}, "\t") + "\n" +
		strings.Join([]string{fs(45, "im"), "im", "im", "이미 main", "-", utc(42)}, "\t") + "\n" +
		strings.Join([]string{fs(400, "stale"), "stale", "stale", "넣음", "stale-reland", utc(390)}, "\t") + "\n" // 이 줄 접수 전의 옛 반송
	remote := fmt.Sprintf("aaaaaaaaaa11\trefs/merge-failed/%s\nbbbbbbbbbb22\trefs/merge-failed/%s\ncccccccccc33\trefs/merge-failed/%s\n"+
		"dddddddddd44\trefs/merge-failed/%s\neeeeeeeeee55\trefs/merge-failed/%s\nffffffffff66\trefs/merge-queue/%d-resub\n"+
		"1212121212aa\trefs/merge-failed/%s\n",
		fs(50, "rl"), fs(45, "im"), fs(200, "old"), fs(50, "alive"), fs(40, "resub"), at(20).Unix(), fs(30, "fresh"))
	for p, s := range map[string]string{Q: q.String(), AT: au.String(), DATA + "/last.tsv": last.String(), DATA + "/reland.tsv": ledger, lsr: remote} {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	call := func(args ...string) string {
		return captured(t, func() {
			qHeld = true
			defer func() {
				qHeld = false
				if e, ok := recover().(lockExit); !ok || e.code != 0 {
					t.Fatalf("종료 0이어야: %v", e)
				}
			}()
			statusCmd(args)
		})
	}
	nolog := "반송됨(이어 받은 줄 기록 없음)"
	line := func(out, name string) string {
		for _, l := range strings.Split(out, "\n") {
			if strings.HasPrefix(l, name+" ") {
				return l
			}
		}
		return ""
	}
	out := call() // 기본 표(숨김 규칙이 먹는다)
	for n, want := range map[string]string{
		"rl":    "반송됨 → rl-reland",
		"im":    "반송됨 — 이미 main",
		"old":   nolog, // 옛 줄 — 글만 사실대로, 숨기지 않음
		"fresh": nolog, // 반송 직후 장부 처리 전 — 같은 글, 숨기지 않음
	} {
		if l := line(out, n); !strings.Contains(l, want) || strings.Contains(l, "제출함") {
			t.Errorf("%s: %q, 바람 %q", n, l, want)
		}
	}
	// 넣은 뒤는 지금까지(초 단위 반올림이라 5h00m·5h01m — 끝난 줄처럼 반송 시각 200분 전에서 멈추면 1h40m)
	if strings.Contains(out, "숨김") || !strings.Contains(line(out, "old"), " 5h0") {
		t.Errorf("장부에 줄이 없는 반송은 끝난 줄이 아니다(숨기지 않고 넣은 뒤도 지금까지):\n%s", out)
	}
	if l := line(out, "rl"); !strings.Contains(l, "4h20m") {
		t.Errorf("넣은 뒤는 끝난 때(넣음 시각)까지: %q", l)
	}
	if l := line(out, "alive"); !strings.Contains(l, asubAud) || strings.Contains(l, "반송됨") {
		t.Errorf("살아서 고치는 줄은 그대로: %q", l)
	}
	if l := line(out, "resub"); !strings.Contains(l, "제출됨(병합기 줄 맨 앞)") || strings.Contains(l, "반송됨") {
		t.Errorf("다시 제출한 줄은 그대로: %q", l)
	}
	if l := line(out, "stale"); strings.Contains(l, "반송됨") {
		t.Errorf("이 줄 접수 전의 옛 반송 기록은 안 본다: %q", l)
	}
	// 끝난 줄의 한 정의(opsstate.Ended)가 그 글을 끝난 줄로 읽는다 — 손댈 것·상태 한 장이 같은 답
	for _, s := range []string{"반송됨 → rl-reland", "반송됨 — 이미 main"} {
		if !opsstate.Ended(opsstate.Lane{Stage: s}) {
			t.Errorf("Ended(%q) = 거짓", s)
		}
	}
	for _, s := range []string{"반송됨 — 다시 띄움 대기", nolog} {
		if opsstate.Ended(opsstate.Lane{Stage: s}) {
			t.Errorf("Ended(%q) = 참 — 끝난 줄이 아니다", s)
		}
	}
	// 두 번째 실행 — 같은 답
	if again := call(); again != out {
		t.Errorf("두 번째 실행이 다르다:\n%s\n---\n%s", out, again)
	}
	// 다시 맞춤 꺼짐(장부 없음) → 같은 글·안 숨김
	if err := os.Remove(DATA + "/reland.tsv"); err != nil {
		t.Fatal(err)
	}
	if o := call(); !strings.Contains(line(o, "old"), nolog) || !strings.Contains(line(o, "fresh"), nolog) || strings.Contains(o, "숨김") {
		t.Errorf("장부 없음:\n%s", o)
	}
	// 장부가 넣음을 적은 뒤 → 끝남
	led2 := ledger + strings.Join([]string{fs(30, "fresh"), "fresh", "fresh", "넣음", "fresh-reland", utc(5)}, "\t") + "\n"
	if err := os.WriteFile(DATA+"/reland.tsv", []byte(led2), 0o644); err != nil {
		t.Fatal(err)
	}
	if l := line(call(), "fresh"); !strings.Contains(l, "반송됨 → fresh-reland") || !strings.Contains(l, "4h55m") {
		t.Errorf("장부가 넣음을 적은 뒤: %q", l)
	}
	// 원격을 못 읽음 → 지금 글 그대로
	if err := os.WriteFile(bin+"/remote-bad", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	bad := call("--all")
	for _, n := range []string{"rl", "im", "old", "fresh"} {
		if l := line(bad, n); strings.Contains(l, "반송됨") || l == "" {
			t.Errorf("원격 모름(%s): %q", n, l)
		}
	}
}
