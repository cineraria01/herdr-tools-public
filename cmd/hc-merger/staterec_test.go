//go:build !windows

package main

// ht-merger-state-record(2026-11-26 사용자 「구조를 다시 짜기」): 병합기가 자기 상태를 한 장(<기록 폴더>/state.json, internal/mergerstate)에
// 쓰고 hc-fill은 그것만 읽는다. 여기서는 쓰는 쪽 — 추측 기다림의 기다리는 대상(역할·주소 — 나누지 않은 보통 이름도)·진행, 부하 대기,
// 안쪽 단계, 다시 띄운 병합기의 이어 쓰기, 착지 중 맥박, 읽는 명령(--state). 고치기 전 코드에는 stSpecWait가 없어 컴파일부터 실패한다.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/mergerstate"
)

func recOf(t *testing.T, s *sfx) mergerstate.Rec {
	t.Helper()
	r, err := mergerstate.Read(s.clone + "/.git/hc-merger")
	if err != nil {
		t.Fatalf("기록: %v", err)
	}
	return r
}

// ① 나누지 않은 보통 추측 일꾼이 살아 진행 중 — 기다리는 대상에 그 터미널 주소(이름이 아니라 hc get이 준 주소), 진행 방금.
// 22:4x 두 번째 거짓 경보(살아 있는 spec-e4e8e9f3인데 hc-fill spec_term 빈 값)의 병합기 쪽
func TestStateRecSpecWaitAddr(t *testing.T) {
	s, d, _ := frontFx(t, 100)
	s.specTerm("v1", d[1], 0)
	s.round(1)
	r := recOf(t, s)
	if r.Phase != mergerstate.SpecWait || len(r.Waits) != 1 || r.Waits[0].Addr != "v1/t9" || r.Waits[0].Role != "추측" || r.Waits[0].Machine != "v1" ||
		time.Now().Unix()-r.Progress > 5 || len(r.Members) != 1 || r.Members[0] != "p" || r.Front == "" || r.Machine != "box" {
		t.Fatalf("① 추측 기다림 기록: %+v", r)
	}
	// ④ 병합기를 다시 띄움(새 일꾼 — s.round는 바퀴마다 새 M) — 같은 맨 앞이면 단계 시작·진행을 잇고, 잴 때가 아니면 재지 않는다
	since, prog := r.Since, r.Progress
	time.Sleep(1100 * time.Millisecond)
	s.round(1)
	if r2 := recOf(t, s); r2.Since != since || r2.Progress != prog || r2.Waits[0].Addr != "v1/t9" || r2.At <= r.At {
		t.Fatalf("④ 다시 띄운 병합기가 이어 쓰기: 앞 %+v 뒤 %+v", r, r2)
	}
	// 일꾼이 정말 죽음(터미널 없음) — 재도 진행이 아니고 주소는 앞 것 그대로(hc-fill이 그 주소가 사라졌음을 본다)
	s.probe = -1
	os.Remove(s.dir + "/idle/get-spec-" + d[1][:8])
	time.Sleep(1100 * time.Millisecond)
	s.round(1)
	if r3 := recOf(t, s); r3.Progress != prog || r3.Waits[0].Addr != "v1/t9" || r3.Probe <= r.Probe {
		t.Fatalf("죽은 일꾼: %+v", r3)
	}
}

// ② 부하 대기(지휘 t1cj ②·t34x — 22:4x 검증 기계 부하 60.8이 기준 50 아래로 내려가길 기다리던 정상 대기): 일꾼이 쓴 <key>.wait 줄 →
// 단계 load-wait, 까닭에 기계·부하·기준. 안쪽 단계(검증 스크립트가 HERDR_LAND_PHASE에 쓴 줄 — 착지와 같은 계약) → 기록의 inner
func TestStateRecLoadWaitInner(t *testing.T) {
	s, d, _ := frontFx(t, 100)
	s.probe = -1
	s.specTerm("v1", d[1], 0)
	wt := s.dir + "/v1.spec-wt/" + d[1][:8]
	os.WriteFile(wt+".wait", []byte("부하 대기 — v1 load=60.80 cpus=12 verifies=0 · 기준 부하 50%\n"), 0o666)
	s.round(1)
	r := recOf(t, s)
	if r.Phase != mergerstate.LoadWait || !strings.Contains(r.Why, "load=60.80") || !strings.Contains(r.Why, "기준 부하 50%") || r.Waits[0].Addr != "v1/t9" {
		t.Fatalf("② 부하 대기: %+v", r)
	}
	// 부하가 내려가 검증 시작(.wait 지움) — 같은 기다림이라 단계 시작은 잇고, 검증 스크립트의 안쪽 단계가 보인다
	os.Remove(wt + ".wait")
	os.WriteFile(wt+".phase", []byte("go 시험 3/8 묶음\n"), 0o666)
	time.Sleep(1100 * time.Millisecond)
	s.round(1)
	r2 := recOf(t, s)
	if r2.Phase != mergerstate.SpecWait || r2.Since != r.Since || r2.Inner == nil || r2.Inner.Line != "go 시험 3/8 묶음" {
		t.Fatalf("② 부하 대기 끝·안쪽 단계: %+v", r2)
	}
}

