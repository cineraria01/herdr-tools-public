//go:build !windows

package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// ht-probe-cordon(docs/hc-fill/cordon.md 「점검 항목」): 원격 쓰기·에이전트 풀·키체인·임시 폴더·시계가 같은 격리 길로.
// 틀은 cordon_test.go의 ctFix — 가짜 hc-lane은 $CD/<기계>(점검 줄 포함)를 그대로, --remote-auth면 $CD/<기계>.auth(remote-write 줄 포함)를 싣는다.
// 고정 자료의 쓰기 거절 글은 2026-11-28 실제 확인(git push --dry-run, 자격 도우미 끔)에서 받은 글 — 호스트는 시험 대역 값

const (
	rwGood = "remote-write 1 0 git.example.test -"
	rwBad  = "remote-write 1 128 git.example.test error: unable to read askpass response from '/usr/bin/false' | fatal: could not read Username for 'https://git.example.test': terminal prompts disabled"
	rwDNS  = "remote-write 1 128 nonexistent.invalid fatal: unable to access 'https://nonexistent.invalid/o/r.git/': Could not resolve host: nonexistent.invalid"
)

// withProbe: 좋은 읽기 점검 출력 사이에 점검 줄들
func withProbe(ls ...string) string {
	return "load=0.10 cpus=8 verifies=0 gates=0\n" + strings.Join(ls, "\n") + "\nrepo-read 1 ok\nrepo-read 2 ok\nrepo-read end 2"
}

// piItem: 항목 하나 — 나쁨·좋음을 b에 싣는 법
type piItem struct {
	ck        *cordonCk
	bad, good func(c *ctx)
	why       string // 기계 줄의 격리 까닭 머리
}

func piItems() []piItem {
	probe := func(l string) func(c *ctx) { return func(c *ctx) { c.set("b", withProbe(l)) } }
	write := func(l string) func(c *ctx) { return func(c *ctx) { c.auth("b", raGood+"\n"+l) } }
	return []piItem{
		{cdWrite, write(rwBad), write(rwGood), "원격 쓰기 거절 — p git.example.test: fatal: could not read Username"},
		{cdPool, probe("probe pool bad 쓸 계정 0"), probe("probe pool ok 쓸 계정 2"), "에이전트 풀 빔 — 쓸 계정 0"},
		{cdKeychain, probe("probe keychain bad rc 36 User interaction is not allowed"), probe("probe keychain ok 열림"),
			"키체인 막힘 — rc 36 User interaction is not allowed"},
		{cdTmp, probe("probe tmp bad /h/.hctmp.noindex 쓰기가 5s 안에 안 끝남"), probe("probe tmp ok /h/.hctmp.noindex"),
			"임시 폴더 못 씀 — /h/.hctmp.noindex 쓰기가 5s 안에 안 끝남"},
	}
}

func piFix(t *testing.T) *ctx {
	t.Helper()
	c := ctFix(t)
	poolCmd = func() string { return "fake-pool" }
	c.auth("a", raGood+"\n"+rwGood)
	c.auth("b", raGood+"\n"+rwGood)
	return c
}

