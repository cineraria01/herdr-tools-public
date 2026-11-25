//go:build !windows

package main

// 맡길 검증 기계 고르기(merger-verifier-pick): 맡기기 전에 후보 기계마다 싼 확인 한 번(hc exec --on <기계> … hc-lane merger --spec-idle —
// 도는 검증 수·부하·코어 수)으로 실제로 한가한 기계에만 맡긴다. 확인을 못 한 기계는 모름(고르지 않음), 한가한 기계가 없으면 기다리는 줄에
// 넣지 않고 곧바로 손잡이 front_local. 맡은 기계의 일꾼이 그새 바빠 검증 자리를 못 얻으면 「바쁨」(spec 참조 state=busy)을 남기고, 병합기는
// 맨 앞 기다림 상한을 기다리지 않고 다음 바퀴에 그 기계를 뺀 한가한 기계로 다시 고른다. 「바쁨」은 반송·범인 판정에 안 쓴다.
// 틀은 specrun_test.go(specFx — 가짜 hc의 exec는 $IDLEDIR/<기계> 글을 그 기계의 확인 결과로 돌려준다: 없으면 한가함, unreach면 hc 실패).

import (
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/verified"
)

// fakeHcIdle: 가짜 hc의 exec(한가함 확인) — --on 기계의 $IDLEDIR/<기계> 글을 표준 출력으로(없으면 한가함), unreach면 hc 자신의 실패
//
// get <기계>/spec-<아이디8>은 $IDLEDIR/get-spec-<아이디8>이 있으면 그 글(살아 있는 터미널), 없으면 not_found(ht-spec-overrun). exec … -- sh …은
// 이 기계에서 그대로 돌려 그 출력을 돌려준다(추측 진행 확인 — 검증 기계 클론이 이 기계 임시 폴더에 있다)
const fakeHcIdle = `if [ "$1" = get ]; then
  case ${2##*/} in spec-*)
    if [ -f "$IDLEDIR/get-${2##*/}" ]; then cat "$IDLEDIR/get-${2##*/}"; exit 0; fi
    echo '{"error":{"kind":"not_found","message":"no terminal"}}'; exit 3 ;;
  esac
fi
if [ "$1" = exec ]; then
  m=
  while [ $# -gt 0 ]; do [ "$1" = --on ] && m=$2; [ "$1" = -- ] && break; shift; done
  [ $# -gt 0 ] && shift
  if [ "$1" = sh ]; then
    o=$("$@"); r=$?
    printf '{"code":%d,"signal":null,"stderr":"","stdout":"%s","timed_out":false,"truncated":false}\n' "$r" "$(printf '%s\n' "$o" | base64 | tr -d '\n')"
    exit 0
  fi
  f="$IDLEDIR/$m"
  o="load=0.00 cpus=8 verifies=0"
  [ -f "$f" ] && o=$(cat "$f")
  if [ "$o" = unreach ]; then echo '{"error":{"kind":"unreachable","message":"machine not reachable"}}' >&2; exit 1; fi
  printf '{"code":0,"signal":null,"stderr":"","stdout":"%s","timed_out":false,"truncated":false}\n' "$(printf '%s\n' "$o" | base64 | tr -d '\n')"
  exit 0
fi
`

// idle: 그 기계의 확인 결과(가짜 hc exec가 돌려줄 표준 출력)
func (s *sfx) idle(m, out string) {
	s.t.Helper()
	if err := os.WriteFile(s.dir+"/idle/"+m, []byte(out+"\n"), 0o666); err != nil {
		s.t.Fatal(err)
	}
}

const (
	idleOut    = "load=0.50 cpus=8 verifies=0"
	verifyBusy = "load=0.50 cpus=8 verifies=1" // 다른 검증이 도는 중(검증 잠금·자리를 쥠)
	loadBusy   = "load=7.00 cpus=8 verifies=0" // 부하가 코어 수에 견줘 높음
)