// ③ 나눔 몫·마무리 — 역할 「몫 i/K」, 주소는 몫마다(이름 꼴은 병합기만 안다)
func TestStateRecShards(t *testing.T) {
	s, d, _ := frontFx(t, 100)
	for _, i := range []string{"1", "2"} {
		n := "spec-" + d[1][:8] + "-" + i
		os.WriteFile(s.dir+"/idle/get-"+n, []byte(`{"address":"v`+i+`/t`+i+`","state":"live","cwd":"`+s.dir+`/v`+i+`","name":"`+n+`","status":"working","pid":1,"created_ms":1}`+"\n"), 0o666)
	}
	mj := s.clone + "/.git/hc-merger"
	// 맨 앞 제출 SHA는 기록이 맞춘다(나눔 상태의 front 칸은 이 시험에서 쓰지 않는다)
	os.WriteFile(mj+"/specshard", []byte(d[1]+" - "+itoa64(time.Now().Unix())+" k=2 1:v1 2:v2\n"), 0o666)
	s.round(1)
	r := recOf(t, s)
	if r.Phase != mergerstate.SpecWait || len(r.Waits) != 2 || r.Waits[0].Role != "몫 1/2" || r.Waits[1].Addr != "v2/t2" || !strings.Contains(r.Why, "나눔 2") {
		t.Fatalf("③ 나눔: %+v", r)
	}
	// 마무리(fin) — 보통 이름의 일꾼 하나, 역할 마무리
	os.WriteFile(mj+"/specshard", []byte(d[1]+" - "+itoa64(time.Now().Unix())+" k=2 1:v1 2:v2 fin=v1@1\n"), 0o666)
	s.specTerm("v1", d[1], 0)
	s.probe = -1
	s.round(1)
	if r := recOf(t, s); len(r.Waits) != 1 || r.Waits[0].Role != "마무리" || r.Waits[0].Addr != "v1/t9" {
		t.Fatalf("③ 마무리: %+v", r)
	}
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }

// 범인 가르기(묶음 실패 뒤 앞 무리 추측을 기다림 — prefixWait) — 앞 무리 추측마다 기다리는 대상(역할·주소)
func TestStateRecCulpritPrefix(t *testing.T) {
	s := specFx(t)
	id := strings.Repeat("c", 40)
	s.specTerm("v1", id, 0)
	m := s.merger()
	m.b = []entry{{l: "ln-a", s: strings.Repeat("a", 40)}, {l: "ln-b", s: strings.Repeat("b", 40)}}
	m.wpre = []specRow{{id: id, machine: "v1", at: time.Now().Unix() - 60}}
	m.stSpecWait()
	r := recOf(t, s)
	if r.Phase != mergerstate.SpecWait || !strings.Contains(r.Why, "범인 가르기") || len(r.Waits) != 1 || r.Waits[0].Role != "앞 무리 추측" ||
		r.Waits[0].Addr != "v1/t9" || time.Now().Unix()-r.Progress > 5 {
		t.Fatalf("범인 가르기: %+v", r)
	}
}

// 추가 사례 2(지휘 t34x, 23:0x): 병합 검증 자리로 맡긴 맨 앞(일지·specwait의 기계 칸 「*<기계>」)의 나눔 뒤 마무리 검증 — hc-fill이
// 「*<기계>」를 기계 이름으로 써서 터미널을 못 찾았다. 기록에는 * 없는 기계와 hc get이 준 주소가 그대로(역할 마무리)
func TestStateRecGateFinish(t *testing.T) {
	s := specFx(t)
	id := strings.Repeat("e", 40)
	s.specTerm("v1", id, 0)
	m := s.merger()
	m.b = []entry{{l: "ln-a", s: strings.Repeat("a", 40)}}
	os.WriteFile(m.mj+"/specshard", []byte(id+" - 1 k=2 1:v1 2:v2 fin=v1@1\n"), 0o666)
	m.wmach, m.wrow = "*v1", specRow{id: id, machine: "v1", at: time.Now().Unix() - 60}
	m.stSpecWait()
	r := recOf(t, s)
	if len(r.Waits) != 1 || r.Waits[0].Machine != "v1" || r.Waits[0].Role != "마무리" || r.Waits[0].Addr != "v1/t9" || strings.Contains(r.Why, "*") {
		t.Fatalf("병합 검증 자리 마무리: %+v", r)
	}
}

