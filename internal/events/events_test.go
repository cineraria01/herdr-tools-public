package events

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBcutKeepsWholeRunes(t *testing.T) {
	s := strings.Repeat("가", 120) // 360바이트
	if got := Bcut(300, s); len(got) != 300 || got != strings.Repeat("가", 100) {
		t.Fatalf("300바이트에서 온 글자로: %d", len(got))
	}
	if got := Bcut(301, s); got != strings.Repeat("가", 100) {
		t.Fatal("반쪽 글자는 버려야 한다")
	}
}

// F10: 가운데 깨진 바이트 뒤를 다 버리지 않는다 — 끝의 반쪽 글자만. 셸 bcut 300의 실제 결과는 297바이트(iconv -c가 가운데 \xff도 뺌),
// 여기는 \xff를 그대로 두어 298바이트(의도된 차이)
func TestBcutOnlyTrimsTail(t *testing.T) {
	s := "앞글자\xff" + strings.Repeat("뒤", 200)
	if got := Bcut(300, s); got != s[:298] {
		t.Fatalf("끝의 반쪽 글자만 깎아야: %d바이트", len(got))
	}
	if got := Bcut(2, "가나"); got != "" {
		t.Fatalf("반쪽뿐이면 빈 글: %q", got)
	}
	if got := Bcut(3, "ab\xff\xfe"); got != "ab\xff" {
		t.Fatalf("끝이 완결된 깨진 바이트면 그대로: %q", got)
	}
	if Bcut(-5, "abc") != "" || Bcut(0, "abc") != "" {
		t.Fatal("0 이하는 빈 글(패닉 없이)")
	}
}

// M09: 꼬리 칸이 길어도 패닉하지 않고 늘 12칸, 줄이는 것은 한 줄 칸뿐. 지킬 수 없으면 Append가 쓰기 전에 오류
func TestLineLongTailNeverPanics(t *testing.T) {
	base := Event{Time: "2026-11-21T00:00:00Z", Machine: "box", Lane: "l-a", State: "완료", SHA: "abcdef12", Msg: strings.Repeat("가", 400)}
	mid := base
	mid.To = strings.Repeat("t", 800)
	l := mid.Line()
	p, ok := Parse(l)
	if strings.Count(l, "\t") != 11 || !ok || p.Lane != "l-a" || p.To != mid.To || p.State != "완료" || len(l) > MaxLine {
		t.Fatalf("12칸·앞 칸 그대로·1000바이트 안: %d칸 %d바이트 %+v", strings.Count(l, "\t")+1, len(l), p.Lane)
	}
	if err := mid.Check(); err != nil {
		t.Fatalf("들어가면 오류 아님: %v", err)
	}
	for _, f := range []func(*Event){func(e *Event) { e.To = strings.Repeat("t", 1200) }, func(e *Event) { e.Key = strings.Repeat("k", 990) },
		func(e *Event) { e.Run = strings.Repeat("r", 5000) }, func(e *Event) { e.To = "a\tb" }} {
		e := base
		f(&e)
		l := e.Line() // 패닉하면 시험 실패
		if strings.Count(l, "\t") != 11 {
			t.Fatalf("늘 12칸: %d", strings.Count(l, "\t")+1)
		}
		if e.Check() == nil {
			t.Fatalf("지킬 수 없는 칸은 Check 오류: To %d Key %d Run %d", len(e.To), len(e.Key), len(e.Run))
		}
		p := filepath.Join(t.TempDir(), "events.tsv")
		if err := Append(p, e); err == nil {
			t.Fatal("Append가 쓰기 전에 오류여야")
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatal("오류면 파일을 건드리지 않는다")
		}
	}
}

// report.sh와 같은 꼴: 빈 칸은 -, 탭·줄바꿈은 빈칸, 앞 칸 64바이트, 줄 1000바이트 안
func TestLineMatchesReportFormat(t *testing.T) {
	e := Event{Time: "2026-11-21T00:00:00Z", Machine: "box", Lane: "l-a", State: "완료", SHA: "abcdef12", Msg: "한\t줄\n끝"}
	want := "2026-11-21T00:00:00Z\tbox\t-\tl-a\t완료\tabcdef12\t한 줄 끝\t-\t-\t-\t-\t-"
	if got := e.Line(); got != want {
		t.Fatalf("줄 꼴:\n%q\n%q", got, want)
	}
	long := Event{Time: "2026-11-21T00:00:00Z", Lane: strings.Repeat("x", 100), State: "중간", Msg: strings.Repeat("가", 500), Key: "k1"}
	l := long.Line()
	if len(l) > MaxLine || !strings.HasSuffix(l, "\t-\t-\t-\t-\tk1") {
		t.Fatalf("1000바이트 안·꼬리 칸은 그대로: %d %q", len(l), l[len(l)-20:])
	}
	p, ok := Parse(l)
	if !ok || len(p.Lane) != 64 || p.Key != "k1" {
		t.Fatalf("레인 칸 64바이트, 다시 읽힘: %v %d", ok, len(p.Lane))
	}
}

func TestParseOldAndBroken(t *testing.T) {
	if _, ok := Parse("a\tb\tc"); ok {
		t.Fatal("7칸이 안 되면 거절")
	}
	e, ok := Parse("t\tm\t-\tl\t착지\t-\t글")
	if !ok || e.Msg != "글" || e.Key != "" {
		t.Fatalf("옛 7칸 줄: %+v", e)
	}
}

func TestAppendAndScan(t *testing.T) {
	p := filepath.Join(t.TempDir(), "events.tsv")
	if err := ScanFile(p, func(Event) bool { t.Fatal("없는 파일은 사건 없음"); return false }); err != nil {
		t.Fatal(err)
	}
	for _, st := range []string{"중간", "완료", "착지"} {
		if err := Append(p, Event{Lane: "l", State: st}); err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("깨진\t줄\n")
	f.Close()
	var got []string
	if err := ScanFile(p, func(e Event) bool { got = append(got, e.State); return e.State != "완료" }); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "중간,완료" {
		t.Fatalf("fn이 false면 멈춘다: %v", got)
	}
}