// busyAs: 일꾼이 「바쁨」을 남긴 꼴 — spec 참조를 그 기계·state=busy로 고쳐 쓴다(트리·부모·본문은 그대로)
func (s *sfx) busyAs(id, mach string) {
	s.t.Helper()
	og := "--git-dir=" + s.origin
	ref := specRefs + id
	cm := s.git(s.dir, og, "rev-parse", ref)
	r, ok := parseSpec(s.git(s.dir, og, "log", "-1", "--format=%s", cm))
	if !ok {
		s.t.Fatalf("spec 참조 제목을 못 읽음: %s", cm)
	}
	nc := s.git(s.dir, og, "commit-tree", cm+"^{tree}", "-p", cm+"^", "-m", specMsg(r.parent, r.group, mach, r.at, "busy"),
		"-m", "waited "+mach+" verifies=1 load=0.50/8 0s")
	s.git(s.dir, og, "update-ref", ref, nc, cm)
}

// ① 목록 첫 기계는 검증 잠금이 잡힘(다른 검증이 도는 중) · 둘째는 한가함 → 둘째에 맡긴다(고치기 전: 첫 기계에 맡김)
func TestPickSkipsVerifyBusyMachine(t *testing.T) {
	s := specFx(t)
	s.idle("v1", verifyBusy)
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || d[0][0] != "v2" || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("① 한가한 v2에 맡겨야: 띄움 %v 착지 %v", d, s.lines(s.landlog))
	}
	if rc := s.verifyOn(d[0]); rc != 0 {
		t.Fatalf("일꾼 종료 %d", rc)
	}
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|a" {
		t.Fatalf("① v2 기록으로 착지: %q", got)
	}
	// ⑦ 일지: 맡긴 기계와 걸린 초
	if w := jrows(t, "추측 기다림"); len(w) != 1 || !strings.Contains(w[0][11], "검증 맡김(v2) ") || !strings.Contains(w[0][11], "초") {
		t.Fatalf("⑦ 일지 「검증 맡김(v2) <초>」:\n%s", jdump())
	}
}

// ① 부하가 높은 기계도 바쁨 — 목록 차례는 한가한 기계끼리의 차례
func TestPickSkipsLoadBusyMachine(t *testing.T) {
	s := specFx(t)
	s.idle("v1", loadBusy)
	s.sub(1000000001, "a", false)
	s.round(1)
	if d := s.dispatched(); len(d) != 1 || d[0][0] != "v2" {
		t.Fatalf("① 부하 높은 v1을 건너뛰고 v2에: %v", d)
	}
}

// ② 모두 바쁨 → 맡기지 않고 곧바로 front_local 길: 1(기본)이면 병합기 기계 직접 검증 한 번, 일지 까닭 「맡길 한가한 기계 없음」
func TestPickAllBusyVerifiesLocally(t *testing.T) {
	s := specFx(t)
	s.idle("v1", verifyBusy)
	s.idle("v2", loadBusy)
	s.sub(1000000001, "a", false)
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a" || len(s.dispatched()) != 0 || !s.onMain("a.txt") {
		t.Fatalf("② 맡기지 않고 직접 검증 한 번: 착지 %q 띄움 %v", got, s.dispatched())
	}
	if l := jrows(t, "착지"); len(l) != 1 || !strings.Contains(l[0][11], "직접 검증(맡길 한가한 기계 없음)") {
		t.Fatalf("⑦ 착지 줄 까닭 「맡길 한가한 기계 없음」:\n%s", jdump())
	}
}

// ② 모두 바쁨 + front_local 0 → 기다리며 막힘 사건 한 번(띄우지 않음)
func TestPickAllBusyNoLocalWaits(t *testing.T) {
	s := specFx(t)
	t.Setenv("HC_LANE_FRONT_LOCAL", "0")
	s.idle("v1", verifyBusy)
	s.idle("v2", verifyBusy)
	s.sub(1000000001, "a", false)
	s.round(1)
	s.round(1)
	if len(s.lines(s.landlog)) != 0 || len(s.dispatched()) != 0 || len(s.evs()) != 1 || !strings.Contains(s.evs()[0], "막힘") {
		t.Fatalf("② 손잡이 0: 착지 %v 띄움 %v 사건 %v", s.lines(s.landlog), s.dispatched(), s.evs())
	}
}

