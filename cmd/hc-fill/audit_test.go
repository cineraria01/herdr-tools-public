package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// 가짜 git SHA 40자. 한 줄에 hex 40자가 이어지면 공개 내보내기의 비밀 꼴에 걸려 둘로 끊는다(publish-lint-go)
const auTSha = "abc1234000000000000" + "000000000000000000000"

// auTEv: 사건 줄(시각 기계 주소 이름 상태 sha 한줄 8칸 9칸 실행)
func auTEv(t, addr, name, st, sha, msg, run string) string {
	return strings.Join([]string{t, "m", addr, name, st, sha, msg, "1", "-", run}, "\t")
}

func TestAuditVerdictBinding(t *testing.T) {
	an, aid := "audit-a1", "a1"
	ok := auTEv("2026-11-21T01:00:00Z", "m/audit-a1", an, "판정", auTSha, "감사 완료 a1 높음 0·중간 0·낮음 1", "-")
	EV = []string{
		// 6칸이 40자 sha가 아님, 다른 차수 ID 둘
		auTEv("2026-11-21T00:59:00Z", "m/audit-a1", an, "판정", "abc1234", "감사 완료 a1 높음 0·중간 0·낮음 0", "-"),
		auTEv("2026-11-21T00:59:30Z", "m/audit-a1", an, "판정", auTSha, "감사 완료 a1x 높음 0·중간 0·낮음 0", "-"),
		auTEv("2026-11-21T00:59:40Z", "m/audit-a1", an, "판정", auTSha, "감사 완료 azzzzzz-zzzzzz 높음 0·중간 0·낮음 0", "-"),
	}
	if v := auVerdict(an, aid, auTSha); v != "" {
		t.Fatalf("sha·차수 ID가 다른 판정을 받았다: %q", v)
	}
	EV = append(EV, ok, auTEv("2026-11-21T01:01:00Z", "m/audit-a1", "audit-b2", "판정", auTSha, "감사 완료 a1 높음 0·중간 0·낮음 0", "-"))
	if v := auVerdict(an, aid, auTSha); v != ok {
		t.Fatalf("이 차수 판정을 못 골랐다: %q", v)
	}
}

func TestAuditRunlast(t *testing.T) {
	run := "w1@1700000000000" // 2023-11-14T22:13:20Z
	same := auTEv("2026-11-21T02:00:00Z", "m/w1", "w1", "완료", "bbb2222", "같은 실행", run)
	EV = []string{
		auTEv("2026-11-21T01:00:00Z", "m/w1", "w1", "완료", "aaa1111", "앞 실행", "w1@1600000000000"),
		same,
		auTEv("2026-11-21T03:00:00Z", "m/w1", "w1", "완료", "ccc3333", "다른 실행 키", "w1@1800000000000"),
	}
	if l := runlast("w1", "m/w1", run, "완료"); l != same {
		t.Fatalf("10칸 실행 키가 다른 사건을 골랐거나 같은 실행을 못 골랐다: %q", l)
	}
	// 10칸 없는 옛 사건: 그 실행 시작 이후면 고른다, 이전이면 안 고른다
	old := "2026-11-21T04:00:00Z\tm\tm/w1\tw1\t완료\tddd4444\t옛 꼴"
	EV = []string{"2020-01-01T00:00:00Z\tm\tm/w1\tw1\t완료\teee5555\t시작 전", old}
	if l := runlast("w1", "m/w1", run, "완료"); l != old {
		t.Fatalf("옛 꼴 사건 고르기: %q", l)
	}
	EV = EV[:1]
	if l := runlast("w1", "m/w1", run, "완료"); l != "" {
		t.Fatalf("실행 시작 전 옛 사건을 골랐다: %q", l)
	}
	if got := auRunStart("-"); got != "9" {
		t.Fatalf("못 푸는 실행 키의 시작은 9: %q", got)
	}
}

