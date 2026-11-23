package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// merger-alive(2026-11-24): 큐에 제출이 있는데 병합기 터미널이 없으면 주인 지휘에 막힘 한 번, 모름은 「없음」으로 안 읽고, 회복은 한 줄,
// 지휘가 hc-fill merger restart <프로젝트>로 지금 띄우는 길(hc run --bg … hc-lane merger --repo <클론>) 그대로 한 번 다시 띄운다

// maFx: 가짜 git(원격 큐)·hc-lane(사건 기록)·hc(run 기록, ls --all 응답)를 PATH 앞에 둔다. 사건·hc 부름 기록 읽기를 돌려준다
func maFx(t *testing.T, queue string, lsAll string) (events func() string, hcCalls func() string) {
	t.Helper()
	DATA = t.TempDir()
	Q, AT, LOCAL, dry, CONN = DATA+"/queue.tsv", DATA+"/audit.tsv", "m", false, []string{"m2"}
	confLines = []string{"repo app m " + DATA, "merger app m2"}
	MQP, MQS, MQBAD, maSnap = "", nil, "", mqSnap{}
	wr := func(f, s string) {
		if err := os.WriteFile(DATA+"/"+f, []byte(s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	wr("merger_dead", "5\n") // 손잡이 표식 파일
	wr("mq.txt", queue)
	wr("lsall.json", lsAll)
	wr("git", "#!/bin/sh\nd='"+DATA+"'\ncase \" $* \" in\n"+
		"*\" remote get-url \"*) echo x;;\n"+
		"*\" ls-remote \"*) echo x >>\"$d/lsr.log\"; [ -f \"$d/mqbad\" ] && exit 128; cat \"$d/mq.txt\"; [ -s \"$d/mq.txt\" ] || exit 2;;\n"+
		"esac\nexit 0\n")
	wr("hc-lane", "#!/bin/sh\nprintf '%s\\n' \"$*\" >>'"+DATA+"/ev.log'\n")
	wr("hc", "#!/bin/sh\nprintf '%s\\n' \"$*\" >>'"+DATA+"/hc.log'\ncase \"$1\" in ls) cat '"+DATA+"/lsall.json';; run) echo m2/t9;; esac\n")
	t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
	rd := func(f string) func() string {
		return func() string { b, _ := os.ReadFile(DATA + "/" + f); return string(b) }
	}
	return rd("ev.log"), rd("hc.log")
}

func maQ(lane string, ago time.Duration) string {
	return fmt.Sprintf("%s\trefs/merge-queue/%d-%s\n", strings.Repeat("a", 40), time.Now().Add(-ago).Unix(), lane)
}

func maMerger() Term {
	return Term{M: "m2", Addr: "m2/t1", Agent: "zsh", Name: "merger-app", Status: "-", Cwd: "/c/app"}
}

// 바퀴를 n번 — 앞 바퀴의 「처음 못 본 때」를 6분 전으로 당겨 문턱(5분)을 넘긴다
func maTick(n int) {
	for i := 0; i < n; i++ {
		MQP, MQS, MQBAD, maSnap = "", nil, "", mqSnap{}
		mergerAliveTick()
		var rs []Row
		for _, r := range readRows(DATA + "/merger-alive.tsv") {
			if r.F(4) != "-" && isDigits(r.F(4)) {
				r = r.Set(4, fmt.Sprint(atoi10(r.F(4))-360))
			}
			rs = append(rs, r)
		}
		if len(rs) > 0 {
			writeAtomic(DATA+"/merger-alive.tsv", rowsText(rs))
		}
	}
}

// ① 큐 있음 + 병합기 터미널 없음 → 사건 1번(다음 바퀴에 되풀이 안 함), 주인 지휘에
func TestMergerAliveDeadOnce(t *testing.T) {
	ev, _ := maFx(t, maQ("w1", 20*time.Minute), "")
	os.WriteFile(Q, []byte(strings.Join([]string{"w1", "app", "b", "-", "-", "up", "m2", "m2/t5", "-", "t", "-", "t0", "normal", "-", "auto", "-", "r1"}, "\t")+"\n"), 0o644)
	TERMS = []Term{{M: "m2", Addr: "m2/t5", Agent: "claude-code", Name: "w1", Status: "idle", Cwd: "/w"}}
	maTick(3)
	e := ev()
	if strings.Count(e, "report 막힘") != 1 || !strings.Contains(e, "병합기 app(m2)") || !strings.Contains(e, "큐 1건") ||
		!strings.Contains(e, "--to t0") || !strings.Contains(e, "hc-fill merger restart app") {
		t.Fatalf("사건: %q", e)
	}
}

// ② 병합기 터미널이 살아 있음(긴 검증 중이어도 터미널은 산다) → 사건 없음
func TestMergerAliveVerifying(t *testing.T) {
	ev, _ := maFx(t, maQ("w1", 90*time.Minute), "")
	TERMS = []Term{maMerger()}
	maTick(3)
	if e := ev(); e != "" {
		t.Fatalf("살아 있는 병합기를 알렸다: %q", e)
	}
}

// ③ 기계 연결 실패·큐 못 읽음·그 기계 목록 항목 모름 → 「없음」으로 읽지 않음(문턱 안엔 사건 없음, 모름이 오래면 따로 한 번)
func TestMergerAliveUnknown(t *testing.T) {
	for _, c := range []struct {
		name string
		set  func()
	}{
		{"연결 안 됨", func() { CONN = nil }},
		{"큐 못 읽음", func() { os.WriteFile(DATA+"/mqbad", nil, 0o644) }},
		{"목록 항목 모름", func() { TERMS = []Term{{M: "m2", Addr: "m2/t3", Agent: "-", Name: "-", Status: "-", Cwd: "-"}} }},
	} {
		t.Run(c.name, func(t *testing.T) {
			ev, _ := maFx(t, maQ("w1", 20*time.Minute), "")
			TERMS = nil
			c.set()
			maTick(2)
			if e := ev(); strings.Contains(e, "돌지 않음") {
				t.Fatalf("모름을 없음으로 읽었다: %q", e)
			}
			os.WriteFile(DATA+"/merger_unknown", []byte("5\n"), 0o644)
			maTick(3)
			if e := ev(); strings.Count(e, "상태 모름") != 1 {
				t.Fatalf("오래 이어진 모름을 한 번 알려야 한다: %q", e)
			}
		})
	}
}

// ④ 큐가 비어 있음 → 사건 없음
func TestMergerAliveEmptyQueue(t *testing.T) {
	ev, _ := maFx(t, "", "")
	TERMS = nil
	maTick(3)
	if e := ev(); e != "" {
		t.Fatalf("빈 큐에 알렸다: %q", e)
	}
}

// ⑤ restart → 지금 띄우는 길(끝난 병합기 터미널의 이름·클론)로 hc run 한 번. 살아 있으면 띄우지 않음
func TestMergerRestart(t *testing.T) {
	ls := `{"terminals":[{"address":"m2/t1","state":"finished","name":"merger-app","agent":"zsh","cwd":"/c/app"}]}`
	_, hc := maFx(t, "", ls)
	TERMS = nil
	if rc := mergerRestart("app"); rc != 0 {
		t.Fatalf("rc %d", rc)
	}
	want := "run --bg --on m2 --name merger-app --cwd /c/app --no-parent -- zsh -lc export HC_LANE_MACHINE='m2'; exec hc-lane merger --repo '/c/app'"
	if h := hc(); strings.Count(h, "run --bg") != 1 || !strings.Contains(h, want) {
		t.Fatalf("hc 부름: %q", h)
	}
	os.WriteFile(DATA+"/lsall.json", []byte(`{"terminals":[{"address":"m2/t1","state":"live","name":"merger-app","agent":"zsh","cwd":"/c/app"}]}`), 0o644)
	if rc := mergerRestart("app"); rc == 0 || strings.Count(hc(), "run --bg") != 1 {
		t.Fatalf("살아 있는데 또 띄웠다: rc %d %q", rc, hc())
	}
}

// ⑤-2 설정 줄 넷째 칸 클론(재부팅으로 끝난 터미널 기록이 없을 때)
func TestMergerRestartConfClone(t *testing.T) {
	_, hc := maFx(t, "", `{"terminals":[]}`)
	confLines = append(confLines[:1], "merger app m2 /c/x y")
	TERMS = nil
	if rc := mergerRestart("app"); rc != 0 || !strings.Contains(hc(), "--name merger-app --cwd /c/x y --no-parent") {
		t.Fatalf("rc %d %q", rc, hc())
	}
}

// ⑥ 회복 알림 — 알린 뒤 터미널이 다시 보이면 한 줄, 그 뒤로는 조용
func TestMergerAliveRecovered(t *testing.T) {
	ev, _ := maFx(t, maQ("w1", 20*time.Minute), "")
	TERMS = nil
	maTick(2)
	TERMS = []Term{maMerger()}
	maTick(3)
	e := ev()
	if strings.Count(e, "report 막힘") != 1 || strings.Count(e, "다시 돎") != 1 || !strings.Contains(e, "m2/t1") {
		t.Fatalf("사건: %q", e)
	}
}

// 재기는 원격 호출을 늘리지 않는다: mergerTick이 이 바퀴에 읽은 큐를 그대로 쓴다(ls-remote 한 번). status·ls 한 줄은 기록에서
func TestMergerAliveOneRead(t *testing.T) {
	maFx(t, maQ("w1", 20*time.Minute), "")
	TERMS = []Term{maMerger()}
	mergerTick()
	mergerAliveTick()
	b, _ := os.ReadFile(DATA + "/lsr.log")
	if n := strings.Count(string(b), "x"); n != 1 {
		t.Fatalf("ls-remote %d번", n)
	}
	l := maLines()
	if len(l) != 1 || !strings.HasPrefix(l[0], "병합기 app m2: 살아 있음(m2/t1) 0m째 · 큐 1건(가장 오래 20m) · 확인 0m 전") {
		t.Fatalf("한 줄: %q", l)
	}
}

// 손잡이 표식 파일 merger_restart: 알릴 때 스스로 한 번 다시 띄우고 글에 결과
func TestMergerAliveAutoRestart(t *testing.T) {
	ev, hc := maFx(t, maQ("w1", 20*time.Minute), `{"terminals":[{"address":"m2/t1","state":"finished","name":"merger-app","agent":"zsh","cwd":"/c/app"}]}`)
	os.WriteFile(DATA+"/merger_restart", nil, 0o644)
	TERMS = nil
	maTick(3)
	if strings.Count(hc(), "run --bg") != 1 || !strings.Contains(ev(), "스스로 다시 띄움") {
		t.Fatalf("hc %q 사건 %q", hc(), ev())
	}
}