// ① 나쁨이 문턱을 넘김 → 격리·알림 한 번·새 레인은 a로 ② 문턱 전 → 격리 없음 ⑤ 되풀이 없음(두 번째 실행 포함) ④ 돌아오면 풀림·알림 한 번
func TestProbeItemsThresholdRecover(t *testing.T) {
	for _, it := range piItems() {
		c := piFix(t)
		it.bad(c)
		c.tick(0)
		c.tick(3 * 60)
		if got, _ := c.place(); got != "b" || c.ev() != "" || cordonOn("b") {
			t.Fatalf("%s ② 문턱 전: b에 뜨고 알림 없음: %q %q", it.ck.name, got, c.ev())
		}
		if l := strings.Join(machineLines(true), "\n"); !strings.Contains(l, it.ck.name+" "+it.ck.bad+" 3m째(") || !strings.Contains(l, "격리 전") {
			t.Fatalf("%s ② 격리 전 기계 줄: %s", it.ck.name, l)
		}
		c.tick(3 * 60)
		got, why := c.place()
		if got != "a" || why != it.ck.why() || !cordonOn("b") || cordonLoad()["b"].On {
			t.Fatalf("%s ① 그 까닭으로만 격리: %q %q", it.ck.name, got, why)
		}
		e := c.ev()
		if strings.Count(e, "report 막힘") != 1 || !strings.Contains(e, "--key hf:cordon"+it.ck.key+":b:") || !strings.Contains(e, it.ck.look) ||
			!strings.Contains(e, it.ck.name+" "+it.ck.bad+" 6분째") {
			t.Fatalf("%s ① 막힘 한 번: %q", it.ck.name, e)
		}
		if l := strings.Join(machineLines(true), "\n"); !strings.Contains(l, "격리 6m째("+it.why) {
			t.Fatalf("%s ① 기계 줄 까닭: %s", it.ck.name, l)
		}
		if r := knobRemoteOf(knobEntries(), "p", time.Now()); r["cordon@b"].Value != "1" { // ⑫ 병합기는 같은 cordon@ 줄을 읽는다
			t.Fatalf("%s ⑫ 원격 줄: %+v", it.ck.name, r["cordon@b"])
		}
		c.tick(0)
		REPOCK, AUTHCK, PROBE = map[string]repoCk{}, map[string]string{}, nil
		cordonTick() // 같은 바퀴를 두 번
		cordonTick()
		c.tick(30 * 60)
		if strings.Count(c.ev(), "report 막힘") != 1 {
			t.Fatalf("%s ⑤ 되풀이: %q", it.ck.name, c.ev())
		}
		it.good(c)
		c.tick(60)
		if got, _ := c.place(); got != "b" || cordonOn("b") {
			t.Fatalf("%s ④ 돌아오면 다음 판정에 풀림: %q", it.ck.name, got)
		}
		c.tick(0)
		if e := c.ev(); strings.Count(e, "report 중간") != 1 || !strings.Contains(e, it.ck.name+" 돌아옴") || !strings.Contains(e, "--key hf:cordon"+it.ck.key+":b:") {
			t.Fatalf("%s ④ 회복 한 번: %q", it.ck.name, e)
		}
	}
}

// ③ 모름 꼴 전부 → 격리·알림 없음, 앞 상태(격리 중이면 격리) 그대로
func TestProbeItemsUnknown(t *testing.T) {
	probe := func(l string) func(c *ctx) { return func(c *ctx) { c.set("b", withProbe(l)) } }
	write := func(l string) func(c *ctx) { return func(c *ctx) { c.auth("b", raGood+"\n"+l) } }
	unk := map[*cordonCk]map[string]func(c *ctx){
		cdWrite: {"DNS": write(rwDNS), "안 끝남": write("remote-write 1 timeout git.example.test -"),
			"5xx":                   write("remote-write 1 128 git.example.test fatal: unable to access 'x': The requested URL returned error: 502"),
			"속도 제한":                 write("remote-write 1 128 git.example.test remote: API rate limit exceeded | fatal: The requested URL returned error: 403"),
			"src refspec(커밋 없는 클론)": write("remote-write 1 1 git.example.test error: src refspec HEAD does not match any"),
			"옛 판(줄 없음)":             write(""), "원격 로그인이 먼저 거절": func(c *ctx) { c.auth("b", raRevoked+"\n"+rwBad) }},
		cdPool: {"명령 없음": probe("probe pool unknown 명령 없음"), "안 끝남": probe("probe pool unknown 5s 안에 안 끝남"),
			"못 읽음": probe("probe pool unknown 쓸 계정 수를 못 읽음(0)"), "옛 판(줄 없음)": probe("")},
		cdKeychain: {"모르는 종료": probe("probe keychain unknown exit status 51"), "리눅스": probe("probe keychain na macOS 아님"), "옛 판": probe("")},
		cdTmp:      {"끔": probe("probe tmp na 작업 임시 폴더 끔"), "설정 못 읽음": probe("probe tmp unknown 설정 jobtmp.root을 못 읽음"), "옛 판": probe("")},
	}
	for _, it := range piItems() {
		for name, set := range unk[it.ck] {
			c := piFix(t)
			set(c)
			c.tick(0)
			c.tick(30 * 60)
			c.tick(30 * 60)
			if CORDON[it.ck.key]["b"].On || CORDON[it.ck.key]["b"].St == "bad" || strings.Contains(c.ev(), "--key hf:cordon"+it.ck.key+":") {
				t.Fatalf("%s ③ %s: 모름으로 격리·알림: %+v %q", it.ck.name, name, CORDON[it.ck.key]["b"], c.ev())
			}
			it.bad(c) // 격리한 뒤 모름 — 풀지 않는다
			c.tick(it.ck.every)
			c.tick(6 * 60)
			set(c)
			c.tick(60)
			if !CORDON[it.ck.key]["b"].On {
				t.Fatalf("%s ③ %s: 모름이 격리를 풂: %+v", it.ck.name, name, CORDON[it.ck.key]["b"])
			}
		}
	}
}

