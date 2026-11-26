//go:build !windows

package main

// ht-merger-withdraw: 지휘가 거둔 제출(hc-fill withdraw — 큐 참조·기록을 반송 참조로 한 번의 원자 push)이 든 추측·착지의 결과는 쓰지 않는다.
// 남은 구성원을 그 실패로 반송·줄이지 않고, 다음 바퀴에 남은 줄로 다시 짓는다. 틀은 specrun_test.go(specFx — 가짜 착지 hc-lane land).

import (
	"os"
	"slices"
	"strings"
	"testing"

	"herdr-tools/internal/mergerstate"
)

// wdMove: hc-fill withdraw가 원격에 하는 일 — 큐 표식·기록을 같은 이름의 반송 참조로(lease)
func (s *sfx) wdMove(name string) {
	s.t.Helper()
	q, qm := "refs/merge-queue/"+name, "refs/merge-queue-meta/"+name
	sha := s.git(s.seed, "--git-dir="+s.origin, "rev-parse", q)
	rec := s.git(s.seed, "--git-dir="+s.origin, "rev-parse", qm)
	s.git(s.seed, "push", "-q", "--atomic", "origin", sha+":refs/merge-failed/"+name, rec+":refs/merge-failed-meta/"+name, ":"+q, ":"+qm,
		"--force-with-lease="+q+":"+sha, "--force-with-lease="+qm+":"+rec)
}

// wdSh: withdraw와 같은 일을 하는 셸 한 줄(가짜 착지 안에서 — 착지 실행 중에 거둠)
func (s *sfx) wdSh(name string) string {
	g := "git --git-dir=" + s.origin
	return "(cd " + s.seed + " && sha=$(" + g + " rev-parse refs/merge-queue/" + name + ") && rec=$(" + g + " rev-parse refs/merge-queue-meta/" + name +
		") && git push -q --atomic origin $sha:refs/merge-failed/" + name + " $rec:refs/merge-failed-meta/" + name +
		" :refs/merge-queue/" + name + " :refs/merge-queue-meta/" + name + ")"
}

// onLand: 가짜 착지 첫머리(착지 중 표식을 올린 뒤 — 검증·push 전)에 한 번 돌릴 셸(착지 실행 중에 지휘가 거둠)
func (s *sfx) onLand(script string) {
	s.t.Helper()
	old, _ := os.ReadFile(gSelf)
	at := "  git fetch -q origin || exit 1\n"
	hook := "  if [ ! -e \"$LANDLOG.hooked\" ]; then : >\"$LANDLOG.hooked\"; " + script + "; fi\n" + at
	os.WriteFile(gSelf, []byte(strings.Replace(string(old), at, hook, 1)), 0o777)
}

// leaseOnly: 그 레인 일지에 「lease 바뀜」이 있고 착지·돌려보냄·묶음 실패가 없다
func (s *sfx) leaseOnly(lane string) bool {
	got := s.jresults(lane)
	for _, r := range got {
		if r == "착지" || r == "돌려보냄" || r == jBatchFail {
			return false
		}
	}
	return slices.Contains(got, jLease)
}

func (s *sfx) jresults(lane string) []string {
	b, _ := os.ReadFile(gEvdir + "/merger-journal.tsv")
	var o []string
	for _, l := range strings.Split(string(b), "\n") {
		if c := strings.Split(l, "\t"); len(c) == 12 && c[2] == lane {
			o = append(o, c[9])
		}
	}
	return o
}

// ④ 맨 앞 혼자를 병합기가 직접 검증하는 중에 거둠 — 검증이 통과해도 실패해도 착지·반송 사건이 없다(이중 착지·이중 반송 없음),
// 일지는 「lease 바뀜」(후보를 버림)
func TestWithdrawDuringDirectLand(t *testing.T) {
	for _, bad := range []bool{false, true} {
		t.Run(map[bool]string{false: "통과", true: "실패"}[bad], func(t *testing.T) {
			oldFront(t)
			s := specFx(t)
			s.sub(1000000001, "a", bad)
			s.onLand(s.wdSh("1000000001-a"))
			s.round(1)
			if s.onMain("a.txt") {
				t.Fatal("거둔 제출이 착지했다")
			}
			if ev := s.evs(); len(ev) != 0 {
				t.Fatalf("병합기가 사건을 냈다(반송은 거둔 쪽이 이미 알렸다): %q", ev)
			}
			if !s.leaseOnly("a") {
				t.Fatalf("일지: %q, 바람 「%s」(착지·돌려보냄·묶음 실패 없음)", s.jresults("a"), jLease)
			}
			if !s.has("refs/merge-failed/1000000001-a") || s.has("refs/merge-queue/1000000001-a") {
				t.Fatal("거둔 흔적(반송 참조)이 그대로여야")
			}
		})
	}
}

// 반송하려던 찰나(같은 바퀴 — 추측 반송·미리 훑기)에 지휘가 먼저 거둠: 서버가 반송 push를 거절했으니 「응답만 끊김」이 아니다 —
// 반송 사건·일지 줄을 또 쓰지 않는다(거둔 쪽이 이미 알렸다)
func TestWithdrawBeatsMergerFail(t *testing.T) {
	s := specFx(t)
	q, sha, rec, _ := s.submit(1000000001, "a", "보통")
	s.git(s.clone, "fetch", "-q", "origin", "+refs/merge-queue/*:refs/hc-merger/q/*", "+refs/merge-queue-meta/*:refs/hc-merger/meta/*")
	s.wdMove("1000000001-a")
	m := s.merger()
	if !m.fail(q, sha, "a", rec, "-", "착지 실패(종료 1)") {
		t.Fatal("반송을 다음 바퀴로 미뤘다")
	}
	if ev := s.evs(); len(ev) != 0 {
		t.Fatalf("거둔 제출에 반송 사건을 또 냄: %q", ev)
	}
	for _, r := range s.jresults("a") {
		if r == "돌려보냄" {
			t.Fatal("일지에 돌려보냄")
		}
	}
}

