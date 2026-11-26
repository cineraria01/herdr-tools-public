//go:build !windows

package main

// ht-shard-busy-redispatch(2026-11-26 22:49 — 나눔 2의 몫 1이 검증 자리를 30초 안에 못 얻어 일 없이 「바쁨」으로 끝났고, 마무리가 몫 2가
// 끝난 뒤 그 절반을 처음부터 10분 넘게 혼자 돌았다. 그때 빈 기계가 있었다): 몫이 일 없이 바쁨(끝 state=busy·통과 조각 0)으로 끝나면 그
// 바퀴에 같은 몫(같은 번호·같은 아이디)을 다른 한가한 기계에 다시 맡긴다 — 보통 추측의 「바쁨 → 다른 기계로」와 같은 상한 busy_max(몫마다).
// 틀은 shards_test.go(shardFx): 나눔 2(v1 몫 1/2 = p1·p3, v2 몫 2/2 = p2), v3은 빈 기계

import (
	"os"
	"strings"
	"testing"

	"herdr-tools/internal/verified"
)

// twoShards: 맨 앞 a를 v1·v2에 둘로 나눠 띄운 것
func twoShards(t *testing.T, s *sfx) [][3]string {
	t.Helper()
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.launches()
	if len(d) != 2 || d[0][0] != "v1" || d[0][2] != "1/2" || d[1][0] != "v2" || d[1][2] != "2/2" || d[1][1] != d[0][1] {
		t.Fatalf("나눔 둘(v1 1/2 · v2 2/2): %v", d)
	}
	return d
}

// busyShard: 그 몫 일꾼이 검증 자리를 busy_wait(0) 안에 못 얻어 일 없이 바쁨으로 끝남(실제 일꾼)
func (s *sfx) busyShard(d [3]string) {
	s.t.Helper()
	s.t.Setenv("HC_LANE_SPEC_BUSY_WAIT", "0")
	old := idleProbe
	idleProbe = func() (idleInfo, bool) { return parseIdle(verifyBusy) }
	defer func() { idleProbe = old }()
	if rc := s.runOn(d); rc != 0 {
		s.t.Fatalf("바쁨 몫 %v 종료 %d", d, rc)
	}
	if !s.has(specShards+d[1]+"-s"+d[2][:1]+"of2") || s.has(specParts+d[1]+"-s"+d[2][:1]) {
		s.t.Fatalf("바쁨 몫은 끝 참조만(조각 없음): %v %v", s.refs(specShards), s.refs(specParts))
	}
}

// ① 몫 1 바쁨(조각 0) + 빈 기계 v3 → 그 바퀴에 v3에 몫 1/2를 다시 맡김(기록의 기다리는 대상에도 v3) → 마무리는 빈틈 0 → 착지
func TestShardBusyRedispatched(t *testing.T) {
	s, hl, _ := shardFx(t, "2")
	d := twoShards(t, s)
	s.busyShard(d[0])
	if got := ran(t, hl); got != "" {
		t.Fatalf("바쁨 몫은 일하지 않는다: %q", got)
	}
	s.round(1)
	l := s.launches()
	if len(l) != 3 || l[2] != [3]string{"v3", d[0][1], "1/2"} || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("① 빈 v3에 같은 몫 1/2를 다시: %v 착지 %v\n%s", l, s.lines(s.landlog), jdump())
	}
	if s.has(specShards + d[0][1] + "-s1of2") {
		t.Fatal("① 다시 맡긴 몫의 바쁨 끝 참조는 지운다(마무리가 그 몫을 끝남으로 보지 않게)")
	}
	r := recOf(t, s)
	if len(r.Waits) != 2 || r.Waits[0].Machine != "v3" || r.Waits[0].Role != "몫 1/2" || r.Waits[1].Machine != "v2" || r.Waits[1].Role != "몫 2/2" {
		t.Fatalf("① 기록의 기다리는 대상에 다시 맡긴 몫(v3): %+v", r.Waits)
	}
	s.runOn(l[2])
	s.runOn(d[1])
	if got := ran(t, hl); got != "run p1@v3\nrun p3@v3\nrun p2@v2" {
		t.Fatalf("① v3가 몫 1의 조각을: %q", got)
	}
	s.round(1)
	l = s.launches()
	if len(l) != 4 || l[3][2] != "" {
		t.Fatalf("① 몫이 다 끝나 마무리: %v", l)
	}
	s.runOn(l[3])
	if got := ran(t, hl); got != "" {
		t.Fatalf("① 마무리는 빈틈 0: %q", got)
	}
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|a" {
		t.Fatalf("① 착지: %q\n%s", got, jdump())
	}
	if w := jrows(t, "추측 기다림"); len(w) != 1 || !strings.Contains(w[0][11], "바쁨으로 다시 고름(v1→v3)") {
		t.Fatalf("① 일지 「바쁨으로 다시 고름(v1→v3)」:\n%s", jdump())
	}
}

