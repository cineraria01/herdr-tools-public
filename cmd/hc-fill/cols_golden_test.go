package main

// fill-named-cols(2026-11-24): 칸을 번호 대신 이름으로 읽게 옮기기 전에 둔 견본 — 파일 종류마다 옛·새 꼴 줄을 읽은 결과와
// 쓰기가 만든 줄이 바이트 단위로 전후 같다(옮기기 전 코드로 통과, 옮긴 뒤에도 같은 기대값). 시각 칸만 가린다

import (
	"os"
	"strings"
	"testing"
)

func cgWrite(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func cgRead(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func cgEq(t *testing.T, what, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s:\n받음 %q\n기대 %q", what, got, want)
	}
}

// 대기열: 10칸·14칸 옛 줄과 17칸 새 줄 — qset이 상태·기계·주소·남긴것과 먼저·감사 칸을 채우는 꼴, qrow·qown 읽기
func TestColsGoldenQueue(t *testing.T) {
	setData(t)
	old10 := "a\tapp\t/b/a.md\t-\t-\twait\t-\t-\t-\tT0"
	old14 := "b\tapp\t/b/b.md\tx\tm2\twait\t-\t-\t-\tT0\t-\tc1\thigh\t-"
	new17 := "c\tapp\t/b/c.md\t-\t-\twait\t-\t-\t-\tT0\tm:/p#r1\tc2\tlow\tno-early\t-\tsolo\tr9-1-1"
	cgWrite(t, Q, old10+"\n"+old14+"\n"+new17+"\n")
	qHeld = true
	defer func() { qHeld = false }()
	if rc := qset(false, "", "a", "up", "m1", "m1:t1", "st", "early", "auto"); rc != 0 {
		t.Fatalf("qset a: %d", rc)
	}
	if rc := qset(false, "", "b", "up", "m2", "m2:t2", "-", "", "auto"); rc != 0 {
		t.Fatalf("qset b: %d", rc)
	}
	if rc := qset(true, "r0-0-0", "c", "up", "m3", "m3:t3", "-", "", ""); rc != 1 {
		t.Fatalf("qset c 다른 줄 ID: %d, 1이어야", rc)
	}
	if rc := qset(true, "r9-1-1", "c", "sent", "m3", "m3:t3", "-", "", ""); rc != 0 {
		t.Fatalf("qset c: %d", rc)
	}
	cgEq(t, "대기열", cgRead(t, Q),
		"a\tapp\t/b/a.md\t-\t-\tup\tm1\tm1:t1\tst\tT0\t-\t-\tnormal\tearly\tauto\n"+
			"b\tapp\t/b/b.md\tx\tm2\tup\tm2\tm2:t2\t-\tT0\t-\tc1\thigh\t-\tauto\n"+
			"c\tapp\t/b/c.md\t-\t-\tsent\tm3\tm3:t3\t-\tT0\tm:/p#r1\tc2\tlow\tno-early\t-\tsolo\tr9-1-1\n")
	qHeld = false
	cgEq(t, "qrow b", qrow("b").Line(), "b\tapp\t/b/b.md\tx\tm2\tup\tm2\tm2:t2\t-\tT0\t-\tc1\thigh\t-\tauto")
	cgEq(t, "qown a(12칸 없음)", qown("a"), "-")
	cgEq(t, "qown c", qown("c"), "c2")
	r := pad16("d", "app", "/b/d.md", "-", "-", "wait")
	if r.NF() != 17 || r.F(qOverlap) != "-" || !sinceIDRe.MatchString(r.F(qRID)) {
		t.Fatalf("pad16: %q", r.Line())
	}
}

// 감사 기록: 옛 7칸 줄을 차수 줄로 옮기기(amigrate), 17칸 줄 고치기(aupd)·읽기(arow·auRest)
func TestColsGoldenAudit(t *testing.T) {
	setData(t)
	old := "ln\tsha1\t2\t1\tqueued\tx\ty"
	done := "lm\tsha2\t1\t0\tpass\tx\ty"
	cur := "a1\tlk\t1\trun@1\tsha3\tm1\tm1:t1\tqueued\t0\t-\t0\t0\t-\t-\t0\t-\t비고 하나\t둘"
	cgWrite(t, AT, old+"\n"+done+"\n"+cur+"\n")
	amigrate()
	got := cgRead(t, AT)
	ids := []string{}
	for _, l := range lines(got) {
		ids = append(ids, strings.SplitN(l, "\t", 2)[0])
	}
	cgEq(t, "옮긴 감사 기록", got,
		ids[0]+"\tln\t2\t-\tsha1\t-\t-\tescalate:legacy\t1\t-\t0\t0\t-\t-\t0\t-\tln 옛 감사 기록(sha1 2번째) — 판정을 차수에 묶지 못한 채 hc-fill이 바뀌었다. 지휘 판단(옛 감사 줄 정리 포함)\n"+
			ids[1]+"\tlm\t1\t-\tsha2\t-\t-\tlegacy\t0\t-\t0\t0\t-\t-\t0\t-\t-\n"+cur+"\n")
	r := arow("a1")
	cgEq(t, "arow", r.Line(), cur)
	cgEq(t, "auRest 17", auRest(r, atNote), "비고 하나\t둘")
	aupd("a1", cv{8, "deliver"}, cv{9, "2"}, cv{10, "pass"}, cv{16, "hml=0.0.1"}, cv{17, "보고\t탭"}) // 칸 번호 그대로 — 이름의 값이 꼴과 같은지도 본다
	want := "a1\tlk\t1\trun@1\tsha3\tm1\tm1:t1\tdeliver\t2\tpass\t0\t0\t-\t-\t0\thml=0.0.1\t보고 탭\t둘"
	cgEq(t, "aupd", arow("a1").Line(), want)
	aupd("a1", cv{atChecked, ""})
	cgEq(t, "aupd 빈 값", strings.Split(arow("a1").Line(), "\t")[12], "-")
}

// 받은편지 장부: lget은 마지막 줄, rfix가 쓰는 lset은 그 줄의 칸과 9칸(시각)을 바꾼다
func TestColsGoldenLedger(t *testing.T) {
	setData(t)
	cgWrite(t, LED, "rq1\tln\tm:/p\topen\t-\t-\t-\t-\tT0\n"+"rq2\tlm\tm:/q\tsent\tresult\th\t3\t-\tT0\n")
	qHeld = true
	defer func() { qHeld = false }()
	cgEq(t, "lget", lget("rq2").Line(), "rq2\tlm\tm:/q\tsent\tresult\th\t3\t-\tT0")
	if rc := rfix("rq1", "result", "결과: 성공", true); rc != 0 {
		t.Fatalf("rfix: %d", rc)
	}
	r := lget("rq1")
	if r.NF() != 9 || r.F(ldTime) == "T0" || r.F(ldTime) == "" {
		t.Fatalf("lset 시각 칸: %q", r.Line())
	}
	cgEq(t, "lset", strings.Join(r[:8], "\t"), "rq1\tln\tm:/p\tdeliver\tresult\t"+sha256hex([]byte("결과: 성공\n"))+"\t"+"15\t-")
	cgEq(t, "다른 줄", lget("rq2").Line(), "rq2\tlm\tm:/q\tsent\tresult\th\t3\t-\tT0")
}

// 시도 기록: tryput은 같은 이름 줄을 바꾸고 7칸(빈 값 -), attemptRun은 6칸 주소·7칸 실행 키
func TestColsGoldenTry(t *testing.T) {
	setData(t)
	f := DATA + "/try.tsv"
	cgWrite(t, f, "x\tr1\tm1\t-\t-\t?\t-\tm1:t9\t3\n"+"y\tr2\tm2\t-\t-\tm2:t2\tm2:t2@5\n")
	if rc := tryput("x", "r3", "m3", "early", "", "m3:t3", ""); rc != 0 {
		t.Fatalf("tryput: %d", rc)
	}
	cgEq(t, "try.tsv", cgRead(t, f), "y\tr2\tm2\t-\t-\tm2:t2\tm2:t2@5\n"+"x\tr3\tm3\tearly\t-\tm3:t3\t-\n")
	cgEq(t, "attemptRun", attemptRun("y", "m2:t2"), "m2:t2@5")
	cgEq(t, "attemptRun 다른 주소", attemptRun("y", "m2:t3"), "")
}

// 사건 줄: lastev는 4칸 이름·3칸 터미널·5칸 상태, 옛 7칸 줄도 같은 칸
func TestColsGoldenEvents(t *testing.T) {
	old := EV
	defer func() { EV = old }()
	e1 := "T1\tm1\tm1:t1\tln\t완료\tsha\t한 줄"
	e2 := "T2\tm1\tm1:t1\tln\t중간\t-\t둘\t3\t-\trun@1\tc1\tk"
	e3 := "T3\tm1\tm1:t2\tln\t완료\tsha2\t셋"
	EV = []string{e1, e2, e3, "모름 m9"}
	cgEq(t, "lastev", lastev("ln", "m1:t1", ""), e2)
	cgEq(t, "lastev 상태", lastev("ln", "m1:t1", "완료"), e1)
	cgEq(t, "evF 7", evF(e2, evMsg), "둘")
	cgEq(t, "lcCut 탭 없음", lcCut("모름 m9", evState), "모름 m9")
}
