//go:build !windows

package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// ht-cordon-remote-auth: 원격 로그인이 거절되는 기계도 같은 격리 길로(docs/hc-fill/cordon.md 「원격 로그인 점검」). 틀은 cordon_test.go의 ctFix —
// 가짜 hc-lane은 --remote-auth면 $CD/<기계>.auth 한 줄을 점검 줄 앞에 싣고(파일이 없으면 옛 판처럼 종료 2 → --repo-read만 다시),
// 부를 때마다 $CD/auth.calls에 기계 이름. 고정 자료는 2026-11-27 실제 확인(git 2.56.1)에서 받은 글 — 호스트·저장소는 시험 대역 값

const (
	raGood    = "remote-auth 1 0 git.example.test -"
	raEmpty   = "remote-auth 1 2 git.example.test -" // 빈 원격(--exit-code 2) — 자격은 통과
	raNoCred  = "remote-auth 1 128 git.example.test error: unable to read askpass response from '/usr/bin/false' | fatal: could not read Username for 'https://git.example.test': terminal prompts disabled"
	raRevoked = "remote-auth 1 128 git.example.test remote: Invalid username or token. Password authentication is not supported for Git operations. | fatal: Authentication failed for 'https://git.example.test/o/r.git/'"
	raSSH     = "remote-auth 1 128 git.example.test git@git.example.test: Permission denied (publickey). | fatal: Could not read from remote repository. | Please make sure you have the correct access rights | and the repository exists."
)

// 모름 고정 자료: 안 끝남·DNS·연결 실패·5xx·속도 제한(403이어도)·429·폴더 없음·호출 실패·꼴이 틀림·옛 판(줄 없음)
var raUnknown = map[string]string{
	"안 끝남":   "remote-auth 1 timeout git.example.test -",
	"DNS":    "remote-auth 1 128 nonexistent.invalid fatal: unable to access 'https://nonexistent.invalid/o/r.git/': Could not resolve host: nonexistent.invalid",
	"연결":     "remote-auth 1 128 git.example.test fatal: unable to access 'https://git.example.test/o/r.git/': Failed to connect to git.example.test port 443 after 3 ms: Couldn't connect to server",
	"5xx":    "remote-auth 1 128 git.example.test fatal: unable to access 'https://git.example.test/o/r.git/': The requested URL returned error: 502",
	"속도 제한":  "remote-auth 1 128 git.example.test remote: API rate limit exceeded for installation. | fatal: unable to access 'https://git.example.test/o/r.git/': The requested URL returned error: 403",
	"429":    "remote-auth 1 128 git.example.test fatal: unable to access 'https://git.example.test/o/r.git/': The requested URL returned error: 429",
	"폴더 없음":  "remote-auth 0 none - -",
	"호출 실패":  "remote-auth 1 -1 git.example.test -",
	"꼴":      "remote-auth 1",
	"차례 넘침":  "remote-auth 9 128 git.example.test fatal: Authentication failed",
	"옛 판 워커": "",
}

func (c *ctx) auth(m, line string) {
	c.t.Helper()
	if err := os.WriteFile(c.cd+"/"+m+".auth", []byte(line+"\n"), 0o644); err != nil {
		c.t.Fatal(err)
	}
}

func (c *ctx) authCalls(m string) int {
	b, _ := os.ReadFile(c.cd + "/auth.calls")
	n := 0
	for _, l := range strings.Split(string(b), "\n") {
		if l == m {
			n++
		}
	}
	return n
}

func authOn(m string) bool { cordonOn(m); return CORDON[cdAuth.key][m].On }

