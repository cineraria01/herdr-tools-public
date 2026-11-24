package main

import (
	"os"
	"strings"
	"testing"
)

// watch-idle-open(2026-11-24): 바퀴마다 열린 줄(떠 있음·주차함)이 있는 주인 지휘마다 hc-lane coord --watch를 한 번 — watch가 한가로
// 끝난 뒤에도 그 지휘의 착지·막힘 알림을 받게 한다. 지휘 확인·에이전트 확인·watch 하나만은 hc-lane이 한다(⑤는 hc-lane 시험)
func TestWatchOwners(t *testing.T) {
	setData(t)
	dir := t.TempDir()
	fake := "#!/bin/sh\necho \"$*\" >>\"" + dir + "/calls\"\n[ \"$3\" = t-gone ] && exit 1\nexit 0\n"
	if err := os.WriteFile(dir+"/hc-lane", []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	row := func(name, st, owner, early string) string {
		return pad16(name, "app", "b", "-", "-", st, "-", "-", "-", "t", "-", owner, "normal", early, "-", "-").Line() + "\n"
	}
	q := row("a", "up", "t-up", "-") + // ④ 떠 있는 레인
		row("a2", "up", "t-up", "-") + // 같은 주인은 한 번
		row("b", "wait", "t-park", "parked") + // ④ 주차한 레인
		row("c", "wait", "t-wait", "-") + // ⑥ 아직 안 띄운 줄만
		row("d", "up", "-", "-") + // 주인 없음
		row("e", "up", "t-gone", "-") + // 지휘 없음(hc-lane 종료 1) — 넘어간다
		row("f", "up", "t-landed", "-") + // ⑥ 마지막 보고가 착지인 떠 있음 줄(hc-fill rm 전까지 남는다)은 닫힘
		row("g", "up", "t-judged", "-") // ⑥ 판정도 닫힘
	if err := os.WriteFile(Q, []byte(q), 0o644); err != nil {
		t.Fatal(err)
	}
	st := map[string]string{"f": "착지", "g": "판정", "a": "완료"}
	last := ""
	for _, r := range qRows() { // 줄 ID는 읽을 때 붙는다 — last.tsv는 레인·줄 ID로 맞춘다(evTick)
		if s := st[r.F(qName)]; s != "" {
			last += strings.Join([]string{r.F(qName), s, "-", "10:00", "-", "10:01", "fresh", r.F(qRID)}, "\t") + "\n"
		}
	}
	if err := os.WriteFile(DATA+"/last.tsv", []byte(last), 0o644); err != nil {
		t.Fatal(err)
	}
	watchOwners()
	b, _ := os.ReadFile(dir + "/calls")
	got := strings.TrimSpace(string(b))
	want := "coord --watch t-up\ncoord --watch t-park\ncoord --watch t-gone"
	if got != want {
		t.Fatalf("부른 것:\n%s\n바람:\n%s", got, want)
	}
}
