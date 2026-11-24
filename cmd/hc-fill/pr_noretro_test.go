//go:build !windows

package main

// 새 PR은 살아 있는 레인의 새 보고에만(pr-inbox-no-retro, prfresh.go): pr-inbox-lanes가 깔리자 며칠 전 끝난 레인 여섯에 PR이 열리고 끝난 레인
// 105개에 「PR 없음 … 착지가 먼저」가 한 줄씩 찍혔다. 대기열 줄은 끝나도 「up」이라 지난 보고를 다 훑었다

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// gone: 그 레인 터미널이 이번 바퀴 표에 없다(끝남)
func (k *prT) gone(name string) {
	var keep []Term
	for _, t := range TERMS {
		if t.Name != name {
			keep = append(keep, t)
		}
	}
	TERMS = keep
}

// fresh: 이 기능을 지금 처음 켠 프로젝트(prSetup이 둔 옛 켠 시각을 지운다)
func (k *prT) fresh() {
	if err := os.Remove(DATA + "/pr-on.tsv"); err != nil {
		k.t.Fatal(err)
	}
}

const prOld, prOld2 = "2026-11-22T01:00:00Z", "2026-11-22T02:00:00Z" // 켜기 전(며칠 전)

func prLater(min int) string {
	return time.Now().UTC().Add(time.Duration(min) * time.Minute).Format("2006-01-02T15:04:05Z")
}

// ① 끝난 받은편지 레인(터미널 없음, SHA 든 중간·제출이 과거, 기준에 안 듦) → PR 안 엶(고치기 전 코드로 실패: pr create 1)
// ⑥ 끝난 대기열(auto) 레인의 옛 완료 기록 → 안 엶(고치기 전 코드로 실패: pr create 1)
func TestPRNoRetroFinished(t *testing.T) {
	k := prSetup(t, "pr app")
	k.fresh()
	k.gone("w1")
	k.ev(prOld, "here", "w1", "완료", prSHA1, "옛 완료 (감사 hc-fill 자동)")
	k.pinRow("w2", "-")
	k.gone("w2")
	k.ev(prOld, "here", "w2", "중간", prSHA1, "구현·커밋")
	k.pinSubmit(prOld2, "w2", prSHA2)
	out := captured(t, prTick)
	if k.n("pr create") != 0 || k.n("git push") != 0 || strings.Contains(out, "PR 없음") {
		t.Fatalf("끝난 옛 레인에 열었다:\nlog=%s\nout=%s", k.logged(), out)
	}
	// 켜진 뒤라도 끝난 레인(터미널 없음·병합 줄에 없음)은 열지 않는다
	k.ev(prLater(1), "here", "w2", "중간", prSHA1, "뒤늦은 보고")
	prTick()
	if k.n("pr create") != 0 {
		t.Fatalf("끝난 레인의 새 보고로 열었다:\n%s", k.logged())
	}
}

// ② 끝난 레인 105개(옛 중간 SHA 뒤 착지) → 「PR 없음」 로그 0줄(고치기 전 코드로 실패: 105줄)
func TestPRNoRetroNoLandLog(t *testing.T) {
	k := prSetup(t, "pr app")
	k.fresh()
	for i := 0; i < 105; i++ {
		n := "old" + strconv.Itoa(i)
		k.pinRow(n, "-")
		k.gone(n)
		k.ev(prOld, "here", n, "중간", prSHA1, "커밋")
		k.ev(prOld2, "mg", n, "착지", prSHA2, "착지")
	}
	out := captured(t, prTick)
	if c := strings.Count(out, "PR 없음"); c != 0 || k.n("pr create") != 0 {
		t.Fatalf("「PR 없음」 %d줄:\n%s", c, out)
	}
}

