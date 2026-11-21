package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestNk(t *testing.T) {
	cases := map[string]string{
		"멈춤\tbox/t1\tl\t쉼 turn 3":               "멈춤\tt1\tl\t쉼 turn 3",
		"T\tbox\tbox/t9\tl\t완료\tabc\tm":         "T\t*\tt9\tl\t완료\tabc\tm",
		"T\tbox\tt9\tl\t완료\tabc\tm\t2\t-\t-\t-": "T\t*\tt9\tl\t완료\tabc\tm\t2\t-\t-\t-",
		"#since\t2026": "#since\t2026",
		"짧은\t줄":        "짧은\t줄",
		"":             "",
	}
	for in, want := range cases {
		if got := nk(in); got != want {
			t.Errorf("nk(%q) = %q, want %q", in, got, want)
		}
	}
	// 이미 바꾼 열쇠에 다시 써도 그대로
	k := nk("T\tbox\tbox/t9\tl\t완료\tabc\tm")
	if nk(k) != k {
		t.Errorf("nk 두 번이 다르다: %q", nk(k))
	}
}

func TestEvKey(t *testing.T) {
	f := strings.Split("T\tbox\tt1\tl\t멈춤\t-\tdone turn 4", tab)
	if got := evKey(f); got != "멈춤\tt1\tl\t쉼 turn 4" {
		t.Errorf("evKey 멈춤 = %q", got)
	}
	f = strings.Split("T\tbox\tt1\tl\t완료\tabc\tm", tab)
	if got := evKey(f); got != strings.Join(f, tab) {
		t.Errorf("evKey 보통 = %q", got)
	}
}

func TestLastRep(t *testing.T) {
	r := newLastRep()
	r.rep("착지", "abcdef12", "3", "")
	r.rep("완료", "ABCDEF1234567", "3", "") // 착지 뒤 같은 턴의 같은 sha 완료 — 착지 그대로, turn만
	if r.lst["x"] != "착지" || r.ltn["x"] != "3" {
		t.Fatalf("같은 턴 완료가 착지를 덮었다: %v %v", r.lst, r.ltn)
	}
	r.rep("완료", "abcdef12", "5", "") // 턴이 올랐으면 새 일
	if r.lst["x"] != "완료" {
		t.Fatalf("다음 턴 완료를 못 봤다: %v", r.lst)
	}
	if samesha("-", "-") || samesha("abc", "") || !samesha("ABCdef1", "abcdef1234") {
		t.Fatal("samesha")
	}
}

func TestTurnAfter(t *testing.T) {
	if a, u := turnAfter("3", "4"); a || u {
		t.Error("4 <= 3+1")
	}
	if a, u := turnAfter("3", "5"); !a || u {
		t.Error("5 > 3+1")
	}
	if _, u := turnAfter("3", "-"); !u {
		t.Error("- 는 모름")
	}
}

func TestSinceArg(t *testing.T) {
	if got := sinceArg("2026-11-16T02:00:00Z"); got != "2026-11-16T02:00:00Z" {
		t.Errorf("시각 그대로: %q", got)
	}
	if got := sinceArg(""); got != "" {
		t.Errorf("빈 값: %q", got)
	}
	if !regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$`).MatchString(sinceArg("90m")) {
		t.Errorf("90m: %q", sinceArg("90m"))
	}
	if sinceArg("2") >= sinceArg("1") {
		t.Error("숫자만은 시간 — 2시간 전이 1시간 전보다 앞")
	}
}

func TestWdialog(t *testing.T) {
	e := &ev{wdlg: regexp.MustCompile(`Chat about this|Type something|Do you want to proceed\?`)}
	ask := "글\n────────\n☐ 머리\n질문\n❯ 1. 예\nType something.\n────────\n4. Chat about this"
	if !e.wdialog(ask) {
		t.Error("질문 창을 못 봤다")
	}
	box := "Type something 인용\n────────\n❯ 입력\n────────\n  ? for shortcuts"
	if e.wdialog(box) {
		t.Error("평소 입력 칸을 창으로 봤다")
	}
	if e.wdialog("Do you want to proceed?") {
		t.Error("가로줄 없는 화면은 창이 아니다")
	}
	perm := "╌╌╌╌╌╌\n설명\nDo you want to proceed?\n❯ 1. Yes"
	if !e.wdialog(perm) {
		t.Error("권한 창을 못 봤다")
	}
}

func TestReadCoordAndJSON(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "coord.t1")
	if err := os.WriteFile(p, []byte("t1\t4242\t이름\t0\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	c, ok := readCoord(p)
	if !ok || c[0] != "t1" || c[1] != "4242" || c[2] != "이름" || c[3] != "0" {
		t.Fatalf("readCoord = %v %v", c, ok)
	}
	if err := os.WriteFile(p, []byte("t1\t4242"), 0o666); err != nil {
		t.Fatal(err)
	}
	if _, ok := readCoord(p); ok {
		t.Error("줄바꿈 없는 줄은 read 실패")
	}
	g := `{"address":"t1","foreground":{"name":"node","pid":1},"created_ms":1700000000123,"name":"lane","pid":77,"state":"live","turn":3}`
	m := jparse(g)
	if n, _ := jget(m, "name"); n != "lane" || topNum(m, "turn", "") != "3" || topNum(m, "pid", "foreground") != "77" {
		t.Errorf("최상위 값: %v", m)
	}
	if c, ok := liveCreatedMs(g); !ok || c != "1700000000123" {
		t.Errorf("created_ms: %q %v", c, ok)
	}
	if _, ok := liveCreatedMs(`{"state":"finished","created_ms":1}`); ok {
		t.Error("끝난 터미널의 실행")
	}
	if topNum(jparse(`{"foreground":{"pid":9},"pid":null}`), "pid", "foreground") != "9" {
		t.Error("맨 위 pid가 null이면 foreground.pid")
	}
	recs := jlist(`{"terminals":[{"address":"t1","state":"live","turn":2},{"name":"x"}]}`, "terminals", "address", "parent", "turn")
	if len(recs) != 1 || recs[0][1] != "-" || recs[0][2] != "2" {
		t.Errorf("jrec = %v", recs)
	}
}

func TestStripCtlAndCkey(t *testing.T) {
	if stripCtl("a\x1b[1mb\x03c\x7f") != "a[1mbc" {
		t.Error("stripCtl")
	}
	if ckey("mini-m4/t7.x") != "mini-m4_t7_x" {
		t.Error("ckey")
	}
}
