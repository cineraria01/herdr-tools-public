//go:build !windows

package main

// 착지 실행을 워커에(ht-merger-land-offload): 틀은 specFx(검증 기계 v1·v2 클론·가짜 hc — exec의 sh 명령은 이 기계에서 그 폴더로 돈다).
// 가짜 hc-lane(PATH)은 merger --land-run을 이 시험 바이너리의 landRunMain으로(그 기계 클론에서), 나머지는 가짜 착지(gSelf)로 넘긴다.
// 맨 앞은 손잡이 front_dispatch 0으로 바로 직접 검증 바퀴가 되게 한다(추측을 기다리지 않음).

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// landFx: 워커 착지를 켠 specFx. runs = 가짜 hc-lane이 받은 --land-run의 「<클론 폴더 이름>」 줄.
//   - 가짜 hc run --name land-…: 그 --cwd에서 명령을 끝까지 돌리고 돌아온다(진짜는 --bg로 곧 돌아오고 병합기가 결과 파일을 읽는다 —
//     병합기 쪽 길은 같다). --cwd가 없으면 hc 자신의 실패. get <기계>/land-…은 fakeHcIdle(없으면 not_found = 끝남)
//   - LANDRUN_OLD=<폴더 이름>: 그 기계 hc-lane은 --land-run을 모르는 옛 판(「알 수 없는 인자」 종료 1, 결과 파일 없음)
//   - LAND_STEAL=1: 워커 착지가 push 직전에 차례 참조가 지워진다(병합기가 그 실행을 끊긴 것으로 보고 치운 것과 같다)
//   - LAND_VANISH=<폴더 이름>: 그 기계의 land 터미널이 결과 없이 사라진다(밖에서 SIGTERM — ht-land-offload-lost). LAND_VANISH_AFTER=1이면
//     착지(push)까지 한 뒤 결과 파일 없이 사라진다
func landFx(t *testing.T) (*sfx, func() []string) {
	s := specFx(t)
	t.Setenv("HC_LANE_MERGER_LAND_ON", "auto")
	t.Setenv("HC_LANE_FRONT_DISPATCH", "0")
	old := landPoll
	landPoll = 10 * time.Millisecond
	t.Cleanup(func() { landPoll = old })
	self := s.dir + "/hc-lane-land"
	// LAND_ENV_OUT: 착지가 본 LANDON_SHELL(시험 ~/.zshrc가 넣는 값)을 그 파일에(landon_env_test.go)
	os.WriteFile(self, []byte(fmt.Sprintf("#!/bin/sh\n[ \"$1\" = land ] && [ -n \"$LAND_ENV_OUT\" ] && echo \"${LANDON_SHELL-}\" >\"$LAND_ENV_OUT\"\n"+
		"[ \"$1\" = land ] && [ -n \"$LAND_STEAL\" ] && git push -q origin :%smain\nexec %q \"$@\"\n", landClaims, gSelf)), 0o777)
	t.Setenv("HC_LANE_SELF", self)
	runs := s.dir + "/landrun.log"
	w := fmt.Sprintf(`#!/bin/sh
if [ "$1" = merger ] && [ "$2" = --land-run ]; then
  shift 2
  b=$(basename "$(pwd)")
  echo "$b" >>%q
  if [ "$LANDRUN_OLD" = "$b" ]; then echo "hc-lane merger: 알 수 없는 인자 --land-run" >&2; exit 1; fi
  HC_MERGER_TEST_HELPER=landrun HC_MERGER_TEST_ARGS="$*" exec %q
fi
exec %q "$@"
`, runs, os.Args[0], gSelf)
	if err := os.WriteFile(s.dir+"/bin/hc-lane", []byte(w), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(s.dir+"/bin/hc", s.dir+"/bin/hc.spec"); err != nil {
		t.Fatal(err)
	}
	hc := fmt.Sprintf(`#!/bin/sh
if [ "$1" = run ]; then
  case " $* " in *" --name land-"*)
    shift
    cwd=
    while [ $# -gt 0 ]; do case "$1" in --cwd|--on|--name|--label) [ "$1" = --cwd ] && cwd=$2; shift 2;; --*) shift;; *) break;; esac; done
    cd "$cwd" 2>/dev/null || { echo '{"error":{"kind":"spawn_failed","message":"no such cwd"}}' >&2; exit 1; }
    if [ "$LAND_VANISH" = "$(basename "$cwd")" ]; then
      [ -n "$LAND_VANISH_AFTER" ] && "$@" >/dev/null 2>&1
      rm -f "$cwd".land-wt/*.rc
    else
      "$@" >/dev/null 2>&1
    fi
    echo '{"address":"x/t1"}'; exit 0 ;;
  esac
fi
exec %q "$@"
`, s.dir+"/bin/hc.spec")
	if err := os.WriteFile(s.dir+"/bin/hc", []byte(hc), 0o777); err != nil {
		t.Fatal(err)
	}
	return s, func() []string { return s.lines(runs) }
}

// landOn: 일지 「착지 실행」 줄의 까닭 칸(구성원 꼬리 뺌), 차례대로 — 한 결정은 구성원마다 같은 줄이라 앞 것만
func landOnRows(t *testing.T) []string {
	var o []string
	seen := map[string]bool{}
	for _, r := range jrows(t, jLandOn) {
		w, _, _ := strings.Cut(r[11], " — 구성원 ")
		if !seen[r[7]+w] {
			seen[r[7]+w] = true
			o = append(o, w)
		}
	}
	return o
}

// ① 직접 검증 맨 앞(착지 스크립트 저장소 포함) → 한가한 워커 클론에서 착지, 병합기는 결과만 — 차례 참조는 남지 않는다
func TestLandOnWorker(t *testing.T) {
	s, runs := landFx(t)
	os.WriteFile(s.seed+"/.claude/herdr-land.sh", []byte("#!/bin/sh\nexit 0\n"), 0o777) // 착지 스크립트 저장소(가짜 착지는 안 부른다)
	s.git(s.seed, "add", "-A")
	s.git(s.seed, "commit", "-qm", "land script")
	s.git(s.seed, "push", "-q", "origin", "main")
	s.sub(1000000001, "a", false)
	s.round(1)
	if r := runs(); len(r) != 1 || r[0] != "v1" || !s.onMain("a.txt") || strings.Join(s.lines(s.landlog), " ") != "verify|a" {
		t.Fatalf("① v1에서 착지: 실행 %v 착지 %v\n%s", r, s.lines(s.landlog), jdump())
	}
	if l := landOnRows(t); len(l) != 1 || !strings.HasPrefix(l[0], "v1(검증 자리 남음 · 코어당 부하 0.00") {
		t.Fatalf("① 일지 착지 실행: %q", l)
	}
	if r := s.refs(landClaims); len(r) != 0 || s.has("refs/merge-queue/1000000001-a") {
		t.Fatalf("① 차례·큐가 남음: %v", r)
	}
	if j := jrows(t, "착지"); len(j) != 1 {
		t.Fatalf("① 착지 줄 하나:\n%s", jdump())
	}
}

// ② 모든 워커가 검증 중(문턱 넘음) → 병합기 기계가 아니라 코어당 부하가 가장 낮은 워커
func TestLandOnLeastBusyWorker(t *testing.T) {
	s, runs := landFx(t)
	s.idle("v1", "load=12.00 cpus=8 verifies=1")
	s.idle("v2", "load=4.00 cpus=8 verifies=2")
	s.sub(1000000001, "a", false)
	s.round(1)
	if r := runs(); len(r) != 1 || r[0] != "v2" || !s.onMain("a.txt") {
		t.Fatalf("② v2에서: %v", r)
	}
	if l := landOnRows(t); len(l) != 1 || !strings.HasPrefix(l[0], "v2(모두 검증 중 — 가장 덜 바쁨 · 코어당 부하 0.50") {
		t.Fatalf("② 까닭: %q", l)
	}
}

// ③ 손잡이 local → 지금과 같음(병합기 기계, 워커 실행 없음) · ④ 기계 목록 → 그 안에서만
func TestLandOnKnobLocalAndList(t *testing.T) {
	s, runs := landFx(t)
	t.Setenv("HC_LANE_MERGER_LAND_ON", "local")
	s.sub(1000000001, "a", false)
	s.round(1)
	if len(runs()) != 0 || !s.onMain("a.txt") {
		t.Fatalf("③ local: 워커 실행 %v", runs())
	}
	t.Setenv("HC_LANE_MERGER_LAND_ON", "v2")
	s.sub(1000000002, "b", false)
	s.round(1)
	if r := runs(); len(r) != 1 || r[0] != "v2" || !s.onMain("b.txt") {
		t.Fatalf("④ 목록 v2 안에서만(v1이 먼저·한가해도): %v", r)
	}
	if l := landOnRows(t); len(l) != 2 || l[0] != "box(병합기 기계 · 손잡이 local)" || !strings.HasPrefix(l[1], "v2(") {
		t.Fatalf("③④ 일지: %q", l)
	}
}

// ⑤ 워커 착지 실패 → 지금과 같은 반송(혼자) · 묶음 줄이기(묶음)
func TestLandOnWorkerFailRejectsAndShrinks(t *testing.T) {
	s, runs := landFx(t)
	s.sub(1000000001, "a", false)
	s.sub(1000000002, "b", true)
	s.round(2)
	if len(runs()) != 1 || s.onMain("a.txt") || jrows(t, jBatchFail) == nil {
		t.Fatalf("⑤ 묶음 실패 → 줄이기: 실행 %v\n%s", runs(), jdump())
	}
	if b, _ := os.ReadFile(s.clone + "/.git/hc-merger/cap"); strings.TrimSpace(string(b)) != "1" {
		t.Fatalf("⑤ 묶음 크기 1로: %q", b)
	}
	s.round(2) // a 혼자 착지(그동안 b 추측을 v1에 띄움)
	for _, d := range s.dispatched() {
		s.verifyOn(d) // b 추측 실패 — 맨 앞 b는 직접 검증 바퀴로
	}
	s.round(2) // b 혼자 실패 → 반송
	if !s.onMain("a.txt") || s.onMain("bad.txt") || !s.has("refs/merge-failed/1000000002-b") || len(runs()) != 3 {
		t.Fatalf("⑤ a 착지·b 반송: 실행 %v\n%s", runs(), jdump())
	}
	if r := s.refs(landClaims); len(r) != 0 {
		t.Fatalf("⑤ 실패한 실행의 차례가 남음: %v", r)
	}
}

// ⑥ 맡긴 워커가 착지를 시작 못 함(그 기계 hc-lane이 --land-run을 모르는 옛 판 — 종료 1) → 반송하지 않고 차례를 치우고 일지로 판정,
// 다음 바퀴에 다른 워커로(감사 atp2f36 중간 1)
func TestLandOnWorkerDiesRetriesElsewhere(t *testing.T) {
	s, runs := landFx(t)
	t.Setenv("LANDRUN_OLD", "v1")
	s.sub(1000000001, "a", false)
	s.round(1)
	if r := runs(); len(r) != 1 || r[0] != "v1" || s.onMain("a.txt") || !s.has("refs/merge-queue/1000000001-a") || len(s.refs(landClaims)) != 0 {
		t.Fatalf("⑥ v1 죽음 — 큐 그대로·차례 치움: %v 차례 %v", r, s.refs(landClaims))
	}
	s.round(1)
	if r := runs(); len(r) != 2 || r[1] != "v2" || !s.onMain("a.txt") {
		t.Fatalf("⑥ 다음 바퀴 v2: %v\n%s", r, jdump())
	}
	if s.has("refs/merge-failed/1000000001-a") {
		t.Fatal("⑥ 시작 못 함은 반송이 아니다")
	}
	if l := landOnRows(t); len(l) != 3 || !strings.HasPrefix(l[1], "v1 착지 실행 끊김(워커가 착지를 시작 못 함(종료 1))") {
		t.Fatalf("⑥ 일지: %q", l)
	}
	if !strings.Contains(landOnSum(strings.Split(jdump(), "\n"), 20), "착지 실행 워커 2번(v1 1 · v2 1) · 병합기 기계 0번 · 끊김 1번 · 마지막 v2(") {
		t.Fatalf("⑥ --why: %q", landOnSum(strings.Split(jdump(), "\n"), 20))
	}
}

// ⑦ 착지 차례를 다른 기계(v2)의 살아 있는 실행이 쥐고 있음 → 착지하지 않고 기다림(두 기계가 함께 넣지 않음), 그 실행이 끝나면 치우고 착지
func TestLandOnClaimHeldWaits(t *testing.T) {
	s, runs := landFx(t)
	s.sub(1000000001, "a", false)
	et := s.git(s.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
	cm := s.git(s.seed, "commit-tree", et, "-m", fmt.Sprintf("hc-merger-land machine=v2 at=%d lane=x path=%s", time.Now().Unix(), s.dir+"/v2"))
	s.git(s.seed, "push", "-q", "origin", cm+":"+landClaims+"main")
	term := s.dir + "/idle/get-land-" + cm[:8] // v2의 land-<차례 8> 터미널이 살아 있음
	os.WriteFile(term, []byte(`{"address":"v2/t9","state":"live","cwd":"/x","name":"land-`+cm[:8]+`","status":"working","pid":1,"created_ms":1}`+"\n"), 0o666)
	m0 := s.mainSHA()
	s.round(1)
	s.round(1)
	if s.mainSHA() != m0 || len(runs()) != 0 || !s.has(landClaims+"main") || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("⑦ 기다림: 실행 %v 착지 %v", runs(), s.lines(s.landlog))
	}
	os.Remove(term) // 그 실행이 끝남(--rm — 터미널 없음)
	s.round(1)
	if !s.onMain("a.txt") || len(s.refs(landClaims)) != 0 || len(runs()) != 1 {
		t.Fatalf("⑦ 끝난 차례를 치우고 착지: 실행 %v 차례 %v", runs(), s.refs(landClaims))
	}
}

// ⑧ 닿는 워커가 없음 → 병합기 기계에서 하고 운영 사건 한 번
func TestLandOnNoWorkerLocalWithEvent(t *testing.T) {
	s, runs := landFx(t)
	s.idle("v1", "unreach")
	s.idle("v2", "unreach")
	s.sub(1000000001, "a", false)
	s.round(1)
	if len(runs()) != 0 || !s.onMain("a.txt") {
		t.Fatalf("⑧ 병합기 기계에서: %v", runs())
	}
	n := 0
	for _, e := range s.evs() {
		if strings.Contains(e, "착지를 워커에 맡기지 못해") && strings.Contains(e, "hc-merger-land-local:") {
			n++
		}
	}
	if l := landOnRows(t); n != 1 || len(l) != 1 || !strings.HasPrefix(l[0], "box(병합기 기계 · 닿는 워커 없음(") {
		t.Fatalf("⑧ 사건 %d 일지 %q 사건들 %v", n, l, s.evs())
	}
}

// ⑨ 검증 기계 목록의 클론 경로가 없음(옮김) → hc run이 못 띄움 — 반송하지 않고 다른 워커로(감사 atp2f36 중간 1)
func TestLandOnWorkerPathMissing(t *testing.T) {
	s, runs := landFx(t)
	if err := os.Rename(s.dir+"/v1", s.dir+"/v1-moved"); err != nil {
		t.Fatal(err)
	}
	s.sub(1000000001, "a", false)
	s.round(1)
	if s.onMain("a.txt") || s.has("refs/merge-failed/1000000001-a") || len(s.refs(landClaims)) != 0 {
		t.Fatalf("⑨ 못 띄움 — 큐 그대로·차례 치움: 실행 %v", runs())
	}
	s.round(1)
	if r := runs(); len(r) != 1 || r[0] != "v2" || !s.onMain("a.txt") {
		t.Fatalf("⑨ v2에서: %v\n%s", r, jdump())
	}
}

// ⑩ 차례가 지워진(병합기가 그 실행을 치운) 뒤 늦게 push하는 워커 착지는 lease로 거절 — 기본 브랜치에 넣지 못한다(감사 atp2f36 낮음 2)
func TestLandOnStolenClaimCannotPush(t *testing.T) {
	s, runs := landFx(t)
	t.Setenv("LAND_STEAL", "1")
	s.sub(1000000001, "a", false)
	m0 := s.mainSHA()
	s.round(1)
	if len(runs()) != 1 || s.mainSHA() != m0 || s.onMain("a.txt") || s.has(landClaims+"main") {
		t.Fatalf("⑩ 차례 없는 push는 거절: 실행 %v main 바뀜 %v", runs(), s.mainSHA() != m0)
	}
}

// ⑪ 맡긴 착지 터미널이 결과 없이 사라짐(2026-11-26 05:02 — 밖에서 SIGTERM, 병합기는 표준 오류의 not_found를 못 읽어 15분 넘게 기다렸다):
// 곧 끊김으로 보고 차례를 치운 뒤 일지로 판정 — 착지 안 됨이면 다음 바퀴 다른 워커로, 착지됨이면 완료(다시 돌리지 않음)
func TestLandOnWorkerVanishes(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(fmt.Sprint("착지 뒤 ", after), func(t *testing.T) {
			s, runs := landFx(t)
			t.Setenv("LAND_VANISH", "v1")
			if after {
				t.Setenv("LAND_VANISH_AFTER", "1")
			}
			s.sub(1000000001, "a", false)
			t0 := time.Now()
			s.round(1)
			if d := time.Since(t0); d > 30*time.Second {
				t.Fatalf("⑪ 사라진 터미널을 %v 기다림", d)
			}
			if len(s.refs(landClaims)) != 0 || s.has("refs/merge-failed/1000000001-a") {
				t.Fatalf("⑪ 차례 치움·반송 아님: 차례 %v\n%s", s.refs(landClaims), jdump())
			}
			if l := landOnRows(t); len(l) < 2 || !strings.HasPrefix(l[1], "v1 착지 실행 끊김(결과 없이 끝남)") {
				t.Fatalf("⑪ 일지: %q", l)
			}
			if after {
				if !s.onMain("a.txt") || s.has("refs/merge-queue/1000000001-a") || len(jrows(t, "착지")) != 1 {
					t.Fatalf("⑪ 착지됨 → 완료: 실행 %v\n%s", runs(), jdump())
				}
				s.round(1)
				if len(runs()) != 1 {
					t.Fatalf("⑪ 착지된 것을 다시 돌림: %v", runs())
				}
				return
			}
			if s.onMain("a.txt") || !s.has("refs/merge-queue/1000000001-a") {
				t.Fatalf("⑪ 착지 안 됨 → 큐 그대로: %v", runs())
			}
			s.round(1)
			if r := runs(); len(r) != 1 || r[0] != "v2" || !s.onMain("a.txt") { // 사라진 v1은 --land-run을 돌리지 않았다
				t.Fatalf("⑪ 다음 바퀴 v2: %v\n%s", r, jdump())
			}
		})
	}
}

