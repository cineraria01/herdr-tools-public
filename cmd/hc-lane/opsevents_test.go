package main

// ops-events(2026-11-24): report가 운영 사건에 14칸 종류를 싣고(①), 레인 사건 줄·열쇠는 지금 꼴 그대로(⑤)

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"herdr-tools/internal/events"
)

func TestReportKind(t *testing.T) {
	d := t.TempDir()
	bin := buildLane(t, d)
	st := d + "/st"
	env := append(os.Environ(), "HOME="+d, "XDG_STATE_HOME="+st, "HC_TERMINAL_ID=", "HC_LANE=", "HC_LANE_MACHINE=m",
		"HC_LANE_JOB=", "HC_LANE_OWNER=", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	run := func(a ...string) int {
		c := exec.Command(bin, append([]string{"report"}, a...)...)
		c.Dir, c.Env = d, env
		err := c.Run()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		if err != nil {
			t.Fatal(err)
		}
		return 0
	}
	last := func() []string {
		b, _ := os.ReadFile(st + "/hc-lane/events.tsv")
		ls := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
		return strings.Split(ls[len(ls)-1], tab)
	}
	// ① 운영 이름(--kind 없이 — 옛 판 hc-fill·hc-sweep·병합기가 부르는 꼴 그대로)
	for _, c := range [][]string{
		{"중간", "-", "자리 빔", "--name", events.OpsFillIdle},
		{"막힘", "-", "병합기 죽음", "--name", "merger-p", "--key", "hf:merger:p:dead:1:-"},
		{"중간", "-", "디스크", "--name", events.OpsDiskLow, "--key", "hc-disk-low:x"},
		{"중간", "-", "손으로 운영", "--name", "x", "--kind", "ops"},
	} {
		if rc := run(c...); rc != 0 {
			t.Fatalf("%v: 종료 %d", c, rc)
		}
		if f := last(); len(f) != 14 || f[12] != "-" || f[13] != events.KindOps || events.KindOf(f) != events.KindOps {
			t.Fatalf("%v: 운영 종류가 안 실림: %q", c, f)
		}
	}
	// ⑤ 레인 사건은 12칸 그대로(14칸 없음) — 줄이 곧 열쇠라 배포 전후 같다
	if rc := run("중간", "-", "레인 일", "--name", "merger-alive"); rc != 0 {
		t.Fatal(rc)
	}
	f := last()
	if len(f) != 12 || events.KindOf(f) != events.KindLane {
		t.Fatalf("레인 사건 꼴이 바뀜: %q", f)
	}
	want := events.Event{Time: f[0], Machine: "m", Lane: "merger-alive", State: "중간", Msg: "레인 일"}.Line()
	if strings.Join(f, tab) != want || lineKey(want) != lineKey(strings.Join(f, tab)) {
		t.Fatalf("레인 사건 줄이 옛 꼴과 다르다:\n%q\n%q", strings.Join(f, tab), want)
	}
	// 운영 이름을 레인으로 적기(--kind lane)는 14칸 lane
	if rc := run("중간", "-", "x", "--name", events.OpsFill, "--kind", "lane"); rc != 0 || events.KindOf(last()) != events.KindLane {
		t.Fatalf("--kind lane: %d %q", rc, last())
	}
	// 모르는 종류는 사용 오류, 사건 안 씀
	n := len(last())
	b0, _ := os.ReadFile(st + "/hc-lane/events.tsv")
	if rc := run("중간", "-", "x", "--kind", "운영"); rc != 2 {
		t.Fatalf("--kind 운영: 종료 %d", rc)
	}
	if b1, _ := os.ReadFile(st + "/hc-lane/events.tsv"); string(b1) != string(b0) || n == 0 {
		t.Fatal("잘못된 --kind에 사건을 썼다")
	}
}

// 1000바이트 예산: 긴 운영 사건도 14칸을 잃지 않는다(한 줄 칸을 줄임)
func TestWithRepoKindBudget(t *testing.T) {
	e := events.Event{Time: "2026-11-24T00:00:00Z", Machine: "m", Lane: events.OpsFillIdle, State: "중간", Msg: strings.Repeat("가", 600)}
	for _, rid := range []string{"", "github.com/o/r"} {
		l := withRepoKind(e, rid, "")
		f := strings.Split(l, tab)
		if len(l) > events.MaxLine || len(f) != 14 || f[13] != events.KindOps {
			t.Fatalf("rid %q: %d바이트 %d칸 %q", rid, len(l), len(f), f[len(f)-1])
		}
		if rid != "" && f[12] != rid {
			t.Fatalf("저장소 칸: %q", f[12])
		}
	}
}
