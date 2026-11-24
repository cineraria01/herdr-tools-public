//go:build !windows

package main

// 원래 PR 잇기(fill-polish-1124 나, prof.go): --pr-of로 넣은 레인(w1-fix)의 완료·감사·착지는 원래 PR(#7, 머리 pr/w1)에,
// 원래 PR이 닫힌 뒤면 새 PR. 고치기 전: 꼬리가 -reland가 아니라 새 PR이 열렸다

import (
	"os"
	"strings"
	"testing"
)

// pofFix: w1 완료로 PR #7을 열고, 대기열에 w1-fix(--pr-of w1)를 둔다
func pofFix(t *testing.T) *prT {
	t.Helper()
	k := prSetup(t, "pr app")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	k.row("w1-fix")
	if err := os.MkdirAll(k.dir+"/app.wt/w1-fix", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := prOfPut("w1-fix", "w1"); err != nil {
		t.Fatal(err)
	}
	return k
}

// 완료 → 새 PR 없이 원래 PR에 「다시 완료」·머리 가지 pr/w1 갱신·짝 참조(고치기 전 코드로 실패: pr create 2번)
func TestPROfContinues(t *testing.T) {
	k := pofFix(t)
	k.ev("2026-11-24T03:00:00Z", "here", "w1-fix", "완료", prSHA2, "다시 냄")
	prTick()
	if k.n("pr create") != 1 || k.n("git/refs/heads/pr/w1 -f sha="+prSHA2) != 1 || k.n("pr comment 7") != 1 ||
		!strings.Contains(k.bodies(), "다시 완료 22222222 — 다시 냄 (다시 맞춤 레인 w1-fix)") {
		t.Fatalf("이어 쓰기:\n%s\n%s", k.logged(), k.bodies())
	}
	if k.n("git push -q origin +"+prSHA2+":refs/herdr/pr-of/w1-fix/w1") != 1 {
		t.Fatalf("짝 참조(병합기가 머리를 찾는 길):\n%s", k.logged())
	}
	if pr, _ := prOf("w1-fix"); pr != "PR #7" {
		t.Fatalf("prOf(w1-fix) = %q", pr)
	}
	// 감사 판정(w1-fix 차수) → 원래 PR에 커밋 상태·댓글
	if err := os.MkdirAll(HOME+"/.local/share/hc-fill/reports", 0o755); err != nil {
		t.Fatal(err)
	}
	wr(t, HOME+"/.local/share/hc-fill/reports/audit-a9.md", "보고서 본문")
	at := []string{"a9", "w1-fix", "1", "w1-fix@1", prSHA2, "here", "here/w1-fix", "deliver", "0", "pass", "0", "0", "-", "-", "0", "hml=0.0.1",
		"~/.local/share/hc-fill/reports/audit-a9.md"}
	if err := appendLine(AT, strings.Join(at, "\t")); err != nil {
		t.Fatal(err)
	}
	prTick()
	if !strings.Contains(k.bodies(), "감사 1차 통과") || k.n("statuses/"+prSHA2) != 1 {
		t.Fatalf("감사 판정이 원래 PR에:\n%s\n%s", k.logged(), k.bodies())
	}
	// 병합기 착지(머리 들어감) → 원래 PR이 Merged(닫기 없음)
	k.ev("2026-11-24T04:00:00Z", "mg", "w1-fix", "착지", prSHA2, "병합기: 착지 — 제출 x · PR 머리 22222222 들어감")
	prTick()
	m, _ := prLoad()
	if m["w1"].State != "merged" || k.n("pr close") != 0 || !strings.Contains(k.bodies(), "착지 22222222") {
		t.Fatalf("Merged 길: %+v\n%s\n%s", m["w1"], k.logged(), k.bodies())
	}
}

// 원래 PR이 닫힌 뒤면 새 PR(머리 pr/w1-fix, 짝 참조 없음, 닫힌 PR을 다시 열지 않음) · 한 줄 알림
func TestPROfClosedOpensNew(t *testing.T) {
	k := pofFix(t)
	prCloseReq("w1", "지휘가 닫음")
	prTick()
	k.ev("2026-11-24T03:00:00Z", "here", "w1-fix", "완료", prSHA2, "다시 냄")
	out := captured(t, prTick)
	if k.n("pr create") != 2 || k.n("git/refs/heads/pr/w1-fix -f sha="+prSHA2) != 1 || k.n("pr-of/") != 0 || k.n("pr reopen") != 0 {
		t.Fatalf("새 PR:\n%s", k.logged())
	}
	if !strings.Contains(out, "PR 거울 w1-fix: --pr-of w1 의 PR이 없거나 이미 닫힘·Merged — 새 PR로") {
		t.Fatalf("알림 없음: %s", out)
	}
}

// 기록: add 없이 지우기는 파일을 안 만든다 · 덮어쓰기 · -로 지움 · rm 되돌리기 줄에 붙음
func TestPROfStore(t *testing.T) {
	prSetup(t)
	if err := prOfPut("x", ""); err != nil || isFile(prOfFile()) {
		t.Fatalf("지울 것뿐인데 파일: %v", err)
	}
	if prOfPut("x", "a") != nil || prOfPut("y", "b") != nil || prOfPut("x", "c") != nil || prOfGet("x") != "c" || prOfGet("y") != "b" {
		t.Fatalf("덮어쓰기: x=%q y=%q", prOfGet("x"), prOfGet("y"))
	}
	if u := rmUndo(Row{"x", "app", "/b.md", "-", "-", "wait", "-", "-", "-", "-", "-", "-", "normal"}); !strings.HasSuffix(u, " --pr-of "+shq("c")) {
		t.Fatalf("rm 되돌리기: %s", u)
	}
	if prOfPut("x", "-") != nil || prOfGet("x") != "" || prOfGet("y") != "b" {
		t.Fatal("-로 지움")
	}
	if prBaseOf("y") != "b" || prBaseOf("z-reland") != "z" || prBaseOf("z") != "z" {
		t.Fatal("prBaseOf")
	}
}