// ③⑦ 맡긴 뒤 일꾼이 검증 자리를 못 얻어 「바쁨」 → 다음 바퀴에 바로(기다림 상한 없이) 다른 한가한 기계로, 일지에 「바쁨으로 다시 고름」
func TestBusyResultRepicksNow(t *testing.T) {
	s := specFx(t)
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || d[0][0] != "v1" {
		t.Fatalf("맨 앞 v1에: %v", d)
	}
	s.busyAs(d[0][1], "v1")
	s.round(1)
	d = s.dispatched()
	if len(d) != 2 || d[1][0] != "v2" || d[1][1] != d[0][1] || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("③ 같은 추측을 v2로 다시(착지 없음): 띄움 %v 착지 %v", d, s.lines(s.landlog))
	}
	if rc := s.verifyOn(d[1]); rc != 0 {
		t.Fatalf("일꾼 종료 %d", rc)
	}
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|a" {
		t.Fatalf("③ v2 기록으로 착지: %q", got)
	}
	if w := jrows(t, "추측 기다림"); len(w) != 1 || !strings.Contains(w[0][11], "바쁨으로 다시 고름(v1→v2)") {
		t.Fatalf("⑦ 일지 「바쁨으로 다시 고름(v1→v2)」:\n%s", jdump())
	}
	// ⑦ hc-lane merger --why 요약에도
	if got := whySummary(strings.Split(jdump(), "\n"), 20); !strings.Contains(got, " · 검증 맡김 1번(바쁨으로 다시 고름 1)") {
		t.Fatalf("⑦ --why 요약: %s", got)
	}
}

// ③④ 같은 제출에 바쁨을 낸 기계는 다시 고르지 않는다 — 남은 한가한 기계가 없으면 front_local(직접 검증). 「바쁨」은 반송 근거가 아니다
func TestBusyTwiceGoesLocalNoReject(t *testing.T) {
	s := specFx(t)
	s.sub(1000000001, "a", false)
	s.round(1)
	id := s.dispatched()[0][1]
	s.busyAs(id, "v1")
	s.round(1)
	s.busyAs(id, "v2")
	s.round(1)
	if d := s.dispatched(); len(d) != 2 {
		t.Fatalf("③ 바쁨을 낸 기계에 다시 띄우지 않는다: %v", d)
	}
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a" || s.has("refs/merge-failed/1000000001-a") || len(jrows(t, "돌려보냄")) != 0 {
		t.Fatalf("④ 바쁨은 반송이 아니다 — 직접 검증해 착지: %q\n%s", got, jdump())
	}
}

// ⑤ 확인 못 한 기계(안 닿음·꼴이 다름)는 모름 — 고르지 않는다
func TestPickUnknownNotChosen(t *testing.T) {
	for _, out := range []string{"unreach", "garbage"} {
		t.Run(out, func(t *testing.T) {
			s := specFx(t)
			s.idle("v1", out)
			s.sub(1000000001, "a", false)
			s.round(1)
			if d := s.dispatched(); len(d) != 1 || d[0][0] != "v2" {
				t.Fatalf("⑤ 모름인 v1은 고르지 않는다: %v", d)
			}
		})
	}
}

