//go:build !windows

package main

// fill-edit-hands(2026-11-24): ① 대기 줄 edit --overlap-ok → 그 줄 칸으로 겹침 대기 없이 뜬다 ② --early·--no-early는 먼저 띄움 칸만
// ③ 뜬 줄·끝난 줄·감사 줄·단독 줄·주차 줄은 거절(아무것도 안 바뀜) ④ nudge: 상태 error인 터미널에 글 한 번, 간격 안 두 번째는 안 보냄
// ⑤ 일하는 중·터미널 없음·이미 보고함 → 안 보냄·종료 1 ⑥ 주소는 같고 이름이 다르면 안 보냄 ⑦ 일지에 남는다

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// ehRow: 대기열 파일의 그 레인 줄
func ehRow(t *testing.T, k fillKit, name string) Row {
	t.Helper()
	for _, r := range readRowsT(t, k.data+"/queue.tsv") {
		if r.F(qName) == name {
			return r
		}
	}
	t.Fatalf("대기열에 %s 가 없다", name)
	return nil
}

func readRowsT(t *testing.T, p string) []Row {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var rs []Row
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		rs = append(rs, splitRow(l))
	}
	return rs
}

func ehPut(t *testing.T, p, s string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), mode); err != nil {
		t.Fatal(err)
	}
}

// ehJournal: 일지의 그 조치 줄들
func ehJournal(t *testing.T, k fillKit, action string) []map[string]string {
	t.Helper()
	out, se, rc := k.run(t, "journal", "--json")
	var es []map[string]string
	if rc != 0 || json.Unmarshal([]byte(out), &es) != nil {
		t.Fatalf("journal --json: %d %q %q", rc, out, se)
	}
	var got []map[string]string
	for _, e := range es {
		if e["action"] == action {
			got = append(got, e)
		}
	}
	return got
}

func ehKit(t *testing.T) fillKit {
	k := newFillKit(t)
	k.env = append(k.env, "HERDR_TOOLS_JOURNAL="+k.home+"/journal.jsonl")
	return k
}

// ① 겹침 검사 끄기: 칸이 overlap-ok가 되고(줄 ID·다른 칸 그대로), 그 칸 값으로는 앞 레인 예약과 겹쳐도 뜬다 — 지금 값(-)이면 겹침 대기.
// 고치기 전 코드는 --overlap-ok를 사용법으로 거절(종료 2)
func TestEditOverlapOK(t *testing.T) {
	k := ehKit(t)
	before := ehRow(t, k, "lane-a")
	out, se, rc := k.run(t, "edit", "lane-a", "--overlap-ok", "--why", "문서 하나 겹침")
	if rc != 0 || !strings.Contains(out, "겹침 봄 끔") {
		t.Fatalf("edit --overlap-ok: 종료 0이어야 — %d %q %q", rc, out, se)
	}
	r := ehRow(t, k, "lane-a")
	if r.F(qOverlap) != "overlap-ok" || r.F(qEarly) != before.F(qEarly) || r.F(qAudit) != before.F(qAudit) || r.F(qPrio) != before.F(qPrio) || r.F(qAfter) != before.F(qAfter) {
		t.Fatalf("겹침칸만 바뀌어야: 전 %q 뒤 %q", before, r)
	}
	if js := ehJournal(t, k, "edit"); len(js) != 1 || js[0]["undo"] != "hc-fill edit lane-a --overlap-check" || js[0]["why"] != "문서 하나 겹침" {
		t.Fatalf("일지의 되돌리기: %v", js)
	}
	// 다음 바퀴의 판정(run이 그 줄 칸을 ovlCheck에 넘긴다): 앞 레인 A가 같은 파일을 쥐고 있다
	bf := ecSetup(t)
	OREC = []Row{{"A", "app", "here", "t1", "s", "b", ",-,", "-"}}
	OWN = []Row{{"app", "A", "R", "cmd/x.go"}}
	b := bf("lane-a", "파일: cmd/x.go\n")
	OVWAIT = nil
	if ovlCheck("lane-a", "app", b, "-", "-", "-", "-") {
		t.Fatal("고치기 전 칸(-): 겹침 대기여야 — 시험이 헛돈다")
	}
	OVWAIT = nil
	if !ovlCheck("lane-a", "app", b, "-", "-", dash(r.F(qOverlap)), "-") || len(OVWAIT) != 0 {
		t.Fatalf("edit 뒤 칸(%s): 겹침 대기 없이 떠야 (OVWAIT=%q)", r.F(qOverlap), rowsBar(OVWAIT))
	}
	// 되돌리기
	if _, se, rc := k.run(t, "edit", "lane-a", "--overlap-check"); rc != 0 || ehRow(t, k, "lane-a").F(qOverlap) != "-" {
		t.Fatalf("edit --overlap-check: %d %q", rc, se)
	}
	if _, se, rc := k.run(t, "edit", "lane-a", "--overlap-ok", "--overlap-check"); rc != 2 || !strings.Contains(se, "반대 인자") {
		t.Fatalf("반대 인자 함께: 종료 2여야 — %d %q", rc, se)
	}
}