// ⑦ 섞임(키체인 나쁨·풀 모름·임시 폴더 좋음 + 쓰기 거절) → 나쁜 항목만 격리, 까닭은 「; 」로, advise는 갈래마다
func TestProbeItemsMixed(t *testing.T) {
	c := piFix(t)
	c.set("b", withProbe("probe keychain bad rc 36 User interaction is not allowed", "probe pool unknown 명령 없음", "probe tmp ok /h/t"))
	c.auth("b", raGood+"\n"+rwBad)
	c.tick(0)
	c.tick(6 * 60)
	if !CORDON[cdKeychain.key]["b"].On || !CORDON[cdWrite.key]["b"].On || CORDON[cdPool.key]["b"].St == "bad" || CORDON[cdTmp.key]["b"].St == "bad" {
		t.Fatalf("표대로: %+v", CORDON)
	}
	if e := c.ev(); strings.Count(e, "report 막힘") != 2 {
		t.Fatalf("까닭마다 한 번: %q", e)
	}
	if _, why := c.place(); why != cdWrite.why() {
		t.Fatalf("안 띄운 까닭은 점검 차례 첫 것: %q", why)
	}
	r := knobRemoteOf(knobEntries(), "p", time.Now())["cordon@b"]
	if !strings.Contains(r.Why, "원격 쓰기 거절 — ") || !strings.Contains(r.Why, "; 키체인 막힘 — ") {
		t.Fatalf("원격 줄 까닭: %q", r.Why)
	}
	c.set("b", withProbe("probe keychain ok 열림"))
	c.tick(60)
	if !cordonOn("b") || CORDON[cdKeychain.key]["b"].On {
		t.Fatalf("한 항목이 돌아와도 다른 까닭이 남으면 격리 그대로: %+v", CORDON)
	}
}

