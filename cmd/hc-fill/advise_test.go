package main

// ops-advise(2026-11-24): 모으기(대기 줄 까닭·주소)·사건(새 항목에 한 번, 되풀이 없음)·state의 눈에 띄는 것이 같은 목록.
// 가짜 hc·hc-lane과 가짜 대기열(stFx). 고치기 전 코드로는 adviseNow·adviseTick·stateCollectF가 없어 컴파일부터 실패한다

import (
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/events"
	"herdr-tools/internal/opsstate"
)

// advFx: stFx + 저장소 줄(app이 m·w에 있음) + 사건을 적는 가짜 hc-lane. lane-b는 lane-a 뒤 선행 대기, lane-c는 lane-a와 문서 한 파일 겹침 대기
func advFx(t *testing.T) {
	t.Helper()
	stFx(t, `{"machines":[{"name":"w","state":"connected"},{"name":"x","state":"connected"}]}`)
	confLines = append(confLines, "repo app m /r", "repo app w /r")
	wr := func(f, s string) {
		if err := os.WriteFile(DATA+"/"+f, []byte(s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	wr("hc-lane", "#!/bin/sh\nprintf '%s\\n' \"$*\" >>'"+DATA+"/report.log'\n")
	add := time.Now().Add(-3 * time.Hour).Format("2006-01-02T15:04")
	q, _ := os.ReadFile(DATA + "/queue.tsv")
	wr("queue.tsv", string(q)+strings.Join([]string{"lane-c", "app", DATA + "/briefs/added/lane-c.md", "-", "-", "wait", "-", "-", "-", add, "-", "m/t1", "normal", "-", "-", "-", "r3"}, "\t")+"\n")
	wr("overlap.tsv", "lane-c\tlane-a\tdocs/x.md\t-\tapp\n")
	// 마지막 칸: 그 바퀴 띄우기가 판정한 빈 자리(machRoom — m 자리 3·에이전트 1·레인상한 2·레인 0 → 2). advise는 이것만 읽는다
	wr("load.tsv", "m\t1.20\t12\t"+itoa(time.Now().Unix()-30)+"\t1\t3\t-\t0\t0\t-\t2\n")
}

// reports: 가짜 hc-lane이 받은 사건 — 레인 주인에게 보낸 손댈 것의 참고(키 hf:advise:ref:, ht-advise-claim)는 뺀다(refReports)
func reports(t *testing.T) []string { return reportsRef(t, false) }

func refReports(t *testing.T) []string { return reportsRef(t, true) }

func reportsRef(t *testing.T, ref bool) []string {
	b, _ := os.ReadFile(DATA + "/report.log")
	var o []string
	for _, l := range strings.Split(string(b), "\n") {
		if l != "" && strings.Contains(l, "--key "+events.AdviseRefKey) == ref {
			o = append(o, l)
		}
	}
	return o
}

// 모으기: 대기 줄의 겹침·선행·띄울 수 있는 기계·다시 넣는 줄, 떠 있는 레인 주소 → 풀 수 있는 대기 둘(겹침은 확실·선행은 판단 필요)
func TestAdviseCollect(t *testing.T) {
	advFx(t)
	a := adviseNow(time.Now())
	by := map[string]opsstate.Item{}
	for _, it := range a.Items {
		by[it.Key] = it
	}
	c, ok := by["free-wait:lane-c"]
	if !ok || c.Sure != opsstate.SureYes || len(c.Next) != 1 ||
		c.Next[0] != "hc-fill edit lane-c --overlap-ok --why '겹침 풀기(hc-fill advise)'" {
		t.Errorf("겹침 대기: %+v\n전체 %+v", c, a.Items)
	}
	if !strings.Contains(c.Evidence, "빈 자리 2(m 2)") || !strings.Contains(c.Evidence, "lane-a 작업 중(working)") {
		t.Errorf("근거(m 자리 3·에이전트 1·레인 상한 2 → 2, w는 에이전트 기록 없음): %s", c.Evidence)
	}
	if b, ok := by["free-wait:lane-b"]; !ok || b.Sure != opsstate.SureJudge || b.Next[0] != "hc-fill edit lane-b --after - --why '선행 풀기(hc-fill advise)'" {
		t.Errorf("선행 대기: %+v", b)
	}
	if a.Fingerprint == "" {
		t.Error("지문 없음")
	}
}

// ⑧ 새 항목에 사건 한 번, 간격 안·같은 항목이면 되풀이 없음, 새 항목이 또 생기면 그것만 한 번. 판정 못 함은 사건이 아니다
func TestAdviseTickEvents(t *testing.T) {
	advFx(t)
	now := time.Now()
	adviseTick(now)
	r := reports(t)
	if len(r) != 1 || !strings.Contains(r[0], "--name hc-fill-advise") || !strings.Contains(r[0], "중간 - 손댈 것: ") || !strings.Contains(r[0], " 외 ") {
		t.Fatalf("첫 사건: %q", r)
	}
	if strings.Contains(r[0], "판정 못 함") {
		t.Errorf("판정 못 함이 사건에: %s", r[0])
	}
	adviseTick(now.Add(time.Minute)) // 간격(advise_every 5분) 안
	adviseTick(now.Add(10 * time.Minute))
	if r := reports(t); len(r) != 1 {
		t.Fatalf("같은 항목에 되풀이: %q", r)
	}
	// 새 항목: lane-a 터미널이 error가 됨
	os.WriteFile(DATA+"/lsjson", []byte(`{"terminals":[{"address":"w/t5","name":"lane-a","status":"error"},{"address":"t2","name":"`+watchName("m/t1")+`","status":"idle"}]}`), 0o644)
	adviseTick(now.Add(20 * time.Minute))
	r = reports(t)
	if len(r) != 2 || !strings.Contains(r[1], "손댈 것: 멈춘 레인(터미널 error): lane-a") || strings.Contains(r[1], " 외 ") {
		t.Fatalf("새 항목 사건: %q", r)
	}
	// 끔(advise_every 0)이면 사건 없음
	confLines = append(confLines, "advise_every 0")
	os.WriteFile(DATA+"/lsjson", []byte(`{"terminals":[{"address":"w/t5","name":"lane-a","status":"blocked"}]}`), 0o644)
	adviseTick(now.Add(40 * time.Minute))
	if r := reports(t); len(r) != 2 {
		t.Fatalf("끈 뒤 사건: %q", r)
	}
}

// 보내기를 실패하면 열쇠를 적지 않아 다음에 다시
func TestAdviseTickRetry(t *testing.T) {
	advFx(t)
	os.WriteFile(DATA+"/hc-lane", []byte("#!/bin/sh\nexit 1\n"), 0o755)
	now := time.Now()
	adviseTick(now)
	os.WriteFile(DATA+"/hc-lane", []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >>'"+DATA+"/report.log'\n"), 0o755)
	adviseTick(now.Add(6 * time.Minute))
	if r := reports(t); len(r) != 1 {
		t.Fatalf("실패 뒤 다시 보냄: %q", r)
	}
}

// ⑨ state의 눈에 띄는 것 = 손댈 것의 요약(같은 모으기, 같은 지문)
func TestStateNotableIsAdvise(t *testing.T) {
	advFx(t)
	now := time.Now()
	a := adviseNow(now)
	s, f := stateCollectF(now)
	tr := opsstate.Trim(s, knobInt("state_done"))
	got := stateNotable(tr, f, stateThresholds())
	if strings.Join(got, "\n") != strings.Join(opsstate.Summaries(a.Items), "\n") || len(got) == 0 {
		t.Errorf("눈에 띄는 것:\n%q\n손댈 것:\n%q", got, opsstate.Summaries(a.Items))
	}
	tr.Notable = got
	if opsstate.Fingerprint(tr) != a.Fingerprint {
		t.Errorf("지문이 state와 다름")
	}
}
