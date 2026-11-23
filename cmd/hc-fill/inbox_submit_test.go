package main

import (
	"os"
	"strings"
	"testing"
)

// isEv: 레인 l-x(터미널 t1, 실행 t1@1000)의 사건 한 줄(12칸)
func isEv(tm, mach, st, sha, msg, key string) string {
	return strings.Join([]string{tm, mach, "t1", "l-x", st, sha, msg, "1", "-", "t1@1000", "-", key}, "\t")
}

// 받은편지 회신의 병합기 결과 기다림은 「진짜」 제출(hc-lane land가 적은 제출 ID)만 본다 — 레인이 손으로 적은
// `hc-lane report 제출 감사 완료 …`의 첫 낱말 「감사」를 제출 ID로 읽어 영영 기다리던 일(2026-11-21~23)
func TestInboxMergeWaitsOnlyForRealSubmit(t *testing.T) {
	setData(t)
	if err := os.WriteFile(DATA+"/try.tsv", []byte("l-x\t-\t-\t-\t-\tt1\tt1@1000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oMGM, oEV := MGM, EV
	t.Cleanup(func() { MGM, EV = oMGM, oEV })
	MGM = " mg"
	sid := "box-20261121T010203-4242"
	real := isEv("2026-11-21T01:02:03Z", "box", "제출", "abcdef1234567890", sid+" → refs/merge-queue/1-l-x (병합기 mg 가 main 에 넣는다)", "submit:"+sid)
	hand := isEv("2026-11-21T01:03:00Z", "box", "제출", "abcdef1234567890", "감사 완료(낮음 4) → 병합기에 넘김", "-")
	land := isEv("2026-11-21T01:10:00Z", "mg", "착지", "0123456789abcdef0123", "병합기: 착지 — 제출 ID "+sid+", 묶음 1", "-")
	old := strings.Join(strings.Split(real, "\t")[:11], "\t") // 12칸(키) 없는 옛 제출
	for _, c := range []struct {
		name  string
		ev    []string
		mline string
		wait  bool
	}{
		{"진짜 제출 → 손 제출 + 그 ID의 병합기 착지: 회신(끝 줄에 그 착지)", []string{real, hand, land}, "착지 0123456789ab (병합기 mg, 제출 ID " + sid + ")", false},
		{"손 제출뿐: 기다리지 않고 회신", []string{hand}, "", false},
		{"정상 제출, 병합기 결과 아직: 미룸", []string{real}, "", true},
		{"정상 제출 + 착지(대조)", []string{real, land}, "착지 0123456789ab (병합기 mg, 제출 ID " + sid + ")", false},
		{"12칸 없는 옛 제출(첫 낱말이 ID 꼴) + 착지", []string{old, land}, "착지 0123456789ab (병합기 mg, 제출 ID " + sid + ")", false},
		{"12칸 없는 옛 제출, 결과 아직: 미룸", []string{old}, "", true},
	} {
		EV = c.ev
		mline, wait := inbMerge("l-x", "t1")
		if mline != c.mline || wait != c.wait {
			t.Errorf("%s: inbMerge = (%q, %v), 바람 (%q, %v)", c.name, mline, wait, c.mline, c.wait)
		}
	}
}
