package main

import (
	"os"
	"strings"
	"testing"
)

// 2차 이상 감사가 앞 차수의 지적과 지휘 지시를 알고 시작하게 하는 자리표시(prev-findings-recheck)

const prTSha2 = "def5678000000000000" + "000000000000000000000"

// prTSetup: abrief가 외부 명령 없이(가짜 git) 돌 자리. 틀 tpl, 감사 기록 rows(17칸 줄)
func prTSetup(t *testing.T, tpl string, rows ...[]string) {
	t.Helper()
	DATA = t.TempDir()
	Q, AT, LOCAL, dry, TERMS, AUNK = DATA+"/queue.tsv", DATA+"/audit.tsv", "m", false, nil, nil
	AUD = DATA + "/tpl.md"
	confLines = []string{"repo app m " + DATA + "/app"}
	gitf := "#!/bin/sh\ncase \" $* \" in *\" symbolic-ref \"*) echo origin/main;; esac\nexit 0\n"
	if err := os.WriteFile(DATA+"/git", []byte(gitf), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
	if err := os.WriteFile(AUD, []byte(tpl), 0o644); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(strings.Join(r, "\t") + "\n")
	}
	if err := os.WriteFile(AT, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// prTRow: 감사 기록 한 줄(ID 레인 차수 부모실행 sha 기계 부모주소 상태 지적수 판정 깨움수 기다린초 마지막확인 마지막깨움 판정없음 표시 비고)
func prTRow(aid, rnd, sha, st, fx, v, fl, rep string) []string {
	return []string{aid, "w1", rnd, "w1@1700000000000", sha, "m", "m/w1", st, fx, v, "0", "0", "-", "-", "0", fl, rep}
}

func prTBrief(t *testing.T, aid string) string {
	t.Helper()
	if !abrief(aid, "w1", "m", prTSha2, "app") {
		t.Fatal("abrief 실패")
	}
	b, err := os.ReadFile(DATA + "/briefs/audit/" + aid + ".md")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const prTTpl = "차수[{round}] 앞sha[{prev_sha}] 앞보고[{prev_report}] 앞판정[{prev_verdict}] 지휘[{coord_notes}] 범위 {sha} {audit_id}\n"

// ① 1차: 자리표시는 빈 값(차수만 1)
func TestPrevRoundFirst(t *testing.T) {
	prTSetup(t, prTTpl, prTRow("a1", "1", prTSha2, "opening", "0", "-", "-", "-"))
	if got, want := prTBrief(t, "a1"), "차수[1] 앞sha[] 앞보고[] 앞판정[] 지휘[] 범위 "+prTSha2+" a1\n"; got != want {
		t.Fatalf("1차 지시문\n got %q\nwant %q", got, want)
	}
}

// ② 2차: 앞 차수가 본 sha·보고 경로·판정 수가 채워진다
func TestPrevRoundSecond(t *testing.T) {
	prTSetup(t, prTTpl,
		prTRow("a0", "1", auTSha, "taken", "1", "fix", "hml=1.2.0", "~/.local/share/hc-fill/reports/audit-a0.md"),
		prTRow("a1", "2", prTSha2, "opening", "1", "-", "-", "-"))
	got := prTBrief(t, "a1")
	want := "차수[2] 앞sha[" + auTSha + "] 앞보고[~/.local/share/hc-fill/reports/audit-a0.md] 앞판정[높음 1·중간 2·낮음 0] 지휘[] 범위 " + prTSha2 + " a1\n"
	if got != want {
		t.Fatalf("2차 지시문\n got %q\nwant %q", got, want)
	}
}

// ③ 틀에 새 자리표시가 없으면 지금과 같은 지시문(2차여도)
func TestPrevRoundOldTemplate(t *testing.T) {
	old := "감사 {lane} @{machine} — {sha} {audit_id}\n범위 `git -C {worktree} diff origin/{base}...{sha}` 보고 {report}\n"
	prTSetup(t, old,
		prTRow("a0", "1", auTSha, "taken", "1", "fix", "hml=1.2.0", "~/.local/share/hc-fill/reports/audit-a0.md"),
		prTRow("a1", "2", prTSha2, "opening", "1", "-", "-", "-"))
	want := "감사 w1 @m — " + prTSha2 + " a1\n범위 `git -C " + DATA + "/app.wt/w1 diff origin/main..." + prTSha2 + "` 보고 ~/.local/share/hc-fill/reports/audit-a1.md\n"
	if got := prTBrief(t, "a1"); got != want {
		t.Fatalf("옛 틀 지시문이 바뀌었다\n got %q\nwant %q", got, want)
	}
}

// ⑤ 바로 앞 차수가 판정 없이 superseded(새 sha)면 그 앞 차수(판정을 받은 차수)의 값
func TestPrevRoundSkipsSuperseded(t *testing.T) {
	prTSetup(t, prTTpl,
		prTRow("a0", "1", auTSha, "taken", "1", "fix", "hml=0.1.3", "~/.local/share/hc-fill/reports/audit-a0.md"),
		prTRow("ax", "2", strings.Repeat("9", 40), "superseded", "1", "-", "-", "-"),
		prTRow("a1", "3", prTSha2, "opening", "1", "-", "-", "-"))
	got := prTBrief(t, "a1")
	if !strings.Contains(got, "차수[3] 앞sha["+auTSha+"] 앞보고[~/.local/share/hc-fill/reports/audit-a0.md] 앞판정[높음 0·중간 1·낮음 3]") {
		t.Fatalf("superseded를 건너뛰지 않았다: %q", got)
	}
}

// ④ 지휘 지시 남김: 레인에 같은 글을 보내고(hc send), 레인 기계에 파일을 두고, 다음 감사 지시문에 그 경로
func TestPrevRoundCoordNote(t *testing.T) {
	prTSetup(t, prTTpl,
		prTRow("a0", "1", auTSha, "taken", "1", "fix", "hml=1.0.0", "~/.local/share/hc-fill/reports/audit-a0.md"),
		prTRow("a1", "2", prTSha2, "opening", "1", "-", "-", "-"))
	t.Setenv("HOME", DATA+"/home")
	HOME = DATA + "/home"
	log := DATA + "/hc.log"
	if err := os.WriteFile(DATA+"/hc", []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >>"+log+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	q := strings.Join([]string{"w1", "app", "b", "-", "m", "up", "m", "m/w1", "-", "t", "-", "-", "normal", "-", "-", "auto", "r1"}, "\t")
	if err := os.WriteFile(Q, []byte(q+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	note := cmds["note"]
	if note == nil {
		t.Fatal("hc-fill note 명령이 없다")
	}
	note([]string{"w1", "캐시 키는 sha로만 — 경로 정규화는 이 레인 밖"})
	b, err := os.ReadFile(log)
	if err != nil || !strings.Contains(string(b), "send m/w1 ") || !strings.Contains(string(b), "캐시 키는 sha로만") {
		t.Fatalf("레인에 지시를 보내지 않았다: %q %v", b, err)
	}
	nf, err := os.ReadFile(DATA + "/home/.local/share/hc-fill/verdicts/w1/coord-notes.md")
	if err != nil || !strings.Contains(string(nf), "캐시 키는 sha로만") {
		t.Fatalf("레인 기계에 지휘 지시 파일이 없다: %q %v", nf, err)
	}
	if got := prTBrief(t, "a1"); !strings.Contains(got, "지휘[~/.local/share/hc-fill/verdicts/w1/coord-notes.md]") {
		t.Fatalf("다음 감사 지시문에 지휘 지시 경로가 없다: %q", got)
	}
}
