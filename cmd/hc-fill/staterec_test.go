package main

// ht-merger-state-record(2026-11-26 사용자 「구조를 다시 짜기」): hc-fill은 병합기가 쓴 상태 한 장(internal/mergerstate — run 바퀴가
// merger-alive.tsv 그 줄 13칸에 받아 둠)만 읽는다. 실제 일: merger_shards 2로 나눈 추측 검증 두 몫이 돌고 있는데 「살아 있는 spec 터미널
// 없음 · 마지막 바퀴 28분 전」 merge-stall — 이름 정규식이 몫 터미널 이름을 못 맞추고, 마지막 바퀴가 일지 줄 시각이었다.
// 고치기 전 코드에는 기록 칸(maRec)이 없어 컴파일부터 실패한다.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/mergerstate"
	"herdr-tools/internal/opsstate"
)

// srFx: stFx + 병합기(이 기계 m, 터미널 t9) 줄 + 큐 lane-a(28분째). terms = hc ls에 더할 터미널(주소)
func srFx(t *testing.T, rec *mergerstate.Rec, why string, terms ...string) {
	t.Helper()
	stFx(t, `{"machines":[{"name":"w","state":"connected"},{"name":"x","state":"connected"}]}`)
	confLines = append(confLines, "repo app m /r", "merger app m")
	ts := []string{`{"address":"w/t5","name":"lane-a","state":"live","status":"working","cwd":"/r"}`,
		`{"address":"t9","name":"merger-app","state":"live","status":"working","cwd":"/clone"}`}
	for _, a := range terms {
		ts = append(ts, `{"address":"`+a+`","name":"아무 이름","state":"live","status":"working","cwd":"/x"}`)
	}
	sqWr(t, DATA+"/lsjson", `{"terminals":[`+strings.Join(ts, ",")+`]}`)
	sqWr(t, DATA+"/last.tsv", "lane-a\t제출\tabcd1234\t"+time.Now().Add(-2*time.Hour).Format("15:04")+"\tx → refs/merge-queue/1-lane-a\t-\tfresh\tr1\n")
	srMA(t, rec, why)
}

// srMA: 병합기 살아 있음 줄(기록 칸 포함)
func srMA(t *testing.T, rec *mergerstate.Rec, why string) {
	t.Helper()
	now := time.Now().Unix()
	r := "-"
	if rec != nil {
		rec.V = mergerstate.Version
		b, err := json.Marshal(rec)
		if err != nil {
			t.Fatal(err)
		}
		r = string(b)
	}
	sqWr(t, DATA+"/merger-alive.tsv", strings.Join([]string{"app", "m", "alive", itoa(now), "t9", "1", itoa(now - 28*60), itoa(now),
		"-", "-", "lane-a", "lane-a", r, dash(why)}, "\t")+"\n")
}

// srShards: 맨 앞 추측을 K몫으로 나눠 기다리는 기록 — 몫마다 기계·주소, 진행 prog 전
func srShards(addrs []string, prog time.Duration) *mergerstate.Rec {
	now := time.Now()
	r := &mergerstate.Rec{At: now.Unix() - 5, Progress: now.Add(-prog).Unix(), Machine: "m", Term: "t9", Base: "main",
		Phase: mergerstate.SpecWait, Since: now.Add(-28 * time.Minute).Unix(), Why: "추측 검증 기다림 — lane-a",
		Front: strings.Repeat("a", 40), Members: []string{"lane-a"}, Queue: []string{"lane-a"}}
	for i, a := range addrs {
		m, _, _ := strings.Cut(a, "/")
		r.Waits = append(r.Waits, mergerstate.Wait{Machine: m, Addr: a, Role: "몫 " + itoa(int64(i+1)) + "/" + itoa(int64(len(addrs))), At: now.Add(-28 * time.Minute).Unix()})
	}
	return r
}

// srJudge: 상태 한 장의 병합 줄 · merge-stall 항목 근거(없으면 빈 값)
func srJudge(t *testing.T) (opsstate.MergeLine, string, bool) {
	t.Helper()
	now := time.Now()
	var g opsstate.MergeLine
	for _, x := range stateCollect(now).Merge {
		if x.Project == "app" {
			g = x
		}
	}
	for _, it := range adviseNow(now).Items {
		if it.Key == "merge-stall:app" {
			return g, it.Evidence, true
		}
	}
	return g, "", false
}

// ① 완료 조건 1: merger_shards 2로 나눈 검증이 살아 진행 중이면 merge-stall·「살아 있는 … 터미널 없음」이 안 뜬다.
// 「마지막 바퀴」는 병합기가 남긴 진행 시각(30초 전)이다(완료 조건 3)
func TestStateRecShardsAlive(t *testing.T) {
	srFx(t, srShards([]string{"w/t3", "x/t4"}, 30*time.Second), "", "w/t3", "x/t4")
	g, ev, stall := srJudge(t)
	if stall || strings.Contains(g.Why, "터미널 없음") {
		t.Fatalf("① 진행 중인 나눔 검증에 안 움직임: %q · %+v", ev, g)
	}
	if g.LastRoundAgeS == nil || *g.LastRoundAgeS > 120 || g.Phase != mergerstate.SpecWait || len(g.Waits) != 2 || g.Waits[0].Live != "live" {
		t.Errorf("① 병합 줄: %+v", g)
	}
}

