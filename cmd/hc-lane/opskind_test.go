package main

// ops-events(2026-11-24): events|wait --kind ops|lane 거름(③), watch 글 머리 「운영: …」·끝 「→ <다음>」(④)

import (
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/events"
)

// ③ events --kind: 14칸 운영 · 옛 꼴 운영 이름(14칸 없음) · 레인 사건을 가른다
func TestCliEventsKind(t *testing.T) {
	bin := buildLane(t, t.TempDir())
	d, _, env := cuEnv(t)
	now := time.Now()
	ops14 := strings.TrimSuffix(cuEvent(now.Add(-4*time.Second), "x", "중간"), "\n") + "\t-\t-\t-\t-\t-\t" + events.KindOps + "\n"
	ev := ops14 + cuEvent(now.Add(-3*time.Second), events.OpsFillIdle, "중간") + // 옛 꼴 운영
		cuEvent(now.Add(-2*time.Second), "lane-a", "완료") + cuEvent(now.Add(-time.Second), "merger-alive", "착지")
	if err := os.WriteFile(d+"/state/hc-lane/events.tsv", []byte(ev), 0o644); err != nil {
		t.Fatal(err)
	}
	r := runBin(t, bin, env, "events", "--kind", "ops")
	if r.rc != 0 || strings.Count(r.out, "\n") != 2 || !strings.Contains(r.out, "\tx\t") || !strings.Contains(r.out, "\t"+events.OpsFillIdle+"\t") {
		t.Fatalf("--kind ops: %+v", r)
	}
	r = runBin(t, bin, env, "events", "--kind", "lane")
	if r.rc != 0 || strings.Count(r.out, "\n") != 2 || !strings.Contains(r.out, "\tlane-a\t") || !strings.Contains(r.out, "\tmerger-alive\t") {
		t.Fatalf("--kind lane: %+v", r)
	}
	if r = runBin(t, bin, env, "events", "--kind", "ops", "--lane", "lane-a"); r.rc != 0 || r.out != "" {
		t.Fatalf("--kind와 --lane은 둘 다 맞아야: %+v", r)
	}
	if r = runBin(t, bin, env, "events", "--kind", "운영"); r.rc != 1 || r.out != "" {
		t.Fatalf("모르는 종류는 인자 오류: %+v", r)
	}
	if r = runBin(t, bin, env, "watch", "--kind", "ops", "--to", "w1"); r.rc != 1 {
		t.Fatalf("watch는 --kind를 받지 않는다: %+v", r)
	}
}

// ③ wait --kind lane → 운영 사건에는 깨지 않고, 거른 운영 사건은 커서에 안 적힌다
func TestCliWaitKind(t *testing.T) {
	bin := buildLane(t, t.TempDir())
	d, _, env := cuEnv(t)
	evp := d + "/state/hc-lane/events.tsv"
	if err := os.WriteFile(evp, []byte(cuEvent(time.Now().Add(-time.Second), events.OpsFillIdle, "중간")), 0o644); err != nil {
		t.Fatal(err)
	}
	w := []string{"wait", "--state", "중간", "--timeout", "0"}
	if r := runBin(t, bin, env, append(w, "--kind", "lane")...); r.rc != 75 || r.out != "" {
		t.Fatalf("운영 사건에는 깨지 않는다(75): %+v", r)
	}
	if r := runBin(t, bin, env, append(w, "--kind", "ops")...); r.rc != 0 || !strings.Contains(r.out, events.OpsFillIdle) {
		t.Fatalf("--kind ops는 운영 사건에 깬다: %+v", r)
	}
}

// ④ watch 한 사건 글: 운영이면 머리 「운영: 」·끝 「 → <다음>」, 레인 사건은 지금 꼴 그대로
func TestWatchPartKind(t *testing.T) {
	ln := func(term, lane, key string, extra ...string) string {
		f := []string{"T", "m1", term, lane, "중간", "-", "글", "-", "-", "-", "-", key}
		return strings.Join(append(f, extra...), tab)
	}
	cases := []struct{ l, want string }{
		{ln("t1", "lane-a", "-"), "lane-a 중간 글 (t1)"},
		{ln("-", events.OpsFillIdle, "-"), "운영: hc-fill-idle 중간 글 (m1) → hc-fill ls"},
		{ln("-", "merger-p", "hf:merger:p:dead:1:-"), "운영: merger-p 중간 글 (m1) → hc-fill status"},
		{ln("t1", "merger-alive", "-"), "merger-alive 중간 글 (t1)"},
		{ln("-", "x", "-", "-", events.KindOps), "운영: x 중간 글 (m1)"}, // 표에 없는 운영 이름 — 다음 명령 없음
	}
	for _, c := range cases {
		if got := wpart(c.l); got != c.want {
			t.Errorf("wpart(%q)\n got %q\nwant %q", c.l, got, c.want)
		}
	}
	// 옛 7칸 레인 사건 — 지금 꼴 그대로
	if got := wpart("T\tm1\tt1\tlane-a\t완료\tabc\t글"); got != "lane-a 완료 abc 글 (t1)" {
		t.Errorf("7칸: %q", got)
	}
}
