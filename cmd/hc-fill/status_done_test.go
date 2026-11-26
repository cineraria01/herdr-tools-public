package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 끝난 레인(ht-fill-status-done): 레인마다 가장 늦은 표식, 접수 뒤 표식만 착지, 줄에 있으면 안 끝남, 되돌려준 시각, hold로 뺌
func TestStatusDoneParts(t *testing.T) {
	out := "aaaaaaaa11\trefs/hc-landed/a/1795000100\n" +
		"bbbbbbbb22\trefs/hc-landed/a/1795000900\n" +
		"cccccccc33\trefs/hc-landed/b/1795000300\n" +
		"dddddddd44\trefs/hc-landed/mini-1795000000-123\n" +
		"eeeeeeee55\trefs/merge-queue/1795000200-c\n"
	mk := stMarks(out)
	if len(mk) != 2 || mk["a"] != (stMark{1795000900, "bbbbbbbb22"}) || mk["b"].SHA != "cccccccc33" {
		t.Fatalf("표식 %v", mk)
	}
	row := func(name, rid string) Row {
		r := make(Row, qNF)
		for i := range r {
			r[i] = "-"
		}
		return r.Set(qName, name).Set(qRID, rid).Set(qAdded, "2026-11-20T10:00")
	}
	when := time.Unix(1795000900, 0).Format("01-02 15:04")
	if s, e := stEnd(row("a", "r1795000500-1-1"), 0, mk); s != "bbbbbbbb "+when || e != 1795000900 {
		t.Errorf("접수 뒤 표식 = 착지: %q %d", s, e)
	}
	if s, _ := stEnd(row("a", "r1795000950-1-1"), 0, mk); s != "" {
		t.Errorf("접수 전 표식(같은 이름 옛 착지)은 착지 아님: %q", s)
	}
	if s, _ := stEnd(row("a", "r1795000500-1-1"), 2, mk); s != "" {
		t.Errorf("병합기 줄에 있으면(다시 낸 제출) 안 끝남: %q", s)
	}
	if s, _ := stEnd(row("z", "r1795000500-1-1"), 0, mk); s != "" {
		t.Errorf("표식 없음: %q", s)
	}

	r := row("s", "r1-1-1").Set(qReply, "m:/in/done/s.md#rq1")
	want, _ := time.ParseInLocation("2006-01-02T15:04", "2026-11-21T09:30", time.Local)
	if at := stSentAt(r, map[string]string{"rq1": "2026-11-21T09:30"}); at != want.Unix() {
		t.Errorf("되돌려준 시각 %d", at)
	}
	if stSentAt(r, nil) != 0 || stSentAt(row("s", "x"), map[string]string{"rq1": "2026-11-21T09:30"}) != 0 {
		t.Error("장부에 없음·손으로 넣은 줄은 0")
	}

	now := want.Add(61 * time.Minute)
	if !stOld(want.Unix(), "-", 60, now) || stOld(now.Unix()-60*60, "-", 60, now) || !stOld(now.Unix()-1, "-", 0, now) {
		t.Error("state_done분 넘게 끝난 줄만 뺌(0은 끝나자마자 — opsstate.Trim과 같은 비교)")
	}
	if !stOld(0, "2026-11-20T10:00", 60, now) || stOld(0, "?", 60, now) {
		t.Error("끝난 시각 모름은 넣은 때로, 그것도 모르면 안 뺌")
	}
}

// 착지한 레인은 대기열이 제출했어도 「착지 …」만(제출함 덧말 없음), 터미널이 아직 살아 있어도(정리 중) 착지
func TestStatusStageLanded(t *testing.T) {
	for _, in := range []stIn{
		{State: "up", Land: "beef0001 11-26 12:00", Aud: asubAud},
		{State: "up", Land: "beef0001 11-26 12:00", Aud: asubAud, Live: "idle", Last: Row{"l", "정리됨", "-", "12:00", "x"}},
	} {
		if s := stStage(in); s != "착지 beef0001 11-26 12:00" {
			t.Errorf("%+v: %q", in, s)
		}
	}
}

