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
