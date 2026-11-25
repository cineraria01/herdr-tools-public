package main

// 시험 고아 점검(2026-11-26 ht-test-orphan-leak, docs/hc-sweep/test-orphans.md): 시험 바이너리가 끊겨 죽으면 새 무리로 띄운 자식이
// 부모 1로 남아 며칠씩 돈다(워커 맥의 hc-merger 3일, herdr-sync --check 30시간 CPU 275%, 지휘 기계의 fake-agent 9일). 건강 점검
// 길에서 이 기계의 부모 없는(ppid 1) 프로세스 가운데 인자에 시험 임시 경로(resident.TestPath)가 든 것이 손잡이 sweep_test_orphan분을
// 넘겼으면 운영 사건 hc-test-orphan 한 번(같은 pid·시작 시각에 한 번). 끄지 않는다 — 끄기는 사람·지휘가 정확한 pid로.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"herdr-tools/internal/atomic"
	"herdr-tools/internal/resident"
)

// testOrphan: ps로 본 시험 고아 하나
type testOrphan struct {
	pid   int
	start time.Time
	cpu   string
	cmd   string
}

// testOrphans: LC_ALL=C ps -axww -o pid=,ppid=,%cpu=,lstart=,command= 출력에서 ppid 1이고 시험 경로 인자가 든 것(풀 수 없는 줄은 건너뜀)
func testOrphans(out string) []testOrphan {
	var o []testOrphan
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) < 9 || f[1] != "1" || !resident.TestArgs(f[8:]) {
			continue
		}
		ps := resident.ParsePS(f[0]+" "+strings.Join(f[3:], " "), time.Local)
		if len(ps) != 1 {
			continue
		}
		o = append(o, testOrphan{pid: ps[0].Pid, start: ps[0].Start, cpu: f[2], cmd: strings.Join(f[8:], " ")})
	}
	return o
}

// testOrphanCheck: 한 바퀴. state는 보낸 키 목록 파일(지금 남은 고아의 키만 남긴다). 찍을 줄들
func testOrphanCheck(machine, state string, dry bool, r runner, report func(msg, key string) bool, now time.Time, age time.Duration) []string {
	res := r(15*time.Second, append(os.Environ(), "LC_ALL=C"), "/bin/ps", "-axww", "-o", "pid=,ppid=,%cpu=,lstart=,command=")
	if res.err != nil || res.rc != 0 {
		return []string{"시험 고아 점검: 모름(ps 실패 " + rcText(res) + ") — 다음 바퀴에"}
	}
	sent := map[string]bool{}
	if b, err := os.ReadFile(state); err == nil {
		for _, k := range strings.Fields(string(b)) {
			sent[k] = true
		}
	}
	var lines, keep []string
	for _, o := range testOrphans(res.out) {
		if now.Sub(o.start) < age {
			continue
		}
		key := fmt.Sprintf("hc-test-orphan:%s:%d:%d", machine, o.pid, o.start.Unix())
		if sent[key] {
			keep = append(keep, key)
			continue
		}
		cmd := []rune(o.cmd)
		if len(cmd) > 160 {
			cmd = cmd[:160]
		}
		msg := fmt.Sprintf("부모 없는 시험 고아: pid %d (시작 %s, %s째, CPU %s%%) %s — 끄기는 그 pid로: kill %d (%s)",
			o.pid, o.start.Format("01-02 15:04"), now.Sub(o.start).Round(time.Minute), o.cpu, string(cmd), o.pid, machine)
		switch {
		case dry:
			lines = append(lines, "시험 고아 점검 (보낼 것): "+msg)
		case report(msg, key):
			keep = append(keep, key)
			lines = append(lines, "시험 고아 점검: "+msg)
		default:
			lines = append(lines, "시험 고아 점검: "+msg+" — 알림 실패(hc-lane report), 다음 바퀴에 같은 키로 다시")
		}
	}
	if dry {
		return lines
	}
	sort.Strings(keep)
	err := os.MkdirAll(filepath.Dir(state), 0o755)
	if err == nil {
		err = atomic.WriteFile(state, []byte(strings.Join(keep, "\n")+"\n"), 0o644)
	}
	if err != nil { // 보낸 키를 잊으면 다음 바퀴가 한 번 더 알린다(hc-lane이 같은 키는 한 번만 적는다) — 놓치지 않는 쪽
		lines = append(lines, "시험 고아 점검 기록을 못 씀: "+err.Error())
	}
	return lines
}
