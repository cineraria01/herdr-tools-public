//go:build !windows

package main

// ht-worker-cordon(docs/hc-fill/cordon.md): 워커 쪽 저장소 읽기 점검(--spec-idle --repo-read)과 병합기 거르기 — hc-fill이 원격 손잡이
// 참조에 실은 기한 있는 「cordon@<기계>」 기계는 추측·범인 가리기·미리 검증·나눠 검증(specPick)·착지 실행(landPick)으로 고르지 않는다.
// 기한이 지났거나 줄이 없으면 모름 = 빼지 않는다. 사람의 pause@는 원격에 안 실린다(병합 검증과 상관없음 — hc-fill 시험).

import (
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/knob"
	"herdr-tools/internal/verified"
)

// 읽기 점검: 좋음 · 폴더 없음(막힘 아님) · 읽기 오류 · 상한 안에 안 끝남(막힌 읽기 흉내 — 2026-11-27 고장은 21초 막힘) · 끝 줄.
// 막힌 폴더가 있어도 상한 안에 끝난다
func TestRepoRead(t *testing.T) {
	d := t.TempDir()
	ok := d + "/with space"
	os.MkdirAll(ok+"/.git", 0o755)
	os.WriteFile(ok+"/README", []byte("x"), 0o644)
	noperm := d + "/noperm"
	os.MkdirAll(noperm, 0o000)
	t.Cleanup(func() { os.Chmod(noperm, 0o755) })
	hang := d + "/hang"
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	oldR, oldM := repoReadOne, repoReadMax
	repoReadOne = func(p string) string {
		if p == hang {
			<-release
		}
		return repoReadDir(p)
	}
	repoReadMax = 300 * time.Millisecond
	t.Cleanup(func() { repoReadOne, repoReadMax = oldR, oldM })
	t0 := time.Now()
	got := repoRead([]string{ok, d + "/gone", noperm, hang})
	if el := time.Since(t0); el > time.Second {
		t.Fatalf("막힌 폴더가 있어도 상한 안에: %s", el)
	}
	want := []string{"repo-read 1 ok", "repo-read 2 missing", "repo-read 3 bad open " + noperm + ": permission denied",
		"repo-read 4 bad 읽기가 300ms 안에 안 끝남", "repo-read end 4"}
	if os.Geteuid() == 0 {
		want[2] = "repo-read 3 ok"
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("점검 줄:\n%s\n바라는 것:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// --spec-idle은 지금 꼴 그대로(인자 없음 — 병합기 고르기), --repo-read면 한가함 줄 뒤에 점검 줄, 다른 인자는 거절(옛 판과 같은 종료 2)
func TestSpecIdleRepoRead(t *testing.T) {
	old := idleProbe
	idleProbe = func() (idleInfo, bool) { return idleInfo{load: 0.5, cpus: 8, gatesKnown: true}, true }
	t.Cleanup(func() { idleProbe = old })
	if rc := specIdleMain([]string{"--bogus"}); rc != 2 {
		t.Fatalf("모르는 인자: %d", rc)
	}
	if rc := specIdleMain([]string{"--repo-read", t.TempDir()}); rc != 0 {
		t.Fatalf("점검: %d", rc)
	}
	if rc := specIdleMain(nil); rc != 0 {
		t.Fatalf("지금 꼴: %d", rc)
	}
}

// cordonKnob: hc-fill이 올리는 꼴 그대로 원격 손잡이 참조에 cordon@<기계>(기한 until)
func (s *sfx) cordonKnob(mach string, until time.Time) {
	s.t.Helper()
	s.pushKnobs(map[string]knob.Entry{knob.CordonPre + mach: {Value: "1", Until: until.Unix()}})
}

// ⑧ 추측 검증: 격리된 v1(목록 첫 기계·한가함)을 건너 v2에 · 기한 지난 줄은 모름 = v1 그대로
func TestCordonSkipsSpecMachine(t *testing.T) {
	t.Cleanup(func() { rk = nil })
	s := specFx(t)
	s.cordonKnob("v1", time.Now().Add(15*time.Minute))
	s.sub(1000000001, "a", false)
	s.round(1)
	if d := s.dispatched(); len(d) != 1 || d[0][0] != "v2" {
		t.Fatalf("⑧ 격리된 v1을 건너 v2에: %v", d)
	}
	s2 := specFx(t)
	s2.cordonKnob("v1", time.Now().Add(-time.Minute)) // hc-fill이 한참 안 돎 — 낡은 기록
	s2.sub(1000000001, "a", false)
	s2.round(1)
	if d := s2.dispatched(); len(d) != 1 || d[0][0] != "v1" {
		t.Fatalf("⑧ 낡은 기록은 빼지 않음: %v", d)
	}
}

// ⑧ 범인 가리기·미리 검증·나눠 검증이 함께 쓰는 specPick: 격리된 기계는 한가해도 안 고름, 모두 격리면 빈 값
func TestCordonSpecPick(t *testing.T) {
	t.Cleanup(func() { rk = nil })
	m := &M{mme: "box"}
	c := &specCtx{vers: []verified.Verifier{{Machine: "v1", Path: "/c1"}, {Machine: "v2", Path: "/c2"}},
		idle: map[string]int{"v1": idIdle, "v2": idIdle}}
	rk = knob.Remote{knob.CordonPre + "v1": "1"}
	if v := m.specPick(c, nil, nil, "", true); v.Machine != "v2" {
		t.Fatalf("격리된 v1 건너뜀: %v", v)
	}
	rk = knob.Remote{knob.CordonPre + "v1": "1", knob.CordonPre + "v2": "1"}
	if v := m.specPick(c, nil, nil, "", true); v.Machine != "" {
		t.Fatalf("모두 격리면 고르지 않음: %v", v)
	}
	rk = knob.Remote{"pause@v1": "1"} // 사람의 멈춤은 병합기가 모른다(원격에 안 실림 — 실려도 안 읽음)
	if v := m.specPick(c, nil, nil, "", true); v.Machine != "v1" {
		t.Fatalf("pause@만 걸린 기계는 그대로: %v", v)
	}
}

// ⑧ 착지 실행: 격리된 v1을 건너 v2에서 · 모두 격리면 병합기 기계(까닭에 격리)
func TestCordonSkipsLandOn(t *testing.T) {
	t.Cleanup(func() { rk = nil })
	s, runs := landFx(t)
	s.cordonKnob("v1", time.Now().Add(15*time.Minute))
	s.sub(1000000001, "a", false)
	s.round(1)
	if r := runs(); len(r) != 1 || r[0] != "v2" || !s.onMain("a.txt") {
		t.Fatalf("⑧ v2에서 착지: %v\n%s", r, jdump())
	}
	s2, runs2 := landFx(t)
	s2.pushKnobs(map[string]knob.Entry{knob.CordonPre + "v1": {Value: "1", Until: time.Now().Add(time.Hour).Unix()},
		knob.CordonPre + "v2": {Value: "1", Until: time.Now().Add(time.Hour).Unix()}})
	s2.sub(1000000001, "a", false)
	s2.round(1)
	if r := runs2(); len(r) != 0 || !s2.onMain("a.txt") {
		t.Fatalf("⑧ 모두 격리면 병합기 기계에서: %v", r)
	}
	if l := landOnRows(t); len(l) == 0 || !strings.Contains(l[len(l)-1], "v1 격리") || !strings.Contains(l[len(l)-1], "v2 격리") {
		t.Fatalf("⑧ 까닭에 격리: %q", l)
	}
}
