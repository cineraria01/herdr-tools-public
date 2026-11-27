//go:build !windows

package main

// ht-merger-dead-worker(2026-11-28 — 검증 기계 한 대가 맨 앞 추측 도중 전원이 꺼졌는데 병합기는 맨 앞 기다림 상한 1500초까지 「결과를
// 기다린다」를 되풀이했다): 맡긴 일꾼이 죽었다고 확신하면(그 기계가 연속으로 deadAfter초 넘게 안 닿음 · 닿는 기계에 일꾼 터미널이 없고 결과도
// 없음) 상한을 기다리지 않고 「모름」으로 — 지금 모름 길(한가한 다른 기계에 다시 맡김 → 없으면 직접 검증) 그대로.
// 틀은 specfrontwait_test.go(frontFx — v1에 띄운 맨 앞 추측, 떴다는 표식 있음)·specoverrun_test.go(specTerm).

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/verified"
)

// machines: 가짜 hc machine ls --json — 기계=상태 쌍
func (s *sfx) machines(kv ...string) {
	s.t.Helper()
	var ms []string
	for _, x := range kv {
		n, st, _ := strings.Cut(x, "=")
		ms = append(ms, fmt.Sprintf(`{"name":"%s","state":"%s","version":"0.1.22"}`, n, st))
	}
	if err := os.WriteFile(s.dir+"/idle/machines", []byte(`{"machines":[`+strings.Join(ms, ",")+"]}\n"), 0o666); err != nil {
		s.t.Fatal(err)
	}
}

// downSince: 앞 바퀴들이 그 기계를 first초 전부터 last초 전까지 안 닿음으로 본 기록(병합기 일지 폴더 specdead)
func (s *sfx) downSince(mach string, first, last int64) {
	s.t.Helper()
	now := time.Now().Unix()
	if err := os.WriteFile(s.clone+"/.git/hc-merger/"+deadFile, []byte(fmt.Sprintf("m %s %d %d\n", mach, now-first, now-last)), 0o666); err != nil {
		s.t.Fatal(err)
	}
}

func (s *sfx) deadState() string {
	b, _ := os.ReadFile(s.clone + "/.git/hc-merger/" + deadFile)
	return string(b)
}

// ① 맡긴 기계가 문턱 넘게 안 닿음(상한 안) → 놓고 한가한 v2에 다시 맡김, 일지 한 줄 · v2도 바쁘면 직접 검증(까닭 「추측 끊김」)
func TestDeadUnreachMoves(t *testing.T) {
	s, d, m0 := frontFx(t, 100)
	s.machines("v1=unreachable", "v2=connected")
	s.downSince("v1", 200, 5)
	s.round(1)
	if got := s.dispatched(); len(s.lines(s.landlog)) != 0 || s.mainSHA() != m0 || len(got) != 2 || got[1][0] != "v2" || got[1][1] != d[1] {
		t.Fatalf("① 끊긴 v1의 맨 앞은 v2로: 착지 %v 띄움 %v", s.lines(s.landlog), got)
	}
	if n := jcount(jDead, "추측 일꾼 끊김 — p (v1, 안 닿은 지 20"); n != 1 {
		t.Fatalf("① 일지 한 줄: %d\n%s", n, jdump())
	}
	if n := jcount(jDead, "→ 다른 기계에 다시 맡김(v1→v2)"); n != 1 {
		t.Fatalf("① 일지의 다음 길: %d\n%s", n, jdump())
	}
	// 두 번째 실행(같은 바퀴 또 돎): v2는 닿고 도는 중 — 다시 옮기지도, 일지를 또 쓰지도 않는다
	s.round(1)
	if got := s.dispatched(); len(got) != 2 || len(s.lines(s.landlog)) != 0 || jcount(jDead, "") != 1 {
		t.Fatalf("① 두 번째 바퀴는 그대로 기다림: 띄움 %v 착지 %v 일지 %d", got, s.lines(s.landlog), jcount(jDead, ""))
	}
	// ⑤ 놓은 뒤 v1이 돌아와 늦은 통과를 냄 — 통과 기록은 트리 기준이라 그대로 쓴다(검증 없이 착지), 뒤에 v2가 끝나도 이중 착지·반송 없음.
	// (옛 일꾼이 옛 spec 커밋을 쥐고 있으면 specWrite가 결과를 버린다 — 여기서는 기록을 남긴 쪽, 더 나쁜 경우를 본다)
	if rc := s.verifyOn(d); rc != 0 {
		t.Fatalf("v1 일꾼 종료 %d", rc)
	}
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|p" {
		t.Fatalf("⑤ 늦은 통과로 검증 없이 착지: %q", got)
	}
	s.verifyOn([2]string{"v2", d[1]})
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|p" || len(s.refs("refs/merge-failed/")) != 0 {
		t.Fatalf("⑤ v2가 뒤에 끝나도 이중 착지·반송 없음: %q %v", got, s.refs("refs/merge-failed/"))
	}
}

