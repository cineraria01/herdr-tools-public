//go:build !windows

package main

// ht-spec-overrun(2026-11-25 — 맨 앞 추측이 기다림 상한에 걸리면 살아서 도는 중이어도 병합기가 맥에서 처음부터 전체 직접 검증을 새로
// 시작했다, 23~34분): ① 상한을 넘겨도 진행 중(spec-<아이디 8> 터미널 살아 있음 + 로그·작업 트리 갱신 — hc-fill advise_spec_fresh와 같은
// 판정)이면 다른 기계로 옮기거나(workerRedo, ht-merger-spec-reuse) 직접 검증하지 않고 merger_spec_timeout까지 더 기다린다. 멈췄으면
// 지금 길 그대로 — 한가한 워커에 한 번 옮기고, 워커가 없으면 직접 검증 ② 「직접 검증(모름)」을 까닭별로 가른다.
// 틀은 specfrontwait_test.go(frontFx)·verifierpick_test.go(가짜 hc get·exec).

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// specTerm: 그 기계에 살아 있는 spec-<아이디 8> 터미널과, 마지막으로 age 전에 고친 로그 <클론>.spec-wt/<아이디 8>.log
func (s *sfx) specTerm(mach, id string, age time.Duration) {
	s.t.Helper()
	n := "spec-" + id[:8]
	js := fmt.Sprintf(`{"address":"%s/t9","state":"live","cwd":"%s","name":"%s","status":"working","pid":1,"created_ms":1}`, mach, s.dir+"/"+mach, n)
	if err := os.WriteFile(s.dir+"/idle/get-"+n, []byte(js+"\n"), 0o666); err != nil {
		s.t.Fatal(err)
	}
	lg := s.dir + "/" + mach + ".spec-wt/" + id[:8] + ".log"
	os.MkdirAll(filepath.Dir(lg), 0o777)
	os.WriteFile(lg, []byte("go test ...\n"), 0o666)
	at := time.Now().Add(-age)
	if err := os.Chtimes(lg, at, at); err != nil {
		s.t.Fatal(err)
	}
}

func jcount(kind, sub string) int {
	b, _ := os.ReadFile(gEvdir + "/merger-journal.tsv")
	n := 0
	for _, l := range strings.Split(string(b), "\n") {
		if c := strings.Split(l, "\t"); len(c) == 12 && c[9] == kind && strings.Contains(c[11], sub) {
			n++
		}
	}
	return n
}

// ① 상한 넘김 + 진행 중 → 한가한 워커(v2)가 있어도 옮기지 않고, 직접 검증도 새로 시작하지 않고 더 기다린다(고치기 전: v2로 옮김).
// 일지에 넘김 한 줄(바퀴마다가 아니라 한 번)
func TestOverrunProgressWaits(t *testing.T) {
	s, d, m0 := frontFx(t, 700)
	s.specTerm("v1", d[1], 0)
	s.round(1)
	s.round(1)
	if len(s.lines(s.landlog)) != 0 || s.mainSHA() != m0 || len(s.dispatched()) != 1 {
		t.Fatalf("① 진행 중이면 더 기다린다: 착지 %v 띄움 %v", s.lines(s.landlog), s.dispatched())
	}
	if n := jcount(jOverWait, "추측 상한 넘김 — 진행 중이라 더 기다림(11분째"); n != 1 {
		t.Fatalf("① 일지 「추측 상한 넘김 — 진행 중이라 더 기다림(n분째)」 한 줄: %d\n%s", n, jdump())
	}
	if rc := s.verifyOn(d); rc != 0 {
		t.Fatalf("일꾼 종료 %d", rc)
	}
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|p" {
		t.Fatalf("① 기다린 추측의 통과로 검증 없이 착지: %q", got)
	}
}

