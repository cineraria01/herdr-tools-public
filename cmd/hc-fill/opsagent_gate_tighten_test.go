package main

// ops-gate-tighten(2026-11-24): 운영 에이전트 관문의 구멍 넷 — 지은 바이너리로(main의 관문을 지나게).
// ①②③⑨ machine --rename은 어느 방식·끝난 실행·두 번째 실행에서도 거절(설정·상태 바이트 그대로) ④ 모르는 인자는 쓰기 ⑤ 읽기 꼴은 통과
// ⑥ --force(고정 손잡이 덮기)는 조치에서도 거절 ⑦ 상한이 지난 실행의 쓰기는 거절(바퀴가 안 돌아도) ⑧ 손댈 것 「모름」 바퀴에 시도 기록을 안 지운다.
// 고치기 전 코드로는 ①②③④⑥⑦⑧⑨가 실패한다(⑤는 대조)

import (
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/opsstate"
)

// tgSnap: 설정 폴더·손잡이·상태 파일 — 관문이 거절한 뒤 그대로여야 하는 것(일지·사건은 뺀다)
func tgSnap(t *testing.T, k fillKit) map[string]string {
	t.Helper()
	m := map[string]string{}
	for p, s := range fileSnap(t, k.home) {
		if strings.Contains(p, "/conf/") || strings.Contains(p, "/.local/state/") || strings.HasSuffix(p, "/ops-agent/state.json") || strings.HasSuffix(p, "/queue.tsv") {
			m[p] = s
		}
	}
	return m
}

func tgSame(t *testing.T, what string, a, b map[string]string) {
	t.Helper()
	if len(a) != len(b) {
		t.Fatalf("%s: 파일 수가 바뀜 %d → %d", what, len(a), len(b))
	}
	for p, s := range a {
		if b[p] != s {
			t.Fatalf("%s: %s가 바뀜\n전 %q\n뒤 %q", what, p, s, b[p])
		}
	}
}

func tgState(t *testing.T, k fillKit, js string) {
	t.Helper()
	ehPut(t, k.data+"/ops-agent/state.json", js, 0o644)
}

// ①②③ machine --rename: 판단만·조치·끔·끝난 실행 모두 거절, 설정·상태 바이트 그대로
func TestOpsGateRename(t *testing.T) {
	for _, c := range []struct{ name, mode, run string }{
		{"판단만", "판단만", "oa-1"}, {"조치", "조치", "oa-1"}, {"끔", "끔", "oa-1"}, {"끝난 실행", "조치", "oa-old"},
	} {
		t.Run(c.name, func(t *testing.T) {
			k, reps := gateKit(t, c.mode)
			k.env = append(k.env, oaEnvRun+"="+c.run)
			before := tgSnap(t, k)
			_, se, rc := k.run(t, "machine", "--rename", "box", "box2", "--why", "x")
			if rc != 1 || !strings.Contains(se, "사람에게 넘겼다") {
				t.Fatalf("거절이어야: %d %q", rc, se)
			}
			tgSame(t, "rename 거절 뒤", before, tgSnap(t, k))
			if r := reps(); len(r) != 1 || !strings.Contains(r[0], "사람이 정한다") {
				t.Fatalf("사건: %q", r)
			}
		})
	}
}

// ⑨ 두 번째 실행: 앞 실행이 남긴 실행 폴더·결과·못 닫은 터미널·항목 기록이 있는 채로도 ①이 같다
func TestOpsGateRenameSecondRun(t *testing.T) {
	k, _ := gateKit(t, "판단만")
	if err := os.MkdirAll(k.data+"/ops-agent/oa-0", 0o755); err != nil {
		t.Fatal(err)
	}
	ehPut(t, k.data+"/ops-agent/oa-0/result", "앞 실행 끝\n", 0o644)
	ehPut(t, k.data+"/ops-agent/oa-0/brief.md", "# 틀\n", 0o644)
	tgState(t, k, `{"run":{"id":"oa-1","addr":"t9","mode":"판단만","dir":"`+k.data+`/ops-agent/oa-1","deadline":9999999999},`+
		`"keys":{"free-wait:lane-a":{"tries":1,"done":true}},"recent":[{"id":"oa-0","result":"끝남","summary":"앞 실행 끝"}],"left":"t8"}`)
	before := tgSnap(t, k)
	if _, se, rc := k.run(t, "machine", "--rename", "box", "box2", "--why", "x"); rc != 1 || !strings.Contains(se, "사람에게 넘겼다") {
		t.Fatalf("두 번째 실행의 rename: %d %q", rc, se)
	}
	tgSame(t, "두 번째 실행", before, tgSnap(t, k))
}

