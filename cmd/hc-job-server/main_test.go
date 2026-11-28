package main

// 계약 시험 — 가짜 hc(HC_JOB_HC)로 한 틱(HC_JOB_ONCE)만. 2026-11-23 mini-m1 D1 루프: /loop가 최대 시간에 닿아 Claude는 프롬프트에
// 살아 있고(live, status done) 회차가 없는데, 감독이 live를 늘 채택해 11시간 다시 띄우지 않았다 — 멈춘 루프(done·idle이고 status_ms가
// HC_JOB_STALE초 넘게 그대로)는 hc kill, 그 밖의 live는 건드리지 않는다

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/clitest"
)

// TestMain: 끝나면 clitest.Bin 폴더를 지운다
func TestMain(m *testing.M) { clitest.Main(m) }

// fakeHC: 부른 인자를 T_LOG에. get은 T_GET을(T_GET_ERR가 있으면 그것을 표준 오류에도) 찍고 T_GET_RC로 끝난다
const fakeHC = `#!/bin/sh
echo "$*" >>"$T_LOG"
case $1 in
  get) printf '%s\n' "$T_GET"; [ -z "$T_GET_ERR" ] || printf '%s\n' "$T_GET_ERR" >&2; exit "${T_GET_RC:-0}" ;;
esac
exit 0
`

// fakeSweep: hc 곁의 가짜 hc-sweep(ht-rest-one-gate 쉼 묻기) — 부른 인자를 T_LOG에(「sweep …」), --rest면 진짜처럼 hc get을 PATH 이름으로
// 불러(못 찾으면 「hc get 실패」 모름 — 감사 atp7a3w M1) T_REST 줄(없으면 쉼), T_REST_RC가 있으면 그 종료(옛 판은 2)
const fakeSweep = `#!/bin/sh
echo "sweep $*" >>"$T_LOG"
[ "$1" = --rest ] || exit 2
[ -z "$T_REST_RC" ] || exit "$T_REST_RC"
hc get "$2" --json >/dev/null 2>&1 || { printf 'REST\tunknown\t-\thc get 실패\tend\n'; exit 0; }
if [ -n "$T_REST" ]; then printf '%s\n' "$T_REST"; else printf 'REST\tquiet\t-\t-\tend\n'; fi
`

// tick: 한 틱 돌리고 가짜 hc가 받은 호출들
func tick(t *testing.T, get string, rc int, kv ...string) string {
	t.Helper()
	T := clitest.TempDir(t)
	hc := filepath.Join(T, "hc")
	clitest.Put(t, hc, fakeHC)
	clitest.Chmod(t, hc, 0o755)
	clitest.Put(t, filepath.Join(T, "hc-sweep"), fakeSweep)
	clitest.Chmod(t, filepath.Join(T, "hc-sweep"), 0o755)
	script := filepath.Join(T, "resume.sh")
	clitest.Put(t, script, "#!/bin/sh\nexit 0\n") // --resume 없음: pgrep을 보지 않는다
	log := filepath.Join(T, "log")
	env := append([]string{"HOME=" + T, "HC_JOB_HC=" + hc, "HC_JOB_ONCE=1", "T_LOG=" + log, "T_GET=" + get,
		fmt.Sprintf("T_GET_RC=%d", rc), "HC_JOB_STALE", "HC_JOB_GAP", "HC_JOB_LAST", "T_REST=", "T_REST_RC="}, kv...)
	r := clitest.Cmd{Env: env, Max: 30 * time.Second}.Run(clitest.Bin(t, "hc-job-server"), "job1", "lbl", T, script)
	if r.RC != 0 {
		t.Fatalf("종료 %d\n%s%s", r.RC, r.Out, r.Err)
	}
	t.Logf("출력: %s", r.Out)
	return clitest.Read(log)
}

func get(state, status string, ageSec int64) string {
	ms := time.Now().UnixMilli() - ageSec*1000
	st := "null"
	if status != "" {
		st = fmt.Sprintf(`{"app":"claude-code","state":%q}`, status)
	}
	return fmt.Sprintf(`{"address":"t9","name":"job1","state":%q,"status":%s,"status_ms":%d,"pid":1}`, state, st, ms)
}