func TestDeadUnreachDirect(t *testing.T) {
	s, _, _ := frontFx(t, 100)
	s.idle("v2", verifyBusy)
	s.machines("v1=unreachable", "v2=connected")
	s.downSince("v1", 200, 5)
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|p" || len(s.dispatched()) != 1 {
		t.Fatalf("① 맡길 기계가 없으면 직접 검증: %q 띄움 %v", got, s.dispatched())
	}
	if w := s.jwhy("p", "착지"); !strings.HasSuffix(w, ", 직접 검증(추측 끊김)") {
		t.Fatalf("① 까닭 %q", w)
	}
	if n := jcount(jDead, "→ 직접 검증"); n != 1 {
		t.Fatalf("① 일지 다음 길 「직접 검증」: %d\n%s", n, jdump())
	}
}

// ② 문턱 전·한 번 실패·처음 봄·바퀴 사이가 김(병합기가 한참 안 돎)·모르는 상태 낱말·목록에 없는 기계 → 그대로 기다림
func TestDeadNotYetWaits(t *testing.T) {
	for _, c := range []struct {
		name        string
		first, last int64 // 0이면 기록 없음
		ms          []string
		fresh       bool // 바퀴 뒤 기록이 지금부터 다시 셈이어야
	}{
		{"처음 봄", 0, 0, []string{"v1=unreachable", "v2=connected"}, true},
		{"문턱 전", 60, 5, []string{"v1=unreachable", "v2=connected"}, false},
		{"hc 못 읽음", 200, 5, nil, false},
		{"바퀴 사이가 김", 900, 600, []string{"v1=unreachable", "v2=connected"}, true},
		{"모르는 낱말", 200, 5, []string{"v1=sleeping", "v2=connected"}, false},
		{"기계 이름 바뀜", 200, 5, []string{"v1-new=connected", "v2=connected"}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, _, m0 := frontFx(t, 100)
			if c.ms != nil {
				s.machines(c.ms...)
			}
			if c.first > 0 {
				s.downSince("v1", c.first, c.last)
			}
			s.round(1)
			if len(s.lines(s.landlog)) != 0 || s.mainSHA() != m0 || len(s.dispatched()) != 1 || jcount(jDead, "") != 0 {
				t.Fatalf("② 기다림: 착지 %v 띄움 %v\n%s", s.lines(s.landlog), s.dispatched(), jdump())
			}
			if c.fresh {
				var first, last int64
				fmt.Sscanf(s.deadState(), "m v1 %d %d", &first, &last)
				if now := time.Now().Unix(); now-first > 5 || first != last {
					t.Fatalf("② 지금부터 다시 셈: %q", s.deadState())
				}
			}
		})
	}
}