// ② 먼저 띄움: no-early 줄에 --early → 칸 「-」(run의 먼저 띄움 후보 조건), --no-early → no-early. 되돌리는 명령이 일지에
func TestEditEarly(t *testing.T) {
	k := ehKit(t)
	ehPut(t, k.data+"/queue.tsv", "lane-a\tapp\t/nowhere/b.md\tlane-z\t-\twait\t-\t-\t-\t1\t-\t-\thigh\tno-early\tno-audit\t-\trid1\n", 0o644)
	if out, se, rc := k.run(t, "edit", "lane-a", "--early"); rc != 0 || !strings.Contains(out, "먼저 띄움 켬") {
		t.Fatalf("edit --early: %d %q %q", rc, out, se)
	}
	r := ehRow(t, k, "lane-a")
	if r.F(qEarly) != "-" || r.F(qRID) != "rid1" || r.F(qOverlap) != "-" {
		t.Fatalf("먼저 띄움 칸만 「-」로(줄 ID 그대로): %q", r)
	}
	if _, se, rc := k.run(t, "edit", "lane-a", "--no-early"); rc != 0 || ehRow(t, k, "lane-a").F(qEarly) != "no-early" {
		t.Fatalf("edit --no-early: %d %q", rc, se)
	}
	js := ehJournal(t, k, "edit")
	if len(js) != 2 || js[0]["undo"] != "hc-fill edit lane-a --no-early" || js[1]["undo"] != "hc-fill edit lane-a --early" {
		t.Fatalf("일지의 되돌리기: %v", js)
	}
	// 옛 13칸 줄도 16칸까지 채워 바꾼다(우선은 그대로)
	ehPut(t, k.data+"/queue.tsv", "lane-b\tapp\t/nowhere/b.md\t-\t-\twait\t-\t-\t-\t1\t-\t-\tlow\n", 0o644)
	if _, se, rc := k.run(t, "edit", "lane-b", "--no-early"); rc != 0 {
		t.Fatalf("옛 줄 edit: %d %q", rc, se)
	}
	if r := ehRow(t, k, "lane-b"); r.NF() != 16 || r.F(qEarly) != "no-early" || r.F(qPrio) != "low" || r.F(qAudit) != "-" {
		t.Fatalf("옛 줄 채움: %q", r)
	}
}