// landParse: 결과 파일 꼴
func TestLandParse(t *testing.T) {
	for in, want := range map[string]string{"0": "0|", "1": "1|", "nostart 127": "0|워커가 착지를 시작 못 함(종료 127)", "x": "0|결과 꼴 모름: x"} {
		rc, lost := landParse(in)
		if got := fmt.Sprintf("%d|%s", rc, lost); got != want {
			t.Errorf("%q → %q, 기대 %q", in, got, want)
		}
	}
}

// landBest: 자리 남은 기계 먼저, 그 안에서 코어당 부하(코어 수가 달라도 견준다)
func TestLandBest(t *testing.T) {
	c := func(m string, load float64, cpus, v int, ok bool) *landCand {
		x := &landCand{ok: ok}
		x.v.Machine, x.i = m, idleInfo{load: load, cpus: cpus, verifies: v}
		return x
	}
	if b := landBest([]*landCand{c("a", 1, 4, 1, true), c("b", 9, 16, 0, true), c("c", 2, 16, 0, true), c("d", 0, 8, 0, false)}); b == nil || b.v.Machine != "c" {
		t.Fatalf("자리 남은 c(0.125)가 먼저: %+v", b)
	}
	if b := landBest([]*landCand{c("a", 8, 4, 1, true), c("b", 8, 16, 2, true)}); b == nil || b.v.Machine != "b" {
		t.Fatalf("모두 검증 중이면 코어당 부하: %+v", b)
	}
	if landBest([]*landCand{c("a", 0, 4, 0, false)}) != nil {
		t.Fatal("닿은 것 없으면 nil")
	}
}