// ② 완료 조건 2: 병합기가 터미널 이름 꼴·나누기 수를 바꿔도 판정이 같다 — 기록에 아무 주소·몫 수를 넣어도
func TestStateRecNamesFree(t *testing.T) {
	for _, addrs := range [][]string{{"w/t3", "x/t4"}, {"w/zz-9", "x/q", "w/아무"}, {"x/t1"}} {
		srFx(t, srShards(addrs, 30*time.Second), "", addrs...)
		if g, ev, stall := srJudge(t); stall || len(g.Waits) != len(addrs) {
			t.Errorf("%v: 진행 중인데 %q %+v", addrs, ev, g)
		}
		// 같은 기록에서 일꾼이 정말 죽음(주소가 사라짐 + 진행 40분 전) — 경보, 근거에 그 주소와 터미널 없음
		srFx(t, srShards(addrs, 40*time.Minute), "")
		if _, ev, stall := srJudge(t); !stall || !strings.Contains(ev, addrs[0]+" 터미널 없음") || !strings.Contains(ev, "마지막 진행 40분 전") {
			t.Errorf("%v: 죽은 일꾼에 경보 없음·근거: %v %q", addrs, stall, ev)
		}
	}
}

// ⑥ 2026-11-26 22:4x 두 번째 거짓 경보(지휘 t1cj·t3yx·t34x 전달): 나눔 뒤 마무리 실행(몫 꼴이 아닌 보통 이름)과 나누지 않은 보통 추측
// 일꾼이 살아 진행 중인데 「살아 있는 spec 터미널 없음 · 마지막 바퀴 18분 전」. 역할이 무엇이든 기록의 주소로만 본다 — 경보 없음,
// 기다리는 대상의 주소가 빈 값이 아니고 살아 있음(옛 spec_term 빈 값이 원인)
// (추가 사례 2 — 23:0x 병합 검증 자리로 맡긴 2몫 나눔의 마무리: hc-fill이 specwait 기계 칸 「*<기계>」를 그대로 써 못 찾음. 이제 기계
// 칸을 풀지 않고 주소만 본다)
func TestStateRecRoles(t *testing.T) {
	for _, role := range []string{"마무리", "추측", "몫 2/2"} {
		r := srShards([]string{"box-1/t9z8"}, 30*time.Second)
		r.Waits[0].Role = role
		if role == "마무리" { // ⑧ 기계 칸이 어떤 꼴이든(병합 검증 자리의 「*」 접두 등) 판정은 주소로만 — 기계 칸을 풀지 않는다
			r.Waits[0].Machine = "*box-1"
		}
		srFx(t, r, "", "box-1/t9z8")
		g, ev, stall := srJudge(t)
		if stall || len(g.Waits) != 1 || g.Waits[0].Addr != "box-1/t9z8" || g.Waits[0].Live != "live" || strings.Contains(g.Why, "터미널 없음") {
			t.Errorf("%s: %v %q %+v", role, stall, ev, g)
		}
	}
}

// ⑦ 부하 대기(지휘 t1cj ②·t34x): 추측 일꾼이 검증 기계 부하가 기준 아래로 내려가길 기다리는 정상 대기 — 병합기가 단계 load-wait와
// 까닭(기계·부하·기준)을 쓰고, 일꾼의 부하 대기 표시가 갱신되는 동안은 진행이라 merge-stall이 안 뜬다. status 덧말에 「부하 대기」,
// state 단계 글에 기계·부하·기준. 「마지막 진행」은 병합기가 남긴 진행 시각(몇 초 전)이지 18분 전이 아니다
func TestStateRecLoadWait(t *testing.T) {
	r := srShards([]string{"box-1/t9z8"}, 20*time.Second)
	r.Waits[0].Role = "추측"
	r.Phase, r.Why = mergerstate.LoadWait, "추측 검증 부하 대기 — box-1 load=60.80 cpus=12 verifies=0 · 기준 부하 50% — lane-a"
	srFx(t, r, "", "box-1/t9z8")
	g, ev, stall := srJudge(t)
	if stall || g.Phase != mergerstate.LoadWait || !strings.Contains(g.PhaseWhy, "load=60.80") || !strings.Contains(g.PhaseWhy, "기준 부하 50%") ||
		g.LastRoundAgeS == nil || *g.LastRoundAgeS > 60 {
		t.Errorf("부하 대기: %v %q %+v", stall, ev, g)
	}
	if st, _, _ := sqLane(t); !strings.Contains(st, "묶음 검증 중 box-1 28분째 — 부하 대기") {
		t.Errorf("부하 대기 status 덧말: %q", st)
	}
}

// ③ 진행이 멎음(주소는 살아 있음) — 경보, 근거에 「진행 없음」
func TestStateRecStuck(t *testing.T) {
	srFx(t, srShards([]string{"w/t3", "x/t4"}, 40*time.Minute), "", "w/t3", "x/t4")
	if _, ev, stall := srJudge(t); !stall || !strings.Contains(ev, "w/t3") || strings.Contains(ev, "터미널 없음") {
		t.Errorf("③ 멈춘 일꾼: %v %q", stall, ev)
	}
}