// ③ 아직 안 뜬 줄만: 뜬 줄·끝난 줄·감사 줄·단독 줄의 겹침·주차 줄의 먼저 띄움은 거절(종료 2), 대기열 그대로
func TestEditHandsRefused(t *testing.T) {
	k := ehKit(t)
	q := "up-1\tapp\t/b.md\t-\t-\tup\there\tt1\t-\t1\t-\t-\tnormal\t-\t-\t-\tr1\n" +
		"sent-1\tapp\t/b.md\t-\t-\tsent\there\tt2\t-\t1\t-\t-\tnormal\t-\t-\t-\tr2\n" +
		"audit-x\tapp\t/b.md\t-\t-\twait\t-\t-\t-\t1\t-\t-\turgent\tno-early\taudit=w\t-\tr3\n" +
		"solo-1\tapp\t/b.md\t-\t-\twait\t-\t-\t-\t1\t-\t-\tnormal\t-\t-\tsolo\tr4\n" +
		"park-1\tapp\t/b.md\tp0\t-\twait\t-\t-\t-\t1\t-\t-\tnormal\tparked\t-\t-\tr5\n"
	ehPut(t, k.data+"/queue.tsv", q, 0o644)
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"edit", "up-1", "--overlap-ok"}, "이미 떴다"},
		{[]string{"edit", "up-1", "--no-early"}, "이미 떴다"},
		{[]string{"edit", "sent-1", "--early"}, "이미 끝났다"},
		{[]string{"edit", "audit-x", "--overlap-ok"}, "감사 줄"},
		{[]string{"edit", "solo-1", "--overlap-ok"}, "단독"},
		{[]string{"edit", "park-1", "--no-early"}, "주차 중"},
	} {
		if _, se, rc := k.run(t, c.args...); rc != 2 || !strings.Contains(se, c.want) {
			t.Errorf("%q: 종료 2·「%s」여야 — %d %q", c.args, c.want, rc, se)
		}
	}
	if b, _ := os.ReadFile(k.data + "/queue.tsv"); string(b) != q {
		t.Fatalf("거절했는데 대기열이 바뀌었다:\n%s", b)
	}
	// 주차 줄의 겹침은 바꿀 수 있다(아직 안 뜬 줄)
	if _, se, rc := k.run(t, "edit", "park-1", "--overlap-ok"); rc != 0 || ehRow(t, k, "park-1").F(qOverlap) != "overlap-ok" || ehRow(t, k, "park-1").F(qEarly) != "parked" {
		t.Fatalf("주차 줄 --overlap-ok: %d %q", rc, se)
	}
}

