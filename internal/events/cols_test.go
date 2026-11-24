package events

// events-named-cols(2026-11-24): 사건 줄을 이름 붙은 칸(Cols·Raw·Record·LineN)으로 — 옮기기 전 코드(아래 old*, 그대로 베낌)와
// 칸 수별 견본(실제 사건 파일 꼴: 8·12·13·14칸 + 옛 7·9·10·11칸, 깨진 줄)에서 같은 결과인지

import (
	"strings"
	"testing"
)

// Corpus: 칸 수별 견본 줄(값은 가짜). cmd/hc-lane·hc-sweep 시험도 이 꼴을 쓴다
var corpus = []string{
	"2026-11-21T00:00:00Z\tbox\t-\tl-a\t착지\t-\t글",                          // 옛 7칸
	"2026-11-24T04:09:01Z\tbox\tt20r\tl-a\t정리됨\t-\thc-sweep R2\t2",         // hc-sweep 8칸
	"2026-11-24T04:09:01Z\tm2\tm2/t20r\tl-a\t정리됨\t-\thc-sweep R2\t2",       // 원격 8칸
	"2026-11-21T00:00:01Z\tbox\tt1\tl-a\t완료\tabcdef12\t글\t3\t-",            // 9칸
	"2026-11-21T00:00:02Z\tbox\tt1\tl-a\t완료\tabcdef12\t글\t3\t-\tt1@1795",   // 10칸
	"2026-11-21T00:00:03Z\tbox\tt1\tl-a\t중간\t-\t글\t3\t-\tt1@1795\tt9@1700", // 11칸
	"2026-11-21T00:00:04Z\tbox\tt1\tl-a\t제출\tabcdef12\tbox-20261121T010203-42 → q\t0\t-\tt1@1795\tt9@1700\tsubmit:box-20261121T010203-42",
	"2026-11-21T00:00:05Z\tbox\tt1\tl-a\t완료\tabcdef12\t글\t0\t-\tt1@1795\tt9@1700\t-\tgithub.com/org/repo", // 13칸
	"2026-11-21T00:00:06Z\tbox\tt1\tmerger\t막힘\t-\t병합기 멈춤\t0\t-\tt1@1795\t-\t-\t-\tops",                   // 14칸 운영
	"2026-11-21T00:00:07Z\tbox\tt1\tl-a\t중간\t-\t글\t0\t-\tt1@1795\t-\t-\tgithub.com/org/repo\tlane",        // 14칸 레인
	"2026-11-21T00:00:08Z\tbox\tt1\tl-a\t중간\t-\t글\t0\t-\tt1@1795\t-\t-\t-\tlane\t덧\t칸",                    // 16칸(뒤 칸 그대로)
	"2026-11-21T00:00:09Z\tbox\t-\thc-fill-idle\t중간\t-\t자리 빔\t-\t-\t-\t-\t-",                              // 12칸 운영(이름 표)
	"2026-11-21T00:00:10Z\tbox\t-\tmerger-proj\t막힘\t-\t멈춤\t-\t-\t-\t-\thf:merger:proj:dead:1:-",           // 키 머리 운영
	"2026-11-21T00:00:11Z\tbox\tt1\tmerger-alive\t중간\t-\t글\t-\t-\t-\t-\t-",                                // 이름만 merger-
	"2026-11-21T00:00:12Z\tbox\tt1\tl-a\t멈춤\t-\tdone turn 4\t4\t-\t-\t-\t-",
	"2026-11-21T00:00:13Z\tbox\tt1\thc-lane-wait\t중간\t-\t글\t-\tt9,t8\t-\tt9@1700\t-",
	"2026-11-21T00:00:14Z\t\t\tl-b\t\t\t\t\t\t\t\t", // 빈 칸들
	"모름 box", "#since\t2026", "멈춤\tbox/t1\tl-a\t쉼 turn 3", "", "\t", "a\tb\tc",
}

func oldKindOf(f []string) string {
	if len(f) >= 14 && (f[13] == KindOps || f[13] == KindLane) {
		return f[13]
	}
	get := func(i int) string {
		if len(f) > i {
			return f[i]
		}
		return ""
	}
	if _, ok := OpOf(get(3), get(11)); ok {
		return KindOps
	}
	return KindLane
}

func oldWithKind(line, kind string) string {
	f := strings.Split(line, "\t")
	if kind == "" || kind == oldKindOf(f) && kind == KindLane || len(f) < 12 || len(f) > 13 {
		return line
	}
	if len(f) == 12 {
		line += "\t-"
	}
	return line + "\t" + kind
}

func oldParse(line string) ([12]string, bool) {
	c := strings.Split(strings.TrimRight(line, "\r\n"), "\t")
	if len(c) < 7 {
		return [12]string{}, false
	}
	for len(c) < 12 {
		c = append(c, "")
	}
	return [12]string(c[:12]), true
}