// ④ 옛 판 병합기(기록 없음) — 짐작하지 않고 모름, merge-stall을 정하지 않는다
func TestStateRecOldMerger(t *testing.T) {
	srFx(t, nil, maOldMerger)
	g, ev, stall := srJudge(t)
	if stall || !strings.Contains(g.Why, "옛 판 병합기 — 다시 띄우면 기록") || g.LastRoundAgeS != nil {
		t.Errorf("④ 옛 판: %v %q %+v", stall, ev, g)
	}
}

// ⑤ 공개 선점을 기다리는 착지 — 「착지 검증 N분째」가 아니라 그 단계(누가 쥐었는지·몇 분째)로, merge-slow가 그 까닭을
func TestStateRecInnerPhase(t *testing.T) {
	now := time.Now()
	r := &mergerstate.Rec{At: now.Unix(), Progress: now.Unix() - 10, Machine: "m", Phase: mergerstate.Land, Since: now.Add(-40 * time.Minute).Unix(),
		Why: "착지 — lane-a(병합기 기계)", Members: []string{"lane-a"}, Queue: []string{"lane-a"},
		Inner: &mergerstate.Inner{Line: "공개 선점 기다림 — 쥔 쪽 ln-x(box-2)", At: now.Add(-12 * time.Minute).Unix()}}
	srFx(t, r, "")
	var slow string
	for _, it := range adviseNow(now).Items {
		if it.Key == "merge-slow:app" {
			slow = it.What
		}
		if it.Key == "merge-stall:app" {
			t.Errorf("⑤ 착지 중인데 안 움직임: %+v", it)
		}
	}
	if !strings.Contains(slow, "공개 선점 기다림 — 쥔 쪽 ln-x(box-2) 12분째") || strings.Contains(slow, "착지 검증") {
		t.Errorf("⑤ 느림 까닭: %q", slow)
	}
	if st, _, _ := sqLane(t); !strings.Contains(st, "착지 중 40분째 — 공개 선점 기다림 — 쥔 쪽 ln-x(box-2) 12분째") {
		t.Errorf("⑤ status 덧말: %q", st)
	}
}

// ⑥ 병합기를 바퀴 도중에 다시 띄움 — 새 병합기가 앞 기록을 이어 써 단계 시작·진행이 그대로면 경보가 거짓으로 뜨지 않는다
// (이어 쓰기는 병합기 쪽 시험 staterec_test.go가 본다 — 여기서는 기록의 터미널(term)이 바뀌어도 판정이 같음)
func TestStateRecRestarted(t *testing.T) {
	r := srShards([]string{"w/t3", "x/t4"}, 20*time.Second)
	r.Term = "t77"
	srFx(t, r, "", "w/t3", "x/t4")
	if _, ev, stall := srJudge(t); stall {
		t.Errorf("⑥ 다시 띄운 뒤 경보: %q", ev)
	}
}

// 지킴: hc-fill·opsstate의 판정 코드에 병합기 이름 꼴·기록 파일 꼴이 다시 생기면 실패(완료 조건 2 — 병합기가 바꿔도 여기는 안 바뀐다).
// 범위는 저장소 전체 훑기 표(2026-11-26 지휘 t3yx 요청)의 「판정」 칸: hc-fill·opsstate가 병합기 상태를 판정하는 곳이고, 병합기 이름 꼴로
// 진행을 재는 verified.FreshSh는 병합기만 부른다. hc-lane logs·stats(일지 이력·로그 모으기)·internal/landing(병합기가 올린 착지 중 표식 —
// 계약)·resident(상주 종류)·hcjson ServiceTerm(라벨)은 판정이 아니라 범위 밖
func TestStateRecNoMergerNames(t *testing.T) {
	fs, err := filepath.Glob("../../cmd/*/*.go")
	if err != nil {
		t.Fatal(err)
	}
	in, _ := filepath.Glob("../../internal/*/*.go")
	for _, f := range append(fs, in...) {
		if strings.HasSuffix(f, "_test.go") || strings.Contains(f, "/cmd/hc-merger/") || strings.Contains(f, "/internal/verified/") {
			continue
		}
		if b, err := os.ReadFile(f); err == nil && strings.Contains(string(b), "FreshSh") {
			t.Errorf("%s: verified.FreshSh(병합기 이름 꼴로 진행 재기)는 병합기만 부른다 — 병합기 기록을 읽는다", f)
		}
	}
	bad := regexp.MustCompile(`"(spec|land)-"|\.(spec|land)-wt|spec-\[|land-\[|specwait|specshard|merger-logs|hc-merger/child|"착지 실행"`)
	for _, dir := range []string{".", "../../internal/opsstate"} {
		fs, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range fs {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for i, l := range strings.Split(string(b), "\n") {
				if bad.MatchString(l) {
					t.Errorf("%s:%d 병합기 이름·기록 꼴: %s", f, i+1, strings.TrimSpace(l))
				}
			}
		}
	}
}