// ⑤ 병합기 기계 착지 — 단계 land, 맥박이 beatEvery마다 다시 쓰고(진행), 착지 스크립트가 HERDR_LAND_PHASE에 쓴 줄을 안쪽 단계로.
// 멈춘 뒤의 맥박은 쓰지 않는다. 바퀴 끝(stEnd)은 착지 단계를 끝내고 줄 빔이면 idle
func TestStateRecLandBeat(t *testing.T) {
	mj := t.TempDir()
	m := &M{mj: mj, mme: "box", base: "main", mlab: "ln-a", b: []entry{{l: "ln-a", s: strings.Repeat("a", 40)}}}
	m.rs.beatEvery = 20 * time.Millisecond
	t.Cleanup(m.rs.beats.Wait) // 임시 폴더를 지우기 전에 맥박이 끝난다(Cleanup은 거꾸로 돈다)
	pf, stop := m.stLandLocal()
	if pf != filepath.Join(mj, landPhaseLocal) {
		t.Fatalf("안쪽 단계 파일: %s", pf)
	}
	r0, _ := mergerstate.Read(mj)
	os.WriteFile(pf, []byte("공개 선점 기다림 — 쥔 쪽 ln-x(box-2)\n"), 0o666)
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if r, _ := mergerstate.Read(mj); r.Inner != nil {
			break
		}
	}
	stop()
	r, err := mergerstate.Read(mj)
	if err != nil || r.Phase != mergerstate.Land || r.Inner == nil || r.Inner.Line != "공개 선점 기다림 — 쥔 쪽 ln-x(box-2)" || r.At < r0.At || len(r.Members) != 1 {
		t.Fatalf("⑤ 착지 맥박: %+v %v", r, err)
	}
	m.rs.queue = []string{}
	m.stEnd(76)
	m.rs.beats.Wait() // 멈춘 뒤 한 번 더 돈 맥박도 끝났다 — 그 맥박이 덮어쓰지 않았다
	if r, _ := mergerstate.Read(mj); r.Phase != mergerstate.Idle || r.Inner != nil {
		t.Fatalf("⑤ 바퀴 끝: %+v", r)
	}
}

// 워커 착지(land-on) — 맥박이 그 워커의 착지 터미널을 재어 주소·진행·안쪽 단계(<클론>.land-wt/<차례 12>.phase)를 적고, 착지 실행 결정을 싣는다
func TestStateRecLandRemoteBeat(t *testing.T) {
	s := specFx(t)
	cm := strings.Repeat("d", 40)
	n := "land-" + cm[:8]
	os.WriteFile(s.dir+"/idle/get-"+n, []byte(`{"address":"v1/t7","state":"live","cwd":"`+s.dir+`/v1","name":"`+n+`","status":"working","pid":1,"created_ms":1}`+"\n"), 0o666)
	wt := s.dir + "/v1.land-wt/" + cm[:12]
	os.MkdirAll(filepath.Dir(wt), 0o777)
	os.WriteFile(wt+".log", []byte("go test ...\n"), 0o666)
	os.WriteFile(wt+".phase", []byte("공개 선점 기다림 — 쥔 쪽 ln-x(box-2)\n"), 0o666)
	m := s.merger()
	m.mlab, m.b = "ln-a", []entry{{l: "ln-a", s: strings.Repeat("a", 40)}}
	m.rs.beatEvery, m.rs.probeEvery = 20*time.Millisecond, -1
	t.Cleanup(m.rs.beats.Wait) // 멈춘 뒤 한 번 더 도는 맥박이 가짜 hc·임시 폴더를 다음 시험까지 끌지 않게
	m.stLandOn("v1(검증 자리 남음)")
	stop := m.stLandRemote("v1", cm)
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end) && recOf(t, s).Inner == nil; {
		time.Sleep(50 * time.Millisecond) // 맥박 한 번(가짜 hc get·exec)이 쓸 때까지
	}
	stop()
	r := recOf(t, s)
	if r.Phase != mergerstate.LandOn || len(r.Waits) != 1 || r.Waits[0].Addr != "v1/t7" || r.Waits[0].Role != "착지 실행" || r.LandOn != "v1(검증 자리 남음)" ||
		r.Inner == nil || r.Inner.Line != "공개 선점 기다림 — 쥔 쪽 ln-x(box-2)" || time.Now().Unix()-r.Progress > 5 {
		t.Fatalf("워커 착지 맥박: %+v", r)
	}
}

// 읽는 명령: hc-merger --state <클론> — 기록 그대로(0), 없음 3(옛 판 병합기), 깨짐 1
func TestStateMain(t *testing.T) {
	d := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", d).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	if rc := stateMain([]string{d}); rc != 3 {
		t.Fatalf("기록 없음: %d", rc)
	}
	mj := filepath.Join(d, ".git", "hc-merger")
	os.MkdirAll(mj, 0o777)
	if err := mergerstate.Write(mj, mergerstate.Rec{Phase: mergerstate.Idle}); err != nil {
		t.Fatal(err)
	}
	if rc := stateMain([]string{d}); rc != 0 {
		t.Fatalf("기록 있음: %d", rc)
	}
	os.WriteFile(filepath.Join(mj, mergerstate.File), []byte(`{"v":99}`), 0o666)
	if rc := stateMain([]string{d}); rc != 1 {
		t.Fatalf("모르는 판: %d", rc)
	}
}