func TestStep(t *testing.T) {
	now := fmt.Sprint(time.Now().Unix())
	cases := []struct {
		name     string
		get      string
		rc       int
		env      []string
		kill, up bool // hc kill t9 · hc run
	}{
		{"멈춘 루프(done, 2시간)", get("live", "done", 7200), 0, nil, true, false},
		{"멈춘 루프(idle, 2시간)", get("live", "idle", 7200), 0, nil, true, false},
		{"회차 사이(done, 50분)", get("live", "done", 3000), 0, nil, false, false},
		{"도는 중(working, 2시간)", get("live", "working", 7200), 0, nil, false, false},
		{"사람 답 기다림(blocked, 2시간)", get("live", "blocked", 7200), 0, nil, false, false},
		{"status 없음(Claude 아님)", get("live", "", 7200), 0, nil, false, false},
		{"HC_JOB_STALE=0이면 끔", get("live", "done", 7200), 0, []string{"HC_JOB_STALE=0"}, false, false},
		{"HC_JOB_STALE로 늘림", get("live", "done", 7200), 0, []string{"HC_JOB_STALE=9000"}, false, false},
		{"HC_JOB_GAP 안이면 안 끔", get("live", "done", 7200), 0, []string{"HC_JOB_LAST=" + now}, false, false},
		{"absent → 띄움", `{"error":{"kind":"not_found"}}`, 3, nil, false, true},
		{"finished → rm 뒤 띄움", get("finished", "done", 7200), 0, nil, false, true},
		{"코어 오류 → 아무것도", "", 1, nil, false, false},
		// ht-rest-one-gate: 상태는 done이 2시간이어도 안에서 배경 작업·하위 에이전트가 돌면 멈춘 루프가 아니다. 모름(옛 판 hc-sweep·꼴 틀림)은
		// HC_JOB_STALE의 네 배(기본 6시간) 넘게 그대로일 때만 끈다
		{"활동(배경 작업) → 안 끔", get("live", "done", 7200), 0, []string{"T_REST=REST\tbusy\ts1\t배경 작업 1개 도는 중\tend"}, false, false},
		{"모름(옛 판 hc-sweep) 2시간 → 안 끔", get("live", "done", 7200), 0, []string{"T_REST_RC=2"}, false, false},
		{"모름(잘린 답) 2시간 → 안 끔", get("live", "idle", 7200), 0, []string{"T_REST=REST\tqui"}, false, false},
		{"모름 7시간 → 끔", get("live", "done", 7*3600), 0, []string{"T_REST_RC=2"}, true, false},
		// 감사 atp7a3w M1: launchd의 짧은 PATH(HC_JOB_HC 폴더 없음)에서도 묻기 안의 hc가 찾아져 쉼 → 끔(90분 그대로, 6시간으로 밀리지 않음)
		{"짧은 PATH에서도 쉼 → 끔", get("live", "done", 7200), 0, []string{"PATH=/usr/bin:/bin"}, true, false},
		{"활동 7시간 → 안 끔(판정이 r3로 쉼을 정한다)", get("live", "done", 7*3600), 0, []string{"T_REST=REST\tbusy\ts1\t배경 작업 1개 도는 중\tend"}, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			calls := tick(t, c.get, c.rc, c.env...)
			if got := clitest.HasLine(calls, "kill t9"); got != c.kill {
				t.Errorf("hc kill t9 = %v, 바람 %v\n%s", got, c.kill, calls)
			}
			if got := strings.Contains(calls, "\nrun ") || strings.HasPrefix(calls, "run "); got != c.up {
				t.Errorf("hc run = %v, 바람 %v\n%s", got, c.up, calls)
			}
		})
	}
	if calls := tick(t, get("finished", "done", 1), 0); !clitest.HasLine(calls, "rm t9") {
		t.Errorf("finished인데 hc rm t9 없음\n%s", calls)
	}
}

func TestVersion(t *testing.T) {
	if r := (clitest.Cmd{}).Run(clitest.Bin(t, "hc-job-server"), "--version"); r.RC != 0 || !strings.HasPrefix(r.Out, "hc-job-server") {
		t.Fatalf("--version: %d %q", r.RC, r.Out)
	}
	if r := (clitest.Cmd{}).Run(clitest.Bin(t, "hc-job-server"), "a"); r.RC != 2 {
		t.Fatalf("인자 모자람은 2: %d", r.RC)
	}
}
