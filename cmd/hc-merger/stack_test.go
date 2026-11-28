//go:build !windows

package main

// 쌓인 제출(stack.go — docs/hc-fill/stack.md 「병합기 규칙 넷」): 선행보다 먼저 드는 길이 없고(④), 선행이 반송·거둠이면 뒤는 주차(⑤⑪),
// 선행이 다시 쓰인 sha로 들면 뒤는 자기 커밋만 얹어 승인 그대로 든다(⑥), 꼴이 틀리면 반송. 칸 없는 제출은 지금 그대로(⑧⑫ — 기존 시험 전부와
// 아래 섞임). 틀은 batch_audited_test.go(batchFx — 진짜 land처럼 승인 대조·rebase·push하는 가짜 hc-lane land).

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// stackSub: 선행 제출 P 위에 파일 하나를 얹은 쌓인 제출 — 제출 기록 stack=<선행>@<P>·fork=P, 판정과 승인 patch-id(P..제출).
// st가 빈 값이 아니면 stack= 칸을 그 값으로(틀린 꼴 흉내)
func (f *fx) stackSub(sec int, lane, pred, P, prio, st string) (q, sha string) {
	f.git(f.seed, "checkout", "-q", "-B", lane, P)
	os.WriteFile(f.seed+"/"+lane+".txt", []byte(lane+"\n"), 0o666)
	f.git(f.seed, "add", "-A")
	f.git(f.seed, "commit", "-qm", lane)
	sha = f.git(f.seed, "rev-parse", "HEAD")
	if st == "" {
		st = pred + "@" + P
	}
	et := f.git(f.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
	meta := f.git(f.seed, "commit-tree", et, "-p", sha, "-m",
		fmt.Sprintf("merge-queue lane=%s id=box-%s orig=%s fork=%s base=main host=box term=- owner=- job=- created_ms=- priority=%s stack=%s verdict=v-%s patch=%s",
			lane, lane, sha, P, prio, st, lane, f.patchOf(P, sha)))
	name := fmt.Sprintf("%d-%s", sec, lane)
	q = "refs/merge-queue/" + name
	f.git(f.seed, "push", "-q", "origin", sha+":"+q, meta+":refs/merge-queue-meta/"+name, sha+":refs/heads/lane/"+lane)
	f.git(f.seed, "checkout", "-q", "main")
	return
}

// order: 큐 차례(레인 이름) — 병합기가 바퀴 처음에 읽는 것과 같게
func (f *fx) order(m *M) string {
	f.t.Helper()
	m.qs = f.git(f.clone, "ls-remote", "origin", "refs/heads/main", "refs/merge-queue/*", "refs/merge-queue-meta/*", "refs/merge-done/*")
	f.git(f.clone, "fetch", "-q", "origin", "+refs/merge-queue-meta/*:refs/hc-merger/meta/*")
	q, ok := m.queue()
	if !ok {
		f.t.Fatal("queue 실패")
	}
	var o []string
	for _, e := range q {
		o = append(o, qLane(strings.TrimPrefix(e[0], "refs/merge-queue/")))
	}
	return strings.Join(o, ",")
}

func (f *fx) log(n int) string {
	return f.git(f.seed, "--git-dir="+f.origin, "log", fmt.Sprintf("-%d", n), "--format=%s", "main")
}

// ④ 뒤가 이름(초)·우선(긴급) 모두 앞서도 차례는 선행 → 뒤, 같은 묶음에 앞 → 뒤로 들고, 칸 없는 옛 제출(c)은 그대로
func TestStackOrderSameBatch(t *testing.T) {
	f, landlog := batchFx(t)
	_, P := f.sub2(1000000005, "a", "v-a", "", "a.txt", "a\n", false)
	f.stackSub(1000000001, "b", "a", P, "긴급", "")
	f.sub2(1000000003, "c", "", "", "c.txt", "c\n", false)
	if got := f.order(f.merger()); got != "c,a,b" {
		t.Fatalf("쌓인 b는 선행 a 바로 뒤: %s", got)
	}
	f.merger().round()
	if got := landLines(t, landlog); len(got) != 1 || got[0] != "land|||c" {
		t.Fatalf("한 번의 묶음 착지: %v", got)
	}
	if got := f.log(3); got != "b\na\nc" {
		t.Fatalf("main에 c → a → b 차례로: %q", got)
	}
	jr, _ := os.ReadFile(gEvdir + "/merger-journal.tsv")
	if !strings.Contains(string(jr), "쌓임 a") {
		t.Fatalf("일지 까닭에 「쌓임 a」: %s", jr)
	}
}

// ④ 묶음 크기 1(cap)이어도 선행 먼저, 뒤는 선행이 든 다음 바퀴에(merge-done 원본 = P로 듦을 안다)
func TestStackCapOneFrontFirst(t *testing.T) {
	f, landlog := batchFx(t)
	_, P := f.sub2(1000000005, "a", "v-a", "", "a.txt", "a\n", false)
	qb, _ := f.stackSub(1000000001, "b", "a", P, "긴급", "")
	m := f.merger()
	m.writeCap(1)
	m.round()
	if got := landLines(t, landlog); len(got) != 1 || !strings.HasPrefix(got[0], "land|v-a|") || !strings.HasSuffix(got[0], "|a") {
		t.Fatalf("크기 1이면 선행 a 혼자 먼저(승인 착지): %v", got)
	}
	if !f.has(qb) || f.onMain("b.txt") {
		t.Fatal("뒤 b는 줄에 그대로")
	}
	f.merger().round()
	if !f.onMain("b.txt") || f.has(qb) {
		t.Fatalf("선행이 든 다음 바퀴에 b가 든다: %v", landLines(t, landlog))
	}
}

// ④ 지휘가 선행을 거둔 뒤 다시 넣어(새 이름 — 뒤보다 늦음) 둘 다 줄에 있으면 여전히 선행 → 뒤
func TestStackWithdrawRequeueOrder(t *testing.T) {
	f, _ := batchFx(t)
	qa, P := f.sub2(1000000001, "a", "v-a", "", "a.txt", "a\n", false)
	f.stackSub(1000000002, "b", "a", P, "보통", "")
	rec := f.git(f.seed, "--git-dir="+f.origin, "rev-parse", "refs/merge-queue-meta/1000000001-a")
	f.git(f.seed, "push", "-q", "--atomic", "origin", ":"+qa, ":refs/merge-queue-meta/1000000001-a",
		P+":refs/merge-queue/1000000009-a", rec+":refs/merge-queue-meta/1000000009-a")
	if got := f.order(f.merger()); got != "a,b" {
		t.Fatalf("다시 넣은 선행이 뒤보다 늦은 이름이어도 a → b: %s", got)
	}
}

// ④ 선행이 반송을 다시 하는 중(fail/<이름>)이라 pickBatch가 건너뛰면 쌓인 뒤도 건너뛴다 — 뒤만 든 묶음 없음
func TestStackPickSkipsOrphan(t *testing.T) {
	f, _ := batchFx(t)
	_, P := f.sub2(1000000001, "a", "v-a", "", "a.txt", "a\n", false)
	f.stackSub(1000000002, "b", "a", P, "보통", "")
	f.sub2(1000000003, "c", "", "", "c.txt", "c\n", false)
	m := f.merger()
	os.WriteFile(m.mj+"/fail/1000000001-a", []byte("x\n"), 0o666)
	f.order(m)
	if !m.pickBatch() || len(m.b) != 1 || m.b[0].l != "c" {
		t.Fatalf("선행을 건너뛰면 뒤도: %v", m.b)
	}
}

// ④ 묶음을 줄이거나(keep — 승인 다름·충돌로 뺌) 추측 길에서 선행 무리가 실패해 건너뛰면 쌓인 뒤도 빠진다
func TestStackKeepAndSpecOrphan(t *testing.T) {
	m := &M{}
	m.stk = map[string]stackRec{"bb": {pred: "a", p: "aa"}, "dd": {pred: "x", p: "xx", done: true}}
	m.b = []entry{{s: "cc", l: "c"}, {s: "aa", l: "a"}, {s: "bb", l: "b"}, {s: "dd", l: "d"}}
	m.keep(0, 2, 3)
	if got := fmt.Sprint(shas(m.b)); got != "[cc dd]" {
		t.Fatalf("선행 a가 빠지면 b도 빠지고 선행이 든 d는 남는다: %s", got)
	}
	m.b = []entry{{s: "bb", l: "b"}}
	if m.keep(0); len(m.b) != 1 {
		t.Fatal("맨 앞은 늘 남긴다")
	}
	pass, fail := &specRow{state: specPass}, &specRow{state: specFail}
	st := []specStep{{g: specGroup{shas: []string{"cc"}}, row: pass}, {g: specGroup{shas: []string{"aa"}}, row: fail}, {g: specGroup{shas: []string{"bb"}}}}
	if !m.stackOrphan(st) {
		t.Fatal("선행 무리가 실패해 건너뛰면 뒤 무리는 짓지 않는다")
	}
	st[1].row = pass
	if m.stackOrphan(st) {
		t.Fatal("선행이 길에 있으면 짓는다")
	}
	st = []specStep{{g: specGroup{shas: []string{"cc"}}, row: pass}, {g: specGroup{shas: []string{"aa", "bb"}}}}
	if m.stackOrphan(st) {
		t.Fatal("같은 무리에서 선행 뒤면 짓는다")
	}
}

// parked: 주차 참조·사건(까닭 why)·일지 — 실패(merge-failed·막힘·돌려보냄)가 없어야
func (f *fx) parked(name, lane, pred, why string) {
	f.t.Helper()
	if f.has("refs/merge-queue/"+name) || !f.has("refs/merge-parked/"+name) || !f.has("refs/merge-parked-meta/"+name) || f.has("refs/merge-queue-meta/"+name) {
		f.t.Fatal("쌓인 제출은 큐·기록에서 merge-parked로")
	}
	if f.has("refs/merge-failed/" + name) {
		f.t.Fatal("주차는 반송이 아니다")
	}
	ev, _ := os.ReadFile(f.events)
	if !strings.Contains(string(ev), "report|중간|-|병합기: 주차("+why+") "+pred+" — "+lane+" 제출 ") || strings.Contains(string(ev), "report|막힘|-|병합기: 주차") {
		f.t.Fatalf("중간 사건 「주차(%s)」: %s", why, ev)
	}
	jr, _ := os.ReadFile(gEvdir + "/merger-journal.tsv")
	n := 0
	for _, l := range strings.Split(string(jr), "\n") {
		if c := strings.Split(l, "\t"); len(c) == 12 && c[2] == lane {
			if c[9] == "돌려보냄" || c[9] == "착지" {
				f.t.Fatalf("뒤 %s 는 반송·착지가 아니다: %s", lane, l)
			}
			if c[9] == "주차" {
				n++
			}
		}
	}
	if n != 1 {
		f.t.Fatalf("주차 일지 한 줄: %s", jr)
	}
}

// ⑤ 선행이 묶음째 실패 → 혼자 실패로 반송되면 뒤는 범인·반송이 아니라 주차
func TestStackPredRejectedParks(t *testing.T) {
	f, _ := batchFx(t)
	_, P := f.sub2(1000000001, "a", "v-a", "", "bad.txt", "bad\n", false)
	f.stackSub(1000000002, "b", "a", P, "보통", "")
	for i := 0; i < 3; i++ {
		f.merger().round()
	}
	if !f.has("refs/merge-failed/1000000001-a") {
		t.Fatal("선행 a는 반송")
	}
	f.parked("1000000002-b", "b", "a", "선행 빠짐")
}

// ht-smalls-1129: 주차 글은 바퀴에 읽어 둔 원격(m.qs)으로 사실대로 — 선행이 새 커밋으로 다시 제출됨(큐에 같은 레인, 다른 커밋) ·
// 선행이 다른 커밋으로 착지함(merge-done 원본이 P가 아님). 고치기 전은 둘 다 「선행 반송」. 주차·다시 쌓기 동작은 같다
func TestStackParkWhy(t *testing.T) {
	t.Run("다시 제출됨", func(t *testing.T) {
		f, _ := batchFx(t)
		qa, P := f.sub2(1000000001, "a", "v-a", "", "a.txt", "a\n", false)
		f.stackSub(1000000002, "b", "a", P, "보통", "")
		rec := f.git(f.seed, "--git-dir="+f.origin, "rev-parse", "refs/merge-queue-meta/1000000001-a")
		f.git(f.seed, "push", "-q", "--atomic", "origin", P+":refs/merge-failed/1000000001-a", rec+":refs/merge-failed-meta/1000000001-a", ":"+qa, ":refs/merge-queue-meta/1000000001-a", ":refs/heads/lane/a")
		f.sub2(1000000003, "a", "v-a2", "", "a.txt", "a2\n", false)
		f.merger().round()
		f.parked("1000000002-b", "b", "a", "선행이 새 커밋으로 다시 제출됨")
	})
	t.Run("다른 커밋으로 착지함", func(t *testing.T) {
		f, _ := batchFx(t)
		qa, P := f.sub2(1000000001, "a", "v-a", "", "a.txt", "a\n", false)
		f.stackSub(1000000002, "b", "a", P, "보통", "")
		os.WriteFile(f.seed+"/a.txt", []byte("a2\n"), 0o666)
		f.git(f.seed, "add", "-A")
		f.git(f.seed, "commit", "-qm", "a (다시 제출한 새 커밋)")
		Q := f.git(f.seed, "rev-parse", "HEAD")
		et := f.git(f.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
		done := f.git(f.seed, "commit-tree", et, "-p", Q, "-m", "merge-queue lane=a")
		f.git(f.seed, "push", "-q", "--atomic", "origin", "main", ":"+qa, ":refs/merge-queue-meta/1000000001-a", done+":refs/merge-done/a")
		f.merger().round()
		f.parked("1000000002-b", "b", "a", "선행이 다른 커밋 "+Q[:8]+"로 착지함")
	})
}

// ⑪ 지휘가 선행을 거둠(withdraw) → 다음 바퀴에 뒤 주차, 같은 바퀴를 다시 돌아도 사건은 하나
func TestStackPredWithdrawnParks(t *testing.T) {
	f, landlog := batchFx(t)
	qa, P := f.sub2(1000000001, "a", "v-a", "", "a.txt", "a\n", false)
	f.stackSub(1000000002, "b", "a", P, "보통", "")
	rec := f.git(f.seed, "--git-dir="+f.origin, "rev-parse", "refs/merge-queue-meta/1000000001-a")
	f.git(f.seed, "push", "-q", "--atomic", "origin", P+":refs/merge-failed/1000000001-a", rec+":refs/merge-failed-meta/1000000001-a", ":"+qa, ":refs/merge-queue-meta/1000000001-a")
	f.merger().round()
	f.merger().round()
	f.parked("1000000002-b", "b", "a", "선행 빠짐")
	if len(landLines(t, landlog)) != 0 {
		t.Fatal("착지하지 않는다")
	}
	ev, _ := os.ReadFile(f.events)
	if strings.Count(string(ev), "주차(선행 빠짐)") != 1 {
		t.Fatalf("주차 사건은 하나: %s", ev)
	}
}

// ⑥ 선행이 다시 쓰인 sha로 듦(merge-done 원본 = P, main 커밋은 다른 sha) → 뒤는 P 뒤 자기 커밋만 얹혀 승인 그대로 든다(판정 대신 main lease)
func TestStackPredRewrittenLands(t *testing.T) {
	f, landlog := batchFx(t)
	qa, P := f.sub2(1000000001, "a", "v-a", "", "a.txt", "a\n", false)
	qb, S := f.stackSub(1000000002, "b", "a", P, "보통", "")
	rec := f.git(f.seed, "--git-dir="+f.origin, "rev-parse", "refs/merge-queue-meta/1000000001-a")
	os.WriteFile(f.seed+"/a.txt", []byte("a\n"), 0o666)
	f.git(f.seed, "add", "-A")
	f.git(f.seed, "commit", "-qm", "a (착지 스크립트가 다시 씀)")
	f.git(f.seed, "push", "-q", "--atomic", "origin", "main", ":"+qa, ":refs/merge-queue-meta/1000000001-a", rec+":refs/merge-done/a")
	f.merger().round()
	if got := landLines(t, landlog); len(got) != 1 || got[0] != "land|||b" {
		t.Fatalf("판정을 넘기지 않고 착지 한 번: %v", got)
	}
	if !f.onMain("b.txt") || f.has(qb) || f.has("refs/merge-parked/1000000002-b") {
		t.Fatal("b가 들어야")
	}
	if got := f.log(3); got != "b\na (착지 스크립트가 다시 씀)\nbase" {
		t.Fatalf("선행 커밋을 다시 얹지 않는다: %q", got)
	}
	f.git(f.seed, "fetch", "-q", "origin")
	if got := f.patchOf("origin/main~1", "origin/main"); got != f.patchOf(P, S) {
		t.Fatalf("든 변경 = 승인 patch-id(P..S): %s", got)
	}
}

// 꼴이 틀린 stack= 칸·P가 제출의 조상이 아님 → 그 제출만 반송(「쌓임 기록 꼴」), 줄의 다른 제출은 그대로
func TestStackBadFormRejected(t *testing.T) {
	f, _ := batchFx(t)
	_, P := f.sub2(1000000001, "a", "v-a", "", "a.txt", "a\n", false)
	f.stackSub(1000000002, "b", "a", P, "보통", "A@nope")
	_, X := f.sub2(1000000003, "x", "", "", "x.txt", "x\n", false)
	f.stackSub(1000000004, "d", "x", P, "보통", "x@"+X)
	f.order(f.merger())
	for _, n := range []string{"1000000002-b", "1000000004-d"} {
		if !f.has("refs/merge-failed/"+n) || f.has("refs/merge-queue/"+n) {
			t.Fatalf("%s 반송", n)
		}
	}
	ev, _ := os.ReadFile(f.events)
	if !strings.Contains(string(ev), "쌓임 기록 꼴(stack=A@nope)") || !strings.Contains(string(ev), "가 제출의 조상이 아님") {
		t.Fatalf("반송 까닭: %s", ev)
	}
	if !f.has("refs/merge-queue/1000000001-a") || !f.has("refs/merge-queue/1000000003-x") {
		t.Fatal("다른 제출은 그대로")
	}
}