// ⑧ 시계 틀림 → 알리기만(격리 안 함), 돌아오면 한 번
func TestProbeClock(t *testing.T) {
	c := piFix(t)
	if err := os.WriteFile(c.cd+"/b.date", []byte(fmt.Sprint(time.Now().Unix()+600)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.tick(0)
	c.tick(6 * 60)
	if got, _ := c.place(); got != "b" || cordonOn("b") {
		t.Fatalf("시계는 격리하지 않는다: %q", got)
	}
	e := c.ev()
	if strings.Count(e, "report 막힘") != 1 || !strings.Contains(e, "--key hf:cordon-clock:b:") || !strings.Contains(e, "알리기만") || !strings.Contains(e, "초 빠름") {
		t.Fatalf("알림 한 번: %q", e)
	}
	if l := strings.Join(machineLines(true), "\n"); !strings.Contains(l, "시계 틀림 6m째(그 기계 시각이 ") || !strings.Contains(l, "— 알리기만)") {
		t.Fatalf("기계 줄: %s", l)
	}
	if r := knobRemoteOf(knobEntries(), "p", time.Now()); r["cordon@b"].Value == "1" {
		t.Fatalf("병합기에도 격리 줄 없음: %+v", r["cordon@b"])
	}
	os.Remove(c.cd + "/b.date")
	c.tick(60)
	if e := c.ev(); strings.Count(e, "report 중간") != 1 || !strings.Contains(e, "시계 돌아옴") {
		t.Fatalf("회복 한 번: %q", e)
	}
	// 오가는 시간 안이면 0 — 받는 쪽 시각과 견줘 1초 단위로 버린 몫만큼은 틀림이 아니다
	probeTake("b", "clock 100 99", false)
	if v, _ := clockVerdict("b"); v != 1 || PROBE["b"]["clock"] != "-1" {
		t.Fatalf("1초 느림은 좋음: %v %q", v, PROBE["b"]["clock"])
	}
}

// ⑨ 손잡이 0 → 알리기만
func TestProbeKnobZero(t *testing.T) {
	c := piFix(t)
	confLines = append(confLines[:len(confLines)-1], "cordon 0")
	c.set("b", withProbe("probe keychain bad rc 36 User interaction is not allowed"))
	c.tick(0)
	c.tick(6 * 60)
	if got, _ := c.place(); got != "b" || cordonOn("b") || !strings.Contains(c.ev(), "알리기만") {
		t.Fatalf("손잡이 0: %q %q", got, c.ev())
	}
}

// ⑩ 주소 안 자격·토큰 꼴은 기록·알림·기계 줄에 안 남는다(쓰기 거절 글)
func TestProbeRedact(t *testing.T) {
	c := piFix(t)
	tok := "ghs_" + strings.Repeat("Zq9", 12)
	c.auth("b", raGood+"\nremote-write 1 128 git.example.test fatal: Authentication failed for 'https://x-access-token:"+tok+"@git.example.test/o/r.git/'")
	c.tick(0)
	c.tick(6 * 60)
	b, _ := os.ReadFile(cdWrite.file())
	if !CORDON[cdWrite.key]["b"].On || strings.Contains(string(b)+c.ev()+strings.Join(machineLines(true), "\n"), tok) || strings.Contains(string(b), "x-access-token") {
		t.Fatalf("자격이 남음: %s %s", b, c.ev())
	}
}

// ⑪ 배포 직후: 좋은 기계도 간격을 기다리지 않고 한 번 — 그 뒤는 간격대로
func TestProbeAfterDeploy(t *testing.T) {
	c := piFix(t)
	c.tick(0) // 처음 봄 — 모두 물음
	calls := func() int { b, _ := os.ReadFile(c.cd + "/exec.calls"); return strings.Count(string(b), "b\n") }
	n := calls()
	c.tick(0)
	if calls() != n {
		t.Fatalf("간격 안에는 따로 안 부름: %d → %d", n, calls())
	}
	d := HOME + "/.local/state/herdr-sync"
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	line := time.Now().UTC().Format(time.RFC3339) + "\tsync\t-\tabc123abc123\t-\t-\t-\ta=같음 b=씀\n"
	if err := os.WriteFile(d+"/deploy.tsv", []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	DEPLOYAT = nil
	c.tick(5) // 마지막 판정이 배포보다 5초 앞
	if calls() != n+1 || c.authCalls("b") < 2 {
		t.Fatalf("배포 직후 한 번: 부름 %d → %d, 원격 로그인 %d", n, calls(), c.authCalls("b"))
	}
	c.tick(0)
	if calls() != n+1 {
		t.Fatalf("한 번만: %d", calls())
	}
}

// 합격 기준 — 기계당 원격 호출 수가 늘지 않는다: 좋은 기계 b, 1분 바퀴 30번. 옛 판(origin/main d4db104a)에서 같은 틀로 잰 값 b 원격 호출
// 17번·원격 로그인 5번(풀 점검을 켜도 같은 호출에 실린다)
func TestProbeCallBudget(t *testing.T) {
	for _, pool := range []string{"", "fake-pool"} {
		c := piFix(t)
		poolCmd = func() string { return pool }
		t0 := time.Now()
		for i := 0; i < 30; i++ {
			c.tick(60)
		}
		b, _ := os.ReadFile(c.cd + "/exec.calls")
		n := strings.Count(string(b), "b\n")
		t.Logf("측정(풀 %q): b 원격 호출 %d번 · 원격 로그인 %d번 · 30바퀴 %s", pool, n, c.authCalls("b"), time.Since(t0).Round(time.Millisecond))
		if n > 17 || c.authCalls("b") > 5 {
			t.Fatalf("원격 호출이 늘었다(풀 %q): %d번·원격 로그인 %d번", pool, n, c.authCalls("b"))
		}
	}
}

// 작업 중 발견해 함께 고침: 「한 문」(gate.go opsKeyDone)은 풀림 = 막힘 열쇠 + :up만 맞춘다 — 격리·닿지 않음은 주인을 맨 끝에 붙여
// 막힘 hf:cordon:<기계>:<시작>:<주인> · 풀림 …:<시작>:up:<주인>이라 격리 막힘이 풀림으로 안 읽혔다
func TestOpsUpKeyIsBlockKeyPlusUp(t *testing.T) {
	c := piFix(t)
	c.set("b", ctBad)
	c.tick(0)
	c.tick(6 * 60)
	c.set("b", ctGood)
	c.tick(0) // 때를 당기지 않는다 — 키의 나쁨 시작이 그대로여야
	var blk, up string
	for _, l := range strings.Split(c.ev(), "\n") {
		if _, k, ok := strings.Cut(l, "--key "); ok {
			k, _, _ = strings.Cut(k, " ")
			if strings.HasPrefix(l, "report 막힘") {
				blk = k
			} else if strings.HasPrefix(l, "report 중간") {
				up = k
			}
		}
	}
	if blk == "" || up != blk+":up" {
		t.Fatalf("격리 풀림 열쇠 = 막힘 열쇠 + :up: 막힘 %q 풀림 %q", blk, up)
	}
	if got := ownKey("hf:unreach:b:1700:up", "t0"); got != "hf:unreach:b:1700:t0:up" || ownKey("hf:unreach:b:1700", "t0") != "hf:unreach:b:1700:t0" {
		t.Fatalf("닿지 않음도 같은 꼴: %q", got)
	}
}

// 운영 장면 — 이미 쌓인 상태에 처음 돈다: 지금 꼴 cordon.tsv(격리 중인 b) 그대로 새 항목 기록 없이 → 격리 그대로·알림 다시 없음, 새 항목은 처음 봄부터
func TestProbeFirstRunOnOldState(t *testing.T) {
	c := piFix(t)
	now := time.Now().Unix()
	old := cordonRow{M: "b", St: "bad", Since: now - 3600, At: now - 60, On: true, Alerted: true, Why: "p /r/b: open /r/b: interrupted system call"}
	if err := os.WriteFile(cordonFile(), []byte(old.line()+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.set("b", strings.Replace(ctBad, "load=0.10 cpus=8 verifies=0 gates=0", "load=0.10 cpus=8 verifies=0 gates=0\nprobe keychain bad rc 36 User interaction is not allowed", 1))
	c.tick(0)
	if !cordonLoad()["b"].On || c.ev() != "" || CORDON[cdKeychain.key]["b"].On || CORDON[cdKeychain.key]["b"].Since < now {
		t.Fatalf("판 올림으로 풀리거나 알림이 다시 나감, 또는 새 항목이 처음 봄부터 세지 않음: %+v %+v %q", cordonLoad()["b"], CORDON[cdKeychain.key]["b"], c.ev())
	}
}
