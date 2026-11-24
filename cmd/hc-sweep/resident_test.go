package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/resident"
)

// 상주 판 점검 — 가짜 ps. 옛 판 상주에 사건 한 번, 같은 프로세스에 되풀이 없음, ps 실패는 모름(사건 없음)

type fakePS struct {
	out string
	rc  int
}

func (f *fakePS) run(_ time.Duration, _ []string, name string, _ ...string) result {
	if filepath.Base(name) != "ps" {
		return result{rc: 1}
	}
	return result{out: f.out, rc: f.rc}
}

// ⑤ 같은 프로세스에 두 번 알리지 않는다(①의 사건 글·다시 띄우기 줄도 본다)
func TestResidentOnce(t *testing.T) {
	dir := t.TempDir()
	start := time.Now().Add(-20 * time.Hour).Truncate(time.Second)
	f := &fakePS{out: "  41 " + start.Format("Mon Jan _2 15:04:05 2006") + " /bin/hc-lane watch --to t3yx\n"}
	var sent []string
	rep := func(msg, key string) bool { sent = append(sent, msg+"|"+key); return true }
	now := time.Now()
	residentCheck("worker-a", dir, false, f.run, rep, now, 5*time.Minute)
	residentCheck("worker-a", dir, false, f.run, rep, now.Add(time.Minute), 5*time.Minute)
	if len(sent) != 1 || !strings.Contains(sent[0], "hc-lane watch") || !strings.Contains(sent[0], "다시 띄우기: ") || !strings.Contains(sent[0], "hc-stale-resident:worker-a:41:") {
		t.Fatalf("사건 한 번이어야: %v", sent)
	}
	r, err := resident.LoadReport(resident.ReportPath(dir))
	if err != nil || len(r.Findings) != 1 {
		t.Fatalf("결과 한 장: %+v %v", r, err)
	}
}

// 진짜 ps(LC_ALL=C) 출력을 푼다 — 이 시험 프로세스가 시작 시각과 함께 보인다
func TestResidentRealPS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS ps 꼴")
	}
	res := run(15*time.Second, append(os.Environ(), "LC_ALL=C"), "/bin/ps", "-axww", "-o", "pid=,lstart=,command=")
	if res.err != nil || res.rc != 0 {
		t.Fatalf("ps: %v %d", res.err, res.rc)
	}
	for _, p := range resident.ParsePS(res.out, time.Local) {
		if p.Pid == os.Getpid() {
			if time.Since(p.Start) < 0 || time.Since(p.Start) > time.Hour {
				t.Fatalf("시작 시각: %v", p.Start)
			}
			return
		}
	}
	t.Fatalf("ps 출력에서 이 프로세스(%d)를 못 찾음", os.Getpid())
}

// ps를 못 읽으면 모름 — 사건 없음, 줄에 모름
func TestResidentPSFailIsUnknown(t *testing.T) {
	dir := t.TempDir()
	var sent []string
	rep := func(msg, key string) bool { sent = append(sent, msg); return true }
	lines := residentCheck("worker-a", dir, false, (&fakePS{rc: 1}).run, rep, time.Now(), 5*time.Minute)
	if len(sent) != 0 || len(lines) == 0 || !strings.Contains(strings.Join(lines, "\n"), "모름") {
		t.Fatalf("ps 실패: 사건 %v 줄 %v", sent, lines)
	}
}
