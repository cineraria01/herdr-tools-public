package events

// ops-events(2026-11-24): 사건 종류 — 옛 꼴 줄은 이름 표로(②), 이름 표 = 문서 목록 = 코드가 내는 이름(⑥)

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestKindOf(t *testing.T) {
	ln := func(lane, key string, extra ...string) []string {
		f := []string{"T", "m", "-", lane, "중간", "-", "글", "-", "-", "-", "-", key}
		return append(f, extra...)
	}
	cases := []struct {
		f    []string
		want string
	}{
		{ln("hc-fill-idle", "-"), KindOps},
		{ln("hc-fill", "hf:orphans:1:2"), KindOps},
		{ln("merger", "-"), KindOps},
		{ln("hc-disk-low", "hc-disk-low:2026"), KindOps},
		{ln("hc-tailscale", "hc-tailscale:off:m:2026"), KindOps},
		{ln("on-land-app", "-"), KindOps},
		{ln("merger-proj", "hf:merger:proj:dead:1:-"), KindOps},
		{ln("merger-alive", "-"), KindLane}, // 진짜 레인 — 이름만 merger-
		{ln("merger-alive", "hf:a1:noack"), KindLane},
		{ln("on-land-", "-"), KindLane},
		{ln("ops-events", "-"), KindLane},
		{[]string{"T", "m", "-", "hc-fill-idle", "중간", "-", "글"}, KindOps}, // 옛 7칸
		{ln("ops-events", "-", "-", KindOps), KindOps},                     // 14칸이 이긴다
		{ln("hc-fill-idle", "-", "-", KindLane), KindLane},
		{ln("ops-events", "-", "-", "이상"), KindLane},
	}
	for _, c := range cases {
		if got := KindOf(c.f); got != c.want {
			t.Errorf("KindOf(%q) = %s, want %s", c.f, got, c.want)
		}
	}
}

func TestWithKind(t *testing.T) {
	l12 := Event{Time: "T", Machine: "m", Lane: "ops-events", State: "중간"}.Line()
	if WithKind(l12, KindLane) != l12 || WithKind(l12, "") != l12 {
		t.Fatal("레인 사건 줄이 바뀌었다(열쇠가 바뀐다)")
	}
	if got := WithKind(l12, KindOps); got != l12+"\t-\t"+KindOps {
		t.Fatalf("12칸 운영: %q", got)
	}
	if got := WithKind(l12+"\th/r", KindOps); got != l12+"\th/r\t"+KindOps {
		t.Fatalf("13칸 운영: %q", got)
	}
	idle := Event{Time: "T", Lane: OpsFillIdle, State: "중간"}.Line()
	if got := WithKind(idle, KindLane); got != idle+"\t-\t"+KindLane {
		t.Fatalf("운영 이름을 레인으로 적기: %q", got)
	}
}

// 문서 「## <이름>」·「- 다음: `<명령>`」 = Ops 표
func TestOpsDoc(t *testing.T) {
	b, err := os.ReadFile("../../docs/ops/events.md")
	if err != nil {
		t.Fatal(err)
	}
	var doc []Op
	for _, l := range strings.Split(string(b), "\n") {
		if n, ok := strings.CutPrefix(l, "## "); ok {
			doc = append(doc, Op{Name: n})
		} else if nx, ok := strings.CutPrefix(l, "- 다음: `"); ok && len(doc) > 0 {
			doc[len(doc)-1].Next = strings.TrimSuffix(nx, "`")
		}
	}
	if len(doc) != len(Ops) {
		t.Fatalf("문서 %d개 · 표 %d개\n문서 %v\n표 %v", len(doc), len(Ops), doc, Ops)
	}
	for i := range Ops {
		if doc[i] != Ops[i] {
			t.Errorf("%d번째: 문서 %v · 표 %v", i, doc[i], Ops[i])
		}
	}
}

// 코드에서 운영 사건을 내는 자리(--name 글자·hcLaneReport·aesc·hc-sweep laneReport)의 이름 = Ops 표 이름.
// 새 운영 사건을 더하면 표·문서·이 시험이 함께 바뀐다. 변수 이름(진짜 레인)은 잡지 않는다
func TestOpsNamesInCode(t *testing.T) {
	res := []*regexp.Regexp{
		regexp.MustCompile(`"report",[^\n]*"--name", "([a-z][a-z-]*)"`), // hc run --name(터미널 이름)은 아님
		regexp.MustCompile(`hcLaneReport\([^,]+, [^,]+, "([a-z][a-z-]*)"(\+\w+)?,`),
		regexp.MustCompile(`aesc\("([a-z][a-z-]*)"(\+\w+)?,`),
		regexp.MustCompile(`laneReport\(r, lane, "([a-z][a-z-]*)"`),
	}
	fs, err := filepath.Glob("../../cmd/*/*.go")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, p := range fs {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, re := range res {
			for _, m := range re.FindAllStringSubmatch(string(b), -1) {
				n := m[1]
				if m := re.FindStringSubmatch(m[0]); len(m) > 2 && m[2] != "" {
					n += "<>"
				}
				// 이름 꼴로: merger-+p → merger-<프로젝트>
				if strings.HasSuffix(n, "<>") {
					if o, ok := OpOf(strings.TrimSuffix(n, "<>")+"x", mergerKeyPre); ok {
						n = o.Name
					}
				}
				got[n] = true
			}
		}
	}
	var g, w []string
	for n := range got {
		g = append(g, n)
	}
	for _, o := range Ops {
		w = append(w, o.Name)
	}
	sort.Strings(g)
	sort.Strings(w)
	if strings.Join(g, " ") != strings.Join(w, " ") {
		t.Fatalf("코드가 내는 운영 사건 이름 %v ≠ 표 %v", g, w)
	}
}