// ④ 모르는 인자는 쓰기로 본다 — 판단만이면 「하려던 것」·종료 0(고치기 전에는 읽기로 보고 사용법 종료 2)
func TestOpsGateUnknownArg(t *testing.T) {
	k, _ := gateKit(t, "판단만")
	out, se, rc := k.run(t, "machine", "--zzz")
	if rc != 0 || !strings.Contains(out, "판단만") {
		t.Fatalf("모르는 인자: %d %q %q", rc, out, se)
	}
	if js := ehJournal(t, k, "하려던 것"); len(js) != 1 || js[0]["target"] != "machine" {
		t.Fatalf("일지: %+v", js)
	}
	// 읽기 명령에 모르는 인자가 붙어도 쓰기로 본다(새 쓰기 인자가 생겨도 새지 않게)
	if out, _, rc := k.run(t, "knob", "ls", "--zzz"); rc != 0 || !strings.Contains(out, "판단만") {
		t.Fatalf("knob ls --zzz: %d %q", rc, out)
	}
}

// ⑤ 대조: 읽기 꼴은 판단만에서도 관문을 지난다(「하려던 것」이 없다)
func TestOpsGateReadForms(t *testing.T) {
	k, _ := gateKit(t, "판단만")
	for _, a := range [][]string{
		{"machine"}, {"machine", "--json"}, {"machines"}, {"knob", "ls"}, {"knob", "ls", "--json"}, {"ls"}, {"ls", "--json"},
		{"status"}, {"status", "--mine"}, {"status", "--owner", "t1"}, {"state", "--json"}, {"state", "--all", "--json"}, {"advise"}, {"advise", "--json"},
		{"journal"}, {"journal", "--by", "t9", "--since", "1h", "--json"}, {"agent", "status"}, {"agent", "status", "--json"},
		{"machine", "box", "--seats", "2", "--why", "미리", "--dry-run"},
	} {
		if out, _, _ := k.run(t, a...); strings.Contains(out, "판단만 — 하지 않고") {
			t.Errorf("읽기 꼴이 막힘: %q → %q", a, out)
		}
	}
	if js := ehJournal(t, k, "하려던 것"); len(js) != 0 {
		t.Fatalf("읽기 꼴이 「하려던 것」으로: %+v", js)
	}
}

// ⑥ 조치에서도 --force(고정 손잡이 덮기)는 거절 — knob set·unset·machine. 대조: 고정이 아닌 손잡이는 --force 없이 그대로 먹는다
func TestOpsGateForce(t *testing.T) {
	k, _ := gateKit(t, "조치")
	human := append(append([]string(nil), k.env...), oaEnvRun+"=")
	hk := k
	hk.env = human
	if _, se, rc := hk.run(t, "knob", "set", "idle", "20", "--pin", "--why", "사람이 고정"); rc != 0 {
		t.Fatalf("사람 고정: %d %q", rc, se)
	}
	before := tgSnap(t, k)
	for _, a := range [][]string{
		{"knob", "set", "idle", "30", "--force", "--why", "덮기"},
		{"knob", "unset", "idle", "--force"},
		{"machine", "box", "--seats", "2", "--force", "--why", "덮기"},
	} {
		if _, se, rc := k.run(t, a...); rc != 1 || !strings.Contains(se, "사람에게 넘겼다") {
			t.Fatalf("%q: 거절이어야 — %d %q", a, rc, se)
		}
	}
	tgSame(t, "--force 거절 뒤", before, tgSnap(t, k))
	// --why 글 안의 「--force」는 인자가 아니다
	if _, se, rc := k.run(t, "knob", "set", "hold", "40", "--why", "--force 없이"); rc != 0 {
		t.Fatalf("대조(고정 아님, --force 없음): %d %q", rc, se)
	}
	if !fileHas(k.home+"/.local/state/herdr-ops/knobs.tsv", "hold") {
		t.Fatal("대조: 손잡이가 안 먹음")
	}
}

