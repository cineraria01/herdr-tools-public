package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/opsstate"
)

// 끝난 레인 판정은 단계 글이 아니라 더 할 일이 있는가로(2026-11-24 지휘 지시 — ops-state-trim 뒤 실제 대기열에서 레인 절 199줄 가운데
// 179줄이 착지·정리까지 끝났는데 단계 글이 「감사 통과 받음」이라 숨지 않았다)
func TestStateDoneByLastEvent(t *testing.T) {
	stFx(t, `{"machines":[{"name":"w","state":"connected"},{"name":"x","state":"connected"}]}`)
	now := time.Now()
	add := now.Add(-5 * time.Hour).Format("2006-01-02T15:04")
	ago := now.Add(-2 * time.Hour).Format("15:04")
	row := func(name, addr string) string {
		return strings.Join([]string{name, "app", "b", "-", "-", "up", "w", addr, "-", add, "-", "m/t1", "normal", "-", "auto", "-", "r-" + name}, "\t") + "\n"
	}
	aud := func(name string) string { // 17칸: 감사 통과를 레인이 받음(taken·pass)
		return strings.Join([]string{"a" + name, name, "1", "-", "-", "-", "-", "taken", "0", "pass", "-", "-", "-", "-", "-", "-", "-"}, "\t") + "\n"
	}
	var q, l, a strings.Builder
	for i := range 100 {
		n := "done-" + itoa(int64(i))
		q.WriteString(row(n, "w/t"+itoa(int64(100+i))))
		l.WriteString(n + "\t정리됨\t-\t" + ago + "\thc-lane clean\t-\tfresh\tr-" + n + "\n")
		a.WriteString(aud(n))
	}
	// 감사 통과를 받았지만 아직 착지 전: 하나는 터미널이 살아 있고, 하나는 착지 사건이 없다(마지막 완료)
	q.WriteString(row("keep-live", "w/t5") + row("keep-nolanding", "w/t6"))
	l.WriteString("keep-live\t착지\tabcd1234\t" + ago + "\t-\t-\tfresh\tr-keep-live\n")
	l.WriteString("keep-nolanding\t완료\tabcd1234\t" + ago + "\t-\t-\tfresh\tr-keep-nolanding\n")
	a.WriteString(aud("keep-live") + aud("keep-nolanding"))
	for f, s := range map[string]string{"queue.tsv": q.String(), "last.tsv": l.String(), "audit.tsv": a.String(),
		"lsjson": `{"terminals":[{"address":"w/t5","name":"keep-live","status":"idle"}]}`} {
		if err := os.WriteFile(DATA+"/"+f, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := stateCollect(now)
	if len(s.Lanes) != 102 || !strings.HasPrefix(s.Lanes[0].Stage, "감사 통과 받음") {
		t.Fatalf("틀: 레인 %d줄, 첫 단계 %q", len(s.Lanes), s.Lanes[0].Stage)
	}
	tr := opsstate.Trim(s, 60)
	var names []string
	for _, x := range tr.Lanes {
		names = append(names, x.Name)
	}
	if strings.Join(names, ",") != "keep-live,keep-nolanding" || tr.Hidden.Lanes != 100 {
		t.Fatalf("기본 출력 레인: %v (숨김 %d)", names, tr.Hidden.Lanes)
	}
	var b bytes.Buffer
	if err := opsstate.Human(&b, tr); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(b.String(), "\n"); n > 80 {
		t.Fatalf("기본 출력 %d줄(80줄 안이어야):\n%s", n, b.String())
	}
}
