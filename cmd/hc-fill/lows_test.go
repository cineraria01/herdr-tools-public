package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// 낮음 후속 시험 차림: 가짜 hc-lane(인자를 로그에)·hc(get은 살아 있는 실행), 대기열에 레인 줄, 감사 기록에 통과 차수, 보고 파일
type lowsT struct {
	t        *testing.T
	log, rep string
}

const lowsTRep = "# 감사\n판정: 높음 0 · 중간 0 · 낮음 2\n## 지적\n### [낮음] 1. 가 고칠 것\n가 본문\n### [낮음] 2. 나 고칠 것\n나 본문\n## 확인한 것\n- 문제 없음\n"

func lowsSetup(t *testing.T, lane, flag, q15, q11 string) *lowsT {
	DATA = t.TempDir()
	Q, AT, LOCAL, dry, P, EVBAD, CONN, AUNK = DATA+"/queue.tsv", DATA+"/audit.tsv", "m", false, "", false, nil, nil
	confLines = []string{"lows app", "repo app m " + DATA}
	x := &lowsT{t: t, log: DATA + "/hc-lane.log", rep: DATA + "/reports/audit-a1.md"}
	lowsW(t, DATA+"/hc-lane", "#!/bin/sh\nprintf '%s\\n' \"$*\" >>"+x.log+"\n")
	lowsW(t, DATA+"/hc", "#!/bin/sh\necho '{\"created_ms\":1700000000000,\"name\":\""+lane+"\",\"state\":\"running\"}'\n")
	if err := os.Chmod(DATA+"/hc-lane", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(DATA+"/hc", 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
	if err := os.MkdirAll(DATA+"/reports", 0o755); err != nil {
		t.Fatal(err)
	}
	lowsW(t, x.rep, lowsTRep)
	lowsW(t, Q, strings.Join([]string{lane, "app", "b", "-", "m", "up", "m", "m/" + lane, "-", "t", q11, "t1", "normal", "-", q15, "-", "r1"}, "\t")+"\n")
	lowsW(t, AT, strings.Join([]string{"a1", lane, "1", lane + "@1700000000000", auTSha, "m", "m/" + lane, "taken", "0", "pass", "0", "0", "-", "-", "0", flag, x.rep}, "\t")+"\n")
	return x
}

func lowsW(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (x *lowsT) logText() string {
	b, err := os.ReadFile(x.log)
	if err != nil {
		return ""
	}
	return string(b)
}

// 낮음 있음: <레인>-lows를 넣는다(선행 레인·주인 같음·우선 낮음·먼저 띄움 끔·감사 칸 -), 지시문에 낮음 원문, 주인에 한 줄, 다시 안 넣음
func TestLowsAdds(t *testing.T) {
	x := lowsSetup(t, "w1", "lows=2", "auto", "-")
	lowsTick()
	r := qrow("w1-lows")
	if r == nil {
		t.Fatalf("후속을 안 넣었다: %q", readLines(Q))
	}
	if r.F(2) != "app" || r.F(4) != "w1" || r.F(6) != "wait" || r.F(12) != "t1" || r.F(13) != "low" || r.F(14) != "no-early" || r.F(15) != "-" {
		t.Fatalf("후속 줄 칸: %q", r)
	}
	b, err := os.ReadFile(r.F(3))
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"### [낮음] 1. 가 고칠 것\n가 본문", "### [낮음] 2. 나 고칠 것\n나 본문", "고칠 필요가 없으면 까닭 한 줄", "runs/w1-run.md"} {
		if !strings.Contains(string(b), h) {
			t.Errorf("지시문에 %q 없음:\n%s", h, b)
		}
	}
	if strings.Contains(string(b), "문제 없음") {
		t.Errorf("낮음 밖 글이 들어갔다:\n%s", b)
	}
	if got := x.logText(); !strings.Contains(got, "report 중간 - 후속 넣음 w1-lows: 낮음 2개") || !strings.Contains(got, "--name w1 --to t1 --key hf:a1:lows") {
		t.Fatalf("주인 알림 꼴: %q", got)
	}
	if f := arow("a1").F(16); f != "lows=2,lowsq" {
		t.Fatalf("표시: %q", f)
	}
	lowsTick()
	if n := len(readLines(Q)); n != 2 {
		t.Fatalf("다시 넣었다: %d줄", n)
	}
}

// 판정 받을 때(aQueued): 통과 + 낮음 n이면 16칸에 lows=n, 낮음 0이나 지적(중간 있음)이면 안 적는다
func TestLowsVerdictFlag(t *testing.T) {
	for _, c := range []struct{ msg, want string }{
		{"높음 0·중간 0·낮음 3", "lows=3"},
		{"높음 0·중간 0·낮음 0", "-"},
		{"높음 0·중간 1·낮음 2", "-"},
	} {
		lowsSetup(t, "w1", "-", "auto", "-")
		lowsW(t, AT, strings.Join([]string{"a1", "w1", "1", "w1@1700000000000", auTSha, "m", "m/w1", "queued", "0", "-", "0", "0", "-", "-", "0", "-", "-"}, "\t")+"\n")
		q := strings.Join([]string{"audit-a1", "app", "b", "-", "m", "up", "m", "m/audit-a1", "-", "t", "-", "t1", "urgent", "no-early", "audit=w1", "-", "r2"}, "\t")
		lowsW(t, Q, string(mustRead(t, Q))+q+"\n")
		TERMS = []Term{{M: "m", Addr: "m/audit-a1", Name: "audit-a1"}, {M: "m", Addr: "m/w1", Name: "w1"}}
		EV = []string{
			auTEv("2026-11-21T01:00:00Z", "m/w1", "w1", "완료", auTSha[:8], "끝", "w1@1700000000000"),
			auTEv("2026-11-21T01:10:00Z", "m/audit-a1", "audit-a1", "판정", auTSha, "감사 완료 a1 "+c.msg, "-"),
		}
		r := arow("a1")
		auAid, auLn, auRnd, auPrun, auSha, auAm, auPa, auAst = r.F(1), r.F(2), r.F(3), r.F(4), r.F(5), r.F(6), r.F(7), r.F(8)
		auAfx, auAv, auAwk, auAwt, auAlc, auAlw, auAnv, auAfl = r.F(9), r.F(10), r.F(11), r.F(12), r.F(13), r.F(14), r.F(15), r.F(16)
		aRound()
		if got := arow("a1"); got.F(8) == "queued" || got.F(16) != c.want {
			t.Errorf("%s: 상태 %s 표시 %q, 바람 %q", c.msg, got.F(8), got.F(16), c.want)
		}
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// 넣지 않는 것: 표시 없음(낮음 0)·-lows 사슬·꺼진 프로젝트·설정 없음·받은편지·lows_min 미만 — 대기열 그대로, 알림 없음
func TestLowsSkips(t *testing.T) {
	cases := []struct {
		name, lane, flag, q11 string
		conf                  []string
		mark                  bool // lowsx 표시가 남아야 함
	}{
		{"낮음 없음", "w1", "-", "-", nil, false},
		{"사슬", "w1-lows", "lows=2", "-", nil, true},
		{"꺼진 프로젝트", "w1", "lows=2", "-", []string{"lows other", "repo app m /x"}, true},
		{"설정 없음", "w1", "lows=2", "-", []string{"repo app m /x"}, false},
		{"받은편지", "w1", "lows=2", "box#rq1", nil, true},
		{"lows_min 미만", "w1", "lows=2", "-", []string{"lows *", "lows_min 3"}, true},
	}
	for _, c := range cases {
		x := lowsSetup(t, c.lane, c.flag, "auto", c.q11)
		if c.conf != nil {
			confLines = c.conf
		}
		lowsTick()
		if n := len(readLines(Q)); n != 1 {
			t.Errorf("%s: 넣었다 %q", c.name, readLines(Q))
		}
		if got := x.logText(); got != "" {
			t.Errorf("%s: 알렸다 %q", c.name, got)
		}
		if f := arow("a1").F(16); strings.Contains(f, "lowsx") != c.mark {
			t.Errorf("%s: 표시 %q", c.name, f)
		}
	}
}

// 시간당 상한: 지난 한 시간에 lows_hour개 넣었으면 미룸(표시 없음 — 다음 바퀴에), 한 시간 지난 기록은 안 셈
func TestLowsHourCap(t *testing.T) {
	lowsSetup(t, "w1", "lows=2", "auto", "-")
	confLines = []string{"lows app", "lows_hour 1"}
	lowsW(t, DATA+"/lows.tsv", itoa64(nowUnix()-60)+"\tax\tz\tz-lows\t1\n")
	lowsTick()
	if qrow("w1-lows") != nil || arow("a1").F(16) != "lows=2" {
		t.Fatalf("상한인데 넣었거나 표시했다: %q %q", readLines(Q), arow("a1").F(16))
	}
	lowsW(t, DATA+"/lows.tsv", itoa64(nowUnix()-3700)+"\tax\tz\tz-lows\t1\n")
	lowsTick()
	if qrow("w1-lows") == nil {
		t.Fatal("한 시간 지난 기록을 셌다")
	}
}

// 보고 꼴 못 읽음(뽑은 수가 판정 낮음 수와 다름)·보고 없음: 넣지 않고 주인에 한 줄, lowsx
func TestLowsUnreadable(t *testing.T) {
	for _, c := range []struct{ name, rep, want string }{
		{"꼴 다름", "# 감사\n낮음이 둘 있었다는 글만\n", "보고에서 낮음 2개 중 0개를 뽑음"},
		{"보고 없음", "", "보고 파일 없음"},
	} {
		x := lowsSetup(t, "w1", "lows=2", "auto", "-")
		if c.rep == "" {
			if err := os.Remove(x.rep); err != nil {
				t.Fatal(err)
			}
		} else {
			lowsW(t, x.rep, c.rep)
		}
		lowsTick()
		if qrow("w1-lows") != nil {
			t.Errorf("%s: 넣었다", c.name)
		}
		if got := x.logText(); !strings.Contains(got, "report 중간 - 후속 못 넣음 w1-lows: "+c.want) || !strings.Contains(got, "--to t1 --key hf:a1:lows") {
			t.Errorf("%s: 알림 꼴 %q", c.name, got)
		}
		if f := arow("a1").F(16); f != "lows=2,lowsx" {
			t.Errorf("%s: 표시 %q", c.name, f)
		}
	}
}

// 이름: 32자를 넘으면 앞 22자-cksum4-lows(32자)
func TestLowsName(t *testing.T) {
	if got := lowsName("w1"); got != "w1-lows" {
		t.Fatalf("짧은 이름: %q", got)
	}
	long := strings.Repeat("a", 30)
	got := lowsName(long)
	if len(got) != 32 || !strings.HasPrefix(got, strings.Repeat("a", 22)+"-") || !strings.HasSuffix(got, "-lows") || !okname(got) {
		t.Fatalf("긴 이름: %q", got)
	}
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }
