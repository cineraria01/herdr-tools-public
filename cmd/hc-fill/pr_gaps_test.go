//go:build !windows

package main

// PR 거울의 구멍(2026-11-24 pr-mirror-gaps, pr2.go): ① 다시 맞춤 2차(…-reland2)도 원래 PR에 ② --pr-of 받은편지 레인의 첫 보고가
// 「제출」이어도 짝 참조를 올린다 ③ 이미 올린 감사 판정의 보고서를 다시 읽지 않는다 ④ 「정리됨」으로(착지·제출 없이) 끝난 레인의 열린 PR은
// 닫는다 ⑥ 이미 Merged·Closed인 옛 PR이 쌓인 채 첫 바퀴 — 호출 0

import (
	"os"
	"strings"
	"testing"
)

var prSHA3 = strings.Repeat("3", 40)

// 가짜 hc: exec(원격 보고서 읽기)만 적고 본문을 낸다, 그 밖(hc get 등)은 모름
const prGapHc = `#!/bin/sh
[ "$1" = exec ] || exit 1
echo "hc $1 $2 $3" >>"$HC_T_LOG"
echo "원격 보고서 본문"
`

// ① 다시 맞춤 두 번: 장부의 둘째 「넣음」 줄은 레인 칸이 앞 다시 맞춤 레인(w1-reland) — 사슬을 따라가야 원래 PR에 잇는다
// (고치기 전: w1-reland2가 새 PR — pr create 2)
func TestPRGapsReland2(t *testing.T) {
	k := prSetup(t, "pr app")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	wr(t, DATA+"/reland.tsv", "#since\t2000-01-01T00:00:00Z\n"+
		"100-w1\tw1\tw1\t넣음\tw1-reland\t2026-11-24T01:30:00Z\n"+
		"200-w1-reland\tw1-reland\tw1\t넣음\tw1-reland2\t2026-11-24T02:30:00Z\n")
	for _, n := range []string{"w1-reland", "w1-reland2"} {
		k.row(n)
		if err := os.MkdirAll(k.dir+"/app.wt/"+n, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	k.ev("2026-11-24T02:00:00Z", "here", "w1-reland", "완료", prSHA2, "다시 맞춤 1")
	prTick()
	k.ev("2026-11-24T03:00:00Z", "here", "w1-reland2", "완료", prSHA3, "다시 맞춤 2")
	prTick()
	prTick()
	if k.n("pr create") != 1 || k.n("pr comment 7") != 2 || k.n("git/refs/heads/pr/w1 -f sha="+prSHA3) != 1 || k.n("heads/pr/w1-reland") != 0 {
		t.Fatalf("둘 다 원래 PR에:\n%s", k.logged())
	}
	if k.n("+"+prSHA2+":refs/herdr/pr-of/w1-reland/w1") != 1 || k.n("+"+prSHA3+":refs/herdr/pr-of/w1-reland2/w1") != 1 {
		t.Fatalf("짝 참조 둘:\n%s", k.logged())
	}
	if !strings.Contains(k.bodies(), "다시 완료 33333333 — 다시 맞춤 2 (다시 맞춤 레인 w1-reland2)") {
		t.Fatalf("본문:\n%s", k.bodies())
	}
}

// ① 세 번 이은 사슬·줄인 이름·장부에 없는 이름·고리도 맨 처음 레인까지 — 레인 칸으로 사슬을 따라간다(바탕 칸 값에 기대지 않는다)
func TestPRGapsRelandChain(t *testing.T) {
	prSetup(t)
	wr(t, DATA+"/reland.tsv", "#since\t2000-01-01T00:00:00Z\n"+
		"100-a\ta\ta\t넣음\ta-reland\tT\n"+
		"200-a-reland\ta-reland\tmain\t넣음\ta-reland2\tT\n"+
		"300-a-reland2\ta-reland2\t-\t넣음\ta-reland3\tT\n"+
		"400-long-name-cut\tlong-name\tlong-name\t넣음\tlong-na-reland\tT\n"+ // 줄인 이름 — 꼬리를 떼도 원래 이름이 아니다
		"500-x\ty\ty\t넣음\tx\tT\n600-y\tx\tx\t넣음\ty\tT\n") // 고리
	for n, want := range map[string]string{"a-reland": "a", "a-reland2": "a", "a-reland3": "a", "long-na-reland": "long-name", "b-reland4": "b", "c": "c"} {
		if got := prRelandBase(n); got != want {
			t.Errorf("prRelandBase(%s) = %s, 바람 %s", n, got, want)
		}
	}
	if got := prRelandBase("x"); got != "x" && got != "y" {
		t.Errorf("고리: %s", got)
	}
}

// ② --pr-of 받은편지 레인의 첫 보고가 「제출」: 여는 길은 건너뛰어도(머리를 옮기지 않음) 짝 참조는 한 번 올린다
// (고치기 전: pr-of push 0 — 병합기가 원래 머리를 못 찾아 원래 PR이 Merged가 아니라 닫힘)
func TestPRGapsPrOfSubmitFirst(t *testing.T) {
	k := prSetup(t, "pr app")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	k.pinRow("w1-fix", "-")
	if err := prOfPut("w1-fix", "w1"); err != nil {
		t.Fatal(err)
	}
	k.pinSubmit("2026-11-24T03:00:00Z", "w1-fix", prSHA2)
	prTick()
	prTick()
	if k.n("+"+prSHA2+":refs/herdr/pr-of/w1-fix/w1") != 1 {
		t.Fatalf("짝 참조:\n%s", k.logged())
	}
	if k.n("pr create") != 1 || k.n("git/refs/heads/pr/w1 -f sha="+prSHA2) != 0 || k.n("heads/pr/w1-fix") != 0 {
		t.Fatalf("새 PR·머리 옮김 없음:\n%s", k.logged())
	}
	if !strings.Contains(k.bodies(), "제출됨 — 병합기 줄에 넣음") {
		t.Fatalf("제출 댓글이 원래 PR에:\n%s", k.bodies())
	}
}

// gapVerdict: 레인 기계 far의 감사 보고서를 가리키는 판정 줄
func gapVerdict(t *testing.T, id, lane, sha string) {
	t.Helper()
	at := []string{id, lane, "1", lane + "@1", sha, "far", "far/" + lane, "deliver", "0", "pass", "0", "0", "-", "-", "0", "hml=0.0.1",
		"~/.local/share/hc-fill/reports/audit-" + id + ".md"}
	if err := appendLine(AT, strings.Join(at, "\t")); err != nil {
		t.Fatal(err)
	}
}

func gapHc(t *testing.T, k *prT) {
	t.Helper()
	wr(t, k.dir+"/hc", prGapHc)
	if err := os.Chmod(k.dir+"/hc", 0o755); err != nil {
		t.Fatal(err)
	}
}

// ③ 판정을 이미 올린 PR → 다음 바퀴부터 레인 기계 읽기 0번(고치기 전: 바퀴마다 hc exec 한 번)
func TestPRGapsVerdictNoReread(t *testing.T) {
	k := prSetup(t, "pr app")
	gapHc(t, k)
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	gapVerdict(t, "a1", "w1", prSHA1)
	prTick()
	if k.n("hc exec --on far") != 1 || !strings.Contains(k.bodies(), "원격 보고서 본문") || k.n("pr comment 7") != 1 {
		t.Fatalf("첫 판정:\n%s\n%s", k.logged(), k.bodies())
	}
	prTick()
	prTick()
	if k.n("hc exec") != 1 || k.n("pr comment") != 1 {
		t.Fatalf("다시 읽었다:\n%s", k.logged())
	}
}

// ③ 보고서 읽기도 바퀴당 상한에 센다 — 읽고 올릴 자리(2)가 없으면 읽지 않고 다음 바퀴에(읽기만 하고 못 올리는 바퀴를 되풀이하지 않게)
func TestPRGapsVerdictBudget(t *testing.T) {
	k := prSetup(t, "pr app", "pr_max 2")
	gapHc(t, k)
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick() // 커밋 올리기·머리 가지
	prTick() // 열기
	if k.n("pr create") != 1 {
		t.Fatalf("열기:\n%s", k.logged())
	}
	gapVerdict(t, "a1", "w1", prSHA1)
	prTick() // 상태 하나 — 읽고 올릴 자리가 모자라 멈춤
	if k.n("statuses/") != 1 || k.n("hc exec") != 0 {
		t.Fatalf("자리 하나로 읽음:\n%s", k.logged())
	}
	prTick() // 읽기 + 댓글
	prTick()
	if k.n("hc exec") != 1 || k.n("pr comment 7") != 1 {
		t.Fatalf("다음 바퀴:\n%s", k.logged())
	}
}

// ④ 「정리됨」으로 끝난 레인(마지막 보고가 완료 — 제출·착지 없음)의 열린 PR → 닫힘 한 번, 그 뒤 호출 0
// (고치기 전: 닫지 않아 PR이 열린 채 — 판정 보고서를 바퀴마다 다시 읽었다)
func TestPRGapsSweptCloses(t *testing.T) {
	k := prSetup(t, "pr app")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	TERMS = nil
	k.ev("2026-11-24T02:00:00Z", "here", "w1", "정리됨", "-", "hc-sweep: done 30분")
	out := captured(t, prTick)
	if k.n("pr close 7") != 1 || !strings.Contains(k.bodies(), "정리 — 착지 없이 끝났다") || !strings.Contains(out, "PR #7 닫음 w1") {
		t.Fatalf("닫기:\n%s\n%s\n%s", k.logged(), k.bodies(), out)
	}
	if m, _ := prLoad(); m["w1"].State != "closed" {
		t.Fatalf("상태: %+v", m["w1"])
	}
	n := len(k.logged())
	prTick()
	prTick()
	if len(k.logged()) != n {
		t.Fatalf("닫은 뒤 호출:\n%s", k.logged()[n:])
	}
}

// ④ 대조: 제출 뒤 정리(병합기가 착지할 것)·정리 뒤 같은 이름이 다시 보고·이어 쓰는 다시 맞춤 레인이 살아 있음 — 닫지 않는다
func TestPRGapsSweptKeeps(t *testing.T) {
	for _, c := range []string{"제출 뒤", "다시 보고", "이어 씀"} {
		k := prSetup(t, "pr app")
		k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
		prTick()
		TERMS = nil
		switch c {
		case "제출 뒤":
			k.pinSubmit("2026-11-24T01:30:00Z", "w1", prSHA1)
			k.ev("2026-11-24T02:00:00Z", "here", "w1", "정리됨", "-", "hc-sweep")
		case "다시 보고":
			k.ev("2026-11-24T02:00:00Z", "here", "w1", "정리됨", "-", "hc-sweep")
			k.ev("2026-11-24T02:10:00Z", "here", "w1", "중간", "-", "다시 띄움")
		case "이어 씀":
			k.ev("2026-11-24T02:00:00Z", "here", "w1", "정리됨", "-", "hc-sweep")
			k.row("w1-reland") // 살아 있음
		}
		prTick()
		prTick()
		if k.n("pr close") != 0 {
			t.Fatalf("%s: 닫았다:\n%s", c, k.logged())
		}
	}
}

// ④ 감사 1차 중간 1: 원래 레인 w1이 막힘 뒤 정리되고 --pr-of 레인 w1-fix가 제출한 뒤 바로 정리돼도(hc-sweep은 제출한 터미널을 기다리지 않는다)
// 병합기 착지 전에 원래 PR을 닫지 않는다 — 착지 사건이 오면 Merged(고치기 전: 「정리 — 착지 없이 끝났다」로 닫힘)
func TestPRGapsSweptPrOfSubmitted(t *testing.T) {
	k := prSetup(t, "pr app")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	k.ev("2026-11-24T01:10:00Z", "here", "w1", "막힘", "-", "못 풂")
	k.pinRow("w1-fix", "-")
	if err := prOfPut("w1-fix", "w1"); err != nil {
		t.Fatal(err)
	}
	TERMS = nil
	k.ev("2026-11-24T01:40:00Z", "here", "w1", "정리됨", "-", "hc-sweep")
	prTick()
	k.pinSubmit("2026-11-24T02:00:00Z", "w1-fix", prSHA2)
	prTick()
	k.ev("2026-11-24T02:01:00Z", "here", "w1-fix", "정리됨", "-", "hc-sweep 제출 뒤")
	prTick()
	prTick()
	if k.n("pr close") != 0 {
		t.Fatalf("착지 전에 닫았다:\n%s\n%s", k.logged(), k.bodies())
	}
	k.ev("2026-11-24T02:05:00Z", "mg", "w1-fix", "착지", prSHA2, "병합기: 착지 — 제출 x · PR 머리 22222222 들어감")
	prTick()
	if m, _ := prLoad(); m["w1"].State != "merged" || k.n("pr close") != 0 {
		t.Fatalf("Merged 길: %+v\n%s", m["w1"], k.logged())
	}
}

// ④ 같은 PR로 가는 레인이 모두 착지·제출 없이 정리되면(원래 w1 완료 뒤 정리, 이어 쓴 w1-fix도 중간 뒤 정리) 그때 한 번 닫는다
func TestPRGapsSweptFamilyAllDone(t *testing.T) {
	k := prSetup(t, "pr app")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	k.pinRow("w1-fix", "-")
	if err := prOfPut("w1-fix", "w1"); err != nil {
		t.Fatal(err)
	}
	TERMS = nil
	k.ev("2026-11-24T01:40:00Z", "here", "w1", "정리됨", "-", "hc-sweep")
	prTick()
	if k.n("pr close") != 0 {
		t.Fatalf("이어 쓸 줄이 아직 안 끝났는데 닫았다:\n%s", k.logged())
	}
	k.ev("2026-11-24T02:00:00Z", "here", "w1-fix", "중간", "-", "그만둠")
	k.ev("2026-11-24T02:30:00Z", "here", "w1-fix", "정리됨", "-", "hc-sweep")
	prTick()
	prTick()
	if k.n("pr close 7") != 1 {
		t.Fatalf("다 끝났는데 안 닫음:\n%s", k.logged())
	}
}

// ⑥ 운영 장면: 이미 Merged·Closed인 옛 PR들(판정·사건·정리됨이 다 쌓임)이 상태 파일에 있는 채 첫 바퀴 → gh·hc·git 호출 0
func TestPRGapsOldPRsQuiet(t *testing.T) {
	k := prSetup(t, "pr app")
	gapHc(t, k)
	TERMS = nil
	var pr []string
	for i, n := range []string{"o1", "o2", "o3"} {
		st := map[int]string{0: "merged", 1: "closed", 2: "closed"}[i]
		pr = append(pr, prEnt{n, "app", "github.com/example-org/proj", "1" + string(rune('0'+i)), "u", st, "2026-11-20T00:00:00Z", "bot", prSHA1}.line())
		k.ev("2026-11-20T01:00:00Z", "here", n, "완료", prSHA1, "끝")
		k.ev("2026-11-20T02:00:00Z", "mg", n, "착지", prSHA1, "병합기: 착지 · PR 머리 11111111 들어감")
		k.ev("2026-11-20T03:00:00Z", "here", n, "정리됨", "-", "hc-sweep")
		gapVerdict(t, "a"+n, n, prSHA1)
	}
	wr(t, DATA+"/pr.tsv", strings.Join(pr, "\n")+"\n")
	prTick()
	if l := k.logged(); l != "" {
		t.Fatalf("옛 PR에 호출:\n%s", l)
	}
}