// 판정이 sha·차수 ID가 다르면 a_queued는 판정 없음으로 센다(외부 명령 없이: 감사 터미널이 이 바퀴 목록에 없음)
func TestAuditQueuedRejectsForeignVerdict(t *testing.T) {
	DATA = t.TempDir()
	Q, AT, LOCAL, dry, TERMS, AUNK = DATA+"/queue.tsv", DATA+"/audit.tsv", "m", false, nil, nil
	q := strings.Join([]string{"audit-a1", "app", "b", "-", "m", "up", "m", "m/audit-a1", "-", "t", "-", "-", "urgent", "no-early", "audit=w1", "-", "r1"}, "\t")
	at := strings.Join([]string{"a1", "w1", "1", "w1@1700000000000", auTSha, "m", "m/w1", "queued", "0", "-", "0", "0", "-", "-", "0", "-", "-"}, "\t")
	if err := os.WriteFile(Q, []byte(q+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(AT, []byte(at+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	EV = []string{auTEv("2026-11-21T01:00:00Z", "m/audit-a1", "audit-a1", "판정", strings.Repeat("f", 40), "감사 완료 a1 높음 0·중간 0·낮음 0", "-")}
	r := arow("a1")
	auAid, auLn, auRnd, auPrun, auSha, auAm, auPa, auAst = r.F(1), r.F(2), r.F(3), r.F(4), r.F(5), r.F(6), r.F(7), r.F(8)
	auAfx, auAv, auAwk, auAwt, auAlc, auAlw, auAnv, auAfl = r.F(9), r.F(10), r.F(11), r.F(12), r.F(13), r.F(14), r.F(15), r.F(16)
	aRound()
	if got := arow("a1"); got.F(8) != "queued" || got.F(15) != "1" {
		t.Fatalf("다른 sha 판정을 받았거나 판정 없음 바퀴를 안 셌다: %v", got)
	}
}

// 완료 뒤 멈춤: 받은편지 줄(감사 칸 -)의 실행 마지막 사건이 완료이고 done_stall분 지나면 모든 지휘에 키 붙은 막힘 한 번,
// 그 sha 차수가 열려 있으면 알리지 않음
func TestAuditStallTick(t *testing.T) {
	DATA = t.TempDir()
	Q, AT, LOCAL, dry, EVBAD, CONN = DATA+"/queue.tsv", DATA+"/audit.tsv", "m", false, false, nil
	confLines = []string{"done_stall 30", "repo app m " + DATA}
	TERMS, FETCHED, FETCHBAD = []Term{{M: "m", Addr: "m/inq", Name: "inq"}}, "", "" // 터미널 살아 있음(「끝남」 아님)
	log := DATA + "/hc-lane.log"
	if err := os.WriteFile(DATA+"/hc-lane", []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >>"+log+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 가짜 git: 원격 있음·기본 origin/main, 완료 sha는 이 클론에 있고 기본 브랜치에는 없음(merge-base 1)
	// 큐 ls-remote --exit-code는 진짜 git처럼 맞는 참조 없음 = 종료 2(빈 출력에 0이면 gitx.LsRemote가 모름으로 읽는다), config --get 값 없음 = 종료 1
	gitf := "#!/bin/sh\ncase \" $* \" in *\" symbolic-ref \"*) echo origin/main;; *\" merge-base \"*) exit 1;; *\" ls-remote --exit-code \"*) exit 2;; *\" config --get \"*) exit 1;; esac\nexit 0\n"
	if err := os.WriteFile(DATA+"/git", []byte(gitf), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
	q := strings.Join([]string{"inq", "app", "b", "-", "m", "up", "m", "m/inq", "-", "t", "box#rq1", "-", "normal", "-", "-", "-", "r1"}, "\t")
	if err := os.WriteFile(Q, []byte(q+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(DATA+"/try.tsv", []byte("inq\tr1\tm\t-\t-\tm/inq\tinq@1700000000000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	EV = []string{auTEv(time.Now().Add(-2*time.Hour).UTC().Format("2006-01-02T15:04:05Z"), "m/inq", "inq", "완료", "abcdef1234", "끝", "inq@1700000000000")}
	at := strings.Join([]string{"a1", "inq", "1", "inq@1700000000000", "abcdef1234" + strings.Repeat("0", 30), "m", "m/inq", "queued", "0", "-", "0", "0", "-", "-", "0", "-", "-"}, "\t")
	if err := os.WriteFile(AT, []byte(at+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stallTick()
	if _, err := os.Stat(log); err == nil {
		t.Fatal("감사 차수가 열려 있는데 알렸다")
	}
	if err := os.WriteFile(AT, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	stallTick()
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); !strings.Contains(got, "report 막힘 - inq 완료 뒤 멈춤 — ") || !strings.Contains(got, "(m/inq, 완료 abcdef12) --name inq --key hf:inq:stall:abcdef1234") || strings.Contains(got, "--to") {
		t.Fatalf("알림 꼴: %q", got)
	}
	confLines = []string{"done_stall 0"}
	if err := os.Remove(log); err != nil {
		t.Fatal(err)
	}
	stallTick()
	if _, err := os.Stat(log); err == nil {
		t.Fatal("done_stall 0인데 알렸다")
	}
}

func TestAuditShq(t *testing.T) {
	for in, want := range map[string]string{
		"/Users/other/src/app.wt/w1": "/Users/other/src/app.wt/w1",
		"~/work/app.wt/w1":           "~/work/app.wt/w1",
		"/Users/a b/app.wt/w1":       "'/Users/a b/app.wt/w1'",
		"~/my dir/it's.wt/w1":        `~/'my dir/it'\''s.wt/w1'`,
		"/tmp/x$(id).wt/w1":          "'/tmp/x$(id).wt/w1'",
	} {
		if got := auShq(in); got != want {
			t.Errorf("auShq(%q) = %q, 바라는 값 %q", in, got, want)
		}
	}
}
