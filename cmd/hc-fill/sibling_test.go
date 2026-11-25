package main

// add 때 다른 지휘가 같은 프로젝트에 막 넣은 레인 알림(siblingNote)과, 규칙 문서 언급은 겹침이 아님(ruleDoc) — ht-add-sibling-notice(2026-11-25)

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestSiblingNote(t *testing.T) {
	setData(t)
	LOCAL = "here"
	confLines = lines("local here\n")
	d := t.TempDir()
	now := time.Date(2026, 11, 25, 21, 30, 0, 0, time.Local)
	at := func(minAgo int) string {
		return now.Add(-time.Duration(minAgo) * time.Minute).Format("2006-01-02T15:04")
	}
	row := func(n, p, st, added, own, k15 string) string {
		b := dupBrief(t, d, n, "\n# "+n+" — 추측 검증 상한 넘으면 맥 직접 검증 (herdr-tools, 지휘 "+own+")\n본문\n")
		r := pad16(n, p, b, "-", "-", st, "here", "-", "-", added, "-", own, "normal", "-", k15, "-")
		return strings.Join(r, "\t") + "\n"
	}
	q := row("other-recent", "app", "wait", at(5), "t30x", "-") + // 보임
		row("other-up", "app", "up", at(30), "t30x", "auto") + // 떠 있음·감사 중 — 보임
		row("mine", "app", "wait", at(3), "t3yx", "-") + // 같은 지휘 — 안 보임
		row("no-owner", "app", "wait", at(3), "-", "-") + // 주인 없음 — 안 보임
		row("other-prj", "other", "wait", at(3), "t30x", "-") + // 다른 프로젝트 — 안 보임
		row("old", "app", "wait", at(200), "t30x", "-") + // N분 넘음 — 안 보임
		row("landed", "app", "up", at(10), "t30x", "-") + // 착지 — 안 보임
		row("cleaned", "app", "up", at(10), "t30x", "-") + // 정리됨 — 안 보임
		row("sent-x", "app", "sent", at(10), "t30x", "-") + // 받은편지 보냄 — 안 보임
		row("audit-other-up", "app", "up", at(10), "t30x", "audit=other-up") // 감사 줄 — 안 보임
	if err := os.WriteFile(Q, []byte(q), 0o644); err != nil {
		t.Fatal(err)
	}
	last := "landed\t착지\tx\t21:25\n" + "cleaned\t정리됨\tx\t21:25\n" + "other-up\t완료\tx\t21:20\n"
	if err := os.WriteFile(DATA+"/last.tsv", []byte(last), 0o644); err != nil {
		t.Fatal(err)
	}
	got := dupCapture(t, func() { siblingNote("new", "app", "t3yx", now) })
	want := "알림: 같은 프로젝트에 다른 지휘가 막 넣은 레인 — 같은 일이면 합쳐라: other-recent (지휘 t30x, 5분 전) — other-recent — 추측 검증 상한 넘으면 맥 직접 검증 (herdr-tools, 지휘 t30x)\n" +
		"알림: 같은 프로젝트에 다른 지휘가 막 넣은 레인 — 같은 일이면 합쳐라: other-up (지휘 t30x, 30분 전) — other-up — 추측 검증 상한 넘으면 맥 직접 검증 (herdr-tools, 지휘 t30x)\n"
	if got != want {
		t.Fatalf("알림\n got %q\nwant %q", got, want)
	}
	// 손잡이(설정 줄) 0이면 끔, 10이면 10분 안만
	confLines = lines("local here\nsibling_min 0\n")
	if got := dupCapture(t, func() { siblingNote("new", "app", "t3yx", now) }); got != "" {
		t.Fatalf("0이면 끔: %q", got)
	}
	confLines = lines("local here\nsibling_min 10\n")
	if got := dupCapture(t, func() { siblingNote("new", "app", "t3yx", now) }); !strings.Contains(got, "other-recent") || strings.Contains(got, "other-up") {
		t.Fatalf("10분: %q", got)
	}
}

func TestRuleDocNotOverlap(t *testing.T) {
	setData(t)
	repo := t.TempDir()
	if err := os.MkdirAll(repo+"/cmd/hc-merger", 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"CONTRACT.md", "CLAUDE.md", "AGENTS.md", "cmd/hc-merger/specfront.go"} {
		if err := os.WriteFile(repo+"/"+f, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	LOCAL = "here"
	confLines = lines("local here\nrepo app here " + repo + "\n")
	d := t.TempDir()
	rules := "이 저장소의 CLAUDE.md·AGENTS.md·CONTRACT.md 규칙을 그대로 따른다\n규칙 문서 (CONTRACT.md) 참고\n"
	a := dupBrief(t, d, "a", "# a\n"+rules+"추측 검증을 고친다\n")
	b := dupBrief(t, d, "b", "# b\n"+rules+"cmd/hc-merger/specfront.go 를 고친다\n")
	c := dupBrief(t, d, "c", "# c\n"+rules+"cmd/hc-merger/specfront.go 도 손댄다\n")
	own := func(lane, brief string) []Row {
		var o []Row
		for _, k := range lkeys("app", brief) {
			o = append(o, append(Row{lane}, k...))
		}
		return o
	}
	if ov := ovcmp(own("a", a), lkeys("app", b)); len(ov) != 0 {
		t.Fatalf("규칙 문서만 같이 언급 — 겹침 아님: %v", ov)
	}
	if ov := ovcmp(own("c", c), lkeys("app", b)); len(ov) != 1 || ov[0].F(ovKeys) != "cmd/hc-merger/specfront.go" {
		t.Fatalf("같은 파일 — 겹침: %v", ov)
	}
	// 규칙 문서를 「파일:」로 예약하면 그대로 예약 겹침
	r1 := dupBrief(t, d, "r1", "# r1\n- 파일: CONTRACT.md\n")
	r2 := dupBrief(t, d, "r2", "# r2\n- 파일: CONTRACT.md\n")
	if ov := ovcmp(own("r1", r1), lkeys("app", r2)); len(ov) != 1 || ov[0].F(ovGrade) != "예약" {
		t.Fatalf("규칙 문서 예약 — 겹침: %v", ov)
	}
}
