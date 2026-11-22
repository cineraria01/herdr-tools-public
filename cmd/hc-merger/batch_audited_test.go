//go:build !windows

package main

// 감사 판정이 붙은 제출도 묶음에 담는다(merger-batch-audited, 2026-11-22): 구성원마다 자기 승인 patch-id를 확인해야만 묶음에 들고,
// 다르면 그 구성원만 빠져 혼자 차례에 착지가 지금처럼 판정을 본다. 가짜 hc-lane land는 진짜 land처럼 승인 착지(HERDR_LAND_PATCHID)면
// HEAD가 제출 SHA인지·갈래점 뒤 patch-id가 승인과 같은지 보고, 그다음 rebase → push(함께 실을 인자 포함)한다.

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const fakeLand = `if [ "$1" = land ]; then
  h=$(git rev-parse HEAD)
  printf 'land|%s|%s|%s\n' "$HERDR_LAND_VERDICT" "$HERDR_LAND_PATCHID" "$HC_LANE" >>"$LANDLOG"
  if [ -n "$HERDR_LAND_PATCHID" ]; then
    [ "$h" = "$HERDR_LAND_SHA" ] || exit 2
    mb=$(git merge-base HEAD origin/main) || exit 1
    p=$(git --literal-pathspecs diff -U0 --binary "$mb" HEAD -- | git patch-id --stable | cut -d' ' -f1)
    [ "$p" = "$HERDR_LAND_PATCHID" ] || exit 2
  fi
  git fetch -q origin || exit 1
  git rebase -q origin/main || exit 3
  [ ! -e bad.txt ] || exit 1
  git push -q --atomic origin HEAD:refs/heads/main $HERDR_LAND_PUSH_EXTRA || exit 1
  exit 0
fi
`

// batchFx: 켬 표식과 착지까지 하는 가짜 hc-lane
func batchFx(t *testing.T) (*fx, string) {
	f := newFx(t)
	f.git(f.seed, "push", "-q", "origin", "main:refs/herdr/merger/box")
	landlog := f.dir + "/land.log"
	t.Setenv("LANDLOG", landlog)
	old, err := os.ReadFile(gSelf)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Replace(string(old), "#!/bin/sh\n", "#!/bin/sh\n"+fakeLand, 1)
	if err := os.WriteFile(gSelf, []byte(body), 0o777); err != nil {
		t.Fatal(err)
	}
	return f, landlog
}

func (f *fx) patchOf(a, b string) string {
	f.t.Helper()
	c := exec.Command("sh", "-c", `git --literal-pathspecs diff -U0 --binary "$1" "$2" -- | git patch-id --stable | cut -d' ' -f1`, "-", a, b)
	c.Dir = f.seed
	o, err := c.Output()
	if err != nil {
		f.t.Fatal(err)
	}
	return strings.TrimSpace(string(o))
}

