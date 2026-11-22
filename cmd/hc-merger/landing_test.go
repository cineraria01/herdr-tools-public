package main

import (
	"strings"
	"testing"
)

// event-owner-merger: 병합기가 띄운 착지(hc-lane land)가 내는 중간 사건(「land 실패」)의 주인 = 제출 기록의 지휘.
// 기록에 지휘가 없거나 묶음 구성원의 지휘가 갈리면 빈 값(사건 주인 '-') — 병합기 자신의 환경 값은 물려주지 않는다
func TestOwnerEnv(t *testing.T) {
	m := &M{}
	for _, c := range []struct {
		b    []entry
		want string
	}{
		{[]entry{{ow: "t1cj", owr: "1793568278378"}}, "HC_LANE_OWNER=t1cj HC_LANE_OWNER_RUN=1793568278378"},
		{[]entry{{}}, "HC_LANE_OWNER= HC_LANE_OWNER_RUN="},
		{[]entry{{ow: "t1cj", owr: "1"}, {ow: "t1cj", owr: "1"}}, "HC_LANE_OWNER=t1cj HC_LANE_OWNER_RUN=1"},
		{[]entry{{ow: "t1cj", owr: "1"}, {ow: "t1cj", owr: "2"}}, "HC_LANE_OWNER=t1cj HC_LANE_OWNER_RUN="},
		{[]entry{{ow: "t1cj", owr: "1"}, {ow: "t44p", owr: "1"}}, "HC_LANE_OWNER= HC_LANE_OWNER_RUN="},
		{[]entry{{ow: "t1cj"}, {}}, "HC_LANE_OWNER= HC_LANE_OWNER_RUN="},
	} {
		m.b = c.b
		if got := strings.Join(m.ownerEnv(), " "); got != c.want {
			t.Errorf("%v: %q, 바람 %q", c.b, got, c.want)
		}
	}
}

// 제출 기록 첫 줄의 owner·owner_run을 읽는다(owner=- 는 없음)
func TestReadMetaOwner(t *testing.T) {
	f := newFx(t)
	_, sha, _, _ := f.submit(1000000001, "a", "")
	f.git(f.clone, "fetch", "-q", "origin", "+refs/heads/lane/*:refs/hc-merger/l/*")
	et := f.git(f.clone, "hash-object", "-t", "tree", "-w", "/dev/null")
	for _, c := range [][3]string{{"owner=t1cj owner_run=17", "t1cj", "17"}, {"owner=-", "", ""}, {"", "", ""}} {
		rec := f.git(f.clone, "commit-tree", et, "-p", sha, "-m", "merge-queue lane=a id=box-a "+c[0])
		mt, ok := readMeta(rec, sha)
		if !ok || mt.owner != c[1] || mt.ownerRun != c[2] {
			t.Errorf("%q: owner %q run %q ok %v", c[0], mt.owner, mt.ownerRun, ok)
		}
	}
}

// 착지 직전 묶음 제출마다 원격 refs/herdr/landing/<큐 이름> = 제출 SHA, 다음 바퀴 시작에 남은 표식을 지운다
func TestLandMarkUnmark(t *testing.T) {
	f := newFx(t)
	q1, s1, _, _ := f.submit(1000000001, "a", "")
	q2, s2, _, _ := f.submit(1000000002, "b", "")
	f.git(f.clone, "fetch", "-q", "origin", "+refs/merge-queue/*:refs/hc-merger/q/*")
	m := f.merger()
	m.b = []entry{{q: q1, s: s1, l: "a"}, {q: q2, s: s2, l: "b"}}
	m.landMark()
	for name, sha := range map[string]string{"refs/herdr/landing/1000000001-a": s1, "refs/herdr/landing/1000000002-b": s2} {
		if got := f.git(f.clone, "--git-dir="+f.origin, "rev-parse", name); got != sha {
			t.Fatalf("표식 %s = %q, 바람 %s", name, got, sha)
		}
	}
	m.qs = f.git(f.clone, "ls-remote", "origin", "refs/merge-queue/*", "refs/herdr/landing/*")
	m.landUnmark()
	if f.has("refs/herdr/landing/1000000001-a") || f.has("refs/herdr/landing/1000000002-b") {
		t.Fatal("남은 표식을 지우지 않았다")
	}
	if !f.has(q1) || !f.has(q2) {
		t.Fatal("큐 표식은 그대로여야")
	}
	m.qs = ""
	m.landUnmark() // 표식이 없으면 push하지 않는다(실패 글 없음)
}