// ③ 살아 있는 받은편지 레인이 켠 뒤 SHA 든 중간 보고 → 엶
// ④ 살아 있지만 보고가 켜기 전 것뿐 → 안 엶(고치기 전 코드로 실패: 엶), 그 뒤 새 보고 → 엶
func TestPRNoRetroLiveAfterOn(t *testing.T) {
	k := prSetup(t, "pr app")
	k.fresh()
	k.pinRow("w2", "-")
	k.ev(prOld, "here", "w2", "중간", prSHA1, "켜기 전 커밋")
	prTick()
	if k.n("pr create") != 0 {
		t.Fatalf("켜기 전 보고로 열었다:\n%s", k.logged())
	}
	if s, _ := readFile(DATA + "/pr-on.tsv"); !strings.Contains(s, "app\tpr\t") || !strings.Contains(s, "app\tinbox\t") {
		t.Fatalf("켠 시각을 안 적었다: %q", s)
	}
	k.ev(prLater(1), "here", "w2", "중간", prSHA2, "켠 뒤 커밋")
	prTick()
	if k.n("pr create") != 1 || k.n("git/refs/heads/pr/w2 -f sha="+prSHA2) != 1 {
		t.Fatalf("켠 뒤 보고로 안 열었다:\n%s", k.logged())
	}
}

// pr_inbox를 뒤에 켜면 그때가 완료 없는 레인의 기준 — pr만 켜져 있던 동안의 보고로는 안 연다
func TestPRNoRetroInboxLater(t *testing.T) {
	k := prSetup(t, "pr app", "pr_inbox 0")
	k.fresh()
	k.pinRow("w2", "-")
	prTick()
	if s, _ := readFile(DATA + "/pr-on.tsv"); strings.Contains(s, "inbox") {
		t.Fatalf("pr_inbox 0인데 inbox 시각: %q", s)
	}
	k.ev(prOld, "here", "w2", "중간", prSHA1, "pr_inbox 끔 동안") // pr 켠 뒤(2000년), inbox 켜기 전
	for i, l := range confLines {
		if l == "pr_inbox 0" {
			confLines[i] = "pr_inbox 1"
		}
	}
	wr(t, DATA+"/pr-on.tsv", "app\tpr\t2000-01-01T00:00:00Z\n") // pr은 옛날에, inbox는 이 바퀴에 처음 본다
	prTick()
	if k.n("pr create") != 0 {
		t.Fatalf("pr_inbox 켜기 전 보고로 열었다:\n%s", k.logged())
	}
	k.ev(prLater(2), "here", "w2", "중간", prSHA2, "켠 뒤")
	prTick()
	if k.n("pr create") != 1 {
		t.Fatalf("켠 뒤 보고로 안 열었다:\n%s", k.logged())
	}
}

// ⑤ 병합 줄에 제출이 서 있는 레인(터미널은 끝남) + 켠 뒤 제출 → 엶. 병합 줄을 못 읽으면(모름) 이 바퀴는 안 열고 다음 바퀴에
func TestPRNoRetroQueuedSubmit(t *testing.T) {
	k := prSetup(t, "pr app")
	k.fresh()
	prTick() // 켠 시각을 적는다
	k.pinRow("w5", "-")
	k.gone("w5")
	k.pinSubmit(prLater(1), "w5", prSHA1)
	MQBAD = ",app"
	prTick()
	if k.n("pr create") != 0 {
		t.Fatalf("병합 줄을 모르는데 열었다:\n%s", k.logged())
	}
	MQBAD, MQS = "", []Row{{"app", "refs/merge-queue/1764000000-w5", prSHA1}}
	prTick()
	if k.n("pr create") != 1 || k.n("--head pr/w5") != 1 {
		t.Fatalf("병합 줄에 선 제출로 안 열었다:\n%s", k.logged())
	}
}

// ⑦ 이미 열린 PR은 레인 터미널이 끝나도 뒤 사건(착지·닫기) 그대로
func TestPRNoRetroOpenPRKeeps(t *testing.T) {
	k := prSetup(t, "pr app")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝 (감사 hc-fill 자동)")
	prTick()
	if k.n("pr create") != 1 {
		t.Fatalf("열기:\n%s", k.logged())
	}
	k.gone("w1")
	k.ev("2026-11-24T02:00:00Z", "here", "w1", "착지", prSHA1, "손 착지")
	prTick()
	if k.n("pr comment 7") != 2 || k.n("pr close 7") != 1 {
		t.Fatalf("끝난 레인의 열린 PR 착지·닫기:\n%s", k.logged())
	}
}