// nudgeKit: 떠 있는 줄 lane-a(here의 t1)와 가짜 hc(get은 get.json·get.rc, send는 sent에 덧붙임)·가짜 hc-lane(events는 events 파일)
func nudgeKit(t *testing.T) (fillKit, string) {
	k := ehKit(t)
	f := k.home + "/fake"
	if err := os.MkdirAll(f, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := k.bin[:strings.LastIndexByte(k.bin, '/')]
	ehPut(t, bin+"/hc", `#!/bin/sh
F=`+f+`
case "$1" in
get) rc=$(cat "$F/get.rc" 2>/dev/null || echo 0); if [ "$rc" = 3 ]; then cat "$F/get.json" >&2; else cat "$F/get.json"; fi; exit "$rc" ;;
send) printf '%s|%s\n' "$2" "$3" >> "$F/sent"; exit 0 ;;
esac
exit 1
`, 0o755)
	ehPut(t, bin+"/hc-lane", "#!/bin/sh\n[ \"$1\" = events ] && { cat "+f+"/events 2>/dev/null; exit 0; }\nexit 1\n", 0o755)
	ehPut(t, k.data+"/queue.tsv", "lane-a\tapp\t/b.md\t-\t-\tup\there\tt1\t-\t1\t-\t-\tnormal\t-\t-\t-\tr1\n", 0o644)
	nudgeGet(t, f, "lane-a", "error")
	return k, f
}

func nudgeGet(t *testing.T, f, name, status string) {
	t.Helper()
	ehPut(t, f+"/get.json", `{"address":"t1","state":"live","status":"`+status+`","cwd":"/w","name":"`+name+`","created_ms":1700000000000}`, 0o644)
}

func nudgeSent(f string) []string {
	b, _ := os.ReadFile(f + "/sent")
	if len(b) == 0 {
		return nil
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

// ④ error 터미널에 정해진 글 한 번 · 간격(nudge_every 기본 10분) 안 두 번째는 안 보냄(종료 1) · 손잡이 0이면 다시 보냄 · ⑦ 일지
func TestNudgeOnceThenWait(t *testing.T) {
	k, f := nudgeKit(t)
	out, se, rc := k.run(t, "nudge", "lane-a", "--why", "API 일시 오류")
	if rc != 0 || !strings.Contains(out, "보냄 lane-a → t1") {
		t.Fatalf("nudge: 종료 0이어야 — %d %q %q", rc, out, se)
	}
	if s := nudgeSent(f); len(s) != 1 || s[0] != "t1|"+nudgeMsg {
		t.Fatalf("보낸 글: %q", s)
	}
	_, se, rc = k.run(t, "nudge", "lane-a", "다른 글")
	if rc != 1 || !strings.Contains(se, "이미 보냈다") || len(nudgeSent(f)) != 1 {
		t.Fatalf("간격 안 두 번째: 종료 1·안 보냄이어야 — %d %q %q", rc, se, nudgeSent(f))
	}
	if _, se, rc := k.run(t, "knob", "set", "nudge_every", "0", "--why", "시험"); rc != 0 {
		t.Fatalf("knob set: %d %q", rc, se)
	}
	if _, se, rc := k.run(t, "nudge", "lane-a", "다른 글"); rc != 0 || len(nudgeSent(f)) != 2 || nudgeSent(f)[1] != "t1|다른 글" {
		t.Fatalf("손잡이 0: 다시 보내야 — %d %q %q", rc, se, nudgeSent(f))
	}
	js := ehJournal(t, k, "nudge")
	if len(js) != 2 || js[0]["target"] != "lane-a" || js[0]["result"] != "보냄" || js[0]["why"] != "API 일시 오류" || !strings.Contains(js[0]["state"], "상태 error") {
		t.Fatalf("일지: %v", js)
	}
}

// ⑤ 일하는 중·입력 기다림·터미널 없음(not_found)·이미 완료를 보고한 실행·떠 있지 않은 줄·사건 못 읽음 → 안 보냄·종료 1. 「중간」 보고는 보낸다
func TestNudgeRefuses(t *testing.T) {
	k, f := nudgeKit(t)
	try := func(want string) {
		t.Helper()
		if _, se, rc := k.run(t, "nudge", "lane-a"); rc != 1 || !strings.Contains(se, want) || len(nudgeSent(f)) != 0 {
			t.Fatalf("종료 1·「%s」·안 보냄이어야 — %d %q %q", want, rc, se, nudgeSent(f))
		}
	}
	for _, st := range []string{"working", "idle", "blocked"} {
		nudgeGet(t, f, "lane-a", st)
		try("상태가 " + st)
	}
	ehPut(t, f+"/get.json", `{"error":{"kind":"not_found","message":"없음"}}`, 0o644)
	ehPut(t, f+"/get.rc", "3", 0o644)
	try("가 없다")
	ehPut(t, f+"/get.rc", "0", 0o644)
	nudgeGet(t, f, "lane-a", "done")
	ev := "2026-11-24T01:00:00Z\there\tt1\tlane-a\t%s\tabcdef12\t끝\t-\t-\tt1@1700000000000\n"
	ehPut(t, f+"/events", strings.Replace(ev, "%s", "완료", 1), 0o644)
	try("「완료」를 보고했다")
	ehPut(t, f+"/events", "모름 here\n", 0o644)
	try("사건을 읽지 못했다")
	// 다른 실행(옛 시작 시각)의 완료는 이 실행의 보고가 아니다 · 중간 보고는 멈춘 것으로 본다
	ehPut(t, f+"/events", strings.Replace(strings.Replace(ev, "%s", "완료", 1), "@1700000000000", "@1600000000000", 1)+
		strings.Replace(ev, "%s", "중간", 1), 0o644)
	if _, se, rc := k.run(t, "nudge", "lane-a"); rc != 0 || len(nudgeSent(f)) != 1 {
		t.Fatalf("중간 보고·옛 실행 완료: 보내야 — %d %q", rc, se)
	}
	ehPut(t, k.data+"/queue.tsv", "lane-a\tapp\t/b.md\t-\t-\twait\t-\t-\t-\t1\t-\t-\tnormal\t-\t-\t-\tr1\n", 0o644)
	if _, se, rc := k.run(t, "nudge", "lane-a"); rc != 1 || !strings.Contains(se, "떠 있는 레인이 아니다") {
		t.Fatalf("대기 줄: 종료 1이어야 — %d %q", rc, se)
	}
	if _, se, rc := k.run(t, "nudge", "nolane"); rc != 1 || !strings.Contains(se, "대기열에 없다") {
		t.Fatalf("없는 레인: 종료 1이어야 — %d %q", rc, se)
	}
}

// ⑥ 주소는 같은데 이름이 다르면(주소 재사용) 안 보냄
func TestNudgeNameMismatch(t *testing.T) {
	k, f := nudgeKit(t)
	nudgeGet(t, f, "other-lane", "error")
	if _, se, rc := k.run(t, "nudge", "lane-a"); rc != 1 || !strings.Contains(se, "주소 재사용") || len(nudgeSent(f)) != 0 {
		t.Fatalf("이름 다름: 종료 1·안 보냄이어야 — %d %q %q", rc, se, nudgeSent(f))
	}
}