// ② 상한 넘김 + 멈춤(갱신 없음·터미널 없음) · 워커 없음(v2 바쁨) → 지금처럼 직접 검증, 까닭이 경우마다 다름 · 손잡이 0이면 진행을 보지 않음
func TestOverrunStalledVerifies(t *testing.T) {
	for _, c := range []struct {
		name string
		age  time.Duration // 0보다 작으면 터미널 없음
		env  string
		want string
	}{
		{"갱신 없음", 20 * time.Minute, "", "직접 검증(맨 앞 기다림 상한·갱신 없음)"},
		{"터미널 없음", -1, "", "직접 검증(맨 앞 기다림 상한·터미널 없음)"},
		{"손잡이 0", 0, "0", "직접 검증(맨 앞 기다림 상한)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HC_LANE_SPEC_FRESH", c.env)
			s, d, _ := frontFx(t, 700)
			s.idle("v2", verifyBusy)
			if c.age >= 0 {
				s.specTerm("v1", d[1], c.age)
			}
			s.round(1)
			if got := strings.Join(s.lines(s.landlog), " "); got != "verify|p" || len(s.dispatched()) != 1 {
				t.Fatalf("② 직접 검증: %q 띄움 %v", got, s.dispatched())
			}
			if w := s.jwhy("p", "착지"); !strings.HasSuffix(w, ", "+c.want) {
				t.Fatalf("② 까닭 %q, 바람 …%q", w, c.want)
			}
		})
	}
}

// ② 상한 넘김 + 멈춤 · 한가한 워커 있음 → 진행 확인 뒤 지금 길(workerRedo) 그대로 그 워커에 옮긴다 — 직접 검증 안 함
func TestOverrunStalledMovesToWorker(t *testing.T) {
	s, d, _ := frontFx(t, 700)
	s.specTerm("v1", d[1], 20*time.Minute)
	s.round(1)
	if got := s.dispatched(); len(s.lines(s.landlog)) != 0 || len(got) != 2 || got[1][0] != "v2" {
		t.Fatalf("② 멈춘 추측은 한가한 v2로: 착지 %v 띄움 %v", s.lines(s.landlog), got)
	}
	if n := jcount(jOverWait, ""); n != 0 {
		t.Fatalf("② 멈춘 추측은 「진행 중이라 더 기다림」이 아니다: %d", n)
	}
}

// ③ merger_spec_timeout을 넘기면 진행 중이어도 직접 검증(까닭 「추측 시간 넘김」 — 워커 없음)
func TestOverrunSpecTimeoutVerifies(t *testing.T) {
	s, d, _ := frontFx(t, 4000)
	s.idle("v2", verifyBusy)
	s.specTerm("v1", d[1], 0)
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|p" {
		t.Fatalf("③ 시간 넘김은 직접 검증: %q", got)
	}
	if w := s.jwhy("p", "착지"); !strings.HasSuffix(w, ", 직접 검증(추측 시간 넘김)") {
		t.Fatalf("③ 까닭 %q", w)
	}
}

// ④ 착지 스크립트의 가장 좁은 검증 줄(「기준: 추측 통과 트리 … 이후 N개」) — 0개면 건너뜀, 아니면 바뀐 파일 수(고치기 전: 둘 다 모름)
func TestVtagLandScriptNarrow(t *testing.T) {
	d := t.TempDir()
	m := &M{vw: vwhy{why: "통과", id: "0123abcd", mach: "v1"}, lgf: filepath.Join(d, "l.log")}
	for _, c := range []struct{ log, want string }{
		{"▸ 검증\n  기준: 추측 통과 트리 0123456789ab 이후 0개\n  건너뜀 unit\n", ", 검증 건너뜀(추측 0123abcd · v1 · 착지 스크립트 0개)"},
		{"  기준: 추측 통과 트리 0123456789ab 이후 3개\n  실행 unit\n", ", 직접 검증(추측 통과 뒤 바뀐 파일 3개)"},
	} {
		os.WriteFile(m.lgf, []byte(c.log), 0o666)
		if got := m.vtag(); got != c.want {
			t.Errorf("%q → %q, 바람 %q", c.log, got, c.want)
		}
	}
}