// ② 빈 기계 없음 → 지금 길(다시 맡기지 않고, 몫 2가 끝나면 마무리가 몫 1의 조각을 돈다)
func TestShardBusyNoMachineFinishRuns(t *testing.T) {
	s, hl, _ := shardFx(t, "2")
	d := twoShards(t, s)
	s.idle("v3", verifyBusy)
	s.busyShard(d[0])
	s.round(1)
	if l := s.launches(); len(l) != 2 {
		t.Fatalf("② 맡길 기계가 없으면 다시 맡기지 않는다: %v", l)
	}
	s.runOn(d[1])
	ran(t, hl)
	s.round(1)
	l := s.launches()
	if len(l) != 3 || l[2][2] != "" {
		t.Fatalf("② 마무리: %v", l)
	}
	s.runOn(l[2])
	if got := ran(t, hl); got != "run p1@"+l[2][0]+"\nrun p3@"+l[2][0] {
		t.Fatalf("② 마무리가 몫 1의 조각을: %q", got)
	}
}

// ③ 조각을 일부 남긴 채 끝난 몫(환경 실패)은 빈 기계가 있어도 다시 맡기지 않고 지금 그대로 마무리가 이어 쓴다
func TestShardEnvWithPartsNotRedispatched(t *testing.T) {
	s, hl, pd := shardFx(t, "2")
	d := twoShards(t, s)
	os.WriteFile(pd+"/envfail-p3-v1", nil, 0o666)
	s.runOn(d[0])
	if got := ran(t, hl); got != "run p1@v1\nrun p3@v1" || !s.has(specParts+d[0][1]+"-s1") {
		t.Fatalf("③ 몫 1은 p1 통과 뒤 p3에서 환경 실패: %q %v", got, s.refs(specParts))
	}
	s.round(1)
	if l := s.launches(); len(l) != 2 {
		t.Fatalf("③ 환경 실패 몫은 다시 맡기지 않는다: %v", l)
	}
	s.runOn(d[1])
	ran(t, hl)
	s.round(1)
	l := s.launches()
	if len(l) != 3 || l[2][2] != "" {
		t.Fatalf("③ 마무리: %v", l)
	}
	s.runOn(l[2])
	if got := ran(t, hl); got != "run p3@"+l[2][0] {
		t.Fatalf("③ 마무리는 남은 p3만: %q", got)
	}
}

// ④ 다시 맡기기 횟수 상한(busy_max, 몫마다): 1이면 v1 바쁨 → v3 한 번, 몫 2가 아직 도는데 v3도 바쁨이면 빈 v4가 있어도 다시 맡기지
// 않고, 몫 2가 끝나면 마무리가
func TestShardBusyRedispatchCap(t *testing.T) {
	s, hl, _ := shardFx(t, "2")
	s.git(s.dir, "clone", "-q", s.origin, s.dir+"/v4")
	var vs []verified.Verifier
	for _, v := range []string{"v1", "v2", "v3", "v4"} {
		vs = append(vs, verified.Verifier{Machine: v, Path: s.dir + "/" + v})
	}
	et := s.git(s.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
	s.git(s.seed, "push", "-q", "-f", "origin", s.git(s.seed, "commit-tree", et, "-m", verified.FormatVerifiers(vs))+":"+verified.VerifiersRef)
	t.Setenv("HC_LANE_SPEC_BUSY_MAX", "1")
	d := twoShards(t, s)
	s.busyShard(d[0])
	s.round(1)
	l := s.launches()
	if len(l) != 3 || l[2] != [3]string{"v3", d[0][1], "1/2"} {
		t.Fatalf("④ 첫 바쁨은 v3로: %v", l)
	}
	s.busyShard(l[2])
	s.round(1)
	if l = s.launches(); len(l) != 3 {
		t.Fatalf("④ 상한을 넘으면 빈 v4가 있어도 다시 맡기지 않는다: %v", l)
	}
	s.runOn(d[1])
	ran(t, hl)
	s.round(1)
	l = s.launches()
	if len(l) != 4 || l[3][2] != "" {
		t.Fatalf("④ 몫 2가 끝나면 마무리: %v", l)
	}
	s.runOn(l[3])
	if got := ran(t, hl); got != "run p1@"+l[3][0]+"\nrun p3@"+l[3][0] {
		t.Fatalf("④ 마무리가 몫 1의 조각을: %q", got)
	}
}

// 나눌 기계를 띄우기 직전에 한 번 더 잰다 — 그새 다른 병합기가 잡은 기계(검증이 돎)는 빼고, 모두 잡혔으면 그대로(일꾼의 바쁨 → 다시 맡김)
func TestShardRecheckSkipsGrabbed(t *testing.T) {
	s, _, _ := shardFx(t, "3")
	m := s.merger()
	vs := []verified.Verifier{{Machine: "v1"}, {Machine: "v2"}, {Machine: "v3"}}
	s.idle("v2", verifyBusy)
	if got := m.shardRecheck(vs); len(got) != 2 || got[0].Machine != "v1" || got[1].Machine != "v3" {
		t.Fatalf("잡힌 v2를 뺀다: %v", got)
	}
	s.idle("v1", verifyBusy)
	s.idle("v3", loadBusy)
	if got := m.shardRecheck(vs); len(got) != 3 {
		t.Fatalf("모두 잡혔으면 그대로: %v", got)
	}
}