// ① 거절이 문턱을 넘김 → 격리·알림 한 번·새 레인은 a로, 까닭 글과 사람이 볼 것 ② 문턱 전 → 격리 없음
// ⑤ 같은 격리 동안 되풀이 없음(두 번째 실행 포함) ④ 돌아오면 풀림·알림 한 번
func TestCordonAuthThresholdAndRecover(t *testing.T) {
	for _, bad := range []string{raNoCred, raRevoked, raSSH} {
		c := ctFix(t)
		c.auth("a", raGood)
		c.auth("b", bad)
		c.tick(0)
		c.tick(3 * 60)
		if got, _ := c.place(); got != "b" || c.ev() != "" || cordonOn("b") {
			t.Fatalf("② 문턱 전: b에 뜨고 알림 없음: %q %q", got, c.ev())
		}
		if l := strings.Join(machineLines(true), "\n"); !strings.Contains(l, "원격 로그인 거절 3m째(p git.example.test: ") || !strings.Contains(l, "격리 전") {
			t.Fatalf("② 격리 전 기계 줄: %s", l)
		}
		c.tick(3 * 60)
		got, why := c.place()
		if got != "a" || why != "격리(원격 로그인 거절)" || !cordonOn("b") || cordonLoad()["b"].On {
			t.Fatalf("① 원격 까닭으로만 격리(저장소 읽기 기록은 그대로): %q %q %+v", got, why, cordonLoad()["b"])
		}
		e := c.ev()
		if strings.Count(e, "report 막힘") != 1 || !strings.Contains(e, "--name hc-fill-cordon") || !strings.Contains(e, "--key hf:cordon-auth:b:") ||
			!strings.Contains(e, "gh auth status") || !strings.Contains(e, "원격 로그인 거절 6분째") {
			t.Fatalf("① 막힘 한 번: %q", e)
		}
		if l := strings.Join(machineLines(true), "\n"); !strings.Contains(l, "격리 6m째(원격 로그인 거절 — p git.example.test: ") {
			t.Fatalf("① 기계 줄 까닭: %s", l)
		}
		c.tick(0)
		c.tick(0)
		c.tick(30 * 60)
		if strings.Count(c.ev(), "report 막힘") != 1 {
			t.Fatalf("⑤ 되풀이: %q", c.ev())
		}
		c.auth("b", raEmpty)
		c.tick(60)
		if got, _ := c.place(); got != "b" || cordonOn("b") {
			t.Fatalf("④ 돌아오면 다음 판정에 풀림: %q", got)
		}
		c.tick(0)
		if e := c.ev(); strings.Count(e, "report 중간") != 1 || !strings.Contains(e, "원격 로그인 돌아옴") {
			t.Fatalf("④ 회복 한 번: %q", e)
		}
	}
}

// ③ 모름(안 끝남·DNS·5xx·속도 제한 …) → 격리 없음·알림 없음, 앞 상태 유지(격리 중이면 그대로)
func TestCordonAuthUnknown(t *testing.T) {
	for name, l := range raUnknown {
		c := ctFix(t)
		c.auth("b", l)
		c.tick(0)
		c.tick(30 * 60)
		c.tick(30 * 60)
		if cordonOn("b") || c.ev() != "" {
			t.Fatalf("③ %s: 모름으로 격리: %+v %q", name, CORDON[cdAuth.key]["b"], c.ev())
		}
		c.auth("b", raRevoked)
		c.tick(cdAuth.every) // 물었는데 모름은 다음 확인을 간격 뒤로 미뤘다
		c.tick(6 * 60)
		c.auth("b", l)
		c.tick(60)
		if !authOn("b") || strings.Count(c.ev(), "report 중간") != 0 {
			t.Fatalf("③ %s: 격리 중 모름은 그대로: %+v %q", name, CORDON[cdAuth.key]["b"], c.ev())
		}
	}
}

// ⑥ 두 점검의 표: 읽기 나쁨 + 원격 좋음 · 그 반대 · 둘 다 나쁨 → 어느 하나라도 격리면 격리, 까닭 글은 점검마다
// ⑦ 한 점검의 모름이 다른 점검의 나쁨을 안 지움 · 한 점검이 돌아와도 다른 점검의 격리는 그대로
func TestCordonTwoChecks(t *testing.T) {
	c := ctFix(t)
	c.set("b", ctBad)
	c.auth("b", raRevoked)
	c.tick(0)
	c.tick(6 * 60)
	if _, why := c.place(); why != cordonWhy || !cordonLoad()["b"].On || !authOn("b") {
		t.Fatalf("⑥ 둘 다 나쁨: 둘 다 격리, 까닭은 앞 점검: %q", why)
	}
	if e := c.ev(); strings.Count(e, "report 막힘") != 2 || !strings.Contains(e, "hf:cordon:b:") || !strings.Contains(e, "hf:cordon-auth:b:") {
		t.Fatalf("⑥ 까닭마다 한 번: %q", e)
	}
	l := strings.Join(machineLines(true), "\n")
	if !strings.Contains(l, "interrupted system call") || !strings.Contains(l, "원격 로그인 거절 — ") {
		t.Fatalf("⑥ 기계 줄에 까닭 둘: %s", l)
	}
	s, _ := stateCollectF(time.Now())
	for _, m := range s.Machines {
		if m.Name == "b" && (m.Cordon == nil || !strings.Contains(m.Cordon.Why, "interrupted") || !strings.Contains(m.Cordon.Why, "원격 로그인 거절")) {
			t.Fatalf("⑥ 상태 한 장 까닭 둘: %+v", m.Cordon)
		}
	}
	// 읽기만 돌아옴 · 원격은 모름 → 원격 격리 그대로(기계는 격리)
	c.set("b", ctGood)
	c.auth("b", raUnknown["안 끝남"])
	c.tick(60)
	if _, why := c.place(); why != "격리(원격 로그인 거절)" || cordonLoad()["b"].On {
		t.Fatalf("⑦ 읽기 돌아옴 + 원격 모름 → 원격 까닭으로 격리 그대로: %q", why)
	}
	// 원격만 나쁨(읽기 좋음) → 격리 · 원격 돌아옴 → 풀림
	c.auth("b", raGood)
	c.tick(60)
	if got, _ := c.place(); got != "b" {
		t.Fatalf("⑥ 둘 다 좋음 → 풀림: %q", got)
	}
	// 읽기 나쁨 + 원격 좋음 → 읽기 까닭으로
	c.set("b", ctBad)
	c.tick(cdRepo.every) // 방금 좋음 — 읽기 점검 간격 뒤에 다시 본다
	c.tick(6 * 60)
	if _, why := c.place(); why != cordonWhy || authOn("b") {
		t.Fatalf("⑥ 읽기 나쁨 + 원격 좋음: %q", why)
	}
}

