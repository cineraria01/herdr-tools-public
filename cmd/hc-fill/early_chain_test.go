package main

// early-own-chain 시험: 먼저 띄움은 후보의 선행 사슬(--after·--then 받은 줄)이 쥔 예약을 겹침으로 세지 않는다 — 사슬 밖 레인과의
// 겹침·일반 배치·선행이 아직 안 뜬 때는 그대로. 「실제 변경 못 읽음」 줄에 까닭

import (
	"os"
	"strings"
	"testing"
)

func ecSetup(t *testing.T) func(n, s string) string {
	t.Helper()
	actSave(t)
	oe, oc := OVEARLY, OVCHAIN
	t.Cleanup(func() { OVEARLY, OVCHAIN = oe, oc })
	d := t.TempDir()
	confLines = lines("local here\nrepo app here " + d + "/app\n")
	writeT(t, d+"/app/cmd/x.go", "package x\n")
	LOCAL, OVM, TERMS, ACT, ACTPARK = "here", "block", nil, nil, nil
	QEDGES, THENMAP, OVEARLY = nil, nil, false
	return func(n, s string) string {
		writeT(t, d+"/"+n+".md", s)
		return d + "/" + n + ".md"
	}
}

func ecCheck(n, brief, after string, early bool) bool {
	OVWAIT, OVCHAIN, OVEARLY = nil, nil, early
	defer func() { OVEARLY = false }()
	return ovlCheck(n, "app", brief, after, "-", "-", "-")
}

// ① A(예약 X) ← B(--after A, 예약 X): 먼저 띄움이면 뜨고(선행 예약은 까닭으로 남김), 일반 배치 판정은 지금처럼 막힘
func TestEarlyChainOwnPred(t *testing.T) {
	bf := ecSetup(t)
	OREC = []Row{{"A", "app", "here", "t1", "s", "b", ",-,", "-"}}
	OWN = []Row{{"app", "A", "R", "cmd/x.go"}}
	QEDGES = []Row{{"B", ",A,"}}
	b := bf("B", "파일: cmd/x.go\n")
	if ecCheck("B", b, "A", false) {
		t.Error("일반 배치: 선행 A의 예약과 겹치는데 떴다(지금 동작이 바뀜)")
	}
	if !ecCheck("B", b, "A", true) {
		t.Errorf("먼저 띄움: 자기 선행 A의 예약에 막힘 (OVWAIT=%q)", rowsBar(OVWAIT))
	} else if got := rowsBar(OVCHAIN); got != "A\t예약\tcmd/x.go|" {
		t.Errorf("선행 예약 겹침 기록: %q", got)
	}
	if got := rowsBar(OVWARN); got != "" {
		t.Errorf("선행 예약 겹침이 겹침 경고로도 나감: %q", got)
	}
}

// ② 사슬 밖 C(예약 X)는 먼저 띄움이어도 그대로 겹침 대기
func TestEarlyChainOutsider(t *testing.T) {
	bf := ecSetup(t)
	OREC = []Row{{"A", "app", "here", "t1", "s", "b", ",-,", "-"}, {"C", "app", "here", "t2", "s", "b", ",-,", "-"}}
	OWN = []Row{{"app", "A", "R", "cmd/x.go"}, {"app", "C", "R", "cmd/x.go"}}
	QEDGES = []Row{{"B", ",A,"}}
	if ecCheck("B", bf("B", "파일: cmd/x.go\n"), "A", true) {
		t.Error("먼저 띄움: 사슬 밖 C의 예약과 겹치는데 떴다")
	} else if got := rowsBar(OVWAIT); got != "B\tC\tcmd/x.go\t-\tapp|" {
		t.Errorf("겹침 대기 기록: %q", got)
	}
}