// ② 연결이 돌아오면 기록을 지운다(잠깐 끊김을 다음 끊김에 이어 세지 않음)
func TestDeadReconnectClears(t *testing.T) {
	s, d, _ := frontFx(t, 100)
	s.specTerm("v1", d[1], 0)
	s.machines("v1=connected", "v2=connected")
	s.downSince("v1", 100, 5)
	s.round(1)
	if strings.Contains(s.deadState(), "m v1 ") || len(s.dispatched()) != 1 {
		t.Fatalf("② 다시 닿으면 기록을 지우고 기다림: %q 띄움 %v", s.deadState(), s.dispatched())
	}
}

// ③ 기계는 닿는데 일꾼 터미널이 없고 결과 기록도 없음 → 놓고 v2에(일지 「일꾼 터미널 없음」)
func TestDeadNoTerminalMoves(t *testing.T) {
	s, d, _ := frontFx(t, 100)
	s.machines("v1=connected", "v2=connected")
	s.round(1)
	if got := s.dispatched(); len(got) != 2 || got[1][0] != "v2" || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("③ 일꾼이 사라진 맨 앞은 v2로: 띄움 %v 착지 %v", got, s.lines(s.landlog))
	}
	if n := jcount(jDead, "추측 일꾼 끊김 — p (v1, 일꾼 터미널 없음) → 다른 기계에 다시 맡김(v1→v2)"); n != 1 {
		t.Fatalf("③ 일지: %d\n%s", n, jdump())
	}
	_ = d
}

// ③ 터미널이 없다고 읽는 그 사이 일꾼이 통과 기록을 남기고 끝남 → 놓지 않고 그 기록으로 착지(다음 바퀴)
func TestDeadNoTerminalRaceKeeps(t *testing.T) {
	s, d, _ := frontFx(t, 100)
	s.machines("v1=connected", "v2=connected")
	tree := s.specTree(d[1])
	if rc := s.verifyOn(d); rc != 0 {
		t.Fatalf("일꾼 종료 %d", rc)
	}
	ref := verified.Prefix + tree
	sha := s.git(s.dir, "--git-dir="+s.origin, "rev-parse", ref)
	s.git(s.dir, "--git-dir="+s.origin, "update-ref", "-d", ref) // 바퀴의 ls-remote 뒤에 생기게 — hc get 때 되살린다
	hook := fmt.Sprintf("#!/bin/sh\ngit --git-dir=%s update-ref %s %s\n", s.origin, ref, sha)
	os.WriteFile(s.dir+"/idle/get-hook", []byte(hook), 0o777)
	s.round(1)
	if len(s.dispatched()) != 1 || len(s.lines(s.landlog)) != 0 || jcount(jDead, "") != 0 {
		t.Fatalf("③ 결과가 생겼으면 놓지 않는다: 띄움 %v 착지 %v\n%s", s.dispatched(), s.lines(s.landlog), jdump())
	}
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|p" {
		t.Fatalf("③ 그 기록으로 착지: %q", got)
	}
}

// ④ 일꾼이 살아 있고 진행 중 → 기다림(상한 넘겨도 지금 규칙대로 — 진행 중이라 더 기다림)
func TestDeadAliveWaits(t *testing.T) {
	for _, sec := range []int64{100, 700} {
		s, d, m0 := frontFx(t, sec)
		s.specTerm("v1", d[1], 0)
		s.machines("v1=connected", "v2=connected")
		s.round(1)
		if len(s.lines(s.landlog)) != 0 || s.mainSHA() != m0 || len(s.dispatched()) != 1 || jcount(jDead, "") != 0 {
			t.Fatalf("④ %d초 살아 도는 추측은 기다림: 착지 %v 띄움 %v", sec, s.lines(s.landlog), s.dispatched())
		}
	}
}