// sub2: main 위에서 레인 가지에 파일 file=body 커밋(merge면 곁가지를 병합한 커밋)을 낸다. verdict가 있으면 승인 patch-id를 싣는다
// (patch가 빈 값이면 진짜 값, 아니면 그 값 — 틀린 승인 흉내)
func (f *fx) sub2(sec int, lane, verdict, patch, file, body string, merge bool) (q, sha string) {
	f.git(f.seed, "checkout", "-q", "-B", lane, "main")
	if merge {
		f.git(f.seed, "checkout", "-q", "-B", lane+"-side", "main")
		os.WriteFile(f.seed+"/"+lane+"-side.txt", []byte("side\n"), 0o666)
		f.git(f.seed, "add", "-A")
		f.git(f.seed, "commit", "-qm", lane+" side")
		f.git(f.seed, "checkout", "-q", lane)
	}
	os.WriteFile(f.seed+"/"+file, []byte(body), 0o666)
	f.git(f.seed, "add", "-A")
	f.git(f.seed, "commit", "-qm", lane)
	if merge {
		f.git(f.seed, "merge", "-q", "--no-ff", "-m", "merge side", lane+"-side")
	}
	sha = f.git(f.seed, "rev-parse", "HEAD")
	fork := f.git(f.seed, "merge-base", "HEAD", "main")
	vx := ""
	if verdict != "" {
		if patch == "" {
			patch = f.patchOf(fork, sha)
		}
		vx = " verdict=" + verdict + " patch=" + patch
	}
	et := f.git(f.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
	meta := f.git(f.seed, "commit-tree", et, "-p", sha, "-m",
		fmt.Sprintf("merge-queue lane=%s id=box-%s orig=%s fork=%s base=main host=box term=- owner=- job=- created_ms=- priority=보통%s", lane, lane, sha, fork, vx))
	name := fmt.Sprintf("%d-%s", sec, lane)
	q = "refs/merge-queue/" + name
	f.git(f.seed, "push", "-q", "origin", sha+":"+q, meta+":refs/merge-queue-meta/"+name, sha+":refs/heads/lane/"+lane)
	f.git(f.seed, "checkout", "-q", "main")
	return
}

func landLines(t *testing.T, p string) []string {
	b, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return strings.Fields(string(b))
}

func (f *fx) onMain(file string) bool {
	return exec.Command("git", "--git-dir="+f.origin, "cat-file", "-e", "main:"+file).Run() == nil
}

// 틀린 승인 patch-id(40자 hex — 비밀 꼴 검사에 걸리지 않게 실행 중에 만든다)
var badPatch = strings.Repeat("0123456789abcdef", 2) + "01234567"

// 판정 둘: 한 번의 착지(묶음 2)로 들어가고, 착지에는 묶음 전체 판정을 넘기지 않으며, 병합기 일지에 구성원별 승인 확인이 남는다
func TestBatchAuditedTwoLand(t *testing.T) {
	f, landlog := batchFx(t)
	qa, _ := f.sub2(1000000001, "a1", "v-a1", "", "a1.txt", "a1\n", false)
	qb, _ := f.sub2(1000000002, "a2", "v-a2", "", "a2.txt", "a2\n", false)
	m := f.merger()
	m.round()
	got := landLines(t, landlog)
	if len(got) != 1 || got[0] != "land|||a1" {
		t.Fatalf("묶음 2를 판정 없이 착지 한 번: %v", got)
	}
	if !f.onMain("a1.txt") || !f.onMain("a2.txt") || f.has(qa) || f.has(qb) {
		t.Fatal("둘 다 main에 들고 큐가 비어야")
	}
	jr, _ := os.ReadFile(gEvdir + "/merger-journal.tsv")
	for _, l := range []string{"a1", "a2"} {
		ok := false
		for _, line := range strings.Split(string(jr), "\n") {
			c := strings.Split(line, "\t")
			if len(c) == 12 && c[2] == l && c[3] == "box-"+l && c[6] == "2" && c[9] == "착지" && strings.Contains(c[11], "승인 확인 판정 v-"+l+" patch-id ") {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("%s 일지 줄에 묶음 2·승인 확인이 있어야:\n%s", l, jr)
		}
	}
}

// 한 구성원의 patch-id가 승인과 다르면 그것만 빠지고(큐에 그대로) 나머지가 묶음으로, 빠진 것은 혼자 차례에 착지가 판정을 보고 반송
func TestBatchAuditedMismatchDropped(t *testing.T) {
	f, landlog := batchFx(t)
	f.sub2(1000000001, "b1", "v-b1", "", "b1.txt", "b1\n", false)
	qb2, _ := f.sub2(1000000002, "b2", "v-b2", badPatch, "b2.txt", "b2\n", false)
	f.sub2(1000000003, "b3", "v-b3", "", "b3.txt", "b3\n", false)
	m := f.merger()
	m.round()
	if got := landLines(t, landlog); len(got) != 1 || got[0] != "land|||b1" {
		t.Fatalf("b1+b3 묶음 착지 한 번: %v", got)
	}
	if !f.onMain("b1.txt") || !f.onMain("b3.txt") || f.onMain("b2.txt") || !f.has(qb2) {
		t.Fatal("b2만 빠져 큐에 남아야")
	}
	m.round()
	got := landLines(t, landlog)
	if len(got) != 2 || got[1] != "land|v-b2|"+badPatch+"|b2" {
		t.Fatalf("b2는 혼자 자기 판정으로 착지를 불러야: %v", got)
	}
	if f.onMain("b2.txt") || f.has(qb2) || !f.has("refs/merge-failed/1000000002-b2") {
		t.Fatal("승인과 다른 b2는 반송돼야")
	}
}

// 맨 앞의 승인이 다르면 맨 앞이 혼자(지금처럼 반송), 뒤는 다음 바퀴에
func TestBatchAuditedHeadMismatchSolo(t *testing.T) {
	f, landlog := batchFx(t)
	f.sub2(1000000001, "h1", "v-h1", badPatch, "h1.txt", "h1\n", false)
	f.sub2(1000000002, "h2", "v-h2", "", "h2.txt", "h2\n", false)
	m := f.merger()
	m.round()
	if got := landLines(t, landlog); len(got) != 1 || got[0] != "land|v-h1|"+badPatch+"|h1" {
		t.Fatalf("h1 혼자 자기 판정으로: %v", got)
	}
	if !f.has("refs/merge-failed/1000000001-h1") || f.onMain("h2.txt") {
		t.Fatal("h1만 반송, h2는 아직")
	}
	m.round()
	if !f.onMain("h2.txt") {
		t.Fatal("h2는 다음 바퀴에 들어가야")
	}
}

// 판정 있음 + 없음이 섞여도 한 묶음
func TestBatchAuditedMixed(t *testing.T) {
	f, landlog := batchFx(t)
	f.sub2(1000000001, "x1", "", "", "x1.txt", "x1\n", false)
	f.sub2(1000000002, "x2", "v-x2", "", "x2.txt", "x2\n", false)
	f.sub2(1000000003, "x3", "", "", "x3.txt", "x3\n", false)
	m := f.merger()
	m.round()
	if got := landLines(t, landlog); len(got) != 1 || got[0] != "land|||x1" {
		t.Fatalf("셋이 착지 한 번: %v", got)
	}
	for _, x := range []string{"x1.txt", "x2.txt", "x3.txt"} {
		if !f.onMain(x) {
			t.Fatalf("%s 가 main에 있어야", x)
		}
	}
}

// 병합 커밋이 있는 제출은 혼자(판정과 함께 원본 그대로), 판정 제출 묶음은 그 뒤
func TestBatchAuditedMergeCommitSolo(t *testing.T) {
	f, landlog := batchFx(t)
	_, sha := f.sub2(1000000001, "g1", "v-g1", "", "g1.txt", "g1\n", true)
	f.sub2(1000000002, "g2", "v-g2", "", "g2.txt", "g2\n", false)
	f.sub2(1000000003, "g3", "v-g3", "", "g3.txt", "g3\n", false)
	m := f.merger()
	m.round()
	got := landLines(t, landlog)
	if len(got) != 1 || !strings.HasPrefix(got[0], "land|v-g1|") || !strings.HasSuffix(got[0], "|g1") {
		t.Fatalf("병합 커밋 g1은 혼자 자기 판정으로: %v", got)
	}
	if f.git(f.clone, "ls-remote", "origin", "refs/hc-landed/box-g1") == "" || !f.onMain("g1-side.txt") || f.onMain("g2.txt") {
		t.Fatalf("g1만 먼저 들어가야(원본 %s)", first8(sha))
	}
	m.round()
	if got := landLines(t, landlog); len(got) != 2 || got[1] != "land|||g2" || !f.onMain("g2.txt") || !f.onMain("g3.txt") {
		t.Fatalf("g2+g3 묶음: %v", got)
	}
}

// 앞 구성원과 충돌해 하나만 남으면 원본 제출 그대로 혼자 승인 착지(HEAD = 제출 SHA)
func TestBatchAuditedConflictLeavesOneSolo(t *testing.T) {
	f, landlog := batchFx(t)
	f.sub2(1000000001, "k1", "v-k1", "", "same.txt", "k1\n", false)
	qk2, _ := f.sub2(1000000002, "k2", "v-k2", "", "same.txt", "k2\n", false)
	// main이 앞서 있으면 묶음 쌓기의 첫 rebase가 k1의 SHA를 바꾼다 — 혼자 남으면 원본으로 되돌려야 승인 착지가 된다
	os.WriteFile(f.seed+"/other.txt", []byte("other\n"), 0o666)
	f.git(f.seed, "add", "-A")
	f.git(f.seed, "commit", "-qm", "other")
	f.git(f.seed, "push", "-q", "origin", "main")
	m := f.merger()
	m.round()
	if got := landLines(t, landlog); len(got) != 1 || !strings.HasPrefix(got[0], "land|v-k1|") {
		t.Fatalf("k1 혼자 자기 판정으로: %v", got)
	}
	if !f.onMain("same.txt") || !f.has(qk2) || f.has("refs/merge-failed/1000000001-k1") {
		t.Fatal("k1은 들어가고 k2는 큐에 그대로")
	}
}

// 판정 묶음이 실패하면 마지막을 빼고 다시(cap) — 실패 제출만 혼자일 때 반송
func TestBatchAuditedFailShrinks(t *testing.T) {
	f, landlog := batchFx(t)
	f.sub2(1000000001, "f1", "v-f1", "", "f1.txt", "f1\n", false)
	f.sub2(1000000002, "f2", "v-f2", "", "f2.txt", "f2\n", false)
	f.sub2(1000000003, "f3", "v-f3", "", "bad.txt", "bad\n", false)
	m := f.merger()
	m.round()
	if c, _ := os.ReadFile(m.mj + "/cap"); strings.TrimSpace(string(c)) != "2" {
		t.Fatalf("묶음 3 실패 뒤 cap 2: %q", c)
	}
	m.round()
	if !f.onMain("f1.txt") || !f.onMain("f2.txt") || f.onMain("bad.txt") {
		t.Fatal("f1+f2가 묶음 2로 들어가야")
	}
	m.round()
	got := landLines(t, landlog)
	if len(got) != 3 || got[0] != "land|||f1" || got[1] != "land|||f1" || !strings.HasPrefix(got[2], "land|v-f3|") {
		t.Fatalf("셋 → 둘 → f3 혼자(자기 판정): %v", got)
	}
	if !f.has("refs/merge-failed/1000000003-f3") {
		t.Fatal("f3은 혼자일 때 반송")
	}
}