// ③ 두 단 사슬 A ← B ← C(먼저 뜬 B도 X를 쥠), 그리고 --then으로 B에 이어 붙은 이름을 기다리는 D
func TestEarlyChainTwoHops(t *testing.T) {
	bf := ecSetup(t)
	OREC = []Row{{"A", "app", "here", "t1", "s", "b", ",-,", "-"}, {"B", "app", "here", "t2", "s", "b", ",A,", "-"}}
	OWN = []Row{{"app", "A", "R", "cmd/x.go"}, {"app", "B", "R", "cmd/x.go"}}
	QEDGES = []Row{{"C", ",B,"}, {"D", ",b2,"}}
	THENMAP = []Row{{"b2", "B"}}
	if !ecCheck("C", bf("C", "파일: cmd/x.go\n"), "B", true) {
		t.Errorf("먼저 띄움: 두 단 사슬의 선행 예약에 막힘 (OVWAIT=%q)", rowsBar(OVWAIT))
	} else if got := rowsBar(OVCHAIN); !strings.Contains(got, "A\t예약\tcmd/x.go|") || !strings.Contains(got, "B\t예약\tcmd/x.go|") {
		t.Errorf("두 단 사슬 기록: %q", got)
	}
	if !ecCheck("D", bf("D", "파일: cmd/x.go\n"), "b2", true) {
		t.Errorf("먼저 띄움: --then 받은 줄 B를 건넌 사슬의 예약에 막힘 (OVWAIT=%q)", rowsBar(OVWAIT))
	}
}

// ⑤ 선행이 대기열에서 아직 안 떴으면(반송돼 wait로 돌아옴 등) 먼저 띄움은 지금처럼 건너뛴다 — 겹침 판정까지 가지 않음
func TestEarlyChainPredWaiting(t *testing.T) {
	bf := ecSetup(t)
	setData(t)
	ob, oe2, orows := ELIST, ECAP, ROWS
	t.Cleanup(func() { ELIST, ECAP, ROWS = ob, oe2, orows })
	b := bf("B", "파일: cmd/x.go\n")
	a := pad16("A", "app", b, "-", "-", "wait").Set(17, "r1-1-1")
	bb := pad16("B", "app", b, "A", "-", "wait").Set(17, "r2-1-1")
	if err := os.WriteFile(Q, []byte(a.Line()+"\n"+bb.Line()+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ROWS, ELIST = []Row{a, bb}, ",B=origin/main"
	OREC, OWN = nil, nil
	out := dupCapture(t, earlyRows)
	if !strings.Contains(out, "먼저 띄움 건너뜀 B: 선행 A 가 대기열에서 아직 안 떴다") {
		t.Errorf("선행이 wait인데 건너뛰지 않음:\n%s", out)
	}
}

// ⑥ 「실제 변경 못 읽음」 줄에 레인마다 까닭 — 떠 있는데 작업 트리도 제출·주차 가지도 없는 레인
func TestActMissWhy(t *testing.T) {
	actSave(t)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	d := t.TempDir()
	org, repo := d+"/o.git", d+"/app"
	actGitT(t, d, "init", "-q", "--bare", "-b", "main", org)
	actGitT(t, d, "clone", "-q", org, repo)
	writeT(t, repo+"/base.txt", "b\n")
	actGitT(t, repo, "add", "base.txt")
	actGitT(t, repo, "commit", "-q", "-m", "base")
	actGitT(t, repo, "push", "-q", "origin", "HEAD:main")
	actGitT(t, repo, "remote", "set-head", "origin", "main")
	confLines = lines("local here\nrepo app here " + repo + "\n")
	LOCAL, dry, FETCHED, FETCHBAD = "here", false, "", ""
	OREC = []Row{{"nowt", "app", "here", "t9", "s", "b", ",-,", "-"}}
	TERMS = []Term{{M: "here", Addr: "t9", Name: "nowt"}}
	out := dupCapture(t, func() { actTick(nil) })
	if !strings.Contains(out, "실제 변경 못 읽음 nowt(작업 트리 없음") || !strings.Contains(out, "제출·주차 가지 없음") {
		t.Errorf("못 읽음 줄에 까닭이 없음:\n%s", out)
	}
}

// ⑥ 기계 단위 작업 트리 조회 실패의 까닭: 종료 코드와 끝 줄(없으면 없음), 종료 0인데 끝 표식이 없으면 잘림
func TestShFail(t *testing.T) {
	for _, c := range []struct {
		out  string
		rc   int
		want string
	}{
		{"", 1, "종료 1, 출력 없음"},
		{"app\tl\t+\nfatal: 무엇\n", 128, "종료 128, 끝 줄 「fatal: 무엇」"},
		{"app\tl\t+\n", 0, "끝 표식 없음(출력 잘림), 끝 줄 「app\tl\t+」"},
	} {
		if got := shFail(c.out, c.rc); got != c.want {
			t.Errorf("shFail(%q, %d) = %q, 바람 %q", c.out, c.rc, got, c.want)
		}
	}
}
