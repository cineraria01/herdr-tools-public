package main

// agent-skill-guides ⑤: 모르는 인자에 가까운 인자가 없으면 둘째 줄 「다음:」(안내를 가리킴), 가까우면 전처럼 「대신 …?」만 — 첫 줄·종료 코드 그대로

import (
	"strings"
	"testing"
)

func TestUnknownArgNextHint(t *testing.T) {
	bin := buildLane(t, t.TempDir())
	_, _, env := cuEnv(t)
	r := runBin(t, bin, env, "events", "--bogus")
	ls := strings.Split(strings.TrimRight(r.err, "\n"), "\n")
	if r.rc != 1 || len(ls) != 2 || ls[0] != "hc-lane: 알 수 없는 인자: --bogus" || !strings.HasPrefix(ls[1], "hc-lane: 다음: hc-lane --skill") {
		t.Fatalf("가까운 인자 없음: 종료 1·첫 줄 그대로·「다음:」이어야 — %+v", r)
	}
	r = runBin(t, bin, env, "stats", "--sinse", "1h")
	if r.rc != 1 || !strings.Contains(r.err, "--sinse 대신 --since?") || strings.Contains(r.err, "다음:") {
		t.Fatalf("가까운 인자: 「대신 …?」만이어야 — %+v", r)
	}
}