// ③ 실제 일꾼: 맡은 기계가 그새 바빠 검증 자리를 busy_wait 안에 못 얻음 → 검증하지 않고 spec 참조를 state=busy(그 기계·기다린 것 한 줄)로,
// 통과·실패 기록은 없음 → 병합기는 다음 바퀴에 바로 다른 한가한 기계로
func TestWorkerBusyLeavesBusy(t *testing.T) {
	s := specFx(t)
	t.Setenv("HC_LANE_SPEC_BUSY_WAIT", "0")
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || d[0][0] != "v1" || !strings.Contains(strings.Join(s.lines(s.hclog), " "), "env HC_LANE_SPEC_BUSY_WAIT=0 HC_LANE_SPEC_IDLE_LOAD=50 hc-lane") {
		t.Fatalf("v1에 띄우며 손잡이 busy_wait를 넘긴다: %v %v", d, s.lines(s.hclog))
	}
	idleProbe = func() (idleInfo, bool) { return idleInfo{load: 0.5, cpus: 8, verifies: 1}, true }
	if rc := s.verifyOn(d[0]); rc != 0 {
		t.Fatalf("바쁨 일꾼 종료 %d", rc)
	}
	og := "--git-dir=" + s.origin
	msg := s.git(s.dir, og, "log", "-1", "--format=%B", specRefs+d[0][1])
	if !strings.Contains(msg, "machine=v1") || !strings.Contains(msg, "state=busy") || !strings.Contains(msg, "waited v1 load=0.50 cpus=8 verifies=1") {
		t.Fatalf("spec 참조 state=busy·기다린 것 한 줄:\n%s", msg)
	}
	if len(s.refs(verified.Prefix)) != 0 || len(s.refs(specFails)) != 0 {
		t.Fatalf("바쁨은 통과도 실패도 아니다: %v %v", s.refs(verified.Prefix), s.refs(specFails))
	}
	s.round(1)
	if d := s.dispatched(); len(d) != 2 || d[1][0] != "v2" || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("③ 다음 바퀴에 v2로 다시: %v 착지 %v", d, s.lines(s.landlog))
	}
}

// 손잡이 idle_load(병합기 일지 폴더 파일)는 실행 중에 바로 먹는다 — 부하 7/8도 한가함으로 읽게 올리면 v1에
func TestPickIdleLoadKnob(t *testing.T) {
	s := specFx(t)
	s.idle("v1", loadBusy)
	if err := os.WriteFile(s.merger().mj+"/idle_load", []byte("95\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	s.sub(1000000001, "a", false)
	s.round(1)
	if d := s.dispatched(); len(d) != 1 || d[0][0] != "v1" {
		t.Fatalf("idle_load 95면 부하 7/8은 한가함: %v", d)
	}
}

func TestParseIdle(t *testing.T) {
	for in, ok := range map[string]bool{idleOut: true, "load=1 cpus=0 verifies=0": false, "load=x cpus=8 verifies=0": false,
		"load=1 cpus=8": false, "load=1 cpus=8 verifies=0 more=1": false, "": false, "cpus=8 load=0.1 verifies=2": true} {
		if _, got := parseIdle(in); got != ok {
			t.Errorf("parseIdle(%q) = %v", in, got)
		}
	}
	if i, _ := parseIdle(idleOut); !i.idle(50) || (idleInfo{load: 4, cpus: 8}).idle(50) || (idleInfo{cpus: 8, verifies: 1}).idle(50) {
		t.Error("한가함 = 검증 0 · 부하 < 코어×%")
	}
}

// ⑥ 미리 검증은 한가한 기계가 없으면 띄우지 않고 다음 바퀴에 다시 본다
func TestPreSpecNeedsIdle(t *testing.T) {
	s := preFx(t)
	s.idle("v1", verifyBusy)
	s.idle("v2", loadBusy)
	s.pre("p", time.Now().Unix(), false)
	s.round(1)
	if d := s.dispatched(); len(d) != 0 {
		t.Fatalf("⑥ 한가한 기계가 없으면 미리 검증을 띄우지 않는다: %v", d)
	}
	s.idle("v2", idleOut)
	s.round(1)
	if d := s.dispatched(); len(d) != 1 || d[0][0] != "v2" {
		t.Fatalf("⑥ 한가해지면 다음 바퀴에 띄운다: %v", d)
	}
}
