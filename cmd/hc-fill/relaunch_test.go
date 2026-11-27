package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// ht-relaunch-unify: 14칸 값 ↔ 뜻(relMode)은 왕복해도 같은 글자다 — 옛 판이 쓴 값·모르는 값(새 판이 쓴 것)도 되쓰면 그대로라
// 섞여 도는 동안 값을 지우지 않는다. 뜻은 두 갈래(먼저 띄움 여부 · 다시 띄울 출처)로만 읽힌다
func TestRelModeRoundTrip(t *testing.T) {
	want := map[string]relMode{
		"": {}, "-": {Early: "-"}, "no-early": mNoEarly, "early": mEarly, "stacked": mStacked,
		"parked": {Src: srcPark}, "resumed": {Src: srcPark, Ran: true}, "bounced": {Src: srcBounce}, "zzz": {Early: "zzz"},
	}
	for v, m := range want {
		if got := modeOf(v); got != m {
			t.Errorf("modeOf(%q) = %+v, 바람 %+v", v, got, m)
		}
		if got := modeOf(v).val(); got != v {
			t.Errorf("modeOf(%q).val() = %q", v, got)
		}
		if r := setMode(Row{"n"}, modeOf(v)); rowMode(r).val() != v {
			t.Errorf("setMode·rowMode %q: %v", v, r)
		}
	}
	for _, c := range []struct {
		m    relMode
		want string
	}{{mParked, "주차"}, {mResumed, "주차"}, {mBounced, "제출"}, {mEarly, ""}, {modeOf("-"), ""}} {
		if got := c.m.end(); got != c.want {
			t.Errorf("%+v.end() = %q, 바람 %q", c.m, got, c.want)
		}
	}
}

// 이번 띄우기에 적는 14칸: 쌓아 띄움 > 반송(표시 지움) > 주차에서 이어 > 먼저 띄움 > 그대로 — 옛 launch2의 덮어쓰기 차례 그대로
func TestLaunchedMode(t *testing.T) {
	osrc, ost := RSRC, STACK
	defer func() { RSRC, STACK = osrc, ost }()
	for _, c := range []struct {
		src   relSrc
		stack string
		early bool
		want  string
	}{{srcNew, "", false, ""}, {srcNew, "", true, "early"}, {srcPark, "", true, "resumed"}, {srcBounce, "", true, "-"},
		{srcPark, "a@x", false, "stacked"}, {srcBounce, "a@x", true, "stacked"}} {
		RSRC, STACK = c.src, c.stack
		if got := launchedMode(c.early); got != c.want {
			t.Errorf("%+v: %q", c, got)
		}
	}
}

// 14칸은 relaunch.go(뜻)·cols.go(칸 번호)·queue.go(qset — 칸을 그대로 쓰는 한 곳) 밖에서 이름으로도 읽거나 쓰지 않는다.
// 다른 곳은 rowMode·setMode·val로 — 다시 띄우는 길을 더할 때 새 값을 이 파일 한 곳에 더하게(ht-relaunch-unify)
func TestQEarlyOnlyInRelaunch(t *testing.T) {
	fs, err := filepath.Glob("*.go")
	if err != nil || len(fs) == 0 {
		t.Fatalf("꾸러미 파일 목록: %v %d", err, len(fs))
	}
	re := regexp.MustCompile(`\bqEarly\b`)
	var hits []string
	for _, f := range fs {
		if strings.HasSuffix(f, "_test.go") || f == "relaunch.go" || f == "cols.go" || f == "queue.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, l := range strings.Split(string(b), "\n") {
			if re.MatchString(l) {
				hits = append(hits, f+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(l))
			}
		}
	}
	if len(hits) > 0 {
		t.Fatalf("14칸(qEarly)을 relaunch.go 밖에서 읽음 %d곳 — rowMode·setMode를 쓴다:\n%s", len(hits), strings.Join(hits, "\n"))
	}
}