// ⑧ 좋은 기계는 간격(cdAuth.every) 안에 다시 안 부름 · 나쁜 기계는 바퀴마다 · 물었는데 모름도 간격 뒤로(장애 때 호출이 늘지 않게) ·
// 바퀴 순서 반대(hc-fill이 한참 안 돎) → 간격 지난 것만 부름
func TestCordonAuthEvery(t *testing.T) {
	c := ctFix(t)
	c.auth("a", raGood)
	c.auth("b", raGood)
	c.tick(0)
	c.tick(60)
	c.tick(60)
	if n := c.authCalls("b"); n != 1 {
		t.Fatalf("⑧ 좋은 기계는 간격 안에 한 번: %d", n)
	}
	c.tick(cdAuth.every)
	if n := c.authCalls("b"); n != 2 {
		t.Fatalf("⑧ 간격 지나면 다시: %d", n)
	}
	c.auth("b", raUnknown["5xx"])
	c.tick(cdAuth.every)
	c.tick(60)
	c.tick(60)
	if n := c.authCalls("b"); n != 3 {
		t.Fatalf("⑧ 물었는데 모름도 간격 뒤로: %d", n)
	}
	c.auth("b", raRevoked)
	c.tick(cdAuth.every)
	c.tick(60)
	c.tick(60)
	if n := c.authCalls("b"); n != 6 {
		t.Fatalf("⑧ 나쁜 기계는 바퀴마다: %d", n)
	}
	// 같은 바퀴 부하 조각(loadTake)이 이미 물었으면 cordonTick은 다시 안 부른다
	c.auth("a", raGood)
	os.Remove(cdAuth.file()) // a도 물을 차례로(기록 없음)
	before := c.authCalls("a")
	REPOCK, AUTHCK = map[string]repoCk{}, map[string]string{}
	loadTake("a", "load 0.1 8\n"+authAsked+"\nload=0.10 cpus=8 verifies=0 gates=0\n"+raGood+"\nrepo-read 1 ok\nrepo-read end 1")
	if AUTHCK["a"] != raGood {
		t.Fatalf("⑧ 표식이 든 출력은 물은 것: %q", AUTHCK["a"])
	}
	cordonTick()
	if c.authCalls("a") != before {
		t.Fatal("⑧ 같은 바퀴에 이미 물은 기계를 다시 부름")
	}
}

// ⑨ 손잡이 0 → 격리 없이 알리기만 ⑩ 오류 글의 자격·토큰 꼴은 기록·알림·기계 줄에 안 남는다
func TestCordonAuthKnobZeroAndRedact(t *testing.T) {
	c := ctFix(t)
	confLines = append(confLines[:len(confLines)-1], "cordon 0")
	tok := "ghs_" + strings.Repeat("Ab3", 12)
	c.auth("b", "remote-auth 1 128 git.example.test fatal: Authentication failed for 'https://x-access-token:"+tok+"@git.example.test/o/r.git/' | token "+tok)
	c.tick(0)
	c.tick(6 * 60)
	if cordonOn("b") || strings.Count(c.ev(), "report 막힘") != 1 || !strings.Contains(c.ev(), "격리 안 함(손잡이 cordon 0") {
		t.Fatalf("⑨ 알리기만: %q", c.ev())
	}
	b, _ := os.ReadFile(cdAuth.file())
	all := c.ev() + string(b) + strings.Join(machineLines(true), "\n")
	if strings.Contains(all, tok) || strings.Contains(all, "x-access-token") || !strings.Contains(all, "Authentication failed") {
		t.Fatalf("⑩ 자격이 남음: %s", all)
	}
}