// status 명령(가짜 대기열·가짜 원격): ① 착지한 자동 제출 레인 → 「착지 …」, 넣은 뒤는 착지 시각에서 멈춤 ② 병합기 줄에서 기다리는
// 제출 → 지금 글·차례(접수 전 같은 이름 옛 표식은 착지 아님) ③ state_done분 넘게 끝난 줄은 기본에서 숨김·개수 줄, --all에 보임
// ④ --mine도 같은 규칙(제 줄만 세어 숨김)
func TestStatusDoneCmd(t *testing.T) {
	setData(t)
	HOME, LOCAL = t.TempDir(), "here"
	bin := t.TempDir()
	lsr := filepath.Join(bin, "ls-remote.txt")
	for n, body := range map[string]string{
		"git": "#!/bin/sh\nfor a; do [ \"$a\" = ls-remote ] && exec cat '" + lsr + "'; done\nexit 1\n",
		"hc":  "#!/bin/sh\n[ \"$1\" = ls ] && echo '{\"terminals\":[]}' && exit 0\nexit 1\n",
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
	add := func(m int) string { return at(m).Format("2006-01-02T15:04") }
	row := func(name, state, owner string, addedMin int) Row {
		r := make(Row, qNF)
		for i := range r {
			r[i] = "-"
		}
		return r.Set(qName, name).Set(qProj, "app").Set(qMach, "here").Set(qAddr, "t-"+name).Set(qState, state).
			Set(qOwner, owner).Set(qAdded, add(addedMin)).Set(qRID, fmt.Sprintf("r%d-1-1", at(addedMin).Unix()))
	}
	rows := []Row{
		row("landnew", "up", "t9", 90).Set(qAudit, "auto"),
		row("landold", "up", "t9", 300).Set(qAudit, "auto"),
		row("queued", "up", "t1", 50).Set(qAudit, "auto"),
		row("sentold", "sent", "t9", 14400).Set(qReply, "m:/in/x.md#rq1"),
		row("sentnew", "sent", "t1", 120).Set(qReply, "m:/in/y.md#rq2"),
		row("waiting", "wait", "t9", 5),
	}
	var q, au, led strings.Builder
	for _, r := range rows {
		q.WriteString(r.Line() + "\n")
	}
	for _, n := range []string{"landnew", "landold", "queued"} { // 대기열이 제출한 차수(anote → asubAud)
		au.WriteString(strings.Join([]string{"a-" + n, n, "1", "x", "s", "here", "t-" + n, "taken", "0", "pass", "0", "0", "-", "-", "0", "asub,asubok", "-"}, "\t") + "\n")
	}
	led.WriteString(strings.Join([]string{"rq1", "sentold", "-", "sent", "result", "-", "-", "-", add(14000)}, "\t") + "\n")
	led.WriteString(strings.Join([]string{"rq2", "sentnew", "-", "sent", "result", "-", "-", "-", add(10)}, "\t") + "\n")
	remote := fmt.Sprintf("1111111111aa\trefs/hc-landed/landnew/%d\n2222222222bb\trefs/hc-landed/landold/%d\n"+
		"3333333333cc\trefs/hc-landed/queued/%d\n4444444444dd\trefs/merge-queue/%d-queued\n",
		at(30).Unix(), at(200).Unix(), at(500).Unix(), at(20).Unix())
	for p, s := range map[string]string{Q: q.String(), AT: au.String(), LED: led.String(), lsr: remote} {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	call := func(args ...string) string {
		return captured(t, func() {
			qHeld = true // exit가 lockExit로 올라온다
			defer func() {
				qHeld = false
				if e, ok := recover().(lockExit); !ok || e.code != 0 {
					t.Fatalf("%v: 종료 0이어야: %v", args, e)
				}
			}()
			statusCmd(args)
		})
	}
	line := func(out, name string) string {
		for _, l := range strings.Split(out, "\n") {
			if strings.HasPrefix(l, name+" ") {
				return l
			}
		}
		return ""
	}

	out := call()
	if l := line(out, "landnew"); !strings.Contains(l, "1h00m") || !strings.Contains(l, "착지 11111111 "+at(30).Format("01-02 15:04")) ||
		strings.Contains(l, "제출함") {
		t.Errorf("① 착지 줄(넣은 뒤는 착지까지 1h00m, 제출함 아님): %q", l)
	}
	if l := line(out, "queued"); !strings.Contains(l, "제출됨(병합기 줄 맨 앞) · 대기열이 제출함") {
		t.Errorf("② 줄에서 기다리는 제출: %q", l)
	}
	if l := line(out, "sentnew"); !strings.Contains(l, "1h50m") || !strings.Contains(l, "되돌려줌") {
		t.Errorf("막 되돌려준 줄은 보이고 넣은 뒤는 되돌려준 때까지: %q", l)
	}
	if line(out, "waiting") == "" || line(out, "landold") != "" || line(out, "sentold") != "" ||
		!strings.Contains(out, "\n끝난 레인 2개 숨김(--all로 모두)\n") {
		t.Errorf("③ 오래 끝난 줄 숨김:\n%s", out)
	}
	all := call("--all")
	if l := line(all, "landold"); !strings.Contains(l, "착지 22222222") || line(all, "sentold") == "" || strings.Contains(all, "숨김") {
		t.Errorf("③ --all은 모두:\n%s", all)
	}
	t.Setenv("HC_TERMINAL_ID", "t9")
	mine := call("--mine")
	if line(mine, "landnew") == "" || line(mine, "waiting") == "" || line(mine, "queued") != "" || line(mine, "sentnew") != "" ||
		line(mine, "landold") != "" || !strings.Contains(mine, "\n끝난 레인 2개 숨김(--all로 모두)\n") {
		t.Errorf("④ --mine:\n%s", mine)
	}
	if m := call("--mine", "--all"); line(m, "landold") == "" || line(m, "queued") != "" {
		t.Errorf("④ --mine --all:\n%s", m)
	}
}