// 작업 중 발견해 함께 고침(지휘 t3yx, 2026-11-27 — 레인·지휘가 다시 넣는 길을 몰라 원격 lane 가지를 손으로 지우려 함): 검증 실패 반송 사건
// 글 끝에 다시 넣는 길(거짓 실패면 requeue, 진짜면 고쳐 다시 제출)
func TestBounceEventRequeueHint(t *testing.T) {
	oldFront(t)
	s := specFx(t)
	s.sub(1000000001, "a", true)
	s.round(1)
	ev := s.evs()
	if len(ev) != 1 || !strings.Contains(ev[0], "병합기: 착지 실패(종료 1)") ||
		!strings.Contains(ev[0], " — 거짓 실패(부하·흔들림)면 주인 지휘가 hc-fill requeue a --why <까닭>, 진짜 실패면 고쳐 다시 제출|") {
		t.Fatalf("검증 실패 반송 글에 다시 넣는 길: %q", ev)
	}
}

// 상태 한 장의 fails: 지금 줄에 선 제출(큐 이름)마다 그 제출이 든 「묶음 실패」 바퀴 수 — 같은 레인의 옛 제출·다른 결과는 세지 않는다
func TestStateRecFails(t *testing.T) {
	s := specFx(t)
	j := func(n, lane, res string) string {
		return strings.Join([]string{"t", n, lane, "-", "-", "-", "2", "-", "-", res, "-", "x"}, "\t") + "\n"
	}
	os.WriteFile(gEvdir+"/merger-journal.tsv", []byte(j("1000000001-a", "a", jBatchFail)+j("1000000001-a", "a", jBatchFail)+
		j("1000000001-a", "a", jBatchFail)+j("1000000002-b", "b", jBatchFail)+j("1000000002-b", "b", jLease)+j("0999999999-a", "a", jBatchFail)), 0o666)
	m := s.merger()
	m.stQueue([][2]string{{"refs/merge-queue/1000000001-a", "x"}, {"refs/merge-queue/1000000002-b", "y"}, {"refs/merge-queue/1000000003-c", "z"}})
	m.stPhase(mergerstate.Round, "x", nil, false, nil)
	if r := recOf(t, s); len(r.Fails) != 2 || r.Fails["a"] != 3 || r.Fails["b"] != 1 {
		t.Fatalf("fails: %v, 바람 a 3·b 1(c 없음)", r.Fails)
	}
}

// ③ 묶음 착지(직접 검증) 중에 구성원 b를 거둠 + 검증 실패 — 남은 a를 그 실패로 줄이지(cap) 않고 다음 바퀴에 남은 줄로 다시 → a 착지
func TestWithdrawBatchMemberDuringLand(t *testing.T) {
	oldFront(t)
	s := specFx(t)
	s.sub(1000000001, "a", false)
	s.sub(1000000002, "b", true)
	s.onLand(s.wdSh("1000000002-b"))
	s.round(2)
	if _, err := os.Stat(s.merger().mj + "/cap"); err == nil {
		t.Fatal("거둔 구성원이 든 묶음 실패로 묶음을 줄였다(cap)")
	}
	for _, l := range []string{"a", "b"} {
		if !s.leaseOnly(l) {
			t.Fatalf("%s 일지: %q, 바람 「%s」(착지·돌려보냄·묶음 실패 없음)", l, s.jresults(l), jLease)
		}
	}
	if ev := s.evs(); len(ev) != 0 {
		t.Fatalf("사건: %q", ev)
	}
	s.round(2)
	if !s.onMain("a.txt") || s.onMain("bad.txt") || s.has("refs/merge-failed/1000000001-a") {
		t.Fatal("남은 a는 다음 바퀴에 착지, 거둔 b는 안 들어감")
	}
}

// ③ 묶음 추측(검증 기계)이 도는 중에 구성원 b를 거둠 → 그 추측이 시험 실패로 끝나도 a를 반송·줄이지 않고, a 혼자로 다시 짓는다
func TestWithdrawBatchMemberDuringSpec(t *testing.T) {
	s := specFx(t)
	s.sub(1000000001, "a", false)
	s.sub(1000000002, "b", true)
	s.round(2)
	d := s.dispatched()
	if len(d) != 1 {
		t.Fatalf("묶음 a+b 추측 하나: %v", d)
	}
	s.wdMove("1000000002-b")
	if rc := s.verifyOn(d[0]); rc != 0 {
		t.Fatalf("일꾼 종료 %d", rc)
	}
	s.round(2)
	if _, err := os.Stat(s.merger().mj + "/cap"); err == nil || s.has("refs/merge-failed/1000000001-a") {
		t.Fatalf("거둔 b가 든 추측 실패로 a를 줄이거나 반송했다(cap 있음 %v)", err == nil)
	}
	for _, r := range s.jresults("a") {
		if r == jBatchFail || r == "돌려보냄" {
			t.Fatalf("a 일지에 %s", r)
		}
	}
	d2 := s.dispatched()
	if len(d2) != 2 || d2[1][1] == d[0][1] {
		t.Fatalf("a 혼자 추측을 새로 띄워야: %v", d2)
	}
	if rc := s.verifyOn(d2[1]); rc != 0 {
		t.Fatalf("a 일꾼 종료 %d", rc)
	}
	s.round(2)
	if !s.onMain("a.txt") || s.onMain("bad.txt") {
		t.Fatal("a는 자기 추측으로 착지, b는 안 들어감")
	}
}
