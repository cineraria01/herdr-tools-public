package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// knob-test-leak ①: 2026-11-24 15:00 — 지휘가 merger_idle_load 100을 걸자 병합기가 일꾼에 넘긴 HC_LANE_SPEC_IDLE_LOAD=100이 그 기계의
// 저장소 검증(go test)까지 이어져 「한가함 문턱」을 기본값으로 보는 시험 넷이 깨졌다. 운영 손잡이(그 꼴의 손잡이 파일을 둔 HOME과 환경
// 변수)를 넣고 표식 없이 이 시험 바이너리를 다시 불러도 고르기 시험이 통과한다(고치기 전: 넷 다 실패 — 환경 변수 길)
func TestKnobsDoNotLeakIntoPick(t *testing.T) {
	p := t.TempDir()
	k := filepath.Join(p, ".local/state/herdr-ops/knobs.tsv")
	if err := os.MkdirAll(filepath.Dir(k), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(k, []byte("# 독\nmerger_idle_load\t100\tpoison\tpoison\t1\t-\t1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "HERDR_SANDBOX_HOME=") && !strings.HasPrefix(kv, "HOME=") {
			env = append(env, kv)
		}
	}
	env = append(env, "HOME="+p, "XDG_STATE_HOME="+p+"/.local/state", "HC_LANE_SPEC_IDLE_LOAD=100", "HC_LANE_SPEC_BUSY_WAIT=0")
	// 넷 다 같은 길(한가함 문턱)이라 하나만 다시 분다 — 검증 시간
	c := exec.Command(os.Args[0], "-test.count=1", "-test.run=^TestPickSkipsLoadBusyMachine$")
	c.Env = env
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("운영 손잡이를 넣자 고르기 시험이 깨졌다(%v):\n%s", err, out)
	}
}