// ① 견본을 읽은 결과가 전후 같다(칸마다 awk $i와 같은 값·칸 수) ③ 다시 이은 줄이 바이트 그대로
func TestColsMatchSplit(t *testing.T) {
	for _, l := range corpus {
		f := strings.Split(l, "\t")
		e := Cols(l)
		if e.N != len(f) {
			t.Errorf("%q: N %d, 칸 %d", l, e.N, len(f))
		}
		named := e.named()
		for i := range named {
			want := ""
			if i < len(f) {
				want = f[i]
			}
			if named[i] != want {
				t.Errorf("%q: %d칸 %q, 바람 %q", l, i+1, named[i], want)
			}
		}
		if e.Raw() != l {
			t.Errorf("Raw가 줄을 바꿨다: %q → %q", l, e.Raw())
		}
		if KindOf(f) != oldKindOf(f) || e.KindOf() != oldKindOf(f) {
			t.Errorf("%q: 종류 %s/%s, 전 %s", l, KindOf(f), e.KindOf(), oldKindOf(f))
		}
		for _, k := range []string{"", KindLane, KindOps} {
			if WithKind(l, k) != oldWithKind(l, k) {
				t.Errorf("%q %q: WithKind %q, 전 %q", l, k, WithKind(l, k), oldWithKind(l, k))
			}
		}
		p, ok := Parse(l + "\n")
		o, ook := oldParse(l + "\n")
		pn := p.named()
		if ok != ook || ok && [12]string(pn[:12]) != o {
			t.Errorf("%q: Parse %v %+v, 전 %v %v", l, ok, p, ook, o)
		}
	}
}

// 고친 칸만 바뀌고 나머지는 그대로(watch의 원격 주소·정리됨 → 멈춤)
func TestRawAfterEdit(t *testing.T) {
	e := Cols(corpus[1])
	e.State, e.SHA, e.Msg = "멈춤", "-", "뜻밖의 종료(정리됨) "+e.Msg
	if got, want := e.Raw(), "2026-11-24T04:09:01Z\tbox\tt20r\tl-a\t멈춤\t-\t뜻밖의 종료(정리됨) hc-sweep R2\t2"; got != want {
		t.Fatalf("%q\n%q", got, want)
	}
	if (Event{Time: "T", State: "중간"}).Raw() != (Event{Time: "T", State: "중간"}).Line() {
		t.Fatal("읽지 않은 사건의 Raw는 Line")
	}
}

// oldWithRepoKind: hc-lane report.go withRepoKind(옮기기 전) 그대로
func oldWithRepoKind(e Event, rid, kind string) string {
	line := e.Line()
	if kind == "" {
		kind = oldKindOf(strings.Split(line, "\t"))
	}
	kc := ""
	if oldWithKind(line, kind) != line {
		kc = "\t" + kind
	}
	tail := ""
	switch {
	case rid != "":
		tail = "\t" + rid + kc
	case kc != "":
		tail = "\t" + "-" + kc
	}
	if over := len(line) + len(tail) - MaxLine; over > 0 {
		f := strings.Split(line, "\t")
		shown := f[6]
		e.Msg = Bcut(len(shown)-over, shown)
		line = e.Line()
	}
	if len(line)+len(tail) > MaxLine && rid != "" {
		return oldWithRepoKind(e, "", kind)
	}
	return line + tail
}

// ② 쓰기가 만든 줄이 바이트 단위로 같다(report가 적는 13·14칸, 1000바이트 깎기, 저장소 칸 빼기)
func TestRecordMatchesOld(t *testing.T) {
	base := Event{Time: "2026-11-21T00:00:00Z", Machine: "box", Term: "t1", Lane: "l-a", State: "완료", SHA: "abcdef12", Msg: "글", Turn: "3",
		To: "-", Run: "t1@1795", Owner: "t9@1700", Key: "-"}
	long := strings.Repeat("가", 400)
	for _, msg := range []string{"글", "", long, strings.Repeat("x", 990)} {
		for _, lane := range []string{"l-a", OpsFillIdle, "merger-p"} {
			for _, key := range []string{"-", "hf:merger:p:x", strings.Repeat("k", 900)} {
				for _, rid := range []string{"", "github.com/org/repo", strings.Repeat("r", 200)} {
					for _, kind := range []string{"", KindLane, KindOps} {
						e := base
						e.Msg, e.Lane, e.Key = msg, lane, key
						want := oldWithRepoKind(e, rid, kind)
						e.Repo, e.Kind = rid, kind
						if got := e.Record(); got != want {
							t.Fatalf("msg %d lane %s key %d rid %d kind %q:\n%q\n%q", len(msg), lane, len(key), len(rid), kind, got, want)
						}
					}
				}
			}
		}
	}
}

// LineN: 앞 n칸, 상한은 Line과 같이(64바이트 레인·기계·터미널, 탭 지우기, 빈 칸 「-」)
func TestLineN(t *testing.T) {
	e := Event{Time: "2026-11-21T00:00:00Z", Machine: "box", Term: "t1", Lane: strings.Repeat("l", 100), State: "정리됨", Msg: "hc-sweep a\tb", Turn: "2"}
	got := e.LineN(8)
	if want := "2026-11-21T00:00:00Z\tbox\tt1\t" + strings.Repeat("l", 64) + "\t정리됨\t-\thc-sweep a b\t2"; got != want {
		t.Fatalf("%q\n%q", got, want)
	}
	if e.LineN(12) != e.Line() || e.LineN(20) != e.Line() {
		t.Fatal("12칸 이상이면 Line")
	}
}