// ⑪ 병합기에: 원격 까닭의 격리도 같은 cordon@<기계> 줄(병합기 읽는 쪽은 까닭을 보지 않는다 — cmd/hc-merger/cordon_test.go)
func TestCordonAuthRemoteLine(t *testing.T) {
	c := ctFix(t)
	c.auth("b", raRevoked)
	c.tick(0)
	c.tick(6 * 60)
	if r := knobRemoteOf(knobEntries(), "p", time.Now()); r["cordon@b"].Value != "1" || !strings.Contains(r["cordon@b"].Why, "원격 로그인 거절") {
		t.Fatalf("⑪ 원격 줄: %+v", r["cordon@b"])
	}
}

// 감사 atp61ld 중간 1: 조각을 만들 때는 차례가 아니었는데(간격 직전) 출력을 받을 때 간격을 넘겼다 → 묻지 않은 것이니 「못 물음」 —
// 다음 확인을 미루지 않고 다음 바퀴에 실제로 묻는다(「물었는가」는 조각 출력의 표식 하나로만 정한다)
func TestCordonAuthAskedOnce(t *testing.T) {
	c := ctFix(t)
	c.auth("b", raGood)
	now := time.Now().Unix()
	put := func(at int64) {
		for _, ck := range []*cordonCk{cdAuth, cdWrite} { // 쓰기 자격은 같은 호출에 실려 같은 때 판정된다(ht-probe-cordon)
			if err := os.WriteFile(ck.file(), []byte(cordonRow{M: "b", St: "ok", At: at}.line()+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	put(now - cdAuth.every + 1)
	snip := loadSnip("b")
	if strings.Contains(snip, "--remote-auth") {
		t.Fatalf("간격 전에는 안 묻는다: %s", snip)
	}
	put(now - cdAuth.every - 1) // 조각이 도는 사이 간격을 넘김
	REPOCK, AUTHCK = map[string]repoCk{}, map[string]string{}
	out, _ := onm(nil, "b", snip)
	loadTake("b", out)
	before := c.authCalls("b")
	cordonTick()
	if _, asked := AUTHCK["b"]; asked || c.authCalls("b") != before || cdAuth.load()["b"].At != now-cdAuth.every-1 {
		t.Fatalf("묻지 않았는데 물었는데 모름으로: 있음=%v 부름 %d→%d 기록 %+v", asked, before, c.authCalls("b"), cdAuth.load()["b"])
	}
	REPOCK, AUTHCK = map[string]repoCk{}, map[string]string{}
	cordonTick()
	if c.authCalls("b") != before+1 || now-cdAuth.load()["b"].At > 5 {
		t.Fatalf("다음 바퀴에 실제로 묻는다: 부름 %d→%d 기록 %+v", before, c.authCalls("b"), cdAuth.load()["b"])
	}
}

// advise 항목도 까닭으로 가른다(작업 중 발견 — 원격 까닭에도 「저장소 폴더를 못 읽음」·worker-mac-check로 나왔다)
func TestCordonAuthAdvise(t *testing.T) {
	c := ctFix(t)
	c.auth("b", raRevoked)
	c.tick(0)
	c.tick(6 * 60)
	var it []string
	for _, i := range adviseNow(time.Now()).Items {
		if i.Kind == "cordon" {
			it = append(it, i.What+" · "+i.Evidence+" · "+i.Cost)
		}
	}
	if len(it) != 1 || !strings.Contains(it[0], "원격 로그인 거절") || !strings.Contains(it[0], "gh auth status") || strings.Contains(it[0], "저장소 폴더") ||
		strings.Contains(it[0], "worker-mac-check") {
		t.Fatalf("advise 원격 까닭: %v", it)
	}
}

// 운영 장면: 오늘 꼴 cordon.tsv(저장소 읽기 격리 중)에 처음 돌 때 — 판 올림으로 풀리거나 알림이 다시 나가지 않는다(원격 기록은 따로 파일)
func TestCordonAuthUpgradeKeepsRepoCordon(t *testing.T) {
	c := ctFix(t)
	now := time.Now().Unix()
	row := cordonRow{M: "b", St: "bad", Since: now - 600, At: now - 60, On: true, Alerted: true, Why: "p /r/b: open /r/b: interrupted system call"}
	if err := os.WriteFile(cordonFile(), []byte(row.line()+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.set("b", ctBad)
	c.auth("b", raGood)
	c.tick(0)
	if !cordonLoad()["b"].On || c.ev() != "" || authOn("b") {
		t.Fatalf("판 올림 뒤 격리 그대로·알림 없음: %+v %q", cordonLoad()["b"], c.ev())
	}
}