// ⑥ 직접 검증으로 넘긴 뒤 옛 기계의 늦은 실패 → 이미 착지한 레인을 반송하지 않는다 · 늦은 통과 → 이중 착지 없음(afterLate)
func TestDeadLateResults(t *testing.T) {
	for _, late := range []string{"pass", "fail"} {
		t.Run(late, func(t *testing.T) {
			s, d, _ := frontFx(t, 100)
			s.idle("v2", verifyBusy)
			s.machines("v1=unreachable", "v2=connected")
			s.downSince("v1", 200, 5)
			s.round(1)
			if got := strings.Join(s.lines(s.landlog), " "); got != "verify|p" {
				t.Fatalf("직접 검증: %q", got)
			}
			if late == "pass" {
				if rc := s.verifyOn(d); rc != 0 {
					t.Fatalf("일꾼 종료 %d", rc)
				}
			} else {
				et := s.git(s.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
				fc := s.git(s.seed, "commit-tree", et, "-m", fmt.Sprintf("hc-merger-spec-fail id=%s machine=v1 at=%d log=- exit=1 test=1", d[1], time.Now().Unix()))
				s.git(s.seed, "push", "-q", "origin", fc+":"+specFails+d[1])
			}
			s.afterLate("⑥ " + late)
		})
	}
}

// ⑦ 앞 무리·범인 가리기·나눠 돌리기 몫·뒤 무리에도 같은 판정 — 도는 추측이면 어느 자리든(specDead는 줄마다 잰다)
func TestDeadAnyRowAndShards(t *testing.T) {
	s := specFx(t)
	m := s.merger()
	now := time.Now().Unix()
	id1, id2, id3 := strings.Repeat("1", 40), strings.Repeat("2", 40), strings.Repeat("3", 40)
	row := func(id, mach string) specRow {
		return specRow{id: id, machine: mach, group: "g" + id[:1], at: now - 100, tree: strings.Repeat("a", 40), state: specRun}
	}
	c := &specCtx{refs: map[string]string{}, rows: []specRow{row(id1, "v1"), row(id2, "v2"), row(id3, "box")}}
	for _, id := range []string{id1, id2, id3} {
		c.refs[specStarts+id] = "x"
	}
	s.machines("v1=unreachable", "v2=connected", "v3=unreachable")
	os.WriteFile(m.mj+"/"+deadFile, []byte(fmt.Sprintf("m v1 %d %d\nm v3 %d %d\n", now-200, now-5, now-200, now-5)), 0o666)
	s.specTerm("v2", id2, 0)
	m.specDead(c, nil, now)
	if c.rows[0].state != specLost || c.rows[0].lost != "끊김" || c.rows[1].state != specRun || c.rows[2].state != specRun {
		t.Fatalf("⑦ 끊긴 v1의 줄만 모름(병합기 기계 줄은 재지 않음): %+v", c.rows)
	}
	// 나눔: 몫 1(v3, 안 닿음)이 안 끝났으면 맨 앞은 모름 · 몫 1이 끝났고 몫 2(v2)가 살아 있으면 그대로
	c.rows = []specRow{row(id2, "v2")}
	os.WriteFile(m.mj+"/specshard", []byte(fmt.Sprintf("%s front %d k=2 1:v3 2:v2\n", id2, now-100)), 0o666)
	s.specTerm("v2", id2+"x", 0) // 몫 터미널 이름은 spec-<아이디 8>-<i>
	os.Rename(s.dir+"/idle/get-spec-"+id2[:8], s.dir+"/idle/get-spec-"+id2[:8]+"-2")
	m.specDead(c, nil, now)
	if c.rows[0].state != specLost {
		t.Fatalf("⑦ 안 끝난 몫의 기계가 끊기면 맨 앞은 모름: %+v", c.rows[0])
	}
	c.rows = []specRow{row(id2, "v2")}
	c.refs[specShards+id2+"-s1of2"] = "x"
	m.specDead(c, nil, now)
	if c.rows[0].state != specRun {
		t.Fatalf("⑦ 끝난 몫의 기계는 보지 않음 · 남은 몫이 살아 있으면 그대로: %+v", c.rows[0])
	}
}
