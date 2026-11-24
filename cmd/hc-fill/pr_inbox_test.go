//go:build !windows

package main

// 「완료」 없이 제출하는 레인(받은편지·--no-audit — 감사 칸이 auto가 아닌 줄)의 PR(pr-inbox-lanes, prinbox.go):
// 커밋 SHA가 든 첫 보고(중간·완료·제출)에서 PR을 열고, 바뀐 SHA는 다시 완료처럼, 제출은 「제출됨」 + 자기 감사 리뷰 한 번.
// 손잡이 pr_inbox 0이면 그 레인들은 지금처럼(완료만) — 대기열(auto) 레인은 어느 쪽이든 그대로

import (
	"os"
	"strings"
	"testing"
)

const pinSID = "fx-20261124T010203-4242"

// pinRow: 받은편지 레인 줄(감사 칸 aud, 회신 칸 받은편지 꼴)과 작업 트리
func (k *prT) pinRow(name, aud string) {
	q := []string{name, "app", k.dir + "/brief.md", "-", "-", "up", "here", "here/" + name, "-", "t", "lp:/inbox#r1-abcdef", "c/coord", "normal", "-", aud, "-", "r-" + name}
	TERMS = append(TERMS, Term{M: "here", Addr: "here/" + name, Name: name, Status: "working"}) // 살아 있는 레인
	f, err := os.OpenFile(Q, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		k.t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(strings.Join(q, "\t") + "\n"); err != nil {
		k.t.Fatal(err)
	}
	if err := os.MkdirAll(k.dir+"/app.wt/"+name, 0o755); err != nil {
		k.t.Fatal(err)
	}
}

// pinSubmit: hc-lane land가 적는 꼴의 제출 사건(12칸 submit:<ID>)
func (k *prT) pinSubmit(at, lane, sha string) {
	EV = append(EV, strings.Join([]string{at, "here", "here/" + lane, lane, "제출", sha, pinSID + " → refs/merge-queue/1-" + lane + " (병합기 mg 가 main 에 넣는다)", "1", "-", "-", "-", "submit:" + pinSID}, "\t"))
}

// ① 「완료」 없이 「중간 <sha>」 보고 → PR 열림(고치기 전 코드로 실패: pr create 0)
// ② 같은 SHA의 중간 → 아무것도 안 씀, 바뀐 SHA의 중간 → 머리 갱신 + 다시 완료 댓글, 새 PR 없음
func TestPRInboxMidOpens(t *testing.T) {
	k := prSetup(t, "pr app")
	k.pinRow("w2", "-")
	k.ev("2026-11-24T01:00:00Z", "here", "w2", "중간", "-", "구현 시작")
	k.ev("2026-11-24T01:05:00Z", "here", "w2", "중간", prSHA1, "구현·커밋, 감사 시작")
	prTick()
	if k.n("pr create") != 1 || k.n("--head pr/w2") != 1 || k.n("git/refs/heads/pr/w2 -f sha="+prSHA1) != 1 || !strings.Contains(k.logged(), "--title w2 — 구현·커밋, 감사 시작") {
		t.Fatalf("중간 보고로 열기:\n%s", k.logged())
	}
	if pr, _ := prOf("w2"); pr != "PR #7" {
		t.Fatalf("prOf = %q", pr)
	}
	k.ev("2026-11-24T01:20:00Z", "here", "w2", "중간", prSHA1, "감사 완료 높음 0·중간 0·낮음 2")
	prTick()
	if k.n("pr create") != 1 || k.n("pr comment") != 0 || k.n("api -X PATCH") != 1 {
		t.Fatalf("같은 SHA를 다시 썼다:\n%s", k.logged())
	}
	k.ev("2026-11-24T01:30:00Z", "here", "w2", "중간", prSHA2, "고침 커밋")
	prTick()
	prTick()
	if k.n("pr create") != 1 || k.n("git/refs/heads/pr/w2 -f sha="+prSHA2) != 1 || k.n("pr comment 7") != 1 || !strings.Contains(k.bodies(), "다시 완료 22222222 — 고침 커밋") {
		t.Fatalf("바뀐 SHA:\n%s\n%s", k.logged(), k.bodies())
	}
}

// ③ 제출 → 「제출됨」 댓글 + 자기 감사 리뷰(작성자 다름 = 승인) 한 번. 몇 바퀴를 돌아도 한 번. --no-audit 레인은 리뷰 없음
func TestPRInboxSubmitReview(t *testing.T) {
	k := prSetup(t, "pr app", "pr_token_cmd echo tok-x")
	k.pinRow("w2", "-")
	k.ev("2026-11-24T01:05:00Z", "here", "w2", "중간", prSHA1, "구현·커밋, 감사 시작")
	prTick()
	k.ev("2026-11-24T01:20:00Z", "here", "w2", "중간", prSHA1, "감사 완료 높음 0·중간 0·낮음 2 (보고 ~/r/a.md)")
	k.pinSubmit("2026-11-24T01:21:00Z", "w2", prSHA1)
	prTick()
	prTick()
	if k.n("pr create") != 1 || k.n("--approve") != 1 || k.n("pr comment 7") != 1 || k.n("api -X PATCH") != 1 {
		t.Fatalf("제출:\n%s", k.logged())
	}
	b := k.bodies()
	if !strings.Contains(b, "제출됨 — 병합기 줄에 넣음") || !strings.Contains(b, "자기 감사 통과") || !strings.Contains(b, "감사 완료 높음 0·중간 0·낮음 2 (보고 ~/r/a.md)") || !strings.Contains(b, "보고서는 레인 기계") {
		t.Fatalf("본문:\n%s", b)
	}

	k = prSetup(t, "pr app", "pr_token_cmd echo tok-x")
	k.pinRow("w3", "no-audit")
	k.ev("2026-11-24T01:05:00Z", "here", "w3", "중간", prSHA1, "구현·커밋")
	prTick()
	k.pinSubmit("2026-11-24T01:21:00Z", "w3", prSHA1)
	prTick()
	if k.n("pr create") != 1 || k.n("pr review") != 0 || k.n("pr comment 7") != 1 {
		t.Fatalf("--no-audit 제출:\n%s", k.logged())
	}
}

// 감사 글이 없는 받은편지 제출 → 승인 없이 댓글 한 줄(판정을 대기열 기계에서 못 읽음)
func TestPRInboxSubmitNoAuditText(t *testing.T) {
	k := prSetup(t, "pr app", "pr_token_cmd echo tok-x")
	k.pinRow("w2", "-")
	k.ev("2026-11-24T01:05:00Z", "here", "w2", "중간", prSHA1, "구현·커밋")
	prTick()
	k.pinSubmit("2026-11-24T01:21:00Z", "w2", prSHA1)
	prTick()
	if k.n("pr review") != 0 || k.n("pr comment 7") != 2 || !strings.Contains(k.bodies(), "자기 감사 판정을 대기열 기계에서 못 읽음") {
		t.Fatalf("감사 글 없음:\n%s\n%s", k.logged(), k.bodies())
	}
}

// ④ 숨은 참조를 못 올림(작업 트리가 이미 정리됨·push 실패)인데 제출 사건이 있음 → 저장소 폴더에서 원격을 읽어 그 커밋으로 머리 가지
func TestPRInboxSubmitRefOnly(t *testing.T) {
	k := prSetup(t, "pr app")
	k.pinRow("w2", "-")
	if err := os.Remove(k.dir + "/app.wt/w2"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(k.dir+"/app", 0o755); err != nil {
		t.Fatal(err)
	}
	wr(t, k.dir+"/git", "#!/bin/sh\ncase \"$1 $2\" in\n\"remote get-url\") echo \"git@github.com:example-org/proj.git\" ;;\n\"push -q\") echo \"git $*\" >>\"$HC_T_LOG\"; exit 1 ;;\n*) exit 9 ;;\nesac\n")
	k.pinSubmit("2026-11-24T01:21:00Z", "w2", prSHA1)
	prTick()
	if k.n("pr create") != 1 || k.n("git/refs/heads/pr/w2 -f sha="+prSHA1) != 1 || k.n("--name hc-fill-pr") != 0 {
		t.Fatalf("제출 참조로 머리:\n%s", k.logged())
	}
	// 제출이 아닌 보고는 그대로 — 작업 트리가 없으면 건너뜀
	k2 := prSetup(t, "pr app")
	k2.pinRow("w4", "-")
	if err := os.Remove(k2.dir + "/app.wt/w4"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(k2.dir+"/app", 0o755); err != nil {
		t.Fatal(err)
	}
	k2.ev("2026-11-24T01:05:00Z", "here", "w4", "중간", prSHA1, "구현·커밋")
	prTick()
	if k2.n("gh ") != 0 {
		t.Fatalf("작업 트리 없는 중간 보고:\n%s", k2.logged())
	}
}

// ⑤ 제출 때 이미 착지(병합기가 바퀴보다 빠름) → PR 안 엶 + 로그 한 줄, 오류 사건 없음. gh가 「No commits between」이면 같은 취급
func TestPRInboxLandedFirst(t *testing.T) {
	k := prSetup(t, "pr app")
	k.pinRow("w2", "-")
	k.pinSubmit("2026-11-24T01:21:00Z", "w2", prSHA1)
	k.ev("2026-11-24T01:21:40Z", "mg", "w2", "착지", prSHA2, "병합기 착지 — 제출 ID x")
	out := captured(t, prTick)
	prTick()
	if k.n("gh ") != 0 || k.n("--name hc-fill-pr") != 0 || strings.Count(out, "PR 없음 w2 — 착지가 먼저") != 1 {
		t.Fatalf("착지가 먼저:\n%s\n%s", k.logged(), out)
	}

	k = prSetup(t, "pr app")
	k.pinRow("w2", "-")
	wr(t, k.dir+"/gh", "#!/bin/sh\necho \"gh $*\" >>\"$HC_T_LOG\"\ncase \"$1 $2\" in\n\"pr create\") echo 'pull request create failed: GraphQL: No commits between main and pr/w2 (createPullRequest)' >&2; exit 1 ;;\nesac\nexit 0\n")
	k.pinSubmit("2026-11-24T01:21:00Z", "w2", prSHA1)
	out = captured(t, prTick)
	prTick()
	if k.n("pr create") != 1 || k.n("--name hc-fill-pr") != 0 || !strings.Contains(out, "PR 없음 w2 — 착지가 먼저") || isFile(DATA+"/pr-fail") {
		t.Fatalf("No commits between:\n%s\n%s", k.logged(), out)
	}
}

// ⑥ 받은편지 보고 글의 개인 값 → 제목·본문에서 가려짐
func TestPRInboxMask(t *testing.T) {
	k := prSetup(t, "pr app")
	k.pinRow("w2", "-")
	k.ev("2026-11-24T01:05:00Z", "here", "w2", "중간", prSHA1, "secret-project 문의 box-77 고침")
	prTick()
	if all := k.logged() + k.bodies(); k.n("pr create") != 1 || strings.Contains(all, "secret-project") || strings.Contains(all, "box-77") || !strings.Contains(all, "‹가림›") {
		t.Fatalf("가림:\n%s", all)
	}
}

// ⑦ 손잡이 pr_inbox 0 → 완료 없는 레인은 PR 없음, 대기열 레인은 그대로
// ⑧ 대기열(auto) 레인은 중간 SHA 보고로 열지 않는다(전후 같음) — 완료에서 연다
func TestPRInboxKnobAndAuto(t *testing.T) {
	k := prSetup(t, "pr app", "pr_inbox 0")
	k.pinRow("w2", "-")
	k.ev("2026-11-24T01:05:00Z", "here", "w2", "중간", prSHA1, "구현·커밋")
	k.pinSubmit("2026-11-24T01:21:00Z", "w2", prSHA1)
	k.ev("2026-11-24T01:06:00Z", "here", "w1", "완료", prSHA2, "기능 X 끝")
	prTick()
	if k.n("pr create") != 1 || k.n("--head pr/w1") != 1 || k.n("pr/w2") != 0 {
		t.Fatalf("pr_inbox 0:\n%s", k.logged())
	}

	k = prSetup(t, "pr app")
	k.ev("2026-11-24T01:05:00Z", "here", "w1", "중간", prSHA1, "중간 커밋")
	prTick()
	if k.logged() != "" {
		t.Fatalf("auto 레인이 중간으로 열었다:\n%s", k.logged())
	}
	k.ev("2026-11-24T01:10:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	if k.n("pr create") != 1 {
		t.Fatalf("auto 레인 완료:\n%s", k.logged())
	}
}