// ⑦ 상한이 지난 실행의 쓰기 명령은 거절 — 바퀴가 실행을 닫지 않았어도(상태에 run이 그대로). 읽기는 그대로
func TestOpsGateDeadline(t *testing.T) {
	for _, mode := range []string{"조치", "판단만"} {
		t.Run(mode, func(t *testing.T) {
			k, _ := gateKit(t, mode)
			tgState(t, k, `{"run":{"id":"oa-1","addr":"t9","mode":"`+mode+`","deadline":`+itoa(time.Now().Add(-time.Minute).Unix())+`},"keys":{}}`)
			q := queueOf(t, k)
			if _, se, rc := k.run(t, "prio", "lane-a", "낮음", "--why", "늦음"); rc != 1 || !strings.Contains(se, "실행 상한 지남") || queueOf(t, k) != q {
				t.Fatalf("상한 지남: %d %q", rc, se)
			}
			if js := ehJournal(t, k, "하려던 것"); len(js) != 0 {
				t.Fatalf("상한 지난 실행이 「하려던 것」을 남김: %+v", js)
			}
			if out, _, _ := k.run(t, "knob", "ls"); strings.Contains(out, "상한") && strings.Contains(out, "지남") {
				t.Fatalf("읽기가 막힘: %q", out)
			}
		})
	}
}

// ⑧ 손댈 것을 못 읽은(모름) 바퀴는 시도 기록을 지우지 않는다 — 회복 뒤 시도 수가 이어진다. 읽었고 없으면 지운다(대조)
func TestOpsAgentUnknownKeepsTries(t *testing.T) {
	oaFx(t, "ops_agent 조치", "ops_agent_tries 5")
	mls := `{"machines":[{"name":"w","state":"connected"},{"name":"x","state":"disconnected"}]}`
	ehPut(t, DATA+"/mls", mls, 0o644) // 기계 x 끊김 → 항목 machine:x
	key := "machine:x"
	now := time.Now()
	// 한 바퀴 띄우고(시도 +1) 시간 넘김으로 끝낸다(결과 없음 — 끝까지 봄 아님). 다음 바퀴 시각을 돌려준다
	round := func(at time.Time) time.Time {
		t.Helper()
		opsAgentTick(at)
		st, err := oaLoad()
		if err != nil {
			t.Fatal(err)
		}
		if st.Run == nil {
			return at.Add(time.Minute)
		}
		end := time.Unix(st.Run.Deadline, 0).Add(time.Second)
		opsAgentTick(end)
		return end.Add(time.Minute)
	}
	tries := func() int {
		st, err := oaLoad()
		if err != nil {
			t.Fatal(err)
		}
		if k := st.Keys[key]; k != nil {
			return k.Tries
		}
		return 0
	}
	at := round(now)
	if n := tries(); n != 1 {
		t.Fatalf("첫 실행 뒤 시도 1이어야: %d", n)
	}
	// hc machine ls가 먹통인 바퀴: machine:x가 목록에서 빠지고 「판정 못 함: 기계 x」가 생긴다
	if err := os.Remove(DATA + "/mls"); err != nil {
		t.Fatal(err)
	}
	if it := adviseNow(at).Items; grepKey(it, key) || !grepKey(it, "unknown:"+key) {
		t.Fatalf("시험이 헛돈다 — 모름 바퀴의 목록: %+v", it)
	}
	at = round(at)
	if n := tries(); n != 1 {
		t.Fatalf("모름 바퀴가 시도 기록을 지움: %d", n)
	}
	ehPut(t, DATA+"/mls", mls, 0o644)
	at = round(at) // 회복 — 다시 띄우면 시도 2(0부터가 아님)
	if n := tries(); n != 2 {
		t.Fatalf("회복 뒤 시도 수가 이어져야(2): %d", n)
	}
	// 대조: 목록을 읽었고 항목이 없으면 지운다(이 꾸밈에는 늘 「판정 못 함: 재료 deploy」가 있다 — 그것이 다른 항목을 붙들지 않는다)
	ehPut(t, DATA+"/mls", `{"machines":[{"name":"w","state":"connected"},{"name":"x","state":"connected"}]}`, 0o644)
	if it := adviseNow(at).Items; grepKey(it, key) || !grepKey(it, "unknown:source:deploy") {
		t.Fatalf("시험이 헛돈다 — 대조 바퀴의 목록: %+v", it)
	}
	round(at)
	if st, _ := oaLoad(); st.Keys[key] != nil {
		t.Fatalf("읽었고 없으면 지워야: %+v", st.Keys[key])
	}
}

func grepKey(it []opsstate.Item, key string) bool {
	for _, x := range it {
		if x.Key == key {
			return true
		}
	}
	return false
}
