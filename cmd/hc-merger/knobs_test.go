package main

import (
	"os"
	"strconv"
	"testing"
	"time"

	"herdr-tools/internal/knob"
)

// pushKnobs: 지휘 기계의 hc-fill이 올리는 꼴 그대로(knob.FormatRemote) origin의 refs/herdr/knobs를 바꾼다. 빈 es면 참조를 지운다
func (f *fx) pushKnobs(es map[string]knob.Entry) {
	f.t.Helper()
	for n, e := range es {
		e.Name = n
		es[n] = e
	}
	msg := knob.FormatRemote(es, time.Now())
	if msg == "" {
		f.git(f.seed, "push", "-q", "origin", ":"+knob.Ref)
		return
	}
	et := f.git(f.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
	c := f.git(f.seed, "commit-tree", et, "-m", msg)
	f.git(f.seed, "push", "-q", "-f", "origin", c+":"+knob.Ref)
}

// round0: 상주가 새로 띄운 바퀴 일꾼처럼(묶음은 시작 때 값 3) 바퀴의 ls-remote 한 번과 손잡이 읽기
func (f *fx) round0() *M {
	f.t.Helper()
	m := f.merger()
	qs, rc := lsRemote("origin", "refs/heads/main", "refs/merge-queue/*", knob.Ref)
	if rc != 0 {
		f.t.Fatalf("ls-remote %d", rc)
	}
	m.qs = qs
	m.loadKnobs()
	return m
}

func kv(v string) knob.Entry { return knob.Entry{Value: v} }

// ① 도는 중에 손잡이를 바꾸면 다음 바퀴에 먹는다(재시작 없이 — 묶음·추측 상한·옛 파일 손잡이·빈 바퀴 쉼)
// ② 손잡이 없음 → 지금 기본 ③ 손잡이 > 일지 폴더 파일 > 환경 변수 > 기본 ④ 범위 밖·기한 지난 값은 안 먹는다
func TestKnobLiveNextRound(t *testing.T) {
	f := newFx(t)
	t.Cleanup(func() { rk = nil })
	t.Setenv("HC_LANE_SPEC_TIMEOUT", "900")
	t.Setenv("HC_LANE_SPEC_BUSY_MAX", "3")

	// ② 손잡이 없음: 시작 값·환경 변수·기본 그대로
	m := f.round0()
	if m.batch != 3 || specTimeout() != 900 || m.busyMax() != 3 || specFrontWait() != 600 || m.conflictMode() != "agent" || tickKnob(m.mj, 10) != 10 {
		t.Fatalf("손잡이 없음: batch %d timeout %d busyMax %d frontWait %d conflict %s tick %d",
			m.batch, specTimeout(), m.busyMax(), specFrontWait(), m.conflictMode(), tickKnob(m.mj, 10))
	}
	// 옛 길(일지 폴더 파일)은 환경 변수보다 앞선다 — 지금 그대로
	if err := os.WriteFile(m.mj+"/busy_max", []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if m.busyMax() != 1 {
		t.Fatalf("일지 폴더 파일 busy_max = %d", m.busyMax())
	}

	// ① 손잡이를 올리면 다음 바퀴(새 일꾼)에 먹는다 ③ 손잡이가 환경 변수·일지 폴더 파일보다 앞선다
	f.pushKnobs(map[string]knob.Entry{"merger_batch": kv("5"), "merger_spec_timeout": kv("120"), "merger_busy_max": kv("4"),
		"merger_conflict": kv("event"), "merger_tick": kv("2"), "merger_front_wait": kv("60")})
	m = f.round0()
	if m.batch != 5 || specTimeout() != 120 || m.busyMax() != 4 || m.conflictMode() != "event" || tickKnob(m.mj, 10) != 2 || specFrontWait() != 60 {
		t.Fatalf("손잡이 뒤: batch %d timeout %d busyMax %d conflict %s tick %d frontWait %d",
			m.batch, specTimeout(), m.busyMax(), m.conflictMode(), tickKnob(m.mj, 10), specFrontWait())
	}
	// 다시 바꾸면 그다음 바퀴에
	f.pushKnobs(map[string]knob.Entry{"merger_batch": kv("7")})
	m = f.round0()
	if m.batch != 7 || specTimeout() != 900 || m.busyMax() != 1 || m.conflictMode() != "agent" || tickKnob(m.mj, 10) != 10 {
		t.Fatalf("두 번째: batch %d timeout %d busyMax %d conflict %s tick %d", m.batch, specTimeout(), m.busyMax(), m.conflictMode(), tickKnob(m.mj, 10))
	}

	// ④ 범위 밖 값(손으로 올린 참조)과 기한 지난 값은 안 먹는다
	et := f.git(f.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
	c := f.git(f.seed, "commit-tree", et, "-m", "hc-knobs\n\nmerger_batch\t0\t-\nmerger_tick\t5\t1\nmerger_spec_timeout\t30\t-\n")
	f.git(f.seed, "push", "-q", "-f", "origin", c+":"+knob.Ref)
	m = f.round0()
	if m.batch != 3 || tickKnob(m.mj, 10) != 10 || specTimeout() != 900 {
		t.Fatalf("범위 밖·기한 지남: batch %d tick %d timeout %d", m.batch, tickKnob(m.mj, 10), specTimeout())
	}

	// ② 참조를 지우면 다음 바퀴에 지금 값으로
	f.pushKnobs(nil)
	m = f.round0()
	if m.batch != 3 || specTimeout() != 900 || tickKnob(m.mj, 10) != 10 || len(rk) != 0 {
		t.Fatalf("참조 지운 뒤: batch %d timeout %d tick %d rk %v", m.batch, specTimeout(), tickKnob(m.mj, 10), rk)
	}
}

// 빈 바퀴 쉼 손잡이는 사본의 기한을 쉴 때마다 다시 본다(바퀴 일꾼이 다시 읽기 전에 기한이 지나도 기본으로)
func TestTickKnobExpiry(t *testing.T) {
	d := t.TempDir()
	if tickKnob(d, 10) != 10 {
		t.Fatal("사본 없음")
	}
	past := time.Now().Unix() - 1
	os.WriteFile(knobCache(d), []byte("x\nhc-knobs\n\nmerger_tick\t3\t"+strconv.FormatInt(past, 10)+"\n"), 0o644)
	if tickKnob(d, 10) != 10 {
		t.Fatal("기한 지난 merger_tick을 먹었다")
	}
	os.WriteFile(knobCache(d), []byte("x\nhc-knobs\n\nmerger_tick\t3\t-\n"), 0o644)
	if tickKnob(d, 10) != 3 {
		t.Fatal("merger_tick 3을 안 먹었다")
	}
}
